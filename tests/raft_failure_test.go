package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"orbit/internal/node"
	"orbit/internal/raft"
)

// TestPartitionLeaderCannotCommit verifies that an isolated leader cannot commit writes
// because it cannot obtain a majority quorum.
func TestPartitionLeaderCannotCommit(t *testing.T) {
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

	// 1. Elect initial leader
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

	// 2. Isolate the leader
	net.Isolate(leader.ID())

	// 3. Propose write on isolated leader
	idx, _, err := leader.Raft().Propose("SET", "partition_key", "partition_val")
	if err != nil {
		t.Fatalf("Propose on isolated leader: %v", err)
	}

	// Wait for commit should time out because isolated leader only has 1 match (self), needed 2
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err = leader.Raft().WaitForCommit(ctx, idx)
	if err == nil {
		t.Fatalf("expected commit to fail/timeout on isolated leader, but it succeeded")
	}

	// 4. Remaining 2 nodes must elect a new leader
	var newLeader *node.Node
	failoverDeadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(failoverDeadline) {
		for _, n := range nodes {
			if n.ID() != leader.ID() && n.Raft().Role() == raft.Leader {
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
		t.Fatal("remaining nodes failed to elect a new leader")
	}

	// 5. New leader can commit writes with the remaining follower (2 nodes = majority)
	newIdx, _, err := newLeader.Raft().Propose("SET", "new_leader_key", "quorum_val")
	if err != nil {
		t.Fatalf("Propose on new leader: %v", err)
	}
	commitCtx, cancelCommit := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelCommit()
	if err := newLeader.Raft().WaitForCommit(commitCtx, newIdx); err != nil {
		t.Fatalf("new leader failed to commit with quorum: %v", err)
	}

	// 6. Reconnect old leader
	net.Rejoin(leader.ID())
	time.Sleep(200 * time.Millisecond)

	// Old leader must have stepped down upon receiving heartbeat from new leader
	if leader.Raft().Role() != raft.Follower {
		t.Fatalf("old leader did not step down after rejoining, role=%v", leader.Raft().Role())
	}

	// Wait for old leader to catch up with the newly committed write
	catchupDeadline := time.Now().Add(6 * time.Second)
	caughtUp := false
	for time.Now().Before(catchupDeadline) {
		if v, ok := leader.Engine().Get("new_leader_key"); ok && v == "quorum_val" {
			caughtUp = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !caughtUp {
		t.Fatal("rejoined old leader failed to replicate and apply new_leader_key")
	}
}

// TestMinorityPartitionAllowsMajorityToContinue verifies that when a minority node is partitioned,
// the majority continues to serve and commit writes without interruption.
func TestMinorityPartitionAllowsMajorityToContinue(t *testing.T) {
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
	var follower *node.Node
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range nodes {
			if n.Raft().Role() == raft.Leader {
				leader = n
			} else {
				follower = n
			}
		}
		if leader != nil && follower != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if leader == nil || follower == nil {
		t.Fatal("failed to elect leader and follower")
	}

	// Partition the follower away
	net.Isolate(follower.ID())

	// Leader must still be able to commit with the other follower
	idx, _, err := leader.Raft().Propose("SET", "maj_key", "maj_val")
	if err != nil {
		t.Fatalf("Propose on leader with minority partition: %v", err)
	}

	commitCtx, cancelCommit := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelCommit()
	if err := leader.Raft().WaitForCommit(commitCtx, idx); err != nil {
		t.Fatalf("majority failed to commit: %v", err)
	}

	// Rejoin the partitioned follower
	net.Rejoin(follower.ID())

	// Follower must catch up
	catchupDeadline := time.Now().Add(6 * time.Second)
	caughtUp := false
	for time.Now().Before(catchupDeadline) {
		if v, ok := follower.Engine().Get("maj_key"); ok && v == "maj_val" {
			caughtUp = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !caughtUp {
		t.Fatal("rejoined follower failed to catch up with committed entries")
	}
}

// TestNodeCrashAndRestartInCluster verifies a node can crash, restart,
// recover its storage and Raft state, and rejoin the cluster cleanly.
func TestNodeCrashAndRestartInCluster(t *testing.T) {
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

	// Propose 3 entries
	for i := 1; i <= 3; i++ {
		idx, _, err := leader.Raft().Propose("SET", fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
		if err != nil {
			t.Fatalf("propose: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = leader.Raft().WaitForApply(ctx, idx)
		cancel()
	}

	time.Sleep(150 * time.Millisecond)

	// Pick a follower to crash
	var followerIdx int
	for i, n := range nodes {
		if n.ID() != leader.ID() {
			followerIdx = i
			break
		}
	}
	crashedCfg := configs[followerIdx]

	// Stop follower
	if err := nodes[followerIdx].Stop(); err != nil {
		t.Fatalf("stop follower: %v", err)
	}

	// Remaining nodes commit another write
	idx4, _, err := leader.Raft().Propose("SET", "k4", "v4")
	if err != nil {
		t.Fatalf("propose while one node down: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := leader.Raft().WaitForApply(ctx, idx4); err != nil {
		t.Fatalf("commit while one node down: %v", err)
	}
	cancel()

	// Restart the crashed node
	newTr := net.CreateTransport(crashedCfg.ID, crashedCfg.RaftAddress)
	restartedNode, err := node.NewNode(crashedCfg, newTr)
	if err != nil {
		t.Fatalf("restart node: %v", err)
	}
	nodes[followerIdx] = restartedNode

	if err := restartedNode.Start(); err != nil {
		t.Fatalf("start restarted node: %v", err)
	}
	defer func() {
		for _, n := range nodes {
			_ = n.Stop()
		}
	}()

	// Wait for restarted node to catch up with all entries including k4
	catchupDeadline := time.Now().Add(8 * time.Second)
	caughtUp := false
	for time.Now().Before(catchupDeadline) {
		if v, ok := restartedNode.Engine().Get("k4"); ok && v == "v4" {
			caughtUp = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !caughtUp {
		t.Fatal("restarted node failed to catch up to k4=v4")
	}

	// Verify all keys 1 through 4 are present on restarted node
	for i := 1; i <= 4; i++ {
		key := fmt.Sprintf("k%d", i)
		expected := fmt.Sprintf("v%d", i)
		if val, ok := restartedNode.Engine().Get(key); !ok || val != expected {
			t.Fatalf("restarted node missing %s: got %q, expected %q", key, val, expected)
		}
	}
}
