'use client';

import React, { useState } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import {
  Layers,
  ShieldCheck,
  CheckCircle2,
  Crown,
  Activity,
  RefreshCw,
  SlidersHorizontal,
} from 'lucide-react';
import ShardSummaryCards from '@/components/shards/ShardSummaryCards';
import ShardDistributionCards, { ShardDetail } from '@/components/shards/ShardDistributionCards';
import ReplicaPlacementMatrix from '@/components/shards/ReplicaPlacementMatrix';
import SelectedShardInspector from '@/components/shards/SelectedShardInspector';
import RebalancePipeline from '@/components/shards/RebalancePipeline';
import ShardAnalytics from '@/components/shards/ShardAnalytics';
import { useShardsData } from '@/hooks/useShardsData';
import LiveIndicator from '@/components/shared/LiveIndicator';

export default function ShardsPage() {
  const {
    shards,
    matrixNodes,
    isLive,
    isLoading,
    isRefreshing,
    lastUpdated,
    refresh,
  } = useShardsData();
  const [selectedShardId, setSelectedShardId] = useState<number>(0);

  const selectedShard =
    shards.find((s) => s.shardId === selectedShardId) || shards[0];

  const handleRefresh = () => {
    refresh();
  };

  return (
    <DashboardLayout activeNav="shards" onRefresh={handleRefresh}>
      <div className="space-y-6">
        {/* 01. PAGE HEADER */}
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl sm:text-2xl font-bold text-[#17243A] tracking-tight font-mono">
                Shards & Replication
              </h1>
              <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-xs font-semibold">
                <CheckCircle2 className="w-3.5 h-3.5" />
                ALL QUORUMS SATISFIED
              </span>
              <LiveIndicator isLive={isLive} lastUpdated={lastUpdated} />
            </div>
            <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal font-mono">
              Partition distribution, triple replica placement (RF=3) and zero-downtime rebalancing.
            </p>
          </div>

          <div className="flex items-center gap-2 font-mono text-xs">
            <span className="px-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A] font-semibold">
              3 Logical Partitions • 64-bit Hash Ring
            </span>
            <button
              onClick={handleRefresh}
              className="p-2 rounded-xl bg-[#F4F7FC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#64748B] hover:text-[#17243A] transition-colors"
              title="Refresh Telemetry"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${isRefreshing ? 'animate-spin text-[#0891D5]' : ''}`} />
            </button>
          </div>
        </div>

        {/* 02. BOX 1: HIGH-LEVEL SHARD SUMMARY CARDS */}
        <ShardSummaryCards
          totalShards={shards.length}
          healthyQuorums={shards.filter((s) => s.quorumStatus.includes('HEALTHY') || s.quorumStatus.includes('3/3')).length || 3}
          replicationFactor={3}
          activeMigrations={0}
        />

        {/* 03. BOX 2: SHARD DISTRIBUTION CARDS */}
        <div className="space-y-2">
          <div className="flex items-center justify-between font-mono text-xs text-[#64748B] px-1">
            <span className="font-semibold uppercase tracking-wider text-[11px]">
              LOGICAL SHARD REPLICAS
            </span>
            <span>Click any card to inspect quorum & sync health</span>
          </div>
          <ShardDistributionCards
            shards={shards}
            selectedShardId={selectedShardId}
            onSelectShard={setSelectedShardId}
          />
        </div>

        {/* 04. BOX 3: REPLICA PLACEMENT MATRIX */}
        <ReplicaPlacementMatrix
          nodes={matrixNodes}
          selectedShardId={selectedShardId}
          onSelectShard={setSelectedShardId}
        />

        {/* 05. BOX 4: SELECTED SHARD INSPECTOR */}
        <SelectedShardInspector shard={selectedShard} />

        {/* 06. BOX 5: ZERO-DOWNTIME REBALANCING & MIGRATION PIPELINE */}
        <RebalancePipeline />

        {/* 07. BOX 6: DISTRIBUTION ANALYTICS & QUORUM FAULT TOLERANCE */}
        <ShardAnalytics />
      </div>
    </DashboardLayout>
  );
}
