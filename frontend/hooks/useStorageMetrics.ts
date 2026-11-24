'use client';

import { useState, useEffect, useCallback } from 'react';
import { fetchStorageMetrics, getActiveNodeEndpoint } from '@/lib/api/client';
import { StorageService } from '@/lib/services/storage.service';
import { StorageMetrics, StorageStatsResponse } from '@/lib/api/types';

export const EMPTY_STORAGE_METRICS: StorageMetrics = {
  activeMemtableBytes: 0,
  immutableMemtablesBytes: 0,
  immutableMemtablesCount: 0,
  walSizeBytes: 0,
  walRecordCount: 0,
  sstableCountL0: 0,
  sstableCountL1: 0,
  sstableCountL2: 0,
  sstableTotalBytes: 0,
  compactionCount: 0,
  compactionBytesTotal: 0,
  writeAmplification: 1.0,
  readAmplification: 1.0,
  spaceAmplification: 1.0,
};

export function useStorageMetrics() {
  const [metrics, setMetrics] = useState<StorageMetrics>(EMPTY_STORAGE_METRICS);
  const [detailedStats, setDetailedStats] = useState<StorageStatsResponse | null>(null);
  const [isLive, setIsLive] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isRefreshing, setIsRefreshing] = useState<boolean>(false);
  const [lastUpdated, setLastUpdated] = useState<Date>(new Date());

  const refresh = useCallback(async () => {
    setIsRefreshing(true);
    try {
      const endpoint = await getActiveNodeEndpoint();
      if (endpoint) {
        setIsLive(true);
        const [metricData, detailed] = await Promise.all([
          fetchStorageMetrics(),
          StorageService.getDetailedStats(),
        ]);
        if (metricData) {
          setMetrics(metricData);
        }
        if (detailed) {
          setDetailedStats(detailed);
          setMetrics((prev) => ({
            ...prev,
            activeMemtableBytes: detailed.active_memtable?.byte_size ?? prev.activeMemtableBytes,
            walSizeBytes: detailed.wal_size_bytes ?? prev.walSizeBytes,
            walRecordCount: detailed.wal_record_count ?? prev.walRecordCount,
            immutableMemtablesCount: detailed.immutable_count ?? prev.immutableMemtablesCount,
            compactionCount: detailed.compaction?.compaction_count ?? prev.compactionCount,
            sstableTotalBytes: detailed.total_sstable_bytes ?? prev.sstableTotalBytes,
            writeAmplification: detailed.write_amplification ?? prev.writeAmplification,
            readAmplification: detailed.read_amplification ?? prev.readAmplification,
            spaceAmplification: detailed.space_amplification ?? prev.spaceAmplification,
          }));
        }
      } else {
        setIsLive(false);
        setDetailedStats(null);
        setMetrics(EMPTY_STORAGE_METRICS);
      }
      setLastUpdated(new Date());
    } catch {
      setIsLive(false);
      setDetailedStats(null);
      setMetrics(EMPTY_STORAGE_METRICS);
    } finally {
      setIsLoading(false);
      setIsRefreshing(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 3000);
    return () => clearInterval(interval);
  }, [refresh]);

  return {
    metrics,
    detailedStats,
    isLive,
    isLoading,
    isRefreshing,
    lastUpdated,
    refresh,
  };
}
