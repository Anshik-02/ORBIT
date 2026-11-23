package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	ManifestVersion uint32 = 1
	ManifestFileName       = "MANIFEST"
)

var (
	ErrCorruptManifest = errors.New("storage: manifest is corrupted")
	ErrMissingSSTable  = errors.New("storage: manifest references missing sstable file")
)

// SSTableRecord is the persistent metadata descriptor for a single SSTable in the Manifest.
type SSTableRecord struct {
	ID         uint64    `json:"id"`
	Filename   string    `json:"filename"`
	Level      uint32    `json:"level"`
	NumEntries uint64    `json:"num_entries"`
	MinKey     string    `json:"min_key"`
	MaxKey     string    `json:"max_key"`
	SizeBytes  int64     `json:"size_bytes"`
	Checksum   uint32    `json:"checksum"`
	CreatedAt  time.Time `json:"created_at"`
}

// manifestContainer is the on-disk JSON structure protected by CRC32-IEEE.
type manifestContainer struct {
	Version             uint32                   `json:"version"`
	Sequence            uint64                   `json:"sequence"`
	NextSSTableID       uint64                   `json:"next_sstable_id"`
	LastFlushedWALSeq   uint64                   `json:"last_flushed_wal_seq"`
	LastFlushedRaftIdx  uint64                   `json:"last_flushed_raft_idx"`
	Levels              map[uint32][]SSTableRecord `json:"levels"` // Level -> list of SSTables
	CreatedAt           time.Time                `json:"created_at"`
	UpdatedAt           time.Time                `json:"updated_at"`
	Checksum            uint32                   `json:"checksum"`
}

// Manifest coordinates durable tracking of active SSTable versions, levels, and bounds.
type Manifest struct {
	mu                 sync.RWMutex
	path               string
	version            uint32
	sequence           uint64
	nextSSTableID      uint64
	lastFlushedWALSeq  uint64
	lastFlushedRaftIdx uint64
	levels             map[uint32][]SSTableRecord
	createdAt          time.Time
	updatedAt          time.Time
}

// NewManifest initializes a fresh in-memory Manifest.
func NewManifest(path string) *Manifest {
	now := time.Now()
	return &Manifest{
		path:          path,
		version:       ManifestVersion,
		sequence:      1,
		nextSSTableID: 1,
		levels:        make(map[uint32][]SSTableRecord),
		createdAt:     now,
		updatedAt:     now,
	}
}

// LoadManifest reads, validates checksums, and constructs a Manifest from disk.
func LoadManifest(manifestPath string) (*Manifest, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty manifest file", ErrCorruptManifest)
	}

	var container manifestContainer
	if err := json.Unmarshal(data, &container); err != nil {
		return nil, fmt.Errorf("%w: parse manifest json: %v", ErrCorruptManifest, err)
	}

	if container.Version != ManifestVersion {
		return nil, fmt.Errorf("%w: unsupported manifest version %d", ErrCorruptManifest, container.Version)
	}

	// Verify checksum over container contents without the Checksum field
	storedChecksum := container.Checksum
	container.Checksum = 0
	serializedNoChecksum, err := json.Marshal(container)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal for checksum validation: %v", ErrCorruptManifest, err)
	}
	computedChecksum := crc32.ChecksumIEEE(serializedNoChecksum)
	if storedChecksum != computedChecksum {
		return nil, fmt.Errorf("%w: checksum mismatch (stored %d, computed %d)",
			ErrCorruptManifest, storedChecksum, computedChecksum)
	}

	if container.Levels == nil {
		container.Levels = make(map[uint32][]SSTableRecord)
	}

	return &Manifest{
		path:               manifestPath,
		version:            container.Version,
		sequence:           container.Sequence,
		nextSSTableID:      container.NextSSTableID,
		lastFlushedWALSeq:  container.LastFlushedWALSeq,
		lastFlushedRaftIdx: container.LastFlushedRaftIdx,
		levels:             container.Levels,
		createdAt:          container.CreatedAt,
		updatedAt:          container.UpdatedAt,
	}, nil
}

// Save atomically writes the Manifest to disk using a temporary file, fsync, and atomic rename.
func (m *Manifest) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.saveLocked()
}

func (m *Manifest) saveLocked() error {
	m.sequence++
	m.updatedAt = time.Now()

	container := manifestContainer{
		Version:            m.version,
		Sequence:           m.sequence,
		NextSSTableID:      m.nextSSTableID,
		LastFlushedWALSeq:  m.lastFlushedWALSeq,
		LastFlushedRaftIdx: m.lastFlushedRaftIdx,
		Levels:             m.levels,
		CreatedAt:          m.createdAt,
		UpdatedAt:          m.updatedAt,
		Checksum:           0,
	}

	rawBytes, err := json.Marshal(container)
	if err != nil {
		return fmt.Errorf("manifest: marshal container: %w", err)
	}

	container.Checksum = crc32.ChecksumIEEE(rawBytes)
	finalBytes, err := json.MarshalIndent(container, "", "  ")
	if err != nil {
		return fmt.Errorf("manifest: marshal indent: %w", err)
	}

	return atomicWriteFileWithDirSync(m.path, finalBytes, 0600)
}

// AddSSTable records a new SSTable in the manifest at the given level.
func (m *Manifest) AddSSTable(rec SSTableRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.levels == nil {
		m.levels = make(map[uint32][]SSTableRecord)
	}

	m.levels[rec.Level] = append(m.levels[rec.Level], rec)
	if rec.ID >= m.nextSSTableID {
		m.nextSSTableID = rec.ID + 1
	}

	return m.saveLocked()
}

// ReplaceSSTables atomically removes obsolete SSTables (e.g. compacted inputs)
// and adds new SSTables (e.g. compacted outputs) within a single atomic manifest commit.
func (m *Manifest) ReplaceSSTables(toRemove []uint64, toAdd []SSTableRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	removeMap := make(map[uint64]bool, len(toRemove))
	for _, id := range toRemove {
		removeMap[id] = true
	}

	// Filter out removed tables across all levels
	for level, tables := range m.levels {
		kept := make([]SSTableRecord, 0, len(tables))
		for _, t := range tables {
			if !removeMap[t.ID] {
				kept = append(kept, t)
			}
		}
		m.levels[level] = kept
	}

	// Add new tables
	for _, rec := range toAdd {
		m.levels[rec.Level] = append(m.levels[rec.Level], rec)
		if rec.ID >= m.nextSSTableID {
			m.nextSSTableID = rec.ID + 1
		}
	}

	return m.saveLocked()
}

// UpdateFlushedOffsets sets the WAL and Raft sequence numbers corresponding to durable flushes.
func (m *Manifest) UpdateFlushedOffsets(walSeq uint64, raftIdx uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if walSeq > m.lastFlushedWALSeq {
		m.lastFlushedWALSeq = walSeq
	}
	if raftIdx > m.lastFlushedRaftIdx {
		m.lastFlushedRaftIdx = raftIdx
	}

	return m.saveLocked()
}

// AllTables returns all active SSTables sorted by Level ascending, then ID ascending.
func (m *Manifest) AllTables() []SSTableRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []SSTableRecord
	for _, tables := range m.levels {
		all = append(all, tables...)
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].Level != all[j].Level {
			return all[i].Level > all[j].Level // Higher level (older) comes before lower level (newer)
		}
		return all[i].ID < all[j].ID // Lower ID (older) comes before higher ID (newer)
	})

	return all
}

// LevelTables returns all active SSTables in a specific level.
func (m *Manifest) LevelTables(level uint32) []SSTableRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tables := m.levels[level]
	copied := make([]SSTableRecord, len(tables))
	copy(copied, tables)
	return copied
}

// NextID returns and advances the monotonic SSTable ID counter.
func (m *Manifest) NextID() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextSSTableID
	m.nextSSTableID++
	_ = m.saveLocked()
	return id
}

// PeekNextID returns the next SSTable ID without incrementing.
func (m *Manifest) PeekNextID() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nextSSTableID
}

// LastFlushedOffsets returns the recorded last flushed WAL sequence and Raft index.
func (m *Manifest) LastFlushedOffsets() (walSeq uint64, raftIdx uint64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastFlushedWALSeq, m.lastFlushedRaftIdx
}

// CleanOrphanFiles scans the sstDir on disk and removes any .sst files that are not
// registered in this Manifest (such as files created by a crash mid-flush or mid-compaction).
// Returns the list of deleted orphan filenames.
func (m *Manifest) CleanOrphanFiles(sstDir string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeFiles := make(map[string]bool)
	for _, tables := range m.levels {
		for _, t := range tables {
			activeFiles[t.Filename] = true
		}
	}

	dirEntries, err := os.ReadDir(sstDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sstable dir: %w", err)
	}

	var orphans []string
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// If temporary file, clean it up immediately
		if filepath.Ext(name) == ".tmp" {
			_ = os.Remove(filepath.Join(sstDir, name))
			orphans = append(orphans, name)
			continue
		}
		if filepath.Ext(name) == ".sst" && !activeFiles[name] {
			_ = os.Remove(filepath.Join(sstDir, name))
			orphans = append(orphans, name)
		}
	}

	return orphans, nil
}
