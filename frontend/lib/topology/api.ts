import { TopologyEvent, ConnectionState } from './types';

const DEFAULT_API_URL = process.env.NEXT_PUBLIC_ORBIT_API || 'http://localhost:8001';
const DEFAULT_WS_URL = process.env.NEXT_PUBLIC_ORBIT_WS || 'ws://localhost:8001/ws/events';

export async function fetchClusterStatus(): Promise<any> {
  try {
    const res = await fetch(`${DEFAULT_API_URL}/cluster/status`, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      next: { revalidate: 2 },
    });
    if (res.ok) {
      return await res.json();
    }
  } catch {
    // Backend offline / development mode
  }
  return null;
}

export async function fetchClusterHealth(): Promise<any> {
  try {
    const res = await fetch(`${DEFAULT_API_URL}/cluster/health`, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      next: { revalidate: 2 },
    });
    if (res.ok) {
      return await res.json();
    }
  } catch {
    // Backend offline
  }
  return null;
}

export function subscribeToClusterEvents(
  onEvent: (event: TopologyEvent) => void,
  onStateChange: (state: ConnectionState) => void
): () => void {
  let ws: WebSocket | null = null;
  let retryTimer: NodeJS.Timeout | null = null;
  let isClosed = false;

  const connect = () => {
    if (isClosed) return;
    try {
      onStateChange('reconnecting');
      ws = new WebSocket(DEFAULT_WS_URL);

      ws.onopen = () => {
        onStateChange('connected');
      };

      ws.onmessage = (event) => {
        try {
          const raw = JSON.parse(event.data);
          const topologyEvt: TopologyEvent = {
            id: raw.event_id || `ws-${Date.now()}`,
            timestamp: new Date().toLocaleTimeString('en-US', { hour12: false }),
            nodeId: raw.node_id || 'cluster',
            type: raw.type || 'CLUSTER_EVENT',
            category: raw.type?.includes('HEARTBEAT')
              ? 'NETWORK'
              : raw.type?.includes('LEADER') || raw.type?.includes('TERM')
              ? 'RAFT'
              : raw.type?.includes('SSTABLE') || raw.type?.includes('MEMTABLE')
              ? 'STORAGE'
              : 'CLUSTER',
            description: raw.message || raw.type || 'Cluster telemetry tick',
            level: raw.type?.includes('ERROR') || raw.type?.includes('OFFLINE') ? 'error' : 'info',
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
