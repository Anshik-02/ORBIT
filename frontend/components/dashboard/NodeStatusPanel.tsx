'use client';

import React, { useState } from 'react';
import { Server, Crown, ChevronRight, Activity, HardDrive, Cpu, Radio } from 'lucide-react';

interface NodeDetail {
  id: string;
  name: string;
  address: string;
  raftPort: number;
  apiPort: number;
  status: 'Online' | 'Degraded' | 'Offline';
  cpu: string;
  ram: string;
  disk: string;
  replicas: {
    shardId: number;
    role: 'Leader' | 'Follower';
    term: number;
    appliedIndex: number;
  }[];
  peers: string[];
  lastLog: string;
}

export default function NodeStatusPanel() {
  const [expandedNode, setExpandedNode] = useState<string | null>('node-a');

  const nodes: NodeDetail[] = [
    {
      id: 'node-a',
      name: 'Node A',
      address: '127.0.0.1',
      apiPort: 8000,
      raftPort: 9000,
      status: 'Online',
      cpu: '1.2%',
      ram: '48.2 MB',
      disk: '2.4 GB',
      replicas: [
        { shardId: 0, role: 'Leader', term: 4, appliedIndex: 14892 },
        { shardId: 1, role: 'Follower', term: 6, appliedIndex: 12450 },
      ],
      peers: ['Node B (9001)', 'Node C (9002)', 'Node D (9003)', 'Node E (9004)'],
      lastLog: '[RAFT] Heartbeat broadcast sent to 4 peers for Shard 0 (term 4)',
    },
    {
      id: 'node-b',
      name: 'Node B',
      address: '127.0.0.1',
      apiPort: 8001,
      raftPort: 9001,
      status: 'Online',
      cpu: '1.5%',
      ram: '52.1 MB',
      disk: '2.6 GB',
      replicas: [
        { shardId: 0, role: 'Follower', term: 4, appliedIndex: 14892 },
        { shardId: 2, role: 'Leader', term: 5, appliedIndex: 13910 },
      ],
      peers: ['Node A (9000)', 'Node C (9002)', 'Node D (9003)', 'Node E (9004)'],
      lastLog: '[ROUTER] Proxy write request key "user:942" -> Shard 0 leader',
    },
    {
      id: 'node-c',
      name: 'Node C',
      address: '127.0.0.1',
      apiPort: 8002,
      raftPort: 9002,
      status: 'Online',
      cpu: '0.9%',
      ram: '44.8 MB',
      disk: '2.1 GB',
      replicas: [
        { shardId: 0, role: 'Follower', term: 4, appliedIndex: 14892 },
        { shardId: 2, role: 'Follower', term: 5, appliedIndex: 13910 },
      ],
      peers: ['Node A (9000)', 'Node B (9001)', 'Node D (9003)', 'Node E (9004)'],
      lastLog: '[STORAGE] Memtable flushed to SSTable L0-00042 (8.4MB)',
    },
    {
      id: 'node-d',
      name: 'Node D',
      address: '127.0.0.1',
      apiPort: 8003,
      raftPort: 9003,
      status: 'Online',
      cpu: '1.4%',
      ram: '51.3 MB',
      disk: '2.5 GB',
      replicas: [
        { shardId: 1, role: 'Leader', term: 6, appliedIndex: 12450 },
        { shardId: 2, role: 'Follower', term: 5, appliedIndex: 13910 },
      ],
      peers: ['Node A (9000)', 'Node B (9001)', 'Node C (9002)', 'Node E (9004)'],
      lastLog: '[RAFT] Shard 1 AppendEntries quorum confirmed (3/3 acks)',
    },
    {
      id: 'node-e',
      name: 'Node E',
      address: '127.0.0.1',
      apiPort: 8004,
      raftPort: 9004,
      status: 'Online',
      cpu: '0.7%',
      ram: '38.4 MB',
      disk: '1.8 GB',
      replicas: [
        { shardId: 1, role: 'Follower', term: 6, appliedIndex: 12450 },
      ],
      peers: ['Node A (9000)', 'Node B (9001)', 'Node C (9002)', 'Node D (9003)'],
      lastLog: '[RAFT] Heartbeat received from Node D for Shard 1',
    },
  ];

  return (
    <div className="bg-[#0a0f1d]/80 backdrop-blur-md border border-slate-800/80 rounded-xl p-5 flex flex-col justify-between overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between pb-3 mb-3 border-b border-slate-800/80">
        <div>
          <div className="flex items-center gap-2">
            <Server className="w-4 h-4 text-sky-400" />
            <h2 className="text-base font-semibold text-white tracking-wide">
              Node Fleet Status
            </h2>
          </div>
          <p className="text-xs text-slate-400 mt-0.5">
            5 physical cluster nodes hosting 7 shard replica instances
          </p>
        </div>
        <span className="text-[11px] font-mono px-2 py-0.5 rounded-full bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
          5 / 5 ACTIVE
        </span>
      </div>

      {/* Nodes List */}
      <div className="space-y-2 max-h-[380px] overflow-y-auto pr-1">
        {nodes.map((node) => {
          const isExpanded = expandedNode === node.id;
          const hasLeader = node.replicas.some((r) => r.role === 'Leader');

          return (
            <div
              key={node.id}
              className={`border rounded-lg transition-all duration-200 overflow-hidden ${
                isExpanded
                  ? 'bg-slate-900/90 border-sky-500/40 shadow-lg'
                  : 'bg-slate-950/40 border-slate-800/80 hover:border-slate-700/80'
              }`}
            >
              {/* Row Header */}
              <button
                onClick={() => setExpandedNode(isExpanded ? null : node.id)}
                className="w-full flex items-center justify-between p-3 text-left transition-colors"
              >
                <div className="flex items-center gap-3">
                  <div className="relative">
                    <span className="w-2.5 h-2.5 rounded-full bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.8)] block" />
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-semibold text-white font-mono">
                        {node.name}
                      </span>
                      {hasLeader && (
                        <span className="flex items-center gap-1 text-[10px] font-mono px-1.5 py-0.2 rounded bg-amber-500/10 border border-amber-500/30 text-amber-400">
                          <Crown className="w-2.5 h-2.5" />
                          LEADER
                        </span>
                      )}
                    </div>
                    <span className="text-[11px] font-mono text-slate-400">
                      :{node.apiPort} · {node.replicas.length} Replicas
                    </span>
                  </div>
                </div>

                <div className="flex items-center gap-3 font-mono text-xs">
                  <div className="hidden sm:flex items-center gap-3 text-slate-400 text-[11px]">
                    <span>CPU: <strong className="text-slate-200">{node.cpu}</strong></span>
                    <span>RAM: <strong className="text-slate-200">{node.ram}</strong></span>
                  </div>
                  <ChevronRight
                    className={`w-4 h-4 text-slate-400 transition-transform duration-200 ${
                      isExpanded ? 'rotate-90 text-sky-400' : ''
                    }`}
                  />
                </div>
              </button>

              {/* Expanded Details Drawer */}
              {isExpanded && (
                <div className="p-3 pt-0 border-t border-slate-800/60 bg-slate-950/60 font-mono text-xs text-slate-300 space-y-2.5 animate-in slide-in-from-top-1 duration-150">
                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 pt-2">
                    <div className="p-2 rounded bg-slate-900/60 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">Raft Port</span>
                      <span className="text-slate-200 font-semibold">{node.raftPort}</span>
                    </div>
                    <div className="p-2 rounded bg-slate-900/60 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">Disk Space</span>
                      <span className="text-slate-200 font-semibold">{node.disk}</span>
                    </div>
                    <div className="p-2 rounded bg-slate-900/60 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">CPU Usage</span>
                      <span className="text-emerald-400 font-semibold">{node.cpu}</span>
                    </div>
                    <div className="p-2 rounded bg-slate-900/60 border border-slate-800">
                      <span className="text-[10px] text-slate-400 block">Memory RSS</span>
                      <span className="text-sky-400 font-semibold">{node.ram}</span>
                    </div>
                  </div>

                  {/* Replicas on this node */}
                  <div>
                    <span className="text-[10px] text-slate-400 uppercase tracking-wider block mb-1">
                      Assigned Shards
                    </span>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                      {node.replicas.map((rep) => (
                        <div
                          key={rep.shardId}
                          className="flex items-center justify-between p-2 rounded bg-slate-900/40 border border-slate-800"
                        >
                          <div className="flex items-center gap-1.5">
                            <span className="w-2 h-2 rounded-full bg-sky-400" />
                            <span className="font-semibold text-slate-200">Shard {rep.shardId}</span>
                          </div>
                          <div className="flex items-center gap-2 text-[11px]">
                            <span className="text-slate-400">Term {rep.term}</span>
                            <span
                              className={`px-1.5 py-0.5 rounded text-[10px] font-semibold ${
                                rep.role === 'Leader'
                                  ? 'bg-amber-500/10 text-amber-400 border border-amber-500/30'
                                  : 'bg-slate-800 text-slate-300'
                              }`}
                            >
                              {rep.role}
                            </span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>

                  {/* Last Log Entry */}
                  <div className="p-2 rounded bg-slate-950 border border-slate-800/80">
                    <span className="text-[10px] text-slate-400 block mb-0.5">Telemetry Trace</span>
                    <p className="text-[11px] text-sky-300/90 font-mono truncate">
                      {node.lastLog}
                    </p>
                  </div>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
