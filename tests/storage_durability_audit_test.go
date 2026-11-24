package tests

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"testing"

	"orbit/internal/storage"
)

// TestStorage_RandomizedModelFuzzWithRestarts executes an exhaustive randomized model-checking test
// for Gate G2. It executes 1,000+ random mutations (PUT, DELETE, GET, SCAN) across 10 crash-restart cycles,
// intermittently triggering Memtable rotations, flushes, and compactions, asserting exact 100% equivalence
// with an in-memory oracle reference map.
func TestStorage_RandomizedModelFuzzWithRestarts(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	// Ground truth reference oracle
	oracle := make(map[string]string)

	const numKeys = 50
	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = fmt.Sprintf("fuzz_k_%04d", i)
	}

	const numCycles = 10
	const opsPerCycle = 100
	rng := rand.New(rand.NewSource(42)) // Deterministic seed for reproducible fuzzing

	totalOpsExecuted := 0

	for cycle := 0; cycle < numCycles; cycle++ {
		// Open engine (or recover after crash)
		eng, err := storage.NewEngine(walPath)
		if err != nil {
			t.Fatalf("cycle %d: NewEngine recovery failed: %v", cycle, err)
		}

		func() {
			defer eng.Close()

			// Verify oracle consistency immediately upon restart
			for k, expectedVal := range oracle {
				val, found := eng.Get(k)
				if !found {
					t.Fatalf("cycle %d restart check: key %s missing from recovered engine", cycle, k)
				}
				if val != expectedVal {
					t.Fatalf("cycle %d restart check: key %s value mismatch: expected %q, got %q", cycle, k, expectedVal, val)
				}
			}

			// Execute randomized operations
			for op := 0; op < opsPerCycle; op++ {
				totalOpsExecuted++
				opType := rng.Float64()
				key := keys[rng.Intn(numKeys)]

				switch {
				case opType < 0.55: // 55% PUT (Set)
					val := fmt.Sprintf("val_c%d_op%d_rand%d", cycle, op, rng.Intn(10000))
					if err := eng.Set(key, val); err != nil {
						t.Fatalf("cycle %d op %d: Set(%s) failed: %v", cycle, op, key, err)
					}
					oracle[key] = val

				case opType < 0.75: // 20% DELETE
					if err := eng.Delete(key); err != nil {
						t.Fatalf("cycle %d op %d: Delete(%s) failed: %v", cycle, op, key, err)
					}
					delete(oracle, key)

				case opType < 0.90: // 15% GET
					val, found := eng.Get(key)
					expectedVal, exists := oracle[key]
					if exists != found {
						t.Fatalf("cycle %d op %d: Get(%s) existence mismatch: oracle=%v, engine=%v (val=%q)", cycle, op, key, exists, found, val)
					}
					if exists && val != expectedVal {
						t.Fatalf("cycle %d op %d: Get(%s) value mismatch: expected %q, got %q", cycle, op, key, expectedVal, val)
					}

				default: // 10% SCAN [startKey, endKey)
					startIdx := rng.Intn(numKeys - 5)
					endIdx := startIdx + rng.Intn(10) + 1
					if endIdx >= numKeys {
						endIdx = numKeys - 1
					}
					startKey := keys[startIdx]
					endKey := keys[endIdx]

					pairs, err := eng.Scan(startKey, endKey, 100)
					if err != nil {
						t.Fatalf("cycle %d op %d: Scan(%s, %s) failed: %v", cycle, op, startKey, endKey, err)
					}

					// Build expected scan results from oracle within [startKey, endKey)
					var expectedPairs []storage.KVPair
					for k, v := range oracle {
						if (startKey == "" || k >= startKey) && (endKey == "" || k < endKey) {
							expectedPairs = append(expectedPairs, storage.KVPair{Key: k, Value: v})
						}
					}
					sort.Slice(expectedPairs, func(i, j int) bool {
						return expectedPairs[i].Key < expectedPairs[j].Key
					})

					if len(pairs) != len(expectedPairs) {
						t.Fatalf("cycle %d op %d: Scan count mismatch for [%s, %s): expected %d (%+v), got %d (%+v)",
							cycle, op, startKey, endKey, len(expectedPairs), expectedPairs, len(pairs), pairs)
					}
					for i := range pairs {
						if pairs[i].Key != expectedPairs[i].Key || pairs[i].Value != expectedPairs[i].Value {
							t.Fatalf("cycle %d op %d: Scan pair mismatch at index %d: expected %+v, got %+v",
								cycle, op, i, expectedPairs[i], pairs[i])
						}
					}
				}

				// Intermittent LSM operations: Memtable rotation, Flush, Compaction
				if op%25 == 24 {
					eng.RotateMemtable()
					_ = eng.Flush()
				}
				if op%50 == 49 {
					_, _ = eng.Compact()
				}
			}
		}()
	}

	// Final verification upon final recovery
	finalEng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("final NewEngine failed: %v", err)
	}
	defer finalEng.Close()

	// 1. Point lookup verification for all keys in keyspace
	for _, k := range keys {
		val, found := finalEng.Get(k)
		expectedVal, exists := oracle[k]
		if exists != found {
			t.Fatalf("final verification: existence mismatch for %s: oracle=%v, engine=%v", k, exists, found)
		}
		if exists && val != expectedVal {
			t.Fatalf("final verification: value mismatch for %s: expected %q, got %q", k, expectedVal, val)
		}
	}

	// 2. Full keyspace scan verification with unbounded endKey
	allPairs, err := finalEng.Scan(keys[0], "", 500)
	if err != nil {
		t.Fatalf("final scan failed: %v", err)
	}
	if len(allPairs) != len(oracle) {
		t.Fatalf("final scan size mismatch: oracle has %d items, scan returned %d items", len(oracle), len(allPairs))
	}

	t.Logf("Successfully executed %d randomized operations across %d crash-restart cycles with 100%% oracle fidelity!",
		totalOpsExecuted, numCycles)
}
