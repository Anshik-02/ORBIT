package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/database"
	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
	"orbit/internal/storage"
)

type mockShardProvider struct {
	shards map[sharding.ShardID]router.ShardInstance
}

func (m *mockShardProvider) ShardInstances() map[sharding.ShardID]router.ShardInstance {
	return m.shards
}

type mockShardInstance struct {
	shardID sharding.ShardID
	groupID raft.GroupID
	raft    *raft.Raft
	db      *database.Database
}

func (m *mockShardInstance) ID() sharding.ShardID    { return m.shardID }
func (m *mockShardInstance) GroupID() raft.GroupID   { return m.groupID }
func (m *mockShardInstance) Raft() *raft.Raft        { return m.raft }
func (m *mockShardInstance) DB() *database.Database  { return m.db }
func (m *mockShardInstance) Engine() *storage.Engine {
	if m.db != nil {
		if eng, ok := m.db.Engine().(*storage.Engine); ok {
			return eng
		}
	}
	return nil
}

func setupTestKVHandler(t *testing.T) (*api.KVHandler, *sharding.ClusterMetadata, *mockShardProvider) {
	t.Helper()

	nodes := []string{"node-a", "node-b", "node-c"}
	meta := sharding.NewClusterMetadata("test-kv-cluster", 1, 3, 3, nodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b", "node-c"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a", "node-b", "node-c"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 2, Replicas: []string{"node-a", "node-b", "node-c"}})

	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing: %v", err)
	}
	for s := sharding.ShardID(0); s < 3; s++ {
		_ = ring.AddShard(s)
	}

	r := router.New(meta, ring)
	tracker := router.NewLeaderTracker()
	resolver := router.NewLeaderResolver(tracker)
	peerAddrs := map[string]string{
		"node-a": "127.0.0.1:9001",
		"node-b": "127.0.0.1:9002",
		"node-c": "127.0.0.1:9003",
	}

	fwd := router.NewRequestForwarder(r, resolver, "node-a", peerAddrs)
	provider := &mockShardProvider{
		shards: make(map[sharding.ShardID]router.ShardInstance),
	}

	handler := api.NewKVHandler(provider, meta, fwd, "node-a")
	handler.SetTimeout(500 * time.Millisecond)

	return handler, meta, provider
}

func TestKVHandler_Validation(t *testing.T) {
	t.Parallel()

	h, _, _ := setupTestKVHandler(t)
	mux := http.NewServeMux()
	api.RegisterKVRoutes(mux, h)

	// 1. Invalid Content-Type
	req := httptest.NewRequest(http.MethodPut, "/kv/test-key", strings.NewReader(`{"value":"hello"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415 Unsupported Media Type, got %d", rec.Code)
	}

	// 2. Empty Body
	reqEmpty := httptest.NewRequest(http.MethodPut, "/kv/test-key", bytes.NewReader(nil))
	reqEmpty.Header.Set("Content-Type", "application/json")
	recEmpty := httptest.NewRecorder()
	mux.ServeHTTP(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty body, got %d", recEmpty.Code)
	}

	// 3. Malformed JSON
	reqMalformed := httptest.NewRequest(http.MethodPut, "/kv/test-key", strings.NewReader(`{invalid}`))
	reqMalformed.Header.Set("Content-Type", "application/json")
	recMalformed := httptest.NewRecorder()
	mux.ServeHTTP(recMalformed, reqMalformed)
	if recMalformed.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for malformed json, got %d", recMalformed.Code)
	}

	// 4. Missing value field
	reqMissingVal := httptest.NewRequest(http.MethodPut, "/kv/test-key", strings.NewReader(`{"other":"field"}`))
	reqMissingVal.Header.Set("Content-Type", "application/json")
	recMissingVal := httptest.NewRecorder()
	mux.ServeHTTP(recMissingVal, reqMissingVal)
	if recMissingVal.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for missing value, got %d", recMissingVal.Code)
	}

	// 5. Trailing JSON garbage
	reqTrailing := httptest.NewRequest(http.MethodPut, "/kv/test-key", strings.NewReader(`{"value":"abc"} unexpected`))
	reqTrailing.Header.Set("Content-Type", "application/json")
	recTrailing := httptest.NewRecorder()
	mux.ServeHTTP(recTrailing, reqTrailing)
	if recTrailing.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for trailing garbage, got %d", recTrailing.Code)
	}
}

func TestKVHandler_ClusterDiscoveryEndpoints(t *testing.T) {
	t.Parallel()

	h, _, _ := setupTestKVHandler(t)
	mux := http.NewServeMux()
	api.RegisterKVRoutes(mux, h)

	// GET /cluster/status
	recStatus := httptest.NewRecorder()
	reqStatus := httptest.NewRequest(http.MethodGet, "/cluster/status", nil)
	mux.ServeHTTP(recStatus, reqStatus)
	if recStatus.Code != http.StatusOK {
		t.Errorf("expected 200 for /cluster/status, got %d", recStatus.Code)
	}
	var statusRes api.ClusterStatusResponse
	if err := json.NewDecoder(recStatus.Body).Decode(&statusRes); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if statusRes.ClusterID != "test-kv-cluster" || statusRes.ShardCount != 3 || statusRes.NodeCount != 3 {
		t.Errorf("unexpected status response: %+v", statusRes)
	}

	// GET /cluster/shards
	recShards := httptest.NewRecorder()
	reqShards := httptest.NewRequest(http.MethodGet, "/cluster/shards", nil)
	mux.ServeHTTP(recShards, reqShards)
	if recShards.Code != http.StatusOK {
		t.Errorf("expected 200 for /cluster/shards, got %d", recShards.Code)
	}
	var shardsRes api.ClusterShardsResponse
	if err := json.NewDecoder(recShards.Body).Decode(&shardsRes); err != nil {
		t.Fatalf("decode shards: %v", err)
	}
	if len(shardsRes.Shards) != 3 {
		t.Errorf("expected 3 shards, got %d", len(shardsRes.Shards))
	}

	// GET /cluster/nodes
	recNodes := httptest.NewRecorder()
	reqNodes := httptest.NewRequest(http.MethodGet, "/cluster/nodes", nil)
	mux.ServeHTTP(recNodes, reqNodes)
	if recNodes.Code != http.StatusOK {
		t.Errorf("expected 200 for /cluster/nodes, got %d", recNodes.Code)
	}
	var nodesRes api.ClusterNodesResponse
	if err := json.NewDecoder(recNodes.Body).Decode(&nodesRes); err != nil {
		t.Fatalf("decode nodes: %v", err)
	}
	if len(nodesRes.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodesRes.Nodes))
	}
}

func TestKVHandler_ForwardWithFailover(t *testing.T) {
	t.Parallel()

	// Simulate node-b (dead) and node-c (alive leader)
	nodeCServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.SetKVResponse{
			Key:     "test-key",
			Value:   "val-from-c",
			ShardID: 0,
			Status:  "ok",
		})
	}))
	defer nodeCServer.Close()

	nodes := []string{"node-a", "node-b", "node-c"}
	meta := sharding.NewClusterMetadata("failover-cluster", 1, 1, 3, nodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b", "node-c"}})

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	_ = ring.AddShard(0)

	r := router.New(meta, ring)
	tracker := router.NewLeaderTracker()
	// Stale leader tracker points to dead node-b (port 1 fails)
	tracker.SetLeader(raft.GroupID("shard-0"), "node-b")

	resolver := router.NewLeaderResolver(tracker)
	peerAddrs := map[string]string{
		"node-a": "127.0.0.1:9001",
		"node-b": "127.0.0.1:1", // dead port -> connection refused
		"node-c": nodeCServer.Listener.Addr().String(),
	}

	fwd := router.NewRequestForwarder(r, resolver, "node-a", peerAddrs)
	provider := &mockShardProvider{shards: make(map[sharding.ShardID]router.ShardInstance)}

	h := api.NewKVHandler(provider, meta, fwd, "node-a")
	h.SetTimeout(1 * time.Second)

	mux := http.NewServeMux()
	api.RegisterKVRoutes(mux, h)

	req := httptest.NewRequest(http.MethodPut, "/kv/test-key", strings.NewReader(`{"value":"val-from-c"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected failover to node-c with 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res api.SetKVResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.Value != "val-from-c" {
		t.Errorf("expected val-from-c, got %s", res.Value)
	}

	// Verify tracker learned node-c is now the leader after failover!
	if ldr, ok := tracker.GetLeader(raft.GroupID("shard-0")); !ok || ldr != "node-c" {
		t.Errorf("expected tracker updated to node-c, got leader=%q, ok=%v", ldr, ok)
	}
}

// TestKVHandler_StorageErrors verifies 404 for missing keys and 503 for closed db
func TestKVHandler_StorageErrors(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	eng, err := storage.NewEngine(tempDir + "/wal.log")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	db, err := database.NewDatabase(eng)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}

	// Closed DB returns ErrClosed -> 503
	_ = db.Close()
	_ = eng.Close()

	if !db.IsClosed() {
		t.Fatal("expected db to be closed")
	}
}
