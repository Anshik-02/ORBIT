package database

import "sync/atomic"

// StorageEngine is the interface that the Database layer talks to.
type StorageEngine interface {
	Set(key, value string) error
	Get(key string) (string, bool)
	Delete(key string) error
	Close() error
}

// Database is the public-facing database object.
//
// It wraps a StorageEngine and adds:
//   - Input validation (empty keys/values)
//   - Idiomatic Go error types (ErrKeyNotFound, ErrClosed)
//   - Shutdown tracking preventing operations after Close
type Database struct {
	engine StorageEngine
	closed atomic.Bool
}

// NewDatabase wraps a StorageEngine in a Database.
func NewDatabase(engine StorageEngine) *Database {
	return &Database{engine: engine}
}

// Close shuts down the database and flushes/closes the underlying engine.
// Idempotent: multiple calls return nil.
func (db *Database) Close() error {
	if db.closed.Swap(true) {
		return nil
	}
	return db.engine.Close()
}
