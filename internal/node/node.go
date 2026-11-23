package node

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"orbit/internal/api"
	"orbit/internal/database"
	"orbit/internal/raft"
	"orbit/internal/storage"
)

// Node coordinates the storage engine, database layer, Raft consensus engine,
// and network servers for an independent member of an ORBIT cluster.
type Node struct {
	mu         sync.Mutex
	config     NodeConfig
	engine     *storage.Engine
	db         *database.Database
	raftLog    raft.Storage
	transport  raft.Transport
	raft       *raft.Raft
	httpServer *http.Server
	started    bool
	closed     bool
}

// NewNode initializes an ORBIT cluster node from config.
// If customTransport is provided, it is used (useful for mock in-memory network tests).
// Otherwise, an HTTP-based transport is automatically initialized.
func NewNode(cfg NodeConfig, customTransport raft.Transport) (*Node, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("node config invalid: %w", err)
	}

	// 1. Create data directory with owner-only permissions (0700)
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, fmt.Errorf("create data dir %q: %w", cfg.DataDir, err)
	}

	// 2. Open storage engine (WAL + Memtable) with process-level locking
	walPath := filepath.Join(cfg.DataDir, "wal.log")
	eng, err := storage.NewEngine(walPath)
	if err != nil {
		return nil, fmt.Errorf("open storage engine: %w", err)
	}

	// 3. Wrap storage engine in Database layer
	db, err := database.NewDatabase(eng)
	if err != nil {
		_ = eng.Close()
		return nil, fmt.Errorf("wrap database: %w", err)
	}

	// 4. Open persistent Raft storage (raft.log + metadata.json)
	rStorage, err := raft.OpenFileStorage(cfg.DataDir)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open raft storage: %w", err)
	}

	// 5. Select transport (custom or HTTPTransport)
	trans := customTransport
	if trans == nil {
		trans = raft.NewHTTPTransport(cfg.RaftAddress)
	}

	// Convert peer configs to map[string]string (id -> raftAddress)
	peerMap := make(map[string]string)
	for _, p := range cfg.Peers {
		peerMap[p.ID] = p.RaftAddress
	}

	n := &Node{
		config:    cfg,
		engine:    eng,
		db:        db,
		raftLog:   rStorage,
		transport: trans,
	}

	// 6. Initialize Raft engine with commit callback wired to storage.Engine.ApplyCommitted
	rEngine, err := raft.New(raft.Config{
		ID:                 cfg.ID,
		Peers:              peerMap,
		Storage:            rStorage,
		Transport:          trans,
		HeartbeatInterval:  cfg.HeartbeatInterval,
		ElectionTimeoutMin: cfg.ElectionTimeoutMin,
		ElectionTimeoutMax: cfg.ElectionTimeoutMax,
		CommitCallback:     n.onCommittedEntry,
		InitialLastApplied: eng.LastAppliedIndex(), // seed from recovered WAL to prevent re-application
	})
	if err != nil {
		_ = rStorage.Close()
		_ = db.Close()
		return nil, fmt.Errorf("init raft engine: %w", err)
	}
	n.raft = rEngine

	return n, nil
}

// onCommittedEntry is invoked by the Raft engine when an entry reaches majority quorum.
// It applies the mutation to the local database Memtable and records it in the storage WAL.
func (n *Node) onCommittedEntry(entry raft.LogEntry) error {
	return n.engine.ApplyCommitted(storage.CommittedCommand{
		RaftIndex: entry.Index,
		RaftTerm:  entry.Term,
		Operation: entry.Operation,
		Key:       entry.Key,
		Value:     entry.Value,
	})
}

// Start boots the Raft consensus engine and the client HTTP server.
// It is safe and idempotent across repeated calls.
func (n *Node) Start() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return errors.New("node is already closed")
	}
	if n.started {
		return nil
	}

	// Start Raft consensus engine
	if err := n.raft.Start(); err != nil {
		return fmt.Errorf("start raft: %w", err)
	}
	n.started = true

	// Start client HTTP API server if address provided
	if n.config.Address != "" {
		mux := http.NewServeMux()
		peerHTTPAddrs := make(map[string]string)
		for _, p := range n.config.Peers {
			peerHTTPAddrs[p.ID] = p.Address
		}
		handler := api.NewRaftHandler(n.raft, n.db, n.config.ID, n.config.Address, peerHTTPAddrs)
		api.RegisterRaftRoutes(mux, handler)

		n.httpServer = &http.Server{
			Addr:         n.config.Address,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		go func() {
			if err := n.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				// Server shut down
			}
		}()
	}

	return nil
}

// Stop cleanly terminates HTTP server, Raft engine, and closes database storage.
// It is safe and idempotent across repeated calls.
func (n *Node) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return nil
	}
	n.closed = true
	n.started = false

	// 1. Drain HTTP server
	if n.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = n.httpServer.Shutdown(ctx)
	}

	// 2. Stop Raft consensus engine
	if n.raft != nil {
		_ = n.raft.Stop()
	}

	// 3. Close database and release storage lock
	return n.db.Close()
}

// Raft returns the underlying Raft consensus engine.
func (n *Node) Raft() *raft.Raft {
	return n.raft
}

// DB returns the public database interface.
func (n *Node) DB() *database.Database {
	return n.db
}

// Engine returns the storage engine.
func (n *Node) Engine() *storage.Engine {
	return n.engine
}

// ID returns the node ID.
func (n *Node) ID() string {
	return n.config.ID
}
