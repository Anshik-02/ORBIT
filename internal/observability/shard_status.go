package observability

import (
	"time"

	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
)

// ShardState indicates the consensus and replication status of a logical shard.
type ShardState string

const (
	// ShardHealthy indicates the leader has verified majority quorum and the shard is operational.
	ShardHealthy ShardState = "Healthy"

	// ShardDegraded indicates the shard has quorum but some replicas are lagging or offline.
	ShardDegraded ShardState = "Degraded"

	// ShardUnavailable indicates no usable quorum or leader exists.
	ShardUnavailable ShardState = "Unavailable"

	// ShardRecovering indicates a replica is actively catching up missing entries after an outage.
	ShardRecovering ShardState = "Recovering"
)

// ShardStatus reports real-time consensus and replication progress for a logical shard.
type ShardStatus struct {
	ShardID             sharding.ShardID                    `json:"shard_id"`
	State               ShardState                          `json:"state"`
	ReplicaNodeIDs      []string                            `json:"replica_node_ids"`
	CurrentLeader       string                              `json:"current_leader"`
	CurrentTerm         uint64                              `json:"current_term"`
	CommitIndex         uint64                              `json:"commit_index"`
	LastAppliedIndex    uint64                              `json:"last_applied_index"`
	ReplicationProgress map[string]raft.ReplicationProgress `json:"replication_progress,omitempty"`
	ReplicationLag      map[string]uint64                   `json:"replication_lag,omitempty"`
	AvailableReplicas   []string                            `json:"available_replicas"`
	QuorumAvailable     bool                                `json:"quorum_available"`
	IsLeader            bool                                `json:"is_leader"`
	Healthy             bool                                `json:"healthy"`
	LastCommunication   time.Time                           `json:"last_communication"`
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
		State:          ShardUnavailable,
		ReplicaNodeIDs: replicas,
		CurrentLeader:  knownLeader,
		Healthy:        false,
	}

	if len(replicas) == 0 {
		return st
	}

	totalReplicas := len(replicas)
	quorum := (totalReplicas / 2) + 1

	if replica == nil || replica.Raft() == nil {
		if knownLeader != "" {
			st.Healthy = true
			st.State = ShardHealthy
			st.QuorumAvailable = true
			st.AvailableReplicas = []string{knownLeader}
		} else {
			st.State = ShardUnavailable
			st.Healthy = false
			st.QuorumAvailable = false
		}
		return st
	}

	r := replica.Raft()
	st.CurrentTerm = r.CurrentTerm()
	st.CommitIndex = r.CommitIndex()
	st.LastAppliedIndex = r.LastApplied()
	st.IsLeader = (r.Role() == raft.Leader)
	st.LastCommunication = r.LastActivity()

	if st.IsLeader {
		st.CurrentLeader = r.ID()
		st.ReplicationProgress = r.ReplicationStatus()

		// Calculate replication lag and count responsive peers
		st.ReplicationLag = make(map[string]uint64, len(st.ReplicationProgress))
		availableCount := 1 // self
		st.AvailableReplicas = append(st.AvailableReplicas, r.ID())

		lastIdx := st.CommitIndex
		for peerID, prog := range st.ReplicationProgress {
			if lastIdx > prog.MatchIndex {
				st.ReplicationLag[peerID] = lastIdx - prog.MatchIndex
			} else {
				st.ReplicationLag[peerID] = 0
			}

			// Peer is available if it has responded recently (within 3 seconds)
			peerResponsive := !prog.LastContact.IsZero() && time.Since(prog.LastContact) < 3*time.Second
			if peerResponsive {
				availableCount++
				st.AvailableReplicas = append(st.AvailableReplicas, peerID)
			}
		}

		st.QuorumAvailable = availableCount >= quorum
		if st.QuorumAvailable {
			st.Healthy = true
			if availableCount == totalReplicas {
				st.State = ShardHealthy
			} else {
				st.State = ShardDegraded
			}
		} else {
			st.State = ShardUnavailable
			st.Healthy = false
		}
		return st
	}

	// Follower or Candidate path
	if r.Role() == raft.Candidate {
		st.State = ShardUnavailable
		st.Healthy = false
		st.QuorumAvailable = false
		st.CurrentLeader = ""
		return st
	}

	// Follower: check whether active communication with leader exists
	leaderID := r.LeaderID()
	if leaderID != "" {
		st.CurrentLeader = leaderID
	}

	timeSinceActivity := time.Since(st.LastCommunication)
	if st.CurrentLeader != "" && timeSinceActivity <= 3*time.Second {
		st.QuorumAvailable = true
		st.Healthy = true
		st.AvailableReplicas = []string{r.ID(), st.CurrentLeader}

		// Check if follower is recovering/catching up (commitIndex far ahead of lastApplied)
		if st.CommitIndex > st.LastAppliedIndex+10 {
			st.State = ShardRecovering
		} else {
			st.State = ShardHealthy
		}
	} else {
		// Follower disconnected from leader or leader inactive
		st.State = ShardUnavailable
		st.Healthy = false
		st.QuorumAvailable = false
	}

	return st
}
