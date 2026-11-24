/**
 * Query Console Service
 * Full Key-Value interaction: GET, PUT, DELETE, Batch mutations, Range & Prefix scanning.
 */

import {
  executeGetKV,
  executePutKV,
  executeDeleteKV,
  executeScanKV,
  request,
} from '../api/client';
import { ENDPOINTS } from '../api/endpoints';
import {
  GetKVResponse,
  SetKVResponse,
  DeleteKVResponse,
  BatchOp,
  BatchWriteResponse,
  ScanResponse,
  RequestOptions,
} from '../api/types';

export class QueryService {
  /**
   * Retrieve a value by key.
   */
  public static async get(
    key: string,
    options?: RequestOptions
  ): Promise<{
    ok: boolean;
    status: number;
    data: GetKVResponse | null;
    error?: string;
    latencyMs: number;
  }> {
    return await executeGetKV(key, options);
  }

  /**
   * Store or update a key-value pair.
   */
  public static async put(
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
    return await executePutKV(key, value, options);
  }

  /**
   * Remove a key from the cluster.
   */
  public static async delete(
    key: string,
    options?: RequestOptions
  ): Promise<{
    ok: boolean;
    status: number;
    data: DeleteKVResponse | null;
    error?: string;
    latencyMs: number;
  }> {
    return await executeDeleteKV(key, options);
  }

  /**
   * Execute atomic multi-shard batch write.
   */
  public static async batchWrite(
    ops: BatchOp[],
    options?: RequestOptions
  ): Promise<BatchWriteResponse> {
    return await request<BatchWriteResponse>(ENDPOINTS.KV_BATCH, 'POST', { ops }, options);
  }

  /**
   * Execute range scan across lexicographical key intervals.
   */
  public static async scan(
    params?: {
      start?: string;
      end?: string;
      limit?: number;
      cursor?: string;
    },
    options?: RequestOptions
  ): Promise<ScanResponse | null> {
    return await executeScanKV(params, options);
  }

  /**
   * Execute prefix scan for matching key namespaces.
   */
  public static async prefixScan(
    prefix: string,
    limit: number = 50,
    options?: RequestOptions
  ): Promise<ScanResponse | null> {
    return await executeScanKV({ prefix, limit }, options);
  }
}
