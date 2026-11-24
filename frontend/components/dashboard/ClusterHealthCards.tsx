'use client';

import React from 'react';
import { Server, Database, ShieldCheck, Activity } from 'lucide-react';

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
          badgeBg: 'bg-emerald-50 border-emerald-200 text-emerald-700',
          dot: 'bg-[#10B981] shadow-[0_0_8px_rgba(16,185,129,0.5)]',
          icon: 'text-[#10B981] bg-emerald-50 border-emerald-100',
          gradient: 'from-emerald-100/40 to-transparent',
        };
      case 'blue':
        return {
          badgeBg: 'bg-sky-50 border-sky-200 text-[#0891D5]',
          dot: 'bg-[#0891D5] shadow-[0_0_8px_rgba(8,145,213,0.5)]',
          icon: 'text-[#0891D5] bg-sky-50 border-sky-100',
          gradient: 'from-sky-100/40 to-transparent',
        };
      case 'purple':
        return {
          badgeBg: 'bg-purple-50 border-purple-200 text-[#7965E8]',
          dot: 'bg-[#7965E8] shadow-[0_0_8px_rgba(121,101,232,0.5)]',
          icon: 'text-[#7965E8] bg-purple-50 border-purple-100',
          gradient: 'from-purple-100/40 to-transparent',
        };
      case 'amber':
      default:
        return {
          badgeBg: 'bg-amber-50 border-amber-200 text-amber-700',
          dot: 'bg-[#F59E0B] shadow-[0_0_8px_rgba(245,158,11,0.5)]',
          icon: 'text-[#F59E0B] bg-amber-50 border-amber-100',
          gradient: 'from-amber-100/40 to-transparent',
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
            className="group relative bg-white border border-[#DCE5F0] hover:border-[#CBD5E1] rounded-2xl p-6 transition-all duration-200 shadow-sm hover:shadow-md flex flex-col justify-between overflow-hidden"
          >
            {/* Top ambient highlight */}
            <div className={`absolute -top-12 -right-12 w-28 h-28 rounded-full blur-2xl pointer-events-none opacity-60 bg-gradient-to-br ${color.gradient}`} />

            <div>
              <div className="flex items-center justify-between mb-4">
                <span className="text-[11px] font-mono font-semibold tracking-wider text-[#64748B]">
                  {card.label}
                </span>
                <div className={`w-9 h-9 rounded-xl flex items-center justify-center border ${color.icon}`}>
                  <Icon className="w-4 h-4" />
                </div>
              </div>

              <div className="flex items-baseline gap-2 mb-1.5">
                <span className="text-3xl font-extrabold text-[#17243A] tracking-tight">
                  {card.value}
                </span>
                {card.subValue && (
                  <span className="text-xs font-mono text-[#64748B]">
                    {card.subValue}
                  </span>
                )}
              </div>

              <p className="text-xs text-[#64748B] line-clamp-1 mb-4 leading-relaxed">
                {card.description}
              </p>
            </div>

            <div className="pt-3.5 border-t border-[#DCE5F0] flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className={`w-2 h-2 rounded-full ${color.dot}`} />
                <span className="text-xs font-medium text-[#17243A]">
                  {card.statusText}
                </span>
              </div>
              {card.badge && (
                <span className={`text-[10px] font-mono uppercase px-2.5 py-0.5 rounded-full border font-bold ${color.badgeBg}`}>
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
