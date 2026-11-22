package raft

import (
	"errors"
	"fmt"
)

// Sentinel Raft errors.
var (
	ErrNotLeader     = errors.New("raft: node is not the cluster leader")
	ErrStopped       = errors.New("raft: consensus engine is stopped")
	ErrNilStorage    = errors.New("raft: storage implementation cannot be nil")
	ErrNilTransport  = errors.New("raft: transport implementation cannot be nil")
	ErrEntryNotFound = errors.New("raft: log entry not found")
	ErrInvalidIndex  = errors.New("raft: invalid log index")
	ErrCorruptState  = errors.New("raft: persistent state is corrupted")
	ErrQuorumLost    = errors.New("raft: lost majority quorum; cannot confirm leadership")
)

// Role represents the current consensus role of a node in the Raft cluster.
type Role int

const (
	Follower Role = iota
	Candidate
	Leader
)

// String returns human-readable representation of the Raft role.
func (r Role) String() string {
	switch r {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return fmt.Sprintf("Unknown(%d)", int(r))
	}
}

// LogEntry represents an individual replicated command within the Raft log.
//
// Invariant Decoupling:
//   - Index: Continuous monotonic consensus log position across the distributed cluster.
//   - Term: The election generation during which the leader appended this entry.
//   - Physical WAL sequence in storage.Engine is separate and decoupled.
type LogEntry struct {
	Index     uint64 `json:"index"`
	Term      uint64 `json:"term"`
	Operation string `json:"operation"` // e.g. "SET", "DELETE"
	Key       string `json:"key"`
	Value     string `json:"value,omitempty"`
}

// RequestVoteRequest is invoked by candidates to gather votes during elections (M3).
type RequestVoteRequest struct {
	Term         uint64 `json:"term"`           // candidate's term
	CandidateID  string `json:"candidate_id"`   // candidate requesting vote
	LastLogIndex uint64 `json:"last_log_index"` // index of candidate's last log entry
	LastLogTerm  uint64 `json:"last_log_term"`  // term of candidate's last log entry
}

// RequestVoteResponse is returned by nodes in response to RequestVoteRequest (M3).
type RequestVoteResponse struct {
	Term        uint64 `json:"term"`         // currentTerm, for candidate to update itself
	VoteGranted bool   `json:"vote_granted"` // true means candidate received vote
}

// AppendEntriesRequest is invoked by the leader to replicate log entries (M5)
// and periodically as heartbeats (M4).
type AppendEntriesRequest struct {
	Term         uint64     `json:"term"`           // leader's term
	LeaderID     string     `json:"leader_id"`     // so follower can redirect clients
	PrevLogIndex uint64     `json:"prev_log_index"` // index of log entry immediately preceding new ones
	PrevLogTerm  uint64     `json:"prev_log_term"`  // term of prevLogIndex entry
	Entries      []LogEntry `json:"entries"`        // log entries to store (empty for heartbeat)
	LeaderCommit uint64     `json:"leader_commit"`  // leader's commitIndex
}

// AppendEntriesResponse is returned by followers in response to AppendEntriesRequest (M4 & M5).
type AppendEntriesResponse struct {
	Term          uint64 `json:"term"`           // currentTerm, for leader to update itself
	Success       bool   `json:"success"`        // true if follower contained entry matching prevLogIndex and prevLogTerm
	ConflictIndex uint64 `json:"conflict_index"` // fast recovery: first index for conflicting term (or log length)
	ConflictTerm  uint64 `json:"conflict_term"`  // fast recovery: term of conflicting entry
}
