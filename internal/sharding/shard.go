// Package sharding implements the consistent hashing ring, shard metadata, and
// placement model for ORBIT's distributed multi-shard architecture.
//
// # Architecture
//
// ORBIT's sharding layer sits between the client-facing request router and the
// per-shard Raft consensus groups.  Its sole responsibility at this milestone is
// to answer two questions:
//
//  1. Which shard owns a given key?
//  2. Which physical nodes host replicas of that shard?
//
// It does not (yet) manage Raft group membership or trigger data migration.
package sharding

import "fmt"

// ShardID is an opaque, stable integer identifier for a logical shard.
// IDs are assigned at cluster bootstrap and never change, even when the
// cluster topology changes.  Stability is required so that Raft log entries
// can reference a shard by ID without ambiguity across restarts.
type ShardID uint32

// String returns the canonical human-readable representation of a shard ID.
func (s ShardID) String() string {
	return fmt.Sprintf("shard-%d", uint32(s))
}

// ShardInfo describes the logical identity and replica assignment of one shard.
// It is the authoritative descriptor stored in ClusterMetadata and distributed
// to all physical nodes.
type ShardInfo struct {
	// ID is the unique, immutable identifier for this shard.
	ID ShardID

	// Replicas is an ordered list of physical node IDs that maintain a full
	// replica of this shard's log and state machine.  The first node listed is
	// the preferred initial leader candidate; Raft elections will always elect
	// the most up-to-date node regardless of this order.
	Replicas []string

	// MinKey and MaxKey document the logical key range this shard is responsible
	// for.  These are informational when using consistent hashing (the ring itself
	// is authoritative), but are useful for diagnostics and future range-query
	// optimisations.
	MinKey string
	MaxKey string
}

// ReplicaCount returns the configured number of replicas for this shard.
func (s *ShardInfo) ReplicaCount() int {
	return len(s.Replicas)
}

// ContainsNode reports whether the given physical node ID is a replica member
// of this shard.
func (s *ShardInfo) ContainsNode(nodeID string) bool {
	for _, r := range s.Replicas {
		if r == nodeID {
			return true
		}
	}
	return false
}
