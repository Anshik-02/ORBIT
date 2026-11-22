package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/node"
	"orbit/internal/raft"
)

// TestRaftHandlerDirect tests RaftHandler behaviors using httptest recorders.
func TestRaftHandlerDirect(t *testing.T) {
	net := raft.NewMockNetwork()
	baseDir := t.TempDir()
	configs := node.ClusterConfig3Nodes(baseDir)

	nodes := make([]*node.Node, 3)
	for i, cfg := range configs {
		tr := net.CreateTransport(cfg.ID, cfg.RaftAddress)
		n, err := node.NewNode(cfg, tr)
		if err != nil {
			t.Fatalf("NewNode %s: %v", cfg.ID, err)
		}
		nodes[i] = n
	}

	for _, n := range nodes {
		if err := n.Raft().Start(); err != nil {
			t.Fatalf("Start raft: %v", err)
		}
	}
	defer func() {
		for _, n := range nodes {
			_ = n.Raft().Stop()
			_ = n.DB().Close()
		}
	}()

	// Wait for leader
	var leaderNode *node.Node
	var followerNode *node.Node
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range nodes {
			if n.Raft().Role() == raft.Leader {
				leaderNode = n
			} else {
				followerNode = n
			}
		}
		if leaderNode != nil && followerNode != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if leaderNode == nil || followerNode == nil {
		t.Fatal("failed to elect leader and follower")
	}

	peerAddrs := make(map[string]string)
	for _, cfg := range configs {
		peerAddrs[cfg.ID] = cfg.Address
	}

	leaderMux := http.NewServeMux()
	leaderHandler := api.NewRaftHandler(leaderNode.Raft(), leaderNode.DB(), leaderNode.ID(), configs[0].Address, peerAddrs)
	api.RegisterRaftRoutes(leaderMux, leaderHandler)

	followerMux := http.NewServeMux()
	followerHandler := api.NewRaftHandler(followerNode.Raft(), followerNode.DB(), followerNode.ID(), configs[1].Address, peerAddrs)
	api.RegisterRaftRoutes(followerMux, followerHandler)

	// 1. Follower receives write -> must return 307 Temporary Redirect
	body := strings.NewReader(`{"key":"user:1","value":"Anshik"}`)
	req := httptest.NewRequest("POST", "/api/v1/kv", body)
	rec := httptest.NewRecorder()
	followerMux.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 on follower, got %d: %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/api/v1/kv") {
		t.Fatalf("expected Location header to contain /api/v1/kv, got %q", loc)
	}

	var notLeaderResp api.NotLeaderResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &notLeaderResp); err != nil {
		t.Fatalf("unmarshal not leader response: %v", err)
	}
	if notLeaderResp.LeaderID != leaderNode.ID() {
		t.Fatalf("expected leaderID %q, got %q", leaderNode.ID(), notLeaderResp.LeaderID)
	}

	// 2. Leader receives write -> must commit via Raft and return 201 Created
	bodyLeader := strings.NewReader(`{"key":"user:1","value":"Anshik"}`)
	reqLeader := httptest.NewRequest("POST", "/api/v1/kv", bodyLeader)
	recLeader := httptest.NewRecorder()
	leaderMux.ServeHTTP(recLeader, reqLeader)

	if recLeader.Code != http.StatusCreated {
		t.Fatalf("expected 201 on leader, got %d: %s", recLeader.Code, recLeader.Body.String())
	}

	// 3. Leader receives read for existing key -> must return 200 OK
	reqGet := httptest.NewRequest("GET", "/api/v1/kv/user:1", nil)
	recGet := httptest.NewRecorder()
	leaderMux.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 on leader GET, got %d: %s", recGet.Code, recGet.Body.String())
	}
	if !strings.Contains(recGet.Body.String(), "Anshik") {
		t.Fatalf("expected body to contain 'Anshik', got %s", recGet.Body.String())
	}

	// 4. Leader receives read for non-existent key -> must return 404 Not Found
	reqGetMissing := httptest.NewRequest("GET", "/api/v1/kv/nonexistent", nil)
	recGetMissing := httptest.NewRecorder()
	leaderMux.ServeHTTP(recGetMissing, reqGetMissing)

	if recGetMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on missing key, got %d", recGetMissing.Code)
	}

	// 5. Follower receives read -> must redirect to leader for linearizability
	reqFollowerGet := httptest.NewRequest("GET", "/api/v1/kv/user:1", nil)
	recFollowerGet := httptest.NewRecorder()
	followerMux.ServeHTTP(recFollowerGet, reqFollowerGet)

	if recFollowerGet.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 on follower GET, got %d", recFollowerGet.Code)
	}

	// 6. Leader receives delete -> must commit via Raft and return 200 OK
	reqDelete := httptest.NewRequest("DELETE", "/api/v1/kv/user:1", nil)
	recDelete := httptest.NewRecorder()
	leaderMux.ServeHTTP(recDelete, reqDelete)

	if recDelete.Code != http.StatusOK {
		t.Fatalf("expected 200 on leader DELETE, got %d: %s", recDelete.Code, recDelete.Body.String())
	}

	// Verify key is gone
	recGetAfterDelete := httptest.NewRecorder()
	leaderMux.ServeHTTP(recGetAfterDelete, reqGet)
	if recGetAfterDelete.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", recGetAfterDelete.Code)
	}

	// 7. Health check on leader -> writable: true
	reqHealthLeader := httptest.NewRequest("GET", "/health", nil)
	recHealthLeader := httptest.NewRecorder()
	leaderMux.ServeHTTP(recHealthLeader, reqHealthLeader)

	if recHealthLeader.Code != http.StatusOK {
		t.Fatalf("expected 200 on health check, got %d", recHealthLeader.Code)
	}
	var healthResp api.RaftHealthResponse
	if err := json.Unmarshal(recHealthLeader.Body.Bytes(), &healthResp); err != nil {
		t.Fatalf("unmarshal health response: %v", err)
	}
	if !healthResp.Writable || healthResp.Role != "leader" {
		t.Fatalf("expected writable=true, role=leader; got writable=%v, role=%s", healthResp.Writable, healthResp.Role)
	}

	// 8. Health check on follower -> writable: false
	reqHealthFollower := httptest.NewRequest("GET", "/health", nil)
	recHealthFollower := httptest.NewRecorder()
	followerMux.ServeHTTP(recHealthFollower, reqHealthFollower)

	var followerHealthResp api.RaftHealthResponse
	if err := json.Unmarshal(recHealthFollower.Body.Bytes(), &followerHealthResp); err != nil {
		t.Fatalf("unmarshal follower health: %v", err)
	}
	if followerHealthResp.Writable || followerHealthResp.Role != "follower" {
		t.Fatalf("expected writable=false, role=follower; got writable=%v, role=%s", followerHealthResp.Writable, followerHealthResp.Role)
	}
}

// TestClusterHTTPRealNetwork tests full HTTP cluster with real TCP listeners.
func TestClusterHTTPRealNetwork(t *testing.T) {
	baseDir := t.TempDir()

	// Assign dynamic distinct ports for testing
	ports := []struct {
		apiPort  int
		raftPort int
	}{
		{2825, 3825},
		{2826, 3826},
		{2827, 3827},
	}

	configs := make([]node.NodeConfig, 3)
	nodeIDs := []string{"node-a", "node-b", "node-c"}

	for i := 0; i < 3; i++ {
		var peers []node.PeerConfig
		for j := 0; j < 3; j++ {
			if i != j {
				peers = append(peers, node.PeerConfig{
					ID:          nodeIDs[j],
					Address:     fmt.Sprintf("127.0.0.1:%d", ports[j].apiPort),
					RaftAddress: fmt.Sprintf("127.0.0.1:%d", ports[j].raftPort),
				})
			}
		}

		configs[i] = node.NodeConfig{
			ID:                 nodeIDs[i],
			Address:            fmt.Sprintf("127.0.0.1:%d", ports[i].apiPort),
			RaftAddress:        fmt.Sprintf("127.0.0.1:%d", ports[i].raftPort),
			DataDir:            fmt.Sprintf("%s/%s", baseDir, nodeIDs[i]),
			Peers:              peers,
			HeartbeatInterval:  30 * time.Millisecond,
			ElectionTimeoutMin: 80 * time.Millisecond,
			ElectionTimeoutMax: 160 * time.Millisecond,
		}
	}

	nodes := make([]*node.Node, 3)
	for i, cfg := range configs {
		n, err := node.NewNode(cfg, nil) // uses real HTTPTransport
		if err != nil {
			t.Fatalf("NewNode %s: %v", cfg.ID, err)
		}
		nodes[i] = n
	}

	for _, n := range nodes {
		if err := n.Start(); err != nil {
			t.Fatalf("Start node: %v", err)
		}
	}
	defer func() {
		for _, n := range nodes {
			_ = n.Stop()
		}
	}()

	// Wait for leader
	var leaderNode *node.Node
	var leaderCfg node.NodeConfig
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		for i, n := range nodes {
			if n.Raft().Role() == raft.Leader {
				leaderNode = n
				leaderCfg = configs[i]
				break
			}
		}
		if leaderNode != nil {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	if leaderNode == nil {
		t.Fatal("cluster failed to elect leader within timeout")
	}

	// 1. Write through leader HTTP endpoint
	client := &http.Client{Timeout: 3 * time.Second}
	writeBody := []byte(`{"key":"city","value":"Bengaluru"}`)
	resp, err := client.Post(fmt.Sprintf("http://%s/api/v1/kv", leaderCfg.Address), "application/json", bytes.NewReader(writeBody))
	if err != nil {
		t.Fatalf("client.Post to leader: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created from leader, got %d", resp.StatusCode)
	}

	// 2. Read through leader HTTP endpoint
	getResp, err := client.Get(fmt.Sprintf("http://%s/api/v1/kv/city", leaderCfg.Address))
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from GET, got %d", getResp.StatusCode)
	}

	// 3. Write through follower with redirect disabled -> verify 307
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // do not follow redirect
		},
		Timeout: 2 * time.Second,
	}

	var followerCfg node.NodeConfig
	for _, cfg := range configs {
		if cfg.ID != leaderCfg.ID {
			followerCfg = cfg
			break
		}
	}

	followerResp, err := noRedirectClient.Post(
		fmt.Sprintf("http://%s/api/v1/kv", followerCfg.Address),
		"application/json",
		bytes.NewReader(writeBody),
	)
	if err != nil {
		t.Fatalf("client.Post to follower: %v", err)
	}
	defer followerResp.Body.Close()

	if followerResp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 Temporary Redirect from follower, got %d", followerResp.StatusCode)
	}
}
