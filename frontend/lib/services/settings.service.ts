/**
 * Settings Service
 * Configuration management, JSON validation, and safe persistence for node & cluster topologies.
 */

import { fetchClusterSettings, saveClusterSettings } from '../api/client';
import {
  NodeConfigFile,
  ClusterConfigFile,
  SettingsSaveResult,
  RequestOptions,
} from '../api/types';

export class SettingsService {
  /**
   * Fetch current configuration files for node-a and the cluster.
   */
  public static async getSettings(options?: RequestOptions): Promise<{
    nodeConfig: NodeConfigFile;
    clusterConfig: ClusterConfigFile;
  }> {
    return await fetchClusterSettings(options);
  }

  /**
   * Validate and persist configuration changes.
   */
  public static async saveSettings(
    data: {
      nodeConfig: Partial<NodeConfigFile>;
      clusterConfig?: Partial<ClusterConfigFile>;
    },
    options?: RequestOptions
  ): Promise<SettingsSaveResult> {
    const errors = this.validate(data);
    if (errors.length > 0) {
      return {
        ok: false,
        restartRequired: false,
        message: 'Validation failed',
        errors,
      };
    }

    return await saveClusterSettings(data, options);
  }

  /**
   * Client-side validation for configuration fields to prevent corrupt configurations.
   */
  public static validate(data: {
    nodeConfig: Partial<NodeConfigFile>;
    clusterConfig?: Partial<ClusterConfigFile>;
  }): string[] {
    const errors: string[] = [];

    if (data.nodeConfig.id !== undefined && !data.nodeConfig.id.trim()) {
      errors.push('Node ID cannot be empty');
    }

    if (data.nodeConfig.heartbeat_interval !== undefined) {
      if (!/^\d+(ms|s)$/.test(data.nodeConfig.heartbeat_interval)) {
        errors.push('Heartbeat interval must be a valid Go duration (e.g. 50ms, 1s)');
      }
    }

    if (data.nodeConfig.election_timeout_min !== undefined) {
      if (!/^\d+(ms|s)$/.test(data.nodeConfig.election_timeout_min)) {
        errors.push('Election timeout minimum must be a valid Go duration (e.g. 150ms)');
      }
    }

    if (data.nodeConfig.election_timeout_max !== undefined) {
      if (!/^\d+(ms|s)$/.test(data.nodeConfig.election_timeout_max)) {
        errors.push('Election timeout maximum must be a valid Go duration (e.g. 300ms)');
      }
    }

    if (data.clusterConfig?.shard_count !== undefined && data.clusterConfig.shard_count < 1) {
      errors.push('Shard count must be at least 1');
    }

    if (data.clusterConfig?.replication_factor !== undefined && data.clusterConfig.replication_factor < 1) {
      errors.push('Replication factor must be at least 1');
    }

    return errors;
  }
}
