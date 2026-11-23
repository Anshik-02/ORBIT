package observability

import (
	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
)

// ShardStatus reports real-time consensus and replication progress for a logical shard.
type ShardStatus struct {
	ShardID             sharding.ShardID                 `json:"shard_id"`
	ReplicaNodeIDs      []string                         `json:"replica_node_ids"`
	CurrentLeader       string                           `json:"current_leader"`
	CurrentTerm         uint64                           `json:"current_term"`
	CommitIndex         uint64                           `json:"commit_index"`
	LastAppliedIndex    uint64                           `json:"last_applied_index"`
	ReplicationProgress map[string]raft.ReplicationProgress `json:"replication_progress,omitempty"`
	IsLeader            bool                             `json:"is_leader"`
	Healthy             bool                             `json:"healthy"`
}

// BuildShardStatus builds a real-time ShardStatus from metadata, a local replica instance, and leader tracker.
func BuildShardStatus(
	shardID sharding.ShardID,
	replicas []string,
	replica router.ShardInstance,
	knownLeader string,
) ShardStatus {
	st := ShardStatus{
		ShardID:        shardID,
		ReplicaNodeIDs: replicas,
		CurrentLeader:  knownLeader,
	}

	if replica == nil || replica.Raft() == nil {
		st.Healthy = (st.CurrentLeader != "")
		return st
	}

	r := replica.Raft()
	st.CurrentTerm = r.CurrentTerm()
	st.CommitIndex = r.CommitIndex()
	st.LastAppliedIndex = r.LastApplied()
	st.IsLeader = (r.Role() == raft.Leader)

	if st.IsLeader {
		st.CurrentLeader = r.ID()
		st.ReplicationProgress = r.ReplicationStatus()
	} else if r.LeaderID() != "" {
		st.CurrentLeader = r.LeaderID()
	}

	// A shard is healthy if it has a confirmed leader
	st.Healthy = (st.CurrentLeader != "")

	return st
}
