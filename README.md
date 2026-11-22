# ORBIT — Distributed Consensus Key-Value Database

ORBIT is a high-performance, crash-resilient, distributed Key-Value database written in Go. It combines an ACID-compliant single-node storage engine (Write-Ahead Logging with streaming recovery and process locking) with a production-grade implementation of the **Raft Consensus Algorithm** (leader election, randomized timeouts, log replication, majority commitment, fault tolerance, and automatic client request forwarding/redirects).

---

## Architecture Overview

```
                          CLIENT (HTTP)
                                |
                                v
                     +---------------------+
                     |    NODE A (2525)    |
                     |       LEADER        |
                     +----------+----------+
                                |
                  Raft RPC (AppendEntries)
                                |
               +----------------+----------------+
               |                                 |
               v                                 v
     +-------------------+             +-------------------+
     |   NODE B (2526)   |             |   NODE C (2527)   |
     |     FOLLOWER      |             |     FOLLOWER      |
     +-------------------+             +-------------------+

   Each Node Architecture:
   +-------------------------------------------------------+
   |  Client HTTP API (2525) -> Leader-aware / Redirects  |
   +-------------------------------------------------------+
   |  Raft Transport (3525)  -> AppendEntries / Vote RPCs  |
   +-------------------------------------------------------+
   |  Raft Consensus Engine  -> Election / Quorum Commit   |
   |    - raft.log (consensus log) + metadata.json         |
   +-------------------------------------------------------+
   |  Storage Engine (ApplyCommitted)                      |
   |    - wal.log (applied state WAL)                      |
   |    - Memtable (concurrent in-memory state)            |
   +-------------------------------------------------------+
```

### Two Distinct Logs Architectural Decoupling
1. **Raft Consensus Log (`data/<node>/raft/raft.log` + `metadata.json`)**:
   Tracks in-flight and replicated log proposals across the distributed cluster. Uncommitted entries may be overwritten if a leader crashes before quorum.
2. **Storage Engine WAL (`data/<node>/wal.log`)**:
   Only receives entries **after** they have reached strict majority quorum commitment (`ApplyCommitted`). The local storage engine remains strictly decoupled from consensus terms and indexes.

---

## Phase 2 Milestones Implemented

- **M0: Cluster Topologies & Node Configuration (`internal/node/config.go`, `node.go`)**:
  Structured `NodeConfig`, `PeerConfig`, duration unmarshaling (human-readable `"50ms"` or nanoseconds), validation, and standard 3-node cluster layouts.
- **M1: Persistent Raft Storage Engine (`internal/raft/storage.go`)**:
  Durable `raft.log` with length-prefixed CRC32 checksums, atomic term/vote metadata persistence via `metadata.json.tmp`, log truncation, and in-memory cache with sentinel at index 0.
- **M2: Inter-Node Transport Subsystem (`internal/raft/transport.go`, `http_transport.go`, `mock_transport.go`)**:
  Pluggable transport architecture with HTTP REST transport (`POST /raft/vote`, `POST /raft/append`) and an in-memory `MockNetwork` for deterministic testing with network partition simulation (`Isolate`, `Rejoin`, `DropVotes`).
- **M3: Randomized Election Timeouts & Leader Election (`internal/raft/raft.go`)**:
  Randomized jittered election timers, parallel vote solicitation, strict majority vote counting, split-vote resolution, and candidate log up-to-date validation (§5.4.1).
- **M4: Heartbeat Mechanism & Leadership Maintenance (`internal/raft/raft.go`)**:
  Periodic leader heartbeats suppressing follower elections, authority validation, and immediate step-down upon discovering higher terms.
- **M5: Replicated Log Proposals & Catch-Up Replication (`internal/raft/raft.go`)**:
  Log proposal dispatch, conflict detection, fast backtracking via `ConflictIndex` hints, and log overwriting.
- **M6: Majority Commitment & State Machine Application (`internal/raft/raft.go`, `tests/raft_commit_test.go`)**:
  Quorum calculation ($N/2 + 1$), Raft §5.4.2 current-term commitment constraint, sequential asynchronous state machine draining via `applyLoop` and `sync.Cond` advancing `lastApplied` strictly per-entry on callback success, and restart recovery seeding (`InitialLastApplied`).
- **M7: Client Request Integration (`internal/api/raft_handlers.go`, `tests/raft_api_test.go`)**:
  Leader-aware HTTP routing: writes to leader block until quorum commit; writes to followers return `307 Temporary Redirect` with `Location` header and structured JSON redirect metadata; **Quorum-Confirmed Reads (ReadIndex)**: reads verify current leadership with a majority heartbeat round to prevent stale reads from isolated leaders.
- **M8: Failure Handling & Recovery (`tests/raft_failure_test.go`, `tests/raft_hardening_test.go`)**:
  Survives leader isolation (uncommitted entries do not commit without quorum; remaining 2 nodes elect new leader), minority follower partitions (majority continues without pause), node crash & restart with full WAL replay, disk persistence failure rejections, and state-machine apply error handling.
- **M9: End-to-End Cluster Testing & CLI Runner (`cmd/orbit/main.go`, `configs/`, `tests/raft_e2e_test.go`, `tests/multi_process_cluster_test.go`)**:
  CLI runner supporting `--config configs/node-X.json`, full concurrent client stress testing with race detector verification, and actual three-process live cluster failover testing.

---

## API Endpoints

| Method | Endpoint | Request Body | Status Codes | Description |
|---|---|---|---|---|
| `GET` | `/health` | _None_ | `200 OK` / `503 Service Unavailable` | Cluster health, role, leader ID, term, commit/applied index |
| `POST` | `/api/v1/kv` | `{"key":"user:1","value":"Anshik"}` | `201 Created` / `307 Temporary Redirect` | Write KV pair. Automatically redirects to leader if sent to follower |
| `GET` | `/api/v1/kv/{key}` | _None_ | `200 OK` / `307 Temporary Redirect` / `404 Not Found` | Consistent read for key |
| `DELETE` | `/api/v1/kv/{key}` | _None_ | `200 OK` / `307 Temporary Redirect` | Remove key via consensus tombstone |

---

## Running a 3-Node Cluster Locally

Configuration files for local 3-node cluster are provided in `configs/`:

### Terminal 1 (Node A)
```bash
go run ./cmd/orbit --config configs/node-a.json
```
- Client HTTP API: `http://127.0.0.1:2525`
- Raft Transport: `http://127.0.0.1:3525`

### Terminal 2 (Node B)
```bash
go run ./cmd/orbit --config configs/node-b.json
```
- Client HTTP API: `http://127.0.0.1:2526`
- Raft Transport: `http://127.0.0.1:3526`

### Terminal 3 (Node C)
```bash
go run ./cmd/orbit --config configs/node-c.json
```
- Client HTTP API: `http://127.0.0.1:2527`
- Raft Transport: `http://127.0.0.1:3527`

---

## Interacting with the Cluster

### 1. Check Node Health & Discover Leader
```bash
curl http://127.0.0.1:2525/health
```
Response:
```json
{
  "status": "ok",
  "node": "node-a",
  "role": "leader",
  "leader_id": "node-a",
  "term": 1,
  "commit_index": 0,
  "last_applied": 0,
  "storage": "healthy",
  "writable": true,
  "cluster": "raft"
}
```

### 2. Write Data to Leader
```bash
curl -i -X POST http://127.0.0.1:2525/api/v1/kv \
  -H "Content-Type: application/json" \
  -d '{"key":"planet","value":"Saturn"}'
```
Response:
```http
HTTP/1.1 201 Created
Content-Type: application/json

{"key":"planet","value":"Saturn"}
```

### 3. Write Data to Follower (Automatic Leader Redirect)
Send a request to Node B (follower on port 2526):
```bash
curl -i -X POST http://127.0.0.1:2526/api/v1/kv \
  -H "Content-Type: application/json" \
  -d '{"key":"galaxy","value":"Milky Way"}'
```
Response:
```http
HTTP/1.1 307 Temporary Redirect
Location: http://127.0.0.1:2525/api/v1/kv
Content-Type: application/json

{"error":"not_leader","leader_id":"node-a","leader_address":"127.0.0.1:2525"}
```
Using `curl -L` will automatically follow the redirect to the leader and complete the write!

---

## Running the Automated Test Suite

To run the complete test suite including single-node durability tests, multi-process lock tests, Raft elections, replication, commitments, failure scenarios, and concurrent end-to-end tests with the Go race detector:

```bash
go test ./tests/... -v -race -count=1
```
