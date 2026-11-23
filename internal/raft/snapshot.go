package raft

import (
	"errors"
	"fmt"
	"hash/crc32"
	"time"
)

const (
	SnapshotCurrentVersion uint32 = 1
	MaxSnapshotSizeBytes   int64  = 256 * 1024 * 1024 // 256 MB maximum snapshot payload size
	MaxSnapshotChunkBytes  int64  = 1024 * 1024       // 1 MB maximum chunk size
)

// SnapshotMetadata captures the consensus indexing and integrity metadata of a snapshot.
type SnapshotMetadata struct {
	Version           uint32    `json:"version"`
	GroupID           GroupID   `json:"group_id"`
	LastIncludedIndex uint64    `json:"last_included_index"`
	LastIncludedTerm  uint64    `json:"last_included_term"`
	CreatedAt         time.Time `json:"created_at"`
	Checksum          uint32    `json:"checksum"` // CRC32-IEEE checksum computed over Data
	Size              int64     `json:"size"`     // Byte size of Data
}

// Snapshot represents an immutable snapshot of state machine data at a specific Raft index.
type Snapshot struct {
	Metadata SnapshotMetadata `json:"metadata"`
	Data     []byte           `json:"data"`
}

// ComputeSnapshotChecksum computes the CRC32-IEEE checksum of snapshot payload data.
func ComputeSnapshotChecksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// Validate verifies the checksum, size, and indexing invariants of the snapshot.
func (s *Snapshot) Validate() error {
	if s == nil {
		return errors.New("snapshot: nil snapshot")
	}
	if s.Metadata.LastIncludedIndex == 0 {
		return fmt.Errorf("%w: invalid last_included_index 0", ErrCorruptSnapshot)
	}
	if int64(len(s.Data)) != s.Metadata.Size {
		return fmt.Errorf("%w: size mismatch: metadata %d, actual %d",
			ErrCorruptSnapshot, s.Metadata.Size, len(s.Data))
	}
	computed := ComputeSnapshotChecksum(s.Data)
	if computed != s.Metadata.Checksum {
		return fmt.Errorf("%w: checksum mismatch: stored %d, computed %d",
			ErrChecksumMismatch, s.Metadata.Checksum, computed)
	}
	return nil
}
