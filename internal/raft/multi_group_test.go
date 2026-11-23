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
)

// Task 2: Strengthen independent election tests
// Create two groups:
// GROUP 0: Node A, Node B, Node C
// GROUP 1: Node A, Node D, Node E
// Both groups use the same transport instance on Node A.
//
// Verify:
// 1. Both groups elect leaders.
// 2. Both groups can have different leaders.
// 3. Both groups maintain independent terms.
// 4. Group 0 can commit entries while Group 1 continues operating.
// 5. A term increase in Group 0 does not change Group 1's term.
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
		minTimeout := 250 * time.Millisecond
		maxTimeout := 350 * time.Millisecond
		if id == "node-b" || id == "node-d" {
			minTimeout = 40 * time.Millisecond
			maxTimeout = 80 * time.Millisecond
		}
		cfg := node.NodeConfig{
			ID:                 id,
			Address:            "127.0.0.1:0",
			RaftAddress:        "127.0.0.1:0",
			DataDir:            filepath.Join(baseDir, id),
			HeartbeatInterval:  20 * time.Millisecond,
			ElectionTimeoutMin: minTimeout,
			ElectionTimeoutMax: maxTimeout,
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

	// 2. Both groups can have different leaders
	if leader0 == leader1 {
		t.Fatalf("expected different leaders for Group 0 and Group 1, got same leader %s", leader0)
	}
	t.Logf("Group 0 leader: %s, Group 1 leader: %s", leader0, leader1)

	replica0, _ := nodes[leader0].Shard(0)
	replica1, _ := nodes[leader1].Shard(1)

	// 3. Both groups maintain independent terms
	term0 := replica0.Raft().CurrentTerm()
	term1 := replica1.Raft().CurrentTerm()
	t.Logf("Group 0 term: %d, Group 1 term: %d", term0, term1)

	// 4. Group 0 can commit entries while Group 1 continues operating
	idx0, _, err := replica0.Raft().Propose("SET", "user:group0", "Alice")
	if err != nil {
		t.Fatalf("propose on Group 0: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := replica0.Raft().WaitForApply(ctx, idx0); err != nil {
		t.Fatalf("wait for apply on Group 0: %v", err)
	}

	val0, err := replica0.DB().Get("user:group0")
	if err != nil || val0 != "Alice" {
		t.Errorf("Group 0 DB: got %q (err=%v), want 'Alice'", val0, err)
	}

	// Verify Group 1 continues operating by committing its own entry
	idx1, _, err := replica1.Raft().Propose("SET", "user:group1", "Bob")
	if err != nil {
		t.Fatalf("propose on Group 1: %v", err)
	}
	if err := replica1.Raft().WaitForApply(ctx, idx1); err != nil {
		t.Fatalf("wait for apply on Group 1: %v", err)
	}
	val1, err := replica1.DB().Get("user:group1")
	if err != nil || val1 != "Bob" {
		t.Errorf("Group 1 DB: got %q (err=%v), want 'Bob'", val1, err)
	}

	// 5. A term increase in Group 0 does not change Group 1's term
	term1Before := replica1.Raft().CurrentTerm()

	// Trigger a term increase in Group 0 by sending a higher term RequestVote to Group 0 on Node A
	senderTr := net.CreateTransport("test-sender")
	higherTerm := replica0.Raft().CurrentTerm() + 5
	_, err = senderTr.SendRequestVote(ctx, "node-a", raft.GroupID("shard-0"), &raft.RequestVoteRequest{
		Term:         higherTerm,
		CandidateID:  "node-b",
		LastLogIndex: 10,
		LastLogTerm:  higherTerm,
	})
	if err != nil {
		t.Fatalf("send higher term vote to Group 0: %v", err)
	}

	nodeAReplica0, _ := nodes["node-a"].Shard(0)
	nodeAReplica1, _ := nodes["node-a"].Shard(1)

	if nodeAReplica0.Raft().CurrentTerm() != higherTerm {
		t.Fatalf("expected Node A Group 0 term to be %d, got %d", higherTerm, nodeAReplica0.Raft().CurrentTerm())
	}

	// Verify Group 1 term was NOT modified on Node A or on Group 1's leader
	if nodeAReplica1.Raft().CurrentTerm() != term1Before {
		t.Errorf("TERM LEAKAGE: Node A Group 1 term changed to %d, expected %d",
			nodeAReplica1.Raft().CurrentTerm(), term1Before)
	}
	if replica1.Raft().CurrentTerm() != term1Before {
		t.Errorf("TERM LEAKAGE: Group 1 leader term changed to %d, expected %d",
			replica1.Raft().CurrentTerm(), term1Before)
	}
}

// Task 3: Test actual storage recovery
// 1. Start a node hosting two shards.
// 2. Write user:1 = Alice into Group 0.
// 3. Write user:1 = Bob into Group 1.
// 4. Persist the data through the normal Raft commit path.
// 5. Stop the node.
// 6. Recreate the node using the same data directory.
// 7. Verify both values are recovered correctly.
// 8. Verify Raft terms and log entries are recovered independently.
func TestMultiGroup_ActualStorageRecovery(t *testing.T) {
	t.Parallel()

	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	allNodes := []string{"node-a"}
	meta := sharding.NewClusterMetadata("orbit-multi-test", 1, 2, 1, allNodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a"}})

	peerAddrs := map[string]string{
		"node-a": "node-a",
	}

	cfgA := node.NodeConfig{
		ID:                 "node-a",
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		DataDir:            filepath.Join(baseDir, "node-a"),
		HeartbeatInterval:  30 * time.Millisecond,
		ElectionTimeoutMin: 100 * time.Millisecond,
		ElectionTimeoutMax: 200 * time.Millisecond,
	}

	// 1. Start a node hosting two shards
	trA := net.CreateTransport("node-a")
	nodeA, err := node.NewShardedNode(cfgA, meta, peerAddrs, trA)
	if err != nil {
		t.Fatalf("NewShardedNode: %v", err)
	}

	if err := nodeA.Start(); err != nil {
		t.Fatalf("start nodeA: %v", err)
	}

	shard0, ok0 := nodeA.Shard(0)
	shard1, ok1 := nodeA.Shard(1)
	if !ok0 || !ok1 {
		t.Fatalf("expected node-a to host shard 0 and shard 1")
	}

	// Wait for leadership in both 1-node groups
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if shard0.Raft().Role() == raft.Leader && shard1.Raft().Role() == raft.Leader {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if shard0.Raft().Role() != raft.Leader || shard1.Raft().Role() != raft.Leader {
		t.Fatalf("expected leaders for shard 0 and 1, got r0=%v, r1=%v", shard0.Raft().Role(), shard1.Raft().Role())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 2. Write user:1 = Alice into Group 0 through normal Raft commit path
	idx0, term0, err := shard0.Raft().Propose("SET", "user:1", "Alice")
	if err != nil {
		t.Fatalf("propose on Group 0: %v", err)
	}
	if err := shard0.Raft().WaitForApply(ctx, idx0); err != nil {
		t.Fatalf("wait for apply Group 0: %v", err)
	}

	// 3. Write user:1 = Bob into Group 1 through normal Raft commit path
	idx1, term1, err := shard1.Raft().Propose("SET", "user:1", "Bob")
	if err != nil {
		t.Fatalf("propose on Group 1: %v", err)
	}
	if err := shard1.Raft().WaitForApply(ctx, idx1); err != nil {
		t.Fatalf("wait for apply Group 1: %v", err)
	}

	// Verify before restart
	v0Pre, _ := shard0.DB().Get("user:1")
	v1Pre, _ := shard1.DB().Get("user:1")
	if v0Pre != "Alice" || v1Pre != "Bob" {
		t.Fatalf("pre-restart mismatch: v0=%s, v1=%s", v0Pre, v1Pre)
	}

	// 5. Stop the node (persists to WAL, raft.log, metadata.json, and closes files)
	if err := nodeA.Stop(); err != nil {
		t.Fatalf("stop node-a: %v", err)
	}

	// Verify file system isolation
	dir0 := filepath.Join(cfgA.DataDir, "shard-0")
	dir1 := filepath.Join(cfgA.DataDir, "shard-1")

	wal0 := filepath.Join(dir0, "wal.log")
	wal1 := filepath.Join(dir1, "wal.log")
	meta0 := filepath.Join(dir0, "raft", "metadata.json")
	meta1 := filepath.Join(dir1, "raft", "metadata.json")
	log0 := filepath.Join(dir0, "raft", "raft.log")
	log1 := filepath.Join(dir1, "raft", "raft.log")

	for _, p := range []string{wal0, wal1, meta0, meta1, log0, log1} {
		if !fileExists(p) {
			t.Errorf("expected persistent file %q to exist on disk", p)
		}
	}

	// 6. Recreate the node using the same data directory
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

	// 7. Verify both values are recovered correctly from disk
	val0, err := recovered0.DB().Get("user:1")
	if err != nil || val0 != "Alice" {
		t.Errorf("recovered Group 0 DB: got %q (err=%v), want 'Alice'", val0, err)
	}

	val1, err := recovered1.DB().Get("user:1")
	if err != nil || val1 != "Bob" {
		t.Errorf("recovered Group 1 DB: got %q (err=%v), want 'Bob'", val1, err)
	}

	// 8. Verify Raft terms and log entries are recovered independently
	recoveredTerm0, _, errState0 := recovered0.Raft().Storage().GetState()
	if errState0 != nil || recoveredTerm0 != term0 {
		t.Errorf("recovered Group 0 term: got %d (err=%v), want %d", recoveredTerm0, errState0, term0)
	}

	recoveredTerm1, _, errState1 := recovered1.Raft().Storage().GetState()
	if errState1 != nil || recoveredTerm1 != term1 {
		t.Errorf("recovered Group 1 term: got %d (err=%v), want %d", recoveredTerm1, errState1, term1)
	}

	entry0, ok0 := recovered0.Raft().Storage().GetEntry(idx0)
	if !ok0 || entry0.Key != "user:1" || entry0.Value != "Alice" {
		t.Errorf("recovered Group 0 log entry: ok=%v, got %+v, want Key=user:1, Value=Alice", ok0, entry0)
	}

	entry1, ok1 := recovered1.Raft().Storage().GetEntry(idx1)
	if !ok1 || entry1.Key != "user:1" || entry1.Value != "Bob" {
		t.Errorf("recovered Group 1 log entry: ok=%v, got %+v, want Key=user:1, Value=Bob", ok1, entry1)
	}
}

// Task 4: Test group failure isolation
// Stop Group 0 while Group 1 is running.
// Verify:
// - Group 0 rejects new RPCs.
// - Group 1 continues receiving RPCs.
// - Group 1 can still elect or retain its leader.
// - Group 1 can commit another entry.
// - The shared HTTP transport remains operational.
func TestMultiGroup_GroupFailureIsolation(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()

	allNodes := []string{"node-a"}
	meta := sharding.NewClusterMetadata("orbit-multi-test", 1, 2, 1, allNodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a"}})

	peerAddrs := map[string]string{
		"node-a": "127.0.0.1:0",
	}

	cfgA := node.NodeConfig{
		ID:                 "node-a",
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		DataDir:            filepath.Join(baseDir, "node-a"),
		HeartbeatInterval:  30 * time.Millisecond,
		ElectionTimeoutMin: 100 * time.Millisecond,
		ElectionTimeoutMax: 200 * time.Millisecond,
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
	if !ok0 || !ok1 {
		t.Fatalf("expected node-a to host shard 0 and shard 1")
	}

	// Wait for both groups to become leaders
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if shard0.Raft().Role() == raft.Leader && shard1.Raft().Role() == raft.Leader {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	client := raft.NewHTTPTransport("127.0.0.1:0")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Initial verification: both groups accept RPCs
	_, err = client.SendAppendEntries(ctx, httpTrA.Addr(), raft.GroupID("shard-0"), &raft.AppendEntriesRequest{
		Term:     1,
		LeaderID: "node-a",
	})
	if err != nil {
		t.Fatalf("initial append to shard-0: %v", err)
	}
	_, err = client.SendAppendEntries(ctx, httpTrA.Addr(), raft.GroupID("shard-1"), &raft.AppendEntriesRequest{
		Term:     1,
		LeaderID: "node-a",
	})
	if err != nil {
		t.Fatalf("initial append to shard-1: %v", err)
	}

	// Stop Group 0 while Group 1 is running
	if err := shard0.Stop(); err != nil {
		t.Fatalf("stop shard-0: %v", err)
	}

	// 1. Group 0 rejects new RPCs (returns ErrGroupNotFound)
	_, errRejected0 := client.SendAppendEntries(ctx, httpTrA.Addr(), raft.GroupID("shard-0"), &raft.AppendEntriesRequest{
		Term:     2,
		LeaderID: "node-x",
	})
	if errRejected0 == nil {
		t.Fatal("expected error sending to stopped Group 0, got nil")
	}
	if !errors.Is(errRejected0, raft.ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got: %v", errRejected0)
	}

	// 2. Group 1 continues receiving RPCs
	appendResp1, errReceived1 := client.SendAppendEntries(ctx, httpTrA.Addr(), raft.GroupID("shard-1"), &raft.AppendEntriesRequest{
		Term:     shard1.Raft().CurrentTerm(),
		LeaderID: "node-a",
	})
	if errReceived1 != nil {
		t.Fatalf("Group 1 should continue receiving RPCs, got err: %v", errReceived1)
	}
	if appendResp1 == nil {
		t.Fatal("expected non-nil response for Group 1")
	}

	// 3. Group 1 can still elect or retain its leader
	if shard1.Raft().Role() != raft.Leader {
		t.Fatalf("expected Group 1 to retain leader role, got %v", shard1.Raft().Role())
	}

	// 4. Group 1 can commit another entry
	idx1, _, err := shard1.Raft().Propose("SET", "group1:isolated", "preserved")
	if err != nil {
		t.Fatalf("propose on Group 1: %v", err)
	}
	if err := shard1.Raft().WaitForApply(ctx, idx1); err != nil {
		t.Fatalf("wait for apply Group 1: %v", err)
	}
	val1, err := shard1.DB().Get("group1:isolated")
	if err != nil || val1 != "preserved" {
		t.Errorf("Group 1 DB: got %q (err=%v), want 'preserved'", val1, err)
	}

	// 5. The shared HTTP transport remains operational
	httpResp, err := http.Get("http://" + httpTrA.Addr() + "/raft/append")
	if err != nil {
		t.Fatalf("expected transport to remain alive, got: %v", err)
	}
	_ = httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected StatusMethodNotAllowed from active HTTP transport, got %d", httpResp.StatusCode)
	}
}

// TestMultiGroup_TermIsolation verifies that term changes remain strictly isolated across groups.
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
	if !ok0 || !ok1 {
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

// TestMultiGroup_PartialStartupCleanup verifies Task 5:
// If a node hosts 3 shards and shard 2 fails to start, the node cleanly stops
// shards 0 and 1, closes the shared transport, and marks the node as closed.
func TestMultiGroup_PartialStartupCleanup(t *testing.T) {
	t.Parallel()

	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	allNodes := []string{"node-a"}
	meta := sharding.NewClusterMetadata("orbit-multi-test", 1, 3, 1, allNodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 2, Replicas: []string{"node-a"}})

	peerAddrs := map[string]string{
		"node-a": "node-a",
	}

	cfgA := node.NodeConfig{
		ID:                 "node-a",
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		DataDir:            filepath.Join(baseDir, "node-a"),
		HeartbeatInterval:  30 * time.Millisecond,
		ElectionTimeoutMin: 100 * time.Millisecond,
		ElectionTimeoutMax: 200 * time.Millisecond,
	}

	trA := net.CreateTransport("node-a")
	nodeA, err := node.NewShardedNode(cfgA, meta, peerAddrs, trA)
	if err != nil {
		t.Fatalf("NewShardedNode: %v", err)
	}

	shard0, ok0 := nodeA.Shard(0)
	shard1, ok1 := nodeA.Shard(1)
	shard2, ok2 := nodeA.Shard(2)
	if !ok0 || !ok1 || !ok2 {
		t.Fatalf("expected node-a to host shards 0, 1, and 2")
	}

	// Deliberately stop shard 2 before starting nodeA so that shard2.Start() fails with ErrStopped
	if err := shard2.Stop(); err != nil {
		t.Fatalf("shard2.Stop(): %v", err)
	}

	// Starting nodeA must fail because shard 2 cannot start
	errStart := nodeA.Start()
	if errStart == nil {
		t.Fatal("expected nodeA.Start() to fail, got nil")
	}

	// Verify that shards 0 and 1 were rolled back and stopped
	if err := shard0.Start(); !errors.Is(err, raft.ErrStopped) {
		t.Errorf("expected shard 0 to be stopped, got %v", err)
	}
	if err := shard1.Start(); !errors.Is(err, raft.ErrStopped) {
		t.Errorf("expected shard 1 to be stopped, got %v", err)
	}

	// Verify node is marked closed
	if err := nodeA.Start(); err == nil {
		t.Error("expected subsequent nodeA.Start() to return closed error, got nil")
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
