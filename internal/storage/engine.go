package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"orbit/internal/wal"
)

// Sentinel errors.
var (
	ErrClosed                  = errors.New("storage: engine is closed")
	ErrRaftIndexGap            = errors.New("storage: raft index gap detected")
	ErrRaftIndexOverflow       = errors.New("storage: raft index overflow")
	ErrSnapshotIndexRegression = errors.New("storage: snapshot index must be greater than or equal to last applied index")
)

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

// Engine is the core storage component of ORBIT.
//
// Invariants & Concurrency:
//   - Exclusive OS-level file locking prevents multiple processes on the same data.
//   - Mutations (ApplyCommitted, Set, Delete) serialize through `writeMu`.
//   - Read operations (Get) are non-blocking and concurrent via Memtable RLock.
//   - Single-pass streaming WAL replay on boot reconstructs Memtable AND restores lastAppliedIndex.
//   - Enforces contiguous sequential application for consensus commands (no gaps allowed).
type Engine struct {
	memtable         *Memtable
	wal              *wal.Log
	writeMu          sync.Mutex  // serializes WAL append + Memtable update transaction
	closed           atomic.Bool // tracks shutdown state
	closeErr         error       // sticky error from Close()
	lastAppliedIndex uint64      // highest applied Raft index for idempotency
}

// NewEngine opens the WAL at walPath, executes a single streaming replay pass
// to reconstruct the Memtable and consensus metadata, and returns a ready-to-use Engine.
func NewEngine(walPath string) (*Engine, error) {
	w, err := wal.Open(walPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open wal: %w", err)
	}

	engine := &Engine{
		memtable: NewMemtable(),
		wal:      w,
	}

	// Single streaming replay pass: populates Memtable and recovers consensus progress
	if err := engine.recover(); err != nil {
		w.Close()
		return nil, fmt.Errorf("storage: recovery failed: %w", err)
	}

	return engine, nil
}

// recover streams all WAL records directly into the Memtable and restores lastAppliedIndex.
// Memory overhead is strictly O(1) regardless of WAL size.
func (e *Engine) recover() error {
	return e.wal.Replay(func(r wal.Record) error {
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

	return nil
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
//  3. Atomically replaces the in-memory Memtable with the snapshot state.
//  4. Advances lastAppliedIndex to meta.Index.
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

	newMem := NewMemtable()
	for k, v := range state {
		newMem.Set(k, v)
	}

	e.memtable = newMem
	e.lastAppliedIndex = meta.Index
	return nil
}

// ExportSnapshot returns the current applied Raft index and a point-in-time copy of memtable state.
func (e *Engine) ExportSnapshot() (uint64, map[string]string, error) {
	if e.closed.Load() {
		return 0, nil, ErrClosed
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return 0, nil, ErrClosed
	}

	return e.lastAppliedIndex, e.memtable.All(), nil
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
// In standalone mode, this routes through ApplyCommitted with RaftIndex=0.
func (e *Engine) Set(key, value string) error {
	return e.ApplyCommitted(CommittedCommand{
		Operation: wal.OperationSet,
		Key:       key,
		Value:     value,
	})
}

// Get retrieves the value for key.
// Returns ("", false) if closed or key is not found.
func (e *Engine) Get(key string) (string, bool) {
	if e.closed.Load() {
		return "", false
	}
	return e.memtable.Get(key)
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

// Close safely flushes the WAL, closes file handles, and releases the exclusive process lock.
// Retains sticky error: if close fails, subsequent invocations return the same error.
func (e *Engine) Close() error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return e.closeErr
	}
	e.closed.Store(true)

	if err := e.wal.Close(); err != nil {
		e.closeErr = fmt.Errorf("storage: close wal: %w", err)
		return e.closeErr
	}
	return nil
}
