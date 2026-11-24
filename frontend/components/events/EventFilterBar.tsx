'use client';

import React from 'react';
import {
  Search,
  Filter,
  Play,
  Pause,
  Download,
  RotateCcw,
  Layers,
  Server,
  AlertTriangle,
} from 'lucide-react';

export type EventSeverity = 'ALL' | 'INFO' | 'WARN' | 'ERROR' | 'CRITICAL';
export type EventCategory = 'ALL' | 'CLUSTER' | 'RAFT' | 'NETWORK' | 'STORAGE' | 'REPLICATION' | 'SYSTEM';

interface EventFilterBarProps {
  search: string;
  onSearchChange: (val: string) => void;
  severity: EventSeverity;
  onSeverityChange: (s: EventSeverity) => void;
  category: EventCategory;
  onCategoryChange: (c: EventCategory) => void;
  selectedNode: string;
  onNodeChange: (n: string) => void;
  selectedShard: string;
  onShardChange: (s: string) => void;
  isPaused: boolean;
  onTogglePause: () => void;
  onExportJson: () => void;
  onExportCsv: () => void;
  onResetFilters: () => void;
}

export default function EventFilterBar({
  search,
  onSearchChange,
  severity,
  onSeverityChange,
  category,
  onCategoryChange,
  selectedNode,
  onNodeChange,
  selectedShard,
  onShardChange,
  isPaused,
  onTogglePause,
  onExportJson,
  onExportCsv,
  onResetFilters,
}: EventFilterBarProps) {
  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-4 shadow-xs space-y-3 font-mono text-xs">
      {/* 01. TOP ROW: SEARCH & ACTION BUTTONS */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        {/* Search */}
        <div className="relative flex-1 min-w-[240px] max-w-md">
          <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-[#64748B]" />
          <input
            type="text"
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder="Search events by description, payload or ID..."
            className="w-full pl-9 pr-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] focus:border-[#0891D5] focus:bg-white outline-none transition-all text-[#17243A] placeholder-[#94A3B8]"
          />
        </div>

        {/* Streaming & Export Controls */}
        <div className="flex flex-wrap items-center gap-2">
          {/* Pause / Resume */}
          <button
            onClick={onTogglePause}
            className={`px-3 py-1.5 rounded-xl border font-semibold flex items-center gap-1.5 transition-all shadow-2xs ${
              isPaused
                ? 'bg-amber-50 border-amber-200 text-[#D97706]'
                : 'bg-[#F4F7FC] border-[#DCE5F0] text-[#17243A] hover:bg-[#EAF1FA]'
            }`}
          >
            {isPaused ? (
              <>
                <Play className="w-3 h-3 fill-current" />
                <span>Resume Stream</span>
              </>
            ) : (
              <>
                <Pause className="w-3 h-3 fill-current" />
                <span>Pause Stream</span>
              </>
            )}
          </button>

          {/* Export JSON */}
          <button
            onClick={onExportJson}
            className="px-2.5 py-1.5 rounded-xl bg-[#F4F7FC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#64748B] hover:text-[#17243A] transition-colors flex items-center gap-1"
            title="Export as JSON"
          >
            <Download className="w-3 h-3" />
            <span>JSON</span>
          </button>

          {/* Export CSV */}
          <button
            onClick={onExportCsv}
            className="px-2.5 py-1.5 rounded-xl bg-[#F4F7FC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#64748B] hover:text-[#17243A] transition-colors flex items-center gap-1"
            title="Export as CSV"
          >
            <Download className="w-3 h-3" />
            <span>CSV</span>
          </button>

          {/* Reset Filters */}
          <button
            onClick={onResetFilters}
            className="p-1.5 rounded-xl bg-[#F4F7FC] hover:bg-slate-200/60 border border-[#DCE5F0] text-[#64748B]"
            title="Reset Filters"
          >
            <RotateCcw className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* 02. BOTTOM ROW: MULTI-LEVEL FILTER CHIPS */}
      <div className="flex flex-wrap items-center gap-2 pt-2 border-t border-[#F1F5F9]">
        {/* Severity */}
        <div className="flex items-center gap-1 p-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0]">
          {(['ALL', 'INFO', 'WARN', 'ERROR'] as EventSeverity[]).map((s) => (
            <button
              key={s}
              onClick={() => onSeverityChange(s)}
              className={`px-2 py-0.5 rounded-lg transition-all ${
                severity === s
                  ? 'bg-white text-[#0891D5] shadow-xs font-bold'
                  : 'text-[#64748B] hover:text-[#17243A]'
              }`}
            >
              {s}
            </button>
          ))}
        </div>

        {/* Category */}
        <div className="flex items-center gap-1.5 px-3 py-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A]">
          <span className="text-[#64748B]">Category:</span>
          <select
            value={category}
            onChange={(e) => onCategoryChange(e.target.value as EventCategory)}
            className="bg-transparent font-semibold outline-none cursor-pointer"
          >
            <option value="ALL">All Categories</option>
            <option value="CLUSTER">Cluster Lifecycle</option>
            <option value="RAFT">Raft Consensus</option>
            <option value="STORAGE">LSM Storage</option>
            <option value="NETWORK">Network Transport</option>
            <option value="REPLICATION">Replication Sync</option>
          </select>
        </div>

        {/* Node */}
        <div className="flex items-center gap-1.5 px-3 py-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A]">
          <Server className="w-3 h-3 text-[#0891D5]" />
          <select
            value={selectedNode}
            onChange={(e) => onNodeChange(e.target.value)}
            className="bg-transparent font-semibold outline-none cursor-pointer"
          >
            <option value="ALL">All Nodes</option>
            <option value="node-a">Node A</option>
            <option value="node-b">Node B</option>
            <option value="node-c">Node C</option>
            <option value="node-d">Node D</option>
            <option value="node-e">Node E</option>
          </select>
        </div>

        {/* Shard */}
        <div className="flex items-center gap-1.5 px-3 py-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A]">
          <Layers className="w-3 h-3 text-[#7965E8]" />
          <select
            value={selectedShard}
            onChange={(e) => onShardChange(e.target.value)}
            className="bg-transparent font-semibold outline-none cursor-pointer"
          >
            <option value="ALL">All Shards</option>
            <option value="0">Shard 0</option>
            <option value="1">Shard 1</option>
            <option value="2">Shard 2</option>
          </select>
        </div>
      </div>
    </div>
  );
}
