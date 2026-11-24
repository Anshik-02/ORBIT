'use client';

import React from 'react';
import { HardDrive, Layers, Database, RefreshCw, Cpu } from 'lucide-react';

interface StorageOverviewCardsProps {
  activeMemtables?: number;
  immutableMemtables?: number;
  walSizeBytes?: number;
  sstableCount?: number;
  compactionStatus?: string;
}

export default function StorageOverviewCards({
  activeMemtables = 3,
  immutableMemtables = 1,
  walSizeBytes = 12582912, // 12 MB
  sstableCount = 8,
  compactionStatus = 'IDLE (Optimal)',
}: StorageOverviewCardsProps) {
  const formatMB = (bytes: number) => (bytes / (1024 * 1024)).toFixed(1);

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4 font-mono">
      {/* 01. ACTIVE MEMTABLES */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>ACTIVE MEMTABLES</span>
            <span className="p-1.5 rounded-lg bg-[#0891D5]/10 text-[#0891D5]">
              <Cpu className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#17243A]">
            {activeMemtables} / 3
          </div>
        </div>
        <p className="text-[11px] text-[#10B981] mt-2 font-semibold">
          SkipList in-memory writes
        </p>
      </div>

      {/* 02. IMMUTABLE MEMTABLES */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>IMMUTABLE TABLES</span>
            <span className="p-1.5 rounded-lg bg-[#7965E8]/10 text-[#7965E8]">
              <Layers className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#7965E8]">
            {immutableMemtables}
          </div>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2">
          Read-only; queued for L0 flush
        </p>
      </div>

      {/* 03. WAL SIZE */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>ACTIVE WAL SIZE</span>
            <span className="p-1.5 rounded-lg bg-[#0891D5]/10 text-[#0891D5]">
              <HardDrive className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#0891D5]">
            {formatMB(walSizeBytes)} <span className="text-sm font-normal text-[#64748B]">MB</span>
          </div>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2">
          Synchronous append-only log
        </p>
      </div>

      {/* 04. SSTABLE COUNT */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>TOTAL SSTABLES</span>
            <span className="p-1.5 rounded-lg bg-[#10B981]/10 text-[#10B981]">
              <Database className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#10B981]">
            {sstableCount}
          </div>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2">
          L0: 2 • L1: 4 • L2: 2
        </p>
      </div>

      {/* 05. COMPACTION STATUS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between sm:col-span-2 lg:col-span-1">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>COMPACTION ENGINE</span>
            <span className="p-1.5 rounded-lg bg-amber-50 text-[#F59E0B]">
              <RefreshCw className="w-4 h-4" />
            </span>
          </div>
          <div className="text-xl font-bold text-[#17243A]">
            {compactionStatus}
          </div>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2">
          Size-tiered leveling active
        </p>
      </div>
    </div>
  );
}
