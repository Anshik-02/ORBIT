'use client';

import React from 'react';
import { FileText, CheckCircle2, Check, ArrowRight } from 'lucide-react';

export interface RaftLogEntry {
  index: number;
  term: number;
  type: 'KV_PUT' | 'KV_DELETE' | 'CONF_CHANGE' | 'NO_OP';
  payloadSummary: string;
  committed: boolean;
  applied: boolean;
  timestamp: string;
}

interface RaftLogInspectorProps {
  entries: RaftLogEntry[];
}

export default function RaftLogInspector({ entries }: RaftLogInspectorProps) {
  const getCommandBadge = (type: RaftLogEntry['type']) => {
    switch (type) {
      case 'KV_PUT':
        return 'bg-blue-50 text-[#0891D5] border-blue-200';
      case 'KV_DELETE':
        return 'bg-rose-50 text-rose-600 border-rose-200';
      case 'CONF_CHANGE':
        return 'bg-purple-50 text-[#7965E8] border-purple-200';
      case 'NO_OP':
      default:
        return 'bg-slate-100 text-[#64748B] border-slate-200';
    }
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-3 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
              <FileText className="w-3.5 h-3.5 text-[#0891D5]" />
            </div>
            <h3 className="text-base font-bold text-[#17243A] font-mono">
              Consensus Write-Ahead Log (WAL) Inspector
            </h3>
          </div>
          <p className="text-xs text-[#64748B] font-mono mt-0.5">
            Committed Raft entries awaiting / applied to the persistent LSM storage engine
          </p>
        </div>

        <span className="text-xs font-mono text-[#64748B] px-3 py-1 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] self-start sm:self-auto">
          Tail Inspection (Last {entries.length} Entries)
        </span>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left font-mono text-xs">
          <thead>
            <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase bg-[#F8FAFC]">
              <th className="py-2.5 px-3">Log Index</th>
              <th className="py-2.5 px-3">Term</th>
              <th className="py-2.5 px-3">Command Type</th>
              <th className="py-2.5 px-3">Payload Summary</th>
              <th className="py-2.5 px-3">Raft Quorum</th>
              <th className="py-2.5 px-3">LSM State Machine</th>
              <th className="py-2.5 px-3 text-right">Commit Time</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9]">
            {entries.map((entry) => (
              <tr key={entry.index} className="hover:bg-[#F8FAFC]">
                <td className="py-2.5 px-3 font-bold text-[#0891D5]">
                  #{entry.index.toLocaleString()}
                </td>
                <td className="py-2.5 px-3 text-[#17243A] font-semibold">
                  Term {entry.term}
                </td>
                <td className="py-2.5 px-3">
                  <span
                    className={`px-2 py-0.5 rounded-full border text-[10px] font-bold ${getCommandBadge(
                      entry.type
                    )}`}
                  >
                    {entry.type}
                  </span>
                </td>
                <td className="py-2.5 px-3 text-[#17243A] font-mono text-[11px]">
                  {entry.payloadSummary}
                </td>
                <td className="py-2.5 px-3">
                  {entry.committed ? (
                    <span className="inline-flex items-center gap-1 text-[#10B981] font-semibold text-[11px]">
                      <CheckCircle2 className="w-3.5 h-3.5" />
                      Committed
                    </span>
                  ) : (
                    <span className="text-amber-500 font-semibold text-[11px]">
                      Pending Quorum
                    </span>
                  )}
                </td>
                <td className="py-2.5 px-3">
                  {entry.applied ? (
                    <span className="inline-flex items-center gap-1 text-[#0891D5] font-semibold text-[11px]">
                      <Check className="w-3.5 h-3.5" />
                      Applied to LSM
                    </span>
                  ) : (
                    <span className="text-slate-400 font-semibold text-[11px]">
                      Queueing
                    </span>
                  )}
                </td>
                <td className="py-2.5 px-3 text-right text-[#64748B]">
                  {entry.timestamp}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
