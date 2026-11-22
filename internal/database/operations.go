package database

import "strings"

// Set stores key -> value in the database.
func (db *Database) Set(key, value string) error {
	if db.closed.Load() {
		return ErrClosed
	}
	if strings.TrimSpace(key) == "" {
		return ErrEmptyKey
	}
	if strings.TrimSpace(value) == "" {
		return ErrEmptyValue
	}
	return db.engine.Set(key, value)
}

// Get retrieves the value stored under key.
func (db *Database) Get(key string) (string, error) {
	if db.closed.Load() {
		return "", ErrClosed
	}
	if strings.TrimSpace(key) == "" {
		return "", ErrEmptyKey
	}

	value, found := db.engine.Get(key)
	if !found {
		// Double check if database was closed during retrieval
		if db.closed.Load() {
			return "", ErrClosed
		}
		return "", ErrKeyNotFound
	}
	return value, nil
}

// Delete removes key from the database.
func (db *Database) Delete(key string) error {
	if db.closed.Load() {
		return ErrClosed
	}
	if strings.TrimSpace(key) == "" {
		return ErrEmptyKey
	}
	return db.engine.Delete(key)
}
