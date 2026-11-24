'use client';

import React, { useState } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { Flame, Play, ShieldAlert, AlertTriangle, CheckCircle2 } from 'lucide-react';

export default function ChaosLabPage() {
  const [activeChaos, setActiveChaos] = useState<string | null>(null);

  const experiments = [
    {
      id: 'follower-crash',
      name: 'Follower Crash Simulation',
      desc: 'Abruptly terminates a non-leader follower to verify write quorum durability (RF=3).',
      duration: '5 seconds',
      impact: 'Zero data loss, 2/3 nodes satisfy quorum',
    },
    {
      id: 'leader-election',
      name: 'Leader Kill & Re-Election',
      desc: 'Sends SIGKILL to active shard leader to trigger election timeout and term increment.',
      duration: '8 seconds',
      impact: 'Transient 503 during election, new leader elected in < 300ms',
    },
    {
      id: 'network-partition',
      name: 'Split-Brain Network Partition',
      desc: 'Isolates 2 nodes from 3 nodes. Tests minority isolation safety.',
      duration: '10 seconds',
      impact: 'Majority partition commits; minority rejects writes',
    },
    {
      id: 'wal-corruption-recovery',
      name: 'WAL Corruption Recovery',
      desc: 'Injects truncated CRC into final WAL record to verify crash-recovery boundary check.',
      duration: '3 seconds',
      impact: 'Engine skips partial record and recovers all valid committed writes',
    },
  ];

  return (
    <DashboardLayout activeNav="chaos">
      <div className="space-y-6">
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs">
          <div className="flex items-center gap-2 mb-1">
            <div className="w-8 h-8 rounded-lg bg-rose-50 flex items-center justify-center text-rose-600">
              <Flame className="w-4 h-4" />
            </div>
            <h1 className="text-xl font-bold text-[#17243A]">Chaos Engineering & Fault Injection</h1>
          </div>
          <p className="text-xs text-[#64748B]">
            Execute deterministic fault-injection experiments to prove distributed safety and zero data loss
          </p>

          <div className="mt-6 grid grid-cols-1 md:grid-cols-2 gap-4">
            {experiments.map((exp) => (
              <div
                key={exp.id}
                className="p-5 rounded-2xl bg-[#F8FAFC] border border-[#E2E8F0] hover:border-[#CBD5E1] transition-all flex flex-col justify-between"
              >
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <h2 className="font-bold text-[#17243A] text-sm font-mono">
                      {exp.name}
                    </h2>
                    <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-slate-100 text-[#64748B]">
                      {exp.duration}
                    </span>
                  </div>
                  <p className="text-xs text-[#64748B] mb-3 leading-relaxed">
                    {exp.desc}
                  </p>
                  <div className="text-[11px] font-mono text-[#10B981] flex items-center gap-1.5 mb-4">
                    <CheckCircle2 className="w-3.5 h-3.5" />
                    <span>Safety: {exp.impact}</span>
                  </div>
                </div>

                <button
                  onClick={() => {
                    setActiveChaos(exp.id);
                    setTimeout(() => setActiveChaos(null), 3000);
                  }}
                  className={`w-full py-2 px-3 rounded-xl font-mono text-xs font-bold flex items-center justify-center gap-2 transition-all ${
                    activeChaos === exp.id
                      ? 'bg-rose-600 text-white animate-pulse'
                      : 'bg-white hover:bg-slate-100 border border-[#DCE5F0] text-[#17243A]'
                  }`}
                >
                  <Play className="w-3.5 h-3.5" />
                  <span>
                    {activeChaos === exp.id ? 'Injecting Fault...' : 'Run Experiment'}
                  </span>
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>
    </DashboardLayout>
  );
}
