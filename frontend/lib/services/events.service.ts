/**
 * Events Service
 * Historical event retrieval, CSV/JSON export, and WebSocket stream orchestration.
 */

import { fetchClusterEvents } from '../api/client';
import { ClusterEvent, RequestOptions } from '../api/types';

export class EventsService {
  /**
   * Fetch historical events since sequence number.
   */
  public static async getEvents(since: number = 0, options?: RequestOptions): Promise<ClusterEvent[]> {
    const res = await fetchClusterEvents(since, options);
    return res?.events || [];
  }

  /**
   * Export cluster events to downloadable JSON.
   */
  public static exportToJson(events: ClusterEvent[], filename = 'orbit-cluster-events.json') {
    const blob = new Blob([JSON.stringify(events, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }

  /**
   * Export cluster events to downloadable CSV.
   */
  public static exportToCsv(events: ClusterEvent[], filename = 'orbit-cluster-events.csv') {
    const headers = ['Sequence', 'Timestamp', 'Event ID', 'Type', 'Node ID', 'Shard ID', 'Severity'];
    const rows = events.map((e) => [
      e.sequence,
      e.timestamp,
      e.event_id,
      e.type,
      e.node_id,
      e.shard_id ?? '',
      e.severity ?? 'INFO',
    ]);

    const csvContent = [headers.join(','), ...rows.map((r) => r.map((c) => `"${c}"`).join(','))].join('\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }
}
