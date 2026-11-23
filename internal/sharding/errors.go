package sharding

import "errors"

// Sentinel errors for the sharding layer.
var (
	// ErrInvalidShardID is returned when a provided ShardID is out of range or malformed.
	ErrInvalidShardID = errors.New("sharding: invalid shard ID")

	// ErrShardNotFound is returned when no shard exists for a given lookup key or ID.
	ErrShardNotFound = errors.New("sharding: shard not found")

	// ErrDuplicateShard is returned when attempting to register a shard with an ID that
	// already exists in the ring or metadata.
	ErrDuplicateShard = errors.New("sharding: duplicate shard ID")

	// ErrInvalidPlacement is returned when a replica placement assignment violates
	// consistency or capacity constraints (e.g., fewer nodes than replication factor).
	ErrInvalidPlacement = errors.New("sharding: invalid replica placement")

	// ErrEmptyRing is returned when a key lookup is attempted on a ring with no shards.
	ErrEmptyRing = errors.New("sharding: hash ring is empty")

	// ErrInvalidReplicationFactor is returned when the replication factor is zero or
	// exceeds the number of available physical nodes.
	ErrInvalidReplicationFactor = errors.New("sharding: invalid replication factor")

	// ErrInvalidVirtualNodes is returned when the virtual node count is zero or negative.
	ErrInvalidVirtualNodes = errors.New("sharding: virtual node count must be positive")
)
