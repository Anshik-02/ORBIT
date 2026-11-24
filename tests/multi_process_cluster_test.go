package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/node"
)

// TestMultiProcessHTTPCluster verifies an actual three-process cluster
// communicating over real HTTP listeners using cmd/orbit binary.
func TestMultiProcessHTTPCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-process cluster test in short mode")
	}

	// 1. Build orbit binary in a temporary directory
	tempDir := t.TempDir()
	binName := "orbit_cluster_test"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tempDir, binName)

	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/orbit")
	buildCmd.Dir = ".."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		// Try from local directory if relative path differs
		buildCmd2 := exec.Command("go", "build", "-o", binPath, "./cmd/orbit")
		if out2, err2 := buildCmd2.CombinedOutput(); err2 != nil {
			t.Fatalf("build orbit binary failed: %v\nOutput: %s\nOutput2: %s", err, string(out), string(out2))
		}
	}

	// 2. Prepare 3 node configurations with dynamically allocated free ports
	ports := make([]struct {
		apiPort  int
		raftPort int
	}, 3)
	for i := 0; i < 3; i++ {
		apiLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen api port %d: %v", i, err)
		}
		apiP := apiLn.Addr().(*net.TCPAddr).Port
		_ = apiLn.Close()

		raftLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen raft port %d: %v", i, err)
		}
		raftP := raftLn.Addr().(*net.TCPAddr).Port
		_ = raftLn.Close()

		ports[i] = struct {
			apiPort  int
			raftPort int
		}{apiPort: apiP, raftPort: raftP}
	}
	nodeIDs := []string{"node-p1", "node-p2", "node-p3"}
	configPaths := make([]string, 3)

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

		cfg := node.NodeConfig{
			ID:                 nodeIDs[i],
			Address:            fmt.Sprintf("127.0.0.1:%d", ports[i].apiPort),
			RaftAddress:        fmt.Sprintf("127.0.0.1:%d", ports[i].raftPort),
			DataDir:            filepath.Join(tempDir, nodeIDs[i]),
			HeartbeatInterval:  30 * time.Millisecond,
			ElectionTimeoutMin: 80 * time.Millisecond,
			ElectionTimeoutMax: 160 * time.Millisecond,
			Peers:              peers,
		}

		cfgData, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			t.Fatalf("marshal config %d: %v", i, err)
		}

		cPath := filepath.Join(tempDir, fmt.Sprintf("%s.json", nodeIDs[i]))
		if err := os.WriteFile(cPath, cfgData, 0600); err != nil {
			t.Fatalf("write config file: %v", err)
		}
		configPaths[i] = cPath
	}

	// 3. Launch 3 separate OS processes
	procs := make([]*exec.Cmd, 3)
	for i := 0; i < 3; i++ {
		cmd := exec.Command(binPath, "--config", configPaths[i])
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start process %d: %v", i, err)
		}
		procs[i] = cmd
	}

	defer func() {
		for _, p := range procs {
			if p != nil && p.Process != nil {
				_ = p.Process.Kill()
				_ = p.Wait()
			}
		}
	}()

	client := &http.Client{Timeout: 2 * time.Second}

	// 4. Poll health endpoints until a leader is discovered
	var leaderID string
	var leaderPort int
	deadline := time.Now().Add(12 * time.Second)

	for time.Now().Before(deadline) {
		for i := 0; i < 3; i++ {
			url := fmt.Sprintf("http://127.0.0.1:%d/health", ports[i].apiPort)
			resp, err := client.Get(url)
			if err != nil {
				continue
			}
			var health api.RaftHealthResponse
			if err := json.NewDecoder(resp.Body).Decode(&health); err == nil {
				resp.Body.Close()
				if health.Role == "leader" && health.Writable {
					leaderID = health.Node
					leaderPort = ports[i].apiPort
					break
				}
			} else {
				resp.Body.Close()
			}
		}
		if leaderID != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if leaderID == "" {
		t.Fatal("three-process cluster failed to elect a leader within timeout")
	}

	// 5. Submit write to leader HTTP API
	writePayload := []byte(`{"key":"country","value":"India"}`)
	postURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/kv", leaderPort)
	postResp, err := client.Post(postURL, "application/json", bytes.NewReader(writePayload))
	if err != nil {
		t.Fatalf("POST to leader: %v", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created from leader process, got %d", postResp.StatusCode)
	}

	// 6. Read back from leader
	getURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/kv/country", leaderPort)
	getResp, err := client.Get(getURL)
	if err != nil {
		t.Fatalf("GET from leader: %v", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from leader process, got %d", getResp.StatusCode)
	}

	// 7. Verify write to follower redirects to leader
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 2 * time.Second,
	}

	var followerPort int
	for i := 0; i < 3; i++ {
		if ports[i].apiPort != leaderPort {
			followerPort = ports[i].apiPort
			break
		}
	}

	followerPostURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/kv", followerPort)
	followerResp, err := noRedirectClient.Post(followerPostURL, "application/json", bytes.NewReader(writePayload))
	if err != nil {
		t.Fatalf("POST to follower: %v", err)
	}
	defer followerResp.Body.Close()

	if followerResp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 Temporary Redirect from follower process, got %d", followerResp.StatusCode)
	}
	loc := followerResp.Header.Get("Location")
	if loc != postURL {
		t.Fatalf("expected Location %q, got %q", postURL, loc)
	}

	// 8. Kill the leader process to verify real process failover!
	var leaderProcIdx int
	for i := 0; i < 3; i++ {
		if ports[i].apiPort == leaderPort {
			leaderProcIdx = i
			break
		}
	}

	_ = procs[leaderProcIdx].Process.Kill()
	_ = procs[leaderProcIdx].Wait()
	procs[leaderProcIdx] = nil

	// 9. Remaining 2 processes must elect a new leader
	var newLeaderPort int
	failoverDeadline := time.Now().Add(12 * time.Second)

	for time.Now().Before(failoverDeadline) {
		for i := 0; i < 3; i++ {
			if i == leaderProcIdx {
				continue
			}
			url := fmt.Sprintf("http://127.0.0.1:%d/health", ports[i].apiPort)
			resp, err := client.Get(url)
			if err != nil {
				continue
			}
			var health api.RaftHealthResponse
			if err := json.NewDecoder(resp.Body).Decode(&health); err == nil {
				resp.Body.Close()
				if health.Role == "leader" && health.Writable {
					newLeaderPort = ports[i].apiPort
					break
				}
			} else {
				resp.Body.Close()
			}
		}
		if newLeaderPort != 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if newLeaderPort == 0 {
		t.Fatal("remaining 2 processes failed to elect a new leader after leader was killed")
	}

	// 10. Write to new leader process succeeds
	writePayload2 := []byte(`{"key":"state","value":"Karnataka"}`)
	newPostURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/kv", newLeaderPort)
	newPostResp, err := client.Post(newPostURL, "application/json", bytes.NewReader(writePayload2))
	if err != nil {
		t.Fatalf("POST to new leader process: %v", err)
	}
	defer newPostResp.Body.Close()

	if newPostResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created from new leader process, got %d", newPostResp.StatusCode)
	}

	// 11. Read back both keys from new leader to confirm consistency
	for _, key := range []string{"country", "state"} {
		url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/kv/%s", newLeaderPort, key)
		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("GET %s from new leader: %v", key, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK for key %s from new leader, got %d", key, resp.StatusCode)
		}
	}

	// 12. Restart the killed node process with the same configuration file
	restartedCmd := exec.Command(binPath, "--config", configPaths[leaderProcIdx])
	restartedCmd.Stdout = os.Stdout
	restartedCmd.Stderr = os.Stderr
	if err := restartedCmd.Start(); err != nil {
		t.Fatalf("restart killed process %d: %v", leaderProcIdx, err)
	}
	procs[leaderProcIdx] = restartedCmd

	// 13. Poll restarted node until it catches up and is healthy
	restartedAPIPort := ports[leaderProcIdx].apiPort
	restartedURL := fmt.Sprintf("http://127.0.0.1:%d/health", restartedAPIPort)
	restartedDeadline := time.Now().Add(6 * time.Second)
	restartedHealthy := false

	for time.Now().Before(restartedDeadline) {
		resp, err := client.Get(restartedURL)
		if err == nil {
			var health api.RaftHealthResponse
			if err := json.NewDecoder(resp.Body).Decode(&health); err == nil {
				resp.Body.Close()
				// Node should catch up with cluster commit index (at least 2 entries: country + state)
				if health.Status == "ok" && health.CommitIndex >= 2 && health.LastApplied >= 2 && health.LeaderID != "" {
					restartedHealthy = true
					break
				}
			} else {
				resp.Body.Close()
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !restartedHealthy {
		t.Fatal("restarted process failed to catch up to cluster commit index within timeout")
	}

	// 14. Query restarted process for both keys:
	// Verify that country=India survived the restart (durability)
	// and state=Karnataka was replicated to the restarted node (convergence).
	for key, expectedVal := range map[string]string{"country": "India", "state": "Karnataka"} {
		var kv struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		var finalStatusCode int
		readDeadline := time.Now().Add(6 * time.Second)
		success := false

		for time.Now().Before(readDeadline) {
			url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/kv/%s", restartedAPIPort, key)
			resp, err := client.Get(url)
			if err == nil {
				finalStatusCode = resp.StatusCode
				if resp.StatusCode == http.StatusOK {
					if err := json.NewDecoder(resp.Body).Decode(&kv); err == nil {
						resp.Body.Close()
						if kv.Value == expectedVal {
							success = true
							break
						}
					} else {
						resp.Body.Close()
					}
				} else {
					resp.Body.Close()
				}
			}
			time.Sleep(50 * time.Millisecond)
		}

		if !success {
			t.Fatalf("key %s: expected %q on restarted node, got %q (status: %d)", key, expectedVal, kv.Value, finalStatusCode)
		}
	}
}
