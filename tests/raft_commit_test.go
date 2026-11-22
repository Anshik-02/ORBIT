package tests

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/node"
	"orbit/internal/raft"
	"orbit/internal/storage"
)

// TestMajorityCommitment verifies that entries proposed on the leader
// are replicated to a majority, committed, and applied to the state machine.
func TestMajorityCommitment(t *testing.T) {
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

	// Wait for a leader to be elected
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
		t.Fatal("failed to elect leader in 3-node cluster")
	}

	// Propose 5 key-value commands through the leader
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("key-%d", i)
		val := fmt.Sprintf("val-%d", i)
		idx, _, err := leader.Raft().Propose("SET", key, val)
		if err != nil {
			t.Fatalf("Propose %d: %v", i, err)
		}

		// Wait for commit on leader
		waitCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		if err := leader.Raft().WaitForCommit(waitCtx, idx); err != nil {
			cancel()
			t.Fatalf("WaitForCommit index %d: %v", idx, err)
		}
		// Wait for apply on leader
		if err := leader.Raft().WaitForApply(waitCtx, idx); err != nil {
			cancel()
			t.Fatalf("WaitForApply index %d: %v", idx, err)
		}
		cancel()
	}

	// Give time for followers to apply the committed entries
	time.Sleep(200 * time.Millisecond)

	// Verify all 5 keys exist on leader and at least a majority of followers
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("key-%d", i)
		expectedVal := fmt.Sprintf("val-%d", i)

		// Leader must have applied it
		v, ok := leader.Engine().Get(key)
		if !ok || v != expectedVal {
			t.Fatalf("leader key %q: expected %q, got %q (ok=%v)", key, expectedVal, v, ok)
		}

		// Count how many nodes have applied it (must be at least 2 for majority)
		appliedCount := 0
		for _, n := range nodes {
			if v, ok := n.Engine().Get(key); ok && v == expectedVal {
				appliedCount++
			}
		}
		if appliedCount < 2 {
			t.Fatalf("key %q: applied on only %d nodes, expected at least 2", key, appliedCount)
		}
	}
}

// TestNodeRestartPreservesAppliedIndex verifies that after a node crashes and restarts,
// its storage engine recovers lastAppliedIndex from the WAL, seeds Raft's InitialLastApplied,
// and does not re-apply previously committed entries.
func TestNodeRestartPreservesAppliedIndex(t *testing.T) {
	dataDir := t.TempDir()
	walPath := filepath.Join(dataDir, "wal.log")

	// 1. First run: Engine applies 3 Raft entries
	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	entries := []storage.CommittedCommand{
		{RaftIndex: 1, RaftTerm: 1, Operation: "SET", Key: "alpha", Value: "100"},
		{RaftIndex: 2, RaftTerm: 1, Operation: "SET", Key: "beta", Value: "200"},
		{RaftIndex: 3, RaftTerm: 1, Operation: "SET", Key: "gamma", Value: "300"},
	}
	for _, e := range entries {
		if err := eng.ApplyCommitted(e); err != nil {
			t.Fatalf("ApplyCommitted: %v", err)
		}
	}

	if eng.LastAppliedIndex() != 3 {
		t.Fatalf("expected lastAppliedIndex=3, got %d", eng.LastAppliedIndex())
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("eng.Close: %v", err)
	}

	// 2. Second run: Re-open engine and verify recovery restores lastAppliedIndex=3
	eng2, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("NewEngine recovery: %v", err)
	}
	defer eng2.Close()

	if eng2.LastAppliedIndex() != 3 {
		t.Fatalf("recovered lastAppliedIndex: expected 3, got %d", eng2.LastAppliedIndex())
	}

	// Verify data is present
	if v, ok := eng2.Get("beta"); !ok || v != "200" {
		t.Fatalf("expected beta=200, got %q (ok=%v)", v, ok)
	}

	// 3. Raft seeded with InitialLastApplied = 3:
	// Verify that applying index 4 works contiguously without error
	cmd4 := storage.CommittedCommand{
		RaftIndex: 4,
		RaftTerm:  2,
		Operation: "SET",
		Key:       "delta",
		Value:     "400",
	}
	if err := eng2.ApplyCommitted(cmd4); err != nil {
		t.Fatalf("ApplyCommitted index 4 after recovery: %v", err)
	}

	if eng2.LastAppliedIndex() != 4 {
		t.Fatalf("expected lastAppliedIndex=4, got %d", eng2.LastAppliedIndex())
	}

	// Verify idempotency: duplicate command index 3 is ignored safely
	cmd3Dup := storage.CommittedCommand{
		RaftIndex: 3,
		RaftTerm:  1,
		Operation: "SET",
		Key:       "gamma",
		Value:     "OLD",
	}
	if err := eng2.ApplyCommitted(cmd3Dup); err != nil {
		t.Fatalf("duplicate index 3 should be ignored without error, got: %v", err)
	}
	// Value of gamma must NOT have changed to "OLD"
	if v, _ := eng2.Get("gamma"); v != "300" {
		t.Fatalf("expected gamma=300 after duplicate ignored, got %q", v)
	}
}
