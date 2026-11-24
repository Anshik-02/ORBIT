'use client';

import React from 'react';
import { Server, CheckCircle2, AlertOctagon, Layers } from 'lucide-react';
import { ClusterSummaryData } from '../../lib/topology/types';

interface TopologySummaryProps {
  summary: ClusterSummaryData;
}

export default function TopologySummary({ summary }: TopologySummaryProps) {
  const isDegraded = summary.offlineNodes > 0;
  const isUnavailable = summary.replicationHealth === 'unavailable';

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      {/* Card 1: Physical Nodes */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-shadow">
        <div className="flex items-center justify-between mb-3">
          <span className="text-[11px] font-mono uppercase tracking-wider text-[#64748B] font-semibold">
            PHYSICAL NODES
          </span>
          <div className="w-8 h-8 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
            <Server className="w-4 h-4 text-[#0891D5]" />
          </div>
        </div>
        <div className="flex items-baseline gap-2">
          <span className="text-3xl font-extrabold text-[#17243A] tracking-tight font-mono">
            {String(summary.totalNodes).padStart(2, '0')}
          </span>
          <span className="text-xs text-[#64748B]">Total machines</span>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2 border-t border-[#F1F5F9] pt-2">
          Registered cluster infrastructure
        </p>
      </div>

      {/* Card 2: Online Nodes */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-shadow">
        <div className="flex items-center justify-between mb-3">
          <span className="text-[11px] font-mono uppercase tracking-wider text-[#64748B] font-semibold">
            ONLINE NODES
          </span>
          <div className="w-8 h-8 rounded-lg bg-[#10B981]/10 flex items-center justify-center">
            <CheckCircle2 className="w-4 h-4 text-[#10B981]" />
          </div>
        </div>
        <div className="flex items-baseline gap-2">
          <span className="text-3xl font-extrabold text-[#10B981] tracking-tight font-mono">
            {String(summary.availableNodes).padStart(2, '0')}
          </span>
          <span className="text-xs text-[#64748B]">Operational</span>
        </div>
        <p className="text-[11px] text-[#10B981] mt-2 border-t border-[#F1F5F9] pt-2 font-medium">
          Passing heartbeat liveness probes
        </p>
      </div>

      {/* Card 3: Offline Nodes */}
      <div
        className={`border rounded-2xl p-5 shadow-xs hover:shadow-md transition-all ${
          isUnavailable
            ? 'bg-rose-50/50 border-rose-200'
            : isDegraded
            ? 'bg-amber-50/50 border-amber-200'
            : 'bg-white border-[#DCE5F0]'
        }`}
      >
        <div className="flex items-center justify-between mb-3">
          <span
            className={`text-[11px] font-mono uppercase tracking-wider font-semibold ${
              isUnavailable
                ? 'text-rose-600'
                : isDegraded
                ? 'text-amber-600'
                : 'text-[#64748B]'
            }`}
          >
            OFFLINE NODES
          </span>
          <div
            className={`w-8 h-8 rounded-lg flex items-center justify-center ${
              isUnavailable
                ? 'bg-rose-100 text-rose-600'
                : isDegraded
                ? 'bg-amber-100 text-amber-600'
                : 'bg-slate-100 text-slate-400'
            }`}
          >
            <AlertOctagon className="w-4 h-4" />
          </div>
        </div>
        <div className="flex items-baseline gap-2">
          <span
            className={`text-3xl font-extrabold tracking-tight font-mono ${
              isUnavailable
                ? 'text-rose-600'
                : isDegraded
                ? 'text-amber-600'
                : 'text-[#64748B]'
            }`}
          >
            {String(summary.offlineNodes).padStart(2, '0')}
          </span>
          <span className="text-xs text-[#64748B]">
            {summary.offlineNodes > 0 ? 'Requires attention' : 'Unavailable'}
          </span>
        </div>
        <p
          className={`text-[11px] mt-2 border-t border-[#F1F5F9] pt-2 font-medium ${
            isUnavailable
              ? 'text-rose-600'
              : isDegraded
              ? 'text-amber-600'
              : 'text-[#64748B]'
          }`}
        >
          {summary.offlineNodes === 0
            ? 'Zero unreachable machines'
            : `${summary.offlineNodes} machine(s) disconnected`}
        </p>
      </div>

      {/* Card 4: Logical Shards */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-shadow">
        <div className="flex items-center justify-between mb-3">
          <span className="text-[11px] font-mono uppercase tracking-wider text-[#64748B] font-semibold">
            LOGICAL SHARDS
          </span>
          <div className="w-8 h-8 rounded-lg bg-[#7965E8]/10 flex items-center justify-center">
            <Layers className="w-4 h-4 text-[#7965E8]" />
          </div>
        </div>
        <div className="flex items-baseline gap-2">
          <span className="text-3xl font-extrabold text-[#7965E8] tracking-tight font-mono">
            {String(summary.totalShards).padStart(2, '0')}
          </span>
          <span className="text-xs text-[#64748B]">Active groups</span>
        </div>
        <p className="text-[11px] text-[#7965E8] mt-2 border-t border-[#F1F5F9] pt-2 font-medium">
          3 shards replicated with RF=3
        </p>
      </div>
    </div>
  );
}
