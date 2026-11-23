package observability

import (
	"time"

	"orbit/internal/router"
	"orbit/internal/sharding"
)

// NodeStatus reports the real-time operational status of an ORBIT physical node.
type NodeStatus struct {
	NodeID               string             `json:"node_id"`
	Status               string             `json:"status"` // "online" or "offline"
	Online               bool               `json:"online"`
	HostedShardIDs       []sharding.ShardID `json:"hosted_shard_ids"`
	RaftGroupCount       int                `json:"raft_group_count"`
	LastObservedActivity time.Time          `json:"last_observed_activity"`
}

// NodeStatusProvider abstracts nodes or mock structures that can report hosted shards and activity.
type NodeStatusProvider interface {
	ID() string
	IsOnline() bool
	HostedShards() map[sharding.ShardID]router.ShardInstance
	LastActivity() time.Time
}

// BuildNodeStatus constructs a real-time NodeStatus descriptor from a physical node.
func BuildNodeStatus(
	nodeID string,
	online bool,
	hostedShards map[sharding.ShardID]router.ShardInstance,
	lastActivity time.Time,
) NodeStatus {
	shardIDs := make([]sharding.ShardID, 0, len(hostedShards))
	for sid := range hostedShards {
		shardIDs = append(shardIDs, sid)
	}

	statusStr := "offline"
	if online {
		statusStr = "online"
	}

	if lastActivity.IsZero() {
		// If node has active shards, determine the latest activity among them
		for _, rep := range hostedShards {
			if rep != nil && rep.Raft() != nil {
				act := rep.Raft().LastActivity()
				if act.After(lastActivity) {
					lastActivity = act
				}
			}
		}
	}

	return NodeStatus{
		NodeID:               nodeID,
		Status:               statusStr,
		Online:               online,
		HostedShardIDs:       shardIDs,
		RaftGroupCount:       len(hostedShards),
		LastObservedActivity: lastActivity,
	}
}
