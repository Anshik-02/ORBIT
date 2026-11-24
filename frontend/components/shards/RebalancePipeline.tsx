'use client';

import React, { useState } from 'react';
import {
  GitCommit,
  ArrowRight,
  Play,
  RotateCcw,
  CheckCircle2,
  Clock,
  Layers,
  Database,
  ShieldCheck,
  Send,
} from 'lucide-react';

export type MigrationStage =
  | 'idle'
  | 'planned'
  | 'snapshot_transfer'
  | 'catching_up'
  | 'placement_commit'
  | 'completed';

export default function RebalancePipeline() {
  const [stage, setStage] = useState<MigrationStage>('idle');
  const [progress, setProgress] = useState(0);

  const stages = [
    {
      id: 'planned',
      title: '01. PLANNED',
      desc: 'Calculate optimal placement based on node storage load & CPU',
      badge: 'Epoch Verified',
    },
    {
      id: 'snapshot_transfer',
      title: '02. SNAPSHOT TRANSFER',
      desc: 'Stream baseline SSTable snapshot chunk-by-chunk to target peer',
      badge: 'Zero Write Lock',
    },
    {
      id: 'catching_up',
      title: '03. CATCHING UP',
      desc: 'Replay delta Raft log entries accumulated during snapshot transfer',
      badge: 'Log Streaming',
    },
    {
      id: 'placement_commit',
      title: '04. PLACEMENT COMMIT',
      desc: 'Propose and commit ConfChange membership change via Raft quorum',
      badge: 'Joint Consensus',
    },
    {
      id: 'completed',
      title: '05. COMPLETED',
      desc: 'Target node is active replica; decommission source replica cleanly',
      badge: 'Quorum Active',
    },
  ];

  const handleStartSimulation = () => {
    setStage('planned');
    setProgress(15);

    setTimeout(() => {
      setStage('snapshot_transfer');
      setProgress(40);
    }, 1200);

    setTimeout(() => {
      setStage('catching_up');
      setProgress(68);
    }, 2400);

    setTimeout(() => {
      setStage('placement_commit');
      setProgress(88);
    }, 3600);

    setTimeout(() => {
      setStage('completed');
      setProgress(100);
    }, 4800);
  };

  const handleReset = () => {
    setStage('idle');
    setProgress(0);
  };

  const getStepIndex = (s: MigrationStage) => {
    switch (s) {
      case 'planned':
        return 0;
      case 'snapshot_transfer':
        return 1;
      case 'catching_up':
        return 2;
      case 'placement_commit':
        return 3;
      case 'completed':
        return 4;
      default:
        return -1;
    }
  };

  const currentIdx = getStepIndex(stage);

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h3 className="text-base font-bold text-[#17243A] font-mono">
              Live Shard Migration & Rebalancing Pipeline
            </h3>
            <span className="px-2 py-0.5 rounded-full bg-blue-50 border border-blue-200 text-[#0891D5] font-mono text-[10px] font-bold">
              ZERO-DOWNTIME
            </span>
          </div>
          <p className="text-xs text-[#64748B] font-mono mt-0.5">
            5-phase state machine governing replica movements across physical machines
          </p>
        </div>

        <div className="flex items-center gap-2">
          {stage === 'idle' ? (
            <button
              onClick={handleStartSimulation}
              className="px-3.5 py-1.5 rounded-xl bg-[#0891D5] hover:bg-[#0284C7] text-white font-mono text-xs font-semibold flex items-center gap-1.5 transition-all shadow-xs"
            >
              <Play className="w-3.5 h-3.5 fill-current" />
              <span>Simulate Rebalance</span>
            </button>
          ) : (
            <button
              onClick={handleReset}
              className="px-3.5 py-1.5 rounded-xl bg-[#F4F7FC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#17243A] font-mono text-xs font-semibold flex items-center gap-1.5 transition-all shadow-2xs"
            >
              <RotateCcw className="w-3.5 h-3.5" />
              <span>Reset Pipeline</span>
            </button>
          )}
        </div>
      </div>

      {/* Progress Bar */}
      <div className="space-y-1.5 font-mono text-xs">
        <div className="flex items-center justify-between text-[#64748B] text-[11px]">
          <span>Migration Lifecycle Progress</span>
          <span className="font-semibold text-[#17243A]">{progress}%</span>
        </div>
        <div className="w-full h-2 rounded-full bg-[#F1F5F9] overflow-hidden">
          <div
            className="h-full bg-gradient-to-r from-[#0891D5] to-[#7965E8] transition-all duration-500 rounded-full"
            style={{ width: `${progress}%` }}
          />
        </div>
      </div>

      {/* 5 Stages Flow */}
      <div className="grid grid-cols-1 md:grid-cols-5 gap-3">
        {stages.map((st, i) => {
          const isDone = currentIdx > i || stage === 'completed';
          const isCurrent = currentIdx === i;

          return (
            <div
              key={st.id}
              className={`p-3.5 rounded-xl border font-mono transition-all flex flex-col justify-between ${
                isCurrent
                  ? 'bg-blue-50/50 border-[#0891D5] shadow-xs ring-1 ring-[#0891D5]/30'
                  : isDone
                  ? 'bg-emerald-50/30 border-emerald-200'
                  : 'bg-[#F8FAFC] border-[#E2E8F0]'
              }`}
            >
              <div>
                <div className="flex items-center justify-between mb-2">
                  <span
                    className={`text-[10px] font-bold ${
                      isCurrent
                        ? 'text-[#0891D5]'
                        : isDone
                        ? 'text-[#10B981]'
                        : 'text-[#64748B]'
                    }`}
                  >
                    {st.title}
                  </span>
                  {isDone ? (
                    <CheckCircle2 className="w-3.5 h-3.5 text-[#10B981]" />
                  ) : isCurrent ? (
                    <span className="w-2 h-2 rounded-full bg-[#0891D5] animate-ping" />
                  ) : (
                    <span className="w-2 h-2 rounded-full bg-slate-300" />
                  )}
                </div>
                <p className="text-[11px] text-[#64748B] leading-snug">
                  {st.desc}
                </p>
              </div>

              <div className="mt-3 pt-2 border-t border-[#E2E8F0]">
                <span
                  className={`text-[9px] px-1.5 py-0.5 rounded font-semibold ${
                    isCurrent
                      ? 'bg-blue-100 text-[#0891D5]'
                      : isDone
                      ? 'bg-emerald-100 text-[#10B981]'
                      : 'bg-slate-200/60 text-[#64748B]'
                  }`}
                >
                  {st.badge}
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
