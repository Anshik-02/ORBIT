/**
 * Cluster Service
 * High-level orchestration for cluster telemetry, node fleet health, and topology state.
 */

import {
  fetchClusterHealth,
  fetchClusterStatus,
  fetchClusterNodes,
  probePhysicalNodes,
  getActiveNodeEndpoint,
} from '../api/client';
import { ClusterHealthResponse, ClusterStatusResponse, RequestOptions } from '../api/types';

export class ClusterService {
  /**
   * Fetch complete cluster health including node availability and multi-raft replica health.
   */
  public static async getHealth(options?: RequestOptions): Promise<{
    isLive: boolean;
    data: ClusterHealthResponse | null;
    endpoint: string | null;
  }> {
    return await fetchClusterHealth(options);
  }

  /**
   * Fetch static cluster configuration metadata (cluster_id, epoch, physical nodes list).
   */
  public static async getStatus(options?: RequestOptions): Promise<ClusterStatusResponse | null> {
    return await fetchClusterStatus(options);
  }

  /**
   * Direct probe of all 5 physical nodes to detect network partition or unresponsive daemons.
   */
  public static async probeAllNodes(): Promise<
    Record<string, { online: boolean; port: number; latencyMs: number }>
  > {
    return await probePhysicalNodes();
  }

  /**
   * Return currently resolved healthy cluster endpoint.
   */
  public static async getActiveEndpoint(): Promise<string | null> {
    return await getActiveNodeEndpoint();
  }
}
