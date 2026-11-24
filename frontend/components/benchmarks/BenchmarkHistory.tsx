'use client';

import React, { useState } from 'react';
import { Download, CheckCircle2, History, RotateCcw, GitCompare, ArrowUpRight, ArrowDownRight, FileSpreadsheet, FileJson, X } from 'lucide-react';
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
  onExportCsv?: (run: BenchmarkRunRecord) => void;
}

export default function BenchmarkHistory({
  runs,
  onRerun,
  onExport,
  onExportCsv,
}: BenchmarkHistoryProps) {
  const [selectedIds, setSelectedIds] = useState<string[]>([]);

  const toggleSelect = (id: string) => {
    setSelectedIds((prev) => {
      if (prev.includes(id)) {
        return prev.filter((item) => item !== id);
      }
      if (prev.length >= 2) {
        return [prev[1], id];
      }
      return [...prev, id];
    });
  };

  const selectedRuns = runs.filter((r) => selectedIds.includes(r.id));
  const runA = selectedRuns[0];
  const runB = selectedRuns[1];

  let diffQps = 0;
  let diffQpsPct = 0;
  let diffP99 = 0;
  let diffP99Pct = 0;
  let diffAvg = 0;

  if (runA && runB) {
    diffQps = runA.metrics.qps - runB.metrics.qps;
    diffQpsPct = runB.metrics.qps > 0 ? (diffQps / runB.metrics.qps) * 100 : 0;
    diffP99 = runA.metrics.p99Ms - runB.metrics.p99Ms;
    diffP99Pct = runB.metrics.p99Ms > 0 ? (diffP99 / runB.metrics.p99Ms) * 100 : 0;
    diffAvg = runA.metrics.avgLatencyMs - runB.metrics.avgLatencyMs;
  }

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
            Audit trail of throughput tests. Select any 2 runs to compare baseline vs candidate performance.
          </p>
        </div>

        <div className="flex items-center gap-2 self-start sm:self-auto">
          {selectedIds.length > 0 && (
            <button
              onClick={() => setSelectedIds([])}
              className="text-[11px] text-[#64748B] hover:text-[#17243A] px-2.5 py-1 rounded-lg bg-[#F1F5F9] border border-[#E2E8F0] flex items-center gap-1"
            >
              <X className="w-3 h-3" />
              Clear Selection ({selectedIds.length}/2)
            </button>
          )}
          <span className="text-[11px] text-[#64748B] px-3 py-1 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
            {runs.length} Saved Benchmarks
          </span>
        </div>
      </div>

      {/* COMPARISON PANEL */}
      {runA && runB && (
        <div className="bg-[#F8FAFC] border border-[#DCE5F0] rounded-xl p-4 space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 font-bold text-[#17243A]">
              <GitCompare className="w-4 h-4 text-[#7965E8]" />
              <span>Comparative Performance Delta: {runA.id} vs {runB.id}</span>
            </div>
            <span className="text-[10px] text-[#64748B]">
              Baseline: {runB.config.workload} ({runB.timestamp}) | Candidate: {runA.config.workload} ({runA.timestamp})
            </span>
          </div>

          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <div className="bg-white border border-[#DCE5F0] rounded-lg p-3">
              <span className="text-[#64748B] text-[10px] block uppercase font-medium">Throughput Delta</span>
              <div className="flex items-baseline gap-1.5 mt-1">
                <span className={`text-base font-bold ${diffQps >= 0 ? 'text-[#10B981]' : 'text-rose-600'}`}>
                  {diffQps >= 0 ? '+' : ''}{diffQps.toLocaleString()} QPS
                </span>
                <span className={`text-[11px] font-semibold flex items-center ${diffQps >= 0 ? 'text-[#10B981]' : 'text-rose-600'}`}>
                  {diffQps >= 0 ? <ArrowUpRight className="w-3 h-3" /> : <ArrowDownRight className="w-3 h-3" />}
                  {diffQpsPct.toFixed(1)}%
                </span>
              </div>
            </div>

            <div className="bg-white border border-[#DCE5F0] rounded-lg p-3">
              <span className="text-[#64748B] text-[10px] block uppercase font-medium">P99 Latency Delta</span>
              <div className="flex items-baseline gap-1.5 mt-1">
                <span className={`text-base font-bold ${diffP99 <= 0 ? 'text-[#10B981]' : 'text-rose-600'}`}>
                  {diffP99 > 0 ? '+' : ''}{diffP99.toFixed(2)}ms
                </span>
                <span className={`text-[11px] font-semibold flex items-center ${diffP99 <= 0 ? 'text-[#10B981]' : 'text-rose-600'}`}>
                  {diffP99 <= 0 ? <ArrowDownRight className="w-3 h-3" /> : <ArrowUpRight className="w-3 h-3" />}
                  {diffP99Pct.toFixed(1)}%
                </span>
              </div>
            </div>

            <div className="bg-white border border-[#DCE5F0] rounded-lg p-3">
              <span className="text-[#64748B] text-[10px] block uppercase font-medium">Avg Latency Delta</span>
              <div className="flex items-baseline gap-1.5 mt-1">
                <span className={`text-base font-bold ${diffAvg <= 0 ? 'text-[#10B981]' : 'text-rose-600'}`}>
                  {diffAvg > 0 ? '+' : ''}{diffAvg.toFixed(2)}ms
                </span>
              </div>
            </div>

            <div className="bg-white border border-[#DCE5F0] rounded-lg p-3">
              <span className="text-[#64748B] text-[10px] block uppercase font-medium">Error Count Delta</span>
              <div className="flex items-baseline gap-1.5 mt-1">
                <span className="text-base font-bold text-[#17243A]">
                  {runA.metrics.errorCount - runB.metrics.errorCount}
                </span>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase bg-[#F8FAFC]">
              <th className="py-2.5 px-3 w-8">Compare</th>
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
            {runs.map((r) => {
              const isSelected = selectedIds.includes(r.id);
              return (
                <tr key={r.id} className={`hover:bg-[#F8FAFC] ${isSelected ? 'bg-indigo-50/40' : ''}`}>
                  <td className="py-2.5 px-3">
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => toggleSelect(r.id)}
                      className="rounded border-[#DCE5F0] text-[#0891D5] focus:ring-[#0891D5]/20 cursor-pointer"
                      title="Select to compare (max 2)"
                    />
                  </td>
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
                        className="px-2 py-1 rounded-lg bg-[#F8FAFC] hover:bg-slate-100 border border-[#DCE5F0] text-[#64748B] flex items-center gap-1"
                        title="Export JSON report"
                      >
                        <FileJson className="w-3.5 h-3.5 text-[#7965E8]" />
                        <span>JSON</span>
                      </button>
                      {onExportCsv && (
                        <button
                          onClick={() => onExportCsv(r)}
                          className="px-2 py-1 rounded-lg bg-[#F8FAFC] hover:bg-slate-100 border border-[#DCE5F0] text-[#64748B] flex items-center gap-1"
                          title="Export CSV spreadsheet"
                        >
                          <FileSpreadsheet className="w-3.5 h-3.5 text-[#10B981]" />
                          <span>CSV</span>
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
