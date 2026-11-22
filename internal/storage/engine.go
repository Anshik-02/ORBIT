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

// Engine is the core storage component of ORBIT.
//
// It coordinates two sub-systems:
//   - WAL (Write-Ahead Log): guarantees durability by recording every
//     mutation to disk before it touches in-memory state.
//   - Memtable: an in-memory map that serves all read requests at O(1).
//
// Invariants & Concurrency:
//   - Operations check `closed` state and return ErrClosed if shut down.
//   - All mutations serialize through `writeMu`:
//     [writeMu.Lock] -> WAL append + sync -> Memtable update -> [writeMu.Unlock]
//   - Reads (Get) are concurrent and non-blocking using Memtable RLock.
type Engine struct {
	memtable *Memtable
	wal      *wal.Log
	writeMu  sync.Mutex  // serializes WAL append + Memtable update transaction
	closed   atomic.Bool // tracks shutdown state
}

// NewEngine opens the WAL at walPath, replays it to reconstruct the
// Memtable, and returns a ready-to-use Engine.
func NewEngine(walPath string) (*Engine, error) {
	w, err := wal.Open(walPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open wal: %w", err)
	}

	engine := &Engine{
		memtable: NewMemtable(),
		wal:      w,
	}

	// Replay WAL to restore state
	if err := engine.recover(); err != nil {
		w.Close()
		return nil, fmt.Errorf("storage: recovery failed: %w", err)
	}

	return engine, nil
}

// recover replays all WAL records into the Memtable.
// Any unknown operation or invalid record is rejected with an error.
func (e *Engine) recover() error {
	records, err := e.wal.Replay()
	if err != nil {
		return err
	}

	for _, r := range records {
		switch r.Operation {
		case wal.OperationSet:
			e.memtable.Set(r.Key, r.Value)
		case wal.OperationDelete:
			e.memtable.Delete(r.Key)
		default:
			return fmt.Errorf("storage: unexpected operation %q during recovery (seq %d)", r.Operation, r.Sequence)
		}
	}

	return nil
}

// Set stores key -> value in the database.
func (e *Engine) Set(key, value string) error {
	if e.closed.Load() {
		return ErrClosed
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return ErrClosed
	}

	record := wal.Record{
		Operation: wal.OperationSet,
		Key:       key,
		Value:     value,
	}

	if err := e.wal.Append(record); err != nil {
		return fmt.Errorf("storage: wal append (SET %q): %w", key, err)
	}

	e.memtable.Set(key, value)
	return nil
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
func (e *Engine) Delete(key string) error {
	if e.closed.Load() {
		return ErrClosed
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if e.closed.Load() {
		return ErrClosed
	}

	record := wal.Record{
		Operation: wal.OperationDelete,
		Key:       key,
	}

	if err := e.wal.Append(record); err != nil {
		return fmt.Errorf("storage: wal append (DELETE %q): %w", key, err)
	}

	e.memtable.Delete(key)
	return nil
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
