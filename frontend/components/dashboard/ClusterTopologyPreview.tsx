'use client';

import React, { useState } from 'react';
import Link from 'next/link';
import { Crown, Server, Layers, Zap, Info, ShieldCheck, ArrowRight, AlertTriangle } from 'lucide-react';
import { useClusterHealth } from '@/hooks/useClusterHealth';

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
  status: 'online' | 'degraded' | 'offline';
  lastActivity?: string;
}

const NODE_POSITIONS: Record<string, { x: number; y: number; name: string; port: number }> = {
  'node-a': { x: 300, y: 90, name: 'Node A', port: 2525 },
  'node-b': { x: 480, y: 200, name: 'Node B', port: 2526 },
  'node-c': { x: 410, y: 380, name: 'Node C', port: 2527 },
  'node-d': { x: 190, y: 380, name: 'Node D', port: 2528 },
  'node-e': { x: 120, y: 200, name: 'Node E', port: 2529 },
};

const SHARD_COLORS = ['#0891D5', '#7965E8', '#10B981'];

export default function ClusterTopologyPreview() {
  const [selectedShard, setSelectedShard] = useState<number | 'all'>('all');
  const [hoveredNode, setHoveredNode] = useState<NodeData | null>(null);
  const { health, isLive, endpoint } = useClusterHealth(3000);

  // Dynamically derive shard configurations from live health telemetry
  const shardConfigs = Object.values(health?.shards || {}).map((s) => ({
    id: s.shard_id,
    name: `Shard ${s.shard_id}`,
    leader: s.current_leader || 'None',
    nodes: s.replica_node_ids || [],
    color: SHARD_COLORS[s.shard_id % SHARD_COLORS.length],
    healthy: s.healthy,
    quorumAvailable: s.quorum_available,
  }));

  // Dynamically build nodes from live cluster health
  const nodeIds = Object.keys(NODE_POSITIONS);
  const nodes: NodeData[] = nodeIds.map((id) => {
    const pos = NODE_POSITIONS[id];
    const nodeStatus = health?.nodes?.[id];
    const isOnline = isLive && (nodeStatus?.online ?? false);

    // Find shards hosted by this node
    const hostedShards: { shardId: number; role: 'Leader' | 'Follower' }[] = [];
    if (health?.shards) {
      Object.values(health.shards).forEach((shard) => {
        if (shard.replica_node_ids?.includes(id)) {
          hostedShards.push({
            shardId: shard.shard_id,
            role: shard.current_leader === id ? 'Leader' : 'Follower',
          });
        }
      });
    }

    return {
      id,
      name: pos.name,
      port: pos.port,
      ip: `127.0.0.1:${pos.port}`,
      x: pos.x,
      y: pos.y,
      shards: hostedShards,
      status: isOnline ? 'online' : isLive ? 'offline' : 'offline',
      lastActivity: nodeStatus?.last_observed_activity,
    };
  });

  const isEdgeActive = (node1Id: string, node2Id: string, shardId: number) => {
    if (selectedShard !== 'all' && selectedShard !== shardId) return false;
    const cfg = shardConfigs.find((s) => s.id === shardId);
    if (!cfg) return false;
    // Edge is active only if both nodes are assigned to the shard and at least one is online
    const node1Online = nodes.find((n) => n.id === node1Id)?.status === 'online';
    const node2Online = nodes.find((n) => n.id === node2Id)?.status === 'online';
    return isLive && cfg.nodes.includes(node1Id) && cfg.nodes.includes(node2Id) && (node1Online || node2Online);
  };

  const getNodeById = (id: string) => nodes.find((n) => n.id === id);

  const totalOnlineNodes = nodes.filter((n) => n.status === 'online').length;

  return (
    <div className="relative bg-white border border-[#DCE5F0] rounded-2xl p-6 flex flex-col justify-between overflow-hidden shadow-sm">
      {/* Header and Controls */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4 z-10">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-base font-bold text-[#17243A] tracking-wide">
              Cluster Topology Preview
            </h2>
            {isLive ? (
              <span className="text-[11px] font-mono px-2.5 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-emerald-700 font-semibold flex items-center gap-1.5">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
                {totalOnlineNodes}/5 Nodes Online
              </span>
            ) : (
              <span className="text-[11px] font-mono px-2.5 py-0.5 rounded-full bg-rose-50 border border-rose-200 text-rose-700 font-semibold flex items-center gap-1.5">
                <AlertTriangle className="w-3 h-3 text-rose-500" />
                Cluster Offline
              </span>
            )}
          </div>
          <p className="text-xs text-[#64748B] mt-0.5">
            {isLive
              ? `Connected to ${endpoint || 'local cluster'} · 3 multi-raft partitions with quorum`
              : 'Backend disconnected · Showing discovery slots (nodes a–e)'}
          </p>
        </div>

        <div className="flex items-center gap-2 self-start sm:self-auto">
          {/* Shard Filter Tabs */}
          <div className="flex items-center gap-1.5 p-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] font-mono text-xs">
            <button
              onClick={() => setSelectedShard('all')}
              className={`px-3 py-1 rounded-lg transition-colors font-semibold cursor-pointer ${
                selectedShard === 'all'
                  ? 'bg-white text-[#0891D5] border border-[#DCE5F0] shadow-xs'
                  : 'text-[#64748B] hover:text-[#17243A]'
              }`}
            >
              All Shards
            </button>
            {shardConfigs.map((shard) => {
              const isSelected = selectedShard === shard.id;
              return (
                <button
                  key={shard.id}
                  onClick={() => setSelectedShard(shard.id)}
                  className={`flex items-center gap-1.5 px-3 py-1 rounded-lg transition-colors font-semibold cursor-pointer ${
                    isSelected
                      ? 'bg-white text-[#17243A] border border-[#DCE5F0] shadow-xs'
                      : 'text-[#64748B] hover:text-[#17243A]'
                  }`}
                >
                  <span
                    className="w-2 h-2 rounded-full"
                    style={{ backgroundColor: shard.color }}
                  />
                  Shard {shard.id}
                </button>
              );
            })}
          </div>

          <Link
            href="/dashboard/topology"
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-[#0891D5]/10 hover:bg-[#0891D5]/20 text-[#0891D5] border border-[#0891D5]/30 text-xs font-mono font-semibold transition-colors"
          >
            <span>3D Explorer</span>
            <ArrowRight className="w-3.5 h-3.5" />
          </Link>
        </div>
      </div>

      {/* SVG Canvas in light styling */}
      <div className="relative w-full h-[400px] flex items-center justify-center bg-[#F8FAFC] rounded-xl border border-[#DCE5F0] overflow-hidden">
        {/* Subtle dot matrix background */}
        <div
          className="absolute inset-0 opacity-40 pointer-events-none"
          style={{
            backgroundImage: `radial-gradient(circle at 1px 1px, #CBD5E1 1px, transparent 0)`,
            backgroundSize: '24px 24px',
          }}
        />

        {!isLive && (
          <div className="absolute top-4 left-4 z-10 px-3 py-1.5 rounded-lg bg-amber-50/90 border border-amber-200 text-amber-800 text-[11px] font-mono flex items-center gap-2 backdrop-blur-xs">
            <AlertTriangle className="w-3.5 h-3.5 text-amber-600" />
            <span>Telemetry unavailable. Start cluster via <code>python scripts/run_cluster.py</code></span>
          </div>
        )}

        <svg viewBox="0 0 600 460" className="w-full h-full max-h-[420px] select-none">
          {/* Shard Replication Links */}
          {shardConfigs.map((shard) => {
            if (shard.nodes.length < 2) return null;
            const pairs: [string, string][] = [
              [shard.nodes[0], shard.nodes[1]],
              ...(shard.nodes.length > 2 ? [[shard.nodes[1], shard.nodes[2]] as [string, string], [shard.nodes[2], shard.nodes[0]] as [string, string]] : []),
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
                    strokeWidth={active ? (selectedShard === shard.id ? 3 : 2) : 0.8}
                    strokeOpacity={active ? (selectedShard === shard.id ? 0.95 : 0.5) : 0.12}
                    strokeDasharray={active ? 'none' : '4 4'}
                    className="transition-all duration-300"
                  />

                  {/* Pulsing data packets along active links */}
                  {active && (
                    <circle r="3" fill={shard.color}>
                      <animateMotion
                        path={`M ${node1.x} ${node1.y} L ${node2.x} ${node2.y}`}
                        dur={`${2.2 + idx * 0.4}s`}
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
            const isOnline = node.status === 'online';

            return (
              <g
                key={node.id}
                transform={`translate(${node.x}, ${node.y})`}
                className="cursor-pointer group"
                onMouseEnter={() => setHoveredNode(node)}
                onMouseLeave={() => setHoveredNode(null)}
              >
                {/* Leader Outer Pulse Ring */}
                {isOnline && hasLeader && (
                  <circle
                    r="34"
                    fill="none"
                    stroke="#F59E0B"
                    strokeWidth="1.5"
                    strokeOpacity="0.3"
                    className="animate-ping"
                    style={{ transformOrigin: 'center', animationDuration: '3s' }}
                  />
                )}

                {/* Node Outer Ring */}
                <circle
                  r={isHovered ? 28 : 24}
                  fill="#FFFFFF"
                  stroke={
                    !isOnline
                      ? '#CBD5E1'
                      : isHovered
                      ? '#0891D5'
                      : hasLeader
                      ? '#F59E0B'
                      : isRelevant
                      ? '#94A3B8'
                      : '#E2E8F0'
                  }
                  strokeWidth={isHovered ? 3 : 2}
                  strokeOpacity={isOnline ? (isRelevant ? 1 : 0.4) : 0.3}
                  filter="drop-shadow(0px 4px 8px rgba(0,0,0,0.06))"
                  className="transition-all duration-200"
                />

                {/* Node Inner Fill */}
                <circle
                  r={isHovered ? 24 : 20}
                  fill={!isOnline ? '#F1F5F9' : isHovered ? '#F0F9FF' : '#F8FAFC'}
                />

                {/* Leader Crown Badge or Node Name */}
                {isOnline && hasLeader ? (
                  <g transform="translate(-7, -8)">
                    <path
                      d="M2 11h10M2 11l1.5-6L6 8l1.5-5 1.5 5 2.5-3 1.5 6H2z"
                      fill="#F59E0B"
                      stroke="#F59E0B"
                      strokeWidth="1"
                    />
                  </g>
                ) : (
                  <text
                    textAnchor="middle"
                    dy="4"
                    className={`text-[12px] font-mono font-bold pointer-events-none ${
                      isOnline ? 'fill-[#17243A]' : 'fill-[#94A3B8]'
                    }`}
                  >
                    {node.name.replace('Node ', '')}
                  </text>
                )}

                {/* Node Title Below */}
                <text
                  textAnchor="middle"
                  dy="42"
                  className={`text-xs font-mono font-bold transition-colors pointer-events-none ${
                    !isOnline
                      ? 'fill-[#94A3B8]'
                      : isHovered
                      ? 'fill-[#0891D5]'
                      : isRelevant
                      ? 'fill-[#17243A]'
                      : 'fill-[#94A3B8]'
                  }`}
                >
                  {node.name}
                </text>

                {/* Shard Indicator Dots Under Node */}
                <g transform="translate(0, 54)">
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
                        fill={cfg?.color || '#94A3B8'}
                        fillOpacity={isOnline ? (isSelected ? 1 : 0.3) : 0.2}
                        stroke={isOnline && s.role === 'Leader' ? '#F59E0B' : '#FFFFFF'}
                        strokeWidth="1.5"
                      />
                    );
                  })}
                </g>
              </g>
            );
          })}
        </svg>

        {/* Hover Tooltip Overlay in Light Card Style */}
        {hoveredNode && (
          <div
            className="absolute z-20 pointer-events-none p-4 rounded-xl bg-white/95 border border-[#DCE5F0] shadow-xl backdrop-blur-md w-64 text-xs animate-in fade-in duration-150"
            style={{
              left: Math.min(Math.max(hoveredNode.x - 120, 16), 340),
              top: Math.min(hoveredNode.y + 60, 230),
            }}
          >
            <div className="flex items-center justify-between pb-2 mb-2 border-b border-[#DCE5F0]">
              <div className="flex items-center gap-2">
                <Server className="w-4 h-4 text-[#0891D5]" />
                <span className="font-bold text-[#17243A] font-mono">{hoveredNode.name}</span>
              </div>
              <span
                className={`text-[10px] font-mono px-2 py-0.5 rounded-full font-semibold ${
                  hoveredNode.status === 'online'
                    ? 'bg-emerald-50 text-emerald-700 border border-emerald-200'
                    : 'bg-slate-100 text-slate-600 border border-slate-200'
                }`}
              >
                {hoveredNode.status.toUpperCase()}
              </span>
            </div>

            <div className="space-y-1.5 text-[#17243A] font-mono text-[11px]">
              <div className="flex justify-between">
                <span className="text-[#64748B]">Address:</span>
                <span className="font-medium">{hoveredNode.ip}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-[#64748B]">Port:</span>
                <span className="font-medium">:{hoveredNode.port}</span>
              </div>
              {hoveredNode.lastActivity && (
                <div className="flex justify-between">
                  <span className="text-[#64748B]">Last Observed:</span>
                  <span className="font-medium">{new Date(hoveredNode.lastActivity).toLocaleTimeString()}</span>
                </div>
              )}
            </div>

            <div className="mt-3 pt-2 border-t border-[#DCE5F0]">
              <span className="text-[10px] text-[#64748B] uppercase font-mono block mb-1 font-semibold">
                Hosted Shard Replicas:
              </span>
              {hoveredNode.shards.length === 0 ? (
                <span className="text-[11px] font-mono text-[#64748B] italic">No shards assigned</span>
              ) : (
                <div className="space-y-1">
                  {hoveredNode.shards.map((s) => {
                    const cfg = shardConfigs.find((c) => c.id === s.shardId);
                    return (
                      <div
                        key={s.shardId}
                        className="flex items-center justify-between text-[11px] font-mono px-2 py-1 rounded bg-[#F8FAFC] border border-[#DCE5F0]"
                      >
                        <div className="flex items-center gap-1.5">
                          <span
                            className="w-2 h-2 rounded-full"
                            style={{ backgroundColor: cfg?.color || '#94A3B8' }}
                          />
                          <span className="font-semibold text-[#17243A]">Shard {s.shardId}</span>
                        </div>
                        <span
                          className={`text-[10px] font-bold ${
                            s.role === 'Leader' ? 'text-amber-600 flex items-center gap-1' : 'text-[#64748B]'
                          }`}
                        >
                          {s.role === 'Leader' && <Crown className="w-2.5 h-2.5" />}
                          {s.role}
                        </span>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Legend Footer */}
      <div className="mt-4 pt-3 border-t border-[#DCE5F0] flex flex-wrap items-center justify-between gap-3 text-xs text-[#64748B] font-mono">
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-1.5">
            <span className="w-2.5 h-2.5 rounded-full border border-amber-500 bg-amber-100" />
            <span className="text-[#17243A]">Raft Leader</span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="w-2.5 h-2.5 rounded-full border border-slate-400 bg-slate-100" />
            <span className="text-[#17243A]">Raft Follower</span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="w-3.5 h-0.5 bg-[#0891D5] inline-block" />
            <span className="text-[#17243A]">Replication Link</span>
          </div>
        </div>
        <span className="text-[11px]">
          Hover any node for live address & replica assignments
        </span>
      </div>
    </div>
  );
}
