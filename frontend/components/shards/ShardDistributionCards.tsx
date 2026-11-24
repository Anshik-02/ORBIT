'use client';

import React from 'react';
import { Crown, Layers, ShieldCheck, ArrowRight, Activity, Database } from 'lucide-react';

export interface ShardDetail {
  shardId: number;
  name: string;
  color: string;
  hashRange: string;
  leader: string;
  replicas: string[];
  term: number;
  commitIndex: number;
  appliedIndex: number;
  quorumStatus: string;
  keyPercentage: string;
}

interface ShardDistributionCardsProps {
  shards: ShardDetail[];
  selectedShardId: number;
  onSelectShard: (shardId: number) => void;
}

export default function ShardDistributionCards({
  shards,
  selectedShardId,
  onSelectShard,
}: ShardDistributionCardsProps) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
      {shards.map((sh) => {
        const isSelected = selectedShardId === sh.shardId;

        return (
          <div
            key={sh.shardId}
            onClick={() => onSelectShard(sh.shardId)}
            className={`cursor-pointer rounded-2xl p-6 transition-all border flex flex-col justify-between ${
              isSelected
                ? 'bg-white border-[#0891D5] shadow-md ring-2 ring-[#0891D5]/20'
                : 'bg-white border-[#DCE5F0] hover:border-[#CBD5E1] shadow-xs hover:shadow-md'
            }`}
          >
            <div>
              {/* Card Top Title & Badge */}
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-2.5">
                  <span
                    className="w-3.5 h-3.5 rounded-full ring-4 ring-offset-2"
                    style={{
                      backgroundColor: sh.color,
                      boxShadow: `0 0 12px ${sh.color}60`,
                    }}
                  />
                  <div>
                    <h3 className="text-base font-bold text-[#17243A] font-mono leading-tight">
                      {sh.name}
                    </h3>
                    <span className="text-[11px] font-mono text-[#64748B]">
                      {sh.hashRange}
                    </span>
                  </div>
                </div>

                <span className="px-2.5 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-[10px] font-bold">
                  {sh.quorumStatus}
                </span>
              </div>

              {/* Primary Leader Panel */}
              <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#F59E0B]/30 mb-4 flex items-center justify-between font-mono">
                <div>
                  <span className="text-[10px] text-[#64748B] uppercase block font-semibold tracking-wider">
                    RAFT LEADER
                  </span>
                  <div className="flex items-center gap-1.5 mt-0.5">
                    <Crown className="w-4 h-4 text-[#F59E0B]" />
                    <span className="text-sm font-bold text-[#D97706]">
                      {sh.leader.toUpperCase()}
                    </span>
                  </div>
                </div>
                <div className="text-right">
                  <span className="text-[10px] text-[#64748B] uppercase block font-semibold tracking-wider">
                    CONSENSUS TERM
                  </span>
                  <span className="text-sm font-bold text-[#17243A]">
                    #{sh.term}
                  </span>
                </div>
              </div>

              {/* Replicas List */}
              <div className="space-y-2 font-mono text-xs mb-4">
                <div className="flex items-center justify-between">
                  <span className="text-[10px] uppercase text-[#64748B] font-semibold tracking-wider">
                    PARTICIPATING REPLICAS (RF=3)
                  </span>
                  <span className="text-[10px] text-[#0891D5] font-semibold">
                    Quorum Min: 2
                  </span>
                </div>

                {sh.replicas.map((nodeName) => {
                  const isLeader = nodeName.toLowerCase() === sh.leader.toLowerCase();
                  return (
                    <div
                      key={nodeName}
                      className="flex items-center justify-between p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]"
                    >
                      <div className="flex items-center gap-2">
                        <span
                          className={`w-2 h-2 rounded-full ${
                            isLeader ? 'bg-[#F59E0B]' : 'bg-[#10B981]'
                          }`}
                        />
                        <span className="font-semibold text-[#17243A]">
                          {nodeName.toUpperCase()}
                        </span>
                      </div>
                      <span
                        className={`text-[10px] px-2 py-0.5 rounded-full font-semibold ${
                          isLeader
                            ? 'bg-amber-50 text-[#D97706] border border-amber-200'
                            : 'bg-slate-100 text-[#64748B]'
                        }`}
                      >
                        {isLeader ? 'Leader' : 'Follower'}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Bottom Progress & Click to Inspect */}
            <div className="pt-3 border-t border-[#E2E8F0] space-y-2 font-mono text-xs">
              <div className="flex items-center justify-between text-[11px] text-[#64748B]">
                <span>Commit Index:</span>
                <span className="text-[#10B981] font-bold">
                  {sh.commitIndex.toLocaleString()}
                </span>
              </div>

              <div className="flex items-center justify-between text-[11px] text-[#64748B]">
                <span>Hash Ring Share:</span>
                <span className="text-[#17243A] font-semibold">
                  {sh.keyPercentage}
                </span>
              </div>

              <div className="pt-2 flex items-center justify-end">
                <span className="text-[11px] font-semibold text-[#0891D5] flex items-center gap-1 group-hover:underline">
                  <span>{isSelected ? 'INSPECTING' : 'CLICK TO INSPECT'}</span>
                  <ArrowRight className="w-3 h-3" />
                </span>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}
