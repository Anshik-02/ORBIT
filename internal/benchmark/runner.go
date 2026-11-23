package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"orbit/internal/database"
	"orbit/internal/storage"
)

// RunStorageBenchmark executes a benchmark against a local storage.Engine.
func RunStorageBenchmark(eng *storage.Engine, cfg BenchmarkConfig) BenchmarkResult {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.NumKeys <= 0 {
		cfg.NumKeys = 1000
	}
	if cfg.ValueSize <= 0 {
		cfg.ValueSize = 128
	}

	payloadVal := string(bytes.Repeat([]byte("v"), cfg.ValueSize))

	var totalOps, readOps, writeOps, errorOps atomic.Uint64
	latenciesMu := sync.Mutex{}
	latencies := make([]int64, 0, 50000)

	var startMem, endMem runtime.MemStats
	runtime.ReadMemStats(&startMem)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	// Scenario: background compaction
	if cfg.Scenario == "during_compaction" {
		go func() {
			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_, _ = eng.Compact()
				}
			}
		}()
	}

	// Scenario: background snapshots
	if cfg.Scenario == "during_snapshots" {
		go func() {
			ticker := time.NewTicker(30 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_, _, _ = eng.ExportSnapshotBytes()
				}
			}
		}()
	}

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(cfg.Concurrency)

	for w := 0; w < cfg.Concurrency; w++ {
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)*1000))
			localLats := make([]int64, 0, 2000)

			for {
				select {
				case <-ctx.Done():
					latenciesMu.Lock()
					latencies = append(latencies, localLats...)
					latenciesMu.Unlock()
					return
				default:
				}

				key := fmt.Sprintf("bench_key_%08d", r.Intn(cfg.NumKeys))
				isRead := r.Float64() < cfg.ReadRatio

				t0 := time.Now()
				var err error
				if isRead {
					_, found := eng.Get(key)
					if !found {
						// reading non-existent key in benchmark is not an internal error
					}
					readOps.Add(1)
				} else {
					err = eng.Set(key, payloadVal)
					writeOps.Add(1)
				}
				durUs := time.Since(t0).Microseconds()
				localLats = append(localLats, durUs)

				totalOps.Add(1)
				if err != nil {
					errorOps.Add(1)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	runtime.ReadMemStats(&endMem)

	return ComputeSummary(
		cfg,
		totalOps.Load(),
		readOps.Load(),
		writeOps.Load(),
		errorOps.Load(),
		duration,
		latencies,
		startMem.Alloc,
		endMem.Alloc,
		endMem.Sys,
		endMem.NumGC-startMem.NumGC,
		runtime.NumGoroutine(),
	)
}

// RunDatabaseBenchmark executes a benchmark against a single-node Database (with WAL).
func RunDatabaseBenchmark(db *database.Database, cfg BenchmarkConfig) BenchmarkResult {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.NumKeys <= 0 {
		cfg.NumKeys = 1000
	}
	if cfg.ValueSize <= 0 {
		cfg.ValueSize = 128
	}

	payloadVal := string(bytes.Repeat([]byte("d"), cfg.ValueSize))

	var totalOps, readOps, writeOps, errorOps atomic.Uint64
	latenciesMu := sync.Mutex{}
	latencies := make([]int64, 0, 50000)

	var startMem, endMem runtime.MemStats
	runtime.ReadMemStats(&startMem)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	if cfg.Scenario == "during_snapshots" {
		go func() {
			ticker := time.NewTicker(30 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_, _, _ = db.ExportSnapshotBytes()
				}
			}
		}()
	}

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(cfg.Concurrency)

	for w := 0; w < cfg.Concurrency; w++ {
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)*5000))
			localLats := make([]int64, 0, 2000)

			for {
				select {
				case <-ctx.Done():
					latenciesMu.Lock()
					latencies = append(latencies, localLats...)
					latenciesMu.Unlock()
					return
				default:
				}

				key := fmt.Sprintf("db_bench_%08d", r.Intn(cfg.NumKeys))
				isRead := r.Float64() < cfg.ReadRatio

				t0 := time.Now()
				var err error
				if isRead {
					_, _ = db.Get(key)
					readOps.Add(1)
				} else {
					err = db.Set(key, payloadVal)
					writeOps.Add(1)
				}
				durUs := time.Since(t0).Microseconds()
				localLats = append(localLats, durUs)

				totalOps.Add(1)
				if err != nil {
					errorOps.Add(1)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	runtime.ReadMemStats(&endMem)

	return ComputeSummary(
		cfg,
		totalOps.Load(),
		readOps.Load(),
		writeOps.Load(),
		errorOps.Load(),
		duration,
		latencies,
		startMem.Alloc,
		endMem.Alloc,
		endMem.Sys,
		endMem.NumGC-startMem.NumGC,
		runtime.NumGoroutine(),
	)
}

// RunClusterBenchmark executes a client HTTP benchmark against a live or test Orbit cluster.
func RunClusterBenchmark(client *http.Client, cfg BenchmarkConfig) BenchmarkResult {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.NumKeys <= 0 {
		cfg.NumKeys = 1000
	}
	if cfg.ValueSize <= 0 {
		cfg.ValueSize = 128
	}

	payloadVal := string(bytes.Repeat([]byte("c"), cfg.ValueSize))
	putBody, _ := json.Marshal(map[string]string{"value": payloadVal})

	var totalOps, readOps, writeOps, errorOps atomic.Uint64
	latenciesMu := sync.Mutex{}
	latencies := make([]int64, 0, 50000)

	var startMem, endMem runtime.MemStats
	runtime.ReadMemStats(&startMem)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(cfg.Concurrency)

	for w := 0; w < cfg.Concurrency; w++ {
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)*9999))
			localLats := make([]int64, 0, 2000)

			for {
				select {
				case <-ctx.Done():
					latenciesMu.Lock()
					latencies = append(latencies, localLats...)
					latenciesMu.Unlock()
					return
				default:
				}

				key := fmt.Sprintf("cluster_bench_%08d", r.Intn(cfg.NumKeys))
				isRead := r.Float64() < cfg.ReadRatio

				url := fmt.Sprintf("%s/api/v1/kv/%s", cfg.BaseURL, key)
				var req *http.Request
				if isRead {
					req, _ = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				} else {
					req, _ = http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(putBody))
					req.Header.Set("Content-Type", "application/json")
				}

				t0 := time.Now()
				resp, err := client.Do(req)
				durUs := time.Since(t0).Microseconds()
				localLats = append(localLats, durUs)

				totalOps.Add(1)
				if isRead {
					readOps.Add(1)
				} else {
					writeOps.Add(1)
				}

				if err != nil {
					if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
						errorOps.Add(1)
					}
				} else {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
					if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
						errorOps.Add(1)
					}
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	runtime.ReadMemStats(&endMem)

	return ComputeSummary(
		cfg,
		totalOps.Load(),
		readOps.Load(),
		writeOps.Load(),
		errorOps.Load(),
		duration,
		latencies,
		startMem.Alloc,
		endMem.Alloc,
		endMem.Sys,
		endMem.NumGC-startMem.NumGC,
		runtime.NumGoroutine(),
	)
}
