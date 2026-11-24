package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/node"
	"orbit/internal/raft"
)

// errInjected is a sentinel error for fault injection tests.
var errInjected = errors.New("fault injection: disk/storage failure")

// failingStorage wraps raft.Storage and allows injecting errors into SaveState or AppendEntries.
type failingStorage struct {
	raft.Storage
	failSaveState     bool
	failAppendEntries bool
	failTruncate      bool
}

func (f *failingStorage) SaveState(term uint64, votedFor string) error {
	if f.failSaveState {
		return errInjected
	}
	return f.Storage.SaveState(term, votedFor)
}

func (f *failingStorage) AppendEntries(entries []raft.LogEntry) error {
	if f.failAppendEntries {
		return errInjected
	}
	return f.Storage.AppendEntries(entries)
}

func (f *failingStorage) Truncate(fromIndex uint64) error {
	if f.failTruncate {
		return errInjected
	}
	return f.Storage.Truncate(fromIndex)
}

// TestApplyLoopStopsOnStateMachineError verifies P0 issue 1:
// State-machine application errors MUST NOT be silently ignored,
// lastApplied must NOT advance past a failing entry, and WaitForApply must surface the error.
func TestApplyLoopStopsOnStateMachineError(t *testing.T) {
	st, err := raft.OpenFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer st.Close()

	net := raft.NewMockNetwork()
	tr := net.CreateTransport("node-1")

	appliedIndices := make([]uint64, 0)
	failOnIndex := uint64(2)

	r, err := raft.New(raft.Config{
		ID:        "node-1",
		Storage:   st,
		Transport: tr,
		CommitCallback: func(entry raft.LogEntry) error {
			if entry.Index == failOnIndex {
				return errInjected
			}
			appliedIndices = append(appliedIndices, entry.Index)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("create raft: %v", err)
	}

	if err := r.Start(); err != nil {
		t.Fatalf("start raft: %v", err)
	}
	defer r.Stop()

	// Wait for single node to become leader
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if r.Role() == raft.Leader {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r.Role() != raft.Leader {
		t.Fatalf("expected single node to become leader, got role: %v", r.Role())
	}

	// Propose 3 entries (single node commits immediately)
	idx1, _, err := r.Propose("SET", "k1", "v1")
	if err != nil {
		t.Fatalf("propose 1: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := r.WaitForApply(ctx, idx1); err != nil {
		t.Fatalf("entry 1 should succeed, got: %v", err)
	}

	// Propose entry 2 which will fail in callback
	idx2, _, err := r.Propose("SET", "k2", "v2")
	if err != nil {
		t.Fatalf("propose 2: %v", err)
	}

	// WaitForApply for entry 2 MUST fail with the injected error
	err = r.WaitForApply(ctx, idx2)
	if err == nil {
		t.Fatal("expected WaitForApply(2) to fail, but it returned success")
	}
	if !strings.Contains(err.Error(), "state-machine apply failed") {
		t.Fatalf("expected state-machine error, got: %v", err)
	}

	// Invariant: lastApplied must remain 1, NEVER advancing to 2 or 3
	if r.LastApplied() != 1 {
		t.Fatalf("expected lastApplied to remain 1 after failure, got: %d", r.LastApplied())
	}

	// ApplyErr must be non-nil
	if r.ApplyErr() == nil {
		t.Fatal("expected ApplyErr() to report the failure")
	}
}

// TestPersistenceFailureRejectsVote verifies P0 issue 2:
// A vote is NEVER granted if persisting the vote to disk fails.
func TestPersistenceFailureRejectsVote(t *testing.T) {
	rawStorage, err := raft.OpenFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer rawStorage.Close()

	failSt := &failingStorage{Storage: rawStorage, failSaveState: true}
	net := raft.NewMockNetwork()
	tr := net.CreateTransport("voter")

	r, err := raft.New(raft.Config{
		ID:        "voter",
		Storage:   failSt,
		Transport: tr,
	})
	if err != nil {
		t.Fatalf("create raft: %v", err)
	}

	req := &raft.RequestVoteRequest{
		Term:         1,
		CandidateID:  "candidate-a",
		LastLogIndex: 0,
		LastLogTerm:  0,
	}

	resp, err := r.HandleRequestVote(context.Background(), req)
	if err == nil && resp.VoteGranted {
		t.Fatal("vote was granted despite disk persistence failure!")
	}
	if resp != nil && resp.VoteGranted {
		t.Fatal("VoteGranted must be false when disk write fails")
	}
	if r.VotedFor() == "candidate-a" {
		t.Fatal("votedFor in memory must not be set to candidate if disk write failed")
	}
}

// TestPersistenceFailureAbortsElection verifies P0 issue 2:
// If persisting the incremented term fails, the candidate aborts election.
func TestPersistenceFailureAbortsElection(t *testing.T) {
	rawStorage, err := raft.OpenFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer rawStorage.Close()

	failSt := &failingStorage{Storage: rawStorage, failSaveState: true}
	net := raft.NewMockNetwork()
	tr := net.CreateTransport("node-x")

	r, err := raft.New(raft.Config{
		ID:                 "node-x",
		Storage:            failSt,
		Transport:          tr,
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 50 * time.Millisecond,
		ElectionTimeoutMax: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("create raft: %v", err)
	}

	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer r.Stop()

	// Wait past election timeout
	time.Sleep(150 * time.Millisecond)

	// Since SaveState fails, node must NOT have become Candidate or Leader
	if r.Role() == raft.Leader {
		t.Fatal("node became leader despite persistence failures on disk")
	}
	if r.CurrentTerm() > 0 {
		t.Fatalf("expected term 0 because persistence failed, got %d", r.CurrentTerm())
	}
}

// TestIsolatedLeaderStaleReadPrevented verifies P0 issue 3:
// An isolated partition leader CANNOT serve stale reads because VerifyLeadership
// fails to achieve majority quorum confirmations.
func TestIsolatedLeaderStaleReadPrevented(t *testing.T) {
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

	// Elect initial leader
	var leader *node.Node
	var leaderIdx int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for i, n := range nodes {
			if n.Raft().Role() == raft.Leader {
				leader = n
				leaderIdx = i
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

	// 1. Initial write of account balance = 100
	idx, _, err := leader.Raft().Propose("SET", "balance", "100")
	if err != nil {
		t.Fatalf("propose initial balance: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := leader.Raft().WaitForApply(ctx, idx); err != nil {
		cancel()
		t.Fatalf("apply initial balance: %v", err)
	}
	cancel()

	// 2. Isolate the leader completely from B and C
	net.Isolate(leader.ID())

	// 3. Remaining 2 nodes elect a new leader
	var newLeader *node.Node
	failoverDeadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(failoverDeadline) {
		for i, n := range nodes {
			if i != leaderIdx && n.Raft().Role() == raft.Leader {
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
		t.Fatal("new leader not elected")
	}

	// 4. Update balance to 500 on the new leader
	newIdx, _, err := newLeader.Raft().Propose("SET", "balance", "500")
	if err != nil {
		t.Fatalf("propose on new leader: %v", err)
	}
	ctxNew, cancelNew := context.WithTimeout(context.Background(), 2*time.Second)
	if err := newLeader.Raft().WaitForApply(ctxNew, newIdx); err != nil {
		cancelNew()
		t.Fatalf("apply on new leader: %v", err)
	}
	cancelNew()

	// 5. Send GET request to old isolated leader
	// If it used naive role check, it would return stale balance "100".
	// With quorum-confirmed reads, VerifyLeadership MUST reject this stale read!
	peerAddrs := make(map[string]string)
	for _, c := range configs {
		peerAddrs[c.ID] = c.Address
	}
	oldLeaderHandler := api.NewRaftHandler(leader.Raft(), leader.DB(), leader.ID(), configs[leaderIdx].Address, peerAddrs)
	oldLeaderMux := http.NewServeMux()
	api.RegisterRaftRoutes(oldLeaderMux, oldLeaderHandler)

	req := httptest.NewRequest("GET", "/api/v1/kv/balance", nil)
	rec := httptest.NewRecorder()
	oldLeaderMux.ServeHTTP(rec, req)

	// It must NOT return 200 OK with stale "100"
	if rec.Code == http.StatusOK {
		t.Fatalf("stale read occurred! Isolated leader returned 200 OK with body: %s", rec.Body.String())
	}
	// Must return 503 Service Unavailable (unable to confirm quorum) or 307 redirect
	if rec.Code != http.StatusServiceUnavailable && rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 503 or 307 on isolated leader read, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestStorageAppendEntriesFailureLeavesMemoryIntact verifies P1 issue 4:
// If writing to disk fails during AppendEntries, in-memory cache is NOT mutated.
func TestStorageAppendEntriesFailureLeavesMemoryIntact(t *testing.T) {
	st, err := raft.OpenFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer st.Close()

	// Initially append 1 entry
	if err := st.AppendEntries([]raft.LogEntry{{Index: 1, Term: 1, Operation: "SET", Key: "a", Value: "1"}}); err != nil {
		t.Fatalf("append entry 1: %v", err)
	}

	// Close the file handle to force subsequent writes to fail
	_ = st.Close()

	// Attempt to append entry 2
	err = st.AppendEntries([]raft.LogEntry{{Index: 2, Term: 1, Operation: "SET", Key: "b", Value: "2"}})
	if err == nil {
		t.Fatal("expected append to fail on closed storage")
	}

	// Entry 2 must NOT exist in storage cache
	if _, ok := st.GetEntry(2); ok {
		t.Fatal("entry 2 was cached in memory despite disk failure!")
	}
}

// TestConflictingSuffixReplacementAndRecovery verifies P1 issue 5:
// Follower has conflicting suffix entries from an old leader.
// New leader truncates conflicting entries, appends new ones.
// Follower reboots from disk and verifies state is completely consistent.
func TestConflictingSuffixReplacementAndRecovery(t *testing.T) {
	dir := t.TempDir()

	// 1. Follower receives initial entries 1, 2, 3 (term 1)
	st, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}

	_ = st.SaveState(2, "")
	_ = st.AppendEntries([]raft.LogEntry{
		{Index: 1, Term: 1, Operation: "SET", Key: "k1", Value: "1"},
		{Index: 2, Term: 1, Operation: "SET", Key: "k2", Value: "old2"},
		{Index: 3, Term: 1, Operation: "SET", Key: "k3", Value: "old3"},
	})

	// 2. New leader for term 2 truncates from index 2 and appends new entries
	if err := st.Truncate(2); err != nil {
		t.Fatalf("truncate from index 2: %v", err)
	}

	if err := st.AppendEntries([]raft.LogEntry{
		{Index: 2, Term: 2, Operation: "SET", Key: "k2", Value: "new2"},
		{Index: 3, Term: 2, Operation: "SET", Key: "k3", Value: "new3"},
		{Index: 4, Term: 2, Operation: "SET", Key: "k4", Value: "new4"},
	}); err != nil {
		t.Fatalf("append new entries: %v", err)
	}

	_ = st.Close()

	// 3. Restart and recover from disk
	st2, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("recover storage after conflict truncate: %v", err)
	}
	defer st2.Close()

	lastIdx, lastTerm := st2.LastLogInfo()
	if lastIdx != 4 || lastTerm != 2 {
		t.Fatalf("expected lastIdx=4, lastTerm=2; got idx=%d, term=%d", lastIdx, lastTerm)
	}

	e2, ok := st2.GetEntry(2)
	if !ok || e2.Term != 2 || e2.Value != "new2" {
		t.Fatalf("entry 2 not recovered properly: %+v", e2)
	}
}
