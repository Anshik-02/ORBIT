package sharding

import (
	"fmt"
	"sort"
)

// ClusterMetadata is the authoritative, cluster-wide configuration that describes
// how the keyspace is partitioned into shards and which physical nodes host each
// shard's replicas.
//
// At Milestone 3.0 the metadata is static (set at bootstrap and read-only during
// normal operation).  Dynamic membership changes and shard migration will be
// introduced in later milestones.
//
// # Validity invariants (enforced by Validate)
//
//  1. ShardCount must equal len(Shards).
//  2. Every ShardInfo.ID must be unique within Shards.
//  3. ReplicationFactor must be in [1, number of distinct node IDs].
//  4. Every ShardInfo.Replicas must have exactly ReplicationFactor entries.
//  5. Every replica listed in ShardInfo.Replicas must appear in PhysicalNodes.
//  6. ShardCount must be >= 1.
type ClusterMetadata struct {
	// ShardCount is the total number of logical shards in the cluster.
	// Must equal len(Shards).
	ShardCount int

	// ReplicationFactor is the number of physical nodes that maintain a full
	// replica of each shard.  Must be <= len(PhysicalNodes).
	ReplicationFactor int

	// Shards maps ShardID -> ShardInfo for every shard in the cluster.
	Shards map[ShardID]*ShardInfo

	// PhysicalNodes is the set of all physical node IDs participating in the
	// cluster.  Every node listed in any ShardInfo.Replicas must appear here.
	PhysicalNodes []string
}

// NewClusterMetadata constructs a ClusterMetadata with an empty shard map.
func NewClusterMetadata(shardCount, replicationFactor int, physicalNodes []string) *ClusterMetadata {
	return &ClusterMetadata{
		ShardCount:        shardCount,
		ReplicationFactor: replicationFactor,
		Shards:            make(map[ShardID]*ShardInfo, shardCount),
		PhysicalNodes:     physicalNodes,
	}
}

// AddShard registers a ShardInfo.  Returns ErrDuplicateShard if the shard is
// already registered, or ErrInvalidShardID if the info is nil.
func (m *ClusterMetadata) AddShard(info *ShardInfo) error {
	if info == nil {
		return ErrInvalidShardID
	}
	if _, exists := m.Shards[info.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateShard, info.ID)
	}
	m.Shards[info.ID] = info
	return nil
}

// GetShard returns the ShardInfo for the given ID, or ErrShardNotFound.
func (m *ClusterMetadata) GetShard(id ShardID) (*ShardInfo, error) {
	info, ok := m.Shards[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrShardNotFound, id)
	}
	return info, nil
}

// ShardsForNode returns a slice of all ShardInfos that include nodeID as a replica.
// The slice is sorted by ShardID for deterministic ordering.
func (m *ClusterMetadata) ShardsForNode(nodeID string) []*ShardInfo {
	var result []*ShardInfo
	for _, info := range m.Shards {
		if info.ContainsNode(nodeID) {
			result = append(result, info)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// Validate checks all invariants documented on ClusterMetadata.
// Returns the first validation error encountered, or nil if the metadata is consistent.
func (m *ClusterMetadata) Validate() error {
	if m.ShardCount < 1 {
		return fmt.Errorf("%w: shard count must be at least 1, got %d", ErrInvalidShardID, m.ShardCount)
	}
	if m.ShardCount != len(m.Shards) {
		return fmt.Errorf("%w: declared ShardCount %d does not match len(Shards) %d",
			ErrInvalidShardID, m.ShardCount, len(m.Shards))
	}
	if m.ReplicationFactor < 1 {
		return fmt.Errorf("%w: must be at least 1, got %d", ErrInvalidReplicationFactor, m.ReplicationFactor)
	}
	if m.ReplicationFactor > len(m.PhysicalNodes) {
		return fmt.Errorf("%w: factor %d exceeds physical node count %d",
			ErrInvalidReplicationFactor, m.ReplicationFactor, len(m.PhysicalNodes))
	}

	// Build a set of valid physical nodes for O(1) lookup.
	nodeSet := make(map[string]bool, len(m.PhysicalNodes))
	for _, n := range m.PhysicalNodes {
		nodeSet[n] = true
	}

	// Validate every registered shard.
	for id, info := range m.Shards {
		if info == nil {
			return fmt.Errorf("%w: nil ShardInfo for %s", ErrInvalidShardID, id)
		}
		if info.ID != id {
			return fmt.Errorf("%w: map key %s does not match ShardInfo.ID %s",
				ErrInvalidShardID, id, info.ID)
		}
		if len(info.Replicas) != m.ReplicationFactor {
			return fmt.Errorf("%w: shard %s has %d replicas, want %d",
				ErrInvalidPlacement, id, len(info.Replicas), m.ReplicationFactor)
		}
		seen := make(map[string]bool, len(info.Replicas))
		for _, replica := range info.Replicas {
			if !nodeSet[replica] {
				return fmt.Errorf("%w: shard %s references unknown node %q",
					ErrInvalidPlacement, id, replica)
			}
			if seen[replica] {
				return fmt.Errorf("%w: shard %s lists node %q more than once",
					ErrInvalidPlacement, id, replica)
			}
			seen[replica] = true
		}
	}

	return nil
}

// ─── Placement helper ─────────────────────────────────────────────────────────

// AssignReplicas generates a balanced replica assignment across physicalNodes for
// shardCount shards with the given replicationFactor.
//
// The assignment is deterministic: shard i receives replicas from nodes at
// positions [i*rf % n, (i*rf+1) % n, ..., (i*rf+rf-1) % n] where n is the
// number of physical nodes and rf is the replication factor.
//
// This simple round-robin strategy guarantees:
//   - Each shard gets exactly replicationFactor replicas.
//   - No shard has duplicate replicas.
//   - Load is spread as evenly as possible across nodes.
//
// Returns ErrInvalidPlacement if replicationFactor > len(physicalNodes), or
// ErrInvalidReplicationFactor if replicationFactor < 1.
func AssignReplicas(shardCount, replicationFactor int, physicalNodes []string) (map[ShardID][]string, error) {
	if replicationFactor < 1 {
		return nil, ErrInvalidReplicationFactor
	}
	if replicationFactor > len(physicalNodes) {
		return nil, fmt.Errorf("%w: factor %d exceeds node count %d",
			ErrInvalidPlacement, replicationFactor, len(physicalNodes))
	}
	if shardCount < 1 {
		return nil, fmt.Errorf("%w: shard count must be >= 1", ErrInvalidShardID)
	}

	n := len(physicalNodes)
	assignment := make(map[ShardID][]string, shardCount)
	for i := 0; i < shardCount; i++ {
		replicas := make([]string, replicationFactor)
		for r := 0; r < replicationFactor; r++ {
			replicas[r] = physicalNodes[(i+r)%n]
		}
		assignment[ShardID(i)] = replicas
	}
	return assignment, nil
}
