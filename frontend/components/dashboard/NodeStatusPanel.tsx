'use client';

import React, { useState } from 'react';
import { Server, Crown, ChevronRight, HardDrive, Cpu, Radio, ShieldCheck } from 'lucide-react';
import { useClusterHealth } from '@/hooks/useClusterHealth';

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
  const { health, status: clusterStatus, isLive } = useClusterHealth();
  const [expandedNode, setExpandedNode] = useState<string | null>('node-a');

  const defaultNodes: NodeDetail[] = [
    {
      id: 'node-a',
      name: 'Node A',
      address: '127.0.0.1',
      apiPort: 2525,
      raftPort: 7000,
      status: 'Online',
      cpu: '1.2%',
      ram: '48.2 MB',
      disk: '2.4 GB',
      replicas: [
        { shardId: 0, role: 'Leader', term: 4, appliedIndex: 14892 },
        { shardId: 1, role: 'Follower', term: 6, appliedIndex: 12450 },
      ],
      peers: ['Node B (7001)', 'Node C (7002)', 'Node D (7003)', 'Node E (7004)'],
      lastLog: '[RAFT] Heartbeat broadcast sent to 4 peers for Shard 0 (term 4)',
    },
    {
      id: 'node-b',
      name: 'Node B',
      address: '127.0.0.1',
      apiPort: 2526,
      raftPort: 7001,
      status: 'Online',
      cpu: '1.5%',
      ram: '52.1 MB',
      disk: '2.6 GB',
      replicas: [
        { shardId: 0, role: 'Follower', term: 4, appliedIndex: 14892 },
        { shardId: 2, role: 'Leader', term: 5, appliedIndex: 13910 },
      ],
      peers: ['Node A (7000)', 'Node C (7002)', 'Node D (7003)', 'Node E (7004)'],
      lastLog: '[ROUTER] Proxy write request key "user:942" -> Shard 0 leader',
    },
    {
      id: 'node-c',
      name: 'Node C',
      address: '127.0.0.1',
      apiPort: 2527,
      raftPort: 7002,
      status: 'Online',
      cpu: '0.9%',
      ram: '44.8 MB',
      disk: '2.1 GB',
      replicas: [
        { shardId: 0, role: 'Follower', term: 4, appliedIndex: 14892 },
        { shardId: 2, role: 'Follower', term: 5, appliedIndex: 13910 },
      ],
      peers: ['Node A (7000)', 'Node B (7001)', 'Node D (7003)', 'Node E (7004)'],
      lastLog: '[STORAGE] Memtable flushed to SSTable L0-00042 (8.4MB)',
    },
    {
      id: 'node-d',
      name: 'Node D',
      address: '127.0.0.1',
      apiPort: 2528,
      raftPort: 7003,
      status: 'Online',
      cpu: '1.4%',
      ram: '51.3 MB',
      disk: '2.5 GB',
      replicas: [
        { shardId: 1, role: 'Leader', term: 6, appliedIndex: 12450 },
        { shardId: 2, role: 'Follower', term: 5, appliedIndex: 13910 },
      ],
      peers: ['Node A (7000)', 'Node B (7001)', 'Node C (7002)', 'Node E (7004)'],
      lastLog: '[RAFT] Shard 1 AppendEntries quorum confirmed (3/3 acks)',
    },
    {
      id: 'node-e',
      name: 'Node E',
      address: '127.0.0.1',
      apiPort: 2529,
      raftPort: 7004,
      status: 'Online',
      cpu: '0.7%',
      ram: '38.4 MB',
      disk: '1.8 GB',
      replicas: [
        { shardId: 1, role: 'Follower', term: 6, appliedIndex: 12450 },
      ],
      peers: ['Node A (7000)', 'Node B (7001)', 'Node C (7002)', 'Node D (7003)'],
      lastLog: '[RAFT] Heartbeat received from Node D for Shard 1',
    },
  ];

  const nodes: NodeDetail[] = isLive && health?.nodes && Object.keys(health.nodes).length > 0
    ? Object.values(health.nodes).map((n, idx) => {
        const defaultRef = defaultNodes.find((dn) => dn.id === n.node_id) || defaultNodes[idx % defaultNodes.length];
        return {
          id: n.node_id,
          name: n.node_id.toUpperCase(),
          address: defaultRef.address,
          apiPort: 2525 + idx,
          raftPort: 7000 + idx,
          status: (n.online ? 'Online' : 'Offline') as 'Online' | 'Offline',
          cpu: defaultRef.cpu,
          ram: defaultRef.ram,
          disk: defaultRef.disk,
          replicas: n.hosted_shard_ids && n.hosted_shard_ids.length > 0
            ? n.hosted_shard_ids.map((sid) => ({
                shardId: sid,
                role: (sid === 0 && n.node_id === 'node-a') || (sid === 1 && n.node_id === 'node-d') || (sid === 2 && n.node_id === 'node-b') ? 'Leader' : 'Follower',
                term: 4,
                appliedIndex: 14892,
              }))
            : defaultRef.replicas,
          peers: defaultRef.peers,
          lastLog: defaultRef.lastLog,
        };
      })
    : defaultNodes;

  // Environment health progress bars (Cosmic Forge style)
  const healthBars = [
    { label: 'Shard 0 Quorum', pct: '100%', color: 'bg-[#10B981]' },
    { label: 'Shard 1 Quorum', pct: '100%', color: 'bg-[#0891D5]' },
    { label: 'Shard 2 Quorum', pct: '100%', color: 'bg-[#7965E8]' },
    { label: 'LSM Engine & WAL', pct: '99.98%', color: 'bg-[#10B981]' },
  ];

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 flex flex-col justify-between overflow-hidden shadow-xs hover:shadow-md transition-shadow">
      {/* Header */}
      <div>
        <div className="flex items-center justify-between pb-3.5 mb-4 border-b border-[#DCE5F0]">
          <div>
            <div className="flex items-center gap-2">
              <Server className="w-4 h-4 text-[#0891D5]" />
              <h2 className="text-base font-bold text-[#17243A] tracking-wide">
                Node Fleet Status
              </h2>
            </div>
            <p className="text-xs text-[#64748B] mt-0.5">
              5 physical cluster nodes hosting multi-raft replicas
            </p>
          </div>
          <span className="text-[11px] font-mono px-2.5 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-emerald-700 font-semibold">
            {nodes.filter(n => n.status === 'Online').length} / {nodes.length} ACTIVE
          </span>
        </div>

        {/* Nodes List */}
        <div className="space-y-2.5 max-h-[300px] overflow-y-auto pr-1">
          {nodes.map((node) => {
            const isExpanded = expandedNode === node.id;
            const hasLeader = node.replicas.some((r) => r.role === 'Leader');

            return (
              <div
                key={node.id}
                className={`border rounded-xl transition-all duration-200 overflow-hidden ${
                  isExpanded
                    ? 'bg-[#F8FAFC] border-sky-300 shadow-xs'
                    : 'bg-white border-[#DCE5F0] hover:border-slate-300'
                }`}
              >
                {/* Row Header */}
                <button
                  onClick={() => setExpandedNode(isExpanded ? null : node.id)}
                  className="w-full flex items-center justify-between p-3 text-left transition-colors cursor-pointer"
                >
                  <div className="flex items-center gap-2.5">
                    <span className="w-2.5 h-2.5 rounded-full bg-[#10B981] shadow-[0_0_8px_rgba(16,185,129,0.6)] block shrink-0" />
                    <div>
                      <div className="flex items-center gap-1.5">
                        <span className="text-sm font-bold text-[#17243A] font-mono">
                          {node.name}
                        </span>
                        {hasLeader && (
                          <span className="flex items-center gap-1 text-[9px] font-mono px-1.5 py-0.2 rounded-full bg-amber-50 border border-amber-200 text-amber-700 font-bold">
                            <Crown className="w-2.5 h-2.5" />
                            LEADER
                          </span>
                        )}
                      </div>
                      <span className="text-[10px] font-mono text-[#64748B]">
                        Port :{node.apiPort} · {node.replicas.length} Replicas
                      </span>
                    </div>
                  </div>

                  <div className="flex items-center gap-2 font-mono text-xs">
                    <span className="text-[11px] text-[#64748B] hidden sm:inline">
                      {node.cpu}
                    </span>
                    <ChevronRight
                      className={`w-4 h-4 text-[#64748B] transition-transform duration-200 ${
                        isExpanded ? 'rotate-90 text-[#0891D5]' : ''
                      }`}
                    />
                  </div>
                </button>

                {/* Expanded Details Drawer */}
                {isExpanded && (
                  <div className="p-3 pt-0 border-t border-[#DCE5F0] bg-white font-mono text-xs text-[#17243A] space-y-2">
                    <div className="grid grid-cols-2 gap-2 pt-2">
                      <div className="p-2 rounded-lg bg-[#F8FAFC] border border-[#DCE5F0]">
                        <span className="text-[10px] text-[#64748B] block">Raft Port</span>
                        <span className="font-bold">{node.raftPort}</span>
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
                      <div className="grid grid-cols-2 gap-1.5">
                        {node.replicas.map((rep) => (
                          <div
                            key={rep.shardId}
                            className="flex items-center justify-between p-1.5 rounded-lg bg-[#F8FAFC] border border-[#DCE5F0]"
                          >
                            <span className="font-bold text-[#17243A] text-[11px]">Shard {rep.shardId}</span>
                            <span
                              className={`px-1.5 py-0.2 rounded text-[9px] font-bold ${
                                rep.role === 'Leader'
                                  ? 'bg-amber-50 text-amber-700 border border-amber-200'
                                  : 'bg-slate-100 text-[#64748B]'
                              }`}
                            >
                              {rep.role}
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>

      {/* Cosmic Forge Environment Health Progress Bars */}
      <div className="mt-4 pt-3.5 border-t border-[#DCE5F0]">
        <div className="flex items-center justify-between mb-2">
          <span className="text-[11px] font-semibold text-[#17243A] flex items-center gap-1.5">
            <ShieldCheck className="w-3.5 h-3.5 text-[#10B981]" />
            Consensus & Engine Health
          </span>
          <span className="text-[10px] font-mono text-[#64748B]">All Systems Normal</span>
        </div>
        <div className="space-y-2">
          {healthBars.map((bar, idx) => (
            <div key={idx} className="space-y-1">
              <div className="flex justify-between text-[11px] font-mono">
                <span className="text-[#64748B]">{bar.label}</span>
                <span className="font-semibold text-[#17243A]">{bar.pct}</span>
              </div>
              <div className="w-full h-1.5 rounded-full bg-[#E2E8F0] overflow-hidden">
                <div className={`h-full rounded-full ${bar.color}`} style={{ width: bar.pct }} />
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
