'use client';

import React, { useState } from 'react';
import { Activity, Clock, TrendingUp, Zap, BarChart2 } from 'lucide-react';

export default function PerformanceCharts() {
  const [timeRange, setTimeRange] = useState<'1m' | '5m' | '15m' | '1h' | '24h'>('5m');

  // Throughput mock series (RPS)
  const throughputData = [
    1240, 1310, 1280, 1420, 1580, 1510, 1690, 1850, 1720, 1910,
    2150, 2420, 2210, 2380, 2560, 2410, 2300, 2240, 2390, 2450
  ];
  const maxRPS = 2800;
  const currentRPS = throughputData[throughputData.length - 1];
  const peakRPS = Math.max(...throughputData);
  const avgRPS = Math.round(
    throughputData.reduce((acc, v) => acc + v, 0) / throughputData.length
  );

  // Latency percentiles
  const p50 = '2.4ms';
  const p95 = '6.8ms';
  const p99 = '12.1ms';
  const avgLatency = '3.2ms';

  // SVG coordinates for throughput area chart
  const width = 500;
  const height = 150;
  const points = throughputData.map((val, idx) => {
    const x = (idx / (throughputData.length - 1)) * width;
    const y = height - (val / maxRPS) * (height - 20) - 10;
    return `${x},${y}`;
  });
  const pathData = `M 0,${height} L ${points.join(' L ')} L ${width},${height} Z`;
  const lineData = `M ${points.join(' L ')}`;

  // Latency multi-curve mock data points (P50, P95, P99)
  const p50Curve = [2.2, 2.3, 2.5, 2.4, 2.3, 2.6, 2.4, 2.5, 2.4, 2.4];
  const p95Curve = [6.1, 6.4, 6.9, 7.2, 6.8, 6.7, 7.0, 6.8, 6.6, 6.8];
  const p99Curve = [11.2, 11.8, 12.5, 13.0, 12.2, 11.9, 12.8, 12.4, 12.0, 12.1];

  const getCurvePath = (data: number[], maxVal: number) => {
    const pts = data.map((v, i) => {
      const x = (i / (data.length - 1)) * width;
      const y = height - (v / maxVal) * (height - 30) - 15;
      return `${x},${y}`;
    });
    return `M ${pts.join(' L ')}`;
  };

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
      {/* Request Throughput Chart */}
      <div className="bg-[#0a0f1d]/80 backdrop-blur-md border border-slate-800/80 rounded-xl p-5 flex flex-col justify-between overflow-hidden">
        <div>
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
            <div>
              <div className="flex items-center gap-2">
                <Activity className="w-4 h-4 text-sky-400" />
                <h3 className="text-base font-semibold text-white tracking-wide">
                  Request Throughput
                </h3>
              </div>
              <p className="text-xs text-slate-400 mt-0.5">
                Total operations committed across consensus multi-raft groups
              </p>
            </div>

            {/* Time range selector */}
            <div className="flex items-center gap-1 p-0.5 rounded-lg bg-slate-900/90 border border-slate-800 self-start sm:self-auto font-mono text-xs">
              {(['1m', '5m', '15m', '1h', '24h'] as const).map((r) => (
                <button
                  key={r}
                  onClick={() => setTimeRange(r)}
                  className={`px-2 py-0.5 rounded transition-colors ${
                    timeRange === r
                      ? 'bg-sky-500/20 text-sky-300 border border-sky-500/30 font-semibold'
                      : 'text-slate-400 hover:text-slate-200'
                  }`}
                >
                  {r}
                </button>
              ))}
            </div>
          </div>

          {/* Metric highlights */}
          <div className="grid grid-cols-3 gap-3 mb-4 font-mono">
            <div className="p-2.5 rounded-lg bg-slate-900/50 border border-slate-800/80">
              <span className="text-[10px] text-slate-400 uppercase block">Current RPS</span>
              <span className="text-lg font-bold text-white tracking-tight">
                {currentRPS.toLocaleString()} <span className="text-[10px] text-slate-400 font-normal">req/s</span>
              </span>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-900/50 border border-slate-800/80">
              <span className="text-[10px] text-slate-400 uppercase block">Peak Throughput</span>
              <span className="text-lg font-bold text-sky-400 tracking-tight">
                {peakRPS.toLocaleString()} <span className="text-[10px] text-slate-400 font-normal">req/s</span>
              </span>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-900/50 border border-slate-800/80">
              <span className="text-[10px] text-slate-400 uppercase block">Mean Throughput</span>
              <span className="text-lg font-bold text-emerald-400 tracking-tight">
                {avgRPS.toLocaleString()} <span className="text-[10px] text-slate-400 font-normal">req/s</span>
              </span>
            </div>
          </div>
        </div>

        {/* SVG Throughput Graph */}
        <div className="relative w-full h-[150px] bg-slate-950/40 rounded-lg border border-slate-900 overflow-hidden">
          <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-full preserve-3d" preserveAspectRatio="none">
            <defs>
              <linearGradient id="throughputGrad" x1="0%" y1="0%" x2="0%" y2="100%">
                <stop offset="0%" stopColor="#38bdf8" stopOpacity="0.35" />
                <stop offset="100%" stopColor="#38bdf8" stopOpacity="0.0" />
              </linearGradient>
            </defs>

            {/* Subtle horizontal grid lines */}
            <line x1="0" y1={height * 0.25} x2={width} y2={height * 0.25} stroke="#334155" strokeWidth="0.5" strokeDasharray="3 3" />
            <line x1="0" y1={height * 0.5} x2={width} y2={height * 0.5} stroke="#334155" strokeWidth="0.5" strokeDasharray="3 3" />
            <line x1="0" y1={height * 0.75} x2={width} y2={height * 0.75} stroke="#334155" strokeWidth="0.5" strokeDasharray="3 3" />

            {/* Fill Area */}
            <path d={pathData} fill="url(#throughputGrad)" />

            {/* Main Line */}
            <path d={lineData} fill="none" stroke="#38bdf8" strokeWidth="2" strokeLinecap="round" />
          </svg>
        </div>
      </div>

      {/* Request Latency Chart */}
      <div className="bg-[#0a0f1d]/80 backdrop-blur-md border border-slate-800/80 rounded-xl p-5 flex flex-col justify-between overflow-hidden">
        <div>
          <div className="flex items-center justify-between mb-4">
            <div>
              <div className="flex items-center gap-2">
                <Clock className="w-4 h-4 text-indigo-400" />
                <h3 className="text-base font-semibold text-white tracking-wide">
                  Request Latency (End-to-End)
                </h3>
              </div>
              <p className="text-xs text-slate-400 mt-0.5">
                Linearizable client round-trip including leader forwarding & replication
              </p>
            </div>
            <span className="text-[11px] font-mono px-2 py-0.5 rounded-full bg-indigo-500/10 border border-indigo-500/30 text-indigo-400">
              P99 &lt; 15ms SLA
            </span>
          </div>

          {/* Latency percentiles breakdown */}
          <div className="grid grid-cols-4 gap-2 mb-4 font-mono">
            <div className="p-2.5 rounded-lg bg-slate-900/50 border border-slate-800/80">
              <span className="text-[10px] text-slate-400 uppercase block">Mean</span>
              <span className="text-base font-bold text-slate-200">{avgLatency}</span>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-900/50 border border-slate-800/80">
              <span className="text-[10px] text-slate-400 uppercase block">P50</span>
              <span className="text-base font-bold text-emerald-400">{p50}</span>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-900/50 border border-slate-800/80">
              <span className="text-[10px] text-slate-400 uppercase block">P95</span>
              <span className="text-base font-bold text-amber-400">{p95}</span>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-900/50 border border-slate-800/80">
              <span className="text-[10px] text-slate-400 uppercase block">P99</span>
              <span className="text-base font-bold text-rose-400">{p99}</span>
            </div>
          </div>
        </div>

        {/* Multi-line Latency Curves */}
        <div className="relative w-full h-[150px] bg-slate-950/40 rounded-lg border border-slate-900 overflow-hidden">
          <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-full" preserveAspectRatio="none">
            {/* Horizontal guidelines */}
            <line x1="0" y1={height * 0.25} x2={width} y2={height * 0.25} stroke="#334155" strokeWidth="0.5" strokeDasharray="3 3" />
            <line x1="0" y1={height * 0.5} x2={width} y2={height * 0.5} stroke="#334155" strokeWidth="0.5" strokeDasharray="3 3" />
            <line x1="0" y1={height * 0.75} x2={width} y2={height * 0.75} stroke="#334155" strokeWidth="0.5" strokeDasharray="3 3" />

            {/* P99 Line (Rose) */}
            <path d={getCurvePath(p99Curve, 15)} fill="none" stroke="#f43f5e" strokeWidth="2" strokeDasharray="4 2" />

            {/* P95 Line (Amber) */}
            <path d={getCurvePath(p95Curve, 15)} fill="none" stroke="#f59e0b" strokeWidth="2" />

            {/* P50 Line (Emerald) */}
            <path d={getCurvePath(p50Curve, 15)} fill="none" stroke="#10b981" strokeWidth="2" />
          </svg>

          {/* Overlay Legend */}
          <div className="absolute top-2 right-2 flex items-center gap-3 font-mono text-[10px] px-2 py-1 rounded bg-slate-900/90 border border-slate-800">
            <div className="flex items-center gap-1 text-emerald-400">
              <span className="w-2 h-0.5 bg-emerald-400 inline-block" /> P50
            </div>
            <div className="flex items-center gap-1 text-amber-400">
              <span className="w-2 h-0.5 bg-amber-400 inline-block" /> P95
            </div>
            <div className="flex items-center gap-1 text-rose-400">
              <span className="w-2 h-0.5 bg-rose-400 inline-block" /> P99
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
