package tests

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"orbit/internal/raft"
)

// TestRaftStorageStatePersistence verifies currentTerm and votedFor survive engine restart.
func TestRaftStorageStatePersistence(t *testing.T) {
	dir := t.TempDir()

	// 1. Open fresh storage
	s1, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("OpenFileStorage: %v", err)
	}

	term, votedFor, err := s1.GetState()
	if err != nil {
		t.Fatalf("GetState fresh: %v", err)
	}
	if term != 0 || votedFor != "" {
		t.Fatalf("expected term 0, votedFor '', got term %d, votedFor %q", term, votedFor)
	}

	// 2. Save term and vote
	if err := s1.SaveState(5, "node-b"); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	if err := s1.Close(); err != nil {
		t.Fatalf("Close s1: %v", err)
	}

	// 3. Reopen and verify persistence
	s2, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	defer s2.Close()

	term2, votedFor2, err := s2.GetState()
	if err != nil {
		t.Fatalf("GetState s2: %v", err)
	}
	if term2 != 5 || votedFor2 != "node-b" {
		t.Fatalf("expected term 5, votedFor 'node-b', got term %d, votedFor %q", term2, votedFor2)
	}
}

// TestRaftStorageLogAppendAndRecovery verifies log entries survive restarts with checksums.
func TestRaftStorageLogAppendAndRecovery(t *testing.T) {
	dir := t.TempDir()

	s1, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}

	entries := []raft.LogEntry{
		{Index: 1, Term: 1, Operation: "SET", Key: "k1", Value: "v1"},
		{Index: 2, Term: 1, Operation: "SET", Key: "k2", Value: "v2"},
		{Index: 3, Term: 2, Operation: "DELETE", Key: "k1"},
	}

	if err := s1.AppendEntries(entries); err != nil {
		t.Fatalf("AppendEntries: %v", err)
	}

	lastIdx, lastTerm := s1.LastLogInfo()
	if lastIdx != 3 || lastTerm != 2 {
		t.Fatalf("expected lastIdx 3, lastTerm 2, got (%d, %d)", lastIdx, lastTerm)
	}

	if err := s1.Close(); err != nil {
		t.Fatalf("Close s1: %v", err)
	}

	// Reopen and verify replay
	s2, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	defer s2.Close()

	for _, want := range entries {
		got, ok := s2.GetEntry(want.Index)
		if !ok {
			t.Fatalf("entry at index %d not found after replay", want.Index)
		}
		if got.Index != want.Index || got.Term != want.Term || got.Operation != want.Operation ||
			got.Key != want.Key || got.Value != want.Value {
			t.Fatalf("mismatch at index %d: want %+v, got %+v", want.Index, want, got)
		}
	}

	// Verify GetEntries from index 2
	sub := s2.GetEntries(2)
	if len(sub) != 2 || sub[0].Index != 2 || sub[1].Index != 3 {
		t.Fatalf("unexpected GetEntries(2): %+v", sub)
	}
}

// TestRaftStorageTruncation verifies log truncation when replacing conflicting entries.
func TestRaftStorageTruncation(t *testing.T) {
	dir := t.TempDir()

	s, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}

	// Initial log: 1, 2, 3, 4, 5
	initial := []raft.LogEntry{
		{Index: 1, Term: 1, Operation: "SET", Key: "k1", Value: "v1"},
		{Index: 2, Term: 1, Operation: "SET", Key: "k2", Value: "v2"},
		{Index: 3, Term: 1, Operation: "SET", Key: "k3", Value: "v3"},
		{Index: 4, Term: 1, Operation: "SET", Key: "k4", Value: "v4"},
		{Index: 5, Term: 1, Operation: "SET", Key: "k5", Value: "v5"},
	}
	if err := s.AppendEntries(initial); err != nil {
		t.Fatalf("AppendEntries: %v", err)
	}

	// Truncate from index 4 onward (removes 4 and 5)
	if err := s.Truncate(4); err != nil {
		t.Fatalf("Truncate(4): %v", err)
	}

	lastIdx, lastTerm := s.LastLogInfo()
	if lastIdx != 3 || lastTerm != 1 {
		t.Fatalf("expected last index 3, got (%d, %d)", lastIdx, lastTerm)
	}

	// Append new entries with higher term (e.g. from new leader)
	replacement := []raft.LogEntry{
		{Index: 4, Term: 2, Operation: "SET", Key: "k4", Value: "new-v4"},
		{Index: 5, Term: 2, Operation: "SET", Key: "k5", Value: "new-v5"},
	}
	if err := s.AppendEntries(replacement); err != nil {
		t.Fatalf("AppendEntries replacement: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen and ensure file matches truncated + replaced log
	sReopened, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer sReopened.Close()

	e4, ok := sReopened.GetEntry(4)
	if !ok || e4.Term != 2 || e4.Value != "new-v4" {
		t.Fatalf("expected index 4 to be term 2 'new-v4', got %+v", e4)
	}
}

// TestRaftStorageChecksumBitrotDetected verifies corrupted records on disk are rejected.
func TestRaftStorageChecksumBitrotDetected(t *testing.T) {
	dir := t.TempDir()

	s, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	if err := s.AppendEntries([]raft.LogEntry{
		{Index: 1, Term: 1, Operation: "SET", Key: "k1", Value: "v1"},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Deliberately corrupt raft.log bitrot
	logPath := filepath.Join(dir, "raft", "raft.log")
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	// Mutate character inside the JSON
	corrupted := []byte(string(raw)[:len(raw)-15] + "9" + string(raw)[len(raw)-14:])
	if err := os.WriteFile(logPath, corrupted, 0600); err != nil {
		t.Fatalf("write corrupted: %v", err)
	}

	// Reopen should detect checksum mismatch
	_, err = raft.OpenFileStorage(dir)
	if !errors.Is(err, raft.ErrChecksumMismatch) && !errors.Is(err, raft.ErrCorruptState) {
		t.Fatalf("expected ErrChecksumMismatch or ErrCorruptState, got: %v", err)
	}
}
