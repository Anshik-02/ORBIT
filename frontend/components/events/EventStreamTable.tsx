'use client';

import React, { useState } from 'react';
import {
  Clock,
  Eye,
  X,
  AlertTriangle,
  AlertCircle,
  Info,
  CheckCircle2,
  Copy,
  Check,
} from 'lucide-react';
import { EventCategory, EventSeverity } from './EventFilterBar';

export interface ClusterEventItem {
  id: string;
  timestamp: string;
  category: EventCategory;
  severity: 'INFO' | 'WARN' | 'ERROR' | 'CRITICAL';
  nodeId: string;
  shardId?: number;
  description: string;
  metadata?: any;
}

interface EventStreamTableProps {
  events: ClusterEventItem[];
}

export default function EventStreamTable({ events }: EventStreamTableProps) {
  const [selectedEvent, setSelectedEvent] = useState<ClusterEventItem | null>(null);
  const [copied, setCopied] = useState(false);

  const getSeverityBadge = (s: ClusterEventItem['severity']) => {
    switch (s) {
      case 'CRITICAL':
        return 'bg-rose-100 text-rose-700 border-rose-300 font-bold';
      case 'ERROR':
        return 'bg-rose-50 text-rose-600 border-rose-200 font-bold';
      case 'WARN':
        return 'bg-amber-50 text-[#D97706] border-amber-200 font-semibold';
      case 'INFO':
      default:
        return 'bg-blue-50 text-[#0891D5] border-blue-200';
    }
  };

  const getCategoryBadge = (c: EventCategory) => {
    switch (c) {
      case 'RAFT':
        return 'bg-amber-50/50 text-[#D97706] border-[#F59E0B]/30';
      case 'STORAGE':
        return 'bg-purple-50/50 text-[#7965E8] border-[#7965E8]/30';
      case 'CLUSTER':
        return 'bg-blue-50/50 text-[#0891D5] border-blue-200';
      case 'NETWORK':
        return 'bg-emerald-50/50 text-[#10B981] border-emerald-200';
      default:
        return 'bg-slate-100 text-[#64748B] border-slate-200';
    }
  };

  const handleCopyJson = () => {
    if (!selectedEvent) return;
    navigator.clipboard.writeText(JSON.stringify(selectedEvent, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      <div className="flex items-center justify-between pb-3 border-b border-[#DCE5F0]">
        <div>
          <h3 className="text-base font-bold text-[#17243A]">
            Real-Time Cluster Event Stream
          </h3>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Streaming live raft state transitions, membership updates and WAL checkpoints
          </p>
        </div>

        <span className="text-[11px] text-[#64748B] px-3 py-1 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          Showing {events.length} Events
        </span>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase bg-[#F8FAFC]">
              <th className="py-2.5 px-3">Time</th>
              <th className="py-2.5 px-3">Severity</th>
              <th className="py-2.5 px-3">Category</th>
              <th className="py-2.5 px-3">Node</th>
              <th className="py-2.5 px-3">Shard</th>
              <th className="py-2.5 px-3">Description</th>
              <th className="py-2.5 px-3 text-right">Inspect</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9]">
            {events.length === 0 ? (
              <tr>
                <td colSpan={7} className="py-8 text-center text-[#64748B]">
                  No events found matching the active filters.
                </td>
              </tr>
            ) : (
              events.map((evt) => (
                <tr
                  key={evt.id}
                  onClick={() => setSelectedEvent(evt)}
                  className="hover:bg-[#F8FAFC] cursor-pointer transition-colors"
                >
                  <td className="py-2.5 px-3 text-[#64748B] whitespace-nowrap">
                    {evt.timestamp}
                  </td>
                  <td className="py-2.5 px-3">
                    <span
                      className={`px-2 py-0.5 rounded-full border text-[10px] ${getSeverityBadge(
                        evt.severity
                      )}`}
                    >
                      {evt.severity}
                    </span>
                  </td>
                  <td className="py-2.5 px-3">
                    <span
                      className={`px-2 py-0.5 rounded-md border text-[10px] font-semibold ${getCategoryBadge(
                        evt.category
                      )}`}
                    >
                      {evt.category}
                    </span>
                  </td>
                  <td className="py-2.5 px-3 font-bold text-[#17243A]">
                    {evt.nodeId.toUpperCase()}
                  </td>
                  <td className="py-2.5 px-3 text-[#64748B]">
                    {evt.shardId !== undefined ? `Shard ${evt.shardId}` : '—'}
                  </td>
                  <td className="py-2.5 px-3 text-[#17243A] max-w-md truncate">
                    {evt.description}
                  </td>
                  <td className="py-2.5 px-3 text-right">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        setSelectedEvent(evt);
                      }}
                      className="p-1.5 rounded-lg bg-white hover:bg-slate-100 border border-[#DCE5F0] text-[#0891D5]"
                      title="Inspect Event Payload"
                    >
                      <Eye className="w-3.5 h-3.5" />
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* EVENT DETAIL MODAL */}
      {selectedEvent && (
        <div className="fixed inset-0 z-50 bg-black/40 backdrop-blur-xs flex items-center justify-center p-4">
          <div className="bg-white border border-[#DCE5F0] rounded-2xl w-full max-w-xl p-6 shadow-2xl space-y-4 font-mono animate-fadeIn">
            <div className="flex items-center justify-between pb-3 border-b border-[#DCE5F0]">
              <div className="flex items-center gap-2">
                <span
                  className={`px-2 py-0.5 rounded-full border text-[10px] ${getSeverityBadge(
                    selectedEvent.severity
                  )}`}
                >
                  {selectedEvent.severity}
                </span>
                <h4 className="text-sm font-bold text-[#17243A]">
                  Event ID: {selectedEvent.id}
                </h4>
              </div>

              <div className="flex items-center gap-2">
                <button
                  onClick={handleCopyJson}
                  className="p-1.5 rounded-lg hover:bg-slate-100 border border-[#DCE5F0] text-[#64748B]"
                  title="Copy JSON"
                >
                  {copied ? <Check className="w-3.5 h-3.5 text-[#10B981]" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
                <button
                  onClick={() => setSelectedEvent(null)}
                  className="p-1.5 rounded-lg text-[#64748B] hover:text-[#17243A]"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
            </div>

            <div className="space-y-2 text-xs">
              <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] space-y-1.5">
                <div className="flex justify-between">
                  <span className="text-[#64748B]">Timestamp:</span>
                  <span className="font-semibold text-[#17243A]">{selectedEvent.timestamp}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-[#64748B]">Category:</span>
                  <span className="font-semibold text-[#17243A]">{selectedEvent.category}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-[#64748B]">Originating Node:</span>
                  <span className="font-semibold text-[#0891D5]">{selectedEvent.nodeId.toUpperCase()}</span>
                </div>
                {selectedEvent.shardId !== undefined && (
                  <div className="flex justify-between">
                    <span className="text-[#64748B]">Shard Partition:</span>
                    <span className="font-semibold text-[#7965E8]">Shard {selectedEvent.shardId}</span>
                  </div>
                )}
                <div className="pt-1 border-t border-[#E2E8F0]">
                  <span className="text-[#64748B] block mb-0.5">Description:</span>
                  <p className="text-[#17243A] leading-relaxed">{selectedEvent.description}</p>
                </div>
              </div>

              {selectedEvent.metadata && (
                <div>
                  <span className="text-[11px] font-semibold text-[#64748B] uppercase block mb-1">
                    METADATA PAYLOAD
                  </span>
                  <pre className="p-3 rounded-xl bg-[#0F172A] text-slate-100 text-xs overflow-x-auto max-h-[200px]">
                    <code>{JSON.stringify(selectedEvent.metadata, null, 2)}</code>
                  </pre>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
