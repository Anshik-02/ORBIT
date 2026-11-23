package observability

import (
	"encoding/json"
	"net/http"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// LatencySummary captures percentile distributions in milliseconds.
type LatencySummary struct {
	P50 float64 `json:"p50_ms"`
	P95 float64 `json:"p95_ms"`
	P99 float64 `json:"p99_ms"`
	Avg float64 `json:"avg_ms"`
}

// MetricsSnapshot represents a point-in-time performance and resource summary.
type MetricsSnapshot struct {
	Timestamp      time.Time      `json:"timestamp"`
	UptimeSeconds  float64        `json:"uptime_seconds"`
	TotalRequests  uint64         `json:"total_requests"`
	ReadRequests   uint64         `json:"read_requests"`
	WriteRequests  uint64         `json:"write_requests"`
	ErrorRequests  uint64         `json:"error_requests"`
	RequestsPerSec float64        `json:"requests_per_sec"`
	ReadsPerSec    float64        `json:"reads_per_sec"`
	WritesPerSec   float64        `json:"writes_per_sec"`
	ErrorRate      float64        `json:"error_rate"`
	Latency        LatencySummary `json:"latency"`
	MemoryAllocMB  float64        `json:"memory_alloc_mb"`
	MemorySysMB    float64        `json:"memory_sys_mb"`
	NumGoroutines  int            `json:"num_goroutines"`
	NumGC          uint32         `json:"num_gc"`
}

// MetricsCollector records throughput, latency distributions, and runtime statistics.
type MetricsCollector struct {
	mu           sync.RWMutex
	readCount    atomic.Uint64
	writeCount   atomic.Uint64
	errorCount   atomic.Uint64
	totalCount   atomic.Uint64
	latencies    []int64 // rolling microseconds
	maxLatencies int
	startTime    time.Time
}

// NewMetricsCollector initializes a collector with rolling sample buffers.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		latencies:    make([]int64, 0, 10000),
		maxLatencies: 10000,
		startTime:    time.Now(),
	}
}

// RecordRequest records a request operation, duration, and error status.
func (m *MetricsCollector) RecordRequest(op string, duration time.Duration, isErr bool) {
	m.totalCount.Add(1)
	if isErr {
		m.errorCount.Add(1)
	}

	switch op {
	case "GET", "SCAN", "PREFIX":
		m.readCount.Add(1)
	case "PUT", "POST", "DELETE":
		m.writeCount.Add(1)
	}

	us := duration.Microseconds()
	m.mu.Lock()
	if len(m.latencies) >= m.maxLatencies {
		m.latencies = m.latencies[1:]
	}
	m.latencies = append(m.latencies, us)
	m.mu.Unlock()
}

// Snapshot computes current rates, percentiles, and runtime diagnostics.
func (m *MetricsCollector) Snapshot() MetricsSnapshot {
	uptime := time.Since(m.startTime).Seconds()
	if uptime <= 0 {
		uptime = 1
	}

	tot := m.totalCount.Load()
	reads := m.readCount.Load()
	writes := m.writeCount.Load()
	errs := m.errorCount.Load()

	m.mu.RLock()
	latCopy := make([]int64, len(m.latencies))
	copy(latCopy, m.latencies)
	m.mu.RUnlock()

	var p50, p95, p99, avg float64
	if len(latCopy) > 0 {
		sort.Slice(latCopy, func(i, j int) bool {
			return latCopy[i] < latCopy[j]
		})

		var sum int64
		for _, v := range latCopy {
			sum += v
		}
		avg = float64(sum) / float64(len(latCopy)) / 1000.0 // convert us to ms

		p50Idx := int(float64(len(latCopy)) * 0.50)
		p95Idx := int(float64(len(latCopy)) * 0.95)
		p99Idx := int(float64(len(latCopy)) * 0.99)

		if p50Idx >= len(latCopy) {
			p50Idx = len(latCopy) - 1
		}
		if p95Idx >= len(latCopy) {
			p95Idx = len(latCopy) - 1
		}
		if p99Idx >= len(latCopy) {
			p99Idx = len(latCopy) - 1
		}

		p50 = float64(latCopy[p50Idx]) / 1000.0
		p95 = float64(latCopy[p95Idx]) / 1000.0
		p99 = float64(latCopy[p99Idx]) / 1000.0
	}

	var mStats runtime.MemStats
	runtime.ReadMemStats(&mStats)

	errRate := 0.0
	if tot > 0 {
		errRate = float64(errs) / float64(tot)
	}

	return MetricsSnapshot{
		Timestamp:      time.Now().UTC(),
		UptimeSeconds:  uptime,
		TotalRequests:  tot,
		ReadRequests:   reads,
		WriteRequests:  writes,
		ErrorRequests:  errs,
		RequestsPerSec: float64(tot) / uptime,
		ReadsPerSec:    float64(reads) / uptime,
		WritesPerSec:   float64(writes) / uptime,
		ErrorRate:      errRate,
		Latency: LatencySummary{
			P50: p50,
			P95: p95,
			P99: p99,
			Avg: avg,
		},
		MemoryAllocMB: float64(mStats.Alloc) / (1024 * 1024),
		MemorySysMB:   float64(mStats.Sys) / (1024 * 1024),
		NumGoroutines: runtime.NumGoroutine(),
		NumGC:         mStats.NumGC,
	}
}

// ServeHTTP exposes metrics snapshot in JSON format.
func (m *MetricsCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	snap := m.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(snap)
}
