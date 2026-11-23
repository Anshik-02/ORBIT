package observability

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"orbit/internal/sharding"
)

// EventType defines valid cluster lifecycle and consensus event types.
type EventType string

const (
	EventNodeJoined           EventType = "NODE_JOINED"
	EventNodeDisconnected     EventType = "NODE_DISCONNECTED"
	EventNodeSuspectedOffline EventType = "NODE_SUSPECTED_OFFLINE"
	EventNodeRecovered        EventType = "NODE_RECOVERED"
	EventLeaderChanged        EventType = "LEADER_CHANGED"
	EventLeaderElected        EventType = "LEADER_ELECTED"
	EventLeaderSteppedDown    EventType = "LEADER_STEPPED_DOWN"
	EventTermChanged          EventType = "TERM_CHANGED"
	EventEntryCommitted       EventType = "ENTRY_COMMITTED"
	EventShardUpdated         EventType = "SHARD_UPDATED"
	EventReplicaCatchupDone   EventType = "REPLICA_CATCHUP_COMPLETED"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// ClusterEvent defines the standard JSON schema for cluster observability events.
type ClusterEvent struct {
	EventID   string            `json:"event_id"`
	Type      EventType         `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	NodeID    string            `json:"node_id"`
	ShardID   *sharding.ShardID `json:"shard_id,omitempty"`
	Data      map[string]any    `json:"data"`
}

// GenerateEventID generates a random 16-byte hex unique event identifier.
func GenerateEventID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// NewClusterEvent creates a new timestamped ClusterEvent with a unique ID.
func NewClusterEvent(eventType EventType, nodeID string, shardID *sharding.ShardID, data map[string]any) ClusterEvent {
	if data == nil {
		data = make(map[string]any)
	}
	return ClusterEvent{
		EventID:   GenerateEventID(),
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		NodeID:    nodeID,
		ShardID:   shardID,
		Data:      data,
	}
}

// EventHub is an in-memory pub/sub broker distributing ClusterEvents to local and WebSocket listeners.
type EventHub struct {
	mu          sync.RWMutex
	subscribers map[chan ClusterEvent]struct{}
	closed      bool
}

// NewEventHub creates an initialized EventHub.
func NewEventHub() *EventHub {
	return &EventHub{
		subscribers: make(map[chan ClusterEvent]struct{}),
	}
}

// Publish broadcasts an event to all active subscriber channels without blocking.
func (h *EventHub) Publish(event ClusterEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.closed {
		return
	}

	for ch := range h.subscribers {
		select {
		case ch <- event:
		default:
			// Non-blocking drop if consumer buffer is full
		}
	}
}

// Subscribe registers a new subscriber channel with a buffer.
func (h *EventHub) Subscribe() chan ClusterEvent {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan ClusterEvent, 128)
	if h.closed {
		close(ch)
		return ch
	}

	h.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe closes and unregisters a subscriber channel.
func (h *EventHub) Unsubscribe(ch chan ClusterEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.subscribers[ch]; ok {
		delete(h.subscribers, ch)
		close(ch)
	}
}

// Close shuts down the event hub and terminates all subscriptions.
func (h *EventHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	h.closed = true

	for ch := range h.subscribers {
		close(ch)
	}
	h.subscribers = nil
}

// DistributedEventBus coordinates cross-node peer-to-peer event propagation and deduplication.
type DistributedEventBus struct {
	localHub   *EventHub
	localNode  string
	peerAddrs  map[string]string // nodeID -> HTTP address
	httpClient *http.Client
	mu         sync.Mutex
	seenEvents map[string]time.Time
}

// NewDistributedEventBus creates a DistributedEventBus with event deduplication.
func NewDistributedEventBus(localHub *EventHub, localNode string, peerAddrs map[string]string) *DistributedEventBus {
	return &DistributedEventBus{
		localHub:   localHub,
		localNode:  localNode,
		peerAddrs:  peerAddrs,
		httpClient: &http.Client{Timeout: 3 * time.Second},
		seenEvents: make(map[string]time.Time),
	}
}

// Broadcast broadcasts an event locally and relays it asynchronously to all peers.
func (b *DistributedEventBus) Broadcast(evt ClusterEvent) {
	b.mu.Lock()
	b.seenEvents[evt.EventID] = time.Now()
	b.cleanOldSeenLocked()
	b.mu.Unlock()

	b.localHub.Publish(evt)

	// Relay to peers in background
	go b.relayToPeers(evt)
}

func (b *DistributedEventBus) relayToPeers(evt ClusterEvent) {
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}

	for peerID, addr := range b.peerAddrs {
		if peerID == b.localNode || addr == "" {
			continue
		}

		targetURL := addr
		if len(targetURL) < 4 || targetURL[:4] != "http" {
			targetURL = "http://" + addr
		}
		targetURL += "/cluster/events/relay"

		// Send relay to each peer in parallel so a dead or partitioned peer never stalls other peers
		go b.sendRelayToPeer(targetURL, payload)
	}
}

func (b *DistributedEventBus) sendRelayToPeer(targetURL string, payload []byte) {
	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Orbit-Origin-Node", b.localNode)

	resp, err := b.httpClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

// HandleRelayHTTP handles an inbound relayed event from a peer node with strict validation and deduplication.
func (b *DistributedEventBus) HandleRelayHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var evt ClusterEvent
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required event attributes
	if evt.EventID == "" || evt.Type == "" {
		http.Error(w, "EventID and Type are required", http.StatusBadRequest)
		return
	}

	b.mu.Lock()
	if _, seen := b.seenEvents[evt.EventID]; seen {
		b.mu.Unlock()
		w.WriteHeader(http.StatusOK) // Deduplicated successfully
		return
	}
	b.seenEvents[evt.EventID] = time.Now()
	b.cleanOldSeenLocked()
	b.mu.Unlock()

	b.localHub.Publish(evt)
	w.WriteHeader(http.StatusOK)
}

// PruneSeen evicts event deduplication records older than the specified TTL.
func (b *DistributedEventBus) PruneSeen(ttl time.Duration) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().Add(-ttl)
	pruned := 0
	for id, t := range b.seenEvents {
		if t.Before(cutoff) {
			delete(b.seenEvents, id)
			pruned++
		}
	}
	return pruned
}

// SeenCount returns the number of deduplicated event IDs currently held in the cache.
func (b *DistributedEventBus) SeenCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.seenEvents)
}

func (b *DistributedEventBus) cleanOldSeenLocked() {
	if len(b.seenEvents) > 1000 {
		cutoff := time.Now().Add(-5 * time.Minute)
		for id, t := range b.seenEvents {
			if t.Before(cutoff) {
				delete(b.seenEvents, id)
			}
		}
	}
}

// WebSocketClient encapsulates an active RFC 6455 WebSocket client with slow-client isolation.
type WebSocketClient struct {
	conn    net.Conn
	rw      *bufio.ReadWriter
	sendCh  chan []byte
	closeCh chan struct{}
	mu      sync.Mutex
	closed  bool
}

func newWebSocketClient(conn net.Conn, rw *bufio.ReadWriter) *WebSocketClient {
	c := &WebSocketClient{
		conn:    conn,
		rw:      rw,
		sendCh:  make(chan []byte, 64),
		closeCh: make(chan struct{}),
	}
	go c.writePump()
	return c
}

func (c *WebSocketClient) writePump() {
	for {
		select {
		case <-c.closeCh:
			return
		case msg, ok := <-c.sendCh:
			if !ok {
				return
			}
			c.mu.Lock()
			_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			_, err := c.rw.Write(msg)
			if err == nil {
				err = c.rw.Flush()
			}
			c.mu.Unlock()

			if err != nil {
				c.Close()
				return
			}
		}
	}
}

// Close cleanly terminates the client writer.
func (c *WebSocketClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.closeCh)
	_ = c.conn.Close()
}

// WebSocketHub manages live WebSocket connections streaming cluster events to dashboard clients.
type WebSocketHub struct {
	hub       *EventHub
	mu        sync.Mutex
	clients   map[*WebSocketClient]struct{}
	closeCh   chan struct{}
	closeOnce sync.Once
}

// NewWebSocketHub constructs a WebSocketHub connected to the given EventHub.
func NewWebSocketHub(eventHub *EventHub) *WebSocketHub {
	wsh := &WebSocketHub{
		hub:     eventHub,
		clients: make(map[*WebSocketClient]struct{}),
		closeCh: make(chan struct{}),
	}

	go wsh.eventLoop()
	return wsh
}

// RegisterRoutes attaches the WebSocket endpoint handler to the provided HTTP mux.
func (wsh *WebSocketHub) RegisterRoutes(mux *http.ServeMux, path string) {
	mux.HandleFunc(path, wsh.HandleWebSocket)
}

func (wsh *WebSocketHub) eventLoop() {
	events := wsh.hub.Subscribe()
	defer wsh.hub.Unsubscribe(events)

	for {
		select {
		case <-wsh.closeCh:
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			wsh.broadcastEvent(evt)
		}
	}
}

func (wsh *WebSocketHub) broadcastEvent(evt ClusterEvent) {
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}

	frame := encodeTextFrame(data)

	wsh.mu.Lock()
	defer wsh.mu.Unlock()

	for client := range wsh.clients {
		select {
		case client.sendCh <- frame:
		default:
			// Client buffer is full (slow client): isolate and disconnect cleanly
			client.Close()
			delete(wsh.clients, client)
		}
	}
}

// HandleWebSocket upgrades an HTTP request to an RFC 6455 WebSocket connection.
func (wsh *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "Expected websocket upgrade", http.StatusBadRequest)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}

	conn, rw, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Calculate RFC 6455 accept token
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Send handshake response
	handshake := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: %s\r\n\r\n", accept)

	if _, err := rw.WriteString(handshake); err != nil {
		_ = conn.Close()
		return
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return
	}

	client := newWebSocketClient(conn, rw)

	wsh.mu.Lock()
	wsh.clients[client] = struct{}{}
	wsh.mu.Unlock()

	// Drain incoming client frames until disconnection
	go func() {
		defer func() {
			wsh.mu.Lock()
			delete(wsh.clients, client)
			wsh.mu.Unlock()
			client.Close()
		}()

		for {
			opcode, payload, err := readFrame(rw)
			if err != nil {
				return
			}
			if opcode == 0x8 { // Close frame
				client.mu.Lock()
				_, _ = client.rw.Write(encodeCloseFrame(payload))
				_ = client.rw.Flush()
				client.mu.Unlock()
				return
			}
			if opcode == 0x9 { // Ping -> Pong
				client.mu.Lock()
				_, _ = client.rw.Write(encodePongFrame(payload))
				_ = client.rw.Flush()
				client.mu.Unlock()
			}
		}
	}()
}

// Close disconnects all WebSocket clients and shuts down the hub.
func (wsh *WebSocketHub) Close() {
	wsh.closeOnce.Do(func() {
		close(wsh.closeCh)
		wsh.mu.Lock()
		defer wsh.mu.Unlock()
		for client := range wsh.clients {
			client.Close()
		}
		wsh.clients = nil
	})
}

// encodeTextFrame constructs an unmasked RFC 6455 text frame (opcode 0x1, FIN = 1).
func encodeTextFrame(payload []byte) []byte {
	length := len(payload)
	var header []byte

	if length <= 125 {
		header = []byte{0x81, byte(length)}
	} else if length <= 65535 {
		header = []byte{0x81, 126, byte(length >> 8), byte(length & 0xFF)}
	} else {
		header = make([]byte, 10)
		header[0] = 0x81
		header[1] = 127
		for i := 0; i < 8; i++ {
			header[2+i] = byte((uint64(length) >> (56 - 8*i)) & 0xFF)
		}
	}

	return append(header, payload...)
}

func encodePongFrame(payload []byte) []byte {
	length := len(payload)
	header := []byte{0x8A, byte(length)}
	return append(header, payload...)
}

func encodeCloseFrame(payload []byte) []byte {
	length := len(payload)
	header := []byte{0x88, byte(length)}
	return append(header, payload...)
}

func readFrame(rw *bufio.ReadWriter) (byte, []byte, error) {
	b0, err := rw.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	opcode := b0 & 0x0F

	b1, err := rw.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	masked := (b1 & 0x80) != 0
	payloadLen := int(b1 & 0x7F)

	if payloadLen == 126 {
		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(rw, lenBytes); err != nil {
			return 0, nil, err
		}
		payloadLen = int(lenBytes[0])<<8 | int(lenBytes[1])
	} else if payloadLen == 127 {
		lenBytes := make([]byte, 8)
		if _, err := io.ReadFull(rw, lenBytes); err != nil {
			return 0, nil, err
		}
		var l uint64
		for i := 0; i < 8; i++ {
			l = (l << 8) | uint64(lenBytes[i])
		}
		payloadLen = int(l)
	}

	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(rw, mask[:]); err != nil {
			return 0, nil, err
		}
	}

	if payloadLen > 10*1024*1024 {
		return 0, nil, errors.New("frame payload exceeds maximum limit")
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(rw, payload); err != nil {
		return 0, nil, err
	}

	if masked {
		for i := 0; i < payloadLen; i++ {
			payload[i] ^= mask[i%4]
		}
	}

	return opcode, payload, nil
}
