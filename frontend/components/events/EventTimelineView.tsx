'use client';

import React from 'react';
import {
  Clock,
  Crown,
  CheckCircle2,
  AlertTriangle,
  Server,
  Layers,
  ArrowRight,
} from 'lucide-react';
import { ClusterEventItem } from './EventStreamTable';

interface EventTimelineViewProps {
  events: ClusterEventItem[];
}

export default function EventTimelineView({ events }: EventTimelineViewProps) {
  const timelineEvents = events.slice(0, 6);

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      <div className="flex items-center justify-between pb-3 border-b border-[#DCE5F0]">
        <div>
          <h3 className="text-base font-bold text-[#17243A]">
            Cluster Milestone Timeline
          </h3>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Key consensus transitions & operational checkpoints
          </p>
        </div>
        <span className="text-xs text-[#0891D5] font-semibold flex items-center gap-1">
          <Clock className="w-3.5 h-3.5" />
          <span>Real-time Stream</span>
        </span>
      </div>

      <div className="relative pl-6 space-y-3 before:content-[''] before:absolute before:left-2.5 before:top-2 before:bottom-2 before:w-0.5 before:bg-[#E2E8F0]">
        {timelineEvents.map((evt) => (
          <div key={evt.id} className="relative group">
            <div className="absolute -left-6 top-1.5 w-4 h-4 rounded-full bg-white border-2 border-[#0891D5] shadow-xs group-hover:scale-125 transition-transform" />
            <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] hover:border-[#CBD5E1] transition-all flex flex-col sm:flex-row sm:items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <span className="text-[#64748B] whitespace-nowrap font-semibold">
                  {evt.timestamp}
                </span>
                <span className="w-1.5 h-1.5 rounded-full bg-slate-300" />
                <span className="font-bold text-[#17243A]">
                  {evt.nodeId.toUpperCase()}:
                </span>
                <span className="text-[#17243A]">{evt.description}</span>
              </div>
              <span className="text-[10px] px-2 py-0.5 rounded-md bg-white border border-[#E2E8F0] text-[#64748B] font-semibold self-start sm:self-auto">
                {evt.category}
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
