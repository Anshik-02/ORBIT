package raft

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Config holds configuration parameters for a Raft consensus node.
type Config struct {
	ID                 string
	GroupID            GroupID           // Consensus group ID (e.g. "shard-0"). Defaults to DefaultGroupID.
	Peers              map[string]string // PeerID -> Network/Raft address
	Storage            Storage
	Transport          Transport
	HeartbeatInterval  time.Duration
	ElectionTimeoutMin time.Duration
	ElectionTimeoutMax time.Duration
	CommitCallback     func(LogEntry) error // Invoked sequentially when an entry is committed
	LeaderCallback     func(GroupID, string, uint64) // Invoked asynchronously whenever leader changes or is discovered with its term

	// InitialLastApplied seeds lastApplied on startup from a recovered storage engine.
	// Set this to engine.LastAppliedIndex() so the applyLoop skips already-applied entries
	// after a node restart, preventing duplicate state-machine application.
	InitialLastApplied uint64

	// SnapshotCallback captures state machine state and returns the exact applied index and serialized payload.
	SnapshotCallback func() (uint64, []byte, error)

	// SnapshotRestoreCallback restores state machine state when an InstallSnapshot RPC is received.
	SnapshotRestoreCallback func(SnapshotMetadata, []byte) error

	// MaxBatchEntries caps the maximum number of LogEntries replicated in a single AppendEntries RPC.
	// Defaults to 1000 entries.
	MaxBatchEntries uint64
}

type snapshotAssembly struct {
	snapshotID        string
	leaderID          string
	lastIncludedIndex uint64
	lastIncludedTerm  uint64
	totalSize         int64
	checksum          uint32
	expectedOffset    int64
	data              []byte
	lastActivity      time.Time
}

// Raft implements a consensus engine following the Raft consensus algorithm.
//
// Invariants & Safety:
//   - All mutable state is strictly serialized by mu.
//   - Network RPCs are dispatched outside of mu to prevent head-of-line blocking.
//   - Terms and voting records are durably persisted before RPC responses.
//   - Entries are committed only when replicated across a strict majority quorum.
//   - A dummy sentinel entry is preserved at index 0 (term 0) in the log.
type Raft struct {
	mu         sync.Mutex
	snapshotMu sync.Mutex // serializes heavy disk persistence and state machine restore outside mu
	// stateMu serializes all state-machine callbacks (commit and snapshot restore) so a snapshot
	// restore can never execute concurrently with a commit callback that is mid-flight in applyLoop.
	// Lock order: stateMu is always acquired BEFORE r.mu, and NEVER while r.mu is held.
	stateMu sync.Mutex

	id        string
	groupID   GroupID
	peers     map[string]string // peerID -> address (voting members)
	learners  map[string]string // peerID -> address (non-voting learners)
	storage   Storage
	transport Transport

	// Persistent state on all servers (mirrored in storage)
	currentTerm uint64
	votedFor    string

	// Volatile state on all servers
	role        Role
	leaderID    string // current known leader ID
	commitIndex uint64
	lastApplied uint64
	applyErr    error // fatal error during state-machine application
	storageErr  error // fatal error during durable persistence

	// Volatile state on leaders
	nextIndex               map[string]uint64
	matchIndex              map[string]uint64
	peerLastContact         map[string]time.Time
	activeSnapshotTransfers map[string]bool // prevents duplicate concurrent snapshot transfers to the same peer

	// Timers and timing parameters
	heartbeatInterval  time.Duration
	electionTimeoutMin time.Duration
	electionTimeoutMax time.Duration
	electionTimeout    time.Duration
	lastHeartbeat      time.Time
	maxBatchEntries    uint64

	// Coordination & lifecycle
	commitCallback          func(LogEntry) error
	leaderCallback          func(GroupID, string, uint64)
	snapshotCallback        func() (uint64, []byte, error)
	snapshotRestoreCallback func(SnapshotMetadata, []byte) error
	snapshotAssembly        *snapshotAssembly
	stopCh                  chan struct{}
	stoppedCh               chan struct{}
	applyCond      *sync.Cond
	rng            *rand.Rand
	started        bool
	closed         bool
}

// New creates a new Raft consensus node.
func New(cfg Config) (*Raft, error) {
	if cfg.Storage == nil {
		return nil, ErrNilStorage
	}
	if cfg.Transport == nil {
		return nil, ErrNilTransport
	}

	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 50 * time.Millisecond
	}
	if cfg.ElectionTimeoutMin == 0 {
		cfg.ElectionTimeoutMin = 150 * time.Millisecond
	}
	if cfg.ElectionTimeoutMax == 0 {
		cfg.ElectionTimeoutMax = 300 * time.Millisecond
	}
	if cfg.MaxBatchEntries == 0 {
		cfg.MaxBatchEntries = 1000
	}

	term, votedFor, err := cfg.Storage.GetState()
	if err != nil {
		return nil, fmt.Errorf("read raft initial state: %w", err)
	}

	groupID := cfg.GroupID
	if groupID == "" {
		groupID = DefaultGroupID
	}

	r := &Raft{
		id:                 cfg.ID,
		groupID:            groupID,
		peers:              cfg.Peers,
		learners:           make(map[string]string),
		storage:            cfg.Storage,
		transport:          cfg.Transport,
		currentTerm:        term,
		votedFor:           votedFor,
		role:               Follower,
		lastApplied:        cfg.InitialLastApplied, // seed from recovered storage engine
		heartbeatInterval:  cfg.HeartbeatInterval,
		electionTimeoutMin: cfg.ElectionTimeoutMin,
		electionTimeoutMax: cfg.ElectionTimeoutMax,
		maxBatchEntries:    cfg.MaxBatchEntries,
		commitCallback:          cfg.CommitCallback,
		leaderCallback:          cfg.LeaderCallback,
		snapshotCallback:        cfg.SnapshotCallback,
		snapshotRestoreCallback: cfg.SnapshotRestoreCallback,
		nextIndex:               make(map[string]uint64),
		matchIndex:              make(map[string]uint64),
		peerLastContact:         make(map[string]time.Time),
		activeSnapshotTransfers: make(map[string]bool),
		stopCh:                  make(chan struct{}),
		stoppedCh:               make(chan struct{}),
	}

	var idHash int64 = 17
	for _, b := range []byte(cfg.ID + "/" + string(r.groupID)) {
		idHash = idHash*37 + int64(b)
	}
	r.rng = rand.New(rand.NewSource(time.Now().UnixNano() ^ (idHash << 16)))

	r.applyCond = sync.NewCond(&r.mu)
	r.resetElectionTimeoutLocked()
	r.lastHeartbeat = time.Now()

	// Register Raft as the RPC handler for this group on the transport
	if err := r.transport.RegisterHandler(r.groupID, r); err != nil {
		return nil, fmt.Errorf("register raft handler for group %q: %w", r.groupID, err)
	}

	return r, nil
}

// Start begins the Raft background event loop and transport listener.
// Calling Start multiple times is safe and idempotent.
func (r *Raft) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrStopped
	}
	if r.started {
		return nil
	}

	if err := r.transport.Start(); err != nil {
		return fmt.Errorf("start raft transport: %w", err)
	}

	r.started = true
	go r.runLoop()
	go r.applyLoop()
	return nil
}

// Stop terminates the consensus engine and releases resources.
// Calling Stop multiple times is safe and idempotent.
func (r *Raft) Stop() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	wasStarted := r.started
	if wasStarted {
		close(r.stopCh)
	}
	r.applyCond.Broadcast()
	r.mu.Unlock()

	if wasStarted {
		<-r.stoppedCh
	}

	r.transport.UnregisterHandler(r.groupID)
	return r.storage.Close()
}

// GroupID returns the consensus group identifier for this Raft instance.
func (r *Raft) GroupID() GroupID {
	return r.groupID
}

// ID returns the local node identifier.
func (r *Raft) ID() string {
	return r.id
}

// Storage returns the underlying persistent storage engine for testing and diagnostics.
func (r *Raft) Storage() Storage {
	return r.storage
}

// Role returns the current role of the node (Follower, Candidate, Leader).
func (r *Raft) Role() Role {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.role
}

// LeaderID returns the currently recognized cluster leader ID, or empty if unknown.
func (r *Raft) LeaderID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.leaderID
}

// SetLeaderCallback dynamically registers a callback invoked when leadership changes.
func (r *Raft) SetLeaderCallback(cb func(GroupID, string, uint64)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.leaderCallback = cb
}

// CurrentTerm returns the current election term.
func (r *Raft) CurrentTerm() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentTerm
}

// VotedFor returns the candidate ID voted for in the current term.
func (r *Raft) VotedFor() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.votedFor
}

// SetMaxBatchEntries dynamically updates the maximum log entries replicated per AppendEntries RPC.
func (r *Raft) SetMaxBatchEntries(limit uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit > 0 {
		r.maxBatchEntries = limit
	}
}

// ReplicationLag returns a map of peerID -> number of entries behind the leader's lastLogIndex.
// If the node is not the leader, it returns an empty map.
func (r *Raft) ReplicationLag() map[string]uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	lags := make(map[string]uint64)
	if r.role != Leader {
		return lags
	}

	lastIdx, _ := r.storage.LastLogInfo()
	for peerID := range r.peers {
		match := r.matchIndex[peerID]
		if lastIdx > match {
			lags[peerID] = lastIdx - match
		} else {
			lags[peerID] = 0
		}
	}
	return lags
}

// AddLearner registers a non-voting learner replica into this Raft group.
// The leader replicates log entries to learners so they catch up, but learners
// do not count towards voting quorums until explicitly promoted.
func (r *Raft) AddLearner(peerID, addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrStopped
	}
	if peerID == r.id {
		return errors.New("cannot add self as learner")
	}
	if _, exists := r.peers[peerID]; exists {
		return ErrPeerAlreadyExists
	}
	if _, exists := r.learners[peerID]; exists {
		return ErrPeerAlreadyExists
	}

	if r.learners == nil {
		r.learners = make(map[string]string)
	}
	r.learners[peerID] = addr

	if r.role == Leader {
		lastLogIndex, _ := r.storage.LastLogInfo()
		r.nextIndex[peerID] = lastLogIndex + 1
		r.matchIndex[peerID] = 0
		r.broadcastAppendEntriesLocked(true)
	}
	return nil
}

// PromoteLearner promotes a caught-up learner into an active voting peer.
// Invariant: The learner must have replicated entries up to at least the current leader's commitIndex.
func (r *Raft) PromoteLearner(peerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrStopped
	}
	if r.role != Leader {
		return ErrNotLeader
	}
	addr, ok := r.learners[peerID]
	if !ok {
		return ErrPeerNotFound
	}

	if r.matchIndex[peerID] < r.commitIndex {
		return fmt.Errorf("%w: matched %d, needed commitIndex %d", ErrLearnerNotCaughtUp, r.matchIndex[peerID], r.commitIndex)
	}

	delete(r.learners, peerID)
	if r.peers == nil {
		r.peers = make(map[string]string)
	}
	r.peers[peerID] = addr
	return nil
}

// RemovePeer removes a peer from active voting peers and learners.
// If the removed node is the local node, it steps down to Follower.
func (r *Raft) RemovePeer(peerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrStopped
	}

	delete(r.peers, peerID)
	delete(r.learners, peerID)
	delete(r.nextIndex, peerID)
	delete(r.matchIndex, peerID)

	if peerID == r.id {
		r.role = Follower
		r.leaderID = ""
	} else if r.role == Leader {
		r.checkAndUpdateCommitIndexLocked()
	}
	return nil
}

// Peers returns a snapshot of active voting peers.
func (r *Raft) Peers() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := make(map[string]string, len(r.peers))
	for k, v := range r.peers {
		res[k] = v
	}
	return res
}

// Learners returns a snapshot of active non-voting learners.
func (r *Raft) Learners() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := make(map[string]string, len(r.learners))
	for k, v := range r.learners {
		res[k] = v
	}
	return res
}

// CommitIndex returns the highest log index known to be committed.
func (r *Raft) CommitIndex() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.commitIndex
}

// LastApplied returns the highest log index applied to the state machine.
func (r *Raft) LastApplied() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastApplied
}

// RaftLogEntryInfo represents an inspectable log entry for dashboard visualization.
type RaftLogEntryInfo struct {
	Index     uint64 `json:"index"`
	Term      uint64 `json:"term"`
	Type      string `json:"type"`
	Committed bool   `json:"committed"`
	Applied   bool   `json:"applied"`
}

// RecentLogEntries extracts up to limit recent log entries from the underlying storage.
func (r *Raft) RecentLogEntries(limit int) []RaftLogEntryInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	lastIdx, _ := r.storage.LastLogInfo()
	if lastIdx == 0 {
		return nil
	}

	start := uint64(1)
	if lastIdx > uint64(limit) {
		start = lastIdx - uint64(limit) + 1
	}

	var entries []RaftLogEntryInfo
	for i := lastIdx; i >= start && i > 0; i-- {
		entry, ok := r.storage.GetEntry(i)
		if !ok {
			continue
		}

		entryType := "KV_PUT"
		if entry.Operation == "DELETE" {
			entryType = "KV_DELETE"
		} else if entry.Operation == "" {
			entryType = "NO_OP"
		}

		entries = append(entries, RaftLogEntryInfo{
			Index:     entry.Index,
			Term:      entry.Term,
			Type:      entryType,
			Committed: entry.Index <= r.commitIndex,
			Applied:   entry.Index <= r.lastApplied,
		})
	}

	return entries
}

// WaitForCommit blocks until the given log index has been committed (commitIndex >= index)
// or the context is canceled. Used by HTTP handlers to wait for a proposed entry to reach quorum.
func (r *Raft) WaitForCommit(ctx context.Context, index uint64) error {
	for {
		r.mu.Lock()
		committed := r.commitIndex >= index
		closed := r.closed
		r.mu.Unlock()

		if closed {
			return ErrStopped
		}
		if committed {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// WaitForApply blocks until the given log index has been applied to the state machine
// (lastApplied >= index), or returns immediately if a fatal application error occurred
// or the context is canceled.
func (r *Raft) WaitForApply(ctx context.Context, index uint64) error {
	for {
		r.mu.Lock()
		if r.applyErr != nil {
			err := r.applyErr
			r.mu.Unlock()
			return err
		}
		applied := r.lastApplied >= index
		closed := r.closed
		r.mu.Unlock()

		if closed {
			return ErrStopped
		}
		if applied {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// ApplyErr returns any fatal error encountered during state machine application.
func (r *Raft) ApplyErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.applyErr
}

// ReplicationProgress holds per-peer replication progress tracked by the leader.
type ReplicationProgress struct {
	MatchIndex  uint64    `json:"match_index"`
	NextIndex   uint64    `json:"next_index"`
	LastContact time.Time `json:"last_contact,omitempty"`
}

// ReplicationStatus returns a copy of per-peer match and next indexes.
func (r *Raft) ReplicationStatus() map[string]ReplicationProgress {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := make(map[string]ReplicationProgress, len(r.peers))
	for peerID := range r.peers {
		res[peerID] = ReplicationProgress{
			MatchIndex:  r.matchIndex[peerID],
			NextIndex:   r.nextIndex[peerID],
			LastContact: r.peerLastContact[peerID],
		}
	}
	return res
}

// LastActivity returns the timestamp of the last heartbeat or RPC processed.
func (r *Raft) LastActivity() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastHeartbeat
}

// PeerIDs returns the list of configured remote peer IDs.
func (r *Raft) PeerIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := make([]string, 0, len(r.peers))
	for id := range r.peers {
		res = append(res, id)
	}
	return res
}

// StorageErr returns any fatal error encountered during durable state persistence.
func (r *Raft) StorageErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.storageErr
}

// hasCommittedInCurrentTermLocked verifies whether the leader has committed at least
// one entry from its current term (Raft §5.4.2 & §6.4 Leader Completeness Property).
func (r *Raft) hasCommittedInCurrentTermLocked() bool {
	lastIdx, _ := r.storage.LastLogInfo()
	if r.commitIndex >= lastIdx {
		return true // All log entries are committed; state machine is up to date
	}
	entry, ok := r.storage.GetEntry(r.commitIndex)
	if !ok {
		return false
	}
	return entry.Term == r.currentTerm
}

// ReadIndex implements the Raft ReadIndex protocol (Raft dissertation §6.4).
// It returns a safe commit index up to which the local state machine must apply before
// reading, guaranteeing linearizable reads without writing dummy entries to the log.
//
// Invariants verified:
//  1. The node must be the current Leader.
//  2. No fatal persistent storage error has halted consensus.
//  3. The leader must have committed an entry in its current term (Leader Completeness Property).
//  4. Heartbeats confirmed by a strict majority quorum to ensure the leader has not been deposed.
func (r *Raft) ReadIndex(ctx context.Context) (uint64, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return 0, ErrStopped
	}
	if r.storageErr != nil {
		err := r.storageErr
		r.mu.Unlock()
		return 0, fmt.Errorf("%w: %w", ErrStorageHalted, err)
	}
	if r.role != Leader {
		r.mu.Unlock()
		return 0, ErrNotLeader
	}

	// Raft §6.4: A leader cannot serve linearizable reads until it has
	// committed at least one entry in its current term.
	if !r.hasCommittedInCurrentTermLocked() {
		r.mu.Unlock()
		return 0, ErrLeaderNotReady
	}

	readIndex := r.commitIndex
	term := r.currentTerm
	leaderID := r.id

	// Single-node cluster is inherently unanimous
	if len(r.peers) == 0 {
		r.mu.Unlock()
		return readIndex, nil
	}

	peersCopy := make(map[string]string, len(r.peers))
	for id, addr := range r.peers {
		peersCopy[id] = addr
	}
	r.mu.Unlock()

	totalNodes := len(peersCopy) + 1
	majority := (totalNodes / 2) + 1

	// Self counts as 1 confirmation
	var confirmations int32 = 1
	var wg sync.WaitGroup
	confCh := make(chan bool, len(peersCopy))

	for peerID, peerAddr := range peersCopy {
		wg.Add(1)
		go func(pID, pAddr string) {
			defer wg.Done()

			// Pure heartbeat with PrevLogIndex=0 and PrevLogTerm=0:
			// Verifies term authority without interfering with in-flight replication
			// or triggering follower log divergence rejections.
			req := &AppendEntriesRequest{
				Term:         term,
				LeaderID:     leaderID,
				PrevLogIndex: 0,
				PrevLogTerm:  0,
				Entries:      nil,
				LeaderCommit: readIndex,
			}

			resp, err := r.transport.SendAppendEntries(ctx, pAddr, r.groupID, req)
			if err != nil {
				return
			}

			r.mu.Lock()
			if resp.Term > r.currentTerm {
				if err := r.stepDownLocked(resp.Term); err != nil {
					r.storageErr = err
				}
				r.mu.Unlock()
				return
			}
			isStillLeader := (r.role == Leader && r.currentTerm == term && r.storageErr == nil)
			r.mu.Unlock()

			if isStillLeader && resp.Term == term {
				confCh <- true
			}
		}(peerID, peerAddr)
	}

	go func() {
		wg.Wait()
		close(confCh)
	}()

	for range confCh {
		confirmations++
		if int(confirmations) >= majority {
			r.mu.Lock()
			stillLeader := (r.role == Leader && r.currentTerm == term && r.storageErr == nil)
			r.mu.Unlock()
			if !stillLeader {
				return 0, ErrNotLeader
			}
			return readIndex, nil
		}
	}

	return 0, ErrQuorumLost
}

// VerifyLeadership confirms that this node is currently the legitimate leader
// by verifying a majority quorum (Raft §6.4).
func (r *Raft) VerifyLeadership(ctx context.Context) error {
	_, err := r.ReadIndex(ctx)
	return err
}

// resetElectionTimeoutLocked sets a random election timeout within [min, max].
func (r *Raft) resetElectionTimeoutLocked() {
	jitter := r.electionTimeoutMax - r.electionTimeoutMin
	if jitter <= 0 {
		r.electionTimeout = r.electionTimeoutMin
		return
	}
	r.electionTimeout = r.electionTimeoutMin + time.Duration(r.rng.Int63n(int64(jitter)))
}

func (r *Raft) notifyLeaderChangeLocked(newLeader string) {
	if r.leaderCallback == nil {
		return
	}
	cb := r.leaderCallback
	gid := r.groupID
	term := r.currentTerm
	go cb(gid, newLeader, term)
}

// stepDownLocked transitions the node to Follower for a new, higher term.
func (r *Raft) stepDownLocked(newTerm uint64) error {
	if err := r.storage.SaveState(newTerm, ""); err != nil {
		return fmt.Errorf("step down save state: %w", err)
	}
	r.currentTerm = newTerm
	r.role = Follower
	r.leaderID = ""
	r.notifyLeaderChangeLocked("")
	r.votedFor = ""
	r.lastHeartbeat = time.Now()
	r.resetElectionTimeoutLocked()
	return nil
}

// runLoop is the core timer ticker driving heartbeats and election timeouts.
func (r *Raft) runLoop() {
	defer close(r.stoppedCh)
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case now := <-ticker.C:
			r.tick(now)
		}
	}
}

// tick checks timers and triggers elections or heartbeats.
func (r *Raft) tick(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed || r.storageErr != nil {
		return
	}

	if r.role == Leader {
		// Leaders send periodic heartbeats to maintain authority
		if now.Sub(r.lastHeartbeat) >= r.heartbeatInterval {
			r.lastHeartbeat = now
			r.broadcastAppendEntriesLocked(true)
		}
		return
	}

	// Followers and Candidates check election timeout
	if now.Sub(r.lastHeartbeat) >= r.electionTimeout {
		r.startElectionLocked()
	}
}

// startElectionLocked begins a new election as a Candidate (M3).
func (r *Raft) startElectionLocked() {
	if r.closed || r.storageErr != nil {
		return
	}

	newTerm := r.currentTerm + 1
	// Invariant: term and vote must be durably saved BEFORE candidate campaigns
	if err := r.storage.SaveState(newTerm, r.id); err != nil {
		r.storageErr = fmt.Errorf("start election save state: %w", err)
		return
	}

	r.role = Candidate
	r.leaderID = ""
	r.notifyLeaderChangeLocked("")
	r.currentTerm = newTerm
	r.votedFor = r.id
	r.lastHeartbeat = time.Now()
	r.resetElectionTimeoutLocked()

	term := r.currentTerm
	candidateID := r.id
	lastLogIndex, lastLogTerm := r.storage.LastLogInfo()

	// If single-node cluster, become leader immediately
	if len(r.peers) == 0 {
		r.becomeLeaderLocked()
		return
	}

	// Copy peers to dispatch RPCs outside the lock
	peersCopy := make(map[string]string, len(r.peers))
	for id, addr := range r.peers {
		peersCopy[id] = addr
	}

	// Vote for self counts as 1
	var votesReceived int32 = 1
	totalNodes := len(peersCopy) + 1
	majority := (totalNodes / 2) + 1

	req := &RequestVoteRequest{
		Term:         term,
		CandidateID:  candidateID,
		LastLogIndex: lastLogIndex,
		LastLogTerm:  lastLogTerm,
	}

	// Fan out vote requests in parallel to all peers
	for peerID, peerAddr := range peersCopy {
		go func(targetID, targetAddr string) {
			ctx, cancel := context.WithTimeout(context.Background(), r.electionTimeoutMin)
			defer cancel()

			resp, err := r.transport.SendRequestVote(ctx, targetAddr, r.groupID, req)
			if err != nil {
				return
			}

			r.mu.Lock()
			defer r.mu.Unlock()
			r.peerLastContact[targetID] = time.Now()

			// Check if higher term seen
			if resp.Term > r.currentTerm {
				if err := r.stepDownLocked(resp.Term); err != nil {
					r.storageErr = err
				}
				return
			}

			// Ensure candidate is still running in the same term
			if r.role != Candidate || r.currentTerm != term || r.storageErr != nil {
				return
			}

			if resp.VoteGranted {
				votesReceived++
				if int(votesReceived) >= majority {
					r.becomeLeaderLocked()
				}
			}
		}(peerID, peerAddr)
	}
}

// becomeLeaderLocked promotes the node to Leader and initializes tracking state (M3 & M4).
func (r *Raft) becomeLeaderLocked() {
	r.role = Leader
	r.leaderID = r.id
	r.notifyLeaderChangeLocked(r.id)
	lastLogIndex, _ := r.storage.LastLogInfo()

	for peerID := range r.peers {
		r.nextIndex[peerID] = lastLogIndex + 1
		r.matchIndex[peerID] = 0
	}
	r.matchIndex[r.id] = lastLogIndex

	r.lastHeartbeat = time.Now()
	// Immediately send empty heartbeats to suppress other candidates
	r.broadcastAppendEntriesLocked(true)
}

// Propose submits a new command to the replicated Raft log (M5).
// Returns (logIndex, term, err). Returns ErrNotLeader if not the cluster leader.
func (r *Raft) Propose(op, key, value string) (uint64, uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, 0, ErrStopped
	}
	if r.storageErr != nil {
		return 0, 0, fmt.Errorf("%w: %w", ErrStorageHalted, r.storageErr)
	}
	if r.role != Leader {
		return 0, 0, ErrNotLeader
	}

	lastIndex, _ := r.storage.LastLogInfo()
	newIndex := lastIndex + 1
	entry := LogEntry{
		Index:     newIndex,
		Term:      r.currentTerm,
		Operation: op,
		Key:       key,
		Value:     value,
	}

	// Append to local persistent log
	if err := r.storage.AppendEntries([]LogEntry{entry}); err != nil {
		r.storageErr = fmt.Errorf("append local log: %w", err)
		return 0, 0, fmt.Errorf("append local log: %w", err)
	}
	r.matchIndex[r.id] = newIndex

	// Single-node cluster commits immediately
	if len(r.peers) == 0 {
		r.commitIndex = newIndex
		r.applyCond.Broadcast()
		return newIndex, r.currentTerm, nil
	}

	// Broadcast replication to peers immediately
	r.broadcastAppendEntriesLocked(false)

	return newIndex, r.currentTerm, nil
}

// broadcastAppendEntriesLocked dispatches AppendEntries to all peers in parallel.
func (r *Raft) broadcastAppendEntriesLocked(isHeartbeat bool) {
	term := r.currentTerm
	leaderID := r.id
	leaderCommit := r.commitIndex
	firstIndex := r.storage.FirstIndex()

	targets := make(map[string]string, len(r.peers)+len(r.learners))
	for k, v := range r.peers {
		targets[k] = v
	}
	for k, v := range r.learners {
		targets[k] = v
	}

	for peerID, peerAddr := range targets {
		nextIdx := r.nextIndex[peerID]
		if nextIdx == 0 {
			nextIdx = 1
		}

		if nextIdx <= firstIndex {
			// Follower is behind the compacted log boundary; stream snapshot
			if !r.activeSnapshotTransfers[peerID] {
				r.activeSnapshotTransfers[peerID] = true
				go r.sendInstallSnapshotToPeer(peerID, peerAddr, term)
			}
			continue
		}

		prevLogIndex := nextIdx - 1
		prevEntry, ok := r.storage.GetEntry(prevLogIndex)
		var prevLogTerm uint64
		if ok {
			prevLogTerm = prevEntry.Term
		}

		lastLogIndex, _ := r.storage.LastLogInfo()
		var entries []LogEntry
		if !isHeartbeat || nextIdx <= lastLogIndex {
			entries = r.storage.GetEntries(nextIdx)
			if r.maxBatchEntries > 0 && uint64(len(entries)) > r.maxBatchEntries {
				entries = entries[:r.maxBatchEntries]
			}
		}

		req := &AppendEntriesRequest{
			Term:         term,
			LeaderID:     leaderID,
			PrevLogIndex: prevLogIndex,
			PrevLogTerm:  prevLogTerm,
			Entries:      entries,
			LeaderCommit: leaderCommit,
		}

		go r.sendAppendEntriesToPeer(peerID, peerAddr, req, term)
	}
}

// sendAppendEntriesToPeer transmits an AppendEntries RPC and handles follower responses (M5).
func (r *Raft) sendAppendEntriesToPeer(peerID, peerAddr string, req *AppendEntriesRequest, term uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), r.heartbeatInterval*3)
	defer cancel()

	resp, err := r.transport.SendAppendEntries(ctx, peerAddr, r.groupID, req)
	if err != nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.peerLastContact[peerID] = time.Now()

	// Check if remote peer operates on higher term
	if resp.Term > r.currentTerm {
		if err := r.stepDownLocked(resp.Term); err != nil {
			r.storageErr = err
		}
		return
	}

	// Verify sender is still Leader in the same term
	if r.role != Leader || r.currentTerm != term || r.storageErr != nil {
		return
	}

	if resp.Success {
		// Log replicated successfully up through req.PrevLogIndex + len(req.Entries)
		replicatedUpTo := req.PrevLogIndex + uint64(len(req.Entries))
		if replicatedUpTo > r.matchIndex[peerID] {
			r.matchIndex[peerID] = replicatedUpTo
		}
		r.nextIndex[peerID] = r.matchIndex[peerID] + 1

		// Check whether entries can now be committed
		r.checkAndUpdateCommitIndexLocked()
	} else {
		// Follower rejected: log mismatch at PrevLogIndex
		if resp.ConflictIndex > 0 {
			// Fast recovery: jump directly to follower's conflict hint
			r.nextIndex[peerID] = resp.ConflictIndex
		} else if r.nextIndex[peerID] > 1 {
			r.nextIndex[peerID]--
		}

		firstIndex := r.storage.FirstIndex()
		if r.nextIndex[peerID] <= firstIndex {
			// Next index fell behind the compacted log boundary; stream snapshot
			if !r.activeSnapshotTransfers[peerID] {
				r.activeSnapshotTransfers[peerID] = true
				go r.sendInstallSnapshotToPeer(peerID, peerAddr, r.currentTerm)
			}
			return
		}

		// If entries are still pending for this peer, trigger immediate retry
		lastIndex, _ := r.storage.LastLogInfo()
		if r.nextIndex[peerID] <= lastIndex {
			nextIdx := r.nextIndex[peerID]
			prevIdx := nextIdx - 1
			pEntry, _ := r.storage.GetEntry(prevIdx)

			retryReq := &AppendEntriesRequest{
				Term:         r.currentTerm,
				LeaderID:     r.id,
				PrevLogIndex: prevIdx,
				PrevLogTerm:  pEntry.Term,
				Entries:      r.storage.GetEntries(nextIdx),
				LeaderCommit: r.commitIndex,
			}
			go r.sendAppendEntriesToPeer(peerID, peerAddr, retryReq, r.currentTerm)
		}
	}
}

// sendInstallSnapshotToPeer streams snapshot data chunks to a lagging follower (Section 7).
func (r *Raft) sendInstallSnapshotToPeer(peerID, peerAddr string, term uint64) {
	defer func() {
		r.mu.Lock()
		delete(r.activeSnapshotTransfers, peerID)
		r.mu.Unlock()
	}()

	snap, err := r.storage.GetSnapshot()
	if err != nil || snap == nil {
		return
	}

	snapshotID := fmt.Sprintf("snap-%s-%d-%d", r.id, snap.Metadata.LastIncludedIndex, time.Now().UnixNano())
	totalSize := snap.Metadata.Size
	checksum := snap.Metadata.Checksum
	const chunkSize = 256 * 1024 // 256KB chunks

	var offset int64 = 0
	for offset < totalSize || (totalSize == 0 && offset == 0) {
		end := offset + chunkSize
		if end > totalSize {
			end = totalSize
		}
		done := (end == totalSize)
		chunk := snap.Data[offset:end]

		req := &InstallSnapshotRequest{
			GroupID:           r.groupID,
			Term:              term,
			LeaderID:          r.id,
			SnapshotID:        snapshotID,
			LastIncludedIndex: snap.Metadata.LastIncludedIndex,
			LastIncludedTerm:  snap.Metadata.LastIncludedTerm,
			Offset:            offset,
			Data:              chunk,
			Done:              done,
			TotalSize:         totalSize,
			Checksum:          checksum,
		}

		ctx, cancel := context.WithTimeout(context.Background(), r.heartbeatInterval*5)
		resp, err := r.transport.SendInstallSnapshot(ctx, peerAddr, r.groupID, req)
		cancel()

		if err != nil {
			return
		}

		r.mu.Lock()
		r.peerLastContact[peerID] = time.Now()

		if resp.Term > r.currentTerm {
			if err := r.stepDownLocked(resp.Term); err != nil {
				r.storageErr = err
			}
			r.mu.Unlock()
			return
		}

		if r.role != Leader || r.currentTerm != term || r.storageErr != nil {
			r.mu.Unlock()
			return
		}

		if !resp.Success {
			// If follower requested resync to accepted offset, adjust offset and retry
			if resp.AcceptedOffset >= 0 && resp.AcceptedOffset < totalSize && resp.AcceptedOffset != offset {
				offset = resp.AcceptedOffset
				r.mu.Unlock()
				continue
			}
			r.mu.Unlock()
			return
		}

		offset = resp.AcceptedOffset

		if done {
			if snap.Metadata.LastIncludedIndex > r.matchIndex[peerID] {
				r.matchIndex[peerID] = snap.Metadata.LastIncludedIndex
			}
			r.nextIndex[peerID] = r.matchIndex[peerID] + 1
			r.checkAndUpdateCommitIndexLocked()

			// If follower still needs uncompacted log entries, trigger replication
			lastLogIndex, _ := r.storage.LastLogInfo()
			if r.nextIndex[peerID] <= lastLogIndex {
				r.broadcastAppendEntriesLocked(false)
			}
			r.mu.Unlock()
			return
		}

		r.mu.Unlock()
	}
}

// checkAndUpdateCommitIndexLocked advances commitIndex if a majority matches (M6 baseline).
func (r *Raft) checkAndUpdateCommitIndexLocked() {
	lastIndex, _ := r.storage.LastLogInfo()
	totalNodes := len(r.peers) + 1
	majority := (totalNodes / 2) + 1

	// Check from lastIndex down to commitIndex + 1
	for N := lastIndex; N > r.commitIndex; N-- {
		entry, ok := r.storage.GetEntry(N)
		if !ok || entry.Term != r.currentTerm {
			// Raft invariant: Leader commits entries from previous terms ONLY indirectly
			// by committing an entry from its current term (Section 5.4.2)
			continue
		}

		matches := 1 // self
		for peerID := range r.peers {
			if r.matchIndex[peerID] >= N {
				matches++
			}
		}

		if matches >= majority {
			r.commitIndex = N
			r.applyCond.Broadcast()
			break
		}
	}
}

// applyLoop continuously drains committed entries and invokes the commit callback sequentially.
// Invariant: lastApplied advances ONLY after each entry has been successfully applied to the state machine.
// Any callback failure halts application and records applyErr to prevent state machine divergence.
func (r *Raft) applyLoop() {
	for {
		r.mu.Lock()
		for r.commitIndex <= r.lastApplied && !r.closed && r.applyErr == nil {
			r.applyCond.Wait()
		}

		if r.closed || r.applyErr != nil {
			r.mu.Unlock()
			return
		}

		nextIndex := r.lastApplied + 1
		entry, ok := r.storage.GetEntry(nextIndex)
		if !ok {
			firstIdx := r.storage.FirstIndex()
			if nextIndex <= firstIdx {
				r.lastApplied = firstIdx
				r.mu.Unlock()
				continue
			}
			r.applyErr = fmt.Errorf("raft: committed entry at index %d not found in storage", nextIndex)
			r.applyCond.Broadcast()
			r.mu.Unlock()
			return
		}
		cb := r.commitCallback
		r.mu.Unlock()

		// Apply sequentially to state machine outside mu.
		// stateMu serializes this callback with snapshot restore:
		// a restore cannot proceed until any in-flight callback finishes.
		var err error
		if cb != nil {
			r.stateMu.Lock()
			err = cb(entry)
			r.stateMu.Unlock()
		}

		r.mu.Lock()
		if err != nil {
			r.applyErr = fmt.Errorf("raft: state-machine apply failed for entry %d: %w", nextIndex, err)
			r.applyCond.Broadcast()
			r.mu.Unlock()
			return
		}

		// Advance lastApplied ONLY upon successful application of this entry
		r.lastApplied = nextIndex
		r.applyCond.Broadcast()
		r.mu.Unlock()
	}
}

// HandleRequestVote processes inbound RequestVote RPCs from candidates (M3).
func (r *Raft) HandleRequestVote(ctx context.Context, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if req.CandidateID != "" {
		r.peerLastContact[req.CandidateID] = time.Now()
	}

	if r.storageErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageHalted, r.storageErr)
	}

	// 1. If req.Term < currentTerm, reject vote
	if req.Term < r.currentTerm {
		return &RequestVoteResponse{
			Term:        r.currentTerm,
			VoteGranted: false,
		}, nil
	}

	// 2. If req.Term > currentTerm, step down to follower
	if req.Term > r.currentTerm {
		if err := r.stepDownLocked(req.Term); err != nil {
			r.storageErr = err
			return &RequestVoteResponse{
				Term:        r.currentTerm,
				VoteGranted: false,
			}, fmt.Errorf("step down to term %d: %w", req.Term, err)
		}
	}

	// 3. Check vote availability: votedFor is null or candidateID
	canVote := (r.votedFor == "" || r.votedFor == req.CandidateID)

	// 4. Check log up-to-date rule (Section 5.4.1):
	// Candidate's log is at least as up-to-date if:
	// - candidate's lastLogTerm > receiver's lastLogTerm, OR
	// - candidate's lastLogTerm == receiver's lastLogTerm AND candidate's lastLogIndex >= receiver's lastLogIndex
	lastLogIndex, lastLogTerm := r.storage.LastLogInfo()
	logOk := (req.LastLogTerm > lastLogTerm) ||
		(req.LastLogTerm == lastLogTerm && req.LastLogIndex >= lastLogIndex)

	if canVote && logOk {
		// Invariant: Vote MUST be durably saved before granting it
		if err := r.storage.SaveState(r.currentTerm, req.CandidateID); err != nil {
			r.storageErr = fmt.Errorf("durable vote save failed: %w", err)
			return &RequestVoteResponse{
				Term:        r.currentTerm,
				VoteGranted: false,
			}, fmt.Errorf("durable vote save failed: %w", err)
		}
		r.votedFor = req.CandidateID
		r.lastHeartbeat = time.Now() // Granting vote resets election timeout

		return &RequestVoteResponse{
			Term:        r.currentTerm,
			VoteGranted: true,
		}, nil
	}

	return &RequestVoteResponse{
		Term:        r.currentTerm,
		VoteGranted: false,
	}, nil
}

// HandleAppendEntries processes inbound AppendEntries RPCs from leaders (M4 & M5).
func (r *Raft) HandleAppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if req.LeaderID != "" {
		r.peerLastContact[req.LeaderID] = time.Now()
	}

	if r.storageErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageHalted, r.storageErr)
	}

	// 1. Reply false if req.Term < currentTerm (Section 5.1)
	if req.Term < r.currentTerm {
		return &AppendEntriesResponse{
			Term:    r.currentTerm,
			Success: false,
		}, nil
	}

	// 2. If req.Term > currentTerm, step down to Follower
	if req.Term > r.currentTerm {
		if err := r.stepDownLocked(req.Term); err != nil {
			r.storageErr = err
			return &AppendEntriesResponse{
				Term:    r.currentTerm,
				Success: false,
			}, fmt.Errorf("step down to term %d: %w", req.Term, err)
		}
	} else if r.role == Candidate {
		// Valid leader recognized for current term
		r.role = Follower
	}
	prevLeader := r.leaderID
	r.leaderID = req.LeaderID
	if req.LeaderID != "" && req.LeaderID != prevLeader {
		r.notifyLeaderChangeLocked(req.LeaderID)
	}

	// Valid heartbeat recognized: reset election timer
	r.lastHeartbeat = time.Now()

	// 3. Log consistency check: Reply false if log doesn't contain an entry at
	// prevLogIndex matching prevLogTerm (Section 5.3)
	firstLogIndex := r.storage.FirstIndex()
	if len(req.Entries) > 0 && req.PrevLogIndex < firstLogIndex {
		return &AppendEntriesResponse{
			Term:          r.currentTerm,
			Success:       false,
			ConflictIndex: firstLogIndex + 1,
			ConflictTerm:  0,
		}, nil
	}

	lastLogIndex, _ := r.storage.LastLogInfo()
	if req.PrevLogIndex > lastLogIndex {
		// Follower log is shorter than leader's prevLogIndex
		return &AppendEntriesResponse{
			Term:          r.currentTerm,
			Success:       false,
			ConflictIndex: lastLogIndex + 1,
			ConflictTerm:  0,
		}, nil
	}

	if req.PrevLogIndex > 0 {
		prevEntry, ok := r.storage.GetEntry(req.PrevLogIndex)
		if !ok || prevEntry.Term != req.PrevLogTerm {
			// Term mismatch at prevLogIndex: find first index for conflicting term
			conflictTerm := prevEntry.Term
			conflictIdx := firstLogIndex + 1
			for i := req.PrevLogIndex; i > firstLogIndex; i-- {
				if e, found := r.storage.GetEntry(i); found && e.Term == conflictTerm {
					conflictIdx = i
				} else {
					break
				}
			}
			return &AppendEntriesResponse{
				Term:          r.currentTerm,
				Success:       false,
				ConflictIndex: conflictIdx,
				ConflictTerm:  conflictTerm,
			}, nil
		}
	}

	// 4. Resolve conflicts and append new entries (Section 5.3)
	for i, entry := range req.Entries {
		existing, ok := r.storage.GetEntry(entry.Index)
		if ok {
			if existing.Term != entry.Term {
				// Existing entry conflicts with new one: truncate this and all subsequent entries
				if err := r.storage.Truncate(entry.Index); err != nil {
					r.storageErr = fmt.Errorf("truncate conflicting entries: %w", err)
					return nil, fmt.Errorf("truncate conflicting entries: %w", err)
				}
				// Append remaining entries
				if err := r.storage.AppendEntries(req.Entries[i:]); err != nil {
					r.storageErr = fmt.Errorf("append new entries after truncate: %w", err)
					return nil, fmt.Errorf("append new entries after truncate: %w", err)
				}
				break
			}
			// Identical entry already in log, continue
		} else {
			// Entry not present: append remaining entries
			if err := r.storage.AppendEntries(req.Entries[i:]); err != nil {
				r.storageErr = fmt.Errorf("append new entries: %w", err)
				return nil, fmt.Errorf("append new entries: %w", err)
			}
			break
		}
	}

	// 5. Update commitIndex: if leaderCommit > commitIndex,
	// commitIndex = min(leaderCommit, index of last new entry)
	if req.LeaderCommit > r.commitIndex {
		newLastIndex, _ := r.storage.LastLogInfo()
		minCommit := req.LeaderCommit
		if newLastIndex < minCommit {
			minCommit = newLastIndex
		}
		if minCommit > r.commitIndex {
			r.commitIndex = minCommit
			r.applyCond.Broadcast()
		}
	}

	return &AppendEntriesResponse{
		Term:    r.currentTerm,
		Success: true,
	}, nil
}

// HandleInstallSnapshot processes inbound InstallSnapshot RPCs from leaders (Section 7).
func (r *Raft) HandleInstallSnapshot(ctx context.Context, req *InstallSnapshotRequest) (*InstallSnapshotResponse, error) {
	r.mu.Lock()

	if req.LeaderID != "" {
		r.peerLastContact[req.LeaderID] = time.Now()
	}

	if r.storageErr != nil {
		err := r.storageErr
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: %w", ErrStorageHalted, err)
	}

	// 0. Validate GroupID if present in request
	if req.GroupID != "" && req.GroupID != r.groupID {
		term := r.currentTerm
		r.mu.Unlock()
		return &InstallSnapshotResponse{
			Term:    term,
			Success: false,
			Error:   fmt.Sprintf("group mismatch: expected %s, got %s", r.groupID, req.GroupID),
		}, nil
	}

	// 0b. Expire stale snapshot assembly older than 30 seconds (leader change / interrupted transfer)
	const snapshotAssemblyTTL = 30 * time.Second
	if r.snapshotAssembly != nil && time.Since(r.snapshotAssembly.lastActivity) > snapshotAssemblyTTL {
		r.snapshotAssembly = nil
	}

	// 1. Reply immediately if req.Term < currentTerm
	if req.Term < r.currentTerm {
		term := r.currentTerm
		r.mu.Unlock()
		return &InstallSnapshotResponse{
			Term:           term,
			Success:        false,
			AcceptedOffset: 0,
			Error:          "term outdated",
		}, nil
	}

	// 2. If req.Term > currentTerm, step down to Follower
	if req.Term > r.currentTerm {
		if err := r.stepDownLocked(req.Term); err != nil {
			r.storageErr = err
			term := r.currentTerm
			r.mu.Unlock()
			return &InstallSnapshotResponse{
				Term:           term,
				Success:        false,
				AcceptedOffset: 0,
				Error:          err.Error(),
			}, fmt.Errorf("step down to term %d: %w", req.Term, err)
		}
	} else if r.role == Candidate {
		r.role = Follower
	}
	prevLeader := r.leaderID
	r.leaderID = req.LeaderID
	if req.LeaderID != "" && req.LeaderID != prevLeader {
		r.notifyLeaderChangeLocked(req.LeaderID)
	}

	// Valid heartbeat recognized: reset election timer
	r.lastHeartbeat = time.Now()

	// 3. If snapshot is already behind our applied state, reply success without re-installing
	if req.LastIncludedIndex <= r.lastApplied {
		term := r.currentTerm
		acceptedOffset := req.Offset + int64(len(req.Data))
		r.mu.Unlock()
		return &InstallSnapshotResponse{
			Term:           term,
			Success:        true,
			AcceptedOffset: acceptedOffset,
		}, nil
	}

	// 4. Size limits check
	if req.TotalSize > MaxSnapshotSizeBytes || int64(len(req.Data)) > MaxSnapshotChunkBytes {
		term := r.currentTerm
		r.mu.Unlock()
		return &InstallSnapshotResponse{
			Term:           term,
			Success:        false,
			AcceptedOffset: 0,
			Error:          fmt.Sprintf("snapshot size limit exceeded: total=%d (max %d), chunk=%d (max %d)", req.TotalSize, MaxSnapshotSizeBytes, len(req.Data), MaxSnapshotChunkBytes),
		}, nil
	}

	// 5. Chunk assembly & session validation
	if req.Offset == 0 {
		r.snapshotAssembly = &snapshotAssembly{
			snapshotID:        req.SnapshotID,
			leaderID:          req.LeaderID,
			lastIncludedIndex: req.LastIncludedIndex,
			lastIncludedTerm:  req.LastIncludedTerm,
			totalSize:         req.TotalSize,
			checksum:          req.Checksum,
			expectedOffset:    0,
			data:              make([]byte, 0, req.TotalSize),
			lastActivity:      time.Now(),
		}
	}

	if r.snapshotAssembly == nil || r.snapshotAssembly.snapshotID != req.SnapshotID || r.snapshotAssembly.leaderID != req.LeaderID || r.snapshotAssembly.lastIncludedIndex != req.LastIncludedIndex {
		term := r.currentTerm
		r.mu.Unlock()
		return &InstallSnapshotResponse{
			Term:           term,
			Success:        false,
			AcceptedOffset: 0,
			Error:          "snapshot session mismatch or missing assembly",
		}, nil
	}

	if req.Offset != r.snapshotAssembly.expectedOffset {
		term := r.currentTerm
		expected := r.snapshotAssembly.expectedOffset
		r.mu.Unlock()
		return &InstallSnapshotResponse{
			Term:           term,
			Success:        false,
			AcceptedOffset: expected,
			Error:          fmt.Sprintf("chunk offset mismatch: expected %d, got %d", expected, req.Offset),
		}, nil
	}

	r.snapshotAssembly.data = append(r.snapshotAssembly.data, req.Data...)
	r.snapshotAssembly.expectedOffset += int64(len(req.Data))
	r.snapshotAssembly.lastActivity = time.Now()

	if !req.Done {
		term := r.currentTerm
		acceptedOffset := r.snapshotAssembly.expectedOffset
		r.mu.Unlock()
		return &InstallSnapshotResponse{
			Term:           term,
			Success:        true,
			AcceptedOffset: acceptedOffset,
		}, nil
	}

	// Full snapshot data assembled
	fullData := r.snapshotAssembly.data
	expectedChecksum := r.snapshotAssembly.checksum
	lastIncludedIndex := r.snapshotAssembly.lastIncludedIndex
	lastIncludedTerm := r.snapshotAssembly.lastIncludedTerm
	finalOffset := r.snapshotAssembly.expectedOffset
	installTerm := r.currentTerm
	r.snapshotAssembly = nil

	// Verify checksum before proceeding to disk operations
	computedChecksum := ComputeSnapshotChecksum(fullData)
	if expectedChecksum != 0 && computedChecksum != expectedChecksum {
		r.mu.Unlock()
		return &InstallSnapshotResponse{
			Term:           installTerm,
			Success:        false,
			AcceptedOffset: 0,
			Error:          fmt.Sprintf("snapshot checksum mismatch: computed %d, expected %d", computedChecksum, expectedChecksum),
		}, nil
	}

	// Validate that LastIncludedTerm is non-zero and plausible (must be ≤ sender's term)
	if lastIncludedTerm == 0 {
		r.mu.Unlock()
		return &InstallSnapshotResponse{
			Term:    installTerm,
			Success: false,
			Error:   "snapshot LastIncludedTerm must be > 0",
		}, nil
	}
	if lastIncludedTerm > installTerm {
		r.mu.Unlock()
		return &InstallSnapshotResponse{
			Term:    installTerm,
			Success: false,
			Error:   fmt.Sprintf("snapshot LastIncludedTerm %d exceeds sender term %d", lastIncludedTerm, installTerm),
		}, nil
	}

	// Release r.mu during slow disk persistence, compaction, and state-machine restore
	r.mu.Unlock()

	// snapshotMu: serialize concurrent snapshot creations/installations.
	// stateMu:    serialize with any in-flight applyLoop commit callback.
	// Lock order: snapshotMu → stateMu (never the reverse).
	r.snapshotMu.Lock()
	defer r.snapshotMu.Unlock()

	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	meta := SnapshotMetadata{
		Version:           SnapshotCurrentVersion,
		GroupID:           r.groupID,
		LastIncludedIndex: lastIncludedIndex,
		LastIncludedTerm:  lastIncludedTerm,
		CreatedAt:         time.Now(),
		Size:              int64(len(fullData)),
		Checksum:          computedChecksum,
	}

	// 1. Durably persist snapshot to disk
	if err := r.storage.SaveSnapshot(meta, fullData); err != nil {
		r.mu.Lock()
		r.storageErr = fmt.Errorf("install snapshot save: %w", err)
		r.mu.Unlock()
		return nil, fmt.Errorf("install snapshot save: %w", err)
	}

	// 2. Discard/compact Raft log
	if err := r.storage.CompactLog(lastIncludedIndex, lastIncludedTerm); err != nil {
		r.mu.Lock()
		r.storageErr = fmt.Errorf("install snapshot compact log: %w", err)
		r.mu.Unlock()
		return nil, fmt.Errorf("install snapshot compact log: %w", err)
	}

	// 3. Reset state machine
	if r.snapshotRestoreCallback != nil {
		if err := r.snapshotRestoreCallback(meta, fullData); err != nil {
			r.mu.Lock()
			r.applyErr = fmt.Errorf("install snapshot restore state machine: %w", err)
			r.applyCond.Broadcast()
			r.mu.Unlock()
			return nil, fmt.Errorf("install snapshot restore state machine: %w", err)
		}
	}

	// 4. Re-acquire r.mu and finalize indices
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, ErrStopped
	}

	if lastIncludedIndex > r.commitIndex {
		r.commitIndex = lastIncludedIndex
	}
	if lastIncludedIndex > r.lastApplied {
		r.lastApplied = lastIncludedIndex
	}
	r.applyCond.Broadcast()

	return &InstallSnapshotResponse{
		Term:           r.currentTerm,
		Success:        true,
		AcceptedOffset: finalOffset,
	}, nil
}

// CreateSnapshot creates and persists a snapshot with provided data up to appliedIndex,
// compacting obsolete Raft log entries.
func (r *Raft) CreateSnapshot(appliedIndex uint64, data []byte) (*SnapshotMetadata, error) {
	r.snapshotMu.Lock()
	defer r.snapshotMu.Unlock()

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, ErrStopped
	}
	if r.storageErr != nil {
		err := r.storageErr
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: %w", ErrStorageHalted, err)
	}

	firstIndex := r.storage.FirstIndex()
	if appliedIndex <= firstIndex {
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: snapshot index %d <= first log index %d", ErrSnapshotBehind, appliedIndex, firstIndex)
	}

	entry, ok := r.storage.GetEntry(appliedIndex)
	if !ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("raft: entry at applied index %d not found", appliedIndex)
	}
	lastTerm := entry.Term
	r.mu.Unlock()

	meta := SnapshotMetadata{
		Version:           SnapshotCurrentVersion,
		GroupID:           r.groupID,
		LastIncludedIndex: appliedIndex,
		LastIncludedTerm:  lastTerm,
		CreatedAt:         time.Now(),
		Size:              int64(len(data)),
		Checksum:          ComputeSnapshotChecksum(data),
	}

	if err := r.storage.SaveSnapshot(meta, data); err != nil {
		r.mu.Lock()
		r.storageErr = err
		r.mu.Unlock()
		return nil, fmt.Errorf("save snapshot: %w", err)
	}

	if err := r.storage.CompactLog(appliedIndex, lastTerm); err != nil {
		r.mu.Lock()
		r.storageErr = err
		r.mu.Unlock()
		return nil, fmt.Errorf("compact log: %w", err)
	}

	return &meta, nil
}

// TakeSnapshot captures a snapshot of the state machine using the registered SnapshotCallback
// and compacts the Raft log up to the exact index returned by the state machine.
func (r *Raft) TakeSnapshot() (*SnapshotMetadata, error) {
	r.snapshotMu.Lock()
	defer r.snapshotMu.Unlock()

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, ErrStopped
	}
	if r.storageErr != nil {
		err := r.storageErr
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: %w", ErrStorageHalted, err)
	}

	if r.snapshotCallback == nil {
		r.mu.Unlock()
		return nil, errors.New("raft: snapshot callback not configured")
	}
	cb := r.snapshotCallback
	r.mu.Unlock()

	// Capture engine snapshot and its exact applied index outside r.mu
	appliedIndex, data, err := cb()
	if err != nil {
		return nil, fmt.Errorf("raft: snapshot callback: %w", err)
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, ErrStopped
	}
	if r.storageErr != nil {
		err := r.storageErr
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: %w", ErrStorageHalted, err)
	}

	firstIndex := r.storage.FirstIndex()
	if appliedIndex <= firstIndex {
		if snap, _ := r.storage.GetSnapshot(); snap != nil {
			r.mu.Unlock()
			return &snap.Metadata, nil
		}
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: state machine applied index %d <= first log index %d",
			ErrSnapshotBehind, appliedIndex, firstIndex)
	}

	// Raft invariant: state machine cannot represent an index higher than commitIndex
	if appliedIndex > r.commitIndex {
		r.mu.Unlock()
		return nil, fmt.Errorf("raft: state machine applied index %d exceeds commit index %d",
			appliedIndex, r.commitIndex)
	}

	// Advance lastApplied to reflect the state machine's progress
	if appliedIndex > r.lastApplied {
		r.lastApplied = appliedIndex
	}

	entry, ok := r.storage.GetEntry(appliedIndex)
	if !ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("raft: entry at applied index %d not found in storage", appliedIndex)
	}
	lastAppliedTerm := entry.Term
	r.mu.Unlock()

	meta := SnapshotMetadata{
		Version:           SnapshotCurrentVersion,
		GroupID:           r.groupID,
		LastIncludedIndex: appliedIndex,
		LastIncludedTerm:  lastAppliedTerm,
		CreatedAt:         time.Now(),
		Size:              int64(len(data)),
		Checksum:          ComputeSnapshotChecksum(data),
	}

	if err := r.storage.SaveSnapshot(meta, data); err != nil {
		r.mu.Lock()
		r.storageErr = err
		r.mu.Unlock()
		return nil, fmt.Errorf("save snapshot: %w", err)
	}

	if err := r.storage.CompactLog(appliedIndex, lastAppliedTerm); err != nil {
		r.mu.Lock()
		r.storageErr = err
		r.mu.Unlock()
		return nil, fmt.Errorf("compact log: %w", err)
	}

	return &meta, nil
}

// FirstIndex returns the lowest index present in Raft storage (the snapshot sentinel index).
func (r *Raft) FirstIndex() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.storage.FirstIndex()
}

// Snapshot returns the current durable snapshot, or (nil, nil) if none exists.
func (r *Raft) Snapshot() (*Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.storage.GetSnapshot()
}
