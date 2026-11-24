import { TopologyEvent, ConnectionState } from './types';

// Potential cluster endpoints in order of priority (Node A through Node E)
const CLUSTER_API_CANDIDATES = [
  process.env.NEXT_PUBLIC_ORBIT_API,
  'http://localhost:2525',
  'http://localhost:2526',
  'http://localhost:2527',
  'http://localhost:2528',
  'http://localhost:2529',
].filter(Boolean) as string[];

let activeApiEndpoint: string | null = null;

export async function getActiveApiEndpoint(): Promise<string | null> {
  if (activeApiEndpoint) {
    try {
      const res = await fetch(`${activeApiEndpoint}/cluster/health`, {
        method: 'GET',
        headers: { Accept: 'application/json' },
        signal: AbortSignal.timeout(1000),
      });
      if (res.ok) return activeApiEndpoint;
    } catch {
      activeApiEndpoint = null;
    }
  }

  for (const endpoint of CLUSTER_API_CANDIDATES) {
    try {
      const res = await fetch(`${endpoint}/cluster/health`, {
        method: 'GET',
        headers: { Accept: 'application/json' },
        signal: AbortSignal.timeout(800),
      });
      if (res.ok) {
        activeApiEndpoint = endpoint;
        return endpoint;
      }
    } catch {
      // Continue probing next node
    }
  }

  return null;
}

export async function fetchLiveClusterData(): Promise<{
  isLive: boolean;
  health: any | null;
  shards: any | null;
  nodes: any | null;
  events: TopologyEvent[] | null;
}> {
  const endpoint = await getActiveApiEndpoint();
  if (!endpoint) {
    return {
      isLive: false,
      health: null,
      shards: null,
      nodes: null,
      events: null,
    };
  }

  try {
    const [healthRes, shardsRes, nodesRes, eventsRes] = await Promise.allSettled([
      fetch(`${endpoint}/cluster/health`, {
        headers: { Accept: 'application/json' },
        signal: AbortSignal.timeout(1500),
      }).then((r) => (r.ok ? r.json() : null)),

      fetch(`${endpoint}/cluster/shards`, {
        headers: { Accept: 'application/json' },
        signal: AbortSignal.timeout(1500),
      }).then((r) => (r.ok ? r.json() : null)),

      fetch(`${endpoint}/cluster/nodes`, {
        headers: { Accept: 'application/json' },
        signal: AbortSignal.timeout(1500),
      }).then((r) => (r.ok ? r.json() : null)),

      fetch(`${endpoint}/cluster/events`, {
        headers: { Accept: 'application/json' },
        signal: AbortSignal.timeout(1500),
      }).then((r) => (r.ok ? r.json() : null)),
    ]);

    const health = healthRes.status === 'fulfilled' ? healthRes.value : null;
    const shards = shardsRes.status === 'fulfilled' ? shardsRes.value : null;
    const nodes = nodesRes.status === 'fulfilled' ? nodesRes.value : null;
    const rawEvents = eventsRes.status === 'fulfilled' && eventsRes.value?.events ? eventsRes.value.events : null;

    let parsedEvents: TopologyEvent[] | null = null;
    if (Array.isArray(rawEvents)) {
      parsedEvents = rawEvents.map((e: any) => ({
        id: e.event_id || `evt-${Date.now()}-${Math.random()}`,
        timestamp: e.timestamp ? new Date(e.timestamp).toLocaleTimeString('en-US', { hour12: false }) : 'Just now',
        nodeId: e.node_id || 'cluster',
        type: e.type || 'CLUSTER_EVENT',
        category: e.type?.includes('HEARTBEAT')
          ? 'NETWORK'
          : e.type?.includes('LEADER') || e.type?.includes('TERM')
          ? 'RAFT'
          : e.type?.includes('SSTABLE') || e.type?.includes('MEMTABLE')
          ? 'STORAGE'
          : 'CLUSTER',
        description: e.data?.message || e.type || 'Cluster event',
        level: e.type?.includes('ERROR') || e.type?.includes('DISCONNECTED') ? 'error' : 'info',
      }));
    }

    return {
      isLive: health !== null,
      health,
      shards,
      nodes,
      events: parsedEvents,
    };
  } catch {
    return {
      isLive: false,
      health: null,
      shards: null,
      nodes: null,
      events: null,
    };
  }
}

export function subscribeToClusterEvents(
  onEvent: (event: TopologyEvent) => void,
  onStateChange: (state: ConnectionState) => void
): () => void {
  let ws: WebSocket | null = null;
  let retryTimer: NodeJS.Timeout | null = null;
  let isClosed = false;

  const connect = async () => {
    if (isClosed) return;
    const endpoint = await getActiveApiEndpoint();
    if (!endpoint) {
      onStateChange('disconnected');
      if (!isClosed) {
        retryTimer = setTimeout(connect, 4000);
      }
      return;
    }

    const wsUrl = endpoint.replace('http://', 'ws://').replace('https://', 'wss://') + '/ws/events';

    try {
      onStateChange('reconnecting');
      ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        onStateChange('connected');
      };

      ws.onmessage = (event) => {
        try {
          const raw = JSON.parse(event.data);
          const topologyEvt: TopologyEvent = {
            id: raw.event_id || `ws-${Date.now()}`,
            timestamp: raw.timestamp
              ? new Date(raw.timestamp).toLocaleTimeString('en-US', { hour12: false })
              : new Date().toLocaleTimeString('en-US', { hour12: false }),
            nodeId: raw.node_id || 'cluster',
            type: raw.type || 'CLUSTER_EVENT',
            category: raw.type?.includes('HEARTBEAT')
              ? 'NETWORK'
              : raw.type?.includes('LEADER') || raw.type?.includes('TERM')
              ? 'RAFT'
              : raw.type?.includes('SSTABLE') || raw.type?.includes('MEMTABLE')
              ? 'STORAGE'
              : 'CLUSTER',
            description: raw.data?.message || raw.message || raw.type || 'Consensus telemetry event',
            level: raw.type?.includes('ERROR') || raw.type?.includes('DISCONNECTED') ? 'error' : 'info',
          };
          onEvent(topologyEvt);
        } catch {
          // Ignore parse errors
        }
      };

      ws.onerror = () => {
        onStateChange('disconnected');
      };

      ws.onclose = () => {
        onStateChange('disconnected');
        if (!isClosed) {
          retryTimer = setTimeout(connect, 4000);
        }
      };
    } catch {
      onStateChange('disconnected');
      if (!isClosed) {
        retryTimer = setTimeout(connect, 4000);
      }
    }
  };

  connect();

  return () => {
    isClosed = true;
    if (retryTimer) clearTimeout(retryTimer);
    if (ws) {
      ws.close();
      ws = null;
    }
  };
}
