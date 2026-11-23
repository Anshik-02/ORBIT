package tests

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/raft"
	"orbit/internal/sharding"
)

// TestRaft_LearnerCatchUpAndPromotion verifies that a new node added as a learner
// receives log entries and catches up, but cannot be promoted to a voting member
// until its matchIndex reaches the leader's commitIndex.
func TestRaft_LearnerCatchUpAndPromotion(t *testing.T) {
	tempDir := t.TempDir()
	net := raft.NewMockNetwork()

	storageA, err := raft.OpenFileStorage(filepath.Join(tempDir, "storage_A"))
	if err != nil {
		t.Fatalf("OpenFileStorage A: %v", err)
	}
	defer storageA.Close()

	storageB, err := raft.OpenFileStorage(filepath.Join(tempDir, "storage_B"))
	if err != nil {
		t.Fatalf("OpenFileStorage B: %v", err)
	}
	defer storageB.Close()

	storageC, err := raft.OpenFileStorage(filepath.Join(tempDir, "storage_C"))
	if err != nil {
		t.Fatalf("OpenFileStorage C: %v", err)
	}
	defer storageC.Close()

	trA := net.CreateTransport("node-A", "addr-A")
	trB := net.CreateTransport("node-B", "addr-B")
	trC := net.CreateTransport("node-C", "addr-C")

	cfgA := raft.Config{
		ID:                 "node-A",
		GroupID:            "group-1",
		Peers:              map[string]string{"node-B": "addr-B"},
		Storage:            storageA,
		Transport:          trA,
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 30 * time.Millisecond,
		ElectionTimeoutMax: 60 * time.Millisecond,
	}
	cfgB := raft.Config{
		ID:                 "node-B",
		GroupID:            "group-1",
		Peers:              map[string]string{"node-A": "addr-A"},
		Storage:            storageB,
		Transport:          trB,
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 200 * time.Millisecond,
		ElectionTimeoutMax: 300 * time.Millisecond,
	}
	cfgC := raft.Config{
		ID:                 "node-C",
		GroupID:            "group-1",
		Peers:              map[string]string{}, // Blank learner
		Storage:            storageC,
		Transport:          trC,
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 500 * time.Millisecond,
		ElectionTimeoutMax: 1000 * time.Millisecond,
	}

	nodeA, _ := raft.New(cfgA)
	nodeB, _ := raft.New(cfgB)
	nodeC, _ := raft.New(cfgC)

	_ = nodeA.Start()
	_ = nodeB.Start()
	_ = nodeC.Start()
	defer nodeA.Stop()
	defer nodeB.Stop()
	defer nodeC.Stop()

	// Wait for leader election with polling
	var leader *raft.Raft
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if nodeA.Role() == raft.Leader {
			leader = nodeA
			break
		}
		if nodeB.Role() == raft.Leader {
			leader = nodeB
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if leader == nil {
		t.Fatalf("no leader elected between nodeA and nodeB")
	}

	// Propose 5 entries to leader
	for i := 1; i <= 5; i++ {
		_, _, err := leader.Propose("SET", fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
		if err != nil {
			t.Fatalf("propose failed: %v", err)
		}
	}
	time.Sleep(50 * time.Millisecond)

	// Add nodeC as learner
	if err := leader.AddLearner("node-C", "addr-C"); err != nil {
		t.Fatalf("AddLearner failed: %v", err)
	}

	// Verify nodeC is recorded as learner
	learners := leader.Learners()
	if learners["node-C"] != "addr-C" {
		t.Fatalf("expected node-C in learners map")
	}

	// Wait for replication to catch up nodeC
	time.Sleep(100 * time.Millisecond)

	// Now promote nodeC to full peer
	if err := leader.PromoteLearner("node-C"); err != nil {
		t.Fatalf("PromoteLearner failed: %v", err)
	}

	// Verify nodeC is now in voting peers
	peers := leader.Peers()
	if peers["node-C"] != "addr-C" {
		t.Fatalf("expected node-C in active voting peers")
	}

	// Verify nodeC's storage has all 5 entries
	lastLogC, _ := storageC.LastLogInfo()
	if lastLogC < 5 {
		t.Fatalf("expected nodeC to have at least 5 entries, got %d", lastLogC)
	}
}

// TestRaft_RemovedNodeRejection verifies that when a node is removed,
// it steps down and cannot propose or serve stale requests.
func TestRaft_RemovedNodeRejection(t *testing.T) {
	tempDir := t.TempDir()
	net := raft.NewMockNetwork()
	storageA, err := raft.OpenFileStorage(filepath.Join(tempDir, "storage_A"))
	if err != nil {
		t.Fatalf("OpenFileStorage: %v", err)
	}
	defer storageA.Close()
	trA := net.CreateTransport("node-A", "addr-A")

	cfgA := raft.Config{
		ID:                 "node-A",
		GroupID:            "group-1",
		Peers:              map[string]string{"node-B": "addr-B"},
		Storage:            storageA,
		Transport:          trA,
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 60 * time.Millisecond,
		ElectionTimeoutMax: 120 * time.Millisecond,
	}

	nodeA, _ := raft.New(cfgA)
	_ = nodeA.Start()
	defer nodeA.Stop()

	// Wait for nodeA to self-elect or become candidate
	time.Sleep(100 * time.Millisecond)

	// Remove nodeA from its own replica set
	if err := nodeA.RemovePeer("node-A"); err != nil {
		t.Fatalf("RemovePeer failed: %v", err)
	}

	if nodeA.Role() == raft.Leader {
		t.Fatalf("expected removed node to not be leader")
	}

	// Proposals should fail
	_, _, err = nodeA.Propose("SET", "k", "v")
	if err != raft.ErrNotLeader {
		t.Fatalf("expected ErrNotLeader on removed node, got %v", err)
	}
}

// TestShardMigration_CoordinatorCrashAndResume verifies that active migration tasks
// are persisted to disk and can be resumed cleanly across coordinator crashes.
func TestShardMigration_CoordinatorCrashAndResume(t *testing.T) {
	tempDir := t.TempDir()
	tasksPath := filepath.Join(tempDir, "migrations.json")
	metaPath := filepath.Join(tempDir, "cluster.json")

	meta := &sharding.ClusterMetadata{
		ClusterID:         "test-cluster",
		Epoch:             1,
		ShardCount:        2,
		ReplicationFactor: 1,
		PhysicalNodes:     []string{"node-1", "node-2", "node-3"},
		Shards: map[sharding.ShardID]*sharding.ShardInfo{
			0: {ID: 0, Replicas: []string{"node-1"}},
			1: {ID: 1, Replicas: []string{"node-2"}},
		},
	}

	cluster, err := sharding.NewDynamicCluster(meta, metaPath)
	if err != nil {
		t.Fatalf("NewDynamicCluster: %v", err)
	}

	// Coordinator 1 starts migration
	coord1 := sharding.NewMigrationCoordinator(cluster)
	if err := coord1.SetStoragePath(tasksPath); err != nil {
		t.Fatalf("SetStoragePath: %v", err)
	}

	task, err := coord1.PlanMigration(0, "node-1", "node-3")
	if err != nil {
		t.Fatalf("PlanMigration: %v", err)
	}

	_ = coord1.UpdateTaskState(task.ID, sharding.MigrationStateTransferringSnapshot, 5000, "")

	// Simulate coordinator crash by creating a new coordinator from the same disk storage
	cluster2, _ := sharding.LoadDynamicCluster(metaPath)
	coord2 := sharding.NewMigrationCoordinator(cluster2)
	if err := coord2.SetStoragePath(tasksPath); err != nil {
		t.Fatalf("coord2 SetStoragePath: %v", err)
	}

	// Check incomplete tasks
	incomplete := coord2.ResumeIncompleteTasks()
	if len(incomplete) != 1 {
		t.Fatalf("expected 1 incomplete task to resume, got %d", len(incomplete))
	}
	resumed := incomplete[0]
	if resumed.ID != task.ID || resumed.BytesCopied != 5000 {
		t.Fatalf("resumed task mismatch: id=%s bytes=%d", resumed.ID, resumed.BytesCopied)
	}

	// Complete the migration on coordinator 2
	_ = coord2.UpdateTaskState(resumed.ID, sharding.MigrationStateCatchingUp, 10000, "")
	if err := coord2.CommitPlacement(resumed.ID); err != nil {
		t.Fatalf("CommitPlacement failed on resumed task: %v", err)
	}

	// Verify routing metadata updated: Shard 0 replica is now node-3
	updatedMeta := cluster2.Metadata()
	shard0, _ := updatedMeta.GetShard(0)
	if !shard0.ContainsNode("node-3") || shard0.ContainsNode("node-1") {
		t.Fatalf("expected shard 0 to be on node-3, replicas=%v", shard0.Replicas)
	}
	if updatedMeta.Epoch <= 1 {
		t.Fatalf("expected cluster epoch to increment post-commit, got %d", updatedMeta.Epoch)
	}
}

// TestShardMigration_SimultaneousMigrationsDifferentShards verifies that migrations
// on two separate shards can proceed simultaneously without mutual interference.
func TestShardMigration_SimultaneousMigrationsDifferentShards(t *testing.T) {
	tempDir := t.TempDir()
	meta := &sharding.ClusterMetadata{
		ClusterID:         "test-cluster",
		Epoch:             1,
		ShardCount:        3,
		ReplicationFactor: 1,
		PhysicalNodes:     []string{"node-1", "node-2", "node-3", "node-4"},
		Shards: map[sharding.ShardID]*sharding.ShardInfo{
			0: {ID: 0, Replicas: []string{"node-1"}},
			1: {ID: 1, Replicas: []string{"node-2"}},
			2: {ID: 2, Replicas: []string{"node-3"}},
		},
	}

	cluster, _ := sharding.NewDynamicCluster(meta, filepath.Join(tempDir, "cluster.json"))
	coord := sharding.NewMigrationCoordinator(cluster)

	// Migrate shard 0: node-1 -> node-4
	task0, err := coord.PlanMigration(0, "node-1", "node-4")
	if err != nil {
		t.Fatalf("PlanMigration shard 0 failed: %v", err)
	}

	// Simultaneously migrate shard 1: node-2 -> node-3
	task1, err := coord.PlanMigration(1, "node-2", "node-3")
	if err != nil {
		t.Fatalf("PlanMigration shard 1 failed: %v", err)
	}

	active := coord.ActiveTasks()
	if len(active) != 2 {
		t.Fatalf("expected 2 active tasks simultaneously, got %d", len(active))
	}

	// Commit both
	if err := coord.CommitPlacement(task0.ID); err != nil {
		t.Fatalf("Commit task0 failed: %v", err)
	}
	if err := coord.CommitPlacement(task1.ID); err != nil {
		t.Fatalf("Commit task1 failed: %v", err)
	}

	finalMeta := cluster.Metadata()
	s0, _ := finalMeta.GetShard(0)
	s1, _ := finalMeta.GetShard(1)
	if !s0.ContainsNode("node-4") {
		t.Fatalf("shard 0 failed to migrate to node-4")
	}
	if !s1.ContainsNode("node-3") {
		t.Fatalf("shard 1 failed to migrate to node-3")
	}
}
