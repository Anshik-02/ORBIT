package observability

import (
	"sync"
	"time"

	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
)

// ReplicationHealth values indicate the operational state of cluster replication.
const (
	ReplicationHealthy     = "healthy"
	ReplicationDegraded    = "degraded"
	ReplicationUnavailable = "unavailable"
)

// ClusterStatus reports the live, aggregated health of the entire ORBIT cluster.
type ClusterStatus struct {
	ClusterID         string                           `json:"cluster_id"`
	Timestamp         time.Time                        `json:"timestamp"`
	TotalNodes        int                              `json:"total_nodes"`
	AvailableNodes    int                              `json:"available_nodes"`
	TotalShards       int                              `json:"total_shards"`
	HealthyShards     int                              `json:"healthy_shards"`
	LeaderlessShards  int                              `json:"leaderless_shards"`
	ReplicationHealth string                           `json:"replication_health"`
	Nodes             map[string]NodeStatus            `json:"nodes"`
	Shards            map[sharding.ShardID]ShardStatus `json:"shards"`
}

// ClusterStatusSnapshot captures a versioned snapshot of cluster status.
type ClusterStatusSnapshot struct {
	Version   uint64        `json:"version"`
	Timestamp time.Time     `json:"timestamp"`
	Status    ClusterStatus `json:"status"`
}

// ClusterMonitor builds real-time ClusterStatus snapshots from actual node and Raft states.
type ClusterMonitor struct {
	mu           sync.RWMutex
	metadata     *sharding.ClusterMetadata
	tracker      *router.LeaderTracker
	liveness     *NodeLivenessTracker
	version      uint64
	lastSnapshot ClusterStatusSnapshot
}

// NewClusterMonitor creates a new ClusterMonitor backed by cluster metadata, leader tracker, and liveness.
func NewClusterMonitor(metadata *sharding.ClusterMetadata, tracker *router.LeaderTracker) *ClusterMonitor {
	return &ClusterMonitor{
		metadata: metadata,
		tracker:  tracker,
		liveness: NewNodeLivenessTracker(2 * time.Second),
	}
}

// Liveness returns the underlying NodeLivenessTracker.
func (cm *ClusterMonitor) Liveness() *NodeLivenessTracker {
	return cm.liveness
}

// Collect evaluates actual node states and hosted shard replicas to assemble a ClusterStatus.
func (cm *ClusterMonitor) Collect(
	localNodeID string,
	localShards map[sharding.ShardID]router.ShardInstance,
	nodeLivenessOverride map[string]bool,
) ClusterStatus {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now().UTC()
	totalNodes := len(cm.metadata.PhysicalNodes)
	availableNodes := 0

	// Local node is always online
	cm.liveness.RecordHeartbeat(localNodeID, now)

	// Refresh peer liveness from local Raft shard instances
	if localShards != nil {
		for _, replica := range localShards {
			if replica == nil || replica.Raft() == nil {
				continue
			}
			r := replica.Raft()
			if r.Role() == raft.Leader {
				for peerID, prog := range r.ReplicationStatus() {
					if !prog.LastContact.IsZero() {
						cm.liveness.RecordHeartbeat(peerID, prog.LastContact)
					} else if prog.NextIndex > 1 || prog.MatchIndex > 0 {
						cm.liveness.RecordHeartbeat(peerID, now)
					}
				}
			} else if leaderID := r.LeaderID(); leaderID != "" {
				act := r.LastActivity()
				if !act.IsZero() {
					cm.liveness.RecordHeartbeat(leaderID, act)
				} else {
					cm.liveness.RecordHeartbeat(leaderID, now)
				}
			}
		}
	}
	if cm.tracker != nil {
		for sid := sharding.ShardID(0); sid < sharding.ShardID(cm.metadata.ShardCount); sid++ {
			if ldr, ok := cm.tracker.GetLeader(raft.GroupID(sid.GroupID())); ok && ldr != "" {
				if cm.liveness.LastSeen(ldr).IsZero() {
					cm.liveness.RecordHeartbeat(ldr, now)
				}
			}
		}
	}

	nodesMap := make(map[string]NodeStatus, totalNodes)
	for _, nid := range cm.metadata.PhysicalNodes {
		online := false
		if nodeLivenessOverride != nil {
			online = nodeLivenessOverride[nid]
		} else {
			online = cm.liveness.IsOnline(nid, now)
		}

		if online {
			availableNodes++
		}

		var hosted map[sharding.ShardID]router.ShardInstance
		var lastAct time.Time
		if nid == localNodeID {
			hosted = localShards
		} else {
			lastAct = cm.liveness.LastSeen(nid)
		}

		nodesMap[nid] = BuildNodeStatus(nid, online, hosted, lastAct)
	}

	totalShards := cm.metadata.ShardCount
	healthyShards := 0
	leaderlessShards := 0
	shardsMap := make(map[sharding.ShardID]ShardStatus, totalShards)

	for sid := sharding.ShardID(0); sid < sharding.ShardID(totalShards); sid++ {
		info, err := cm.metadata.GetShard(sid)
		var replicas []string
		if err == nil && info != nil {
			replicas = info.Replicas
		}

		var localReplica router.ShardInstance
		if localShards != nil {
			localReplica = localShards[sid]
		}

		var knownLeader string
		if cm.tracker != nil {
			knownLeader, _ = cm.tracker.GetLeader(raft.GroupID(sid.GroupID()))
		}

		shardSt := BuildShardStatus(sid, replicas, localReplica, knownLeader)
		shardsMap[sid] = shardSt

		if shardSt.Healthy {
			healthyShards++
		} else {
			leaderlessShards++
		}
	}

	// Calculate overall replication health from actual values
	repHealth := ReplicationHealthy
	if healthyShards == 0 || availableNodes == 0 {
		repHealth = ReplicationUnavailable
	} else if healthyShards < totalShards || availableNodes < totalNodes {
		repHealth = ReplicationDegraded
	}

	st := ClusterStatus{
		ClusterID:         cm.metadata.ClusterID,
		Timestamp:         now,
		TotalNodes:        totalNodes,
		AvailableNodes:    availableNodes,
		TotalShards:       totalShards,
		HealthyShards:     healthyShards,
		LeaderlessShards:  leaderlessShards,
		ReplicationHealth: repHealth,
		Nodes:             nodesMap,
		Shards:            shardsMap,
	}

	cm.version++
	cm.lastSnapshot = ClusterStatusSnapshot{
		Version:   cm.version,
		Timestamp: now,
		Status:    st,
	}

	return st
}

// LatestSnapshot returns the most recently captured ClusterStatusSnapshot.
func (cm *ClusterMonitor) LatestSnapshot() ClusterStatusSnapshot {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.lastSnapshot
}
