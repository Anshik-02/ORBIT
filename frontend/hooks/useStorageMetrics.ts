'use client';

import { useState, useEffect, useCallback } from 'react';
import { fetchStorageMetrics, getActiveNodeEndpoint } from '@/lib/api/client';
import { StorageMetrics } from '@/lib/api/types';

export function useStorageMetrics() {
  const [metrics, setMetrics] = useState<StorageMetrics>({
    activeMemtableBytes: 18432000,
    immutableMemtablesBytes: 67108864,
    immutableMemtablesCount: 1,
    walSizeBytes: 31457280,
    walRecordCount: 44460,
    sstableCountL0: 3,
    sstableCountL1: 5,
    sstableCountL2: 12,
    sstableTotalBytes: 34603008,
    compactionCount: 19,
    compactionBytesTotal: 48234496,
    writeAmplification: 2.3,
    readAmplification: 1.8,
    spaceAmplification: 1.15,
  });
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
        const data = await fetchStorageMetrics();
        if (data) {
          setMetrics(data);
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
    const interval = setInterval(refresh, 4000);
    return () => clearInterval(interval);
  }, [refresh]);

  return {
    metrics,
    isLive,
    isLoading,
    isRefreshing,
    lastUpdated,
    refresh,
  };
}
