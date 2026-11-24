'use client';

import React, { useState, useMemo } from 'react';
import {
  Server,
  Crown,
  Eye,
  Power,
  Search,
  Filter,
  ArrowUpDown,
  CheckCircle2,
  XCircle,
} from 'lucide-react';
import { PhysicalNode } from '../../lib/topology/types';

interface NodeInventoryProps {
  nodes: PhysicalNode[];
  selectedNodeId: string | null;
  onSelectNode: (nodeId: string) => void;
  onToggleOffline: (nodeId: string) => void;
}

type StatusFilter = 'all' | 'online' | 'offline';
type RoleFilter = 'all' | 'leader' | 'follower';
type SortField = 'id' | 'status' | 'shards';

export default function NodeInventory({
  nodes,
  selectedNodeId,
  onSelectNode,
  onToggleOffline,
}: NodeInventoryProps) {
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [roleFilter, setRoleFilter] = useState<RoleFilter>('all');
  const [sortField, setSortField] = useState<SortField>('id');
  const [sortAsc, setSortAsc] = useState(true);

  // Filter & Sort Logic
  const filteredAndSortedNodes = useMemo(() => {
    return nodes
      .filter((node) => {
        // Search query
        const matchesSearch =
          node.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
          node.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
          node.httpAddress.toLowerCase().includes(searchQuery.toLowerCase());

        // Status filter
        const matchesStatus =
          statusFilter === 'all' ||
          (statusFilter === 'online' && node.online) ||
          (statusFilter === 'offline' && !node.online);

        // Role filter
        const isLeader = node.hostedShards.some((s) => s.role === 'Leader');
        const matchesRole =
          roleFilter === 'all' ||
          (roleFilter === 'leader' && isLeader) ||
          (roleFilter === 'follower' && !isLeader);

        return matchesSearch && matchesStatus && matchesRole;
      })
      .sort((a, b) => {
        let cmp = 0;
        if (sortField === 'id') {
          cmp = a.name.localeCompare(b.name);
        } else if (sortField === 'status') {
          cmp = (a.online === b.online ? 0 : a.online ? -1 : 1);
        } else if (sortField === 'shards') {
          cmp = a.hostedShards.length - b.hostedShards.length;
        }
        return sortAsc ? cmp : -cmp;
      });
  }, [nodes, searchQuery, statusFilter, roleFilter, sortField, sortAsc]);

  const toggleSort = (field: SortField) => {
    if (sortField === field) {
      setSortAsc(!sortAsc);
    } else {
      setSortField(field);
      setSortAsc(true);
    }
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs overflow-hidden">
      {/* 01. HEADER */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-4 mb-4 border-b border-[#DCE5F0]">
        <div className="flex items-center gap-2.5">
          <div className="w-8 h-8 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
            <Server className="w-4 h-4 text-[#0891D5]" />
          </div>
          <div>
            <h3 className="text-base font-bold text-[#17243A] font-mono tracking-tight">
              Physical Node Inventory
            </h3>
            <p className="text-xs text-[#64748B] font-mono">
              Cluster hardware units, network addresses & multi-raft participation
            </p>
          </div>
        </div>

        <span className="text-xs font-mono text-[#64748B] px-3 py-1 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] self-start sm:self-auto">
          {nodes.length} Machines Registered
        </span>
      </div>

      {/* 02. CONTROLS: SEARCH, FILTERS & SORT */}
      <div className="flex flex-wrap items-center justify-between gap-3 mb-4 font-mono text-xs">
        {/* Search input */}
        <div className="relative flex-1 min-w-[200px] max-w-sm">
          <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-[#64748B]" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search nodes by name, ID or port..."
            className="w-full pl-9 pr-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] focus:border-[#0891D5] focus:bg-white outline-none transition-all text-[#17243A] placeholder-[#94A3B8]"
          />
        </div>

        {/* Filter Badges */}
        <div className="flex flex-wrap items-center gap-2">
          {/* Status Filter */}
          <div className="flex items-center gap-1 p-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0]">
            <button
              onClick={() => setStatusFilter('all')}
              className={`px-2 py-0.5 rounded-lg transition-all ${
                statusFilter === 'all'
                  ? 'bg-white text-[#0891D5] shadow-xs font-semibold'
                  : 'text-[#64748B]'
              }`}
            >
              All
            </button>
            <button
              onClick={() => setStatusFilter('online')}
              className={`px-2 py-0.5 rounded-lg transition-all ${
                statusFilter === 'online'
                  ? 'bg-white text-[#10B981] shadow-xs font-semibold'
                  : 'text-[#64748B]'
              }`}
            >
              Online
            </button>
            <button
              onClick={() => setStatusFilter('offline')}
              className={`px-2 py-0.5 rounded-lg transition-all ${
                statusFilter === 'offline'
                  ? 'bg-white text-[#EF4444] shadow-xs font-semibold'
                  : 'text-[#64748B]'
              }`}
            >
              Offline
            </button>
          </div>

          {/* Role Filter */}
          <div className="flex items-center gap-1 p-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0]">
            <button
              onClick={() => setRoleFilter('all')}
              className={`px-2 py-0.5 rounded-lg transition-all ${
                roleFilter === 'all'
                  ? 'bg-white text-[#0891D5] shadow-xs font-semibold'
                  : 'text-[#64748B]'
              }`}
            >
              All Roles
            </button>
            <button
              onClick={() => setRoleFilter('leader')}
              className={`px-2 py-0.5 rounded-lg transition-all flex items-center gap-1 ${
                roleFilter === 'leader'
                  ? 'bg-white text-[#D97706] shadow-xs font-semibold'
                  : 'text-[#64748B]'
              }`}
            >
              <Crown className="w-3 h-3 text-[#F59E0B]" />
              Leaders
            </button>
          </div>
        </div>
      </div>

      {/* 03. STRUCTURED TABLE */}
      <div className="overflow-x-auto">
        <table className="w-full text-left font-mono text-xs">
          <thead>
            <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase tracking-wider bg-[#F8FAFC]">
              <th
                onClick={() => toggleSort('id')}
                className="py-2.5 px-3 font-semibold cursor-pointer hover:text-[#17243A]"
              >
                <div className="flex items-center gap-1">
                  <span>Node</span>
                  <ArrowUpDown className="w-3 h-3" />
                </div>
              </th>
              <th
                onClick={() => toggleSort('status')}
                className="py-2.5 px-3 font-semibold cursor-pointer hover:text-[#17243A]"
              >
                <div className="flex items-center gap-1">
                  <span>Status</span>
                  <ArrowUpDown className="w-3 h-3" />
                </div>
              </th>
              <th className="py-2.5 px-3 font-semibold">HTTP Address</th>
              <th
                onClick={() => toggleSort('shards')}
                className="py-2.5 px-3 font-semibold cursor-pointer hover:text-[#17243A]"
              >
                <div className="flex items-center gap-1">
                  <span>Hosted Shards</span>
                  <ArrowUpDown className="w-3 h-3" />
                </div>
              </th>
              <th className="py-2.5 px-3 font-semibold">Raft Groups</th>
              <th className="py-2.5 px-3 font-semibold">Last Activity</th>
              <th className="py-2.5 px-3 font-semibold text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9]">
            {filteredAndSortedNodes.length === 0 ? (
              <tr>
                <td colSpan={7} className="py-6 text-center text-[#64748B]">
                  No nodes match the selected search and filter criteria.
                </td>
              </tr>
            ) : (
              filteredAndSortedNodes.map((node) => {
                const isSelected = selectedNodeId === node.id;

                return (
                  <tr
                    key={node.id}
                    onClick={() => onSelectNode(node.id)}
                    className={`cursor-pointer transition-all ${
                      isSelected
                        ? 'bg-sky-50/80 font-medium'
                        : 'hover:bg-[#F8FAFC]'
                    }`}
                  >
                    {/* Node */}
                    <td className="py-3 px-3">
                      <div className="flex items-center gap-2">
                        <span
                          className={`w-2 h-2 rounded-full ${
                            node.online ? 'bg-[#10B981]' : 'bg-[#EF4444]'
                          }`}
                        />
                        <span className="font-bold text-[#17243A] text-sm">
                          {node.name}
                        </span>
                        <span className="text-[11px] text-[#64748B]">
                          ({node.id})
                        </span>
                      </div>
                    </td>

                    {/* Status */}
                    <td className="py-3 px-3">
                      {node.online ? (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-semibold text-[11px]">
                          <span className="w-1.5 h-1.5 rounded-full bg-[#10B981]" />
                          Online
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-rose-50 border border-rose-200 text-[#EF4444] font-semibold text-[11px]">
                          <span className="w-1.5 h-1.5 rounded-full bg-[#EF4444]" />
                          Offline
                        </span>
                      )}
                    </td>

                    {/* HTTP Address */}
                    <td className="py-3 px-3 text-[#0891D5] font-semibold">
                      {node.httpAddress}
                    </td>

                    {/* Hosted Shards */}
                    <td className="py-3 px-3">
                      <div className="flex items-center gap-1.5 flex-wrap">
                        {node.hostedShards.map((sh) => (
                          <span
                            key={sh.shardId}
                            className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] border"
                            style={{
                              backgroundColor: `${sh.color}15`,
                              borderColor: `${sh.color}40`,
                              color: sh.color,
                            }}
                          >
                            {sh.shardName}
                            {sh.role === 'Leader' && (
                              <Crown className="w-3 h-3 text-[#F59E0B]" />
                            )}
                          </span>
                        ))}
                      </div>
                    </td>

                    {/* Raft Groups */}
                    <td className="py-3 px-3 text-[#17243A]">
                      {node.raftGroupCount} groups{' '}
                      <span className="text-[#64748B] text-[11px]">
                        ({node.leaderGroupCount}L, {node.followerGroupCount}F)
                      </span>
                    </td>

                    {/* Last Activity */}
                    <td className="py-3 px-3 text-[#64748B]">
                      {node.lastObservedActivity}
                    </td>

                    {/* Actions */}
                    <td className="py-3 px-3 text-right">
                      <div
                        className="inline-flex items-center gap-2"
                        onClick={(e) => e.stopPropagation()}
                      >
                        <button
                          onClick={() => onSelectNode(node.id)}
                          className={`p-1.5 rounded-lg border transition-all ${
                            isSelected
                              ? 'bg-[#0891D5] text-white border-[#0891D5]'
                              : 'bg-white hover:bg-slate-100 border-[#DCE5F0] text-[#0891D5]'
                          }`}
                          title="Highlight in 3D & Inspect"
                        >
                          <Eye className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={() => onToggleOffline(node.id)}
                          className={`p-1.5 rounded-lg border text-xs transition-all ${
                            node.online
                              ? 'bg-rose-50 hover:bg-rose-100/80 border-rose-200 text-rose-600'
                              : 'bg-emerald-50 hover:bg-emerald-100/80 border-emerald-200 text-emerald-700'
                          }`}
                          title={node.online ? 'Simulate Crash (Demo)' : 'Restart Node'}
                        >
                          <Power className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
