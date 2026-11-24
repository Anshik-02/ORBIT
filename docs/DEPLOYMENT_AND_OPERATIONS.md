# ORBIT — Deployment, Operations & Production Guide

This guide covers deployment, cluster orchestration, monitoring, backup procedures, and operational troubleshooting for ORBIT.

---

## 1. Quickstart with Docker Compose

Deploy a complete five-node, three-shard cluster in seconds:

```bash
docker compose up -d
```

Check the status of all five nodes:
```bash
docker compose ps
curl http://localhost:2525/live
curl http://localhost:2525/cluster/status
```

---

## 2. Configuration & Environment Overrides

Every configuration parameter can be customized through JSON configuration files or overridden via environment variables:

| Environment Variable | Description | Default |
| --- | --- | --- |
| `ORBIT_CONFIG_PATH` | Path to node JSON configuration | `configs/node-a.json` |
| `ORBIT_CLUSTER_CONFIG` | Path to cluster topology JSON | `configs/cluster.json` |
| `ORBIT_NODE_ID` | Node physical identity override | Config value |
| `ORBIT_ADDRESS` / `ORBIT_HTTP_ADDR` | HTTP client listening address | `127.0.0.1:2525` |
| `ORBIT_RAFT_ADDRESS` / `ORBIT_RAFT_ADDR` | Raft inter-node RPC address | `127.0.0.1:3525` |
| `ORBIT_DATA_DIR` | Base storage directory for WAL and SSTables | `data/{node-id}` |

---

## 3. Backup & Disaster Recovery Procedures

### 3.1 Creating Backups
To create an atomic, point-in-time backup of a node's data directory:
```bash
./bin/orbit-admin -cmd backup -data data/node-a -backup /var/backups/orbit
```
This safely copies all active SSTables, MANIFEST, and WAL log files, validating their CRC32 checksums.

### 3.2 Restoring from Backup
To restore data into a clean node data directory:
```bash
./bin/orbit-admin -cmd restore -backup /var/backups/orbit/backup-12345678 -data data/node-a
```

---

## 4. Operational Runbook & Incident Playbooks

### 4.1 Leader Crashes
- **Symptoms**: Writes to the shard return transient 503 or 508 loop errors while elections occur.
- **Action**: Surviving followers detect election timeout (150-300ms) and initiate a new term election. The new leader commits in its term. Forwarder routes update automatically via `X-Orbit-Forward-Path` and LeaderTracker cache refresh. No manual action required.

### 4.2 Node Goes Offline
- **Symptoms**: `/ready` endpoint returns 503 or node process stops.
- **Action**: Verify process logs. With 5 nodes and replication factor 3 across 3 shards, any single node loss leaves all shards with a 2-node operational majority. Restart the node via systemd or docker: `docker compose restart node-x`. The node rejoins as follower and catches up logs sequentially.

### 4.3 Shard Loses Quorum
- **Symptoms**: Writes to keys on that shard time out with HTTP 504 Gateway Timeout; ReadIndex requests return `ErrQuorumLost`.
- **Action**: Unaffected shards continue operating independently without interruption. Restore network connectivity or restart at least 1 failed follower replica to restore majority (2 of 3). Once restored, the shard resumes consensus immediately.

### 4.4 Follower Falls Behind / High Replication Lag
- **Symptoms**: `ReplicationLag()` on shard leader shows increasing matchIndex delta.
- **Action**: If follower was disconnected while WAL logs were compacted, leader triggers `InstallSnapshot` RPC in streaming chunks. Wait for snapshot download and verification. Follower replaces local state, installs snapshot, and resumes AppendEntries.

### 4.5 Disk Becomes Unavailable
- **Symptoms**: WAL append errors or SSTable I/O failures trigger error logs.
- **Action**: State machine safely stops applying unpersisted writes to prevent silent corruption. Re-mount or fix disk volume, then restart node. Engine validates MANIFEST checksums and replays intact WAL records.

### 4.6 Migration Gets Stuck
- **Symptoms**: Migration task state remains in `TransferringSnapshot` or `CatchingUp` beyond SLA timeout.
- **Action**: Query `/cluster/status` to inspect target replica health. The coordinator supports cancellation/rollback via `UpdateTaskState(taskID, MigrationStateFailed)`. Rebalance plan can be re-executed idempotently.

### 4.7 Node Needs Permanent Replacement
- **Symptoms**: Hardware failure requires decommissioning an old node and adding a new server.
- **Action**:
  1. Boot new replacement node with fresh data directory.
  2. Register new node as learner via `AddNode(newNodeID)` to avoid altering voter quorum prematurely.
  3. Wait for learner to replicate consensus logs and install shard snapshots.
  4. Promote learner to full voter via atomic membership change.
  5. Remove decommissioned node via `RemoveNode(oldNodeID)`.

### 4.8 Backup Restoration is Required
- **Symptoms**: Complete cluster data recovery from disaster.
- **Action**:
  1. Stop all node processes: `docker compose down`.
  2. Restore backup into each node data directory:
     `./orbit-admin -cmd restore -backup /var/backups/orbit/backup-<timestamp> -data data/<node-id>`
  3. Start cluster: `docker compose up -d`.
  4. Verify data via `orbit-admin -cmd verify -data data/<node-id>`.

### 4.9 Cluster Cannot Elect a Leader
- **Symptoms**: Split vote or symmetric tie across network partition.
- **Action**: Raft incorporates randomized election timeouts (150ms to 300ms) to ensure split ties break within one election window. If persistent, check for network partition isolating nodes from each other, verify firewall ports 3525-3529, and inspect mTLS certificate expiry.
