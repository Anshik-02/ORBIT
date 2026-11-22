package wal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"
)

// CurrentFormatVersion defines the current WAL record serialization schema version.
const CurrentFormatVersion uint32 = 1

// Operation constants for WAL records.
const (
	OperationSet    = "SET"
	OperationDelete = "DELETE"
)

// Sentinel validation errors.
var (
	ErrEmptyKey           = errors.New("wal: record key must not be empty")
	ErrDeleteWithValue    = errors.New("wal: DELETE record must not have a value")
	ErrUnknownOperation   = errors.New("wal: unknown operation")
	ErrUnsupportedVersion = errors.New("wal: unsupported record format version")
	ErrChecksumMismatch   = errors.New("wal: record checksum mismatch (corrupted data)")
)

// Record represents a single entry in the Write-Ahead Log.
//
// Schema Versioning & Integrity:
//   - Version: Allows schema evolution (e.g. adding Raft terms/indexes, transaction IDs).
//   - RaftIndex & RaftTerm: Persists distributed consensus metadata so applied index is durable.
//   - Checksum: CRC32-IEEE checksum over unambiguously length-prefixed payload fields.
type Record struct {
	Version   uint32 `json:"version"`
	Sequence  uint64 `json:"sequence"`
	RaftIndex uint64 `json:"raft_index,omitempty"`
	RaftTerm  uint64 `json:"raft_term,omitempty"`
	Operation string `json:"operation"`
	Key       string `json:"key"`
	Value     string `json:"value,omitempty"`
	Checksum  uint32 `json:"checksum"`
}

// ComputeChecksum computes a CRC32-IEEE checksum over unambiguous length-prefixed payload fields:
// Version, Sequence, RaftIndex, RaftTerm, Operation, Key, and Value.
// Length prefixing variable-length strings eliminates boundary collision ambiguity.
func (r Record) ComputeChecksum() uint32 {
	h := crc32.NewIEEE()

	var numBuf [8]byte

	// Version (uint32)
	binary.BigEndian.PutUint32(numBuf[:4], r.Version)
	h.Write(numBuf[:4])

	// Sequence (uint64)
	binary.BigEndian.PutUint64(numBuf[:], r.Sequence)
	h.Write(numBuf[:])

	// RaftIndex (uint64)
	binary.BigEndian.PutUint64(numBuf[:], r.RaftIndex)
	h.Write(numBuf[:])

	// RaftTerm (uint64)
	binary.BigEndian.PutUint64(numBuf[:], r.RaftTerm)
	h.Write(numBuf[:])

	// Length-prefixed Operation
	binary.BigEndian.PutUint32(numBuf[:4], uint32(len(r.Operation)))
	h.Write(numBuf[:4])
	h.Write([]byte(r.Operation))

	// Length-prefixed Key
	binary.BigEndian.PutUint32(numBuf[:4], uint32(len(r.Key)))
	h.Write(numBuf[:4])
	h.Write([]byte(r.Key))

	// Length-prefixed Value
	binary.BigEndian.PutUint32(numBuf[:4], uint32(len(r.Value)))
	h.Write(numBuf[:4])
	h.Write([]byte(r.Value))

	return h.Sum32()
}

// Validate verifies that a record satisfies WAL integrity and format rules:
//   - Schema version must match CurrentFormatVersion.
//   - Key must not be empty or whitespace-only.
//   - Operation must be either SET or DELETE.
//   - Standard KV semantics: empty Value is permitted for SET.
//   - DELETE must not have a non-empty Value.
//   - Checksum must match the computed CRC32.
func (r Record) Validate() error {
	if strings.TrimSpace(r.Key) == "" {
		return ErrEmptyKey
	}

	if r.Version != CurrentFormatVersion {
		return fmt.Errorf("%w: got %d, expected %d", ErrUnsupportedVersion, r.Version, CurrentFormatVersion)
	}

	switch r.Operation {
	case OperationSet:
		// In standard key-value semantics, empty values are valid.
	case OperationDelete:
		if r.Value != "" {
			return ErrDeleteWithValue
		}
	default:
		return fmt.Errorf("%w: %q", ErrUnknownOperation, r.Operation)
	}

	if r.Checksum != r.ComputeChecksum() {
		return fmt.Errorf("%w: got %d, expected %d", ErrChecksumMismatch, r.Checksum, r.ComputeChecksum())
	}

	return nil
}
