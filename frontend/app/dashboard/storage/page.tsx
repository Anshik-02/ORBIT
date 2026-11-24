'use client';

import React from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { HardDrive, Layers, Database, ShieldAlert, Cpu } from 'lucide-react';

export default function StoragePage() {
  return (
    <DashboardLayout activeNav="storage">
      <div className="space-y-6">
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs">
          <div className="flex items-center gap-2 mb-1">
            <div className="w-8 h-8 rounded-lg bg-[#7965E8]/10 flex items-center justify-center">
              <HardDrive className="w-4 h-4 text-[#7965E8]" />
            </div>
            <h1 className="text-xl font-bold text-[#17243A]">LSM Storage Engine & Durability</h1>
          </div>
          <p className="text-xs text-[#64748B]">
            Memtable lifecycle, Write-Ahead Log (WAL) checkpoints, SSTable level compactions & Bloom filters
          </p>

          <div className="mt-6 grid grid-cols-1 md:grid-cols-4 gap-4 font-mono text-xs">
            <div className="p-4 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[10px] uppercase text-[#64748B] block">Active Memtables</span>
              <span className="text-2xl font-bold text-[#17243A] block mt-1">3 / 3</span>
              <span className="text-[11px] text-[#10B981]">SkipList in memory</span>
            </div>
            <div className="p-4 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[10px] uppercase text-[#64748B] block">WAL Size (Active)</span>
              <span className="text-2xl font-bold text-[#0891D5] block mt-1">12.4 MB</span>
              <span className="text-[11px] text-[#64748B]">Sync on commit (fdatasync)</span>
            </div>
            <div className="p-4 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[10px] uppercase text-[#64748B] block">L0 / L1 SSTables</span>
              <span className="text-2xl font-bold text-[#7965E8] block mt-1">18 files</span>
              <span className="text-[11px] text-[#10B981]">Bloom filter 1% FPR</span>
            </div>
            <div className="p-4 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[10px] uppercase text-[#64748B] block">Compaction State</span>
              <span className="text-2xl font-bold text-[#10B981] block mt-1">IDLE</span>
              <span className="text-[11px] text-[#64748B]">Read amp: 1.4x</span>
            </div>
          </div>
        </div>
      </div>
    </DashboardLayout>
  );
}
