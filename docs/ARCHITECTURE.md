# ORBIT — System Architecture & Distributed Blueprint

ORBIT is a high-performance, distributed, linearizable, sharded Key-Value database written in **pure Go** using standard library primitives exclusively.

---

## 1. High-Level Architecture Overview

```mermaid
graph TD
    Client[Distributed Client / HTTP Application] -->|HTTP / REST API| IngressNode[Physical Node Ingress]
    
    subgraph RoutingLayer [Routing & Sharding Layer]
        IngressNode --> Ring[Consistent Hash Ring]
        Ring --> Router[Shard Router & Request Forwarder]
        Router --> Resolver[Term-Aware Leader Resolver]
    end

    subgraph MultiRaftEngine [Multi-Raft Consensus Subsystem]
        Resolver -->|Route to Shard 0 Leader| RaftG0[Raft Group 0: Shard 0]
        Resolver -->|Route to Shard 1 Leader| RaftG1[Raft Group 1: Shard 1]
        Resolver -->|Route to Shard 2 Leader| RaftG2[Raft Group 2: Shard 2]
    end

    subgraph StorageLayer [LSM-Tree Storage Engine]
        RaftG0 -->|ApplyCommitted| Engine0[LSM Engine: Shard 0]
        RaftG1 -->|ApplyCommitted| Engine1[LSM Engine: Shard 1]
        RaftG2 -->|ApplyCommitted| Engine2[LSM Engine: Shard 2]
    end

    subgraph ObservabilityLayer [Observability & Event Bus]
        Engine0 -.-> EventHub[EventHub & WebSocket Hub]
        RaftG0 -.-> EventHub
        EventHub --> EventBus[Distributed Event Bus]
    end
```

---

## 2. Core Architectural Pillars

### 2.1 Multi-Raft Partitioning
- The total keyspace is partitioned across $N$ logical shards (default: 3 shards across 5 physical nodes).
- Each shard is governed by an independent Raft consensus group ($2F+1$ replicas per group, default 3 replicas).
- A node hosts replicas for multiple shard groups concurrently without cross-group interference or lock contention.
- Independent consensus lifecycles: leader elections, log compaction, snapshot creation, and network partitions in one group do not stall unrelated groups.

### 2.2 Routing & Request Forwarding
- A consistent hash ring maps arbitrary UTF-8 keys into logical shard IDs (`murmur3`-equivalent standard hash).
- Nodes maintain term-aware leader tracking caches.
- Requests sent to a non-leader physical replica are automatically and transparently forwarded to the active shard leader with bounded retries and loop detection.

### 2.3 LSM-Tree Storage Engine
- **WAL (Write-Ahead Log)**: Sequential, CRC32 checksummed, fsynced on commit. Supports checkpointing and space reclamation (`TruncateBefore`).
- **Memtable**: Thread-safe in-memory skiplist/B-tree with byte size tracking and monotonic sequence stamps.
- **Immutable Memtables**: Rotated queue processed by background flush workers with backpressure limits (`maxImmutableCount`).
- **MANIFEST**: Durable, versioned file atomically recording all active SSTables, levels, bounds, checksums, and flushed WAL offsets.
- **Leveled Compaction**: Multi-way merge compactor transitioning Level 0 tables into non-overlapping Level 1 sorted tables with tombstone purging.
- **SSTables**: Length-prefixed block data, binary-searchable sparse index, and false-positive configurable Bloom filters for sub-millisecond point lookups.

---

## 3. Failure-Recovery Sequence

```mermaid
sequenceDiagram
    autonumber
    participant Client
    participant Follower as Node-B (Follower)
    participant Leader as Node-A (Leader)
    participant Engine as LSM Storage Engine
    participant WAL as Write-Ahead Log

    Client->>Follower: PUT /kv/user:100 {"name":"Alice"}
    Note over Follower: Detects Node-A is leader for Shard 0
    Follower->>Leader: Forward PUT request with X-Forwarded-From
    Leader->>WAL: Append(OperationSet, key, value, term, raftIndex)
    WAL-->>Leader: Fsync durable
    Leader->>Follower: AppendEntries(Entries, Term, CommitIndex)
    Follower-->>Leader: AppendEntriesResponse(Success=true)
    Note over Leader: Quorum commitment reached (2/3 replicas)
    Leader->>Engine: ApplyCommitted(Key, Value, RaftIndex)
    Engine->>Engine: Update Active Memtable
    Leader-->>Follower: HTTP 200 OK
    Follower-->>Client: HTTP 200 OK {"key":"user:100","status":"COMMITTED"}

    Note over Leader: Crash Simulation (SIGKILL)
    Leader-xLeader: Process terminated
    Note over Follower: Heartbeat timeout elapsed (150ms-300ms)
    Follower->>Follower: Advance term, become Candidate
    Follower->>Client: Elects new Leader, serves subsequent reads & writes
```
