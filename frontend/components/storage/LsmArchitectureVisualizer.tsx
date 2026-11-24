'use client';

import React, { useState } from 'react';
import {
  ArrowDown,
  ArrowRight,
  HardDrive,
  Cpu,
  Layers,
  Database,
  Play,
  RotateCcw,
  CheckCircle2,
  Sparkles,
} from 'lucide-react';

export default function LsmArchitectureVisualizer() {
  const [activeStep, setActiveStep] = useState<number>(-1);
  const [isSimulating, setIsSimulating] = useState(false);

  const steps = [
    {
      title: '01. Client Write Request',
      badge: 'HTTP / Raft Proposal',
      color: '#0891D5',
      desc: 'KV operation proposed to Raft quorum.',
    },
    {
      title: '02. Write-Ahead Log (WAL)',
      badge: 'Append-Only Disk',
      color: '#0891D5',
      desc: 'Synchronously fsynced to disk for crash durability.',
    },
    {
      title: '03. Memtable (SkipList)',
      badge: 'RAM Memory',
      color: '#10B981',
      desc: 'Concurrent ordered key-value SkipList in RAM memory.',
    },
    {
      title: '04. Immutable Memtable',
      badge: 'Read-Only Freeze',
      color: '#7965E8',
      desc: 'Frozen when threshold reached (64MB). Awaiting background flush.',
    },
    {
      title: '05. L0 SSTables',
      badge: 'Sorted String Table',
      color: '#F59E0B',
      desc: 'Flushed to persistent SSTable with Bloom filters & Sparse Indexes.',
    },
    {
      title: '06. Leveled Compaction',
      badge: 'L1 & L2 Merging',
      color: '#17243A',
      desc: 'Background k-way merge sort purges tombstones & compacts levels.',
    },
  ];

  const handleSimulate = () => {
    setIsSimulating(true);
    setActiveStep(0);

    const runStep = (idx: number) => {
      if (idx < steps.length) {
        setActiveStep(idx);
        setTimeout(() => runStep(idx + 1), 900);
      } else {
        setIsSimulating(false);
      }
    };

    setTimeout(() => runStep(1), 900);
  };

  const handleReset = () => {
    setActiveStep(-1);
    setIsSimulating(false);
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-6 font-mono text-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-4 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <span className="p-1.5 rounded-lg bg-[#0891D5]/10 text-[#0891D5]">
              <Layers className="w-4 h-4" />
            </span>
            <h3 className="text-base font-bold text-[#17243A]">
              LSM-Tree Storage Engine Architecture Pipeline
            </h3>
          </div>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Durability guarantee and tiered disk compaction hierarchy
          </p>
        </div>

        <div className="flex items-center gap-2">
          {activeStep === -1 ? (
            <button
              onClick={handleSimulate}
              className="px-3.5 py-1.5 rounded-xl bg-[#0891D5] hover:bg-[#0284C7] text-white font-bold flex items-center gap-1.5 shadow-xs transition-all"
            >
              <Play className="w-3.5 h-3.5 fill-current" />
              <span>Trace Write Pipeline</span>
            </button>
          ) : (
            <button
              onClick={handleReset}
              className="px-3.5 py-1.5 rounded-xl bg-[#F4F7FC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#17243A] font-semibold flex items-center gap-1.5 transition-all shadow-2xs"
            >
              <RotateCcw className="w-3.5 h-3.5" />
              <span>Reset Pipeline</span>
            </button>
          )}
        </div>
      </div>

      {/* Visual Pipeline Blocks */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-6 gap-3">
        {steps.map((st, i) => {
          const isActive = activeStep === i;
          const isDone = activeStep > i;

          return (
            <div
              key={st.title}
              className={`p-4 rounded-xl border transition-all flex flex-col justify-between ${
                isActive
                  ? 'bg-blue-50/70 border-[#0891D5] ring-2 ring-[#0891D5]/30 shadow-md scale-102'
                  : isDone
                  ? 'bg-emerald-50/30 border-emerald-200 shadow-2xs'
                  : 'bg-[#F8FAFC] border-[#E2E8F0]'
              }`}
            >
              <div>
                <div className="flex items-center justify-between mb-2">
                  <span
                    className={`text-[10px] font-bold ${
                      isActive
                        ? 'text-[#0891D5]'
                        : isDone
                        ? 'text-[#10B981]'
                        : 'text-[#64748B]'
                    }`}
                  >
                    {st.title}
                  </span>
                  {isDone ? (
                    <CheckCircle2 className="w-3.5 h-3.5 text-[#10B981]" />
                  ) : isActive ? (
                    <span className="w-2 h-2 rounded-full bg-[#0891D5] animate-ping" />
                  ) : (
                    <span className="w-2 h-2 rounded-full bg-slate-300" />
                  )}
                </div>

                <p className="text-[11px] text-[#64748B] leading-relaxed">
                  {st.desc}
                </p>
              </div>

              <div className="mt-3 pt-2 border-t border-[#E2E8F0]">
                <span
                  className={`text-[9px] px-1.5 py-0.5 rounded font-bold ${
                    isActive
                      ? 'bg-blue-100 text-[#0891D5]'
                      : isDone
                      ? 'bg-emerald-100 text-[#10B981]'
                      : 'bg-slate-200/60 text-[#64748B]'
                  }`}
                >
                  {st.badge}
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
