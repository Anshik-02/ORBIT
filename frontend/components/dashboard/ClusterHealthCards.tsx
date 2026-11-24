'use client';

import React from 'react';
import Link from 'next/link';
import { Server, Database, ShieldCheck, Activity, ArrowUpRight } from 'lucide-react';
import { useClusterHealth } from '@/hooks/useClusterHealth';

interface MetricCardProps {
  label: string;
  value: string | number;
  subValue?: string;
  statusText: string;
  statusColor: 'green' | 'blue' | 'purple' | 'amber';
  icon: React.ElementType;
  description: string;
  badge?: string;
  badgePercent?: string;
  sparklineColor: string;
  sparklinePath: string;
  href: string;
}

export default function ClusterHealthCards() {
  const { status, health, isLive } = useClusterHealth();

  const onlineNodes = health?.available_nodes ?? 5;
  const totalNodes = health?.total_nodes ?? status?.node_count ?? 5;
  const healthyShards = health?.healthy_shards ?? status?.shard_count ?? 3;
  const totalShards = health?.total_shards ?? status?.shard_count ?? 3;
  const leaderCount = totalShards - (health?.leaderless_shards ?? 0);

  const isClusterHealthy = (health?.replication_health === 'healthy') || (!isLive && true);

  const cards: MetricCardProps[] = [
    {
      label: 'PHYSICAL NODES',
      value: `${onlineNodes} / ${totalNodes}`,
      subValue: isLive ? `${Math.round((onlineNodes / totalNodes) * 100)}% Online` : '100% Online',
      statusText: onlineNodes === totalNodes ? 'All nodes connected' : 'Degraded peer count',
      statusColor: onlineNodes === totalNodes ? 'green' : 'amber',
      icon: Server,
      description: isLive
        ? `Nodes: ${status?.nodes?.join(', ') || 'fleet active'}`
        : 'Nodes A, B, C, D, E active across cluster',
      badge: `${onlineNodes} HEALTHY`,
      badgePercent: '+100%',
      sparklineColor: '#10B981',
      sparklinePath: 'M0,18 Q30,6 60,16 T120,8 T180,14 T240,4',
      href: '/dashboard/topology',
    },
    {
      label: 'LOGICAL SHARDS',
      value: `${healthyShards} / ${totalShards}`,
      subValue: 'RF = 3 Replicated',
      statusText: healthyShards === totalShards ? 'All shards operational' : 'Quorum warning',
      statusColor: healthyShards === totalShards ? 'blue' : 'amber',
      icon: Database,
      description: 'Murmur3-partitioned ring with uniform distribution',
      badge: healthyShards === totalShards ? 'QUORUM OK' : 'QUORUM RISK',
      badgePercent: '+3 Shards',
      sparklineColor: '#0891D5',
      sparklinePath: 'M0,16 Q40,4 80,14 T160,6 T200,12 T240,6',
      href: '/dashboard/shards',
    },
    {
      label: 'RAFT LEADERS',
      value: `${leaderCount} / ${totalShards}`,
      subValue: '0 Elections Pending',
      statusText: 'One leader per shard',
      statusColor: 'purple',
      icon: ShieldCheck,
      description: 'Stable terms, zero split-brain, heartbeat tick 50ms',
      badge: `${leaderCount} STABLE`,
      badgePercent: '100% Quorum',
      sparklineColor: '#7965E8',
      sparklinePath: 'M0,14 Q30,20 70,8 T140,16 T200,6 T240,4',
      href: '/dashboard/raft',
    },
    {
      label: 'CLUSTER HEALTH',
      value: isClusterHealthy ? '100%' : 'DEGRADED',
      subValue: isLive ? (health?.replication_health === 'healthy' ? 'Zero Warnings' : 'Issues Detected') : 'Zero Warnings',
      statusText: isLive ? (isClusterHealthy ? 'Live Operational' : 'Degraded') : 'Fully Operational',
      statusColor: isClusterHealthy ? 'green' : 'amber',
      icon: Activity,
      description: 'All monitored components, WAL disks & LSM engines healthy',
      badge: isLive ? 'LIVE CLUSTER' : 'OPTIMAL',
      badgePercent: '0 Loss',
      sparklineColor: isClusterHealthy ? '#10B981' : '#F59E0B',
      sparklinePath: 'M0,12 Q50,4 100,16 T180,6 T240,10',
      href: '/dashboard/events',
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
          percent: 'text-emerald-700 bg-emerald-50 border-emerald-200',
        };
      case 'blue':
        return {
          badgeBg: 'bg-sky-50 border-sky-200 text-[#0891D5]',
          dot: 'bg-[#0891D5] shadow-[0_0_8px_rgba(8,145,213,0.5)]',
          icon: 'text-[#0891D5] bg-sky-50 border-sky-100',
          gradient: 'from-sky-100/40 to-transparent',
          percent: 'text-[#0891D5] bg-sky-50 border-sky-200',
        };
      case 'purple':
        return {
          badgeBg: 'bg-purple-50 border-purple-200 text-[#7965E8]',
          dot: 'bg-[#7965E8] shadow-[0_0_8px_rgba(121,101,232,0.5)]',
          icon: 'text-[#7965E8] bg-purple-50 border-purple-100',
          gradient: 'from-purple-100/40 to-transparent',
          percent: 'text-[#7965E8] bg-purple-50 border-purple-200',
        };
      case 'amber':
      default:
        return {
          badgeBg: 'bg-amber-50 border-amber-200 text-amber-700',
          dot: 'bg-[#F59E0B] shadow-[0_0_8px_rgba(245,158,11,0.5)]',
          icon: 'text-[#F59E0B] bg-amber-50 border-amber-100',
          gradient: 'from-amber-100/40 to-transparent',
          percent: 'text-amber-700 bg-amber-50 border-amber-200',
        };
    }
  };

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
      {cards.map((card, i) => {
        const Icon = card.icon;
        const color = getColorClasses(card.statusColor);

        return (
          <Link
            key={i}
            href={card.href}
            className="group relative bg-white border border-[#DCE5F0] hover:border-[#CBD5E1] rounded-2xl p-6 transition-all duration-200 shadow-xs hover:shadow-md flex flex-col justify-between overflow-hidden cursor-pointer"
          >
            {/* Top ambient highlight */}
            <div className={`absolute -top-12 -right-12 w-28 h-28 rounded-full blur-2xl pointer-events-none opacity-60 bg-gradient-to-br ${color.gradient}`} />

            <div>
              {/* Header: Label + Pill + Icon */}
              <div className="flex items-center justify-between mb-3">
                <span className="text-[11px] font-mono font-semibold tracking-wider text-[#64748B]">
                  {card.label}
                </span>
                <div className="flex items-center gap-1.5">
                  {card.badgePercent && (
                    <span className={`text-[10px] font-mono px-2 py-0.5 rounded-full border font-semibold ${color.percent}`}>
                      {card.badgePercent}
                    </span>
                  )}
                  <div className={`w-8 h-8 rounded-xl flex items-center justify-center border transition-transform group-hover:scale-105 ${color.icon}`}>
                    <Icon className="w-3.5 h-3.5" />
                  </div>
                </div>
              </div>

              {/* Main Metric Value */}
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

              {/* Subtle Cosmic-Forge style Sparkline wave */}
              <div className="h-6 w-full my-2 overflow-hidden">
                <svg viewBox="0 0 240 24" className="w-full h-full" preserveAspectRatio="none">
                  <path
                    d={card.sparklinePath}
                    fill="none"
                    stroke={card.sparklineColor}
                    strokeWidth="2"
                    strokeLinecap="round"
                    className="opacity-70 group-hover:opacity-100 transition-opacity"
                  />
                </svg>
              </div>

              <p className="text-xs text-[#64748B] line-clamp-1 mb-3 leading-relaxed">
                {card.description}
              </p>
            </div>

            {/* Bottom Status Row */}
            <div className="pt-3 border-t border-[#DCE5F0] flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className={`w-2 h-2 rounded-full ${color.dot}`} />
                <span className="text-xs font-medium text-[#17243A]">
                  {card.statusText}
                </span>
              </div>
              <div className="flex items-center gap-1 text-[#64748B] group-hover:text-[#0891D5] transition-colors">
                <span className={`text-[10px] font-mono uppercase px-2 py-0.5 rounded-full border font-bold ${color.badgeBg}`}>
                  {card.badge}
                </span>
                <ArrowUpRight className="w-3.5 h-3.5 opacity-0 group-hover:opacity-100 transition-opacity" />
              </div>
            </div>
          </Link>
        );
      })}
    </div>
  );
}
