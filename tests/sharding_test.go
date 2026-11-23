package tests

import (
	"fmt"
	"sort"
	"testing"

	"orbit/internal/sharding"
)

// ─── Hash Ring Tests ──────────────────────────────────────────────────────────

// TestHashRingKeyDeterminism verifies Invariant 2:
// The same key always maps to the same shard for an unchanged ring.
func TestHashRingKeyDeterminism(t *testing.T) {
	t.Parallel()

	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing: %v", err)
	}

	for _, id := range []sharding.ShardID{0, 1, 2} {
		if err := ring.AddShard(id); err != nil {
			t.Fatalf("AddShard(%d): %v", id, err)
		}
	}

	keys := []string{
		"user:101", "user:205", "product:10", "order:501",
		"session:xyz", "cache:abc", "", "a", "z",
	}

	// Collect first assignment
	first := make(map[string]sharding.ShardID, len(keys))
	for _, k := range keys {
		id, err := ring.LookupKey(k)
		if err != nil {
			t.Fatalf("LookupKey(%q): %v", k, err)
		}
		first[k] = id
	}

	// Repeat 10 times and confirm identical results
	for round := 0; round < 10; round++ {
		for _, k := range keys {
			id, err := ring.LookupKey(k)
			if err != nil {
				t.Fatalf("round %d LookupKey(%q): %v", round, k, err)
			}
			if id != first[k] {
				t.Errorf("round %d: key %q: got shard %d, want %d", round, k, id, first[k])
			}
		}
	}
}

// TestHashRingEmptyRing verifies that LookupKey returns ErrEmptyRing when no
// shards have been added.
func TestHashRingEmptyRing(t *testing.T) {
	t.Parallel()

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	_, err := ring.LookupKey("any-key")
	if err == nil {
		t.Fatal("expected ErrEmptyRing, got nil")
	}
}

// TestHashRingSingleShard verifies Invariant 1:
// Every valid key maps to exactly one shard.
// When only one shard is present every key should map to it.
func TestHashRingSingleShard(t *testing.T) {
	t.Parallel()

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err := ring.AddShard(sharding.ShardID(7)); err != nil {
		t.Fatalf("AddShard: %v", err)
	}

	keys := []string{"alpha", "beta", "gamma", "delta", "epsilon", ""}
	for _, k := range keys {
		id, err := ring.LookupKey(k)
		if err != nil {
			t.Fatalf("LookupKey(%q): %v", k, err)
		}
		if id != sharding.ShardID(7) {
			t.Errorf("key %q: expected shard 7, got %d", k, id)
		}
	}
}

// TestHashRingAddShardReducesKeyRemapping verifies Invariant 3:
// Adding a shard should move only a subset of keys.
//
// We seed a 2-shard ring, record the assignment for a large number of keys,
// add a third shard, then verify that at least some keys were remapped
// but not more than roughly 1/(shard count) fraction of the total.
func TestHashRingAddShardReducesKeyRemapping(t *testing.T) {
	t.Parallel()

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	for _, id := range []sharding.ShardID{0, 1} {
		if err := ring.AddShard(id); err != nil {
			t.Fatalf("AddShard(%d): %v", id, err)
		}
	}

	const keyCount = 10_000
	keys := make([]string, keyCount)
	for i := range keys {
		keys[i] = fmt.Sprintf("key:%d", i)
	}

	before := make(map[string]sharding.ShardID, keyCount)
	for _, k := range keys {
		id, _ := ring.LookupKey(k)
		before[k] = id
	}

	// Add a third shard
	if err := ring.AddShard(sharding.ShardID(2)); err != nil {
		t.Fatalf("AddShard(2): %v", err)
	}

	remapped := 0
	for _, k := range keys {
		after, _ := ring.LookupKey(k)
		if after != before[k] {
			remapped++
		}
	}

	remappedFraction := float64(remapped) / float64(keyCount)
	t.Logf("remapped %d / %d keys (%.1f%%) after adding shard 2", remapped, keyCount, remappedFraction*100)

	// With 3 shards we expect close to 1/3 of keys remapped. Allow 10%–55% as a broad tolerance.
	if remappedFraction < 0.10 || remappedFraction > 0.55 {
		t.Errorf("unexpected remap fraction %.2f; expected ~0.33", remappedFraction)
	}
}

// TestHashRingRemoveShardNoInvalidAssignment verifies Invariant 4:
// After removing a shard no key should map to the removed shard.
func TestHashRingRemoveShardNoInvalidAssignment(t *testing.T) {
	t.Parallel()

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	for _, id := range []sharding.ShardID{0, 1, 2} {
		if err := ring.AddShard(id); err != nil {
			t.Fatalf("AddShard(%d): %v", id, err)
		}
	}

	if err := ring.RemoveShard(sharding.ShardID(1)); err != nil {
		t.Fatalf("RemoveShard(1): %v", err)
	}

	for i := 0; i < 5000; i++ {
		k := fmt.Sprintf("key-%d", i)
		id, err := ring.LookupKey(k)
		if err != nil {
			t.Fatalf("LookupKey(%q): %v", k, err)
		}
		if id == sharding.ShardID(1) {
			t.Errorf("key %q mapped to removed shard 1", k)
		}
	}
}

// TestHashRingDuplicateShardRejected verifies Invariant 5 / ErrDuplicateShard:
// Adding the same ShardID twice returns ErrDuplicateShard.
func TestHashRingDuplicateShardRejected(t *testing.T) {
	t.Parallel()

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err := ring.AddShard(sharding.ShardID(0)); err != nil {
		t.Fatalf("first AddShard: %v", err)
	}
	err := ring.AddShard(sharding.ShardID(0))
	if err == nil {
		t.Fatal("expected ErrDuplicateShard, got nil")
	}
}

// TestHashRingRemoveMissing verifies that RemoveShard returns ErrShardNotFound
// for a shard that was never added.
func TestHashRingRemoveMissing(t *testing.T) {
	t.Parallel()

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	err := ring.RemoveShard(sharding.ShardID(99))
	if err == nil {
		t.Fatal("expected ErrShardNotFound, got nil")
	}
}

// TestHashRingInvalidVirtualNodes verifies that NewHashRing rejects zero or negative
// virtual node counts.
func TestHashRingInvalidVirtualNodes(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, -1, -100} {
		_, err := sharding.NewHashRing(n)
		if err == nil {
			t.Errorf("NewHashRing(%d) expected error, got nil", n)
		}
	}
}

// TestHashRingTokenCountProportional verifies that the total token count on the ring
// equals shardCount * virtualNodes after adding N shards.
func TestHashRingTokenCountProportional(t *testing.T) {
	t.Parallel()

	const vnodes = 32
	ring, _ := sharding.NewHashRing(vnodes)

	for i := 0; i < 5; i++ {
		if err := ring.AddShard(sharding.ShardID(i)); err != nil {
			t.Fatalf("AddShard(%d): %v", i, err)
		}
		expectedTokens := (i + 1) * vnodes
		got := ring.TokenCount()
		// Allow a small tolerance for SHA-256 hash collisions being deduplicated.
		if got < expectedTokens-5 || got > expectedTokens {
			t.Errorf("after %d shards: token count %d, want ≈%d", i+1, got, expectedTokens)
		}
	}
}

// TestHashRingDistributionUniformity checks that virtual nodes spread keys roughly
// uniformly across shards.  This is a soft bound: we allow each shard to hold
// between 15% and 52% of keys for a 3-shard ring (expected ~33%).
func TestHashRingDistributionUniformity(t *testing.T) {
	t.Parallel()

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	for _, id := range []sharding.ShardID{0, 1, 2} {
		if err := ring.AddShard(id); err != nil {
			t.Fatalf("AddShard(%d): %v", id, err)
		}
	}

	counts := make(map[sharding.ShardID]int)
	const keyCount = 30_000
	for i := 0; i < keyCount; i++ {
		id, _ := ring.LookupKey(fmt.Sprintf("test-key-%d", i))
		counts[id]++
	}

	t.Logf("Key distribution across 3 shards (N=%d):", keyCount)
	for id := sharding.ShardID(0); id < 3; id++ {
		pct := float64(counts[id]) / float64(keyCount) * 100
		t.Logf("  shard %d: %d keys (%.1f%%)", id, counts[id], pct)
		if pct < 15 || pct > 52 {
			t.Errorf("shard %d has %.1f%% of keys; want 15%%–52%%", id, pct)
		}
	}
}

// TestHashRingAddRemoveAddStability verifies Invariant 6 / shard ID stability:
// A shard that is removed and re-added produces consistent assignments
// (deterministic virtual-node positions are reproduced identically).
func TestHashRingAddRemoveAddStability(t *testing.T) {
	t.Parallel()

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	for _, id := range []sharding.ShardID{0, 1, 2} {
		if err := ring.AddShard(id); err != nil {
			t.Fatalf("AddShard(%d): %v", id, err)
		}
	}

	const keyCount = 2000
	keys := make([]string, keyCount)
	for i := range keys {
		keys[i] = fmt.Sprintf("stable-key-%d", i)
	}

	original := make(map[string]sharding.ShardID, keyCount)
	for _, k := range keys {
		id, _ := ring.LookupKey(k)
		original[k] = id
	}

	// Remove shard 1 and re-add it
	if err := ring.RemoveShard(sharding.ShardID(1)); err != nil {
		t.Fatalf("RemoveShard: %v", err)
	}
	if err := ring.AddShard(sharding.ShardID(1)); err != nil {
		t.Fatalf("re-AddShard: %v", err)
	}

	for _, k := range keys {
		id, _ := ring.LookupKey(k)
		if id != original[k] {
			t.Errorf("key %q: after remove+re-add got shard %d, want %d", k, id, original[k])
		}
	}
}

// TestHashRingConcurrency checks that concurrent reads and writes do not race.
func TestHashRingConcurrency(t *testing.T) {
	t.Parallel()

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	// Pre-populate some shards
	for _, id := range []sharding.ShardID{0, 1, 2} {
		_ = ring.AddShard(id)
	}

	done := make(chan struct{})
	for g := 0; g < 8; g++ {
		go func(g int) {
			for i := 0; i < 500; i++ {
				_, _ = ring.LookupKey(fmt.Sprintf("g%d-key-%d", g, i))
			}
			done <- struct{}{}
		}(g)
	}
	// One writer concurrently adds and removes a shard
	go func() {
		for i := 0; i < 20; i++ {
			_ = ring.AddShard(sharding.ShardID(10))
			_ = ring.RemoveShard(sharding.ShardID(10))
		}
		done <- struct{}{}
	}()

	for i := 0; i < 9; i++ {
		<-done
	}
}

// ─── ShardID / ShardInfo Tests ────────────────────────────────────────────────

// TestShardIDString verifies the canonical string format.
func TestShardIDString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id   sharding.ShardID
		want string
	}{
		{0, "shard-0"},
		{1, "shard-1"},
		{255, "shard-255"},
	}
	for _, tc := range cases {
		got := tc.id.String()
		if got != tc.want {
			t.Errorf("ShardID(%d).String() = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// TestShardInfoContainsNode verifies membership query on ShardInfo.
func TestShardInfoContainsNode(t *testing.T) {
	t.Parallel()

	info := &sharding.ShardInfo{
		ID:       sharding.ShardID(0),
		Replicas: []string{"node-a", "node-b", "node-c"},
	}
	if !info.ContainsNode("node-a") {
		t.Error("expected ContainsNode(node-a) = true")
	}
	if info.ContainsNode("node-x") {
		t.Error("expected ContainsNode(node-x) = false")
	}
	if info.ReplicaCount() != 3 {
		t.Errorf("expected ReplicaCount 3, got %d", info.ReplicaCount())
	}
}

// ─── ClusterMetadata Tests ────────────────────────────────────────────────────

func buildValidMetadata() *sharding.ClusterMetadata {
	nodes := []string{"node-a", "node-b", "node-c"}
	m := sharding.NewClusterMetadata(3, 2, nodes)
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	_ = m.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-b", "node-c"}})
	_ = m.AddShard(&sharding.ShardInfo{ID: 2, Replicas: []string{"node-c", "node-a"}})
	return m
}

// TestClusterMetadataValidateHappyPath verifies that a correctly constructed
// ClusterMetadata passes validation.
func TestClusterMetadataValidateHappyPath(t *testing.T) {
	t.Parallel()

	m := buildValidMetadata()
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
}

// TestClusterMetadataValidateShardCountMismatch verifies that declaring ShardCount
// inconsistent with the actual number of registered shards returns an error.
func TestClusterMetadataValidateShardCountMismatch(t *testing.T) {
	t.Parallel()

	m := buildValidMetadata()
	m.ShardCount = 99 // wrong
	if err := m.Validate(); err == nil {
		t.Fatal("expected Validate() error for shard count mismatch, got nil")
	}
}

// TestClusterMetadataValidateReplicationFactorExceedsNodes verifies that
// replicationFactor > number of physical nodes is rejected.
func TestClusterMetadataValidateReplicationFactorExceedsNodes(t *testing.T) {
	t.Parallel()

	m := sharding.NewClusterMetadata(1, 5, []string{"node-a", "node-b"})
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b", "?", "??", "???"}})
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for replication factor > node count, got nil")
	}
}

// TestClusterMetadataValidateUnknownReplica verifies that listing a node not in
// PhysicalNodes as a replica is rejected.
func TestClusterMetadataValidateUnknownReplica(t *testing.T) {
	t.Parallel()

	nodes := []string{"node-a", "node-b", "node-c"}
	m := sharding.NewClusterMetadata(1, 2, nodes)
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-UNKNOWN"}})
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for unknown replica node, got nil")
	}
}

// TestClusterMetadataValidateDuplicateReplica verifies that listing the same node
// twice as a replica within a shard is rejected.
func TestClusterMetadataValidateDuplicateReplica(t *testing.T) {
	t.Parallel()

	nodes := []string{"node-a", "node-b"}
	m := sharding.NewClusterMetadata(1, 2, nodes)
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-a"}})
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for duplicate replica, got nil")
	}
}

// TestClusterMetadataValidateWrongReplicaCount verifies that a shard with the wrong
// number of replicas is rejected.
func TestClusterMetadataValidateWrongReplicaCount(t *testing.T) {
	t.Parallel()

	nodes := []string{"node-a", "node-b", "node-c"}
	m := sharding.NewClusterMetadata(1, 3, nodes)
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}}) // only 2, wants 3
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for wrong replica count, got nil")
	}
}

// TestClusterMetadataDuplicateShardRejected verifies that AddShard returns
// ErrDuplicateShard on a repeated ShardID.
func TestClusterMetadataDuplicateShardRejected(t *testing.T) {
	t.Parallel()

	m := sharding.NewClusterMetadata(2, 1, []string{"node-a"})
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a"}})
	err := m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a"}})
	if err == nil {
		t.Fatal("expected ErrDuplicateShard, got nil")
	}
}

// TestClusterMetadataGetShard verifies successful and failing shard lookups.
func TestClusterMetadataGetShard(t *testing.T) {
	t.Parallel()

	m := buildValidMetadata()
	info, err := m.GetShard(sharding.ShardID(1))
	if err != nil {
		t.Fatalf("GetShard(1): %v", err)
	}
	if info.ID != sharding.ShardID(1) {
		t.Errorf("expected shard 1, got %v", info.ID)
	}

	_, err = m.GetShard(sharding.ShardID(99))
	if err == nil {
		t.Fatal("expected ErrShardNotFound for shard 99, got nil")
	}
}

// TestClusterMetadataShardsForNode verifies per-node shard listing.
func TestClusterMetadataShardsForNode(t *testing.T) {
	t.Parallel()

	m := buildValidMetadata()
	// node-a hosts shard-0 and shard-2
	infos := m.ShardsForNode("node-a")
	if len(infos) != 2 {
		t.Fatalf("node-a: expected 2 shards, got %d", len(infos))
	}
	ids := []sharding.ShardID{infos[0].ID, infos[1].ID}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if ids[0] != 0 || ids[1] != 2 {
		t.Errorf("node-a shards: got %v, want [0 2]", ids)
	}

	// node-x is not in the cluster
	others := m.ShardsForNode("node-x")
	if len(others) != 0 {
		t.Errorf("node-x expected 0 shards, got %d", len(others))
	}
}

// ─── AssignReplicas Tests ─────────────────────────────────────────────────────

// TestAssignReplicasBalance verifies that round-robin assignment spreads replicas
// evenly and that no shard has duplicate node assignments.
func TestAssignReplicasBalance(t *testing.T) {
	t.Parallel()

	const shardCount = 5
	const rf = 2
	nodes := []string{"n0", "n1", "n2", "n3", "n4"}

	assignment, err := sharding.AssignReplicas(shardCount, rf, nodes)
	if err != nil {
		t.Fatalf("AssignReplicas: %v", err)
	}
	if len(assignment) != shardCount {
		t.Fatalf("expected %d shards in assignment, got %d", shardCount, len(assignment))
	}

	for id, replicas := range assignment {
		if len(replicas) != rf {
			t.Errorf("shard %d: expected %d replicas, got %d", id, rf, len(replicas))
		}
		seen := make(map[string]bool)
		for _, r := range replicas {
			if seen[r] {
				t.Errorf("shard %d: duplicate replica %q", id, r)
			}
			seen[r] = true
		}
	}
}

// TestAssignReplicasInvalidFactor verifies error handling for impossible placements.
func TestAssignReplicasInvalidFactor(t *testing.T) {
	t.Parallel()

	nodes := []string{"n0", "n1"}
	_, err := sharding.AssignReplicas(3, 5, nodes) // 5 replicas, only 2 nodes
	if err == nil {
		t.Fatal("expected error for rf > node count, got nil")
	}
	_, err = sharding.AssignReplicas(3, 0, nodes) // zero rf
	if err == nil {
		t.Fatal("expected error for rf = 0, got nil")
	}
}

// TestAssignReplicasDeterminism verifies that calling AssignReplicas twice with
// the same arguments produces identical results.
func TestAssignReplicasDeterminism(t *testing.T) {
	t.Parallel()

	nodes := []string{"n0", "n1", "n2", "n3", "n4"}
	a1, err := sharding.AssignReplicas(5, 3, nodes)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	a2, err := sharding.AssignReplicas(5, 3, nodes)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	for id := range a1 {
		r1, r2 := fmt.Sprint(a1[id]), fmt.Sprint(a2[id])
		if r1 != r2 {
			t.Errorf("shard %d: got %v vs %v", id, a1[id], a2[id])
		}
	}
}

// ─── Full Stack: Ring + Metadata Integration ──────────────────────────────────

// TestRingAndMetadataIntegration exercises the full Milestone 3.0 stack:
// Add shards to both the ring and the metadata, look up a key, then
// verify the responsible shard has valid replicas.
func TestRingAndMetadataIntegration(t *testing.T) {
	t.Parallel()

	const shardCount = 3
	const rf = 2
	nodes := []string{"node-a", "node-b", "node-c", "node-d", "node-e"}

	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing: %v", err)
	}

	assignment, err := sharding.AssignReplicas(shardCount, rf, nodes)
	if err != nil {
		t.Fatalf("AssignReplicas: %v", err)
	}

	meta := sharding.NewClusterMetadata(shardCount, rf, nodes)
	for i := 0; i < shardCount; i++ {
		id := sharding.ShardID(i)
		if err := ring.AddShard(id); err != nil {
			t.Fatalf("ring.AddShard(%d): %v", i, err)
		}
		if err := meta.AddShard(&sharding.ShardInfo{
			ID:       id,
			Replicas: assignment[id],
		}); err != nil {
			t.Fatalf("meta.AddShard(%d): %v", i, err)
		}
	}

	if err := meta.Validate(); err != nil {
		t.Fatalf("meta.Validate(): %v", err)
	}

	// Look up several keys and verify the responsible shard has valid replicas
	keys := []string{"user:101", "user:205", "product:10", "order:501", "session:xyz"}
	for _, k := range keys {
		sid, err := ring.LookupKey(k)
		if err != nil {
			t.Fatalf("ring.LookupKey(%q): %v", k, err)
		}
		info, err := meta.GetShard(sid)
		if err != nil {
			t.Fatalf("meta.GetShard(%d) for key %q: %v", sid, k, err)
		}
		if len(info.Replicas) != rf {
			t.Errorf("shard %d (key %q): expected %d replicas, got %d", sid, k, rf, len(info.Replicas))
		}
		t.Logf("key %q -> %s replicas=%v", k, sid, info.Replicas)
	}
}

// TestRingKeyExhaustion ensures every key in a large sample set gets
// a valid assignment (Invariant 1: every key maps to exactly one shard).
func TestRingKeyExhaustion(t *testing.T) {
	t.Parallel()

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	for _, id := range []sharding.ShardID{0, 1, 2, 3, 4} {
		if err := ring.AddShard(id); err != nil {
			t.Fatalf("AddShard: %v", err)
		}
	}

	validShards := map[sharding.ShardID]bool{0: true, 1: true, 2: true, 3: true, 4: true}
	for i := 0; i < 50_000; i++ {
		k := fmt.Sprintf("exhaust-key-%d", i)
		id, err := ring.LookupKey(k)
		if err != nil {
			t.Fatalf("LookupKey(%q): %v", k, err)
		}
		if !validShards[id] {
			t.Errorf("key %q mapped to unknown shard %d", k, id)
		}
	}
}
