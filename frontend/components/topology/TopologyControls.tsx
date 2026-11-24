'use client';

import React from 'react';
import { Box, Grid, Network, Share2, RotateCcw, ZoomIn, ZoomOut } from 'lucide-react';
import { ViewMode, NetworkMode } from '../../lib/topology/types';

interface TopologyControlsProps {
  viewMode: ViewMode;
  networkMode: NetworkMode;
  onViewModeChange: (mode: ViewMode) => void;
  onNetworkModeChange: (mode: NetworkMode) => void;
  onResetView?: () => void;
  onZoomIn?: () => void;
  onZoomOut?: () => void;
}

export default function TopologyControls({
  viewMode,
  networkMode,
  onViewModeChange,
  onNetworkModeChange,
  onResetView,
  onZoomIn,
  onZoomOut,
}: TopologyControlsProps) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 p-3 bg-white/90 backdrop-blur-md border border-[#DCE5F0] rounded-xl shadow-xs">
      {/* Left: View Mode (3D vs 2D) */}
      <div className="flex items-center gap-1 p-1 rounded-lg bg-[#F4F7FC] border border-[#DCE5F0]">
        <button
          onClick={() => onViewModeChange('3d')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-mono font-semibold transition-all ${
            viewMode === '3d'
              ? 'bg-white text-[#0891D5] shadow-xs border border-[#DCE5F0]'
              : 'text-[#64748B] hover:text-[#17243A]'
          }`}
        >
          <Box className="w-3.5 h-3.5" />
          <span>3D VIEW</span>
        </button>
        <button
          onClick={() => onViewModeChange('2d')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-mono font-semibold transition-all ${
            viewMode === '2d'
              ? 'bg-white text-[#0891D5] shadow-xs border border-[#DCE5F0]'
              : 'text-[#64748B] hover:text-[#17243A]'
          }`}
        >
          <Grid className="w-3.5 h-3.5" />
          <span>2D VIEW</span>
        </button>
      </div>

      {/* Middle: Network Connections Mode (Physical vs Raft) */}
      <div className="flex items-center gap-1 p-1 rounded-lg bg-[#F4F7FC] border border-[#DCE5F0]">
        <button
          onClick={() => onNetworkModeChange('physical')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-mono font-semibold transition-all ${
            networkMode === 'physical'
              ? 'bg-white text-[#0891D5] shadow-xs border border-[#DCE5F0]'
              : 'text-[#64748B] hover:text-[#17243A]'
          }`}
        >
          <Network className="w-3.5 h-3.5" />
          <span>PHYSICAL NETWORK</span>
        </button>
        <button
          onClick={() => onNetworkModeChange('raft')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-mono font-semibold transition-all ${
            networkMode === 'raft'
              ? 'bg-white text-[#7965E8] shadow-xs border border-[#DCE5F0]'
              : 'text-[#64748B] hover:text-[#17243A]'
          }`}
        >
          <Share2 className="w-3.5 h-3.5" />
          <span>RAFT RELATIONSHIPS</span>
        </button>
      </div>

      {/* Right: Camera Controls */}
      <div className="flex items-center gap-1.5">
        {onZoomIn && (
          <button
            onClick={onZoomIn}
            className="p-1.5 rounded-lg bg-[#F4F7FC] hover:bg-slate-200/70 border border-[#DCE5F0] text-[#64748B] hover:text-[#17243A] transition-all"
            title="Zoom In"
          >
            <ZoomIn className="w-4 h-4" />
          </button>
        )}
        {onZoomOut && (
          <button
            onClick={onZoomOut}
            className="p-1.5 rounded-lg bg-[#F4F7FC] hover:bg-slate-200/70 border border-[#DCE5F0] text-[#64748B] hover:text-[#17243A] transition-all"
            title="Zoom Out"
          >
            <ZoomOut className="w-4 h-4" />
          </button>
        )}
        {onResetView && (
          <button
            onClick={onResetView}
            className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-[#F4F7FC] hover:bg-slate-200/70 border border-[#DCE5F0] text-xs font-mono text-[#64748B] hover:text-[#17243A] transition-all"
            title="Reset Camera & Selection"
          >
            <RotateCcw className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Reset View</span>
          </button>
        )}
      </div>
    </div>
  );
}
