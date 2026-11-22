package tests

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"orbit/internal/database"
	"orbit/internal/storage"
)

// newTestDB creates a fresh Database backed by a real storage engine
// using a temporary WAL file. The engine is closed automatically when
// the test finishes.
func newTestDB(t *testing.T) *database.Database {
	t.Helper()
	dir := t.TempDir()
	engine, err := storage.NewEngine(dir + "/wal.log")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := engine.Close(); closeErr != nil {
			t.Errorf("cleanup close engine: %v", closeErr)
		}
	})
	db, err := database.NewDatabase(engine)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	return db
}

// TestSetAndGet verifies the happy path: store a value, retrieve it.
func TestSetAndGet(t *testing.T) {
	db := newTestDB(t)

	if err := db.Set("user:1", "Anshik"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := db.Get("user:1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "Anshik" {
		t.Errorf("Get: expected %q, got %q", "Anshik", got)
	}
}

// TestGetMissingKey verifies that Get returns ErrKeyNotFound for absent keys.
func TestGetMissingKey(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Get("does-not-exist")
	if !errors.Is(err, database.ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

// TestOverwriteKey verifies that setting a key twice returns the latest value.
func TestOverwriteKey(t *testing.T) {
	db := newTestDB(t)

	if err := db.Set("user:1", "Anshik"); err != nil {
		t.Fatalf("Set first: %v", err)
	}
	if err := db.Set("user:1", "Anshika"); err != nil { // overwrite
		t.Fatalf("Set overwrite: %v", err)
	}

	got, err := db.Get("user:1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "Anshika" {
		t.Errorf("expected %q after overwrite, got %q", "Anshika", got)
	}
}

// TestDelete verifies that a deleted key is no longer retrievable.
func TestDelete(t *testing.T) {
	db := newTestDB(t)

	if err := db.Set("user:2", "Rahul"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := db.Delete("user:2"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := db.Get("user:2")
	if !errors.Is(err, database.ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound after delete, got %v", err)
	}
}

// TestDeleteNonExistent verifies that deleting a missing key is a no-op (not an error).
func TestDeleteNonExistent(t *testing.T) {
	db := newTestDB(t)

	if err := db.Delete("ghost-key"); err != nil {
		t.Errorf("Delete of non-existent key should not error, got: %v", err)
	}
}

// TestEmptyKeyRejected verifies that blank keys are rejected with ErrEmptyKey.
func TestEmptyKeyRejected(t *testing.T) {
	db := newTestDB(t)

	if err := db.Set("", "value"); !errors.Is(err, database.ErrEmptyKey) {
		t.Errorf("Set with empty key: expected ErrEmptyKey, got %v", err)
	}
	if err := db.Set("   ", "value"); !errors.Is(err, database.ErrEmptyKey) {
		t.Errorf("Set with whitespace key: expected ErrEmptyKey, got %v", err)
	}
	if _, err := db.Get(""); !errors.Is(err, database.ErrEmptyKey) {
		t.Errorf("Get with empty key: expected ErrEmptyKey, got %v", err)
	}
	if err := db.Delete(""); !errors.Is(err, database.ErrEmptyKey) {
		t.Errorf("Delete with empty key: expected ErrEmptyKey, got %v", err)
	}
}

// TestEmptyValueAllowed verifies that empty values are permitted under standard KV semantics.
func TestEmptyValueAllowed(t *testing.T) {
	db := newTestDB(t)

	if err := db.Set("key:empty", ""); err != nil {
		t.Fatalf("Set with empty value failed: %v", err)
	}

	got, err := db.Get("key:empty")
	if err != nil {
		t.Fatalf("Get with empty value failed: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}

	// Whitespace-only values are also valid raw values
	if err := db.Set("key:spaces", "   "); err != nil {
		t.Fatalf("Set with spaces failed: %v", err)
	}
	gotSpaces, err := db.Get("key:spaces")
	if err != nil || gotSpaces != "   " {
		t.Errorf("expected 3 spaces, got %q", gotSpaces)
	}
}

// TestMultipleKeys verifies independent keys don't interfere with each other.
func TestMultipleKeys(t *testing.T) {
	db := newTestDB(t)

	entries := map[string]string{
		"user:1": "Anshik",
		"user:2": "Rahul",
		"user:3": "Kunal",
	}

	for k, v := range entries {
		if err := db.Set(k, v); err != nil {
			t.Fatalf("Set(%q): %v", k, err)
		}
	}

	for k, want := range entries {
		got, err := db.Get(k)
		if err != nil {
			t.Fatalf("Get(%q): %v", k, err)
		}
		if got != want {
			t.Errorf("Get(%q) = %q, want %q", k, got, want)
		}
	}
}

// TestConcurrentDatabaseWritesAndReads verifies that simultaneous writers and readers
// preserve consistency and have zero data races under the Go race detector.
func TestConcurrentDatabaseWritesAndReads(t *testing.T) {
	db := newTestDB(t)

	const writers = 8
	const opsPerWriter = 25
	var wg sync.WaitGroup

	// Start concurrent writers
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			for i := 0; i < opsPerWriter; i++ {
				key := fmt.Sprintf("key-%d", i%5) // high collision across writers
				val := fmt.Sprintf("val-%d-%d", wid, i)
				if err := db.Set(key, val); err != nil {
					t.Errorf("Set error: %v", err)
				}
			}
		}(w)
	}

	// Start concurrent readers
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerWriter*2; i++ {
				key := fmt.Sprintf("key-%d", i%5)
				_, err := db.Get(key)
				if err != nil && !errors.Is(err, database.ErrKeyNotFound) {
					t.Errorf("Get error during concurrent reads: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}

// TestNewDatabaseNilEngineRejected verifies constructor contract enforcement.
func TestNewDatabaseNilEngineRejected(t *testing.T) {
	// Raw nil
	db, err := database.NewDatabase(nil)
	if !errors.Is(err, database.ErrNilEngine) {
		t.Fatalf("expected ErrNilEngine for raw nil, got db=%v, err=%v", db, err)
	}

	// Typed nil pointer
	var nilEngine *storage.Engine = nil
	db2, err := database.NewDatabase(nilEngine)
	if !errors.Is(err, database.ErrNilEngine) {
		t.Fatalf("expected ErrNilEngine for typed nil pointer, got db=%v, err=%v", db2, err)
	}
}
