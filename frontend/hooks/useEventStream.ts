'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { getActiveNodeEndpoint, fetchClusterEvents } from '@/lib/api/client';
import { ClusterEventItem } from '@/components/events/EventStreamTable';
import { EventCategory, EventSeverity } from '@/components/events/EventFilterBar';

const INITIAL_EVENTS: ClusterEventItem[] = [
  { id: 'evt-101', timestamp: '14:30:21', category: 'CLUSTER', severity: 'INFO', nodeId: 'node-a', description: 'Node A heartbeat acknowledged by all cluster peers.', metadata: { rttAvgMs: 0.8, ackCount: 4 } },
  { id: 'evt-102', timestamp: '14:30:19', category: 'RAFT', severity: 'INFO', nodeId: 'node-d', shardId: 1, description: 'Shard 1 committed log entry #14818 via majority quorum.', metadata: { commitIndex: 14818, term: 2, entriesCount: 1 } },
  { id: 'evt-103', timestamp: '14:30:16', category: 'RAFT', severity: 'INFO', nodeId: 'node-a', shardId: 0, description: 'Leader elected: Node A won majority quorum (2/3) for Shard 0.', metadata: { term: 3, votesReceived: ['node-a', 'node-b'] } },
  { id: 'evt-104', timestamp: '14:30:12', category: 'REPLICATION', severity: 'INFO', nodeId: 'node-b', shardId: 0, description: 'Replica synchronization completed: MatchIndex caught up to CommitIndex.', metadata: { matchIndex: 14820, lag: 0 } },
  { id: 'evt-105', timestamp: '14:30:08', category: 'CLUSTER', severity: 'INFO', nodeId: 'node-c', description: 'Node C established Raft transport peering with all active nodes.', metadata: { remoteAddr: '127.0.0.1:3527', transport: 'TCP' } },
  { id: 'evt-106', timestamp: '14:28:44', category: 'STORAGE', severity: 'INFO', nodeId: 'node-a', shardId: 0, description: 'LSM Memtable flushed to L0 SSTable file sst_001.sst (42.1 KB).', metadata: { sstableId: 'sst_001', sizeBytes: 43110, level: 0 } },
  { id: 'evt-107', timestamp: '14:25:30', category: 'NETWORK', severity: 'WARN', nodeId: 'node-e', shardId: 1, description: 'Transient latency spike detected on Raft heartbeat (34ms). Restored to baseline.', metadata: { peakLatencyMs: 34.2, normalMs: 1.1 } },
];

export function useEventStream() {
  const [events, setEvents] = useState<ClusterEventItem[]>(INITIAL_EVENTS);
  const [isPaused, setIsPaused] = useState<boolean>(false);
  const [isConnected, setIsConnected] = useState<boolean>(false);
  const [lastSeq, setLastSeq] = useState<number>(0);
  const [activeEndpoint, setActiveEndpoint] = useState<string | null>(null);

  const wsRef = useRef<WebSocket | null>(null);
  const isPausedRef = useRef<boolean>(isPaused);
  isPausedRef.current = isPaused;

  // Map backend event type to UI Category & Severity
  const mapBackendEvent = (e: any): ClusterEventItem => {
    let category: EventCategory = 'CLUSTER';
    let severity: EventSeverity = 'INFO';

    const t = (e.type || '').toUpperCase();
    if (t.includes('ENTRY') || t.includes('LEADER') || t.includes('RAFT') || t.includes('TERM') || t.includes('VOTE')) {
      category = 'RAFT';
    } else if (t.includes('REPLICA') || t.includes('CATCHUP') || t.includes('SNAPSHOT')) {
      category = 'REPLICATION';
    } else if (t.includes('FLUSH') || t.includes('COMPACT') || t.includes('SSTABLE') || t.includes('WAL')) {
      category = 'STORAGE';
    } else if (t.includes('HEARTBEAT') || t.includes('LATENCY') || t.includes('TIMEOUT') || t.includes('NET')) {
      category = 'NETWORK';
    }

    if (t.includes('FAIL') || t.includes('CORRUPT') || t.includes('CRASH') || t.includes('ERROR')) {
      severity = 'ERROR';
    } else if (t.includes('WARN') || t.includes('TIMEOUT') || t.includes('DEGRADED') || t.includes('RETRY')) {
      severity = 'WARN';
    }

    const desc = e.data?.message ||
      (t === 'ENTRY_COMMITTED' ? `Entry committed on key "${e.data?.key || 'unknown'}" (Index #${e.data?.index || 0})` :
      t === 'LEADER_CHANGED' ? `Leader changed: Node ${e.node_id} elected for Shard ${e.shard_id ?? 0}` :
      t === 'NODE_JOINED' ? `Node ${e.node_id} joined active cluster topology` :
      t === 'NODE_FAILED' ? `Node ${e.node_id} heartbeat lost; marked offline` :
      `${e.type} observed on ${e.node_id}`);

    return {
      id: e.event_id || `evt-${Date.now()}-${Math.random()}`,
      timestamp: e.timestamp ? new Date(e.timestamp).toLocaleTimeString() : new Date().toLocaleTimeString(),
      category,
      severity,
      nodeId: e.node_id || 'cluster',
      shardId: typeof e.shard_id === 'number' ? e.shard_id : undefined,
      description: desc,
      metadata: e.data,
    };
  };

  // Connect WebSocket & Polling
  useEffect(() => {
    let cancelled = false;
    let pollTimer: any = null;

    async function initConnection() {
      const endpoint = await getActiveNodeEndpoint();
      if (cancelled) return;
      setActiveEndpoint(endpoint);

      if (!endpoint) {
        setIsConnected(false);
        return;
      }

      // 1. Fetch initial historical events
      try {
        const hist = await fetchClusterEvents(0);
        if (hist && Array.isArray(hist.events) && hist.events.length > 0) {
          const mapped = hist.events.map(mapBackendEvent);
          setEvents((prev) => {
            const existingIds = new Set(prev.map((x) => x.id));
            const fresh = mapped.filter((x) => !existingIds.has(x.id));
            return [...fresh, ...prev].slice(0, 100);
          });
        }
      } catch (err) {
        console.warn('Initial events fetch error:', err);
      }

      // 2. Establish WebSocket connection
      try {
        const wsUrl = endpoint.replace(/^http/, 'ws') + '/ws/events';
        const ws = new WebSocket(wsUrl);
        wsRef.current = ws;

        ws.onopen = () => {
          if (!cancelled) setIsConnected(true);
        };

        ws.onmessage = (event) => {
          if (isPausedRef.current) return;
          try {
            const raw = JSON.parse(event.data);
            const item = mapBackendEvent(raw);
            setEvents((prev) => [item, ...prev].slice(0, 150));
          } catch {}
        };

        ws.onerror = () => {
          if (!cancelled) setIsConnected(false);
        };

        ws.onclose = () => {
          if (!cancelled) setIsConnected(false);
        };
      } catch {
        setIsConnected(false);
      }

      // 3. Keep fallback polling in case WS misses packets or is blocked
      pollTimer = setInterval(async () => {
        if (isPausedRef.current || cancelled) return;
        try {
          const fresh = await fetchClusterEvents(0);
          if (fresh && Array.isArray(fresh.events) && fresh.events.length > 0) {
            const mapped = fresh.events.map(mapBackendEvent);
            setEvents((prev) => {
              const existingIds = new Set(prev.map((x) => x.id));
              const newItems = mapped.filter((x) => !existingIds.has(x.id));
              if (newItems.length === 0) return prev;
              return [...newItems, ...prev].slice(0, 150);
            });
          }
        } catch {}
      }, 4000);
    }

    initConnection();

    return () => {
      cancelled = true;
      if (wsRef.current) {
        wsRef.current.close();
      }
      if (pollTimer) clearInterval(pollTimer);
    };
  }, []);

  const exportJSON = useCallback(() => {
    const jsonStr = JSON.stringify(events, null, 2);
    const blob = new Blob([jsonStr], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `orbit-cluster-events-${Date.now()}.json`;
    a.click();
    URL.revokeObjectURL(url);
  }, [events]);

  const exportCSV = useCallback(() => {
    const headers = ['ID', 'Timestamp', 'Category', 'Severity', 'Node', 'Shard', 'Description'];
    const rows = events.map((e) => [
      e.id,
      e.timestamp,
      e.category,
      e.severity,
      e.nodeId,
      e.shardId ?? 'N/A',
      `"${e.description.replace(/"/g, '""')}"`,
    ]);
    const csvContent = [headers.join(','), ...rows.map((r) => r.join(','))].join('\n');
    const blob = new Blob([csvContent], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `orbit-cluster-events-${Date.now()}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }, [events]);

  return {
    events,
    isPaused,
    setIsPaused,
    isConnected,
    activeEndpoint,
    exportJSON,
    exportCSV,
    clearEvents: () => setEvents([]),
  };
}
