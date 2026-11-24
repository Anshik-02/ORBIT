'use client';

import React from 'react';
import { Clock, RotateCcw, ArrowRight, CheckCircle2, XCircle } from 'lucide-react';
import { QueryMethod } from './QueryEditor';

export interface HistoryItem {
  id: string;
  timestamp: string;
  method: QueryMethod;
  key: string;
  statusCode: number;
  durationMs: number;
  targetNode: string;
}

interface QueryHistoryProps {
  history: HistoryItem[];
  onRerun: (item: HistoryItem) => void;
  onClear: () => void;
}

export default function QueryHistory({
  history,
  onRerun,
  onClear,
}: QueryHistoryProps) {
  const getMethodBadge = (m: QueryMethod) => {
    switch (m) {
      case 'GET':
        return 'bg-blue-50 text-[#0891D5] border-blue-200';
      case 'PUT':
        return 'bg-emerald-50 text-[#10B981] border-emerald-200';
      case 'DELETE':
        return 'bg-rose-50 text-rose-600 border-rose-200';
    }
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      <div className="flex items-center justify-between pb-3 border-b border-[#DCE5F0]">
        <div>
          <h3 className="text-base font-bold text-[#17243A]">
            Query History & Audit Log
          </h3>
          <p className="text-[#64748B] text-xs font-normal">
            Past client operations and consensus execution metrics
          </p>
        </div>

        {history.length > 0 && (
          <button
            onClick={onClear}
            className="text-[11px] text-[#64748B] hover:text-[#17243A] hover:underline"
          >
            Clear History
          </button>
        )}
      </div>

      {history.length === 0 ? (
        <div className="py-8 text-center text-[#64748B]">
          No past queries recorded in this session.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left">
            <thead>
              <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase bg-[#F8FAFC]">
                <th className="py-2 px-3">Time</th>
                <th className="py-2 px-3">Method</th>
                <th className="py-2 px-3">Key</th>
                <th className="py-2 px-3">Node</th>
                <th className="py-2 px-3">Status</th>
                <th className="py-2 px-3">Latency</th>
                <th className="py-2 px-3 text-right">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#F1F5F9]">
              {history.map((h) => {
                const isSuccess = h.statusCode >= 200 && h.statusCode < 300;
                return (
                  <tr key={h.id} className="hover:bg-[#F8FAFC]">
                    <td className="py-2.5 px-3 text-[#64748B]">{h.timestamp}</td>
                    <td className="py-2.5 px-3">
                      <span
                        className={`px-2 py-0.5 rounded-full border text-[10px] font-bold ${getMethodBadge(
                          h.method
                        )}`}
                      >
                        {h.method}
                      </span>
                    </td>
                    <td className="py-2.5 px-3 font-bold text-[#17243A]">
                      {h.key}
                    </td>
                    <td className="py-2.5 px-3 text-[#64748B]">
                      {h.targetNode.toUpperCase()}
                    </td>
                    <td className="py-2.5 px-3">
                      <span
                        className={`inline-flex items-center gap-1 font-semibold text-[11px] ${
                          isSuccess ? 'text-[#10B981]' : 'text-rose-600'
                        }`}
                      >
                        {isSuccess ? (
                          <CheckCircle2 className="w-3 h-3" />
                        ) : (
                          <XCircle className="w-3 h-3" />
                        )}
                        <span>{h.statusCode}</span>
                      </span>
                    </td>
                    <td className="py-2.5 px-3 text-[#17243A]">
                      {h.durationMs.toFixed(1)}ms
                    </td>
                    <td className="py-2.5 px-3 text-right">
                      <button
                        onClick={() => onRerun(h)}
                        className="px-2.5 py-1 rounded-lg bg-[#F8FAFC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#0891D5] font-semibold flex items-center gap-1 ml-auto"
                        title="Load into Query Editor"
                      >
                        <RotateCcw className="w-3 h-3" />
                        <span>Rerun</span>
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
