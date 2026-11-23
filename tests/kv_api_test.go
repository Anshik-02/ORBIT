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
			HeartbeatInterval:  30 * time.Millisecond,
			ElectionTimeoutMin: 100 * time.Millisecond,
			ElectionTimeoutMax: 200 * time.Millisecond,
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
	deadline := time.Now().Add(4 * time.Second)
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
		time.Sleep(20 * time.Millisecond)
	}

	if len(leaders) < 2 {
		t.Fatalf("leaders not elected for all shards in time: %+v", leaders)
	}

	// Build HTTP test servers and wire KVHandlers
	servers := make(map[string]*httptest.Server)
	handlers := make(map[string]*api.KVHandler)
	peerHTTPAddrs := make(map[string]string)
	routers := make(map[string]*router.ShardRouter)

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

		mux := http.NewServeMux()
		api.RegisterKVRoutes(mux, h)
		srv := httptest.NewServer(mux)
		servers[id] = srv
		peerHTTPAddrs[id] = srv.Listener.Addr().String()
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
	}
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
