/**
 * Nodes Service
 * Telemetry and individual node inspection across the 5-node cluster mesh.
 */

import { request, probePhysicalNodes } from '../api/client';
import { ENDPOINTS } from '../api/endpoints';
import { NodeDetailResponse, RequestOptions } from '../api/types';

export class NodesService {
  /**
   * List all configured physical node IDs in the cluster.
   */
  public static async listNodes(options?: RequestOptions): Promise<string[]> {
    try {
      const res = await request<{ nodes: string[] }>(ENDPOINTS.CLUSTER_NODES, 'GET', undefined, options);
      return res.nodes || ['node-a', 'node-b', 'node-c', 'node-d', 'node-e'];
    } catch {
      return ['node-a', 'node-b', 'node-c', 'node-d', 'node-e'];
    }
  }

  /**
   * Fetch comprehensive telemetry for a specific physical node.
   */
  public static async getNodeDetail(nodeId: string, options?: RequestOptions): Promise<NodeDetailResponse | null> {
    try {
      return await request<NodeDetailResponse>(ENDPOINTS.NODE_DETAIL(nodeId), 'GET', undefined, options);
    } catch {
      return null;
    }
  }

  /**
   * Check connection and response latency for all 5 physical nodes.
   */
  public static async pingAllNodes(): Promise<
    Record<string, { online: boolean; port: number; latencyMs: number }>
  > {
    return await probePhysicalNodes();
  }
}
