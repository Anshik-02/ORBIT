package sharding

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseClusterMetadata_Valid(t *testing.T) {
	data := []byte(`{
		"cluster_id": "test-cluster",
		"epoch": 2,
		"shard_count": 2,
		"replication_factor": 2,
		"physical_nodes": ["n1", "n2"],
		"shards": [
			{"id": 0, "replicas": ["n1", "n2"]},
			{"id": 1, "replicas": ["n2", "n1"]}
		]
	}`)

	meta, err := ParseClusterMetadata(data)
	if err != nil {
		t.Fatalf("ParseClusterMetadata failed: %v", err)
	}

	if meta.ClusterID != "test-cluster" {
		t.Errorf("got ClusterID %q, want test-cluster", meta.ClusterID)
	}
	if meta.Epoch != 2 {
		t.Errorf("got Epoch %d, want 2", meta.Epoch)
	}
	if meta.ShardCount != 2 {
		t.Errorf("got ShardCount %d, want 2", meta.ShardCount)
	}
	if meta.ReplicationFactor != 2 {
		t.Errorf("got ReplicationFactor %d, want 2", meta.ReplicationFactor)
	}
	if len(meta.Shards) != 2 {
		t.Fatalf("got %d shards, want 2", len(meta.Shards))
	}

	if err := meta.Validate(); err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestParseClusterMetadata_UnknownFieldsRejected(t *testing.T) {
	data := []byte(`{
		"cluster_id": "test-cluster",
		"epoch": 1,
		"shard_count": 1,
		"replication_factor": 1,
		"physical_nodes": ["n1"],
		"shards": [{"id": 0, "replicas": ["n1"]}],
		"extra_rogue_field": "disallowed"
	}`)

	_, err := ParseClusterMetadata(data)
	if err == nil {
		t.Fatal("expected error due to unknown fields, got nil")
	}
}

func TestParseClusterMetadata_DuplicateShardInJSON(t *testing.T) {
	data := []byte(`{
		"cluster_id": "test-cluster",
		"epoch": 1,
		"shard_count": 2,
		"replication_factor": 1,
		"physical_nodes": ["n1"],
		"shards": [
			{"id": 0, "replicas": ["n1"]},
			{"id": 0, "replicas": ["n1"]}
		]
	}`)

	_, err := ParseClusterMetadata(data)
	if err == nil {
		t.Fatal("expected error due to duplicate shard id in json, got nil")
	}
}

func TestLoadClusterMetadata_File(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "cluster.json")

	content := `{
		"cluster_id": "orbit-file-test",
		"epoch": 1,
		"shard_count": 1,
		"replication_factor": 1,
		"physical_nodes": ["node-1"],
		"shards": [
			{"id": 0, "replicas": ["node-1"]}
		]
	}`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	meta, err := LoadClusterMetadata(cfgPath)
	if err != nil {
		t.Fatalf("LoadClusterMetadata: %v", err)
	}
	if meta.ClusterID != "orbit-file-test" {
		t.Errorf("got %q, want orbit-file-test", meta.ClusterID)
	}

	// Test non-existent file
	_, err = LoadClusterMetadata(filepath.Join(tmpDir, "missing.json"))
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestMarshalClusterMetadata_RoundTrip(t *testing.T) {
	meta := NewClusterMetadata("roundtrip-cluster", 42, 2, 2, []string{"n1", "n2"})
	_ = meta.AddShard(&ShardInfo{ID: 0, Replicas: []string{"n1", "n2"}})
	_ = meta.AddShard(&ShardInfo{ID: 1, Replicas: []string{"n2", "n1"}})

	data, err := MarshalClusterMetadata(meta)
	if err != nil {
		t.Fatalf("MarshalClusterMetadata failed: %v", err)
	}

	parsed, err := ParseClusterMetadata(data)
	if err != nil {
		t.Fatalf("ParseClusterMetadata failed: %v", err)
	}

	if parsed.ClusterID != meta.ClusterID || parsed.Epoch != meta.Epoch {
		t.Errorf("mismatch: %+v vs %+v", parsed, meta)
	}
	if err := parsed.Validate(); err != nil {
		t.Errorf("parsed metadata failed validation: %v", err)
	}
}
