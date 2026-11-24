'use client';

import React from 'react';
import { Cpu, HardDrive, RefreshCw, CheckCircle2, Layers } from 'lucide-react';

export interface MemtableItem {
  id: string;
  shardId: number;
  type: 'ACTIVE' | 'IMMUTABLE';
  entryCount: number;
  sizeBytes: number;
  maxSizeBytes: number;
  flushStatus: string;
}

interface MemtableInspectorProps {
  memtables: MemtableItem[];
  onTriggerFlush?: (id: string) => void;
}

export default function MemtableInspector({
  memtables,
  onTriggerFlush,
}: MemtableInspectorProps) {
  const formatKB = (b: number) => (b / 1024).toFixed(1);

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-3 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
              <Cpu className="w-3.5 h-3.5 text-[#0891D5]" />
            </div>
            <h3 className="text-base font-bold text-[#17243A]">
              Memtable Memory Inspector
            </h3>
          </div>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Active in-memory SkipLists & frozen immutable tables queued for disk write
          </p>
        </div>

        <span className="text-[11px] text-[#64748B] px-3 py-1 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] self-start sm:self-auto">
          SkipList Concurrent Engine
        </span>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {memtables.map((m) => {
          const pct = Math.min(100, Math.round((m.sizeBytes / m.maxSizeBytes) * 100));

          return (
            <div
              key={m.id}
              className={`p-4 rounded-xl border transition-all ${
                m.type === 'ACTIVE'
                  ? 'bg-[#F8FAFC] border-[#E2E8F0]'
                  : 'bg-purple-50/40 border-[#7965E8]/30'
              }`}
            >
              <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-2">
                  <span
                    className={`w-2.5 h-2.5 rounded-full ${
                      m.type === 'ACTIVE' ? 'bg-[#10B981]' : 'bg-[#7965E8]'
                    }`}
                  />
                  <span className="font-bold text-[#17243A] text-sm">{m.id}</span>
                  <span className="text-[#64748B] text-[11px]">
                    (Shard {m.shardId})
                  </span>
                </div>

                <span
                  className={`px-2 py-0.5 rounded-full text-[10px] font-bold border ${
                    m.type === 'ACTIVE'
                      ? 'bg-emerald-50 text-[#10B981] border-emerald-200'
                      : 'bg-purple-50 text-[#7965E8] border-purple-200'
                  }`}
                >
                  {m.type}
                </span>
              </div>

              {/* Memory capacity bar */}
              <div className="space-y-1.5 mb-3">
                <div className="flex justify-between text-[11px] text-[#64748B]">
                  <span>Memory Fill: {formatKB(m.sizeBytes)} KB</span>
                  <span className="font-semibold text-[#17243A]">{pct}%</span>
                </div>
                <div className="w-full h-2 rounded-full bg-[#E2E8F0] overflow-hidden">
                  <div
                    className={`h-full rounded-full transition-all ${
                      m.type === 'ACTIVE' ? 'bg-[#0891D5]' : 'bg-[#7965E8]'
                    }`}
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </div>

              <div className="flex items-center justify-between pt-2 border-t border-[#E2E8F0] text-[11px] text-[#64748B]">
                <span>Entries: {m.entryCount.toLocaleString()} keys</span>
                <span className="text-[#10B981] font-semibold">{m.flushStatus}</span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
