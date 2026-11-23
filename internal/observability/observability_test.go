package observability_test

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"orbit/internal/observability"
	"orbit/internal/raft"
	"orbit/internal/router"
	"orbit/internal/sharding"
)

func TestNodeStatus_Build(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := observability.BuildNodeStatus("node-a", true, nil, now)

	if st.NodeID != "node-a" || !st.Online || st.Status != "online" {
		t.Errorf("unexpected node status: %+v", st)
	}
	if st.RaftGroupCount != 0 {
		t.Errorf("expected 0 groups, got %d", st.RaftGroupCount)
	}
	if !st.LastObservedActivity.Equal(now) {
		t.Errorf("unexpected activity time: %v", st.LastObservedActivity)
	}

	offlineSt := observability.BuildNodeStatus("node-b", false, nil, time.Time{})
	if offlineSt.Online || offlineSt.Status != "offline" {
		t.Errorf("expected offline status, got %+v", offlineSt)
	}
}

func TestShardStatus_Build(t *testing.T) {
	t.Parallel()

	replicas := []string{"node-a", "node-b", "node-c"}
	st := observability.BuildShardStatus(0, replicas, nil, "node-b")

	if st.ShardID != 0 {
		t.Errorf("expected shard 0, got %d", st.ShardID)
	}
	if len(st.ReplicaNodeIDs) != 3 {
		t.Errorf("expected 3 replicas, got %d", len(st.ReplicaNodeIDs))
	}
	if st.CurrentLeader != "node-b" {
		t.Errorf("expected leader node-b, got %s", st.CurrentLeader)
	}
	if !st.Healthy {
		t.Errorf("expected shard with leader to be healthy")
	}

	leaderlessSt := observability.BuildShardStatus(1, replicas, nil, "")
	if leaderlessSt.Healthy {
		t.Errorf("expected leaderless shard to be unhealthy")
	}
}

func TestClusterStatus_Collect(t *testing.T) {
	t.Parallel()

	meta := sharding.NewClusterMetadata("test-obs-cluster", 1, 2, 2, []string{"node-a", "node-b"})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: []string{"node-a", "node-b"}})
	_ = meta.AddShard(&sharding.ShardInfo{ID: 1, Replicas: []string{"node-a", "node-b"}})

	tracker := router.NewLeaderTracker()
	tracker.SetLeader(raft.GroupID("shard-0"), "node-a")
	tracker.SetLeader(raft.GroupID("shard-1"), "node-b")

	monitor := observability.NewClusterMonitor(meta, tracker)

	// Case 1: All nodes online, all shards have leaders -> healthy
	liveness := map[string]bool{"node-a": true, "node-b": true}
	status := monitor.Collect("node-a", nil, liveness)

	if status.TotalNodes != 2 || status.AvailableNodes != 2 {
		t.Errorf("expected 2/2 nodes, got total=%d available=%d", status.TotalNodes, status.AvailableNodes)
	}
	if status.TotalShards != 2 || status.HealthyShards != 2 || status.LeaderlessShards != 0 {
		t.Errorf("expected 2 healthy shards, got %+v", status)
	}
	if status.ReplicationHealth != observability.ReplicationHealthy {
		t.Errorf("expected replication healthy, got %s", status.ReplicationHealth)
	}

	// Case 2: One node down -> degraded
	liveness["node-b"] = false
	statusDegraded := monitor.Collect("node-a", nil, liveness)
	if statusDegraded.ReplicationHealth != observability.ReplicationDegraded {
		t.Errorf("expected degraded, got %s", statusDegraded.ReplicationHealth)
	}

	// Case 3: All nodes down -> unavailable
	liveness["node-a"] = false
	statusUnavail := monitor.Collect("node-a", nil, liveness)
	if statusUnavail.ReplicationHealth != observability.ReplicationUnavailable {
		t.Errorf("expected unavailable, got %s", statusUnavail.ReplicationHealth)
	}
}

func TestEventHub_PubSub(t *testing.T) {
	t.Parallel()

	hub := observability.NewEventHub()
	defer hub.Close()

	ch1 := hub.Subscribe()
	ch2 := hub.Subscribe()

	sid := sharding.ShardID(0)
	evt := observability.NewClusterEvent(
		observability.EventLeaderChanged,
		"node-a",
		&sid,
		map[string]any{"previous_leader": "node-b", "current_leader": "node-a"},
	)

	hub.Publish(evt)

	select {
	case received := <-ch1:
		if received.EventID != evt.EventID || received.Type != observability.EventLeaderChanged {
			t.Errorf("ch1 received unexpected event: %+v", received)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ch1 timed out waiting for event")
	}

	select {
	case received := <-ch2:
		if received.EventID != evt.EventID {
			t.Errorf("ch2 received unexpected event: %+v", received)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ch2 timed out waiting for event")
	}

	// Unsubscribe ch1
	hub.Unsubscribe(ch1)
	hub.Publish(evt)

	select {
	case _, ok := <-ch1:
		if ok {
			t.Errorf("expected ch1 to be closed after unsubscribe")
		}
	default:
	}
}

func TestWebSocket_HandshakeAndStreaming(t *testing.T) {
	t.Parallel()

	eventHub := observability.NewEventHub()
	defer eventHub.Close()

	wsHub := observability.NewWebSocketHub(eventHub)
	defer wsHub.Close()

	mux := http.NewServeMux()
	wsHub.RegisterRoutes(mux, "/ws/events")

	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect TCP client directly to server
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer conn.Close()

	// Perform RFC 6455 handshake
	secKey := "dGhlIHNhbXBsZSBub25jZQ=="
	handshakeReq := fmt.Sprintf(
		"GET /ws/events HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: %s\r\n"+
			"Sec-WebSocket-Version: 13\r\n\r\n",
		server.Listener.Addr().String(), secKey,
	)

	if _, err := conn.Write([]byte(handshakeReq)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	resp, err := http.ReadResponse(rw.Reader, nil)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}

	// Verify Sec-WebSocket-Accept
	h := sha1.New()
	h.Write([]byte(secKey + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	expectedAccept := base64.StdEncoding.EncodeToString(h.Sum(nil))
	if resp.Header.Get("Sec-WebSocket-Accept") != expectedAccept {
		t.Errorf("expected accept %s, got %s", expectedAccept, resp.Header.Get("Sec-WebSocket-Accept"))
	}

	// Wait a moment for server to register connection
	time.Sleep(50 * time.Millisecond)

	// Publish test event
	sid := sharding.ShardID(1)
	testEvt := observability.NewClusterEvent(
		observability.EventLeaderChanged,
		"node-a",
		&sid,
		map[string]any{"previous_leader": "node-b", "current_leader": "node-a"},
	)
	eventHub.Publish(testEvt)

	// Read WebSocket frame from server
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	b0, err := rw.ReadByte()
	if err != nil {
		t.Fatalf("read frame byte 0: %v", err)
	}
	if b0 != 0x81 { // FIN=1, Text Opcode=0x1
		t.Fatalf("expected opcode 0x81 (text), got 0x%X", b0)
	}

	b1, err := rw.ReadByte()
	if err != nil {
		t.Fatalf("read frame byte 1: %v", err)
	}
	payloadLen := int(b1 & 0x7F)
	if payloadLen == 126 {
		var l uint16
		var lenBytes [2]byte
		_, _ = io.ReadFull(rw, lenBytes[:])
		l = uint16(lenBytes[0])<<8 | uint16(lenBytes[1])
		payloadLen = int(l)
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(rw, payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}

	var received observability.ClusterEvent
	if err := json.Unmarshal(payload, &received); err != nil {
		t.Fatalf("unmarshal ClusterEvent: %v (raw: %s)", err, string(payload))
	}

	if received.EventID != testEvt.EventID || received.Type != observability.EventLeaderChanged {
		t.Errorf("unexpected event received over WS: %+v", received)
	}
	if received.Data["current_leader"] != "node-a" {
		t.Errorf("unexpected event data: %+v", received.Data)
	}
}

func TestDistributedEventBus_ValidationAndDeduplication(t *testing.T) {
	t.Parallel()

	hub := observability.NewEventHub()
	defer hub.Close()

	bus := observability.NewDistributedEventBus(hub, "node-a", nil)
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	// 1. Missing EventID returns 400 Bad Request
	reqMissingID := httptest.NewRequest(http.MethodPost, "/cluster/events/relay", strings.NewReader(`{"type":"LEADER_CHANGED","node_id":"node-b"}`))
	recMissingID := httptest.NewRecorder()
	bus.HandleRelayHTTP(recMissingID, reqMissingID)
	if recMissingID.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing event_id, got %d", recMissingID.Code)
	}

	// 2. Missing Type returns 400 Bad Request
	reqMissingType := httptest.NewRequest(http.MethodPost, "/cluster/events/relay", strings.NewReader(`{"event_id":"evt-1","node_id":"node-b"}`))
	recMissingType := httptest.NewRecorder()
	bus.HandleRelayHTTP(recMissingType, reqMissingType)
	if recMissingType.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing type, got %d", recMissingType.Code)
	}

	// 3. Valid event is accepted and published
	validPayload := `{"event_id":"evt-dedup-test-1","type":"LEADER_CHANGED","node_id":"node-b"}`
	reqValid := httptest.NewRequest(http.MethodPost, "/cluster/events/relay", strings.NewReader(validPayload))
	recValid := httptest.NewRecorder()
	bus.HandleRelayHTTP(recValid, reqValid)
	if recValid.Code != http.StatusOK {
		t.Errorf("expected 200 for valid event, got %d", recValid.Code)
	}

	select {
	case evt := <-ch:
		if evt.EventID != "evt-dedup-test-1" {
			t.Errorf("expected evt-dedup-test-1, got %s", evt.EventID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected event publication on hub")
	}

	// 4. Duplicate event is acknowledged with 200 OK but NOT published to hub
	reqDup := httptest.NewRequest(http.MethodPost, "/cluster/events/relay", strings.NewReader(validPayload))
	recDup := httptest.NewRecorder()
	bus.HandleRelayHTTP(recDup, reqDup)
	if recDup.Code != http.StatusOK {
		t.Errorf("expected 200 for duplicate event, got %d", recDup.Code)
	}

	select {
	case evt := <-ch:
		t.Fatalf("unexpected duplicate event published to hub: %+v", evt)
	case <-time.After(100 * time.Millisecond):
		// Success: no duplicate received
	}

	if bus.SeenCount() != 1 {
		t.Errorf("expected 1 seen event, got %d", bus.SeenCount())
	}

	// 5. Test cache pruning
	pruned := bus.PruneSeen(0) // evict everything older than 0s
	if pruned != 1 {
		t.Errorf("expected 1 pruned event, got %d", pruned)
	}
	if bus.SeenCount() != 0 {
		t.Errorf("expected 0 seen events after prune, got %d", bus.SeenCount())
	}
}

func TestNodeLivenessTracker_Transitions(t *testing.T) {
	t.Parallel()

	tracker := observability.NewNodeLivenessTracker(50 * time.Millisecond)

	var mu sync.Mutex
	transitions := make([]struct {
		nodeID string
		online bool
	}, 0)

	tracker.SetCallback(func(nodeID string, online bool) {
		mu.Lock()
		transitions = append(transitions, struct {
			nodeID string
			online bool
		}{nodeID, online})
		mu.Unlock()
	})

	now := time.Now()
	// Record initial heartbeat -> online
	tracker.RecordHeartbeat("node-1", now)

	// Allow goroutine callback to run
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(transitions) != 1 || !transitions[0].online || transitions[0].nodeID != "node-1" {
		t.Fatalf("expected 1 online transition, got %+v", transitions)
	}
	mu.Unlock()

	// Another heartbeat while already online should NOT fire callback again
	tracker.RecordHeartbeat("node-1", now.Add(10*time.Millisecond))
	time.Sleep(30 * time.Millisecond)

	mu.Lock()
	if len(transitions) != 1 {
		t.Fatalf("expected still only 1 transition, got %d", len(transitions))
	}
	mu.Unlock()

	// Check transitions after timeout -> offline transition
	tracker.CheckTransitions(now.Add(100 * time.Millisecond))
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(transitions) != 2 || transitions[1].online || transitions[1].nodeID != "node-1" {
		t.Fatalf("expected offline transition at index 1, got %+v", transitions)
	}
	mu.Unlock()
}

func TestWebSocket_ValidationRFC6455(t *testing.T) {
	t.Parallel()

	eventHub := observability.NewEventHub()
	defer eventHub.Close()

	wsHub := observability.NewWebSocketHub(eventHub)
	defer wsHub.Close()

	mux := http.NewServeMux()
	wsHub.RegisterRoutes(mux, "/ws/events")

	// 1. Method Not Allowed (POST)
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/ws/events", nil)
	mux.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", rec1.Code)
	}

	// 2. Missing Connection: Upgrade
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/ws/events", nil)
	req2.Header.Set("Upgrade", "websocket")
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing Connection header, got %d", rec2.Code)
	}

	// 3. Missing Upgrade: websocket
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/ws/events", nil)
	req3.Header.Set("Connection", "Upgrade")
	mux.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing Upgrade header, got %d", rec3.Code)
	}

	// 4. Invalid Sec-WebSocket-Version (!= 13)
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodGet, "/ws/events", nil)
	req4.Header.Set("Connection", "Upgrade")
	req4.Header.Set("Upgrade", "websocket")
	req4.Header.Set("Sec-WebSocket-Version", "12")
	mux.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for version 12, got %d", rec4.Code)
	}
	if rec4.Header().Get("Sec-WebSocket-Version") != "13" {
		t.Errorf("expected Sec-WebSocket-Version: 13 header in response, got %s", rec4.Header().Get("Sec-WebSocket-Version"))
	}

	// 5. Invalid Sec-WebSocket-Key (not 16 bytes)
	rec5 := httptest.NewRecorder()
	req5 := httptest.NewRequest(http.MethodGet, "/ws/events", nil)
	req5.Header.Set("Connection", "Upgrade")
	req5.Header.Set("Upgrade", "websocket")
	req5.Header.Set("Sec-WebSocket-Version", "13")
	req5.Header.Set("Sec-WebSocket-Key", "not-16-bytes")
	mux.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid key, got %d", rec5.Code)
	}
}
