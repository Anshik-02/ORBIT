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
)

// Engine is the core storage component of ORBIT.
//
// Invariants & Concurrency:
//   - Exclusive OS-level file locking prevents multiple processes on the same data.
//   - Mutations (ApplyCommitted, Set, Delete) serialize through `writeMu`.
//   - Memtable rotation triggers when active memtable reaches `memtableSizeLimit`.
//   - Background flushing converts immutable memtables into SSTables without blocking new writes.
//   - Read operations (Get) check active memtable, immutable memtables, and SSTables (newest to oldest).
//   - Tombstones prevent deleted keys from resurfacing in older SSTables.
//   - Single-pass streaming WAL replay on boot reconstructs state AND restores lastAppliedIndex.
//   - Durable snapshots in engine.snapshot survive node restarts without WAL dependency.
type Engine struct {
	memtable          *Memtable
	immutable         []*Memtable
	sstables          []*SSTableReader
	wal               *wal.Log
	dataDir           string
	sstDir            string
	snapPath          string
	writeMu           sync.Mutex  // serializes WAL append + Memtable update transaction
	flushMu           sync.Mutex  // serializes background and foreground flushing of immutable memtables
	closed            atomic.Bool // tracks shutdown state
	closeErr          error       // sticky error from Close()
	lastAppliedIndex  uint64      // highest applied Raft index for idempotency
	memtableSizeLimit int64       // byte limit before memtable rotation
	nextSSTableID     uint64      // monotonic sequence counter for SSTable filenames
}

// NewEngine opens the WAL at walPath, recovers durable snapshot state if present,
// executes a single streaming replay pass to reconstruct the Memtable and consensus metadata,
// and returns a ready-to-use Engine.
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

	engine := &Engine{
		memtable:          NewMemtable(),
		immutable:         make([]*Memtable, 0),
		sstables:          make([]*SSTableReader, 0),
		wal:               w,
		dataDir:           dataDir,
		sstDir:            sstDir,
		snapPath:          filepath.Join(dataDir, "engine.snapshot"),
		memtableSizeLimit: DefaultMemtableLimit,
		nextSSTableID:     1,
	}

	// Reconstruct state machine from durable snapshot, SSTables, and subsequent WAL entries
	if err := engine.recover(); err != nil {
		w.Close()
		return nil, fmt.Errorf("storage: recovery failed: %w", err)
	}

	return engine, nil
}

// recover restores Memtable, SSTables, and lastAppliedIndex from disk,
// then streams subsequent WAL records.
func (e *Engine) recover() error {
	// 0. Crash recovery for engine.snapshot.tmp and cleanup incomplete .sst.tmp files
	tmpSnap := e.snapPath + ".tmp"
	if _, err := os.Stat(tmpSnap); err == nil {
		if _, err := os.Stat(e.snapPath); errors.Is(err, os.ErrNotExist) {
			_ = os.Rename(tmpSnap, e.snapPath)
		} else {
			_ = os.Remove(tmpSnap)
		}
	}

	// Clean up stale temporary sstable files from crashes
	if entries, err := os.ReadDir(e.sstDir); err == nil {
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".tmp" {
				_ = os.Remove(filepath.Join(e.sstDir, entry.Name()))
			}
		}
	}

	// 1. Recover existing SSTables in numeric sequence order
	if entries, err := os.ReadDir(e.sstDir); err == nil {
		var sstFiles []string
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sst" {
				sstFiles = append(sstFiles, entry.Name())
			}
		}
		sort.Strings(sstFiles)

		for _, fname := range sstFiles {
			fullPath := filepath.Join(e.sstDir, fname)
			reader, err := OpenSSTableReader(fullPath)
			if err != nil {
				return fmt.Errorf("storage: open sstable %s: %w", fname, err)
			}
			e.sstables = append(e.sstables, reader)

			var fileID uint64
			if _, sErr := fmt.Sscanf(fname, "%06d.sst", &fileID); sErr == nil {
				if fileID >= e.nextSSTableID {
					e.nextSSTableID = fileID + 1
				}
			}
		}
	}

	// 2. Recover engine.snapshot if present
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

	// 3. Stream WAL records, skipping entries already incorporated into the snapshot
	return e.wal.Replay(func(r wal.Record) error {
		if r.RaftIndex > 0 && r.RaftIndex <= e.lastAppliedIndex {
			return nil
		}

		switch r.Operation {
		case wal.OperationSet:
			e.memtable.Set(r.Key, r.Value)
		case wal.OperationDelete:
			e.memtable.Delete(r.Key)
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

// ApplyCommitted applies an entry that has reached quorum commit in the cluster.
//
// Invariants enforced:
//  1. Idempotency: duplicate commands (RaftIndex <= lastAppliedIndex) are safe no-ops.
//  2. Contiguity: incoming RaftIndex must strictly equal lastAppliedIndex + 1 (no gaps).
//  3. Durability: RaftIndex and RaftTerm are persisted in the WAL record.
func (e *Engine) ApplyCommitted(cmd CommittedCommand) error {
	if e.closed.Load() {
		return ErrClosed
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return ErrClosed
	}

	// Consensus index validation for Raft commands
	if cmd.RaftIndex != 0 {
		// Overflow check: prevent lastAppliedIndex + 1 overflow
		if e.lastAppliedIndex == math.MaxUint64 {
			return ErrRaftIndexOverflow
		}

		// Idempotency: ignore duplicate or already-committed commands
		if cmd.RaftIndex <= e.lastAppliedIndex {
			return nil
		}

		// Contiguity check: incoming index must be strictly contiguous
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

	// Persist to local WAL (persists consensus metadata with checksum)
	if err := e.wal.Append(rec); err != nil {
		return fmt.Errorf("storage: wal append committed (op %q, key %q, raft_idx %d): %w",
			cmd.Operation, cmd.Key, cmd.RaftIndex, err)
	}

	// Update local Memtable
	if cmd.Operation == wal.OperationSet {
		e.memtable.Set(cmd.Key, cmd.Value)
	} else {
		e.memtable.Delete(cmd.Key)
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
// and creates a fresh active memtable, triggering background flushing.
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
	go func() {
		_ = e.Flush()
	}()
}

// Flush synchronously drains all immutable memtables to SSTables on disk.
func (e *Engine) Flush() error {
	e.flushMu.Lock()
	defer e.flushMu.Unlock()

	for {
		if e.closed.Load() {
			return ErrClosed
		}

		e.writeMu.Lock()
		if len(e.immutable) == 0 {
			e.writeMu.Unlock()
			return nil
		}
		oldest := e.immutable[0]
		sstID := e.nextSSTableID
		e.nextSSTableID++
		sstPath := filepath.Join(e.sstDir, fmt.Sprintf("%06d.sst", sstID))
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
			if _, err := writer.Finish(); err != nil {
				return fmt.Errorf("storage flush: finish sstable: %w", err)
			}

			reader, err := OpenSSTableReader(sstPath)
			if err != nil {
				return fmt.Errorf("storage flush: open sstable reader: %w", err)
			}

			e.writeMu.Lock()
			e.sstables = append(e.sstables, reader)
			if len(e.immutable) > 0 && e.immutable[0] == oldest {
				e.immutable = e.immutable[1:]
			}
			e.writeMu.Unlock()
		} else {
			e.writeMu.Lock()
			if len(e.immutable) > 0 && e.immutable[0] == oldest {
				e.immutable = e.immutable[1:]
			}
			e.writeMu.Unlock()
		}
	}
}

// SetMemtableSizeLimit configures the byte threshold that triggers memtable rotation.
func (e *Engine) SetMemtableSizeLimit(limit int64) {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	if limit > 0 {
		e.memtableSizeLimit = limit
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

// SetLastAppliedIndex explicitly sets the applied Raft index baseline, e.g. when restoring
// from a consensus snapshot in Section 2.
// Rejects regressions where index < lastAppliedIndex to prevent state inconsistencies.
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

// InstallSnapshot atomically installs a snapshot into the storage engine:
//  1. Validates that the engine is open.
//  2. Enforces monotonicity: rejects regressions where meta.Index < e.lastAppliedIndex.
//  3. Durably persists the snapshot container to engine.snapshot on disk.
//  4. Truncates and resets the WAL to discard obsolete records.
//  5. Atomically replaces the in-memory Memtable with the snapshot state.
//  6. Advances lastAppliedIndex to meta.Index.
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

	// 3. Purge existing SSTables since snapshot replaces authoritative state
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

	// 1. Overlay SSTables (oldest to newest)
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

	// 2. Overlay immutable memtables (oldest to newest)
	for _, imm := range e.immutable {
		for _, entry := range imm.EntriesSorted() {
			if entry.IsTombstone {
				delete(state, entry.Key)
			} else {
				state[entry.Key] = entry.Value
			}
		}
	}

	// 3. Overlay active memtable
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
// Also durably checkpoints engine.snapshot on disk.
func (e *Engine) ExportSnapshotBytes() (uint64, []byte, error) {
	idx, state, err := e.ExportSnapshot()
	if err != nil {
		return 0, nil, err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return 0, nil, fmt.Errorf("storage: marshal snapshot state: %w", err)
	}

	// Persist engine snapshot durably on disk as a checkpoint
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

// Set stores key -> value in the database.
// In standalone mode, this routes through ApplyCommitted with RaftIndex=0.
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
//
// Returns ("", false) if closed, absent, or marked with a tombstone.
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

// Delete removes key from the database.
// In standalone mode, this routes through ApplyCommitted with RaftIndex=0.
func (e *Engine) Delete(key string) error {
	return e.ApplyCommitted(CommittedCommand{
		Operation: wal.OperationDelete,
		Key:       key,
	})
}

// Health checks whether the storage engine is open and its WAL is unpoisoned.
func (e *Engine) Health() error {
	if e.closed.Load() {
		return ErrClosed
	}
	return e.wal.Health()
}

// Close safely flushes the WAL, closes all SSTables, and releases resources.
// Retains sticky error: if close fails, subsequent invocations return the same error.
func (e *Engine) Close() error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return e.closeErr
	}
	e.closed.Store(true)

	// Close all SSTable readers
	for _, reader := range e.sstables {
		if err := reader.Close(); err != nil && e.closeErr == nil {
			e.closeErr = err
		}
	}
	e.sstables = nil
	e.immutable = nil

	if err := e.wal.Close(); err != nil {
		e.closeErr = fmt.Errorf("storage: close wal: %w", err)
		return e.closeErr
	}
	return e.closeErr
}
