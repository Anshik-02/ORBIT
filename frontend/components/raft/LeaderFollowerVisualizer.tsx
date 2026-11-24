'use client';

import React from 'react';
import { Crown, Check, CheckCircle2, ArrowDown, ArrowUp, Send, ShieldCheck, Heart } from 'lucide-react';

interface LeaderFollowerVisualizerProps {
  shardName: string;
  shardColor: string;
  leader: string;
  leaderHttpPort: string;
  term: number;
  commitIndex: number;
  followers: Array<{
    id: string;
    name: string;
    httpPort: string;
    online: boolean;
    matchIndex: number;
    latency: string;
    lastHeartbeat: string;
  }>;
}

export default function LeaderFollowerVisualizer({
  shardName,
  shardColor,
  leader,
  leaderHttpPort,
  term,
  commitIndex,
  followers,
}: LeaderFollowerVisualizerProps) {
  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs overflow-hidden">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-4 mb-6 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <span
              className="w-3 h-3 rounded-full"
              style={{ backgroundColor: shardColor }}
            />
            <h3 className="text-base font-bold text-[#17243A] font-mono">
              Consensus Topology & Heartbeat Mesh — {shardName}
            </h3>
          </div>
          <p className="text-xs text-[#64748B] font-mono mt-0.5">
            Active AppendEntries RPC pipeline and quorum verification (RF=3)
          </p>
        </div>

        <div className="flex items-center gap-2 font-mono text-xs">
          <span className="px-2.5 py-1 rounded-xl bg-emerald-50 border border-emerald-200 text-[#10B981] font-semibold flex items-center gap-1.5">
            <Heart className="w-3.5 h-3.5 fill-[#10B981] animate-pulse" />
            <span>Leader Heartbeat: 50ms</span>
          </span>
        </div>
      </div>

      {/* Visual Consensus Hierarchy */}
      <div className="relative py-4 flex flex-col items-center">
        {/* LEADER NODE (TOP CENTER) */}
        <div className="relative z-10 w-full max-w-xs p-4 rounded-2xl bg-[#FFFBEB] border-2 border-[#F59E0B] shadow-md text-center font-mono space-y-2">
          <div className="flex items-center justify-center gap-1.5 text-xs font-bold text-[#D97706] uppercase tracking-wider">
            <Crown className="w-4 h-4 text-[#F59E0B]" />
            <span>RAFT GROUP LEADER</span>
          </div>

          <div className="text-xl font-bold text-[#17243A]">
            {leader.toUpperCase()}{' '}
            <span className="text-xs font-normal text-[#64748B]">
              ({leaderHttpPort})
            </span>
          </div>

          <div className="flex items-center justify-around text-[11px] pt-2 border-t border-[#F59E0B]/30 text-[#64748B]">
            <div>
              <span className="block text-[10px] text-[#94A3B8]">TERM</span>
              <span className="font-bold text-[#17243A]">#{term}</span>
            </div>
            <div>
              <span className="block text-[10px] text-[#94A3B8]">COMMIT INDEX</span>
              <span className="font-bold text-[#0891D5]">
                {commitIndex.toLocaleString()}
              </span>
            </div>
            <div>
              <span className="block text-[10px] text-[#94A3B8]">STATE</span>
              <span className="font-bold text-[#10B981]">LEADER</span>
            </div>
          </div>
        </div>

        {/* CONNECTING FLOW ARROWS */}
        <div className="w-full max-w-md h-12 flex items-center justify-around relative">
          <div className="w-1/2 h-full border-l-2 border-r-2 border-b-2 border-dashed border-[#0891D5]/60 rounded-b-2xl relative">
            <span className="absolute -bottom-3 left-1/2 -translate-x-1/2 px-2 py-0.5 rounded-full bg-white border border-[#DCE5F0] text-[9px] font-mono text-[#0891D5] font-bold shadow-2xs">
              AppendEntries RPC ⇄ Ack
            </span>
          </div>
        </div>

        {/* FOLLOWER NODES (BOTTOM ROW) */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 w-full max-w-2xl mt-4">
          {followers.map((f) => (
            <div
              key={f.id}
              className={`p-4 rounded-2xl border font-mono transition-all ${
                f.online
                  ? 'bg-white border-[#DCE5F0] shadow-xs hover:border-[#CBD5E1]'
                  : 'bg-rose-50/50 border-rose-200 shadow-xs'
              }`}
            >
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2">
                  <span
                    className={`w-2.5 h-2.5 rounded-full ${
                      f.online ? 'bg-[#10B981]' : 'bg-[#EF4444]'
                    }`}
                  />
                  <span className="font-bold text-[#17243A] text-sm">
                    {f.name}
                  </span>
                  <span className="text-[11px] text-[#64748B]">
                    ({f.httpPort})
                  </span>
                </div>

                <span
                  className={`text-[10px] px-2 py-0.5 rounded-full font-semibold border ${
                    f.online
                      ? 'bg-slate-100 text-[#64748B] border-slate-200'
                      : 'bg-rose-100 text-rose-700 border-rose-300'
                  }`}
                >
                  {f.online ? 'FOLLOWER' : 'DISCONNECTED'}
                </span>
              </div>

              <div className="space-y-1.5 text-xs text-[#64748B] bg-[#F8FAFC] p-3 rounded-xl border border-[#E2E8F0] mt-3">
                <div className="flex items-center justify-between">
                  <span>Match Index:</span>
                  <span className="font-bold text-[#17243A]">
                    {f.matchIndex.toLocaleString()}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span>Replication Lag:</span>
                  <span className="text-[#10B981] font-semibold">
                    {commitIndex - f.matchIndex} entries (synchronized)
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span>Heartbeat Latency:</span>
                  <span className="text-[#0891D5] font-semibold">
                    {f.online ? f.latency : 'Timeout'}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span>Last AppendEntries:</span>
                  <span className="text-[#17243A]">{f.lastHeartbeat}</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
