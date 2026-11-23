# ORBIT — Multi-Raft Consensus Subsystem

ORBIT implements an audited, multi-group Raft consensus engine following the Raft specification with extensions for multi-shard concurrency, batch replication, term-aware leader tracking, and state-machine snapshot installation.

---

## 1. Multi-Raft Isolation

Each shard group operates as an autonomous state machine:
- **Group Isolation**: Every Raft RPC carries a `GroupID`. Network transports multiplex messages without cross-shard serialization bottlenecks.
- **Independent Elections**: Election timeouts are randomized per group (150ms to 300ms) to prevent split-vote storms across shards.
- **Failure Resilience**: Complete partition or leader loss in Shard 0 has zero effect on Shard 1 and Shard 2 write processing.

---

## 2. Batch Replication & Lag Measurement

- **Configurable Batching**: Controlled by `MaxBatchEntries` (default: 1000) to prevent oversized network payloads and GC spikes during replication catch-up.
- **Replication Lag Measurement**: Exposes `ReplicationLag(followerID)` measuring the exact entry difference between the leader's commit watermark and the follower's `matchIndex`.

---

## 3. Snapshot Protocol & Log Compaction

When a shard's Raft log exceeds the compaction threshold:
1. **Atomic Snapshot Capture**: The engine freezes state atomically and exports `lastAppliedIndex`, `lastAppliedTerm`, and the state machine data with IEEE CRC32 checksum verification.
2. **Safe Log Truncation**: `CompactLog(lastIncludedIndex, lastIncludedTerm)` preserves the sentinel index while reclaiming disk space used by compacted log entries.
3. **InstallSnapshot RPC**:
   - Transferred in chunks with total byte offsets, sequential chunk ordering, and SHA/CRC verification.
   - Lagging followers install snapshots atomically. If an in-progress apply loop is running, synchronization guards guarantee the restored snapshot cannot be overwritten by stale in-flight applies.
   - Incomplete snapshot assemblies from disconnected leaders are cleanly garbage-collected upon session timeout.
