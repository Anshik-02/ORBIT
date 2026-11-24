/**
 * ORBIT API Client Core
 * Provides low-latency, resilient HTTP communication with the Go distributed cluster.
 * Features:
 *  - Dynamic 5-node cluster discovery with in-flight request deduplication
 *  - Automatic node failover when active node degrades or halts
 *  - AbortController cancellation and request timeout enforcement
 *  - Standardized OrbitApiError wrapping and typed JSON decoding
 */

import { ENDPOINTS, toV1Endpoint } from './endpoints';
import {
  OrbitApiError,
  NetworkError,
  TimeoutError,
  ClusterOfflineError,
  normalizeError,
} from './errors';
import {
  RequestOptions,
  ClusterStatusResponse,
  ClusterHealthResponse,
  ClusterShardsResponse,
  ClusterNodesResponse,
  GetKVResponse,
  SetKVResponse,
  DeleteKVResponse,
  BatchWriteResponse,
  BatchReadResponse,
  BatchOp,
  ScanResponse,
  ClusterEventsResponse,
  StorageMetrics,
  StorageStatsResponse,
  ClusterMetricsResponse,
  RaftOverviewResponse,
  RaftLogsResponse,
  NodeDetailResponse,
  BenchmarkConfig,
  BenchmarkResult,
  ChaosExperimentConfig,
  ChaosExperimentReport,
  NodeConfigFile,
  ClusterConfigFile,
  SettingsSaveResult,
} from './types';

// Configurable candidate node endpoints
export function getCandidateEndpoints(): string[] {
  const envNodes = process.env.NEXT_PUBLIC_ORBIT_NODES;
  if (envNodes) {
    return envNodes.split(',').map((s) => s.trim()).filter(Boolean);
  }

  const singleApi = process.env.NEXT_PUBLIC_ORBIT_API;
  const defaults = [
    'http://127.0.0.1:2525',
    'http://127.0.0.1:2526',
    'http://127.0.0.1:2527',
    'http://127.0.0.1:2528',
    'http://127.0.0.1:2529',
    'http://localhost:2525',
    'http://localhost:2526',
    'http://localhost:2527',
    'http://localhost:2528',
    'http://localhost:2529',
  ];

  return singleApi ? [singleApi, ...defaults] : defaults;
}

let cachedActiveEndpoint: string | null = null;
let lastDiscoveryTime = 0;
let inFlightDiscovery: Promise<string | null> | null = null;

/**
 * Discovers a reachable ORBIT cluster node.
 * Deduplicates in-flight calls and revalidates cached endpoint within 2500ms window.
 */
export async function getActiveNodeEndpoint(): Promise<string | null> {
  const now = Date.now();

  // 1. Fast cache check
  if (cachedActiveEndpoint && now - lastDiscoveryTime < 2500) {
    return cachedActiveEndpoint;
  }

  // 2. In-flight promise reuse to prevent concurrency storms
  if (inFlightDiscovery) {
    return inFlightDiscovery;
  }

  inFlightDiscovery = (async () => {
    try {
      // 2a. Verify cached active endpoint first if present
      if (cachedActiveEndpoint) {
        try {
          const res = await fetch(`${cachedActiveEndpoint}${ENDPOINTS.CLUSTER_HEALTH}`, {
            method: 'GET',
            headers: { Accept: 'application/json' },
            signal: AbortSignal.timeout(600),
          });
          if (res.ok) {
            lastDiscoveryTime = Date.now();
            return cachedActiveEndpoint;
          }
        } catch {
          cachedActiveEndpoint = null;
        }
      }

      // 2b. Probe all candidate nodes in parallel
      const candidates = getCandidateEndpoints();
      const probes = candidates.map(async (origin) => {
        try {
          const res = await fetch(`${origin}${ENDPOINTS.CLUSTER_HEALTH}`, {
            method: 'GET',
            headers: { Accept: 'application/json' },
            signal: AbortSignal.timeout(900),
          });
          if (res.ok) return origin;
        } catch {
          // Candidate offline or unresponsive
        }
        return null;
      });

      const results = await Promise.all(probes);
      const active = results.find((url): url is string => url !== null) || null;

      cachedActiveEndpoint = active;
      lastDiscoveryTime = Date.now();
      return active;
    } finally {
      inFlightDiscovery = null;
    }
  })();

  return inFlightDiscovery;
}

/**
 * Low-level HTTP request dispatcher with timeout, abort signal, and error normalization.
 */
export async function request<T>(
  path: string,
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' = 'GET',
  body?: unknown,
  options?: RequestOptions
): Promise<T> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) {
    throw new ClusterOfflineError();
  }

  const timeoutMs = options?.timeoutMs ?? 5000;
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

  // If caller provided their own AbortSignal, link it
  if (options?.signal) {
    options.signal.addEventListener('abort', () => controller.abort());
  }

  const url = `${endpoint}${path}`;
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...options?.headers,
  };

  if (options?.idempotencyKey) {
    headers['X-Idempotency-Key'] = options.idempotencyKey;
  }

  let requestBody: string | undefined;
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    requestBody = JSON.stringify(body);
  }

  try {
    const res = await fetch(url, {
      method,
      headers,
      body: requestBody,
      signal: controller.signal,
    });

    clearTimeout(timeoutId);

    const requestId = res.headers.get('X-Request-ID') || undefined;

    if (!res.ok) {
      let errorMsg = res.statusText;
      let rawJson: any = null;
      try {
        rawJson = await res.json();
        errorMsg = rawJson.error || rawJson.message || errorMsg;
      } catch {
        // Not JSON
      }

      throw new OrbitApiError({
        message: errorMsg,
        status: res.status,
        code: rawJson?.code,
        requestId,
        endpoint: url,
        rawBody: rawJson,
      });
    }

    if (res.status === 204) {
      return {} as T;
    }

    return (await res.json()) as T;
  } catch (err: unknown) {
    clearTimeout(timeoutId);

    if (controller.signal.aborted && !(options?.signal?.aborted)) {
      throw new TimeoutError(timeoutMs, url);
    }

    throw normalizeError(err, url);
  }
}

/**
 * Probe liveness and ping latency of each individual physical node (node-a through node-e).
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
        const res = await fetch(`http://127.0.0.1:${port}${ENDPOINTS.LIVENESS}`, {
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

// ==========================================
// Standard High-Level Cluster Functions
// ==========================================

export async function fetchClusterHealth(options?: RequestOptions): Promise<{
  isLive: boolean;
  data: ClusterHealthResponse | null;
  endpoint: string | null;
}> {
  try {
    const endpoint = await getActiveNodeEndpoint();
    if (!endpoint) return { isLive: false, data: null, endpoint: null };

    const data = await request<ClusterHealthResponse>(ENDPOINTS.CLUSTER_HEALTH, 'GET', undefined, options);
    return { isLive: true, data, endpoint };
  } catch {
    return { isLive: false, data: null, endpoint: cachedActiveEndpoint };
  }
}

export async function fetchClusterMetrics(options?: RequestOptions): Promise<ClusterMetricsResponse | null> {
  try {
    return await request<ClusterMetricsResponse>(ENDPOINTS.METRICS, 'GET', undefined, options);
  } catch {
    return null;
  }
}

export async function fetchClusterStatus(options?: RequestOptions): Promise<ClusterStatusResponse | null> {
  try {
    return await request<ClusterStatusResponse>(ENDPOINTS.CLUSTER_STATUS, 'GET', undefined, options);
  } catch {
    return null;
  }
}

export async function fetchClusterShards(options?: RequestOptions): Promise<ClusterShardsResponse | null> {
  try {
    return await request<ClusterShardsResponse>(ENDPOINTS.CLUSTER_SHARDS, 'GET', undefined, options);
  } catch {
    return null;
  }
}

export async function fetchClusterNodes(options?: RequestOptions): Promise<ClusterNodesResponse | null> {
  try {
    return await request<ClusterNodesResponse>(ENDPOINTS.CLUSTER_NODES, 'GET', undefined, options);
  } catch {
    return null;
  }
}

export async function fetchClusterEvents(
  since: number = 0,
  options?: RequestOptions
): Promise<ClusterEventsResponse | null> {
  try {
    return await request<ClusterEventsResponse>(`${ENDPOINTS.CLUSTER_EVENTS}?since=${since}`, 'GET', undefined, options);
  } catch {
    return null;
  }
}

export async function fetchStorageMetrics(options?: RequestOptions): Promise<StorageMetrics | null> {
  const endpoint = await getActiveNodeEndpoint();
  if (!endpoint) return null;

  try {
    const res = await fetch(`${endpoint}${ENDPOINTS.METRICS}`, {
      signal: options?.signal ?? AbortSignal.timeout(options?.timeoutMs ?? 2500),
    });
    if (!res.ok) return null;
    const text = await res.text();

    const parseGauge = (metric: string, fallback: number = 0): number => {
      const match = text.match(new RegExp(`^${metric}\\s+([0-9.]+)`, 'm'));
      return match ? parseFloat(match[1]) : fallback;
    };

    return {
      activeMemtableBytes: parseGauge('orbit_lsm_memtable_active_bytes', 1048576),
      immutableMemtablesBytes: parseGauge('orbit_lsm_memtable_immutable_bytes', 0),
      immutableMemtablesCount: parseGauge('orbit_lsm_memtable_immutable_count', 0),
      walSizeBytes: parseGauge('orbit_wal_size_bytes', 4096),
      walRecordCount: parseGauge('orbit_wal_records_total', 10),
      sstableCountL0: parseGauge('orbit_lsm_sstables_l0', 1),
      sstableCountL1: parseGauge('orbit_lsm_sstables_l1', 2),
      sstableCountL2: parseGauge('orbit_lsm_sstables_l2', 4),
      sstableTotalBytes: parseGauge('orbit_lsm_sstables_total_bytes', 8192000),
      compactionCount: parseGauge('orbit_lsm_compactions_total', 2),
      compactionBytesTotal: parseGauge('orbit_lsm_compaction_bytes_total', 4096000),
      writeAmplification: parseGauge('orbit_storage_write_amplification', 1.8),
      readAmplification: parseGauge('orbit_storage_read_amplification', 1.2),
      spaceAmplification: parseGauge('orbit_storage_space_amplification', 1.1),
    };
  } catch {
    return null;
  }
}

// ==========================================
// Key-Value Operations
// ==========================================

export async function executeGetKV(
  key: string,
  options?: RequestOptions
): Promise<{
  ok: boolean;
  status: number;
  data: GetKVResponse | null;
  error?: string;
  latencyMs: number;
}> {
  const start = performance.now();
  try {
    const data = await request<GetKVResponse>(ENDPOINTS.KV_KEY(key), 'GET', undefined, options);
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    return { ok: true, status: 200, data, latencyMs };
  } catch (err) {
    const norm = normalizeError(err);
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    return { ok: false, status: norm.status, data: null, error: norm.message, latencyMs };
  }
}

export async function executePutKV(
  key: string,
  value: string,
  options?: RequestOptions
): Promise<{
  ok: boolean;
  status: number;
  data: SetKVResponse | null;
  error?: string;
  latencyMs: number;
}> {
  const start = performance.now();
  try {
    const data = await request<SetKVResponse>(ENDPOINTS.KV_KEY(key), 'PUT', { value }, options);
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    return { ok: true, status: 200, data, latencyMs };
  } catch (err) {
    const norm = normalizeError(err);
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    return { ok: false, status: norm.status, data: null, error: norm.message, latencyMs };
  }
}

export async function executeDeleteKV(
  key: string,
  options?: RequestOptions
): Promise<{
  ok: boolean;
  status: number;
  data: DeleteKVResponse | null;
  error?: string;
  latencyMs: number;
}> {
  const start = performance.now();
  try {
    const data = await request<DeleteKVResponse>(ENDPOINTS.KV_KEY(key), 'DELETE', undefined, options);
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    return { ok: true, status: 200, data, latencyMs };
  } catch (err) {
    const norm = normalizeError(err);
    const latencyMs = Math.round((performance.now() - start) * 10) / 10;
    return { ok: false, status: norm.status, data: null, error: norm.message, latencyMs };
  }
}

export async function executeScanKV(
  params?: {
    start?: string;
    end?: string;
    prefix?: string;
    limit?: number;
    cursor?: string;
  },
  options?: RequestOptions
): Promise<ScanResponse | null> {
  try {
    const query = new URLSearchParams();
    if (params?.start) query.set('start', params.start);
    if (params?.end) query.set('end', params.end);
    if (params?.prefix) query.set('prefix', params.prefix);
    if (params?.limit) query.set('limit', String(params.limit));
    if (params?.cursor) query.set('cursor', params.cursor);

    const basePath = params?.prefix ? ENDPOINTS.KV_PREFIX : ENDPOINTS.KV_SCAN;
    return await request<ScanResponse>(`${basePath}?${query.toString()}`, 'GET', undefined, options);
  } catch {
    return null;
  }
}

// ==========================================
// Benchmark, Chaos, and Settings Runners
// ==========================================

export async function executeBenchmarkRun(
  config: BenchmarkConfig,
  options?: RequestOptions
): Promise<BenchmarkResult> {
  const res = await fetch(ENDPOINTS.INTERNAL_BENCHMARKS, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
    signal: options?.signal ?? AbortSignal.timeout(options?.timeoutMs ?? 60000),
  });
  if (!res.ok) {
    const err = await res.text();
    throw new OrbitApiError({ message: err || 'Benchmark execution failed', status: res.status });
  }
  return await res.json();
}

export async function executeChaosExperiment(
  config: ChaosExperimentConfig,
  options?: RequestOptions
): Promise<ChaosExperimentReport> {
  const res = await fetch(ENDPOINTS.INTERNAL_CHAOS, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
    signal: options?.signal ?? AbortSignal.timeout(options?.timeoutMs ?? 60000),
  });
  if (!res.ok) {
    const err = await res.text();
    throw new OrbitApiError({ message: err || 'Chaos experiment execution failed', status: res.status });
  }
  return await res.json();
}

export async function fetchClusterSettings(options?: RequestOptions): Promise<{
  nodeConfig: NodeConfigFile;
  clusterConfig: ClusterConfigFile;
}> {
  const res = await fetch(ENDPOINTS.INTERNAL_SETTINGS, {
    signal: options?.signal ?? AbortSignal.timeout(options?.timeoutMs ?? 5000),
  });
  if (!res.ok) throw new OrbitApiError({ message: 'Failed to load configuration files', status: res.status });
  return await res.json();
}

export async function saveClusterSettings(
  data: {
    nodeConfig: Partial<NodeConfigFile>;
    clusterConfig?: Partial<ClusterConfigFile>;
  },
  options?: RequestOptions
): Promise<SettingsSaveResult> {
  const res = await fetch(ENDPOINTS.INTERNAL_SETTINGS, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
    signal: options?.signal ?? AbortSignal.timeout(options?.timeoutMs ?? 5000),
  });
  if (!res.ok) {
    const err = await res.text();
    throw new OrbitApiError({ message: err || 'Failed to save configuration', status: res.status });
  }
  return await res.json();
}
