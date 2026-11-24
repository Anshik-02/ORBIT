'use client';

import React from 'react';
import {
  Flame,
  CheckCircle2,
  Clock,
  Crown,
  ShieldCheck,
  AlertTriangle,
} from 'lucide-react';

export interface ChaosTimelineStep {
  id: string;
  title: string;
  desc: string;
  status: 'PENDING' | 'ACTIVE' | 'DONE' | 'FAILED';
  timestamp?: string;
}

interface ChaosExecutionTimelineProps {
  steps: ChaosTimelineStep[];
  currentStepIndex: number;
}

export default function ChaosExecutionTimeline({
  steps,
  currentStepIndex,
}: ChaosExecutionTimelineProps) {
  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-5 font-mono text-xs">
      <div className="flex items-center justify-between pb-3 border-b border-[#DCE5F0]">
        <div>
          <h3 className="text-base font-bold text-[#17243A]">
            Chaos Lifecycle & Fault Recovery Timeline
          </h3>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Stage-by-stage progression from failure injection through automated consensus healing
          </p>
        </div>

        <span className="text-[11px] text-[#0891D5] font-semibold px-3 py-1 rounded-xl bg-blue-50 border border-blue-200">
          State Machine Verified
        </span>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-7 gap-2">
        {steps.map((st, i) => {
          const isDone = currentStepIndex > i;
          const isActive = currentStepIndex === i;

          return (
            <div
              key={st.id}
              className={`p-3 rounded-xl border transition-all flex flex-col justify-between ${
                isActive
                  ? 'bg-rose-50 border-rose-300 ring-2 ring-rose-200 shadow-xs'
                  : isDone
                  ? 'bg-emerald-50/40 border-emerald-200'
                  : 'bg-[#F8FAFC] border-[#E2E8F0]'
              }`}
            >
              <div>
                <div className="flex items-center justify-between mb-1.5">
                  <span
                    className={`text-[9px] font-bold ${
                      isActive
                        ? 'text-rose-600'
                        : isDone
                        ? 'text-[#10B981]'
                        : 'text-[#64748B]'
                    }`}
                  >
                    0{i + 1}. {st.title}
                  </span>
                  {st.status === 'FAILED' ? (
                    <AlertTriangle className="w-3.5 h-3.5 text-rose-600" />
                  ) : isDone ? (
                    <CheckCircle2 className="w-3.5 h-3.5 text-[#10B981]" />
                  ) : isActive ? (
                    <span className="w-2 h-2 rounded-full bg-rose-500 animate-ping" />
                  ) : (
                    <span className="w-2 h-2 rounded-full bg-slate-300" />
                  )}
                </div>

                <p className="text-[10px] text-[#64748B] leading-tight">
                  {st.desc}
                </p>
              </div>

              {st.timestamp && (
                <div className="mt-2 pt-1 border-t border-[#E2E8F0] text-[9px] text-[#94A3B8]">
                  {st.timestamp}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
