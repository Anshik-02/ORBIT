'use client';

import React from 'react';
import { HardDrive, CheckCircle2, ShieldCheck, FileText, ArrowRight } from 'lucide-react';

export interface WalLogFile {
  filename: string;
  shardId: number;
  sizeBytes: number;
  entryCount: number;
  lastIndex: number;
  status: 'ACTIVE_APPEND' | 'ROTATED_CLEAN' | 'REPLAYED';
}

interface WalExplorerProps {
  files: WalLogFile[];
}

export default function WalExplorer({ files }: WalExplorerProps) {
  const formatMB = (bytes: number) => (bytes / (1024 * 1024)).toFixed(2);

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-3 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
              <HardDrive className="w-3.5 h-3.5 text-[#0891D5]" />
            </div>
            <h3 className="text-base font-bold text-[#17243A]">
              Write-Ahead Log (WAL) Durability Files
            </h3>
          </div>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Synchronous disk journals ensuring crash recovery & non-volatile durability
          </p>
        </div>

        <span className="text-[11px] text-[#10B981] font-semibold px-3 py-1 rounded-xl bg-emerald-50 border border-emerald-200 self-start sm:self-auto flex items-center gap-1.5">
          <CheckCircle2 className="w-3.5 h-3.5" />
          <span>fsync: STRICT_SYNC</span>
        </span>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase bg-[#F8FAFC]">
              <th className="py-2.5 px-3">WAL Segment</th>
              <th className="py-2.5 px-3">Shard</th>
              <th className="py-2.5 px-3">File Size</th>
              <th className="py-2.5 px-3">Journal Entries</th>
              <th className="py-2.5 px-3">Latest Raft Index</th>
              <th className="py-2.5 px-3 text-right">Recovery State</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9]">
            {files.map((f) => (
              <tr key={f.filename} className="hover:bg-[#F8FAFC]">
                <td className="py-2.5 px-3 font-bold text-[#17243A] flex items-center gap-2">
                  <FileText className="w-3.5 h-3.5 text-[#0891D5]" />
                  <span>{f.filename}</span>
                </td>
                <td className="py-2.5 px-3 text-[#7965E8] font-semibold">
                  Shard {f.shardId}
                </td>
                <td className="py-2.5 px-3 text-[#17243A]">
                  {formatMB(f.sizeBytes)} MB
                </td>
                <td className="py-2.5 px-3 text-[#64748B]">
                  {f.entryCount.toLocaleString()} writes
                </td>
                <td className="py-2.5 px-3 font-bold text-[#0891D5]">
                  #{f.lastIndex.toLocaleString()}
                </td>
                <td className="py-2.5 px-3 text-right">
                  <span
                    className={`px-2 py-0.5 rounded-full border text-[10px] font-bold ${
                      f.status === 'ACTIVE_APPEND'
                        ? 'bg-blue-50 text-[#0891D5] border-blue-200'
                        : 'bg-emerald-50 text-[#10B981] border-emerald-200'
                    }`}
                  >
                    {f.status}
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
