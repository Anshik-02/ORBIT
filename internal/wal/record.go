package wal

import (
	"errors"
	"fmt"
	"strings"
)

// Operation constants for WAL records.
const (
	OperationSet    = "SET"
	OperationDelete = "DELETE"
)

// Sentinel validation errors.
var (
	ErrEmptyKey         = errors.New("wal: record key must not be empty")
	ErrEmptySetValue    = errors.New("wal: SET record value must not be empty")
	ErrDeleteWithValue  = errors.New("wal: DELETE record must not have a value")
	ErrUnknownOperation = errors.New("wal: unknown operation")
)

// Record represents a single entry in the Write-Ahead Log.
type Record struct {
	Sequence  uint64 `json:"sequence"`
	Operation string `json:"operation"`
	Key       string `json:"key"`
	Value     string `json:"value,omitempty"`
}

// Validate validates that a record conforms to WAL integrity rules:
//   - Key must not be empty or whitespace-only.
//   - Operation must be either SET or DELETE.
//   - SET must have a non-empty Value.
//   - DELETE must have an empty Value.
func (r Record) Validate() error {
	if strings.TrimSpace(r.Key) == "" {
		return ErrEmptyKey
	}

	switch r.Operation {
	case OperationSet:
		if strings.TrimSpace(r.Value) == "" {
			return ErrEmptySetValue
		}
	case OperationDelete:
		if r.Value != "" {
			return ErrDeleteWithValue
		}
	default:
		return fmt.Errorf("%w: %q", ErrUnknownOperation, r.Operation)
	}

	return nil
}
