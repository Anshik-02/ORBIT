export type NodeId = 'node-a' | 'node-b' | 'node-c' | 'node-d' | 'node-e' | string;
export type NodeHealthStatus = 'online' | 'offline' | 'degraded';
export type RaftRole = 'Leader' | 'Follower';

export interface HostedShardInfo {
  shardId: number;
  shardName: string;
  role: RaftRole;
  term: number;
  commitIndex: number;
  appliedIndex: number;
  color: string;
}

export interface PhysicalNode {
  id: string;
  name: string;
  status: NodeHealthStatus;
  online: boolean;
  httpAddress: string;
  raftAddress: string;
  hostedShards: HostedShardInfo[];
  raftGroupCount: number;
  leaderGroupCount: number;
  followerGroupCount: number;
  lastObservedActivity: string;
  lastHeartbeatMs: number;
  position3D: [number, number, number];
  position2D: { x: number; y: number };
}

export type NetworkLinkType = 'physical' | 'raft';
export type NetworkLinkStatus = 'healthy' | 'idle' | 'degraded' | 'disconnected';

export interface NetworkLink {
  id: string;
  source: string;
  destination: string;
  type: NetworkLinkType;
  shardId?: number;
  status: NetworkLinkStatus;
  latencyMs?: number;
  lastContact: string;
  activeTraffic: boolean;
}

export type EventCategory = 'NETWORK' | 'RAFT' | 'STORAGE' | 'CLUSTER';

export interface TopologyEvent {
  id: string;
  timestamp: string;
  nodeId: string;
  type: string;
  category: EventCategory;
  description: string;
  level: 'info' | 'success' | 'warn' | 'error';
}

export interface ClusterSummaryData {
  totalNodes: number;
  availableNodes: number;
  offlineNodes: number;
  totalShards: number;
  healthyShards: number;
  leaderlessShards: number;
  replicationHealth: 'healthy' | 'degraded' | 'unavailable';
}

export type NodeFilterOption = 'all' | 'online' | 'offline' | 'leaders' | 'followers';
export type ShardFilterOption = 'all' | '0' | '1' | '2';
export type ViewMode = '3d' | '2d';
export type NetworkMode = 'physical' | 'raft';
export type ConnectionState = 'connected' | 'reconnecting' | 'disconnected';
