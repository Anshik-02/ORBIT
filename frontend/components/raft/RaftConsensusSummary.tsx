'use client';

import React from 'react';
import { Crown, ShieldCheck, Vote, Activity, Zap } from 'lucide-react';

interface RaftConsensusSummaryProps {
  activeGroups?: number;
  activeLeaders?: number;
  totalElections?: number;
  replicationHealth?: number;
}

export default function RaftConsensusSummary({
  activeGroups = 3,
  activeLeaders = 3,
  totalElections = 8,
  replicationHealth = 100,
}: RaftConsensusSummaryProps) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      {/* CARD 1: ACTIVE RAFT GROUPS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-all">
        <div className="flex items-center justify-between text-xs font-mono text-[#64748B] mb-2">
          <span>ACTIVE RAFT GROUPS</span>
          <span className="p-1.5 rounded-lg bg-[#0891D5]/10 text-[#0891D5]">
            <Zap className="w-4 h-4" />
          </span>
        </div>
        <div className="flex items-baseline justify-between mb-2">
          <div className="text-3xl font-bold font-mono text-[#17243A] tracking-tight">
            {activeGroups}
          </div>
          <span className="px-2 py-0.5 rounded-full bg-blue-50 border border-blue-200 text-[#0891D5] text-[11px] font-mono font-semibold">
            Multi-Raft
          </span>
        </div>
        <div className="h-5 w-full">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 14 Q 25 8, 50 12 T 100 6"
              fill="none"
              stroke="#0891D5"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2 font-mono">
          Independent consensus per shard
        </p>
      </div>

      {/* CARD 2: ACTIVE LEADERS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-all">
        <div className="flex items-center justify-between text-xs font-mono text-[#64748B] mb-2">
          <span>ELECTED LEADERS</span>
          <span className="p-1.5 rounded-lg bg-[#F59E0B]/10 text-[#F59E0B]">
            <Crown className="w-4 h-4" />
          </span>
        </div>
        <div className="flex items-baseline justify-between mb-2">
          <div className="text-3xl font-bold font-mono text-[#D97706] tracking-tight">
            {activeLeaders} / {activeGroups}
          </div>
          <span className="px-2 py-0.5 rounded-full bg-amber-50 border border-amber-200 text-[#D97706] text-[11px] font-mono font-semibold">
            100% Stability
          </span>
        </div>
        <div className="h-5 w-full">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 15 Q 30 15, 60 7 T 100 5"
              fill="none"
              stroke="#F59E0B"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2 font-mono">
          Zero leaderless partitions detected
        </p>
      </div>

      {/* CARD 3: TOTAL ELECTIONS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-all">
        <div className="flex items-center justify-between text-xs font-mono text-[#64748B] mb-2">
          <span>TERM ELECTIONS</span>
          <span className="p-1.5 rounded-lg bg-[#7965E8]/10 text-[#7965E8]">
            <Vote className="w-4 h-4" />
          </span>
        </div>
        <div className="flex items-baseline justify-between mb-2">
          <div className="text-3xl font-bold font-mono text-[#7965E8] tracking-tight">
            {totalElections}
          </div>
          <span className="px-2 py-0.5 rounded-full bg-purple-50 border border-purple-200 text-[#7965E8] text-[11px] font-mono font-semibold">
            Cluster Lifetime
          </span>
        </div>
        <div className="h-5 w-full">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 16 Q 35 12, 70 8 T 100 7"
              fill="none"
              stroke="#7965E8"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2 font-mono">
          Average term lifespan: &gt; 4.2 hours
        </p>
      </div>

      {/* CARD 4: REPLICATION HEALTH */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-all">
        <div className="flex items-center justify-between text-xs font-mono text-[#64748B] mb-2">
          <span>REPLICATION HEALTH</span>
          <span className="p-1.5 rounded-lg bg-[#10B981]/10 text-[#10B981]">
            <ShieldCheck className="w-4 h-4" />
          </span>
        </div>
        <div className="flex items-baseline justify-between mb-2">
          <div className="text-3xl font-bold font-mono text-[#10B981] tracking-tight">
            {replicationHealth}%
          </div>
          <span className="px-2 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] text-[11px] font-mono font-semibold">
            Quorum Synced
          </span>
        </div>
        <div className="h-5 w-full">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 10 L 100 10"
              fill="none"
              stroke="#10B981"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2 font-mono">
          CommitIndex == AppliedIndex across followers
        </p>
      </div>
    </div>
  );
}
