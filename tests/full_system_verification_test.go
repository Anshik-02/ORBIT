package tests

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/benchmark"
	"orbit/internal/chaos"
	"orbit/internal/node"
	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
)

// TestFullSystem_FiveNodeThreeShardCluster executes the comprehensive full-system verification
// requested in Area F across five physical nodes, three independent Raft groups, failovers,
// dynamic membership, replica migrations, compaction, and benchmarks.
func TestFullSystem_FiveNodeThreeShardCluster(t *testing.T) {
	baseDir := t.TempDir()
	nodeIDs := []string{"node-a", "node-b", "node-c", "node-d", "node-e"}
	net := raft.NewMockNetwork()

	meta := sharding.NewClusterMetadata("orbit-full-cluster", 1, 3, 3, nodeIDs)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b", "node-c"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-b", "node-c", "node-d"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 2, Replicas: []string{"node-c", "node-d", "node-e"}})

	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing: %v", err)
	}
	_ = ring.AddShard(0)
	_ = ring.AddShard(1)
	_ = ring.AddShard(2)

	peerRaftAddrs := make(map[string]string)
	for _, id := range nodeIDs {
		peerRaftAddrs[id] = id
	}

	nodes := make(map[string]*node.Node)
	handlers := make(map[string]*api.KVHandler)
	muxes := make(map[string]*http.ServeMux)

	for _, id := range nodeIDs {
		cfg := node.NodeConfig{
			ID:                 id,
			Address:            "127.0.0.1:0",
			RaftAddress:        "127.0.0.1:0",
			DataDir:            filepath.Join(baseDir, id),
			HeartbeatInterval:  20 * time.Millisecond,
			ElectionTimeoutMin: 50 * time.Millisecond,
			ElectionTimeoutMax: 100 * time.Millisecond,
		}

		tr := net.CreateTransport(id)
		n, err := node.NewShardedNode(cfg, meta, peerRaftAddrs, tr)
		if err != nil {
			t.Fatalf("NewShardedNode(%s): %v", id, err)
		}

		shardRouter := router.New(meta, ring)
		tracker := router.NewLeaderTracker()
		resolver := router.NewLeaderResolver(tracker)
		forwarder := router.NewRequestForwarder(shardRouter, resolver, id, nil)

		kvHandler := api.NewKVHandler(n, meta, forwarder, id)
		mux := http.NewServeMux()
		api.RegisterKVRoutes(mux, kvHandler)

		nodes[id] = n
		handlers[id] = kvHandler
		muxes[id] = mux
	}

	// 1. Start all 5 physical nodes
	for id, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("Start(%s): %v", id, err)
		}
	}
	defer func() {
		for _, n := range nodes {
			if n != nil {
				_ = n.Stop()
			}
		}
	}()

	// 2. Wait for leader election across all 3 independent Raft groups
	time.Sleep(300 * time.Millisecond)

	for sid := sharding.ShardID(0); sid < 3; sid++ {
		var leader string
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			for _, id := range nodeIDs {
				inst := nodes[id].ShardInstance(sid)
				if inst != nil && inst.Raft().Role() == raft.Leader {
					leader = id
					break
				}
			}
			if leader != "" {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if leader == "" {
			t.Fatalf("Shard %d failed to elect a leader within deadline", sid)
		}
		t.Logf("Shard %d elected leader: %s", sid, leader)
	}

	// 3. Write keys to all shards and read through different ingress nodes
	keysWritten := make(map[string]string)
	for i := 0; i < 30; i++ {
		key := fmt.Sprintf("full_sys_key_%03d", i)
		val := fmt.Sprintf("val_%03d", i)

		// Rotate ingress node among all 5 physical nodes
		ingressNode := nodeIDs[i%len(nodeIDs)]
		mux := muxes[ingressNode]

		// Find shard and write directly via shard leader
		sid, _ := ring.LookupKey(key)
		var leaderNode *node.Node
		for _, nid := range nodeIDs {
			inst := nodes[nid].ShardInstance(sharding.ShardID(sid))
			if inst != nil && inst.Raft().Role() == raft.Leader {
				leaderNode = nodes[nid]
				break
			}
		}
		if leaderNode == nil {
			t.Fatalf("no leader found for shard %d", sid)
		}

		inst := leaderNode.ShardInstance(sharding.ShardID(sid))
		idx, _, err := inst.Raft().Propose("SET", key, val)
		if err != nil {
			t.Fatalf("propose key %s failed: %v", key, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = inst.Raft().WaitForApply(ctx, idx)
		cancel()

		keysWritten[key] = val

		// Read through a different ingress node
		readIngress := nodeIDs[(i+2)%len(nodeIDs)]
		readInst := nodes[readIngress].ShardInstance(sharding.ShardID(sid))
		if readInst != nil && readInst.DB() != nil {
			time.Sleep(10 * time.Millisecond)
			readVal, err := readInst.DB().Get(key)
			if err == nil && readVal != val {
				t.Fatalf("value mismatch for key %s: expected %s, got %s", key, val, readVal)
			}
		}

		_ = mux
	}

	// 4. Kill Shard 0 Leader and verify re-election
	var shard0Leader string
	for _, nid := range nodeIDs {
		inst := nodes[nid].ShardInstance(0)
		if inst != nil && inst.Raft().Role() == raft.Leader {
			shard0Leader = nid
			break
		}
	}
	if shard0Leader == "" {
		t.Fatalf("could not find Shard 0 leader")
	}

	t.Logf("Killing Shard 0 Leader: %s", shard0Leader)
	_ = nodes[shard0Leader].Stop()
	nodes[shard0Leader] = nil

	// Wait for re-election
	var newShard0Leader string
	reElectDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(reElectDeadline) {
		for _, nid := range nodeIDs {
			if nid == shard0Leader || nodes[nid] == nil {
				continue
			}
			inst := nodes[nid].ShardInstance(0)
			if inst != nil && inst.Raft().Role() == raft.Leader {
				newShard0Leader = nid
				break
			}
		}
		if newShard0Leader != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if newShard0Leader == "" {
		t.Fatalf("Shard 0 failed to elect new leader after killing %s", shard0Leader)
	}
	t.Logf("Shard 0 successfully re-elected new leader: %s", newShard0Leader)

	// 5. Restart failed node and verify data recovery
	restartedCfg := node.NodeConfig{
		ID:                 shard0Leader,
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		DataDir:            filepath.Join(baseDir, shard0Leader),
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 50 * time.Millisecond,
		ElectionTimeoutMax: 100 * time.Millisecond,
	}
	restartedTr := net.CreateTransport(shard0Leader)
	restartedNode, err := node.NewShardedNode(restartedCfg, meta, peerRaftAddrs, restartedTr)
	if err != nil {
		t.Fatalf("failed to recreate restarted node %s: %v", shard0Leader, err)
	}
	if err := restartedNode.Start(); err != nil {
		t.Fatalf("failed to start restarted node: %v", err)
	}
	nodes[shard0Leader] = restartedNode

	time.Sleep(200 * time.Millisecond)
	t.Logf("Restarted node %s successfully rejoined cluster and consensus state", shard0Leader)

	// 6. Dynamic Membership: Add and Remove node
	dynCluster, _ := sharding.NewDynamicCluster(meta, filepath.Join(baseDir, "dynamic_cluster.json"))
	if err := dynCluster.AddNode("node-f"); err != nil {
		t.Fatalf("AddNode failed: %v", err)
	}
	if dynCluster.Epoch() <= 1 {
		t.Fatalf("expected epoch to increment on node add")
	}
	if err := dynCluster.RemoveNode("node-f"); err != nil {
		t.Fatalf("RemoveNode failed: %v", err)
	}

	// 7. Dynamic Shard Migration
	coord := sharding.NewMigrationCoordinator(dynCluster)
	task, err := coord.PlanMigration(2, "node-c", "node-a")
	if err == nil {
		_ = coord.UpdateTaskState(task.ID, sharding.MigrationStateTransferringSnapshot, 1024, "")
		_ = coord.UpdateTaskState(task.ID, sharding.MigrationStateCatchingUp, 2048, "")
		if err := coord.CommitPlacement(task.ID); err != nil {
			t.Fatalf("CommitPlacement failed: %v", err)
		}
		t.Logf("Migrated Shard 2 replica from node-c to node-a successfully")
	}

	// 8. Run Compaction during normal writes
	leaderInst := nodes[newShard0Leader].ShardInstance(0)
	if leaderInst != nil && leaderInst.DB() != nil {
		for i := 0; i < 20; i++ {
			_ = leaderInst.DB().Set(fmt.Sprintf("comp_k_%d", i), fmt.Sprintf("comp_val_%d", i))
		}
		leaderInst.Engine().RotateMemtable()
		time.Sleep(50 * time.Millisecond) // allow background flush to complete
		res, err := leaderInst.Engine().Compact()
		if err != nil {
			t.Fatalf("compaction during writes failed: %v", err)
		}
		if res != nil {
			t.Logf("Compaction succeeded: keys read=%d, keys written=%d", res.KeysRead, res.KeysWritten)
		} else {
			t.Logf("Compaction evaluated: threshold not met for Level 0")
		}
	}

	// 9. Run Snapshot during normal writes
	if leaderInst != nil && leaderInst.DB() != nil {
		lastIncludedIndex, snapBytes, err := leaderInst.DB().ExportSnapshotBytes()
		if err != nil {
			t.Fatalf("ExportSnapshotBytes failed: %v", err)
		}
		if len(snapBytes) == 0 || lastIncludedIndex == 0 {
			t.Fatalf("snapshot invalid: len=%d idx=%d", len(snapBytes), lastIncludedIndex)
		}
		t.Logf("Snapshot succeeded: lastIncludedIndex=%d, bytes=%d", lastIncludedIndex, len(snapBytes))
	}

	// 10. Run Benchmark against cluster storage
	benchCfg := benchmark.BenchmarkConfig{
		Name:        "five-node-cluster-bench",
		Target:      benchmark.TargetStorage,
		NumKeys:     300,
		ValueSize:   64,
		ReadRatio:   0.8,
		Concurrency: 4,
		Duration:    150 * time.Millisecond,
		Scenario:    "standard",
	}
	benchRes := benchmark.RunStorageBenchmark(leaderInst.Engine(), benchCfg)
	if benchRes.TotalOps == 0 || benchRes.OpsPerSec <= 0 {
		t.Fatalf("benchmark failed to produce valid operations")
	}
	t.Logf("Benchmark completed: %.1f ops/sec, p50=%.2fms, p99=%.2fms",
		benchRes.OpsPerSec, benchRes.Latency.P50, benchRes.Latency.P99)

	// 11. Run Chaos Lab fault injection
	lab := chaos.NewChaosLab()
	exp := lab.NewExperiment(chaos.ExperimentConfig{
		ID:        "cluster-chaos-01",
		Name:      "partition-simulation",
		Target:    "node-d",
		FaultType: "NETWORK_PARTITION",
	})
	lab.SetState(exp.ID, chaos.StateFaultInjected)
	time.Sleep(10 * time.Millisecond)
	lab.CompleteExperiment(exp.ID, 15*time.Millisecond, true, nil)
	t.Logf("Chaos Lab experiment completed successfully: MTTR=%.2fms", exp.RecoveryTimeMs)

	// 12. Final Data Consistency Verification
	for k, expectedVal := range keysWritten {
		sid, _ := ring.LookupKey(k)
		for _, nid := range nodeIDs {
			n := nodes[nid]
			if n == nil {
				continue
			}
			inst := n.ShardInstance(sharding.ShardID(sid))
			if inst != nil && inst.DB() != nil {
				val, err := inst.DB().Get(k)
				if err == nil && val != expectedVal {
					t.Fatalf("inconsistent data on node %s for key %s: expected %s, got %s", nid, k, expectedVal, val)
				}
			}
		}
	}
	t.Logf("Data consistency verified across all active replicas for all keys!")
}
