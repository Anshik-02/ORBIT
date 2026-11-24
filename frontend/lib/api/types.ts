// ORBIT API TypeScript Contracts matching backend structs in internal/api, internal/observability, internal/sharding

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

export interface ReplicationProgress {
  match_index: number;
  next_index: number;
  state: string;
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

export interface ClusterEvent {
  event_id: string;
  sequence: number;
  type: string;
  timestamp: string;
  node_id: string;
  shard_id?: number;
  data?: Record<string, any>;
}

export interface ClusterEventsResponse {
  events: ClusterEvent[];
  total: number;
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
  timestamp: string;
}

export interface ChaosExperimentConfig {
  faultType: 'wal-crash' | 'flush-crash' | 'compaction-crash' | 'corrupt-sstable' | 'corrupt-snapshot' | 'partition';
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
