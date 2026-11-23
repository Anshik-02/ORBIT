package raft

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

type mockIsolationHandler struct {
	mu         sync.Mutex
	voteReqs   []*RequestVoteRequest
	appendReqs []*AppendEntriesRequest
}

func (m *mockIsolationHandler) HandleRequestVote(ctx context.Context, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.voteReqs = append(m.voteReqs, req)
	return &RequestVoteResponse{
		Term:        req.Term,
		VoteGranted: true,
	}, nil
}

func (m *mockIsolationHandler) HandleAppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendReqs = append(m.appendReqs, req)
	return &AppendEntriesResponse{
		Term:    req.Term,
		Success: true,
	}, nil
}

func (m *mockIsolationHandler) VoteCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.voteReqs)
}

func (m *mockIsolationHandler) AppendCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.appendReqs)
}

// TestTransportIsolation verifies all scenarios requested in Task 1:
// - Register Group 0 and Group 1: Both register successfully
// - Register Group 0 twice: ErrDuplicateGroup
// - Send RPC to Group 0: Only Group 0 handler receives it
// - Send RPC to Group 1: Only Group 1 handler receives it
// - Unregister Group 0: Group 1 continues working
// - Send RPC to unknown group: Returns ErrGroupNotFound
// - Stopping or unregistering one group must not close the shared HTTP transport.
func TestTransportIsolation(t *testing.T) {
	t.Parallel()

	transport := NewHTTPTransport("127.0.0.1:0")

	h0 := &mockIsolationHandler{}
	h1 := &mockIsolationHandler{}

	group0 := GroupID("group-0")
	group1 := GroupID("group-1")
	groupUnknown := GroupID("group-unknown")

	// 1. Register Group 0 and Group 1 -> Both register successfully
	if err := transport.RegisterHandler(group0, h0); err != nil {
		t.Fatalf("RegisterHandler(group-0) failed: %v", err)
	}
	if err := transport.RegisterHandler(group1, h1); err != nil {
		t.Fatalf("RegisterHandler(group-1) failed: %v", err)
	}

	// 2. Register Group 0 twice -> ErrDuplicateGroup
	errDup := transport.RegisterHandler(group0, h0)
	if errDup == nil {
		t.Fatal("expected error registering group-0 twice, got nil")
	}
	if !errors.Is(errDup, ErrDuplicateGroup) {
		t.Fatalf("expected ErrDuplicateGroup, got: %v", errDup)
	}

	// Start the shared HTTP transport
	if err := transport.Start(); err != nil {
		t.Fatalf("transport.Start() failed: %v", err)
	}
	defer transport.Close()

	client := NewHTTPTransport("127.0.0.1:0")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 3. Send RPC to Group 0 -> Only Group 0 handler receives it
	voteResp, err := client.SendRequestVote(ctx, transport.Addr(), group0, &RequestVoteRequest{
		Term:         1,
		CandidateID:  "cand-node-a",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if err != nil {
		t.Fatalf("SendRequestVote to group-0 failed: %v", err)
	}
	if !voteResp.VoteGranted {
		t.Errorf("expected VoteGranted=true for group-0, got false")
	}
	if h0.VoteCount() != 1 {
		t.Errorf("expected group-0 handler to receive 1 vote request, got %d", h0.VoteCount())
	}
	if h1.VoteCount() != 0 {
		t.Errorf("group-1 handler should not have received vote request, got %d", h1.VoteCount())
	}

	// 4. Send RPC to Group 1 -> Only Group 1 handler receives it
	appendResp, err := client.SendAppendEntries(ctx, transport.Addr(), group1, &AppendEntriesRequest{
		Term:         2,
		LeaderID:     "leader-node-b",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
	})
	if err != nil {
		t.Fatalf("SendAppendEntries to group-1 failed: %v", err)
	}
	if !appendResp.Success {
		t.Errorf("expected Success=true for group-1, got false")
	}
	if h1.AppendCount() != 1 {
		t.Errorf("expected group-1 handler to receive 1 append request, got %d", h1.AppendCount())
	}
	if h0.AppendCount() != 0 {
		t.Errorf("group-0 handler should not have received append request, got %d", h0.AppendCount())
	}

	// 5. Unregister Group 0 -> Group 1 continues working
	transport.UnregisterHandler(group0)

	// Verify shared HTTP transport remains active and Group 1 still handles RPCs
	appendResp2, err := client.SendAppendEntries(ctx, transport.Addr(), group1, &AppendEntriesRequest{
		Term:         3,
		LeaderID:     "leader-node-b",
		PrevLogIndex: 1,
		PrevLogTerm:  2,
	})
	if err != nil {
		t.Fatalf("group-1 should continue working after unregistering group-0, got error: %v", err)
	}
	if !appendResp2.Success {
		t.Errorf("expected Success=true for group-1 after unregistering group-0")
	}
	if h1.AppendCount() != 2 {
		t.Errorf("expected group-1 to have received 2 append requests, got %d", h1.AppendCount())
	}

	// 6. Send RPC to unknown group (including now-unregistered Group 0) -> Returns ErrGroupNotFound
	_, errNotFound0 := client.SendRequestVote(ctx, transport.Addr(), group0, &RequestVoteRequest{
		Term:        4,
		CandidateID: "cand-node-a",
	})
	if errNotFound0 == nil {
		t.Fatal("expected ErrGroupNotFound for unregistered group-0, got nil")
	}
	if !errors.Is(errNotFound0, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound for group-0, got: %v", errNotFound0)
	}

	_, errNotFoundUnknown := client.SendAppendEntries(ctx, transport.Addr(), groupUnknown, &AppendEntriesRequest{
		Term:     4,
		LeaderID: "leader-node-x",
	})
	if errNotFoundUnknown == nil {
		t.Fatal("expected ErrGroupNotFound for non-existent group-unknown, got nil")
	}
	if !errors.Is(errNotFoundUnknown, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound for group-unknown, got: %v", errNotFoundUnknown)
	}

	// 7. Important: Stopping or unregistering one group must not close the shared HTTP transport.
	// Verify transport is not closed by testing raw HTTP response
	httpResp, err := http.Get("http://" + transport.Addr() + "/raft/vote")
	if err != nil {
		t.Fatalf("expected transport HTTP listener to remain alive, got err: %v", err)
	}
	_ = httpResp.Body.Close()
	// Method GET should return StatusMethodNotAllowed (proving the server is alive and responding)
	if httpResp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected StatusMethodNotAllowed from alive server, got %d", httpResp.StatusCode)
	}
}
