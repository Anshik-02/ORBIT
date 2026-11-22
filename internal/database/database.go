package database

// StorageEngine is the interface that the Database layer talks to.
//
// Defining this interface here (rather than in the storage package) means:
//  1. The database package does NOT import the storage package directly.
//     It depends only on a behaviour, not a concrete implementation.
//  2. Section 2 (Raft) can supply a different StorageEngine implementation
//     — e.g. one that routes writes through the Raft leader — without
//     changing any code in this package or in the API layer.
//  3. Tests can supply a mock engine without touching the real WAL or disk.
//
// Every method mirrors the Engine methods from internal/storage/engine.go.
// As long as storage.Engine satisfies this interface (it does), callers
// can use either interchangeably.
type StorageEngine interface {
	Set(key, value string) error
	Get(key string) (string, bool)
	Delete(key string) error
	Close() error
}

// Database is the public-facing database object.
//
// It wraps a StorageEngine and adds:
//   - Input validation (empty keys/values, etc.)
//   - Idiomatic Go error types (ErrKeyNotFound instead of a boolean)
//   - A single point where future cross-cutting concerns (metrics, tracing,
//     auth checks) can be inserted without touching the storage layer.
//
// The API layer and Section 2 should work with *Database, not directly
// with the StorageEngine.
type Database struct {
	engine StorageEngine
}

// NewDatabase wraps a StorageEngine in a Database.
// The engine must already be initialised and ready to serve requests.
func NewDatabase(engine StorageEngine) *Database {
	return &Database{engine: engine}
}

// Close shuts down the database and flushes/closes the underlying engine.
// Must be called before the process exits.
func (db *Database) Close() error {
	return db.engine.Close()
}
