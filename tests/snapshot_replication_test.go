package tests

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/database"
	"orbit/internal/raft"
	"orbit/internal/storage"
)

type testRaftNode struct {
	id      string
	dir     string
	engine  *storage.Engine
	db      *database.Database
	storage raft.Storage
	raft    *raft.Raft
}

func createTestRaftNode(t *testing.T, id string, peers map[string]string, transport raft.Transport) *testRaftNode {
	dir := filepath.Join(t.TempDir(), id)

	walPath := filepath.Join(dir, "wal.log")
	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("create engine for %s: %v", id, err)
	}

	db, err := database.NewDatabase(eng)
	if err != nil {
		t.Fatalf("create database for %s: %v", id, err)
	}

	rStorage, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("create raft storage for %s: %v", id, err)
	}

	commitCallback := func(entry raft.LogEntry) error {
		return eng.ApplyCommitted(storage.CommittedCommand{
			RaftIndex: entry.Index,
			RaftTerm:  entry.Term,
			Operation: entry.Operation,
			Key:       entry.Key,
			Value:     entry.Value,
		})
	}

	snapshotCallback := func() (uint64, []byte, error) {
		return eng.ExportSnapshotBytes()
	}

	snapshotRestoreCallback := func(meta raft.SnapshotMetadata, data []byte) error {
		return eng.RestoreSnapshotBytes(storage.SnapshotMetadata{
			Index: meta.LastIncludedIndex,
			Term:  meta.LastIncludedTerm,
		}, data)
	}

	r, err := raft.New(raft.Config{
		ID:                      id,
		GroupID:                 raft.DefaultGroupID,
		Peers:                   peers,
		Storage:                 rStorage,
		Transport:               transport,
		HeartbeatInterval:       25 * time.Millisecond,
		ElectionTimeoutMin:      80 * time.Millisecond,
		ElectionTimeoutMax:      160 * time.Millisecond,
		CommitCallback:          commitCallback,
		InitialLastApplied:      eng.LastAppliedIndex(),
		SnapshotCallback:        snapshotCallback,
		SnapshotRestoreCallback: snapshotRestoreCallback,
	})
	if err != nil {
		t.Fatalf("create raft engine for %s: %v", id, err)
	}

	return &testRaftNode{
		id:      id,
		dir:     dir,
		engine:  eng,
		db:      db,
		storage: rStorage,
		raft:    r,
	}
}

// TestInstallSnapshot_LaggingFollowerCatchesUp verifies that a follower lagging behind
// compacted log entries catches up via InstallSnapshot RPC, restores its state machine,
// receives subsequent entries, and recovers correctly on restart.
func TestInstallSnapshot_LaggingFollowerCatchesUp(t *testing.T) {
	net := raft.NewMockNetwork()

	trA := net.CreateTransport("node-a")
	trB := net.CreateTransport("node-b")
	trC := net.CreateTransport("node-c")

	peersA := map[string]string{"node-b": "node-b", "node-c": "node-c"}
	peersB := map[string]string{"node-a": "node-a", "node-c": "node-c"}
	peersC := map[string]string{"node-a": "node-a", "node-b": "node-b"}

	nodeA := createTestRaftNode(t, "node-a", peersA, trA)
	nodeB := createTestRaftNode(t, "node-b", peersB, trB)
	nodeC := createTestRaftNode(t, "node-c", peersC, trC)

	nodes := []*testRaftNode{nodeA, nodeB, nodeC}
	for _, n := range nodes {
		if err := n.raft.Start(); err != nil {
			t.Fatalf("start %s: %v", n.id, err)
		}
	}
	defer func() {
		for _, n := range nodes {
			_ = n.raft.Stop()
			_ = n.db.Close()
		}
	}()

	// 1. Wait for a leader to be elected
	var leader *testRaftNode
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range nodes {
			if n.raft.Role() == raft.Leader {
				leader = n
				break
			}
		}
		if leader != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if leader == nil {
		t.Fatal("failed to elect cluster leader")
	}

	// 2. Identify the follower to isolate (choose nodeC or another follower)
	var followerToIsolate *testRaftNode
	for _, n := range nodes {
		if n.id != leader.id {
			followerToIsolate = n
			break
		}
	}

	// Commit initial entry on all 3
	idx, _, err := leader.raft.Propose("SET", "initial-key", "initial-val")
	if err != nil {
		t.Fatalf("propose initial entry: %v", err)
	}

	// Wait for commit
	waitForApplied := func(node *testRaftNode, targetIdx uint64, timeout time.Duration) error {
		d := time.Now().Add(timeout)
		for time.Now().Before(d) {
			if node.raft.LastApplied() >= targetIdx {
				return nil
			}
			time.Sleep(15 * time.Millisecond)
		}
		return fmt.Errorf("timeout waiting for %s to apply %d (lastApplied=%d)",
			node.id, targetIdx, node.raft.LastApplied())
	}

	if err := waitForApplied(leader, idx, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := waitForApplied(followerToIsolate, idx, 10*time.Second); err != nil {
		t.Fatal(err)
	}

	// 3. Isolate followerToIsolate
	net.Isolate(followerToIsolate.id)

	// 4. Propose 15 more entries on leader (quorum is satisfied by leader + remaining follower)
	var lastProposedIdx uint64
	for i := 1; i <= 15; i++ {
		k := fmt.Sprintf("snap-key-%d", i)
		v := fmt.Sprintf("val-%d", i)
		pIdx, _, err := leader.raft.Propose("SET", k, v)
		if err != nil {
			t.Fatalf("propose entry %d: %v", i, err)
		}
		lastProposedIdx = pIdx
	}

	// Wait for active nodes to commit and apply entries up to lastProposedIdx
	for _, n := range nodes {
		if n.id != followerToIsolate.id {
			if err := waitForApplied(n, lastProposedIdx, 10*time.Second); err != nil {
				t.Fatal(err)
			}
		}
	}

	// 5. Active quorum nodes take snapshots at lastProposedIdx and compact their logs!
	for _, n := range nodes {
		if n.id != followerToIsolate.id {
			snapMeta, err := n.raft.TakeSnapshot()
			if err != nil {
				t.Fatalf("node %s TakeSnapshot failed: %v", n.id, err)
			}
			if snapMeta.LastIncludedIndex != lastProposedIdx {
				t.Fatalf("expected node %s snapshot LastIncludedIndex %d, got %d", n.id, lastProposedIdx, snapMeta.LastIncludedIndex)
			}
			if n.raft.FirstIndex() != lastProposedIdx {
				t.Fatalf("expected node %s FirstIndex %d, got %d", n.id, lastProposedIdx, n.raft.FirstIndex())
			}
		}
	}

	// 6. Propose 3 more entries after snapshot (these exist in leader log)
	var postSnapIdx uint64
	for i := 16; i <= 18; i++ {
		k := fmt.Sprintf("post-snap-key-%d", i)
		v := fmt.Sprintf("val-%d", i)
		pIdx, _, err := leader.raft.Propose("SET", k, v)
		if err != nil {
			t.Fatalf("propose post-snap entry %d: %v", i, err)
		}
		postSnapIdx = pIdx
	}
	if err := waitForApplied(leader, postSnapIdx, 10*time.Second); err != nil {
		t.Fatal(err)
	}

	// Verify followerToIsolate is still far behind at index 1
	if followerToIsolate.raft.LastApplied() > idx {
		t.Fatalf("isolated follower advanced unexpectedly to %d", followerToIsolate.raft.LastApplied())
	}

	// 7. Reconnect isolated follower!
	net.Rejoin(followerToIsolate.id)

	// Leader's nextIndex for followerToIsolate is <= 2 (which is <= leader's FirstIndex of 16).
	// Leader MUST trigger InstallSnapshot to bring followerToIsolate to index 16,
	// and then replicate post-snapshot entries 17..19!
	if err := waitForApplied(followerToIsolate, postSnapIdx, 15*time.Second); err != nil {
		t.Fatalf("follower failed to catch up via snapshot: %v", err)
	}

	// 8. Verify all snapshot and post-snapshot data is present in follower's database
	for i := 1; i <= 15; i++ {
		k := fmt.Sprintf("snap-key-%d", i)
		expectedVal := fmt.Sprintf("val-%d", i)
		gotVal, err := followerToIsolate.db.Get(k)
		if err != nil || gotVal != expectedVal {
			t.Fatalf("follower missing snapshot key %q: got %q (err=%v), want %q",
				k, gotVal, err, expectedVal)
		}
	}
	for i := 16; i <= 18; i++ {
		k := fmt.Sprintf("post-snap-key-%d", i)
		expectedVal := fmt.Sprintf("val-%d", i)
		gotVal, err := followerToIsolate.db.Get(k)
		if err != nil || gotVal != expectedVal {
			t.Fatalf("follower missing post-snapshot key %q: got %q (err=%v), want %q",
				k, gotVal, err, expectedVal)
		}
	}

	// 9. Verify follower persistence across restart
	folDir := followerToIsolate.dir
	folID := followerToIsolate.id
	folPeers := peersC
	if followerToIsolate.id == "node-b" {
		folPeers = peersB
	}

	_ = followerToIsolate.raft.Stop()
	_ = followerToIsolate.db.Close()

	// Reopen storage on follower dir
	reopenedStorage, err := raft.OpenFileStorage(folDir)
	if err != nil {
		t.Fatalf("reopen follower storage: %v", err)
	}
	defer reopenedStorage.Close()

	if reopenedStorage.FirstIndex() != lastProposedIdx {
		t.Fatalf("reopened follower expected first index %d, got %d",
			lastProposedIdx, reopenedStorage.FirstIndex())
	}

	snap, err := reopenedStorage.GetSnapshot()
	if err != nil || snap == nil {
		t.Fatalf("reopened follower snapshot missing: %v", err)
	}
	if snap.Metadata.LastIncludedIndex != lastProposedIdx {
		t.Fatalf("reopened follower snapshot index: got %d, want %d",
			snap.Metadata.LastIncludedIndex, lastProposedIdx)
	}

	// Verify follower engine and database recovery directly from engine.snapshot and remaining WAL
	reopenedEng, err := storage.NewEngine(filepath.Join(folDir, "wal.log"))
	if err != nil {
		t.Fatalf("reopen follower engine: %v", err)
	}
	defer reopenedEng.Close()

	reopenedDB, err := database.NewDatabase(reopenedEng)
	if err != nil {
		t.Fatalf("reopen follower database: %v", err)
	}
	defer reopenedDB.Close()

	if reopenedEng.LastAppliedIndex() != postSnapIdx {
		t.Fatalf("reopened follower engine last applied: got %d, want %d",
			reopenedEng.LastAppliedIndex(), postSnapIdx)
	}

	for i := 1; i <= 15; i++ {
		k := fmt.Sprintf("snap-key-%d", i)
		expectedVal := fmt.Sprintf("val-%d", i)
		gotVal, err := reopenedDB.Get(k)
		if err != nil || gotVal != expectedVal {
			t.Fatalf("reopened follower missing snapshot key %q: got %q (err=%v), want %q",
				k, gotVal, err, expectedVal)
		}
	}
	for i := 16; i <= 18; i++ {
		k := fmt.Sprintf("post-snap-key-%d", i)
		expectedVal := fmt.Sprintf("val-%d", i)
		gotVal, err := reopenedDB.Get(k)
		if err != nil || gotVal != expectedVal {
			t.Fatalf("reopened follower missing post-snapshot key %q: got %q (err=%v), want %q",
				k, gotVal, err, expectedVal)
		}
	}

	_ = folID
	_ = folPeers
}

// TestHTTPTransport_InstallSnapshot verifies the /raft/install-snapshot HTTP endpoint.
func TestHTTPTransport_InstallSnapshot(t *testing.T) {
	serverTransport := raft.NewHTTPTransport("127.0.0.1:0")

	var receivedReq *raft.InstallSnapshotRequest
	handler := &mockSnapshotHandler{
		onInstallSnapshot: func(req *raft.InstallSnapshotRequest) (*raft.InstallSnapshotResponse, error) {
			receivedReq = req
			return &raft.InstallSnapshotResponse{
				Term:           req.Term,
				Success:        true,
				AcceptedOffset: req.Offset + int64(len(req.Data)),
			}, nil
		},
	}

	if err := serverTransport.RegisterHandler(raft.GroupID("shard-0"), handler); err != nil {
		t.Fatalf("register handler: %v", err)
	}
	if err := serverTransport.Start(); err != nil {
		t.Fatalf("start transport: %v", err)
	}
	defer serverTransport.Close()

	clientTransport := raft.NewHTTPTransport("127.0.0.1:0")
	defer clientTransport.Close()

	payload := []byte(`{"key1":"value1"}`)
	req := &raft.InstallSnapshotRequest{
		GroupID:           raft.GroupID("shard-0"),
		Term:              5,
		LeaderID:          "node-leader",
		SnapshotID:        "test-snap-001",
		LastIncludedIndex: 100,
		LastIncludedTerm:  4,
		Offset:            0,
		Data:              payload,
		Done:              true,
		TotalSize:         int64(len(payload)),
		Checksum:          raft.ComputeSnapshotChecksum(payload),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := clientTransport.SendInstallSnapshot(ctx, serverTransport.Addr(), raft.GroupID("shard-0"), req)
	if err != nil {
		t.Fatalf("SendInstallSnapshot failed: %v", err)
	}
	if resp.Term != 5 {
		t.Fatalf("expected term 5, got %d", resp.Term)
	}
	if !resp.Success {
		t.Fatalf("expected success true, got false (err: %s)", resp.Error)
	}
	if resp.AcceptedOffset != int64(len(payload)) {
		t.Fatalf("expected accepted offset %d, got %d", len(payload), resp.AcceptedOffset)
	}
	if receivedReq == nil || receivedReq.LastIncludedIndex != 100 {
		t.Fatalf("handler did not receive expected request: %+v", receivedReq)
	}
}

// TestMultiShard_SnapshotIsolationAndRecovery verifies that independent shards on a node
// maintain separate snapshots, log compaction boundaries, and recovery state without interference.
func TestMultiShard_SnapshotIsolationAndRecovery(t *testing.T) {
	dir := t.TempDir()

	// Shard 0 storage
	s0Dir := filepath.Join(dir, "shard-0")
	s0, err := raft.OpenFileStorage(s0Dir)
	if err != nil {
		t.Fatalf("open shard-0 storage: %v", err)
	}

	// Shard 1 storage
	s1Dir := filepath.Join(dir, "shard-1")
	s1, err := raft.OpenFileStorage(s1Dir)
	if err != nil {
		t.Fatalf("open shard-1 storage: %v", err)
	}

	// Populate Shard 0 with entries 1..10
	e0 := make([]raft.LogEntry, 10)
	for i := 1; i <= 10; i++ {
		e0[i-1] = raft.LogEntry{Index: uint64(i), Term: 1, Operation: "SET", Key: fmt.Sprintf("s0-k%d", i), Value: "val0"}
	}
	if err := s0.AppendEntries(e0); err != nil {
		t.Fatalf("s0 append: %v", err)
	}

	// Populate Shard 1 with entries 1..5
	e1 := make([]raft.LogEntry, 5)
	for i := 1; i <= 5; i++ {
		e1[i-1] = raft.LogEntry{Index: uint64(i), Term: 2, Operation: "SET", Key: fmt.Sprintf("s1-k%d", i), Value: "val1"}
	}
	if err := s1.AppendEntries(e1); err != nil {
		t.Fatalf("s1 append: %v", err)
	}

	// Snapshot Shard 0 at index 8
	meta0 := raft.SnapshotMetadata{GroupID: "shard-0", LastIncludedIndex: 8, LastIncludedTerm: 1, CreatedAt: time.Now()}
	if err := s0.SaveSnapshot(meta0, []byte(`{"shard0":"state8"}`)); err != nil {
		t.Fatalf("s0 save snapshot: %v", err)
	}
	if err := s0.CompactLog(8, 1); err != nil {
		t.Fatalf("s0 compact: %v", err)
	}

	// Verify Shard 0 is compacted to 8, but Shard 1 is untouched (FirstIndex=0, len=6)
	if s0.FirstIndex() != 8 {
		t.Fatalf("s0 first index: got %d, want 8", s0.FirstIndex())
	}
	if s1.FirstIndex() != 0 {
		t.Fatalf("s1 first index: got %d, want 0", s1.FirstIndex())
	}
	lastIdx1, lastTerm1 := s1.LastLogInfo()
	if lastIdx1 != 5 || lastTerm1 != 2 {
		t.Fatalf("s1 last log info: got %d term %d, want 5 term 2", lastIdx1, lastTerm1)
	}

	// Close both
	_ = s0.Close()
	_ = s1.Close()

	// Reopen both from disk and verify independent recovery
	reopened0, err := raft.OpenFileStorage(s0Dir)
	if err != nil {
		t.Fatalf("reopen s0: %v", err)
	}
	defer reopened0.Close()

	reopened1, err := raft.OpenFileStorage(s1Dir)
	if err != nil {
		t.Fatalf("reopen s1: %v", err)
	}
	defer reopened1.Close()

	if reopened0.FirstIndex() != 8 {
		t.Fatalf("reopened0 first index: got %d, want 8", reopened0.FirstIndex())
	}
	snap0, err := reopened0.GetSnapshot()
	if err != nil || snap0 == nil || snap0.Metadata.LastIncludedIndex != 8 {
		t.Fatalf("reopened0 snapshot invalid: %+v, err: %v", snap0, err)
	}

	if reopened1.FirstIndex() != 0 {
		t.Fatalf("reopened1 first index: got %d, want 0", reopened1.FirstIndex())
	}
	snap1, err := reopened1.GetSnapshot()
	if err != nil || snap1 != nil {
		t.Fatalf("reopened1 should have no snapshot, got %+v, err: %v", snap1, err)
	}
}

type mockSnapshotHandler struct {
	onInstallSnapshot func(*raft.InstallSnapshotRequest) (*raft.InstallSnapshotResponse, error)
}

func (m *mockSnapshotHandler) HandleRequestVote(ctx context.Context, req *raft.RequestVoteRequest) (*raft.RequestVoteResponse, error) {
	return &raft.RequestVoteResponse{Term: req.Term, VoteGranted: true}, nil
}

func (m *mockSnapshotHandler) HandleAppendEntries(ctx context.Context, req *raft.AppendEntriesRequest) (*raft.AppendEntriesResponse, error) {
	return &raft.AppendEntriesResponse{Term: req.Term, Success: true}, nil
}

func (m *mockSnapshotHandler) HandleInstallSnapshot(ctx context.Context, req *raft.InstallSnapshotRequest) (*raft.InstallSnapshotResponse, error) {
	if m.onInstallSnapshot != nil {
		return m.onInstallSnapshot(req)
	}
	return &raft.InstallSnapshotResponse{
		Term:           req.Term,
		Success:        true,
		AcceptedOffset: req.Offset + int64(len(req.Data)),
	}, nil
}

// TestInstallSnapshot_ChunkProtocolHardening verifies strict chunk offset continuity,
// session identity enforcement, checksum validation, and size limit checks.
func TestInstallSnapshot_ChunkProtocolHardening(t *testing.T) {
	dir := t.TempDir()
	rStorage, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer rStorage.Close()

	net := raft.NewMockNetwork()
	transport := net.CreateTransport("node-target")

	r, err := raft.New(raft.Config{
		ID:                 "node-target",
		GroupID:            raft.DefaultGroupID,
		Peers:              map[string]string{"node-leader": "node-leader"},
		Storage:            rStorage,
		Transport:          transport,
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 150 * time.Millisecond,
		ElectionTimeoutMax: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new raft: %v", err)
	}

	ctx := context.Background()

	// 1. Chunk out of order at start (offset 100 != 0)
	resp, err := r.HandleInstallSnapshot(ctx, &raft.InstallSnapshotRequest{
		GroupID:           raft.DefaultGroupID,
		Term:              1,
		LeaderID:          "node-leader",
		SnapshotID:        "session-1",
		LastIncludedIndex: 5,
		LastIncludedTerm:  1,
		Offset:            100,
		Data:              []byte("chunk"),
		Done:              false,
		TotalSize:         200,
		Checksum:          12345,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected failure for out-of-order initial offset 100")
	}

	// 2. Valid first chunk (offset 0, len 5)
	resp, err = r.HandleInstallSnapshot(ctx, &raft.InstallSnapshotRequest{
		GroupID:           raft.DefaultGroupID,
		Term:              1,
		LeaderID:          "node-leader",
		SnapshotID:        "session-1",
		LastIncludedIndex: 5,
		LastIncludedTerm:  1,
		Offset:            0,
		Data:              []byte("hello"),
		Done:              false,
		TotalSize:         10,
		Checksum:          12345,
	})
	if err != nil || !resp.Success || resp.AcceptedOffset != 5 {
		t.Fatalf("valid chunk 1 failed: resp=%+v, err=%v", resp, err)
	}

	// 3. Gap in chunks (offset 8 != 5)
	resp, err = r.HandleInstallSnapshot(ctx, &raft.InstallSnapshotRequest{
		GroupID:           raft.DefaultGroupID,
		Term:              1,
		LeaderID:          "node-leader",
		SnapshotID:        "session-1",
		LastIncludedIndex: 5,
		LastIncludedTerm:  1,
		Offset:            8,
		Data:              []byte("world"),
		Done:              false,
		TotalSize:         10,
		Checksum:          12345,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected failure for offset gap")
	}
	if resp.AcceptedOffset != 5 {
		t.Fatalf("expected AcceptedOffset 5, got %d", resp.AcceptedOffset)
	}

	// 4. Session ID mismatch mid-transfer
	resp, err = r.HandleInstallSnapshot(ctx, &raft.InstallSnapshotRequest{
		GroupID:           raft.DefaultGroupID,
		Term:              1,
		LeaderID:          "node-leader",
		SnapshotID:        "session-DIFFERENT",
		LastIncludedIndex: 5,
		LastIncludedTerm:  1,
		Offset:            5,
		Data:              []byte("world"),
		Done:              true,
		TotalSize:         10,
		Checksum:          12345,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected failure for session mismatch")
	}

	// 5. Corrupted checksum on Done: true
	payload := []byte("helloworld")
	correctChecksum := raft.ComputeSnapshotChecksum(payload)
	badChecksum := correctChecksum ^ 0xFFFF

	// Re-send from offset 0 with bad checksum
	resp, err = r.HandleInstallSnapshot(ctx, &raft.InstallSnapshotRequest{
		GroupID:           raft.DefaultGroupID,
		Term:              1,
		LeaderID:          "node-leader",
		SnapshotID:        "session-2",
		LastIncludedIndex: 5,
		LastIncludedTerm:  1,
		Offset:            0,
		Data:              payload,
		Done:              true,
		TotalSize:         int64(len(payload)),
		Checksum:          badChecksum,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected failure for corrupted checksum")
	}

	// 6. Oversized chunk rejection (> MaxSnapshotChunkBytes)
	resp, err = r.HandleInstallSnapshot(ctx, &raft.InstallSnapshotRequest{
		GroupID:           raft.DefaultGroupID,
		Term:              1,
		LeaderID:          "node-leader",
		SnapshotID:        "session-3",
		LastIncludedIndex: 5,
		LastIncludedTerm:  1,
		Offset:            0,
		Data:              make([]byte, raft.MaxSnapshotChunkBytes+1),
		Done:              false,
		TotalSize:         raft.MaxSnapshotChunkBytes + 1,
		Checksum:          0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected failure for oversized chunk")
	}
}

// TestTakeSnapshot_AtomicIndexConsistency verifies that TakeSnapshot() captures
// the exact applied index reported by the state machine and compacts the log to that exact index.
func TestTakeSnapshot_AtomicIndexConsistency(t *testing.T) {
	dir := t.TempDir()
	rStorage, err := raft.OpenFileStorage(dir)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer rStorage.Close()

	// Append entries 1..10
	entries := make([]raft.LogEntry, 10)
	for i := 1; i <= 10; i++ {
		entries[i-1] = raft.LogEntry{
			Index:     uint64(i),
			Term:      1,
			Operation: "SET",
			Key:       fmt.Sprintf("k%d", i),
			Value:     fmt.Sprintf("v%d", i),
		}
	}
	if err := rStorage.AppendEntries(entries); err != nil {
		t.Fatalf("append entries: %v", err)
	}

	// State machine represents state through index 8
	var currentEngineIndex uint64 = 8
	snapshotData := []byte(`{"k8":"v8"}`)

	net := raft.NewMockNetwork()
	transport := net.CreateTransport("node-snap")

	r, err := raft.New(raft.Config{
		ID:                 "node-snap",
		GroupID:            raft.DefaultGroupID,
		Peers:              map[string]string{},
		Storage:            rStorage,
		Transport:          transport,
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 150 * time.Millisecond,
		ElectionTimeoutMax: 300 * time.Millisecond,
		InitialLastApplied: 5, // simulate lastApplied lagging behind engine
		SnapshotCallback: func() (uint64, []byte, error) {
			return currentEngineIndex, snapshotData, nil
		},
	})
	if err != nil {
		t.Fatalf("new raft: %v", err)
	}

	// Commit index must be at least currentEngineIndex for snapshot validity
	// In Raft, commitIndex is advanced by consensus. We simulate committed up to 10.
	// Since r is unexported, we can call TakeSnapshot directly after setting commitIndex.
	// In single node cluster or leader, commitIndex is tracked.
	// Let's elect this node as single leader so commitIndex advances.
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer r.Stop()

	// Wait for election
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && r.Role() != raft.Leader {
		time.Sleep(20 * time.Millisecond)
	}
	if r.Role() != raft.Leader {
		t.Fatal("failed to elect leader")
	}

	// Propose entry to establish leader readiness and advance commit index
	idx, _, err := r.Propose("SET", "sync-k", "sync-v")
	if err != nil {
		t.Fatalf("propose: %v", err)
	}

	// Update currentEngineIndex to the newly committed index
	currentEngineIndex = idx

	meta, err := r.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot failed: %v", err)
	}

	if meta.LastIncludedIndex != idx {
		t.Fatalf("expected snapshot LastIncludedIndex %d, got %d", idx, meta.LastIncludedIndex)
	}

	if r.LastApplied() != idx {
		t.Fatalf("expected lastApplied to advance to %d, got %d", idx, r.LastApplied())
	}

	if rStorage.FirstIndex() != idx {
		t.Fatalf("expected storage FirstIndex %d, got %d", idx, rStorage.FirstIndex())
	}
}
