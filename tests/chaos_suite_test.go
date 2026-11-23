package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/chaos"
	"orbit/internal/raft"
	"orbit/internal/storage"
)

func TestChaos_NetworkPartitionsAndRecovery(t *testing.T) {
	net := raft.NewMockNetwork()
	mockTransA := net.CreateTransport("node-A")
	mockTransB := net.CreateTransport("node-B")
	cTransA := chaos.NewChaosTransport("node-A", mockTransA)
	cTransB := chaos.NewChaosTransport("node-B", mockTransB)

	// Partition node-A from node-B
	cTransA.PartitionBidirectional("node-B")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Verify RPC fails with network partition error
	_, err := cTransA.SendRequestVote(ctx, "node-B", raft.GroupID("1"), &raft.RequestVoteRequest{
		Term:        1,
		CandidateID: "node-A",
	})
	if err == nil || err != chaos.ErrNetworkPartition {
		t.Fatalf("expected ErrNetworkPartition, got %v", err)
	}

	// Heal partition
	cTransA.HealAll()
	cTransB.HealAll()

	// Verify RPC succeeds past chaos filter (falls through to underlying mock transport)
	// Underlying mock transport will return err or response, but not ErrNetworkPartition
	_, err = cTransA.SendRequestVote(ctx, "node-B", raft.GroupID("1"), &raft.RequestVoteRequest{
		Term:        1,
		CandidateID: "node-A",
	})
	if err == chaos.ErrNetworkPartition {
		t.Fatalf("expected partition to be healed, but got ErrNetworkPartition")
	}
}

func TestChaos_PacketLossAndRPCTimeout(t *testing.T) {
	net := raft.NewMockNetwork()
	mockTrans := net.CreateTransport("node-1")
	cTrans := chaos.NewChaosTransport("node-1", mockTrans)

	// Test 100% packet loss
	cTrans.SetPacketLoss(1.0)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := cTrans.SendAppendEntries(ctx, "node-2", raft.GroupID("1"), &raft.AppendEntriesRequest{Term: 1})
	if err != chaos.ErrPacketDropped {
		t.Fatalf("expected ErrPacketDropped, got %v", err)
	}

	// Reset loss, test forced timeout
	cTrans.SetPacketLoss(0.0)
	cTrans.SetForceTimeout(true)

	_, err = cTrans.SendAppendEntries(ctx, "node-2", raft.GroupID("1"), &raft.AppendEntriesRequest{Term: 1})
	if err != chaos.ErrRPCTimeout {
		t.Fatalf("expected ErrRPCTimeout, got %v", err)
	}
}

func TestChaos_CrashDuringWALAppendAndRecovery(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	// 1. Initialize clean engine and write valid entries
	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	_ = eng.Set("key1", "val1")
	_ = eng.Set("key2", "val2")
	_ = eng.Close()

	// 2. Inject torn/corrupt bytes at end of WAL
	if err := chaos.CrashDuringWALAppend(walPath); err != nil {
		t.Fatalf("failed to inject WAL crash: %v", err)
	}

	// 3. Restart engine — it must recover valid entries and truncate/ignore the corrupt tail
	recoveredEng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("failed to recover engine after WAL crash: %v", err)
	}
	defer recoveredEng.Close()

	v1, ok1 := recoveredEng.Get("key1")
	v2, ok2 := recoveredEng.Get("key2")
	if !ok1 || v1 != "val1" || !ok2 || v2 != "val2" {
		t.Fatalf("expected recovered data intact, got v1=%s(ok=%t), v2=%s(ok=%t)", v1, ok1, v2, ok2)
	}
}

func TestChaos_CrashDuringMemtableFlushAndCompaction(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")
	walPath := filepath.Join(tempDir, "wal.log")

	// Inject orphan .tmp files from mid-flush crash and mid-compaction crash
	tmpFlush, err := chaos.CrashDuringMemtableFlush(dataDir)
	if err != nil {
		t.Fatalf("failed to simulate flush crash: %v", err)
	}
	tmpCompact, err := chaos.CrashDuringCompaction(dataDir)
	if err != nil {
		t.Fatalf("failed to simulate compaction crash: %v", err)
	}

	// Verify temp files exist
	if _, err := os.Stat(tmpFlush); err != nil {
		t.Fatalf("expected flush tmp file to exist: %v", err)
	}
	if _, err := os.Stat(tmpCompact); err != nil {
		t.Fatalf("expected compaction tmp file to exist: %v", err)
	}

	// Start engine pointing to that directory
	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}
	defer eng.Close()

	// Clean orphan files via manifest cleaner
	if _, err := eng.Manifest().CleanOrphanFiles(filepath.Dir(walPath)); err != nil {
		t.Fatalf("failed to clean orphan files: %v", err)
	}
}

func TestChaos_CorruptSSTableDetection(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "wal.log")

	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Write enough keys and flush to produce an SSTable
	for i := 0; i < 50; i++ {
		_ = eng.Set(fmt.Sprintf("corrupt_k_%03d", i), "value_to_be_corrupted")
	}
	eng.RotateMemtable()
	if err := eng.Flush(); err != nil {
		t.Fatalf("failed to flush SSTable: %v", err)
	}
	_ = eng.Close()

	// Find the created .sst file in sstables dir
	sstDir := filepath.Join(tempDir, "sstables")
	entries, err := os.ReadDir(sstDir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	var sstFile string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".sst" {
			sstFile = filepath.Join(sstDir, e.Name())
			break
		}
	}
	if sstFile == "" {
		t.Fatalf("expected an SSTable file to be created")
	}

	// Corrupt the SSTable
	if err := chaos.CorruptSSTable(sstFile); err != nil {
		t.Fatalf("failed to corrupt sstable: %v", err)
	}

	// Reopening corrupted SSTable reader should fail checksum verification
	r, openErr := storage.OpenSSTableReader(sstFile)
	if openErr == nil {
		_ = r.Close()
		t.Fatalf("expected SSTable checksum error upon opening corrupted file, but got nil")
	}
}

func TestChaos_LabExperimentLifecycleAndReport(t *testing.T) {
	lab := chaos.NewChaosLab()

	cfg := chaos.ExperimentConfig{
		ID:        "exp-leader-kill-01",
		Name:      "Leader Crash Under Write Load",
		Target:    "shard-1-leader",
		FaultType: "kill-leader",
		Duration:  50 * time.Millisecond,
	}

	rep := lab.NewExperiment(cfg)
	if rep.State != chaos.StatePending {
		t.Fatalf("expected state PENDING, got %s", rep.State)
	}

	lab.RecordTimeline(cfg.ID, "FAULT_INJECT", "Leader process SIGKILL sent", map[string]any{"node_id": "node-1"})
	lab.SetState(cfg.ID, chaos.StateRecovering)
	lab.RecordTimeline(cfg.ID, "LEADER_ELECTED", "New leader node-2 elected for shard-1", map[string]any{"term": 2})
	lab.SetState(cfg.ID, chaos.StateVerifying)

	completed := lab.CompleteExperiment(cfg.ID, 35*time.Millisecond, true, nil)
	if completed.State != chaos.StateCompleted {
		t.Fatalf("expected StateCompleted, got %s", completed.State)
	}
	if completed.RecoveryTimeMs <= 0 {
		t.Fatalf("expected positive recovery time")
	}
	if !completed.DataConsistent {
		t.Fatalf("expected data consistent")
	}

	md, err := lab.GenerateMarkdownReport(cfg.ID)
	if err != nil {
		t.Fatalf("failed to generate markdown report: %v", err)
	}
	if len(md) == 0 {
		t.Fatalf("expected non-empty markdown report")
	}

	outFile := filepath.Join(t.TempDir(), "chaos_report.json")
	if err := lab.ExportReportJSON(cfg.ID, outFile); err != nil {
		t.Fatalf("failed to export JSON report: %v", err)
	}
}
