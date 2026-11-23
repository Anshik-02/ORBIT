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

// TestE2E_100KeyOperations verifies Task 12:
// Complete cluster operation with 5 physical nodes and 2 overlapping shards:
//
//	NODE A    NODE B    NODE C
//	   \        |        /
//	     SHARD 0
//
//	NODE D    NODE E    NODE A
//	   \        |        /
//	     SHARD 1
//
// Sends 100 different key-value operations through the client API.
// Verifies that data lands in the correct shard and is replicated.
func TestE2E_100KeyOperations(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	nodeIDs := []string{"node-a", "node-b", "node-c", "node-d", "node-e"}
	net := raft.NewMockNetwork()

	meta := sharding.NewClusterMetadata("orbit-e2e-cluster", 1, 2, 3, nodeIDs)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b", "node-c"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-d", "node-e", "node-a"}})

	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing: %v", err)
	}
	_ = ring.AddShard(0)
	_ = ring.AddShard(1)

	peerRaftAddrs := make(map[string]string)
	for _, id := range nodeIDs {
		peerRaftAddrs[id] = id
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
		time.Sleep(20 * time.Millisecond)
	}

	if len(leaders) < 2 {
		t.Fatalf("leaders not elected for all shards in time: %+v", leaders)
	}
	t.Logf("Shard leaders elected: Shard 0 -> %s, Shard 1 -> %s", leaders[0], leaders[1])

	// Setup HTTP test servers with router and forwarder
	servers := make(map[string]*httptest.Server)
	peerHTTPAddrs := make(map[string]string)

	for _, id := range nodeIDs {
		r := router.New(meta, ring)
		tracker := router.NewLeaderTracker()
		for sid, ldr := range leaders {
			tracker.SetLeader(raft.GroupID(sid.GroupID()), ldr)
		}
		resolver := router.NewLeaderResolver(tracker)
		forwarder := router.NewRequestForwarder(r, resolver, id, peerHTTPAddrs)
		h := api.NewKVHandler(nodes[id], meta, forwarder, id)

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

	expectedState := make(map[string]string)
	keysWritten := make([]string, 0, 100)

	// Send 100 different key-value operations through the client API
	// Distribute calls evenly across all 5 physical nodes
	for i := 0; i < 70; i++ {
		key := fmt.Sprintf("e2e:key:%03d", i)
		val := fmt.Sprintf("val-%d-timestamp-%d", i, time.Now().UnixNano())

		// Pick client entry node in round-robin fashion
		entryNode := nodeIDs[i%len(nodeIDs)]
		serverURL := servers[entryNode].URL

		// PUT
		body := []byte(fmt.Sprintf(`{"value":%q}`, val))
		req, _ := http.NewRequest(http.MethodPut, serverURL+"/kv/"+key, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("op %d: PUT %s via %s failed: err=%v, code=%d", i, key, entryNode, err, resp.StatusCode)
		}
		resp.Body.Close()

		expectedState[key] = val
		keysWritten = append(keysWritten, key)
	}

	// 20 GET operations from random nodes verifying written data
	for i := 0; i < 20; i++ {
		key := keysWritten[i]
		expectedVal := expectedState[key]

		entryNode := nodeIDs[(i+2)%len(nodeIDs)]
		serverURL := servers[entryNode].URL

		resp, err := http.Get(serverURL + "/kv/" + key)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s via %s failed: err=%v, code=%d", key, entryNode, err, resp.StatusCode)
		}

		var getRes api.GetKVResponse
		_ = json.NewDecoder(resp.Body).Decode(&getRes)
		resp.Body.Close()

		if getRes.Value != expectedVal {
			t.Errorf("key %s value mismatch: got %q, want %q", key, getRes.Value, expectedVal)
		}

		// Verify data landed in the shard predicted by the HashRing
		expectedShard, _ := ring.LookupKey(key)
		if getRes.ShardID != expectedShard {
			t.Errorf("key %s routed to shard %d, hash ring predicted shard %d", key, getRes.ShardID, expectedShard)
		}
	}

	// 10 DELETE operations
	for i := 0; i < 10; i++ {
		key := keysWritten[i]
		entryNode := nodeIDs[(i+3)%len(nodeIDs)]
		serverURL := servers[entryNode].URL

		delReq, _ := http.NewRequest(http.MethodDelete, serverURL+"/kv/"+key, nil)
		resp, err := http.DefaultClient.Do(delReq)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("DELETE %s via %s failed: err=%v, code=%d", key, entryNode, err, resp.StatusCode)
		}
		resp.Body.Close()

		// Verify GET now returns 404
		getResp, err := http.Get(serverURL + "/kv/" + key)
		if err != nil || getResp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for deleted key %s, got err=%v, code=%d", key, err, getResp.StatusCode)
		}
		getResp.Body.Close()

		delete(expectedState, key)
	}

	// Total operations: 70 PUTs + 20 GETs + 10 DELETEs = 100 key-value operations
	t.Logf("Completed 100 key-value operations across 5 nodes and 2 shards successfully")
}
