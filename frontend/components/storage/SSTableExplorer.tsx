'use client';

import React from 'react';
import { Database, Filter, Layers, HardDrive, CheckCircle2 } from 'lucide-react';

export interface SSTableItem {
  filename: string;
  level: 'L0' | 'L1' | 'L2';
  shardId: number;
  sizeBytes: number;
  keyCount: number;
  keyRange: string;
  bloomFilterBits: number;
  status: 'ACTIVE' | 'COMPACTING';
}

interface SSTableExplorerProps {
  sstables: SSTableItem[];
}

export default function SSTableExplorer({ sstables }: SSTableExplorerProps) {
  const formatKB = (b: number) => (b / 1024).toFixed(1);

  const getLevelBadge = (lvl: SSTableItem['level']) => {
    switch (lvl) {
      case 'L0':
        return 'bg-amber-50 text-[#D97706] border-amber-200';
      case 'L1':
        return 'bg-blue-50 text-[#0891D5] border-blue-200';
      case 'L2':
        return 'bg-purple-50 text-[#7965E8] border-purple-200';
    }
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-3 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-[#10B981]/10 flex items-center justify-center">
              <Database className="w-3.5 h-3.5 text-[#10B981]" />
            </div>
            <h3 className="text-base font-bold text-[#17243A]">
              Persistent SSTable Disk Files (Levels L0 – L2)
            </h3>
          </div>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Immutable sorted key blocks with Bloom filter acceleration & sparse indexes
          </p>
        </div>

        <span className="text-[11px] text-[#64748B] px-3 py-1 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] self-start sm:self-auto">
          {sstables.length} SSTables Indexed
        </span>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase bg-[#F8FAFC]">
              <th className="py-2.5 px-3">SSTable File</th>
              <th className="py-2.5 px-3">Level</th>
              <th className="py-2.5 px-3">Shard</th>
              <th className="py-2.5 px-3">Disk Size</th>
              <th className="py-2.5 px-3">Keys Indexed</th>
              <th className="py-2.5 px-3">Key Range Boundaries</th>
              <th className="py-2.5 px-3 text-right">Bloom Filter</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9]">
            {sstables.map((s) => (
              <tr key={s.filename} className="hover:bg-[#F8FAFC]">
                <td className="py-2.5 px-3 font-bold text-[#17243A]">
                  {s.filename}
                </td>
                <td className="py-2.5 px-3">
                  <span
                    className={`px-2 py-0.5 rounded-full border text-[10px] font-bold ${getLevelBadge(
                      s.level
                    )}`}
                  >
                    {s.level}
                  </span>
                </td>
                <td className="py-2.5 px-3 text-[#7965E8] font-semibold">
                  Shard {s.shardId}
                </td>
                <td className="py-2.5 px-3 text-[#17243A]">
                  {formatKB(s.sizeBytes)} KB
                </td>
                <td className="py-2.5 px-3 text-[#64748B]">
                  {s.keyCount.toLocaleString()} keys
                </td>
                <td className="py-2.5 px-3 text-[#0891D5] font-semibold text-[11px]">
                  {s.keyRange}
                </td>
                <td className="py-2.5 px-3 text-right">
                  <span className="inline-flex items-center gap-1 text-[#10B981] font-semibold text-[11px]">
                    <CheckCircle2 className="w-3 h-3" />
                    <span>10 bits/key (1% FPR)</span>
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
