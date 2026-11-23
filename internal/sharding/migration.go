package sharding

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MigrationState represents the current execution phase of a shard migration task.
type MigrationState string

const (
	MigrationStatePending              MigrationState = "PENDING"
	MigrationStateTransferringSnapshot MigrationState = "TRANSFERRING_SNAPSHOT"
	MigrationStateCatchingUp           MigrationState = "CATCHING_UP"
	MigrationStateCommittingPlacement  MigrationState = "COMMITTING_PLACEMENT"
	MigrationStateCompleted            MigrationState = "COMPLETED"
	MigrationStateFailed               MigrationState = "FAILED"
	MigrationStateAborted              MigrationState = "ABORTED"
)

// ShardMigrationTask tracks the migration of a shard replica from source to target node.
type ShardMigrationTask struct {
	ID          string         `json:"id"`
	ShardID     ShardID        `json:"shard_id"`
	SourceNode  string         `json:"source_node"`
	TargetNode  string         `json:"target_node"`
	State       MigrationState `json:"state"`
	BytesCopied int64          `json:"bytes_copied"`
	Error       string         `json:"error,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt time.Time      `json:"completed_at,omitempty"`
}

// MigrationCoordinator coordinates shard data transfers, log catch-up, and atomic placement commits.
type MigrationCoordinator struct {
	mu      sync.RWMutex
	cluster *DynamicCluster
	tasks   map[string]*ShardMigrationTask
	nextID  int
}

// NewMigrationCoordinator initializes a coordinator attached to dynamic cluster topology.
func NewMigrationCoordinator(cluster *DynamicCluster) *MigrationCoordinator {
	return &MigrationCoordinator{
		cluster: cluster,
		tasks:   make(map[string]*ShardMigrationTask),
		nextID:  1,
	}
}

// PlanMigration validates and registers a pending migration task.
func (mc *MigrationCoordinator) PlanMigration(shardID ShardID, sourceNode, targetNode string) (*ShardMigrationTask, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	meta := mc.cluster.Metadata()
	shard, err := meta.GetShard(shardID)
	if err != nil {
		return nil, err
	}

	if !shard.ContainsNode(sourceNode) {
		return nil, fmt.Errorf("source node %s does not host shard %s", sourceNode, shardID)
	}
	if shard.ContainsNode(targetNode) {
		return nil, fmt.Errorf("target node %s already hosts shard %s", targetNode, shardID)
	}

	// Verify target node exists in physical topology
	targetExists := false
	for _, n := range meta.PhysicalNodes {
		if n == targetNode {
			targetExists = true
			break
		}
	}
	if !targetExists {
		return nil, fmt.Errorf("target node %s not found in physical nodes", targetNode)
	}

	// Check if shard is already in migration
	for _, t := range mc.tasks {
		if t.ShardID == shardID && (t.State != MigrationStateCompleted && t.State != MigrationStateFailed && t.State != MigrationStateAborted) {
			return nil, fmt.Errorf("%w: shard %s task %s", ErrMigrationInProgress, shardID, t.ID)
		}
	}

	taskID := fmt.Sprintf("mig-%d-%s-%d", shardID, targetNode, mc.nextID)
	mc.nextID++

	task := &ShardMigrationTask{
		ID:         taskID,
		ShardID:    shardID,
		SourceNode: sourceNode,
		TargetNode: targetNode,
		State:      MigrationStatePending,
		StartedAt:  time.Now(),
	}
	mc.tasks[taskID] = task
	return task, nil
}

// UpdateTaskState updates the progress and state of an active migration task.
func (mc *MigrationCoordinator) UpdateTaskState(taskID string, state MigrationState, bytesCopied int64, errStr string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	task, ok := mc.tasks[taskID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrMigrationNotFound, taskID)
	}

	task.State = state
	task.BytesCopied = bytesCopied
	task.Error = errStr
	if state == MigrationStateCompleted || state == MigrationStateFailed || state == MigrationStateAborted {
		task.CompletedAt = time.Now()
	}
	return nil
}

// CommitPlacement finalizes the migration: replaces source with target in cluster metadata and increments Epoch.
func (mc *MigrationCoordinator) CommitPlacement(taskID string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	task, ok := mc.tasks[taskID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrMigrationNotFound, taskID)
	}

	meta := mc.cluster.Metadata()
	shard, err := meta.GetShard(task.ShardID)
	if err != nil {
		return err
	}

	var newReplicas []string
	replaced := false
	for _, r := range shard.Replicas {
		if r == task.SourceNode {
			newReplicas = append(newReplicas, task.TargetNode)
			replaced = true
		} else {
			newReplicas = append(newReplicas, r)
		}
	}

	if !replaced {
		return fmt.Errorf("source node %s no longer in replica set for shard %s", task.SourceNode, task.ShardID)
	}

	if err := mc.cluster.UpdateShardPlacement(task.ShardID, newReplicas); err != nil {
		task.State = MigrationStateFailed
		task.Error = err.Error()
		task.CompletedAt = time.Now()
		return fmt.Errorf("commit placement: %w", err)
	}

	task.State = MigrationStateCompleted
	task.CompletedAt = time.Now()
	return nil
}

// GetTask returns the current status of a migration task.
func (mc *MigrationCoordinator) GetTask(taskID string) (*ShardMigrationTask, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	task, ok := mc.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrMigrationNotFound, taskID)
	}
	cp := *task
	return &cp, nil
}

// ActiveTasks returns a snapshot list of all currently active migration tasks.
func (mc *MigrationCoordinator) ActiveTasks() []ShardMigrationTask {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	var result []ShardMigrationTask
	for _, t := range mc.tasks {
		if t.State != MigrationStateCompleted && t.State != MigrationStateFailed && t.State != MigrationStateAborted {
			result = append(result, *t)
		}
	}
	return result
}

// RebalancePlan represents an ordered set of migration moves to balance replica distribution.
type RebalancePlan struct {
	Moves []ShardMigrationTask `json:"moves"`
}

// CalculateRebalance analyzes current cluster topology and generates the minimum migration moves
// needed to achieve uniform replica distribution across all physical nodes.
func CalculateRebalance(meta *ClusterMetadata) (*RebalancePlan, error) {
	if meta == nil {
		return nil, errors.New("cluster metadata cannot be nil")
	}
	if err := meta.Validate(); err != nil {
		return nil, fmt.Errorf("validate metadata: %w", err)
	}

	// 1. Calculate per-node replica count
	nodeLoad := make(map[string]int)
	for _, n := range meta.PhysicalNodes {
		nodeLoad[n] = 0
	}
	for _, shard := range meta.Shards {
		for _, r := range shard.Replicas {
			nodeLoad[r]++
		}
	}

	// Simulated replica placements: map[ShardID][]string
	simulated := make(map[ShardID][]string, len(meta.Shards))
	for id, s := range meta.Shards {
		cp := make([]string, len(s.Replicas))
		copy(cp, s.Replicas)
		simulated[id] = cp
	}

	var moves []ShardMigrationTask
	moveIdx := 1

	// Iterate rebalancing until difference between max load and min load <= 1
	for {
		// Find most overloaded node and most underloaded node
		type nodePair struct {
			node string
			load int
		}
		var sortedNodes []nodePair
		for n, l := range nodeLoad {
			sortedNodes = append(sortedNodes, nodePair{node: n, load: l})
		}
		sort.Slice(sortedNodes, func(i, j int) bool {
			return sortedNodes[i].load > sortedNodes[j].load
		})

		maxNode := sortedNodes[0].node
		maxLoad := sortedNodes[0].load
		minNode := sortedNodes[len(sortedNodes)-1].node
		minLoad := sortedNodes[len(sortedNodes)-1].load

		if maxLoad-minLoad <= 1 {
			// Optimal balance achieved
			break
		}

		// Find a shard on maxNode that does not exist on minNode
		moved := false
		for id, reps := range simulated {
			hasMax := false
			hasMin := false
			for _, r := range reps {
				if r == maxNode {
					hasMax = true
				}
				if r == minNode {
					hasMin = true
				}
			}

			if hasMax && !hasMin {
				// Eligible for move
				var newReps []string
				for _, r := range reps {
					if r == maxNode {
						newReps = append(newReps, minNode)
					} else {
						newReps = append(newReps, r)
					}
				}
				simulated[id] = newReps
				nodeLoad[maxNode]--
				nodeLoad[minNode]++

				moves = append(moves, ShardMigrationTask{
					ID:         fmt.Sprintf("rebalance-%d-%s-to-%s", moveIdx, maxNode, minNode),
					ShardID:    id,
					SourceNode: maxNode,
					TargetNode: minNode,
					State:      MigrationStatePending,
					StartedAt:  time.Now(),
				})
				moveIdx++
				moved = true
				break
			}
		}

		if !moved {
			// Cannot balance further due to replication constraints
			break
		}
	}

	return &RebalancePlan{
		Moves: moves,
	}, nil
}

// ExecuteRebalance executes all migration tasks in plan sequentially through the coordinator.
func (mc *MigrationCoordinator) ExecuteRebalance(ctx context.Context, plan *RebalancePlan) error {
	if plan == nil {
		return nil
	}

	for _, move := range plan.Moves {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		task, err := mc.PlanMigration(move.ShardID, move.SourceNode, move.TargetNode)
		if err != nil {
			return fmt.Errorf("plan migration %s: %w", move.ID, err)
		}

		// Simulate snapshot transfer & catchup
		_ = mc.UpdateTaskState(task.ID, MigrationStateTransferringSnapshot, 1024, "")
		_ = mc.UpdateTaskState(task.ID, MigrationStateCatchingUp, 2048, "")

		// Commit placement
		if err := mc.CommitPlacement(task.ID); err != nil {
			return fmt.Errorf("commit migration %s: %w", task.ID, err)
		}
	}

	return nil
}
