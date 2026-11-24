'use client';

import { useState, useEffect, useCallback } from 'react';
import { fetchClusterShards, fetchClusterHealth } from '@/lib/api/client';
import { ShardDetail } from '@/components/shards/ShardDistributionCards';

export interface MatrixNodeRow {
  id: string;
  name: string;
  httpPort: string;
  roles: {
    0: 'Leader' | 'Follower' | null;
    1: 'Leader' | 'Follower' | null;
    2: 'Leader' | 'Follower' | null;
  };
}

const DEFAULT_SHARDS: ShardDetail[] = [
  {
    shardId: 0,
    name: 'Shard 0',
    color: '#0891D5',
    hashRange: '0x00000000..0x55555555',
    leader: 'node-a',
    replicas: ['node-a', 'node-b', 'node-c'],
    term: 3,
    commitIndex: 14820,
    appliedIndex: 14820,
    quorumStatus: 'HEALTHY (3/3)',
    keyPercentage: '33.3%',
  },
  {
    shardId: 1,
    name: 'Shard 1',
    color: '#7965E8',
    hashRange: '0x55555556..0xAAAAAAAA',
    leader: 'node-d',
    replicas: ['node-d', 'node-e', 'node-a'],
    term: 2,
    commitIndex: 14818,
    appliedIndex: 14818,
    quorumStatus: 'HEALTHY (3/3)',
    keyPercentage: '33.3%',
  },
  {
    shardId: 2,
    name: 'Shard 2',
    color: '#10B981',
    hashRange: '0xAAAAAAAB..0xFFFFFFFF',
    leader: 'node-b',
    replicas: ['node-b', 'node-c', 'node-d'],
    term: 4,
    commitIndex: 14822,
    appliedIndex: 14822,
    quorumStatus: 'HEALTHY (3/3)',
    keyPercentage: '33.4%',
  },
];

const DEFAULT_MATRIX: MatrixNodeRow[] = [
  { id: 'node-a', name: 'Node A', httpPort: ':2525', roles: { 0: 'Leader', 1: 'Follower', 2: null } },
  { id: 'node-b', name: 'Node B', httpPort: ':2526', roles: { 0: 'Follower', 1: null, 2: 'Leader' } },
  { id: 'node-c', name: 'Node C', httpPort: ':2527', roles: { 0: 'Follower', 1: null, 2: 'Follower' } },
  { id: 'node-d', name: 'Node D', httpPort: ':2528', roles: { 0: null, 1: 'Leader', 2: 'Follower' } },
  { id: 'node-e', name: 'Node E', httpPort: ':2529', roles: { 0: null, 1: 'Follower', 2: null } },
];

export function useShardsData() {
  const [shards, setShards] = useState<ShardDetail[]>(DEFAULT_SHARDS);
  const [matrixNodes, setMatrixNodes] = useState<MatrixNodeRow[]>(DEFAULT_MATRIX);
  const [isLive, setIsLive] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isRefreshing, setIsRefreshing] = useState<boolean>(false);
  const [lastUpdated, setLastUpdated] = useState<Date>(new Date());

  const refresh = useCallback(async () => {
    setIsRefreshing(true);
    try {
      const [healthRes, shardsRes] = await Promise.allSettled([
        fetchClusterHealth(),
        fetchClusterShards(),
      ]);

      const health = healthRes.status === 'fulfilled' ? healthRes.value : null;
      const rawShards = shardsRes.status === 'fulfilled' ? shardsRes.value : null;

      if (health?.isLive && health.data) {
        setIsLive(true);
        const healthShards = health.data.shards;
        const colorPalette = ['#0891D5', '#7965E8', '#10B981', '#F59E0B'];
        const ranges = [
          '0x00000000..0x55555555',
          '0x55555556..0xAAAAAAAA',
          '0xAAAAAAAB..0xFFFFFFFF',
        ];

        // Transform backend shard health into ShardDetail
        const dynamicShards: ShardDetail[] = Object.entries(healthShards).map(([sidStr, s]) => {
          const sid = Number(sidStr);
          const totalReps = s.replica_node_ids?.length || 3;
          const availReps = s.available_replicas?.length || (s.quorum_available ? totalReps : 1);
          return {
            shardId: sid,
            name: `Shard ${sid}`,
            color: colorPalette[sid % colorPalette.length],
            hashRange: ranges[sid] || `Shard Range ${sid}`,
            leader: s.current_leader || 'Electing...',
            replicas: s.replica_node_ids || [],
            term: s.current_term || 1,
            commitIndex: s.commit_index || 0,
            appliedIndex: s.last_applied_index || 0,
            quorumStatus: `${s.healthy ? 'HEALTHY' : 'DEGRADED'} (${availReps}/${totalReps})`,
            keyPercentage: `${Math.round(100 / Object.keys(healthShards).length)}%`,
          };
        });

        if (dynamicShards.length > 0) {
          setShards(dynamicShards);
        }

        // Dynamically compute matrix rows for physical nodes
        const physicalNodes = ['node-a', 'node-b', 'node-c', 'node-d', 'node-e'];
        const ports = [2525, 2526, 2527, 2528, 2529];
        const newMatrix: MatrixNodeRow[] = physicalNodes.map((nid, idx) => {
          const roles: { 0: 'Leader' | 'Follower' | null; 1: 'Leader' | 'Follower' | null; 2: 'Leader' | 'Follower' | null } = {
            0: null,
            1: null,
            2: null,
          };

          [0, 1, 2].forEach((sid) => {
            const sh = healthShards[sid];
            if (sh) {
              if (sh.current_leader === nid) {
                roles[sid as 0 | 1 | 2] = 'Leader';
              } else if (sh.replica_node_ids?.includes(nid)) {
                roles[sid as 0 | 1 | 2] = 'Follower';
              }
            }
          });

          return {
            id: nid,
            name: `Node ${nid.replace('node-', '').toUpperCase()}`,
            httpPort: `:${ports[idx]}`,
            roles,
          };
        });

        setMatrixNodes(newMatrix);
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
    const interval = setInterval(refresh, 3500);
    return () => clearInterval(interval);
  }, [refresh]);

  return {
    shards,
    matrixNodes,
    isLive,
    isLoading,
    isRefreshing,
    lastUpdated,
    refresh,
  };
}
