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
	onlineState      map[string]bool
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
		onlineState:      make(map[string]bool),
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

	prevOnline := t.onlineState[nodeID]
	t.lastHeartbeat[nodeID] = ts
	t.onlineState[nodeID] = true

	if !prevOnline && t.onStateChange != nil {
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

// CheckTransitions inspects all registered nodes for offline timeouts and triggers callbacks strictly on edge transitions.
func (t *NodeLivenessTracker) CheckTransitions(now time.Time) {
	t.mu.Lock()
	cb := t.onStateChange
	var transitionedOffline []string
	for nid, last := range t.lastHeartbeat {
		if t.onlineState[nid] && now.Sub(last) > t.heartbeatTimeout {
			t.onlineState[nid] = false
			transitionedOffline = append(transitionedOffline, nid)
		}
	}
	t.mu.Unlock()

	if cb != nil {
		for _, nid := range transitionedOffline {
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
