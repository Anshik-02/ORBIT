package tests

import (
	"context"
	"path/filepath"
	"testing"

	"orbit/internal/sharding"
)

// TestDynamicCluster_NodeMembershipAndPersistence verifies node addition, removal,
// epoch incrementing, and persistence to disk.
func TestDynamicCluster_NodeMembershipAndPersistence(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "cluster_metadata.json")

	nodes := []string{"n1", "n2", "n3"}
	meta := sharding.NewClusterMetadata("orbit-test", 1, 1, 2, nodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"n1", "n2"}})

	dc, err := sharding.NewDynamicCluster(meta, metaPath)
	if err != nil {
		t.Fatalf("NewDynamicCluster: %v", err)
	}

	if dc.Epoch() != 1 {
		t.Errorf("expected initial epoch 1, got %d", dc.Epoch())
	}

	// 1. Add node n4
	if err := dc.AddNode("n4"); err != nil {
		t.Fatalf("AddNode n4 failed: %v", err)
	}
	if dc.Epoch() != 2 {
		t.Errorf("expected epoch 2 after AddNode, got %d", dc.Epoch())
	}

	// Adding duplicate node should fail
	if err := dc.AddNode("n4"); err == nil {
		t.Errorf("expected error adding duplicate node n4, got nil")
	}

	// 2. Try removing node n1 (which hosts shard 0) - must fail with ErrNodeInUse
	if err := dc.RemoveNode("n1"); err == nil {
		t.Errorf("expected error removing node n1 in use, got nil")
	}

	// 3. Remove idle node n4
	if err := dc.RemoveNode("n4"); err != nil {
		t.Fatalf("RemoveNode n4 failed: %v", err)
	}
	if dc.Epoch() != 3 {
		t.Errorf("expected epoch 3 after RemoveNode, got %d", dc.Epoch())
	}

	// 4. Reload from disk
	loadedDC, err := sharding.LoadDynamicCluster(metaPath)
	if err != nil {
		t.Fatalf("LoadDynamicCluster failed: %v", err)
	}
	if loadedDC.Epoch() != 3 {
		t.Errorf("expected loaded epoch 3, got %d", loadedDC.Epoch())
	}
	if len(loadedDC.Metadata().PhysicalNodes) != 3 {
		t.Errorf("expected 3 physical nodes, got %d", len(loadedDC.Metadata().PhysicalNodes))
	}
}

// TestMigrationCoordinator_PlanAndCommit verifies shard migration workflow and metadata updates.
func TestMigrationCoordinator_PlanAndCommit(t *testing.T) {
	nodes := []string{"n1", "n2", "n3", "n4"}
	meta := sharding.NewClusterMetadata("orbit-test", 1, 1, 2, nodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"n1", "n2"}})

	dc, err := sharding.NewDynamicCluster(meta, "")
	if err != nil {
		t.Fatalf("NewDynamicCluster: %v", err)
	}

	coord := sharding.NewMigrationCoordinator(dc)

	// 1. Plan migration of shard 0 from n2 to n3
	task, err := coord.PlanMigration(0, "n2", "n3")
	if err != nil {
		t.Fatalf("PlanMigration failed: %v", err)
	}
	if task.State != sharding.MigrationStatePending {
		t.Errorf("expected state PENDING, got %s", task.State)
	}

	// Planning another migration for shard 0 concurrently must fail
	if _, err := coord.PlanMigration(0, "n1", "n4"); err == nil {
		t.Errorf("expected error planning concurrent migration on shard 0")
	}

	// 2. Update progress
	_ = coord.UpdateTaskState(task.ID, sharding.MigrationStateTransferringSnapshot, 5000, "")
	_ = coord.UpdateTaskState(task.ID, sharding.MigrationStateCatchingUp, 7500, "")

	// 3. Commit placement
	if err := coord.CommitPlacement(task.ID); err != nil {
		t.Fatalf("CommitPlacement failed: %v", err)
	}

	// Verify shard 0 replicas are now [n1, n3]
	updatedShard, _ := dc.Metadata().GetShard(0)
	if !updatedShard.ContainsNode("n1") || !updatedShard.ContainsNode("n3") || updatedShard.ContainsNode("n2") {
		t.Errorf("unexpected updated replicas: %+v", updatedShard.Replicas)
	}

	if dc.Epoch() != 2 {
		t.Errorf("expected epoch 2 after commit, got %d", dc.Epoch())
	}
}

// TestRebalancing_CalculationAndExecution verifies that an uneven replica distribution is detected,
// optimal migration moves are generated, and executing them balances the cluster.
func TestRebalancing_CalculationAndExecution(t *testing.T) {
	// 5 physical nodes: n1, n2, n3, n4, n5
	nodes := []string{"n1", "n2", "n3", "n4", "n5"}

	// 3 shards with RF=3 (total 9 replicas)
	// Artificially create an unbalanced placement:
	// Shard 0 -> n1, n2, n3
	// Shard 1 -> n1, n2, n3
	// Shard 2 -> n1, n2, n4
	// Load: n1: 3, n2: 3, n3: 2, n4: 1, n5: 0 (heavily skewed!)
	meta := sharding.NewClusterMetadata("orbit-test", 1, 3, 3, nodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"n1", "n2", "n3"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"n1", "n2", "n3"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 2, Replicas: []string{"n1", "n2", "n4"}})

	dc, err := sharding.NewDynamicCluster(meta, "")
	if err != nil {
		t.Fatalf("NewDynamicCluster: %v", err)
	}

	coord := sharding.NewMigrationCoordinator(dc)

	// 1. Calculate rebalance plan
	plan, err := sharding.CalculateRebalance(dc.Metadata())
	if err != nil {
		t.Fatalf("CalculateRebalance failed: %v", err)
	}

	if len(plan.Moves) == 0 {
		t.Fatalf("expected rebalance moves for skewed cluster, got 0")
	}

	// 2. Execute rebalance plan
	if err := coord.ExecuteRebalance(context.Background(), plan); err != nil {
		t.Fatalf("ExecuteRebalance failed: %v", err)
	}

	// 3. Verify final load distribution: max load - min load <= 1
	finalMeta := dc.Metadata()
	load := make(map[string]int)
	for _, n := range finalMeta.PhysicalNodes {
		load[n] = 0
	}
	for _, s := range finalMeta.Shards {
		for _, r := range s.Replicas {
			load[r]++
		}
	}

	maxLoad := 0
	minLoad := 999
	for _, count := range load {
		if count > maxLoad {
			maxLoad = count
		}
		if count < minLoad {
			minLoad = count
		}
	}

	if maxLoad-minLoad > 1 {
		t.Errorf("cluster not balanced after execution: loads=%+v (max=%d, min=%d)",
			load, maxLoad, minLoad)
	}
}
