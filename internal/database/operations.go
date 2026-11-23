package database

import (
	"strings"

	"orbit/internal/storage"
)

// Set stores key -> value in the database.
// Under standard key-value semantics, empty string values are valid.
func (db *Database) Set(key, value string) error {
	db.closeMu.RLock()
	defer db.closeMu.RUnlock()

	if db.closed {
		return ErrClosed
	}
	if strings.TrimSpace(key) == "" {
		return ErrEmptyKey
	}
	return db.engine.Set(key, value)
}

// Get retrieves the value stored under key.
func (db *Database) Get(key string) (string, error) {
	db.closeMu.RLock()
	defer db.closeMu.RUnlock()

	if db.closed {
		return "", ErrClosed
	}
	if strings.TrimSpace(key) == "" {
		return "", ErrEmptyKey
	}

	value, found := db.engine.Get(key)
	if !found {
		return "", ErrKeyNotFound
	}
	return value, nil
}

// Delete removes key from the database.
func (db *Database) Delete(key string) error {
	db.closeMu.RLock()
	defer db.closeMu.RUnlock()

	if db.closed {
		return ErrClosed
	}
	if strings.TrimSpace(key) == "" {
		return ErrEmptyKey
	}
	return db.engine.Delete(key)
}

// ApplyCommitted routes a consensus-replicated committed command to the storage engine.
func (db *Database) ApplyCommitted(cmd storage.CommittedCommand) error {
	db.closeMu.RLock()
	defer db.closeMu.RUnlock()

	if db.closed {
		return ErrClosed
	}
	if strings.TrimSpace(cmd.Key) == "" {
		return ErrEmptyKey
	}
	return db.engine.ApplyCommitted(cmd)
}

// Health checks whether the database is accepting operations and the
// underlying storage engine is healthy and unpoisoned.
func (db *Database) Health() error {
	db.closeMu.RLock()
	defer db.closeMu.RUnlock()

	if db.closed {
		return ErrClosed
	}
	return db.engine.Health()
}

// ExportSnapshotBytes serializes the underlying state machine's key-value data with its applied Raft index.
func (db *Database) ExportSnapshotBytes() (uint64, []byte, error) {
	db.closeMu.RLock()
	defer db.closeMu.RUnlock()

	if db.closed {
		return 0, nil, ErrClosed
	}
	return db.engine.ExportSnapshotBytes()
}

// RestoreSnapshotBytes restores the state machine to snapshot state.
func (db *Database) RestoreSnapshotBytes(meta storage.SnapshotMetadata, data []byte) error {
	db.closeMu.RLock()
	defer db.closeMu.RUnlock()

	if db.closed {
		return ErrClosed
	}
	return db.engine.RestoreSnapshotBytes(meta, data)
}

// Scan returns key-value entries in [startKey, endKey) up to limit entries.
func (db *Database) Scan(startKey, endKey string, limit int) ([]storage.KVPair, error) {
	db.closeMu.RLock()
	defer db.closeMu.RUnlock()

	if db.closed {
		return nil, ErrClosed
	}
	return db.engine.Scan(startKey, endKey, limit)
}

// PrefixScan returns all key-value entries matching prefix up to limit entries.
func (db *Database) PrefixScan(prefix string, limit int) ([]storage.KVPair, error) {
	db.closeMu.RLock()
	defer db.closeMu.RUnlock()

	if db.closed {
		return nil, ErrClosed
	}
	return db.engine.PrefixScan(prefix, limit)
}

// Engine returns the underlying storage engine.
func (db *Database) Engine() StorageEngine {
	return db.engine
}
