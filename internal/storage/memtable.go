package storage

import "sync"

// Memtable is the in-memory representation of the current database state.
// It is a map protected by a read-write mutex so that:
//   - Multiple concurrent GET requests can proceed in parallel (RLock).
//   - SET and DELETE requests require exclusive access (Lock).
//
// The Memtable is rebuilt from the WAL on every startup — it is never
// written directly to disk.
type Memtable struct {
	data map[string]string
	mu   sync.RWMutex
}

// NewMemtable creates an empty, ready-to-use Memtable.
func NewMemtable() *Memtable {
	return &Memtable{
		data: make(map[string]string),
	}
}

// Set stores key → value.
// Acquires an exclusive write lock for the duration of the operation.
func (m *Memtable) Set(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

// Get retrieves the value for a key.
// Returns (value, true) if found, ("", false) if absent.
// Acquires a shared read lock — safe to call concurrently with other Gets.
func (m *Memtable) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, exists := m.data[key]
	return value, exists
}

// Delete removes a key from the Memtable.
// If the key doesn't exist this is a no-op (safe to call either way).
// Acquires an exclusive write lock.
func (m *Memtable) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
}

// All returns a point-in-time copy of all key-value entries currently in the memtable.
// Acquires a shared read lock so it is safe to call concurrently with Gets.
func (m *Memtable) All() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copied := make(map[string]string, len(m.data))
	for k, v := range m.data {
		copied[k] = v
	}
	return copied
}
