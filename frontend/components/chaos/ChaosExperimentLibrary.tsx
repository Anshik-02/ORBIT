'use client';

import React from 'react';
import {
  Flame,
  Power,
  Crown,
  Network,
  RotateCcw,
  ZapOff,
  Clock,
  HardDrive,
  ShieldAlert,
  CheckCircle2,
} from 'lucide-react';

export interface ChaosExperimentDef {
  id: string;
  name: string;
  desc: string;
  icon: any;
  category: 'CONSENSUS' | 'NETWORK' | 'NODE' | 'STORAGE';
  durationSec: number;
  expectedOutcome: string;
  safetyCheck: string;
}

export const CHAOS_EXPERIMENTS: ChaosExperimentDef[] = [
  {
    id: 'leader-kill',
    name: 'Leader Kill & Re-Election',
    desc: 'Terminates the active Raft leader process to test heartbeat loss detection and election timeout.',
    icon: Crown,
    category: 'CONSENSUS',
    durationSec: 8,
    expectedOutcome: 'New leader elected in < 300ms. Term increments by 1.',
    safetyCheck: 'Ensures at least 2 other healthy replicas exist for quorum.',
  },
  {
    id: 'follower-crash',
    name: 'Follower Crash Simulation',
    desc: 'Abruptly stops a follower node while writes are actively ongoing.',
    icon: Power,
    category: 'NODE',
    durationSec: 5,
    expectedOutcome: 'Zero write interruption. Leader and 2nd follower maintain majority quorum (2/3).',
    safetyCheck: 'Verifies leader and remaining follower are fully synchronized.',
  },
  {
    id: 'network-partition',
    name: 'Split-Brain Network Partition',
    desc: 'Isolates 2 nodes into a minority island and 3 nodes into a majority island.',
    icon: Network,
    category: 'NETWORK',
    durationSec: 10,
    expectedOutcome: 'Majority continues accepting writes; minority safely rejects or queues.',
    safetyCheck: 'Requires at least 5 participating nodes to form 3 vs 2 partition.',
  },
  {
    id: 'packet-loss',
    name: 'Raft Packet Drop (25%)',
    desc: 'Simulates unreliable network medium with dropped AppendEntries RPCs.',
    icon: ZapOff,
    category: 'NETWORK',
    durationSec: 8,
    expectedOutcome: 'Raft bounded retries prevent split-brain. Slight latency increase.',
    safetyCheck: 'Only applies to peer-to-peer transport, not client gateway.',
  },
  {
    id: 'network-latency',
    name: 'Heartbeat Delay Injection (120ms)',
    desc: 'Adds artificial network delay to test ReadIndex lease safety margins.',
    icon: Clock,
    category: 'NETWORK',
    durationSec: 8,
    expectedOutcome: 'Lease verification falls back to explicit heartbeat exchange. No stale reads.',
    safetyCheck: 'Delay kept below election timeout floor (150ms).',
  },
  {
    id: 'wal-recovery',
    name: 'Sudden Crash & WAL Recovery',
    desc: 'Forces immediate process exit without graceful flush, then reboots node to test WAL replay.',
    icon: HardDrive,
    category: 'STORAGE',
    durationSec: 12,
    expectedOutcome: 'LSM engine replays uncommitted WAL records from disk. Zero data loss.',
    safetyCheck: 'Verifies strict fsync checkpoint on disk.',
  },
];

interface ChaosExperimentLibraryProps {
  selectedExpId: string;
  onSelect: (id: string) => void;
  isRunning: boolean;
}

export default function ChaosExperimentLibrary({
  selectedExpId,
  onSelect,
  isRunning,
}: ChaosExperimentLibraryProps) {
  return (
    <div className="space-y-3 font-mono text-xs">
      <div className="flex items-center justify-between px-1">
        <span className="text-[11px] font-semibold text-[#64748B] uppercase tracking-wider">
          CHAOS EXPERIMENT CATALOG ({CHAOS_EXPERIMENTS.length})
        </span>
        <span className="text-[#64748B]">Click an experiment to configure & test</span>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {CHAOS_EXPERIMENTS.map((exp) => {
          const isSelected = selectedExpId === exp.id;
          const Icon = exp.icon;

          return (
            <div
              key={exp.id}
              onClick={() => !isRunning && onSelect(exp.id)}
              className={`p-5 rounded-2xl border transition-all cursor-pointer flex flex-col justify-between ${
                isSelected
                  ? 'bg-white border-[#0891D5] shadow-md ring-2 ring-[#0891D5]/20'
                  : 'bg-white border-[#DCE5F0] hover:border-[#CBD5E1] shadow-xs hover:shadow-md'
              } ${isRunning ? 'opacity-60 cursor-not-allowed' : ''}`}
            >
              <div>
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <span
                      className={`p-2 rounded-xl ${
                        isSelected
                          ? 'bg-[#0891D5] text-white'
                          : 'bg-[#F4F7FC] text-[#0891D5]'
                      }`}
                    >
                      <Icon className="w-4 h-4" />
                    </span>
                    <h4 className="font-bold text-[#17243A] text-sm">
                      {exp.name}
                    </h4>
                  </div>
                  <span className="text-[10px] px-2 py-0.5 rounded-full bg-[#F4F7FC] border border-[#DCE5F0] text-[#64748B] font-bold">
                    {exp.category}
                  </span>
                </div>

                <p className="text-[11px] text-[#64748B] leading-relaxed mb-3">
                  {exp.desc}
                </p>
              </div>

              <div className="pt-3 border-t border-[#F1F5F9] space-y-1.5 text-[10px]">
                <div className="text-[#10B981] font-semibold flex items-center gap-1">
                  <CheckCircle2 className="w-3 h-3 shrink-0" />
                  <span className="truncate">{exp.expectedOutcome}</span>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
