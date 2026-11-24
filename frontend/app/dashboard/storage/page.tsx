'use client';

import React, { useState } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { HardDrive, CheckCircle2, RefreshCw, Database } from 'lucide-react';
import StorageOverviewCards from '@/components/storage/StorageOverviewCards';
import LsmArchitectureVisualizer from '@/components/storage/LsmArchitectureVisualizer';
import MemtableInspector, { MemtableItem } from '@/components/storage/MemtableInspector';
import WalExplorer, { WalLogFile } from '@/components/storage/WalExplorer';
import SSTableExplorer, { SSTableItem } from '@/components/storage/SSTableExplorer';
import CompactionMonitor from '@/components/storage/CompactionMonitor';
import StorageAnalytics from '@/components/storage/StorageAnalytics';

const INITIAL_MEMTABLES: MemtableItem[] = [
  {
    id: 'memtable-active-s0',
    shardId: 0,
    type: 'ACTIVE',
    entryCount: 1420,
    sizeBytes: 18432000, // 18.4 MB
    maxSizeBytes: 67108864, // 64 MB
    flushStatus: 'Accepting concurrent writes',
  },
  {
    id: 'memtable-active-s1',
    shardId: 1,
    type: 'ACTIVE',
    entryCount: 980,
    sizeBytes: 12288000,
    maxSizeBytes: 67108864,
    flushStatus: 'Accepting concurrent writes',
  },
  {
    id: 'memtable-active-s2',
    shardId: 2,
    type: 'ACTIVE',
    entryCount: 1110,
    sizeBytes: 14336000,
    maxSizeBytes: 67108864,
    flushStatus: 'Accepting concurrent writes',
  },
  {
    id: 'memtable-frozen-s0',
    shardId: 0,
    type: 'IMMUTABLE',
    entryCount: 4890,
    sizeBytes: 67108864, // 64 MB
    maxSizeBytes: 67108864,
    flushStatus: 'Queued for L0 SSTable flush',
  },
];

const INITIAL_WAL_FILES: WalLogFile[] = [
  {
    filename: 'wal_shard0_0003.log',
    shardId: 0,
    sizeBytes: 12582912,
    entryCount: 14820,
    lastIndex: 14820,
    status: 'ACTIVE_APPEND',
  },
  {
    filename: 'wal_shard1_0002.log',
    shardId: 1,
    sizeBytes: 8388608,
    entryCount: 14818,
    lastIndex: 14818,
    status: 'ACTIVE_APPEND',
  },
  {
    filename: 'wal_shard2_0004.log',
    shardId: 2,
    sizeBytes: 10485760,
    entryCount: 14822,
    lastIndex: 14822,
    status: 'ACTIVE_APPEND',
  },
  {
    filename: 'wal_shard0_0002.log',
    shardId: 0,
    sizeBytes: 67108864,
    entryCount: 4890,
    lastIndex: 13400,
    status: 'ROTATED_CLEAN',
  },
];

const INITIAL_SSTABLES: SSTableItem[] = [
  {
    filename: 'sst_s0_l0_001.sst',
    level: 'L0',
    shardId: 0,
    sizeBytes: 421000,
    keyCount: 3200,
    keyRange: 'user:0001 .. user:1400',
    bloomFilterBits: 10,
    status: 'ACTIVE',
  },
  {
    filename: 'sst_s1_l0_002.sst',
    level: 'L0',
    shardId: 1,
    sizeBytes: 389000,
    keyCount: 2900,
    keyRange: 'session:0001 .. session:2900',
    bloomFilterBits: 10,
    status: 'ACTIVE',
  },
  {
    filename: 'sst_s0_l1_001.sst',
    level: 'L1',
    shardId: 0,
    sizeBytes: 2097152, // 2MB
    keyCount: 16000,
    keyRange: 'account:0001 .. account:8000',
    bloomFilterBits: 10,
    status: 'ACTIVE',
  },
  {
    filename: 'sst_s0_l1_002.sst',
    level: 'L1',
    shardId: 0,
    sizeBytes: 2097152,
    keyCount: 15800,
    keyRange: 'account:8001 .. account:9999',
    bloomFilterBits: 10,
    status: 'ACTIVE',
  },
  {
    filename: 'sst_s1_l1_001.sst',
    level: 'L1',
    shardId: 1,
    sizeBytes: 2097152,
    keyCount: 14200,
    keyRange: 'metric:0001 .. metric:5000',
    bloomFilterBits: 10,
    status: 'ACTIVE',
  },
  {
    filename: 'sst_s2_l1_001.sst',
    level: 'L1',
    shardId: 2,
    sizeBytes: 2097152,
    keyCount: 15100,
    keyRange: 'order:0001 .. order:7500',
    bloomFilterBits: 10,
    status: 'ACTIVE',
  },
  {
    filename: 'sst_s0_l2_001.sst',
    level: 'L2',
    shardId: 0,
    sizeBytes: 10485760, // 10MB
    keyCount: 82000,
    keyRange: 'archive:0001 .. archive:9999',
    bloomFilterBits: 10,
    status: 'ACTIVE',
  },
  {
    filename: 'sst_s1_l2_001.sst',
    level: 'L2',
    shardId: 1,
    sizeBytes: 10485760,
    keyCount: 79000,
    keyRange: 'event_log:0001 .. event_log:9999',
    bloomFilterBits: 10,
    status: 'ACTIVE',
  },
];

export default function StoragePage() {
  const [memtables, setMemtables] = useState<MemtableItem[]>(INITIAL_MEMTABLES);
  const [walFiles, setWalFiles] = useState<WalLogFile[]>(INITIAL_WAL_FILES);
  const [sstables, setSstables] = useState<SSTableItem[]>(INITIAL_SSTABLES);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const handleRefresh = () => {
    setIsRefreshing(true);
    setTimeout(() => setIsRefreshing(false), 400);
  };

  return (
    <DashboardLayout activeNav="storage" onRefresh={handleRefresh}>
      <div className="space-y-6">
        {/* 01. PAGE HEADER */}
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-4 font-mono">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl sm:text-2xl font-bold text-[#17243A] tracking-tight">
                LSM-Tree Storage Engine & Durability
              </h1>
              <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] text-xs font-semibold">
                <CheckCircle2 className="w-3.5 h-3.5" />
                STRICT_SYNC WAL PERSISTENCE
              </span>
            </div>
            <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal">
              Memtable memory buffers, Write-Ahead Log checkpoints and multi-level SSTable compaction.
            </p>
          </div>

          <div className="flex items-center gap-2 text-xs">
            <span className="px-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A] font-semibold">
              SkipList Memory + Leveled Disk
            </span>
            <button
              onClick={handleRefresh}
              className="p-2 rounded-xl bg-[#F4F7FC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#64748B] hover:text-[#17243A] transition-colors"
              title="Refresh Storage Telemetry"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${isRefreshing ? 'animate-spin text-[#0891D5]' : ''}`} />
            </button>
          </div>
        </div>

        {/* 02. SECTION A: STORAGE OVERVIEW METRICS */}
        <StorageOverviewCards
          activeMemtables={3}
          immutableMemtables={1}
          walSizeBytes={31457280}
          sstableCount={sstables.length}
          compactionStatus="IDLE (Optimal)"
        />

        {/* 03. SECTION B: LSM ARCHITECTURE VISUALIZATION */}
        <LsmArchitectureVisualizer />

        {/* 04. SECTION C: MEMTABLE INSPECTOR */}
        <MemtableInspector memtables={memtables} />

        {/* 05. SECTION D: WAL EXPLORER */}
        <WalExplorer files={walFiles} />

        {/* 06. SECTION E: SSTABLE LEVEL EXPLORER */}
        <SSTableExplorer sstables={sstables} />

        {/* 07. SECTION F: COMPACTION MONITOR */}
        <CompactionMonitor />

        {/* 08. SECTION G: STORAGE ANALYTICS */}
        <StorageAnalytics />
      </div>
    </DashboardLayout>
  );
}
