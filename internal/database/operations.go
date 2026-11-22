package database

import "strings"

// Set stores key → value in the database.
//
// Validation:
//   - key must not be blank (whitespace-only counts as blank)
//   - value must not be blank
//
// On success the write is durable: the WAL has been fsynced to disk
// before this returns.
func (db *Database) Set(key, value string) error {
	if strings.TrimSpace(key) == "" {
		return ErrEmptyKey
	}
	if strings.TrimSpace(value) == "" {
		return ErrEmptyValue
	}
	return db.engine.Set(key, value)
}

// Get retrieves the value stored under key.
//
// Returns:
//   - (value, nil) if the key exists.
//   - ("", ErrKeyNotFound) if the key does not exist.
//   - ("", ErrEmptyKey) if key is blank.
//
// Get is a pure in-memory lookup — no disk I/O.
func (db *Database) Get(key string) (string, error) {
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
//
// Appends a tombstone record to the WAL so the deletion survives a restart.
// Deleting a non-existent key is a no-op and returns nil — callers should
// not need to check for existence first.
func (db *Database) Delete(key string) error {
	if strings.TrimSpace(key) == "" {
		return ErrEmptyKey
	}
	return db.engine.Delete(key)
}
