'use client';

import React, { useState } from 'react';
import { Activity, Clock, Filter, AlertTriangle, CheckCircle, Info, ShieldAlert, Cpu } from 'lucide-react';
import { useEventStream } from '@/hooks/useEventStream';

export type EventCategory = 'ALL' | 'RAFT' | 'REPLICATION' | 'STORAGE' | 'CLUSTER' | 'RECOVERY' | 'CHAOS';

interface ClusterEventItem {
  id: string;
  category: Exclude<EventCategory, 'ALL'>;
  timestamp: string;
  node: string;
  level: 'info' | 'success' | 'warn' | 'error';
  message: string;
}

export default function RecentClusterEvents() {
  const [filter, setFilter] = useState<EventCategory>('ALL');
  const { events: liveEvents, isConnected } = useEventStream();

  const defaultEvents: ClusterEventItem[] = [
    {
      id: 'evt-1',
      category: 'RAFT',
      timestamp: '14s ago',
      node: 'Node A',
      level: 'success',
      message: 'Shard 0 heartbeat confirmed across majority quorum (3/3 nodes acked)',
    },
    {
      id: 'evt-2',
      category: 'REPLICATION',
      timestamp: '28s ago',
      node: 'Node D',
      level: 'info',
      message: 'Log index 12450 committed & applied to LSM state machine on Shard 1',
    },
    {
      id: 'evt-3',
      category: 'STORAGE',
      timestamp: '45s ago',
      node: 'Node C',
      level: 'info',
      message: 'Memtable rotation triggered: frozen 4MB flushed to L0 SSTable L0-00042',
    },
    {
      id: 'evt-4',
      category: 'CLUSTER',
      timestamp: '1m ago',
      node: 'Node B',
      level: 'success',
      message: 'Consistent hash ring membership verified: 3 shards mapped with RF=3',
    },
    {
      id: 'evt-5',
      category: 'RECOVERY',
      timestamp: '2m ago',
      node: 'Node E',
      level: 'success',
      message: 'WAL replay completed: 184 uncommitted records recovered to memtable',
    },
    {
      id: 'evt-6',
      category: 'STORAGE',
      timestamp: '3m ago',
      node: 'Node A',
      level: 'warn',
      message: 'Level 0 compaction initiated: merging 4 SSTables into L1',
    },
  ];

  const events: ClusterEventItem[] = liveEvents.length > 0
    ? liveEvents.map((e) => {
        let cat: Exclude<EventCategory, 'ALL'> = 'CLUSTER';
        const c = (e.category || '').toUpperCase();
        if (c.includes('RAFT')) cat = 'RAFT';
        else if (c.includes('REPLICATION')) cat = 'REPLICATION';
        else if (c.includes('STORAGE')) cat = 'STORAGE';
        else if (c.includes('CHAOS')) cat = 'CHAOS';
        else if (c.includes('RECOVERY')) cat = 'RECOVERY';

        let lvl: 'info' | 'success' | 'warn' | 'error' = 'info';
        if (e.severity === 'ERROR') lvl = 'error';
        else if (e.severity === 'WARN') lvl = 'warn';
        else lvl = 'info';

        return {
          id: e.id,
          category: cat,
          timestamp: e.timestamp,
          node: e.nodeId ? e.nodeId.toUpperCase() : 'Cluster',
          level: lvl,
          message: e.description,
        };
      })
    : defaultEvents;

  const filteredEvents =
    filter === 'ALL' ? events : events.filter((e) => e.category === filter);

  const getCategoryBadge = (category: ClusterEventItem['category']) => {
    switch (category) {
      case 'RAFT':
        return 'bg-[#F59E0B]/10 text-[#D97706] border-[#F59E0B]/25';
      case 'REPLICATION':
        return 'bg-[#0891D5]/10 text-[#0891D5] border-[#0891D5]/25';
      case 'STORAGE':
        return 'bg-[#7965E8]/10 text-[#7965E8] border-[#7965E8]/25';
      case 'CLUSTER':
        return 'bg-[#10B981]/10 text-[#10B981] border-[#10B981]/25';
      case 'RECOVERY':
        return 'bg-indigo-50 text-indigo-600 border-indigo-200';
      case 'CHAOS':
        return 'bg-rose-50 text-rose-600 border-rose-200';
      default:
        return 'bg-[#F1F5F9] text-[#64748B] border-[#E2E8F0]';
    }
  };

  const getLevelDot = (level: ClusterEventItem['level']) => {
    switch (level) {
      case 'success':
        return 'bg-[#10B981] shadow-[0_0_6px_rgba(16,185,129,0.5)]';
      case 'warn':
        return 'bg-[#F59E0B] shadow-[0_0_6px_rgba(245,158,11,0.5)]';
      case 'error':
        return 'bg-[#EF4444] shadow-[0_0_6px_rgba(239,68,68,0.5)]';
      case 'info':
      default:
        return 'bg-[#0891D5] shadow-[0_0_6px_rgba(8,145,213,0.5)]';
    }
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs hover:shadow-md transition-shadow flex flex-col justify-between overflow-hidden">
      <div>
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-4 mb-4 border-b border-[#DCE5F0]">
          <div>
            <div className="flex items-center gap-2">
              <div className="w-8 h-8 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
                <Activity className="w-4 h-4 text-[#0891D5]" />
              </div>
              <h3 className="text-base font-semibold text-[#17243A] tracking-tight">
                Recent Cluster Events
              </h3>
              <span className="flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[10px] font-mono font-semibold bg-emerald-50 text-[#10B981] border border-emerald-200">
                <span className="w-1.5 h-1.5 rounded-full bg-[#10B981] animate-pulse" />
                {isConnected ? 'LIVE WS' : 'STREAM BUS'}
              </span>
            </div>
            <p className="text-xs text-[#64748B] mt-1">
              Live audit journal of consensus ticks, storage flushes & network transitions
            </p>
          </div>

          {/* Filter tabs */}
          <div className="flex flex-wrap items-center gap-1 p-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] self-start sm:self-auto font-mono text-[11px]">
            {(['ALL', 'RAFT', 'REPLICATION', 'STORAGE', 'CLUSTER', 'RECOVERY', 'CHAOS'] as const).map((cat) => (
              <button
                key={cat}
                onClick={() => setFilter(cat)}
                className={`px-2 py-1 rounded-lg transition-all ${
                  filter === cat
                    ? 'bg-white text-[#0891D5] shadow-xs font-semibold border border-[#DCE5F0]'
                    : 'text-[#64748B] hover:text-[#17243A]'
                }`}
              >
                {cat}
              </button>
            ))}
          </div>
        </div>

        {/* Events Table / Stream */}
        <div className="space-y-2 max-h-[300px] overflow-y-auto pr-1 font-mono text-xs">
          {filteredEvents.map((evt) => (
            <div
              key={evt.id}
              className="flex items-center justify-between p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] hover:border-[#CBD5E1] transition-colors"
            >
              <div className="flex items-center gap-3">
                <span className={`w-2 h-2 rounded-full ${getLevelDot(evt.level)} shrink-0`} />
                <span
                  className={`text-[9px] px-2 py-0.5 rounded-full border uppercase tracking-wider shrink-0 font-semibold ${getCategoryBadge(
                    evt.category
                  )}`}
                >
                  {evt.category}
                </span>
                <span className="text-[#64748B] text-[11px] shrink-0 font-semibold">
                  [{evt.node}]
                </span>
                <span className="text-[#17243A] text-xs line-clamp-1 font-medium">
                  {evt.message}
                </span>
              </div>

              <span className="text-[11px] text-[#64748B] shrink-0 ml-3">
                {evt.timestamp}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
