'use client';

import React, { useState, useEffect } from 'react';
import { BarChart3, AlertTriangle } from 'lucide-react';
import { fetchClusterMetrics } from '@/lib/api/client';
import { ClusterMetricsResponse } from '@/lib/api/types';

export default function ClusterActivitySummary() {
  const [metrics, setMetrics] = useState<ClusterMetricsResponse | null>(null);
  const [isLive, setIsLive] = useState<boolean>(false);

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      try {
        const res = await fetchClusterMetrics();
        if (mounted) {
          if (res) {
            setMetrics(res);
            setIsLive(true);
          } else {
            setIsLive(false);
          }
        }
      } catch {
        if (mounted) setIsLive(false);
      }
    };

    load();
    const interval = setInterval(load, 3000);
    return () => {
      mounted = false;
      clearInterval(interval);
    };
  }, []);

  const totalReqs = metrics?.total_requests ?? 0;
  const writes = metrics?.write_requests ?? 0;
  const reads = metrics?.read_requests ?? 0;
  const errors = metrics?.error_requests ?? 0;

  const totalOpsFormatted = totalReqs >= 1000000 
    ? `${(totalReqs / 1000000).toFixed(2)}M` 
    : totalReqs >= 1000 
    ? `${(totalReqs / 1000).toFixed(1)}k` 
    : `${totalReqs}`;

  const successRate = totalReqs > 0
    ? `${(((totalReqs - errors) / totalReqs) * 100).toFixed(1)}%`
    : isLive
    ? '100%'
    : '0%';

  const sumReqs = writes + reads;
  const putPercent = sumReqs > 0 ? Math.round((writes / sumReqs) * 100) : 0;
  const getPercent = sumReqs > 0 ? 100 - putPercent : 0;
  const deletePercent = 0;

  const activeGoroutines = metrics?.num_goroutines ?? 0;
  const avgRTT = metrics?.latency?.avg_ms != null ? `${metrics.latency.avg_ms.toFixed(1)}ms` : '0.0ms';

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
              Live operation telemetry & protocol breakdown from /metrics
            </p>
          </div>
          {isLive ? (
            <span className="text-[11px] font-mono px-2.5 py-1 rounded-full bg-[#10B981]/10 border border-[#10B981]/30 text-[#10B981] font-semibold flex items-center gap-1.5">
              <span className="w-1.5 h-1.5 rounded-full bg-[#10B981] animate-pulse" />
              {successRate} SUCCESS
            </span>
          ) : (
            <span className="text-[11px] font-mono px-2.5 py-1 rounded-full bg-slate-100 border border-slate-200 text-slate-600 font-semibold flex items-center gap-1.5">
              <AlertTriangle className="w-3 h-3 text-slate-500" />
              OFFLINE
            </span>
          )}
        </div>

        {/* Big numbers */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-5 font-mono">
          <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
            <span className="text-[10px] text-[#64748B] font-semibold uppercase block tracking-wider">Total Ops</span>
            <span className="text-xl font-bold text-[#17243A] tracking-tight">{totalOpsFormatted}</span>
          </div>
          <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
            <span className="text-[10px] text-[#64748B] font-semibold uppercase block tracking-wider">Goroutines</span>
            <span className="text-xl font-bold text-[#0891D5] tracking-tight">{activeGoroutines}</span>
          </div>
          <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
            <span className="text-[10px] text-[#64748B] font-semibold uppercase block tracking-wider">Avg Latency</span>
            <span className="text-xl font-bold text-[#10B981] tracking-tight">{avgRTT}</span>
          </div>
          <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
            <span className="text-[10px] text-[#64748B] font-semibold uppercase block tracking-wider">Mem Alloc</span>
            <span className="text-xl font-bold text-[#7965E8] tracking-tight">
              {metrics ? `${metrics.memory_alloc_mb.toFixed(1)}MB` : '0MB'}
            </span>
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
                <span className="w-2.5 h-2.5 rounded-full bg-[#EF4444] inline-block" /> ERRORS ({errors})
              </span>
            </div>
          </div>

          <div className="w-full h-3 rounded-full bg-[#F1F5F9] overflow-hidden flex border border-[#E2E8F0]">
            {sumReqs === 0 ? (
              <div className="w-full bg-slate-200 h-full" title="No operations recorded" />
            ) : (
              <>
                <div style={{ width: `${putPercent}%` }} className="bg-[#0891D5] h-full" title={`PUT: ${putPercent}%`} />
                <div style={{ width: `${getPercent}%` }} className="bg-[#10B981] h-full" title={`GET: ${getPercent}%`} />
              </>
            )}
          </div>
        </div>
      </div>

      <div className="mt-5 pt-3 border-t border-[#E2E8F0] flex items-center justify-between text-[11px] font-mono text-[#64748B]">
        <span>Uptime: {metrics ? `${Math.floor(metrics.uptime_seconds)}s` : '0s'}</span>
        <span>{metrics ? `GC Runs: ${metrics.num_gc}` : 'Awaiting metrics'}</span>
      </div>
    </div>
  );
}
