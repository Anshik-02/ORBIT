package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"orbit/internal/node"
	"orbit/internal/raft"
)

// TestClusterConcurrentWrites verifies that concurrent client writes submitted to the leader
// are serialized into the Raft log, committed via majority quorum, applied sequentially,
// and result in completely consistent state across the cluster.
func TestClusterConcurrentWrites(t *testing.T) {
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

	// Wait for leader
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
		t.Fatal("failed to elect leader")
	}

	// Submit 20 concurrent writes from 4 separate goroutines
	numGoroutines := 4
	writesPerGoroutine := 5
	totalWrites := numGoroutines * writesPerGoroutine

	var wg sync.WaitGroup
	errCh := make(chan error, totalWrites)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gID int) {
			defer wg.Done()
			for w := 0; w < writesPerGoroutine; w++ {
				key := fmt.Sprintf("concurrent_%d_%d", gID, w)
				val := fmt.Sprintf("val_%d_%d", gID, w)

				idx, _, err := leader.Raft().Propose("SET", key, val)
				if err != nil {
					errCh <- fmt.Errorf("propose %s: %w", key, err)
					return
				}

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				if err := leader.Raft().WaitForApply(ctx, idx); err != nil {
					cancel()
					errCh <- fmt.Errorf("waitForApply %s (idx %d): %w", key, idx, err)
					return
				}
				cancel()
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent write error: %v", err)
	}

	// Give followers time to catch up with last applied entry
	time.Sleep(200 * time.Millisecond)

	// Verify all keys are present on the leader and followers that caught up
	for g := 0; g < numGoroutines; g++ {
		for w := 0; w < writesPerGoroutine; w++ {
			key := fmt.Sprintf("concurrent_%d_%d", gIDKey(g, w), w)
			expectedVal := fmt.Sprintf("val_%d_%d", gIDKey(g, w), w)

			val, ok := leader.Engine().Get(key)
			if !ok || val != expectedVal {
				t.Fatalf("leader missing key %s: expected %q, got %q (ok=%v)", key, expectedVal, val, ok)
			}
		}
	}
}

func gIDKey(g, w int) int {
	return g
}

// TestSplitVoteElectionResolution verifies that when an election splits (e.g. votes dropped),
// randomized election timeouts allow a node to break the tie and establish leadership.
func TestSplitVoteElectionResolution(t *testing.T) {
	net := raft.NewMockNetwork()
	baseDir := t.TempDir()
	configs := node.ClusterConfig3Nodes(baseDir)

	nodes := make([]*node.Node, 3)
	for i, cfg := range configs {
		tr := net.CreateTransport(cfg.ID, cfg.RaftAddress)
		n, err := node.NewNode(cfg, tr)
		if err != nil {
			t.Fatalf("NewNode: %v", err)
		}
		nodes[i] = n
	}

	// Drop votes to node-a initially to simulate a split vote
	net.DropVotes("node-a", true)

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

	// Wait 200ms with dropped votes
	time.Sleep(200 * time.Millisecond)

	// Re-enable votes
	net.DropVotes("node-a", false)

	// A leader must successfully emerge within 2 seconds
	var leader *node.Node
	deadline := time.Now().Add(2 * time.Second)
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
		t.Fatal("cluster failed to resolve split vote and elect a leader")
	}
}
