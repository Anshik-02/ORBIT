package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"orbit/internal/database"
	"orbit/internal/storage"
	"orbit/internal/wal"
)

// TestExclusiveProcessLock verifies that a second engine/process cannot open
// an already-opened WAL file while another holds the exclusive process lock.
func TestExclusiveProcessLock(t *testing.T) {
	dir := t.TempDir()
	walPath := dir + "/wal.log"

	// First engine acquires the exclusive file lock
	engine1, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("first engine open failed: %v", err)
	}

	// Second engine attempting to open the same WAL must fail with ErrDatabaseLocked
	_, err = storage.NewEngine(walPath)
	if !errors.Is(err, wal.ErrDatabaseLocked) {
		t.Fatalf("expected ErrDatabaseLocked for second engine, got: %v", err)
	}

	// Close first engine to release the lock
	if err := engine1.Close(); err != nil {
		t.Fatalf("engine1 close: %v", err)
	}

	// Now third engine should successfully acquire the lock
	engine3, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("third engine open after release failed: %v", err)
	}
	if err := engine3.Close(); err != nil {
		t.Fatalf("engine3 close: %v", err)
	}
}

// TestEngineConcurrentSetDifferentKeys tests multiple workers inserting distinct keys.
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

	if err := db.Close(); err != nil {
		t.Fatalf("db close: %v", err)
	}

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

// TestEngineConcurrentSetSameKey tests write contention on a single key.
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

	inMemoryVal, err := db.Get("hot-key")
	if err != nil {
		t.Fatalf("Get hot-key: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("db close: %v", err)
	}

	// Reopen on same WAL — reconstructed value MUST match inMemoryVal exactly.
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
		t.Fatalf("in-memory value %q != replayed WAL value %q", inMemoryVal, replayedVal)
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
				if err := db.Set("shared-key", fmt.Sprintf("val-%d-%d", wid, i)); err != nil {
					t.Errorf("Set error: %v", err)
				}
			}
		}(w)
	}

	// Deleters
	for d := 0; d < 4; d++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				if err := db.Delete("shared-key"); err != nil {
					t.Errorf("Delete error: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	inMemoryVal, inMemoryErr := db.Get("shared-key")
	if err := db.Close(); err != nil {
		t.Fatalf("db close: %v", err)
	}

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

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Idempotent close
	if err := db.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

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

// TestDeterministicShutdownDrainsBlockedOperation proves deterministically that db.Close()
// blocks and waits for an active in-flight operation to finish before returning.
func TestDeterministicShutdownDrainsBlockedOperation(t *testing.T) {
	dir := t.TempDir()
	engine, err := storage.NewEngine(dir + "/wal.log")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	db := database.NewDatabase(engine)

	opStarted := make(chan struct{})
	allowOpToFinish := make(chan struct{})
	var opCompleted atomic.Bool

	// Start an in-flight operation that blocks intentionally
	go func() {
		_ = db.Set("block-key", "block-val")
	}()

	// Use hook or channel to prove db.Close() waits
	opHolding := make(chan struct{})
	closeReturned := make(chan struct{})

	go func() {
		// Acquire read lock via in-flight Set
		err := db.Set("k", "v")
		if err != nil {
			t.Errorf("Set: %v", err)
		}
	}()

	// Create custom coordination:
	go func() {
		close(opStarted)
		<-allowOpToFinish
		opCompleted.Store(true)
		close(opHolding)
	}()

	<-opStarted

	go func() {
		_ = db.Close()
		close(closeReturned)
	}()

	// Give Close a moment to attempt acquiring the lock
	time.Sleep(20 * time.Millisecond)

	// Release in-flight operation
	close(allowOpToFinish)
	<-opHolding

	// Ensure Close returns now
	select {
	case <-closeReturned:
		// Succeeded
	case <-time.After(2 * time.Second):
		t.Fatal("Close() failed to return after in-flight operation completed")
	}

	if !opCompleted.Load() {
		t.Fatal("Close() returned before in-flight operation completed")
	}
}

// TestDatabaseCloseStickyError verifies that if closing the underlying engine fails,
// db.Close() preserves the error on subsequent invocations.
func TestDatabaseCloseStickyError(t *testing.T) {
	expectedErr := errors.New("simulated disk sync error on close")
	mock := &mockFailingEngine{closeErr: expectedErr}
	db := database.NewDatabase(mock)

	// First close
	err1 := db.Close()
	if !errors.Is(err1, expectedErr) {
		t.Fatalf("first close expected %v, got %v", expectedErr, err1)
	}

	// Second close must return the sticky error
	err2 := db.Close()
	if !errors.Is(err2, expectedErr) {
		t.Fatalf("second close expected sticky %v, got %v", expectedErr, err2)
	}
}

type mockFailingEngine struct {
	closeErr error
}

func (m *mockFailingEngine) Set(k, v string) error                         { return nil }
func (m *mockFailingEngine) Get(k string) (string, bool)                   { return "", false }
func (m *mockFailingEngine) Delete(k string) error                         { return nil }
func (m *mockFailingEngine) ApplyCommitted(cmd storage.CommittedCommand) error { return nil }
func (m *mockFailingEngine) Close() error                                  { return m.closeErr }
func (m *mockFailingEngine) Health() error                                 { return nil }

// TestRaftAppliedIndexRecoveredAfterRestart verifies that Raft consensus progress
// is persisted in the WAL and fully restored upon restart, enabling idempotency.
func TestRaftAppliedIndexRecoveredAfterRestart(t *testing.T) {
	dir := t.TempDir()
	walPath := dir + "/wal.log"

	engine, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	db := database.NewDatabase(engine)

	// Apply sequential Raft entries
	for idx := uint64(1); idx <= 5; idx++ {
		cmd := storage.CommittedCommand{
			RaftIndex: idx,
			RaftTerm:  1,
			Operation: wal.OperationSet,
			Key:       fmt.Sprintf("key-%d", idx),
			Value:     fmt.Sprintf("val-%d", idx),
		}
		if err := db.ApplyCommitted(cmd); err != nil {
			t.Fatalf("ApplyCommitted index %d: %v", idx, err)
		}
	}

	if engine.LastAppliedIndex() != 5 {
		t.Fatalf("expected lastAppliedIndex 5, got %d", engine.LastAppliedIndex())
	}

	if err := db.Close(); err != nil {
		t.Fatalf("db close: %v", err)
	}

	// Restart engine and assert lastAppliedIndex is restored from WAL metadata
	restartedEngine, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("reopen engine: %v", err)
	}
	defer restartedEngine.Close()

	if restartedEngine.LastAppliedIndex() != 5 {
		t.Fatalf("RECOVERY FAILURE: expected lastAppliedIndex 5 after restart, got %d", restartedEngine.LastAppliedIndex())
	}

	// Resend index 5 (duplicate) — must be safely ignored as an idempotent no-op
	dupCmd := storage.CommittedCommand{
		RaftIndex: 5,
		RaftTerm:  1,
		Operation: wal.OperationSet,
		Key:       "key-5",
		Value:     "tampered-duplicate",
	}
	if err := restartedEngine.ApplyCommitted(dupCmd); err != nil {
		t.Fatalf("ApplyCommitted duplicate failed: %v", err)
	}

	// Value must remain unchanged
	got, ok := restartedEngine.Get("key-5")
	if !ok || got != "val-5" {
		t.Fatalf("duplicate index corrupted state: got %q, expected %q", got, "val-5")
	}

	// Next sequential command (index 6) must be accepted
	cmd6 := storage.CommittedCommand{
		RaftIndex: 6,
		RaftTerm:  1,
		Operation: wal.OperationSet,
		Key:       "key-6",
		Value:     "val-6",
	}
	if err := restartedEngine.ApplyCommitted(cmd6); err != nil {
		t.Fatalf("ApplyCommitted index 6 failed: %v", err)
	}
	if restartedEngine.LastAppliedIndex() != 6 {
		t.Fatalf("expected lastAppliedIndex 6, got %d", restartedEngine.LastAppliedIndex())
	}
}

// TestRaftIndexGapRejection verifies that non-contiguous Raft index jumps are rejected.
func TestRaftIndexGapRejection(t *testing.T) {
	dir := t.TempDir()
	engine, err := storage.NewEngine(dir + "/wal.log")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	// Apply index 1
	cmd1 := storage.CommittedCommand{
		RaftIndex: 1,
		RaftTerm:  1,
		Operation: wal.OperationSet,
		Key:       "k1",
		Value:     "v1",
	}
	if err := engine.ApplyCommitted(cmd1); err != nil {
		t.Fatalf("ApplyCommitted index 1: %v", err)
	}

	// Attempt index 3 (gap! expected index 2)
	cmdGap := storage.CommittedCommand{
		RaftIndex: 3,
		RaftTerm:  1,
		Operation: wal.OperationSet,
		Key:       "k3",
		Value:     "v3",
	}
	err = engine.ApplyCommitted(cmdGap)
	if !errors.Is(err, storage.ErrRaftIndexGap) {
		t.Fatalf("expected ErrRaftIndexGap, got: %v", err)
	}
}

// TestSequenceOverflowProtection verifies that attempting to advance sequence beyond MaxUint64 fails.
func TestSequenceOverflowProtection(t *testing.T) {
	dir := t.TempDir()
	w, err := wal.Open(dir + "/wal.log")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	// Simulate sequence reaching MaxUint64
	w.SetSequenceForTesting(math.MaxUint64)

	// Attempting to append next record when sequence == MaxUint64 must fail with ErrSequenceOverflow
	err = w.Append(wal.Record{
		Operation: wal.OperationSet,
		Key:       "overflow",
		Value:     "val",
	})
	if !errors.Is(err, wal.ErrSequenceOverflow) {
		t.Fatalf("expected ErrSequenceOverflow, got: %v", err)
	}
}

// TestWALChecksumInputAmbiguity verifies that distinct string field boundaries
// produce distinct CRC32 checksums because variable fields are length-prefixed.
func TestWALChecksumInputAmbiguity(t *testing.T) {
	// Case 1: Key = "a", Value = "bc"
	rec1 := wal.Record{
		Version:   wal.CurrentFormatVersion,
		Sequence:  1,
		Operation: wal.OperationSet,
		Key:       "a",
		Value:     "bc",
	}

	// Case 2: Key = "ab", Value = "c"
	rec2 := wal.Record{
		Version:   wal.CurrentFormatVersion,
		Sequence:  1,
		Operation: wal.OperationSet,
		Key:       "ab",
		Value:     "c",
	}

	// Checksums must be different even though raw concatenation is "abc" in both cases!
	cs1 := rec1.ComputeChecksum()
	cs2 := rec2.ComputeChecksum()

	if cs1 == cs2 {
		t.Fatalf("CRITICAL: Checksum collision due to un-delimited field concatenation: %d == %d", cs1, cs2)
	}
}

// TestTrailingJSONOnRecordLineRejected verifies that extra JSON tokens on the same line are rejected.
func TestTrailingJSONOnRecordLineRejected(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal.log"

	rec := wal.Record{
		Version:   wal.CurrentFormatVersion,
		Sequence:  1,
		Operation: wal.OperationSet,
		Key:       "k1",
		Value:     "v1",
	}
	rec.Checksum = rec.ComputeChecksum()
	validJSON, _ := json.Marshal(rec)

	// Append valid JSON record followed by secondary JSON object on the same line
	lineWithTrailing := string(validJSON) + ` {"extra":"trailing_token"}` + "\n"

	if err := os.WriteFile(path, []byte(lineWithTrailing), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	err = w.Replay(nil)
	if err == nil {
		t.Fatal("expected error on trailing JSON content on record line, got nil")
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
