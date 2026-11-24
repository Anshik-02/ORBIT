'use client';

import React from 'react';
import { BarChart3, CheckCircle2, Globe, Wifi, Repeat } from 'lucide-react';

export default function ClusterActivitySummary() {
  const totalOps = '1.48M';
  const successRate = '99.98%';
  const putPercent = 48;
  const getPercent = 46;
  const deletePercent = 6;
  const activeConnections = 42;
  const avgReplicationRTT = '1.8ms';

  return (
    <div className="bg-[#0a0f1d]/80 backdrop-blur-md border border-slate-800/80 rounded-xl p-5 flex flex-col justify-between overflow-hidden">
      <div>
        {/* Header */}
        <div className="flex items-center justify-between pb-3 mb-4 border-b border-slate-800/80">
          <div>
            <div className="flex items-center gap-2">
              <BarChart3 className="w-4 h-4 text-emerald-400" />
              <h3 className="text-base font-semibold text-white tracking-wide">
                Cluster Activity Summary
              </h3>
            </div>
            <p className="text-xs text-slate-400 mt-0.5">
              Cumulative 24-hour operation telemetry & protocol breakdown
            </p>
          </div>
          <span className="text-[11px] font-mono px-2 py-0.5 rounded-full bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
            {successRate} SUCCESS
          </span>
        </div>

        {/* Big numbers */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-5 font-mono">
          <div className="p-3 rounded-lg bg-slate-900/50 border border-slate-800/80">
            <span className="text-[10px] text-slate-400 uppercase block">Total Operations (24h)</span>
            <span className="text-xl font-bold text-white tracking-tight">{totalOps}</span>
          </div>
          <div className="p-3 rounded-lg bg-slate-900/50 border border-slate-800/80">
            <span className="text-[10px] text-slate-400 uppercase block">Active Clients</span>
            <span className="text-xl font-bold text-sky-400 tracking-tight">{activeConnections}</span>
          </div>
          <div className="p-3 rounded-lg bg-slate-900/50 border border-slate-800/80">
            <span className="text-[10px] text-slate-400 uppercase block">Replication RTT</span>
            <span className="text-xl font-bold text-emerald-400 tracking-tight">{avgReplicationRTT}</span>
          </div>
          <div className="p-3 rounded-lg bg-slate-900/50 border border-slate-800/80">
            <span className="text-[10px] text-slate-400 uppercase block">Consensus Success</span>
            <span className="text-xl font-bold text-indigo-400 tracking-tight">{successRate}</span>
          </div>
        </div>

        {/* Operation Breakdown Proportional Bar */}
        <div>
          <div className="flex items-center justify-between text-xs font-mono mb-2">
            <span className="text-slate-400">Command Distribution:</span>
            <div className="flex items-center gap-4 text-[11px]">
              <span className="text-sky-400 flex items-center gap-1">
                <span className="w-2 h-2 rounded-full bg-sky-400 inline-block" /> PUT ({putPercent}%)
              </span>
              <span className="text-emerald-400 flex items-center gap-1">
                <span className="w-2 h-2 rounded-full bg-emerald-400 inline-block" /> GET ({getPercent}%)
              </span>
              <span className="text-rose-400 flex items-center gap-1">
                <span className="w-2 h-2 rounded-full bg-rose-400 inline-block" /> DELETE ({deletePercent}%)
              </span>
            </div>
          </div>

          <div className="w-full h-3 rounded-full bg-slate-900 overflow-hidden flex border border-slate-800">
            <div style={{ width: `${putPercent}%` }} className="bg-sky-500 h-full" title={`PUT: ${putPercent}%`} />
            <div style={{ width: `${getPercent}%` }} className="bg-emerald-500 h-full" title={`GET: ${getPercent}%`} />
            <div style={{ width: `${deletePercent}%` }} className="bg-rose-500 h-full" title={`DELETE: ${deletePercent}%`} />
          </div>
        </div>
      </div>

      <div className="mt-4 pt-3 border-t border-slate-800/60 flex items-center justify-between text-[11px] font-mono text-slate-400">
        <span>Idempotency deduplication active</span>
        <span className="text-slate-400">Zero unhandled leader re-election errors</span>
      </div>
    </div>
  );
}
