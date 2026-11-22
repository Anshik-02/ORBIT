package tests

import (
	"fmt"
	"os"
	"sync"
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

// TestWALPartialWrite verifies that an incomplete trailing write is skipped
// and the file is truncated back to the clean record boundary.
func TestWALPartialWrite(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal.log"

	// Write a valid record followed by an incomplete line (no newline).
	validLine := `{"sequence":1,"operation":"SET","key":"user:1","value":"Anshik"}` + "\n"
	truncatedLine := `{"sequence":2,"operation":"SET","key":"use`

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
		t.Fatalf("Replay failed on partial line: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Key != "user:1" {
		t.Errorf("expected key user:1, got %q", records[0].Key)
	}

	// Verify file was physically truncated to the valid line length.
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.Size() != int64(len(validLine)) {
		t.Errorf("expected file size %d after truncation, got %d", len(validLine), stat.Size())
	}
}

// TestWALSequenceGapDetected verifies that non-contiguous sequences (e.g. 1 -> 3)
// are detected as corruptions during replay.
func TestWALSequenceGapDetected(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal.log"

	// Sequence 1 followed by sequence 3 (gap!)
	content := `{"sequence":1,"operation":"SET","key":"k1","value":"v1"}` + "\n" +
		`{"sequence":3,"operation":"SET","key":"k2","value":"v2"}` + "\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := wal.Open(path)
	if err == nil {
		t.Fatal("expected error on sequence gap, got nil")
	}
}

// TestWALSequenceDuplicateDetected verifies that duplicate sequence numbers are rejected.
func TestWALSequenceDuplicateDetected(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal.log"

	content := `{"sequence":1,"operation":"SET","key":"k1","value":"v1"}` + "\n" +
		`{"sequence":1,"operation":"SET","key":"k2","value":"v2"}` + "\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := wal.Open(path)
	if err == nil {
		t.Fatal("expected error on duplicate sequence, got nil")
	}
}

// TestWALCorruptRecordRejected verifies that a complete line with invalid JSON
// is rejected and not ignored.
func TestWALCorruptRecordRejected(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wal.log"

	content := `{"sequence":1,"operation":"SET","key":"k1","value":"v1"}` + "\n" +
		`{"corrupted": not-a-json}` + "\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := wal.Open(path)
	if err == nil {
		t.Fatal("expected error on corrupt record, got nil")
	}
}

// TestWALConcurrentAppends tests concurrent writes directly against the WAL,
// ensuring no interleaved bytes or race conditions occur.
func TestWALConcurrentAppends(t *testing.T) {
	dir := t.TempDir()
	w, err := wal.Open(dir + "/wal.log")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	const numGoroutines = 10
	const writesPerGoroutine = 20
	var wg sync.WaitGroup

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < writesPerGoroutine; i++ {
				rec := wal.Record{
					Operation: wal.OperationSet,
					Key:       fmt.Sprintf("k-%d-%d", gid, i),
					Value:     fmt.Sprintf("v-%d-%d", gid, i),
				}
				if err := w.Append(rec); err != nil {
					t.Errorf("Append failed: %v", err)
				}
			}
		}(g)
	}

	wg.Wait()

	// Replay and verify all records are present with contiguous sequences
	records, err := w.Replay()
	if err != nil {
		t.Fatalf("Replay after concurrent writes failed: %v", err)
	}

	expectedTotal := numGoroutines * writesPerGoroutine
	if len(records) != expectedTotal {
		t.Fatalf("expected %d records, got %d", expectedTotal, len(records))
	}

	for i, r := range records {
		expectedSeq := uint64(i + 1)
		if r.Sequence != expectedSeq {
			t.Errorf("record[%d]: sequence = %d, want %d", i, r.Sequence, expectedSeq)
		}
	}
}
