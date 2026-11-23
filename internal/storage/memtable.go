package storage

import (
	"sort"
	"sync"
)

// memtableEntryOverhead is the estimated fixed memory overhead per entry (hash map node, struct fields).
const memtableEntryOverhead = int64(32)

// MemtableEntry represents an entry stored in the in-memory Memtable,
// which can be either an active key-value pair or a tombstone indicating deletion.
type MemtableEntry struct {
	Key         string
	Value       string
	IsTombstone bool
}

// Memtable is the in-memory write buffer for the LSM-Tree storage architecture.
//
// Invariants & Concurrency:
//   - Multiple concurrent GET requests can proceed in parallel (RLock).
//   - SET and DELETE requests acquire exclusive access (Lock).
//   - Tracks accurate in-memory byte size (keys, values, and struct overhead).
//   - Tombstones are tracked explicitly so deletions shadow values in immutable memtables & SSTables.
//   - Entries can be extracted in deterministic lexicographically sorted order for SSTable flushing.
type Memtable struct {
	mu        sync.RWMutex
	data      map[string]MemtableEntry
	sizeBytes int64
}

// NewMemtable creates an empty, ready-to-use Memtable.
func NewMemtable() *Memtable {
	return &Memtable{
		data:      make(map[string]MemtableEntry),
		sizeBytes: 0,
	}
}

// Set stores key -> value in the Memtable.
// Updates the size tracking accounting for key, value, and entry overhead.
func (m *Memtable) Set(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	old, exists := m.data[key]
	if exists {
		if old.IsTombstone {
			m.sizeBytes -= int64(len(old.Key)) + memtableEntryOverhead
		} else {
			m.sizeBytes -= int64(len(old.Key) + len(old.Value)) + memtableEntryOverhead
		}
	}

	entrySize := int64(len(key)+len(value)) + memtableEntryOverhead
	m.sizeBytes += entrySize

	m.data[key] = MemtableEntry{
		Key:         key,
		Value:       value,
		IsTombstone: false,
	}
}

// Delete stores a tombstone for key in the Memtable.
// The tombstone shadows older values in immutable memtables and SSTables.
func (m *Memtable) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	old, exists := m.data[key]
	if exists {
		if old.IsTombstone {
			return // already a tombstone
		}
		m.sizeBytes -= int64(len(old.Key) + len(old.Value)) + memtableEntryOverhead
	}

	tombstoneSize := int64(len(key)) + memtableEntryOverhead
	m.sizeBytes += tombstoneSize

	m.data[key] = MemtableEntry{
		Key:         key,
		Value:       "",
		IsTombstone: true,
	}
}

// Get retrieves the value for a key from this Memtable.
// Returns (value, true) if found and not a tombstone.
// Returns ("", false) if absent OR if a tombstone is present.
func (m *Memtable) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.data[key]
	if !exists || entry.IsTombstone {
		return "", false
	}
	return entry.Value, true
}

// GetEntry retrieves the MemtableEntry for key.
// Returns (entry, true) if present in this Memtable (even if tombstone).
// Returns (empty, false) if the key is not present in this Memtable.
func (m *Memtable) GetEntry(key string) (MemtableEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.data[key]
	return entry, exists
}

// SizeBytes returns the approximate in-memory byte size of all entries in the Memtable.
func (m *Memtable) SizeBytes() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sizeBytes
}

// Len returns the total number of entries in the Memtable (including tombstones).
func (m *Memtable) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// EntriesSorted returns all entries in the Memtable sorted by key in ascending order.
// Used by the SSTable writer during Memtable flushing.
func (m *Memtable) EntriesSorted() []MemtableEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]MemtableEntry, len(keys))
	for i, k := range keys {
		entries[i] = m.data[k]
	}
	return entries
}

// All returns a point-in-time copy of all live (non-tombstone) key-value entries.
// Used for snapshot exports and state reconciliation.
func (m *Memtable) All() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	copied := make(map[string]string, len(m.data))
	for k, entry := range m.data {
		if !entry.IsTombstone {
			copied[k] = entry.Value
		}
	}
	return copied
}
