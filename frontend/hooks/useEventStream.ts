'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { getActiveNodeEndpoint, fetchClusterEvents } from '@/lib/api/client';
import { ClusterEventItem } from '@/components/events/EventStreamTable';
import { EventCategory, EventSeverity } from '@/components/events/EventFilterBar';

export function useEventStream() {
  const [events, setEvents] = useState<ClusterEventItem[]>([]);
  const [isPaused, setIsPaused] = useState<boolean>(false);
  const [isConnected, setIsConnected] = useState<boolean>(false);
  const [lastSeq, setLastSeq] = useState<number>(0);
  const [activeEndpoint, setActiveEndpoint] = useState<string | null>(null);

  const wsRef = useRef<WebSocket | null>(null);
  const isPausedRef = useRef<boolean>(isPaused);
  isPausedRef.current = isPaused;
  const lastSeqRef = useRef<number>(0);
  lastSeqRef.current = lastSeq;
  const seenIdsRef = useRef<Set<string>>(new Set());

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

    const desc =
      e.data?.message ||
      (t === 'ENTRY_COMMITTED'
        ? `Entry committed on key "${e.data?.key || 'unknown'}" (Index #${e.data?.index || 0})`
        : t === 'LEADER_CHANGED'
        ? `Leader changed: Node ${e.node_id} elected for Shard ${e.shard_id ?? 0}`
        : t === 'NODE_JOINED'
        ? `Node ${e.node_id} joined active cluster topology`
        : t === 'NODE_FAILED'
        ? `Node ${e.node_id} heartbeat lost; marked offline`
        : `${e.type} observed on ${e.node_id}`);

    return {
      id: e.event_id || `evt-${e.sequence || Date.now()}-${Math.random()}`,
      timestamp: e.timestamp ? new Date(e.timestamp).toLocaleTimeString() : new Date().toLocaleTimeString(),
      category,
      severity,
      nodeId: e.node_id || 'cluster',
      shardId: typeof e.shard_id === 'number' ? e.shard_id : undefined,
      description: desc,
      metadata: e.data,
    };
  };

  // Connect WebSocket & Polling with Exponential Backoff
  useEffect(() => {
    let cancelled = false;
    let pollTimer: any = null;
    let reconnectTimeout: any = null;
    let reconnectAttempts = 0;

    async function initConnection() {
      const endpoint = await getActiveNodeEndpoint();
      if (cancelled) return;
      setActiveEndpoint(endpoint);

      if (!endpoint) {
        setIsConnected(false);
        // Retry discovery in 3 seconds
        reconnectTimeout = setTimeout(initConnection, 3000);
        return;
      }

      // 1. Fetch initial historical events using sequence tracking
      try {
        const hist = await fetchClusterEvents(lastSeqRef.current);
        if (hist && Array.isArray(hist.events) && hist.events.length > 0) {
          const freshItems: ClusterEventItem[] = [];
          for (const ev of hist.events) {
            const evId = ev.event_id || `seq-${ev.sequence}`;
            if (!seenIdsRef.current.has(evId)) {
              seenIdsRef.current.add(evId);
              freshItems.push(mapBackendEvent(ev));
              if (ev.sequence && ev.sequence > lastSeqRef.current) {
                lastSeqRef.current = ev.sequence;
                setLastSeq(ev.sequence);
              }
            }
          }

          if (freshItems.length > 0) {
            setEvents((prev) => [...freshItems, ...prev].slice(0, 200));
          }
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
          if (!cancelled) {
            setIsConnected(true);
            reconnectAttempts = 0;
          }
        };

        ws.onmessage = (event) => {
          if (isPausedRef.current) return;
          try {
            const raw = JSON.parse(event.data);
            const evId = raw.event_id || `seq-${raw.sequence || Date.now()}`;
            if (seenIdsRef.current.has(evId)) return;
            seenIdsRef.current.add(evId);

            if (raw.sequence && raw.sequence > lastSeqRef.current) {
              lastSeqRef.current = raw.sequence;
              setLastSeq(raw.sequence);
            }

            const item = mapBackendEvent(raw);
            setEvents((prev) => [item, ...prev].slice(0, 200));

            // Cross-page sync trigger for significant cluster topology/raft changes
            if (typeof window !== 'undefined') {
              const t = (raw.type || '').toUpperCase();
              if (t.includes('LEADER') || t.includes('FAIL') || t.includes('JOIN') || t.includes('TERM')) {
                window.dispatchEvent(new CustomEvent('orbit:cluster_update', { detail: raw }));
              }
            }
          } catch {}
        };

        ws.onerror = () => {
          if (!cancelled) setIsConnected(false);
        };

        ws.onclose = () => {
          if (!cancelled) {
            setIsConnected(false);
            const backoffMs = Math.min(1000 * Math.pow(1.5, reconnectAttempts), 10000);
            reconnectAttempts++;
            reconnectTimeout = setTimeout(initConnection, backoffMs);
          }
        };
      } catch {
        setIsConnected(false);
        reconnectTimeout = setTimeout(initConnection, 4000);
      }

      // 3. Fallback polling for missed events
      pollTimer = setInterval(async () => {
        if (isPausedRef.current || cancelled) return;
        try {
          const fresh = await fetchClusterEvents(lastSeqRef.current);
          if (fresh && Array.isArray(fresh.events) && fresh.events.length > 0) {
            const freshItems: ClusterEventItem[] = [];
            for (const ev of fresh.events) {
              const evId = ev.event_id || `seq-${ev.sequence}`;
              if (!seenIdsRef.current.has(evId)) {
                seenIdsRef.current.add(evId);
                freshItems.push(mapBackendEvent(ev));
                if (ev.sequence && ev.sequence > lastSeqRef.current) {
                  lastSeqRef.current = ev.sequence;
                  setLastSeq(ev.sequence);
                }
              }
            }

            if (freshItems.length > 0) {
              setEvents((prev) => [...freshItems, ...prev].slice(0, 200));
            }
          }
        } catch {}
      }, 3500);
    }

    initConnection();

    return () => {
      cancelled = true;
      if (wsRef.current) {
        wsRef.current.close();
      }
      if (pollTimer) clearInterval(pollTimer);
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
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
    clearEvents: () => {
      seenIdsRef.current.clear();
      setEvents([]);
    },
  };
}
