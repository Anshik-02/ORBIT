package database

import "errors"

// Sentinel errors returned by Database operations.
// Callers should use errors.Is() to check for these.
//
// Example:
//
//	_, err := db.Get("user:1")
//	if errors.Is(err, ErrKeyNotFound) { ... }
var (
	// ErrKeyNotFound is returned by Get when the key does not exist.
	ErrKeyNotFound = errors.New("key not found")

	// ErrEmptyKey is returned when a blank key is supplied to any operation.
	ErrEmptyKey = errors.New("key must not be empty")

	// ErrEmptyValue is returned by Set when the value is blank.
	ErrEmptyValue = errors.New("value must not be empty")
)
