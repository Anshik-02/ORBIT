package sharding

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
)

const (
	// DefaultVirtualNodes is the default number of virtual nodes (tokens) placed
	// on the hash ring for each physical shard.  More virtual nodes improve
	// key-distribution uniformity at the cost of slightly more ring lookup time.
	// 64 provides good uniformity for clusters with up to ~20 shards.
	DefaultVirtualNodes = 64
)

// ringToken is a single position on the hash ring bound to a shard.
type ringToken struct {
	hash    uint64  // 64-bit position on the [0, 2^64) ring
	shardID ShardID // which shard owns this token
}

// HashRing is a consistent hash ring that maps arbitrary string keys to ShardIDs.
//
// # Design
//
//   - Each shard occupies DefaultVirtualNodes (or a caller-specified count) token
//     positions on a 64-bit ring.
//   - Keys are hashed with SHA-256; the first 8 bytes of the digest form a uint64
//     ring position.
//   - Lookup walks the sorted token list and returns the shard at the first token
//     position >= key hash (wrapping to position 0 at the ring boundary).
//
// # Duplicate token positions
//
// All tokens are retained, even when two tokens for different shards hash to the
// same position.  The token slice is sorted first by hash, then by ShardID as a
// deterministic tie-breaker.  Binary search finds the first token whose hash is
// >= the key hash; at any collision position the lower ShardID is visited first.
//
// This design is critical for correctness: if the token at a colliding position
// were silently dropped, removing the other shard could orphan keys that depended
// on the dropped token.  By keeping all tokens, RemoveShard removes only that
// shard's own tokens and never affects any other shard's coverage.
//
// # Concurrency
//
// All methods are safe for concurrent use.  A read-write mutex protects the token
// slice; reads acquire the shared lock, writes acquire the exclusive lock.
type HashRing struct {
	mu           sync.RWMutex
	tokens       []ringToken // sorted ascending by hash
	shardTokens  int         // virtual node count per shard
	shardPresent map[ShardID]bool
}

// NewHashRing creates an empty HashRing with the given virtual node count per shard.
//
// virtualNodes must be > 0; use DefaultVirtualNodes when no specific value is required.
func NewHashRing(virtualNodes int) (*HashRing, error) {
	if virtualNodes <= 0 {
		return nil, ErrInvalidVirtualNodes
	}
	return &HashRing{
		shardTokens:  virtualNodes,
		shardPresent: make(map[ShardID]bool),
	}, nil
}

// AddShard places virtualNodes tokens for the given ShardID onto the ring.
//
// Returns ErrDuplicateShard if the shard is already present.
func (r *HashRing) AddShard(id ShardID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.shardPresent[id] {
		return ErrDuplicateShard
	}

	newTokens := make([]ringToken, r.shardTokens)
	for i := 0; i < r.shardTokens; i++ {
		key := fmt.Sprintf("shard-%d-vnode-%d", uint32(id), i)
		h := hashKey(key)
		newTokens[i] = ringToken{hash: h, shardID: id}
	}

	r.tokens = mergeTokens(r.tokens, newTokens)
	r.shardPresent[id] = true
	return nil
}

// RemoveShard removes all tokens for the given ShardID from the ring.
//
// Returns ErrShardNotFound if the shard is not currently on the ring.
// After removal, any key previously assigned to this shard will be remapped
// to the next shard on the ring.
func (r *HashRing) RemoveShard(id ShardID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.shardPresent[id] {
		return ErrShardNotFound
	}

	filtered := r.tokens[:0]
	for _, t := range r.tokens {
		if t.shardID != id {
			filtered = append(filtered, t)
		}
	}
	r.tokens = filtered
	delete(r.shardPresent, id)
	return nil
}

// LookupKey returns the ShardID responsible for the given key.
//
// Returns ErrEmptyRing if no shards have been added.
//
// The same key always maps to the same shard for an unchanged ring configuration.
// Adding a new shard moves only the minimum fraction of keys required to populate
// the new shard's token positions.
func (r *HashRing) LookupKey(key string) (ShardID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.tokens) == 0 {
		return 0, ErrEmptyRing
	}

	h := hashKey(key)
	idx := sort.Search(len(r.tokens), func(i int) bool {
		return r.tokens[i].hash >= h
	})

	// Wrap around: if the key hash is beyond the last token, use the first token.
	if idx == len(r.tokens) {
		idx = 0
	}

	return r.tokens[idx].shardID, nil
}

// ShardCount returns the number of distinct shards currently on the ring.
func (r *HashRing) ShardCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.shardPresent)
}

// HasShard reports whether the given ShardID is present on the ring.
func (r *HashRing) HasShard(id ShardID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shardPresent[id]
}

// Shards returns a snapshot of the ShardIDs currently on the ring, in ascending order.
func (r *HashRing) Shards() []ShardID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[ShardID]bool)
	ids := make([]ShardID, 0, len(r.shardPresent))
	for _, t := range r.tokens {
		if !seen[t.shardID] {
			ids = append(ids, t.shardID)
			seen[t.shardID] = true
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// TokenCount returns the total number of virtual-node tokens on the ring.
// When two shards produce tokens at the same hash position, both tokens are
// present in the slice.  Useful for diagnostics and uniformity analysis.
func (r *HashRing) TokenCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tokens)
}

// Distribution returns how many tokens each shard owns (a uniformity audit tool).
// The returned map is a snapshot; it is safe to read without holding the ring lock.
func (r *HashRing) Distribution() map[ShardID]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dist := make(map[ShardID]int, len(r.shardPresent))
	for _, t := range r.tokens {
		dist[t.shardID]++
	}
	return dist
}

// ─── internal helpers ────────────────────────────────────────────────────────

// hashKey derives a uint64 ring position for a string using SHA-256.
// The first 8 bytes of the digest are interpreted as a big-endian uint64.
func hashKey(key string) uint64 {
	digest := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint64(digest[:8])
}

// mergeTokens merges two sorted slices of ringTokens into a single sorted slice.
//
// Sort order: ascending hash, then ascending ShardID as a deterministic tie-breaker.
//
// All tokens are retained — even when two tokens have identical hashes.  Deduplication
// is intentionally avoided so that removing one shard's token never silently drops a
// token that belongs to a different shard at the same position.
func mergeTokens(existing, incoming []ringToken) []ringToken {
	merged := make([]ringToken, 0, len(existing)+len(incoming))
	merged = append(merged, existing...)
	merged = append(merged, incoming...)

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].hash != merged[j].hash {
			return merged[i].hash < merged[j].hash
		}
		// deterministic tie-break: lower ShardID sorts first
		return merged[i].shardID < merged[j].shardID
	})

	return merged
}
