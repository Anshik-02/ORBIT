package tests

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"orbit/internal/raft"
)

// mockRaftHandler implements raft.RPCHandler for transport tests.
type mockRaftHandler struct {
	voteReq   *raft.RequestVoteRequest
	appendReq *raft.AppendEntriesRequest
}

func (m *mockRaftHandler) HandleRequestVote(ctx context.Context, req *raft.RequestVoteRequest) (*raft.RequestVoteResponse, error) {
	m.voteReq = req
	return &raft.RequestVoteResponse{Term: req.Term, VoteGranted: true}, nil
}

func (m *mockRaftHandler) HandleAppendEntries(ctx context.Context, req *raft.AppendEntriesRequest) (*raft.AppendEntriesResponse, error) {
	m.appendReq = req
	return &raft.AppendEntriesResponse{Term: req.Term, Success: true}, nil
}

func (m *mockRaftHandler) HandleInstallSnapshot(ctx context.Context, req *raft.InstallSnapshotRequest) (*raft.InstallSnapshotResponse, error) {
	return &raft.InstallSnapshotResponse{
		Term:           req.Term,
		Success:        true,
		AcceptedOffset: req.Offset + int64(len(req.Data)),
	}, nil
}

func TestHTTPTransport_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	transport := raft.NewHTTPTransport("127.0.0.1:0")
	_ = transport.RegisterHandler(raft.DefaultGroupID, &mockRaftHandler{})
	if err := transport.Start(); err != nil {
		t.Fatalf("start transport failed: %v", err)
	}
	defer transport.Close()

	baseURL := "http://" + transport.Addr()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		for _, endpoint := range []string{"/raft/vote", "/raft/append", "/raft/install-snapshot"} {
			req, err := http.NewRequest(method, baseURL+endpoint, nil)
			if err != nil {
				t.Fatalf("new request failed: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("%s %s got status %d, want %d", method, endpoint, resp.StatusCode, http.StatusMethodNotAllowed)
			}
		}
	}
}

func TestHTTPTransport_MalformedJSON(t *testing.T) {
	t.Parallel()

	transport := raft.NewHTTPTransport("127.0.0.1:0")
	_ = transport.RegisterHandler(raft.DefaultGroupID, &mockRaftHandler{})
	if err := transport.Start(); err != nil {
		t.Fatalf("start transport failed: %v", err)
	}
	defer transport.Close()

	baseURL := "http://" + transport.Addr()

	malformed := []string{
		`{broken json`,
		`not json at all`,
		``,
		`{"term": "not a number"}`,
	}

	for _, payload := range malformed {
		for _, endpoint := range []string{"/raft/vote", "/raft/append"} {
			resp, err := http.Post(baseURL+endpoint, "application/json", strings.NewReader(payload))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s with body %q got status %d, want %d", endpoint, payload, resp.StatusCode, http.StatusBadRequest)
			}
		}
	}
}

func TestHTTPTransport_UnknownFields(t *testing.T) {
	t.Parallel()

	transport := raft.NewHTTPTransport("127.0.0.1:0")
	_ = transport.RegisterHandler(raft.DefaultGroupID, &mockRaftHandler{})
	if err := transport.Start(); err != nil {
		t.Fatalf("start transport failed: %v", err)
	}
	defer transport.Close()

	payload := `{"term": 1, "candidateId": "node-1", "unexpectedField": "evil"}`
	resp, err := http.Post("http://"+transport.Addr()+"/raft/vote", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for unknown fields, got %d", resp.StatusCode)
	}
}

func TestHTTPTransport_TrailingGarbage(t *testing.T) {
	t.Parallel()

	transport := raft.NewHTTPTransport("127.0.0.1:0")
	_ = transport.RegisterHandler(raft.DefaultGroupID, &mockRaftHandler{})
	if err := transport.Start(); err != nil {
		t.Fatalf("start transport failed: %v", err)
	}
	defer transport.Close()

	payload := `{"term": 1, "candidateId": "node-1"} {"extra": 123}`
	resp, err := http.Post("http://"+transport.Addr()+"/raft/vote", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for trailing tokens, got %d", resp.StatusCode)
	}
}

func TestHTTPTransport_OversizedPayload(t *testing.T) {
	t.Parallel()

	transport := raft.NewHTTPTransport("127.0.0.1:0")
	_ = transport.RegisterHandler(raft.DefaultGroupID, &mockRaftHandler{})
	if err := transport.Start(); err != nil {
		t.Fatalf("start transport failed: %v", err)
	}
	defer transport.Close()

	// Create a payload larger than 4MB (e.g. 5MB)
	hugeString := strings.Repeat("A", 5*1024*1024)
	hugeJSON := `{"term": 1, "candidateId": "` + hugeString + `"}`

	resp, err := http.Post("http://"+transport.Addr()+"/raft/vote", "application/json", strings.NewReader(hugeJSON))
	if err != nil {
		// Connection reset or EOF on early abort is also acceptable
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 413 Request Entity Too Large or 400, got %d", resp.StatusCode)
	}
}

func TestHTTPTransport_HandlerNotRegistered(t *testing.T) {
	t.Parallel()

	transport := raft.NewHTTPTransport("127.0.0.1:0")
	// Start without registering handler
	if err := transport.Start(); err != nil {
		t.Fatalf("start transport failed: %v", err)
	}
	defer transport.Close()

	payload := `{"term": 1, "candidateId": "node-1"}`
	resp, err := http.Post("http://"+transport.Addr()+"/raft/vote", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 Service Unavailable, got %d", resp.StatusCode)
	}
}

func TestHTTPTransport_CanceledContext(t *testing.T) {
	t.Parallel()

	transport := raft.NewHTTPTransport("127.0.0.1:0")
	slowHandler := &slowRaftHandler{delay: 600 * time.Millisecond}
	_ = transport.RegisterHandler(raft.DefaultGroupID, slowHandler)
	if err := transport.Start(); err != nil {
		t.Fatalf("start transport failed: %v", err)
	}
	defer transport.Close()

	clientTransport := raft.NewHTTPTransport("127.0.0.1:0")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := clientTransport.SendRequestVote(ctx, transport.Addr(), raft.DefaultGroupID, &raft.RequestVoteRequest{
		Term:        1,
		CandidateID: "candidate",
	})
	duration := time.Since(start)

	if err == nil {
		t.Fatalf("expected error due to canceled context, got nil")
	}
	if duration >= 400*time.Millisecond {
		t.Errorf("expected fast cancelation, but took %v", duration)
	}
}

type slowRaftHandler struct {
	delay time.Duration
}

func (s *slowRaftHandler) HandleRequestVote(ctx context.Context, req *raft.RequestVoteRequest) (*raft.RequestVoteResponse, error) {
	select {
	case <-time.After(s.delay):
		return &raft.RequestVoteResponse{Term: req.Term, VoteGranted: true}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *slowRaftHandler) HandleAppendEntries(ctx context.Context, req *raft.AppendEntriesRequest) (*raft.AppendEntriesResponse, error) {
	select {
	case <-time.After(s.delay):
		return &raft.AppendEntriesResponse{Term: req.Term, Success: true}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *slowRaftHandler) HandleInstallSnapshot(ctx context.Context, req *raft.InstallSnapshotRequest) (*raft.InstallSnapshotResponse, error) {
	select {
	case <-time.After(s.delay):
		return &raft.InstallSnapshotResponse{
			Term:           req.Term,
			Success:        true,
			AcceptedOffset: req.Offset + int64(len(req.Data)),
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestHTTPTransport_MultiGroupRouting(t *testing.T) {
	t.Parallel()

	transport := raft.NewHTTPTransport("127.0.0.1:0")
	h0 := &mockRaftHandler{}
	h1 := &mockRaftHandler{}

	if err := transport.RegisterHandler(raft.GroupID("shard-0"), h0); err != nil {
		t.Fatalf("register shard-0: %v", err)
	}
	if err := transport.RegisterHandler(raft.GroupID("shard-1"), h1); err != nil {
		t.Fatalf("register shard-1: %v", err)
	}
	if err := transport.Start(); err != nil {
		t.Fatalf("start transport: %v", err)
	}
	defer transport.Close()

	client := raft.NewHTTPTransport("127.0.0.1:0")

	// 1. Send to shard-0
	ctx := context.Background()
	_, err := client.SendRequestVote(ctx, transport.Addr(), raft.GroupID("shard-0"), &raft.RequestVoteRequest{
		Term:        10,
		CandidateID: "candidate-a",
	})
	if err != nil {
		t.Fatalf("send vote to shard-0: %v", err)
	}
	if h0.voteReq == nil || h0.voteReq.Term != 10 {
		t.Errorf("shard-0 handler did not receive expected vote request: %+v", h0.voteReq)
	}
	if h1.voteReq != nil {
		t.Errorf("shard-1 handler should not have received vote request, got: %+v", h1.voteReq)
	}

	// 2. Send to shard-1
	_, err = client.SendAppendEntries(ctx, transport.Addr(), raft.GroupID("shard-1"), &raft.AppendEntriesRequest{
		Term:     15,
		LeaderID: "leader-b",
	})
	if err != nil {
		t.Fatalf("send append to shard-1: %v", err)
	}
	if h1.appendReq == nil || h1.appendReq.Term != 15 {
		t.Errorf("shard-1 handler did not receive expected append request: %+v", h1.appendReq)
	}

	// 3. Send to non-existent shard-99 -> should fail with ErrGroupNotFound
	_, err = client.SendRequestVote(ctx, transport.Addr(), raft.GroupID("shard-99"), &raft.RequestVoteRequest{
		Term: 20,
	})
	if err == nil {
		t.Fatal("expected error for non-existent group shard-99, got nil")
	}

	// 4. Unregister shard-0, future requests should fail with ErrGroupNotFound
	transport.UnregisterHandler(raft.GroupID("shard-0"))
	_, err = client.SendRequestVote(ctx, transport.Addr(), raft.GroupID("shard-0"), &raft.RequestVoteRequest{
		Term: 30,
	})
	if err == nil {
		t.Fatal("expected error for unregistered group shard-0, got nil")
	}
}

func TestHTTPTransport_DuplicateGroupRegistrationRejected(t *testing.T) {
	t.Parallel()

	transport := raft.NewHTTPTransport("127.0.0.1:0")
	if err := transport.RegisterHandler(raft.GroupID("shard-0"), &mockRaftHandler{}); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	// Second registration of same group ID must fail
	if err := transport.RegisterHandler(raft.GroupID("shard-0"), &mockRaftHandler{}); err == nil {
		t.Fatal("expected error registering duplicate group ID, got nil")
	}
}
