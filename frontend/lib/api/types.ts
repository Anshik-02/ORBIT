// ORBIT API TypeScript Contracts matching backend structs in internal/api, internal/observability, internal/sharding

export interface RequestOptions {
  timeoutMs?: number;
  signal?: AbortSignal;
  headers?: Record<string, string>;
  idempotencyKey?: string;
}

export interface ClusterStatusResponse {
  cluster_id: string;
  epoch: number;
  shard_count: number;
  node_count: number;
  nodes: string[];
}

export interface NodeStatus {
  node_id: string;
  status: 'online' | 'offline';
  online: boolean;
  hosted_shard_ids: number[];
  raft_group_count: number;
  last_observed_activity: string;
}

export interface NodeDetailResponse {
  node_id: string;
  status: 'online' | 'offline';
  online: boolean;
  http_address: string;
  raft_address: string;
  data_dir: string;
  hosted_shard_ids: number[];
  raft_groups: {
    group_id: string;
    shard_id: number;
    role: 'Leader' | 'Follower' | 'Candidate';
    term: number;
    commit_index: number;
    last_applied: number;
    leader_id: string;
  }[];
  peers: {
    id: string;
    address: string;
    raft_address: string;
    online: boolean;
    latency_ms?: number;
  }[];
  metrics?: {
    cpu_percent: number;
    memory_mb: number;
    goroutines: number;
    uptime_seconds: number;
  };
}

export interface ReplicationProgress {
  match_index: number;
  next_index: number;
  state?: string;
  last_contact: string;
}

export interface ShardStatus {
  shard_id: number;
  state: string;
  replica_node_ids: string[];
  current_leader: string;
  current_term: number;
  commit_index: number;
  last_applied_index: number;
  replication_progress?: Record<string, ReplicationProgress>;
  replication_lag?: Record<string, number>;
  available_replicas: string[];
  quorum_available: boolean;
  is_leader: boolean;
  healthy: boolean;
  last_communication: string;
}

export interface ClusterHealthResponse {
  cluster_id: string;
  timestamp: string;
  total_nodes: number;
  available_nodes: number;
  total_shards: number;
  healthy_shards: number;
  leaderless_shards: number;
  replication_health: 'healthy' | 'degraded' | 'unavailable';
  nodes: Record<string, NodeStatus>;
  shards: Record<number, ShardStatus>;
}

export interface ShardDetail {
  id: number;
  replicas: string[];
  leader?: string;
}

export interface ClusterShardsResponse {
  shards: ShardDetail[];
}

export interface ClusterNodesResponse {
  nodes: string[];
}

// Raft Consensus Telemetry Contracts
export interface RaftPeerProgress {
  match_index: number;
  next_index: number;
  last_contact?: string;
  lag: number;
}

export interface RaftGroupDetail {
  group_id: string;
  shard_id: number;
  role: 'Leader' | 'Follower' | 'Candidate';
  term: number;
  leader_id: string;
  commit_index: number;
  last_applied: number;
  last_log_index: number;
  peers: string[];
  replication_progress?: Record<string, RaftPeerProgress>;
  replication_lag?: Record<string, number>;
}

export interface RaftOverviewResponse {
  cluster_id: string;
  total_groups: number;
  leader_count: number;
  groups: RaftGroupDetail[];
}

export interface RaftLogEntry {
  index: number;
  term: number;
  type: 'KV_PUT' | 'KV_DELETE' | 'CONF_CHANGE' | 'NO_OP';
  key?: string;
  value?: string;
  committed: boolean;
  applied: boolean;
  timestamp?: string;
}

export interface RaftLogsResponse {
  group_id: string;
  shard_id: number;
  entries: RaftLogEntry[];
  total_entries: number;
}

export interface RaftElectionEvent {
  event_id: string;
  timestamp: string;
  shard_id: number;
  group_id: string;
  term: number;
  type: 'ELECTION_STARTED' | 'VOTE_REQUESTED' | 'VOTE_GRANTED' | 'LEADER_ELECTED' | 'LEADER_LOST';
  candidate_id?: string;
  voted_for?: string;
  leader_id?: string;
  votes_received?: number;
  quorum_size?: number;
}

// Storage Engine Telemetry Contracts
export interface MemtableStats {
  id: number;
  byte_size: number;
  entry_count: number;
  is_immutable: boolean;
  created_at?: string;
}

export interface SSTableStats {
  id: number;
  level: number;
  file_name: string;
  file_size: number;
  record_count: number;
  min_key: string;
  max_key: string;
  has_bloom_filter: boolean;
  filter_false_positive_rate?: number;
}

export interface CompactionStats {
  is_running: boolean;
  compaction_count: number;
  total_bytes_compacted: number;
  source_level?: number;
  destination_level?: number;
  input_files?: string[];
  output_files?: string[];
  progress_percent?: number;
  last_compaction_time?: string;
}

export interface StorageStatsResponse {
  cluster_id: string;
  node_id: string;
  active_memtable: MemtableStats;
  immutable_memtables: MemtableStats[];
  immutable_count: number;
  wal_size_bytes: number;
  wal_record_count: number;
  wal_path: string;
  sstables: SSTableStats[];
  sstables_by_level: Record<number, number>;
  total_sstable_bytes: number;
  compaction: CompactionStats;
  write_amplification: number;
  read_amplification: number;
  space_amplification: number;
}

// Key-Value Contracts
export interface GetKVResponse {
  key: string;
  value: string;
  shard_id: number;
}

export interface DeleteKVResponse {
  key: string;
  deleted: boolean;
  shard_id: number;
}

export interface SetKVRequest {
  value: string;
}

export interface SetKVResponse {
  key: string;
  value: string;
  shard_id: number;
  status: string;
}

export interface BatchOp {
  operation: 'SET' | 'DELETE';
  key: string;
  value?: string;
}

export interface BatchOpResult {
  key: string;
  status: 'COMMITTED' | 'ERROR';
  error?: string;
}

export interface BatchWriteResponse {
  results: BatchOpResult[];
  request_id?: string;
}

export interface BatchReadRequest {
  keys: string[];
}

export interface BatchReadItem {
  key: string;
  value?: string;
  found: boolean;
  shard_id?: number;
}

export interface BatchReadResponse {
  items: BatchReadItem[];
  request_id?: string;
}

export interface KVPair {
  key: string;
  value: string;
}

export interface ScanResponse {
  items: KVPair[];
  has_more: boolean;
  next_cursor: string;
  request_id?: string;
}

// Events Telemetry Contracts
export interface ClusterEvent {
  event_id: string;
  sequence: number;
  type: string;
  timestamp: string;
  node_id: string;
  shard_id?: number;
  severity?: 'INFO' | 'WARN' | 'ERROR' | 'CRITICAL';
  data?: Record<string, any>;
}

export interface ClusterEventsResponse {
  events: ClusterEvent[];
  total: number;
}

// Observability & Metrics
export interface ClusterMetricsResponse {
  timestamp: string;
  uptime_seconds: number;
  total_requests: number;
  read_requests: number;
  write_requests: number;
  error_requests: number;
  requests_per_sec: number;
  reads_per_sec: number;
  writes_per_sec: number;
  error_rate: number;
  latency: {
    p50_ms: number;
    p95_ms: number;
    p99_ms: number;
    avg_ms: number;
  };
  memory_alloc_mb: number;
  memory_sys_mb: number;
  num_goroutines: number;
  num_gc: number;
}

export interface StorageMetrics {
  activeMemtableBytes: number;
  immutableMemtablesBytes: number;
  immutableMemtablesCount: number;
  walSizeBytes: number;
  walRecordCount: number;
  sstableCountL0: number;
  sstableCountL1: number;
  sstableCountL2: number;
  sstableTotalBytes: number;
  compactionCount: number;
  compactionBytesTotal: number;
  writeAmplification: number;
  readAmplification: number;
  spaceAmplification: number;
}

// Benchmark Suite Contracts
export interface BenchmarkConfig {
  target: 'storage' | 'database' | 'cluster';
  scenario: 'standard' | 'during_compaction' | 'during_snapshots';
  concurrency: number;
  durationSeconds: number;
  numKeys: number;
  valueSize: number;
  readRatio: number;
  url?: string;
}

export interface BenchmarkResult {
  id?: string;
  name: string;
  target: string;
  scenario: string;
  concurrency: number;
  durationSeconds: number;
  totalOperations: number;
  successfulOperations: number;
  failedOperations: number;
  throughputOpsPerSec: number;
  readThroughputOpsPerSec: number;
  writeThroughputOpsPerSec: number;
  latencyAvgMs: number;
  latencyP50Ms: number;
  latencyP95Ms: number;
  latencyP99Ms: number;
  latencyMaxMs?: number;
  timestamp: string;
}

// Chaos Engineering Lab Contracts
export interface ChaosExperimentConfig {
  faultType: 'wal-crash' | 'flush-crash' | 'compaction-crash' | 'corrupt-sstable' | 'corrupt-snapshot' | 'partition' | 'leader-kill' | 'node-restart';
  target: string;
  durationSeconds: number;
}

export interface ChaosTimelineEntry {
  timestamp: string;
  phase: string;
  description: string;
  details?: Record<string, any>;
}

export interface ChaosExperimentReport {
  id: string;
  name: string;
  faultType: string;
  target: string;
  durationSeconds: number;
  state: 'INITIALIZED' | 'FAULT_INJECTED' | 'QUORUM_TESTED' | 'HEALED' | 'VERIFIED' | 'COMPLETED' | 'FAILED';
  quorumMaintained: boolean;
  dataLossDetected: boolean;
  recoveryDurationMs: number;
  timeline: ChaosTimelineEntry[];
  summary: string;
  timestamp: string;
}

// Configuration & Management Contracts
export interface PeerConfig {
  id: string;
  address: string;
  raft_address: string;
}

export interface NodeConfigFile {
  id: string;
  address: string;
  raft_address: string;
  data_dir: string;
  heartbeat_interval: string;
  election_timeout_min: string;
  election_timeout_max: string;
  peers: PeerConfig[];
}

export interface ClusterConfigFile {
  cluster_id: string;
  epoch: number;
  shard_count: number;
  replication_factor: number;
  physical_nodes: string[];
  shards: {
    id: number;
    replicas: string[];
  }[];
}

export interface SettingsSaveResult {
  ok: boolean;
  restartRequired: boolean;
  message: string;
  errors?: string[];
}
