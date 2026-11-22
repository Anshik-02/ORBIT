package raft

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrNetworkPartition = errors.New("mock network: node is partitioned/unreachable")
	ErrNodeNotFound     = errors.New("mock network: target node not found")
)

// MockNetwork coordinates in-memory simulated networking across nodes.
type MockNetwork struct {
	mu        sync.RWMutex
	nodes     map[string]*MockTransport
	isolated  map[string]bool
	dropVotes map[string]bool
}

// NewMockNetwork creates a simulated in-memory network for fast, deterministic testing.
func NewMockNetwork() *MockNetwork {
	return &MockNetwork{
		nodes:     make(map[string]*MockTransport),
		isolated:  make(map[string]bool),
		dropVotes: make(map[string]bool),
	}
}

// CreateTransport registers and returns a MockTransport for a specific node ID and optional addresses.
func (n *MockNetwork) CreateTransport(nodeID string, addrs ...string) *MockTransport {
	n.mu.Lock()
	defer n.mu.Unlock()

	t := &MockTransport{
		nodeID:  nodeID,
		network: n,
	}
	n.nodes[nodeID] = t
	for _, addr := range addrs {
		n.nodes[addr] = t
	}
	return t
}

// Isolate disconnects a node from the network (simulates complete network partition).
func (n *MockNetwork) Isolate(nodeID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.isolated[nodeID] = true
}

// Rejoin reconnects an isolated node back to the network.
func (n *MockNetwork) Rejoin(nodeID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.isolated, nodeID)
}

// DropVotes enables dropping of vote requests sent to a node (simulating split vote conditions).
func (n *MockNetwork) DropVotes(nodeID string, drop bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if drop {
		n.dropVotes[nodeID] = true
	} else {
		delete(n.dropVotes, nodeID)
	}
}

// MockTransport implements Transport over an in-memory MockNetwork.
type MockTransport struct {
	mu      sync.RWMutex
	nodeID  string
	network *MockNetwork
	handler RPCHandler
	closed  bool
}

func (t *MockTransport) RegisterHandler(handler RPCHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handler = handler
}

func (t *MockTransport) Start() error {
	return nil
}

func (t *MockTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *MockTransport) SendRequestVote(ctx context.Context, target string, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	t.network.mu.RLock()
	targetNode, exists := t.network.nodes[target]
	if !exists {
		t.network.mu.RUnlock()
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, target)
	}

	if t.network.isolated[t.nodeID] || t.network.isolated[targetNode.nodeID] || t.network.dropVotes[targetNode.nodeID] {
		t.network.mu.RUnlock()
		return nil, ErrNetworkPartition
	}
	t.network.mu.RUnlock()

	targetNode.mu.RLock()
	handler := targetNode.handler
	targetClosed := targetNode.closed
	targetNode.mu.RUnlock()

	if targetClosed || handler == nil {
		return nil, ErrNetworkPartition
	}

	return handler.HandleRequestVote(ctx, req)
}

func (t *MockTransport) SendAppendEntries(ctx context.Context, target string, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	t.network.mu.RLock()
	targetNode, exists := t.network.nodes[target]
	if !exists {
		t.network.mu.RUnlock()
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, target)
	}

	if t.network.isolated[t.nodeID] || t.network.isolated[targetNode.nodeID] {
		t.network.mu.RUnlock()
		return nil, ErrNetworkPartition
	}
	t.network.mu.RUnlock()

	targetNode.mu.RLock()
	handler := targetNode.handler
	targetClosed := targetNode.closed
	targetNode.mu.RUnlock()

	if targetClosed || handler == nil {
		return nil, ErrNetworkPartition
	}

	return handler.HandleAppendEntries(ctx, req)
}
