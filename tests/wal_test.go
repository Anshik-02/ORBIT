package tests

import (
	"os"
	"testing"

	"orbit/internal/wal"
)

// TestWALAppendAndReplay verifies that records written to the WAL
// can be read back correctly in sequence order.
func TestWALAppendAndReplay(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal.log"

	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	records := []wal.Record{
		{Operation: wal.OperationSet, Key: "user:1", Value: "Anshik"},
		{Operation: wal.OperationSet, Key: "user:2", Value: "Rahul"},
		{Operation: wal.OperationDelete, Key: "user:1"},
	}

	for _, r := range records {
		if err := w.Append(r); err != nil {
			t.Fatalf("Append(%q): %v", r.Key, err)
		}
	}

	got, err := w.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}

	if len(got) != len(records) {
		t.Fatalf("expected %d records, got %d", len(records), len(got))
	}

	for i, r := range got {
		if r.Operation != records[i].Operation {
			t.Errorf("record[%d]: operation = %q, want %q", i, r.Operation, records[i].Operation)
		}
		if r.Key != records[i].Key {
			t.Errorf("record[%d]: key = %q, want %q", i, r.Key, records[i].Key)
		}
		if r.Value != records[i].Value {
			t.Errorf("record[%d]: value = %q, want %q", i, r.Value, records[i].Value)
		}
	}
}

// TestWALSequenceIsMonotonic verifies that each appended record gets
// a sequence number strictly greater than the previous one.
func TestWALSequenceIsMonotonic(t *testing.T) {
	dir := t.TempDir()
	w, err := wal.Open(dir + "/wal.log")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	for i := 0; i < 10; i++ {
		if err := w.Append(wal.Record{Operation: wal.OperationSet, Key: "k", Value: "v"}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	records, err := w.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}

	for i := 1; i < len(records); i++ {
		if records[i].Sequence <= records[i-1].Sequence {
			t.Errorf("sequence not monotonic at index %d: %d <= %d",
				i, records[i].Sequence, records[i-1].Sequence)
		}
	}
}

// TestWALDeleteOmitsValue verifies that DELETE records do not carry a
// "value" field — the omitempty JSON tag should ensure this.
func TestWALDeleteOmitsValue(t *testing.T) {
	dir := t.TempDir()
	w, err := wal.Open(dir + "/wal.log")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	if err := w.Append(wal.Record{Operation: wal.OperationDelete, Key: "user:1"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Read the raw log line and verify no "value" field is present.
	raw, err := os.ReadFile(dir + "/wal.log")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	line := string(raw)
	if contains := func(s, sub string) bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}; contains(line, `"value"`) {
		t.Errorf("DELETE record should not contain a value field, got: %s", line)
	}
}

// TestWALPartialWrite simulates a partial (truncated) final line,
// which is what happens when the process crashes mid-write.
// Replay must succeed and return only the complete records.
func TestWALPartialWrite(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal.log"

	// Write a valid record manually, then append a truncated line.
	validLine := `{"sequence":1,"operation":"SET","key":"user:1","value":"Anshik"}` + "\n"
	truncatedLine := `{"sequence":2,"operation":"SET","key":"use` // no closing brace, no newline

	if err := os.WriteFile(path, []byte(validLine+truncatedLine), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	records, err := w.Replay()
	if err != nil {
		t.Fatalf("Replay should not error on partial final line, got: %v", err)
	}

	// Only the complete first record should be returned.
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Key != "user:1" {
		t.Errorf("expected key user:1, got %q", records[0].Key)
	}
}
