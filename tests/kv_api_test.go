package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/node"
	"orbit/internal/observability"
	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
)

type testCluster struct {
	nodes     map[string]*node.Node
	handlers  map[string]*api.KVHandler
	servers   map[string]*httptest.Server
	routers   map[string]*router.ShardRouter
	meta      *sharding.ClusterMetadata
	ring      *sharding.HashRing
	peerAddrs map[string]string
	net       *raft.MockNetwork
	leaders   map[sharding.ShardID]string
}

func setupTestKVCluster(t *testing.T) *testCluster {
	t.Helper()

	baseDir := t.TempDir()
	nodeIDs := []string{"node-a", "node-b", "node-c"}
	net := raft.NewMockNetwork()

	meta := sharding.NewClusterMetadata("orbit-kv-test", 1, 2, 3, nodeIDs)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b", "node-c"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a", "node-b", "node-c"}})

	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing: %v", err)
	}
	_ = ring.AddShard(0)
	_ = ring.AddShard(1)

	peerRaftAddrs := map[string]string{
		"node-a": "node-a",
		"node-b": "node-b",
		"node-c": "node-c",
	}

	nodes := make(map[string]*node.Node)
	for _, id := range nodeIDs {
		cfg := node.NodeConfig{
			ID:                 id,
			Address:            "127.0.0.1:0",
			RaftAddress:        "127.0.0.1:0",
			DataDir:            filepath.Join(baseDir, id),
			HeartbeatInterval:  25 * time.Millisecond,
			ElectionTimeoutMin: 80 * time.Millisecond,
			ElectionTimeoutMax: 160 * time.Millisecond,
		}
		tr := net.CreateTransport(id)
		n, err := node.NewShardedNode(cfg, meta, peerRaftAddrs, tr)
		if err != nil {
			t.Fatalf("NewShardedNode(%s): %v", id, err)
		}
		if err := n.Start(); err != nil {
			t.Fatalf("start node %s: %v", id, err)
		}
		nodes[id] = n
	}

	t.Cleanup(func() {
		for _, n := range nodes {
			_ = n.Stop()
		}
	})

	// Wait for leaders to be elected for both shards
	deadline := time.Now().Add(5 * time.Second)
	leaders := make(map[sharding.ShardID]string)
	for time.Now().Before(deadline) && len(leaders) < 2 {
		for sid := sharding.ShardID(0); sid < 2; sid++ {
			if _, ok := leaders[sid]; ok {
				continue
			}
			for _, id := range nodeIDs {
				replica, ok := nodes[id].Shard(sid)
				if ok && replica.Raft().Role() == raft.Leader {
					leaders[sid] = id
					break
				}
			}
		}
		time.Sleep(15 * time.Millisecond)
	}

	if len(leaders) < 2 {
		t.Fatalf("leaders not elected for all shards in time: %+v", leaders)
	}

	// Build HTTP test servers and wire KVHandlers
	servers := make(map[string]*httptest.Server)
	handlers := make(map[string]*api.KVHandler)
	peerHTTPAddrs := make(map[string]string)
	routers := make(map[string]*router.ShardRouter)
	muxes := make(map[string]*http.ServeMux)

	for _, id := range nodeIDs {
		mux := http.NewServeMux()
		muxes[id] = mux
		srv := httptest.NewServer(mux)
		servers[id] = srv
		peerHTTPAddrs[id] = srv.Listener.Addr().String()
	}

	for _, id := range nodeIDs {
		r := router.New(meta, ring)
		routers[id] = r
		tracker := router.NewLeaderTracker()
		for sid, ldr := range leaders {
			tracker.SetLeader(raft.GroupID(sid.GroupID()), ldr)
		}
		resolver := router.NewLeaderResolver(tracker)
		forwarder := router.NewRequestForwarder(r, resolver, id, peerHTTPAddrs)
		h := api.NewKVHandler(nodes[id], meta, forwarder, id)
		handlers[id] = h
		api.RegisterKVRoutes(muxes[id], h)
	}

	t.Cleanup(func() {
		for _, s := range servers {
			s.Close()
		}
	})

	return &testCluster{
		nodes:     nodes,
		handlers:  handlers,
		servers:   servers,
		routers:   routers,
		meta:      meta,
		ring:      ring,
		peerAddrs: peerHTTPAddrs,
		net:       net,
		leaders:   leaders,
	}
}

func findKeyForShard(t *testing.T, ring *sharding.HashRing, targetShard sharding.ShardID) string {
	t.Helper()
	for i := 0; i < 10000; i++ {
		k := fmt.Sprintf("key:for:shard:%d", i)
		sid, err := ring.LookupKey(k)
		if err == nil && sid == targetShard {
			return k
		}
	}
	t.Fatalf("could not find key for shard %d", targetShard)
	return ""
}

func TestKVAPI_PutAndGet(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)
	serverURL := c.servers["node-a"].URL

	// 1. PUT /kv/user:1
	putPayload := []byte(`{"value":"Alice"}`)
	putReq, err := http.NewRequest(http.MethodPut, serverURL+"/kv/user:1", bytes.NewReader(putPayload))
	if err != nil {
		t.Fatalf("NewRequest PUT: %v", err)
	}
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("Do PUT: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT returned status %d, want 200", putResp.StatusCode)
	}

	var setResp api.SetKVResponse
	if err := json.NewDecoder(putResp.Body).Decode(&setResp); err != nil {
		t.Fatalf("decode SetKVResponse: %v", err)
	}
	if setResp.Key != "user:1" || setResp.Value != "Alice" || setResp.Status != "ok" {
		t.Errorf("unexpected SetKVResponse: %+v", setResp)
	}

	// 2. GET /kv/user:1
	getResp, err := http.Get(serverURL + "/kv/user:1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET returned status %d, want 200", getResp.StatusCode)
	}

	var getRes api.GetKVResponse
	if err := json.NewDecoder(getResp.Body).Decode(&getRes); err != nil {
		t.Fatalf("decode GetKVResponse: %v", err)
	}
	if getRes.Key != "user:1" || getRes.Value != "Alice" {
		t.Errorf("unexpected GetKVResponse: %+v", getRes)
	}
}

func TestKVAPI_Delete(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)
	serverURL := c.servers["node-a"].URL

	// PUT
	putReq, _ := http.NewRequest(http.MethodPut, serverURL+"/kv/temp:key", bytes.NewReader([]byte(`{"value":"to-delete"}`)))
	putReq.Header.Set("Content-Type", "application/json")
	pResp, err := http.DefaultClient.Do(putReq)
	if err != nil || pResp.StatusCode != http.StatusOK {
		t.Fatalf("setup PUT failed: err=%v, code=%d", err, pResp.StatusCode)
	}
	pResp.Body.Close()

	// DELETE
	delReq, _ := http.NewRequest(http.MethodDelete, serverURL+"/kv/temp:key", nil)
	dResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	defer dResp.Body.Close()

	if dResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE returned status %d, want 200", dResp.StatusCode)
	}

	var delRes api.DeleteKVResponse
	if err := json.NewDecoder(dResp.Body).Decode(&delRes); err != nil {
		t.Fatalf("decode DeleteKVResponse: %v", err)
	}
	if !delRes.Deleted || delRes.Key != "temp:key" {
		t.Errorf("unexpected DeleteKVResponse: %+v", delRes)
	}

	// GET after delete -> 404
	gResp, err := http.Get(serverURL + "/kv/temp:key")
	if err != nil {
		t.Fatalf("GET after delete: %v", err)
	}
	defer gResp.Body.Close()
	if gResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", gResp.StatusCode)
	}
}

// Scenario 1: A follower receives a PUT request and forwards it to the leader.
func TestKVAPI_FollowerForwardsPut(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)
	targetShard := sharding.ShardID(0)
	leaderID := c.leaders[targetShard]

	// Select a node that is a follower for targetShard
	var followerID string
	for id := range c.nodes {
		if id != leaderID {
			followerID = id
			break
		}
	}

	key := findKeyForShard(t, c.ring, targetShard)
	followerURL := c.servers[followerID].URL

	// Send PUT to follower
	putReq, _ := http.NewRequest(http.MethodPut, followerURL+"/kv/"+key, bytes.NewReader([]byte(`{"value":"forwarded-put"}`)))
	putReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("PUT via follower %s failed: %v", followerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from forwarded PUT, got %d", resp.StatusCode)
	}

	var setResp api.SetKVResponse
	if err := json.NewDecoder(resp.Body).Decode(&setResp); err != nil {
		t.Fatalf("decode SetKVResponse: %v", err)
	}
	if setResp.Value != "forwarded-put" || setResp.ShardID != targetShard {
		t.Errorf("unexpected response: %+v", setResp)
	}

	// Read directly from leader to verify it applied
	leaderURL := c.servers[leaderID].URL
	gResp, err := http.Get(leaderURL + "/kv/" + key)
	if err != nil {
		t.Fatalf("GET from leader: %v", err)
	}
	defer gResp.Body.Close()
	if gResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from leader, got %d", gResp.StatusCode)
	}
}

// Scenario 2: A follower receives a DELETE request and forwards it.
func TestKVAPI_FollowerForwardsDelete(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)
	targetShard := sharding.ShardID(0)
	leaderID := c.leaders[targetShard]

	var followerID string
	for id := range c.nodes {
		if id != leaderID {
			followerID = id
			break
		}
	}

	key := findKeyForShard(t, c.ring, targetShard)

	// Write key first via leader
	pReq, _ := http.NewRequest(http.MethodPut, c.servers[leaderID].URL+"/kv/"+key, bytes.NewReader([]byte(`{"value":"before-delete"}`)))
	pReq.Header.Set("Content-Type", "application/json")
	pResp, err := http.DefaultClient.Do(pReq)
	if err != nil || pResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT key failed: err=%v, code=%d", err, pResp.StatusCode)
	}
	pResp.Body.Close()

	// Send DELETE via follower
	delReq, _ := http.NewRequest(http.MethodDelete, c.servers[followerID].URL+"/kv/"+key, nil)
	dResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE via follower failed: %v", err)
	}
	defer dResp.Body.Close()

	if dResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from forwarded DELETE, got %d", dResp.StatusCode)
	}

	var delRes api.DeleteKVResponse
	_ = json.NewDecoder(dResp.Body).Decode(&delRes)
	if !delRes.Deleted {
		t.Errorf("expected deleted=true, got %+v", delRes)
	}

	// Read back -> 404
	gResp, err := http.Get(c.servers[leaderID].URL + "/kv/" + key)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer gResp.Body.Close()
	if gResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", gResp.StatusCode)
	}
}

// Scenario 3: A leader loses its quorum and must not serve a successful linearizable read.
func TestKVAPI_PartitionedLeaderRejectsRead(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)
	targetShard := sharding.ShardID(0)
	leaderID := c.leaders[targetShard]
	key := findKeyForShard(t, c.ring, targetShard)

	// 1. Initial write
	pReq, _ := http.NewRequest(http.MethodPut, c.servers[leaderID].URL+"/kv/"+key, bytes.NewReader([]byte(`{"value":"linearizable-val"}`)))
	pReq.Header.Set("Content-Type", "application/json")
	pResp, err := http.DefaultClient.Do(pReq)
	if err != nil || pResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT failed: err=%v, code=%d", err, pResp.StatusCode)
	}
	pResp.Body.Close()

	// 2. Configure short timeout on leader's handler so test completes quickly
	c.handlers[leaderID].SetTimeout(150 * time.Millisecond)

	// 3. Partition/isolate the leader from all peers
	c.net.Isolate(leaderID)

	// 4. Try linearizable read on the isolated leader
	client := &http.Client{Timeout: 500 * time.Millisecond}
	gResp, err := client.Get(c.servers[leaderID].URL + "/kv/" + key)
	if err != nil {
		// Context timeout / connection drop is acceptable
		return
	}
	defer gResp.Body.Close()

	// Because leader lost quorum, ReadIndex must fail and return 503 Service Unavailable
	if gResp.StatusCode == http.StatusOK {
		t.Fatalf("linearizability violation: partitioned leader served successful read without quorum!")
	}
	if gResp.StatusCode != http.StatusServiceUnavailable && gResp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 503 Service Unavailable or 502, got status %d", gResp.StatusCode)
	}
}

// Scenario 4: The leader changes, and requests still reach the new leader.
func TestKVAPI_LeaderFailover(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)
	targetShard := sharding.ShardID(0)
	oldLeaderID := c.leaders[targetShard]
	key := findKeyForShard(t, c.ring, targetShard)

	// 1. Write initial value
	pReq, _ := http.NewRequest(http.MethodPut, c.servers[oldLeaderID].URL+"/kv/"+key, bytes.NewReader([]byte(`{"value":"v1"}`)))
	pReq.Header.Set("Content-Type", "application/json")
	pResp, err := http.DefaultClient.Do(pReq)
	if err != nil || pResp.StatusCode != http.StatusOK {
		t.Fatalf("initial write failed: err=%v, code=%d", err, pResp.StatusCode)
	}
	pResp.Body.Close()

	// 2. Stop old leader to force election of a new leader
	_ = c.nodes[oldLeaderID].Stop()
	c.servers[oldLeaderID].Close()

	// Pick a surviving node
	var survivorID string
	for id := range c.nodes {
		if id != oldLeaderID {
			survivorID = id
			break
		}
	}

	// Wait for new leader to be elected among surviving nodes
	var newLeaderID string
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		for id, n := range c.nodes {
			if id == oldLeaderID {
				continue
			}
			rep, ok := n.Shard(targetShard)
			if ok && rep.Raft().Role() == raft.Leader {
				newLeaderID = id
				break
			}
		}
		if newLeaderID != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if newLeaderID == "" {
		t.Fatal("surviving nodes failed to elect new leader for shard 0")
	}

	// Update tracker on survivor with new leader
	c.handlers[survivorID].SetTimeout(2 * time.Second)

	// 3. Write via survivor node to the new leader
	pReq2, _ := http.NewRequest(http.MethodPut, c.servers[survivorID].URL+"/kv/"+key, bytes.NewReader([]byte(`{"value":"v2-after-failover"}`)))
	pReq2.Header.Set("Content-Type", "application/json")
	pResp2, err := http.DefaultClient.Do(pReq2)
	if err != nil || pResp2.StatusCode != http.StatusOK {
		t.Fatalf("write after failover failed: err=%v, code=%d", err, pResp2.StatusCode)
	}
	pResp2.Body.Close()

	// 4. Read back
	gResp, err := http.Get(c.servers[survivorID].URL + "/kv/" + key)
	if err != nil || gResp.StatusCode != http.StatusOK {
		t.Fatalf("GET after failover failed: err=%v, code=%d", err, gResp.StatusCode)
	}
	defer gResp.Body.Close()

	var gRes api.GetKVResponse
	_ = json.NewDecoder(gResp.Body).Decode(&gRes)
	if gRes.Value != "v2-after-failover" {
		t.Errorf("expected v2-after-failover, got %q", gRes.Value)
	}
}

// Scenario 5: A shard has no available leader, and the API returns an appropriate unavailable response.
func TestKVAPI_ShardUnavailable(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)
	targetShard := sharding.ShardID(0)
	key := findKeyForShard(t, c.ring, targetShard)

	// Stop 2 out of 3 nodes so shard 0 cannot achieve quorum
	nodeIDs := []string{"node-a", "node-b", "node-c"}
	_ = c.nodes[nodeIDs[0]].Stop()
	c.servers[nodeIDs[0]].Close()
	_ = c.nodes[nodeIDs[1]].Stop()
	c.servers[nodeIDs[1]].Close()

	// The remaining node cannot achieve quorum
	survivorID := nodeIDs[2]
	c.handlers[survivorID].SetTimeout(100 * time.Millisecond)

	putReq, _ := http.NewRequest(http.MethodPut, c.servers[survivorID].URL+"/kv/"+key, bytes.NewReader([]byte(`{"value":"fail"}`)))
	putReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		return // Connection abort / timeout is acceptable
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected write to fail on leaderless shard, but got 200 OK")
	}
	if resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusBadGateway && resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 503, 502, or 500, got status %d", resp.StatusCode)
	}
}

// Scenario 6: An empty key is rejected with 400 Bad Request JSON.
func TestKVAPI_ValidationEmptyKey(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)
	serverURL := c.servers["node-a"].URL

	req, _ := http.NewRequest(http.MethodGet, serverURL+"/kv/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /kv/ failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for empty key, got %d", resp.StatusCode)
	}

	var errRes api.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errRes); err != nil {
		t.Fatalf("decode ErrorResponse: %v", err)
	}
	if errRes.Error != "key cannot be empty" {
		t.Errorf("unexpected error message: %q", errRes.Error)
	}
}

// Scenario 7: An empty string value is handled correctly, missing field is rejected.
func TestKVAPI_ValidationEmptyStringValue(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)
	serverURL := c.servers["node-a"].URL

	// 1. Valid empty string value: {"value":""}
	putReq, _ := http.NewRequest(http.MethodPut, serverURL+"/kv/empty:val", bytes.NewReader([]byte(`{"value":""}`)))
	putReq.Header.Set("Content-Type", "application/json")
	pResp, err := http.DefaultClient.Do(putReq)
	if err != nil || pResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT with empty string failed: err=%v, code=%d", err, pResp.StatusCode)
	}
	var setRes api.SetKVResponse
	_ = json.NewDecoder(pResp.Body).Decode(&setRes)
	pResp.Body.Close()

	if setRes.Value != "" || setRes.Key != "empty:val" {
		t.Errorf("unexpected set response for empty value: %+v", setRes)
	}

	// Read it back
	gResp, err := http.Get(serverURL + "/kv/empty:val")
	if err != nil || gResp.StatusCode != http.StatusOK {
		t.Fatalf("GET empty value failed: err=%v, code=%d", err, gResp.StatusCode)
	}
	var getRes api.GetKVResponse
	_ = json.NewDecoder(gResp.Body).Decode(&getRes)
	gResp.Body.Close()

	if getRes.Value != "" {
		t.Errorf("expected empty value, got %q", getRes.Value)
	}

	// 2. Missing value field: {} -> must return 400 Bad Request
	putMissing, _ := http.NewRequest(http.MethodPut, serverURL+"/kv/missing:val", bytes.NewReader([]byte(`{}`)))
	putMissing.Header.Set("Content-Type", "application/json")
	mResp, err := http.DefaultClient.Do(putMissing)
	if err != nil {
		t.Fatalf("PUT missing field: %v", err)
	}
	defer mResp.Body.Close()

	if mResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing value field, got %d", mResp.StatusCode)
	}

	// 3. Unknown field rejection
	putUnknown, _ := http.NewRequest(http.MethodPut, serverURL+"/kv/extra:field", bytes.NewReader([]byte(`{"value":"x","foo":"bar"}`)))
	putUnknown.Header.Set("Content-Type", "application/json")
	uResp, err := http.DefaultClient.Do(putUnknown)
	if err != nil {
		t.Fatalf("PUT unknown field: %v", err)
	}
	defer uResp.Body.Close()

	if uResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown fields, got %d", uResp.StatusCode)
	}

	// 4. Unsupported method -> 405 Method Not Allowed
	patchReq, _ := http.NewRequest(http.MethodPatch, serverURL+"/kv/some:key", bytes.NewReader([]byte(`{}`)))
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer patchResp.Body.Close()

	if patchResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d", patchResp.StatusCode)
	}
	if patchResp.Header.Get("Allow") == "" {
		t.Errorf("expected non-empty Allow header in 405 response")
	}
}

func TestKVAPI_ClusterEndpoints(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)
	serverURL := c.servers["node-a"].URL

	// 1. /cluster/status
	resp, err := http.Get(serverURL + "/cluster/status")
	if err != nil {
		t.Fatalf("GET /cluster/status: %v", err)
	}
	defer resp.Body.Close()

	var status api.ClusterStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode ClusterStatusResponse: %v", err)
	}
	if status.ClusterID != "orbit-kv-test" || status.ShardCount != 2 || status.NodeCount != 3 {
		t.Errorf("unexpected status response: %+v", status)
	}

	// 2. /cluster/shards
	respShards, err := http.Get(serverURL + "/cluster/shards")
	if err != nil {
		t.Fatalf("GET /cluster/shards: %v", err)
	}
	defer respShards.Body.Close()

	var shardsRes api.ClusterShardsResponse
	if err := json.NewDecoder(respShards.Body).Decode(&shardsRes); err != nil {
		t.Fatalf("decode ClusterShardsResponse: %v", err)
	}
	if len(shardsRes.Shards) != 2 {
		t.Errorf("expected 2 shards, got %d", len(shardsRes.Shards))
	}

	// 3. /cluster/nodes
	respNodes, err := http.Get(serverURL + "/cluster/nodes")
	if err != nil {
		t.Fatalf("GET /cluster/nodes: %v", err)
	}
	defer respNodes.Body.Close()

	var nodesRes api.ClusterNodesResponse
	if err := json.NewDecoder(respNodes.Body).Decode(&nodesRes); err != nil {
		t.Fatalf("decode ClusterNodesResponse: %v", err)
	}
	if len(nodesRes.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodesRes.Nodes))
	}

	// 4. /cluster/health (live observability)
	respHealth, err := http.Get(serverURL + "/cluster/health")
	if err != nil {
		t.Fatalf("GET /cluster/health: %v", err)
	}
	defer respHealth.Body.Close()

	var healthRes observability.ClusterStatus
	if err := json.NewDecoder(respHealth.Body).Decode(&healthRes); err != nil {
		t.Fatalf("decode ClusterStatus: %v", err)
	}
	if healthRes.TotalNodes != 3 || healthRes.TotalShards != 2 || healthRes.HealthyShards < 1 {
		t.Errorf("unexpected health response: %+v", healthRes)
	}
}

func TestKVAPI_MultiShardRouting(t *testing.T) {
	t.Parallel()

	c := setupTestKVCluster(t)

	// Write 10 keys across different servers, verifying consistent retrieval
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("multi:key:%d", i)
		val := fmt.Sprintf("value-%d", i)

		targetServer := c.servers[fmt.Sprintf("node-%c", 'a'+(i%3))].URL

		putReq, _ := http.NewRequest(http.MethodPut, targetServer+"/kv/"+key, bytes.NewReader([]byte(fmt.Sprintf(`{"value":%q}`, val))))
		putReq.Header.Set("Content-Type", "application/json")
		pResp, err := http.DefaultClient.Do(putReq)
		if err != nil || pResp.StatusCode != http.StatusOK {
			t.Fatalf("PUT %s failed: err=%v, code=%d", key, err, pResp.StatusCode)
		}
		pResp.Body.Close()

		// Read back from a different server
		readServer := c.servers[fmt.Sprintf("node-%c", 'a'+((i+1)%3))].URL
		gResp, err := http.Get(readServer + "/kv/" + key)
		if err != nil || gResp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s from %s failed: err=%v, code=%d", key, readServer, err, gResp.StatusCode)
		}
		var gRes api.GetKVResponse
		_ = json.NewDecoder(gResp.Body).Decode(&gRes)
		gResp.Body.Close()

		if gRes.Value != val {
			t.Errorf("key %s value mismatch: got %q, want %q", key, gRes.Value, val)
		}
	}
}
