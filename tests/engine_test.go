package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"orbit/internal/database"
	"orbit/internal/storage"
	"orbit/internal/wal"
)

// TestEngineConcurrentSetDifferentKeys tests multiple workers inserting distinct keys.
// Verifies that after all writes finish, the in-memory engine and a replayed restarted
// engine contain every single key-value pair.
func TestEngineConcurrentSetDifferentKeys(t *testing.T) {
	dir := t.TempDir()
	walPath := dir + "/wal.log"

	engine, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	db := database.NewDatabase(engine)

	const workers = 8
	const itemsPerWorker = 20
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			for i := 0; i < itemsPerWorker; i++ {
				key := fmt.Sprintf("worker-%d-key-%d", wid, i)
				val := fmt.Sprintf("val-%d-%d", wid, i)
				if err := db.Set(key, val); err != nil {
					t.Errorf("Set failed: %v", err)
				}
			}
		}(w)
	}
	wg.Wait()

	// Verify all keys present in memory
	for w := 0; w < workers; w++ {
		for i := 0; i < itemsPerWorker; i++ {
			key := fmt.Sprintf("worker-%d-key-%d", w, i)
			expected := fmt.Sprintf("val-%d-%d", w, i)
			got, err := db.Get(key)
			if err != nil || got != expected {
				t.Fatalf("key %s: expected %q, got %q (err: %v)", key, expected, got, err)
			}
		}
	}

	db.Close()

	// Reopen and verify WAL replay reconstructs all items
	engineReplayed, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("reopen NewEngine: %v", err)
	}
	dbReplayed := database.NewDatabase(engineReplayed)
	defer dbReplayed.Close()

	for w := 0; w < workers; w++ {
		for i := 0; i < itemsPerWorker; i++ {
			key := fmt.Sprintf("worker-%d-key-%d", w, i)
			expected := fmt.Sprintf("val-%d-%d", w, i)
			got, err := dbReplayed.Get(key)
			if err != nil || got != expected {
				t.Fatalf("after replay key %s: expected %q, got %q (err: %v)", key, expected, got, err)
			}
		}
	}
}

// TestEngineConcurrentSetSameKey tests intense write contention on a single key.
// The in-memory Memtable and the on-disk WAL must never diverge.
func TestEngineConcurrentSetSameKey(t *testing.T) {
	dir := t.TempDir()
	walPath := dir + "/wal.log"

	engine, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	db := database.NewDatabase(engine)

	const workers = 10
	const writesPerWorker = 30
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			for i := 0; i < writesPerWorker; i++ {
				val := fmt.Sprintf("val-worker-%d-seq-%d", wid, i)
				if err := db.Set("hot-key", val); err != nil {
					t.Errorf("Set failed: %v", err)
				}
			}
		}(w)
	}
	wg.Wait()

	// Get final in-memory value
	inMemoryVal, err := db.Get("hot-key")
	if err != nil {
		t.Fatalf("Get hot-key: %v", err)
	}

	db.Close()

	// Reopen on same WAL — the reconstructed value MUST match inMemoryVal exactly.
	engine2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("reopen NewEngine: %v", err)
	}
	db2 := database.NewDatabase(engine2)
	defer db2.Close()

	replayedVal, err := db2.Get("hot-key")
	if err != nil {
		t.Fatalf("replayed Get hot-key: %v", err)
	}

	if inMemoryVal != replayedVal {
		t.Fatalf("CRITICAL CONSISTENCY BUG: in-memory value %q != replayed WAL value %q", inMemoryVal, replayedVal)
	}
}

// TestEngineConcurrentSetAndDeleteSameKey tests concurrent sets and deletes racing on a key.
func TestEngineConcurrentSetAndDeleteSameKey(t *testing.T) {
	dir := t.TempDir()
	walPath := dir + "/wal.log"

	engine, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	db := database.NewDatabase(engine)

	var wg sync.WaitGroup

	// Setters
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				_ = db.Set("shared-key", fmt.Sprintf("val-%d-%d", wid, i))
			}
		}(w)
	}

	// Deleters
	for d := 0; d < 4; d++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				_ = db.Delete("shared-key")
			}
		}()
	}

	wg.Wait()

	inMemoryVal, inMemoryErr := db.Get("shared-key")
	db.Close()

	// Reopen and ensure replay state matches exactly
	engine2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("reopen NewEngine: %v", err)
	}
	db2 := database.NewDatabase(engine2)
	defer db2.Close()

	replayedVal, replayedErr := db2.Get("shared-key")

	if errors.Is(inMemoryErr, database.ErrKeyNotFound) {
		if !errors.Is(replayedErr, database.ErrKeyNotFound) {
			t.Fatalf("expected ErrKeyNotFound in replayed engine, got %v (val: %q)", replayedErr, replayedVal)
		}
	} else {
		if inMemoryVal != replayedVal {
			t.Fatalf("mismatch after race: in-memory %q != replayed %q", inMemoryVal, replayedVal)
		}
	}
}

// TestEngineShutdownRejectsOperations verifies that operations after shutdown return ErrClosed.
func TestEngineShutdownRejectsOperations(t *testing.T) {
	dir := t.TempDir()
	engine, err := storage.NewEngine(dir + "/wal.log")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	db := database.NewDatabase(engine)

	if err := db.Set("k1", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Close database
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Double close is safe (idempotent)
	if err := db.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	// Operations should return ErrClosed
	if err := db.Set("k2", "v2"); !errors.Is(err, database.ErrClosed) {
		t.Errorf("Set after close: expected ErrClosed, got %v", err)
	}
	if _, err := db.Get("k1"); !errors.Is(err, database.ErrClosed) {
		t.Errorf("Get after close: expected ErrClosed, got %v", err)
	}
	if err := db.Delete("k1"); !errors.Is(err, database.ErrClosed) {
		t.Errorf("Delete after close: expected ErrClosed, got %v", err)
	}
}

// TestWALUnknownOperationRejected verifies that unknown operations in the WAL are rejected during replay.
func TestWALUnknownOperationRejected(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal.log"

	rec := wal.Record{
		Version:   wal.CurrentFormatVersion,
		Sequence:  1,
		Operation: "UNKNOWN_OP",
		Key:       "user:1",
		Value:     "Anshik",
	}
	rec.Checksum = rec.ComputeChecksum()
	line, _ := json.Marshal(rec)

	if err := os.WriteFile(path, append(line, '\n'), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	err = w.Replay(nil)
	if err == nil {
		t.Fatal("expected Replay to fail with unknown operation, got nil")
	}
}

// TestWALRecordValidationEmptyKey verifies that records with blank keys fail validation.
func TestWALRecordValidationEmptyKey(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal.log"

	rec := wal.Record{
		Version:   wal.CurrentFormatVersion,
		Sequence:  1,
		Operation: wal.OperationSet,
		Key:       "   ",
		Value:     "Anshik",
	}
	rec.Checksum = rec.ComputeChecksum()
	line, _ := json.Marshal(rec)

	if err := os.WriteFile(path, append(line, '\n'), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	err = w.Replay(nil)
	if err == nil {
		t.Fatal("expected Replay to fail with empty key, got nil")
	}
}

// TestWALRecordValidationEmptyValueAllowed verifies that SET records with empty value are valid in the WAL.
func TestWALRecordValidationEmptyValueAllowed(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal.log"

	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	if err := w.Append(wal.Record{
		Operation: wal.OperationSet,
		Key:       "key:empty",
		Value:     "",
	}); err != nil {
		t.Fatalf("Append with empty value failed: %v", err)
	}

	records, err := w.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(records) != 1 || records[0].Value != "" {
		t.Fatalf("expected 1 record with empty value, got %+v", records)
	}
}

// TestEngineApplyCommitted verifies Section 2's consensus boundary:
// Applying committed commands with Raft log indexes and ensuring idempotency.
func TestEngineApplyCommitted(t *testing.T) {
	dir := t.TempDir()
	walPath := dir + "/wal.log"

	engine, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	db := database.NewDatabase(engine)

	// Apply SET command committed at Raft index 100, term 1
	cmd1 := storage.CommittedCommand{
		RaftIndex: 100,
		RaftTerm:  1,
		Operation: wal.OperationSet,
		Key:       "cluster:leader",
		Value:     "node-a",
	}
	if err := db.ApplyCommitted(cmd1); err != nil {
		t.Fatalf("ApplyCommitted: %v", err)
	}

	val, err := db.Get("cluster:leader")
	if err != nil || val != "node-a" {
		t.Fatalf("Get: expected node-a, got %q (err: %v)", val, err)
	}
	if engine.LastAppliedIndex() != 100 {
		t.Fatalf("expected lastAppliedIndex 100, got %d", engine.LastAppliedIndex())
	}

	// Idempotency: Re-applying duplicate Raft index 100 should be an ignored no-op
	cmdDuplicate := storage.CommittedCommand{
		RaftIndex: 100,
		RaftTerm:  1,
		Operation: wal.OperationSet,
		Key:       "cluster:leader",
		Value:     "node-b-should-be-ignored",
	}
	if err := db.ApplyCommitted(cmdDuplicate); err != nil {
		t.Fatalf("ApplyCommitted duplicate: %v", err)
	}
	valAfterDuplicate, _ := db.Get("cluster:leader")
	if valAfterDuplicate != "node-a" {
		t.Fatalf("idempotency broken: duplicate index altered value to %q", valAfterDuplicate)
	}

	// Apply DELETE command committed at Raft index 101, term 1
	cmdDelete := storage.CommittedCommand{
		RaftIndex: 101,
		RaftTerm:  1,
		Operation: wal.OperationDelete,
		Key:       "cluster:leader",
	}
	if err := db.ApplyCommitted(cmdDelete); err != nil {
		t.Fatalf("ApplyCommitted delete: %v", err)
	}

	_, err = db.Get("cluster:leader")
	if !errors.Is(err, database.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound after committed delete, got %v", err)
	}

	db.Close()

	// Reopen engine and ensure replayed WAL matches final state
	engine2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("reopen NewEngine: %v", err)
	}
	defer engine2.Close()
	db2 := database.NewDatabase(engine2)

	_, err = db2.Get("cluster:leader")
	if !errors.Is(err, database.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound after replay, got %v", err)
	}
}

// TestDatabaseShutdownDrainsInFlightOperations verifies that db.Close() waits
// for all in-flight operations to complete before returning.
func TestDatabaseShutdownDrainsInFlightOperations(t *testing.T) {
	dir := t.TempDir()
	engine, err := storage.NewEngine(dir + "/wal.log")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	db := database.NewDatabase(engine)

	const totalOps = 50
	var wg sync.WaitGroup

	for i := 0; i < totalOps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", idx)
			val := fmt.Sprintf("v-%d", idx)
			_ = db.Set(key, val)
		}(i)
	}

	// Close database concurrently with operations
	db.Close()
	wg.Wait()

	// Any operations initiated after Close() has returned must be rejected with ErrClosed
	if err := db.Set("after-close", "val"); !errors.Is(err, database.ErrClosed) {
		t.Errorf("expected ErrClosed after shutdown, got %v", err)
	}
}
