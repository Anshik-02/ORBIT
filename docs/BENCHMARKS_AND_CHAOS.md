# ORBIT — Benchmarks & Chaos Lab Guide

ORBIT ships with two specialized built-in engineering tools: `orbit-bench` for performance profiling and `orbit-chaos` for deterministic distributed fault injection.

---

## 1. Benchmark Suite (`orbit-bench`)

The benchmark tool profiles raw storage throughput, database latency, and multi-node cluster operations.

### Running Storage Engine Benchmarks
```bash
# Benchmark standalone LSM engine with 8 workers for 10 seconds (80% reads / 20% writes)
go run ./cmd/orbit-bench -target storage -concurrency 8 -duration 10s -read-ratio 0.8 -keys 10000

# Benchmark under concurrent background compaction
go run ./cmd/orbit-bench -target storage -scenario during_compaction -concurrency 4 -duration 5s

# Benchmark under continuous snapshot exports
go run ./cmd/orbit-bench -target storage -scenario during_snapshots -concurrency 4 -duration 5s
```

### Running Cluster HTTP API Benchmarks
```bash
go run ./cmd/orbit-bench -target cluster -url http://localhost:2525 -concurrency 16 -duration 30s -output results/run-latest.json
```

### Comparing Benchmark Runs
To detect regressions against a baseline run:
```bash
go run ./cmd/orbit-bench -target storage -compare results/run-baseline.json
```
If throughput drops $> 10\%$ or p99 latency rises $> 15\%$, a regression alert is raised.

---

## 2. Chaos Lab (`orbit-chaos`)

The Chaos Lab injects real distributed failure scenarios into nodes, networks, and storage subsystems to test resilience:

| Fault Type | Description |
| --- | --- |
| `partition` | Isolates target node from peers using `ChaosTransport` |
| `drop` | Injects randomized packet loss (configurable 0% to 100%) |
| `timeout` | Forces RPC deadlines to expire simulating network blackouts |
| `wal-crash` | Injects a corrupt/incomplete record at the WAL tail |
| `flush-crash` | Leaves orphan uncommitted `.tmp` SSTables on disk |
| `compaction-crash` | Leaves partially merged compaction files on disk |
| `corrupt-sstable` | Injects byte-level bitrot into an SSTable file |
| `corrupt-snapshot` | Injects CRC32 corruption into a state-machine snapshot |

### Executing a Chaos Experiment
```bash
go run ./cmd/orbit-chaos -action run -fault wal-crash -target data/node-a/wal.log -output reports/chaos-exp-1.json
```
Reports record the failure event timeline, exact recovery time in milliseconds, and verify state consistency post-recovery.
