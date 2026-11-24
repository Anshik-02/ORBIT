'use client';

import React from 'react';
import { Wifi, Radio, AlertTriangle } from 'lucide-react';

interface LiveIndicatorProps {
  isLiveBackend: boolean;
  connectionState: 'connected' | 'reconnecting' | 'disconnected';
  onToggleMode?: () => void;
}

export default function LiveIndicator({
  isLiveBackend,
  connectionState,
  onToggleMode,
}: LiveIndicatorProps) {
  if (isLiveBackend) {
    return (
      <div className="flex items-center gap-2">
        <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-xs font-semibold shadow-xs">
          <span className="w-2 h-2 rounded-full bg-[#10B981] animate-pulse" />
          <span>LIVE BACKEND TELEMETRY</span>
        </span>
        {onToggleMode && (
          <button
            onClick={onToggleMode}
            className="text-[10px] font-mono text-[#64748B] hover:text-[#0891D5] underline"
            title="Switch to local simulation"
          >
            Switch to Demo Mode
          </button>
        )}
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2">
      <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-amber-50 border border-amber-200 text-[#D97706] font-mono text-xs font-semibold shadow-xs">
        <AlertTriangle className="w-3.5 h-3.5 text-[#F59E0B]" />
        <span>SIMULATION / DEMO MODE</span>
      </span>
      {onToggleMode && (
        <button
          onClick={onToggleMode}
          className="text-[10px] font-mono text-[#0891D5] hover:underline font-semibold"
          title="Connect to live Go cluster"
        >
          Connect Live Cluster
        </button>
      )}
    </div>
  );
}
