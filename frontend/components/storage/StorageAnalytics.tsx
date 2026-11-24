'use client';

import React from 'react';
import { BarChart3, Database, HardDrive, CheckCircle2 } from 'lucide-react';

export default function StorageAnalytics() {
  const levels = [
    { name: 'Level 0 (Unsorted Flushes)', sstables: 2, sizeMb: 0.8, color: '#F59E0B' },
    { name: 'Level 1 (Non-Overlapping Sorted)', sstables: 4, sizeMb: 4.2, color: '#0891D5' },
    { name: 'Level 2 (Tiered Long-Term Archive)', sstables: 2, sizeMb: 18.6, color: '#7965E8' },
  ];

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 font-mono text-xs">
      {/* 01. LEVEL STORAGE DISTRIBUTION */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between pb-3 mb-4 border-b border-[#DCE5F0]">
            <div>
              <h3 className="text-base font-bold text-[#17243A]">
                SSTable Tiered Level Distribution
              </h3>
              <p className="text-[#64748B] text-xs font-normal mt-0.5">
                Disk space utilization across L0, L1 and L2 partitions
              </p>
            </div>
            <span className="p-1.5 rounded-lg bg-blue-50 text-[#0891D5]">
              <Database className="w-4 h-4" />
            </span>
          </div>

          <div className="space-y-4">
            {levels.map((lvl) => (
              <div key={lvl.name} className="space-y-1.5">
                <div className="flex justify-between">
                  <span className="font-semibold text-[#17243A]">{lvl.name}</span>
                  <span className="text-[#64748B]">
                    {lvl.sstables} files • {lvl.sizeMb} MB
                  </span>
                </div>
                <div className="w-full h-2 rounded-full bg-[#F1F5F9] overflow-hidden">
                  <div
                    className="h-full rounded-full"
                    style={{
                      width: `${(lvl.sizeMb / 24) * 100}%`,
                      backgroundColor: lvl.color,
                    }}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="mt-5 pt-3 border-t border-[#E2E8F0] flex items-center justify-between text-[#64748B]">
          <span>Total Persistent Disk Usage:</span>
          <span className="text-[#17243A] font-bold">23.6 MB</span>
        </div>
      </div>

      {/* 02. AMPLIFICATION METRICS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between pb-3 mb-4 border-b border-[#DCE5F0]">
            <div>
              <h3 className="text-base font-bold text-[#17243A]">
                LSM Amplification & Efficiency Audit
              </h3>
              <p className="text-[#64748B] text-xs font-normal mt-0.5">
                Engine overhead ratios measuring I/O efficiency
              </p>
            </div>
            <span className="p-1.5 rounded-lg bg-emerald-50 text-[#10B981]">
              <BarChart3 className="w-4 h-4" />
            </span>
          </div>

          <div className="grid grid-cols-3 gap-3 text-center">
            <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[10px] text-[#64748B] block">Write Amplification</span>
              <span className="text-xl font-bold text-[#0891D5] block mt-1">1.8x</span>
              <span className="text-[10px] text-[#10B981]">Optimal</span>
            </div>
            <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[10px] text-[#64748B] block">Space Amplification</span>
              <span className="text-xl font-bold text-[#10B981] block mt-1">1.15x</span>
              <span className="text-[10px] text-[#10B981]">&lt; 1.3x Target</span>
            </div>
            <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[10px] text-[#64748B] block">Read Amplification</span>
              <span className="text-xl font-bold text-[#7965E8] block mt-1">1.2x</span>
              <span className="text-[10px] text-[#10B981]">Bloom Accelerated</span>
            </div>
          </div>

          <div className="p-3 rounded-xl bg-emerald-50/60 border border-emerald-200 mt-4 flex items-center gap-2 text-emerald-800">
            <CheckCircle2 className="w-4 h-4 text-emerald-600 shrink-0" />
            <span className="text-[11px] leading-tight font-medium">
              Bloom filter false positive rate &lt; 1.0%, eliminating 99% of unnecessary SSTable disk lookups.
            </span>
          </div>
        </div>

        <div className="mt-5 pt-3 border-t border-[#E2E8F0] flex items-center justify-between text-[#64748B]">
          <span>Sparse Index Resolution:</span>
          <span className="text-[#0891D5] font-semibold">1 Index Entry per 4KB Block</span>
        </div>
      </div>
    </div>
  );
}
