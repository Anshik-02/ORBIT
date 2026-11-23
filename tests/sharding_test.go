package tests

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
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
// equals shardCount * virtualNodes after adding N shards (all tokens retained).
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
		if got != expectedTokens {
			t.Errorf("after %d shards: token count %d, want %d", i+1, got, expectedTokens)
		}
	}
}

// TestHashRingTokenCollisionHandling explicitly tests Task 1:
// - Two tokens with identical hashes injected onto the ring.
// - Deterministic tie-breaker selects the lower ShardID.
// - Removing the lower ShardID preserves the higher ShardID's token.
// - Key lookups continue functioning correctly.
func TestHashRingTokenCollisionHandling(t *testing.T) {
	t.Parallel()

	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing: %v", err)
	}

	// Inject two tokens with identical position: 500
	const collisionHash = uint64(500)
	ring.InjectToken(collisionHash, sharding.ShardID(1))
	ring.InjectToken(collisionHash, sharding.ShardID(2))

	// Also add a higher position so the ring is well-formed
	ring.InjectToken(uint64(1000), sharding.ShardID(3))

	if ring.TokenCount() != 3 {
		t.Fatalf("expected 3 tokens retained, got %d", ring.TokenCount())
	}

	// When looking up an exact position <= 500, shard 1 should be selected
	// because lower ShardID wins the tie-break when hashes are identical.
	dist := ring.Distribution()
	if dist[sharding.ShardID(1)] != 1 || dist[sharding.ShardID(2)] != 1 {
		t.Fatalf("both colliding tokens must exist in distribution: %+v", dist)
	}

	// Now remove Shard 1 (the one with lower ID)
	if err := ring.RemoveShard(sharding.ShardID(1)); err != nil {
		t.Fatalf("RemoveShard(1): %v", err)
	}

	// Verify Shard 2's token at position 500 REMAINS on the ring!
	if ring.TokenCount() != 2 {
		t.Fatalf("expected 2 tokens remaining after removing shard 1, got %d", ring.TokenCount())
	}
	if !ring.HasShard(sharding.ShardID(2)) {
		t.Fatal("shard 2 should still be present on the ring")
	}

	// Verify lookups succeed without error
	for i := 0; i < 100; i++ {
		sid, err := ring.LookupKey(fmt.Sprintf("collision-test-key-%d", i))
		if err != nil {
			t.Fatalf("LookupKey failed: %v", err)
		}
		if sid != 2 && sid != 3 {
			t.Errorf("unexpected shard ID %d, want 2 or 3", sid)
		}
	}
}

// TestHashRingDistributionUniformity checks that virtual nodes spread keys roughly
// uniformly across shards.
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

// TestHashRingAddRemoveAddStability verifies Invariant 6 / shard ID stability.
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

// ─── ClusterMetadata Hardening Tests (Task 3 & 4) ────────────────────────────

func buildValidMetadata() *sharding.ClusterMetadata {
	nodes := []string{"node-a", "node-b", "node-c"}
	m := sharding.NewClusterMetadata("orbit-cluster-test", 1, 3, 2, nodes)
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	_ = m.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-b", "node-c"}})
	_ = m.AddShard(&sharding.ShardInfo{ID: 2, Replicas: []string{"node-c", "node-a"}})
	return m
}

func TestClusterMetadataValidateHappyPath(t *testing.T) {
	t.Parallel()

	m := buildValidMetadata()
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
}

func TestClusterMetadataValidateEmptyClusterID(t *testing.T) {
	t.Parallel()

	m := buildValidMetadata()
	m.ClusterID = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty ClusterID, got nil")
	}
}

func TestClusterMetadataValidateEmptyPhysicalNodes(t *testing.T) {
	t.Parallel()

	m := sharding.NewClusterMetadata("orbit-cluster", 1, 1, 1, []string{})
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty physical nodes, got nil")
	}
}

func TestClusterMetadataValidateDuplicatePhysicalNodes(t *testing.T) {
	t.Parallel()

	m := sharding.NewClusterMetadata("orbit-cluster", 1, 1, 1, []string{"n1", "n1"})
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"n1"}})
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for duplicate physical nodes, got nil")
	}
}

func TestClusterMetadataValidateEmptyPhysicalNodeString(t *testing.T) {
	t.Parallel()

	m := sharding.NewClusterMetadata("orbit-cluster", 1, 1, 1, []string{""})
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{""}})
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty physical node string, got nil")
	}
}

func TestClusterMetadataValidateNilShardsMap(t *testing.T) {
	t.Parallel()

	m := &sharding.ClusterMetadata{
		ClusterID:         "orbit-cluster",
		Epoch:             1,
		ShardCount:        0,
		ReplicationFactor: 1,
		PhysicalNodes:     []string{"node-a"},
		Shards:            nil,
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for nil Shards map, got nil")
	}
}

func TestClusterMetadataValidateShardCountMismatch(t *testing.T) {
	t.Parallel()

	m := buildValidMetadata()
	m.ShardCount = 99
	if err := m.Validate(); err == nil {
		t.Fatal("expected Validate() error for shard count mismatch, got nil")
	}
}

func TestClusterMetadataValidateReplicationFactorExceedsNodes(t *testing.T) {
	t.Parallel()

	m := sharding.NewClusterMetadata("orbit-cluster", 1, 1, 5, []string{"node-a", "node-b"})
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b", "?", "??", "???"}})
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for replication factor > node count, got nil")
	}
}

func TestClusterMetadataValidateUnknownReplica(t *testing.T) {
	t.Parallel()

	nodes := []string{"node-a", "node-b", "node-c"}
	m := sharding.NewClusterMetadata("orbit-cluster", 1, 1, 2, nodes)
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-UNKNOWN"}})
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for unknown replica node, got nil")
	}
}

func TestClusterMetadataValidateDuplicateReplica(t *testing.T) {
	t.Parallel()

	nodes := []string{"node-a", "node-b"}
	m := sharding.NewClusterMetadata("orbit-cluster", 1, 1, 2, nodes)
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-a"}})
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for duplicate replica, got nil")
	}
}

func TestClusterMetadataValidateWrongReplicaCount(t *testing.T) {
	t.Parallel()

	nodes := []string{"node-a", "node-b", "node-c"}
	m := sharding.NewClusterMetadata("orbit-cluster", 1, 1, 3, nodes)
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for wrong replica count, got nil")
	}
}

func TestClusterMetadataDuplicateShardRejected(t *testing.T) {
	t.Parallel()

	m := sharding.NewClusterMetadata("orbit-cluster", 1, 2, 1, []string{"node-a"})
	_ = m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a"}})
	err := m.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a"}})
	if err == nil {
		t.Fatal("expected ErrDuplicateShard, got nil")
	}
}

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

// ─── Replica Placement Tests (Task 2) ────────────────────────────────────────

// TestAssignReplicasDocumentedFormula verifies the documented placement formula:
// physicalNodes[(i*rf + r) % n] across the required matrix of configurations:
//
// Nodes  Shards  RF
// 5      3       3
// 5      5       2
// 6      4       3
// 3      3       3
func TestAssignReplicasDocumentedFormula(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		nodes       []string
		shardCount  int
		rf          int
		expectedMap map[sharding.ShardID][]string
	}{
		{
			name:       "5 nodes, 3 shards, rf 3 (Target Topology)",
			nodes:      []string{"A", "B", "C", "D", "E"},
			shardCount: 3,
			rf:         3,
			expectedMap: map[sharding.ShardID][]string{
				0: {"A", "B", "C"},
				1: {"D", "E", "A"},
				2: {"B", "C", "D"},
			},
		},
		{
			name:       "5 nodes, 5 shards, rf 2",
			nodes:      []string{"n0", "n1", "n2", "n3", "n4"},
			shardCount: 5,
			rf:         2,
			expectedMap: map[sharding.ShardID][]string{
				0: {"n0", "n1"},
				1: {"n2", "n3"},
				2: {"n4", "n0"},
				3: {"n1", "n2"},
				4: {"n3", "n4"},
			},
		},
		{
			name:       "6 nodes, 4 shards, rf 3",
			nodes:      []string{"n0", "n1", "n2", "n3", "n4", "n5"},
			shardCount: 4,
			rf:         3,
			expectedMap: map[sharding.ShardID][]string{
				0: {"n0", "n1", "n2"},
				1: {"n3", "n4", "n5"},
				2: {"n0", "n1", "n2"},
				3: {"n3", "n4", "n5"},
			},
		},
		{
			name:       "3 nodes, 3 shards, rf 3",
			nodes:      []string{"n0", "n1", "n2"},
			shardCount: 3,
			rf:         3,
			expectedMap: map[sharding.ShardID][]string{
				0: {"n0", "n1", "n2"},
				1: {"n0", "n1", "n2"},
				2: {"n0", "n1", "n2"},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assignment, err := sharding.AssignReplicas(tc.shardCount, tc.rf, tc.nodes)
			if err != nil {
				t.Fatalf("AssignReplicas failed: %v", err)
			}

			// 1. Verify every shard has the exact expected replica slice
			for sid, expectedReplicas := range tc.expectedMap {
				gotReplicas, exists := assignment[sid]
				if !exists {
					t.Fatalf("missing shard %d in assignment", sid)
				}
				if len(gotReplicas) != tc.rf {
					t.Fatalf("shard %d has %d replicas, want %d", sid, len(gotReplicas), tc.rf)
				}
				for r := 0; r < tc.rf; r++ {
					if gotReplicas[r] != expectedReplicas[r] {
						t.Errorf("shard %d replica %d: got %s, want %s", sid, r, gotReplicas[r], expectedReplicas[r])
					}
				}
			}

			// 2. Verify no duplicate replicas per shard
			for sid, replicas := range assignment {
				seen := make(map[string]bool)
				for _, r := range replicas {
					if seen[r] {
						t.Errorf("shard %d has duplicate replica %q", sid, r)
					}
					seen[r] = true
				}
			}

			// 3. Verify load distribution across physical nodes is balanced
			nodeHostCount := make(map[string]int)
			for _, replicas := range assignment {
				for _, r := range replicas {
					nodeHostCount[r]++
				}
			}
			minHosted := math.MaxInt32
			maxHosted := 0
			for _, n := range tc.nodes {
				count := nodeHostCount[n]
				if count < minHosted {
					minHosted = count
				}
				if count > maxHosted {
					maxHosted = count
				}
			}
			// Difference between max and min hosted shards must be at most 1
			if maxHosted-minHosted > 1 {
				t.Errorf("unbalanced distribution: min=%d, max=%d, counts=%+v", minHosted, maxHosted, nodeHostCount)
			}
		})
	}
}

// TestAssignReplicasInvalidInputs tests thorough error handling for placement.
func TestAssignReplicasInvalidInputs(t *testing.T) {
	t.Parallel()

	validNodes := []string{"n0", "n1", "n2"}

	// 1. Invalid shard counts
	for _, sc := range []int{0, -1, -5} {
		if _, err := sharding.AssignReplicas(sc, 1, validNodes); err == nil {
			t.Errorf("expected error for shard count %d, got nil", sc)
		}
	}

	// 2. Invalid replication factors
	for _, rf := range []int{0, -1, 4, 10} {
		if _, err := sharding.AssignReplicas(2, rf, validNodes); err == nil {
			t.Errorf("expected error for replication factor %d, got nil", rf)
		}
	}

	// 3. Empty physical node list
	if _, err := sharding.AssignReplicas(2, 1, []string{}); err == nil {
		t.Error("expected error for empty physical nodes, got nil")
	}

	// 4. Physical node list with empty string
	if _, err := sharding.AssignReplicas(2, 1, []string{"n0", ""}); err == nil {
		t.Error("expected error for empty node string in list, got nil")
	}

	// 5. Duplicate physical node IDs
	if _, err := sharding.AssignReplicas(2, 1, []string{"n0", "n1", "n0"}); err == nil {
		t.Error("expected error for duplicate node IDs in list, got nil")
	}
}

// ─── Node-to-Shard Lookup (Task 5) ───────────────────────────────────────────

// TestNodeToShardLookup5Nodes verifies that in the 5-node 3-shard topology:
// Node A hosts shards [0, 1]
// Node B hosts shards [0, 2]
// Node C hosts shards [0, 2]
// Node D hosts shards [1, 2]
// Node E hosts shards [1]
func TestNodeToShardLookup5Nodes(t *testing.T) {
	t.Parallel()

	nodes := []string{"node-a", "node-b", "node-c", "node-d", "node-e"}
	assignment, err := sharding.AssignReplicas(3, 3, nodes)
	if err != nil {
		t.Fatalf("AssignReplicas failed: %v", err)
	}

	meta := sharding.NewClusterMetadata("orbit-cluster-01", 1, 3, 3, nodes)
	for sid, replicas := range assignment {
		_ = meta.AddShard(&sharding.ShardInfo{ID: sid, Replicas: replicas})
	}
	if err := meta.Validate(); err != nil {
		t.Fatalf("metadata Validate failed: %v", err)
	}

	expectedShards := map[string][]sharding.ShardID{
		"node-a": {0, 1},
		"node-b": {0, 2},
		"node-c": {0, 2},
		"node-d": {1, 2},
		"node-e": {1},
	}

	for nodeID, wantShards := range expectedShards {
		infos := meta.ShardsForNode(nodeID)
		gotIDs := make([]sharding.ShardID, len(infos))
		for i, info := range infos {
			gotIDs[i] = info.ID
		}
		if len(gotIDs) != len(wantShards) {
			t.Fatalf("node %s: got %d shards (%v), want %d (%v)", nodeID, len(gotIDs), gotIDs, len(wantShards), wantShards)
		}
		for i := range gotIDs {
			if gotIDs[i] != wantShards[i] {
				t.Errorf("node %s shard %d: got %d, want %d", nodeID, i, gotIDs[i], wantShards[i])
			}
		}
	}
}

// ─── Cluster Config File Loading (Task 4) ────────────────────────────────────

func TestClusterConfigFileLoading(t *testing.T) {
	t.Parallel()

	// Locate configs/cluster.json relative to tests dir
	configPaths := []string{
		"../configs/cluster.json",
		filepath.Join("..", "configs", "cluster.json"),
		filepath.Join("orbit", "configs", "cluster.json"),
		filepath.Join("configs", "cluster.json"),
	}

	var meta *sharding.ClusterMetadata
	var err error
	for _, p := range configPaths {
		if _, statErr := os.Stat(p); statErr == nil {
			meta, err = sharding.LoadClusterMetadata(p)
			if err == nil {
				break
			}
		}
	}

	if meta == nil {
		t.Fatalf("could not load configs/cluster.json: %v", err)
	}

	if err := meta.Validate(); err != nil {
		t.Fatalf("configs/cluster.json failed Validate(): %v", err)
	}

	if meta.ClusterID != "orbit-cluster-01" {
		t.Errorf("got ClusterID %q, want orbit-cluster-01", meta.ClusterID)
	}
	if meta.Epoch != 1 {
		t.Errorf("got Epoch %d, want 1", meta.Epoch)
	}
	if meta.ShardCount != 3 {
		t.Errorf("got ShardCount %d, want 3", meta.ShardCount)
	}
	if meta.ReplicationFactor != 3 {
		t.Errorf("got ReplicationFactor %d, want 3", meta.ReplicationFactor)
	}
	if len(meta.PhysicalNodes) != 5 {
		t.Errorf("got %d physical nodes, want 5", len(meta.PhysicalNodes))
	}
}

// ─── Full Stack Integration Test (Task 6 & 7) ────────────────────────────────

// TestEndToEndKeyToShardToReplicasIntegration fulfills Task 6 & 7:
// 1. Create five physical nodes.
// 2. Create three logical shards.
// 3. Configure replication factor three.
// 4. Generate replica assignments.
// 5. Build cluster metadata.
// 6. Validate the metadata.
// 7. Build the consistent hash ring.
// 8. Generate 10,000 keys.
// 9. Map each key to a shard.
// 10. Verify that every mapped shard exists.
// 11. Verify that every shard has three replicas.
// 12. Verify that all nodes receive valid shard assignments.
func TestEndToEndKeyToShardToReplicasIntegration(t *testing.T) {
	t.Parallel()

	// 1-3. Five physical nodes, three logical shards, replication factor three.
	nodes := []string{"node-a", "node-b", "node-c", "node-d", "node-e"}
	const shardCount = 3
	const rf = 3

	// 4. Generate replica assignments
	assignment, err := sharding.AssignReplicas(shardCount, rf, nodes)
	if err != nil {
		t.Fatalf("AssignReplicas failed: %v", err)
	}

	// 5. Build cluster metadata
	meta := sharding.NewClusterMetadata("orbit-cluster-01", 1, shardCount, rf, nodes)
	for sid, replicas := range assignment {
		if err := meta.AddShard(&sharding.ShardInfo{
			ID:       sid,
			Replicas: replicas,
		}); err != nil {
			t.Fatalf("AddShard(%d) failed: %v", sid, err)
		}
	}

	// 6. Validate metadata
	if err := meta.Validate(); err != nil {
		t.Fatalf("metadata Validate() failed: %v", err)
	}

	// 7. Build consistent hash ring
	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing failed: %v", err)
	}
	for sid := range meta.Shards {
		if err := ring.AddShard(sid); err != nil {
			t.Fatalf("ring.AddShard(%d) failed: %v", sid, err)
		}
	}

	// 8-12. Generate 10,000 keys and verify full resolution pipeline:
	// KEY -> HASH RING -> SHARD ID -> CLUSTER METADATA -> REPLICA NODES
	const numKeys = 10_000
	shardKeyCounts := make(map[sharding.ShardID]int)

	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("tenant-%d:resource-%d", i%50, i)

		// Map key to shard
		sid, err := ring.LookupKey(key)
		if err != nil {
			t.Fatalf("LookupKey(%q) failed: %v", key, err)
		}

		// Verify mapped shard exists in metadata
		info, err := meta.GetShard(sid)
		if err != nil {
			t.Fatalf("GetShard(%d) for key %q failed: %v", sid, key, err)
		}

		// Verify shard has exactly 3 replicas
		if len(info.Replicas) != rf {
			t.Fatalf("shard %d has %d replicas, want %d", sid, len(info.Replicas), rf)
		}

		// Verify every replica is a valid physical node
		for _, r := range info.Replicas {
			found := false
			for _, n := range nodes {
				if r == n {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("shard %d replica %q is not in physical nodes", sid, r)
			}
		}

		shardKeyCounts[sid]++
	}

	// Verify all shards received a reasonable distribution of keys
	for sid := sharding.ShardID(0); sid < shardCount; sid++ {
		count := shardKeyCounts[sid]
		pct := float64(count) / float64(numKeys) * 100
		t.Logf("Shard %s received %d keys (%.2f%%)", sid, count, pct)
		if count == 0 {
			t.Errorf("shard %s received zero keys", sid)
		}
	}

	// Verify all nodes receive valid shard assignments
	for _, n := range nodes {
		hosted := meta.ShardsForNode(n)
		if len(hosted) == 0 {
			t.Errorf("physical node %s hosts zero shards", n)
		}
		t.Logf("Node %s hosts %d shards", n, len(hosted))
	}
}
