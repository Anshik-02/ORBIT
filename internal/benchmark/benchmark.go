package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"orbit/internal/observability"
)

// BenchmarkTarget defines the workload target under test.
type BenchmarkTarget string

const (
	TargetStorage  BenchmarkTarget = "storage"
	TargetDatabase BenchmarkTarget = "database"
	TargetCluster  BenchmarkTarget = "cluster"
)

// BenchmarkConfig holds execution parameters for a benchmark run.
type BenchmarkConfig struct {
	Name         string          `json:"name"`
	Target       BenchmarkTarget `json:"target"`
	NumKeys      int             `json:"num_keys"`
	ValueSize    int             `json:"value_size_bytes"`
	ReadRatio    float64         `json:"read_ratio"` // 0.0 = 100% write, 1.0 = 100% read
	Concurrency  int             `json:"concurrency"`
	Duration     time.Duration   `json:"duration_ns"`
	BaseURL      string          `json:"base_url,omitempty"`
	Scenario     string          `json:"scenario,omitempty"`
	ReplicaCount int             `json:"replica_count,omitempty"`
	ShardCount   int             `json:"shard_count,omitempty"`
}

// BenchmarkResult captures comprehensive performance metrics from a completed run.
type BenchmarkResult struct {
	Timestamp       time.Time                    `json:"timestamp"`
	Config          BenchmarkConfig              `json:"config"`
	TotalOps        uint64                       `json:"total_ops"`
	ReadOps         uint64                       `json:"read_ops"`
	WriteOps        uint64                       `json:"write_ops"`
	ErrorOps        uint64                       `json:"error_ops"`
	DurationSeconds float64                      `json:"duration_seconds"`
	OpsPerSec       float64                      `json:"ops_per_sec"`
	ReadOpsPerSec   float64                      `json:"read_ops_per_sec"`
	WriteOpsPerSec  float64                      `json:"write_ops_per_sec"`
	ErrorRate       float64                      `json:"error_rate"`
	Latency         observability.LatencySummary `json:"latency_ms"`
	MinLatencyMs    float64                      `json:"min_latency_ms"`
	MaxLatencyMs    float64                      `json:"max_latency_ms"`
	MemoryAllocMB   float64                      `json:"memory_alloc_mb"`
	MemorySysMB     float64                      `json:"memory_sys_mb"`
	NumGoroutines   int                          `json:"num_goroutines"`
	NumGC           uint32                       `json:"num_gc"`
	Notes           string                       `json:"notes,omitempty"`
}

// BenchmarkComparison details differences between a baseline and a candidate benchmark run.
type BenchmarkComparison struct {
	BaselineName       string  `json:"baseline_name"`
	CandidateName      string  `json:"candidate_name"`
	BaselineOpsSec     float64 `json:"baseline_ops_sec"`
	CandidateOpsSec    float64 `json:"candidate_ops_sec"`
	OpsSecDeltaPct     float64 `json:"ops_sec_delta_pct"`
	BaselineP50Ms      float64 `json:"baseline_p50_ms"`
	CandidateP50Ms     float64 `json:"candidate_p50_ms"`
	P50DeltaPct        float64 `json:"p50_delta_pct"`
	BaselineP99Ms      float64 `json:"baseline_p99_ms"`
	CandidateP99Ms     float64 `json:"candidate_p99_ms"`
	P99DeltaPct        float64 `json:"p99_delta_pct"`
	ErrorRateDelta     float64 `json:"error_rate_delta"`
	IsThroughputRegressed bool `json:"is_throughput_regressed"`
	IsLatencyRegressed    bool `json:"is_latency_regressed"`
}

// ComputeSummary aggregates collected raw latencies into percentiles and summary rates.
func ComputeSummary(
	cfg BenchmarkConfig,
	totalOps, readOps, writeOps, errorOps uint64,
	duration time.Duration,
	latenciesUs []int64,
	startMemAlloc, endMemAlloc uint64,
	endMemSys uint64,
	numGC uint32,
	numGoroutines int,
) BenchmarkResult {
	durSec := duration.Seconds()
	if durSec <= 0 {
		durSec = 0.000001
	}

	var p50, p95, p99, avg, minMs, maxMs float64
	if len(latenciesUs) > 0 {
		sort.Slice(latenciesUs, func(i, j int) bool {
			return latenciesUs[i] < latenciesUs[j]
		})

		minMs = float64(latenciesUs[0]) / 1000.0
		maxMs = float64(latenciesUs[len(latenciesUs)-1]) / 1000.0

		var sum int64
		for _, v := range latenciesUs {
			sum += v
		}
		avg = float64(sum) / float64(len(latenciesUs)) / 1000.0

		p50Idx := int(float64(len(latenciesUs)) * 0.50)
		p95Idx := int(float64(len(latenciesUs)) * 0.95)
		p99Idx := int(float64(len(latenciesUs)) * 0.99)

		if p50Idx >= len(latenciesUs) {
			p50Idx = len(latenciesUs) - 1
		}
		if p95Idx >= len(latenciesUs) {
			p95Idx = len(latenciesUs) - 1
		}
		if p99Idx >= len(latenciesUs) {
			p99Idx = len(latenciesUs) - 1
		}

		p50 = float64(latenciesUs[p50Idx]) / 1000.0
		p95 = float64(latenciesUs[p95Idx]) / 1000.0
		p99 = float64(latenciesUs[p99Idx]) / 1000.0
	}

	errRate := 0.0
	if totalOps > 0 {
		errRate = float64(errorOps) / float64(totalOps)
	}

	return BenchmarkResult{
		Timestamp:       time.Now().UTC(),
		Config:          cfg,
		TotalOps:        totalOps,
		ReadOps:         readOps,
		WriteOps:        writeOps,
		ErrorOps:        errorOps,
		DurationSeconds: durSec,
		OpsPerSec:       float64(totalOps) / durSec,
		ReadOpsPerSec:   float64(readOps) / durSec,
		WriteOpsPerSec:  float64(writeOps) / durSec,
		ErrorRate:       errRate,
		Latency: observability.LatencySummary{
			P50: p50,
			P95: p95,
			P99: p99,
			Avg: avg,
		},
		MinLatencyMs:  minMs,
		MaxLatencyMs:  maxMs,
		MemoryAllocMB: float64(endMemAlloc) / (1024 * 1024),
		MemorySysMB:   float64(endMemSys) / (1024 * 1024),
		NumGoroutines: numGoroutines,
		NumGC:         numGC,
	}
}

// CompareRuns compares a candidate run against a baseline run.
func CompareRuns(baseline, candidate BenchmarkResult) BenchmarkComparison {
	opsDelta := 0.0
	if baseline.OpsPerSec > 0 {
		opsDelta = ((candidate.OpsPerSec - baseline.OpsPerSec) / baseline.OpsPerSec) * 100.0
	}

	p50Delta := 0.0
	if baseline.Latency.P50 > 0 {
		p50Delta = ((candidate.Latency.P50 - baseline.Latency.P50) / baseline.Latency.P50) * 100.0
	}

	p99Delta := 0.0
	if baseline.Latency.P99 > 0 {
		p99Delta = ((candidate.Latency.P99 - baseline.Latency.P99) / baseline.Latency.P99) * 100.0
	}

	errDelta := candidate.ErrorRate - baseline.ErrorRate

	return BenchmarkComparison{
		BaselineName:          baseline.Config.Name,
		CandidateName:         candidate.Config.Name,
		BaselineOpsSec:        baseline.OpsPerSec,
		CandidateOpsSec:       candidate.OpsPerSec,
		OpsSecDeltaPct:        opsDelta,
		BaselineP50Ms:         baseline.Latency.P50,
		CandidateP50Ms:        candidate.Latency.P50,
		P50DeltaPct:           p50Delta,
		BaselineP99Ms:         baseline.Latency.P99,
		CandidateP99Ms:        candidate.Latency.P99,
		P99DeltaPct:           p99Delta,
		ErrorRateDelta:        errDelta,
		IsThroughputRegressed: opsDelta < -10.0, // more than 10% regression
		IsLatencyRegressed:    p99Delta > 15.0,  // more than 15% latency increase
	}
}

// GenerateMarkdownReport creates a clean markdown summary of the benchmark result.
func GenerateMarkdownReport(res BenchmarkResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Benchmark Report: %s\n\n", res.Config.Name))
	sb.WriteString(fmt.Sprintf("- **Target**: `%s`\n", res.Config.Target))
	sb.WriteString(fmt.Sprintf("- **Scenario**: `%s`\n", res.Config.Scenario))
	sb.WriteString(fmt.Sprintf("- **Concurrency**: `%d` workers\n", res.Config.Concurrency))
	sb.WriteString(fmt.Sprintf("- **Read/Write Ratio**: `%.1f%% Read / %.1f%% Write`\n", res.Config.ReadRatio*100, (1-res.Config.ReadRatio)*100))
	sb.WriteString(fmt.Sprintf("- **Payload Size**: `%d bytes`\n", res.Config.ValueSize))
	sb.WriteString(fmt.Sprintf("- **Duration**: `%.2fs`\n\n", res.DurationSeconds))

	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("| --- | --- |\n")
	sb.WriteString(fmt.Sprintf("| Total Operations | %d |\n", res.TotalOps))
	sb.WriteString(fmt.Sprintf("| Throughput (Ops/sec) | **%.2f** |\n", res.OpsPerSec))
	sb.WriteString(fmt.Sprintf("| Read Throughput (Ops/sec) | %.2f |\n", res.ReadOpsPerSec))
	sb.WriteString(fmt.Sprintf("| Write Throughput (Ops/sec) | %.2f |\n", res.WriteOpsPerSec))
	sb.WriteString(fmt.Sprintf("| Error Rate | %.4f%% (%d errors) |\n", res.ErrorRate*100, res.ErrorOps))
	sb.WriteString(fmt.Sprintf("| Latency p50 | %.2f ms |\n", res.Latency.P50))
	sb.WriteString(fmt.Sprintf("| Latency p95 | %.2f ms |\n", res.Latency.P95))
	sb.WriteString(fmt.Sprintf("| Latency p99 | %.2f ms |\n", res.Latency.P99))
	sb.WriteString(fmt.Sprintf("| Latency Avg | %.2f ms |\n", res.Latency.Avg))
	sb.WriteString(fmt.Sprintf("| Latency Min / Max | %.2f ms / %.2f ms |\n", res.MinLatencyMs, res.MaxLatencyMs))
	sb.WriteString(fmt.Sprintf("| Memory Alloc / Sys | %.2f MB / %.2f MB |\n", res.MemoryAllocMB, res.MemorySysMB))
	sb.WriteString(fmt.Sprintf("| Goroutines / GCs | %d / %d |\n", res.NumGoroutines, res.NumGC))

	return sb.String()
}

// SaveResult persists the benchmark result to a JSON file.
func SaveResult(filePath string, res BenchmarkResult) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// AppendToHistory records the benchmark result into a historical JSON array file.
func AppendToHistory(historyFile string, res BenchmarkResult) error {
	var history []BenchmarkResult
	if data, err := os.ReadFile(historyFile); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &history)
	}
	history = append(history, res)
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(historyFile, data, 0644)
}

// LoadResult reads a benchmark result from a JSON file.
func LoadResult(filePath string) (*BenchmarkResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var res BenchmarkResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
