package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"orbit/internal/node"
	"orbit/internal/raft"
)

// TestRestartedNodeConvergesWithCluster verifies that a node stopped during replication
// converges with the cluster upon restart, applying all committed entries sequentially.
func TestRestartedNodeConvergesWithCluster(t *testing.T) {
	baseDir := t.TempDir()
	configs := node.ClusterConfig3Nodes(baseDir)

	mockNet := raft.NewMockNetwork()
	nodes := make([]*node.Node, 3)

	defer func() {
		for _, n := range nodes {
			if n != nil {
				_ = n.Stop()
			}
		}
	}()

	// 1. Boot initial 3-node cluster
	for i, cfg := range configs {
		tr := mockNet.CreateTransport(cfg.ID, cfg.RaftAddress)
		n, err := node.NewNode(cfg, tr)
		if err != nil {
			t.Fatalf("create node %d: %v", i, err)
		}
		if err := n.Start(); err != nil {
			t.Fatalf("start node %d: %v", i, err)
		}
		nodes[i] = n
	}

	// 2. Wait for a leader to be elected
	var leader *node.Node
	deadline := time.Now().Add(5 * time.Second)
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
		t.Fatal("no leader elected within timeout")
	}

	// 3. Propose 3 entries through leader
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i := 1; i <= 3; i++ {
		idx, _, err := leader.Raft().Propose("SET", fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
		if err != nil {
			t.Fatalf("propose entry %d: %v", i, err)
		}
		if err := leader.Raft().WaitForApply(ctx, idx); err != nil {
			t.Fatalf("wait for apply %d: %v", idx, err)
		}
	}

	// 4. Identify a follower to stop
	var followerIdx int
	for i, n := range nodes {
		if n.ID() != leader.ID() {
			followerIdx = i
			break
		}
	}

	stoppedFollowerID := nodes[followerIdx].ID()
	if err := nodes[followerIdx].Stop(); err != nil {
		t.Fatalf("stop follower: %v", err)
	}
	nodes[followerIdx] = nil

	// 5. Propose 2 more entries with follower stopped (2 nodes form majority)
	for i := 4; i <= 5; i++ {
		idx, _, err := leader.Raft().Propose("SET", fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
		if err != nil {
			t.Fatalf("propose entry %d with stopped follower: %v", i, err)
		}
		if err := leader.Raft().WaitForApply(ctx, idx); err != nil {
			t.Fatalf("wait for apply %d on remaining quorum: %v", idx, err)
		}
	}

	// Verify remaining nodes have committed and applied index 5
	for _, n := range nodes {
		if n != nil {
			waitDeadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(waitDeadline) && n.Engine().LastAppliedIndex() < 5 {
				time.Sleep(10 * time.Millisecond)
			}
			if n.Engine().LastAppliedIndex() != 5 {
				t.Fatalf("node %s: expected applied index 5, got %d", n.ID(), n.Engine().LastAppliedIndex())
			}
		}
	}

	// 6. Restart the stopped follower using its original configuration
	restartedTr := mockNet.CreateTransport(stoppedFollowerID, configs[followerIdx].RaftAddress)
	restartedNode, err := node.NewNode(configs[followerIdx], restartedTr)
	if err != nil {
		t.Fatalf("re-create node %s: %v", stoppedFollowerID, err)
	}
	if err := restartedNode.Start(); err != nil {
		t.Fatalf("start restarted node %s: %v", stoppedFollowerID, err)
	}
	nodes[followerIdx] = restartedNode

	// 7. Wait for restarted follower to catch up with leader
	catchupDeadline := time.Now().Add(5 * time.Second)
	caughtUp := false
	for time.Now().Before(catchupDeadline) {
		if restartedNode.Engine().LastAppliedIndex() == 5 && restartedNode.Raft().CommitIndex() >= 5 {
			caughtUp = true
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	if !caughtUp {
		t.Fatalf("restarted follower %s failed to catch up: applied %d, commit %d",
			stoppedFollowerID, restartedNode.Engine().LastAppliedIndex(), restartedNode.Raft().CommitIndex())
	}

	// 8. Verify restarted node contains all 5 keys
	for i := 1; i <= 5; i++ {
		val, err := restartedNode.DB().Get(fmt.Sprintf("k%d", i))
		if err != nil {
			t.Fatalf("restarted node missing key k%d: %v", i, err)
		}
		expected := fmt.Sprintf("v%d", i)
		if val != expected {
			t.Fatalf("restarted node key k%d: expected %q, got %q", i, expected, val)
		}
	}
}

// TestAllNodesConvergeAfterPartitionHeal verifies that after a network partition where
// an isolated leader is replaced and new entries are committed, healing the partition
// causes the old leader to step down, accept new entries, and achieve identical state.
func TestAllNodesConvergeAfterPartitionHeal(t *testing.T) {
	baseDir := t.TempDir()
	configs := node.ClusterConfig3Nodes(baseDir)

	mockNet := raft.NewMockNetwork()
	nodes := make([]*node.Node, 3)

	defer func() {
		for _, n := range nodes {
			if n != nil {
				_ = n.Stop()
			}
		}
	}()

	for i, cfg := range configs {
		tr := mockNet.CreateTransport(cfg.ID, cfg.RaftAddress)
		n, err := node.NewNode(cfg, tr)
		if err != nil {
			t.Fatalf("create node %d: %v", i, err)
		}
		if err := n.Start(); err != nil {
			t.Fatalf("start node %d: %v", i, err)
		}
		nodes[i] = n
	}

	// 1. Wait for leader
	var leader *node.Node
	deadline := time.Now().Add(5 * time.Second)
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
		t.Fatal("no leader elected initially")
	}

	// Propose initial committed entry
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	idx1, _, err := leader.Raft().Propose("SET", "base", "v1")
	if err != nil {
		t.Fatalf("propose base: %v", err)
	}
	if err := leader.Raft().WaitForApply(ctx, idx1); err != nil {
		t.Fatalf("wait for apply base: %v", err)
	}

	// 2. Partition the leader away from the other 2 nodes
	oldLeaderID := leader.ID()
	mockNet.Isolate(oldLeaderID)

	// 3. Wait for the remaining 2 nodes to elect a replacement leader
	var newLeader *node.Node
	electionDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(electionDeadline) {
		for _, n := range nodes {
			if n.ID() != oldLeaderID && n.Raft().Role() == raft.Leader {
				newLeader = n
				break
			}
		}
		if newLeader != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if newLeader == nil {
		t.Fatal("remaining nodes failed to elect new leader")
	}

	// 4. Propose new entries on new leader
	idx2, _, err := newLeader.Raft().Propose("SET", "cluster_state", "replicated_after_partition")
	if err != nil {
		t.Fatalf("propose on new leader: %v", err)
	}
	if err := newLeader.Raft().WaitForApply(ctx, idx2); err != nil {
		t.Fatalf("wait for apply on new leader: %v", err)
	}

	// 5. Heal partition
	mockNet.Rejoin(oldLeaderID)

	// 6. Wait for all 3 nodes to converge
	healDeadline := time.Now().Add(5 * time.Second)
	converged := false
	for time.Now().Before(healDeadline) {
		allMatch := true
		for _, n := range nodes {
			val, err := n.DB().Get("cluster_state")
			if err != nil || val != "replicated_after_partition" {
				allMatch = false
				break
			}
		}
		if allMatch {
			converged = true
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	if !converged {
		t.Fatal("nodes failed to converge to identical state after partition heal")
	}

	// Verify all 3 nodes report identical base and cluster_state values
	for _, n := range nodes {
		baseVal, err := n.DB().Get("base")
		if err != nil || baseVal != "v1" {
			t.Fatalf("node %s: base key expected 'v1', got %q (err: %v)", n.ID(), baseVal, err)
		}

		stateVal, err := n.DB().Get("cluster_state")
		if err != nil || stateVal != "replicated_after_partition" {
			t.Fatalf("node %s: cluster_state expected 'replicated_after_partition', got %q", n.ID(), stateVal)
		}
	}
}
