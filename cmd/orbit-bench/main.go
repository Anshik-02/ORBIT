package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"orbit/internal/benchmark"
	"orbit/internal/database"
	"orbit/internal/storage"
)

func main() {
	target := flag.String("target", "storage", "Benchmark target: storage, database, or cluster")
	concurrency := flag.Int("concurrency", 4, "Number of concurrent worker goroutines")
	duration := flag.Duration("duration", 3*time.Second, "Duration of the benchmark run")
	numKeys := flag.Int("keys", 5000, "Number of distinct keys")
	valueSize := flag.Int("valsize", 256, "Size of values in bytes")
	readRatio := flag.Float64("read-ratio", 0.5, "Fraction of read operations (0.0 to 1.0)")
	baseURL := flag.String("url", "http://localhost:8001", "Base URL for cluster target")
	scenario := flag.String("scenario", "standard", "Scenario: standard, during_compaction, during_snapshots")
	outputFile := flag.String("output", "", "Optional path to save JSON results")
	historyFile := flag.String("history", "", "Optional path to append historical JSON results")
	compareFile := flag.String("compare", "", "Optional baseline JSON file to compare against")
	flag.Parse()

	cfg := benchmark.BenchmarkConfig{
		Name:        fmt.Sprintf("%s-%s-%dw", *target, *scenario, *concurrency),
		Target:      benchmark.BenchmarkTarget(*target),
		NumKeys:     *numKeys,
		ValueSize:   *valueSize,
		ReadRatio:   *readRatio,
		Concurrency: *concurrency,
		Duration:    *duration,
		BaseURL:     *baseURL,
		Scenario:    *scenario,
	}

	fmt.Printf("=== Starting ORBIT Benchmark ===\n")
	fmt.Printf("Target: %s | Scenario: %s | Workers: %d | Duration: %v\n", cfg.Target, cfg.Scenario, cfg.Concurrency, cfg.Duration)

	var res benchmark.BenchmarkResult
	switch cfg.Target {
	case benchmark.TargetStorage:
		tempDir, err := os.MkdirTemp("", "orbit-bench-storage-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
			os.Exit(1)
		}
		defer os.RemoveAll(tempDir)

		eng, err := storage.NewEngine(filepath.Join(tempDir, "wal.log"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to init storage engine: %v\n", err)
			os.Exit(1)
		}
		defer eng.Close()
		res = benchmark.RunStorageBenchmark(eng, cfg)

	case benchmark.TargetDatabase:
		tempDir, err := os.MkdirTemp("", "orbit-bench-db-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
			os.Exit(1)
		}
		defer os.RemoveAll(tempDir)

		eng, err := storage.NewEngine(filepath.Join(tempDir, "wal.log"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to init storage engine: %v\n", err)
			os.Exit(1)
		}
		defer eng.Close()

		db, err := database.NewDatabase(eng)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to init database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
		res = benchmark.RunDatabaseBenchmark(db, cfg)

	case benchmark.TargetCluster:
		client := &http.Client{Timeout: 10 * time.Second}
		res = benchmark.RunClusterBenchmark(client, cfg)

	default:
		fmt.Fprintf(os.Stderr, "unknown target: %s\n", *target)
		os.Exit(1)
	}

	report := benchmark.GenerateMarkdownReport(res)
	fmt.Println("\n" + report)

	if *outputFile != "" {
		if err := benchmark.SaveResult(*outputFile, res); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save output: %v\n", err)
		} else {
			fmt.Printf("Saved results to %s\n", *outputFile)
		}
	}

	if *historyFile != "" {
		if err := benchmark.AppendToHistory(*historyFile, res); err != nil {
			fmt.Fprintf(os.Stderr, "failed to append history: %v\n", err)
		}
	}

	if *compareFile != "" {
		baseline, err := benchmark.LoadResult(*compareFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load comparison baseline: %v\n", err)
		} else {
			cmp := benchmark.CompareRuns(*baseline, res)
			fmt.Printf("\n### Comparison vs Baseline (%s)\n", cmp.BaselineName)
			fmt.Printf("- Ops/sec: %.2f -> %.2f (%.2f%%)\n", cmp.BaselineOpsSec, cmp.CandidateOpsSec, cmp.OpsSecDeltaPct)
			fmt.Printf("- p50 Latency: %.2fms -> %.2fms (%.2f%%)\n", cmp.BaselineP50Ms, cmp.CandidateP50Ms, cmp.P50DeltaPct)
			fmt.Printf("- p99 Latency: %.2fms -> %.2fms (%.2f%%)\n", cmp.BaselineP99Ms, cmp.CandidateP99Ms, cmp.P99DeltaPct)
			if cmp.IsThroughputRegressed {
				fmt.Println("⚠️  WARNING: Significant throughput regression detected (>10%)")
			}
			if cmp.IsLatencyRegressed {
				fmt.Println("⚠️  WARNING: Significant latency regression detected (>15%)")
			}
		}
	}
}
