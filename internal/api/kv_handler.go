package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"orbit/internal/database"
	"orbit/internal/observability"
	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
	"orbit/internal/storage"
)

const maxRequestBodySize = 1 << 20 // 1 MB

// ShardProvider exposes locally hosted shard replicas to the KV API.
type ShardProvider interface {
	ShardInstances() map[sharding.ShardID]router.ShardInstance
}

// ErrorResponseWithReq represents a standardized JSON error response including request ID and error code.
type ErrorResponseWithReq struct {
	Code      string `json:"code,omitempty"`
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

// IdempotencyCache caches completed mutation responses by idempotency key.
type idempotencyRecord struct {
	statusCode int
	body       []byte
	timestamp  time.Time
}

type IdempotencyCache struct {
	mu      sync.Mutex
	entries map[string]idempotencyRecord
}

func NewIdempotencyCache() *IdempotencyCache {
	return &IdempotencyCache{
		entries: make(map[string]idempotencyRecord),
	}
}

func (c *IdempotencyCache) Get(key string) (idempotencyRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rec, ok := c.entries[key]
	if !ok {
		return idempotencyRecord{}, false
	}
	if time.Since(rec.timestamp) > 10*time.Minute {
		delete(c.entries, key)
		return idempotencyRecord{}, false
	}
	return rec, true
}

func (c *IdempotencyCache) Set(key string, code int, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = idempotencyRecord{
		statusCode: code,
		body:       body,
		timestamp:  time.Now(),
	}
}

// BatchWriteOp describes a single mutation inside a batch request.
type BatchWriteOp struct {
	Operation string  `json:"operation"` // "SET" or "DELETE"
	Key       string  `json:"key"`
	Value     *string `json:"value,omitempty"`
}

// BatchWriteRequest is the JSON payload for POST /kv/batch write operations.
type BatchWriteRequest struct {
	Ops []BatchWriteOp `json:"ops"`
}

// BatchOpResult reports the per-key execution outcome of a batch operation.
type BatchOpResult struct {
	Key    string `json:"key"`
	Status string `json:"status"` // "COMMITTED", "NOT_FOUND", "ERROR"
	Error  string `json:"error,omitempty"`
}

// BatchWriteResponse is the JSON response for POST /kv/batch write operations.
type BatchWriteResponse struct {
	Results   []BatchOpResult `json:"results"`
	RequestID string          `json:"request_id"`
}

// BatchReadRequest is the JSON payload for batch read requests.
type BatchReadRequest struct {
	Keys []string `json:"keys"`
}

// BatchReadResult reports the lookup outcome for a single key in a batch read.
type BatchReadResult struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
	Found bool   `json:"found"`
	Error string `json:"error,omitempty"`
}

// BatchReadResponse is the JSON response for batch read requests.
type BatchReadResponse struct {
	Results   []BatchReadResult `json:"results"`
	RequestID string            `json:"request_id"`
}

// ScanResponse is the paginated response for /kv/scan and /kv/prefix queries.
type ScanResponse struct {
	Items      []storage.KVPair `json:"items"`
	HasMore    bool             `json:"has_more"`
	NextCursor string           `json:"next_cursor,omitempty"`
	RequestID  string           `json:"request_id"`
}

// SetKVRequest is the JSON payload for PUT /kv/:key.
// Value is a pointer so we can distinguish between an omitted field and an explicit empty string.
type SetKVRequest struct {
	Value *string `json:"value"`
}

// SetKVResponse is the JSON response for PUT /kv/:key.
type SetKVResponse struct {
	Key     string           `json:"key"`
	Value   string           `json:"value"`
	ShardID sharding.ShardID `json:"shard_id"`
	Status  string           `json:"status"`
}

// GetKVResponse is the JSON response for GET /kv/:key.
type GetKVResponse struct {
	Key     string           `json:"key"`
	Value   string           `json:"value"`
	ShardID sharding.ShardID `json:"shard_id"`
}

// DeleteKVResponse is the JSON response for DELETE /kv/:key.
type DeleteKVResponse struct {
	Key     string           `json:"key"`
	Deleted bool             `json:"deleted"`
	ShardID sharding.ShardID `json:"shard_id"`
}

// ClusterStatusResponse reports cluster-wide metadata and physical nodes.
type ClusterStatusResponse struct {
	ClusterID  string   `json:"cluster_id"`
	Epoch      uint64   `json:"epoch"`
	ShardCount int      `json:"shard_count"`
	NodeCount  int      `json:"node_count"`
	Nodes      []string `json:"nodes"`
}

// ShardDetail reports shard ID, replica node IDs, and known leader.
type ShardDetail struct {
	ID       sharding.ShardID `json:"id"`
	Replicas []string         `json:"replicas"`
	Leader   string           `json:"leader,omitempty"`
}

// ClusterShardsResponse returns all shard definitions in the cluster.
type ClusterShardsResponse struct {
	Shards []ShardDetail `json:"shards"`
}

// ClusterNodesResponse returns all physical node IDs in the cluster.
type ClusterNodesResponse struct {
	Nodes []string `json:"nodes"`
}

// KVHandler handles client HTTP requests for key-value operations and cluster discovery.
type KVHandler struct {
	provider    ShardProvider
	metadata    *sharding.ClusterMetadata
	forwarder   *router.RequestForwarder
	localNodeID string
	timeout     time.Duration
	monitor     *observability.ClusterMonitor
	eventHub    *observability.EventHub
	wsHub       *observability.WebSocketHub
	eventBus    *observability.DistributedEventBus
	idempotency *IdempotencyCache
}

// NewKVHandler constructs a new KVHandler.
func NewKVHandler(
	provider ShardProvider,
	metadata *sharding.ClusterMetadata,
	forwarder *router.RequestForwarder,
	localNodeID string,
) *KVHandler {
	monitor := observability.NewClusterMonitor(metadata, forwarder.Resolver().Tracker())
	eventHub := observability.NewEventHub()
	wsHub := observability.NewWebSocketHub(eventHub)

	return &KVHandler{
		provider:    provider,
		metadata:    metadata,
		forwarder:   forwarder,
		localNodeID: localNodeID,
		timeout:     5 * time.Second,
		monitor:     monitor,
		eventHub:    eventHub,
		wsHub:       wsHub,
		idempotency: NewIdempotencyCache(),
	}
}

// EventHub returns the internal EventHub.
func (h *KVHandler) EventHub() *observability.EventHub {
	return h.eventHub
}

// WebSocketHub returns the WebSocketHub streaming live cluster events.
func (h *KVHandler) WebSocketHub() *observability.WebSocketHub {
	return h.wsHub
}

// ClusterMonitor returns the real-time ClusterMonitor.
func (h *KVHandler) ClusterMonitor() *observability.ClusterMonitor {
	return h.monitor
}

// Forwarder returns the request forwarder.
func (h *KVHandler) Forwarder() *router.RequestForwarder {
	return h.forwarder
}

// EventBus returns the DistributedEventBus.
func (h *KVHandler) EventBus() *observability.DistributedEventBus {
	return h.eventBus
}

// SetEventBus configures the DistributedEventBus.
func (h *KVHandler) SetEventBus(bus *observability.DistributedEventBus) {
	h.eventBus = bus
}

// BroadcastEvent publishes an event locally and relays it across the distributed cluster.
func (h *KVHandler) BroadcastEvent(evt observability.ClusterEvent) {
	if h.eventBus != nil {
		h.eventBus.Broadcast(evt)
	} else if h.eventHub != nil {
		h.eventHub.Publish(evt)
	}
}

// SetTimeout configures the request processing and forwarding timeout.
func (h *KVHandler) SetTimeout(d time.Duration) {
	h.timeout = d
}

// RegisterKVRoutes registers the shard-aware KV API routes on the provided mux.
func RegisterKVRoutes(mux *http.ServeMux, h *KVHandler) {
	mux.HandleFunc("/kv/", h.handleKV)
	mux.HandleFunc("/kv/batch", func(w http.ResponseWriter, r *http.Request) {
		h.handleBatch(w, r, getOrGenerateReqID(r))
	})
	mux.HandleFunc("/kv/scan", func(w http.ResponseWriter, r *http.Request) {
		h.handleScan(w, r, getOrGenerateReqID(r))
	})
	mux.HandleFunc("/kv/prefix", func(w http.ResponseWriter, r *http.Request) {
		h.handlePrefixScan(w, r, getOrGenerateReqID(r))
	})
	mux.HandleFunc("/cluster/status", h.handleClusterStatus)
	mux.HandleFunc("/cluster/health", h.handleClusterHealth)
	mux.HandleFunc("/cluster/shards", h.handleClusterShards)
	mux.HandleFunc("/cluster/nodes", h.handleClusterNodes)

	// Versioned v1 API routes (/api/v1/...)
	mux.HandleFunc("/api/v1/kv/", h.handleKV)
	mux.HandleFunc("/api/v1/kv/batch", func(w http.ResponseWriter, r *http.Request) {
		h.handleBatch(w, r, getOrGenerateReqID(r))
	})
	mux.HandleFunc("/api/v1/kv/scan", func(w http.ResponseWriter, r *http.Request) {
		h.handleScan(w, r, getOrGenerateReqID(r))
	})
	mux.HandleFunc("/api/v1/kv/prefix", func(w http.ResponseWriter, r *http.Request) {
		h.handlePrefixScan(w, r, getOrGenerateReqID(r))
	})
	mux.HandleFunc("/api/v1/cluster/status", h.handleClusterStatus)
	mux.HandleFunc("/api/v1/cluster/health", h.handleClusterHealth)
	mux.HandleFunc("/api/v1/cluster/shards", h.handleClusterShards)
	mux.HandleFunc("/api/v1/cluster/nodes", h.handleClusterNodes)

	if h.wsHub != nil {
		h.wsHub.RegisterRoutes(mux, "/ws/events")
		h.wsHub.RegisterRoutes(mux, "/cluster/events")
		h.wsHub.RegisterRoutes(mux, "/api/v1/ws/events")
	}

	if h.eventBus != nil {
		mux.HandleFunc("/cluster/events/relay", h.eventBus.HandleRelayHTTP)
	}
}

func getOrGenerateReqID(r *http.Request) string {
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		reqID = hex.EncodeToString(b)
	}
	return reqID
}

func writeJSONErrorWithReq(w http.ResponseWriter, status int, msg, reqID string) {
	w.Header().Set("Content-Type", "application/json")
	if reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponseWithReq{
		Code:      http.StatusText(status),
		Error:     msg,
		RequestID: reqID,
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSONErrorWithReq(w, status, msg, "")
}

func (h *KVHandler) handleKV(w http.ResponseWriter, r *http.Request) {
	reqID := getOrGenerateReqID(r)
	w.Header().Set("X-Request-ID", reqID)

	rawKey := r.URL.Path
	if strings.HasPrefix(rawKey, "/api/v1/kv/") {
		rawKey = strings.TrimPrefix(rawKey, "/api/v1/kv/")
	} else if strings.HasPrefix(rawKey, "/kv/") {
		rawKey = strings.TrimPrefix(rawKey, "/kv/")
	}
	rawKey = strings.TrimPrefix(rawKey, "/")
	key, err := url.PathUnescape(rawKey)
	if err != nil || strings.Contains(key, "\x00") {
		writeJSONErrorWithReq(w, http.StatusBadRequest, "key is invalid", reqID)
		return
	}

	if key == "batch" {
		h.handleBatch(w, r, reqID)
		return
	}
	if key == "scan" {
		h.handleScan(w, r, reqID)
		return
	}
	if key == "prefix" {
		h.handlePrefixScan(w, r, reqID)
		return
	}
	if key == "" {
		writeJSONErrorWithReq(w, http.StatusBadRequest, "key cannot be empty", reqID)
		return
	}
	if strings.Contains(key, "..") {
		writeJSONErrorWithReq(w, http.StatusBadRequest, "path traversal sequences not allowed in key", reqID)
		return
	}

	hops := 0
	if hopsStr := r.Header.Get(router.HeaderForwardHops); hopsStr != "" {
		if val, err := strconv.Atoi(hopsStr); err == nil {
			hops = val
		}
	}
	if hops >= router.MaxForwardHops {
		writeJSONErrorWithReq(w, http.StatusLoopDetected, "forwarding loop detected", reqID)
		return
	}

	switch r.Method {
	case http.MethodPut:
		h.handlePut(w, r, key, reqID, hops)
	case http.MethodGet:
		h.handleGet(w, r, key, reqID, hops)
	case http.MethodDelete:
		h.handleDelete(w, r, key, reqID, hops)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeJSONErrorWithReq(w, http.StatusMethodNotAllowed, fmt.Sprintf("method %s not allowed", r.Method), reqID)
	}
}

func (h *KVHandler) handlePut(w http.ResponseWriter, r *http.Request, key, reqID string, hops int) {
	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = r.Header.Get("Idempotency-Key")
	}
	if idempotencyKey != "" {
		if rec, found := h.idempotency.Get(idempotencyKey); found {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-ID", reqID)
			w.Header().Set("X-Idempotent-Replay", "true")
			w.WriteHeader(rec.statusCode)
			_, _ = w.Write(rec.body)
			return
		}
	}

	// 1. Content-Type validation
	ct := r.Header.Get("Content-Type")
	if ct != "" {
		mediaType, _, _ := mime.ParseMediaType(ct)
		if mediaType != "application/json" {
			writeJSONErrorWithReq(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json", reqID)
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSONErrorWithReq(w, http.StatusRequestEntityTooLarge, "request body too large", reqID)
			return
		}
		writeJSONErrorWithReq(w, http.StatusBadRequest, "failed to read request body", reqID)
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		writeJSONErrorWithReq(w, http.StatusBadRequest, "request body cannot be empty", reqID)
		return
	}

	var req SetKVRequest
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSONErrorWithReq(w, http.StatusBadRequest, fmt.Sprintf("invalid json body: %v", err), reqID)
		return
	}

	// Second decode to ensure no trailing JSON tokens
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeJSONErrorWithReq(w, http.StatusBadRequest, "unexpected trailing content after json body", reqID)
		return
	}

	if req.Value == nil {
		writeJSONErrorWithReq(w, http.StatusBadRequest, "field 'value' is required", reqID)
		return
	}
	val := *req.Value

	route, err := h.forwarder.Route(key, h.provider.ShardInstances())
	if err != nil {
		writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("failed to route key: %v", err), reqID)
		return
	}

	// 1. If routed locally and we are the leader, propose via Raft
	if route.IsLocal && route.Replica != nil && route.Replica.Raft().Role() == raft.Leader {
		if route.Replica.DB().IsClosed() {
			writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "database is shut down", reqID)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		defer cancel()

		idx, _, err := route.Replica.Raft().Propose("SET", key, val)
		if err != nil {
			if errors.Is(err, raft.ErrNotLeader) {
				h.forwarder.Resolver().Tracker().Invalidate(route.GroupID)
				writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "replica is no longer leader", reqID)
				return
			}
			if errors.Is(err, raft.ErrStopped) {
				writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "replica consensus is stopped", reqID)
				return
			}
			writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("failed to propose write: %v", err), reqID)
			return
		}

		if err := route.Replica.Raft().WaitForApply(ctx, idx); err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "write timed out waiting for consensus quorum", reqID)
				return
			}
			writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("failed to commit write: %v", err), reqID)
			return
		}

		h.BroadcastEvent(observability.NewClusterEvent(
			observability.EventEntryCommitted,
			h.localNodeID,
			&route.ShardID,
			map[string]any{
				"key":       key,
				"index":     idx,
				"operation": "SET",
			},
		))

		respObj := SetKVResponse{
			Key:     key,
			Value:   val,
			ShardID: route.ShardID,
			Status:  "ok",
		}
		if idempotencyKey != "" {
			if b, mErr := json.Marshal(respObj); mErr == nil {
				h.idempotency.Set(idempotencyKey, http.StatusOK, b)
			}
		}
		writeJSON(w, http.StatusOK, respObj)
		return
	}

	// 2. Otherwise forward to target leader or candidate replicas with failover
	candidates := route.Candidates(h.localNodeID)
	if len(candidates) == 0 {
		writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "no leader or replica currently available for shard", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := h.forwardWithFailover(ctx, route, candidates, http.MethodPut, r.URL.Path, body, hops, reqID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "ambiguous") {
			writeJSONErrorWithReq(w, http.StatusGatewayTimeout, "forwarded write timed out with ambiguous outcome; check key status before retrying", reqID)
			return
		}
		writeJSONErrorWithReq(w, http.StatusBadGateway, fmt.Sprintf("forwarding failed: %v", err), reqID)
		return
	}
	defer resp.Body.Close()

	h.pipeResponse(w, resp)
}

func (h *KVHandler) handleGet(w http.ResponseWriter, r *http.Request, key, reqID string, hops int) {
	route, err := h.forwarder.Route(key, h.provider.ShardInstances())
	if err != nil {
		writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("failed to route key: %v", err), reqID)
		return
	}

	// If local replica is leader for this shard, execute a linearizable read via Raft ReadIndex
	if route.IsLocal && route.Replica != nil && route.Replica.Raft().Role() == raft.Leader {
		if route.Replica.DB().IsClosed() {
			writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "database is shut down", reqID)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		defer cancel()

		// Step 1: Quorum check via ReadIndex
		readIndex, err := route.Replica.Raft().ReadIndex(ctx)
		if err != nil {
			if errors.Is(err, raft.ErrNotLeader) {
				h.forwarder.Resolver().Tracker().Invalidate(route.GroupID)
				writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "replica is no longer leader", reqID)
				return
			}
			writeJSONErrorWithReq(w, http.StatusServiceUnavailable, fmt.Sprintf("linearizable read failed: %v", err), reqID)
			return
		}

		// Step 2: Wait until state machine has applied through confirmed readIndex
		if err := route.Replica.Raft().WaitForApply(ctx, readIndex); err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "read timed out waiting for consensus quorum", reqID)
				return
			}
			writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("failed to wait for apply index %d: %v", readIndex, err), reqID)
			return
		}

		// Step 3: Read linearizable state from local DB, distinguishing key-not-found from storage failure
		val, err := route.Replica.DB().Get(key)
		if err != nil {
			if errors.Is(err, database.ErrKeyNotFound) || err.Error() == "key not found" {
				writeJSONErrorWithReq(w, http.StatusNotFound, "key not found", reqID)
				return
			}
			if errors.Is(err, database.ErrClosed) {
				writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "database is shut down", reqID)
				return
			}
			writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("storage failure: %v", err), reqID)
			return
		}

		writeJSON(w, http.StatusOK, GetKVResponse{
			Key:     key,
			Value:   val,
			ShardID: route.ShardID,
		})
		return
	}

	// Forward GET to leader or candidate replica with failover
	candidates := route.Candidates(h.localNodeID)
	if len(candidates) == 0 {
		writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "no leader or replica currently available for shard", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := h.forwardWithFailover(ctx, route, candidates, http.MethodGet, r.URL.Path, nil, hops, reqID)
	if err != nil {
		writeJSONErrorWithReq(w, http.StatusBadGateway, fmt.Sprintf("forwarding failed: %v", err), reqID)
		return
	}
	defer resp.Body.Close()

	h.pipeResponse(w, resp)
}

func (h *KVHandler) handleDelete(w http.ResponseWriter, r *http.Request, key, reqID string, hops int) {
	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = r.Header.Get("Idempotency-Key")
	}
	if idempotencyKey != "" {
		if rec, found := h.idempotency.Get(idempotencyKey); found {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-ID", reqID)
			w.Header().Set("X-Idempotent-Replay", "true")
			w.WriteHeader(rec.statusCode)
			_, _ = w.Write(rec.body)
			return
		}
	}

	route, err := h.forwarder.Route(key, h.provider.ShardInstances())
	if err != nil {
		writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("failed to route key: %v", err), reqID)
		return
	}

	if route.IsLocal && route.Replica != nil && route.Replica.Raft().Role() == raft.Leader {
		if route.Replica.DB().IsClosed() {
			writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "database is shut down", reqID)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		defer cancel()

		idx, _, err := route.Replica.Raft().Propose("DELETE", key, "")
		if err != nil {
			if errors.Is(err, raft.ErrNotLeader) {
				h.forwarder.Resolver().Tracker().Invalidate(route.GroupID)
				writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "replica is no longer leader", reqID)
				return
			}
			if errors.Is(err, raft.ErrStopped) {
				writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "replica consensus is stopped", reqID)
				return
			}
			writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("failed to propose delete: %v", err), reqID)
			return
		}

		if err := route.Replica.Raft().WaitForApply(ctx, idx); err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "delete timed out waiting for consensus quorum", reqID)
				return
			}
			writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("failed to commit delete: %v", err), reqID)
			return
		}

		h.BroadcastEvent(observability.NewClusterEvent(
			observability.EventEntryCommitted,
			h.localNodeID,
			&route.ShardID,
			map[string]any{
				"key":       key,
				"index":     idx,
				"operation": "DELETE",
			},
		))

		respObj := DeleteKVResponse{
			Key:     key,
			Deleted: true,
			ShardID: route.ShardID,
		}
		if idempotencyKey != "" {
			if b, mErr := json.Marshal(respObj); mErr == nil {
				h.idempotency.Set(idempotencyKey, http.StatusOK, b)
			}
		}
		writeJSON(w, http.StatusOK, respObj)
		return
	}

	candidates := route.Candidates(h.localNodeID)
	if len(candidates) == 0 {
		writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "no leader or replica currently available for shard", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := h.forwardWithFailover(ctx, route, candidates, http.MethodDelete, r.URL.Path, nil, hops, reqID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "ambiguous") {
			writeJSONErrorWithReq(w, http.StatusGatewayTimeout, "forwarded write timed out with ambiguous outcome; check key status before retrying", reqID)
			return
		}
		writeJSONErrorWithReq(w, http.StatusBadGateway, fmt.Sprintf("forwarding failed: %v", err), reqID)
		return
	}
	defer resp.Body.Close()

	h.pipeResponse(w, resp)
}

func (h *KVHandler) forwardWithFailover(
	ctx context.Context,
	route *router.RouteResult,
	candidates []string,
	method, path string,
	body []byte,
	hops int,
	reqID string,
) (*http.Response, error) {
	var lastResp *http.Response
	var lastErr error

	for _, targetNode := range candidates {
		if ctx.Err() != nil {
			break
		}

		resp, err := h.forwarder.Forward(ctx, targetNode, method, path, body, hops, reqID)
		if err != nil {
			h.forwarder.Resolver().Tracker().InvalidateLeaderIf(route.GroupID, targetNode)
			lastErr = err

			// If the error was a context deadline / timeout after dispatching a mutating write,
			// the write is ambiguous: the remote node may have received and committed it.
			// Do not blindly retry mutating operations across multiple nodes!
			if method != http.MethodGet && (errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded)) {
				return nil, fmt.Errorf("forwarded %s to %s timed out with ambiguous outcome: %w", method, targetNode, err)
			}
			continue
		}

		// If remote node responded with 503 (e.g. not leader or stopped), invalidate and try next replica
		if resp.StatusCode == http.StatusServiceUnavailable {
			h.forwarder.Resolver().Tracker().InvalidateLeaderIf(route.GroupID, targetNode)
			if lastResp != nil {
				_ = lastResp.Body.Close()
			}
			lastResp = resp
			continue
		}

		// If remote node succeeded (200 OK or 404 Not Found), update cached leader
		if resp.StatusCode == http.StatusOK {
			h.forwarder.Resolver().Tracker().SetLeader(route.GroupID, targetNode)
		}

		if lastResp != nil {
			_ = lastResp.Body.Close()
		}
		return resp, nil
	}

	if lastResp != nil {
		return lastResp, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no available replica could satisfy request for shard %d", route.ShardID)
}

func (h *KVHandler) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, ClusterStatusResponse{
		ClusterID:  h.metadata.ClusterID,
		Epoch:      h.metadata.Epoch,
		ShardCount: h.metadata.ShardCount,
		NodeCount:  len(h.metadata.PhysicalNodes),
		Nodes:      h.metadata.PhysicalNodes,
	})
}

func (h *KVHandler) handleClusterHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.monitor == nil {
		h.monitor = observability.NewClusterMonitor(h.metadata, h.forwarder.Resolver().Tracker())
	}

	status := h.monitor.Collect(h.localNodeID, h.provider.ShardInstances(), nil)
	writeJSON(w, http.StatusOK, status)
}

func (h *KVHandler) handleClusterShards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var shards []ShardDetail
	for sid := sharding.ShardID(0); sid < sharding.ShardID(h.metadata.ShardCount); sid++ {
		info, err := h.metadata.GetShard(sid)
		if err == nil {
			leader, _ := h.forwarder.Resolver().Tracker().GetLeader(raft.GroupID(sid.GroupID()))
			shards = append(shards, ShardDetail{
				ID:       sid,
				Replicas: info.Replicas,
				Leader:   leader,
			})
		}
	}

	writeJSON(w, http.StatusOK, ClusterShardsResponse{
		Shards: shards,
	})
}

func (h *KVHandler) handleClusterNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, ClusterNodesResponse{
		Nodes: h.metadata.PhysicalNodes,
	})
}

func (h *KVHandler) pipeResponse(w http.ResponseWriter, resp *http.Response) {
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *KVHandler) handleBatch(w http.ResponseWriter, r *http.Request, reqID string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSONErrorWithReq(w, http.StatusMethodNotAllowed, "method not allowed", reqID)
		return
	}

	ct := r.Header.Get("Content-Type")
	if ct != "" {
		mediaType, _, _ := mime.ParseMediaType(ct)
		if mediaType != "application/json" {
			writeJSONErrorWithReq(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json", reqID)
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONErrorWithReq(w, http.StatusBadRequest, "failed to read request body", reqID)
		return
	}
	defer r.Body.Close()

	// 1. Try BatchWriteRequest
	var writeReq BatchWriteRequest
	if err := json.Unmarshal(body, &writeReq); err == nil && len(writeReq.Ops) > 0 {
		var results []BatchOpResult
		for _, op := range writeReq.Ops {
			if strings.TrimSpace(op.Key) == "" {
				results = append(results, BatchOpResult{
					Key:    op.Key,
					Status: "ERROR",
					Error:  "empty key not allowed",
				})
				continue
			}

			val := ""
			if op.Value != nil {
				val = *op.Value
			}

			opType := strings.ToUpper(op.Operation)
			if opType != "SET" && opType != "DELETE" {
				results = append(results, BatchOpResult{
					Key:    op.Key,
					Status: "ERROR",
					Error:  fmt.Sprintf("unsupported operation %q", op.Operation),
				})
				continue
			}

			route, err := h.forwarder.Route(op.Key, h.provider.ShardInstances())
			if err != nil {
				results = append(results, BatchOpResult{
					Key:    op.Key,
					Status: "ERROR",
					Error:  fmt.Sprintf("routing error: %v", err),
				})
				continue
			}

			if route.IsLocal && route.Replica != nil && route.Replica.Raft().Role() == raft.Leader {
				ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
				idx, _, err := route.Replica.Raft().Propose(opType, op.Key, val)
				if err != nil {
					cancel()
					results = append(results, BatchOpResult{
						Key:    op.Key,
						Status: "ERROR",
						Error:  err.Error(),
					})
					continue
				}
				if err := route.Replica.Raft().WaitForApply(ctx, idx); err != nil {
					cancel()
					results = append(results, BatchOpResult{
						Key:    op.Key,
						Status: "ERROR",
						Error:  err.Error(),
					})
					continue
				}
				cancel()
				results = append(results, BatchOpResult{
					Key:    op.Key,
					Status: "COMMITTED",
				})
			} else {
				subBody, _ := json.Marshal(SetKVRequest{Value: &val})
				httpMethod := http.MethodPut
				if opType == "DELETE" {
					httpMethod = http.MethodDelete
					subBody = nil
				}
				ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
				fResp, fErr := h.forwardWithFailover(ctx, route, route.Candidates(h.localNodeID), httpMethod, "/kv/"+url.PathEscape(op.Key), subBody, 0, reqID)
				cancel()
				if fErr != nil {
					results = append(results, BatchOpResult{
						Key:    op.Key,
						Status: "ERROR",
						Error:  fErr.Error(),
					})
				} else {
					_ = fResp.Body.Close()
					if fResp.StatusCode >= 200 && fResp.StatusCode < 300 {
						results = append(results, BatchOpResult{
							Key:    op.Key,
							Status: "COMMITTED",
						})
					} else {
						results = append(results, BatchOpResult{
							Key:    op.Key,
							Status: "ERROR",
							Error:  fmt.Sprintf("forward returned status %d", fResp.StatusCode),
						})
					}
				}
			}
		}

		writeJSON(w, http.StatusOK, BatchWriteResponse{
			Results:   results,
			RequestID: reqID,
		})
		return
	}

	// 2. Try BatchReadRequest
	var readReq BatchReadRequest
	if err := json.Unmarshal(body, &readReq); err == nil && len(readReq.Keys) > 0 {
		var results []BatchReadResult
		for _, k := range readReq.Keys {
			if strings.TrimSpace(k) == "" {
				results = append(results, BatchReadResult{
					Key:   k,
					Found: false,
					Error: "empty key",
				})
				continue
			}

			route, err := h.forwarder.Route(k, h.provider.ShardInstances())
			if err != nil {
				results = append(results, BatchReadResult{
					Key:   k,
					Found: false,
					Error: err.Error(),
				})
				continue
			}

			if route.IsLocal && route.Replica != nil && route.Replica.Raft().Role() == raft.Leader {
				val, err := route.Replica.DB().Get(k)
				if err != nil {
					results = append(results, BatchReadResult{
						Key:   k,
						Found: false,
					})
				} else {
					results = append(results, BatchReadResult{
						Key:   k,
						Value: val,
						Found: true,
					})
				}
			} else {
				ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
				fResp, fErr := h.forwardWithFailover(ctx, route, route.Candidates(h.localNodeID), http.MethodGet, "/kv/"+url.PathEscape(k), nil, 0, reqID)
				cancel()
				if fErr != nil || fResp.StatusCode != http.StatusOK {
					if fResp != nil {
						_ = fResp.Body.Close()
					}
					results = append(results, BatchReadResult{
						Key:   k,
						Found: false,
					})
				} else {
					var getResp GetKVResponse
					_ = json.NewDecoder(fResp.Body).Decode(&getResp)
					_ = fResp.Body.Close()
					results = append(results, BatchReadResult{
						Key:   k,
						Value: getResp.Value,
						Found: true,
					})
				}
			}
		}

		writeJSON(w, http.StatusOK, BatchReadResponse{
			Results:   results,
			RequestID: reqID,
		})
		return
	}

	writeJSONErrorWithReq(w, http.StatusBadRequest, "invalid batch payload: must contain 'ops' or 'keys'", reqID)
}

func (h *KVHandler) handleScan(w http.ResponseWriter, r *http.Request, reqID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSONErrorWithReq(w, http.StatusMethodNotAllowed, "method not allowed", reqID)
		return
	}

	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	allPairs := make(map[string]string)
	localInstances := h.provider.ShardInstances()

	for sid := sharding.ShardID(0); sid < sharding.ShardID(h.metadata.ShardCount); sid++ {
		inst, ok := localInstances[sid]
		if ok && inst.DB() != nil {
			pairs, err := inst.DB().Scan(start, end, limit+1)
			if err == nil {
				for _, p := range pairs {
					allPairs[p.Key] = p.Value
				}
			}
		}
	}

	sortedKeys := make([]string, 0, len(allPairs))
	for k := range allPairs {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	hasMore := false
	nextCursor := ""
	if len(sortedKeys) > limit {
		hasMore = true
		nextCursor = sortedKeys[limit]
		sortedKeys = sortedKeys[:limit]
	}

	var items []storage.KVPair
	for _, k := range sortedKeys {
		items = append(items, storage.KVPair{
			Key:   k,
			Value: allPairs[k],
		})
	}

	writeJSON(w, http.StatusOK, ScanResponse{
		Items:      items,
		HasMore:    hasMore,
		NextCursor: nextCursor,
		RequestID:  reqID,
	})
}

func (h *KVHandler) handlePrefixScan(w http.ResponseWriter, r *http.Request, reqID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSONErrorWithReq(w, http.StatusMethodNotAllowed, "method not allowed", reqID)
		return
	}

	prefix := r.URL.Query().Get("prefix")
	if prefix == "" {
		h.handleScan(w, r, reqID)
		return
	}

	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	allPairs := make(map[string]string)
	localInstances := h.provider.ShardInstances()

	for sid := sharding.ShardID(0); sid < sharding.ShardID(h.metadata.ShardCount); sid++ {
		inst, ok := localInstances[sid]
		if ok && inst.DB() != nil {
			pairs, err := inst.DB().PrefixScan(prefix, limit+1)
			if err == nil {
				for _, p := range pairs {
					allPairs[p.Key] = p.Value
				}
			}
		}
	}

	sortedKeys := make([]string, 0, len(allPairs))
	for k := range allPairs {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	hasMore := false
	nextCursor := ""
	if len(sortedKeys) > limit {
		hasMore = true
		nextCursor = sortedKeys[limit]
		sortedKeys = sortedKeys[:limit]
	}

	var items []storage.KVPair
	for _, k := range sortedKeys {
		items = append(items, storage.KVPair{
			Key:   k,
			Value: allPairs[k],
		})
	}

	writeJSON(w, http.StatusOK, ScanResponse{
		Items:      items,
		HasMore:    hasMore,
		NextCursor: nextCursor,
		RequestID:  reqID,
	})
}
