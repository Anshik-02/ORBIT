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
type Transport interface {
	SendRequestVote(ctx context.Context, target string, req *RequestVoteRequest) (*RequestVoteResponse, error)
	SendAppendEntries(ctx context.Context, target string, req *AppendEntriesRequest) (*AppendEntriesResponse, error)
	RegisterHandler(handler RPCHandler)
	Start() error
	Close() error
}
