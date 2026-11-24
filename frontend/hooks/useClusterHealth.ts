'use client';

import { useState, useEffect, useCallback } from 'react';
import { fetchClusterHealth, fetchClusterStatus } from '@/lib/api/client';
import { ClusterHealthResponse, ClusterStatusResponse } from '@/lib/api/types';

export function useClusterHealth(pollIntervalMs: number = 3000) {
  const [data, setData] = useState<ClusterHealthResponse | null>(null);
  const [status, setStatus] = useState<ClusterStatusResponse | null>(null);
  const [isLive, setIsLive] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [endpoint, setEndpoint] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<Date>(new Date());

  const refresh = useCallback(async () => {
    try {
      const [healthRes, statusRes] = await Promise.all([
        fetchClusterHealth(),
        fetchClusterStatus(),
      ]);

      setIsLive(healthRes.isLive);
      setEndpoint(healthRes.endpoint);
      if (healthRes.data) {
        setData(healthRes.data);
      }
      if (statusRes) {
        setStatus(statusRes);
      }
      setLastUpdated(new Date());
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, pollIntervalMs);
    return () => clearInterval(interval);
  }, [refresh, pollIntervalMs]);

  return {
    data,
    health: data,
    status,
    isLive,
    isLoading,
    loading: isLoading,
    endpoint,
    activeEndpoint: endpoint,
    lastUpdated,
    refresh,
    refetch: refresh,
  };
}
