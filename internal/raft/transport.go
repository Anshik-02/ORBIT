package raft

import (
	"context"
)

// RPCHandler must be implemented by the Raft consensus engine to process inbound RPCs.
type RPCHandler interface {
	HandleRequestVote(ctx context.Context, req *RequestVoteRequest) (*RequestVoteResponse, error)
	HandleAppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesResponse, error)
}

// Transport abstracts network communication between cluster nodes.
// Enables both real HTTP/TCP transport for production and in-memory mock transport for deterministic testing.
// A single transport instance can multiplex RPCs for multiple independent Raft consensus groups using GroupID.
type Transport interface {
	SendRequestVote(ctx context.Context, target string, groupID GroupID, req *RequestVoteRequest) (*RequestVoteResponse, error)
	SendAppendEntries(ctx context.Context, target string, groupID GroupID, req *AppendEntriesRequest) (*AppendEntriesResponse, error)
	RegisterHandler(groupID GroupID, handler RPCHandler) error
	UnregisterHandler(groupID GroupID)
	Start() error
	Close() error
}
