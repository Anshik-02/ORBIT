'use client';

import React from 'react';
import { History, CheckCircle2, RotateCcw, Download } from 'lucide-react';
import { ChaosImpactReport } from './ChaosImpactAnalysis';

export interface ChaosRunRecord {
  id: string;
  timestamp: string;
  experimentName: string;
  targetNode: string;
  targetShard: number;
  durationSec: number;
  impact: ChaosImpactReport;
  outcome: 'PASSED' | 'FAILED';
}

interface ChaosHistoryProps {
  records: ChaosRunRecord[];
  onRerun: (rec: ChaosRunRecord) => void;
}

export default function ChaosHistory({ records, onRerun }: ChaosHistoryProps) {
  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-3 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <History className="w-4 h-4 text-[#0891D5]" />
            <h3 className="text-base font-bold text-[#17243A]">
              Chaos Fault Injection History & Audit Log
            </h3>
          </div>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Log of previously executed chaos experiments and consensus resilience verifications
          </p>
        </div>

        <span className="text-[11px] text-[#64748B] px-3 py-1 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] self-start sm:self-auto">
          {records.length} Executed Fault Tests
        </span>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase bg-[#F8FAFC]">
              <th className="py-2.5 px-3">Timestamp</th>
              <th className="py-2.5 px-3">Experiment</th>
              <th className="py-2.5 px-3">Target Node</th>
              <th className="py-2.5 px-3">Shard</th>
              <th className="py-2.5 px-3">Election Time</th>
              <th className="py-2.5 px-3">Recovery Time</th>
              <th className="py-2.5 px-3">Consistency</th>
              <th className="py-2.5 px-3 text-right">Outcome</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9]">
            {records.map((r) => (
              <tr key={r.id} className="hover:bg-[#F8FAFC]">
                <td className="py-2.5 px-3 text-[#64748B]">{r.timestamp}</td>
                <td className="py-2.5 px-3 font-bold text-[#17243A]">
                  {r.experimentName}
                </td>
                <td className="py-2.5 px-3 text-[#0891D5] font-semibold">
                  {r.targetNode.toUpperCase()}
                </td>
                <td className="py-2.5 px-3 text-[#7965E8]">
                  Shard {r.targetShard}
                </td>
                <td className="py-2.5 px-3 text-[#17243A]">
                  {r.impact.electionDurationMs} ms
                </td>
                <td className="py-2.5 px-3 text-[#64748B]">
                  {(r.impact.recoveryDurationMs / 1000).toFixed(1)}s
                </td>
                <td className="py-2.5 px-3 text-[#10B981] font-bold">
                  {r.impact.dataConsistencyPct}%
                </td>
                <td className="py-2.5 px-3 text-right">
                  <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-bold text-[10px]">
                    <CheckCircle2 className="w-3 h-3" />
                    <span>{r.outcome}</span>
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
