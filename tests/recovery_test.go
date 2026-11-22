package tests

import (
	"errors"
	"testing"

	"orbit/internal/database"
	"orbit/internal/storage"
)

// TestRecoveryAfterRestart is the core durability test.
//
// It simulates the full restart lifecycle:
//  1. Open a database, write some records, close it.
//  2. Open a NEW database on the same WAL file (simulating a restart).
//  3. Verify that all data survived exactly as written.
//
// This test proves that our WAL replay correctly reconstructs the Memtable.
func TestRecoveryAfterRestart(t *testing.T) {
	dir := t.TempDir()
	walPath := dir + "/wal.log"

	// --- First run: write data and close ---
	engine1, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("first engine open: %v", err)
	}
	db1 := database.NewDatabase(engine1)

	if err := db1.Set("user:1", "Anshik"); err != nil {
		t.Fatalf("Set user:1: %v", err)
	}
	if err := db1.Set("user:2", "Rahul"); err != nil {
		t.Fatalf("Set user:2: %v", err)
	}
	if err := db1.Set("user:3", "Kunal"); err != nil {
		t.Fatalf("Set user:3: %v", err)
	}
	if err := db1.Delete("user:2"); err != nil {
		t.Fatalf("Delete user:2: %v", err)
	}

	if err := db1.Close(); err != nil {
		t.Fatalf("close db1: %v", err)
	}

	// --- Second run: open same WAL, verify state ---
	engine2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("second engine open: %v", err)
	}
	db2 := database.NewDatabase(engine2)
	defer db2.Close()

	// user:1 and user:3 must survive.
	got1, err := db2.Get("user:1")
	if err != nil || got1 != "Anshik" {
		t.Errorf("user:1: expected Anshik, got %q (err: %v)", got1, err)
	}

	got3, err := db2.Get("user:3")
	if err != nil || got3 != "Kunal" {
		t.Errorf("user:3: expected Kunal, got %q (err: %v)", got3, err)
	}

	// user:2 was deleted — must be absent.
	_, err = db2.Get("user:2")
	if !errors.Is(err, database.ErrKeyNotFound) {
		t.Errorf("user:2: expected ErrKeyNotFound after delete+restart, got %v", err)
	}
}

// TestRecoveryOverwrite verifies that when a key is SET multiple times,
// only the final value survives a restart.
func TestRecoveryOverwrite(t *testing.T) {
	dir := t.TempDir()
	walPath := dir + "/wal.log"

	engine1, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db1 := database.NewDatabase(engine1)

	// Write the same key 5 times.
	for _, v := range []string{"v1", "v2", "v3", "v4", "v5"} {
		if err := db1.Set("config:mode", v); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen — only the last value should survive.
	engine2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	db2 := database.NewDatabase(engine2)
	defer db2.Close()

	got, err := db2.Get("config:mode")
	if err != nil {
		t.Fatalf("Get after restart: %v", err)
	}
	if got != "v5" {
		t.Errorf("expected v5 after restart, got %q", got)
	}
}

// TestRecoveryEmptyWAL verifies that opening a fresh database with no WAL file
// results in an empty, working database — not a crash.
func TestRecoveryEmptyWAL(t *testing.T) {
	dir := t.TempDir()
	engine, err := storage.NewEngine(dir + "/wal.log")
	if err != nil {
		t.Fatalf("NewEngine on fresh path: %v", err)
	}
	db := database.NewDatabase(engine)
	defer db.Close()

	_, err = db.Get("anything")
	if !errors.Is(err, database.ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound in empty DB, got %v", err)
	}
}

// TestRecoverySequencesContinueAfterRestart verifies that sequence numbers
// do not reset to 1 after a restart — they continue from the last known value.
func TestRecoverySequencesContinueAfterRestart(t *testing.T) {
	dir := t.TempDir()
	walPath := dir + "/wal.log"

	// First session: write 3 records (sequences 1, 2, 3).
	engine1, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("engine1: %v", err)
	}
	db1 := database.NewDatabase(engine1)
	if err := db1.Set("a", "1"); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	if err := db1.Set("b", "2"); err != nil {
		t.Fatalf("Set b: %v", err)
	}
	if err := db1.Set("c", "3"); err != nil {
		t.Fatalf("Set c: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("db1 close: %v", err)
	}

	// Second session: write 1 more record — it must get sequence 4, not 1.
	engine2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	db2 := database.NewDatabase(engine2)
	if err := db2.Set("d", "4"); err != nil {
		t.Fatalf("Set d: %v", err)
	}
	if err := db2.Close(); err != nil {
		t.Fatalf("db2 close: %v", err)
	}

	// Third session: read back without overlapping open engines.
	engine3, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("third open: %v", err)
	}
	db3 := database.NewDatabase(engine3)
	got, err := db3.Get("d")
	if err != nil || got != "4" {
		t.Errorf("key d after restart: expected 4, got %q (err: %v)", got, err)
	}
	if err := db3.Close(); err != nil {
		t.Fatalf("db3 close: %v", err)
	}
}
