'use client';

import React from 'react';
import { Server, Database, ShieldCheck, Activity, ArrowUpRight } from 'lucide-react';

interface MetricCardProps {
  label: string;
  value: string | number;
  subValue?: string;
  statusText: string;
  statusColor: 'green' | 'blue' | 'purple' | 'amber';
  icon: React.ElementType;
  description: string;
  badge?: string;
}

export default function ClusterHealthCards() {
  const cards: MetricCardProps[] = [
    {
      label: 'PHYSICAL NODES',
      value: '5 / 5',
      subValue: '100% Online',
      statusText: 'All nodes connected',
      statusColor: 'green',
      icon: Server,
      description: 'Nodes A, B, C, D, E active across cluster network',
      badge: '5 HEALTHY',
    },
    {
      label: 'LOGICAL SHARDS',
      value: '3 / 3',
      subValue: 'RF = 3 Replicated',
      statusText: 'All shards operational',
      statusColor: 'blue',
      icon: Database,
      description: 'Murmur3-partitioned ring with uniform distribution',
      badge: 'QUORUM OK',
    },
    {
      label: 'RAFT LEADERS',
      value: '3 / 3',
      subValue: '0 Elections Pending',
      statusText: 'One leader per shard',
      statusColor: 'purple',
      icon: ShieldCheck,
      description: 'Stable terms, zero split-brain, heartbeat tick 50ms',
      badge: '3 STABLE',
    },
    {
      label: 'CLUSTER HEALTH',
      value: '100%',
      subValue: 'Zero Warnings',
      statusText: 'Fully Operational',
      statusColor: 'green',
      icon: Activity,
      description: 'All monitored components, WAL disks & LSM engines healthy',
      badge: 'OPTIMAL',
    },
  ];

  const getColorClasses = (color: MetricCardProps['statusColor']) => {
    switch (color) {
      case 'green':
        return {
          badgeBg: 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400',
          dot: 'bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.6)]',
          icon: 'text-emerald-400 bg-emerald-500/10 border-emerald-500/20',
          gradient: 'from-emerald-500/5 to-transparent',
        };
      case 'blue':
        return {
          badgeBg: 'bg-sky-500/10 border-sky-500/30 text-sky-400',
          dot: 'bg-sky-400 shadow-[0_0_8px_rgba(56,189,248,0.6)]',
          icon: 'text-sky-400 bg-sky-500/10 border-sky-500/20',
          gradient: 'from-sky-500/5 to-transparent',
        };
      case 'purple':
        return {
          badgeBg: 'bg-indigo-500/10 border-indigo-500/30 text-indigo-400',
          dot: 'bg-indigo-400 shadow-[0_0_8px_rgba(129,140,248,0.6)]',
          icon: 'text-indigo-400 bg-indigo-500/10 border-indigo-500/20',
          gradient: 'from-indigo-500/5 to-transparent',
        };
      case 'amber':
      default:
        return {
          badgeBg: 'bg-amber-500/10 border-amber-500/30 text-amber-400',
          dot: 'bg-amber-400 shadow-[0_0_8px_rgba(251,191,36,0.6)]',
          icon: 'text-amber-400 bg-amber-500/10 border-amber-500/20',
          gradient: 'from-amber-500/5 to-transparent',
        };
    }
  };

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
      {cards.map((card, i) => {
        const Icon = card.icon;
        const color = getColorClasses(card.statusColor);

        return (
          <div
            key={i}
            className="group relative bg-[#0a0f1d]/80 backdrop-blur-md border border-slate-800/80 hover:border-slate-700/80 rounded-xl p-5 transition-all duration-200 hover:shadow-[0_8px_30px_rgb(0,0,0,0.4)] flex flex-col justify-between overflow-hidden"
          >
            {/* Top ambient highlight */}
            <div className={`absolute -top-12 -right-12 w-28 h-28 rounded-full blur-2xl pointer-events-none opacity-40 bg-gradient-to-br ${color.gradient}`} />

            <div>
              <div className="flex items-center justify-between mb-3">
                <span className="text-[11px] font-mono font-semibold tracking-wider text-slate-400">
                  {card.label}
                </span>
                <div className={`w-8 h-8 rounded-lg flex items-center justify-center border ${color.icon}`}>
                  <Icon className="w-4 h-4" />
                </div>
              </div>

              <div className="flex items-baseline gap-2 mb-1">
                <span className="text-3xl font-extrabold text-white tracking-tight">
                  {card.value}
                </span>
                {card.subValue && (
                  <span className="text-xs font-mono text-slate-400">
                    {card.subValue}
                  </span>
                )}
              </div>

              <p className="text-xs text-slate-400 line-clamp-1 mb-3">
                {card.description}
              </p>
            </div>

            <div className="pt-3 border-t border-slate-800/60 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className={`w-2 h-2 rounded-full ${color.dot}`} />
                <span className="text-xs font-medium text-slate-300">
                  {card.statusText}
                </span>
              </div>
              {card.badge && (
                <span className={`text-[10px] font-mono uppercase px-2 py-0.5 rounded-full border ${color.badgeBg}`}>
                  {card.badge}
                </span>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
