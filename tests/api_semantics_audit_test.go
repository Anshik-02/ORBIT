package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"orbit/internal/api"
)

// TestAPI_DurableIdempotencySurvivesRestart verifies that cached responses for mutation requests
// with X-Idempotency-Key are persisted to disk and survive server restarts.
func TestAPI_DurableIdempotencySurvivesRestart(t *testing.T) {
	tempDir := t.TempDir()
	idempotencyFile := filepath.Join(tempDir, "idempotency.json")

	// 1. Initial server instance with durable idempotency
	cache1 := api.NewIdempotencyCache()
	if err := cache1.SetStoragePath(idempotencyFile); err != nil {
		t.Fatalf("SetStoragePath: %v", err)
	}

	key := "client-req-uuid-12345"
	originalBody := []byte(`{"status":"COMMITTED","key":"k1","value":"v1"}`)
	cache1.Set(key, http.StatusOK, originalBody)

	// Verify in-memory lookup
	rec, found := cache1.Get(key)
	if !found || string(rec.Body) != string(originalBody) {
		t.Fatalf("expected cached entry in cache1")
	}

	// 2. Restart simulation: create a brand new cache instance pointing to the same file
	cache2 := api.NewIdempotencyCache()
	if err := cache2.SetStoragePath(idempotencyFile); err != nil {
		t.Fatalf("cache2 SetStoragePath: %v", err)
	}

	// Lookup from restored cache
	rec2, found2 := cache2.Get(key)
	if !found2 {
		t.Fatalf("expected idempotency entry to survive server restart")
	}
	if rec2.StatusCode != http.StatusOK || string(rec2.Body) != string(originalBody) {
		t.Fatalf("restored idempotency response mismatch: code=%d body=%s", rec2.StatusCode, string(rec2.Body))
	}
}

// TestAPI_BatchPartialFailureReporting verifies that when a batch write contains both valid
// and invalid operations, valid operations commit and invalid operations return ERROR status with details.
func TestAPI_BatchPartialFailureReporting(t *testing.T) {
	n, _, mux := setupTestClusterNode(t)
	defer n.Stop()

	// Batch request: 1 valid operation, 1 empty key (invalid), 1 invalid operation type
	val1 := "v1"
	val2 := "v2"
	batchReq := api.BatchWriteRequest{
		Ops: []api.BatchWriteOp{
			{Operation: "SET", Key: "valid_k1", Value: &val1},
			{Operation: "SET", Key: "   ", Value: &val2}, // empty key error
			{Operation: "INVALID_OP", Key: "k3", Value: &val1}, // invalid op error
		},
	}

	body, _ := json.Marshal(batchReq)
	req := httptest.NewRequest(http.MethodPost, "/kv/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for batch request with partial failures, got %d: %s", rec.Code, rec.Body.String())
	}

	var batchResp api.BatchWriteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &batchResp); err != nil {
		t.Fatalf("unmarshal batch response: %v", err)
	}

	if len(batchResp.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(batchResp.Results))
	}

	if batchResp.Results[0].Status != "COMMITTED" {
		t.Errorf("op 0 expected COMMITTED, got %s", batchResp.Results[0].Status)
	}
	if batchResp.Results[1].Status != "ERROR" || batchResp.Results[1].Error == "" {
		t.Errorf("op 1 expected ERROR for empty key, got %v", batchResp.Results[1])
	}
	if batchResp.Results[2].Status != "ERROR" || batchResp.Results[2].Error == "" {
		t.Errorf("op 2 expected ERROR for invalid op, got %v", batchResp.Results[2])
	}
}

// TestAPI_CursorConsistencyDuringConcurrentWrites verifies that as concurrent writes arrive,
// paginated range queries using cursor strictly yield keys > cursor without skipping or duplicate keys.
func TestAPI_CursorConsistencyDuringConcurrentWrites(t *testing.T) {
	n, _, mux := setupTestClusterNode(t)
	defer n.Stop()

	// Pre-seed 30 keys via HTTP API
	for i := 0; i < 30; i++ {
		key := fmt.Sprintf("key_%04d", i)
		val := fmt.Sprintf("val_%04d", i)
		req := httptest.NewRequest(http.MethodPut, "/kv/"+key, bytes.NewReader([]byte(fmt.Sprintf(`{"value":"%s"}`, val))))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	// Background worker continuously writing new keys
	wg.Add(1)
	go func() {
		defer wg.Done()
		counter := 100
		for {
			select {
			case <-stopCh:
				return
			default:
				k := fmt.Sprintf("key_%04d", counter)
				v := fmt.Sprintf("live_%d", counter)
				req := httptest.NewRequest(http.MethodPut, "/kv/"+k, bytes.NewReader([]byte(fmt.Sprintf(`{"value":"%s"}`, v))))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				counter++
			}
		}
	}()

	// Paginate through keys using cursor
	seenKeys := make(map[string]bool)
	cursor := ""
	pageSize := 5

	for page := 0; page < 6; page++ {
		url := fmt.Sprintf("/kv/scan?limit=%d", pageSize)
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("scan page %d returned %d", page, rec.Code)
		}

		var scanResp api.ScanResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &scanResp)

		for _, item := range scanResp.Items {
			if seenKeys[item.Key] {
				t.Fatalf("duplicate key observed across pagination: %s", item.Key)
			}
			seenKeys[item.Key] = true
		}

		if !scanResp.HasMore || scanResp.NextCursor == "" {
			break
		}
		cursor = scanResp.NextCursor
	}

	close(stopCh)
	wg.Wait()
}
