'use client';

import React from 'react';
import {
  Box,
  Grid,
  Network,
  Share2,
  RotateCcw,
  Compass,
  ArrowUp,
  Move,
  AlertTriangle,
  Radio,
} from 'lucide-react';
import { ViewMode, NetworkMode } from '../../lib/topology/types';

export type CameraPreset = 'isometric' | 'top' | 'front' | 'reset';

interface TopologyControlsProps {
  viewMode: ViewMode;
  networkMode: NetworkMode;
  isLiveBackend: boolean;
  isSimulationMode: boolean;
  onViewModeChange: (mode: ViewMode) => void;
  onNetworkModeChange: (mode: NetworkMode) => void;
  onCameraPreset?: (preset: CameraPreset) => void;
  onToggleSimulationMode?: () => void;
}

export default function TopologyControls({
  viewMode,
  networkMode,
  isLiveBackend,
  isSimulationMode,
  onViewModeChange,
  onNetworkModeChange,
  onCameraPreset,
  onToggleSimulationMode,
}: TopologyControlsProps) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 p-3 bg-white border border-[#DCE5F0] rounded-2xl shadow-xs">
      {/* Left: View Mode (3D vs 2D) & Network Mode */}
      <div className="flex flex-wrap items-center gap-2">
        {/* 3D vs 2D Switcher */}
        <div className="flex items-center gap-1 p-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0]">
          <button
            onClick={() => onViewModeChange('3d')}
            className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-mono font-semibold transition-all ${
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
            className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-mono font-semibold transition-all ${
              viewMode === '2d'
                ? 'bg-white text-[#0891D5] shadow-xs border border-[#DCE5F0]'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            <Grid className="w-3.5 h-3.5" />
            <span>2D VIEW</span>
          </button>
        </div>

        {/* Physical vs Raft Mode Switcher */}
        <div className="flex items-center gap-1 p-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0]">
          <button
            onClick={() => onNetworkModeChange('physical')}
            className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-mono font-semibold transition-all ${
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
            className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-mono font-semibold transition-all ${
              networkMode === 'raft'
                ? 'bg-white text-[#7965E8] shadow-xs border border-[#DCE5F0]'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            <Share2 className="w-3.5 h-3.5" />
            <span>RAFT RELATIONSHIPS</span>
          </button>
        </div>
      </div>

      {/* Right: Camera Presets & Telemetry Indicator */}
      <div className="flex flex-wrap items-center gap-2">
        {/* Camera Presets in 3D Mode */}
        {viewMode === '3d' && onCameraPreset && (
          <div className="flex items-center gap-1 p-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-xs font-mono">
            <button
              onClick={() => onCameraPreset('isometric')}
              className="px-2.5 py-1 rounded-lg hover:bg-white text-[#64748B] hover:text-[#17243A] transition-all flex items-center gap-1"
              title="Isometric angle"
            >
              <Compass className="w-3 h-3" />
              <span>ISOMETRIC</span>
            </button>
            <button
              onClick={() => onCameraPreset('top')}
              className="px-2.5 py-1 rounded-lg hover:bg-white text-[#64748B] hover:text-[#17243A] transition-all flex items-center gap-1"
              title="Top-down perspective"
            >
              <ArrowUp className="w-3 h-3" />
              <span>TOP VIEW</span>
            </button>
            <button
              onClick={() => onCameraPreset('front')}
              className="px-2.5 py-1 rounded-lg hover:bg-white text-[#64748B] hover:text-[#17243A] transition-all flex items-center gap-1"
              title="Eye-level free angle"
            >
              <Move className="w-3 h-3" />
              <span>FREE VIEW</span>
            </button>
            <button
              onClick={() => onCameraPreset('reset')}
              className="px-2 py-1 rounded-lg bg-white border border-[#DCE5F0] text-[#17243A] hover:text-[#0891D5] transition-all flex items-center gap-1 shadow-2xs font-semibold"
              title="Reset camera & selection"
            >
              <RotateCcw className="w-3 h-3" />
              <span>RESET</span>
            </button>
          </div>
        )}

        {/* Telemetry Badge */}
        {onToggleSimulationMode && (
          <button
            onClick={onToggleSimulationMode}
            className={`px-3 py-1.5 rounded-xl border text-xs font-mono font-semibold flex items-center gap-1.5 transition-all shadow-2xs ${
              isLiveBackend && !isSimulationMode
                ? 'bg-emerald-50 border-emerald-200 text-[#10B981]'
                : 'bg-amber-50 border-amber-200 text-[#D97706]'
            }`}
            title="Click to toggle live cluster vs demo mode"
          >
            {isLiveBackend && !isSimulationMode ? (
              <>
                <span className="w-2 h-2 rounded-full bg-[#10B981] animate-pulse" />
                <span>LIVE TELEMETRY</span>
              </>
            ) : (
              <>
                <AlertTriangle className="w-3.5 h-3.5 text-[#F59E0B]" />
                <span>DEMO / SIMULATION</span>
              </>
            )}
          </button>
        )}
      </div>
    </div>
  );
}
