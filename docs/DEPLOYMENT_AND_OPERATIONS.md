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

## 4. Troubleshooting & Operational Playbooks

### Shard Leader Unreachable
1. Query cluster status: `curl http://localhost:2525/cluster/status`
2. Check follower liveness: `curl http://localhost:2526/ready`
3. If an entire group has lost quorum ($< \lfloor N/2 \rfloor + 1$ alive), restore the partitioned nodes. Raft will re-elect the leader and resume forwarding automatically.

### High Replication Lag
Check lag via `/cluster/shards`:
- If follower lag is growing continuously, inspect disk I/O on the slow follower.
- If log truncation is active, the leader will automatically trigger an `InstallSnapshot` RPC to bring the lagging node up to date in bulk chunks.
