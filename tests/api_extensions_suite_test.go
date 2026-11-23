package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/node"
	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
)

func setupTestClusterNode(t *testing.T) (*node.Node, *api.KVHandler, *http.ServeMux) {
	t.Helper()
	dir := t.TempDir()

	net := raft.NewMockNetwork()
	nodes := []string{"n1"}
	meta := sharding.NewClusterMetadata("test-api-cluster", 1, 1, 1, nodes)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"n1"}})

	cfg := node.NodeConfig{
		ID:                 "n1",
		Address:            "127.0.0.1:0",
		RaftAddress:        "127.0.0.1:0",
		DataDir:            filepath.Join(dir, "n1"),
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 50 * time.Millisecond,
		ElectionTimeoutMax: 100 * time.Millisecond,
	}

	tr := net.CreateTransport("n1")
	n, err := node.NewShardedNode(cfg, meta, map[string]string{"n1": "n1"}, tr)
	if err != nil {
		t.Fatalf("NewShardedNode: %v", err)
	}

	ring, _ := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	_ = ring.AddShard(0)
	shardRouter := router.New(meta, ring)
	tracker := router.NewLeaderTracker()
	resolver := router.NewLeaderResolver(tracker)
	forwarder := router.NewRequestForwarder(shardRouter, resolver, "n1", map[string]string{"n1": "http://127.0.0.1:0"})

	// Start shard replica and wait for leadership
	_ = n.Start()
	time.Sleep(150 * time.Millisecond)

	kvHandler := api.NewKVHandler(n, meta, forwarder, "n1")
	mux := http.NewServeMux()
	api.RegisterKVRoutes(mux, kvHandler)

	return n, kvHandler, mux
}

// TestAPI_IdempotencyKeyReplay verifies that repeated write requests with the same
// X-Idempotency-Key return the cached response without re-proposing.
func TestAPI_IdempotencyKeyReplay(t *testing.T) {
	n, _, mux := setupTestClusterNode(t)
	defer n.Stop()

	payload := `{"value": "idempotent-val"}`
	idempKey := "unique-key-123"

	// Request 1
	req1 := httptest.NewRequest(http.MethodPut, "/kv/test-key", strings.NewReader(payload))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Idempotency-Key", idempKey)
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("request 1 failed: status %d, body: %s", w1.Code, w1.Body.String())
	}
	if w1.Header().Get("X-Idempotent-Replay") == "true" {
		t.Errorf("request 1 should not be a replay")
	}

	// Request 2 with same idempotency key
	req2 := httptest.NewRequest(http.MethodPut, "/kv/test-key", strings.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Idempotency-Key", idempKey)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("request 2 failed: status %d", w2.Code)
	}
	if w2.Header().Get("X-Idempotent-Replay") != "true" {
		t.Errorf("request 2 should be marked as an idempotent replay")
	}
}

// TestAPI_BatchOperations verifies batch write and batch read.
func TestAPI_BatchOperations(t *testing.T) {
	n, _, mux := setupTestClusterNode(t)
	defer n.Stop()

	// 1. Batch Write: set k1, k2, k3
	val1 := "v1"
	val2 := "v2"
	val3 := "v3"
	batchWriteBody, _ := json.Marshal(api.BatchWriteRequest{
		Ops: []api.BatchWriteOp{
			{Operation: "SET", Key: "k1", Value: &val1},
			{Operation: "SET", Key: "k2", Value: &val2},
			{Operation: "SET", Key: "k3", Value: &val3},
		},
	})

	writeReq := httptest.NewRequest(http.MethodPost, "/api/v1/kv/batch", bytes.NewReader(batchWriteBody))
	writeReq.Header.Set("Content-Type", "application/json")
	writeRec := httptest.NewRecorder()
	mux.ServeHTTP(writeRec, writeReq)

	if writeRec.Code != http.StatusOK {
		t.Fatalf("batch write failed: code %d, body: %s", writeRec.Code, writeRec.Body.String())
	}

	var writeResp api.BatchWriteResponse
	if err := json.Unmarshal(writeRec.Body.Bytes(), &writeResp); err != nil {
		t.Fatalf("unmarshal batch write resp: %v", err)
	}
	if len(writeResp.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(writeResp.Results))
	}
	for _, r := range writeResp.Results {
		if r.Status != "COMMITTED" {
			t.Errorf("key %s status: %s (err: %s)", r.Key, r.Status, r.Error)
		}
	}

	// 2. Batch Read: get k1, k2, k_missing
	batchReadBody, _ := json.Marshal(api.BatchReadRequest{
		Keys: []string{"k1", "k2", "k_missing"},
	})
	readReq := httptest.NewRequest(http.MethodPost, "/kv/batch", bytes.NewReader(batchReadBody))
	readReq.Header.Set("Content-Type", "application/json")
	readRec := httptest.NewRecorder()
	mux.ServeHTTP(readRec, readReq)

	if readRec.Code != http.StatusOK {
		t.Fatalf("batch read failed: code %d", readRec.Code)
	}

	var readResp api.BatchReadResponse
	if err := json.Unmarshal(readRec.Body.Bytes(), &readResp); err != nil {
		t.Fatalf("unmarshal batch read resp: %v", err)
	}
	if len(readResp.Results) != 3 {
		t.Fatalf("expected 3 read results, got %d", len(readResp.Results))
	}
	if !readResp.Results[0].Found || readResp.Results[0].Value != "v1" {
		t.Errorf("expected k1 found with v1, got %+v", readResp.Results[0])
	}
	if readResp.Results[2].Found {
		t.Errorf("expected k_missing not found, got %+v", readResp.Results[2])
	}
}

// TestAPI_RangeScanAndPagination verifies /kv/scan and /kv/prefix endpoints.
func TestAPI_RangeScanAndPagination(t *testing.T) {
	n, _, mux := setupTestClusterNode(t)
	defer n.Stop()

	// Insert items
	for i := 1; i <= 10; i++ {
		val := fmt.Sprintf("val%02d", i)
		body, _ := json.Marshal(api.SetKVRequest{Value: &val})
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/kv/user:%02d", i), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("put failed: %d", w.Code)
		}
	}

	// 1. Prefix scan with limit 5
	pReq := httptest.NewRequest(http.MethodGet, "/api/v1/kv/prefix?prefix=user:&limit=5", nil)
	pRec := httptest.NewRecorder()
	mux.ServeHTTP(pRec, pReq)

	if pRec.Code != http.StatusOK {
		t.Fatalf("prefix scan failed: %d", pRec.Code)
	}

	var scanResp api.ScanResponse
	if err := json.Unmarshal(pRec.Body.Bytes(), &scanResp); err != nil {
		t.Fatalf("unmarshal scan resp: %v", err)
	}

	if len(scanResp.Items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(scanResp.Items))
	}
	if !scanResp.HasMore {
		t.Errorf("expected HasMore to be true")
	}
	if scanResp.NextCursor != "user:06" {
		t.Errorf("expected NextCursor user:06, got %s", scanResp.NextCursor)
	}

	// 2. Bounded scan from user:06 to user:09
	bReq := httptest.NewRequest(http.MethodGet, "/kv/scan?start=user:06&end=user:09&limit=10", nil)
	bRec := httptest.NewRecorder()
	mux.ServeHTTP(bRec, bReq)

	if bRec.Code != http.StatusOK {
		t.Fatalf("bounded scan failed: %d", bRec.Code)
	}

	var bScanResp api.ScanResponse
	_ = json.Unmarshal(bRec.Body.Bytes(), &bScanResp)
	if len(bScanResp.Items) != 3 {
		t.Fatalf("expected 3 items, got %d (%+v)", len(bScanResp.Items), bScanResp.Items)
	}
}

// TestAPI_OversizedAndMalformedPayloads verifies limits and strict validation.
func TestAPI_OversizedAndMalformedPayloads(t *testing.T) {
	n, _, mux := setupTestClusterNode(t)
	defer n.Stop()

	// 1. Oversized body (> 1MB)
	hugeBody := strings.Repeat("A", 1024*1024+10)
	req := httptest.NewRequest(http.MethodPut, "/kv/k", strings.NewReader(hugeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 Request Entity Too Large, got %d", w.Code)
	}

	// 2. Invalid Content-Type
	req2 := httptest.NewRequest(http.MethodPut, "/kv/k", strings.NewReader(`{"value":"v"}`))
	req2.Header.Set("Content-Type", "text/plain")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415 Unsupported Media Type, got %d", w2.Code)
	}
}
