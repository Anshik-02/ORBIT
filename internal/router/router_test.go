package router_test

import (
	"testing"

	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
)

func setupTestRouter(t *testing.T) (*router.ShardRouter, *sharding.ClusterMetadata, *sharding.HashRing) {
	t.Helper()

	nodes := []string{"node-a", "node-b", "node-c"}
	meta := sharding.NewClusterMetadata("test-cluster", 1, 3, 2, nodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-b", "node-c"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 2, Replicas: []string{"node-a", "node-c"}})

	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing: %v", err)
	}
	for sid := sharding.ShardID(0); sid < 3; sid++ {
		if err := ring.AddShard(sid); err != nil {
			t.Fatalf("AddShard(%s): %v", sid, err)
		}
	}

	r := router.New(meta, ring)
	return r, meta, ring
}

func TestShardRouter_GetShardID(t *testing.T) {
	t.Parallel()

	r, _, _ := setupTestRouter(t)

	keys := []string{"user:1", "user:2", "product:99", "order:555"}
	for _, k := range keys {
		shardID, err := r.GetShardID(k)
		if err != nil {
			t.Fatalf("GetShardID(%q) error: %v", k, err)
		}
		if shardID < 0 || shardID >= 3 {
			t.Errorf("expected shardID in [0, 2], got %d", shardID)
		}
	}
}

func TestShardRouter_GetShardInfo(t *testing.T) {
	t.Parallel()

	r, _, _ := setupTestRouter(t)

	info, err := r.GetShardInfo("user:1")
	if err != nil {
		t.Fatalf("GetShardInfo error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil ShardInfo")
	}
	if len(info.Replicas) != 2 {
		t.Errorf("expected 2 replicas, got %d", len(info.Replicas))
	}
}

func TestLeaderTracker_SetGetInvalidate(t *testing.T) {
	t.Parallel()

	lt := router.NewLeaderTracker()
	groupID := raft.GroupID("shard-0")

	// Initially unknown
	if leader, ok := lt.GetLeader(groupID); ok || leader != "" {
		t.Errorf("expected no leader initially, got leader=%q, ok=%v", leader, ok)
	}

	// Set leader
	lt.SetLeader(groupID, "node-b")
	leader, ok := lt.GetLeader(groupID)
	if !ok || leader != "node-b" {
		t.Errorf("expected leader node-b, got leader=%q, ok=%v", leader, ok)
	}

	// Invalidate
	lt.Invalidate(groupID)
	if leader, ok := lt.GetLeader(groupID); ok || leader != "" {
		t.Errorf("expected no leader after invalidate, got leader=%q, ok=%v", leader, ok)
	}
}

func TestRequestForwarder_Route(t *testing.T) {
	t.Parallel()

	r, _, _ := setupTestRouter(t)
	tracker := router.NewLeaderTracker()
	resolver := router.NewLeaderResolver(tracker)

	peerAddrs := map[string]string{
		"node-a": "127.0.0.1:8001",
		"node-b": "127.0.0.1:8002",
		"node-c": "127.0.0.1:8003",
	}

	forwarder := router.NewRequestForwarder(r, resolver, "node-a", peerAddrs)

	// Route when local node does not host any shards -> should indicate forward
	res, err := forwarder.Route("any-key", nil)
	if err != nil {
		t.Fatalf("Route error: %v", err)
	}
	if !res.Forward {
		t.Errorf("expected forward=true when key not hosted locally, got false")
	}
	if res.LeaderID == "" {
		t.Errorf("expected non-empty candidate LeaderID")
	}
}
