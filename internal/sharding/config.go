package sharding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// clusterConfigJSON is the wire-format representation of a cluster topology file.
// It maps 1-to-1 with ClusterMetadata but uses JSON-friendly field names and
// primitive shard IDs so the file format stays stable across Go type refactors.
type clusterConfigJSON struct {
	ClusterID         string          `json:"cluster_id"`
	Epoch             uint64          `json:"epoch"`
	ShardCount        int             `json:"shard_count"`
	ReplicationFactor int             `json:"replication_factor"`
	PhysicalNodes     []string        `json:"physical_nodes"`
	Shards            []shardInfoJSON `json:"shards"`
}

type shardInfoJSON struct {
	ID       uint32   `json:"id"`
	Replicas []string `json:"replicas"`
}

// LoadClusterMetadata reads and parses a cluster topology file from path.
//
// The file must be valid JSON matching the structure described by the
// clusterConfigJSON type.  On success the returned *ClusterMetadata is fully
// populated but not yet validated; callers should call Validate() immediately.
//
// Example file:
//
//	{
//	  "cluster_id": "orbit-cluster-01",
//	  "epoch": 1,
//	  "shard_count": 3,
//	  "replication_factor": 3,
//	  "physical_nodes": ["node-a","node-b","node-c","node-d","node-e"],
//	  "shards": [
//	    {"id": 0, "replicas": ["node-a","node-b","node-c"]},
//	    {"id": 1, "replicas": ["node-d","node-e","node-a"]},
//	    {"id": 2, "replicas": ["node-b","node-c","node-d"]}
//	  ]
//	}
func LoadClusterMetadata(path string) (*ClusterMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sharding: read cluster config %q: %w", path, err)
	}
	return ParseClusterMetadata(data)
}

// ParseClusterMetadata deserializes cluster topology JSON from raw bytes.
//
// This is separated from LoadClusterMetadata so callers can supply JSON from
// any source (file, etcd, test fixture) without touching the filesystem.
func ParseClusterMetadata(data []byte) (*ClusterMetadata, error) {
	var cfg clusterConfigJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("sharding: parse cluster config: %w", err)
	}

	m := &ClusterMetadata{
		ClusterID:         cfg.ClusterID,
		Epoch:             cfg.Epoch,
		ShardCount:        cfg.ShardCount,
		ReplicationFactor: cfg.ReplicationFactor,
		PhysicalNodes:     cfg.PhysicalNodes,
		Shards:            make(map[ShardID]*ShardInfo, len(cfg.Shards)),
	}

	for _, s := range cfg.Shards {
		id := ShardID(s.ID)
		if _, exists := m.Shards[id]; exists {
			return nil, fmt.Errorf("%w: %s appears more than once in config file", ErrDuplicateShard, id)
		}
		m.Shards[id] = &ShardInfo{
			ID:       id,
			Replicas: s.Replicas,
		}
	}

	return m, nil
}

// MarshalClusterMetadata serializes m to compact JSON.
// The output can be written to a cluster.json topology file or sent over the wire.
func MarshalClusterMetadata(m *ClusterMetadata) ([]byte, error) {
	shards := make([]shardInfoJSON, 0, len(m.Shards))
	// Emit shards in ascending ID order for deterministic output.
	ids := make([]ShardID, 0, len(m.Shards))
	for id := range m.Shards {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		info := m.Shards[id]
		shards = append(shards, shardInfoJSON{
			ID:       uint32(info.ID),
			Replicas: info.Replicas,
		})
	}

	cfg := clusterConfigJSON{
		ClusterID:         m.ClusterID,
		Epoch:             m.Epoch,
		ShardCount:        m.ShardCount,
		ReplicationFactor: m.ReplicationFactor,
		PhysicalNodes:     m.PhysicalNodes,
		Shards:            shards,
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("sharding: marshal cluster config: %w", err)
	}
	return out, nil
}

// ─── internal helpers ─────────────────────────────────────────────────────────

func sortShardIDs(ids []ShardID) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}
