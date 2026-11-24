'use client';

import { useState, useEffect, useCallback } from 'react';
import { fetchStorageMetrics, getActiveNodeEndpoint } from '@/lib/api/client';
import { StorageService } from '@/lib/services/storage.service';
import { StorageMetrics, StorageStatsResponse } from '@/lib/api/types';

export function useStorageMetrics() {
  const [metrics, setMetrics] = useState<StorageMetrics>({
    activeMemtableBytes: 1048576,
    immutableMemtablesBytes: 0,
    immutableMemtablesCount: 0,
    walSizeBytes: 4096,
    walRecordCount: 1,
    sstableCountL0: 0,
    sstableCountL1: 0,
    sstableCountL2: 0,
    sstableTotalBytes: 0,
    compactionCount: 0,
    compactionBytesTotal: 0,
    writeAmplification: 1.8,
    readAmplification: 1.2,
    spaceAmplification: 1.1,
  });
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
        }
      } else {
        setIsLive(false);
      }
      setLastUpdated(new Date());
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
