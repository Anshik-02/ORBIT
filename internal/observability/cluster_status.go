package observability

import (
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

// ClusterMonitor builds real-time ClusterStatus snapshots from actual node and Raft states.
type ClusterMonitor struct {
	metadata *sharding.ClusterMetadata
	tracker  *router.LeaderTracker
}

// NewClusterMonitor creates a new ClusterMonitor backed by cluster metadata and leader tracker.
func NewClusterMonitor(metadata *sharding.ClusterMetadata, tracker *router.LeaderTracker) *ClusterMonitor {
	return &ClusterMonitor{
		metadata: metadata,
		tracker:  tracker,
	}
}

// Collect evaluates actual node states and hosted shard replicas to assemble a ClusterStatus.
func (cm *ClusterMonitor) Collect(
	localNodeID string,
	localShards map[sharding.ShardID]router.ShardInstance,
	nodeLiveness map[string]bool,
) ClusterStatus {
	now := time.Now().UTC()
	totalNodes := len(cm.metadata.PhysicalNodes)
	availableNodes := 0

	nodesMap := make(map[string]NodeStatus, totalNodes)
	for _, nid := range cm.metadata.PhysicalNodes {
		online := false
		if nodeLiveness != nil {
			online = nodeLiveness[nid]
		} else if nid == localNodeID {
			online = true
		}

		if online {
			availableNodes++
		}

		var hosted map[sharding.ShardID]router.ShardInstance
		var lastAct time.Time
		if nid == localNodeID {
			hosted = localShards
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

	return ClusterStatus{
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
}
