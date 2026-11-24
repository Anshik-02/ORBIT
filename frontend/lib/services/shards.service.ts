/**
 * Shards Service
 * Shard placement, replica synchronization status, and consistent hash ring telemetry.
 */

import { request, fetchClusterHealth } from '../api/client';
import { ENDPOINTS } from '../api/endpoints';
import { ClusterShardsResponse, ShardStatus, RequestOptions } from '../api/types';

export interface ReplicaPlacementCell {
  nodeId: string;
  shardId: number;
  role: 'Leader' | 'Follower' | 'Unavailable' | 'None';
  isReplica: boolean;
  term: number;
  commitIndex: number;
  appliedIndex: number;
  lag: number;
  online: boolean;
}

export class ShardsService {
  /**
   * List configured shards with their assigned replica nodes.
   */
  public static async listShards(options?: RequestOptions): Promise<ClusterShardsResponse | null> {
    try {
      return await request<ClusterShardsResponse>(ENDPOINTS.CLUSTER_SHARDS, 'GET', undefined, options);
    } catch {
      return null;
    }
  }

  /**
   * Fetch detailed live status for a specific shard.
   */
  public static async getShardStatus(shardId: number, options?: RequestOptions): Promise<ShardStatus | null> {
    const health = await fetchClusterHealth(options);
    if (!health.data?.shards) return null;
    return health.data.shards[shardId] || null;
  }

  /**
   * Compute dynamic replica placement matrix mapping all nodes to shard roles.
   */
  public static async getReplicaMatrix(options?: RequestOptions): Promise<ReplicaPlacementCell[][]> {
    const health = await fetchClusterHealth(options);
    const nodes = ['node-a', 'node-b', 'node-c', 'node-d', 'node-e'];
    const shardCount = 3;

    const matrix: ReplicaPlacementCell[][] = [];

    for (let s = 0; s < shardCount; s++) {
      const row: ReplicaPlacementCell[] = [];
      const shardInfo = health.data?.shards?.[s];

      for (const n of nodes) {
        const isReplica = shardInfo?.replica_node_ids?.includes(n) ?? false;
        const isLeader = shardInfo?.current_leader === n;
        const nodeOnline = health.data?.nodes?.[n]?.online ?? true;

        let role: ReplicaPlacementCell['role'] = 'None';
        if (isReplica) {
          if (!nodeOnline) {
            role = 'Unavailable';
          } else if (isLeader) {
            role = 'Leader';
          } else {
            role = 'Follower';
          }
        }

        const lag = shardInfo?.replication_lag?.[n] ?? 0;

        row.push({
          nodeId: n,
          shardId: s,
          role,
          isReplica,
          term: shardInfo?.current_term ?? 1,
          commitIndex: shardInfo?.commit_index ?? 0,
          appliedIndex: shardInfo?.last_applied_index ?? 0,
          lag,
          online: nodeOnline,
        });
      }
      matrix.push(row);
    }

    return matrix;
  }
}
