package sharding

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrNodeAlreadyExists = errors.New("sharding: node already exists in cluster")
	ErrNodeNotFoundInMeta = errors.New("sharding: node not found in cluster metadata")
	ErrNodeInUse         = errors.New("sharding: node cannot be removed while hosting active replicas")
	ErrMigrationInProgress = errors.New("sharding: migration already in progress for shard")
	ErrMigrationNotFound = errors.New("sharding: migration task not found")
)

// DynamicCluster manages versioned cluster topology, dynamic node membership,
// persistence to disk, and atomic epoch updates.
type DynamicCluster struct {
	mu       sync.RWMutex
	meta     *ClusterMetadata
	filePath string
}

// NewDynamicCluster initializes or loads a DynamicCluster from filePath.
func NewDynamicCluster(meta *ClusterMetadata, filePath string) (*DynamicCluster, error) {
	if meta == nil {
		return nil, errors.New("sharding: cluster metadata cannot be nil")
	}
	if err := meta.Validate(); err != nil {
		return nil, fmt.Errorf("sharding: validate metadata: %w", err)
	}

	dc := &DynamicCluster{
		meta:     meta,
		filePath: filePath,
	}

	if filePath != "" {
		if err := dc.Save(); err != nil {
			return nil, fmt.Errorf("sharding: persist initial metadata: %w", err)
		}
	}

	return dc, nil
}

// LoadDynamicCluster loads and validates a versioned cluster metadata file from disk.
func LoadDynamicCluster(filePath string) (*DynamicCluster, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("sharding: read metadata file: %w", err)
	}

	var meta ClusterMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("sharding: parse metadata json: %w", err)
	}

	if err := meta.Validate(); err != nil {
		return nil, fmt.Errorf("sharding: invalid metadata on disk: %w", err)
	}

	return &DynamicCluster{
		meta:     &meta,
		filePath: filePath,
	}, nil
}

// Metadata returns a point-in-time copy of current cluster metadata.
func (dc *DynamicCluster) Metadata() *ClusterMetadata {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.cloneLocked()
}

// Epoch returns the current topology version.
func (dc *DynamicCluster) Epoch() uint64 {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.meta.Epoch
}

// AddNode registers a new physical node into the cluster topology and increments Epoch.
func (dc *DynamicCluster) AddNode(nodeID string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	for _, n := range dc.meta.PhysicalNodes {
		if n == nodeID {
			return fmt.Errorf("%w: %s", ErrNodeAlreadyExists, nodeID)
		}
	}

	dc.meta.PhysicalNodes = append(dc.meta.PhysicalNodes, nodeID)
	dc.meta.Epoch++

	if dc.filePath != "" {
		if err := dc.saveLocked(); err != nil {
			// Rollback on failure
			dc.meta.PhysicalNodes = dc.meta.PhysicalNodes[:len(dc.meta.PhysicalNodes)-1]
			dc.meta.Epoch--
			return err
		}
	}
	return nil
}

// RemoveNode safely removes an unassigned physical node from the cluster.
// If the node still hosts shard replicas, ErrNodeInUse is returned.
func (dc *DynamicCluster) RemoveNode(nodeID string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Verify node exists
	foundIdx := -1
	for idx, n := range dc.meta.PhysicalNodes {
		if n == nodeID {
			foundIdx = idx
			break
		}
	}
	if foundIdx == -1 {
		return fmt.Errorf("%w: %s", ErrNodeNotFoundInMeta, nodeID)
	}

	// Invariant: node cannot host any active shard replicas
	for _, shard := range dc.meta.Shards {
		if shard.ContainsNode(nodeID) {
			return fmt.Errorf("%w: node %s hosts shard %s", ErrNodeInUse, nodeID, shard.ID)
		}
	}

	// Remove node
	dc.meta.PhysicalNodes = append(dc.meta.PhysicalNodes[:foundIdx], dc.meta.PhysicalNodes[foundIdx+1:]...)
	dc.meta.Epoch++

	if dc.filePath != "" {
		if err := dc.saveLocked(); err != nil {
			return err
		}
	}
	return nil
}

// UpdateShardPlacement atomically replaces the replica set for a shard and increments Epoch.
func (dc *DynamicCluster) UpdateShardPlacement(shardID ShardID, newReplicas []string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	shard, ok := dc.meta.Shards[shardID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrShardNotFound, shardID)
	}

	oldReplicas := shard.Replicas
	shard.Replicas = newReplicas
	dc.meta.Epoch++

	if err := dc.meta.Validate(); err != nil {
		shard.Replicas = oldReplicas
		dc.meta.Epoch--
		return fmt.Errorf("sharding: invalid replica placement: %w", err)
	}

	if dc.filePath != "" {
		if err := dc.saveLocked(); err != nil {
			shard.Replicas = oldReplicas
			dc.meta.Epoch--
			return err
		}
	}
	return nil
}

// Save flushes current metadata to disk atomically.
func (dc *DynamicCluster) Save() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return dc.saveLocked()
}

func (dc *DynamicCluster) saveLocked() error {
	if dc.filePath == "" {
		return nil
	}

	data, err := json.MarshalIndent(dc.meta, "", "  ")
	if err != nil {
		return fmt.Errorf("sharding: marshal metadata: %w", err)
	}

	dir := filepath.Dir(dc.filePath)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0700)
	}

	tmpPath := dc.filePath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	_ = os.Remove(dc.filePath)
	return os.Rename(tmpPath, dc.filePath)
}

func (dc *DynamicCluster) cloneLocked() *ClusterMetadata {
	shardsCopy := make(map[ShardID]*ShardInfo, len(dc.meta.Shards))
	for id, s := range dc.meta.Shards {
		reps := make([]string, len(s.Replicas))
		copy(reps, s.Replicas)
		shardsCopy[id] = &ShardInfo{
			ID:       s.ID,
			Replicas: reps,
		}
	}

	nodesCopy := make([]string, len(dc.meta.PhysicalNodes))
	copy(nodesCopy, dc.meta.PhysicalNodes)

	return &ClusterMetadata{
		ClusterID:         dc.meta.ClusterID,
		Epoch:             dc.meta.Epoch,
		ShardCount:        dc.meta.ShardCount,
		ReplicationFactor: dc.meta.ReplicationFactor,
		Shards:            shardsCopy,
		PhysicalNodes:     nodesCopy,
	}
}
