'use client';

import React from 'react';
import { Activity, AlertTriangle, AlertCircle, Wifi, Radio } from 'lucide-react';

interface EventSummaryCardsProps {
  totalEvents: number;
  criticalEvents: number;
  warningEvents: number;
  isStreaming: boolean;
}

export default function EventSummaryCards({
  totalEvents = 1284,
  criticalEvents = 0,
  warningEvents = 2,
  isStreaming = true,
}: EventSummaryCardsProps) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 font-mono">
      {/* 01. TOTAL EVENTS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>TOTAL EVENTS</span>
            <span className="p-1.5 rounded-lg bg-[#0891D5]/10 text-[#0891D5]">
              <Activity className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#17243A]">
            {totalEvents.toLocaleString()}
          </div>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2">
          Cluster telemetry notifications
        </p>
      </div>

      {/* 02. CRITICAL EVENTS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>CRITICAL FAULTS</span>
            <span className="p-1.5 rounded-lg bg-rose-50 text-rose-600">
              <AlertCircle className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-rose-600">
            {criticalEvents}
          </div>
        </div>
        <p className="text-[11px] text-[#10B981] mt-2 font-semibold">
          Quorum fully satisfied
        </p>
      </div>

      {/* 03. WARNINGS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>CLUSTER WARNINGS</span>
            <span className="p-1.5 rounded-lg bg-amber-50 text-[#F59E0B]">
              <AlertTriangle className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#D97706]">
            {warningEvents}
          </div>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2">
          Transient lag catch-ups resolved
        </p>
      </div>

      {/* 04. STREAMING STATUS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>WEBSOCKET STREAM</span>
            <span className="p-1.5 rounded-lg bg-emerald-50 text-[#10B981]">
              <Radio className="w-4 h-4 animate-pulse" />
            </span>
          </div>
          <div className="flex items-center gap-2">
            <span className="w-2.5 h-2.5 rounded-full bg-[#10B981] animate-ping" />
            <span className="text-xl font-bold text-[#10B981]">
              {isStreaming ? 'STREAMING' : 'PAUSED'}
            </span>
          </div>
        </div>
        <p className="text-[11px] text-[#64748B] mt-2">
          Real-time /ws/events connected
        </p>
      </div>
    </div>
  );
}
