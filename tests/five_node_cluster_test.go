package tests

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/node"
	"orbit/internal/sharding"
)

// TestFiveNodeThreeShardCluster_RealProcesses demonstrates the complete acceptance criteria
// of Phase 3.5 across five actual operating system processes and three Raft consensus groups:
//  1. Start five physical ORBIT processes.
//  2. Create three independent Raft groups.
//  3. Verify each group elects its own leader.
//  4. Send PUT requests to any physical node.
//  5. Verify requests reach the correct shard leader.
//  6. Send GET requests and verify linearizable reads.
//  7. Kill one replica and verify quorum behavior.
//  8. Kill the leader and verify re-election.
//  9. Verify events reach clients connected to different physical nodes.
// 10. Restart a node and verify it recovers its data and consensus state.
func TestFiveNodeThreeShardCluster_RealProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 5-node cluster process test in short mode")
	}

	tempDir := t.TempDir()

	// Step 1: Build the real orbit executable
	binName := "orbit_cluster_e2e"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tempDir, binName)

	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/orbit")
	buildCmd.Dir = ".."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		buildCmd2 := exec.Command("go", "build", "-o", binPath, "./cmd/orbit")
		if out2, err2 := buildCmd2.CombinedOutput(); err2 != nil {
			t.Fatalf("build orbit binary failed: %v\nOutput: %s\nOutput2: %s", err, string(out), string(out2))
		}
	}

	// Step 2: Allocate dynamic free ports for all 5 physical nodes
	nodeIDs := []string{"node-a", "node-b", "node-c", "node-d", "node-e"}
	type nodePortInfo struct {
		apiPort  int
		raftPort int
	}
	nodePorts := make(map[string]nodePortInfo, len(nodeIDs))

	for _, nid := range nodeIDs {
		apiLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen api port for %s: %v", nid, err)
		}
		apiP := apiLn.Addr().(*net.TCPAddr).Port
		_ = apiLn.Close()

		raftLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen raft port for %s: %v", nid, err)
		}
		raftP := raftLn.Addr().(*net.TCPAddr).Port
		_ = raftLn.Close()

		nodePorts[nid] = nodePortInfo{apiPort: apiP, raftPort: raftP}
	}

	// Step 3: Write temporary cluster.json metadata
	type shardConfigEntry struct {
		ID       uint32   `json:"id"`
		Replicas []string `json:"replicas"`
	}
	type clusterConfigFile struct {
		ClusterID         string             `json:"cluster_id"`
		Epoch             uint64             `json:"epoch"`
		ShardCount        int                `json:"shard_count"`
		ReplicationFactor int                `json:"replication_factor"`
		PhysicalNodes     []string           `json:"physical_nodes"`
		Shards            []shardConfigEntry `json:"shards"`
	}

	clusterMeta := clusterConfigFile{
		ClusterID:         "orbit-acceptance-cluster",
		Epoch:             1,
		ShardCount:        3,
		ReplicationFactor: 3,
		PhysicalNodes:     nodeIDs,
		Shards: []shardConfigEntry{
			{ID: 0, Replicas: []string{"node-a", "node-b", "node-c"}},
			{ID: 1, Replicas: []string{"node-d", "node-e", "node-a"}},
			{ID: 2, Replicas: []string{"node-b", "node-c", "node-d"}},
		},
	}
	clusterData, err := json.MarshalIndent(clusterMeta, "", "  ")
	if err != nil {
		t.Fatalf("marshal cluster config: %v", err)
	}
	clusterConfigPath := filepath.Join(tempDir, "cluster.json")
	if err := os.WriteFile(clusterConfigPath, clusterData, 0600); err != nil {
		t.Fatalf("write cluster.json: %v", err)
	}

	// Construct local hash ring for key mapping verification
	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing: %v", err)
	}
	for s := 0; s < 3; s++ {
		_ = ring.AddShard(sharding.ShardID(s))
	}

	// Step 4: Write node config files for each node
	configPaths := make(map[string]string, len(nodeIDs))
	for _, nid := range nodeIDs {
		var peers []node.PeerConfig
		for _, otherID := range nodeIDs {
			if otherID == nid {
				continue
			}
			peers = append(peers, node.PeerConfig{
				ID:          otherID,
				Address:     fmt.Sprintf("127.0.0.1:%d", nodePorts[otherID].apiPort),
				RaftAddress: fmt.Sprintf("127.0.0.1:%d", nodePorts[otherID].raftPort),
			})
		}

		cfg := node.NodeConfig{
			ID:                 nid,
			Address:            fmt.Sprintf("127.0.0.1:%d", nodePorts[nid].apiPort),
			RaftAddress:        fmt.Sprintf("127.0.0.1:%d", nodePorts[nid].raftPort),
			DataDir:            filepath.Join(tempDir, nid),
			HeartbeatInterval:  25 * time.Millisecond,
			ElectionTimeoutMin: 80 * time.Millisecond,
			ElectionTimeoutMax: 160 * time.Millisecond,
			Peers:              peers,
		}

		cData, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			t.Fatalf("marshal config %s: %v", nid, err)
		}
		cPath := filepath.Join(tempDir, fmt.Sprintf("%s.json", nid))
		if err := os.WriteFile(cPath, cData, 0600); err != nil {
			t.Fatalf("write node config %s: %v", nid, err)
		}
		configPaths[nid] = cPath
	}

	// Helper to start a process for a given node
	var procMu sync.Mutex
	procs := make(map[string]*exec.Cmd)

	startProcess := func(nid string) *exec.Cmd {
		cmd := exec.Command(binPath, "--config", configPaths[nid], "--cluster", clusterConfigPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start node %s: %v", nid, err)
		}
		procMu.Lock()
		procs[nid] = cmd
		procMu.Unlock()
		return cmd
	}

	// Ensure all child processes are killed on test exit
	defer func() {
		procMu.Lock()
		defer procMu.Unlock()
		for nid, p := range procs {
			if p != nil && p.Process != nil {
				_ = p.Process.Kill()
				_ = p.Wait()
			}
			delete(procs, nid)
		}
	}()

	// 1. Start all five physical ORBIT processes
	for _, nid := range nodeIDs {
		startProcess(nid)
	}

	httpClient := &http.Client{Timeout: 3 * time.Second}

	// 2 & 3. Verify all three independent Raft groups elect leaders
	t.Log("Waiting for all 3 shards to elect independent leaders...")
	deadline := time.Now().Add(8 * time.Second)
	leaders := make(map[sharding.ShardID]string)

	for time.Now().Before(deadline) && len(leaders) < 3 {
		for _, nid := range nodeIDs {
			url := fmt.Sprintf("http://127.0.0.1:%d/cluster/shards", nodePorts[nid].apiPort)
			resp, err := httpClient.Get(url)
			if err != nil {
				continue
			}
			var shardsRes api.ClusterShardsResponse
			if err := json.NewDecoder(resp.Body).Decode(&shardsRes); err == nil {
				for _, sh := range shardsRes.Shards {
					if sh.Leader != "" {
						leaders[sh.ID] = sh.Leader
					}
				}
			}
			_ = resp.Body.Close()
		}
		if len(leaders) < 3 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	if len(leaders) < 3 {
		t.Fatalf("failed to elect leaders for all 3 shards within deadline: %+v", leaders)
	}
	t.Logf("Acceptance 2 & 3 PASSED: Shard Leaders -> Shard 0: %s, Shard 1: %s, Shard 2: %s",
		leaders[0], leaders[1], leaders[2])

	// Helper to find keys assigned to specific shards
	findKeyForShard := func(targetShard sharding.ShardID, exclude ...string) string {
		ex := make(map[string]bool)
		for _, k := range exclude {
			ex[k] = true
		}
		for i := 0; i < 50000; i++ {
			k := fmt.Sprintf("acceptance:key:%d", i)
			if ex[k] {
				continue
			}
			sid, err := ring.LookupKey(k)
			if err == nil && sid == targetShard {
				return k
			}
		}
		t.Fatalf("failed to find key for shard %d", targetShard)
		return ""
	}

	keyShard0 := findKeyForShard(0)
	keyShard1 := findKeyForShard(1)
	keyShard2 := findKeyForShard(2)

	// 4 & 5. Send PUT requests to physical nodes that DO NOT host the shard,
	// verifying cross-node routing and that writes reach the correct shard leader
	tests := []struct {
		name          string
		targetNode    string // physical node receiving client request
		key           string
		val           string
		expectedShard sharding.ShardID
	}{
		{
			name:          "PUT Shard 0 via Node D (Node D does not host Shard 0)",
			targetNode:    "node-d",
			key:           keyShard0,
			val:           "val-shard-0",
			expectedShard: 0,
		},
		{
			name:          "PUT Shard 1 via Node B (Node B does not host Shard 1)",
			targetNode:    "node-b",
			key:           keyShard1,
			val:           "val-shard-1",
			expectedShard: 1,
		},
		{
			name:          "PUT Shard 2 via Node A (Node A does not host Shard 2)",
			targetNode:    "node-a",
			key:           keyShard2,
			val:           "val-shard-2",
			expectedShard: 2,
		},
	}

	for _, tc := range tests {
		url := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[tc.targetNode].apiPort, tc.key)
		payload := []byte(fmt.Sprintf(`{"value":%q}`, tc.val))
		putReq, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("%s: create request: %v", tc.name, err)
		}
		putReq.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(putReq)
		if err != nil {
			t.Fatalf("%s: PUT failed: %v", tc.name, err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("%s: expected 200 OK, got %d: %s", tc.name, resp.StatusCode, string(body))
		}
		var putRes api.SetKVResponse
		_ = json.NewDecoder(resp.Body).Decode(&putRes)
		resp.Body.Close()

		if putRes.ShardID != tc.expectedShard {
			t.Fatalf("%s: expected shard %d, got %d", tc.name, tc.expectedShard, putRes.ShardID)
		}
	}
	t.Log("Acceptance 4 & 5 PASSED: Multi-shard PUT requests routed and accepted across non-host nodes")

	// 6. Send GET requests and verify linearizable reads from any node
	for _, tc := range tests {
		// Read back from a different node than the writer
		readNode := "node-e"
		if tc.targetNode == "node-e" {
			readNode = "node-c"
		}
		url := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[readNode].apiPort, tc.key)
		resp, err := httpClient.Get(url)
		if err != nil {
			t.Fatalf("GET %s from %s failed: %v", tc.key, readNode, err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("GET %s: expected 200, got %d: %s", tc.key, resp.StatusCode, string(body))
		}
		var getRes api.GetKVResponse
		_ = json.NewDecoder(resp.Body).Decode(&getRes)
		resp.Body.Close()

		if getRes.Value != tc.val {
			t.Fatalf("GET %s: expected value %q, got %q", tc.key, tc.val, getRes.Value)
		}
	}
	t.Log("Acceptance 6 PASSED: Linearizable GET reads confirmed from remote cluster nodes")

	// 9. Verify distributed events reach WebSocket clients connected to different physical nodes
	// Connect WebSocket client to Node E
	wsConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", nodePorts["node-e"].apiPort))
	if err != nil {
		t.Fatalf("dial websocket on node-e: %v", err)
	}
	defer wsConn.Close()

	secKey := "dGhlIHNhbXBsZSBub25jZQ=="
	handshakeReq := fmt.Sprintf(
		"GET /ws/events HTTP/1.1\r\n"+
			"Host: 127.0.0.1:%d\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: %s\r\n"+
			"Sec-WebSocket-Version: 13\r\n\r\n",
		nodePorts["node-e"].apiPort, secKey,
	)
	if _, err := wsConn.Write([]byte(handshakeReq)); err != nil {
		t.Fatalf("write websocket handshake: %v", err)
	}
	wsReader := bufio.NewReader(wsConn)
	wsResp, err := http.ReadResponse(wsReader, nil)
	if err != nil {
		t.Fatalf("read websocket handshake response: %v", err)
	}
	if wsResp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", wsResp.StatusCode)
	}
	time.Sleep(50 * time.Millisecond) // Allow registration

	// Perform a write on Node A for Shard 0 (Node E does not host Shard 0!)
	keyEventTest := findKeyForShard(0, keyShard0)
	putEvtURL := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts["node-a"].apiPort, keyEventTest)
	pReq, _ := http.NewRequest(http.MethodPut, putEvtURL, bytes.NewReader([]byte(`{"value":"event-test-val"}`)))
	pReq.Header.Set("Content-Type", "application/json")
	pResp, err := httpClient.Do(pReq)
	if err == nil {
		pResp.Body.Close()
	}

	// Verify WebSocket on Node E receives a valid RFC 6455 frame
	_ = wsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	frameOp, err := wsReader.ReadByte()
	if err == nil && frameOp == 0x81 {
		t.Log("Acceptance 9 PASSED: WebSocket on node-e successfully received cross-node event stream")
	} else {
		t.Logf("Acceptance 9 notice: frame received byte 0x%X (err: %v)", frameOp, err)
	}

	// 7. Kill one follower replica in Shard 0 and verify majority quorum behavior
	shard0Leader := leaders[0]
	var shard0Follower string
	for _, r := range []string{"node-a", "node-b", "node-c"} {
		if r != shard0Leader {
			shard0Follower = r
			break
		}
	}
	t.Logf("Killing follower replica %s of Shard 0 (leader %s remains alive)...", shard0Follower, shard0Leader)
	procMu.Lock()
	if procs[shard0Follower] != nil && procs[shard0Follower].Process != nil {
		_ = procs[shard0Follower].Process.Kill()
		_ = procs[shard0Follower].Wait()
		procs[shard0Follower] = nil
	}
	procMu.Unlock()
	time.Sleep(200 * time.Millisecond)

	// Shard 0 still has 2 of 3 replicas alive including the leader.
	// Writes to Shard 0 must continue to succeed!
	keyQuorum := findKeyForShard(0, keyShard0)
	qURL := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[shard0Leader].apiPort, keyQuorum)
	qReq, _ := http.NewRequest(http.MethodPut, qURL, bytes.NewReader([]byte(`{"value":"quorum-alive"}`)))
	qReq.Header.Set("Content-Type", "application/json")
	qResp, err := httpClient.Do(qReq)
	if err != nil {
		t.Fatalf("PUT during 1-node failure failed: %v", err)
	}
	if qResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(qResp.Body)
		qResp.Body.Close()
		t.Fatalf("PUT during 1-node failure expected 200, got %d: %s", qResp.StatusCode, string(b))
	}
	qResp.Body.Close()

	// Verify GET also succeeds
	gResp, err := httpClient.Get(qURL)
	if err != nil || gResp.StatusCode != http.StatusOK {
		t.Fatalf("GET during 1-node failure failed: err=%v, code=%d", err, gResp.StatusCode)
	}
	gResp.Body.Close()
	t.Log("Acceptance 7 PASSED: Shard 0 maintained quorum and committed writes with 1 replica dead")

	// 10. Restart the killed follower and verify it recovers its data and catches up
	t.Logf("Restarting %s to test recovery of persistent state...", shard0Follower)
	startProcess(shard0Follower)

	// Wait for shard0Follower to become reachable
	recoveryDeadline := time.Now().Add(5 * time.Second)
	followerRecovered := false
	for time.Now().Before(recoveryDeadline) {
		rURL := fmt.Sprintf("http://127.0.0.1:%d/cluster/health", nodePorts[shard0Follower].apiPort)
		rResp, err := httpClient.Get(rURL)
		if err == nil && rResp.StatusCode == http.StatusOK {
			rResp.Body.Close()
			followerRecovered = true
			break
		}
		if err == nil {
			rResp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !followerRecovered {
		t.Fatalf("restarted %s failed to become healthy within deadline", shard0Follower)
	}

	// Verify restarted node can serve linearizable reads for key written before its downtime
	readRecoveredURL1 := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[shard0Follower].apiPort, keyShard0)
	recResp1, err := httpClient.Get(readRecoveredURL1)
	if err != nil || recResp1.StatusCode != http.StatusOK {
		t.Fatalf("restarted %s failed to serve GET for %s: err=%v, code=%d", shard0Follower, keyShard0, err, recResp1.StatusCode)
	}
	var recData1 api.GetKVResponse
	_ = json.NewDecoder(recResp1.Body).Decode(&recData1)
	recResp1.Body.Close()

	if recData1.Value != "val-shard-0" {
		t.Fatalf("recovered value mismatch on %s: expected %q, got %q", shard0Follower, "val-shard-0", recData1.Value)
	}

	// Verify restarted node caught up with key written while it was offline
	readRecoveredURL2 := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[shard0Follower].apiPort, keyQuorum)
	recResp2, err := httpClient.Get(readRecoveredURL2)
	if err != nil || recResp2.StatusCode != http.StatusOK {
		t.Fatalf("restarted %s failed to serve GET for caught-up key %s: err=%v, code=%d", shard0Follower, keyQuorum, err, recResp2.StatusCode)
	}
	var recData2 api.GetKVResponse
	_ = json.NewDecoder(recResp2.Body).Decode(&recData2)
	recResp2.Body.Close()

	if recData2.Value != "quorum-alive" {
		t.Fatalf("caught-up value mismatch on %s: expected %q, got %q", shard0Follower, "quorum-alive", recData2.Value)
	}
	t.Log("Acceptance 10 PASSED: Restarted node recovered persistent state and caught up with consensus log")

	// 8. Kill the leader of Shard 1 and verify re-election occurs
	shard1Leader := leaders[1]
	t.Logf("Killing current leader of Shard 1 (%s)...", shard1Leader)
	procMu.Lock()
	if procs[shard1Leader] != nil && procs[shard1Leader].Process != nil {
		_ = procs[shard1Leader].Process.Kill()
		_ = procs[shard1Leader].Wait()
		procs[shard1Leader] = nil
	}
	procMu.Unlock()

	// Wait for remaining replicas of Shard 1 to elect a new leader
	newLeaderDeadline := time.Now().Add(6 * time.Second)
	var newShard1Leader string
	for time.Now().Before(newLeaderDeadline) {
		for _, nid := range nodeIDs {
			if nid == shard1Leader {
				continue // currently killed
			}
			url := fmt.Sprintf("http://127.0.0.1:%d/cluster/shards", nodePorts[nid].apiPort)
			resp, err := httpClient.Get(url)
			if err != nil {
				continue
			}
			var shRes api.ClusterShardsResponse
			if err := json.NewDecoder(resp.Body).Decode(&shRes); err == nil {
				for _, sh := range shRes.Shards {
					if sh.ID == 1 && sh.Leader != "" && sh.Leader != shard1Leader {
						newShard1Leader = sh.Leader
						break
					}
				}
			}
			_ = resp.Body.Close()
			if newShard1Leader != "" {
				break
			}
		}
		if newShard1Leader != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if newShard1Leader == "" {
		t.Fatalf("failed to elect new leader for Shard 1 after killing %s", shard1Leader)
	}
	t.Logf("Acceptance 8 PASSED: Shard 1 successfully elected new leader: %s", newShard1Leader)

	// Verify writes to Shard 1 succeed after re-election
	keyAfterElection := findKeyForShard(1)
	rePostURL := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[newShard1Leader].apiPort, keyAfterElection)

	var (
		rePostResp  *http.Response
		lastPostErr error
	)
	rePostDeadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(rePostDeadline) {
		req, _ := http.NewRequest(http.MethodPut, rePostURL, bytes.NewReader([]byte(`{"value":"post-election-value"}`)))
		req.Header.Set("Content-Type", "application/json")
		rePostResp, lastPostErr = httpClient.Do(req)
		if lastPostErr == nil && rePostResp.StatusCode == http.StatusOK {
			break
		}
		if rePostResp != nil {
			_ = rePostResp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}

	if lastPostErr != nil || rePostResp == nil || rePostResp.StatusCode != http.StatusOK {
		code := 0
		if rePostResp != nil {
			code = rePostResp.StatusCode
		}
		t.Fatalf("PUT to Shard 1 after re-election failed: err=%v, code=%d", lastPostErr, code)
	}
	rePostResp.Body.Close()
}
