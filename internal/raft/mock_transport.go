package raft

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
		nodeID:   nodeID,
		network:  n,
		handlers: make(map[GroupID]RPCHandler),
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

// MockTransport implements Transport over an in-memory MockNetwork with multi-group support.
type MockTransport struct {
	mu       sync.RWMutex
	nodeID   string
	network  *MockNetwork
	handlers map[GroupID]RPCHandler
	closed   bool
}

func (t *MockTransport) RegisterHandler(groupID GroupID, handler RPCHandler) error {
	if handler == nil {
		return ErrNilHandler
	}
	if strings.TrimSpace(string(groupID)) == "" {
		return ErrInvalidGroupID
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.handlers == nil {
		t.handlers = make(map[GroupID]RPCHandler)
	}
	if _, exists := t.handlers[groupID]; exists {
		return fmt.Errorf("%w: group %q", ErrDuplicateGroup, groupID)
	}
	t.handlers[groupID] = handler
	return nil
}

func (t *MockTransport) UnregisterHandler(groupID GroupID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if groupID == "" {
		groupID = DefaultGroupID
	}
	delete(t.handlers, groupID)
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

func (t *MockTransport) SendRequestVote(ctx context.Context, target string, groupID GroupID, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	if groupID == "" {
		groupID = DefaultGroupID
	}

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
	handler := targetNode.handlers[groupID]
	targetClosed := targetNode.closed
	targetNode.mu.RUnlock()

	if targetClosed {
		return nil, ErrNetworkPartition
	}
	if handler == nil {
		return nil, fmt.Errorf("%w: group %s on target %s", ErrGroupNotFound, groupID, target)
	}

	return handler.HandleRequestVote(ctx, req)
}

func (t *MockTransport) SendAppendEntries(ctx context.Context, target string, groupID GroupID, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	if groupID == "" {
		groupID = DefaultGroupID
	}

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
	handler := targetNode.handlers[groupID]
	targetClosed := targetNode.closed
	targetNode.mu.RUnlock()

	if targetClosed {
		return nil, ErrNetworkPartition
	}
	if handler == nil {
		return nil, fmt.Errorf("%w: group %s on target %s", ErrGroupNotFound, groupID, target)
	}

	return handler.HandleAppendEntries(ctx, req)
}

func (t *MockTransport) SendInstallSnapshot(ctx context.Context, target string, groupID GroupID, req *InstallSnapshotRequest) (*InstallSnapshotResponse, error) {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return nil, ErrTransportClosed
	}
	t.mu.RUnlock()

	if groupID == "" {
		groupID = DefaultGroupID
	}

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
	handler := targetNode.handlers[groupID]
	targetClosed := targetNode.closed
	targetNode.mu.RUnlock()

	if targetClosed {
		return nil, ErrNetworkPartition
	}
	if handler == nil {
		return nil, fmt.Errorf("%w: group %s on target %s", ErrGroupNotFound, groupID, target)
	}

	return handler.HandleInstallSnapshot(ctx, req)
}
