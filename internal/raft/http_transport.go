package raft

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxRPCRequestBodyBytes = 4 * 1024 * 1024 // 4MB maximum Raft RPC payload size

// HTTPTransport implements inter-node Raft communication over HTTP/JSON with multi-group support.
type HTTPTransport struct {
	mu            sync.RWMutex
	addr          string
	handlers      map[GroupID]RPCHandler
	server        *http.Server
	client        *http.Client
	listener      net.Listener
	closed        bool
	tlsConfig     *tls.Config
	clusterSecret string
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
		addr:     addr,
		handlers: make(map[GroupID]RPCHandler),
		client: &http.Client{
			Transport: transport,
			Timeout:   500 * time.Millisecond,
		},
	}
}

// SetTLSConfig configures TLS/mTLS for both server listening and client outbound connections.
func (t *HTTPTransport) SetTLSConfig(cfg *tls.Config) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tlsConfig = cfg
	if tr, ok := t.client.Transport.(*http.Transport); ok {
		tr.TLSClientConfig = cfg
	}
}

// SetClusterSecret configures an internal shared secret header required on all Raft RPCs.
func (t *HTTPTransport) SetClusterSecret(secret string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clusterSecret = secret
}

// RegisterHandler registers an RPCHandler for a specific consensus group.
func (t *HTTPTransport) RegisterHandler(groupID GroupID, handler RPCHandler) error {
	if handler == nil {
		return ErrNilHandler
	}
	if strings.TrimSpace(string(groupID)) == "" {
		return ErrInvalidGroupID
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrTransportClosed
	}
	if t.handlers == nil {
		t.handlers = make(map[GroupID]RPCHandler)
	}
	if _, exists := t.handlers[groupID]; exists {
		return fmt.Errorf("%w: group %q", ErrDuplicateGroup, groupID)
	}
	t.handlers[groupID] = handler
	return nil
}

// UnregisterHandler removes an RPCHandler for a specific consensus group.
func (t *HTTPTransport) UnregisterHandler(groupID GroupID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if groupID == "" {
		groupID = DefaultGroupID
	}
	delete(t.handlers, groupID)
}

func (t *HTTPTransport) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return errors.New("http transport is closed")
	}
	if t.server != nil || t.listener != nil {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/raft/vote", t.serveRequestVote)
	mux.HandleFunc("/raft/append", t.serveAppendEntries)
	mux.HandleFunc("/raft/install-snapshot", t.serveInstallSnapshot)

	ln, err := net.Listen("tcp", t.addr)
	if err != nil {
		return fmt.Errorf("http transport listen on %s: %w", t.addr, err)
	}
	if t.tlsConfig != nil {
		ln = tls.NewListener(ln, t.tlsConfig)
	}
	t.listener = ln
	t.addr = ln.Addr().String()

	t.server = &http.Server{
		Handler:           mux,
		TLSConfig:         t.tlsConfig,
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

func (t *HTTPTransport) authenticateRequest(w http.ResponseWriter, r *http.Request) bool {
	t.mu.RLock()
	secret := t.clusterSecret
	tlsCfg := t.tlsConfig
	t.mu.RUnlock()

	// Check mutual TLS if enforced
	if tlsCfg != nil && tlsCfg.ClientAuth == tls.RequireAndVerifyClientCert {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "client certificate required", http.StatusUnauthorized)
			return false
		}
	}

	// Check cluster secret if configured
	if secret != "" {
		incoming := r.Header.Get("X-Orbit-Cluster-Secret")
		if subtle.ConstantTimeCompare([]byte(incoming), []byte(secret)) != 1 {
			http.Error(w, "invalid cluster secret", http.StatusUnauthorized)
			return false
		}
	}
	return true
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
	t.client.CloseIdleConnections()
	return nil
}

func (t *HTTPTransport) serveRequestVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !t.authenticateRequest(w, r) {
		return
	}

	t.mu.RLock()
	handlersCount := len(t.handlers)
	t.mu.RUnlock()

	if handlersCount == 0 {
		http.Error(w, "no raft handlers registered", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRPCRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var envelope GroupRequestVoteRequest
	if err := dec.Decode(&envelope); err != nil {
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

	targetGroup := envelope.GroupID
	if targetGroup == "" {
		targetGroup = DefaultGroupID
	}

	t.mu.RLock()
	h, exists := t.handlers[targetGroup]
	t.mu.RUnlock()

	if !exists || h == nil {
		http.Error(w, fmt.Sprintf("raft group %q not found on this node", targetGroup), http.StatusNotFound)
		return
	}

	resp, err := h.HandleRequestVote(r.Context(), &envelope.Request)
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
	if !t.authenticateRequest(w, r) {
		return
	}

	t.mu.RLock()
	handlersCount := len(t.handlers)
	t.mu.RUnlock()

	if handlersCount == 0 {
		http.Error(w, "no raft handlers registered", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRPCRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var envelope GroupAppendEntriesRequest
	if err := dec.Decode(&envelope); err != nil {
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

	targetGroup := envelope.GroupID
	if targetGroup == "" {
		targetGroup = DefaultGroupID
	}

	t.mu.RLock()
	h, exists := t.handlers[targetGroup]
	t.mu.RUnlock()

	if !exists || h == nil {
		http.Error(w, fmt.Sprintf("raft group %q not found on this node", targetGroup), http.StatusNotFound)
		return
	}

	resp, err := h.HandleAppendEntries(r.Context(), &envelope.Request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (t *HTTPTransport) SendRequestVote(ctx context.Context, target string, groupID GroupID, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	if groupID == "" {
		groupID = DefaultGroupID
	}
	scheme := "http"
	t.mu.RLock()
	if t.tlsConfig != nil {
		scheme = "https"
	}
	secret := t.clusterSecret
	t.mu.RUnlock()

	url := fmt.Sprintf("%s://%s/raft/vote", scheme, target)
	envelope := GroupRequestVoteRequest{
		GroupID: groupID,
		Request: *req,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal vote request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if secret != "" {
		httpReq.Header.Set("X-Orbit-Cluster-Secret", secret)
	}

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send vote to %s: %w", target, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: group %s on target %s", ErrGroupNotFound, groupID, target)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vote request returned status %d", httpResp.StatusCode)
	}

	var resp RequestVoteResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode vote response: %w", err)
	}

	return &resp, nil
}

func (t *HTTPTransport) SendAppendEntries(ctx context.Context, target string, groupID GroupID, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	if groupID == "" {
		groupID = DefaultGroupID
	}
	scheme := "http"
	t.mu.RLock()
	if t.tlsConfig != nil {
		scheme = "https"
	}
	secret := t.clusterSecret
	t.mu.RUnlock()

	url := fmt.Sprintf("%s://%s/raft/append", scheme, target)
	envelope := GroupAppendEntriesRequest{
		GroupID: groupID,
		Request: *req,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal append request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if secret != "" {
		httpReq.Header.Set("X-Orbit-Cluster-Secret", secret)
	}

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send append to %s: %w", target, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: group %s on target %s", ErrGroupNotFound, groupID, target)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("append request returned status %d", httpResp.StatusCode)
	}

	var resp AppendEntriesResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode append response: %w", err)
	}

	return &resp, nil
}

func (t *HTTPTransport) serveInstallSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !t.authenticateRequest(w, r) {
		return
	}

	t.mu.RLock()
	handlersCount := len(t.handlers)
	t.mu.RUnlock()

	if handlersCount == 0 {
		http.Error(w, "no raft handlers registered", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRPCRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var envelope GroupInstallSnapshotRequest
	if err := dec.Decode(&envelope); err != nil {
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

	targetGroup := envelope.GroupID
	if targetGroup == "" {
		targetGroup = DefaultGroupID
	}

	t.mu.RLock()
	h, exists := t.handlers[targetGroup]
	t.mu.RUnlock()

	if !exists || h == nil {
		http.Error(w, fmt.Sprintf("raft group %q not found on this node", targetGroup), http.StatusNotFound)
		return
	}

	resp, err := h.HandleInstallSnapshot(r.Context(), &envelope.Request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (t *HTTPTransport) SendInstallSnapshot(ctx context.Context, target string, groupID GroupID, req *InstallSnapshotRequest) (*InstallSnapshotResponse, error) {
	if groupID == "" {
		groupID = DefaultGroupID
	}
	scheme := "http"
	t.mu.RLock()
	if t.tlsConfig != nil {
		scheme = "https"
	}
	secret := t.clusterSecret
	t.mu.RUnlock()

	url := fmt.Sprintf("%s://%s/raft/install-snapshot", scheme, target)
	envelope := GroupInstallSnapshotRequest{
		GroupID: groupID,
		Request: *req,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal install snapshot request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if secret != "" {
		httpReq.Header.Set("X-Orbit-Cluster-Secret", secret)
	}

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send install snapshot to %s: %w", target, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: group %s on target %s", ErrGroupNotFound, groupID, target)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("install snapshot request returned status %d", httpResp.StatusCode)
	}

	var resp InstallSnapshotResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode install snapshot response: %w", err)
	}

	return &resp, nil
}
