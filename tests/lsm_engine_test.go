package tests

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"orbit/internal/storage"
	"orbit/internal/wal"
)

// TestMemtable_SizeTrackingAndTombstones verifies accurate memory accounting,
// tombstone behavior, and sorted extraction in Memtable.
func TestMemtable_SizeTrackingAndTombstones(t *testing.T) {
	m := storage.NewMemtable()

	if m.SizeBytes() != 0 || m.Len() != 0 {
		t.Fatalf("expected empty memtable, got size=%d len=%d", m.SizeBytes(), m.Len())
	}

	// 1. Add key-value pairs
	m.Set("k1", "value-one")
	initialSize := m.SizeBytes()
	if initialSize <= 0 {
		t.Fatalf("expected size > 0 after insert, got %d", initialSize)
	}

	m.Set("k2", "value-two")
	secondSize := m.SizeBytes()
	if secondSize <= initialSize {
		t.Fatalf("expected size to grow, got %d <= %d", secondSize, initialSize)
	}

	// 2. Overwrite with shorter value
	m.Set("k2", "v")
	shorterSize := m.SizeBytes()
	if shorterSize >= secondSize {
		t.Fatalf("expected size to shrink with shorter value, got %d >= %d", shorterSize, secondSize)
	}

	// 3. Delete key (tombstone)
	m.Delete("k1")
	if _, ok := m.Get("k1"); ok {
		t.Fatal("deleted key should return ok=false from Get")
	}
	entry, found := m.GetEntry("k1")
	if !found || !entry.IsTombstone {
		t.Fatalf("expected tombstone entry for k1, got %+v (found=%v)", entry, found)
	}

	// 4. Sorted extraction
	m.Set("apple", "fruit")
	m.Set("zebra", "animal")
	m.Set("banana", "fruit")

	sorted := m.EntriesSorted()
	expectedOrder := []string{"apple", "banana", "k1", "k2", "zebra"}
	if len(sorted) != len(expectedOrder) {
		t.Fatalf("expected %d sorted entries, got %d", len(expectedOrder), len(sorted))
	}
	for i, expKey := range expectedOrder {
		if sorted[i].Key != expKey {
			t.Fatalf("sorted position %d: expected %q, got %q", i, expKey, sorted[i].Key)
		}
	}
}

// TestBloomFilter_AccuracyAndSerialization verifies bloom filter insertion,
// containment checks, false-positive bounds, and CRC32 serialization roundtrip.
func TestBloomFilter_AccuracyAndSerialization(t *testing.T) {
	const n = 1000
	bf := storage.NewBloomFilter(n, 0.01)

	for i := 0; i < n; i++ {
		bf.Add(fmt.Sprintf("key-%05d", i))
	}

	// 1. All inserted keys must return MayContain == true (0 false negatives)
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("key-%05d", i)
		if !bf.MayContain(k) {
			t.Fatalf("false negative for key %q", k)
		}
	}

	// 2. Non-existent keys false-positive rate must be <= 3%
	falsePositives := 0
	const testMissing = 10000
	for i := 0; i < testMissing; i++ {
		k := fmt.Sprintf("nonexistent-key-%05d", i)
		if bf.MayContain(k) {
			falsePositives++
		}
	}
	fpRate := float64(falsePositives) / float64(testMissing)
	t.Logf("Empirical Bloom filter false-positive rate: %.4f (%d/%d)", fpRate, falsePositives, testMissing)
	if fpRate > 0.03 {
		t.Fatalf("false-positive rate %.4f exceeds acceptable bound (3%%)", fpRate)
	}

	// 3. Serialization and deserialization roundtrip
	encoded := bf.Encode()
	if len(encoded) == 0 {
		t.Fatal("encoded bloom filter is empty")
	}

	decoded, err := storage.DecodeBloomFilter(encoded)
	if err != nil {
		t.Fatalf("decode bloom filter: %v", err)
	}

	// Verify decoded filter preserves all memberships
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("key-%05d", i)
		if !decoded.MayContain(k) {
			t.Fatalf("decoded filter false negative for key %q", k)
		}
	}

	// 4. Bitrot detection: corrupt a byte in the encoded payload
	corrupted := make([]byte, len(encoded))
	copy(corrupted, encoded)
	corrupted[10] ^= 0xFF
	_, err = storage.DecodeBloomFilter(corrupted)
	if err == nil {
		t.Fatal("expected DecodeBloomFilter to fail on corrupted payload, got nil")
	}
	if !errors.Is(err, storage.ErrCorruptBloomFilter) {
		t.Fatalf("expected ErrCorruptBloomFilter, got: %v", err)
	}
}

// TestSSTable_WriterReaderRoundtrip tests writing sorted entries, sparse indexing,
// and binary search reads in an SSTable.
func TestSSTable_WriterReaderRoundtrip(t *testing.T) {
	dir := t.TempDir()
	sstPath := filepath.Join(dir, "000001.sst")

	writer, err := storage.NewSSTableWriter(sstPath, 0, 4, 0.01)
	if err != nil {
		t.Fatalf("NewSSTableWriter: %v", err)
	}

	const count = 50
	for i := 0; i < count; i++ {
		k := fmt.Sprintf("key-%04d", i)
		v := fmt.Sprintf("value-%04d", i)
		isTomb := (i%10 == 0) // mark every 10th key as tombstone
		if err := writer.Append(storage.MemtableEntry{
			Key:         k,
			Value:       v,
			IsTombstone: isTomb,
		}); err != nil {
			t.Fatalf("writer.Append %d: %v", i, err)
		}
	}

	meta, err := writer.Finish()
	if err != nil {
		t.Fatalf("writer.Finish: %v", err)
	}
	if meta.NumEntries != count {
		t.Fatalf("expected %d entries, got %d", count, meta.NumEntries)
	}

	// Verify temporary file was cleaned up and .sst exists
	if _, err := os.Stat(sstPath + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("temporary file .tmp should not exist after Finish()")
	}
	if _, err := os.Stat(sstPath); err != nil {
		t.Fatalf("expected sstable file %s to exist: %v", sstPath, err)
	}

	// Open Reader
	reader, err := storage.OpenSSTableReader(sstPath)
	if err != nil {
		t.Fatalf("OpenSSTableReader: %v", err)
	}
	defer reader.Close()

	// Verify all keys
	for i := 0; i < count; i++ {
		k := fmt.Sprintf("key-%04d", i)
		val, found, isTomb, err := reader.Get(k)
		if err != nil {
			t.Fatalf("Get %q: %v", k, err)
		}
		if !found {
			t.Fatalf("key %q not found in sstable", k)
		}

		if i%10 == 0 {
			if !isTomb {
				t.Fatalf("key %q expected tombstone, got normal", k)
			}
		} else {
			if isTomb {
				t.Fatalf("key %q expected normal, got tombstone", k)
			}
			expectedVal := fmt.Sprintf("value-%04d", i)
			if val != expectedVal {
				t.Fatalf("key %q: expected value %q, got %q", k, expectedVal, val)
			}
		}
	}

	// Missing keys should return found=false
	missingVal, found, _, err := reader.Get("missing-key")
	if err != nil || found || missingVal != "" {
		t.Fatalf("expected missing key not found, got val=%q found=%v err=%v", missingVal, found, err)
	}

	// Test AllEntries()
	allEntries, err := reader.AllEntries()
	if err != nil {
		t.Fatalf("AllEntries: %v", err)
	}
	if len(allEntries) != count {
		t.Fatalf("expected %d entries from AllEntries(), got %d", count, len(allEntries))
	}
}

// TestSSTable_RejectOutOfOrderKeys verifies that SSTableWriter rejects unsorted inputs.
func TestSSTable_RejectOutOfOrderKeys(t *testing.T) {
	dir := t.TempDir()
	sstPath := filepath.Join(dir, "out_of_order.sst")

	writer, err := storage.NewSSTableWriter(sstPath, 0, 4, 0.01)
	if err != nil {
		t.Fatalf("NewSSTableWriter: %v", err)
	}
	defer writer.Finish()

	if err := writer.Append(storage.MemtableEntry{Key: "zebra", Value: "1"}); err != nil {
		t.Fatalf("first append: %v", err)
	}

	// Appending key alphabetically before "zebra" must fail
	err = writer.Append(storage.MemtableEntry{Key: "apple", Value: "2"})
	if err == nil {
		t.Fatal("expected writer.Append to reject out-of-order key, got nil")
	}
}

// TestLSMEngine_MemtableRotationAndFlush verifies that when active memtable reaches
// the configured size limit, it rotates to immutable and flushes to an SSTable.
func TestLSMEngine_MemtableRotationAndFlush(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	engine, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	// Set small memtable threshold (256 bytes) to trigger rotations quickly
	engine.SetMemtableSizeLimit(256)

	// Insert entries to trigger rotation and flushing
	const count = 40
	for i := 0; i < count; i++ {
		k := fmt.Sprintf("user-%03d", i)
		v := fmt.Sprintf("profile-data-payload-%03d", i)
		if err := engine.Set(k, v); err != nil {
			t.Fatalf("Set %d: %v", i, err)
		}
	}

	// Force synchronous flush of any remaining immutable memtables
	if err := engine.Flush(); err != nil {
		t.Fatalf("engine.Flush(): %v", err)
	}

	// Must have created at least one SSTable on disk
	if engine.SSTablesCount() == 0 {
		t.Fatal("expected at least 1 SSTable to be created after rotations and flush")
	}
	t.Logf("Created %d SSTables after inserting %d items with 256-byte limit", engine.SSTablesCount(), count)

	// All keys must be readable through the LSM hierarchy
	for i := 0; i < count; i++ {
		k := fmt.Sprintf("user-%03d", i)
		expectedVal := fmt.Sprintf("profile-data-payload-%03d", i)
		val, ok := engine.Get(k)
		if !ok || val != expectedVal {
			t.Fatalf("key %q: expected %q, got %q (ok=%v)", k, expectedVal, val, ok)
		}
	}
}

// TestLSMEngine_TombstoneShadowsOlderSSTable verifies that deleting a key in a newer
// tier (memtable) correctly shadows an older value persisted in an SSTable.
func TestLSMEngine_TombstoneShadowsOlderSSTable(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	engine, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	// 1. Write key and flush to SSTable
	if err := engine.Set("target-key", "persisted-old-value"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	engine.RotateMemtable()
	if err := engine.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if engine.SSTablesCount() != 1 {
		t.Fatalf("expected 1 SSTable, got %d", engine.SSTablesCount())
	}

	// Verify key is readable from SSTable
	val, ok := engine.Get("target-key")
	if !ok || val != "persisted-old-value" {
		t.Fatalf("expected persisted-old-value, got %q (ok=%v)", val, ok)
	}

	// 2. Delete key in active memtable (creates tombstone in active tier)
	if err := engine.Delete("target-key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get must return ok=false because active tombstone shadows SSTable value
	val, ok = engine.Get("target-key")
	if ok || val != "" {
		t.Fatalf("tombstone failed to shadow SSTable value: got val=%q, ok=%v", val, ok)
	}

	// 3. Flush tombstone to a second SSTable
	engine.RotateMemtable()
	if err := engine.Flush(); err != nil {
		t.Fatalf("Flush tombstone: %v", err)
	}
	if engine.SSTablesCount() != 2 {
		t.Fatalf("expected 2 SSTables, got %d", engine.SSTablesCount())
	}

	// Even when both SSTables are on disk, the newer SSTable's tombstone must shadow the older SSTable's value
	val, ok = engine.Get("target-key")
	if ok || val != "" {
		t.Fatalf("newer SSTable tombstone failed to shadow older SSTable: got val=%q, ok=%v", val, ok)
	}
}

// TestLSMEngine_CrashRecoveryPreservesSSTables verifies that an engine restarted
// with existing SSTables on disk discovers and loads them all, serving queries seamlessly.
func TestLSMEngine_CrashRecoveryPreservesSSTables(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	engine1, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine 1: %v", err)
	}

	// Populate and flush multiple SSTables
	for batch := 0; batch < 3; batch++ {
		for i := 0; i < 10; i++ {
			k := fmt.Sprintf("batch%d-key%d", batch, i)
			v := fmt.Sprintf("val-%d-%d", batch, i)
			if err := engine1.Set(k, v); err != nil {
				t.Fatalf("Set: %v", err)
			}
		}
		engine1.RotateMemtable()
	}
	if err := engine1.Flush(); err != nil {
		t.Fatalf("Flush engine1: %v", err)
	}

	if engine1.SSTablesCount() < 3 {
		t.Fatalf("expected at least 3 SSTables, got %d", engine1.SSTablesCount())
	}

	// Apply one more write to active WAL/memtable (unflushed)
	if err := engine1.ApplyCommitted(storage.CommittedCommand{
		RaftIndex: 1,
		RaftTerm:  1,
		Operation: wal.OperationSet,
		Key:       "active-unflushed-key",
		Value:     "live-value",
	}); err != nil {
		t.Fatalf("ApplyCommitted: %v", err)
	}

	if err := engine1.Close(); err != nil {
		t.Fatalf("Close engine1: %v", err)
	}

	// 2. Restart engine from disk
	engine2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine 2: %v", err)
	}
	defer engine2.Close()

	if engine2.SSTablesCount() < 3 {
		t.Fatalf("reopened engine expected at least 3 SSTables, got %d", engine2.SSTablesCount())
	}

	// Verify all SSTable keys exist
	for batch := 0; batch < 3; batch++ {
		for i := 0; i < 10; i++ {
			k := fmt.Sprintf("batch%d-key%d", batch, i)
			expectedVal := fmt.Sprintf("val-%d-%d", batch, i)
			val, ok := engine2.Get(k)
			if !ok || val != expectedVal {
				t.Fatalf("reopened key %q: expected %q, got %q (ok=%v)", k, expectedVal, val, ok)
			}
		}
	}

	// Verify unflushed active key replayed from WAL
	v, ok := engine2.Get("active-unflushed-key")
	if !ok || v != "live-value" {
		t.Fatalf("expected unflushed key 'active-unflushed-key' to be replayed from WAL, got %q (ok=%v)", v, ok)
	}
}

// TestLSMEngine_ConcurrentWritesDuringFlush verifies that active writes and reads
// continue without blocking while a background memtable flush is executing.
func TestLSMEngine_ConcurrentWritesDuringFlush(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	engine, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	engine.SetMemtableSizeLimit(512) // small limit to trigger automatic rotations

	const workers = 4
	const writesPerWorker = 50
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			for i := 0; i < writesPerWorker; i++ {
				k := fmt.Sprintf("w%d-key-%03d", wid, i)
				v := fmt.Sprintf("data-val-%03d", i)
				if err := engine.Set(k, v); err != nil {
					t.Errorf("worker %d Set failed: %v", wid, err)
					return
				}
				// Concurrent read
				if val, ok := engine.Get(k); !ok || val != v {
					t.Errorf("worker %d Get %q: expected %q, got %q (ok=%v)", wid, k, v, val, ok)
					return
				}
			}
		}(w)
	}

	wg.Wait()

	if err := engine.Flush(); err != nil {
		t.Fatalf("final Flush(): %v", err)
	}

	// Verify all writes survived
	for w := 0; w < workers; w++ {
		for i := 0; i < writesPerWorker; i++ {
			k := fmt.Sprintf("w%d-key-%03d", w, i)
			expectedVal := fmt.Sprintf("data-val-%03d", i)
			val, ok := engine.Get(k)
			if !ok || val != expectedVal {
				t.Fatalf("verification failed for key %q: expected %q, got %q", k, expectedVal, val)
			}
		}
	}
}
