'use client';

import React from 'react';
import { Download, CheckCircle2, History, RotateCcw } from 'lucide-react';
import { BenchmarkConfig } from './BenchmarkConfigurator';
import { BenchmarkMetrics } from './BenchmarkMetricsCards';

export interface BenchmarkRunRecord {
  id: string;
  timestamp: string;
  config: BenchmarkConfig;
  metrics: BenchmarkMetrics;
}

interface BenchmarkHistoryProps {
  runs: BenchmarkRunRecord[];
  onRerun: (config: BenchmarkConfig) => void;
  onExport: (run: BenchmarkRunRecord) => void;
}

export default function BenchmarkHistory({
  runs,
  onRerun,
  onExport,
}: BenchmarkHistoryProps) {
  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-3 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <History className="w-4 h-4 text-[#0891D5]" />
            <h3 className="text-base font-bold text-[#17243A]">
              Historical Benchmark Runs & Comparison
            </h3>
          </div>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Audit trail of throughput tests across varying concurrency & key sizes
          </p>
        </div>

        <span className="text-[11px] text-[#64748B] px-3 py-1 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] self-start sm:self-auto">
          {runs.length} Saved Benchmarks
        </span>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase bg-[#F8FAFC]">
              <th className="py-2.5 px-3">Run Date</th>
              <th className="py-2.5 px-3">Workload Profile</th>
              <th className="py-2.5 px-3">Concurrency</th>
              <th className="py-2.5 px-3">Duration</th>
              <th className="py-2.5 px-3">Throughput</th>
              <th className="py-2.5 px-3">P95 / P99 Latency</th>
              <th className="py-2.5 px-3">Errors</th>
              <th className="py-2.5 px-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9]">
            {runs.map((r) => (
              <tr key={r.id} className="hover:bg-[#F8FAFC]">
                <td className="py-2.5 px-3 text-[#64748B]">{r.timestamp}</td>
                <td className="py-2.5 px-3">
                  <span className="px-2 py-0.5 rounded-full bg-blue-50 text-[#0891D5] border border-blue-200 text-[10px] font-bold">
                    {r.config.workload.toUpperCase().replace('_', ' ')}
                  </span>
                </td>
                <td className="py-2.5 px-3 text-[#17243A]">
                  {r.config.concurrency} clients
                </td>
                <td className="py-2.5 px-3 text-[#64748B]">
                  {r.config.durationSec}s
                </td>
                <td className="py-2.5 px-3 font-bold text-[#17243A]">
                  {r.metrics.qps.toLocaleString()} QPS
                </td>
                <td className="py-2.5 px-3 text-[#0891D5]">
                  {r.metrics.p95Ms.toFixed(1)}ms / {r.metrics.p99Ms.toFixed(1)}ms
                </td>
                <td className="py-2.5 px-3 text-[#10B981] font-semibold">
                  {r.metrics.errorCount}
                </td>
                <td className="py-2.5 px-3 text-right">
                  <div className="flex items-center justify-end gap-1.5">
                    <button
                      onClick={() => onRerun(r.config)}
                      className="px-2.5 py-1 rounded-lg bg-[#F8FAFC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#0891D5] font-semibold flex items-center gap-1"
                      title="Rerun with same parameters"
                    >
                      <RotateCcw className="w-3 h-3" />
                      <span>Rerun</span>
                    </button>
                    <button
                      onClick={() => onExport(r)}
                      className="p-1.5 rounded-lg bg-[#F8FAFC] hover:bg-slate-100 border border-[#DCE5F0] text-[#64748B]"
                      title="Export Run Report"
                    >
                      <Download className="w-3 h-3" />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
