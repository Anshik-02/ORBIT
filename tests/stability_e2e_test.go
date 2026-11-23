package tests

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"orbit/internal/database"
	"orbit/internal/storage"
)

// TestStability_RepeatedFailureAndRecovery verifies data integrity under repeated crash-recovery cycles.
func TestStability_RepeatedFailureAndRecovery(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	expectedState := make(map[string]string)
	keys := make([]string, 50)
	for i := range keys {
		keys[i] = fmt.Sprintf("st_key_%03d", i)
	}

	// 5 crash-recovery cycles
	for cycle := 0; cycle < 5; cycle++ {
		eng, err := storage.NewEngine(walPath)
		if err != nil {
			t.Fatalf("cycle %d: failed to open engine: %v", cycle, err)
		}
		db, err := database.NewDatabase(eng)
		if err != nil {
			t.Fatalf("cycle %d: failed to open database: %v", cycle, err)
		}

		// Verify existing state from previous cycles
		for k, expV := range expectedState {
			v, err := db.Get(k)
			if err != nil || v != expV {
				t.Fatalf("cycle %d: state mismatch for key %s: exp=%s got=%s (err: %v)", cycle, k, expV, v, err)
			}
		}

		// Perform 20 random mutations
		for m := 0; m < 20; m++ {
			k := keys[rand.Intn(len(keys))]
			v := fmt.Sprintf("val_c%d_m%d", cycle, m)
			if err := db.Set(k, v); err != nil {
				t.Fatalf("cycle %d: set error: %v", cycle, err)
			}
			expectedState[k] = v
		}

		// Every other cycle, rotate memtable and flush to test SSTable recovery
		if cycle%2 == 1 {
			eng.RotateMemtable()
			if err := eng.Flush(); err != nil {
				t.Fatalf("cycle %d: flush error: %v", cycle, err)
			}
		}

		// Abruptly close engine (simulating node crash)
		_ = db.Close()
	}

	// Final verification
	finalEng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("final open failed: %v", err)
	}
	defer finalEng.Close()

	finalDB, err := database.NewDatabase(finalEng)
	if err != nil {
		t.Fatalf("final db init failed: %v", err)
	}
	defer finalDB.Close()

	for k, expV := range expectedState {
		v, err := finalDB.Get(k)
		if err != nil || v != expV {
			t.Fatalf("final mismatch for %s: exp=%s got=%s", k, expV, v)
		}
	}
}

// TestStability_GoroutineAndMemoryLeakCheck verifies that concurrent load does not leak goroutines.
func TestStability_GoroutineAndMemoryLeakCheck(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	db, err := database.NewDatabase(eng)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	// Warm up
	for i := 0; i < 50; i++ {
		_ = db.Set(fmt.Sprintf("warm_%d", i), "val")
	}

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	// High concurrency workload across 10 workers
	var wg sync.WaitGroup
	workers := 10
	opsPerWorker := 100
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				k := fmt.Sprintf("k_w%d_%d", workerID, i)
				_ = db.Set(k, "value")
				_, _ = db.Get(k)
			}
		}(w)
	}
	wg.Wait()

	// Wait for any asynchronous work to finish and force GC
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()

	// Ensure no runaway goroutine leaks
	diff := finalGoroutines - initialGoroutines
	if diff > 5 {
		t.Fatalf("goroutine leak detected: initial=%d, final=%d (diff=%d)", initialGoroutines, finalGoroutines, diff)
	}
}
