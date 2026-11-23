package tests

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/raft"
)

// TestSnapshotPersistenceAndIntegrity verifies snapshot atomic persistence,
// checksum verification, and recovery across restarts.
func TestSnapshotPersistenceAndIntegrity(t *testing.T) {
	dir := t.TempDir()

	// 1. Open fresh storage
	s1, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}

	// Append entries 1..10
	entries := make([]raft.LogEntry, 10)
	for i := 1; i <= 10; i++ {
		entries[i-1] = raft.LogEntry{
			Index:     uint64(i),
			Term:      1,
			Operation: "SET",
			Key:       "user:alice",
			Value:     "engineer",
		}
	}
	if err := s1.AppendEntries(entries); err != nil {
		t.Fatalf("append entries: %v", err)
	}

	// Save snapshot up to index 6
	data := []byte(`{"user:alice":"engineer","counter":"42"}`)
	meta := raft.SnapshotMetadata{
		GroupID:           "default",
		LastIncludedIndex: 6,
		LastIncludedTerm:  1,
		CreatedAt:         time.Now(),
	}
	if err := s1.SaveSnapshot(meta, data); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	// Compact log up to index 6
	if err := s1.CompactLog(6, 1); err != nil {
		t.Fatalf("compact log: %v", err)
	}

	if s1.FirstIndex() != 6 {
		t.Fatalf("expected first index 6, got %d", s1.FirstIndex())
	}
	lastIdx, lastTerm := s1.LastLogInfo()
	if lastIdx != 10 || lastTerm != 1 {
		t.Fatalf("expected last index 10 term 1, got %d term %d", lastIdx, lastTerm)
	}

	// Entries < 6 should not be available
	for i := uint64(1); i < 6; i++ {
		if _, ok := s1.GetEntry(i); ok {
			t.Fatalf("entry %d should be compacted", i)
		}
	}

	// Sentinel entry at 6 must be available
	sentinel, ok := s1.GetEntry(6)
	if !ok || sentinel.Index != 6 || sentinel.Term != 1 {
		t.Fatalf("sentinel entry 6 not found or corrupt: %+v", sentinel)
	}

	// Entries 7..10 must be available
	for i := uint64(7); i <= 10; i++ {
		e, ok := s1.GetEntry(i)
		if !ok || e.Index != i {
			t.Fatalf("entry %d not found after compaction", i)
		}
	}

	if err := s1.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	// 2. Reopen storage and verify recovery from snapshot
	s2, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	defer s2.Close()

	if s2.FirstIndex() != 6 {
		t.Fatalf("reopened: expected first index 6, got %d", s2.FirstIndex())
	}
	lastIdx2, lastTerm2 := s2.LastLogInfo()
	if lastIdx2 != 10 || lastTerm2 != 1 {
		t.Fatalf("reopened: expected last log info 10 term 1, got %d term %d", lastIdx2, lastTerm2)
	}

	snap, err := s2.GetSnapshot()
	if err != nil || snap == nil {
		t.Fatalf("reopened: get snapshot failed: %v", err)
	}
	if snap.Metadata.LastIncludedIndex != 6 || snap.Metadata.LastIncludedTerm != 1 {
		t.Fatalf("reopened: snapshot metadata mismatch: %+v", snap.Metadata)
	}
	if string(snap.Data) != string(data) {
		t.Fatalf("reopened: snapshot data mismatch: got %s, want %s", string(snap.Data), string(data))
	}
}

// TestSnapshotChecksumBitrotDetected verifies corrupted snapshot payloads are rejected.
func TestSnapshotChecksumBitrotDetected(t *testing.T) {
	dir := t.TempDir()

	s, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}

	data := []byte(`{"user:bob":"developer"}`)
	meta := raft.SnapshotMetadata{
		GroupID:           "default",
		LastIncludedIndex: 5,
		LastIncludedTerm:  2,
		CreatedAt:         time.Now(),
	}
	if err := s.SaveSnapshot(meta, data); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	// Corrupt snapshot.snap file
	snapPath := filepath.Join(dir, "raft", "snapshot.snap")
	content, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot container: %v", err)
	}
	content[len(content)-5] ^= 0xFF
	if err := os.WriteFile(snapPath, content, 0600); err != nil {
		t.Fatalf("corrupt snapshot container: %v", err)
	}

	// Reopen storage: must fail with checksum error
	_, err = raft.OpenFileStorage(dir)
	if err == nil {
		t.Fatal("expected OpenFileStorage to fail on corrupted snapshot checksum, got nil")
	}
	if !errors.Is(err, raft.ErrChecksumMismatch) && !errors.Is(err, raft.ErrCorruptSnapshot) {
		t.Fatalf("expected ErrChecksumMismatch or ErrCorruptSnapshot, got: %v", err)
	}
}

// TestSnapshotMonotonicity verifies snapshots cannot regress LastIncludedIndex.
func TestSnapshotMonotonicity(t *testing.T) {
	dir := t.TempDir()

	s, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer s.Close()

	// Initial snapshot at index 50
	if err := s.SaveSnapshot(raft.SnapshotMetadata{
		GroupID:           "default",
		LastIncludedIndex: 50,
		LastIncludedTerm:  3,
	}, []byte("state-50")); err != nil {
		t.Fatalf("save snapshot 50: %v", err)
	}

	// Attempting snapshot at index 49 must fail
	err = s.SaveSnapshot(raft.SnapshotMetadata{
		GroupID:           "default",
		LastIncludedIndex: 49,
		LastIncludedTerm:  3,
	}, []byte("state-49"))
	if !errors.Is(err, raft.ErrSnapshotBehind) {
		t.Fatalf("expected ErrSnapshotBehind, got: %v", err)
	}

	// Snapshot at index 51 must succeed
	if err := s.SaveSnapshot(raft.SnapshotMetadata{
		GroupID:           "default",
		LastIncludedIndex: 51,
		LastIncludedTerm:  3,
	}, []byte("state-51")); err != nil {
		t.Fatalf("save snapshot 51: %v", err)
	}
}
