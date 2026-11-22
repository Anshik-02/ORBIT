# ORBIT — Core Database & Storage Engine (Section 1)

ORBIT is a high-performance, crash-resilient, persistent Key-Value database storage engine written in Go. Section 1 delivers a completely standalone storage node implementing Write-Ahead Logging (WAL), an in-memory Memtable, automatic crash recovery, and an HTTP REST API.

---

## Architecture Overview

```
                      +-------------------+
                      |    HTTP Client    |
                      +---------+---------+
                                |  (GET / POST / DELETE)
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

2. **Write-Ahead Log (`internal/wal/wal.go`, `record.go`)**:
   - Append-only log file format with JSON Lines (`.log`).
   - Monotonically increasing atomic sequence numbering (`sync/atomic`).
   - Synchronous flush and `fsync` on every mutation to guarantee crash durability.
   - Deletions recorded as tombstones (`OperationDelete`).
   - Resilient crash recovery handling: ignores incomplete/truncated final writes while catching corrupted entries.

3. **Storage Engine & Crash Recovery (`internal/storage/engine.go`)**:
   - Coordinates WAL and Memtable operations seamlessly.
   - Self-healing startup: automatically replays the WAL on boot to reconstruct the Memtable before accepting traffic.

4. **Database Operations Layer (`internal/database/`)**:
   - Exposes clean `Set`, `Get`, `Delete` methods.
   - Input validation (rejects blank keys/values).
   - Idiomatic Go sentinel errors (`ErrKeyNotFound`, `ErrEmptyKey`, `ErrEmptyValue`).
   - Defines the `StorageEngine` interface, creating a decoupled contract for Section 2 (Raft consensus) integration.

5. **HTTP REST API (`internal/api/`, `cmd/orbit/main.go`)**:
   - Native Go 1.22+ routing with method matching and path parameters.
   - Graceful shutdown listening for `SIGINT` / `SIGTERM` signals.
   - Health check endpoint at `/health`.

---

## API Endpoints

| Method | Endpoint | Request Body | Success Code | Description |
|---|---|---|---|---|
| `GET` | `/health` | _None_ | `200 OK` | Node health & status check |
| `POST` | `/api/v1/kv` | `{"key": "user:1", "value": "Anshik"}` | `201 Created` | Store a key-value pair |
| `GET` | `/api/v1/kv/{key}` | _None_ | `200 OK` | Retrieve value for key |
| `DELETE` | `/api/v1/kv/{key}` | _None_ | `200 OK` | Delete key (tombstone) |

---

## Running and Verifying

### 1. Run All Automated Tests
Includes unit tests, durability restart simulations, monotonic sequence checks, and partial write crash tests with race detection:
```bash
go test ./tests/... -v
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
3. Inspect `data/node-a/wal.log` to see the durable entries.
4. Restart ORBIT with `go run ./cmd/orbit`.
5. Retrieve the keys via `GET /api/v1/kv/{key}` — data is recovered from the WAL!
