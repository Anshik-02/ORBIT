'use client';

import React from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { HardDrive, CheckCircle2, RefreshCw, Database } from 'lucide-react';
import StorageOverviewCards from '@/components/storage/StorageOverviewCards';
import LsmArchitectureVisualizer from '@/components/storage/LsmArchitectureVisualizer';
import MemtableInspector, { MemtableItem } from '@/components/storage/MemtableInspector';
import WalExplorer, { WalLogFile } from '@/components/storage/WalExplorer';
import SSTableExplorer, { SSTableItem } from '@/components/storage/SSTableExplorer';
import CompactionMonitor from '@/components/storage/CompactionMonitor';
import StorageAnalytics from '@/components/storage/StorageAnalytics';
import { useStorageMetrics } from '@/hooks/useStorageMetrics';
import LiveIndicator from '@/components/shared/LiveIndicator';

export default function StoragePage() {
  const {
    metrics,
    detailedStats,
    isLive,
    isLoading,
    isRefreshing,
    lastUpdated,
    refresh,
  } = useStorageMetrics();

  // Compute live items dynamically strictly from real backend telemetry
  const displayedMemtables = React.useMemo(() => {
    if (!isLive || !detailedStats) return [];

    const items: MemtableItem[] = [
      {
        id: `memtable-active-${detailedStats.node_id || 'node-a'}`,
        shardId: 0,
        type: 'ACTIVE',
        entryCount: detailedStats.active_memtable?.entry_count ?? 0,
        sizeBytes: detailedStats.active_memtable?.byte_size || 0,
        maxSizeBytes: 67108864,
        flushStatus: 'Accepting concurrent writes',
      },
    ];

    if (detailedStats.immutable_memtables && detailedStats.immutable_memtables.length > 0) {
      detailedStats.immutable_memtables.forEach((imm, idx) => {
        items.push({
          id: `memtable-frozen-imm${idx + 1}`,
          shardId: 0,
          type: 'IMMUTABLE',
          entryCount: imm.entry_count ?? 0,
          sizeBytes: imm.byte_size ?? 0,
          maxSizeBytes: 67108864,
          flushStatus: 'Queued for L0 SSTable flush',
        });
      });
    }

    return items;
  }, [isLive, detailedStats]);

  const displayedWalFiles = React.useMemo(() => {
    if (!isLive || !detailedStats) return [];

    const baseName = detailedStats.wal_path
      ? detailedStats.wal_path.replace(/\\/g, '/').split('/').pop()
      : 'wal.log';

    return [
      {
        filename: baseName || 'wal.log',
        shardId: 0,
        sizeBytes: detailedStats.wal_size_bytes || 0,
        entryCount: Number(detailedStats.wal_record_count) || 0,
        lastIndex: Number(detailedStats.wal_record_count) || 0,
        status: 'ACTIVE_APPEND' as const,
      },
    ];
  }, [isLive, detailedStats]);

  const displayedSstables = React.useMemo(() => {
    if (!isLive || !detailedStats || !detailedStats.sstables) {
      return [];
    }

    return detailedStats.sstables.map((s) => ({
      filename: s.file_name,
      level: `L${s.level}` as 'L0' | 'L1' | 'L2',
      shardId: 0,
      sizeBytes: s.file_size,
      keyCount: s.record_count,
      keyRange: s.min_key && s.max_key ? `${s.min_key} .. ${s.max_key}` : 'empty',
      bloomFilterBits: s.has_bloom_filter ? 10 : 0,
      status: 'ACTIVE' as const,
    }));
  }, [isLive, detailedStats]);

  const handleRefresh = () => {
    refresh();
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
              <LiveIndicator isLive={isLive} lastUpdated={lastUpdated} />
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
          activeMemtables={displayedMemtables.filter((m) => m.type === 'ACTIVE').length}
          immutableMemtables={detailedStats?.immutable_count ?? (metrics.immutableMemtablesCount || 0)}
          walSizeBytes={detailedStats?.wal_size_bytes ?? metrics.walSizeBytes}
          sstableCount={detailedStats?.sstables?.length ?? (metrics.sstableCountL0 + metrics.sstableCountL1 + metrics.sstableCountL2)}
          compactionStatus={detailedStats?.compaction?.is_running ? 'COMPACTING (L0 -> L1)' : 'IDLE (Optimal)'}
        />

        {/* 03. SECTION B: LSM ARCHITECTURE VISUALIZATION */}
        <LsmArchitectureVisualizer />

        {/* 04. SECTION C: MEMTABLE INSPECTOR */}
        <MemtableInspector memtables={displayedMemtables} />

        {/* 05. SECTION D: WAL EXPLORER */}
        <WalExplorer files={displayedWalFiles} />

        {/* 06. SECTION E: SSTABLE LEVEL EXPLORER */}
        <SSTableExplorer sstables={displayedSstables} />

        {/* 07. SECTION F: COMPACTION MONITOR */}
        <CompactionMonitor />

        {/* 08. SECTION G: STORAGE ANALYTICS */}
        <StorageAnalytics />
      </div>
    </DashboardLayout>
  );
}
