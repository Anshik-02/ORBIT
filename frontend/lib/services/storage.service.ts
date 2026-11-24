/**
 * Storage Service
 * LSM-Tree engine inspection: Active & Immutable Memtables, WAL log metrics, SSTable hierarchy, and compaction state.
 */

import { fetchStorageMetrics, request } from '../api/client';
import { ENDPOINTS } from '../api/endpoints';
import { StorageMetrics, StorageStatsResponse, RequestOptions } from '../api/types';

export class StorageService {
  /**
   * Fetch aggregated Prometheus storage metrics.
   */
  public static async getMetrics(options?: RequestOptions): Promise<StorageMetrics | null> {
    return await fetchStorageMetrics(options);
  }

  /**
   * Fetch detailed physical storage engine hierarchy from the cluster node.
   */
  public static async getDetailedStats(options?: RequestOptions): Promise<StorageStatsResponse | null> {
    try {
      return await request<StorageStatsResponse>(ENDPOINTS.STORAGE_STATS, 'GET', undefined, options);
    } catch {
      return null;
    }
  }

  /**
   * Fetch current or latest background compaction status.
   */
  public static async getCompactionStatus(options?: RequestOptions): Promise<StorageStatsResponse['compaction'] | null> {
    try {
      return await request<StorageStatsResponse['compaction']>(ENDPOINTS.STORAGE_COMPACTION, 'GET', undefined, options);
    } catch {
      return null;
    }
  }
}
