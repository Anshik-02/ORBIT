'use client';

import React from 'react';
import { Activity, CheckCircle2, Clock, BarChart3, Database } from 'lucide-react';

interface QueryAnalyticsProps {
  totalQueries: number;
  successCount: number;
  failCount: number;
  avgLatencyMs: number;
  methodCounts: {
    GET: number;
    PUT: number;
    DELETE: number;
  };
}

export default function QueryAnalytics({
  totalQueries = 0,
  successCount = 0,
  failCount = 0,
  avgLatencyMs = 0,
  methodCounts = { GET: 0, PUT: 0, DELETE: 0 },
}: QueryAnalyticsProps) {
  const successRate = totalQueries > 0 ? ((successCount / totalQueries) * 100).toFixed(1) : '100.0';
  const getPct = totalQueries > 0 ? Math.round((methodCounts.GET / totalQueries) * 100) : 0;
  const putPct = totalQueries > 0 ? Math.round((methodCounts.PUT / totalQueries) * 100) : 0;
  const deletePct = totalQueries > 0 ? Math.max(0, 100 - getPct - putPct) : 0;

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 font-mono">
      {/* 01. TOTAL OPERATIONS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>TOTAL QUERIES</span>
            <span className="p-1.5 rounded-lg bg-[#0891D5]/10 text-[#0891D5]">
              <Database className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#17243A]">
            {totalQueries}
          </div>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2">
          Client read/write operations
        </p>
      </div>

      {/* 02. SUCCESS RATE */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>SUCCESS RATE</span>
            <span className="p-1.5 rounded-lg bg-[#10B981]/10 text-[#10B981]">
              <CheckCircle2 className="w-4 h-4" />
            </span>
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-3xl font-bold text-[#10B981]">
              {successRate}%
            </span>
            <span className="text-xs text-[#64748B]">
              ({failCount} failed)
            </span>
          </div>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2">
          Quorum acknowledgment ratio
        </p>
      </div>

      {/* 03. AVERAGE LATENCY */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>AVG COMMIT RTT</span>
            <span className="p-1.5 rounded-lg bg-[#7965E8]/10 text-[#7965E8]">
              <Clock className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#7965E8]">
            {avgLatencyMs.toFixed(1)} <span className="text-sm font-normal text-[#64748B]">ms</span>
          </div>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2">
          Round-trip proposal to applied
        </p>
      </div>

      {/* 04. METHOD RATIO */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>OPERATION RATIO</span>
            <span className="p-1.5 rounded-lg bg-[#F59E0B]/10 text-[#F59E0B]">
              <BarChart3 className="w-4 h-4" />
            </span>
          </div>

          <div className="w-full h-3 rounded-full bg-[#F1F5F9] overflow-hidden flex my-2">
            <div style={{ width: `${getPct}%` }} className="bg-[#0891D5]" title={`GET: ${getPct}%`} />
            <div style={{ width: `${putPct}%` }} className="bg-[#10B981]" title={`PUT: ${putPct}%`} />
            <div style={{ width: `${deletePct}%` }} className="bg-rose-500" title={`DELETE: ${deletePct}%`} />
          </div>
        </div>

        <div className="flex items-center justify-between text-[10px] text-[#64748B] pt-1">
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full bg-[#0891D5]" /> GET {getPct}%
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full bg-[#10B981]" /> PUT {putPct}%
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full bg-rose-500" /> DEL {deletePct}%
          </span>
        </div>
      </div>
    </div>
  );
}
