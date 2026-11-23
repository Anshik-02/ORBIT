package observability

import (
	"sync"
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

// NodeLivenessTracker maintains heartbeat and liveness history across physical nodes.
type NodeLivenessTracker struct {
	mu               sync.RWMutex
	lastHeartbeat    map[string]time.Time
	heartbeatTimeout time.Duration
	onStateChange    func(nodeID string, online bool)
}

// NewNodeLivenessTracker creates an initialized NodeLivenessTracker.
func NewNodeLivenessTracker(heartbeatTimeout time.Duration) *NodeLivenessTracker {
	if heartbeatTimeout <= 0 {
		heartbeatTimeout = 2 * time.Second
	}
	return &NodeLivenessTracker{
		lastHeartbeat:    make(map[string]time.Time),
		heartbeatTimeout: heartbeatTimeout,
	}
}

// SetCallback registers a listener invoked when a node transitions between online and offline.
func (t *NodeLivenessTracker) SetCallback(cb func(nodeID string, online bool)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onStateChange = cb
}

// RecordHeartbeat records an active heartbeat receipt from nodeID.
func (t *NodeLivenessTracker) RecordHeartbeat(nodeID string, ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	prev, exists := t.lastHeartbeat[nodeID]
	wasOnline := exists && time.Since(prev) <= t.heartbeatTimeout
	t.lastHeartbeat[nodeID] = ts

	if !wasOnline && t.onStateChange != nil {
		cb := t.onStateChange
		go cb(nodeID, true)
	}
}

// IsOnline determines if nodeID has been observed within the heartbeat timeout window.
func (t *NodeLivenessTracker) IsOnline(nodeID string, now time.Time) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	last, ok := t.lastHeartbeat[nodeID]
	if !ok || last.IsZero() {
		return false
	}
	return now.Sub(last) <= t.heartbeatTimeout
}

// LastSeen returns the last recorded heartbeat timestamp for nodeID.
func (t *NodeLivenessTracker) LastSeen(nodeID string) time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastHeartbeat[nodeID]
}

// CheckTransitions inspects all registered nodes for offline timeouts.
func (t *NodeLivenessTracker) CheckTransitions(now time.Time) {
	t.mu.RLock()
	cb := t.onStateChange
	var offlineNodes []string
	for nid, last := range t.lastHeartbeat {
		if now.Sub(last) > t.heartbeatTimeout {
			offlineNodes = append(offlineNodes, nid)
		}
	}
	t.mu.RUnlock()

	if cb != nil {
		for _, nid := range offlineNodes {
			cb(nid, false)
		}
	}
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
