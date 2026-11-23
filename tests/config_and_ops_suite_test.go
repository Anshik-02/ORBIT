package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/config"
	"orbit/internal/node"
	"orbit/internal/sharding"
	"orbit/internal/storage"
)

func TestConfig_NodeIdentityPersistence(t *testing.T) {
	tempDir := t.TempDir()
	idPath := filepath.Join(tempDir, "node_identity.json")

	orig := config.NodeIdentity{
		NodeID:    "node-a",
		ClusterID: "orbit-prod-01",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	if err := config.SaveNodeIdentity(idPath, orig); err != nil {
		t.Fatalf("failed to save node identity: %v", err)
	}

	loaded, err := config.LoadNodeIdentity(idPath)
	if err != nil {
		t.Fatalf("failed to load node identity: %v", err)
	}
	if loaded.NodeID != orig.NodeID || loaded.ClusterID != orig.ClusterID {
		t.Fatalf("mismatched loaded identity: %+v vs %+v", loaded, orig)
	}
}

func TestConfig_ConflictDetection(t *testing.T) {
	curr := sharding.NewClusterMetadata("orbit-cluster-01", 2, 1, 1, []string{"node-1"})
	_ = curr.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-1"}})

	// Regressed epoch (epoch 1 < epoch 2)
	regressed := sharding.NewClusterMetadata("orbit-cluster-01", 1, 1, 1, []string{"node-1"})
	_ = regressed.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-1"}})
	if err := config.ValidateConfigurationCompatibility(curr, regressed); err == nil {
		t.Fatalf("expected error for regressed epoch")
	}

	// Mismatched cluster ID
	mismatched := sharding.NewClusterMetadata("foreign-cluster-99", 3, 1, 1, []string{"node-1"})
	_ = mismatched.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-1"}})
	if err := config.ValidateConfigurationCompatibility(curr, mismatched); err == nil {
		t.Fatalf("expected error for mismatched cluster id")
	}

	// Valid monotonic progression
	validProposed := sharding.NewClusterMetadata("orbit-cluster-01", 3, 1, 1, []string{"node-1"})
	_ = validProposed.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-1"}})
	if err := config.ValidateConfigurationCompatibility(curr, validProposed); err != nil {
		t.Fatalf("expected valid proposed config to succeed, got %v", err)
	}
}

func TestConfig_HeartbeatManager(t *testing.T) {
	hm := config.NewHeartbeatManager(50 * time.Millisecond)

	hm.RecordHeartbeat("node-1")
	hm.RecordHeartbeat("node-2")

	if !hm.IsNodeAlive("node-1", 50*time.Millisecond) {
		t.Fatalf("expected node-1 to be alive")
	}
	if hm.IsNodeAlive("node-unknown", 50*time.Millisecond) {
		t.Fatalf("expected unknown node to be dead")
	}

	online := hm.OnlineNodes(50 * time.Millisecond)
	if len(online) != 2 {
		t.Fatalf("expected 2 online nodes, got %d", len(online))
	}

	// Wait for heartbeat expiration
	time.Sleep(60 * time.Millisecond)
	if hm.IsNodeAlive("node-1", 50*time.Millisecond) {
		t.Fatalf("expected node-1 to be dead after expiration")
	}
}

func TestOps_StorageBackupAndRestore(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "source_data")
	backupDir := filepath.Join(tempDir, "backups")
	restoreDir := filepath.Join(tempDir, "restored_data")

	walPath := filepath.Join(dataDir, "wal.log")
	eng, err := storage.NewEngine(walPath)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Write keys and flush to create SSTables
	for i := 0; i < 20; i++ {
		_ = eng.Set(string(rune('a'+i)), "backup-test-value")
	}
	eng.RotateMemtable()
	if err := eng.Flush(); err != nil {
		t.Fatalf("failed to flush: %v", err)
	}
	_ = eng.Close()

	// 1. Create backup
	createdBackupPath, err := storage.CreateBackup(dataDir, backupDir)
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	// 2. Restore backup into fresh directory
	if err := storage.RestoreBackup(createdBackupPath, restoreDir); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// 3. Open restored engine and verify all data exists
	restoredWal := filepath.Join(restoreDir, "wal.log")
	restoredEng, err := storage.NewEngine(restoredWal)
	if err != nil {
		t.Fatalf("failed to open restored engine: %v", err)
	}
	defer restoredEng.Close()

	for i := 0; i < 20; i++ {
		k := string(rune('a' + i))
		v, found := restoredEng.Get(k)
		if !found || v != "backup-test-value" {
			t.Fatalf("key %s missing or wrong in restored engine: found=%t val=%s", k, found, v)
		}
	}
}

func TestOps_LivenessAndReadinessEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	handler := api.NewKVHandler(nil, nil, nil, "node-test-1")
	api.RegisterKVRoutes(mux, handler)

	// Liveness probe (/live)
	reqLive := httptest.NewRequest(http.MethodGet, "/live", nil)
	rrLive := httptest.NewRecorder()
	mux.ServeHTTP(rrLive, reqLive)
	if rrLive.Code != http.StatusOK {
		t.Fatalf("expected /live 200, got %d", rrLive.Code)
	}
	var liveBody map[string]any
	if err := json.Unmarshal(rrLive.Body.Bytes(), &liveBody); err != nil || liveBody["status"] != "ALIVE" {
		t.Fatalf("invalid liveness response: %v", liveBody)
	}

	// Readiness probe (/ready)
	reqReady := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rrReady := httptest.NewRecorder()
	mux.ServeHTTP(rrReady, reqReady)
	if rrReady.Code != http.StatusOK {
		t.Fatalf("expected /ready 200, got %d", rrReady.Code)
	}
	var readyBody map[string]any
	if err := json.Unmarshal(rrReady.Body.Bytes(), &readyBody); err != nil || readyBody["status"] != "READY" {
		t.Fatalf("invalid readiness response: %v", readyBody)
	}
}

func TestOps_EnvironmentVariableConfigOverrides(t *testing.T) {
	tempDir := t.TempDir()
	cfgFile := filepath.Join(tempDir, "node.json")

	rawJSON := `{
		"id": "node-original",
		"address": "127.0.0.1:2525",
		"raft_address": "127.0.0.1:3525",
		"data_dir": "data/orig"
	}`
	if err := os.WriteFile(cfgFile, []byte(rawJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Set environment variables
	os.Setenv("ORBIT_NODE_ID", "node-env-override")
	os.Setenv("ORBIT_HTTP_ADDR", "127.0.0.1:9999")
	os.Setenv("ORBIT_DATA_DIR", "data/env-dir")
	defer func() {
		os.Unsetenv("ORBIT_NODE_ID")
		os.Unsetenv("ORBIT_HTTP_ADDR")
		os.Unsetenv("ORBIT_DATA_DIR")
	}()

	loaded, err := node.LoadConfig(cfgFile)
	if err != nil {
		t.Fatalf("failed to load config with env overrides: %v", err)
	}
	if loaded.ID != "node-env-override" {
		t.Fatalf("expected ID to be overridden to node-env-override, got %s", loaded.ID)
	}
	if loaded.Address != "127.0.0.1:9999" {
		t.Fatalf("expected Address overridden to 127.0.0.1:9999, got %s", loaded.Address)
	}
	if loaded.DataDir != "data/env-dir" {
		t.Fatalf("expected DataDir overridden to data/env-dir, got %s", loaded.DataDir)
	}
}
