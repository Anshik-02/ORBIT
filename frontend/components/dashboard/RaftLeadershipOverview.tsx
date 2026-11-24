'use client';

import React from 'react';
import { ShieldCheck, Crown, CheckCircle2, ArrowRight, Layers, Users } from 'lucide-react';

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
      color: '#38bdf8',
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
      color: '#818cf8',
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
      color: '#c084fc',
      replicas: [
        { nodeId: 'node-b', nodeName: 'Node B', role: 'Leader', matchIndex: 13910, syncPercent: 100, isLeader: true },
        { nodeId: 'node-c', nodeName: 'Node C', role: 'Follower', matchIndex: 13910, syncPercent: 100, isLeader: false },
        { nodeId: 'node-d', nodeName: 'Node D', role: 'Follower', matchIndex: 13910, syncPercent: 100, isLeader: false },
      ],
    },
  ];

  return (
    <div className="bg-[#0a0f1d]/80 backdrop-blur-md border border-slate-800/80 rounded-xl p-5 overflow-hidden">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-4 mb-4 border-b border-slate-800/80">
        <div>
          <div className="flex items-center gap-2">
            <ShieldCheck className="w-4 h-4 text-emerald-400" />
            <h2 className="text-base font-semibold text-white tracking-wide">
              Raft Consensus & Leadership Overview
            </h2>
          </div>
          <p className="text-xs text-slate-400 mt-0.5">
            Distributed log state machines, term stability & commit synchronization per shard
          </p>
        </div>
        <div className="flex items-center gap-2 self-start sm:self-auto font-mono text-xs">
          <span className="flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 font-semibold">
            <CheckCircle2 className="w-3.5 h-3.5" />
            ALL LEADERS ELECTED
          </span>
        </div>
      </div>

      {/* Shard Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {shards.map((shard) => (
          <div
            key={shard.shardId}
            className="group relative bg-slate-950/60 border border-slate-800 hover:border-slate-700 rounded-xl p-4 transition-all duration-200 flex flex-col justify-between"
          >
            {/* Top Bar */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2">
                  <span
                    className="w-2.5 h-2.5 rounded-full"
                    style={{ backgroundColor: shard.color }}
                  />
                  <span className="text-sm font-bold text-white font-mono">
                    {shard.name}
                  </span>
                </div>
                <span className="text-[10px] font-mono font-semibold px-2 py-0.5 rounded-full bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
                  {shard.status}
                </span>
              </div>

              {/* Hash boundary */}
              <div className="text-[10px] font-mono text-slate-400 truncate mb-3 bg-slate-900/60 px-2 py-1 rounded border border-slate-800/80">
                Range: {shard.keyRange}
              </div>

              {/* Leader Highlight Box */}
              <div className="p-3 rounded-lg bg-slate-900/80 border border-amber-500/30 mb-3 flex items-center justify-between">
                <div>
                  <span className="text-[10px] uppercase font-mono text-slate-400 block">
                    Current Leader
                  </span>
                  <div className="flex items-center gap-1.5 mt-0.5">
                    <Crown className="w-3.5 h-3.5 text-amber-400" />
                    <span className="text-sm font-bold text-amber-300 font-mono">
                      {shard.leaderName}
                    </span>
                  </div>
                </div>
                <div className="text-right font-mono">
                  <span className="text-[10px] uppercase text-slate-400 block">Term</span>
                  <span className="text-sm font-bold text-white">#{shard.term}</span>
                </div>
              </div>

              {/* Log State Indices */}
              <div className="grid grid-cols-2 gap-2 mb-3 text-xs font-mono">
                <div className="p-2 rounded bg-slate-900/50 border border-slate-800">
                  <span className="text-[10px] text-slate-400 block">Commit Index</span>
                  <span className="text-slate-200 font-bold">{shard.commitIndex.toLocaleString()}</span>
                </div>
                <div className="p-2 rounded bg-slate-900/50 border border-slate-800">
                  <span className="text-[10px] text-slate-400 block">Applied Index</span>
                  <span className="text-emerald-400 font-bold">{shard.appliedIndex.toLocaleString()}</span>
                </div>
              </div>

              {/* Replicas & Sync Progress */}
              <div className="space-y-1.5 font-mono text-xs">
                <span className="text-[10px] uppercase text-slate-400 tracking-wider block">
                  Replica Quorum (RF=3)
                </span>
                {shard.replicas.map((rep) => (
                  <div
                    key={rep.nodeId}
                    className="flex items-center justify-between p-2 rounded bg-slate-900/40 border border-slate-800/80"
                  >
                    <div className="flex items-center gap-2">
                      <span
                        className={`w-1.5 h-1.5 rounded-full ${
                          rep.isLeader ? 'bg-amber-400' : 'bg-slate-400'
                        }`}
                      />
                      <span className="font-semibold text-slate-200 text-xs">
                        {rep.nodeName}
                      </span>
                    </div>

                    <div className="flex items-center gap-2">
                      <span className="text-[11px] text-slate-400">
                        idx {rep.matchIndex.toLocaleString()}
                      </span>
                      <span
                        className={`text-[9px] px-1.5 py-0.2 rounded font-semibold ${
                          rep.isLeader
                            ? 'bg-amber-500/10 text-amber-400 border border-amber-500/30'
                            : 'bg-slate-800 text-slate-400'
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
            <div className="mt-3 pt-2 border-t border-slate-800/80 flex items-center justify-between text-[11px] font-mono text-slate-400">
              <span>Replication Lag:</span>
              <span className="text-emerald-400 font-semibold">{shard.lag}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
