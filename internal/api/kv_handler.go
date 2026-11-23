package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
)

// ShardProvider exposes locally hosted shard replicas to the KV API.
type ShardProvider interface {
	ShardInstances() map[sharding.ShardID]router.ShardInstance
}

// SetKVRequest is the JSON payload for PUT /kv/:key.
type SetKVRequest struct {
	Value string `json:"value"`
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
}

// NewKVHandler constructs a new KVHandler.
func NewKVHandler(
	provider ShardProvider,
	metadata *sharding.ClusterMetadata,
	forwarder *router.RequestForwarder,
	localNodeID string,
) *KVHandler {
	return &KVHandler{
		provider:    provider,
		metadata:    metadata,
		forwarder:   forwarder,
		localNodeID: localNodeID,
		timeout:     5 * time.Second,
	}
}

// RegisterKVRoutes registers the shard-aware KV API routes on the provided mux.
func RegisterKVRoutes(mux *http.ServeMux, h *KVHandler) {
	mux.HandleFunc("/kv/", h.handleKV)
	mux.HandleFunc("/cluster/status", h.handleClusterStatus)
	mux.HandleFunc("/cluster/shards", h.handleClusterShards)
	mux.HandleFunc("/cluster/nodes", h.handleClusterNodes)
}

func (h *KVHandler) handleKV(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/kv/")
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		http.Error(w, `{"error":"key cannot be empty"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		h.handlePut(w, r, key)
	case http.MethodGet:
		h.handleGet(w, r, key)
	case http.MethodDelete:
		h.handleDelete(w, r, key)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *KVHandler) handlePut(w http.ResponseWriter, r *http.Request, key string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req SetKVRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}

	route, err := h.forwarder.Route(key, h.provider.ShardInstances())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to route key: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// 1. If routed locally and we are the leader, propose via Raft
	if route.IsLocal && route.Replica != nil && route.Replica.Raft().Role() == raft.Leader {
		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		defer cancel()

		idx, _, err := route.Replica.Raft().Propose("SET", key, req.Value)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to propose write: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		if err := route.Replica.Raft().WaitForApply(ctx, idx); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to commit write: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SetKVResponse{
			Key:     key,
			Value:   req.Value,
			ShardID: route.ShardID,
			Status:  "ok",
		})
		return
	}

	// 2. Otherwise forward to leader
	targetNode := route.LeaderID
	if targetNode == "" || targetNode == h.localNodeID {
		http.Error(w, `{"error":"no leader currently available for shard"}`, http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := h.forwarder.Forward(ctx, targetNode, http.MethodPut, r.URL.Path, body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"forward failed: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	h.pipeResponse(w, resp)
}

func (h *KVHandler) handleGet(w http.ResponseWriter, r *http.Request, key string) {
	route, err := h.forwarder.Route(key, h.provider.ShardInstances())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to route key: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// If local replica is leader for this shard, read directly from the state machine
	if route.IsLocal && route.Replica != nil && route.Replica.Raft().Role() == raft.Leader {
		val, err := route.Replica.DB().Get(key)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "key not found"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GetKVResponse{
			Key:     key,
			Value:   val,
			ShardID: route.ShardID,
		})
		return
	}

	// Forward GET to leader (or fallback to local replica if leader is unknown)
	targetNode := route.LeaderID
	if targetNode == "" || targetNode == h.localNodeID {
		if route.Replica != nil {
			val, err := route.Replica.DB().Get(key)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "key not found"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(GetKVResponse{
				Key:     key,
				Value:   val,
				ShardID: route.ShardID,
			})
			return
		}
		http.Error(w, `{"error":"no leader or replica available for shard"}`, http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := h.forwarder.Forward(ctx, targetNode, http.MethodGet, r.URL.Path, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"forward failed: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	h.pipeResponse(w, resp)
}

func (h *KVHandler) handleDelete(w http.ResponseWriter, r *http.Request, key string) {
	route, err := h.forwarder.Route(key, h.provider.ShardInstances())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to route key: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if route.IsLocal && route.Replica != nil && route.Replica.Raft().Role() == raft.Leader {
		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		defer cancel()

		idx, _, err := route.Replica.Raft().Propose("DELETE", key, "")
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to propose delete: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		if err := route.Replica.Raft().WaitForApply(ctx, idx); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to commit delete: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeleteKVResponse{
			Key:     key,
			Deleted: true,
			ShardID: route.ShardID,
		})
		return
	}

	targetNode := route.LeaderID
	if targetNode == "" || targetNode == h.localNodeID {
		http.Error(w, `{"error":"no leader currently available for shard"}`, http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := h.forwarder.Forward(ctx, targetNode, http.MethodDelete, r.URL.Path, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"forward failed: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	h.pipeResponse(w, resp)
}

func (h *KVHandler) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ClusterStatusResponse{
		ClusterID:  h.metadata.ClusterID,
		Epoch:      h.metadata.Epoch,
		ShardCount: h.metadata.ShardCount,
		NodeCount:  len(h.metadata.PhysicalNodes),
		Nodes:      h.metadata.PhysicalNodes,
	})
}

func (h *KVHandler) handleClusterShards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ClusterShardsResponse{
		Shards: shards,
	})
}

func (h *KVHandler) handleClusterNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ClusterNodesResponse{
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
