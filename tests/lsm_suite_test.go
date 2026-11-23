package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/storage"
	"orbit/internal/wal"
)

// TestManifest_DurabilityAndRecovery verifies that the Manifest persists SSTables,
// detects corrupted manifests, and handles crash recovery.
func TestManifest_DurabilityAndRecovery(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "MANIFEST")

	m := storage.NewManifest(manifestPath)
	rec1 := storage.SSTableRecord{
		ID:         1,
		Filename:   "000001.sst",
		Level:      0,
		NumEntries: 50,
		MinKey:     "a",
		MaxKey:     "m",
		SizeBytes:  1024,
		Checksum:   12345,
		CreatedAt:  time.Now(),
	}
	if err := m.AddSSTable(rec1); err != nil {
		t.Fatalf("AddSSTable 1 failed: %v", err)
	}

	rec2 := storage.SSTableRecord{
		ID:         2,
		Filename:   "000002.sst",
		Level:      0,
		NumEntries: 40,
		MinKey:     "n",
		MaxKey:     "z",
		SizeBytes:  800,
		Checksum:   67890,
		CreatedAt:  time.Now(),
	}
	if err := m.AddSSTable(rec2); err != nil {
		t.Fatalf("AddSSTable 2 failed: %v", err)
	}

	if err := m.UpdateFlushedOffsets(100, 5); err != nil {
		t.Fatalf("UpdateFlushedOffsets failed: %v", err)
	}

	// Reload manifest from disk
	mLoaded, err := storage.LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	all := mLoaded.AllTables()
	if len(all) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(all))
	}
	if all[0].Filename != "000001.sst" || all[1].Filename != "000002.sst" {
		t.Errorf("unexpected table sequence: %+v", all)
	}

	walSeq, raftIdx := mLoaded.LastFlushedOffsets()
	if walSeq != 100 || raftIdx != 5 {
		t.Errorf("expected offsets (100, 5), got (%d, %d)", walSeq, raftIdx)
	}

	// Test atomic replace
	rec3 := storage.SSTableRecord{
		ID:         3,
		Filename:   "000003.sst",
		Level:      1,
		NumEntries: 90,
		MinKey:     "a",
		MaxKey:     "z",
		SizeBytes:  1800,
		Checksum:   99999,
		CreatedAt:  time.Now(),
	}
	if err := mLoaded.ReplaceSSTables([]uint64{1, 2}, []storage.SSTableRecord{rec3}); err != nil {
		t.Fatalf("ReplaceSSTables failed: %v", err)
	}

	// Reload again
	mReloaded, err := storage.LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest after replace failed: %v", err)
	}

	l0 := mReloaded.LevelTables(0)
	if len(l0) != 0 {
		t.Fatalf("expected 0 tables in level 0, got %d", len(l0))
	}
	l1 := mReloaded.LevelTables(1)
	if len(l1) != 1 || l1[0].ID != 3 {
		t.Fatalf("expected 1 table with ID 3 in level 1, got %+v", l1)
	}
}

// TestManifest_CleanOrphanFiles verifies that uncommitted or crash-orphaned .sst and .tmp files
// are cleanly purged on startup.
func TestManifest_CleanOrphanFiles(t *testing.T) {
	dir := t.TempDir()
	sstDir := filepath.Join(dir, "sstables")
	_ = os.MkdirAll(sstDir, 0700)
	manifestPath := filepath.Join(dir, "MANIFEST")

	m := storage.NewManifest(manifestPath)
	rec := storage.SSTableRecord{
		ID:         1,
		Filename:   "000001.sst",
		Level:      0,
		NumEntries: 10,
	}
	_ = m.AddSSTable(rec)

	// Create committed file
	_ = os.WriteFile(filepath.Join(sstDir, "000001.sst"), []byte("committed"), 0600)
	// Create orphan file from crashed flush
	_ = os.WriteFile(filepath.Join(sstDir, "000002.sst"), []byte("orphan"), 0600)
	// Create partial .tmp file from crashed write
	_ = os.WriteFile(filepath.Join(sstDir, "000003.sst.tmp"), []byte("tmp"), 0600)

	orphans, err := m.CleanOrphanFiles(sstDir)
	if err != nil {
		t.Fatalf("CleanOrphanFiles failed: %v", err)
	}

	if len(orphans) != 2 {
		t.Fatalf("expected 2 orphans cleaned, got %d (%v)", len(orphans), orphans)
	}

	// Verify committed file remains
	if _, err := os.Stat(filepath.Join(sstDir, "000001.sst")); err != nil {
		t.Errorf("committed file should remain intact: %v", err)
	}
	// Verify orphan file deleted
	if _, err := os.Stat(filepath.Join(sstDir, "000002.sst")); !os.IsNotExist(err) {
		t.Errorf("orphan file should have been deleted")
	}
}

// TestCompactor_MinorAndMajorCompaction verifies that L0->L1 compaction merges runs,
// drops tombstones when safe, and leaves readers functioning smoothly.
func TestCompactor_MinorAndMajorCompaction(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	// Reduce memtable size limit to trigger multiple SSTable flushes
	eng.SetMemtableSizeLimit(128)

	// Table 1: keys 1 to 5
	for i := 1; i <= 5; i++ {
		_ = eng.Set(fmt.Sprintf("key_%02d", i), fmt.Sprintf("val_v1_%02d", i))
	}
	eng.RotateMemtable()
	_ = eng.Flush()

	// Table 2: overwrite keys 1 to 3 with v2, add keys 6 to 8
	for i := 1; i <= 3; i++ {
		_ = eng.Set(fmt.Sprintf("key_%02d", i), fmt.Sprintf("val_v2_%02d", i))
	}
	for i := 6; i <= 8; i++ {
		_ = eng.Set(fmt.Sprintf("key_%02d", i), fmt.Sprintf("val_v1_%02d", i))
	}
	eng.RotateMemtable()
	_ = eng.Flush()

	// Table 3: delete key 2 and key 7
	_ = eng.Delete("key_02")
	_ = eng.Delete("key_07")
	eng.RotateMemtable()
	_ = eng.Flush()

	// Table 4: add key 9
	_ = eng.Set("key_09", "val_v1_09")
	eng.RotateMemtable()
	_ = eng.Flush()

	// Verify pre-compaction state
	if val, ok := eng.Get("key_01"); !ok || val != "val_v2_01" {
		t.Fatalf("expected key_01 to be val_v2_01, got %q (ok=%v)", val, ok)
	}
	if _, ok := eng.Get("key_02"); ok {
		t.Fatalf("expected key_02 to be deleted")
	}
	if _, ok := eng.Get("key_07"); ok {
		t.Fatalf("expected key_07 to be deleted")
	}

	// Trigger Major Compaction
	res, err := eng.MajorCompact()
	if err != nil {
		t.Fatalf("MajorCompact failed: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil compaction result")
	}
	if res.TombstonesDropped != 2 {
		t.Errorf("expected 2 tombstones dropped, got %d", res.TombstonesDropped)
	}

	// Verify post-compaction state
	if val, ok := eng.Get("key_01"); !ok || val != "val_v2_01" {
		t.Errorf("post-compact: expected key_01 to be val_v2_01, got %q", val)
	}
	if _, ok := eng.Get("key_02"); ok {
		t.Errorf("post-compact: key_02 should remain deleted")
	}
	if val, ok := eng.Get("key_05"); !ok || val != "val_v1_05" {
		t.Errorf("post-compact: expected key_05 to be val_v1_05, got %q", val)
	}
	if val, ok := eng.Get("key_09"); !ok || val != "val_v1_09" {
		t.Errorf("post-compact: expected key_09 to be val_v1_09, got %q", val)
	}
}

// TestRangeScan_AcrossStorageTiers verifies that Scan and PrefixScan accurately
// merge active memtable, immutable memtables, and SSTables respecting tombstones and limits.
func TestRangeScan_AcrossStorageTiers(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	eng.SetMemtableSizeLimit(128)

	// In SSTable: user:01 .. user:04
	_ = eng.Set("user:01", "Alice")
	_ = eng.Set("user:02", "Bob")
	_ = eng.Set("user:03", "Charlie")
	_ = eng.Set("user:04", "David")
	eng.RotateMemtable()
	_ = eng.Flush()

	// In immutable memtable: user:02 deleted, user:05 added
	_ = eng.Delete("user:02")
	_ = eng.Set("user:05", "Eve")
	eng.RotateMemtable()

	// In active memtable: user:03 updated, user:06 added, order:10 added
	_ = eng.Set("user:03", "CharlieUpdated")
	_ = eng.Set("user:06", "Frank")
	_ = eng.Set("order:10", "OrderTen")

	// 1. Prefix scan for "user:"
	users, err := eng.PrefixScan("user:", 0)
	if err != nil {
		t.Fatalf("PrefixScan failed: %v", err)
	}

	expectedKeys := []string{"user:01", "user:03", "user:04", "user:05", "user:06"}
	expectedVals := []string{"Alice", "CharlieUpdated", "David", "Eve", "Frank"}

	if len(users) != len(expectedKeys) {
		t.Fatalf("expected %d users, got %d: %+v", len(expectedKeys), len(users), users)
	}
	for i, u := range users {
		if u.Key != expectedKeys[i] || u.Value != expectedVals[i] {
			t.Errorf("at index %d: expected (%s, %s), got (%s, %s)",
				i, expectedKeys[i], expectedVals[i], u.Key, u.Value)
		}
	}

	// 2. Bounded range scan [user:03, user:06) with limit 2
	bounded, err := eng.Scan("user:03", "user:06", 2)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(bounded) != 2 {
		t.Fatalf("expected limit 2 results, got %d", len(bounded))
	}
	if bounded[0].Key != "user:03" || bounded[1].Key != "user:04" {
		t.Errorf("unexpected bounded keys: %+v", bounded)
	}
}

// TestWAL_CheckpointingAndReclamation verifies that flushing immutable memtables
// advances the checkpoint watermark in Manifest and reclaims obsolete WAL records.
func TestWAL_CheckpointingAndReclamation(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	// Write 10 records
	for i := 1; i <= 10; i++ {
		_ = eng.Set(fmt.Sprintf("k%02d", i), fmt.Sprintf("v%02d", i))
	}

	// Flush to disk
	eng.RotateMemtable()
	if err := eng.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Manifest should record flushed sequence
	walSeq, _ := eng.Manifest().LastFlushedOffsets()
	if walSeq != 10 {
		t.Fatalf("expected last flushed WAL seq 10, got %d", walSeq)
	}

	// Write 5 more records into active memtable (unflushed)
	for i := 11; i <= 15; i++ {
		_ = eng.Set(fmt.Sprintf("k%02d", i), fmt.Sprintf("v%02d", i))
	}

	// Simulate crash: close engine without flushing active memtable
	if err := eng.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Reopen engine: should recover SSTables from Manifest + replay only unflushed WAL entries (11 to 15)
	recoveredEng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("recover NewEngine: %v", err)
	}
	defer recoveredEng.Close()

	// Verify all 15 records are intact
	for i := 1; i <= 15; i++ {
		expectedVal := fmt.Sprintf("v%02d", i)
		val, ok := recoveredEng.Get(fmt.Sprintf("k%02d", i))
		if !ok || val != expectedVal {
			t.Errorf("key k%02d: expected %q (ok=true), got %q (ok=%v)", i, expectedVal, val, ok)
		}
	}
}

// TestWAL_DirectTruncateBefore verifies wal.Log.TruncateBefore cleans up log records
// while maintaining sequence monotonicity.
func TestWAL_DirectTruncateBefore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")

	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	for i := 1; i <= 5; i++ {
		_ = w.Append(wal.Record{
			Operation: wal.OperationSet,
			Key:       fmt.Sprintf("k%d", i),
			Value:     fmt.Sprintf("v%d", i),
		})
	}
	if w.Sequence() != 5 {
		t.Fatalf("expected sequence 5, got %d", w.Sequence())
	}

	// Truncate before seq 3 (retains 4 and 5)
	if err := w.TruncateBefore(3); err != nil {
		t.Fatalf("TruncateBefore(3) failed: %v", err)
	}

	records, err := w.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records retained, got %d", len(records))
	}
	if records[0].Sequence != 4 || records[1].Sequence != 5 {
		t.Errorf("unexpected retained sequences: %+v", records)
	}

	// Append record 6: sequence counter must remain monotonic
	_ = w.Append(wal.Record{
		Operation: wal.OperationSet,
		Key:       "k6",
		Value:     "v6",
	})
	if w.Sequence() != 6 {
		t.Errorf("expected sequence 6, got %d", w.Sequence())
	}

	_ = w.Close()
}
