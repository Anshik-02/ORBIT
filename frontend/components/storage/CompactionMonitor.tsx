'use client';

import React, { useState } from 'react';
import { RefreshCw, Play, RotateCcw, CheckCircle2, ArrowRight } from 'lucide-react';

export interface CompactionJob {
  jobId: string;
  sourceLevel: string;
  destLevel: string;
  inputFiles: string[];
  outputFiles: string[];
  progressPct: number;
  status: 'IN_PROGRESS' | 'COMPLETED' | 'IDLE';
}

interface CompactionMonitorProps {
  activeJob?: CompactionJob;
}

export default function CompactionMonitor({
  activeJob = {
    jobId: 'compact-881',
    sourceLevel: 'Level 0',
    destLevel: 'Level 1',
    inputFiles: ['sst_001.sst', 'sst_002.sst'],
    outputFiles: ['sst_005.sst'],
    progressPct: 78,
    status: 'IN_PROGRESS',
  },
}: CompactionMonitorProps) {
  const [job, setJob] = useState<CompactionJob>(activeJob);

  const handleSimulateCompaction = () => {
    setJob((prev) => ({ ...prev, progressPct: 15, status: 'IN_PROGRESS' }));
    setTimeout(() => setJob((prev) => ({ ...prev, progressPct: 55 })), 800);
    setTimeout(() => setJob((prev) => ({ ...prev, progressPct: 90 })), 1600);
    setTimeout(
      () =>
        setJob((prev) => ({
          ...prev,
          progressPct: 100,
          status: 'COMPLETED',
        })),
      2400
    );
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-3 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-amber-50 flex items-center justify-center">
              <RefreshCw className="w-3.5 h-3.5 text-[#F59E0B]" />
            </div>
            <h3 className="text-base font-bold text-[#17243A]">
              Background Compaction Engine Monitor
            </h3>
          </div>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            K-way merge sort of overlapping SSTables into higher tiered levels
          </p>
        </div>

        <button
          onClick={handleSimulateCompaction}
          className="px-3.5 py-1.5 rounded-xl bg-[#F4F7FC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#17243A] hover:text-[#0891D5] font-semibold flex items-center gap-1.5 self-start sm:self-auto transition-all shadow-2xs"
        >
          <Play className="w-3 h-3 fill-current text-[#0891D5]" />
          <span>Trigger Compaction</span>
        </button>
      </div>

      <div className="p-4 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <span className="font-bold text-[#17243A]">Job {job.jobId}</span>
            <span className="text-[#64748B]">
              ({job.sourceLevel} <ArrowRight className="w-3 h-3 inline text-[#0891D5]" /> {job.destLevel})
            </span>
          </div>

          <span
            className={`px-2.5 py-0.5 rounded-full border text-[10px] font-bold ${
              job.status === 'COMPLETED'
                ? 'bg-emerald-50 text-[#10B981] border-emerald-200'
                : 'bg-amber-50 text-[#D97706] border-amber-200'
            }`}
          >
            {job.status === 'COMPLETED' ? 'COMPACTION FINISHED' : 'MERGING SSTABLES'}
          </span>
        </div>

        {/* Progress Bar */}
        <div className="space-y-1">
          <div className="flex justify-between text-[11px] text-[#64748B]">
            <span>Merge Sort Progress:</span>
            <span className="font-bold text-[#17243A]">{job.progressPct}%</span>
          </div>
          <div className="w-full h-2 rounded-full bg-[#E2E8F0] overflow-hidden">
            <div
              className="h-full bg-gradient-to-r from-[#0891D5] to-[#10B981] rounded-full transition-all duration-300"
              style={{ width: `${job.progressPct}%` }}
            />
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-[11px] text-[#64748B] pt-2 border-t border-[#E2E8F0]">
          <div>
            <span className="block text-[#94A3B8]">Input Files (Compacted):</span>
            <span className="font-semibold text-[#17243A]">
              {job.inputFiles.join(', ')}
            </span>
          </div>
          <div>
            <span className="block text-[#94A3B8]">Output Target (Sorted):</span>
            <span className="font-semibold text-[#0891D5]">
              {job.outputFiles.join(', ')}
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
