'use client';

import React from 'react';
import { Crown, Layers, ShieldCheck, Zap } from 'lucide-react';

export interface RaftGroupInfo {
  shardId: number;
  name: string;
  color: string;
  leader: string;
  term: number;
  commitIndex: number;
  replicas: string[];
}

interface RaftGroupSelectorProps {
  groups: RaftGroupInfo[];
  selectedShardId: number;
  onSelectGroup: (shardId: number) => void;
}

export default function RaftGroupSelector({
  groups,
  selectedShardId,
  onSelectGroup,
}: RaftGroupSelectorProps) {
  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-4 shadow-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-3">
        <div>
          <span className="text-[10px] font-mono uppercase text-[#64748B] font-semibold tracking-wider block">
            MULTI-RAFT CONSENSUS GROUPS
          </span>
          <h3 className="text-base font-bold text-[#17243A] font-mono">
            Select Consensus Partition
          </h3>
        </div>
        <span className="text-xs font-mono text-[#64748B]">
          Each partition operates an independent Raft consensus state machine
        </span>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        {groups.map((grp) => {
          const isSelected = selectedShardId === grp.shardId;

          return (
            <button
              key={grp.shardId}
              onClick={() => onSelectGroup(grp.shardId)}
              className={`p-3.5 rounded-xl border text-left font-mono transition-all flex items-center justify-between ${
                isSelected
                  ? 'bg-blue-50/50 border-[#0891D5] shadow-xs ring-2 ring-[#0891D5]/20'
                  : 'bg-[#F8FAFC] border-[#E2E8F0] hover:border-[#CBD5E1] hover:bg-white'
              }`}
            >
              <div className="flex items-center gap-3">
                <span
                  className="w-3.5 h-3.5 rounded-full ring-2 ring-offset-2 shrink-0"
                  style={{
                    backgroundColor: grp.color,
                    boxShadow: isSelected ? `0 0 10px ${grp.color}80` : undefined,
                  }}
                />
                <div>
                  <div className="flex items-center gap-1.5">
                    <span className="text-sm font-bold text-[#17243A]">
                      {grp.name}
                    </span>
                    {isSelected && (
                      <span className="px-1.5 py-0.2 rounded bg-[#0891D5] text-white text-[9px] font-bold">
                        ACTIVE
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-1 text-[11px] text-[#64748B] mt-0.5">
                    <Crown className="w-3 h-3 text-[#F59E0B]" />
                    <span>Leader: {grp.leader.toUpperCase()}</span>
                  </div>
                </div>
              </div>

              <div className="text-right text-xs">
                <span className="text-[10px] text-[#64748B] block">Term</span>
                <span className="font-bold text-[#17243A]">#{grp.term}</span>
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}
