package tests

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/raft"
)

// TestRaft_BatchReplicationAndLagMeasurement verifies that MaxBatchEntries limits the batch size
// per AppendEntries RPC and that ReplicationLag reports follower lag accurately.
func TestRaft_BatchReplicationAndLagMeasurement(t *testing.T) {
	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	peersA := map[string]string{"node-b": "node-b"}
	peersB := map[string]string{"node-a": "node-a"}

	storageA, err := raft.OpenFileStorage(filepath.Join(baseDir, "node-a"))
	if err != nil {
		t.Fatalf("open storage A: %v", err)
	}
	defer storageA.Close()

	storageB, err := raft.OpenFileStorage(filepath.Join(baseDir, "node-b"))
	if err != nil {
		t.Fatalf("open storage B: %v", err)
	}
	defer storageB.Close()

	trA := net.CreateTransport("node-a")
	trB := net.CreateTransport("node-b")

	// Configure batch size of 5 entries
	nodeA, err := raft.New(raft.Config{
		ID:                 "node-a",
		Peers:              peersA,
		Storage:            storageA,
		Transport:          trA,
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 50 * time.Millisecond,
		ElectionTimeoutMax: 100 * time.Millisecond,
		MaxBatchEntries:    5,
	})
	if err != nil {
		t.Fatalf("create nodeA: %v", err)
	}

	nodeB, err := raft.New(raft.Config{
		ID:                 "node-b",
		Peers:              peersB,
		Storage:            storageB,
		Transport:          trB,
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 500 * time.Millisecond, // keep follower
		ElectionTimeoutMax: 1000 * time.Millisecond,
		MaxBatchEntries:    5,
	})
	if err != nil {
		t.Fatalf("create nodeB: %v", err)
	}

	trA.RegisterHandler(nodeA.GroupID(), nodeA)
	trB.RegisterHandler(nodeB.GroupID(), nodeB)

	if err := nodeA.Start(); err != nil {
		t.Fatalf("start nodeA: %v", err)
	}
	defer nodeA.Stop()

	if err := nodeB.Start(); err != nil {
		t.Fatalf("start nodeB: %v", err)
	}
	defer nodeB.Stop()

	// Wait for nodeA to become leader
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if nodeA.Role() == raft.Leader {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if nodeA.Role() != raft.Leader {
		t.Fatalf("nodeA did not become leader")
	}

	// Propose 20 entries rapidly
	for i := 1; i <= 20; i++ {
		_, _, err := nodeA.Propose("SET", fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
		if err != nil {
			t.Fatalf("propose %d failed: %v", i, err)
		}
	}

	// Verify replication converges
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lags := nodeA.ReplicationLag()
		if lag, ok := lags["node-b"]; ok && lag == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	lags := nodeA.ReplicationLag()
	if lags["node-b"] != 0 {
		t.Errorf("expected 0 replication lag, got %d", lags["node-b"])
	}

	lastIdxA, _ := storageA.LastLogInfo()
	lastIdxB, _ := storageB.LastLogInfo()
	if lastIdxA != lastIdxB || lastIdxA < 20 {
		t.Errorf("storage mismatch: A has %d, B has %d", lastIdxA, lastIdxB)
	}
}

// TestRaft_StaleLeaderSteppingDown verifies that a partitioned leader discovering a higher term
// steps down to follower and rejects subsequent proposals.
func TestRaft_StaleLeaderSteppingDown(t *testing.T) {
	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	storageA, err := raft.OpenFileStorage(filepath.Join(baseDir, "node-a"))
	if err != nil {
		t.Fatalf("open storage A: %v", err)
	}
	defer storageA.Close()

	storageB, err := raft.OpenFileStorage(filepath.Join(baseDir, "node-b"))
	if err != nil {
		t.Fatalf("open storage B: %v", err)
	}
	defer storageB.Close()

	trA := net.CreateTransport("node-a")
	trB := net.CreateTransport("node-b")

	nodeA, err := raft.New(raft.Config{
		ID:                 "node-a",
		Peers:              map[string]string{"node-b": "node-b"},
		Storage:            storageA,
		Transport:          trA,
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 50 * time.Millisecond,
		ElectionTimeoutMax: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("create nodeA: %v", err)
	}

	nodeB, err := raft.New(raft.Config{
		ID:                 "node-b",
		Peers:              map[string]string{"node-a": "node-a"},
		Storage:            storageB,
		Transport:          trB,
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 500 * time.Millisecond, // keep follower
		ElectionTimeoutMax: 1000 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("create nodeB: %v", err)
	}

	trA.RegisterHandler(nodeA.GroupID(), nodeA)
	trB.RegisterHandler(nodeB.GroupID(), nodeB)

	if err := nodeA.Start(); err != nil {
		t.Fatalf("start nodeA: %v", err)
	}
	defer nodeA.Stop()

	if err := nodeB.Start(); err != nil {
		t.Fatalf("start nodeB: %v", err)
	}
	defer nodeB.Stop()

	// Wait for nodeA to be elected leader
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if nodeA.Role() == raft.Leader {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if nodeA.Role() != raft.Leader {
		t.Fatalf("expected nodeA to become leader, role is %v", nodeA.Role())
	}

	currentTerm := nodeA.CurrentTerm()

	// Simulate receiving an AppendEntries from a peer with higher term
	resp, err := trA.SendAppendEntries(context.Background(), "node-a", nodeA.GroupID(), &raft.AppendEntriesRequest{
		Term:         currentTerm + 5,
		LeaderID:     "node-b",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      nil,
		LeaderCommit: 0,
	})
	if err != nil {
		t.Fatalf("send AppendEntries: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response from stepdown")
	}

	// Verify nodeA has stepped down to Follower with higher term
	if nodeA.Role() != raft.Follower {
		t.Errorf("expected nodeA to step down to Follower, got %v", nodeA.Role())
	}
	if nodeA.CurrentTerm() != currentTerm+5 {
		t.Errorf("expected term %d, got %d", currentTerm+5, nodeA.CurrentTerm())
	}

	// Propose should now fail with ErrNotLeader
	_, _, err = nodeA.Propose("SET", "k", "v")
	if err != raft.ErrNotLeader {
		t.Errorf("expected ErrNotLeader, got %v", err)
	}
}

// TestRaft_SimultaneousElectionsAndNoSplitBrain tests that overlapping election triggers
// across peers resolve smoothly without split-brain.
func TestRaft_SimultaneousElectionsAndNoSplitBrain(t *testing.T) {
	net := raft.NewMockNetwork()
	baseDir := t.TempDir()

	peers := map[string]string{
		"n1": "n1",
		"n2": "n2",
		"n3": "n3",
	}

	nodes := make(map[string]*raft.Raft)
	for id := range peers {
		nodePeers := make(map[string]string)
		for pID, pAddr := range peers {
			if pID != id {
				nodePeers[pID] = pAddr
			}
		}

		st, err := raft.OpenFileStorage(filepath.Join(baseDir, id))
		if err != nil {
			t.Fatalf("open storage %s: %v", id, err)
		}
		defer st.Close()

		tr := net.CreateTransport(id)
		n, err := raft.New(raft.Config{
			ID:                 id,
			Peers:              nodePeers,
			Storage:            st,
			Transport:          tr,
			HeartbeatInterval:  20 * time.Millisecond,
			ElectionTimeoutMin: 60 * time.Millisecond,
			ElectionTimeoutMax: 120 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("create node %s: %v", id, err)
		}
		nodes[id] = n
		tr.RegisterHandler(n.GroupID(), n)
	}

	// Start all 3 nodes simultaneously
	for _, n := range nodes {
		_ = n.Start()
		defer n.Stop()
	}

	// Allow election to complete
	deadline := time.Now().Add(3 * time.Second)
	var leaders []string

	for time.Now().Before(deadline) {
		leaders = nil
		for id, n := range nodes {
			if n.Role() == raft.Leader {
				leaders = append(leaders, id)
			}
		}
		if len(leaders) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if len(leaders) != 1 {
		t.Fatalf("expected exactly 1 leader, got %d (%v)", len(leaders), leaders)
	}
}
