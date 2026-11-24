'use client';

import React, { useState } from 'react';
import { Crown, Server, Layers, Zap, Info, ShieldCheck } from 'lucide-react';

interface NodeData {
  id: string;
  name: string;
  port: number;
  ip: string;
  x: number;
  y: number;
  shards: {
    shardId: number;
    role: 'Leader' | 'Follower';
  }[];
  cpu: string;
  ram: string;
  status: 'online' | 'degraded' | 'offline';
  uptime: string;
}

export default function ClusterTopologyPreview() {
  const [selectedShard, setSelectedShard] = useState<number | 'all'>('all');
  const [hoveredNode, setHoveredNode] = useState<NodeData | null>(null);

  // 5 physical nodes arranged geometrically in a pleasant circular layout
  const nodes: NodeData[] = [
    {
      id: 'node-a',
      name: 'Node A',
      port: 8000,
      ip: '127.0.0.1:8000',
      x: 300,
      y: 90,
      shards: [
        { shardId: 0, role: 'Leader' },
        { shardId: 1, role: 'Follower' },
      ],
      cpu: '1.2%',
      ram: '48.2 MB',
      status: 'online',
      uptime: '4d 18h',
    },
    {
      id: 'node-b',
      name: 'Node B',
      port: 8001,
      ip: '127.0.0.1:8001',
      x: 480,
      y: 200,
      shards: [
        { shardId: 0, role: 'Follower' },
        { shardId: 2, role: 'Leader' },
      ],
      cpu: '1.5%',
      ram: '52.1 MB',
      status: 'online',
      uptime: '4d 18h',
    },
    {
      id: 'node-c',
      name: 'Node C',
      port: 8002,
      ip: '127.0.0.1:8002',
      x: 410,
      y: 380,
      shards: [
        { shardId: 0, role: 'Follower' },
        { shardId: 2, role: 'Follower' },
      ],
      cpu: '0.9%',
      ram: '44.8 MB',
      status: 'online',
      uptime: '4d 18h',
    },
    {
      id: 'node-d',
      name: 'Node D',
      port: 8003,
      ip: '127.0.0.1:8003',
      x: 190,
      y: 380,
      shards: [
        { shardId: 1, role: 'Leader' },
        { shardId: 2, role: 'Follower' },
      ],
      cpu: '1.4%',
      ram: '51.3 MB',
      status: 'online',
      uptime: '4d 18h',
    },
    {
      id: 'node-e',
      name: 'Node E',
      port: 8004,
      ip: '127.0.0.1:8004',
      x: 120,
      y: 200,
      shards: [
        { shardId: 1, role: 'Follower' },
      ],
      cpu: '0.7%',
      ram: '38.4 MB',
      status: 'online',
      uptime: '4d 18h',
    },
  ];

  // Shard replica sets & colors
  const shardConfigs = [
    {
      id: 0,
      name: 'Shard 0',
      leader: 'Node A',
      nodes: ['node-a', 'node-b', 'node-c'],
      color: '#38bdf8', // sky blue
      glow: 'rgba(56, 189, 248, 0.4)',
    },
    {
      id: 1,
      name: 'Shard 1',
      leader: 'Node D',
      nodes: ['node-d', 'node-e', 'node-a'],
      color: '#818cf8', // indigo
      glow: 'rgba(129, 140, 248, 0.4)',
    },
    {
      id: 2,
      name: 'Shard 2',
      leader: 'Node B',
      nodes: ['node-b', 'node-c', 'node-d'],
      color: '#c084fc', // purple
      glow: 'rgba(192, 132, 252, 0.4)',
    },
  ];

  // Helper to check if edge belongs to selected shard
  const isEdgeActive = (node1Id: string, node2Id: string, shardId: number) => {
    if (selectedShard !== 'all' && selectedShard !== shardId) return false;
    const cfg = shardConfigs.find((s) => s.id === shardId);
    return cfg?.nodes.includes(node1Id) && cfg?.nodes.includes(node2Id);
  };

  const getNodeById = (id: string) => nodes.find((n) => n.id === id);

  return (
    <div className="relative bg-[#0a0f1d]/80 backdrop-blur-md border border-slate-800/80 rounded-xl p-5 flex flex-col justify-between overflow-hidden">
      {/* Header and Controls */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4 z-10">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-base font-semibold text-white tracking-wide">
              Cluster Topology Preview
            </h2>
            <span className="text-[11px] font-mono px-2 py-0.5 rounded-full bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
              5 Replicas Quorum
            </span>
          </div>
          <p className="text-xs text-slate-400 mt-0.5">
            Real-time consensus mesh across 5 nodes & 3 multi-raft partitions
          </p>
        </div>

        {/* Shard Filter Tabs */}
        <div className="flex items-center gap-1.5 p-1 rounded-lg bg-slate-900/90 border border-slate-800 self-start sm:self-auto">
          <button
            onClick={() => setSelectedShard('all')}
            className={`px-2.5 py-1 text-xs font-mono rounded transition-colors ${
              selectedShard === 'all'
                ? 'bg-sky-500/20 text-sky-300 border border-sky-500/30 font-semibold'
                : 'text-slate-400 hover:text-slate-200'
            }`}
          >
            All Shards
          </button>
          {shardConfigs.map((shard) => (
            <button
              key={shard.id}
              onClick={() => setSelectedShard(shard.id)}
              className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-mono rounded transition-colors ${
                selectedShard === shard.id
                  ? 'border font-semibold shadow-sm'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
              style={{
                backgroundColor: selectedShard === shard.id ? `${shard.color}25` : undefined,
                borderColor: selectedShard === shard.id ? shard.color : undefined,
                color: selectedShard === shard.id ? shard.color : undefined,
              }}
            >
              <span
                className="w-2 h-2 rounded-full"
                style={{ backgroundColor: shard.color }}
              />
              Shard {shard.id}
            </button>
          ))}
        </div>
      </div>

      {/* SVG Interactive Topology Canvas */}
      <div className="relative w-full h-[400px] flex items-center justify-center bg-slate-950/40 rounded-lg border border-slate-900/80 overflow-hidden">
        {/* Subtle grid pattern background */}
        <div
          className="absolute inset-0 opacity-[0.04] pointer-events-none"
          style={{
            backgroundImage: `radial-gradient(circle at 1px 1px, #fff 1px, transparent 0)`,
            backgroundSize: '24px 24px',
          }}
        />

        <svg
          viewBox="0 0 600 460"
          className="w-full h-full max-h-[420px] select-none"
        >
          <defs>
            {shardConfigs.map((s) => (
              <linearGradient key={`grad-${s.id}`} id={`grad-${s.id}`} x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stopColor={s.color} stopOpacity="0.8" />
                <stop offset="100%" stopColor={s.color} stopOpacity="0.2" />
              </linearGradient>
            ))}

            <filter id="glow" x="-20%" y="-20%" width="140%" height="140%">
              <feGaussianBlur stdDeviation="3" result="blur" />
              <feComposite in="SourceGraphic" in2="blur" operator="over" />
            </filter>
          </defs>

          {/* Shard Replication Links */}
          {shardConfigs.map((shard) => {
            const pairs: [string, string][] = [
              [shard.nodes[0], shard.nodes[1]],
              [shard.nodes[1], shard.nodes[2]],
              [shard.nodes[2], shard.nodes[0]],
            ];

            return pairs.map(([n1, n2], idx) => {
              const active = isEdgeActive(n1, n2, shard.id);
              const node1 = getNodeById(n1);
              const node2 = getNodeById(n2);
              if (!node1 || !node2) return null;

              return (
                <g key={`edge-${shard.id}-${idx}`}>
                  <line
                    x1={node1.x}
                    y1={node1.y}
                    x2={node2.x}
                    y2={node2.y}
                    stroke={shard.color}
                    strokeWidth={active ? (selectedShard === shard.id ? 2.5 : 1.5) : 0.6}
                    strokeOpacity={active ? (selectedShard === shard.id ? 0.9 : 0.45) : 0.08}
                    strokeDasharray={active ? 'none' : '4 4'}
                    className="transition-all duration-300"
                  />

                  {/* Pulsing data packets along active links */}
                  {active && (
                    <circle r="2.5" fill={shard.color} filter="url(#glow)">
                      <animateMotion
                        path={`M ${node1.x} ${node1.y} L ${node2.x} ${node2.y}`}
                        dur={`${2.4 + idx * 0.4}s`}
                        repeatCount="indefinite"
                      />
                    </circle>
                  )}
                </g>
              );
            });
          })}

          {/* Nodes */}
          {nodes.map((node) => {
            const isHovered = hoveredNode?.id === node.id;
            const hasLeader = node.shards.some((s) => s.role === 'Leader');
            const relevantShards =
              selectedShard === 'all'
                ? node.shards
                : node.shards.filter((s) => s.shardId === selectedShard);
            const isRelevant = relevantShards.length > 0;

            return (
              <g
                key={node.id}
                transform={`translate(${node.x}, ${node.y})`}
                className="cursor-pointer group"
                onMouseEnter={() => setHoveredNode(node)}
                onMouseLeave={() => setHoveredNode(null)}
              >
                {/* Leader Outer Pulse Ring */}
                {hasLeader && (
                  <circle
                    r="34"
                    fill="none"
                    stroke="#f59e0b"
                    strokeWidth="1.5"
                    strokeOpacity="0.3"
                    className="animate-ping"
                    style={{ transformOrigin: 'center', animationDuration: '3s' }}
                  />
                )}

                {/* Node Ambient Halo */}
                <circle
                  r={isHovered ? 28 : 24}
                  fill="#030712"
                  stroke={
                    isHovered
                      ? '#38bdf8'
                      : hasLeader
                      ? '#f59e0b'
                      : isRelevant
                      ? '#334155'
                      : '#1e293b'
                  }
                  strokeWidth={isHovered ? 2.5 : 1.5}
                  strokeOpacity={isRelevant ? 1 : 0.4}
                  filter={isHovered ? 'url(#glow)' : undefined}
                  className="transition-all duration-200"
                />

                {/* Node Body Inner Background */}
                <circle
                  r={isHovered ? 24 : 20}
                  fill={isHovered ? '#0f172a' : '#090d16'}
                />

                {/* Leader Crown Badge or Node Name */}
                {hasLeader ? (
                  <g transform="translate(-7, -8)">
                    <path
                      d="M2 11h10M2 11l1.5-6L6 8l1.5-5 1.5 5 2.5-3 1.5 6H2z"
                      fill="#f59e0b"
                      stroke="#f59e0b"
                      strokeWidth="1"
                    />
                  </g>
                ) : (
                  <text
                    textAnchor="middle"
                    dy="4"
                    className="text-[11px] font-mono font-bold fill-slate-300 pointer-events-none"
                  >
                    {node.name.replace('Node ', '')}
                  </text>
                )}

                {/* Node Title Below */}
                <text
                  textAnchor="middle"
                  dy="40"
                  className={`text-xs font-mono font-semibold transition-colors pointer-events-none ${
                    isHovered
                      ? 'fill-sky-400'
                      : isRelevant
                      ? 'fill-slate-200'
                      : 'fill-slate-500'
                  }`}
                >
                  {node.name}
                </text>

                {/* Shard Pills Under Node */}
                <g transform="translate(0, 52)">
                  {node.shards.map((s, idx) => {
                    const cfg = shardConfigs.find((sc) => sc.id === s.shardId);
                    const isSelected = selectedShard === 'all' || selectedShard === s.shardId;
                    const offset = (idx - (node.shards.length - 1) / 2) * 16;
                    return (
                      <circle
                        key={s.shardId}
                        cx={offset}
                        cy={0}
                        r="4"
                        fill={cfg?.color}
                        fillOpacity={isSelected ? 1 : 0.25}
                        stroke={s.role === 'Leader' ? '#f59e0b' : 'none'}
                        strokeWidth="1"
                      />
                    );
                  })}
                </g>
              </g>
            );
          })}
        </svg>

        {/* Hover Tooltip Overlay */}
        {hoveredNode && (
          <div
            className="absolute z-20 pointer-events-none p-3 rounded-lg bg-slate-900/95 border border-slate-700 shadow-2xl backdrop-blur-md w-60 text-xs animate-in fade-in duration-150"
            style={{
              left: Math.min(Math.max(hoveredNode.x - 120, 16), 340),
              top: Math.min(hoveredNode.y + 60, 240),
            }}
          >
            <div className="flex items-center justify-between pb-2 mb-2 border-b border-slate-800">
              <div className="flex items-center gap-2">
                <Server className="w-3.5 h-3.5 text-sky-400" />
                <span className="font-semibold text-white font-mono">{hoveredNode.name}</span>
              </div>
              <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/30">
                {hoveredNode.status.toUpperCase()}
              </span>
            </div>

            <div className="space-y-1.5 text-slate-300 font-mono text-[11px]">
              <div className="flex justify-between">
                <span className="text-slate-400">Address:</span>
                <span className="text-slate-200">{hoveredNode.ip}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-slate-400">CPU / RAM:</span>
                <span className="text-slate-200">{hoveredNode.cpu} · {hoveredNode.ram}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-slate-400">Uptime:</span>
                <span className="text-slate-200">{hoveredNode.uptime}</span>
              </div>
            </div>

            <div className="mt-2.5 pt-2 border-t border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-mono block mb-1">
                Hosted Shard Replicas:
              </span>
              <div className="space-y-1">
                {hoveredNode.shards.map((s) => {
                  const cfg = shardConfigs.find((c) => c.id === s.shardId);
                  return (
                    <div
                      key={s.shardId}
                      className="flex items-center justify-between text-[11px] font-mono px-2 py-0.5 rounded bg-slate-950/60 border border-slate-800"
                    >
                      <div className="flex items-center gap-1.5">
                        <span
                          className="w-1.5 h-1.5 rounded-full"
                          style={{ backgroundColor: cfg?.color }}
                        />
                        <span className="text-slate-200">Shard {s.shardId}</span>
                      </div>
                      <span
                        className={`text-[10px] font-semibold ${
                          s.role === 'Leader' ? 'text-amber-400 flex items-center gap-1' : 'text-slate-400'
                        }`}
                      >
                        {s.role === 'Leader' && <Crown className="w-2.5 h-2.5" />}
                        {s.role}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Legend Footer */}
      <div className="mt-3 pt-3 border-t border-slate-800/60 flex flex-wrap items-center justify-between gap-3 text-xs text-slate-400 font-mono">
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-1.5">
            <span className="w-2 h-2 rounded-full border border-amber-400 bg-amber-400/20" />
            <span>Raft Leader</span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="w-2 h-2 rounded-full border border-slate-400 bg-slate-400/20" />
            <span>Raft Follower</span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="w-3 h-0.5 bg-sky-400 inline-block" />
            <span>Replication Link</span>
          </div>
        </div>
        <span className="text-[11px] text-slate-400">
          Click or hover any node for detailed health & replica telemetry
        </span>
      </div>
    </div>
  );
}
