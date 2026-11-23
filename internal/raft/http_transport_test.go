package raft

import (
	"context"
	"errors"
	"fmt"
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

func (m *mockIsolationHandler) HandleInstallSnapshot(ctx context.Context, req *InstallSnapshotRequest) (*InstallSnapshotResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &InstallSnapshotResponse{
		Term: req.Term,
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

// TestHTTPTransport_RejectNilAndInvalidHandlers verifies Task 1:
// Reject nil handler, duplicate GroupID, and invalid GroupID.
func TestHTTPTransport_RejectNilAndInvalidHandlers(t *testing.T) {
	t.Parallel()

	transport := NewHTTPTransport("127.0.0.1:0")
	handler := &mockIsolationHandler{}

	// 1. Reject nil handler
	if err := transport.RegisterHandler(GroupID("group-0"), nil); err == nil {
		t.Fatal("expected error for nil handler, got nil")
	} else if !errors.Is(err, ErrNilHandler) {
		t.Fatalf("expected ErrNilHandler, got: %v", err)
	}

	// 2. Reject empty GroupID
	if err := transport.RegisterHandler(GroupID(""), handler); err == nil {
		t.Fatal("expected error for empty GroupID, got nil")
	} else if !errors.Is(err, ErrInvalidGroupID) {
		t.Fatalf("expected ErrInvalidGroupID, got: %v", err)
	}

	// 3. Reject whitespace GroupID
	if err := transport.RegisterHandler(GroupID("   "), handler); err == nil {
		t.Fatal("expected error for whitespace GroupID, got nil")
	} else if !errors.Is(err, ErrInvalidGroupID) {
		t.Fatalf("expected ErrInvalidGroupID, got: %v", err)
	}

	// 4. Register valid GroupID
	if err := transport.RegisterHandler(GroupID("group-valid"), handler); err != nil {
		t.Fatalf("first valid registration failed: %v", err)
	}

	// 5. Reject duplicate GroupID
	if err := transport.RegisterHandler(GroupID("group-valid"), handler); err == nil {
		t.Fatal("expected error for duplicate GroupID, got nil")
	} else if !errors.Is(err, ErrDuplicateGroup) {
		t.Fatalf("expected ErrDuplicateGroup, got: %v", err)
	}
}

// TestHTTPTransport_ConcurrentOperations verifies Task 5:
// Concurrently sends RPCs to multiple groups, registers handlers, and unregisters handlers.
func TestHTTPTransport_ConcurrentOperations(t *testing.T) {
	t.Parallel()

	transport := NewHTTPTransport("127.0.0.1:0")

	// Pre-register two static groups
	h0 := &mockIsolationHandler{}
	h1 := &mockIsolationHandler{}
	_ = transport.RegisterHandler(GroupID("static-0"), h0)
	_ = transport.RegisterHandler(GroupID("static-1"), h1)

	if err := transport.Start(); err != nil {
		t.Fatalf("transport.Start() failed: %v", err)
	}
	defer transport.Close()

	client := NewHTTPTransport("127.0.0.1:0")
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Goroutines 1-3: Concurrently send RPCs to static-0 and static-1
	for workerID := 0; workerID < 3; workerID++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_, _ = client.SendRequestVote(ctx, transport.Addr(), GroupID("static-0"), &RequestVoteRequest{
					Term:        uint64(i + 1),
					CandidateID: "cand",
				})
				_, _ = client.SendAppendEntries(ctx, transport.Addr(), GroupID("static-1"), &AppendEntriesRequest{
					Term:     uint64(i + 1),
					LeaderID: "leader",
				})
			}
		}(workerID)
	}

	// Goroutines 4-5: Concurrently register and unregister dynamic groups
	for workerID := 0; workerID < 2; workerID++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				dynGroup := GroupID(fmt.Sprintf("dyn-%d-%d", w, i))
				_ = transport.RegisterHandler(dynGroup, &mockIsolationHandler{})
				// Send an RPC to dynamic group
				_, _ = client.SendRequestVote(ctx, transport.Addr(), dynGroup, &RequestVoteRequest{Term: 1})
				// Unregister dynamic group
				transport.UnregisterHandler(dynGroup)
				// Send another RPC which should return ErrGroupNotFound without panicking
				_, _ = client.SendRequestVote(ctx, transport.Addr(), dynGroup, &RequestVoteRequest{Term: 2})
			}
		}(workerID)
	}

	wg.Wait()
}

// TestHTTPTransport_ConcurrentStress is a more intensive stress test with higher goroutine counts.
// It concurrently sends 50 vote requests to static-0, 50 AppendEntries requests to static-1,
// and concurrently registers/unregisters 20 dynamic handlers — all under -race.
func TestHTTPTransport_ConcurrentStress(t *testing.T) {
	t.Parallel()

	transport := NewHTTPTransport("127.0.0.1:0")

	h0 := &mockIsolationHandler{}
	h1 := &mockIsolationHandler{}
	_ = transport.RegisterHandler(GroupID("static-0"), h0)
	_ = transport.RegisterHandler(GroupID("static-1"), h1)

	if err := transport.Start(); err != nil {
		t.Fatalf("transport.Start() failed: %v", err)
	}
	defer transport.Close()

	client := NewHTTPTransport("127.0.0.1:0")
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 50 goroutines each sending 1 vote request to static-0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = client.SendRequestVote(ctx, transport.Addr(), GroupID("static-0"), &RequestVoteRequest{
				Term:        uint64(n + 1),
				CandidateID: fmt.Sprintf("stress-cand-%d", n),
			})
		}(i)
	}

	// 50 goroutines each sending 1 AppendEntries request to static-1
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = client.SendAppendEntries(ctx, transport.Addr(), GroupID("static-1"), &AppendEntriesRequest{
				Term:     uint64(n + 1),
				LeaderID: fmt.Sprintf("stress-leader-%d", n),
			})
		}(i)
	}

	// 20 goroutines each registering a unique dynamic handler then unregistering it
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			dynGroup := GroupID(fmt.Sprintf("stress-dyn-%d", n))
			_ = transport.RegisterHandler(dynGroup, &mockIsolationHandler{})
			_, _ = client.SendRequestVote(ctx, transport.Addr(), dynGroup, &RequestVoteRequest{Term: 1})
			transport.UnregisterHandler(dynGroup)
			_, _ = client.SendRequestVote(ctx, transport.Addr(), dynGroup, &RequestVoteRequest{Term: 2})
		}(i)
	}

	wg.Wait()
}

// TestHTTPTransport_StartIdempotent verifies that calling Start() twice returns no error.
func TestHTTPTransport_StartIdempotent(t *testing.T) {
	t.Parallel()

	tr := NewHTTPTransport("127.0.0.1:0")
	if err := tr.Start(); err != nil {
		t.Fatalf("first Start() failed: %v", err)
	}
	defer tr.Close()

	if err := tr.Start(); err != nil {
		t.Fatalf("second Start() should be idempotent, got error: %v", err)
	}
}

// TestHTTPTransport_CloseIdempotent verifies that calling Close() twice returns no error.
func TestHTTPTransport_CloseIdempotent(t *testing.T) {
	t.Parallel()

	tr := NewHTTPTransport("127.0.0.1:0")
	if err := tr.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if err := tr.Close(); err != nil {
		t.Fatalf("first Close() failed: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("second Close() should be idempotent, got error: %v", err)
	}
}

// TestHTTPTransport_StartAfterClose verifies that calling Start() after Close() returns an error.
func TestHTTPTransport_StartAfterClose(t *testing.T) {
	t.Parallel()

	tr := NewHTTPTransport("127.0.0.1:0")
	if err := tr.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	err := tr.Start()
	if err == nil {
		t.Fatal("expected error from Start() after Close(), got nil")
	}
}

// TestHTTPTransport_RegisterAfterClose verifies that registering a handler after Close()
// returns ErrTransportClosed.
func TestHTTPTransport_RegisterAfterClose(t *testing.T) {
	t.Parallel()

	tr := NewHTTPTransport("127.0.0.1:0")
	if err := tr.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	err := tr.RegisterHandler(GroupID("post-close-group"), &mockIsolationHandler{})
	if err == nil {
		t.Fatal("expected ErrTransportClosed when registering after Close(), got nil")
	}
	if !errors.Is(err, ErrTransportClosed) {
		t.Fatalf("expected ErrTransportClosed, got: %v", err)
	}
}

// TestHTTPTransport_CloseIdleConnections verifies that Close() calls CloseIdleConnections()
// on the HTTP client (no panic, no error; subsequent operations on a fresh transport still work).
func TestHTTPTransport_CloseIdleConnections(t *testing.T) {
	t.Parallel()

	// Start a server transport and register a handler.
	server := NewHTTPTransport("127.0.0.1:0")
	h := &mockIsolationHandler{}
	_ = server.RegisterHandler(GroupID("idle-group"), h)
	if err := server.Start(); err != nil {
		t.Fatalf("server Start() failed: %v", err)
	}
	defer server.Close()

	// Use a client transport to make a successful request (creates idle connections).
	client := NewHTTPTransport("127.0.0.1:0")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := client.SendRequestVote(ctx, server.Addr(), GroupID("idle-group"), &RequestVoteRequest{
		Term:        1,
		CandidateID: "idle-test-cand",
	})
	if err != nil {
		t.Fatalf("SendRequestVote failed: %v", err)
	}

	// Close the client transport — this must call CloseIdleConnections() without panicking.
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close() failed: %v", err)
	}
}
