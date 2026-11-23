# ORBIT — Distributed Multi-Raft Key-Value Database

ORBIT is a high-performance, crash-resilient, distributed Key-Value database written in Go. It combines an ACID-compliant single-node storage engine (Write-Ahead Logging with streaming recovery and process locking), independent multi-group **Raft consensus**, consistent hash ring sharding, and real-time WebSocket cluster observability.

---

## Architecture Overview

ORBIT partitions the keyspace across multiple shards using consistent hashing. Each shard runs its own independent Raft consensus group replicated across a designated subset of physical nodes. A physical node can host replicas for multiple shards concurrently with strict transport, storage, and consensus isolation.

```
                         CLIENT (HTTP)
                               |
                               v
                     +-------------------+
                     |   SHARD ROUTER    |
                     +-------------------+
                               |
             Consistent Hashing (Virtual Nodes)
                               |
          +--------------------+--------------------+
          |                    |                    |
          v                    v                    v
      +-------+            +-------+            +-------+
      |Shard 0|            |Shard 1|            |Shard 2|
      +-------+            +-------+            +-------+
          |                    |                    |
     Raft Group 0         Raft Group 1         Raft Group 2
 (Node A, B, C)       (Node D, E, A)       (Node B, C, D)
```

### Physical Node Internals

```
+------------------------------------------------------------------------+
|                          PHYSICAL NODE A                               |
|                                                                        |
|  +------------------------------------------------------------------+  |
|  | Client KV API (/kv/:key, /cluster/*, /ws/events)                 |  |
|  | - Transparent Shard Routing & Leader Forwarding                  |  |
|  | - Linearizable Reads via Raft ReadIndex Protocol                 |  |
|  | - Live Event Streaming via RFC 6455 WebSockets                   |  |
|  +------------------------------------------------------------------+  |
|                                                                        |
|  +------------------------------------------------------------------+  |
|  | Multiplexed Group-Aware HTTP Transport (/raft/vote, /raft/append)|  |
|  +------------------------------------------------------------------+  |
|             |                                            |             |
|             v                                            v             |
|  +---------------------+                      +---------------------+  |
|  |   SHARD 0 REPLICA   |                      |   SHARD 1 REPLICA   |  |
|  |   Raft Group 0      |                      |   Raft Group 1      |  |
|  |   Term, Log, State  |                      |   Term, Log, State  |  |
|  |   Isolated WAL/DB   |                      |   Isolated WAL/DB   |  |
|  +---------------------+                      +---------------------+  |
+------------------------------------------------------------------------+
```

### Key Architectural Invariants

1. **Multi-Raft Consensus Independence**:
   Each shard maintains its own term, log entries, commit index, election timer, and leader identity. A failure or term increase in Shard 0 never affects Shard 1.
2. **Storage Isolation**:
   Every shard replica maintains a strictly isolated data directory (`data/<node>/shards/shard-<id>/`), including its own `raft.log`, `metadata.json`, and database `wal.log`.
3. **Transport Multiplexing**:
   A single shared HTTP listener routes consensus RPCs to the appropriate local Raft group using the `group_id` header and payload field.
4. **Linearizable Reads via ReadIndex**:
   Reads query the shard leader, execute the Raft ReadIndex protocol to verify leadership against a strict majority quorum, wait for `lastApplied >= readIndex`, and return linearizable data. Partitioned leaders reject reads if quorum cannot be confirmed.
5. **Deterministic Replica Placement**:
   Replicas are assigned to physical nodes using the formula $R_i = (S \cdot K + i) \pmod N$, ensuring balanced shard distribution across cluster nodes.

---

## Shard-Aware Client KV API

All physical nodes provide the client HTTP API. When a request arrives at any node in the cluster, the router maps the key to its owning shard and forwards mutations or linearizable queries to the shard's active leader.

| Method | Endpoint | Request Body | Description |
|---|---|---|---|
| `PUT` | `/kv/{key}` | `{"value":"Alice"}` | Put key-value pair. Automatically forwards to the shard leader. |
| `GET` | `/kv/{key}` | _None_ | Linearizable read via Raft ReadIndex. Forwards to leader if received on follower. |
| `DELETE` | `/kv/{key}` | _None_ | Delete key. Forwards to leader and replicates tombstone. |
| `GET` | `/cluster/status` | _None_ | Static cluster metadata: cluster ID, epoch, shard count, node list. |
| `GET` | `/cluster/health` | _None_ | Live real-time cluster health: online nodes, healthy shards, leaderless shards, replication health. |
| `GET` | `/cluster/shards` | _None_ | Detailed shard topology, replica assignments, and current leaders. |
| `GET` | `/cluster/nodes` | _None_ | List of physical cluster nodes. |
| `GET` | `/ws/events` | _None_ | RFC 6455 WebSocket stream for live cluster events. |

---

## Live Cluster Events (WebSocket)

Connect to `ws://<node>/ws/events` to receive real-time JSON events:

```json
{
  "event_id": "a1b2c3d4e5f60718",
  "type": "LEADER_CHANGED",
  "timestamp": "2026-11-23T15:00:00Z",
  "node_id": "node-a",
  "shard_id": 1,
  "data": {
    "previous_leader": "node-b",
    "current_leader": "node-a"
  }
}
```

### Event Types
- `NODE_JOINED`: A physical node came online and registered with the cluster.
- `NODE_DISCONNECTED`: A node heartbeat timed out or node stopped.
- `LEADER_CHANGED`: A shard consensus group elected a new leader.
- `TERM_CHANGED`: A Raft group transitioned to a new term.
- `ENTRY_COMMITTED`: A mutation reached quorum commitment and applied to the state machine.
- `SHARD_UPDATED`: Shard replica configuration or topology changed.

---

## Cluster Observability

The `internal/observability` subsystem exposes three levels of real-time operational status built directly from active Raft and node states (without mock or placeholder health values):

1. **Node Status**:
   - `node_id`, `online` / `offline`, `hosted_shard_ids`, `raft_group_count`, `last_observed_activity`.
2. **Shard Status**:
   - `shard_id`, `replica_node_ids`, `current_leader`, `current_term`, `commit_index`, `last_applied_index`, per-replica `replication_progress` (`match_index` and `next_index`).
3. **Cluster Health**:
   - `total_nodes`, `available_nodes`, `total_shards`, `healthy_shards`, `leaderless_shards`, and `replication_health` (`"healthy"`, `"degraded"`, `"unavailable"`).

---

## Running the Automated Test Suite

Run the complete multi-group test suite with the Go race detector enabled across all packages:

```bash
go test -v -race -timeout 180s ./...
```

To run specific test packages:
```bash
# Core consensus & multi-group tests
go test -v -race ./internal/raft/...

# Sharding & consistent hashing tests
go test -v -race ./internal/sharding/...

# Shard routing & forwarder tests
go test -v -race ./internal/router/...

# Cluster observability & WebSocket tests
go test -v -race ./internal/observability/...

# End-to-end integration and API tests
go test -v -race ./tests/...
```
