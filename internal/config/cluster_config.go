package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"orbit/internal/sharding"
)

var (
	ErrClusterIDMismatch = errors.New("config: cluster id mismatch detected")
	ErrEpochRegressed    = errors.New("config: proposed configuration epoch is older than current epoch")
	ErrInvalidConfig     = errors.New("config: invalid cluster configuration")
)

// NodeIdentity represents the immutable, persistent identity of a physical node.
type NodeIdentity struct {
	NodeID    string    `json:"node_id"`
	ClusterID string    `json:"cluster_id"`
	CreatedAt time.Time `json:"created_at"`
}

// SaveNodeIdentity writes node identity atomically to disk.
func SaveNodeIdentity(path string, id NodeIdentity) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadNodeIdentity loads node identity from disk.
func LoadNodeIdentity(path string) (*NodeIdentity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var id NodeIdentity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, err
	}
	return &id, nil
}

// ValidateConfigurationCompatibility verifies that a proposed configuration can be safely applied.
func ValidateConfigurationCompatibility(current, proposed *sharding.ClusterMetadata) error {
	if proposed == nil {
		return ErrInvalidConfig
	}
	if err := proposed.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if current == nil {
		return nil
	}

	if current.ClusterID != "" && proposed.ClusterID != "" && current.ClusterID != proposed.ClusterID {
		return fmt.Errorf("%w: current=%s proposed=%s", ErrClusterIDMismatch, current.ClusterID, proposed.ClusterID)
	}

	if proposed.Epoch < current.Epoch {
		return fmt.Errorf("%w: current=%d proposed=%d", ErrEpochRegressed, current.Epoch, proposed.Epoch)
	}

	return nil
}

// HeartbeatManager tracks periodic node heartbeats and determines liveness transitions.
type HeartbeatManager struct {
	mu             sync.RWMutex
	heartbeats     map[string]time.Time
	defaultTimeout time.Duration
}

// NewHeartbeatManager creates a new tracker.
func NewHeartbeatManager(defaultTimeout time.Duration) *HeartbeatManager {
	if defaultTimeout <= 0 {
		defaultTimeout = 500 * time.Millisecond
	}
	return &HeartbeatManager{
		heartbeats:     make(map[string]time.Time),
		defaultTimeout: defaultTimeout,
	}
}

// RecordHeartbeat logs a fresh heartbeat timestamp for nodeID.
func (h *HeartbeatManager) RecordHeartbeat(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.heartbeats[nodeID] = time.Now()
}

// IsNodeAlive returns whether nodeID sent a heartbeat within timeout.
func (h *HeartbeatManager) IsNodeAlive(nodeID string, timeout time.Duration) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if timeout <= 0 {
		timeout = h.defaultTimeout
	}
	last, ok := h.heartbeats[nodeID]
	if !ok {
		return false
	}
	return time.Since(last) <= timeout
}

// OnlineNodes returns a slice of all nodes currently alive.
func (h *HeartbeatManager) OnlineNodes(timeout time.Duration) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if timeout <= 0 {
		timeout = h.defaultTimeout
	}
	var alive []string
	now := time.Now()
	for nid, last := range h.heartbeats {
		if now.Sub(last) <= timeout {
			alive = append(alive, nid)
		}
	}
	return alive
}
