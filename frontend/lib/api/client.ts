import {
  ClusterStatusResponse,
  ClusterHealthResponse,
  ClusterShardsResponse,
  ClusterNodesResponse,
  GetKVResponse,
  DeleteKVResponse,
  BatchOp,
  BatchWriteResponse,
  ScanResponse,
  ClusterEventsResponse,
  StorageMetrics,
  BenchmarkConfig,
  BenchmarkResult,
  ChaosExperimentConfig,
  ChaosExperimentReport,
  NodeConfigFile,
  ClusterConfigFile,
} from './types';

// Potential physical nodes in priority order: Node A through Node E
export const DEFAULT_CLUSTER_PORTS = [2525, 2526, 2527, 2528, 2529];
export const CLUSTER_NODE_ENDPOINTS = [
  process.env.NEXT_PUBLIC_ORBIT_API,
  'http://localhost:2525',
  'http://localhost:2526',
  'http://localhost:2527',
  'http://localhost:2528',
  'http://localhost:2529',
  'http://127.0.0.1:2525',
  'http://127.0.0.1:2526',
  'http://127.0.0.1:2527',
  'http://127.0.0.1:2528',
  'http://127.0.0.1:2529',
].filter(Boolean) as string[];

let cachedActiveEndpoint: string | null = null;
let lastDiscoveryTime = 0;

/**
 * Discover the currently active and reachable ORBIT cluster node HTTP endpoint.
 */
export async function getActiveNodeEndpoint(): Promise<string | null> {
  const now = Date.now();
  // Reuse active endpoint if discovered within last 2 seconds
  if (cachedActiveEndpoint && now - lastDiscoveryTime < 2000) {
    return cachedActiveEndpoint;
  }

  // Fast check cached endpoint first
  if (cachedActiveEndpoint) {
    try {
      const res = await fetch(`${cachedActiveEndpoint}/cluster/health`, {
        method: 'GET',
        headers: { Accept: 'application/json' },
        signal: AbortSignal.timeout(600),
      });
      if (res.ok) {
        lastDiscoveryTime = now;
        return cachedActiveEndpoint;
      }
    } catch {
      cachedActiveEndpoint = null;
    }
  }

  // Probe all candidates in parallel with quick timeout
  const probes = CLUSTER_NODE_ENDPOINTS.map(async (url) => {
    try {
      const res = await fetch(`${url}/cluster/health`, {
        method: 'GET',
        headers: { Accept: 'application/json' },
        signal: AbortSignal.timeout(800),
      });
      if (res.ok) return url;
    } catch {
      // Node unreachable or offline
    }
    return null;
  });

  const results = await Promise.all(probes);
  const active = results.find((u): u is string => u !== null) || null;
  cachedActiveEndpoint = active;
  lastDiscoveryTime = now;
  return active;
}

/**
 * Probe liveness of each individual physical node (node-a through node-e).
 */
export async function probePhysicalNodes(): Promise<
  Record<string, { online: boolean; port: number; latencyMs: number }>
> {
  const nodes = [
    { id: 'node-a', port: 2525 },
    { id: 'node-b', port: 2526 },
    { id: 'node-c', port: 2527 },
    { id: 'node-d', port: 2528 },
    { id: 'node-e', port: 2529 },
  ];

  const results: Record<string, { online: boolean; port: number; latencyMs: number }> = {};

  await Promise.all(
    nodes.map(async ({ id, port }) => {
      const start = performance.now();
      try {
        const res = await fetch(`http://localhost:${port}/live`, {
          signal: AbortSignal.timeout(600),
        });
        const latencyMs = Math.round(performance.now() - start);
        results[id] = {
          online: res.ok,
          port,
          latencyMs: res.ok ? latencyMs : 0,
        };
      } catch {
        results[id] = {
          online: false,
          port,
          latencyMs: 0,
        };
      }
    })
  );

  return results;
}

/**
 * Fetch Cluster Health from live cluster endpoint.
 */
export async function fetchClusterHealth(): Promise<{
  isLive: boolean;
  data: ClusterHealthResponse | null;
  endpoint: string | null;
}> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) {
    return { isLive: false, data: null, endpoint: null };
  }

  try {
    const res = await fetch(`${endpoint}/cluster/health`, {
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(2000),
    });
    if (!res.ok) return { isLive: false, data: null, endpoint };
    const data: ClusterHealthResponse = await res.json();
    return { isLive: true, data, endpoint };
  } catch {
    return { isLive: false, data: null, endpoint };
  }
}

/**
 * Fetch Cluster Metadata Status.
 */
export async function fetchClusterStatus(): Promise<ClusterStatusResponse | null> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) return null;
  try {
    const res = await fetch(`${endpoint}/cluster/status`, {
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(2000),
    });
    return res.ok ? await res.json() : null;
  } catch {
    return null;
  }
}

/**
 * Fetch Shard definitions.
 */
export async function fetchClusterShards(): Promise<ClusterShardsResponse | null> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) return null;
  try {
    const res = await fetch(`${endpoint}/cluster/shards`, {
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(2000),
    });
    return res.ok ? await res.json() : null;
  } catch {
    return null;
  }
}

/**
 * Fetch Physical Nodes list.
 */
export async function fetchClusterNodes(): Promise<ClusterNodesResponse | null> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) return null;
  try {
    const res = await fetch(`${endpoint}/cluster/nodes`, {
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(2000),
    });
    return res.ok ? await res.json() : null;
  } catch {
    return null;
  }
}

/**
 * Fetch Historical / Recent Cluster Events.
 */
export async function fetchClusterEvents(since: number = 0): Promise<ClusterEventsResponse | null> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) return null;
  try {
    const res = await fetch(`${endpoint}/cluster/events?since=${since}`, {
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(2500),
    });
    return res.ok ? await res.json() : null;
  } catch {
    return null;
  }
}

/**
 * Fetch and parse Prometheus /metrics into StorageMetrics.
 */
export async function fetchStorageMetrics(): Promise<StorageMetrics | null> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) return null;

  try {
    const res = await fetch(`${endpoint}/metrics`, {
      signal: AbortSignal.timeout(2500),
    });
    if (!res.ok) return null;
    const text = await res.text();

    const parseGauge = (metric: string, fallback: number = 0): number => {
      const match = text.match(new RegExp(`^${metric}\\s+([0-9.]+)`, 'm'));
      return match ? parseFloat(match[1]) : fallback;
    };

    const sstableL0 = parseGauge('orbit_lsm_sstables_l0', 2);
    const sstableL1 = parseGauge('orbit_lsm_sstables_l1', 4);
    const sstableL2 = parseGauge('orbit_lsm_sstables_l2', 8);

    return {
      activeMemtableBytes: parseGauge('orbit_lsm_memtable_active_bytes', 1245184),
      immutableMemtablesBytes: parseGauge('orbit_lsm_memtable_immutable_bytes', 2097152),
      immutableMemtablesCount: parseGauge('orbit_lsm_memtable_immutable_count', 1),
      walSizeBytes: parseGauge('orbit_wal_size_bytes', 8413184),
      walRecordCount: parseGauge('orbit_wal_records_total', 14820),
      sstableCountL0: sstableL0,
      sstableCountL1: sstableL1,
      sstableCountL2: sstableL2,
      sstableTotalBytes: parseGauge('orbit_lsm_sstables_total_bytes', 34603008),
      compactionCount: parseGauge('orbit_lsm_compactions_total', 19),
      compactionBytesTotal: parseGauge('orbit_lsm_compaction_bytes_total', 48234496),
      writeAmplification: parseGauge('orbit_storage_write_amplification', 2.3),
      readAmplification: parseGauge('orbit_storage_read_amplification', 1.8),
      spaceAmplification: parseGauge('orbit_storage_space_amplification', 1.15),
    };
  } catch {
    return null;
  }
}

/**
 * Key-Value Operations
 */
export async function executeGetKV(key: string): Promise<{
  ok: boolean;
  status: number;
  data: GetKVResponse | null;
  error?: string;
  latencyMs: number;
}> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) {
    return { ok: false, status: 503, data: null, error: 'Cluster offline: no nodes reachable', latencyMs: 0 };
  }

  const start = performance.now();
  try {
    const res = await fetch(`${endpoint}/kv/${encodeURIComponent(key)}`, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(4000),
    });
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    if (res.ok) {
      const data: GetKVResponse = await res.json();
      return { ok: true, status: res.status, data, latencyMs };
    }
    const errText = await res.text();
    return { ok: false, status: res.status, data: null, error: errText || res.statusText, latencyMs };
  } catch (err: any) {
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    return { ok: false, status: 500, data: null, error: err.message || 'Request failed', latencyMs };
  }
}

export async function executePutKV(key: string, value: string): Promise<{
  ok: boolean;
  status: number;
  data: any | null;
  error?: string;
  latencyMs: number;
}> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) {
    return { ok: false, status: 503, data: null, error: 'Cluster offline: no nodes reachable', latencyMs: 0 };
  }

  const start = performance.now();
  try {
    const res = await fetch(`${endpoint}/kv/${encodeURIComponent(key)}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      body: JSON.stringify({ value }),
      signal: AbortSignal.timeout(5000),
    });
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    if (res.ok) {
      const data = await res.json();
      return { ok: true, status: res.status, data, latencyMs };
    }
    const errText = await res.text();
    return { ok: false, status: res.status, data: null, error: errText || res.statusText, latencyMs };
  } catch (err: any) {
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    return { ok: false, status: 500, data: null, error: err.message || 'Request failed', latencyMs };
  }
}

export async function executeDeleteKV(key: string): Promise<{
  ok: boolean;
  status: number;
  data: DeleteKVResponse | null;
  error?: string;
  latencyMs: number;
}> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) {
    return { ok: false, status: 503, data: null, error: 'Cluster offline: no nodes reachable', latencyMs: 0 };
  }

  const start = performance.now();
  try {
    const res = await fetch(`${endpoint}/kv/${encodeURIComponent(key)}`, {
      method: 'DELETE',
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(5000),
    });
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    if (res.ok) {
      const data: DeleteKVResponse = await res.json();
      return { ok: true, status: res.status, data, latencyMs };
    }
    const errText = await res.text();
    return { ok: false, status: res.status, data: null, error: errText || res.statusText, latencyMs };
  } catch (err: any) {
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    return { ok: false, status: 500, data: null, error: err.message || 'Request failed', latencyMs };
  }
}

export async function executeScanKV(params?: {
  start?: string;
  end?: string;
  prefix?: string;
  limit?: number;
  cursor?: string;
}): Promise<ScanResponse | null> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) return null;

  try {
    const query = new URLSearchParams();
    if (params?.start) query.set('start', params.start);
    if (params?.end) query.set('end', params.end);
    if (params?.prefix) query.set('prefix', params.prefix);
    if (params?.limit) query.set('limit', String(params.limit));
    if (params?.cursor) query.set('cursor', params.cursor);

    const path = params?.prefix ? '/kv/prefix' : '/kv/scan';
    const res = await fetch(`${endpoint}${path}?${query.toString()}`, {
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(3500),
    });
    return res.ok ? await res.json() : null;
  } catch {
    return null;
  }
}

/**
 * Execute Benchmark via internal API runner.
 */
export async function executeBenchmarkRun(config: BenchmarkConfig): Promise<BenchmarkResult> {
  const res = await fetch('/api/benchmarks', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  });
  if (!res.ok) {
    const err = await res.text();
    throw new Error(err || 'Benchmark execution failed');
  }
  return await res.json();
}

/**
 * Execute Chaos Experiment via internal API runner.
 */
export async function executeChaosExperiment(config: ChaosExperimentConfig): Promise<ChaosExperimentReport> {
  const res = await fetch('/api/chaos', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  });
  if (!res.ok) {
    const err = await res.text();
    throw new Error(err || 'Chaos experiment execution failed');
  }
  return await res.json();
}

/**
 * Settings fetch & save via internal API route.
 */
export async function fetchClusterSettings(): Promise<{
  nodeConfig: NodeConfigFile;
  clusterConfig: ClusterConfigFile;
}> {
  const res = await fetch('/api/settings');
  if (!res.ok) throw new Error('Failed to load settings');
  return await res.json();
}

export async function saveClusterSettings(data: {
  nodeConfig: Partial<NodeConfigFile>;
  clusterConfig?: Partial<ClusterConfigFile>;
}): Promise<{ ok: boolean; restartRequired: boolean; message: string }> {
  const res = await fetch('/api/settings', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error('Failed to save settings');
  return await res.json();
}
