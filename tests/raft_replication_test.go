package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"orbit/internal/raft"
)

// TestLogReplicationBasic verifies commands proposed on leader replicate across all followers.
func TestLogReplicationBasic(t *testing.T) {
	_, nodes, cleanup := createTestCluster(t, 3)
	defer cleanup()

	for _, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	// 1. Wait for a leader to emerge
	var leader *raft.Raft
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range nodes {
			if n.Role() == raft.Leader {
				leader = n
				break
			}
		}
		if leader != nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if leader == nil {
		t.Fatal("failed to elect leader")
	}

	// 2. Propose entries on the leader
	commands := []struct {
		op, k, v string
	}{
		{"SET", "user:1", "Anshik"},
		{"SET", "user:2", "Rahul"},
		{"DELETE", "user:1", ""},
	}

	for _, c := range commands {
		_, _, err := leader.Propose(c.op, c.k, c.v)
		if err != nil {
			t.Fatalf("Propose(%q): %v", c.k, err)
		}
	}

	// 3. Wait for replication across all nodes
	repDeadline := time.Now().Add(6 * time.Second)
	allReplicated := false

	for time.Now().Before(repDeadline) {
		replicatedCount := 0
		for _, n := range nodes {
			lastIdx, _ := n.Storage().LastLogInfo()
			if lastIdx >= 3 {
				replicatedCount++
			}
		}
		if replicatedCount == len(nodes) {
			allReplicated = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !allReplicated {
		t.Fatal("entries were not replicated to all 3 nodes within deadline")
	}

	// Verify all 3 nodes have the 3 entries matching exactly
	for _, n := range nodes {
		for idx := uint64(1); idx <= 3; idx++ {
			want := commands[idx-1]
			got, ok := n.Storage().GetEntry(idx)
			if !ok {
				t.Fatalf("node %s missing entry at index %d", n.ID(), idx)
			}
			if got.Operation != want.op || got.Key != want.k || got.Value != want.v {
				t.Fatalf("node %s index %d mismatch: want %+v, got %+v", n.ID(), idx, want, got)
			}
		}
	}
}

// TestNonLeaderProposeRejected verifies non-leader nodes reject client proposals.
func TestNonLeaderProposeRejected(t *testing.T) {
	_, nodes, cleanup := createTestCluster(t, 3)
	defer cleanup()

	for _, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	time.Sleep(300 * time.Millisecond)

	var follower *raft.Raft
	for _, n := range nodes {
		if n.Role() == raft.Follower {
			follower = n
			break
		}
	}
	if follower == nil {
		t.Fatal("no follower found")
	}

	_, _, err := follower.Propose("SET", "k", "v")
	if !errors.Is(err, raft.ErrNotLeader) {
		t.Fatalf("expected ErrNotLeader when proposing to follower, got: %v", err)
	}
}

// TestReplicationCatchesUpPartitionedFollower verifies an offline follower catches up
// when rejoining the cluster.
func TestReplicationCatchesUpPartitionedFollower(t *testing.T) {
	net, nodes, cleanup := createTestCluster(t, 3)
	defer cleanup()

	for _, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	// 1. Wait for leader
	var leader *raft.Raft
	var followerToIsolate *raft.Raft
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range nodes {
			if n.Role() == raft.Leader {
				leader = n
			}
		}
		if leader != nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if leader == nil {
		t.Fatal("leader not elected")
	}

	for _, n := range nodes {
		if n.ID() != leader.ID() {
			followerToIsolate = n
			break
		}
	}

	// 2. Isolate one follower
	net.Isolate(followerToIsolate.ID())

	// 3. Propose entries to leader (replicates to the other follower, reaching majority)
	for i := 0; i < 5; i++ {
		_, _, err := leader.Propose("SET", "key", "val")
		if err != nil {
			t.Fatalf("Propose %d: %v", i, err)
		}
	}

	// Give majority time to replicate
	time.Sleep(100 * time.Millisecond)

	// 4. Rejoin isolated follower
	net.Rejoin(followerToIsolate.ID())

	// Wait for leader to catch up the reconnected follower
	catchUpDeadline := time.Now().Add(6 * time.Second)
	caughtUp := false
	for time.Now().Before(catchUpDeadline) {
		lastIdx, _ := followerToIsolate.Storage().LastLogInfo()
		if lastIdx >= 5 {
			caughtUp = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !caughtUp {
		lastIdx, _ := followerToIsolate.Storage().LastLogInfo()
		t.Fatalf("reconnected follower failed to catch up log entries: lastIdx=%d, want 5", lastIdx)
	}

	// Isolated follower must now be in Follower role with updated term and log
	if followerToIsolate.Role() != raft.Follower {
		t.Fatalf("expected follower to remain follower, got %v", followerToIsolate.Role())
	}
}

// TestFollowerConflictingLogReplacement verifies that divergent uncommitted follower entries
// are truncated and overwritten by the leader's authoritative entries.
func TestFollowerConflictingLogReplacement(t *testing.T) {
	dir := t.TempDir()
	st, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer st.Close()

	// Follower's initial log: index 1 (term 1), index 2 (term 1), index 3 (term 1 - conflicting!)
	_ = st.AppendEntries([]raft.LogEntry{
		{Index: 1, Term: 1, Operation: "SET", Key: "k1", Value: "v1"},
		{Index: 2, Term: 1, Operation: "SET", Key: "k2", Value: "v2"},
		{Index: 3, Term: 1, Operation: "SET", Key: "k3", Value: "conflicting-old-val"},
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

	// Leader sends AppendEntries for term 2:
	// PrevLogIndex: 2, PrevLogTerm: 1
	// Entries: index 3 (term 2, authoritative-new-val), index 4 (term 2, val4)
	resp, err := r.HandleAppendEntries(context.Background(), &raft.AppendEntriesRequest{
		Term:         2,
		LeaderID:     "node-leader",
		PrevLogIndex: 2,
		PrevLogTerm:  1,
		Entries: []raft.LogEntry{
			{Index: 3, Term: 2, Operation: "SET", Key: "k3", Value: "authoritative-new-val"},
			{Index: 4, Term: 2, Operation: "SET", Key: "k4", Value: "val-4"},
		},
		LeaderCommit: 3,
	})
	if err != nil {
		t.Fatalf("HandleAppendEntries: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected AppendEntries to succeed, got Success=false (term=%d)", resp.Term)
	}

	// Verify follower's log now contains authoritative term 2 entry at index 3
	e3, ok := st.GetEntry(3)
	if !ok || e3.Term != 2 || e3.Value != "authoritative-new-val" {
		t.Fatalf("expected index 3 to be replaced by term 2 'authoritative-new-val', got %+v", e3)
	}

	// Verify index 4 appended
	e4, ok := st.GetEntry(4)
	if !ok || e4.Term != 2 || e4.Value != "val-4" {
		t.Fatalf("expected index 4 to be term 2 'val-4', got %+v", e4)
	}
}
