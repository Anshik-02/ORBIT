package node

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"orbit/internal/api"
	"orbit/internal/database"
	"orbit/internal/observability"
	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
	"orbit/internal/storage"
)

// Node coordinates the storage engine, database layer, Raft consensus engine,
// and network servers for an independent member of an ORBIT cluster.
// In Phase 3, a Node can host multiple independent ShardReplica instances.
type Node struct {
	mu        sync.Mutex
	config    NodeConfig
	metadata  *sharding.ClusterMetadata
	transport raft.Transport
	shards    map[sharding.ShardID]*ShardReplica

	// Legacy single-shard fields kept for compatibility:
	engine  *storage.Engine
	db      *database.Database
	raftLog raft.Storage
	raft    *raft.Raft

	kvHandler  *api.KVHandler
	listener   net.Listener
	httpServer *http.Server
	serverErr  atomic.Pointer[error]
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
		shards:    make(map[sharding.ShardID]*ShardReplica),
	}

	// 6. Initialize Raft engine with commit callback wired to storage.Engine.ApplyCommitted
	rEngine, err := raft.New(raft.Config{
		ID:                 cfg.ID,
		GroupID:            raft.DefaultGroupID,
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
	n.shards[0] = &ShardReplica{
		id:      0,
		groupID: raft.DefaultGroupID,
		engine:  eng,
		db:      db,
		raftLog: rStorage,
		raft:    rEngine,
	}

	return n, nil
}

// NewShardedNode initializes an ORBIT physical node that hosts multiple independent shard replicas.
// It consults metadata.ShardsForNode(cfg.ID) to instantiate isolated storage and Raft groups
// only for the shards assigned to this physical node.
func NewShardedNode(
	cfg NodeConfig,
	meta *sharding.ClusterMetadata,
	peerRaftAddrs map[string]string,
	customTransport raft.Transport,
) (*Node, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("node config invalid: %w", err)
	}
	if meta == nil {
		return nil, errors.New("cluster metadata cannot be nil")
	}
	if err := meta.Validate(); err != nil {
		return nil, fmt.Errorf("cluster metadata invalid: %w", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, fmt.Errorf("create data dir %q: %w", cfg.DataDir, err)
	}

	trans := customTransport
	if trans == nil {
		trans = raft.NewHTTPTransport(cfg.RaftAddress)
	}

	n := &Node{
		config:    cfg,
		metadata:  meta,
		transport: trans,
		shards:    make(map[sharding.ShardID]*ShardReplica),
	}

	// For each shard assigned to this node, initialize an isolated replica
	assignedShards := meta.ShardsForNode(cfg.ID)
	for _, shardInfo := range assignedShards {
		// Filter peer map so only peers replicating this shard are configured
		shardPeers := make(map[string]string)
		for _, replicaNodeID := range shardInfo.Replicas {
			if replicaNodeID == cfg.ID {
				continue
			}
			addr, ok := peerRaftAddrs[replicaNodeID]
			if !ok {
				// Fallback to checking cfg.Peers
				for _, p := range cfg.Peers {
					if p.ID == replicaNodeID {
						addr = p.RaftAddress
						ok = true
						break
					}
				}
			}
			if !ok {
				_ = n.Stop()
				return nil, fmt.Errorf("missing raft address for peer %q in shard %s", replicaNodeID, shardInfo.ID)
			}
			shardPeers[replicaNodeID] = addr
		}

		replica, err := NewShardReplica(
			shardInfo.ID,
			cfg.ID,
			cfg.DataDir,
			shardPeers,
			trans,
			cfg.HeartbeatInterval,
			cfg.ElectionTimeoutMin,
			cfg.ElectionTimeoutMax,
		)
		if err != nil {
			_ = n.Stop()
			return nil, fmt.Errorf("create replica for shard %s: %w", shardInfo.ID, err)
		}
		n.shards[shardInfo.ID] = replica

		// For backward compatibility, assign primary/first shard to legacy fields
		if n.engine == nil {
			n.engine = replica.Engine()
			n.db = replica.DB()
			n.raftLog = replica.raftLog
			n.raft = replica.Raft()
		}
	}

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

// Start boots the shared transport, all hosted shard Raft engines, and the client HTTP server.
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

	// 1. Start the shared network transport
	if err := n.transport.Start(); err != nil {
		return fmt.Errorf("start transport: %w", err)
	}

	// 2. Start all hosted shard Raft replicas in deterministic order
	shardIDs := make([]sharding.ShardID, 0, len(n.shards))
	for sid := range n.shards {
		shardIDs = append(shardIDs, sid)
	}
	sort.Slice(shardIDs, func(i, j int) bool {
		return shardIDs[i] < shardIDs[j]
	})

	for _, sid := range shardIDs {
		replica := n.shards[sid]
		if err := replica.Start(); err != nil {
			return n.rollbackStartupLocked(fmt.Errorf("start shard %s: %w", sid, err))
		}
	}
	if n.raft != nil && len(n.shards) == 0 {
		if err := n.raft.Start(); err != nil {
			return n.rollbackStartupLocked(fmt.Errorf("start raft: %w", err))
		}
	}
	n.started = true

	// 3. Start client HTTP API server if address provided
	if n.config.Address != "" {
		mux := http.NewServeMux()

		if n.kvHandler != nil {
			api.RegisterKVRoutes(mux, n.kvHandler)
		} else if n.metadata != nil {
			// Auto-construct sharded router & KVHandler
			ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
			if err != nil {
				return n.rollbackStartupLocked(fmt.Errorf("create hash ring: %w", err))
			}
			for sid := sharding.ShardID(0); sid < sharding.ShardID(n.metadata.ShardCount); sid++ {
				_ = ring.AddShard(sid)
			}
			r := router.New(n.metadata, ring)
			tracker := router.NewLeaderTracker()
			resolver := router.NewLeaderResolver(tracker)
			peerHTTPAddrs := make(map[string]string)
			for _, p := range n.config.Peers {
				peerHTTPAddrs[p.ID] = p.Address
			}
			fwd := router.NewRequestForwarder(r, resolver, n.config.ID, peerHTTPAddrs)
			n.kvHandler = api.NewKVHandler(n, n.metadata, fwd, n.config.ID)
			api.RegisterKVRoutes(mux, n.kvHandler)
		} else {
			// Legacy fallback for single-raft node tests
			peerHTTPAddrs := make(map[string]string)
			for _, p := range n.config.Peers {
				peerHTTPAddrs[p.ID] = p.Address
			}
			handler := api.NewRaftHandler(n.raft, n.db, n.config.ID, n.config.Address, peerHTTPAddrs)
			api.RegisterRaftRoutes(mux, handler)
		}

		ln, err := net.Listen("tcp", n.config.Address)
		if err != nil {
			return n.rollbackStartupLocked(fmt.Errorf("listen on %s: %w", n.config.Address, err))
		}
		n.listener = ln
		n.config.Address = ln.Addr().String()

		n.httpServer = &http.Server{
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		go func() {
			if err := n.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				n.serverErr.Store(&err)
			}
		}()
	}

	n.wireShardLeaderCallbacksLocked()
	return nil
}

// rollbackStartupLocked rolls back running components during a failed Start().
func (n *Node) rollbackStartupLocked(cause error) error {
	var errs []error
	errs = append(errs, cause)
	for _, s := range n.shards {
		if stopErr := s.Stop(); stopErr != nil {
			errs = append(errs, stopErr)
		}
	}
	if n.raft != nil && len(n.shards) == 0 {
		if stopErr := n.raft.Stop(); stopErr != nil {
			errs = append(errs, stopErr)
		}
	}
	if transErr := n.transport.Close(); transErr != nil {
		errs = append(errs, transErr)
	}
	n.closed = true
	return errors.Join(errs...)
}

// Stop cleanly terminates HTTP server, all shard replicas, and closes the shared transport.
// It is safe and idempotent across repeated calls.
func (n *Node) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return nil
	}
	n.closed = true
	n.started = false

	var errs []error

	// 1. Drain HTTP server and close listener
	if n.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := n.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("http server shutdown: %w", err))
			_ = n.httpServer.Close()
		}
	}
	if n.listener != nil {
		_ = n.listener.Close()
	}
	if n.kvHandler != nil && n.kvHandler.EventHub() != nil {
		n.kvHandler.EventHub().Close()
	}

	// 2. Stop all shard replicas and close their storage
	for sid, replica := range n.shards {
		if err := replica.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop shard %s replica: %w", sid, err))
		}
	}

	// 3. Legacy single raft cleanup if not in shards
	if n.raft != nil && len(n.shards) == 0 {
		if err := n.raft.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop legacy raft: %w", err))
		}
	}
	if n.db != nil && len(n.shards) == 0 {
		if err := n.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close legacy db: %w", err))
		}
	}

	// 4. Close the shared transport
	if err := n.transport.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close transport: %w", err))
	}

	return errors.Join(errs...)
}

// Shards returns a copy of the shard replica registry hosted on this physical node.
func (n *Node) Shards() map[sharding.ShardID]*ShardReplica {
	n.mu.Lock()
	defer n.mu.Unlock()
	res := make(map[sharding.ShardID]*ShardReplica, len(n.shards))
	for k, v := range n.shards {
		res[k] = v
	}
	return res
}

// ShardInstances returns a map of hosted shard replicas satisfying router.ShardInstance.
func (n *Node) ShardInstances() map[sharding.ShardID]router.ShardInstance {
	n.mu.Lock()
	defer n.mu.Unlock()
	res := make(map[sharding.ShardID]router.ShardInstance, len(n.shards))
	for k, v := range n.shards {
		res[k] = v
	}
	return res
}

// Shard returns a specific shard replica hosted on this physical node.
func (n *Node) Shard(id sharding.ShardID) (*ShardReplica, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	s, ok := n.shards[id]
	return s, ok
}

// Metadata returns the cluster topology metadata, if configured.
func (n *Node) Metadata() *sharding.ClusterMetadata {
	return n.metadata
}

// Transport returns the underlying network transport.
func (n *Node) Transport() raft.Transport {
	return n.transport
}

// Raft returns the default or primary Raft consensus engine.
func (n *Node) Raft() *raft.Raft {
	return n.raft
}

// DB returns the default or primary public database interface.
func (n *Node) DB() *database.Database {
	return n.db
}

// Engine returns the default or primary storage engine.
func (n *Node) Engine() *storage.Engine {
	return n.engine
}

// ID returns the node ID.
func (n *Node) ID() string {
	return n.config.ID
}

// SetKVHandler attaches the shard-aware KV API handler to this Node and wires leadership callbacks.
func (n *Node) SetKVHandler(h *api.KVHandler) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.kvHandler = h
	n.wireShardLeaderCallbacksLocked()
}

func (n *Node) wireShardLeaderCallbacksLocked() {
	if n.kvHandler == nil {
		return
	}
	for sid, replica := range n.shards {
		shardID := sid
		replica.Raft().SetLeaderCallback(func(gid raft.GroupID, leaderID string) {
			n.onLeaderChanged(shardID, gid, leaderID)
		})
	}
}

func (n *Node) onLeaderChanged(sid sharding.ShardID, gid raft.GroupID, leaderID string) {
	n.mu.Lock()
	handler := n.kvHandler
	n.mu.Unlock()

	if handler == nil {
		return
	}

	if leaderID != "" {
		handler.Forwarder().Resolver().Tracker().SetLeader(gid, leaderID)
	} else {
		handler.Forwarder().Resolver().Tracker().Invalidate(gid)
	}

	evtType := observability.EventLeaderChanged
	if leaderID == n.config.ID {
		evtType = observability.EventLeaderElected
	} else if leaderID == "" {
		evtType = observability.EventLeaderSteppedDown
	}

	handler.BroadcastEvent(observability.NewClusterEvent(
		evtType,
		n.config.ID,
		&sid,
		map[string]any{
			"group_id":  string(gid),
			"leader_id": leaderID,
		},
	))
}

// ServerError returns the last fatal runtime error reported by the HTTP server, if any.
func (n *Node) ServerError() error {
	p := n.serverErr.Load()
	if p == nil {
		return nil
	}
	return *p
}

// KVHandler returns the attached shard-aware KV API handler, if any.
func (n *Node) KVHandler() *api.KVHandler {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.kvHandler
}

// ListenerAddr returns the actual network address this node's HTTP server is listening on.
func (n *Node) ListenerAddr() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.listener != nil {
		return n.listener.Addr().String()
	}
	return n.config.Address
}
