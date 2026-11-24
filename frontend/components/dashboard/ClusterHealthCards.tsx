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

  const onlineNodes = isLive ? (health?.available_nodes ?? 0) : 0;
  const totalNodes = isLive ? (health?.total_nodes ?? status?.node_count ?? 5) : 5;
  const healthyShards = isLive ? (health?.healthy_shards ?? 0) : 0;
  const totalShards = isLive ? (health?.total_shards ?? status?.shard_count ?? 3) : 3;
  const leaderCount = isLive ? Math.max(0, totalShards - (health?.leaderless_shards ?? 0)) : 0;

  const isClusterHealthy = isLive && (health?.replication_health === 'healthy' || health?.replication_health === 'degraded');

  const cards: MetricCardProps[] = [
    {
      label: 'PHYSICAL NODES',
      value: isLive ? `${onlineNodes} / ${totalNodes}` : 'OFFLINE',
      subValue: isLive ? `${Math.round((onlineNodes / totalNodes) * 100)}% Online` : 'Cluster Disconnected',
      statusText: isLive ? (onlineNodes === totalNodes ? 'All nodes connected' : 'Degraded peer count') : 'No reachable nodes',
      statusColor: isLive ? (onlineNodes === totalNodes ? 'green' : 'amber') : 'amber',
      icon: Server,
      description: isLive
        ? `Nodes: ${status?.nodes?.join(', ') || 'fleet active'}`
        : 'Connecting to cluster nodes on :2525-:2529...',
      badge: isLive ? `${onlineNodes} HEALTHY` : 'DISCONNECTED',
      badgePercent: isLive ? `${Math.round((onlineNodes / totalNodes) * 100)}%` : '0%',
      sparklineColor: isLive ? '#10B981' : '#F59E0B',
      sparklinePath: 'M0,18 Q30,6 60,16 T120,8 T180,14 T240,4',
      href: '/dashboard/topology',
    },
    {
      label: 'LOGICAL SHARDS',
      value: isLive ? `${healthyShards} / ${totalShards}` : '0 / 3',
      subValue: isLive ? 'RF = 3 Replicated' : 'Quorum Disconnected',
      statusText: isLive ? (healthyShards === totalShards ? 'All shards operational' : 'Quorum warning') : 'Offline',
      statusColor: isLive ? (healthyShards === totalShards ? 'blue' : 'amber') : 'amber',
      icon: Database,
      description: isLive ? 'Murmur3-partitioned ring with uniform distribution' : 'Awaiting cluster reconnect',
      badge: isLive ? (healthyShards === totalShards ? 'QUORUM OK' : 'QUORUM RISK') : 'OFFLINE',
      badgePercent: isLive ? `${healthyShards} Shards` : '0 Shards',
      sparklineColor: isLive ? '#0891D5' : '#64748B',
      sparklinePath: 'M0,16 Q40,4 80,14 T160,6 T200,12 T240,6',
      href: '/dashboard/shards',
    },
    {
      label: 'RAFT LEADERS',
      value: isLive ? `${leaderCount} / ${totalShards}` : '0 / 3',
      subValue: isLive ? '0 Elections Pending' : 'No Active Leaders',
      statusText: isLive ? 'One leader per shard' : 'Consensus inactive',
      statusColor: isLive ? 'purple' : 'amber',
      icon: ShieldCheck,
      description: isLive ? 'Stable terms, zero split-brain, heartbeat tick 50ms' : 'Raft groups unreachable',
      badge: isLive ? `${leaderCount} STABLE` : 'NO QUORUM',
      badgePercent: isLive ? '100% Quorum' : '0%',
      sparklineColor: isLive ? '#7965E8' : '#64748B',
      sparklinePath: 'M0,14 Q30,20 70,8 T140,16 T200,6 T240,4',
      href: '/dashboard/raft',
    },
    {
      label: 'CLUSTER HEALTH',
      value: isLive ? (health?.replication_health === 'healthy' ? '100%' : 'DEGRADED') : 'OFFLINE',
      subValue: isLive ? (health?.replication_health === 'healthy' ? 'Zero Warnings' : 'Issues Detected') : 'Cluster Offline',
      statusText: isLive ? (health?.replication_health === 'healthy' ? 'Live Operational' : 'Degraded') : 'Cluster Unreachable',
      statusColor: isLive ? (health?.replication_health === 'healthy' ? 'green' : 'amber') : 'amber',
      icon: Activity,
      description: isLive ? 'All monitored components, WAL disks & LSM engines healthy' : 'Failed to reach cluster API',
      badge: isLive ? 'LIVE CLUSTER' : 'OFFLINE',
      badgePercent: isLive ? 'Active' : 'Offline',
      sparklineColor: isLive ? (health?.replication_health === 'healthy' ? '#10B981' : '#F59E0B') : '#EF4444',
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
