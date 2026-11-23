package node

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"orbit/internal/database"
	"orbit/internal/raft"
	"orbit/internal/sharding"
	"orbit/internal/storage"
)

// ShardReplica encapsulates an independent Raft consensus engine, persistent log,
// storage engine (WAL + Memtable), and database layer for one logical shard.
//
// Invariants & Safety:
//   - Storage Isolation (Rule 2): Dedicated directory, WAL, and Raft state.
//   - Raft Group Independence (Rule 1): Dedicated Raft instance, terms, logs, and election timers.
//   - Transport Multiplexing (Rule 3): Shares the physical node's transport, registering with its GroupID.
type ShardReplica struct {
	id      sharding.ShardID
	groupID raft.GroupID
	engine  *storage.Engine
	db      *database.Database
	raftLog raft.Storage
	raft    *raft.Raft
}

// NewShardReplica initializes an isolated shard replica within its own data directory.
func NewShardReplica(
	shardID sharding.ShardID,
	nodeID string,
	baseDir string,
	peers map[string]string,
	transport raft.Transport,
	heartbeatInterval, electionTimeoutMin, electionTimeoutMax time.Duration,
) (*ShardReplica, error) {
	// Storage Isolation: data/{nodeID}/shard-{N}/
	shardDir := filepath.Join(baseDir, shardID.String())
	if err := os.MkdirAll(shardDir, 0700); err != nil {
		return nil, fmt.Errorf("create shard dir %q: %w", shardDir, err)
	}

	walPath := filepath.Join(shardDir, "wal.log")
	eng, err := storage.NewEngine(walPath)
	if err != nil {
		return nil, fmt.Errorf("open shard engine: %w", err)
	}

	db, err := database.NewDatabase(eng)
	if err != nil {
		_ = eng.Close()
		return nil, fmt.Errorf("wrap shard database: %w", err)
	}

	rStorage, err := raft.OpenFileStorage(shardDir)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open shard raft storage: %w", err)
	}

	sr := &ShardReplica{
		id:      shardID,
		groupID: raft.GroupID(shardID.GroupID()),
		engine:  eng,
		db:      db,
		raftLog: rStorage,
	}

	commitCallback := func(entry raft.LogEntry) error {
		return eng.ApplyCommitted(storage.CommittedCommand{
			RaftIndex: entry.Index,
			RaftTerm:  entry.Term,
			Operation: entry.Operation,
			Key:       entry.Key,
			Value:     entry.Value,
		})
	}

	snapshotCallback := func() (uint64, []byte, error) {
		return eng.ExportSnapshotBytes()
	}

	snapshotRestoreCallback := func(meta raft.SnapshotMetadata, data []byte) error {
		return eng.RestoreSnapshotBytes(storage.SnapshotMetadata{
			Index: meta.LastIncludedIndex,
			Term:  meta.LastIncludedTerm,
		}, data)
	}

	rEngine, err := raft.New(raft.Config{
		ID:                      nodeID,
		GroupID:                 sr.groupID,
		Peers:                   peers,
		Storage:                 rStorage,
		Transport:               transport,
		HeartbeatInterval:       heartbeatInterval,
		ElectionTimeoutMin:      electionTimeoutMin,
		ElectionTimeoutMax:      electionTimeoutMax,
		CommitCallback:          commitCallback,
		InitialLastApplied:      eng.LastAppliedIndex(),
		SnapshotCallback:        snapshotCallback,
		SnapshotRestoreCallback: snapshotRestoreCallback,
	})
	if err != nil {
		_ = rStorage.Close()
		_ = db.Close()
		return nil, fmt.Errorf("init shard raft engine: %w", err)
	}
	sr.raft = rEngine

	return sr, nil
}

// ID returns the logical ShardID.
func (s *ShardReplica) ID() sharding.ShardID {
	return s.id
}

// GroupID returns the consensus GroupID.
func (s *ShardReplica) GroupID() raft.GroupID {
	return s.groupID
}

// Raft returns the independent Raft consensus engine for this shard.
func (s *ShardReplica) Raft() *raft.Raft {
	return s.raft
}

// DB returns the isolated Database interface for this shard.
func (s *ShardReplica) DB() *database.Database {
	return s.db
}

// Engine returns the isolated Storage Engine for this shard.
func (s *ShardReplica) Engine() *storage.Engine {
	return s.engine
}

// ReplicationLag returns follower replication lag metrics if this replica is the leader.
func (s *ShardReplica) ReplicationLag() map[string]uint64 {
	return s.raft.ReplicationLag()
}

// Start begins consensus activity for this shard replica.
func (s *ShardReplica) Start() error {
	return s.raft.Start()
}

// Stop cleanly stops the Raft engine and closes database storage for this shard.
func (s *ShardReplica) Stop() error {
	var errs []error
	if err := s.raft.Stop(); err != nil {
		errs = append(errs, err)
	}
	if err := s.db.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
