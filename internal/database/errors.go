package database

import "errors"

// Sentinel errors returned by Database operations.
var (
	// ErrKeyNotFound is returned by Get when the key does not exist.
	ErrKeyNotFound = errors.New("key not found")

	// ErrEmptyKey is returned when a blank key is supplied to any operation.
	ErrEmptyKey = errors.New("key must not be empty")

	// ErrEmptyValue is returned by Set when the value is blank.
	ErrEmptyValue = errors.New("value must not be empty")

	// ErrClosed is returned when an operation is attempted on a closed database.
	ErrClosed = errors.New("database is closed")
)
