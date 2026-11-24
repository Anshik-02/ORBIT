'use client';

import React from 'react';
import Link from 'next/link';
import {
  X,
  Server,
  Activity,
  Layers,
  ShieldCheck,
  Crown,
  Radio,
  Power,
  ArrowRight,
  Wifi,
  WifiOff,
} from 'lucide-react';
import { PhysicalNode } from '../../lib/topology/types';

interface NodeInspectorProps {
  node: PhysicalNode | null;
  onClose: () => void;
  onToggleOffline: (nodeId: string) => void;
}

export default function NodeInspector({
  node,
  onClose,
  onToggleOffline,
}: NodeInspectorProps) {
  if (!node) return null;

  return (
    <div className="w-full lg:w-96 bg-white border border-[#DCE5F0] rounded-2xl shadow-xl flex flex-col justify-between overflow-hidden animate-fadeIn">
      {/* Top Header */}
      <div>
        <div className="p-4 border-b border-[#DCE5F0] flex items-center justify-between bg-[#F8FAFC]">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
              <Server className="w-4 h-4 text-[#0891D5]" />
            </div>
            <div>
              <span className="text-[10px] font-mono uppercase tracking-wider text-[#64748B] block">
                NODE INSPECTOR
              </span>
              <h2 className="text-base font-bold text-[#17243A] leading-tight font-mono">
                {node.name} <span className="text-xs text-[#64748B] font-normal">({node.id})</span>
              </h2>
            </div>
          </div>

          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-[#64748B] hover:text-[#17243A] hover:bg-slate-200/60 transition-colors"
            aria-label="Close Inspector"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Live Status Pill */}
        <div className="p-4 border-b border-[#F1F5F9]">
          <div className="flex items-center justify-between">
            <span className="text-xs font-mono text-[#64748B]">Operational State</span>
            {node.online ? (
              <span className="flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-xs font-semibold">
                <span className="w-2 h-2 rounded-full bg-[#10B981] animate-pulse" />
                ONLINE
              </span>
            ) : (
              <span className="flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-rose-50 border border-rose-200 text-[#EF4444] font-mono text-xs font-semibold">
                <span className="w-2 h-2 rounded-full bg-[#EF4444]" />
                OFFLINE (HEARTBEAT LOST)
              </span>
            )}
          </div>
        </div>

        <div className="p-4 space-y-5 overflow-y-auto max-h-[calc(100vh-380px)] font-mono text-xs">
          {/* GENERAL SECTION */}
          <div>
            <span className="text-[10px] uppercase font-semibold text-[#64748B] tracking-wider block mb-2">
              GENERAL
            </span>
            <div className="space-y-1.5 bg-[#F8FAFC] p-3 rounded-xl border border-[#E2E8F0]">
              <div className="flex items-center justify-between">
                <span className="text-[#64748B]">Node ID</span>
                <span className="font-semibold text-[#17243A]">{node.id}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-[#64748B]">HTTP Address</span>
                <span className="text-[#0891D5] font-semibold">{node.httpAddress}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-[#64748B]">Raft Transport</span>
                <span className="text-[#7965E8] font-semibold">{node.raftAddress}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-[#64748B]">Last Heartbeat</span>
                <span className={node.online ? 'text-[#10B981]' : 'text-rose-500'}>
                  {node.lastObservedActivity}
                </span>
              </div>
            </div>
          </div>

          {/* HOSTED SHARDS */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-[10px] uppercase font-semibold text-[#64748B] tracking-wider">
                HOSTED SHARDS
              </span>
              <span className="text-[10px] text-[#64748B]">{node.hostedShards.length} assigned</span>
            </div>

            <div className="space-y-2">
              {node.hostedShards.map((sh) => (
                <div
                  key={sh.shardId}
                  className="p-3 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] hover:border-[#CBD5E1] transition-all"
                >
                  <div className="flex items-center justify-between mb-1.5">
                    <div className="flex items-center gap-2">
                      <span
                        className="w-2.5 h-2.5 rounded-full"
                        style={{ backgroundColor: sh.color }}
                      />
                      <span className="font-bold text-[#17243A] text-xs">
                        {sh.shardName}
                      </span>
                    </div>

                    {sh.role === 'Leader' ? (
                      <span className="flex items-center gap-1 text-[10px] px-2 py-0.5 rounded-full bg-amber-50 border border-amber-200 text-[#D97706] font-bold">
                        <Crown className="w-3 h-3 text-[#F59E0B]" />
                        LEADER
                      </span>
                    ) : (
                      <span className="text-[10px] px-2 py-0.5 rounded-full bg-slate-100 border border-slate-200 text-[#64748B] font-semibold">
                        FOLLOWER
                      </span>
                    )}
                  </div>

                  <div className="flex items-center justify-between text-[11px] text-[#64748B]">
                    <span>Consensus Term: #{sh.term}</span>
                    <span>Commit Idx: {sh.commitIndex.toLocaleString()}</span>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* RAFT PARTICIPATION */}
          <div>
            <span className="text-[10px] uppercase font-semibold text-[#64748B] tracking-wider block mb-2">
              RAFT PARTICIPATION
            </span>
            <div className="grid grid-cols-3 gap-2">
              <div className="p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] text-center">
                <span className="text-[10px] text-[#64748B] block">Hosted</span>
                <span className="text-base font-bold text-[#17243A]">
                  {node.raftGroupCount}
                </span>
              </div>
              <div className="p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] text-center">
                <span className="text-[10px] text-[#64748B] block">Leader</span>
                <span className="text-base font-bold text-[#D97706]">
                  {node.leaderGroupCount}
                </span>
              </div>
              <div className="p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] text-center">
                <span className="text-[10px] text-[#64748B] block">Follower</span>
                <span className="text-base font-bold text-[#64748B]">
                  {node.followerGroupCount}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Bottom Actions */}
      <div className="p-4 border-t border-[#DCE5F0] bg-[#F8FAFC] space-y-2">
        {/* Toggle Failover Button */}
        <button
          onClick={() => onToggleOffline(node.id)}
          className={`w-full py-2 px-3 rounded-xl font-mono text-xs font-semibold flex items-center justify-center gap-2 transition-all shadow-xs ${
            node.online
              ? 'bg-rose-50 hover:bg-rose-100/80 border border-rose-200 text-rose-600'
              : 'bg-emerald-50 hover:bg-emerald-100/80 border border-emerald-200 text-emerald-700'
          }`}
        >
          <Power className="w-3.5 h-3.5" />
          <span>
            {node.online
              ? `Simulate Kill / Offline (${node.name})`
              : `Restart Node (${node.name})`}
          </span>
        </button>

        {/* Link to Raft page */}
        <Link
          href="/dashboard/raft"
          className="w-full py-2 px-3 rounded-xl bg-white hover:bg-slate-50 border border-[#DCE5F0] text-[#17243A] hover:text-[#0891D5] font-mono text-xs font-semibold flex items-center justify-center gap-1.5 transition-all shadow-2xs"
        >
          <span>VIEW RAFT DETAILS</span>
          <ArrowRight className="w-3.5 h-3.5" />
        </Link>
      </div>
    </div>
  );
}
