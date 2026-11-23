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
	"strconv"
	"strings"
	"time"

	"orbit/internal/database"
	"orbit/internal/observability"
	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
)

const maxRequestBodySize = 1 << 20 // 1 MB

// ShardProvider exposes locally hosted shard replicas to the KV API.
type ShardProvider interface {
	ShardInstances() map[sharding.ShardID]router.ShardInstance
}

// ErrorResponseWithReq represents a standardized JSON error response including request ID.
type ErrorResponseWithReq struct {
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty"`
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

// SetTimeout configures the request processing and forwarding timeout.
func (h *KVHandler) SetTimeout(d time.Duration) {
	h.timeout = d
}

// RegisterKVRoutes registers the shard-aware KV API routes on the provided mux.
func RegisterKVRoutes(mux *http.ServeMux, h *KVHandler) {
	mux.HandleFunc("/kv/", h.handleKV)
	mux.HandleFunc("/cluster/status", h.handleClusterStatus)
	mux.HandleFunc("/cluster/health", h.handleClusterHealth)
	mux.HandleFunc("/cluster/shards", h.handleClusterShards)
	mux.HandleFunc("/cluster/nodes", h.handleClusterNodes)

	if h.wsHub != nil {
		h.wsHub.RegisterRoutes(mux, "/ws/events")
		h.wsHub.RegisterRoutes(mux, "/cluster/events")
	}
}

func writeJSONErrorWithReq(w http.ResponseWriter, status int, msg, reqID string) {
	w.Header().Set("Content-Type", "application/json")
	if reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponseWithReq{
		Error:     msg,
		RequestID: reqID,
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSONErrorWithReq(w, status, msg, "")
}

func (h *KVHandler) handleKV(w http.ResponseWriter, r *http.Request) {
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		reqID = hex.EncodeToString(b)
	}
	w.Header().Set("X-Request-ID", reqID)

	rawKey := strings.TrimPrefix(r.URL.Path, "/kv/")
	rawKey = strings.TrimPrefix(rawKey, "/")
	key, err := url.PathUnescape(rawKey)
	if err != nil || strings.Contains(key, "\x00") {
		writeJSONErrorWithReq(w, http.StatusBadRequest, "key is invalid", reqID)
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
			writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("failed to commit write: %v", err), reqID)
			return
		}

		if h.eventHub != nil {
			h.eventHub.Publish(observability.NewClusterEvent(
				observability.EventEntryCommitted,
				h.localNodeID,
				&route.ShardID,
				map[string]any{
					"key":       key,
					"index":     idx,
					"operation": "SET",
				},
			))
		}

		writeJSON(w, http.StatusOK, SetKVResponse{
			Key:     key,
			Value:   val,
			ShardID: route.ShardID,
			Status:  "ok",
		})
		return
	}

	// 2. Otherwise forward to target leader or candidate replica
	targetNode := route.TargetNode()
	if targetNode == "" || targetNode == h.localNodeID {
		writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "no leader or replica currently available for shard", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := h.forwarder.Forward(ctx, targetNode, http.MethodPut, r.URL.Path, body, hops)
	if err != nil {
		h.forwarder.Resolver().Tracker().Invalidate(route.GroupID)
		writeJSONErrorWithReq(w, http.StatusBadGateway, fmt.Sprintf("forward to %s failed: %v", targetNode, err), reqID)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		h.forwarder.Resolver().Tracker().Invalidate(route.GroupID)
	}

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

	// Forward GET to leader or candidate replica
	targetNode := route.TargetNode()
	if targetNode == "" || targetNode == h.localNodeID {
		writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "no leader or replica currently available for shard", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := h.forwarder.Forward(ctx, targetNode, http.MethodGet, r.URL.Path, nil, hops)
	if err != nil {
		h.forwarder.Resolver().Tracker().Invalidate(route.GroupID)
		writeJSONErrorWithReq(w, http.StatusBadGateway, fmt.Sprintf("forward to %s failed: %v", targetNode, err), reqID)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		h.forwarder.Resolver().Tracker().Invalidate(route.GroupID)
	}

	h.pipeResponse(w, resp)
}

func (h *KVHandler) handleDelete(w http.ResponseWriter, r *http.Request, key, reqID string, hops int) {
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
			writeJSONErrorWithReq(w, http.StatusInternalServerError, fmt.Sprintf("failed to commit delete: %v", err), reqID)
			return
		}

		if h.eventHub != nil {
			h.eventHub.Publish(observability.NewClusterEvent(
				observability.EventEntryCommitted,
				h.localNodeID,
				&route.ShardID,
				map[string]any{
					"key":       key,
					"index":     idx,
					"operation": "DELETE",
				},
			))
		}

		writeJSON(w, http.StatusOK, DeleteKVResponse{
			Key:     key,
			Deleted: true,
			ShardID: route.ShardID,
		})
		return
	}

	targetNode := route.TargetNode()
	if targetNode == "" || targetNode == h.localNodeID {
		writeJSONErrorWithReq(w, http.StatusServiceUnavailable, "no leader or replica currently available for shard", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := h.forwarder.Forward(ctx, targetNode, http.MethodDelete, r.URL.Path, nil, hops)
	if err != nil {
		h.forwarder.Resolver().Tracker().Invalidate(route.GroupID)
		writeJSONErrorWithReq(w, http.StatusBadGateway, fmt.Sprintf("forward to %s failed: %v", targetNode, err), reqID)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		h.forwarder.Resolver().Tracker().Invalidate(route.GroupID)
	}

	h.pipeResponse(w, resp)
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
