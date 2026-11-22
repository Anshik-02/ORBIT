package raft

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Config holds configuration parameters for a Raft consensus node.
type Config struct {
	ID                 string
	Peers              map[string]string // PeerID -> Network/Raft address
	Storage            Storage
	Transport          Transport
	HeartbeatInterval  time.Duration
	ElectionTimeoutMin time.Duration
	ElectionTimeoutMax time.Duration
	CommitCallback     func(LogEntry) error // Invoked sequentially when an entry is committed
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
	mu sync.Mutex

	id        string
	peers     map[string]string // peerID -> address
	storage   Storage
	transport Transport

	// Persistent state on all servers (mirrored in storage)
	currentTerm uint64
	votedFor    string

	// Volatile state on all servers
	role        Role
	commitIndex uint64
	lastApplied uint64

	// Volatile state on leaders
	nextIndex  map[string]uint64
	matchIndex map[string]uint64

	// Timers and timing parameters
	heartbeatInterval  time.Duration
	electionTimeoutMin time.Duration
	electionTimeoutMax time.Duration
	electionTimeout    time.Duration
	lastHeartbeat      time.Time

	// Coordination & lifecycle
	commitCallback func(LogEntry) error
	stopCh         chan struct{}
	stoppedCh      chan struct{}
	applyCond      *sync.Cond
	rng            *rand.Rand
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

	term, votedFor, err := cfg.Storage.GetState()
	if err != nil {
		return nil, fmt.Errorf("read raft initial state: %w", err)
	}

	r := &Raft{
		id:                 cfg.ID,
		peers:              cfg.Peers,
		storage:            cfg.Storage,
		transport:          cfg.Transport,
		currentTerm:        term,
		votedFor:           votedFor,
		role:               Follower,
		heartbeatInterval:  cfg.HeartbeatInterval,
		electionTimeoutMin: cfg.ElectionTimeoutMin,
		electionTimeoutMax: cfg.ElectionTimeoutMax,
		commitCallback:     cfg.CommitCallback,
		nextIndex:          make(map[string]uint64),
		matchIndex:         make(map[string]uint64),
		stopCh:             make(chan struct{}),
		stoppedCh:          make(chan struct{}),
		rng:                rand.New(rand.NewSource(time.Now().UnixNano() + int64(len(cfg.ID)))),
	}

	r.applyCond = sync.NewCond(&r.mu)
	r.resetElectionTimeoutLocked()
	r.lastHeartbeat = time.Now()

	// Register Raft as the RPC handler on the transport
	r.transport.RegisterHandler(r)

	return r, nil
}

// Start begins the Raft background event loop and transport listener.
func (r *Raft) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.transport.Start(); err != nil {
		return fmt.Errorf("start raft transport: %w", err)
	}

	go r.runLoop()
	go r.applyLoop()
	return nil
}

// Stop terminates the consensus engine and releases resources.
func (r *Raft) Stop() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	close(r.stopCh)
	r.applyCond.Broadcast()
	r.mu.Unlock()

	<-r.stoppedCh

	_ = r.transport.Close()
	return r.storage.Close()
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

// resetElectionTimeoutLocked sets a random election timeout within [min, max].
func (r *Raft) resetElectionTimeoutLocked() {
	jitter := r.electionTimeoutMax - r.electionTimeoutMin
	if jitter <= 0 {
		r.electionTimeout = r.electionTimeoutMin
		return
	}
	r.electionTimeout = r.electionTimeoutMin + time.Duration(r.rng.Int63n(int64(jitter)))
}

// stepDownLocked transitions the node to Follower for a new, higher term.
func (r *Raft) stepDownLocked(newTerm uint64) {
	r.currentTerm = newTerm
	r.role = Follower
	r.votedFor = ""
	_ = r.storage.SaveState(r.currentTerm, r.votedFor)
	r.lastHeartbeat = time.Now()
	r.resetElectionTimeoutLocked()
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

	if r.closed {
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
	r.role = Candidate
	r.currentTerm++
	r.votedFor = r.id
	_ = r.storage.SaveState(r.currentTerm, r.votedFor)
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

			resp, err := r.transport.SendRequestVote(ctx, targetAddr, req)
			if err != nil {
				return
			}

			r.mu.Lock()
			defer r.mu.Unlock()

			// Check if higher term seen
			if resp.Term > r.currentTerm {
				r.stepDownLocked(resp.Term)
				return
			}

			// Ensure candidate is still running in the same term
			if r.role != Candidate || r.currentTerm != term {
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

	for peerID, peerAddr := range r.peers {
		nextIdx := r.nextIndex[peerID]
		if nextIdx == 0 {
			nextIdx = 1
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

	resp, err := r.transport.SendAppendEntries(ctx, peerAddr, req)
	if err != nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if remote peer operates on higher term
	if resp.Term > r.currentTerm {
		r.stepDownLocked(resp.Term)
		return
	}

	// Verify sender is still Leader in the same term
	if r.role != Leader || r.currentTerm != term {
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

// applyLoop continuously drains committed entries and invokes the commit callback.
func (r *Raft) applyLoop() {
	for {
		r.mu.Lock()
		for r.commitIndex <= r.lastApplied && !r.closed {
			r.applyCond.Wait()
		}

		if r.closed {
			r.mu.Unlock()
			return
		}

		// Gather entries to apply
		var toApply []LogEntry
		for idx := r.lastApplied + 1; idx <= r.commitIndex; idx++ {
			if entry, ok := r.storage.GetEntry(idx); ok {
				toApply = append(toApply, entry)
			}
		}
		r.lastApplied = r.commitIndex
		cb := r.commitCallback
		r.mu.Unlock()

		for _, entry := range toApply {
			if cb != nil {
				_ = cb(entry)
			}
		}
	}
}

// HandleRequestVote processes inbound RequestVote RPCs from candidates (M3).
func (r *Raft) HandleRequestVote(ctx context.Context, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. If req.Term < currentTerm, reject vote
	if req.Term < r.currentTerm {
		return &RequestVoteResponse{
			Term:        r.currentTerm,
			VoteGranted: false,
		}, nil
	}

	// 2. If req.Term > currentTerm, step down to follower
	if req.Term > r.currentTerm {
		r.stepDownLocked(req.Term)
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
		r.votedFor = req.CandidateID
		_ = r.storage.SaveState(r.currentTerm, r.votedFor)
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

	// 1. Reply false if req.Term < currentTerm (Section 5.1)
	if req.Term < r.currentTerm {
		return &AppendEntriesResponse{
			Term:    r.currentTerm,
			Success: false,
		}, nil
	}

	// 2. If req.Term > currentTerm, step down to Follower
	if req.Term > r.currentTerm {
		r.stepDownLocked(req.Term)
	} else if r.role == Candidate {
		// Valid leader recognized for current term
		r.role = Follower
	}

	// Valid heartbeat recognized: reset election timer
	r.lastHeartbeat = time.Now()

	// 3. Log consistency check: Reply false if log doesn't contain an entry at
	// prevLogIndex matching prevLogTerm (Section 5.3)
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
			var conflictIdx uint64 = 1
			for i := req.PrevLogIndex; i >= 1; i-- {
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
					return nil, fmt.Errorf("truncate conflicting entries: %w", err)
				}
				// Append remaining entries
				if err := r.storage.AppendEntries(req.Entries[i:]); err != nil {
					return nil, fmt.Errorf("append new entries after truncate: %w", err)
				}
				break
			}
			// Identical entry already in log, continue
		} else {
			// Entry not present: append remaining entries
			if err := r.storage.AppendEntries(req.Entries[i:]); err != nil {
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
