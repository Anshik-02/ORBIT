/**
 * ORBIT API Endpoint Definitions
 * Central registry of all backend REST and WebSocket routes.
 * Supports standard routes and versioned /api/v1 prefixes.
 */

export const API_PREFIX_V1 = '/api/v1';

export const ENDPOINTS = {
  // Cluster Topology & Telemetry
  CLUSTER_STATUS: '/cluster/status',
  CLUSTER_HEALTH: '/cluster/health',
  CLUSTER_SHARDS: '/cluster/shards',
  CLUSTER_NODES: '/cluster/nodes',
  NODE_DETAIL: (nodeId: string) => `/cluster/nodes/${encodeURIComponent(nodeId)}`,

  // Probes & Observability
  LIVENESS: '/live',
  READINESS: '/ready',
  METRICS: '/metrics',

  // Raft Consensus & Log Observability
  RAFT_OVERVIEW: '/cluster/raft',
  RAFT_GROUP: (groupId: string | number) => `/cluster/raft/${groupId}`,
  RAFT_LOGS: (groupId: string | number) => `/cluster/raft/${groupId}/logs`,

  // LSM Storage Engine Observability
  STORAGE_STATS: '/cluster/storage',
  STORAGE_COMPACTION: '/cluster/storage/compaction',

  // Key-Value Operations
  KV_KEY: (key: string) => `/kv/${encodeURIComponent(key)}`,
  KV_BATCH: '/kv/batch',
  KV_SCAN: '/kv/scan',
  KV_PREFIX: '/kv/prefix',

  // Cluster Events & WebSockets
  CLUSTER_EVENTS: '/cluster/events',
  WS_EVENTS: '/ws/events',

  // Internal Next.js API Runners
  INTERNAL_BENCHMARKS: '/api/benchmarks',
  INTERNAL_CHAOS: '/api/chaos',
  INTERNAL_SETTINGS: '/api/settings',
} as const;

/**
 * Returns the versioned v1 path for a given endpoint.
 */
export function toV1Endpoint(endpoint: string): string {
  if (endpoint.startsWith('/api/v1')) return endpoint;
  return `${API_PREFIX_V1}${endpoint.startsWith('/') ? endpoint : `/${endpoint}`}`;
}

/**
 * Format a WebSocket URL from an HTTP origin.
 */
export function toWebSocketUrl(httpUrl: string, wsPath: string = ENDPOINTS.WS_EVENTS): string {
  const url = new URL(wsPath, httpUrl);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
}
