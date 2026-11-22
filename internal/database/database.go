package database

import (
	"reflect"
	"sync"

	"orbit/internal/storage"
)

// StorageEngine is the interface that the Database layer talks to.
// Exposes both client operations and the consensus application boundary.
type StorageEngine interface {
	Set(key, value string) error
	Get(key string) (string, bool)
	Delete(key string) error
	ApplyCommitted(cmd storage.CommittedCommand) error
	Close() error
	Health() error
}

// Database is the public-facing database object.
//
// Lifecycle Coordination:
//   - Uses a lifecycle `closeMu sync.RWMutex`.
//   - In-flight operations acquire `closeMu.RLock()`.
//   - Close() acquires `closeMu.Lock()`, ensuring all in-flight operations have
//     completely finished before the underlying storage engine is closed.
//   - Preserves sticky close error if engine.Close() fails, allowing callers to detect failures.
type Database struct {
	engine   StorageEngine
	closeMu  sync.RWMutex
	closed   bool
	closeErr error // records sticky error if engine.Close() fails
}

// isNilEngine checks whether an engine interface is nil or carries a typed nil pointer.
func isNilEngine(engine StorageEngine) bool {
	if engine == nil {
		return true
	}
	val := reflect.ValueOf(engine)
	return val.Kind() == reflect.Ptr && val.IsNil()
}

// NewDatabase wraps a StorageEngine in a Database.
// Enforces the constructor contract: rejects nil engines and typed nil pointers with ErrNilEngine.
func NewDatabase(engine StorageEngine) (*Database, error) {
	if isNilEngine(engine) {
		return nil, ErrNilEngine
	}
	return &Database{engine: engine}, nil
}

// Close gracefully shuts down the database:
//  1. Acquires closeMu.Lock(), blocking until all active operations finish.
//  2. Sets closed = true so future operations are rejected.
//  3. Flushes and closes the underlying storage engine.
//  4. Preserves any close failure so repeated calls return the error.
func (db *Database) Close() error {
	db.closeMu.Lock()
	defer db.closeMu.Unlock()

	if db.closed {
		return db.closeErr
	}
	db.closed = true
	db.closeErr = db.engine.Close()
	return db.closeErr
}
