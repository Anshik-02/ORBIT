package tests

import (
	"errors"
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
	t.Cleanup(func() { engine.Close() })
	return database.NewDatabase(engine)
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

	db.Set("user:1", "Anshik")
	db.Set("user:1", "Anshika") // overwrite

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

	db.Set("user:2", "Rahul")

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

// TestEmptyValueRejected verifies that blank values are rejected with ErrEmptyValue.
func TestEmptyValueRejected(t *testing.T) {
	db := newTestDB(t)

	if err := db.Set("user:1", ""); !errors.Is(err, database.ErrEmptyValue) {
		t.Errorf("Set with empty value: expected ErrEmptyValue, got %v", err)
	}
	if err := db.Set("user:1", "   "); !errors.Is(err, database.ErrEmptyValue) {
		t.Errorf("Set with whitespace value: expected ErrEmptyValue, got %v", err)
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
