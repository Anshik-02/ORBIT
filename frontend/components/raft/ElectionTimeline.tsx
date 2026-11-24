'use client';

import React from 'react';
import {
  Vote,
  Crown,
  CheckCircle2,
  Clock,
  ArrowRight,
  ShieldCheck,
  AlertTriangle,
  Radio,
} from 'lucide-react';

export interface ElectionEvent {
  id: string;
  timestamp: string;
  type: 'election_started' | 'vote_granted' | 'leader_elected' | 'term_updated' | 'heartbeat_stable';
  nodeId: string;
  term: number;
  description: string;
}

interface ElectionTimelineProps {
  events: ElectionEvent[];
}

export default function ElectionTimeline({ events }: ElectionTimelineProps) {
  const getEventIcon = (type: ElectionEvent['type']) => {
    switch (type) {
      case 'leader_elected':
        return <Crown className="w-3.5 h-3.5 text-[#F59E0B]" />;
      case 'vote_granted':
        return <CheckCircle2 className="w-3.5 h-3.5 text-[#10B981]" />;
      case 'election_started':
        return <Vote className="w-3.5 h-3.5 text-[#0891D5]" />;
      case 'term_updated':
        return <ShieldCheck className="w-3.5 h-3.5 text-[#7965E8]" />;
      case 'heartbeat_stable':
      default:
        return <Radio className="w-3.5 h-3.5 text-[#64748B]" />;
    }
  };

  const getBadgeColor = (type: ElectionEvent['type']) => {
    switch (type) {
      case 'leader_elected':
        return 'bg-amber-50 text-[#D97706] border-amber-200';
      case 'vote_granted':
        return 'bg-emerald-50 text-[#10B981] border-emerald-200';
      case 'election_started':
        return 'bg-blue-50 text-[#0891D5] border-blue-200';
      case 'term_updated':
        return 'bg-purple-50 text-[#7965E8] border-purple-200';
      default:
        return 'bg-slate-100 text-[#64748B] border-slate-200';
    }
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4">
      <div className="flex items-center justify-between pb-3 border-b border-[#DCE5F0]">
        <div>
          <h3 className="text-base font-bold text-[#17243A] font-mono">
            Raft Election & Quorum Timeline
          </h3>
          <p className="text-xs text-[#64748B] font-mono mt-0.5">
            Chronological audit log of leader elections, term increments and vote grants
          </p>
        </div>
        <span className="text-xs font-mono text-[#64748B] px-2.5 py-1 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
          {events.length} Historical Records
        </span>
      </div>

      <div className="relative pl-6 space-y-4 before:content-[''] before:absolute before:left-2.5 before:top-2 before:bottom-2 before:w-0.5 before:bg-[#E2E8F0]">
        {events.map((evt) => (
          <div key={evt.id} className="relative font-mono text-xs group">
            {/* Dot marker */}
            <div className="absolute -left-6 top-1 w-5 h-5 rounded-full bg-white border border-[#DCE5F0] flex items-center justify-center shadow-2xs group-hover:border-[#0891D5] transition-colors">
              {getEventIcon(evt.type)}
            </div>

            <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] hover:border-[#CBD5E1] transition-all flex flex-col sm:flex-row sm:items-center justify-between gap-2">
              <div>
                <div className="flex items-center gap-2">
                  <span
                    className={`px-2 py-0.5 rounded-full border text-[10px] font-bold ${getBadgeColor(
                      evt.type
                    )}`}
                  >
                    {evt.type.toUpperCase().replace('_', ' ')}
                  </span>
                  <span className="font-bold text-[#17243A]">
                    {evt.nodeId.toUpperCase()}
                  </span>
                  <span className="text-[#64748B]">Term #{evt.term}</span>
                </div>
                <p className="text-[#64748B] text-[11px] mt-1">
                  {evt.description}
                </p>
              </div>

              <div className="flex items-center gap-1.5 text-[11px] text-[#94A3B8] self-start sm:self-auto shrink-0">
                <Clock className="w-3 h-3" />
                <span>{evt.timestamp}</span>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
