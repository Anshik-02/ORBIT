// Package router implements shard-aware request routing for ORBIT.
// It maps client keys to their owning shards via consistent hashing,
// tracks leader identity per shard group, and forwards KV operations
// to the correct shard leader — locally or to a remote node.
package router

import (
	"fmt"

	"orbit/internal/sharding"
)

// ErrEmptyRing is returned when a key lookup is attempted on an empty hash ring.
var ErrEmptyRing = fmt.Errorf("router: hash ring is empty")

// ErrShardNotFound is returned when a shard ID is not present in the metadata.
var ErrShardNotFound = fmt.Errorf("router: shard not found in metadata")

// ShardRouter maps client keys to shards and locates their ShardInfo.
// It is safe for concurrent use; both the metadata and ring are read-only
// after construction (the ring itself is internally synchronised).
type ShardRouter struct {
	metadata *sharding.ClusterMetadata
	ring     *sharding.HashRing
}

// New creates a ShardRouter backed by the given cluster metadata and hash ring.
// Both arguments must be non-nil.
func New(metadata *sharding.ClusterMetadata, ring *sharding.HashRing) *ShardRouter {
	return &ShardRouter{
		metadata: metadata,
		ring:     ring,
	}
}

// GetShardID returns the ShardID responsible for the given key using
// consistent hashing via the underlying HashRing.
//
// Returns ErrEmptyRing when no shards have been added to the ring.
func (r *ShardRouter) GetShardID(key string) (sharding.ShardID, error) {
	id, err := r.ring.LookupKey(key)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrEmptyRing, err)
	}
	return id, nil
}

// GetShardInfo returns the ShardInfo (replica list) for the shard that owns the
// given key.  It first resolves the ShardID via the hash ring, then fetches the
// descriptor from the cluster metadata.
//
// Returns ErrEmptyRing when the ring is empty, or ErrShardNotFound when the
// shard ID returned by the ring is not registered in the metadata.
func (r *ShardRouter) GetShardInfo(key string) (*sharding.ShardInfo, error) {
	shardID, err := r.GetShardID(key)
	if err != nil {
		return nil, err
	}

	info, err := r.metadata.GetShard(shardID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrShardNotFound, err)
	}
	return info, nil
}
