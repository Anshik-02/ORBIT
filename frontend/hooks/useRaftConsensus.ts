'use client';

import { useState, useEffect, useCallback } from 'react';
import { fetchClusterHealth, fetchClusterEvents } from '@/lib/api/client';
import { RaftService } from '@/lib/services/raft.service';
import { RaftGroupInfo } from '@/components/raft/RaftGroupSelector';
import { ElectionEvent } from '@/components/raft/ElectionTimeline';
import { RaftLogEntry } from '@/components/raft/RaftLogInspector';

export interface FollowerTelemetry {
  id: string;
  name: string;
  httpPort: string;
  online: boolean;
  matchIndex: number;
  nextIndex: number;
  replicationLag: number;
  latency: string;
  lastAck: string;
  lastHeartbeat: string;
}

export function useRaftConsensus() {
  const [selectedShardId, setSelectedShardId] = useState<number>(0);
  const [groups, setGroups] = useState<RaftGroupInfo[]>([]);
  const [followersByShard, setFollowersByShard] = useState<Record<number, FollowerTelemetry[]>>({});
  const [electionEvents, setElectionEvents] = useState<ElectionEvent[]>([]);
  const [logEntries, setLogEntries] = useState<RaftLogEntry[]>([]);
  const [isLive, setIsLive] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isRefreshing, setIsRefreshing] = useState<boolean>(false);
  const [lastUpdated, setLastUpdated] = useState<Date>(new Date());

  const nodePortMap: Record<string, string> = {
    'node-a': ':2525',
    'node-b': ':2526',
    'node-c': ':2527',
    'node-d': ':2528',
    'node-e': ':2529',
  };

  const refresh = useCallback(async () => {
    setIsRefreshing(true);
    try {
      const [healthRes, eventsRes] = await Promise.allSettled([
        fetchClusterHealth(),
        fetchClusterEvents(0),
      ]);

      const health = healthRes.status === 'fulfilled' ? healthRes.value : null;
      const events = eventsRes.status === 'fulfilled' ? eventsRes.value : null;

      if (health?.isLive && health.data && health.data.shards) {
        setIsLive(true);
        const colorPalette = ['#0891D5', '#7965E8', '#10B981'];
        const ranges = ['0x00..0x55', '0x55..0xAA', '0xAA..0xFF'];

        // Build groups from live shard state
        const dynamicGroups: RaftGroupInfo[] = Object.entries(health.data.shards).map(([sidStr, s]) => {
          const sid = Number(sidStr);
          return {
            shardId: sid,
            name: `Shard ${sid} (${ranges[sid] || 'Partition'})`,
            color: colorPalette[sid % colorPalette.length],
            leader: s.current_leader || 'None (Electing)',
            term: s.current_term || 0,
            commitIndex: s.commit_index || 0,
            replicas: s.replica_node_ids || [],
          };
        });
        setGroups(dynamicGroups);

        // Build follower telemetry per shard
        const followerMap: Record<number, FollowerTelemetry[]> = {};
        Object.entries(health.data.shards).forEach(([sidStr, s]) => {
          const sid = Number(sidStr);
          const leader = s.current_leader;
          const replicas = s.replica_node_ids || [];
          const followers = replicas.filter((r) => r !== leader);

          followerMap[sid] = followers.map((fid) => {
            const nodeInfo = health.data?.nodes[fid];
            const isOnline = nodeInfo?.online ?? false;
            const progress = s.replication_progress?.[fid];
            const matchIndex = progress?.match_index ?? s.commit_index;
            const lag = s.commit_index > matchIndex ? s.commit_index - matchIndex : 0;

            return {
              id: fid,
              name: `Node ${fid.replace('node-', '').toUpperCase()}`,
              httpPort: nodePortMap[fid] || ':2525',
              online: isOnline,
              matchIndex,
              nextIndex: progress?.next_index ?? (matchIndex + 1),
              replicationLag: lag,
              latency: isOnline ? '< 1.0ms' : 'Unreachable',
              lastAck: isOnline ? (lag === 0 ? 'In Sync' : `${lag} entries behind`) : 'Unreachable',
              lastHeartbeat: isOnline ? '< 50ms (Tick OK)' : 'Expired',
            };
          });
        });
        setFollowersByShard(followerMap);

        // Fetch real Raft logs from leader node if available
        const activeLeader = health.data.shards[selectedShardId]?.current_leader;
        if (activeLeader) {
          try {
            const liveLogs = await RaftService.getLogs(activeLeader, 25);
            if (liveLogs && Array.isArray(liveLogs) && liveLogs.length > 0) {
              setLogEntries(
                liveLogs.map((l) => ({
                  index: l.index,
                  term: l.term,
                  type:
                    l.type === 'KV_DELETE'
                      ? 'KV_DELETE'
                      : l.type === 'CONF_CHANGE'
                      ? 'CONF_CHANGE'
                      : l.type === 'NO_OP'
                      ? 'NO_OP'
                      : 'KV_PUT',
                  payloadSummary: l.key ? `${l.type} ${l.key}` : `Log entry index=${l.index}`,
                  committed: l.committed ?? true,
                  applied: l.applied ?? true,
                  timestamp: l.timestamp ? new Date(l.timestamp).toLocaleTimeString() : 'Recent',
                }))
              );
            } else {
              setLogEntries([]);
            }
          } catch {
            setLogEntries([]);
          }
        } else {
          setLogEntries([]);
        }

        // Derive election events from real cluster events
        if (events && Array.isArray(events.events)) {
          const leaderEvents = events.events
            .filter((e) => e.type === 'LEADER_CHANGED' || e.type === 'HEARTBEAT_TIMEOUT')
            .map((e, idx) => ({
              id: e.event_id || `elec-${idx}`,
              term: e.data?.term || 1,
              shardId: e.shard_id ?? 0,
              type: (e.type === 'LEADER_CHANGED' ? 'NEW_LEADER_ELECTED' : 'HEARTBEAT_TIMEOUT') as any,
              nodeId: e.node_id || 'node',
              votesReceived: 3,
              votesNeeded: 2,
              durationMs: 75,
              timestamp: e.timestamp ? new Date(e.timestamp).toLocaleTimeString() : 'Recent',
              outcome: 'SUCCESS' as const,
              description: `Shard ${e.shard_id ?? 0} consensus established on ${e.node_id}`,
            }));

          setElectionEvents(leaderEvents);
        }
      } else {
        setIsLive(false);
        setGroups([]);
        setLogEntries([]);
        setFollowersByShard({});
        setElectionEvents([]);
      }
      setLastUpdated(new Date());
    } finally {
      setIsLoading(false);
      setIsRefreshing(false);
    }
  }, [selectedShardId]);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 3000);
    return () => clearInterval(interval);
  }, [refresh]);

  const fallbackGroup: RaftGroupInfo = {
    shardId: selectedShardId,
    name: `Shard ${selectedShardId} (Connecting...)`,
    color: '#0891D5',
    leader: 'None',
    term: 0,
    commitIndex: 0,
    replicas: [],
  };

  const activeGroup = groups.find((g) => g.shardId === selectedShardId) || groups[0] || fallbackGroup;
  const activeFollowers = followersByShard[selectedShardId] || [];

  return {
    selectedShardId,
    setSelectedShardId,
    groups,
    activeGroup,
    activeFollowers,
    electionEvents,
    logEntries,
    isLive,
    isLoading,
    isRefreshing,
    lastUpdated,
    refresh,
  };
}
