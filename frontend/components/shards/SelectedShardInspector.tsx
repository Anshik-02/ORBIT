'use client';

import React, { useState } from 'react';
import {
  Layers,
  Crown,
  ShieldCheck,
  CheckCircle2,
  Clock,
  HardDrive,
  RefreshCw,
  ArrowRight,
  Database,
  Hash,
} from 'lucide-react';
import { ShardDetail } from './ShardDistributionCards';

interface SelectedShardInspectorProps {
  shard: ShardDetail;
  onTriggerRebalance?: (shardId: number) => void;
}

export default function SelectedShardInspector({
  shard,
  onTriggerRebalance,
}: SelectedShardInspectorProps) {
  const [isCatchingUp, setIsCatchingUp] = useState(false);
  const [rebalanceMsg, setRebalanceMsg] = useState<string | null>(null);

  const handleSimulateCatchup = () => {
    setIsCatchingUp(true);
    setRebalanceMsg(null);
    setTimeout(() => {
      setIsCatchingUp(false);
      setRebalanceMsg('Raft logs fully synchronized across all 3 replicas. Commit Index matching.');
    }, 800);
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-6">
      {/* Top Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-4 border-b border-[#DCE5F0]">
        <div className="flex items-center gap-3">
          <div
            className="w-10 h-10 rounded-xl flex items-center justify-center text-white font-bold font-mono text-sm shadow-sm"
            style={{ backgroundColor: shard.color }}
          >
            S{shard.shardId}
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-lg font-bold text-[#17243A] font-mono leading-tight">
                {shard.name} Inspector
              </h3>
              <span className="px-2 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-[11px] font-bold">
                {shard.quorumStatus}
              </span>
            </div>
            <span className="text-xs font-mono text-[#64748B]">
              Key Hash Boundaries: {shard.hashRange}
            </span>
          </div>
        </div>

        <button
          onClick={handleSimulateCatchup}
          disabled={isCatchingUp}
          className="px-3.5 py-1.5 rounded-xl bg-[#F4F7FC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#17243A] hover:text-[#0891D5] font-mono text-xs font-semibold flex items-center gap-2 transition-all shadow-2xs self-start sm:self-auto"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${isCatchingUp ? 'animate-spin text-[#0891D5]' : ''}`} />
          <span>{isCatchingUp ? 'Syncing Logs...' : 'Verify Log Quorum'}</span>
        </button>
      </div>

      {rebalanceMsg && (
        <div className="p-3 rounded-xl bg-emerald-50 border border-emerald-200 text-emerald-800 font-mono text-xs flex items-center gap-2">
          <CheckCircle2 className="w-4 h-4 text-emerald-600 shrink-0" />
          <span>{rebalanceMsg}</span>
        </div>
      )}

      {/* Consensus & Index Metrics Grid */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 font-mono text-xs">
        <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          <span className="text-[10px] text-[#64748B] uppercase block font-semibold mb-1">
            Current Leader
          </span>
          <div className="flex items-center gap-1.5 font-bold text-[#D97706] text-sm">
            <Crown className="w-4 h-4 text-[#F59E0B]" />
            <span>{shard.leader.toUpperCase()}</span>
          </div>
        </div>

        <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          <span className="text-[10px] text-[#64748B] uppercase block font-semibold mb-1">
            Consensus Term
          </span>
          <span className="text-sm font-bold text-[#17243A]">
            #{shard.term}
          </span>
        </div>

        <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          <span className="text-[10px] text-[#64748B] uppercase block font-semibold mb-1">
            Raft Commit Index
          </span>
          <span className="text-sm font-bold text-[#0891D5]">
            {shard.commitIndex.toLocaleString()}
          </span>
        </div>

        <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          <span className="text-[10px] text-[#64748B] uppercase block font-semibold mb-1">
            LSM Applied Index
          </span>
          <span className="text-sm font-bold text-[#10B981]">
            {shard.appliedIndex.toLocaleString()}
          </span>
        </div>
      </div>

      {/* Detailed Replica Health Table */}
      <div>
        <h4 className="text-xs font-mono font-bold text-[#64748B] uppercase tracking-wider mb-3">
          REPLICA SYNCHRONIZATION STATUS
        </h4>
        <div className="overflow-x-auto border border-[#E2E8F0] rounded-xl">
          <table className="w-full text-left border-collapse font-mono text-xs">
            <thead>
              <tr className="bg-[#F8FAFC] border-b border-[#E2E8F0] text-[#64748B] text-[11px]">
                <th className="py-2.5 px-3">Replica Node</th>
                <th className="py-2.5 px-3">Raft Role</th>
                <th className="py-2.5 px-3">Sync State</th>
                <th className="py-2.5 px-3">Log Lag</th>
                <th className="py-2.5 px-3 text-right">Heartbeat Latency</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#F1F5F9]">
              {shard.replicas.map((rep) => {
                const isLeader = rep.toLowerCase() === shard.leader.toLowerCase();
                return (
                  <tr key={rep} className="hover:bg-[#F8FAFC]">
                    <td className="py-2.5 px-3 font-bold text-[#17243A] flex items-center gap-1.5">
                      <span className={`w-2 h-2 rounded-full ${isLeader ? 'bg-[#F59E0B]' : 'bg-[#10B981]'}`} />
                      <span>{rep.toUpperCase()}</span>
                    </td>
                    <td className="py-2.5 px-3">
                      {isLeader ? (
                        <span className="px-2 py-0.5 rounded-full bg-amber-50 text-[#D97706] border border-amber-200 font-bold text-[10px]">
                          LEADER
                        </span>
                      ) : (
                        <span className="px-2 py-0.5 rounded-full bg-slate-100 text-[#64748B] font-semibold text-[10px]">
                          FOLLOWER
                        </span>
                      )}
                    </td>
                    <td className="py-2.5 px-3 text-[#10B981] font-semibold">
                      Synchronized (MatchIndex = {shard.commitIndex})
                    </td>
                    <td className="py-2.5 px-3 text-[#64748B]">
                      0 entries (in sync)
                    </td>
                    <td className="py-2.5 px-3 text-right text-[#17243A]">
                      {isLeader ? '0.0ms (local)' : '0.7ms'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
