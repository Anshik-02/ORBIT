package database

import "strings"

// Set stores key -> value in the database.
func (db *Database) Set(key, value string) error {
	db.closeMu.RLock()
	defer db.closeMu.RUnlock()

	if db.closed {
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
