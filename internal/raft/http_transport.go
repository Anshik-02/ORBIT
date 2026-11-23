package raft

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

const maxRPCRequestBodyBytes = 4 * 1024 * 1024 // 4MB maximum Raft RPC payload size

// HTTPTransport implements inter-node Raft communication over HTTP/JSON.
type HTTPTransport struct {
	mu       sync.RWMutex
	addr     string
	handler  RPCHandler
	server   *http.Server
	client   *http.Client
	listener net.Listener
	closed   bool
}

// NewHTTPTransport creates an HTTP-based transport listening on addr.
func NewHTTPTransport(addr string) *HTTPTransport {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   200 * time.Millisecond,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &HTTPTransport{
		addr: addr,
		client: &http.Client{
			Transport: transport,
			Timeout:   500 * time.Millisecond,
		},
	}
}

func (t *HTTPTransport) RegisterHandler(handler RPCHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handler = handler
}

func (t *HTTPTransport) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/raft/vote", t.serveRequestVote)
	mux.HandleFunc("/raft/append", t.serveAppendEntries)

	ln, err := net.Listen("tcp", t.addr)
	if err != nil {
		return fmt.Errorf("http transport listen on %s: %w", t.addr, err)
	}
	t.listener = ln
	t.addr = ln.Addr().String()

	t.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		if err := t.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// server closed
		}
	}()

	return nil
}

// Addr returns the bound network address.
func (t *HTTPTransport) Addr() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.addr
}

func (t *HTTPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	if t.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = t.server.Shutdown(ctx)
	}
	return nil
}

func (t *HTTPTransport) serveRequestVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	t.mu.RLock()
	h := t.handler
	t.mu.RUnlock()

	if h == nil {
		http.Error(w, "raft handler not registered", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRPCRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req RequestVoteRequest
	if err := dec.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request payload exceeds size limit", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		http.Error(w, "unexpected trailing content after json", http.StatusBadRequest)
		return
	}

	resp, err := h.HandleRequestVote(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (t *HTTPTransport) serveAppendEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	t.mu.RLock()
	h := t.handler
	t.mu.RUnlock()

	if h == nil {
		http.Error(w, "raft handler not registered", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRPCRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req AppendEntriesRequest
	if err := dec.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request payload exceeds size limit", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		http.Error(w, "unexpected trailing content after json", http.StatusBadRequest)
		return
	}

	resp, err := h.HandleAppendEntries(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (t *HTTPTransport) SendRequestVote(ctx context.Context, target string, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	url := fmt.Sprintf("http://%s/raft/vote", target)
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal vote request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send vote to %s: %w", target, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vote request returned status %d", httpResp.StatusCode)
	}

	var resp RequestVoteResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode vote response: %w", err)
	}

	return &resp, nil
}

func (t *HTTPTransport) SendAppendEntries(ctx context.Context, target string, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	url := fmt.Sprintf("http://%s/raft/append", target)
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal append request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send append to %s: %w", target, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("append request returned status %d", httpResp.StatusCode)
	}

	var resp AppendEntriesResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode append response: %w", err)
	}

	return &resp, nil
}
