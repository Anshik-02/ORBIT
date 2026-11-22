package storage

import (
	"fmt"
	"sync"

	"orbit/internal/wal"
)

// Engine is the core storage component of ORBIT.
//
// It coordinates two sub-systems:
//   - WAL (Write-Ahead Log): guarantees durability by recording every
//     mutation to disk before it touches in-memory state.
//   - Memtable: an in-memory map that serves all read requests at O(1).
//
// The invariant Engine enforces:
//
//	[writeMu.Lock] -> WAL append + sync -> Memtable update -> [writeMu.Unlock]
//
// Concurrency Model:
//   - All write operations (Set / Delete) serialize through `writeMu`.
//     This ensures that the order operations appear in the WAL strictly
//     matches the order they mutate the Memtable.
//   - Read operations (Get) only touch the Memtable with a shared RLock,
//     allowing highly concurrent, non-blocking reads.
type Engine struct {
	memtable *Memtable
	wal      *wal.Log
	writeMu  sync.Mutex // serializes WAL append + Memtable update transaction
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
			// Forward compatibility: ignore unknown operations
		}
	}

	return nil
}

// Set stores key -> value in the database.
//
// Serialized through writeMu:
//  1. Append SET record to WAL and sync to physical disk.
//  2. Update the Memtable.
// Both steps complete atomically with respect to other writes.
func (e *Engine) Set(key, value string) error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()

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
// Concurrent and non-blocking: accesses the Memtable via RLock.
func (e *Engine) Get(key string) (string, bool) {
	return e.memtable.Get(key)
}

// Delete removes key from the database.
//
// Serialized through writeMu:
//  1. Append DELETE tombstone record to WAL and sync to physical disk.
//  2. Remove key from the Memtable.
func (e *Engine) Delete(key string) error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()

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

// Close flushes and closes the WAL.
func (e *Engine) Close() error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	if err := e.wal.Close(); err != nil {
		return fmt.Errorf("storage: close wal: %w", err)
	}
	return nil
}
