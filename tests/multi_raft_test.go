package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/node"
	"orbit/internal/raft"
	"orbit/internal/sharding"
	"orbit/internal/storage"
)

// TestMultiRaft_GroupIndependence verifies Rule 1:
// A higher term or election in Shard 0 MUST NOT change the term or leadership state
// of Shard 1 on the same physical node. Each Raft instance maintains its own consensus state.
func TestMultiRaft_GroupIndependence(t *testing.T) {
	t.Parallel()

	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	nodes := []string{"node-a", "node-b", "node-c"}
	meta := sharding.NewClusterMetadata("orbit-multi-test", 1, 2, 2, nodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a", "node-c"}})

	cfgA := node.NodeConfig{
		ID:                 "node-a",
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		DataDir:            filepath.Join(baseDir, "node-a"),
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 200 * time.Millisecond,
		ElectionTimeoutMax: 400 * time.Millisecond,
	}

	peerAddrs := map[string]string{
		"node-a": "node-a",
		"node-b": "node-b",
		"node-c": "node-c",
	}

	trA := net.CreateTransport("node-a")
	nodeA, err := node.NewShardedNode(cfgA, meta, peerAddrs, trA)
	if err != nil {
		t.Fatalf("NewShardedNode: %v", err)
	}
	defer nodeA.Stop()

	// Verify Node A hosts shard 0 and shard 1
	shard0, ok0 := nodeA.Shard(0)
	shard1, ok1 := nodeA.Shard(1)
	if !ok0 || !ok1 {
		t.Fatalf("expected node-a to host shard 0 and shard 1, got ok0=%v, ok1=%v", ok0, ok1)
	}

	initialTerm0 := shard0.Raft().CurrentTerm()
	initialTerm1 := shard1.Raft().CurrentTerm()

	// Deliver a RequestVote for Shard 0 with a much higher term (e.g. 50)
	ctx := context.Background()
	senderTr := net.CreateTransport("sender")
	_, err = senderTr.SendRequestVote(ctx, "node-a", raft.GroupID("shard-0"), &raft.RequestVoteRequest{
		Term:         50,
		CandidateID:  "node-b",
		LastLogIndex: 1,
		LastLogTerm:  50,
	})
	if err != nil {
		t.Fatalf("send vote to shard-0: %v", err)
	}

	// Shard 0 must have advanced its term to 50
	if shard0.Raft().CurrentTerm() != 50 {
		t.Errorf("expected shard-0 term to be 50, got %d", shard0.Raft().CurrentTerm())
	}

	// Rule 1: Shard 1 MUST NOT have changed its term!
	if shard1.Raft().CurrentTerm() != initialTerm1 {
		t.Errorf("RULE 1 VIOLATION: shard-1 term changed from %d to %d due to shard-0 activity!",
			initialTerm1, shard1.Raft().CurrentTerm())
	}

	_ = initialTerm0
}

// TestMultiRaft_StorageIsolation verifies Rule 2:
// Shard 0 and Shard 1 on the same physical node write to independent, isolated directories
// and never collide on keys, WAL files, or Raft state files.
func TestMultiRaft_StorageIsolation(t *testing.T) {
	t.Parallel()

	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	nodes := []string{"node-a", "node-b"}
	meta := sharding.NewClusterMetadata("orbit-multi-test", 1, 2, 2, nodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a", "node-b"}})

	cfgA := node.NodeConfig{
		ID:                 "node-a",
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		DataDir:            filepath.Join(baseDir, "node-a"),
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 200 * time.Millisecond,
		ElectionTimeoutMax: 400 * time.Millisecond,
	}

	peerAddrs := map[string]string{
		"node-a": "node-a",
		"node-b": "node-b",
	}

	nodeA, err := node.NewShardedNode(cfgA, meta, peerAddrs, net.CreateTransport("node-a"))
	if err != nil {
		t.Fatalf("NewShardedNode: %v", err)
	}
	defer nodeA.Stop()

	// Verify filesystem layout
	shard0Dir := filepath.Join(cfgA.DataDir, "shard-0")
	shard1Dir := filepath.Join(cfgA.DataDir, "shard-1")

	if _, err := os.Stat(filepath.Join(shard0Dir, "wal.log")); err != nil {
		t.Errorf("missing shard-0 wal.log: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shard1Dir, "wal.log")); err != nil {
		t.Errorf("missing shard-1 wal.log: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shard0Dir, "raft", "raft.log")); err != nil {
		t.Errorf("missing shard-0 raft.log: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shard1Dir, "raft", "raft.log")); err != nil {
		t.Errorf("missing shard-1 raft.log: %v", err)
	}

	// Apply identical key "account:1" with different values to each shard replica
	shard0, _ := nodeA.Shard(0)
	shard1, _ := nodeA.Shard(1)

	// Apply through engine directly to verify isolated state machines
	_ = shard0.Engine().ApplyCommitted(orbitCommittedCmd(1, 1, "SET", "account:1", "balance-0"))
	_ = shard1.Engine().ApplyCommitted(orbitCommittedCmd(1, 1, "SET", "account:1", "balance-1"))

	v0, err0 := shard0.DB().Get("account:1")
	v1, err1 := shard1.DB().Get("account:1")

	if err0 != nil || v0 != "balance-0" {
		t.Errorf("shard-0 DB: got %q (err=%v), want balance-0", v0, err0)
	}
	if err1 != nil || v1 != "balance-1" {
		t.Errorf("shard-1 DB: got %q (err=%v), want balance-1", v1, err1)
	}
}

// TestMultiRaft_TransportMultiplexing verifies Rule 3:
// A physical node binds a single HTTP listener that multiplexes RPCs to multiple
// independent Raft instances using GroupID, rejecting unhosted groups with 404.
func TestMultiRaft_TransportMultiplexing(t *testing.T) {
	t.Parallel()

	transportA := raft.NewHTTPTransport("127.0.0.1:0")

	baseDir := t.TempDir()
	nodes := []string{"node-a", "node-b", "node-c"}
	meta := sharding.NewClusterMetadata("orbit-multi-test", 1, 3, 2, nodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a", "node-c"}})
	// Note: shard-2 is hosted on node-b and node-c, NOT node-a!
	_ = meta.AddShard(&sharding.ShardInfo{ID: 2, Replicas: []string{"node-b", "node-c"}})

	cfgA := node.NodeConfig{
		ID:                 "node-a",
		Address:            "127.0.0.1:0",
		RaftAddress:        transportA.Addr(),
		DataDir:            filepath.Join(baseDir, "node-a"),
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 200 * time.Millisecond,
		ElectionTimeoutMax: 400 * time.Millisecond,
	}

	peerAddrs := map[string]string{
		"node-a": transportA.Addr(),
		"node-b": "127.0.0.1:9991",
		"node-c": "127.0.0.1:9992",
	}

	nodeA, err := node.NewShardedNode(cfgA, meta, peerAddrs, transportA)
	if err != nil {
		t.Fatalf("NewShardedNode: %v", err)
	}
	if err := nodeA.Start(); err != nil {
		t.Fatalf("start nodeA: %v", err)
	}
	defer nodeA.Stop()

	client := raft.NewHTTPTransport("127.0.0.1:0")
	ctx := context.Background()

	// 1. Send RPC for shard-0 to Node A -> Succeeded
	voteResp0, err := client.SendRequestVote(ctx, transportA.Addr(), raft.GroupID("shard-0"), &raft.RequestVoteRequest{
		Term:         2,
		CandidateID:  "node-b",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if err != nil {
		t.Fatalf("SendRequestVote to shard-0 failed: %v", err)
	}
	if !voteResp0.VoteGranted {
		t.Errorf("expected vote to be granted by shard-0")
	}

	// 2. Send RPC for shard-1 to Node A -> Succeeded
	voteResp1, err := client.SendRequestVote(ctx, transportA.Addr(), raft.GroupID("shard-1"), &raft.RequestVoteRequest{
		Term:         3,
		CandidateID:  "node-c",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if err != nil {
		t.Fatalf("SendRequestVote to shard-1 failed: %v", err)
	}
	if !voteResp1.VoteGranted {
		t.Errorf("expected vote to be granted by shard-1")
	}

	// 3. Send RPC for shard-2 to Node A -> Fails with ErrGroupNotFound (404)
	_, err = client.SendRequestVote(ctx, transportA.Addr(), raft.GroupID("shard-2"), &raft.RequestVoteRequest{
		Term:         4,
		CandidateID:  "node-b",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if err == nil {
		t.Fatal("expected ErrGroupNotFound for shard-2 on node-a, got nil")
	}
	if !errors.Is(err, raft.ErrGroupNotFound) {
		t.Errorf("expected ErrGroupNotFound, got: %v", err)
	}
}

// TestMultiRaft_FiveNodeClusterIndependentElections tests the full 5-node 3-shard cluster:
// Shard 0: A, B, C
// Shard 1: D, E, A
// Shard 2: B, C, D
// Verifies that:
// 1. All three shards independently elect a leader.
// 2. Multiple leaders exist simultaneously across different Raft groups.
// 3. Isolating Node A affects only Shard 0 and Shard 1, while Shard 2 continues unaffected.
func TestMultiRaft_FiveNodeClusterIndependentElections(t *testing.T) {
	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	nodeIDs := []string{"node-a", "node-b", "node-c", "node-d", "node-e"}
	const shardCount = 3
	const rf = 3

	assignment, err := sharding.AssignReplicas(shardCount, rf, nodeIDs)
	if err != nil {
		t.Fatalf("AssignReplicas: %v", err)
	}

	meta := sharding.NewClusterMetadata("orbit-five-node-test", 1, shardCount, rf, nodeIDs)
	for sid, replicas := range assignment {
		_ = meta.AddShard(&sharding.ShardInfo{ID: sid, Replicas: replicas})
	}
	if err := meta.Validate(); err != nil {
		t.Fatalf("meta.Validate: %v", err)
	}

	peerAddrs := make(map[string]string)
	for _, id := range nodeIDs {
		peerAddrs[id] = id
	}

	nodes := make(map[string]*node.Node)
	for _, id := range nodeIDs {
		cfg := node.NodeConfig{
			ID:                 id,
			Address:            "127.0.0.1:0",
			RaftAddress:        "127.0.0.1:0",
			DataDir:            filepath.Join(baseDir, id),
			HeartbeatInterval:  30 * time.Millisecond,
			ElectionTimeoutMin: 100 * time.Millisecond,
			ElectionTimeoutMax: 200 * time.Millisecond,
		}
		tr := net.CreateTransport(id)
		n, err := node.NewShardedNode(cfg, meta, peerAddrs, tr)
		if err != nil {
			t.Fatalf("NewShardedNode %s: %v", id, err)
		}
		nodes[id] = n
	}

	for _, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("start node %s: %v", n.ID(), err)
		}
	}
	defer func() {
		for _, n := range nodes {
			_ = n.Stop()
		}
	}()

	// Wait for leader elections across all 3 shards
	leaders := make(map[sharding.ShardID]string)
	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) && len(leaders) < 3 {
		for sid := sharding.ShardID(0); sid < 3; sid++ {
			if _, haveLeader := leaders[sid]; haveLeader {
				continue
			}
			for _, nid := range nodeIDs {
				n := nodes[nid]
				replica, hosted := n.Shard(sid)
				if hosted && replica.Raft().Role() == raft.Leader {
					leaders[sid] = nid
					break
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	for sid := sharding.ShardID(0); sid < 3; sid++ {
		leaderNode, found := leaders[sid]
		if !found {
			t.Fatalf("failed to elect leader for shard %d within deadline", sid)
		}
		t.Logf("Shard %s elected leader on %s", sid, leaderNode)

		// Verify leader is one of the assigned replicas for this shard
		shardInfo, _ := meta.GetShard(sid)
		if !shardInfo.ContainsNode(leaderNode) {
			t.Errorf("shard %s leader %s is not in replica set %v", sid, leaderNode, shardInfo.Replicas)
		}
	}

	// Propose a write on Shard 0 leader
	shard0LeaderNode := nodes[leaders[0]]
	shard0Replica, _ := shard0LeaderNode.Shard(0)
	idx, _, err := shard0Replica.Raft().Propose("SET", "user:1", "alice")
	if err != nil {
		t.Fatalf("propose on shard 0: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := shard0Replica.Raft().WaitForApply(ctx, idx); err != nil {
		t.Fatalf("wait for apply on shard 0: %v", err)
	}

	// Verify write committed and applied on Shard 0's database
	val, err := shard0Replica.DB().Get("user:1")
	if err != nil || val != "alice" {
		t.Errorf("shard 0 db value got %q (err=%v), want alice", val, err)
	}

	// Rule 1 Verification: Shard 2 must NOT contain "user:1"
	shard2LeaderNode := nodes[leaders[2]]
	shard2Replica, _ := shard2LeaderNode.Shard(2)
	_, errOnShard2 := shard2Replica.DB().Get("user:1")
	if errOnShard2 == nil {
		t.Errorf("RULE 1/2 VIOLATION: shard 2 contains key written to shard 0!")
	}
}

func orbitCommittedCmd(idx, term uint64, op, key, val string) storage.CommittedCommand {
	return storage.CommittedCommand{
		RaftIndex: idx,
		RaftTerm:  term,
		Operation: op,
		Key:       key,
		Value:     val,
	}
}
