package sharding

import (
	"fmt"
	"sort"
)

// ClusterMetadata is the authoritative, cluster-wide configuration that describes
// how the keyspace is partitioned into shards and which physical nodes host each
// shard's replicas.
//
// # Fields
//
//   - ClusterID: a stable string identifier for the cluster (e.g. "orbit-cluster-01").
//     Must not be empty.
//   - Epoch: a monotonically increasing version number.  Every time the topology
//     changes (node addition, shard migration), the epoch is incremented.  All
//     nodes compare epochs to detect stale routing decisions.
//
// # Validity invariants (enforced by Validate)
//
//  1. ClusterID must not be empty.
//  2. ShardCount must be >= 1 and equal len(Shards).
//  3. PhysicalNodes must not be empty; no ID may be empty or duplicated.
//  4. Shards map must not be nil.
//  5. Every ShardInfo must not be nil and its ID must match its map key.
//  6. ReplicationFactor must be in [1, len(PhysicalNodes)].
//  7. Every ShardInfo.Replicas must have exactly ReplicationFactor entries.
//  8. No shard may list the same replica node twice.
//  9. Every replica must appear in PhysicalNodes.
type ClusterMetadata struct {
	// ClusterID is the stable, human-readable name for this cluster.
	ClusterID string

	// Epoch is the topology version.  Incremented on every topology change.
	Epoch uint64

	// ShardCount is the total number of logical shards.  Must equal len(Shards).
	ShardCount int

	// ReplicationFactor is the number of replicas per shard.
	ReplicationFactor int

	// Shards maps ShardID -> ShardInfo for every shard in the cluster.
	Shards map[ShardID]*ShardInfo

	// PhysicalNodes is the ordered list of all physical node IDs in the cluster.
	PhysicalNodes []string
}

// NewClusterMetadata constructs a ClusterMetadata with an empty shard map.
func NewClusterMetadata(clusterID string, epoch uint64, shardCount, replicationFactor int, physicalNodes []string) *ClusterMetadata {
	return &ClusterMetadata{
		ClusterID:         clusterID,
		Epoch:             epoch,
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
	// 1. ClusterID must not be empty.
	if m.ClusterID == "" {
		return fmt.Errorf("%w: cluster ID must not be empty", ErrInvalidShardID)
	}

	// 2. ShardCount >= 1.
	if m.ShardCount < 1 {
		return fmt.Errorf("%w: shard count must be at least 1, got %d", ErrInvalidShardID, m.ShardCount)
	}

	// 3. Shard map must not be nil.
	if m.Shards == nil {
		return fmt.Errorf("%w: Shards map must not be nil", ErrInvalidShardID)
	}

	// 4. ShardCount must match len(Shards).
	if m.ShardCount != len(m.Shards) {
		return fmt.Errorf("%w: declared ShardCount %d does not match len(Shards) %d",
			ErrInvalidShardID, m.ShardCount, len(m.Shards))
	}

	// 5. PhysicalNodes must not be empty; each ID must be non-empty and unique.
	if len(m.PhysicalNodes) == 0 {
		return fmt.Errorf("%w: physical node list must not be empty", ErrInvalidPlacement)
	}
	nodeSet := make(map[string]bool, len(m.PhysicalNodes))
	for _, n := range m.PhysicalNodes {
		if n == "" {
			return fmt.Errorf("%w: physical node ID must not be empty", ErrInvalidPlacement)
		}
		if nodeSet[n] {
			return fmt.Errorf("%w: duplicate physical node ID %q", ErrInvalidPlacement, n)
		}
		nodeSet[n] = true
	}

	// 6. ReplicationFactor must be in [1, len(PhysicalNodes)].
	if m.ReplicationFactor < 1 {
		return fmt.Errorf("%w: must be at least 1, got %d", ErrInvalidReplicationFactor, m.ReplicationFactor)
	}
	if m.ReplicationFactor > len(m.PhysicalNodes) {
		return fmt.Errorf("%w: factor %d exceeds physical node count %d",
			ErrInvalidReplicationFactor, m.ReplicationFactor, len(m.PhysicalNodes))
	}

	// 7–9. Validate every registered shard.
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
			if replica == "" {
				return fmt.Errorf("%w: shard %s has empty replica node ID", ErrInvalidPlacement, id)
			}
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

// AssignReplicas generates a balanced, deterministic replica assignment for
// shardCount shards with the given replicationFactor across physicalNodes.
//
// # Placement algorithm
//
// Shard i receives replicas from nodes at positions:
//
//	[(i*rf + r) % n]  for r in [0, rf)
//
// where n = len(physicalNodes) and rf = replicationFactor.
//
// Example — 5 nodes (A B C D E), 3 shards, rf=3:
//
//	Shard 0 → A, B, C   (positions 0, 1, 2)
//	Shard 1 → D, E, A   (positions 3, 4, 0)
//	Shard 2 → B, C, D   (positions 1, 2, 3)
//
// This guarantees:
//   - Each shard gets exactly replicationFactor distinct replicas.
//   - No shard has the same physical node twice.
//   - Shard load is spread as evenly as possible across all physical nodes.
//
// # Validation
//
// Returns errors for:
//   - replicationFactor < 1 or > len(physicalNodes)
//   - shardCount < 1
//   - empty physical node IDs
//   - duplicate physical node IDs
func AssignReplicas(shardCount, replicationFactor int, physicalNodes []string) (map[ShardID][]string, error) {
	if replicationFactor < 1 {
		return nil, ErrInvalidReplicationFactor
	}
	if shardCount < 1 {
		return nil, fmt.Errorf("%w: shard count must be >= 1", ErrInvalidShardID)
	}
	if len(physicalNodes) == 0 {
		return nil, fmt.Errorf("%w: physical node list must not be empty", ErrInvalidPlacement)
	}
	// Reject empty or duplicate node IDs.
	nodeSet := make(map[string]bool, len(physicalNodes))
	for _, n := range physicalNodes {
		if n == "" {
			return nil, fmt.Errorf("%w: physical node ID must not be empty", ErrInvalidPlacement)
		}
		if nodeSet[n] {
			return nil, fmt.Errorf("%w: duplicate physical node ID %q", ErrInvalidPlacement, n)
		}
		nodeSet[n] = true
	}
	if replicationFactor > len(physicalNodes) {
		return nil, fmt.Errorf("%w: factor %d exceeds node count %d",
			ErrInvalidPlacement, replicationFactor, len(physicalNodes))
	}

	n := len(physicalNodes)
	rf := replicationFactor
	assignment := make(map[ShardID][]string, shardCount)
	for i := 0; i < shardCount; i++ {
		replicas := make([]string, rf)
		for r := 0; r < rf; r++ {
			replicas[r] = physicalNodes[(i*rf+r)%n]
		}
		assignment[ShardID(i)] = replicas
	}
	return assignment, nil
}
