package sharding

// This file exposes internal helpers needed only by the test suite.
// It is compiled into the package for all builds (not just tests) but
// only referenced from test files via the exported names below.

// InjectToken adds a single token with an explicit hash value to the ring.
// This is used in white-box tests that need to manufacture hash collisions
// deterministically rather than waiting for a naturally-occurring SHA-256
// collision (which is effectively impossible in practice).
//
// Callers must hold no lock; InjectToken acquires the write lock internally.
func (r *HashRing) InjectToken(hash uint64, shardID ShardID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tokens = mergeTokens(r.tokens, []ringToken{{hash: hash, shardID: shardID}})
	r.shardPresent[shardID] = true
}
