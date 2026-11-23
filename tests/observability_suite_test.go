package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/observability"
)

func TestObservability_MetricsCollection(t *testing.T) {
	collector := observability.NewMetricsCollector()

	// Record sample requests
	collector.RecordRequest("GET", 2*time.Millisecond, false)
	collector.RecordRequest("GET", 4*time.Millisecond, false)
	collector.RecordRequest("PUT", 10*time.Millisecond, false)
	collector.RecordRequest("PUT", 20*time.Millisecond, true) // error request

	snap := collector.Snapshot()
	if snap.TotalRequests != 4 {
		t.Fatalf("expected 4 total requests, got %d", snap.TotalRequests)
	}
	if snap.ReadRequests != 2 {
		t.Fatalf("expected 2 read requests, got %d", snap.ReadRequests)
	}
	if snap.WriteRequests != 2 {
		t.Fatalf("expected 2 write requests, got %d", snap.WriteRequests)
	}
	if snap.ErrorRequests != 1 {
		t.Fatalf("expected 1 error request, got %d", snap.ErrorRequests)
	}
	if snap.ErrorRate != 0.25 {
		t.Fatalf("expected error rate 0.25, got %f", snap.ErrorRate)
	}
	if snap.Latency.P50 <= 0 || snap.Latency.P95 <= 0 || snap.Latency.P99 <= 0 {
		t.Fatalf("expected positive latency percentiles, got %+v", snap.Latency)
	}

	// Test HTTP endpoint serving metrics JSON
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	collector.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	var httpSnap observability.MetricsSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &httpSnap); err != nil {
		t.Fatalf("failed to decode metrics JSON: %v", err)
	}
	if httpSnap.TotalRequests != 4 {
		t.Fatalf("expected 4 total requests in HTTP metrics snapshot, got %d", httpSnap.TotalRequests)
	}
}

func TestObservability_EventSequencingAndReplay(t *testing.T) {
	hub := observability.NewEventHub()

	// Publish multiple events
	hub.Publish(observability.ClusterEvent{
		EventID: "evt-1",
		Type:    observability.EventLeaderChanged,
		NodeID:  "node-1",
		Data:    map[string]any{"term": 1},
	})
	hub.Publish(observability.ClusterEvent{
		EventID: "evt-2",
		Type:    observability.EventShardUpdated,
		NodeID:  "node-1",
		Data:    map[string]any{"shard": 1},
	})
	hub.Publish(observability.ClusterEvent{
		EventID: "evt-3",
		Type:    observability.EventNodeJoined,
		NodeID:  "node-2",
	})

	// Replay all events since sequence 0
	allEvents := hub.EventsSince(0)
	if len(allEvents) != 3 {
		t.Fatalf("expected 3 events, got %d", len(allEvents))
	}
	if allEvents[0].Sequence != 1 || allEvents[1].Sequence != 2 || allEvents[2].Sequence != 3 {
		t.Fatalf("unexpected sequences: %d, %d, %d", allEvents[0].Sequence, allEvents[1].Sequence, allEvents[2].Sequence)
	}

	// Replay events since sequence 1
	replay := hub.EventsSince(1)
	if len(replay) != 2 {
		t.Fatalf("expected 2 replayed events, got %d", len(replay))
	}
	if replay[0].EventID != "evt-2" || replay[1].EventID != "evt-3" {
		t.Fatalf("unexpected replayed events: %+v", replay)
	}
}

func TestObservability_StructuredLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := observability.NewStructuredLogger(&buf, "storage-engine")

	corrID := "req-corr-999"
	logger.Info(corrID, "memtable flushed successfully", map[string]any{
		"sstable_id": "sst_001.sst",
		"keys_count": 1500,
	})

	output := buf.String()
	if !strings.Contains(output, "req-corr-999") {
		t.Fatalf("expected output to contain correlation ID, got %s", output)
	}
	if !strings.Contains(output, "storage-engine") {
		t.Fatalf("expected output to contain subsystem tag, got %s", output)
	}
	if !strings.Contains(output, "memtable flushed successfully") {
		t.Fatalf("expected output to contain message, got %s", output)
	}

	var rec observability.LogRecord
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("failed to decode JSON log record: %v", err)
	}
	if rec.Level != observability.LogLevelInfo {
		t.Fatalf("expected level INFO, got %s", rec.Level)
	}
	if rec.CorrelationID != corrID {
		t.Fatalf("expected correlation ID %s, got %s", corrID, rec.CorrelationID)
	}
	if rec.Subsystem != "storage-engine" {
		t.Fatalf("expected subsystem storage-engine, got %s", rec.Subsystem)
	}
}

func TestObservability_KVHandlerMetricsIntegration(t *testing.T) {
	mux := http.NewServeMux()
	handler := api.NewKVHandler(nil, nil, nil, "node-1")
	api.RegisterKVRoutes(mux, handler)

	// Verify /metrics route is responsive
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 from /metrics, got %d", rr.Code)
	}

	// Verify /api/v1/metrics route is responsive
	reqV1 := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	rrV1 := httptest.NewRecorder()
	mux.ServeHTTP(rrV1, reqV1)

	if rrV1.Code != http.StatusOK {
		t.Fatalf("expected status 200 from /api/v1/metrics, got %d", rrV1.Code)
	}
}
