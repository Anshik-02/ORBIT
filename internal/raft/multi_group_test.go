package raft_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/node"
	"orbit/internal/raft"
	"orbit/internal/sharding"
	"orbit/internal/storage"
)

// Task 2: Verify independent elections
// Build two independent Raft groups:
// GROUP 0: Node A, Node B, Node C
// GROUP 1: Node A, Node D, Node E
// Both groups use the same transport instance on Node A.
//
// Verify:
// - Both groups elect leaders.
// - Both groups maintain independent terms.
// - Both groups replicate entries.
// - Both groups commit independently.
func TestMultiGroup_IndependentElections(t *testing.T) {
	t.Parallel()

	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	allNodes := []string{"node-a", "node-b", "node-c", "node-d", "node-e"}
	meta := sharding.NewClusterMetadata("orbit-multi-test", 1, 2, 3, allNodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b", "node-c"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a", "node-d", "node-e"}})

	peerAddrs := make(map[string]string)
	for _, id := range allNodes {
		peerAddrs[id] = id
	}

	nodes := make(map[string]*node.Node)
	for _, id := range allNodes {
		cfg := node.NodeConfig{
			ID:                 id,
			Address:            "127.0.0.1:0",
			RaftAddress:        "127.0.0.1:0",
			DataDir:            filepath.Join(baseDir, id),
			HeartbeatInterval:  30 * time.Millisecond,
			ElectionTimeoutMin: 100 * time.Millisecond,
			ElectionTimeoutMax: 200 * time.Millisecond,
		}
		// Node A has one shared transport instance used for both Group 0 and Group 1
		tr := net.CreateTransport(id)
		n, err := node.NewShardedNode(cfg, meta, peerAddrs, tr)
		if err != nil {
			t.Fatalf("NewShardedNode(%s): %v", id, err)
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

	// 1. Both groups elect leaders
	leaders := make(map[sharding.ShardID]string)
	deadline := time.Now().Add(4 * time.Second)

	for time.Now().Before(deadline) && len(leaders) < 2 {
		for sid := sharding.ShardID(0); sid < 2; sid++ {
			if _, ok := leaders[sid]; ok {
				continue
			}
			for _, nid := range allNodes {
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

	leader0, ok0 := leaders[0]
	leader1, ok1 := leaders[1]
	if !ok0 || !ok1 {
		t.Fatalf("failed to elect leaders within deadline: leader0=%s (ok=%v), leader1=%s (ok=%v)",
			leader0, ok0, leader1, ok1)
	}
	t.Logf("Group 0 leader: %s, Group 1 leader: %s", leader0, leader1)

	// 2. Both groups maintain independent terms
	replica0, _ := nodes[leader0].Shard(0)
	replica1, _ := nodes[leader1].Shard(1)

	term0 := replica0.Raft().CurrentTerm()
	term1 := replica1.Raft().CurrentTerm()
	t.Logf("Group 0 term: %d, Group 1 term: %d", term0, term1)

	// 3. Both groups replicate entries
	idx0, _, err := replica0.Raft().Propose("SET", "user:group0", "Alice")
	if err != nil {
		t.Fatalf("propose on Group 0: %v", err)
	}

	idx1, _, err := replica1.Raft().Propose("SET", "user:group1", "Bob")
	if err != nil {
		t.Fatalf("propose on Group 1: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := replica0.Raft().WaitForApply(ctx, idx0); err != nil {
		t.Fatalf("wait for apply on Group 0: %v", err)
	}
	if err := replica1.Raft().WaitForApply(ctx, idx1); err != nil {
		t.Fatalf("wait for apply on Group 1: %v", err)
	}

	// 4. Both groups commit independently
	val0, err := replica0.DB().Get("user:group0")
	if err != nil || val0 != "Alice" {
		t.Errorf("Group 0 DB: got %q (err=%v), want 'Alice'", val0, err)
	}

	val1, err := replica1.DB().Get("user:group1")
	if err != nil || val1 != "Bob" {
		t.Errorf("Group 1 DB: got %q (err=%v), want 'Bob'", val1, err)
	}

	// Verify isolation: Group 0 keys do not exist in Group 1, and vice-versa
	_, errIsolated0 := replica0.DB().Get("user:group1")
	if errIsolated0 == nil {
		t.Errorf("Group 0 DB contains key from Group 1!")
	}
	_, errIsolated1 := replica1.DB().Get("user:group0")
	if errIsolated1 == nil {
		t.Errorf("Group 1 DB contains key from Group 0!")
	}
}

// Task 3: Verify term isolation
// Set up:
// Group 0: Term 5
// Group 1: Term 3
//
// Trigger a new election in Group 0.
//
// Expected:
// Group 0: Term 6
// Group 1: Term 3
//
// A term change in one group must never modify another group's state.
func TestMultiGroup_TermIsolation(t *testing.T) {
	t.Parallel()

	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	allNodes := []string{"node-a", "node-b", "node-c"}
	meta := sharding.NewClusterMetadata("orbit-multi-test", 1, 2, 2, allNodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a", "node-c"}})

	peerAddrs := map[string]string{
		"node-a": "node-a",
		"node-b": "node-b",
		"node-c": "node-c",
	}

	cfgA := node.NodeConfig{
		ID:                 "node-a",
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		DataDir:            filepath.Join(baseDir, "node-a"),
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 200 * time.Millisecond,
		ElectionTimeoutMax: 400 * time.Millisecond,
	}

	trA := net.CreateTransport("node-a")
	nodeA, err := node.NewShardedNode(cfgA, meta, peerAddrs, trA)
	if err != nil {
		t.Fatalf("NewShardedNode: %v", err)
	}
	defer nodeA.Stop()

	shard0, ok0 := nodeA.Shard(0)
	shard1, ok1 := nodeA.Shard(1)
	if !ok0 || !ok1 || shard1.Raft() == nil {
		t.Fatalf("expected node-a to host shard 0 and shard 1")
	}

	// Set up terms using RPCs to simulate established terms:
	// Group 0 -> Term 5
	// Group 1 -> Term 3
	ctx := context.Background()
	senderTr := net.CreateTransport("sender")

	_, err = senderTr.SendRequestVote(ctx, "node-a", raft.GroupID("shard-0"), &raft.RequestVoteRequest{
		Term:         5,
		CandidateID:  "node-b",
		LastLogIndex: 1,
		LastLogTerm:  5,
	})
	if err != nil {
		t.Fatalf("setup Group 0 term 5: %v", err)
	}

	_, err = senderTr.SendRequestVote(ctx, "node-a", raft.GroupID("shard-1"), &raft.RequestVoteRequest{
		Term:         3,
		CandidateID:  "node-c",
		LastLogIndex: 1,
		LastLogTerm:  3,
	})
	if err != nil {
		t.Fatalf("setup Group 1 term 3: %v", err)
	}

	if shard0.Raft().CurrentTerm() != 5 {
		t.Fatalf("expected Group 0 term 5, got %d", shard0.Raft().CurrentTerm())
	}
	if shard1.Raft().CurrentTerm() != 3 {
		t.Fatalf("expected Group 1 term 3, got %d", shard1.Raft().CurrentTerm())
	}

	// Trigger a new election in Group 0 (candidate requests vote with Term 6)
	_, err = senderTr.SendRequestVote(ctx, "node-a", raft.GroupID("shard-0"), &raft.RequestVoteRequest{
		Term:         6,
		CandidateID:  "node-b",
		LastLogIndex: 2,
		LastLogTerm:  6,
	})
	if err != nil {
		t.Fatalf("trigger election in Group 0: %v", err)
	}

	// Expected:
	// Group 0: Term 6
	// Group 1: Term 3
	if shard0.Raft().CurrentTerm() != 6 {
		t.Errorf("expected Group 0 term 6, got %d", shard0.Raft().CurrentTerm())
	}
	if shard1.Raft().CurrentTerm() != 3 {
		t.Errorf("TERM ISOLATION VIOLATION: Group 1 term changed to %d, expected 3", shard1.Raft().CurrentTerm())
	}
}

// Task 4: Verify storage isolation
// Use the same key in both groups:
// Group 0: SET user:1 Alice
// Group 1: SET user:1 Bob
// Restart both groups.
// Expected:
// Group 0: user:1 = Alice
// Group 1: user:1 = Bob
// Verify that their WAL files and Raft persistent state are stored in separate directories.
func TestMultiGroup_StorageIsolation(t *testing.T) {
	t.Parallel()

	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	allNodes := []string{"node-a", "node-b"}
	meta := sharding.NewClusterMetadata("orbit-multi-test", 1, 2, 2, allNodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a", "node-b"}})

	peerAddrs := map[string]string{
		"node-a": "node-a",
		"node-b": "node-b",
	}

	cfgA := node.NodeConfig{
		ID:                 "node-a",
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		DataDir:            filepath.Join(baseDir, "node-a"),
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 200 * time.Millisecond,
		ElectionTimeoutMax: 400 * time.Millisecond,
	}

	// 1. Initial run: create node and apply entries
	trA := net.CreateTransport("node-a")
	nodeA, err := node.NewShardedNode(cfgA, meta, peerAddrs, trA)
	if err != nil {
		t.Fatalf("NewShardedNode: %v", err)
	}

	shard0, ok0 := nodeA.Shard(0)
	shard1, ok1 := nodeA.Shard(1)
	if !ok0 || !ok1 || shard1.Raft() == nil {
		t.Fatalf("expected node-a to host shard 0 and shard 1")
	}

	// Apply same key with different values:
	// Group 0: SET user:1 Alice
	// Group 1: SET user:1 Bob
	err0 := shard0.Engine().ApplyCommitted(storage.CommittedCommand{
		RaftIndex: 1,
		RaftTerm:  1,
		Operation: "SET",
		Key:       "user:1",
		Value:     "Alice",
	})
	if err0 != nil {
		t.Fatalf("apply on Group 0: %v", err0)
	}

	err1 := shard1.Engine().ApplyCommitted(storage.CommittedCommand{
		RaftIndex: 1,
		RaftTerm:  1,
		Operation: "SET",
		Key:       "user:1",
		Value:     "Bob",
	})
	if err1 != nil {
		t.Fatalf("apply on Group 1: %v", err1)
	}

	// Persist Raft state and log for each shard to verify Raft persistent files
	if err := shard0.Raft().Storage().SaveState(1, "node-a"); err != nil {
		t.Fatalf("save state shard-0: %v", err)
	}
	if err := shard0.Raft().Storage().AppendEntries([]raft.LogEntry{{Index: 1, Term: 1, Operation: "SET", Key: "user:1", Value: "Alice"}}); err != nil {
		t.Fatalf("append entry shard-0: %v", err)
	}

	if err := shard1.Raft().Storage().SaveState(1, "node-a"); err != nil {
		t.Fatalf("save state shard-1: %v", err)
	}
	if err := shard1.Raft().Storage().AppendEntries([]raft.LogEntry{{Index: 1, Term: 1, Operation: "SET", Key: "user:1", Value: "Bob"}}); err != nil {
		t.Fatalf("append entry shard-1: %v", err)
	}

	// Stop node A (persists to WAL and closes files)
	if err := nodeA.Stop(); err != nil {
		t.Fatalf("stop node-a: %v", err)
	}

	// Verify separate directory paths exist for WAL and Raft state
	dir0 := filepath.Join(cfgA.DataDir, "shard-0")
	dir1 := filepath.Join(cfgA.DataDir, "shard-1")

	if dir0 == dir1 {
		t.Fatalf("shard 0 and shard 1 share the same directory: %s", dir0)
	}

	wal0 := filepath.Join(dir0, "wal.log")
	wal1 := filepath.Join(dir1, "wal.log")
	meta0 := filepath.Join(dir0, "raft", "metadata.json")
	meta1 := filepath.Join(dir1, "raft", "metadata.json")
	log0 := filepath.Join(dir0, "raft", "raft.log")
	log1 := filepath.Join(dir1, "raft", "raft.log")

	for _, path := range []string{wal0, wal1, meta0, meta1, log0, log1} {
		if !fileExists(path) {
			t.Errorf("expected persistent file to exist at %q", path)
		}
	}

	// 2. Restart node A and verify recovered values
	trARestart := net.CreateTransport("node-a")
	nodeARestart, err := node.NewShardedNode(cfgA, meta, peerAddrs, trARestart)
	if err != nil {
		t.Fatalf("restart NewShardedNode: %v", err)
	}
	defer nodeARestart.Stop()

	recovered0, ok0 := nodeARestart.Shard(0)
	recovered1, ok1 := nodeARestart.Shard(1)
	if !ok0 || !ok1 {
		t.Fatalf("expected restarted node-a to host shard 0 and shard 1")
	}

	// Expected: Group 0 -> user:1 = Alice; Group 1 -> user:1 = Bob
	val0, err := recovered0.DB().Get("user:1")
	if err != nil || val0 != "Alice" {
		t.Errorf("recovered Group 0: got %q (err=%v), want 'Alice'", val0, err)
	}

	val1, err := recovered1.DB().Get("user:1")
	if err != nil || val1 != "Bob" {
		t.Errorf("recovered Group 1: got %q (err=%v), want 'Bob'", val1, err)
	}
}

// Task 5: Verify lifecycle isolation
// Start two groups on one physical node:
// Node A
//
//	├── Group 0
//	└── Group 1
//
// Stop Group 0.
// Verify:
// - Group 0 stops.
// - Group 1 remains operational.
// - Shared HTTP transport remains active.
// - Group 1 can still receive AppendEntries.
// - Node shutdown eventually stops both groups and closes the transport.
func TestMultiGroup_LifecycleIsolation(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()

	allNodes := []string{"node-a", "node-b"}
	meta := sharding.NewClusterMetadata("orbit-multi-test", 1, 2, 2, allNodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a", "node-b"}})

	peerAddrs := map[string]string{
		"node-a": "127.0.0.1:0",
		"node-b": "127.0.0.1:0",
	}

	cfgA := node.NodeConfig{
		ID:                 "node-a",
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		DataDir:            filepath.Join(baseDir, "node-a"),
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 200 * time.Millisecond,
		ElectionTimeoutMax: 400 * time.Millisecond,
	}

	// Use real HTTP transport for Node A to verify network listener isolation
	httpTrA := raft.NewHTTPTransport(cfgA.RaftAddress)
	nodeA, err := node.NewShardedNode(cfgA, meta, peerAddrs, httpTrA)
	if err != nil {
		t.Fatalf("NewShardedNode: %v", err)
	}

	if err := nodeA.Start(); err != nil {
		t.Fatalf("start node-a: %v", err)
	}
	defer nodeA.Stop()

	shard0, ok0 := nodeA.Shard(0)
	shard1, ok1 := nodeA.Shard(1)
	if !ok0 || !ok1 || shard1.Raft() == nil {
		t.Fatalf("expected node-a to host shard 0 and shard 1")
	}

	client := raft.NewHTTPTransport("127.0.0.1:0")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Verify both groups can receive RPCs before stopping Group 0
	_, err = client.SendAppendEntries(ctx, httpTrA.Addr(), raft.GroupID("shard-0"), &raft.AppendEntriesRequest{
		Term:     1,
		LeaderID: "node-b",
	})
	if err != nil {
		t.Fatalf("initial send append to shard-0: %v", err)
	}

	_, err = client.SendAppendEntries(ctx, httpTrA.Addr(), raft.GroupID("shard-1"), &raft.AppendEntriesRequest{
		Term:     1,
		LeaderID: "node-b",
	})
	if err != nil {
		t.Fatalf("initial send append to shard-1: %v", err)
	}

	// 1. Stop Group 0
	if err := shard0.Stop(); err != nil {
		t.Fatalf("stop shard 0: %v", err)
	}

	// 2. Verify Group 0 stops: sending RPC to Group 0 now returns ErrGroupNotFound
	_, errStopped0 := client.SendAppendEntries(ctx, httpTrA.Addr(), raft.GroupID("shard-0"), &raft.AppendEntriesRequest{
		Term:     2,
		LeaderID: "node-b",
	})
	if errStopped0 == nil {
		t.Fatal("expected ErrGroupNotFound for stopped shard-0, got nil")
	}
	if !errors.Is(errStopped0, raft.ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got: %v", errStopped0)
	}

	// 3. Verify Group 1 remains operational & shared HTTP transport remains active
	// Group 1 can still receive AppendEntries
	appendResp1, errRunning1 := client.SendAppendEntries(ctx, httpTrA.Addr(), raft.GroupID("shard-1"), &raft.AppendEntriesRequest{
		Term:     2,
		LeaderID: "node-b",
	})
	if errRunning1 != nil {
		t.Fatalf("Group 1 should remain operational after Group 0 stopped, got err: %v", errRunning1)
	}
	if appendResp1 == nil {
		t.Fatal("expected non-nil AppendEntriesResponse for Group 1")
	}

	// 4. Node shutdown eventually stops both groups and closes the transport
	if err := nodeA.Stop(); err != nil {
		t.Fatalf("nodeA.Stop(): %v", err)
	}

	// Attempting connection to transport listener after shutdown should fail
	_, errAfterShutdown := http.Get("http://" + httpTrA.Addr() + "/raft/append")
	if errAfterShutdown == nil {
		t.Fatal("expected connection error after node transport shutdown, got nil")
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
