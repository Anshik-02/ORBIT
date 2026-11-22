package storage

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"orbit/internal/wal"
)

// Sentinel errors.
var (
	ErrClosed = errors.New("storage: engine is closed")
)

// CommittedCommand represents an entry that has reached majority consensus
// (e.g. via Raft in Section 2) and is ready to be committed to local storage.
//
// Architectural Decoupling:
//   - RaftIndex: Consensus log position across the distributed cluster.
//   - RaftTerm:  The election term when the leader proposed this entry.
//   - WAL Sequence: The physical local node's append-only sequence number.
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
//   - Mutations (ApplyCommitted, Set, Delete) serialize through `writeMu`.
//   - Read operations (Get) are non-blocking and concurrent via Memtable RLock.
//   - Startup recovery uses a single streaming replay pass to rebuild the Memtable.
//   - Standalone Set and Delete operations execute through ApplyCommitted, establishing
//     the exact integration boundary for Section 2 (Raft).
type Engine struct {
	memtable         *Memtable
	wal              *wal.Log
	writeMu          sync.Mutex  // serializes WAL append + Memtable update transaction
	closed           atomic.Bool // tracks shutdown state
	lastAppliedIndex uint64      // highest applied Raft index for idempotency
}

// NewEngine opens the WAL at walPath, executes a single streaming replay pass
// to reconstruct the Memtable, and returns a ready-to-use Engine.
func NewEngine(walPath string) (*Engine, error) {
	w, err := wal.Open(walPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open wal: %w", err)
	}

	engine := &Engine{
		memtable: NewMemtable(),
		wal:      w,
	}

	// Single streaming replay pass: populates Memtable and verifies WAL integrity
	if err := engine.recover(); err != nil {
		w.Close()
		return nil, fmt.Errorf("storage: recovery failed: %w", err)
	}

	return engine, nil
}

// recover streams all WAL records directly into the Memtable.
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
		return nil
	})
}

// ApplyCommitted applies an entry that has reached quorum commit in the cluster.
//
// Idempotency:
// If a command with RaftIndex <= lastAppliedIndex is received (e.g. during replay
// or leadership change), it is safely recognized as an already-applied duplicate.
func (e *Engine) ApplyCommitted(cmd CommittedCommand) error {
	if e.closed.Load() {
		return ErrClosed
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return ErrClosed
	}

	// Idempotency check: duplicate consensus commands are safe no-ops
	if cmd.RaftIndex != 0 && cmd.RaftIndex <= e.lastAppliedIndex {
		return nil
	}

	var rec wal.Record
	switch cmd.Operation {
	case wal.OperationSet:
		rec = wal.Record{
			Operation: wal.OperationSet,
			Key:       cmd.Key,
			Value:     cmd.Value,
		}
	case wal.OperationDelete:
		rec = wal.Record{
			Operation: wal.OperationDelete,
			Key:       cmd.Key,
		}
	default:
		return fmt.Errorf("storage: unsupported committed operation %q", cmd.Operation)
	}

	// Persist to local WAL
	if err := e.wal.Append(rec); err != nil {
		return fmt.Errorf("storage: wal append committed (op %q, key %q): %w", cmd.Operation, cmd.Key, err)
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

// Close safely flushes the WAL and shuts down the storage engine.
// Idempotent: multiple calls return nil.
func (e *Engine) Close() error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Swap(true) {
		return nil
	}

	if err := e.wal.Close(); err != nil {
		return fmt.Errorf("storage: close wal: %w", err)
	}
	return nil
}
