# ORBIT — Core Database & Storage Engine (Section 1)

ORBIT is a high-performance, crash-resilient, persistent Key-Value database storage engine written in Go. Section 1 delivers a completely standalone storage node implementing Write-Ahead Logging (WAL), an in-memory Memtable, automatic crash recovery, lifecycle-coordinated graceful drain, process-level exclusive locking, and an HTTP REST API.

---

## Architecture Overview

```
                      +-------------------+
                      |    HTTP Client    |
                      +---------+---------+
                                |  (GET / POST / DELETE / HEALTH)
                                v
                      +-------------------+
                      |   HTTP API Layer  |
                      | (internal/api)    |
                      +---------+---------+
                                |
                                v
                      +-------------------+
                      |  Database Layer   |
                      | (validation/errs) |
                      +---------+---------+
                                |
                                v
                      +-------------------+
                      |   StorageEngine   |
                      | (internal/storage)|
                      +----+---------+----+
                           |         |
                  (1) Append         | (2) Update State
                           v         v
                 +-----------+     +------------+
                 |    WAL    |     |  Memtable  |
                 | (on disk) |     | (in-memory)|
                 +-----------+     +------------+
```

### The Durability Invariant
Before modifying the in-memory state (`Memtable`), ORBIT **first** writes the mutation record to the Write-Ahead Log (`wal.log`) and explicitly calls `fsync()` to flush to persistent disk. If the process is terminated or loses power, upon restarting it replays the WAL from disk to restore the exact state.

---

## Core Features Implemented

1. **Memtable (`internal/storage/memtable.go`)**:
   - In-memory key-value store using Go `map[string]string`.
   - Thread-safe read/write operations using `sync.RWMutex` (concurrent non-blocking reads).
   - O(1) average lookup and deletion performance.

2. **Write-Ahead Log (`internal/wal/wal.go`, `record.go`, `lock_*.go`)**:
   - **Exclusive Process-Level Lock**: Uses OS-level file locking (`<walPath>.lock` via `LockFileEx` on Windows, `flock` on Unix) preventing multiple processes from concurrently mutating the same data directory.
   - **Schema Versioning & Length-Prefixed Checksums**: Versioned records (`version: 1`) with unambiguous length-prefixed **CRC32-IEEE checksums** eliminating field boundary ambiguity.
   - **Committed Sequence Counter & Overflow Check**: Mutex-protected counter that advances strictly after a successful `fsync`, protected against `MaxUint64` overflow.
   - **Streaming Recovery (O(1) Memory)**: Records stream one-by-one via `HandlerFunc`, eliminating startup memory spikes and redundant scans.
   - **Failure Poisoning**: Any disk write or sync failure marks the log poisoned, halting further writes into an uncertain state.
   - **Strict Invariants**: Enforces contiguous $1 \rightarrow 2 \rightarrow 3$ sequences, rejects trailing JSON tokens on record lines, and rejects blank lines.
   - **Truncation of Incomplete Writes**: Truncates partial trailing writes from crash mid-write back to the clean record boundary.
   - Deletions recorded as tombstones (`OperationDelete`).

3. **Storage Engine & Raft Integration Boundary (`internal/storage/engine.go`)**:
   - **Consensus Application Boundary**: Exposes `ApplyCommitted(CommittedCommand)` separating consensus metadata (**Raft log index**, **Raft term**) from local physical **WAL sequence numbers**.
   - **Durable Progress & Gap Enforcement**: Persists `RaftIndex` and `RaftTerm` into WAL records so `lastAppliedIndex` survives restarts. Enforces strictly contiguous sequential application (`lastAppliedIndex + 1`), rejecting index gaps (`ErrRaftIndexGap`).
   - **Commit Idempotency**: Duplicate consensus entries are safe, idempotent no-ops.
   - Coordinates WAL and Memtable mutations atomically under `writeMu`.
   - Self-healing startup: single-pass streaming WAL replay on boot reconstructs the Memtable and restores consensus progress.
   - Shutdown tracking: returns `ErrClosed` if mutations are attempted after shutdown.

4. **Database Operations Layer (`internal/database/`)**:
   - Exposes clean `Set`, `Get`, `Delete`, `ApplyCommitted`, `Health` methods.
   - **Standard KV Semantics**: Empty values (`""`) are fully valid; rejects only blank keys.
   - **Lifecycle Coordination**: Uses `sync.RWMutex` so `Close()` waits for all active in-flight operations to complete before closing the storage engine.
   - **Sticky Shutdown Errors**: Preserves close failures across retries.
   - Sentinel errors (`ErrKeyNotFound`, `ErrEmptyKey`, `ErrClosed`).
   - Defines the `StorageEngine` interface, creating a decoupled contract for Section 2 (Raft consensus) integration.

5. **HTTP REST API (`internal/api/`, `cmd/orbit/main.go`)**:
   - Native Go 1.22+ routing with method matching and path parameters.
   - Strict input validation: 1MB body limit (returns `413 Request Entity Too Large`), rejection of unknown JSON fields and trailing payloads.
   - **Phased Graceful Shutdown**: Drains active requests within 5s, forcibly terminates lingering sockets on timeout with `server.Close()`, and coordinates cleanup without `log.Fatalf` in goroutines.
   - **Diagnostic Health Check (`/health`)**: Reports process status, storage engine health (detecting poisoned WALs), write availability, and cluster role.

---

## API Endpoints

| Method | Endpoint | Request Body | Success Code | Description |
|---|---|---|---|---|
| `GET` | `/health` | _None_ | `200 OK` | Diagnostic health & readiness check |
| `POST` | `/api/v1/kv` | `{"key": "user:1", "value": "Anshik"}` | `201 Created` | Store a key-value pair (empty values permitted) |
| `GET` | `/api/v1/kv/{key}` | _None_ | `200 OK` | Retrieve value for key |
| `DELETE` | `/api/v1/kv/{key}` | _None_ | `200 OK` | Delete key (tombstone) |

---

## Running and Verifying

### 1. Run All Automated Tests
Includes unit tests, durability restart simulations, monotonic sequence checks, concurrency stress tests, Raft apply and gap tests, exclusive process locking tests, and partial write crash tests with race detection:
```bash
go test ./tests/... -v -race
```

### 2. Start the ORBIT Server
```bash
go run ./cmd/orbit
```
The server will start listening on `http://localhost:2525`.

### 3. Test HTTP Operations

#### Health check:
```bash
curl http://localhost:2525/health
```
Response:
```json
{
  "status": "ok",
  "node": "node-a",
  "storage": "healthy",
  "writable": true,
  "cluster": "standalone"
}
```

#### Store a key-value pair (SET):
```bash
curl -X POST http://localhost:2525/api/v1/kv \
  -H "Content-Type: application/json" \
  -d '{"key":"user:1","value":"Anshik"}'
```

#### Retrieve a value (GET):
```bash
curl http://localhost:2525/api/v1/kv/user:1
```

#### Delete a key (DELETE):
```bash
curl -X DELETE http://localhost:2525/api/v1/kv/user:1
```

#### Verify Crash Durability:
1. Insert keys via `POST /api/v1/kv`.
2. Stop ORBIT (`Ctrl + C`).
3. Inspect `data/node-a/wal.log` to see the durable entries with checksums and schema versions.
4. Restart ORBIT with `go run ./cmd/orbit`.
5. Retrieve the keys via `GET /api/v1/kv/{key}` — data is recovered from the WAL!
