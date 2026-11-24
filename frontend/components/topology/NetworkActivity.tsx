'use client';

import React from 'react';
import { Activity, ShieldCheck, Network, HardDrive, Cpu, Radio } from 'lucide-react';
import { TopologyEvent } from '../../lib/topology/types';

interface NetworkActivityProps {
  events: TopologyEvent[];
}

export default function NetworkActivity({ events }: NetworkActivityProps) {
  const getCategoryBadge = (category: TopologyEvent['category']) => {
    switch (category) {
      case 'NETWORK':
        return 'bg-[#0891D5]/10 text-[#0891D5] border-[#0891D5]/25';
      case 'RAFT':
        return 'bg-[#F59E0B]/10 text-[#D97706] border-[#F59E0B]/25';
      case 'STORAGE':
        return 'bg-[#7965E8]/10 text-[#7965E8] border-[#7965E8]/25';
      case 'CLUSTER':
      default:
        return 'bg-[#10B981]/10 text-[#10B981] border-[#10B981]/25';
    }
  };

  const getLevelDot = (level: TopologyEvent['level']) => {
    switch (level) {
      case 'error':
        return 'bg-[#EF4444] shadow-[0_0_6px_rgba(239,68,68,0.6)]';
      case 'warn':
        return 'bg-[#F59E0B] shadow-[0_0_6px_rgba(245,158,11,0.6)]';
      case 'success':
        return 'bg-[#10B981] shadow-[0_0_6px_rgba(16,185,129,0.6)]';
      case 'info':
      default:
        return 'bg-[#0891D5] shadow-[0_0_6px_rgba(8,145,213,0.6)]';
    }
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs overflow-hidden">
      <div className="flex items-center justify-between pb-4 mb-4 border-b border-[#DCE5F0]">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
            <Activity className="w-4 h-4 text-[#0891D5]" />
          </div>
          <div>
            <h3 className="text-base font-bold text-[#17243A] tracking-tight">
              Network & Consensus Activity Stream
            </h3>
            <p className="text-xs text-[#64748B]">
              Real-time audit journal of peer heartbeats, replication ticks & failover transitions
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2 text-xs font-mono text-[#10B981]">
          <span className="w-2 h-2 rounded-full bg-[#10B981] animate-ping" />
          <span>LIVE AUDIT</span>
        </div>
      </div>

      {/* Events Stream */}
      <div className="space-y-2 max-h-64 overflow-y-auto pr-1 font-mono text-xs">
        {events.map((evt) => (
          <div
            key={evt.id}
            className="flex items-center justify-between p-3 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] hover:border-[#CBD5E1] transition-all"
          >
            <div className="flex items-center gap-3">
              <span className={`w-2 h-2 rounded-full ${getLevelDot(evt.level)} shrink-0`} />
              <span
                className={`text-[9px] px-2 py-0.5 rounded-full border uppercase tracking-wider shrink-0 font-semibold ${getCategoryBadge(
                  evt.category
                )}`}
              >
                {evt.category}
              </span>
              <span className="text-[#64748B] text-[11px] shrink-0 font-semibold">
                [{evt.nodeId}]
              </span>
              <span className="text-[#17243A] text-xs line-clamp-1 font-medium">
                {evt.description}
              </span>
            </div>

            <span className="text-[11px] text-[#64748B] shrink-0 ml-3">
              {evt.timestamp}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
