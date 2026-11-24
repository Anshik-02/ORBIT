'use client';

import React, { useState } from 'react';
import { Server, Crown, ChevronRight, HardDrive, Cpu, Radio } from 'lucide-react';

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
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 flex flex-col justify-between overflow-hidden shadow-sm">
      {/* Header */}
      <div className="flex items-center justify-between pb-3.5 mb-4 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <Server className="w-4 h-4 text-[#0891D5]" />
            <h2 className="text-base font-bold text-[#17243A] tracking-wide">
              Node Fleet Status
            </h2>
          </div>
          <p className="text-xs text-[#64748B] mt-0.5">
            5 physical cluster nodes hosting 7 shard replica instances
          </p>
        </div>
        <span className="text-[11px] font-mono px-2.5 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-emerald-700 font-semibold">
          5 / 5 ACTIVE
        </span>
      </div>

      {/* Nodes List */}
      <div className="space-y-2.5 max-h-[380px] overflow-y-auto pr-1">
        {nodes.map((node) => {
          const isExpanded = expandedNode === node.id;
          const hasLeader = node.replicas.some((r) => r.role === 'Leader');

          return (
            <div
              key={node.id}
              className={`border rounded-xl transition-all duration-200 overflow-hidden ${
                isExpanded
                  ? 'bg-[#F8FAFC] border-sky-300 shadow-sm'
                  : 'bg-white border-[#DCE5F0] hover:border-slate-300'
              }`}
            >
              {/* Row Header */}
              <button
                onClick={() => setExpandedNode(isExpanded ? null : node.id)}
                className="w-full flex items-center justify-between p-3.5 text-left transition-colors"
              >
                <div className="flex items-center gap-3">
                  <span className="w-2.5 h-2.5 rounded-full bg-[#10B981] shadow-[0_0_8px_rgba(16,185,129,0.6)] block" />
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-bold text-[#17243A] font-mono">
                        {node.name}
                      </span>
                      {hasLeader && (
                        <span className="flex items-center gap-1 text-[10px] font-mono px-2 py-0.2 rounded-full bg-amber-50 border border-amber-200 text-amber-700 font-semibold">
                          <Crown className="w-2.5 h-2.5" />
                          LEADER
                        </span>
                      )}
                    </div>
                    <span className="text-[11px] font-mono text-[#64748B]">
                      :{node.apiPort} · {node.replicas.length} Replicas
                    </span>
                  </div>
                </div>

                <div className="flex items-center gap-3 font-mono text-xs">
                  <div className="hidden sm:flex items-center gap-3 text-[#64748B] text-[11px]">
                    <span>CPU: <strong className="text-[#17243A]">{node.cpu}</strong></span>
                    <span>RAM: <strong className="text-[#17243A]">{node.ram}</strong></span>
                  </div>
                  <ChevronRight
                    className={`w-4 h-4 text-[#64748B] transition-transform duration-200 ${
                      isExpanded ? 'rotate-90 text-[#0891D5]' : ''
                    }`}
                  />
                </div>
              </button>

              {/* Expanded Details Drawer */}
              {isExpanded && (
                <div className="p-3.5 pt-0 border-t border-[#DCE5F0] bg-white font-mono text-xs text-[#17243A] space-y-2.5">
                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 pt-2.5">
                    <div className="p-2 rounded-lg bg-[#F8FAFC] border border-[#DCE5F0]">
                      <span className="text-[10px] text-[#64748B] block">Raft Port</span>
                      <span className="font-bold">{node.raftPort}</span>
                    </div>
                    <div className="p-2 rounded-lg bg-[#F8FAFC] border border-[#DCE5F0]">
                      <span className="text-[10px] text-[#64748B] block">Disk Space</span>
                      <span className="font-bold">{node.disk}</span>
                    </div>
                    <div className="p-2 rounded-lg bg-[#F8FAFC] border border-[#DCE5F0]">
                      <span className="text-[10px] text-[#64748B] block">CPU Usage</span>
                      <span className="text-[#10B981] font-bold">{node.cpu}</span>
                    </div>
                    <div className="p-2 rounded-lg bg-[#F8FAFC] border border-[#DCE5F0]">
                      <span className="text-[10px] text-[#64748B] block">Memory RSS</span>
                      <span className="text-[#0891D5] font-bold">{node.ram}</span>
                    </div>
                  </div>

                  {/* Replicas on this node */}
                  <div>
                    <span className="text-[10px] text-[#64748B] uppercase tracking-wider block mb-1 font-semibold">
                      Assigned Shards
                    </span>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                      {node.replicas.map((rep) => (
                        <div
                          key={rep.shardId}
                          className="flex items-center justify-between p-2 rounded-lg bg-[#F8FAFC] border border-[#DCE5F0]"
                        >
                          <div className="flex items-center gap-1.5">
                            <span className="w-2 h-2 rounded-full bg-[#0891D5]" />
                            <span className="font-bold text-[#17243A]">Shard {rep.shardId}</span>
                          </div>
                          <div className="flex items-center gap-2 text-[11px]">
                            <span className="text-[#64748B]">Term {rep.term}</span>
                            <span
                              className={`px-1.5 py-0.5 rounded text-[10px] font-bold ${
                                rep.role === 'Leader'
                                  ? 'bg-amber-50 text-amber-700 border border-amber-200'
                                  : 'bg-slate-100 text-[#64748B]'
                              }`}
                            >
                              {rep.role}
                            </span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>

                  {/* Last Log Entry in Sleek Developer Style */}
                  <div className="p-2.5 rounded-lg bg-[#0B1120] border border-[#1E293B] text-slate-300">
                    <span className="text-[10px] text-slate-400 block mb-0.5">Telemetry Trace</span>
                    <p className="text-[11px] text-[#38BDF8] font-mono truncate">
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
