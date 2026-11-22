package tests

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/node"
	"orbit/internal/raft"
)

// TestNodeConfigValidation verifies configuration constraint enforcement.
func TestNodeConfigValidation(t *testing.T) {
	// Empty ID
	c1 := node.NodeConfig{
		ID:          "",
		Address:     "127.0.0.1:2525",
		RaftAddress: "127.0.0.1:3525",
		DataDir:     "data/node-a",
	}
	if !errors.Is(c1.Validate(), node.ErrEmptyNodeID) {
		t.Fatalf("expected ErrEmptyNodeID, got %v", c1.Validate())
	}

	// Empty DataDir
	c2 := node.NodeConfig{
		ID:          "node-a",
		Address:     "127.0.0.1:2525",
		RaftAddress: "127.0.0.1:3525",
		DataDir:     "",
	}
	if !errors.Is(c2.Validate(), node.ErrEmptyDataDir) {
		t.Fatalf("expected ErrEmptyDataDir, got %v", c2.Validate())
	}

	// Self in peers
	c3 := node.NodeConfig{
		ID:          "node-a",
		Address:     "127.0.0.1:2525",
		RaftAddress: "127.0.0.1:3525",
		DataDir:     "data/node-a",
		Peers: []node.PeerConfig{
			{ID: "node-a", Address: "127.0.0.1:2525", RaftAddress: "127.0.0.1:3525"},
		},
	}
	if !errors.Is(c3.Validate(), node.ErrSelfInPeers) {
		t.Fatalf("expected ErrSelfInPeers, got %v", c3.Validate())
	}

	// Duplicate peer ID
	c4 := node.NodeConfig{
		ID:          "node-a",
		Address:     "127.0.0.1:2525",
		RaftAddress: "127.0.0.1:3525",
		DataDir:     "data/node-a",
		Peers: []node.PeerConfig{
			{ID: "node-b", Address: "127.0.0.1:2526", RaftAddress: "127.0.0.1:3526"},
			{ID: "node-b", Address: "127.0.0.1:2527", RaftAddress: "127.0.0.1:3527"},
		},
	}
	if !errors.Is(c4.Validate(), node.ErrDuplicatePeerID) {
		t.Fatalf("expected ErrDuplicatePeerID, got %v", c4.Validate())
	}

	// Invalid timeouts (min >= max)
	c5 := node.NodeConfig{
		ID:                 "node-a",
		Address:            "127.0.0.1:2525",
		RaftAddress:        "127.0.0.1:3525",
		DataDir:            "data/node-a",
		ElectionTimeoutMin: 200 * time.Millisecond,
		ElectionTimeoutMax: 100 * time.Millisecond,
	}
	if !errors.Is(c5.Validate(), node.ErrInvalidTimeouts) {
		t.Fatalf("expected ErrInvalidTimeouts, got %v", c5.Validate())
	}
}

// TestClusterNodeLifecycle verifies starting and stopping independent ORBIT nodes.
func TestClusterNodeLifecycle(t *testing.T) {
	baseDir := t.TempDir()
	configs := node.ClusterConfig3Nodes(baseDir)

	mockNet := raft.NewMockNetwork()
	nodes := make([]*node.Node, len(configs))

	defer func() {
		for _, n := range nodes {
			if n != nil {
				_ = n.Stop()
			}
		}
	}()

	// Instantiate each node with its own isolated data directory and transport
	for i, cfg := range configs {
		tr := mockNet.CreateTransport(cfg.ID, cfg.RaftAddress)
		n, err := node.NewNode(cfg, tr)
		if err != nil {
			t.Fatalf("NewNode(%s): %v", cfg.ID, err)
		}
		nodes[i] = n
	}

	// Verify each node has independent storage and WAL lock
	for _, n := range nodes {
		walPath := filepath.Join(baseDir, n.ID(), "wal.log")
		// Cannot open second engine on same node's data dir
		_, err := node.NewNode(node.NodeConfig{
			ID:          "imposter",
			Address:     "127.0.0.1:9999",
			RaftAddress: "127.0.0.1:9998",
			DataDir:     filepath.Join(baseDir, n.ID()),
		}, nil)
		if err == nil {
			t.Fatalf("expected error opening second engine on %s data dir, got nil", walPath)
		}
	}

	// Start all nodes
	for _, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("Start node %s: %v", n.ID(), err)
		}
	}

	// Wait for a leader to emerge in the cluster
	var leaderID string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range nodes {
			if n.Raft().Role() == raft.Leader {
				leaderID = n.ID()
				break
			}
		}
		if leaderID != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if leaderID == "" {
		t.Fatal("cluster failed to elect a leader")
	}

	// Clean shutdown of all nodes
	for _, n := range nodes {
		if err := n.Stop(); err != nil {
			t.Errorf("Stop node %s failed: %v", n.ID(), err)
		}
	}
}
