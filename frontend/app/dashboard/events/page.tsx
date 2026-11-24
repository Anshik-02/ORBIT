'use client';

import React, { useState, useMemo } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { Radio, RefreshCw, CheckCircle2 } from 'lucide-react';
import EventSummaryCards from '@/components/events/EventSummaryCards';
import EventFilterBar, { EventCategory, EventSeverity } from '@/components/events/EventFilterBar';
import EventStreamTable, { ClusterEventItem } from '@/components/events/EventStreamTable';
import EventTimelineView from '@/components/events/EventTimelineView';
import { useEventStream } from '@/hooks/useEventStream';
import LiveIndicator from '@/components/shared/LiveIndicator';

export default function EventsPage() {
  const {
    events,
    isPaused,
    setIsPaused,
    isConnected,
    exportJSON,
    exportCSV,
  } = useEventStream();
  const [search, setSearch] = useState('');
  const [severity, setSeverity] = useState<EventSeverity>('ALL');
  const [category, setCategory] = useState<EventCategory>('ALL');
  const [selectedNode, setSelectedNode] = useState('ALL');
  const [selectedShard, setSelectedShard] = useState('ALL');

  const filteredEvents = useMemo(() => {
    return events.filter((evt) => {
      // Search
      const matchesSearch =
        evt.description.toLowerCase().includes(search.toLowerCase()) ||
        evt.id.toLowerCase().includes(search.toLowerCase()) ||
        evt.nodeId.toLowerCase().includes(search.toLowerCase());

      // Severity
      const matchesSeverity =
        severity === 'ALL' || evt.severity === severity;

      // Category
      const matchesCategory =
        category === 'ALL' || evt.category === category;

      // Node
      const matchesNode =
        selectedNode === 'ALL' || evt.nodeId.toLowerCase() === selectedNode.toLowerCase();

      // Shard
      const matchesShard =
        selectedShard === 'ALL' ||
        (evt.shardId !== undefined && evt.shardId.toString() === selectedShard);

      return matchesSearch && matchesSeverity && matchesCategory && matchesNode && matchesShard;
    });
  }, [events, search, severity, category, selectedNode, selectedShard]);

  const handleExportJson = () => {
    const blob = new Blob([JSON.stringify(filteredEvents, null, 2)], {
      type: 'application/json',
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `orbit_events_${Date.now()}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleExportCsv = () => {
    const headers = ['id', 'timestamp', 'severity', 'category', 'nodeId', 'shardId', 'description'];
    const rows = filteredEvents.map((e) => [
      e.id,
      e.timestamp,
      e.severity,
      e.category,
      e.nodeId,
      e.shardId !== undefined ? e.shardId : '',
      `"${e.description.replace(/"/g, '""')}"`,
    ]);
    const csvContent = [headers.join(','), ...rows.map((r) => r.join(','))].join('\n');
    const blob = new Blob([csvContent], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `orbit_events_${Date.now()}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleResetFilters = () => {
    setSearch('');
    setSeverity('ALL');
    setCategory('ALL');
    setSelectedNode('ALL');
    setSelectedShard('ALL');
  };

  return (
    <DashboardLayout activeNav="events">
      <div className="space-y-6">
        {/* 01. PAGE HEADER */}
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-4 font-mono">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl sm:text-2xl font-bold text-[#17243A] tracking-tight">
                Cluster Event Stream & Observability
              </h1>
              <span className={`flex items-center gap-1.5 px-3 py-1 rounded-full font-mono text-xs font-semibold ${
                isConnected
                  ? 'bg-emerald-50 border border-emerald-200 text-[#10B981]'
                  : 'bg-amber-50 border border-amber-200 text-[#F59E0B]'
              }`}>
                <span className={`w-2 h-2 rounded-full ${isConnected ? 'bg-[#10B981] animate-ping' : 'bg-[#F59E0B]'}`} />
                {isConnected ? 'LIVE WEBSOCKET CONNECTED' : 'POLLING EVENT BUS'}
              </span>
            </div>
            <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal">
              High-resolution timeline of distributed events, Raft elections, and storage snapshots.
            </p>
          </div>

          <div className="flex items-center gap-2 text-xs">
            <span className="px-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A] font-semibold">
              WebSocket Channel /ws/events
            </span>
          </div>
        </div>

        {/* 02. SUMMARY CARDS */}
        <EventSummaryCards
          totalEvents={events.length + 1277}
          criticalEvents={0}
          warningEvents={1}
          isStreaming={!isPaused}
        />

        {/* 03. FILTER & EXPORT BAR */}
        <EventFilterBar
          search={search}
          onSearchChange={setSearch}
          severity={severity}
          onSeverityChange={setSeverity}
          category={category}
          onCategoryChange={setCategory}
          selectedNode={selectedNode}
          onNodeChange={setSelectedNode}
          selectedShard={selectedShard}
          onShardChange={setSelectedShard}
          isPaused={isPaused}
          onTogglePause={() => setIsPaused(!isPaused)}
          onExportJson={handleExportJson}
          onExportCsv={handleExportCsv}
          onResetFilters={handleResetFilters}
        />

        {/* 04. TIMELINE VIEW */}
        <EventTimelineView events={filteredEvents} />

        {/* 05. DETAILED EVENT STREAM TABLE */}
        <EventStreamTable events={filteredEvents} />
      </div>
    </DashboardLayout>
  );
}
