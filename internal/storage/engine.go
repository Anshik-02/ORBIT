package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"orbit/internal/wal"
)

// Sentinel errors.
var (
	ErrClosed                  = errors.New("storage: engine is closed")
	ErrRaftIndexGap            = errors.New("storage: raft index gap detected")
	ErrRaftIndexOverflow       = errors.New("storage: raft index overflow")
	ErrSnapshotIndexRegression = errors.New("storage: snapshot index must be greater than or equal to last applied index")
	ErrCorruptEngineSnapshot   = errors.New("storage: engine snapshot is corrupted")
	ErrFlushFailed             = errors.New("storage: background flush failed")
)

// EngineSnapshotContainer represents a durable on-disk snapshot of the state machine.
type EngineSnapshotContainer struct {
	Version   uint32            `json:"version"`
	Index     uint64            `json:"index"`
	Term      uint64            `json:"term"`
	State     map[string]string `json:"state"`
	CreatedAt time.Time         `json:"created_at"`
	Checksum  uint32            `json:"checksum"`
}

// CommittedCommand represents an entry that has reached majority consensus
// (e.g. via Raft in Section 2) and is ready to be committed to local storage.
//
// Architectural Decoupling:
//   - RaftIndex: Consensus log position across the distributed cluster.
//   - RaftTerm:  The election term when the leader proposed this entry.
//   - WAL Sequence: The physical local node's append-only sequence number.
//
// These concepts are strictly decoupled.
type CommittedCommand struct {
	RaftIndex uint64 // consensus replicated log index (0 for standalone mode)
	RaftTerm  uint64 // consensus term
	Operation string // wal.OperationSet or wal.OperationDelete
	Key       string
	Value     string
}

const (
	// DefaultMemtableLimit is the default memory threshold (4MB) before rotating to an immutable memtable.
	DefaultMemtableLimit int64 = 4 * 1024 * 1024
	// DefaultMaxImmutableMemtables is the maximum number of immutable memtables allowed before write backpressure.
	DefaultMaxImmutableMemtables = 4
)

// Engine is the core storage component of ORBIT.
//
// Invariants & Concurrency:
//   - Exclusive OS-level file locking prevents multiple processes on the same data.
//   - Mutations (ApplyCommitted, Set, Delete) serialize through `writeMu`.
//   - Memtable rotation triggers when active memtable reaches `memtableSizeLimit`.
//   - Background persistent worker flushes immutable memtables to SSTables without blocking new writes.
//   - Manifest atomically tracks all active SSTable records, levels, and flush watermarks.
//   - Obsolete WAL records are reclaimed safely after durable SSTable and Manifest installation.
//   - Read operations (Get) check active memtable, immutable memtables, and SSTables (newest to oldest).
//   - Tombstones prevent deleted keys from resurfacing in older SSTables.
//   - Streaming iterators and range scans operate with O(1) memory and deduplicate across tiers.
type Engine struct {
	memtable          *Memtable
	immutable         []*Memtable
	sstables          []*SSTableReader
	manifest          *Manifest
	compactor         *Compactor
	wal               *wal.Log
	dataDir           string
	sstDir            string
	snapPath          string
	writeMu           sync.Mutex   // serializes WAL append + Memtable update transaction
	flushMu           sync.Mutex   // serializes background and foreground flushing of immutable memtables
	flushCond         *sync.Cond   // signals when immutable queue decreases (backpressure release)
	flushCh           chan struct{} // notifies persistent worker of pending flush
	stopWorkerCh      chan struct{} // signals worker to terminate
	workerDoneCh      chan struct{} // closed when worker has exited
	lastFlushErr      error        // sticky flush error for health reporting
	closed            atomic.Bool  // tracks shutdown state
	closeErr          error        // sticky error from Close()
	lastAppliedIndex  uint64       // highest applied Raft index for idempotency
	memtableSizeLimit int64        // byte limit before memtable rotation
	maxImmutableCount int          // threshold for applying write backpressure
}

// NewEngine opens the WAL at walPath, loads or initializes the SSTable Manifest,
// purges crash-orphaned files, recovers durable snapshot and SSTable readers,
// streams subsequent WAL records, and launches the persistent background flush worker.
func NewEngine(walPath string) (*Engine, error) {
	w, err := wal.Open(walPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open wal: %w", err)
	}

	dataDir := filepath.Dir(walPath)
	sstDir := filepath.Join(dataDir, "sstables")
	if err := os.MkdirAll(sstDir, 0700); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("storage: create sstables dir: %w", err)
	}

	manifestPath := filepath.Join(dataDir, ManifestFileName)
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			manifest = NewManifest(manifestPath)
			if err := manifest.Save(); err != nil {
				_ = w.Close()
				return nil, fmt.Errorf("storage: init manifest: %w", err)
			}
		} else {
			_ = w.Close()
			return nil, fmt.Errorf("storage: load manifest: %w", err)
		}
	}

	engine := &Engine{
		memtable:          NewMemtable(),
		immutable:         make([]*Memtable, 0),
		sstables:          make([]*SSTableReader, 0),
		manifest:          manifest,
		wal:               w,
		dataDir:           dataDir,
		sstDir:            sstDir,
		snapPath:          filepath.Join(dataDir, "engine.snapshot"),
		memtableSizeLimit: DefaultMemtableLimit,
		maxImmutableCount: DefaultMaxImmutableMemtables,
		flushCh:           make(chan struct{}, 1),
		stopWorkerCh:      make(chan struct{}),
		workerDoneCh:      make(chan struct{}),
	}
	engine.flushCond = sync.NewCond(&engine.writeMu)
	engine.compactor = NewCompactor(engine, manifest, sstDir)

	// Reconstruct state machine from durable snapshot, SSTables, and subsequent WAL entries
	if err := engine.recover(); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("storage: recovery failed: %w", err)
	}

	// Start persistent background flush worker
	go engine.flushWorker()

	return engine, nil
}

// recover restores Memtable, SSTables, and lastAppliedIndex from disk,
// then streams subsequent WAL records.
func (e *Engine) recover() error {
	// 0. Crash recovery for engine.snapshot.tmp
	tmpSnap := e.snapPath + ".tmp"
	if _, err := os.Stat(tmpSnap); err == nil {
		if _, err := os.Stat(e.snapPath); errors.Is(err, os.ErrNotExist) {
			_ = os.Rename(tmpSnap, e.snapPath)
		} else {
			_ = os.Remove(tmpSnap)
		}
	}

	// 1. Clean up stale/orphaned files not committed to Manifest
	if _, err := e.manifest.CleanOrphanFiles(e.sstDir); err != nil {
		return fmt.Errorf("storage: clean orphan sstables: %w", err)
	}

	// 2. Open readers for all SSTables registered in the manifest
	activeTables := e.manifest.AllTables()
	for _, rec := range activeTables {
		fullPath := filepath.Join(e.sstDir, rec.Filename)
		reader, err := OpenSSTableReader(fullPath)
		if err != nil {
			return fmt.Errorf("storage: open manifest sstable %s: %w", rec.Filename, err)
		}
		// Validate checksum recorded in manifest against reader metadata
		if rec.Checksum != 0 && reader.Metadata().Checksum != rec.Checksum {
			_ = reader.Close()
			return fmt.Errorf("storage: checksum mismatch in sstable %s: recorded %d, found %d",
				rec.Filename, rec.Checksum, reader.Metadata().Checksum)
		}
		e.sstables = append(e.sstables, reader)
	}
	e.sortSSTablesLocked()

	// 3. Recover engine.snapshot if present
	if data, err := os.ReadFile(e.snapPath); err == nil && len(data) > 0 {
		var container EngineSnapshotContainer
		if err := json.Unmarshal(data, &container); err != nil {
			return fmt.Errorf("%w: %v", ErrCorruptEngineSnapshot, err)
		}

		stateBytes, err := json.Marshal(container.State)
		if err != nil {
			return fmt.Errorf("storage: marshal snapshot state for checksum: %w", err)
		}
		computed := crc32.ChecksumIEEE(stateBytes)
		if computed != container.Checksum {
			return fmt.Errorf("%w: checksum mismatch (stored %d, computed %d)",
				ErrCorruptEngineSnapshot, container.Checksum, computed)
		}

		for k, v := range container.State {
			e.memtable.Set(k, v)
		}
		e.lastAppliedIndex = container.Index
	}

	lastFlushedWAL, _ := e.manifest.LastFlushedOffsets()
	if lastFlushedWAL > e.wal.Sequence() {
		e.wal.SetSequence(lastFlushedWAL)
	}

	// 4. Stream WAL records, skipping entries already incorporated into SSTables or snapshot
	return e.wal.Replay(func(r wal.Record) error {
		// Skip if already checkpointed in SSTable or prior to consensus snapshot
		if r.Sequence <= lastFlushedWAL || (r.RaftIndex > 0 && r.RaftIndex <= e.lastAppliedIndex) {
			return nil
		}

		switch r.Operation {
		case wal.OperationSet:
			e.memtable.SetWithSeq(r.Key, r.Value, r.Sequence, r.RaftIndex)
		case wal.OperationDelete:
			e.memtable.DeleteWithSeq(r.Key, r.Sequence, r.RaftIndex)
		default:
			return fmt.Errorf("storage: unexpected operation %q during recovery (seq %d)", r.Operation, r.Sequence)
		}

		// Restore highest applied Raft index from persisted consensus metadata
		if r.RaftIndex > e.lastAppliedIndex {
			e.lastAppliedIndex = r.RaftIndex
		}
		return nil
	})
}

// flushWorker runs as a persistent background goroutine draining immutable memtables.
func (e *Engine) flushWorker() {
	defer close(e.workerDoneCh)

	for {
		select {
		case <-e.stopWorkerCh:
			// Drain any remaining memtables before exit
			_ = e.drainImmutableMemtables()
			return
		case <-e.flushCh:
			if err := e.drainImmutableMemtables(); err != nil {
				e.writeMu.Lock()
				e.lastFlushErr = err
				e.writeMu.Unlock()
			}
		}
	}
}

// ApplyCommitted applies an entry that has reached quorum commit in the cluster.
//
// Invariants enforced:
//  1. Idempotency: duplicate commands (RaftIndex <= lastAppliedIndex) are safe no-ops.
//  2. Contiguity: incoming RaftIndex must strictly equal lastAppliedIndex + 1 (no gaps).
//  3. Durability: RaftIndex and RaftTerm are persisted in the WAL record.
//  4. Backpressure: stalls if immutable memtables exceed maxImmutableCount until flush catches up.
func (e *Engine) ApplyCommitted(cmd CommittedCommand) error {
	if e.closed.Load() {
		return ErrClosed
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return ErrClosed
	}

	// Backpressure: prevent unbounded memory consumption if disk flushing is stalled
	for len(e.immutable) >= e.maxImmutableCount && !e.closed.Load() {
		e.flushCond.Wait()
	}
	if e.closed.Load() {
		return ErrClosed
	}

	// Consensus index validation for Raft commands
	if cmd.RaftIndex != 0 {
		if e.lastAppliedIndex == math.MaxUint64 {
			return ErrRaftIndexOverflow
		}

		if cmd.RaftIndex <= e.lastAppliedIndex {
			return nil
		}

		expectedIndex := e.lastAppliedIndex + 1
		if cmd.RaftIndex != expectedIndex {
			return fmt.Errorf("%w: incoming %d, expected %d", ErrRaftIndexGap, cmd.RaftIndex, expectedIndex)
		}
	}

	var rec wal.Record
	switch cmd.Operation {
	case wal.OperationSet:
		rec = wal.Record{
			RaftIndex: cmd.RaftIndex,
			RaftTerm:  cmd.RaftTerm,
			Operation: wal.OperationSet,
			Key:       cmd.Key,
			Value:     cmd.Value,
		}
	case wal.OperationDelete:
		rec = wal.Record{
			RaftIndex: cmd.RaftIndex,
			RaftTerm:  cmd.RaftTerm,
			Operation: wal.OperationDelete,
			Key:       cmd.Key,
		}
	default:
		return fmt.Errorf("storage: unsupported committed operation %q", cmd.Operation)
	}

	// Persist to local WAL
	if err := e.wal.Append(rec); err != nil {
		return fmt.Errorf("storage: wal append committed (op %q, key %q, raft_idx %d): %w",
			cmd.Operation, cmd.Key, cmd.RaftIndex, err)
	}

	walSeq := e.wal.Sequence()

	// Update local Memtable with sequence metadata
	if cmd.Operation == wal.OperationSet {
		e.memtable.SetWithSeq(cmd.Key, cmd.Value, walSeq, cmd.RaftIndex)
	} else {
		e.memtable.DeleteWithSeq(cmd.Key, walSeq, cmd.RaftIndex)
	}

	if cmd.RaftIndex > 0 {
		e.lastAppliedIndex = cmd.RaftIndex
	}

	// Rotate active memtable to immutable if memory threshold exceeded
	if e.memtable.SizeBytes() >= e.memtableSizeLimit {
		e.rotateMemtableLocked()
	}

	return nil
}

// RotateMemtable moves the current active memtable into the immutable queue
// and creates a fresh active memtable, signaling the background flush worker.
func (e *Engine) RotateMemtable() {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	e.rotateMemtableLocked()
}

func (e *Engine) rotateMemtableLocked() {
	if e.memtable.Len() == 0 {
		return
	}
	e.immutable = append(e.immutable, e.memtable)
	e.memtable = NewMemtable()

	select {
	case e.flushCh <- struct{}{}:
	default:
	}
}

// Flush synchronously drains all immutable memtables to Level 0 SSTables and updates the Manifest.
func (e *Engine) Flush() error {
	return e.drainImmutableMemtables()
}

// drainImmutableMemtables performs the actual disk serialization and manifest updates.
func (e *Engine) drainImmutableMemtables() error {
	e.flushMu.Lock()
	defer e.flushMu.Unlock()

	for {
		e.writeMu.Lock()
		if len(e.immutable) == 0 {
			e.writeMu.Unlock()
			return nil
		}
		oldest := e.immutable[0]
		sstID := e.manifest.NextID()
		sstFilename := fmt.Sprintf("%06d.sst", sstID)
		sstPath := filepath.Join(e.sstDir, sstFilename)
		e.writeMu.Unlock()

		entries := oldest.EntriesSorted()
		if len(entries) > 0 {
			writer, err := NewSSTableWriter(sstPath, 0, defaultSparseInterval, DefaultBloomFPRate)
			if err != nil {
				return fmt.Errorf("storage flush: create sstable writer: %w", err)
			}
			for _, entry := range entries {
				if err := writer.Append(entry); err != nil {
					return fmt.Errorf("storage flush: append entry: %w", err)
				}
			}
			meta, err := writer.Finish()
			if err != nil {
				return fmt.Errorf("storage flush: finish sstable: %w", err)
			}
			meta.ID = sstID

			// Register in Manifest atomically before installing reader
			rec := SSTableRecord{
				ID:         sstID,
				Filename:   sstFilename,
				Level:      0,
				NumEntries: meta.NumEntries,
				MinKey:     meta.MinKey,
				MaxKey:     meta.MaxKey,
				SizeBytes:  meta.SizeBytes,
				Checksum:   meta.Checksum,
				CreatedAt:  meta.CreatedAt,
			}
			if err := e.manifest.AddSSTable(rec); err != nil {
				_ = os.Remove(sstPath)
				return fmt.Errorf("storage flush: register sstable in manifest: %w", err)
			}

			// Update durable WAL sequence watermark in manifest and reclaim obsolete WAL records
			walSeq, raftIdx := oldest.MaxOffsets()
			_ = e.manifest.UpdateFlushedOffsets(walSeq, raftIdx)
			if walSeq > 0 {
				_ = e.wal.TruncateBefore(walSeq)
			}

			reader, err := OpenSSTableReader(sstPath)
			if err != nil {
				return fmt.Errorf("storage flush: open sstable reader: %w", err)
			}

			e.writeMu.Lock()
			e.sstables = append(e.sstables, reader)
			e.sortSSTablesLocked()
			if len(e.immutable) > 0 && e.immutable[0] == oldest {
				e.immutable = e.immutable[1:]
			}
			e.flushCond.Broadcast() // wake stalled writers
			e.writeMu.Unlock()

			// Check Level 0 threshold to trigger background compaction if needed
			if len(e.manifest.LevelTables(0)) >= e.compactor.l0Threshold {
				_, _ = e.compactor.CompactLevel0ToLevel1()
			}
		} else {
			e.writeMu.Lock()
			if len(e.immutable) > 0 && e.immutable[0] == oldest {
				e.immutable = e.immutable[1:]
			}
			e.flushCond.Broadcast()
			e.writeMu.Unlock()
		}
	}
}

// Compact triggers Level 0 to Level 1 minor compaction.
func (e *Engine) Compact() (*CompactionResult, error) {
	return e.compactor.CompactLevel0ToLevel1()
}

// MajorCompact triggers major compaction across all SSTables.
func (e *Engine) MajorCompact() (*CompactionResult, error) {
	return e.compactor.MajorCompaction()
}

// Manifest returns the persistent SSTable manifest.
func (e *Engine) Manifest() *Manifest {
	return e.manifest
}

// SetMemtableSizeLimit configures the byte threshold that triggers memtable rotation.
func (e *Engine) SetMemtableSizeLimit(limit int64) {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	if limit > 0 {
		e.memtableSizeLimit = limit
	}
}

// SetMaxImmutableCount configures the threshold for backpressure on immutable memtables.
func (e *Engine) SetMaxImmutableCount(count int) {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	if count > 0 {
		e.maxImmutableCount = count
	}
}

// MemtableSizeBytes returns the active memtable's in-memory byte size.
func (e *Engine) MemtableSizeBytes() int64 {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return e.memtable.SizeBytes()
}

// ImmutableCount returns the number of immutable memtables pending flush.
func (e *Engine) ImmutableCount() int {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return len(e.immutable)
}

// SSTablesCount returns the number of active SSTables loaded in the engine.
func (e *Engine) SSTablesCount() int {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return len(e.sstables)
}

// SSTables returns a snapshot copy of current SSTable readers.
func (e *Engine) SSTables() []*SSTableReader {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	cp := make([]*SSTableReader, len(e.sstables))
	copy(cp, e.sstables)
	return cp
}

// LastAppliedIndex returns the highest committed consensus index applied to this engine.
func (e *Engine) LastAppliedIndex() uint64 {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return e.lastAppliedIndex
}

// SetLastAppliedIndex explicitly sets the applied Raft index baseline.
func (e *Engine) SetLastAppliedIndex(index uint64) error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return ErrClosed
	}
	if index < e.lastAppliedIndex {
		return fmt.Errorf("%w: new index %d < current index %d", ErrSnapshotIndexRegression, index, e.lastAppliedIndex)
	}
	e.lastAppliedIndex = index
	return nil
}

// SnapshotMetadata captures the consensus state of a snapshot.
type SnapshotMetadata struct {
	Index uint64 // highest Raft index included in the snapshot
	Term  uint64 // term of the highest Raft index
}

// InstallSnapshot atomically installs a snapshot into the storage engine.
func (e *Engine) InstallSnapshot(meta SnapshotMetadata, state map[string]string) error {
	if e.closed.Load() {
		return ErrClosed
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return ErrClosed
	}

	if meta.Index < e.lastAppliedIndex {
		return fmt.Errorf("%w: snapshot index %d < current applied index %d",
			ErrSnapshotIndexRegression, meta.Index, e.lastAppliedIndex)
	}

	// 1. Serialize and persist durable engine snapshot container
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("storage: marshal state for snapshot: %w", err)
	}
	container := EngineSnapshotContainer{
		Version:   1,
		Index:     meta.Index,
		Term:      meta.Term,
		State:     state,
		CreatedAt: time.Now(),
		Checksum:  crc32.ChecksumIEEE(stateBytes),
	}
	containerBytes, err := json.MarshalIndent(container, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: marshal snapshot container: %w", err)
	}
	if err := atomicWriteFileWithDirSync(e.snapPath, containerBytes, 0600); err != nil {
		return fmt.Errorf("storage: persist engine snapshot: %w", err)
	}

	// 2. Truncate/reset WAL to purge obsolete log records
	if err := e.wal.Reset(); err != nil {
		return fmt.Errorf("storage: reset wal after snapshot install: %w", err)
	}

	// 3. Purge existing SSTables and reset manifest
	for _, reader := range e.sstables {
		_ = reader.Close()
	}
	e.sstables = make([]*SSTableReader, 0)
	e.immutable = make([]*Memtable, 0)

	if sstEntries, err := os.ReadDir(e.sstDir); err == nil {
		for _, sstEntry := range sstEntries {
			_ = os.Remove(filepath.Join(e.sstDir, sstEntry.Name()))
		}
	}

	e.manifest = NewManifest(e.manifest.path)
	_ = e.manifest.Save()

	// 4. Atomically update in-memory state
	newMem := NewMemtable()
	for k, v := range state {
		newMem.Set(k, v)
	}

	e.memtable = newMem
	e.lastAppliedIndex = meta.Index
	return nil
}

// allLiveStateLocked constructs a point-in-time map of all live key-values
// by overlaying SSTables (oldest to newest), immutable memtables (oldest to newest),
// and the active memtable, accurately respecting tombstones.
func (e *Engine) allLiveStateLocked() (map[string]string, error) {
	state := make(map[string]string)

	for _, reader := range e.sstables {
		entries, err := reader.AllEntries()
		if err != nil {
			return nil, fmt.Errorf("storage: read sstable entries for snapshot: %w", err)
		}
		for _, entry := range entries {
			if entry.IsTombstone {
				delete(state, entry.Key)
			} else {
				state[entry.Key] = entry.Value
			}
		}
	}

	for _, imm := range e.immutable {
		for _, entry := range imm.EntriesSorted() {
			if entry.IsTombstone {
				delete(state, entry.Key)
			} else {
				state[entry.Key] = entry.Value
			}
		}
	}

	for _, entry := range e.memtable.EntriesSorted() {
		if entry.IsTombstone {
			delete(state, entry.Key)
		} else {
			state[entry.Key] = entry.Value
		}
	}

	return state, nil
}

// SaveSnapshot durably persists the current in-memory database state and applied index to disk.
func (e *Engine) SaveSnapshot(meta SnapshotMetadata) error {
	if e.closed.Load() {
		return ErrClosed
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return ErrClosed
	}

	state, err := e.allLiveStateLocked()
	if err != nil {
		return err
	}

	stateBytes, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("storage: marshal state for snapshot: %w", err)
	}
	container := EngineSnapshotContainer{
		Version:   1,
		Index:     meta.Index,
		Term:      meta.Term,
		State:     state,
		CreatedAt: time.Now(),
		Checksum:  crc32.ChecksumIEEE(stateBytes),
	}
	containerBytes, err := json.MarshalIndent(container, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: marshal snapshot container: %w", err)
	}
	return atomicWriteFileWithDirSync(e.snapPath, containerBytes, 0600)
}

// ExportSnapshot returns the current applied Raft index and a point-in-time copy of all live key-values.
func (e *Engine) ExportSnapshot() (uint64, map[string]string, error) {
	if e.closed.Load() {
		return 0, nil, ErrClosed
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return 0, nil, ErrClosed
	}

	state, err := e.allLiveStateLocked()
	if err != nil {
		return 0, nil, err
	}

	return e.lastAppliedIndex, state, nil
}

// ExportSnapshotBytes exports the current state machine data as serialized JSON bytes with its applied Raft index.
func (e *Engine) ExportSnapshotBytes() (uint64, []byte, error) {
	idx, state, err := e.ExportSnapshot()
	if err != nil {
		return 0, nil, err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return 0, nil, fmt.Errorf("storage: marshal snapshot state: %w", err)
	}

	container := EngineSnapshotContainer{
		Version:   1,
		Index:     idx,
		Term:      0,
		State:     state,
		CreatedAt: time.Now(),
		Checksum:  crc32.ChecksumIEEE(data),
	}
	if containerBytes, mErr := json.MarshalIndent(container, "", "  "); mErr == nil {
		_ = atomicWriteFileWithDirSync(e.snapPath, containerBytes, 0600)
	}

	return idx, data, nil
}

// RestoreSnapshotBytes unmarshals and atomically installs serialized snapshot data.
func (e *Engine) RestoreSnapshotBytes(meta SnapshotMetadata, data []byte) error {
	var state map[string]string
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("storage: unmarshal snapshot state: %w", err)
	}
	return e.InstallSnapshot(meta, state)
}

// Set stores key -> value in the database.
func (e *Engine) Set(key, value string) error {
	return e.ApplyCommitted(CommittedCommand{
		Operation: wal.OperationSet,
		Key:       key,
		Value:     value,
	})
}

// Get retrieves the value for key across the LSM hierarchy:
//  1. Active Memtable (with tombstone check)
//  2. Immutable Memtables from newest to oldest (with tombstone check)
//  3. SSTables from newest to oldest (with Bloom filter and tombstone check)
func (e *Engine) Get(key string) (string, bool) {
	if e.closed.Load() {
		return "", false
	}

	e.writeMu.Lock()
	// 1. Check active Memtable
	if entry, found := e.memtable.GetEntry(key); found {
		e.writeMu.Unlock()
		if entry.IsTombstone {
			return "", false
		}
		return entry.Value, true
	}

	// 2. Check immutable Memtables in reverse (newest to oldest)
	for i := len(e.immutable) - 1; i >= 0; i-- {
		if entry, found := e.immutable[i].GetEntry(key); found {
			e.writeMu.Unlock()
			if entry.IsTombstone {
				return "", false
			}
			return entry.Value, true
		}
	}

	// Snapshot list of SSTable readers so we can search without holding writeMu
	readers := make([]*SSTableReader, len(e.sstables))
	copy(readers, e.sstables)
	e.writeMu.Unlock()

	// 3. Check SSTables from newest to oldest
	for i := len(readers) - 1; i >= 0; i-- {
		val, found, isTomb, err := readers[i].Get(key)
		if err != nil {
			continue
		}
		if found {
			if isTomb {
				return "", false
			}
			return val, true
		}
	}

	return "", false
}

// Scan performs a range query for keys in [startKey, endKey) up to limit entries.
// Merges active memtable, immutable memtables, and all SSTables using streaming iterators.
func (e *Engine) Scan(startKey, endKey string, limit int) ([]KVPair, error) {
	if e.closed.Load() {
		return nil, ErrClosed
	}

	e.writeMu.Lock()
	// 1. Active memtable iterator
	activeIt := NewMemtableIterator(e.memtable, startKey, endKey)

	// 2. Immutable memtable iterators (newest to oldest)
	var immIterators []Iterator
	for i := len(e.immutable) - 1; i >= 0; i-- {
		immIterators = append(immIterators, NewMemtableIterator(e.immutable[i], startKey, endKey))
	}

	// 3. SSTable readers snapshot (newest to oldest)
	sstSnap := make([]*SSTableReader, len(e.sstables))
	copy(sstSnap, e.sstables)
	e.writeMu.Unlock()

	var sstIterators []Iterator
	for i := len(sstSnap) - 1; i >= 0; i-- {
		sstIterators = append(sstIterators, NewSSTableIterator(sstSnap[i], startKey, endKey))
	}

	// 4. Construct MergedIterator (children ordered newest to oldest)
	var allChildren []Iterator
	allChildren = append(allChildren, activeIt)
	allChildren = append(allChildren, immIterators...)
	allChildren = append(allChildren, sstIterators...)

	merged := NewMergedIterator(allChildren)
	defer merged.Close()

	var results []KVPair
	for merged.Valid() {
		if limit > 0 && len(results) >= limit {
			break
		}
		results = append(results, KVPair{
			Key:   merged.Key(),
			Value: merged.Value(),
		})
		merged.Next()
	}

	return results, nil
}

// PrefixScan retrieves all key-value pairs matching the prefix up to limit entries.
func (e *Engine) PrefixScan(prefix string, limit int) ([]KVPair, error) {
	if prefix == "" {
		return e.Scan("", "", limit)
	}

	endKey := prefixLimitKey(prefix)
	return e.Scan(prefix, endKey, limit)
}

func prefixLimitKey(prefix string) string {
	b := []byte(prefix)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < 0xFF {
			b[i]++
			return string(b[:i+1])
		}
	}
	return ""
}

// Delete removes key from the database.
func (e *Engine) Delete(key string) error {
	return e.ApplyCommitted(CommittedCommand{
		Operation: wal.OperationDelete,
		Key:       key,
	})
}

// Health checks whether the storage engine is open and its WAL and background flush are healthy.
func (e *Engine) Health() error {
	if e.closed.Load() {
		return ErrClosed
	}
	e.writeMu.Lock()
	flushErr := e.lastFlushErr
	e.writeMu.Unlock()
	if flushErr != nil {
		return fmt.Errorf("%w: %v", ErrFlushFailed, flushErr)
	}
	return e.wal.Health()
}

// MemtableStat summarizes an active or immutable in-memory memtable.
type MemtableStat struct {
	ID          int   `json:"id"`
	ByteSize    int64 `json:"byte_size"`
	EntryCount  int   `json:"entry_count"`
	IsImmutable bool  `json:"is_immutable"`
}

// SSTableStat describes an on-disk Sorted String Table.
type SSTableStat struct {
	ID             uint64 `json:"id"`
	Level          int    `json:"level"`
	FileName       string `json:"file_name"`
	FileSize       int64  `json:"file_size"`
	RecordCount    int    `json:"record_count"`
	MinKey         string `json:"min_key"`
	MaxKey         string `json:"max_key"`
	HasBloomFilter bool   `json:"has_bloom_filter"`
}

// EngineStats exposes structured LSM-Tree and WAL telemetry for observability endpoints.
type EngineStats struct {
	ActiveMemtable     MemtableStat   `json:"active_memtable"`
	ImmutableMemtables []MemtableStat `json:"immutable_memtables"`
	ImmutableCount     int            `json:"immutable_count"`
	WALSizeBytes       int64          `json:"wal_size_bytes"`
	WALRecordCount     int64          `json:"wal_record_count"`
	WALPath            string         `json:"wal_path"`
	SSTables           []SSTableStat  `json:"sstables"`
	SSTablesByLevel    map[int]int    `json:"sstables_by_level"`
	TotalSSTableBytes  int64          `json:"total_sstable_bytes"`
	Compaction         CompactionStat `json:"compaction"`
}

// Stats returns a thread-safe snapshot of active storage metrics.
func (e *Engine) Stats() EngineStats {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	active := MemtableStat{
		ID:          0,
		ByteSize:    0,
		EntryCount:  0,
		IsImmutable: false,
	}
	if e.memtable != nil {
		active.ByteSize = e.memtable.SizeBytes()
		active.EntryCount = e.memtable.Len()
	}

	var immutables []MemtableStat
	for i, imm := range e.immutable {
		if imm != nil {
			immutables = append(immutables, MemtableStat{
				ID:          i + 1,
				ByteSize:    imm.SizeBytes(),
				EntryCount:  imm.Len(),
				IsImmutable: true,
			})
		}
	}

	var sstables []SSTableStat
	byLevel := make(map[int]int)
	var totalSSTableBytes int64

	for _, reader := range e.sstables {
		if reader == nil {
			continue
		}
		meta := reader.Metadata()
		hasBloom := reader.bloomFilter != nil
		sstables = append(sstables, SSTableStat{
			ID:             meta.ID,
			Level:          int(meta.Level),
			FileName:       meta.Filename,
			FileSize:       meta.SizeBytes,
			RecordCount:    int(meta.NumEntries),
			MinKey:         meta.MinKey,
			MaxKey:         meta.MaxKey,
			HasBloomFilter: hasBloom,
		})
		byLevel[int(meta.Level)]++
		totalSSTableBytes += meta.SizeBytes
	}

	var walSize int64
	var walRecords int64
	var walPath string
	if e.wal != nil {
		walSize = e.wal.SizeBytes()
		walRecords = int64(e.wal.Sequence())
		walPath = e.wal.Path()
	}

	compaction := CompactionStat{}
	if e.compactor != nil {
		compaction = e.compactor.Stats()
	}

	return EngineStats{
		ActiveMemtable:     active,
		ImmutableMemtables: immutables,
		ImmutableCount:     len(immutables),
		WALSizeBytes:       walSize,
		WALRecordCount:     walRecords,
		WALPath:            walPath,
		SSTables:           sstables,
		SSTablesByLevel:    byLevel,
		TotalSSTableBytes:  totalSSTableBytes,
		Compaction:         compaction,
	}
}

// atomicWriteFileWithDirSync safely writes data using a deterministic .tmp file,
// fsync, atomic rename, and directory sync.
func atomicWriteFileWithDirSync(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(path)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if df, err := os.Open(dir); err == nil {
		_ = df.Sync()
		_ = df.Close()
	}

	return nil
}

// CreateBackup creates a point-in-time, isolated backup of the active engine.
// It synchronizes all in-flight flushes and holds write and flush locks so no compaction
// or SSTable deletion can occur concurrently, guaranteeing a 100% consistent snapshot.
func (e *Engine) CreateBackup(backupBaseDir string) (string, error) {
	if e.closed.Load() {
		return "", ErrClosed
	}

	// Drain any in-flight immutable memtables to SSTables first
	if err := e.drainImmutableMemtables(); err != nil {
		return "", fmt.Errorf("storage backup: drain memtables: %w", err)
	}

	e.flushMu.Lock()
	defer e.flushMu.Unlock()
	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	// Persist the manifest cleanly
	if err := e.manifest.saveLocked(); err != nil {
		return "", fmt.Errorf("storage backup: save manifest: %w", err)
	}

	// Now delegate to CreateBackup which will copy the exact manifest, active SSTables, and WAL safely
	return CreateBackup(e.dataDir, backupBaseDir)
}

// Close safely drains flushes, stops the background worker, closes all SSTables,
// persists the manifest, and releases WAL locks.
func (e *Engine) Close() error {
	e.writeMu.Lock()
	if e.closed.Load() {
		e.writeMu.Unlock()
		return e.closeErr
	}
	e.closed.Store(true)
	e.flushCond.Broadcast() // unblock any stalled writers
	e.writeMu.Unlock()

	// Stop background worker
	close(e.stopWorkerCh)
	<-e.workerDoneCh

	// Drain any remaining queued immutable memtables
	_ = e.drainImmutableMemtables()

	// Close all SSTable readers
	e.writeMu.Lock()
	for _, reader := range e.sstables {
		if err := reader.Close(); err != nil && e.closeErr == nil {
			e.closeErr = err
		}
	}
	e.sstables = nil
	e.immutable = nil

	if e.manifest != nil {
		if err := e.manifest.Save(); err != nil && e.closeErr == nil {
			e.closeErr = err
		}
	}
	e.writeMu.Unlock()

	if err := e.wal.Close(); err != nil {
		e.closeErr = fmt.Errorf("storage: close wal: %w", err)
		return e.closeErr
	}
	return e.closeErr
}

// sortSSTablesLocked orders sstables from oldest to newest:
// Higher levels (L2, L1 - older) come first, down to Level 0 (newest).
// Within the same level, lower IDs (older) come first, down to highest ID (newest).
// This guarantees that reverse iteration (len-1 down to 0) encounters newest data first.
func (e *Engine) sortSSTablesLocked() {
	sort.Slice(e.sstables, func(i, j int) bool {
		mi := e.sstables[i].Metadata()
		mj := e.sstables[j].Metadata()
		if mi.Level != mj.Level {
			return mi.Level > mj.Level
		}
		return mi.ID < mj.ID
	})
}
