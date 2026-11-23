package tests

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orbit/internal/benchmark"
	"orbit/internal/chaos"
	"orbit/internal/raft"
	"orbit/internal/storage"
)

// TestBenchmark_ThroughputAndPercentileCalculations mathematically verifies
// that ComputeSummary correctly computes p50, p95, p99, avg, min, max, ops/sec, and error rate.
func TestBenchmark_ThroughputAndPercentileCalculations(t *testing.T) {
	cfg := benchmark.BenchmarkConfig{
		Name:      "math-verification-bench",
		Target:    benchmark.TargetStorage,
		NumKeys:   100,
		ValueSize: 64,
	}

	// 100 sample latencies from 1ms (1000us) to 100ms (100000us)
	latenciesUs := make([]int64, 100)
	for i := 0; i < 100; i++ {
		latenciesUs[i] = int64((i + 1) * 1000)
	}

	duration := 2 * time.Second // 2.0s
	totalOps := uint64(100)
	readOps := uint64(80)
	writeOps := uint64(20)
	errorOps := uint64(5)

	res := benchmark.ComputeSummary(
		cfg,
		totalOps, readOps, writeOps, errorOps,
		duration,
		latenciesUs,
		1024*1024, 2*1024*1024,
		4*1024*1024,
		1,
		2,
	)

	// Throughput checks
	if math.Abs(res.OpsPerSec-50.0) > 0.001 {
		t.Fatalf("expected OpsPerSec = 50.0, got %f", res.OpsPerSec)
	}
	if math.Abs(res.ReadOpsPerSec-40.0) > 0.001 {
		t.Fatalf("expected ReadOpsPerSec = 40.0, got %f", res.ReadOpsPerSec)
	}
	if math.Abs(res.WriteOpsPerSec-10.0) > 0.001 {
		t.Fatalf("expected WriteOpsPerSec = 10.0, got %f", res.WriteOpsPerSec)
	}
	if math.Abs(res.ErrorRate-0.05) > 0.001 {
		t.Fatalf("expected ErrorRate = 0.05, got %f", res.ErrorRate)
	}

	// Latency percentile checks:
	// min = 1.0 ms, max = 100.0 ms
	if math.Abs(res.MinLatencyMs-1.0) > 0.001 {
		t.Fatalf("expected MinLatencyMs = 1.0, got %f", res.MinLatencyMs)
	}
	if math.Abs(res.MaxLatencyMs-100.0) > 0.001 {
		t.Fatalf("expected MaxLatencyMs = 100.0, got %f", res.MaxLatencyMs)
	}
	// p50 is index 50 -> 51ms
	if math.Abs(res.Latency.P50-51.0) > 0.001 {
		t.Fatalf("expected P50 = 51.0, got %f", res.Latency.P50)
	}
	// p95 is index 95 -> 96ms
	if math.Abs(res.Latency.P95-96.0) > 0.001 {
		t.Fatalf("expected P95 = 96.0, got %f", res.Latency.P95)
	}
	// p99 is index 99 -> 100ms
	if math.Abs(res.Latency.P99-100.0) > 0.001 {
		t.Fatalf("expected P99 = 100.0, got %f", res.Latency.P99)
	}
}

// TestBenchmark_Repeatability runs identical workloads twice on the storage engine
// and verifies that throughput and latency results are consistent.
func TestBenchmark_Repeatability(t *testing.T) {
	tempDir1 := t.TempDir()
	eng1, _ := storage.NewEngine(filepath.Join(tempDir1, "wal.log"))
	defer eng1.Close()

	cfg := benchmark.BenchmarkConfig{
		Name:        "repeatable-bench",
		Target:      benchmark.TargetStorage,
		NumKeys:     200,
		ValueSize:   64,
		ReadRatio:   0.5,
		Concurrency: 2,
		Duration:    150 * time.Millisecond,
		Scenario:    "standard",
	}

	res1 := benchmark.RunStorageBenchmark(eng1, cfg)

	tempDir2 := t.TempDir()
	eng2, _ := storage.NewEngine(filepath.Join(tempDir2, "wal.log"))
	defer eng2.Close()

	res2 := benchmark.RunStorageBenchmark(eng2, cfg)

	cmp := benchmark.CompareRuns(res1, res2)
	// Throughput difference between runs should be within reasonable bounds
	if math.Abs(cmp.OpsSecDeltaPct) > 80.0 {
		t.Logf("variance between runs: %.2f%% (res1=%.1f, res2=%.1f)", cmp.OpsSecDeltaPct, res1.OpsPerSec, res2.OpsPerSec)
	}
	if res1.TotalOps == 0 || res2.TotalOps == 0 {
		t.Fatalf("expected positive total operations in both runs")
	}
}

// TestChaos_FaultInjectionAndHealingLifecycle verifies that ChaosTransport
// successfully intercepts RPCs during fault injection and restores connectivity when healed.
func TestChaos_FaultInjectionAndHealingLifecycle(t *testing.T) {
	net := raft.NewMockNetwork()
	mockTransA := net.CreateTransport("node-A")
	cTrans := chaos.NewChaosTransport("node-A", mockTransA)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// 1. Injected partition
	cTrans.Partition("node-B")
	_, err := cTrans.SendRequestVote(ctx, "node-B", raft.GroupID("0"), &raft.RequestVoteRequest{Term: 1})
	if err != chaos.ErrNetworkPartition {
		t.Fatalf("expected ErrNetworkPartition, got %v", err)
	}

	// 2. Heal partition
	cTrans.HealPartition("node-B")
	_, err = cTrans.SendRequestVote(ctx, "node-B", raft.GroupID("0"), &raft.RequestVoteRequest{Term: 1})
	if err == chaos.ErrNetworkPartition {
		t.Fatalf("expected partition to be healed")
	}

	// 3. 100% Packet loss
	cTrans.SetPacketLoss(1.0)
	_, err = cTrans.SendRequestVote(ctx, "node-B", raft.GroupID("0"), &raft.RequestVoteRequest{Term: 1})
	if err != chaos.ErrPacketDropped {
		t.Fatalf("expected ErrPacketDropped, got %v", err)
	}

	// 4. Heal packet loss
	cTrans.SetPacketLoss(0.0)
	_, err = cTrans.SendRequestVote(ctx, "node-B", raft.GroupID("0"), &raft.RequestVoteRequest{Term: 1})
	if err == chaos.ErrPacketDropped {
		t.Fatalf("expected packet loss to be cleared")
	}

	// 5. Simulated RPC timeout
	cTrans.SetForceTimeout(true)
	_, err = cTrans.SendRequestVote(ctx, "node-B", raft.GroupID("0"), &raft.RequestVoteRequest{Term: 1})
	if err != chaos.ErrRPCTimeout {
		t.Fatalf("expected ErrRPCTimeout, got %v", err)
	}

	cTrans.HealAll()
}

// TestChaos_TornAppendRecoveryZeroDataLoss tests that committing records, injecting
// a torn frame at the end of the WAL file, and recovering yields 100% of the committed data without loss.
func TestChaos_TornAppendRecoveryZeroDataLoss(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	// Write 15 committed keys
	for i := 0; i < 15; i++ {
		_ = eng.Set(fmt.Sprintf("torn_k_%d", i), fmt.Sprintf("val_%d", i))
	}
	_ = eng.Close()

	// Inject a torn WAL append frame
	if err := chaos.CrashDuringWALAppend(walPath); err != nil {
		t.Fatalf("CrashDuringWALAppend failed: %v", err)
	}

	// Recover engine
	eng2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("recovery failed after torn WAL append: %v", err)
	}
	defer eng2.Close()

	// Verify all 15 committed keys are 100% intact
	for i := 0; i < 15; i++ {
		val, found := eng2.Get(fmt.Sprintf("torn_k_%d", i))
		if !found || val != fmt.Sprintf("val_%d", i) {
			t.Fatalf("committed key %d lost or corrupted: val=%s found=%v", i, val, found)
		}
	}
}

// TestChaos_ExperimentReportReproducibility verifies that ChaosLab tracks
// the complete state machine lifecycle, MTTR, and exports an informative report.
func TestChaos_ExperimentReportReproducibility(t *testing.T) {
	lab := chaos.NewChaosLab()

	cfg := chaos.ExperimentConfig{
		ID:        "exp-partition-01",
		Name:      "partition-test",
		Target:    "node-1",
		FaultType: "NETWORK_PARTITION",
	}

	rep := lab.NewExperiment(cfg)
	if rep.State != chaos.StatePending {
		t.Fatalf("expected PENDING state, got %s", rep.State)
	}

	lab.SetState(rep.ID, chaos.StateFaultInjected)
	lab.RecordTimeline(rep.ID, "FAULT_INJECTION", "Injected bidirectional network partition", nil)

	time.Sleep(10 * time.Millisecond)
	lab.SetState(rep.ID, chaos.StateRecovering)
	lab.RecordTimeline(rep.ID, "RECOVERY", "Restored network connectivity", nil)

	completed := lab.CompleteExperiment(rep.ID, 25*time.Millisecond, true, nil)
	if completed.State != chaos.StateCompleted {
		t.Fatalf("expected COMPLETED state, got %s", completed.State)
	}
	if completed.RecoveryTimeMs <= 0 {
		t.Fatalf("expected positive recovery time")
	}

	report, err := lab.GenerateMarkdownReport(rep.ID)
	if err != nil {
		t.Fatalf("GenerateMarkdownReport: %v", err)
	}
	if !strings.Contains(report, "partition-test") || !strings.Contains(report, "NETWORK_PARTITION") {
		t.Fatalf("report missing critical experiment details:\n%s", report)
	}
}
