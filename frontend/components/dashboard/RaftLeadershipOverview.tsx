'use client';

import React from 'react';
import { ShieldCheck, Crown, CheckCircle2 } from 'lucide-react';

interface ShardReplica {
  nodeId: string;
  nodeName: string;
  role: 'Leader' | 'Follower';
  matchIndex: number;
  syncPercent: number;
  isLeader: boolean;
}

interface ShardOverview {
  shardId: number;
  name: string;
  keyRange: string;
  leaderName: string;
  leaderNodeId: string;
  term: number;
  commitIndex: number;
  appliedIndex: number;
  lag: string;
  status: 'STABLE' | 'RE-ELECTING' | 'DEGRADED';
  replicas: ShardReplica[];
  color: string;
}

export default function RaftLeadershipOverview() {
  const shards: ShardOverview[] = [
    {
      shardId: 0,
      name: 'Shard 0',
      keyRange: '0x00000000 — 0x55555554',
      leaderName: 'Node A',
      leaderNodeId: 'node-a',
      term: 4,
      commitIndex: 14892,
      appliedIndex: 14892,
      lag: '0 entries (In sync)',
      status: 'STABLE',
      color: '#0891D5',
      replicas: [
        { nodeId: 'node-a', nodeName: 'Node A', role: 'Leader', matchIndex: 14892, syncPercent: 100, isLeader: true },
        { nodeId: 'node-b', nodeName: 'Node B', role: 'Follower', matchIndex: 14892, syncPercent: 100, isLeader: false },
        { nodeId: 'node-c', nodeName: 'Node C', role: 'Follower', matchIndex: 14892, syncPercent: 100, isLeader: false },
      ],
    },
    {
      shardId: 1,
      name: 'Shard 1',
      keyRange: '0x55555555 — 0xAAAAAAA9',
      leaderName: 'Node D',
      leaderNodeId: 'node-d',
      term: 6,
      commitIndex: 12450,
      appliedIndex: 12450,
      lag: '0 entries (In sync)',
      status: 'STABLE',
      color: '#7965E8',
      replicas: [
        { nodeId: 'node-d', nodeName: 'Node D', role: 'Leader', matchIndex: 12450, syncPercent: 100, isLeader: true },
        { nodeId: 'node-e', nodeName: 'Node E', role: 'Follower', matchIndex: 12450, syncPercent: 100, isLeader: false },
        { nodeId: 'node-a', nodeName: 'Node A', role: 'Follower', matchIndex: 12450, syncPercent: 100, isLeader: false },
      ],
    },
    {
      shardId: 2,
      name: 'Shard 2',
      keyRange: '0xAAAAAAAA — 0xFFFFFFFF',
      leaderName: 'Node B',
      leaderNodeId: 'node-b',
      term: 5,
      commitIndex: 13910,
      appliedIndex: 13910,
      lag: '0 entries (In sync)',
      status: 'STABLE',
      color: '#10B981',
      replicas: [
        { nodeId: 'node-b', nodeName: 'Node B', role: 'Leader', matchIndex: 13910, syncPercent: 100, isLeader: true },
        { nodeId: 'node-c', nodeName: 'Node C', role: 'Follower', matchIndex: 13910, syncPercent: 100, isLeader: false },
        { nodeId: 'node-d', nodeName: 'Node D', role: 'Follower', matchIndex: 13910, syncPercent: 100, isLeader: false },
      ],
    },
  ];

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs hover:shadow-md transition-shadow overflow-hidden">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-4 mb-6 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-lg bg-[#10B981]/10 flex items-center justify-center">
              <ShieldCheck className="w-4 h-4 text-[#10B981]" />
            </div>
            <h2 className="text-base font-semibold text-[#17243A] tracking-tight">
              Raft Consensus & Leadership Overview
            </h2>
          </div>
          <p className="text-xs text-[#64748B] mt-1">
            Distributed log state machines, term stability & commit synchronization per shard
          </p>
        </div>
        <div className="flex items-center gap-2 self-start sm:self-auto font-mono text-xs">
          <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-[#10B981]/10 border border-[#10B981]/30 text-[#10B981] font-semibold">
            <CheckCircle2 className="w-3.5 h-3.5" />
            ALL LEADERS ELECTED
          </span>
        </div>
      </div>

      {/* Shard Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
        {shards.map((shard) => (
          <div
            key={shard.shardId}
            className="group relative bg-[#F8FAFC] border border-[#DCE5F0] hover:border-[#CBD5E1] rounded-xl p-5 transition-all duration-200 flex flex-col justify-between shadow-2xs"
          >
            {/* Top Bar */}
            <div>
              <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-2">
                  <span
                    className="w-3 h-3 rounded-full shadow-2xs"
                    style={{ backgroundColor: shard.color }}
                  />
                  <span className="text-sm font-bold text-[#17243A] font-mono">
                    {shard.name}
                  </span>
                </div>
                <span className="text-[10px] font-mono font-semibold px-2 py-0.5 rounded-full bg-[#10B981]/10 border border-[#10B981]/30 text-[#10B981]">
                  {shard.status}
                </span>
              </div>

              {/* Hash boundary */}
              <div className="text-[11px] font-mono text-[#64748B] truncate mb-3 bg-white px-3 py-1.5 rounded-lg border border-[#E2E8F0]">
                Range: {shard.keyRange}
              </div>

              {/* Leader Highlight Box */}
              <div className="p-3.5 rounded-xl bg-white border border-[#F59E0B]/30 shadow-2xs mb-3 flex items-center justify-between">
                <div>
                  <span className="text-[10px] uppercase font-mono text-[#64748B] font-semibold block tracking-wider">
                    Current Leader
                  </span>
                  <div className="flex items-center gap-1.5 mt-0.5">
                    <Crown className="w-4 h-4 text-[#F59E0B]" />
                    <span className="text-sm font-bold text-[#D97706] font-mono">
                      {shard.leaderName}
                    </span>
                  </div>
                </div>
                <div className="text-right font-mono">
                  <span className="text-[10px] uppercase text-[#64748B] font-semibold block">Term</span>
                  <span className="text-sm font-bold text-[#17243A]">#{shard.term}</span>
                </div>
              </div>

              {/* Log State Indices */}
              <div className="grid grid-cols-2 gap-2 mb-3 text-xs font-mono">
                <div className="p-2.5 rounded-lg bg-white border border-[#E2E8F0]">
                  <span className="text-[10px] text-[#64748B] font-semibold block">Commit Index</span>
                  <span className="text-[#17243A] font-bold">{shard.commitIndex.toLocaleString()}</span>
                </div>
                <div className="p-2.5 rounded-lg bg-white border border-[#E2E8F0]">
                  <span className="text-[10px] text-[#64748B] font-semibold block">Applied Index</span>
                  <span className="text-[#10B981] font-bold">{shard.appliedIndex.toLocaleString()}</span>
                </div>
              </div>

              {/* Replicas & Sync Progress */}
              <div className="space-y-1.5 font-mono text-xs">
                <span className="text-[10px] uppercase text-[#64748B] font-semibold tracking-wider block">
                  Replica Quorum (RF=3)
                </span>
                {shard.replicas.map((rep) => (
                  <div
                    key={rep.nodeId}
                    className="flex items-center justify-between p-2 rounded-lg bg-white border border-[#E2E8F0]"
                  >
                    <div className="flex items-center gap-2">
                      <span
                        className={`w-2 h-2 rounded-full ${
                          rep.isLeader ? 'bg-[#F59E0B]' : 'bg-[#94A3B8]'
                        }`}
                      />
                      <span className="font-semibold text-[#17243A] text-xs">
                        {rep.nodeName}
                      </span>
                    </div>

                    <div className="flex items-center gap-2">
                      <span className="text-[11px] text-[#64748B]">
                        idx {rep.matchIndex.toLocaleString()}
                      </span>
                      <span
                        className={`text-[9px] px-2 py-0.5 rounded-full font-semibold ${
                          rep.isLeader
                            ? 'bg-[#F59E0B]/10 text-[#D97706] border border-[#F59E0B]/30'
                            : 'bg-[#F1F5F9] text-[#64748B] border border-[#E2E8F0]'
                        }`}
                      >
                        {rep.role}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            </div>

            {/* Bottom sync status */}
            <div className="mt-4 pt-3 border-t border-[#E2E8F0] flex items-center justify-between text-[11px] font-mono text-[#64748B]">
              <span>Replication Lag:</span>
              <span className="text-[#10B981] font-semibold">{shard.lag}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
