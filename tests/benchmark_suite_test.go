package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orbit/internal/benchmark"
	"orbit/internal/database"
	"orbit/internal/observability"
	"orbit/internal/storage"
)

func TestBenchmark_StorageEnginePerformance(t *testing.T) {
	tempDir := t.TempDir()
	eng, err := storage.NewEngine(filepath.Join(tempDir, "wal.log"))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	cfg := benchmark.BenchmarkConfig{
		Name:        "storage-unit-bench",
		Target:      benchmark.TargetStorage,
		NumKeys:     500,
		ValueSize:   64,
		ReadRatio:   0.5,
		Concurrency: 2,
		Duration:    100 * time.Millisecond,
		Scenario:    "standard",
	}

	res := benchmark.RunStorageBenchmark(eng, cfg)
	if res.TotalOps == 0 {
		t.Fatalf("expected non-zero total operations")
	}
	if res.OpsPerSec <= 0 {
		t.Fatalf("expected positive throughput, got %f", res.OpsPerSec)
	}
	if res.Latency.Avg <= 0 {
		t.Fatalf("expected positive average latency")
	}

	report := benchmark.GenerateMarkdownReport(res)
	if !strings.Contains(report, "storage-unit-bench") {
		t.Fatalf("expected report to contain benchmark name, got: %s", report)
	}
}

func TestBenchmark_StorageDuringCompactionAndSnapshots(t *testing.T) {
	tempDir := t.TempDir()
	eng, err := storage.NewEngine(filepath.Join(tempDir, "wal.log"))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	cfgCompaction := benchmark.BenchmarkConfig{
		Name:        "storage-compaction-bench",
		Target:      benchmark.TargetStorage,
		NumKeys:     200,
		ValueSize:   32,
		ReadRatio:   0.3,
		Concurrency: 2,
		Duration:    100 * time.Millisecond,
		Scenario:    "during_compaction",
	}
	resCompaction := benchmark.RunStorageBenchmark(eng, cfgCompaction)
	if resCompaction.TotalOps == 0 {
		t.Fatalf("expected operations during compaction scenario")
	}

	cfgSnapshots := benchmark.BenchmarkConfig{
		Name:        "storage-snapshots-bench",
		Target:      benchmark.TargetStorage,
		NumKeys:     200,
		ValueSize:   32,
		ReadRatio:   0.3,
		Concurrency: 2,
		Duration:    100 * time.Millisecond,
		Scenario:    "during_snapshots",
	}
	resSnapshots := benchmark.RunStorageBenchmark(eng, cfgSnapshots)
	if resSnapshots.TotalOps == 0 {
		t.Fatalf("expected operations during snapshots scenario")
	}
}

func TestBenchmark_DatabaseWithWAL(t *testing.T) {
	tempDir := t.TempDir()
	eng, err := storage.NewEngine(filepath.Join(tempDir, "wal.log"))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	db, err := database.NewDatabase(eng)
	if err != nil {
		t.Fatalf("failed to init database: %v", err)
	}
	defer db.Close()

	cfg := benchmark.BenchmarkConfig{
		Name:        "db-wal-bench",
		Target:      benchmark.TargetDatabase,
		NumKeys:     300,
		ValueSize:   64,
		ReadRatio:   0.4,
		Concurrency: 2,
		Duration:    100 * time.Millisecond,
		Scenario:    "standard",
	}

	res := benchmark.RunDatabaseBenchmark(db, cfg)
	if res.TotalOps == 0 {
		t.Fatalf("expected operations on database")
	}
	if res.WriteOps == 0 || res.ReadOps == 0 {
		t.Fatalf("expected both read and write operations, got writes: %d, reads: %d", res.WriteOps, res.ReadOps)
	}
}

func TestBenchmark_ClusterHTTPTarget(t *testing.T) {
	// Mock HTTP server responding to /api/v1/kv/ endpoints
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"key": "k", "value": "v"})
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	cfg := benchmark.BenchmarkConfig{
		Name:        "cluster-bench",
		Target:      benchmark.TargetCluster,
		NumKeys:     100,
		ValueSize:   64,
		ReadRatio:   0.5,
		Concurrency: 2,
		Duration:    100 * time.Millisecond,
		BaseURL:     srv.URL,
	}

	res := benchmark.RunClusterBenchmark(srv.Client(), cfg)
	if res.TotalOps == 0 {
		t.Fatalf("expected operations against cluster HTTP endpoint")
	}
	if res.ErrorOps > 0 {
		t.Fatalf("expected 0 errors, got %d", res.ErrorOps)
	}
}

func TestBenchmark_ResultPersistenceAndComparison(t *testing.T) {
	tempDir := t.TempDir()
	outFile := filepath.Join(tempDir, "bench_result.json")
	histFile := filepath.Join(tempDir, "bench_history.json")

	r1 := benchmark.BenchmarkResult{
		Config:    benchmark.BenchmarkConfig{Name: "run-1"},
		TotalOps:  1000,
		OpsPerSec: 1000.0,
		Latency:   observability.LatencySummary{P50: 1.0, P99: 5.0},
	}
	r2 := benchmark.BenchmarkResult{
		Config:    benchmark.BenchmarkConfig{Name: "run-2"},
		TotalOps:  1200,
		OpsPerSec: 1200.0,
		Latency:   observability.LatencySummary{P50: 0.8, P99: 4.0},
	}

	if err := benchmark.SaveResult(outFile, r1); err != nil {
		t.Fatalf("failed to save result: %v", err)
	}
	loaded, err := benchmark.LoadResult(outFile)
	if err != nil {
		t.Fatalf("failed to load result: %v", err)
	}
	if loaded.TotalOps != 1000 {
		t.Fatalf("expected 1000 ops, got %d", loaded.TotalOps)
	}

	if err := benchmark.AppendToHistory(histFile, r1); err != nil {
		t.Fatalf("failed to append r1 to history: %v", err)
	}
	if err := benchmark.AppendToHistory(histFile, r2); err != nil {
		t.Fatalf("failed to append r2 to history: %v", err)
	}

	histData, _ := os.ReadFile(histFile)
	var history []benchmark.BenchmarkResult
	if err := json.Unmarshal(histData, &history); err != nil || len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d (err: %v)", len(history), err)
	}

	cmp := benchmark.CompareRuns(r1, r2)
	if cmp.OpsSecDeltaPct <= 0 {
		t.Fatalf("expected positive throughput delta, got %f", cmp.OpsSecDeltaPct)
	}
	if cmp.IsThroughputRegressed {
		t.Fatalf("expected no throughput regression")
	}
}
