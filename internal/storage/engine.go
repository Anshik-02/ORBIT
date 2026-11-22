package storage

import (
	"fmt"
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
//	WAL write + sync  →  Memtable update  →  return to caller
//
// If the process crashes between the WAL sync and the Memtable update,
// the next startup will replay the WAL and reconstruct the correct state.
// No data is lost.
//
// Other packages (api, database) must use Engine's public methods.
// They must never reach into the WAL or Memtable directly.
type Engine struct {
	memtable *Memtable
	wal      *wal.Log
}

// NewEngine opens the WAL at walPath, replays it to reconstruct the
// Memtable, and returns a ready-to-use Engine.
//
// This is the single entry point for starting the storage layer.
// It handles recovery automatically — callers don't need to know that
// a WAL replay happened.
func NewEngine(walPath string) (*Engine, error) {
	// Open (or create) the WAL file.
	w, err := wal.Open(walPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open wal: %w", err)
	}

	engine := &Engine{
		memtable: NewMemtable(),
		wal:      w,
	}

	// Replay the WAL to rebuild the Memtable from disk state.
	if err := engine.recover(); err != nil {
		w.Close()
		return nil, fmt.Errorf("storage: recovery failed: %w", err)
	}

	return engine, nil
}

// recover reads all WAL records and applies them to the Memtable in order.
//
// Called once during NewEngine(). After this returns, the Memtable reflects
// the exact database state as of the last successful write before shutdown.
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
			// Unknown operations are skipped rather than aborting recovery.
			// This makes forward compatibility easier when new operation types
			// are introduced — old nodes can still start up and serve reads.
		}
	}

	return nil
}

// Set stores key → value in the database.
//
// Steps (in this exact order):
//  1. Append a SET record to the WAL and sync to disk.
//  2. Update the Memtable.
//
// If step 1 fails, we return an error and the Memtable is NOT updated.
// If the process crashes after step 1 but before step 2, recovery will
// correctly replay the SET and the data will not be lost.
func (e *Engine) Set(key, value string) error {
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
//
// Returns (value, true) if the key exists, ("", false) if it does not.
// This is a pure in-memory lookup — no disk I/O.
func (e *Engine) Get(key string) (string, bool) {
	return e.memtable.Get(key)
}

// Delete removes key from the database.
//
// Steps (in this exact order):
//  1. Append a DELETE (tombstone) record to the WAL and sync to disk.
//  2. Remove the key from the Memtable.
//
// The WAL record is a tombstone: during recovery, encountering a DELETE
// record ensures the key is absent from the reconstructed Memtable even
// if earlier SET records for the same key exist in the log.
func (e *Engine) Delete(key string) error {
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
// Must be called before the process exits to ensure no buffered data is lost.
func (e *Engine) Close() error {
	if err := e.wal.Close(); err != nil {
		return fmt.Errorf("storage: close wal: %w", err)
	}
	return nil
}
