'use client';

import React from 'react';
import { Layers, ShieldCheck, Database, RefreshCw, ArrowUpRight } from 'lucide-react';

interface ShardSummaryCardsProps {
  totalShards: number;
  healthyQuorums: number;
  replicationFactor: number;
  activeMigrations: number;
}

export default function ShardSummaryCards({
  totalShards = 3,
  healthyQuorums = 3,
  replicationFactor = 3,
  activeMigrations = 0,
}: ShardSummaryCardsProps) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      {/* CARD 1: TOTAL SHARDS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-all">
        <div className="flex items-center justify-between text-xs font-mono text-[#64748B] mb-2">
          <span>TOTAL SHARDS</span>
          <span className="p-1.5 rounded-lg bg-[#0891D5]/10 text-[#0891D5]">
            <Layers className="w-4 h-4" />
          </span>
        </div>
        <div className="flex items-baseline justify-between mb-2">
          <div className="text-3xl font-bold font-mono text-[#17243A] tracking-tight">
            {totalShards}
          </div>
          <span className="px-2 py-0.5 rounded-full bg-blue-50 border border-blue-200 text-[#0891D5] text-[11px] font-mono font-semibold">
            64-bit Ring
          </span>
        </div>
        {/* Sparkline curve */}
        <div className="h-5 w-full">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 16 Q 25 8, 50 12 T 100 6"
              fill="none"
              stroke="#0891D5"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2 font-mono">
          Uniform partition key distribution
        </p>
      </div>

      {/* CARD 2: QUORUM HEALTH */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-all">
        <div className="flex items-center justify-between text-xs font-mono text-[#64748B] mb-2">
          <span>QUORUM HEALTH</span>
          <span className="p-1.5 rounded-lg bg-[#10B981]/10 text-[#10B981]">
            <ShieldCheck className="w-4 h-4" />
          </span>
        </div>
        <div className="flex items-baseline justify-between mb-2">
          <div className="text-3xl font-bold font-mono text-[#10B981] tracking-tight">
            100%
          </div>
          <span className="px-2 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] text-[11px] font-mono font-semibold">
            {healthyQuorums}/{totalShards} Online
          </span>
        </div>
        {/* Sparkline curve */}
        <div className="h-5 w-full">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 14 Q 30 14, 60 8 T 100 5"
              fill="none"
              stroke="#10B981"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2 font-mono">
          All 3 Raft groups have active leaders
        </p>
      </div>

      {/* CARD 3: REPLICATION FACTOR */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-all">
        <div className="flex items-center justify-between text-xs font-mono text-[#64748B] mb-2">
          <span>REPLICATION FACTOR</span>
          <span className="p-1.5 rounded-lg bg-[#7965E8]/10 text-[#7965E8]">
            <Database className="w-4 h-4" />
          </span>
        </div>
        <div className="flex items-baseline justify-between mb-2">
          <div className="text-3xl font-bold font-mono text-[#7965E8] tracking-tight">
            RF = {replicationFactor}
          </div>
          <span className="px-2 py-0.5 rounded-full bg-purple-50 border border-purple-200 text-[#7965E8] text-[11px] font-mono font-semibold">
            3 Replicas / Shard
          </span>
        </div>
        {/* Sparkline curve */}
        <div className="h-5 w-full">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 15 Q 35 5, 70 12 T 100 8"
              fill="none"
              stroke="#7965E8"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2 font-mono">
          Tolerates 1 node failure per shard
        </p>
      </div>

      {/* CARD 4: REBALANCING & MIGRATIONS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-all">
        <div className="flex items-center justify-between text-xs font-mono text-[#64748B] mb-2">
          <span>ACTIVE MIGRATIONS</span>
          <span className="p-1.5 rounded-lg bg-[#F59E0B]/10 text-[#F59E0B]">
            <RefreshCw className="w-4 h-4" />
          </span>
        </div>
        <div className="flex items-baseline justify-between mb-2">
          <div className="text-3xl font-bold font-mono text-[#17243A] tracking-tight">
            {activeMigrations}
          </div>
          <span className="px-2 py-0.5 rounded-full bg-slate-100 border border-slate-200 text-[#64748B] text-[11px] font-mono font-semibold">
            BALANCED
          </span>
        </div>
        {/* Sparkline curve */}
        <div className="h-5 w-full">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 10 L 100 10"
              fill="none"
              stroke="#64748B"
              strokeWidth="1.5"
              strokeDasharray="4 3"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2 font-mono">
          Zero data transfers in progress
        </p>
      </div>
    </div>
  );
}
