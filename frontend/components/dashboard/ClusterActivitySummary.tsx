'use client';

import React from 'react';
import { BarChart3 } from 'lucide-react';

export default function ClusterActivitySummary() {
  const totalOps = '1.48M';
  const successRate = '99.98%';
  const putPercent = 48;
  const getPercent = 46;
  const deletePercent = 6;
  const activeConnections = 42;
  const avgReplicationRTT = '1.8ms';

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs hover:shadow-md transition-shadow flex flex-col justify-between overflow-hidden">
      <div>
        {/* Header */}
        <div className="flex items-center justify-between pb-4 mb-4 border-b border-[#DCE5F0]">
          <div>
            <div className="flex items-center gap-2">
              <div className="w-8 h-8 rounded-lg bg-[#10B981]/10 flex items-center justify-center">
                <BarChart3 className="w-4 h-4 text-[#10B981]" />
              </div>
              <h3 className="text-base font-semibold text-[#17243A] tracking-tight">
                Cluster Activity Summary
              </h3>
            </div>
            <p className="text-xs text-[#64748B] mt-1">
              Cumulative 24-hour operation telemetry & protocol breakdown
            </p>
          </div>
          <span className="text-[11px] font-mono px-2.5 py-1 rounded-full bg-[#10B981]/10 border border-[#10B981]/30 text-[#10B981] font-semibold">
            {successRate} SUCCESS
          </span>
        </div>

        {/* Big numbers */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-5 font-mono">
          <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
            <span className="text-[10px] text-[#64748B] font-semibold uppercase block tracking-wider">Total Ops (24h)</span>
            <span className="text-xl font-bold text-[#17243A] tracking-tight">{totalOps}</span>
          </div>
          <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
            <span className="text-[10px] text-[#64748B] font-semibold uppercase block tracking-wider">Active Clients</span>
            <span className="text-xl font-bold text-[#0891D5] tracking-tight">{activeConnections}</span>
          </div>
          <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
            <span className="text-[10px] text-[#64748B] font-semibold uppercase block tracking-wider">Replication RTT</span>
            <span className="text-xl font-bold text-[#10B981] tracking-tight">{avgReplicationRTT}</span>
          </div>
          <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
            <span className="text-[10px] text-[#64748B] font-semibold uppercase block tracking-wider">Consensus SLA</span>
            <span className="text-xl font-bold text-[#7965E8] tracking-tight">{successRate}</span>
          </div>
        </div>

        {/* Operation Breakdown Proportional Bar */}
        <div>
          <div className="flex items-center justify-between text-xs font-mono mb-2">
            <span className="text-[#64748B] font-medium">Command Distribution:</span>
            <div className="flex items-center gap-4 text-[11px] font-semibold">
              <span className="text-[#0891D5] flex items-center gap-1.5">
                <span className="w-2.5 h-2.5 rounded-full bg-[#0891D5] inline-block" /> PUT ({putPercent}%)
              </span>
              <span className="text-[#10B981] flex items-center gap-1.5">
                <span className="w-2.5 h-2.5 rounded-full bg-[#10B981] inline-block" /> GET ({getPercent}%)
              </span>
              <span className="text-[#EF4444] flex items-center gap-1.5">
                <span className="w-2.5 h-2.5 rounded-full bg-[#EF4444] inline-block" /> DELETE ({deletePercent}%)
              </span>
            </div>
          </div>

          <div className="w-full h-3 rounded-full bg-[#F1F5F9] overflow-hidden flex border border-[#E2E8F0]">
            <div style={{ width: `${putPercent}%` }} className="bg-[#0891D5] h-full" title={`PUT: ${putPercent}%`} />
            <div style={{ width: `${getPercent}%` }} className="bg-[#10B981] h-full" title={`GET: ${getPercent}%`} />
            <div style={{ width: `${deletePercent}%` }} className="bg-[#EF4444] h-full" title={`DELETE: ${deletePercent}%`} />
          </div>
        </div>
      </div>

      <div className="mt-5 pt-3 border-t border-[#E2E8F0] flex items-center justify-between text-[11px] font-mono text-[#64748B]">
        <span>Idempotency deduplication active</span>
        <span>Zero unhandled leader re-election errors</span>
      </div>
    </div>
  );
}
