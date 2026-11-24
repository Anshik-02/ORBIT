'use client';

import React from 'react';
import { BarChart3, TrendingUp, Layers, Zap } from 'lucide-react';

interface BenchmarkChartsProps {
  throughputHistory: number[];
  readQps: number;
  writeQps: number;
  p50: number;
  p90: number;
  p95: number;
  p99: number;
}

export default function BenchmarkCharts({
  throughputHistory = [12000, 14500, 16800, 18200, 19400, 18900, 20100, 20800, 20400, 21200],
  readQps = 16800,
  writeQps = 4400,
  p50 = 1.2,
  p90 = 2.4,
  p95 = 3.4,
  p99 = 5.8,
}: BenchmarkChartsProps) {
  const maxQps = Math.max(...throughputHistory, 25000);
  const totalQps = readQps + writeQps;
  const readPct = totalQps > 0 ? Math.round((readQps / totalQps) * 100) : 80;
  const writePct = 100 - readPct;

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 font-mono text-xs">
      {/* 01. THROUGHPUT OVER TIME */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between pb-3 mb-4 border-b border-[#DCE5F0]">
            <div>
              <h3 className="text-base font-bold text-[#17243A]">
                Throughput Over Time (QPS)
              </h3>
              <p className="text-[#64748B] text-xs font-normal mt-0.5">
                Sampling interval: 1s rolling window
              </p>
            </div>
            <span className="p-1.5 rounded-lg bg-blue-50 text-[#0891D5]">
              <Zap className="w-4 h-4" />
            </span>
          </div>

          {/* SVG Area Chart */}
          <div className="h-44 w-full relative">
            <svg
              className="w-full h-full overflow-visible"
              viewBox="0 0 500 150"
              preserveAspectRatio="none"
            >
              <defs>
                <linearGradient id="qpsGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#0891D5" stopOpacity="0.25" />
                  <stop offset="100%" stopColor="#0891D5" stopOpacity="0.0" />
                </linearGradient>
              </defs>

              {/* Grid Lines */}
              <line x1="0" y1="30" x2="500" y2="30" stroke="#E2E8F0" strokeDasharray="3 3" />
              <line x1="0" y1="75" x2="500" y2="75" stroke="#E2E8F0" strokeDasharray="3 3" />
              <line x1="0" y1="120" x2="500" y2="120" stroke="#E2E8F0" strokeDasharray="3 3" />

              {/* Path calculation */}
              {(() => {
                const points = throughputHistory.map((val, idx) => {
                  const x = (idx / (throughputHistory.length - 1)) * 500;
                  const y = 140 - (val / maxQps) * 120;
                  return `${x},${y}`;
                });
                const dArea = `M 0,140 L ${points.join(' L ')} L 500,140 Z`;
                const dLine = `M ${points.join(' L ')}`;

                return (
                  <>
                    <path d={dArea} fill="url(#qpsGradient)" />
                    <path d={dLine} fill="none" stroke="#0891D5" strokeWidth="2.5" />
                  </>
                );
              })()}
            </svg>
          </div>
        </div>

        <div className="flex items-center justify-between pt-3 border-t border-[#E2E8F0] text-[#64748B]">
          <span>Current Multi-Shard Bandwidth:</span>
          <span className="font-bold text-[#17243A]">
            {(readQps + writeQps).toLocaleString()} req/sec
          </span>
        </div>
      </div>

      {/* 02. LATENCY PERCENTILE DISTRIBUTION & READ/WRITE SPLIT */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between pb-3 mb-4 border-b border-[#DCE5F0]">
            <div>
              <h3 className="text-base font-bold text-[#17243A]">
                Latency Profile & Workload Split
              </h3>
              <p className="text-[#64748B] text-xs font-normal mt-0.5">
                Round-trip times measured at client edge
              </p>
            </div>
            <span className="p-1.5 rounded-lg bg-emerald-50 text-[#10B981]">
              <TrendingUp className="w-4 h-4" />
            </span>
          </div>

          {/* Percentile horizontal bars */}
          <div className="space-y-3">
            <div className="space-y-1">
              <div className="flex justify-between text-[#64748B]">
                <span>P50 (Median)</span>
                <span className="font-bold text-[#17243A]">{p50.toFixed(2)} ms</span>
              </div>
              <div className="w-full h-2 rounded-full bg-[#F1F5F9] overflow-hidden">
                <div className="h-full bg-[#10B981] rounded-full" style={{ width: `${(p50 / 8) * 100}%` }} />
              </div>
            </div>

            <div className="space-y-1">
              <div className="flex justify-between text-[#64748B]">
                <span>P90 Threshold</span>
                <span className="font-bold text-[#17243A]">{p90.toFixed(2)} ms</span>
              </div>
              <div className="w-full h-2 rounded-full bg-[#F1F5F9] overflow-hidden">
                <div className="h-full bg-[#0891D5] rounded-full" style={{ width: `${(p90 / 8) * 100}%` }} />
              </div>
            </div>

            <div className="space-y-1">
              <div className="flex justify-between text-[#64748B]">
                <span>P95 Quorum Latency</span>
                <span className="font-bold text-[#17243A]">{p95.toFixed(2)} ms</span>
              </div>
              <div className="w-full h-2 rounded-full bg-[#F1F5F9] overflow-hidden">
                <div className="h-full bg-[#F59E0B] rounded-full" style={{ width: `${(p95 / 8) * 100}%` }} />
              </div>
            </div>

            <div className="space-y-1">
              <div className="flex justify-between text-[#64748B]">
                <span>P99 Tail Latency</span>
                <span className="font-bold text-[#17243A]">{p99.toFixed(2)} ms</span>
              </div>
              <div className="w-full h-2 rounded-full bg-[#F1F5F9] overflow-hidden">
                <div className="h-full bg-[#7965E8] rounded-full" style={{ width: `${(p99 / 8) * 100}%` }} />
              </div>
            </div>
          </div>

          {/* Workload ratio bar */}
          <div className="mt-4 pt-3 border-t border-[#E2E8F0]">
            <div className="flex justify-between text-[#64748B] mb-1">
              <span>Read vs Write Breakdown</span>
              <span>Read {readPct}% / Write {writePct}%</span>
            </div>
            <div className="w-full h-2.5 rounded-full bg-[#F1F5F9] overflow-hidden flex">
              <div style={{ width: `${readPct}%` }} className="bg-[#0891D5]" />
              <div style={{ width: `${writePct}%` }} className="bg-[#10B981]" />
            </div>
          </div>
        </div>

        <div className="mt-4 pt-3 border-t border-[#E2E8F0] flex items-center justify-between text-[#64748B]">
          <span>Linearizable ReadIndex Cost:</span>
          <span className="text-[#0891D5] font-semibold">+0.3ms Heartbeat Exchange</span>
        </div>
      </div>
    </div>
  );
}
