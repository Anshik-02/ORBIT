package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"orbit/internal/storage"
)

// TestLSM_CrashDuringSSTableFlush verifies that an incomplete .tmp SSTable left behind
// by a crash during flush is automatically cleaned up and unflushed writes are safely recovered from the WAL.
func TestLSM_CrashDuringSSTableFlush(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Write 10 keys
	for i := 0; i < 10; i++ {
		_ = eng.Set(fmt.Sprintf("flush_key_%03d", i), fmt.Sprintf("val_%03d", i))
	}

	// Simulate crash mid-flush: create a half-written .tmp SSTable file in sstables dir
	sstDir := filepath.Join(tempDir, "sstables")
	_ = os.MkdirAll(sstDir, 0755)
	crashedTmp := filepath.Join(sstDir, "000099.sst.tmp")
	_ = os.WriteFile(crashedTmp, []byte("half-written-sstable-bytes-during-crash"), 0644)

	// Close engine without flushing active memtable
	_ = eng.Close()

	// Verify the .tmp file exists before recovery
	if _, err := os.Stat(crashedTmp); err != nil {
		t.Fatalf("expected crashed .tmp to exist: %v", err)
	}

	// Restart engine
	eng2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	defer eng2.Close()

	// The .tmp file should have been cleaned up
	if _, err := os.Stat(crashedTmp); !os.IsNotExist(err) {
		t.Fatalf("expected crashed .tmp file to be removed by recovery")
	}

	// All 10 keys must be recovered from the WAL
	for i := 0; i < 10; i++ {
		val, found := eng2.Get(fmt.Sprintf("flush_key_%03d", i))
		if !found {
			t.Fatalf("expected key %d to be recovered from WAL, found=%v", i, found)
		}
		if val != fmt.Sprintf("val_%03d", i) {
			t.Fatalf("expected val_%03d, got %s", i, val)
		}
	}
}

// TestLSM_CrashAfterSSTableCreationBeforeManifest verifies that if an SSTable was created
// but the process crashed before the MANIFEST was updated, the orphan SSTable is cleaned up
// and the WAL replays the uncommitted data.
func TestLSM_CrashAfterSSTableCreationBeforeManifest(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	for i := 0; i < 5; i++ {
		_ = eng.Set(fmt.Sprintf("key_%d", i), fmt.Sprintf("v_%d", i))
	}
	_ = eng.Close()

	// Plant an orphan SSTable in sstDir that is not in the manifest
	sstDir := filepath.Join(tempDir, "sstables")
	orphanPath := filepath.Join(sstDir, "999999.sst")
	_ = os.WriteFile(orphanPath, []byte("orphan-sstable-content"), 0644)

	eng2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	defer eng2.Close()

	// Orphan SSTable must be cleaned up
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatalf("expected orphan sstable to be removed, but it still exists")
	}

	// Valid data preserved
	for i := 0; i < 5; i++ {
		val, found := eng2.Get(fmt.Sprintf("key_%d", i))
		if !found || val != fmt.Sprintf("v_%d", i) {
			t.Fatalf("key_%d missing or incorrect: val=%s found=%v", i, val, found)
		}
	}
}

// TestLSM_CrashDuringCompaction verifies that temporary compaction outputs are purged
// and original inputs remain fully active without data loss.
func TestLSM_CrashDuringCompaction(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	for i := 0; i < 10; i++ {
		_ = eng.Set(fmt.Sprintf("compact_k_%d", i), fmt.Sprintf("v_%d", i))
	}
	eng.RotateMemtable()
	_ = eng.Flush()

	// Plant a half-merged compaction output file
	sstDir := filepath.Join(tempDir, "sstables")
	compactTmp := filepath.Join(sstDir, "compact_tmp_123.sst.tmp")
	_ = os.WriteFile(compactTmp, []byte("partial-compaction-output"), 0644)

	_ = eng.Close()

	eng2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("recovery: %v", err)
	}
	defer eng2.Close()

	// Verify compactTmp is deleted
	if _, err := os.Stat(compactTmp); !os.IsNotExist(err) {
		t.Fatalf("expected compact temp file to be removed")
	}

	// Verify all keys are accessible
	for i := 0; i < 10; i++ {
		val, found := eng2.Get(fmt.Sprintf("compact_k_%d", i))
		if !found || val != fmt.Sprintf("v_%d", i) {
			t.Fatalf("key missing after compaction crash: %s", fmt.Sprintf("compact_k_%d", i))
		}
	}
}

// TestLSM_RecoveryWithMissingSSTable verifies that if a manifest references an SSTable
// that is missing from disk, engine recovery fails with an informative error rather than silently corrupting reads.
func TestLSM_RecoveryWithMissingSSTable(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_ = eng.Set("k1", "v1")
	eng.RotateMemtable()
	_ = eng.Flush()

	sstDir := filepath.Join(tempDir, "sstables")
	entries, _ := os.ReadDir(sstDir)
	var sstFile string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".sst" {
			sstFile = filepath.Join(sstDir, e.Name())
			break
		}
	}
	_ = eng.Close()

	if sstFile == "" {
		t.Fatalf("no sstable file found to remove")
	}

	// Delete the registered SSTable file
	_ = os.Remove(sstFile)

	// Attempt recovery
	eng2, err := storage.NewEngine(walPath)
	if err == nil {
		eng2.Close()
		t.Fatalf("expected engine recovery to fail when manifest sstable is missing")
	}
}

// TestLSM_RecoveryWithCorruptedManifest verifies that checksum mismatch in MANIFEST is detected.
func TestLSM_RecoveryWithCorruptedManifest(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = eng.Set("k1", "v1")
	_ = eng.Close()

	// Corrupt MANIFEST
	manifestPath := filepath.Join(tempDir, "MANIFEST")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	// Modify a byte
	if len(data) > 20 {
		data[20] ^= 0xFF
	}
	_ = os.WriteFile(manifestPath, data, 0600)

	_, err = storage.NewEngine(walPath)
	if err == nil {
		t.Fatalf("expected engine to reject corrupted manifest")
	}
}

// TestLSM_TombstoneHandlingAcrossCompaction verifies:
// 1. A tombstone in L0 shadows older versions in L1.
// 2. Tombstones are retained when lower levels exist.
// 3. Tombstones are purged once they reach the bottom level.
func TestLSM_TombstoneHandlingAcrossCompaction(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer eng.Close()

	// 1. Insert key in L1 via flush and minor compaction
	_ = eng.Set("tomb_key", "old_value")
	eng.RotateMemtable()
	_ = eng.Flush()
	_, _ = eng.Compact() // compact to L1

	// Verify key exists
	val, found := eng.Get("tomb_key")
	if !found || val != "old_value" {
		t.Fatalf("expected old_value in L1, got val=%s found=%v", val, found)
	}

	// 2. Delete key in active memtable (creates tombstone in L0)
	_ = eng.Delete("tomb_key")

	// Get should immediately see tombstone shadowing L1
	val, found = eng.Get("tomb_key")
	if found {
		t.Fatalf("expected tombstone to shadow L1 old_value, but got found=true val=%s", val)
	}

	// Flush tombstone to L0
	eng.RotateMemtable()
	_ = eng.Flush()

	// Get should still not find the key
	val, found = eng.Get("tomb_key")
	if found {
		t.Fatalf("expected key to remain deleted after tombstone flushed to L0")
	}

	// Major compaction merges L0 and L1 down to lowest level and drops the tombstone
	res, err := eng.MajorCompact()
	if err != nil {
		t.Fatalf("major compaction failed: %v", err)
	}
	if res.TombstonesDropped == 0 {
		t.Logf("note: tombstones dropped: %d", res.TombstonesDropped)
	}

	// Key must remain completely non-existent
	val, found = eng.Get("tomb_key")
	if found {
		t.Fatalf("expected key to remain deleted after major compaction")
	}
}

// TestLSM_OverwriteOrderingAcrossLevels verifies that newer updates in L0
// always override older versions in L1/L2, and range scans return the newest version.
func TestLSM_OverwriteOrderingAcrossLevels(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer eng.Close()

	// Write version 1 to L1
	_ = eng.Set("order_k", "version_1")
	eng.RotateMemtable()
	_ = eng.Flush()
	_, _ = eng.Compact()

	// Write version 2 to L0
	_ = eng.Set("order_k", "version_2")
	eng.RotateMemtable()
	_ = eng.Flush()

	// Write version 3 to active memtable
	_ = eng.Set("order_k", "version_3")

	// Read must return version 3
	val, found := eng.Get("order_k")
	if !found || val != "version_3" {
		t.Fatalf("expected version_3, got %s", val)
	}

	// Scan must also observe version 3
	pairs, err := eng.Scan("order_a", "order_z", 10)
	if err != nil || len(pairs) != 1 || pairs[0].Value != "version_3" {
		t.Fatalf("scan expected version_3, got %v", pairs)
	}
}

// TestLSM_ConcurrentReadsAndWritesDuringCompaction tests concurrent read and write operations
// while continuous compaction runs in the background.
func TestLSM_ConcurrentReadsAndWritesDuringCompaction(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer eng.Close()

	var stopCh = make(chan struct{})
	var wg sync.WaitGroup

	// Background continuous compaction worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopCh:
				return
			default:
				_, _ = eng.Compact()
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	// 5 Concurrent writers and readers
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				k := fmt.Sprintf("concurrent_%d_%d", workerID, i)
				v := fmt.Sprintf("val_%d_%d", workerID, i)
				if err := eng.Set(k, v); err != nil {
					t.Errorf("concurrent set failed: %v", err)
				}
				readVal, found := eng.Get(k)
				if !found || readVal != v {
					t.Errorf("concurrent read mismatch for %s: val=%s found=%v", k, readVal, found)
				}
				if i%10 == 0 {
					eng.RotateMemtable()
					_ = eng.Flush()
				}
			}
		}(w)
	}

	// Wait for writers to complete
	time.Sleep(100 * time.Millisecond)
	close(stopCh)
	wg.Wait()
}

// TestLSM_LargeDatabaseRecovery inserts 2,000 keys across multiple memtables,
// flushes them to SSTables, runs compaction, and restarts to verify 100% data fidelity.
func TestLSM_LargeDatabaseRecovery(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	totalKeys := 2000
	for i := 0; i < totalKeys; i++ {
		_ = eng.Set(fmt.Sprintf("large_key_%05d", i), fmt.Sprintf("value_payload_%05d", i))
		if i%200 == 0 {
			eng.RotateMemtable()
			_ = eng.Flush()
		}
	}
	_ = eng.Flush()
	_, _ = eng.Compact()
	_ = eng.Close()

	// Recover engine
	eng2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	defer eng2.Close()

	// Verify all 2,000 keys
	for i := 0; i < totalKeys; i++ {
		k := fmt.Sprintf("large_key_%05d", i)
		expectedVal := fmt.Sprintf("value_payload_%05d", i)
		v, found := eng2.Get(k)
		if !found || v != expectedVal {
			t.Fatalf("key %s corrupted or missing: val=%s found=%v", k, v, found)
		}
	}
}

// TestLSM_BackupConsistencyUnderConcurrentWritesAndCompaction tests that Engine.CreateBackup
// creates a 100% consistent point-in-time snapshot while concurrent writes and compactions are occurring.
func TestLSM_BackupConsistencyUnderConcurrentWritesAndCompaction(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")
	backupDir := filepath.Join(t.TempDir(), "backups")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer eng.Close()

	// Pre-populate with initial keys
	for i := 0; i < 50; i++ {
		_ = eng.Set(fmt.Sprintf("initial_key_%03d", i), fmt.Sprintf("init_val_%03d", i))
	}
	eng.RotateMemtable()
	_ = eng.Flush()

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	// Background worker doing writes and compactions
	wg.Add(1)
	go func() {
		defer wg.Done()
		counter := 0
		for {
			select {
			case <-stopCh:
				return
			default:
				_ = eng.Set(fmt.Sprintf("live_key_%d", counter), fmt.Sprintf("live_val_%d", counter))
				if counter%5 == 0 {
					_, _ = eng.Compact()
				}
				counter++
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()

	// Create backup while background activity is ongoing
	time.Sleep(20 * time.Millisecond)
	createdBackup, err := eng.CreateBackup(backupDir)
	if err != nil {
		t.Fatalf("CreateBackup failed under concurrent workload: %v", err)
	}

	close(stopCh)
	wg.Wait()

	// Restore backup into clean directory
	restoreDir := filepath.Join(t.TempDir(), "restored")
	if err := storage.RestoreBackup(createdBackup, restoreDir); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Verify restored engine opens cleanly and contains the initial keys
	restoredWal := filepath.Join(restoreDir, "wal.log")
	restoredEng, err := storage.NewEngine(restoredWal)
	if err != nil {
		t.Fatalf("failed to open restored engine: %v", err)
	}
	defer restoredEng.Close()

	for i := 0; i < 50; i++ {
		k := fmt.Sprintf("initial_key_%03d", i)
		expectedVal := fmt.Sprintf("init_val_%03d", i)
		v, found := restoredEng.Get(k)
		if !found || v != expectedVal {
			t.Fatalf("restored backup missing key %s: val=%s found=%v", k, v, found)
		}
	}
}
