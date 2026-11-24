'use client';

import React from 'react';
import {
  RefreshCw,
  Radio,
  Filter,
  CheckCircle2,
  AlertTriangle,
  XCircle,
  Pause,
  Play,
} from 'lucide-react';
import {
  NodeFilterOption,
  ShardFilterOption,
  ConnectionState,
} from '../../lib/topology/types';

interface TopologyHeaderProps {
  connectionState: ConnectionState;
  nodeFilter: NodeFilterOption;
  shardFilter: ShardFilterOption;
  isLiveUpdates: boolean;
  onNodeFilterChange: (val: NodeFilterOption) => void;
  onShardFilterChange: (val: ShardFilterOption) => void;
  onToggleLiveUpdates: () => void;
  onRefresh: () => void;
}

export default function TopologyHeader({
  connectionState,
  nodeFilter,
  shardFilter,
  isLiveUpdates,
  onNodeFilterChange,
  onShardFilterChange,
  onToggleLiveUpdates,
  onRefresh,
}: TopologyHeaderProps) {
  const getStatusBadge = () => {
    switch (connectionState) {
      case 'connected':
        return (
          <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-xs font-semibold shadow-xs">
            <span className="w-2 h-2 rounded-full bg-[#10B981] animate-pulse" />
            CLUSTER LIVE
          </span>
        );
      case 'reconnecting':
        return (
          <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-amber-50 border border-amber-200 text-[#F59E0B] font-mono text-xs font-semibold shadow-xs">
            <span className="w-2 h-2 rounded-full bg-[#F59E0B] animate-pulse" />
            RECONNECTING
          </span>
        );
      case 'disconnected':
      default:
        return (
          <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-rose-50 border border-rose-200 text-[#EF4444] font-mono text-xs font-semibold shadow-xs">
            <span className="w-2 h-2 rounded-full bg-[#EF4444]" />
            DISCONNECTED
          </span>
        );
    }
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
        {/* Left Title & Subtitle */}
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-xl sm:text-2xl font-bold text-[#17243A] tracking-tight">
              Topology
            </h1>
            {getStatusBadge()}
          </div>
          <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal">
            Physical infrastructure and network relationships across cluster machines.
          </p>
        </div>

        {/* Right Controls & Dropdown Filters */}
        <div className="flex flex-wrap items-center gap-2.5">
          {/* Node Filter */}
          <div className="flex items-center gap-1.5 bg-[#F4F7FC] border border-[#DCE5F0] rounded-xl px-3 py-1.5 text-xs font-mono">
            <Filter className="w-3.5 h-3.5 text-[#64748B]" />
            <select
              value={nodeFilter}
              onChange={(e) => onNodeFilterChange(e.target.value as NodeFilterOption)}
              className="bg-transparent text-[#17243A] font-semibold outline-hidden cursor-pointer"
            >
              <option value="all">All Nodes</option>
              <option value="online">Online Nodes</option>
              <option value="offline">Offline Nodes</option>
              <option value="leaders">Leaders</option>
              <option value="followers">Followers</option>
            </select>
          </div>

          {/* Shard Filter */}
          <div className="flex items-center gap-1.5 bg-[#F4F7FC] border border-[#DCE5F0] rounded-xl px-3 py-1.5 text-xs font-mono">
            <span className="w-2 h-2 rounded-full bg-[#0891D5]" />
            <select
              value={shardFilter}
              onChange={(e) => onShardFilterChange(e.target.value as ShardFilterOption)}
              className="bg-transparent text-[#17243A] font-semibold outline-hidden cursor-pointer"
            >
              <option value="all">All Shards</option>
              <option value="0">Shard 0 (A, B, C)</option>
              <option value="1">Shard 1 (D, E, A)</option>
              <option value="2">Shard 2 (B, C, D)</option>
            </select>
          </div>

          {/* Live Updates Toggle */}
          <button
            onClick={onToggleLiveUpdates}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-xl border text-xs font-mono font-semibold transition-all ${
              isLiveUpdates
                ? 'bg-[#0891D5]/10 border-[#0891D5]/30 text-[#0891D5] hover:bg-[#0891D5]/15'
                : 'bg-[#F4F7FC] border-[#DCE5F0] text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            {isLiveUpdates ? (
              <>
                <Pause className="w-3.5 h-3.5 text-[#0891D5]" />
                <span>Live Updates</span>
              </>
            ) : (
              <>
                <Play className="w-3.5 h-3.5" />
                <span>Paused</span>
              </>
            )}
          </button>

          {/* Refresh Snapshot */}
          <button
            onClick={onRefresh}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-white border border-[#DCE5F0] hover:border-[#0891D5] text-[#17243A] hover:text-[#0891D5] text-xs font-mono font-semibold transition-all shadow-2xs"
            title="Fetch latest cluster snapshot"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            <span>↻ Refresh</span>
          </button>
        </div>
      </div>
    </div>
  );
}
