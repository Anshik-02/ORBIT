package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	// DefaultL0CompactionThreshold is the number of Level 0 SSTables that triggers compaction to Level 1.
	DefaultL0CompactionThreshold = 4
	// DefaultTargetSSTableSize is the target maximum size of a compacted SSTable (2MB).
	DefaultTargetSSTableSize int64 = 2 * 1024 * 1024
)

// CompactionResult summarizes the outcome of a compaction operation.
type CompactionResult struct {
	InputTables  []uint64          `json:"input_tables"`
	OutputTables []SSTableMetadata `json:"output_tables"`
	KeysRead     int               `json:"keys_read"`
	KeysWritten  int               `json:"keys_written"`
	TombstonesDropped int          `json:"tombstones_dropped"`
	Duration     time.Duration     `json:"duration"`
}

// Compactor coordinates Level-based and Major compactions across SSTables.
type Compactor struct {
	mu            sync.Mutex
	engine        *Engine
	manifest      *Manifest
	sstDir        string
	l0Threshold   int
	targetSSTSize int64
}

// NewCompactor creates a Compactor instance attached to engine and manifest.
func NewCompactor(engine *Engine, manifest *Manifest, sstDir string) *Compactor {
	return &Compactor{
		engine:        engine,
		manifest:      manifest,
		sstDir:        sstDir,
		l0Threshold:   DefaultL0CompactionThreshold,
		targetSSTSize: DefaultTargetSSTableSize,
	}
}

// SetL0Threshold configures the Level 0 table count threshold for triggering compaction.
func (c *Compactor) SetL0Threshold(threshold int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if threshold > 1 {
		c.l0Threshold = threshold
	}
}

// CompactLevel0ToLevel1 executes minor/level compaction: merges Level 0 tables into Level 1.
func (c *Compactor) CompactLevel0ToLevel1() (*CompactionResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	startTime := time.Now()

	// 1. Inspect manifest for Level 0 tables
	l0Records := c.manifest.LevelTables(0)
	if len(l0Records) < c.l0Threshold {
		return nil, nil // threshold not reached
	}

	// Also inspect Level 1 tables that might overlap in key range
	l1Records := c.manifest.LevelTables(1)

	// Collect input IDs
	var inputIDs []uint64
	for _, r := range l0Records {
		inputIDs = append(inputIDs, r.ID)
	}
	for _, r := range l1Records {
		inputIDs = append(inputIDs, r.ID)
	}

	// 2. Open readers for all input tables in creation order (oldest to newest)
	// Sort input tables by Level (L1 first, then L0), then by ID ascending
	allInputs := append([]SSTableRecord{}, l1Records...)
	allInputs = append(allInputs, l0Records...)
	sort.Slice(allInputs, func(i, j int) bool {
		if allInputs[i].Level != allInputs[j].Level {
			return allInputs[i].Level > allInputs[j].Level // older level (L1) comes before newer level (L0)
		}
		return allInputs[i].ID < allInputs[j].ID
	})

	var inputReaders []*SSTableReader
	for _, rec := range allInputs {
		fullPath := filepath.Join(c.sstDir, rec.Filename)
		reader, err := OpenSSTableReader(fullPath)
		if err != nil {
			// Close any already opened
			for _, r := range inputReaders {
				_ = r.Close()
			}
			return nil, fmt.Errorf("compactor: open input sstable %s: %w", rec.Filename, err)
		}
		inputReaders = append(inputReaders, reader)
	}
	defer func() {
		for _, r := range inputReaders {
			_ = r.Close()
		}
	}()

	// 3. Multi-way merge: newer inputs override older inputs
	mergedMap := make(map[string]MemtableEntry)
	keysRead := 0

	for _, reader := range inputReaders {
		entries, err := reader.AllEntries()
		if err != nil {
			return nil, fmt.Errorf("compactor: read entries from %s: %w", reader.Metadata().Filename, err)
		}
		keysRead += len(entries)
		for _, entry := range entries {
			mergedMap[entry.Key] = entry
		}
	}

	// Check if higher levels (L2+) exist. If not, tombstones can be dropped!
	hasHigherLevels := false
	for lvl := uint32(2); lvl < 10; lvl++ {
		if len(c.manifest.LevelTables(lvl)) > 0 {
			hasHigherLevels = true
			break
		}
	}

	// 4. Sort merged keys and filter obsolete tombstones
	sortedKeys := make([]string, 0, len(mergedMap))
	for k := range mergedMap {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	var liveEntries []MemtableEntry
	tombstonesDropped := 0

	for _, k := range sortedKeys {
		entry := mergedMap[k]
		if entry.IsTombstone && !hasHigherLevels {
			// Safely drop tombstone since no older data exists below Level 1
			tombstonesDropped++
			continue
		}
		liveEntries = append(liveEntries, entry)
	}

	// 5. Write merged entries to Level 1 output SSTable(s)
	var outputMetas []SSTableMetadata
	var outputRecords []SSTableRecord

	if len(liveEntries) > 0 {
		outID := c.manifest.NextID()
		outFilename := fmt.Sprintf("%06d.sst", outID)
		outPath := filepath.Join(c.sstDir, outFilename)

		writer, err := NewSSTableWriter(outPath, 1, defaultSparseInterval, DefaultBloomFPRate)
		if err != nil {
			return nil, fmt.Errorf("compactor: create output writer: %w", err)
		}

		for _, entry := range liveEntries {
			if err := writer.Append(entry); err != nil {
				return nil, fmt.Errorf("compactor: append to output sstable: %w", err)
			}
		}

		meta, err := writer.Finish()
		if err != nil {
			return nil, fmt.Errorf("compactor: finish output sstable: %w", err)
		}
		meta.ID = outID
		outputMetas = append(outputMetas, *meta)

		outputRecords = append(outputRecords, SSTableRecord{
			ID:         outID,
			Filename:   outFilename,
			Level:      1,
			NumEntries: meta.NumEntries,
			MinKey:     meta.MinKey,
			MaxKey:     meta.MaxKey,
			SizeBytes:  meta.SizeBytes,
			Checksum:   meta.Checksum,
			CreatedAt:  meta.CreatedAt,
		})
	}

	// 6. Atomically update Manifest: replace input tables with output tables
	if err := c.manifest.ReplaceSSTables(inputIDs, outputRecords); err != nil {
		// Clean up output files on failure
		for _, out := range outputMetas {
			_ = os.Remove(out.Path)
		}
		return nil, fmt.Errorf("compactor: manifest replace tables: %w", err)
	}

	// 7. Update Engine's active SSTable readers atomically
	if c.engine != nil {
		var newReaders []*SSTableReader
		for _, out := range outputMetas {
			r, err := OpenSSTableReader(out.Path)
			if err != nil {
				return nil, fmt.Errorf("compactor: open output reader: %w", err)
			}
			newReaders = append(newReaders, r)
		}

		c.engine.writeMu.Lock()
		inputIDMap := make(map[uint64]bool, len(inputIDs))
		for _, id := range inputIDs {
			inputIDMap[id] = true
		}

		var remainingReaders []*SSTableReader
		var toCloseReaders []*SSTableReader
		for _, r := range c.engine.sstables {
			if inputIDMap[r.Metadata().ID] {
				toCloseReaders = append(toCloseReaders, r)
			} else {
				remainingReaders = append(remainingReaders, r)
			}
		}

		// Replace engine SSTables list
		c.engine.sstables = append(remainingReaders, newReaders...)
		c.engine.writeMu.Unlock()

		// Close old readers outside writeMu
		for _, r := range toCloseReaders {
			_ = r.Close()
		}
	}

	// 8. Safely delete obsolete input files from disk now that manifest is updated
	for _, rec := range allInputs {
		fullPath := filepath.Join(c.sstDir, rec.Filename)
		_ = os.Remove(fullPath)
	}

	return &CompactionResult{
		InputTables:       inputIDs,
		OutputTables:      outputMetas,
		KeysRead:          keysRead,
		KeysWritten:       len(liveEntries),
		TombstonesDropped: tombstonesDropped,
		Duration:          time.Since(startTime),
	}, nil
}

// MajorCompaction merges all SSTables across all levels into a single clean set of Level 1 SSTables.
func (c *Compactor) MajorCompaction() (*CompactionResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	startTime := time.Now()
	allTables := c.manifest.AllTables()
	if len(allTables) == 0 {
		return nil, nil
	}

	var inputIDs []uint64
	for _, r := range allTables {
		inputIDs = append(inputIDs, r.ID)
	}

	// Open readers for all input tables (oldest level first, then ID ascending)
	sort.Slice(allTables, func(i, j int) bool {
		if allTables[i].Level != allTables[j].Level {
			return allTables[i].Level > allTables[j].Level
		}
		return allTables[i].ID < allTables[j].ID
	})

	var inputReaders []*SSTableReader
	for _, rec := range allTables {
		fullPath := filepath.Join(c.sstDir, rec.Filename)
		reader, err := OpenSSTableReader(fullPath)
		if err != nil {
			for _, r := range inputReaders {
				_ = r.Close()
			}
			return nil, fmt.Errorf("major compactor: open sstable %s: %w", rec.Filename, err)
		}
		inputReaders = append(inputReaders, reader)
	}
	defer func() {
		for _, r := range inputReaders {
			_ = r.Close()
		}
	}()

	// Merge all entries
	mergedMap := make(map[string]MemtableEntry)
	keysRead := 0

	for _, reader := range inputReaders {
		entries, err := reader.AllEntries()
		if err != nil {
			return nil, fmt.Errorf("major compactor: read entries from %s: %w", reader.Metadata().Filename, err)
		}
		keysRead += len(entries)
		for _, entry := range entries {
			mergedMap[entry.Key] = entry
		}
	}

	// In MajorCompaction, ALL levels are compacted, so all tombstones can be safely discarded
	sortedKeys := make([]string, 0, len(mergedMap))
	for k := range mergedMap {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	var liveEntries []MemtableEntry
	tombstonesDropped := 0

	for _, k := range sortedKeys {
		entry := mergedMap[k]
		if entry.IsTombstone {
			tombstonesDropped++
			continue
		}
		liveEntries = append(liveEntries, entry)
	}

	// Write new SSTable
	var outputMetas []SSTableMetadata
	var outputRecords []SSTableRecord

	if len(liveEntries) > 0 {
		outID := c.manifest.NextID()
		outFilename := fmt.Sprintf("%06d.sst", outID)
		outPath := filepath.Join(c.sstDir, outFilename)

		writer, err := NewSSTableWriter(outPath, 1, defaultSparseInterval, DefaultBloomFPRate)
		if err != nil {
			return nil, fmt.Errorf("major compactor: create output writer: %w", err)
		}

		for _, entry := range liveEntries {
			if err := writer.Append(entry); err != nil {
				return nil, fmt.Errorf("major compactor: append entry: %w", err)
			}
		}

		meta, err := writer.Finish()
		if err != nil {
			return nil, fmt.Errorf("major compactor: finish output: %w", err)
		}
		meta.ID = outID
		outputMetas = append(outputMetas, *meta)

		outputRecords = append(outputRecords, SSTableRecord{
			ID:         outID,
			Filename:   outFilename,
			Level:      1,
			NumEntries: meta.NumEntries,
			MinKey:     meta.MinKey,
			MaxKey:     meta.MaxKey,
			SizeBytes:  meta.SizeBytes,
			Checksum:   meta.Checksum,
			CreatedAt:  meta.CreatedAt,
		})
	}

	// Atomically replace tables in manifest
	if err := c.manifest.ReplaceSSTables(inputIDs, outputRecords); err != nil {
		for _, out := range outputMetas {
			_ = os.Remove(out.Path)
		}
		return nil, fmt.Errorf("major compactor: manifest replace: %w", err)
	}

	// Update Engine readers
	if c.engine != nil {
		var newReaders []*SSTableReader
		for _, out := range outputMetas {
			r, err := OpenSSTableReader(out.Path)
			if err != nil {
				return nil, fmt.Errorf("major compactor: open output reader: %w", err)
			}
			newReaders = append(newReaders, r)
		}

		c.engine.writeMu.Lock()
		inputIDMap := make(map[uint64]bool, len(inputIDs))
		for _, id := range inputIDs {
			inputIDMap[id] = true
		}

		var remainingReaders []*SSTableReader
		var toCloseReaders []*SSTableReader
		for _, r := range c.engine.sstables {
			if inputIDMap[r.Metadata().ID] {
				toCloseReaders = append(toCloseReaders, r)
			} else {
				remainingReaders = append(remainingReaders, r)
			}
		}

		c.engine.sstables = append(remainingReaders, newReaders...)
		c.engine.writeMu.Unlock()

		for _, r := range toCloseReaders {
			_ = r.Close()
		}
	}

	// Delete old files from disk
	for _, rec := range allTables {
		fullPath := filepath.Join(c.sstDir, rec.Filename)
		_ = os.Remove(fullPath)
	}

	return &CompactionResult{
		InputTables:       inputIDs,
		OutputTables:      outputMetas,
		KeysRead:          keysRead,
		KeysWritten:       len(liveEntries),
		TombstonesDropped: tombstonesDropped,
		Duration:          time.Since(startTime),
	}, nil
}
