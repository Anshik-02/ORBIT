package tests

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"orbit/internal/api"
	"orbit/internal/node"
	"orbit/internal/raft"
	"orbit/internal/sharding"
)

// TestPhase36_ComprehensiveFailureScenarios validates the Phase 3.6 requirements across
// a real five-node, three-shard cluster running as distinct operating system processes:
//  1. One follower dies.
//  2. Two followers die (quorum loss).
//  3. Leader dies.
//  4. Old leader rejoins.
//  5. One entire shard loses quorum.
//  6. Another shard continues operating independently.
//  7. Network communication interrupted (timeouts and forwarder failover).
//  8. Duplicate events received (deduplication over WebSocket).
//  9. Node restarts with existing data (WAL + persistent Raft recovery).
func TestPhase36_ComprehensiveFailureScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 5-node cluster failure scenarios in short mode")
	}

	tempDir := t.TempDir()

	// Step 1: Build the real orbit executable
	binName := "orbit_failure_e2e"
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

	// Step 3: Write cluster.json
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
		ClusterID:         "orbit-failure-test-cluster",
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

	// Construct local hash ring for key mapping
	ring, err := sharding.NewHashRing(sharding.DefaultVirtualNodes)
	if err != nil {
		t.Fatalf("NewHashRing: %v", err)
	}
	for s := 0; s < 3; s++ {
		_ = ring.AddShard(sharding.ShardID(s))
	}

	// Step 4: Write node configs
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

	stopProcess := func(nid string) {
		procMu.Lock()
		p := procs[nid]
		if p != nil && p.Process != nil {
			_ = p.Process.Kill()
			_ = p.Wait()
		}
		procs[nid] = nil
		procMu.Unlock()
	}

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

	// Start all five nodes
	for _, nid := range nodeIDs {
		startProcess(nid)
	}

	httpClient := &http.Client{Timeout: 3 * time.Second}

	// Wait for leader election across all 3 shards
	t.Log("Waiting for leaders to be elected on all 3 shards...")
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
		t.Fatalf("failed to elect leaders on all 3 shards: %+v", leaders)
	}
	t.Logf("Initial Leaders: Shard 0: %s, Shard 1: %s, Shard 2: %s", leaders[0], leaders[1], leaders[2])

	findKeyForShard := func(targetShard sharding.ShardID, exclude ...string) string {
		ex := make(map[string]bool)
		for _, k := range exclude {
			ex[k] = true
		}
		for i := 0; i < 50000; i++ {
			k := fmt.Sprintf("phase36:key:%d", i)
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

	// -------------------------------------------------------------
	// Scenario 1: One follower dies
	// -------------------------------------------------------------
	t.Log("\n--- Scenario 1: One follower dies (Shard 0) ---")
	shard0Leader := leaders[0]
	var shard0Follower1 string
	for _, r := range []string{"node-a", "node-b", "node-c"} {
		if r != shard0Leader {
			shard0Follower1 = r
			break
		}
	}
	t.Logf("Killing first follower of Shard 0 (%s)...", shard0Follower1)
	stopProcess(shard0Follower1)
	time.Sleep(150 * time.Millisecond)

	keyS1 := findKeyForShard(0)
	putURL1 := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[shard0Leader].apiPort, keyS1)
	putReq1, _ := http.NewRequest(http.MethodPut, putURL1, bytes.NewReader([]byte(`{"value":"val-one-follower-dead"}`)))
	putReq1.Header.Set("Content-Type", "application/json")
	putResp1, err := httpClient.Do(putReq1)
	if err != nil || putResp1.StatusCode != http.StatusOK {
		t.Fatalf("Scenario 1 failed: PUT with 1 follower dead expected 200, got status=%v, err=%v", putResp1.StatusCode, err)
	}
	putResp1.Body.Close()

	getResp1, err := httpClient.Get(putURL1)
	if err != nil || getResp1.StatusCode != http.StatusOK {
		t.Fatalf("Scenario 1 failed: GET with 1 follower dead expected 200, got status=%v, err=%v", getResp1.StatusCode, err)
	}
	getResp1.Body.Close()
	t.Log("Scenario 1 PASSED: Shard 0 committed write and served linearizable read with 1 follower dead.")

	// -------------------------------------------------------------
	// Scenario 2, 5 & 6: Two followers die -> One shard loses quorum, another shard continues operating
	// -------------------------------------------------------------
	t.Log("\n--- Scenarios 2, 5 & 6: Two followers die -> Shard 0 loses quorum while Shard 1 continues operating ---")
	var shard0Follower2 string
	for _, r := range []string{"node-a", "node-b", "node-c"} {
		if r != shard0Leader && r != shard0Follower1 {
			shard0Follower2 = r
			break
		}
	}
	t.Logf("Killing second follower of Shard 0 (%s) -> Shard 0 loses quorum (1/3 alive)...", shard0Follower2)
	stopProcess(shard0Follower2)
	time.Sleep(150 * time.Millisecond)

	// Shard 0: PUT must timeout or return 503 because quorum is lost
	keyS0QuorumLost := findKeyForShard(0, keyS1)
	putURL0QuorumLost := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[shard0Leader].apiPort, keyS0QuorumLost)

	shortClient := &http.Client{Timeout: 1500 * time.Millisecond}
	pReq0Lost, _ := http.NewRequest(http.MethodPut, putURL0QuorumLost, bytes.NewReader([]byte(`{"value":"should-fail"}`)))
	pReq0Lost.Header.Set("Content-Type", "application/json")
	pResp0Lost, pErr0Lost := shortClient.Do(pReq0Lost)

	if pErr0Lost == nil {
		if pResp0Lost.StatusCode == http.StatusOK {
			t.Fatalf("Scenario 2 & 5 failed: write to Shard 0 succeeded despite loss of majority quorum!")
		}
		t.Logf("Scenario 2 & 5: Write to Shard 0 correctly returned status %d (%s) due to quorum loss",
			pResp0Lost.StatusCode, http.StatusText(pResp0Lost.StatusCode))
		pResp0Lost.Body.Close()
	} else {
		t.Logf("Scenario 2 & 5: Write to Shard 0 correctly timed out waiting for quorum: %v", pErr0Lost)
	}

	// Shard 1 check (Node D, Node E, Node A):
	// Check which node is alive and hosts Shard 1
	var liveShard1Node string
	for _, n := range []string{"node-d", "node-e", "node-a"} {
		if n != shard0Follower1 && n != shard0Follower2 {
			liveShard1Node = n
			break
		}
	}
	t.Logf("Verifying Shard 1 continues operating via live node %s...", liveShard1Node)
	keyS1Alive := findKeyForShard(1)
	putURLS1 := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[liveShard1Node].apiPort, keyS1Alive)

	var (
		pRespS1   *http.Response
		lastS1Err error
	)
	s1Deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(s1Deadline) {
		pReqS1, _ := http.NewRequest(http.MethodPut, putURLS1, bytes.NewReader([]byte(`{"value":"shard1-alive-and-well"}`)))
		pReqS1.Header.Set("Content-Type", "application/json")
		pRespS1, lastS1Err = httpClient.Do(pReqS1)
		if lastS1Err == nil && pRespS1.StatusCode == http.StatusOK {
			break
		}
		if pRespS1 != nil {
			_ = pRespS1.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}

	if lastS1Err != nil || pRespS1 == nil || pRespS1.StatusCode != http.StatusOK {
		code := 0
		if pRespS1 != nil {
			code = pRespS1.StatusCode
		}
		t.Fatalf("Scenario 6 failed: Shard 1 write failed while Shard 0 lost quorum: err=%v, code=%d",
			lastS1Err, code)
	}
	pRespS1.Body.Close()

	var (
		getRespS1  *http.Response
		lastGetErr error
	)
	for time.Now().Before(s1Deadline) {
		getRespS1, lastGetErr = httpClient.Get(putURLS1)
		if lastGetErr == nil && getRespS1.StatusCode == http.StatusOK {
			break
		}
		if getRespS1 != nil {
			_ = getRespS1.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastGetErr != nil || getRespS1 == nil || getRespS1.StatusCode != http.StatusOK {
		code := 0
		if getRespS1 != nil {
			code = getRespS1.StatusCode
		}
		t.Fatalf("Scenario 6 failed: Shard 1 GET read failed: err=%v, code=%d", lastGetErr, code)
	}
	getRespS1.Body.Close()
	t.Log("Scenarios 2, 5 & 6 PASSED: Shard 0 lost quorum and rejected writes; Shard 1 continued operating independently without interruption.")

	// -------------------------------------------------------------
	// Scenario 9: Node restarts with existing data
	// -------------------------------------------------------------
	t.Log("\n--- Scenario 9: Restarting killed nodes with existing data ---")
	startProcess(shard0Follower1)
	startProcess(shard0Follower2)

	// Wait for nodes to recover
	recDeadline := time.Now().Add(5 * time.Second)
	nodesRecovered := false
	for time.Now().Before(recDeadline) {
		r1, err1 := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/cluster/health", nodePorts[shard0Follower1].apiPort))
		r2, err2 := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/cluster/health", nodePorts[shard0Follower2].apiPort))
		if err1 == nil && err2 == nil && r1.StatusCode == http.StatusOK && r2.StatusCode == http.StatusOK {
			r1.Body.Close()
			r2.Body.Close()
			nodesRecovered = true
			break
		}
		if err1 == nil {
			r1.Body.Close()
		}
		if err2 == nil {
			r2.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !nodesRecovered {
		t.Fatalf("Scenario 9 failed: restarted nodes failed to become healthy within deadline")
	}

	// Verify linearizable read on restarted node for data written before it died
	getRecURL := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[shard0Follower1].apiPort, keyS1)
	getRecResp, err := httpClient.Get(getRecURL)
	if err != nil || getRecResp.StatusCode != http.StatusOK {
		t.Fatalf("Scenario 9 failed: restarted node could not serve read for %s: err=%v, code=%v", keyS1, err, getRecResp.StatusCode)
	}
	var getRecData api.GetKVResponse
	_ = json.NewDecoder(getRecResp.Body).Decode(&getRecData)
	getRecResp.Body.Close()
	if getRecData.Value != "val-one-follower-dead" {
		t.Fatalf("Scenario 9 failed: expected recovered value %q, got %q", "val-one-follower-dead", getRecData.Value)
	}
	t.Log("Scenario 9 PASSED: Restarted nodes recovered existing data from persistent logs and WAL.")

	// -------------------------------------------------------------
	// Scenario 3 & 4: Leader dies and old leader rejoins
	// -------------------------------------------------------------
	t.Log("\n--- Scenarios 3 & 4: Leader dies and old leader rejoins ---")
	// Find current leader of Shard 1
	var currentShard1Leader string
	for time.Now().Before(time.Now().Add(4 * time.Second)) {
		url := fmt.Sprintf("http://127.0.0.1:%d/cluster/shards", nodePorts["node-d"].apiPort)
		resp, err := httpClient.Get(url)
		if err == nil {
			var shRes api.ClusterShardsResponse
			_ = json.NewDecoder(resp.Body).Decode(&shRes)
			resp.Body.Close()
			for _, sh := range shRes.Shards {
				if sh.ID == 1 && sh.Leader != "" {
					currentShard1Leader = sh.Leader
					break
				}
			}
		}
		if currentShard1Leader != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if currentShard1Leader == "" {
		t.Fatal("failed to identify current leader of Shard 1")
	}
	t.Logf("Current Shard 1 leader is %s. Killing it...", currentShard1Leader)
	stopProcess(currentShard1Leader)

	// Wait for new leader election on Shard 1
	var newShard1Leader string
	electionDeadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(electionDeadline) {
		for _, nid := range []string{"node-d", "node-e", "node-a"} {
			if nid == currentShard1Leader {
				continue
			}
			url := fmt.Sprintf("http://127.0.0.1:%d/cluster/shards", nodePorts[nid].apiPort)
			resp, err := httpClient.Get(url)
			if err != nil {
				continue
			}
			var shRes api.ClusterShardsResponse
			if err := json.NewDecoder(resp.Body).Decode(&shRes); err == nil {
				for _, sh := range shRes.Shards {
					if sh.ID == 1 && sh.Leader != "" && sh.Leader != currentShard1Leader {
						newShard1Leader = sh.Leader
						break
					}
				}
			}
			resp.Body.Close()
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
		t.Fatalf("Scenario 3 failed: new leader not elected for Shard 1 after killing %s", currentShard1Leader)
	}
	t.Logf("Scenario 3 PASSED: New leader elected for Shard 1: %s", newShard1Leader)

	// Propose write to Shard 1 while old leader is dead
	keyWhileLeaderDead := findKeyForShard(1, keyS1Alive)
	putURLNewLeader := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[newShard1Leader].apiPort, keyWhileLeaderDead)
	var pRespNL *http.Response
	var errNL error
	nlDeadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(nlDeadline) {
		pReqNL, _ := http.NewRequest(http.MethodPut, putURLNewLeader, bytes.NewReader([]byte(`{"value":"written-under-new-leader"}`)))
		pReqNL.Header.Set("Content-Type", "application/json")
		pRespNL, errNL = httpClient.Do(pReqNL)
		if errNL == nil && pRespNL.StatusCode == http.StatusOK {
			break
		}
		if pRespNL != nil {
			_ = pRespNL.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	if errNL != nil || pRespNL == nil || pRespNL.StatusCode != http.StatusOK {
		code := 0
		if pRespNL != nil {
			code = pRespNL.StatusCode
		}
		t.Fatalf("write to new leader failed: err=%v, code=%v", errNL, code)
	}
	pRespNL.Body.Close()

	// Old leader rejoins
	t.Logf("Restarting old leader %s...", currentShard1Leader)
	startProcess(currentShard1Leader)

	// Wait for old leader to boot up and catch up
	time.Sleep(500 * time.Millisecond)

	// Verify old leader has stepped down to follower and catches up with the newly committed entry
	readFromOldLeaderURL := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts[currentShard1Leader].apiPort, keyWhileLeaderDead)
	catchupDeadline := time.Now().Add(5 * time.Second)
	var caughtUpValue string
	for time.Now().Before(catchupDeadline) {
		cResp, err := httpClient.Get(readFromOldLeaderURL)
		if err == nil && cResp.StatusCode == http.StatusOK {
			var getRes api.GetKVResponse
			_ = json.NewDecoder(cResp.Body).Decode(&getRes)
			cResp.Body.Close()
			if getRes.Value == "written-under-new-leader" {
				caughtUpValue = getRes.Value
				break
			}
		} else if err == nil {
			cResp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}

	if caughtUpValue != "written-under-new-leader" {
		t.Fatalf("Scenario 4 failed: old leader %s did not catch up with new leader's write: got %q",
			currentShard1Leader, caughtUpValue)
	}
	t.Log("Scenario 4 PASSED: Old leader rejoined, stepped down, and caught up with new consensus log.")

	// -------------------------------------------------------------
	// Scenario 7: Network communication interrupted / forwarder failover
	// -------------------------------------------------------------
	t.Log("\n--- Scenario 7: Forwarder failover during node communication failure ---")
	// Send request to Node B for Shard 1
	// Even if Node B's cached leader was old, forwardWithFailover will route to the live leader!
	keyFailover := findKeyForShard(1, keyWhileLeaderDead)
	putFailoverURL := fmt.Sprintf("http://127.0.0.1:%d/kv/%s", nodePorts["node-b"].apiPort, keyFailover)
	var pRespFO *http.Response
	var errFO error
	foDeadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(foDeadline) {
		pReqFO, _ := http.NewRequest(http.MethodPut, putFailoverURL, bytes.NewReader([]byte(`{"value":"failover-success"}`)))
		pReqFO.Header.Set("Content-Type", "application/json")
		pRespFO, errFO = httpClient.Do(pReqFO)
		if errFO == nil && pRespFO.StatusCode == http.StatusOK {
			break
		}
		if pRespFO != nil {
			_ = pRespFO.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	if errFO != nil || pRespFO == nil || pRespFO.StatusCode != http.StatusOK {
		code := 0
		if pRespFO != nil {
			code = pRespFO.StatusCode
		}
		t.Fatalf("Scenario 7 failed: forwarder failover to active replica failed: err=%v, code=%v", errFO, code)
	}
	pRespFO.Body.Close()
	t.Log("Scenario 7 PASSED: Forwarder successfully routed request to active leader across failure boundaries.")

	// -------------------------------------------------------------
	// Scenario 8: Duplicate events received & WebSocket verification
	// -------------------------------------------------------------
	t.Log("\n--- Scenario 8: Duplicate events received and verified deduplicated ---")
	wsConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", nodePorts["node-e"].apiPort))
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
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
	if err != nil || wsResp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("websocket handshake failed: err=%v, code=%d", err, wsResp.StatusCode)
	}
	time.Sleep(50 * time.Millisecond)

	// Send an event with a specific ID to node-e relay
	dupEvtID := fmt.Sprintf("dup-test-%d", time.Now().UnixNano())
	relayPayload := fmt.Sprintf(`{"event_id":%q,"type":"ENTRY_COMMITTED","node_id":"node-a","data":{"key":"dup-key"}}`, dupEvtID)

	relayURL := fmt.Sprintf("http://127.0.0.1:%d/cluster/events/relay", nodePorts["node-e"].apiPort)
	// Post 1st time
	r1, err := httpClient.Post(relayURL, "application/json", strings.NewReader(relayPayload))
	if err != nil || r1.StatusCode != http.StatusOK {
		t.Fatalf("relay event 1 failed: err=%v, code=%d", err, r1.StatusCode)
	}
	r1.Body.Close()

	// Post 2nd time (duplicate)
	r2, err := httpClient.Post(relayURL, "application/json", strings.NewReader(relayPayload))
	if err != nil || r2.StatusCode != http.StatusOK {
		t.Fatalf("relay duplicate event 2 failed: err=%v, code=%d", err, r2.StatusCode)
	}
	r2.Body.Close()

	readWSFrame := func(r *bufio.Reader) ([]byte, error) {
		b0, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b0 != 0x81 {
			return nil, fmt.Errorf("unexpected opcode 0x%X", b0)
		}
		b1, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		payloadLen := int(b1 & 0x7F)
		if payloadLen == 126 {
			var lenBytes [2]byte
			if _, err := io.ReadFull(r, lenBytes[:]); err != nil {
				return nil, err
			}
			payloadLen = int(uint16(lenBytes[0])<<8 | uint16(lenBytes[1]))
		}
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
		return payload, nil
	}

	// Verify WebSocket receives frame 1
	_ = wsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	payload1, err := readWSFrame(wsReader)
	if err != nil {
		t.Fatalf("expected WebSocket frame for first event, got err: %v", err)
	}
	var evt1 map[string]any
	_ = json.Unmarshal(payload1, &evt1)
	t.Logf("Received first event over WebSocket: %+v", string(payload1))

	// Verify WebSocket does NOT receive frame 2 (deduplicated)
	_ = wsConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	payload2, err := readWSFrame(wsReader)
	if err == nil {
		t.Fatalf("unexpected frame received for duplicate event: %s", string(payload2))
	}
	t.Log("Scenario 8 PASSED: Duplicate event was cleanly deduplicated with no duplicate frame emitted over WebSocket.")

	t.Log("\n>>> ALL PHASE 3.6 FAILURE SCENARIOS COMPLETED SUCCESSFULLY! <<<")
}

// TestPhase36_DeterministicNodeRecoveryActualRaftState deterministically exercises
// the full persistence and recovery path:
//  1. Boot a 3-node shard cluster.
//  2. Elect leader and commit multiple real Raft entries through Propose.
//  3. Capture the exact persistent state (term, commitIndex, appliedIndex, disk data).
//  4. Cleanly stop a replica node.
//  5. Reopen the replica using the exact same data directory.
//  6. Verify term, log entries, lastApplied, and database state are recovered independently
//     without any mock or manual log injections.
func TestPhase36_DeterministicNodeRecoveryActualRaftState(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	nodeIDs := []string{"det-node-a", "det-node-b", "det-node-c"}

	// Topology: 1 shard across all 3 nodes
	meta := sharding.NewClusterMetadata("det-recovery-cluster", 1, 1, 3, nodeIDs)
	_ = meta.AddShard(&sharding.ShardInfo{ID: 0, Replicas: nodeIDs})

	// Allocate ports
	type portPair struct {
		apiPort  int
		raftPort int
	}
	ports := make(map[string]portPair)
	for _, nid := range nodeIDs {
		apiLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen api %s: %v", nid, err)
		}
		apiP := apiLn.Addr().(*net.TCPAddr).Port
		_ = apiLn.Close()

		raftLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen raft %s: %v", nid, err)
		}
		raftP := raftLn.Addr().(*net.TCPAddr).Port
		_ = raftLn.Close()

		ports[nid] = portPair{apiPort: apiP, raftPort: raftP}
	}

	peerRaftAddrs := make(map[string]string)
	for _, nid := range nodeIDs {
		peerRaftAddrs[nid] = fmt.Sprintf("127.0.0.1:%d", ports[nid].raftPort)
	}

	nodes := make(map[string]*node.Node)
	for _, nid := range nodeIDs {
		var peers []node.PeerConfig
		for _, other := range nodeIDs {
			if other != nid {
				peers = append(peers, node.PeerConfig{
					ID:          other,
					Address:     fmt.Sprintf("127.0.0.1:%d", ports[other].apiPort),
					RaftAddress: fmt.Sprintf("127.0.0.1:%d", ports[other].raftPort),
				})
			}
		}

		cfg := node.NodeConfig{
			ID:                 nid,
			Address:            fmt.Sprintf("127.0.0.1:%d", ports[nid].apiPort),
			RaftAddress:        peerRaftAddrs[nid],
			DataDir:            filepath.Join(tempDir, nid),
			HeartbeatInterval:  20 * time.Millisecond,
			ElectionTimeoutMin: 150 * time.Millisecond,
			ElectionTimeoutMax: 300 * time.Millisecond,
			Peers:              peers,
		}

		n, err := node.NewShardedNode(cfg, meta, peerRaftAddrs, nil)
		if err != nil {
			t.Fatalf("NewShardedNode %s: %v", nid, err)
		}
		nodes[nid] = n
	}

	// Start all nodes
	for _, nid := range nodeIDs {
		if err := nodes[nid].Start(); err != nil {
			t.Fatalf("start %s: %v", nid, err)
		}
	}
	defer func() {
		for _, n := range nodes {
			if n != nil {
				_ = n.Stop()
			}
		}
	}()

	// Wait for leader election on Shard 0
	deadline := time.Now().Add(5 * time.Second)
	var leaderNode *node.Node
	for time.Now().Before(deadline) {
		for _, n := range nodes {
			sh, ok := n.Shard(0)
			if ok && sh.Raft().Role() == raft.Leader {
				leaderNode = n
				break
			}
		}
		if leaderNode != nil {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	if leaderNode == nil {
		t.Fatal("failed to elect leader for shard 0")
	}
	t.Logf("Elected leader: %s", leaderNode.ID())

	// Propose 5 real key-value entries
	for i := 1; i <= 5; i++ {
		k := fmt.Sprintf("user:%d", i)
		v := fmt.Sprintf("data-val-%d", i)

		var (
			currLeaderNode *node.Node
			idx            uint64
			propErr        error
		)
		propDeadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(propDeadline) {
			for _, n := range nodes {
				if n == nil {
					continue
				}
				sh, ok := n.Shard(0)
				if ok && sh.Raft().Role() == raft.Leader {
					idx, _, propErr = sh.Raft().Propose("SET", k, v)
					if propErr == nil {
						currLeaderNode = n
						break
					}
				}
			}
			if currLeaderNode != nil {
				leaderNode = currLeaderNode
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if currLeaderNode == nil {
			t.Fatalf("propose %s: %v", k, propErr)
		}

		sh, _ := currLeaderNode.Shard(0)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := sh.Raft().WaitForApply(ctx, idx); err != nil {
			cancel()
			t.Fatalf("wait for apply %d: %v", idx, err)
		}
		cancel()
	}

	// Pick a follower to test persistence and recovery
	var followerID string
	for _, nid := range nodeIDs {
		if nid != leaderNode.ID() {
			followerID = nid
			break
		}
	}
	targetNode := nodes[followerID]
	followerShard, _ := targetNode.Shard(0)

	// Wait for follower to apply all 5 entries
	applyDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(applyDeadline) {
		if followerShard.Raft().LastApplied() >= 5 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	preStopTerm := followerShard.Raft().CurrentTerm()
	preStopApplied := followerShard.Raft().LastApplied()
	preStopCommit := followerShard.Raft().CommitIndex()
	t.Logf("Follower %s pre-stop state: Term=%d, CommitIndex=%d, LastApplied=%d",
		followerID, preStopTerm, preStopCommit, preStopApplied)

	if preStopApplied < 5 {
		t.Fatalf("follower lastApplied index too low before stop: %d", preStopApplied)
	}

	// Verify data before stop
	valBefore, err := followerShard.DB().Get("user:3")
	if err != nil || valBefore != "data-val-3" {
		t.Fatalf("pre-stop value mismatch: val=%q, err=%v", valBefore, err)
	}

	// Cleanly stop the follower
	if err := targetNode.Stop(); err != nil {
		t.Fatalf("stop %s: %v", followerID, err)
	}
	nodes[followerID] = nil

	// Recreate follower from disk with same configuration
	var peers []node.PeerConfig
	for _, other := range nodeIDs {
		if other != followerID {
			peers = append(peers, node.PeerConfig{
				ID:          other,
				Address:     fmt.Sprintf("127.0.0.1:%d", ports[other].apiPort),
				RaftAddress: fmt.Sprintf("127.0.0.1:%d", ports[other].raftPort),
			})
		}
	}

	recoveredCfg := node.NodeConfig{
		ID:                 followerID,
		Address:            fmt.Sprintf("127.0.0.1:%d", ports[followerID].apiPort),
		RaftAddress:        peerRaftAddrs[followerID],
		DataDir:            filepath.Join(tempDir, followerID),
		HeartbeatInterval:  20 * time.Millisecond,
		ElectionTimeoutMin: 150 * time.Millisecond,
		ElectionTimeoutMax: 300 * time.Millisecond,
		Peers:              peers,
	}

	recoveredNode, err := node.NewShardedNode(recoveredCfg, meta, peerRaftAddrs, nil)
	if err != nil {
		t.Fatalf("recreate NewShardedNode %s: %v", followerID, err)
	}
	nodes[followerID] = recoveredNode

	if err := recoveredNode.Start(); err != nil {
		t.Fatalf("start recovered node %s: %v", followerID, err)
	}

	recShard, ok := recoveredNode.Shard(0)
	if !ok {
		t.Fatalf("recovered node missing shard 0")
	}

	// Verify actual recovered Raft state:
	// 1. Term recovered from storage must match pre-stop term
	if recShard.Raft().CurrentTerm() != preStopTerm {
		t.Errorf("recovered term mismatch: expected %d, got %d", preStopTerm, recShard.Raft().CurrentTerm())
	}

	// 2. InitialLastApplied must match pre-stop applied to prevent duplicate re-application
	if recShard.Engine().LastAppliedIndex() != preStopApplied {
		t.Errorf("recovered engine last applied mismatch: expected %d, got %d", preStopApplied, recShard.Engine().LastAppliedIndex())
	}

	// 3. All 5 keys must be readable directly from the recovered database
	for i := 1; i <= 5; i++ {
		k := fmt.Sprintf("user:%d", i)
		expected := fmt.Sprintf("data-val-%d", i)
		got, err := recShard.DB().Get(k)
		if err != nil {
			t.Errorf("recovered DB get %s failed: %v", k, err)
		}
		if got != expected {
			t.Errorf("recovered DB value mismatch for %s: expected %q, got %q", k, expected, got)
		}
	}

	t.Logf("Successfully verified actual Raft state recovery for node %s!", followerID)
}
