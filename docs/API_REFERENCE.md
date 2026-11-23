# ORBIT — HTTP REST API & Interface Reference

ORBIT provides versioned, standard HTTP endpoints under `/` and `/api/v1/`.

---

## 1. Key-Value Endpoints

### `PUT /api/v1/kv/{key}`
Stores a key-value mapping in the cluster.
- **Headers**:
  - `Content-Type: application/json`
  - `X-Idempotency-Key: <uuid>` (Optional: enables deduplication replay on retries)
  - `Authorization: Bearer <token>` (When security auth is enabled)
- **Request Body**:
  ```json
  {
    "value": "encoded payload or string"
  }
  ```
- **Response**: `200 OK`
  ```json
  {
    "key": "user:100",
    "value": "encoded payload or string",
    "status": "COMMITTED",
    "request_id": "req-xyz"
  }
  ```

### `GET /api/v1/kv/{key}`
Retrieves the linearizable value for a key.
- **Response**: `200 OK`
  ```json
  {
    "key": "user:100",
    "value": "encoded payload or string",
    "request_id": "req-xyz"
  }
  ```
- **Error**: `404 Not Found` if missing, `503 Service Unavailable` if shard leader unresolvable.

### `DELETE /api/v1/kv/{key}`
Deletes a key by applying a durable tombstone.
- **Response**: `200 OK`

---

## 2. Batch & Range Endpoints

### `POST /api/v1/kv/batch`
Executes batched multi-key writes or reads across shards with per-operation status reporting. Each operation is routed to its respective shard leader and executed independently, returning an array of per-key results with explicit status (`COMMITTED` or `ERROR` with error detail).
- **Batch Write Request**:
  ```json
  {
    "ops": [
      {"type": "PUT", "key": "k1", "value": "v1"},
      {"type": "DELETE", "key": "k2"}
    ]
  }
  ```
- **Batch Read Request**:
  ```json
  {
    "keys": ["k1", "k2", "k3"]
  }
  ```

### `GET /api/v1/kv/scan?start={key}&end={key}&limit={n}&cursor={c}`
Executes a sorted paginated range scan across storage tiers.
- **Response**:
  ```json
  {
    "items": [
      {"key": "user:001", "value": "..."},
      {"key": "user:002", "value": "..."}
    ],
    "has_more": true,
    "next_cursor": "user:003"
  }
  ```

### `GET /api/v1/kv/prefix?prefix={p}&limit={n}`
Scans keys matching a prefix.

---

## 3. Observability & Cluster Health

- `GET /metrics` & `GET /api/v1/metrics`: Returns JSON snapshot of throughput (RPS, reads/sec, writes/sec), latency percentiles (p50, p95, p99, avg in ms), error rates, and memory allocations.
- `GET /live`: Process liveness probe (`200 OK`).
- `GET /ready`: Shard & storage engine readiness probe (`200 OK` or `503 Service Unavailable`).
- `GET /cluster/status`: Topology status, hosted shard replicas, and leader states.
- `WS /ws/events`: RFC 6455 compliant WebSocket stream delivering live cluster events (leader changes, splits, membership changes). Supports reconnection replay via `?since={seq}`.
