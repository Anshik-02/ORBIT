package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"orbit/internal/node"
	"orbit/internal/raft"
)

// TestConcurrentReadIndexDuringLeaderStepDown tests high-concurrency ReadIndex
// requests executing at the exact moment the leader is deposed / forced to step down.
// All requests must either safely obtain a quorum-confirmed ReadIndex or cleanly fail
// with ErrNotLeader / ErrQuorumLost without panics, data races, or deadlocks.
func TestConcurrentReadIndexDuringLeaderStepDown(t *testing.T) {
	net := raft.NewMockNetwork()
	baseDir := t.TempDir()
	configs := node.ClusterConfig3Nodes(baseDir)

	nodes := make([]*node.Node, 3)
	for i, cfg := range configs {
		tr := net.CreateTransport(cfg.ID, cfg.RaftAddress)
		n, err := node.NewNode(cfg, tr)
		if err != nil {
			t.Fatalf("NewNode %s: %v", cfg.ID, err)
		}
		nodes[i] = n
	}

	for _, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
	defer func() {
		for _, n := range nodes {
			_ = n.Stop()
		}
	}()

	// Wait for leader election
	var leader *node.Node
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range nodes {
			if n.Raft().Role() == raft.Leader {
				leader = n
				break
			}
		}
		if leader != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if leader == nil {
		t.Fatal("failed to elect initial leader")
	}

	// Commit an initial entry
	idx, _, err := leader.Raft().Propose("SET", "foo", "bar")
	if err != nil {
		t.Fatalf("propose initial: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := leader.Raft().WaitForApply(ctx, idx); err != nil {
		cancel()
		t.Fatalf("apply initial: %v", err)
	}
	cancel()

	const numReaders = 30
	var wg sync.WaitGroup
	var successCount int32
	var errorCount int32

	// Launch concurrent ReadIndex callers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			readCtx, readCancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
			defer readCancel()

			readIdx, readErr := leader.Raft().ReadIndex(readCtx)
			if readErr != nil {
				atomic.AddInt32(&errorCount, 1)
				// Legitimate expected errors when leader steps down
				if !errors.Is(readErr, raft.ErrNotLeader) &&
					!errors.Is(readErr, raft.ErrQuorumLost) &&
					!errors.Is(readErr, context.DeadlineExceeded) &&
					!errors.Is(readErr, context.Canceled) {
					t.Errorf("unexpected ReadIndex error: %v", readErr)
				}
			} else {
				atomic.AddInt32(&successCount, 1)
				if readIdx < idx {
					t.Errorf("ReadIndex returned %d, expected at least %d", readIdx, idx)
				}
			}
		}()
	}

	// Concurrently disconnect the leader so it loses quorum
	time.Sleep(5 * time.Millisecond)
	net.Isolate(leader.ID())

	wg.Wait()
	t.Logf("ReadIndex during leader isolation: %d succeeded before isolation, %d cleanly rejected",
		atomic.LoadInt32(&successCount), atomic.LoadInt32(&errorCount))
}

// TestFatalStorageErrorStopsConsensusAndRecoversOnRestart verifies:
// 1. Storage persistence failure triggers fatal halt (StorageErr() != nil).
// 2. Further proposals return ErrStorageHalted.
// 3. Restarting the node with healthy storage reloads durable state and recovers.
func TestFatalStorageErrorStopsConsensusAndRecoversOnRestart(t *testing.T) {
	dataDir := t.TempDir()
	st, err := raft.OpenFileStorage(dataDir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}

	failStorage := &failingStorage{
		Storage:           st,
		failAppendEntries: true,
	}

	net := raft.NewMockNetwork()
	tr := net.CreateTransport("node-1")

	r, err := raft.New(raft.Config{
		ID:        "node-1",
		Storage:   failStorage,
		Transport: tr,
	})
	if err != nil {
		t.Fatalf("new raft: %v", err)
	}

	if err := r.Start(); err != nil {
		t.Fatalf("start raft: %v", err)
	}

	// Wait for single node to become leader
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r.Role() == raft.Leader {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r.Role() != raft.Leader {
		_ = r.Stop()
		t.Fatalf("single node did not become leader")
	}

	// Propose should fail on AppendEntries persistence
	_, _, err = r.Propose("SET", "alpha", "beta")
	if err == nil {
		_ = r.Stop()
		t.Fatal("expected proposal to fail due to injected persistence error")
	}

	// Verify node entered fatal storage error state
	if r.StorageErr() == nil {
		_ = r.Stop()
		t.Fatal("expected r.StorageErr() to be non-nil after persistence failure")
	}

	// Subsequent proposal should fail immediately with ErrStorageHalted
	_, _, err = r.Propose("SET", "gamma", "delta")
	if !errors.Is(err, raft.ErrStorageHalted) {
		_ = r.Stop()
		t.Fatalf("expected ErrStorageHalted, got: %v", err)
	}

	// Stop the failed engine and close failing storage
	_ = r.Stop()
	_ = failStorage.Close()

	// --- RESTART RECOVERY ---
	// Reopen the real storage cleanly from disk
	recoveredStorage, err := raft.OpenFileStorage(dataDir)
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	defer recoveredStorage.Close()

	r2, err := raft.New(raft.Config{
		ID:        "node-1",
		Storage:   recoveredStorage,
		Transport: net.CreateTransport("node-1"),
	})
	if err != nil {
		t.Fatalf("create restarted raft: %v", err)
	}

	if err := r2.Start(); err != nil {
		t.Fatalf("start restarted raft: %v", err)
	}
	defer r2.Stop()

	// Verify the restarted node has no storage error
	if r2.StorageErr() != nil {
		t.Fatalf("restarted node should have nil StorageErr(), got: %v", r2.StorageErr())
	}

	// Wait for leader election
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r2.Role() == raft.Leader {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r2.Role() != raft.Leader {
		t.Fatalf("restarted node did not become leader")
	}

	// Verify proposals work normally now
	idx, _, err := r2.Propose("SET", "restarted", "healthy")
	if err != nil {
		t.Fatalf("proposal after restart failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r2.WaitForApply(ctx, idx); err != nil {
		t.Fatalf("WaitForApply after restart failed: %v", err)
	}
}

// TestLogTruncationCrashRecovery verifies that OpenFileStorage properly handles
// crashes that occurred mid-atomic file replacement (such as truncation or state save).
func TestLogTruncationCrashRecovery(t *testing.T) {
	dataDir := t.TempDir()

	// 1. Initialize storage and write initial entries
	st, err := raft.OpenFileStorage(dataDir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	err = st.AppendEntries([]raft.LogEntry{
		{Index: 1, Term: 1, Operation: "SET", Key: "k1", Value: "v1"},
		{Index: 2, Term: 1, Operation: "SET", Key: "k2", Value: "v2"},
		{Index: 3, Term: 1, Operation: "SET", Key: "k3", Value: "v3"},
	})
	if err != nil {
		_ = st.Close()
		t.Fatalf("append entries: %v", err)
	}
	_ = st.Close()

	// Scenario A: Simulate crash after primary raft.log was unlinked/missing,
	// but raft.log.tmp was written and fsynced.
	primaryLog := filepath.Join(dataDir, "raft", "raft.log")
	tmpLog := filepath.Join(dataDir, "raft", "raft.log.tmp")

	// Rename primary to .tmp to simulate interrupted rename
	if err := os.Rename(primaryLog, tmpLog); err != nil {
		t.Fatalf("simulate crash rename: %v", err)
	}

	// Reopen storage: OpenFileStorage should detect missing primary + existing .tmp,
	// and automatically promote .tmp to raft.log
	recoveredSt, err := raft.OpenFileStorage(dataDir)
	if err != nil {
		t.Fatalf("OpenFileStorage failed to recover from .tmp: %v", err)
	}
	lastIdx, _ := recoveredSt.LastLogInfo()
	if lastIdx != 3 {
		t.Fatalf("expected LastIndex 3, got %d", lastIdx)
	}
	_ = recoveredSt.Close()

	// Scenario B: Simulate crash before primary was replaced, leaving a stale .tmp
	// Recreate a stale .tmp alongside existing raft.log
	if err := os.WriteFile(tmpLog, []byte("stale unfinished write\n"), 0600); err != nil {
		t.Fatalf("create stale .tmp: %v", err)
	}

	recoveredSt2, err := raft.OpenFileStorage(dataDir)
	if err != nil {
		t.Fatalf("OpenFileStorage failed to clean stale .tmp: %v", err)
	}
	defer recoveredSt2.Close()

	lastIdx2, _ := recoveredSt2.LastLogInfo()
	if lastIdx2 != 3 {
		t.Fatalf("expected LastIndex 3, got %d", lastIdx2)
	}
	// Verify stale .tmp was cleaned up
	if _, err := os.Stat(tmpLog); !os.IsNotExist(err) {
		t.Errorf("expected stale .tmp to be removed, but it still exists")
	}
}

// TestLifecycleRepeatedStartStop tests idempotency of Start and Stop methods on
// both Raft and Node instances.
func TestLifecycleRepeatedStartStop(t *testing.T) {
	dataDir := t.TempDir()
	st, err := raft.OpenFileStorage(dataDir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer st.Close()

	net := raft.NewMockNetwork()
	tr := net.CreateTransport("node-lifecycle")

	r, err := raft.New(raft.Config{
		ID:        "node-lifecycle",
		Storage:   st,
		Transport: tr,
	})
	if err != nil {
		t.Fatalf("create raft: %v", err)
	}

	// Repeated Start() on Raft
	for i := 0; i < 3; i++ {
		if err := r.Start(); err != nil {
			t.Fatalf("repeated Raft.Start() iteration %d failed: %v", i, err)
		}
	}

	// Repeated Stop() on Raft
	for i := 0; i < 3; i++ {
		if err := r.Stop(); err != nil {
			t.Fatalf("repeated Raft.Stop() iteration %d failed: %v", i, err)
		}
	}

	// Test Node lifecycle
	nodeDir := t.TempDir()
	cfg := node.NodeConfig{
		ID:                 "node-1",
		DataDir:            nodeDir,
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 150 * time.Millisecond,
		ElectionTimeoutMax: 300 * time.Millisecond,
	}
	n, err := node.NewNode(cfg, nil)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}

	// Repeated Node.Start()
	for i := 0; i < 3; i++ {
		if err := n.Start(); err != nil {
			t.Fatalf("repeated Node.Start() iteration %d failed: %v", i, err)
		}
	}

	// Repeated Node.Stop()
	for i := 0; i < 3; i++ {
		if err := n.Stop(); err != nil {
			t.Fatalf("repeated Node.Stop() iteration %d failed: %v", i, err)
		}
	}

	// Starting a stopped node should fail gracefully with error, not panic
	if err := n.Start(); err == nil {
		t.Fatal("expected Start() after Stop() to return an error")
	}
}
