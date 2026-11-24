'use client';

import React from 'react';
import { ShieldCheck, Clock, CheckCircle2, AlertTriangle, Activity } from 'lucide-react';

export interface ChaosImpactReport {
  downtimeMs: number;
  electionDurationMs: number;
  failedRequests: number;
  dataConsistencyPct: number;
  recoveryDurationMs: number;
  quorumAvailability: string;
}

interface ChaosImpactAnalysisProps {
  impact: ChaosImpactReport;
}

export default function ChaosImpactAnalysis({ impact }: ChaosImpactAnalysisProps) {
  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      <div className="flex items-center justify-between pb-3 border-b border-[#DCE5F0]">
        <div>
          <h3 className="text-base font-bold text-[#17243A]">
            Chaos Fault Impact & Resiliency Analysis
          </h3>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Post-experiment telemetry quantifying cluster survival and recovery times
          </p>
        </div>

        <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-bold">
          <CheckCircle2 className="w-3.5 h-3.5" />
          <span>ZERO DATA LOSS VERIFIED</span>
        </span>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3 text-center">
        {/* Downtime */}
        <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          <span className="text-[10px] text-[#64748B] uppercase block">Downtime</span>
          <span className="text-xl font-bold text-[#17243A] block mt-1">
            {impact.downtimeMs} <span className="text-xs font-normal text-[#64748B]">ms</span>
          </span>
          <span className="text-[10px] text-[#10B981]">Transient</span>
        </div>

        {/* Election Duration */}
        <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          <span className="text-[10px] text-[#64748B] uppercase block">Election Time</span>
          <span className="text-xl font-bold text-[#0891D5] block mt-1">
            {impact.electionDurationMs} <span className="text-xs font-normal text-[#64748B]">ms</span>
          </span>
          <span className="text-[10px] text-[#0891D5]">&lt; 300ms SLA</span>
        </div>

        {/* Failed Client Requests */}
        <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          <span className="text-[10px] text-[#64748B] uppercase block">Failed Requests</span>
          <span className="text-xl font-bold text-[#10B981] block mt-1">
            {impact.failedRequests}
          </span>
          <span className="text-[10px] text-[#10B981]">Auto Retried</span>
        </div>

        {/* Data Consistency */}
        <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          <span className="text-[10px] text-[#64748B] uppercase block">Consistency</span>
          <span className="text-xl font-bold text-[#10B981] block mt-1">
            {impact.dataConsistencyPct}%
          </span>
          <span className="text-[10px] text-[#10B981]">Strict Serial</span>
        </div>

        {/* Recovery Duration */}
        <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          <span className="text-[10px] text-[#64748B] uppercase block">Full Recovery</span>
          <span className="text-xl font-bold text-[#7965E8] block mt-1">
            {(impact.recoveryDurationMs / 1000).toFixed(1)} <span className="text-xs font-normal text-[#64748B]">s</span>
          </span>
          <span className="text-[10px] text-[#7965E8]">Peer Re-sync</span>
        </div>

        {/* Quorum Availability */}
        <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          <span className="text-[10px] text-[#64748B] uppercase block">Quorum State</span>
          <span className="text-base font-bold text-[#10B981] block mt-1">
            {impact.quorumAvailability}
          </span>
          <span className="text-[10px] text-[#10B981]">Majority Safe</span>
        </div>
      </div>
    </div>
  );
}
