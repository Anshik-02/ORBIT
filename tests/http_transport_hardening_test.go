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

func TestHTTPTransport_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	transport := raft.NewHTTPTransport("127.0.0.1:0")
	transport.RegisterHandler(&mockRaftHandler{})
	if err := transport.Start(); err != nil {
		t.Fatalf("start transport failed: %v", err)
	}
	defer transport.Close()

	baseURL := "http://" + transport.Addr()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		for _, endpoint := range []string{"/raft/vote", "/raft/append"} {
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
	transport.RegisterHandler(&mockRaftHandler{})
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
	transport.RegisterHandler(&mockRaftHandler{})
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
	transport.RegisterHandler(&mockRaftHandler{})
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
	transport.RegisterHandler(&mockRaftHandler{})
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
	transport.RegisterHandler(slowHandler)
	if err := transport.Start(); err != nil {
		t.Fatalf("start transport failed: %v", err)
	}
	defer transport.Close()

	clientTransport := raft.NewHTTPTransport("127.0.0.1:0")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := clientTransport.SendRequestVote(ctx, transport.Addr(), &raft.RequestVoteRequest{
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
