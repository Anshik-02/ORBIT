'use client';

import React, { useState } from 'react';
import { Activity, Clock, Filter, AlertTriangle, CheckCircle, Info, ShieldAlert, Cpu } from 'lucide-react';

export type EventCategory = 'ALL' | 'RAFT' | 'REPLICATION' | 'STORAGE' | 'CLUSTER' | 'RECOVERY' | 'CHAOS';

interface ClusterEvent {
  id: string;
  category: Exclude<EventCategory, 'ALL'>;
  timestamp: string;
  node: string;
  level: 'info' | 'success' | 'warn' | 'error';
  message: string;
}

export default function RecentClusterEvents() {
  const [filter, setFilter] = useState<EventCategory>('ALL');

  const events: ClusterEvent[] = [
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
      level: 'info',
      message: 'Minor compaction finished: 3 L0 SSTables merged into L1-00018 (reduced read amp)',
    },
    {
      id: 'evt-7',
      category: 'CHAOS',
      timestamp: '4m ago',
      node: 'Cluster',
      level: 'info',
      message: 'Chaos experiment "Follower Crash Simulation" healed with zero data loss',
    },
    {
      id: 'evt-8',
      category: 'RAFT',
      timestamp: '6m ago',
      node: 'Node B',
      level: 'success',
      message: 'Shard 2 leader election concluded: Node B elected leader for Term 5',
    },
    {
      id: 'evt-9',
      category: 'REPLICATION',
      timestamp: '8m ago',
      node: 'Node E',
      level: 'info',
      message: 'Catch-up stream completed: 12 log entries synchronized from leader Node D',
    },
  ];

  const filteredEvents =
    filter === 'ALL' ? events : events.filter((e) => e.category === filter);

  const getCategoryBadge = (category: ClusterEvent['category']) => {
    switch (category) {
      case 'RAFT':
        return 'bg-amber-500/10 text-amber-400 border-amber-500/30';
      case 'REPLICATION':
        return 'bg-sky-500/10 text-sky-400 border-sky-500/30';
      case 'STORAGE':
        return 'bg-indigo-500/10 text-indigo-400 border-indigo-500/30';
      case 'CLUSTER':
        return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30';
      case 'RECOVERY':
        return 'bg-purple-500/10 text-purple-400 border-purple-500/30';
      case 'CHAOS':
        return 'bg-rose-500/10 text-rose-400 border-rose-500/30';
      default:
        return 'bg-slate-800 text-slate-300 border-slate-700';
    }
  };

  const getLevelDot = (level: ClusterEvent['level']) => {
    switch (level) {
      case 'success':
        return 'bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.7)]';
      case 'warn':
        return 'bg-amber-400 shadow-[0_0_6px_rgba(251,191,36,0.7)]';
      case 'error':
        return 'bg-rose-400 shadow-[0_0_6px_rgba(244,63,94,0.7)]';
      case 'info':
      default:
        return 'bg-sky-400 shadow-[0_0_6px_rgba(56,189,248,0.7)]';
    }
  };

  return (
    <div className="bg-[#0a0f1d]/80 backdrop-blur-md border border-slate-800/80 rounded-xl p-5 flex flex-col justify-between overflow-hidden">
      <div>
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 mb-3 border-b border-slate-800/80">
          <div>
            <div className="flex items-center gap-2">
              <Activity className="w-4 h-4 text-sky-400" />
              <h3 className="text-base font-semibold text-white tracking-wide">
                Recent Cluster Events
              </h3>
            </div>
            <p className="text-xs text-slate-400 mt-0.5">
              Live audit journal of consensus ticks, storage flushes & network transitions
            </p>
          </div>

          {/* Filter tabs */}
          <div className="flex flex-wrap items-center gap-1 p-1 rounded-lg bg-slate-900/90 border border-slate-800 self-start sm:self-auto font-mono text-[11px]">
            {(['ALL', 'RAFT', 'REPLICATION', 'STORAGE', 'CLUSTER', 'RECOVERY', 'CHAOS'] as const).map((cat) => (
              <button
                key={cat}
                onClick={() => setFilter(cat)}
                className={`px-2 py-0.5 rounded transition-colors ${
                  filter === cat
                    ? 'bg-sky-500/20 text-sky-300 border border-sky-500/30 font-semibold'
                    : 'text-slate-400 hover:text-slate-200'
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
              className="flex items-center justify-between p-2.5 rounded-lg bg-slate-950/40 border border-slate-800/70 hover:border-slate-700/80 transition-colors"
            >
              <div className="flex items-center gap-3">
                <span className={`w-2 h-2 rounded-full ${getLevelDot(evt.level)} shrink-0`} />
                <span
                  className={`text-[9px] px-1.5 py-0.5 rounded border uppercase tracking-wider shrink-0 font-semibold ${getCategoryBadge(
                    evt.category
                  )}`}
                >
                  {evt.category}
                </span>
                <span className="text-slate-400 text-[11px] shrink-0 font-semibold text-slate-300">
                  [{evt.node}]
                </span>
                <span className="text-slate-300 text-xs line-clamp-1">
                  {evt.message}
                </span>
              </div>

              <span className="text-[11px] text-slate-400 shrink-0 ml-3">
                {evt.timestamp}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
