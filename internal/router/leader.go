package router

import (
	"errors"
	"fmt"
	"sync"

	"orbit/internal/database"
	"orbit/internal/raft"
	"orbit/internal/sharding"
)

// ShardInstance abstracts a hosted shard consensus replica with its DB and Raft engine.
type ShardInstance interface {
	ID() sharding.ShardID
	GroupID() raft.GroupID
	Raft() *raft.Raft
	DB() *database.Database
}

var (
	// ErrLeaderUnknown is returned when no leader is currently known for a shard group.
	ErrLeaderUnknown = errors.New("router: leader unknown for shard")
)

// LeaderRecord captures the known leader node ID and the Raft consensus term.
type LeaderRecord struct {
	LeaderID string
	Term     uint64
}

// LeaderTracker tracks the known leader node ID and consensus term for each shard group.
// It is term-aware and prevents outdated leadership updates from overwriting newer terms.
type LeaderTracker struct {
	mu      sync.RWMutex
	leaders map[raft.GroupID]LeaderRecord
}

// NewLeaderTracker creates an empty LeaderTracker.
func NewLeaderTracker() *LeaderTracker {
	return &LeaderTracker{
		leaders: make(map[raft.GroupID]LeaderRecord),
	}
}

// SetLeaderWithTerm records that nodeID is the known leader for groupID at term.
// Stale updates (where term < current cached term) are rejected, returning false.
func (lt *LeaderTracker) SetLeaderWithTerm(groupID raft.GroupID, nodeID string, term uint64) bool {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	current, ok := lt.leaders[groupID]
	if ok && term < current.Term {
		return false // Stale term update rejected
	}

	lt.leaders[groupID] = LeaderRecord{
		LeaderID: nodeID,
		Term:     term,
	}
	return true
}

// SetLeader records that nodeID is the currently known leader for groupID.
func (lt *LeaderTracker) SetLeader(groupID raft.GroupID, nodeID string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	current := lt.leaders[groupID]
	newTerm := current.Term
	if newTerm == 0 {
		newTerm = 1
	}
	lt.leaders[groupID] = LeaderRecord{
		LeaderID: nodeID,
		Term:     newTerm,
	}
}

// GetLeader returns the currently known leader node ID for groupID.
// Returns false if no leader has been registered or if it was invalidated.
func (lt *LeaderTracker) GetLeader(groupID raft.GroupID) (string, bool) {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	rec, ok := lt.leaders[groupID]
	return rec.LeaderID, ok && rec.LeaderID != ""
}

// GetLeaderWithTerm returns the currently known leader node ID and consensus term for groupID.
func (lt *LeaderTracker) GetLeaderWithTerm(groupID raft.GroupID) (string, uint64, bool) {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	rec, ok := lt.leaders[groupID]
	return rec.LeaderID, rec.Term, ok && rec.LeaderID != ""
}

// Invalidate removes cached leader knowledge for groupID, e.g. after receiving
// a NotLeader error or network redirect.
func (lt *LeaderTracker) Invalidate(groupID raft.GroupID) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	delete(lt.leaders, groupID)
}

// InvalidateLeaderIf removes the cached leader if it matches expectedLeader.
func (lt *LeaderTracker) InvalidateLeaderIf(groupID raft.GroupID, expectedLeader string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if current, ok := lt.leaders[groupID]; ok && current.LeaderID == expectedLeader {
		delete(lt.leaders, groupID)
	}
}

// InvalidateLeaderWithTerm removes the cached leader if it matches expectedLeader and term >= cached term.
func (lt *LeaderTracker) InvalidateLeaderWithTerm(groupID raft.GroupID, expectedLeader string, term uint64) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if current, ok := lt.leaders[groupID]; ok && current.LeaderID == expectedLeader && term >= current.Term {
		delete(lt.leaders, groupID)
	}
}

// LeaderResolver inspects local shard replicas and cached tracker data to
// identify current shard leaders.
type LeaderResolver struct {
	tracker *LeaderTracker
}

// NewLeaderResolver creates a LeaderResolver backed by the given LeaderTracker.
func NewLeaderResolver(tracker *LeaderTracker) *LeaderResolver {
	if tracker == nil {
		tracker = NewLeaderTracker()
	}
	return &LeaderResolver{tracker: tracker}
}

// Tracker returns the underlying LeaderTracker.
func (lr *LeaderResolver) Tracker() *LeaderTracker {
	return lr.tracker
}

// ResolveLocal determines the leader for a locally-hosted shard replica.
// If the replica is currently the leader or recognizes a leader, it updates
// the tracker and returns the leader node ID.
func (lr *LeaderResolver) ResolveLocal(shard ShardInstance) (string, error) {
	if shard == nil || shard.Raft() == nil {
		return "", ErrLeaderUnknown
	}

	groupID := shard.GroupID()
	term := shard.Raft().CurrentTerm()

	// Check if the local Raft engine has elected itself leader
	if shard.Raft().Role() == raft.Leader {
		leaderID := shard.Raft().ID()
		lr.tracker.SetLeaderWithTerm(groupID, leaderID, term)
		return leaderID, nil
	}

	// Check if follower/candidate recognizes a remote leader
	if leaderID := shard.Raft().LeaderID(); leaderID != "" {
		lr.tracker.SetLeaderWithTerm(groupID, leaderID, term)
		return leaderID, nil
	}

	// Consult cached tracker entry
	if cached, ok := lr.tracker.GetLeader(groupID); ok {
		return cached, nil
	}

	return "", fmt.Errorf("%w: group %s", ErrLeaderUnknown, groupID)
}

// ResolveShard inspects a map of local replicas for a target shard.
func (lr *LeaderResolver) ResolveShard(shardID sharding.ShardID, replicas map[sharding.ShardID]ShardInstance) (string, error) {
	replica, ok := replicas[shardID]
	if ok {
		return lr.ResolveLocal(replica)
	}

	groupID := raft.GroupID(shardID.GroupID())
	if cached, ok := lr.tracker.GetLeader(groupID); ok {
		return cached, nil
	}

	return "", fmt.Errorf("%w: shard %s", ErrLeaderUnknown, shardID)
}
