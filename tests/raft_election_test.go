package tests

import (
	"context"
	"testing"
	"time"

	"orbit/internal/raft"
)

// helper to create a 3-node test cluster using MockNetwork
func createTestCluster(t *testing.T, count int) (*raft.MockNetwork, []*raft.Raft, func()) {
	t.Helper()
	net := raft.NewMockNetwork()
	nodeIDs := []string{"node-a", "node-b", "node-c"}[:count]

	nodes := make([]*raft.Raft, count)
	storages := make([]raft.Storage, count)

	for i, id := range nodeIDs {
		dir := t.TempDir()
		st, err := raft.OpenFileStorage(dir)
		if err != nil {
			t.Fatalf("open storage %s: %v", id, err)
		}
		storages[i] = st

		peers := make(map[string]string)
		for _, other := range nodeIDs {
			if other != id {
				peers[other] = other
			}
		}

		tr := net.CreateTransport(id)
		r, err := raft.New(raft.Config{
			ID:                 id,
			Peers:              peers,
			Storage:            st,
			Transport:          tr,
			HeartbeatInterval:  30 * time.Millisecond,
			ElectionTimeoutMin: 80 * time.Millisecond,
			ElectionTimeoutMax: 160 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("create raft node %s: %v", id, err)
		}
		nodes[i] = r
	}

	cleanup := func() {
		for _, n := range nodes {
			_ = n.Stop()
		}
	}

	return net, nodes, cleanup
}

// TestSingleNodeElection verifies that a lone node elects itself leader immediately.
func TestSingleNodeElection(t *testing.T) {
	_, nodes, cleanup := createTestCluster(t, 1)
	defer cleanup()

	node := nodes[0]
	if err := node.Start(); err != nil {
		t.Fatalf("Start node: %v", err)
	}

	// Must become leader quickly
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if node.Role() == raft.Leader {
			return // Success
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("single node failed to become leader, role=%v", node.Role())
}

// TestThreeNodeElection verifies that a 3-node cluster elects exactly ONE leader.
func TestThreeNodeElection(t *testing.T) {
	_, nodes, cleanup := createTestCluster(t, 3)
	defer cleanup()

	for _, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("Start node %s: %v", n.ID(), err)
		}
	}

	// Wait for election to resolve
	var leader *raft.Raft
	deadline := time.Now().Add(1 * time.Second)

	for time.Now().Before(deadline) {
		leaders := 0
		for _, n := range nodes {
			if n.Role() == raft.Leader {
				leaders++
				leader = n
			}
		}
		if leaders == 1 {
			// Found stable leader
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if leader == nil {
		t.Fatal("no leader elected after 1s")
	}

	// Verify other nodes are followers
	followerCount := 0
	for _, n := range nodes {
		if n.ID() != leader.ID() {
			if n.Role() == raft.Follower {
				followerCount++
			}
		}
	}
	if followerCount != 2 {
		t.Fatalf("expected 2 followers, got %d", followerCount)
	}
}

// TestLeaderHeartbeatsKeepFollowersQuiet verifies followers do not trigger elections
// while receiving periodic heartbeats from the leader.
func TestLeaderHeartbeatsKeepFollowersQuiet(t *testing.T) {
	_, nodes, cleanup := createTestCluster(t, 3)
	defer cleanup()

	for _, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	// Wait for initial election
	time.Sleep(300 * time.Millisecond)

	var leader *raft.Raft
	for _, n := range nodes {
		if n.Role() == raft.Leader {
			leader = n
			break
		}
	}
	if leader == nil {
		t.Fatal("expected a leader to be elected")
	}

	initialTerm := leader.CurrentTerm()

	// Wait for multiple heartbeat cycles (200ms = ~6 heartbeats)
	time.Sleep(200 * time.Millisecond)

	// Term must NOT have changed, and leader must still be the same
	if leader.Role() != raft.Leader {
		t.Fatalf("leader demoted unexpectedly: %v", leader.Role())
	}
	if leader.CurrentTerm() != initialTerm {
		t.Fatalf("term incremented during stable leader: %d != %d", leader.CurrentTerm(), initialTerm)
	}
}

// TestLeaderFailureTriggersNewElection verifies that when the leader crashes,
// remaining followers elect a new leader.
func TestLeaderFailureTriggersNewElection(t *testing.T) {
	net, nodes, cleanup := createTestCluster(t, 3)
	defer cleanup()

	for _, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	// Wait for leader election
	var oldLeader *raft.Raft
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range nodes {
			if n.Role() == raft.Leader {
				oldLeader = n
				break
			}
		}
		if oldLeader != nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if oldLeader == nil {
		t.Fatal("failed to elect initial leader")
	}

	oldTerm := oldLeader.CurrentTerm()

	// Disconnect / isolate the old leader
	net.Isolate(oldLeader.ID())

	// Wait for remaining 2 followers to detect heartbeat timeout and elect new leader
	var newLeader *raft.Raft
	failoverDeadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(failoverDeadline) {
		for _, n := range nodes {
			if n.ID() != oldLeader.ID() && n.Role() == raft.Leader {
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
		t.Fatal("remaining nodes failed to elect new leader after leader failure")
	}

	if newLeader.CurrentTerm() <= oldTerm {
		t.Fatalf("expected new leader term > %d, got %d", oldTerm, newLeader.CurrentTerm())
	}

	// Reconnect old leader: when it receives heartbeat from new leader, it must step down
	net.Rejoin(oldLeader.ID())
	time.Sleep(100 * time.Millisecond)

	if oldLeader.Role() != raft.Follower {
		t.Fatalf("old leader failed to step down after reconnecting: got role %v", oldLeader.Role())
	}
}

// TestOutdatedCandidateLogRejected verifies a node with a stale log is refused votes.
func TestOutdatedCandidateLogRejected(t *testing.T) {
	dir := t.TempDir()
	st, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer st.Close()

	// Follower has log up to index 3, term 2
	_ = st.AppendEntries([]raft.LogEntry{
		{Index: 1, Term: 1},
		{Index: 2, Term: 1},
		{Index: 3, Term: 2},
	})

	net := raft.NewMockNetwork()
	tr := net.CreateTransport("node-f")

	r, err := raft.New(raft.Config{
		ID:        "node-f",
		Storage:   st,
		Transport: tr,
	})
	if err != nil {
		t.Fatalf("raft.New: %v", err)
	}

	// Candidate with shorter log (index 2, term 1)
	resp, err := r.HandleRequestVote(context.Background(), &raft.RequestVoteRequest{
		Term:         3,
		CandidateID:  "candidate-stale",
		LastLogIndex: 2,
		LastLogTerm:  1,
	})
	if err != nil {
		t.Fatalf("HandleRequestVote: %v", err)
	}

	if resp.VoteGranted {
		t.Fatal("follower granted vote to candidate with outdated log term!")
	}
}
