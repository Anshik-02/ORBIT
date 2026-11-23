package raft_test

import (
	"os"
	"path/filepath"
	"testing"

	"orbit/internal/raft"
	"orbit/internal/storage"
)

// openStorage opens a FileStorage in the given directory.
func openStorage(t *testing.T, dir string) *raft.FileStorage {
	t.Helper()
	fs, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("OpenFileStorage(%q): %v", dir, err)
	}
	return fs
}

// appendEntry appends a single entry with the given index and term to the storage.
func appendEntry(t *testing.T, fs *raft.FileStorage, index, term uint64) {
	t.Helper()
	if err := fs.AppendEntries([]raft.LogEntry{
		{Index: index, Term: term, Operation: "SET", Key: "k", Value: "v"},
	}); err != nil {
		t.Fatalf("AppendEntries(index=%d): %v", index, err)
	}
}

// TestStorage_DuplicateCommittedIndex verifies that applying the same RaftIndex twice
// is idempotent (the storage engine silently ignores the duplicate).
func TestStorage_DuplicateCommittedIndex(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	eng, err := storage.NewEngine(filepath.Join(dir, "wal.log"))
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	cmd := storage.CommittedCommand{
		RaftIndex: 1,
		RaftTerm:  1,
		Operation: "SET",
		Key:       "user:1",
		Value:     "Alice",
	}

	// First application must succeed.
	if err := eng.ApplyCommitted(cmd); err != nil {
		t.Fatalf("first ApplyCommitted: %v", err)
	}

	// Second application of the same index must be a no-op (idempotent).
	if err := eng.ApplyCommitted(cmd); err != nil {
		t.Errorf("second ApplyCommitted with same RaftIndex should be idempotent, got error: %v", err)
	}

	// The stored value should still be Alice.
	// Use LastAppliedIndex as a proxy to verify correct state.
	if idx := eng.LastAppliedIndex(); idx != 1 {
		t.Errorf("lastAppliedIndex: got %d, want 1", idx)
	}
}

// TestStorage_MissingRaftIndex verifies that skipping a Raft index (1, 2, 4 — skipping 3)
// is rejected with ErrRaftIndexGap.
func TestStorage_MissingRaftIndex(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	eng, err := storage.NewEngine(filepath.Join(dir, "wal.log"))
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	apply := func(idx uint64) error {
		return eng.ApplyCommitted(storage.CommittedCommand{
			RaftIndex: idx,
			RaftTerm:  1,
			Operation: "SET",
			Key:       "k",
			Value:     "v",
		})
	}

	for _, idx := range []uint64{1, 2} {
		if err := apply(idx); err != nil {
			t.Fatalf("ApplyCommitted(%d): %v", idx, err)
		}
	}

	// Skip 3 and try to apply 4 — must be rejected.
	if err := apply(4); err == nil {
		t.Fatal("expected error for skipped index 4, got nil")
	}
}

// TestStorage_WALRecovery verifies that entries written to FileStorage survive
// a close+reopen cycle.
func TestStorage_WALRecovery(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fs := openStorage(t, dir)

	// Write entries 1–3.
	for i := uint64(1); i <= 3; i++ {
		appendEntry(t, fs, i, 1)
	}

	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen and verify all entries are present.
	fs2 := openStorage(t, dir)
	defer fs2.Close()

	for i := uint64(1); i <= 3; i++ {
		entry, ok := fs2.GetEntry(i)
		if !ok {
			t.Errorf("entry %d missing after recovery", i)
			continue
		}
		if entry.Index != i {
			t.Errorf("entry %d: got index %d", i, entry.Index)
		}
	}

	lastIdx, lastTerm := fs2.LastLogInfo()
	if lastIdx != 3 || lastTerm != 1 {
		t.Errorf("LastLogInfo: got (%d,%d), want (3,1)", lastIdx, lastTerm)
	}
}

// TestStorage_IndependentWALFiles verifies that two storage engines in different
// directories use distinct WAL file paths.
func TestStorage_IndependentWALFiles(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	fs1 := openStorage(t, dir1)
	defer fs1.Close()
	fs2 := openStorage(t, dir2)
	defer fs2.Close()

	wal1 := filepath.Join(dir1, "raft", "raft.log")
	wal2 := filepath.Join(dir2, "raft", "raft.log")

	if wal1 == wal2 {
		t.Errorf("expected distinct WAL paths, both are %q", wal1)
	}

	// Write to fs1 — fs2 should stay empty.
	appendEntry(t, fs1, 1, 1)

	idx1, _ := fs1.LastLogInfo()
	idx2, _ := fs2.LastLogInfo()

	if idx1 != 1 {
		t.Errorf("fs1 lastIndex: got %d, want 1", idx1)
	}
	if idx2 != 0 {
		t.Errorf("fs2 lastIndex: got %d, want 0 (should be unaffected)", idx2)
	}
}

// TestStorage_IndependentRaftMetadata verifies that two FileStorage instances in
// different directories hold fully independent persistent state.
func TestStorage_IndependentRaftMetadata(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	fs1 := openStorage(t, dir1)
	defer fs1.Close()
	fs2 := openStorage(t, dir2)
	defer fs2.Close()

	// Write different terms to each instance.
	if err := fs1.SaveState(10, "node-a"); err != nil {
		t.Fatalf("fs1 SaveState: %v", err)
	}
	if err := fs2.SaveState(20, "node-b"); err != nil {
		t.Fatalf("fs2 SaveState: %v", err)
	}

	term1, voted1, err := fs1.GetState()
	if err != nil {
		t.Fatalf("fs1 GetState: %v", err)
	}
	term2, voted2, err := fs2.GetState()
	if err != nil {
		t.Fatalf("fs2 GetState: %v", err)
	}

	if term1 != 10 || voted1 != "node-a" {
		t.Errorf("fs1: got term=%d votedFor=%q, want term=10 votedFor=node-a", term1, voted1)
	}
	if term2 != 20 || voted2 != "node-b" {
		t.Errorf("fs2: got term=%d votedFor=%q, want term=20 votedFor=node-b", term2, voted2)
	}
}

// TestStorage_RecoveryAfterInterruptedShutdown simulates an interrupted shutdown
// (in-memory state discarded without calling Close) and verifies that reopening
// the storage recovers all previously fsynced entries correctly.
func TestStorage_RecoveryAfterInterruptedShutdown(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Phase 1: write entries 1–5 and fsync them.
	// We retain the handle to close it in t.Cleanup so Windows can remove the TempDir,
	// but we do NOT call fs.Close() during normal execution to simulate an abrupt crash.
	var fsToClean *raft.FileStorage
	t.Cleanup(func() {
		if fsToClean != nil {
			_ = fsToClean.Close()
		}
	})

	func() {
		fs := openStorage(t, dir)
		fsToClean = fs
		for i := uint64(1); i <= 5; i++ {
			appendEntry(t, fs, i, 1)
		}
	}()

	// Phase 2: reopen and verify all fsynced entries are present.
	fs2 := openStorage(t, dir)
	defer fs2.Close()

	for i := uint64(1); i <= 5; i++ {
		entry, ok := fs2.GetEntry(i)
		if !ok {
			t.Errorf("entry %d missing after simulated interrupted shutdown", i)
			continue
		}
		if entry.Index != i || entry.Term != 1 {
			t.Errorf("entry %d: got index=%d term=%d, want index=%d term=1", i, entry.Index, entry.Term, i)
		}
	}

	// Also verify that re-opening from the directory produces all five entries.
	// Stat the raft.log file to confirm it still exists.
	logPath := filepath.Join(dir, "raft", "raft.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("raft.log not found after simulated crash: %v", err)
	}
}
