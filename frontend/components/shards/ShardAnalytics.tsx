'use client';

import React from 'react';
import { BarChart3, PieChart, ShieldAlert, CheckCircle2, Server } from 'lucide-react';

export default function ShardAnalytics() {
  const nodeLoad = [
    { name: 'Node A (:2525)', replicas: 2, pct: 66, color: '#0891D5', status: 'Optimal' },
    { name: 'Node B (:2526)', replicas: 2, pct: 66, color: '#0891D5', status: 'Optimal' },
    { name: 'Node C (:2527)', replicas: 2, pct: 66, color: '#0891D5', status: 'Optimal' },
    { name: 'Node D (:2528)', replicas: 2, pct: 66, color: '#0891D5', status: 'Optimal' },
    { name: 'Node E (:2529)', replicas: 1, pct: 33, color: '#7965E8', status: 'Headroom' },
  ];

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
      {/* 01. REPLICA LOAD PER PHYSICAL MACHINE (Cosmic Forge style progress bars) */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between mb-4">
            <div>
              <h3 className="text-base font-bold text-[#17243A] font-mono">
                Replica Distribution Across Nodes
              </h3>
              <p className="text-xs text-[#64748B] font-mono mt-0.5">
                Physical hardware utilization: 9 total replica slots across 5 nodes
              </p>
            </div>
            <span className="p-2 rounded-xl bg-blue-50 text-[#0891D5]">
              <Server className="w-4 h-4" />
            </span>
          </div>

          <div className="space-y-4 font-mono text-xs">
            {nodeLoad.map((item) => (
              <div key={item.name} className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <span className="font-semibold text-[#17243A]">{item.name}</span>
                  <div className="flex items-center gap-2">
                    <span className="text-[#64748B]">{item.replicas} of 3 Shards</span>
                    <span className="text-[10px] px-2 py-0.5 rounded-full bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A] font-bold">
                      {item.status}
                    </span>
                  </div>
                </div>

                <div className="w-full h-2.5 rounded-full bg-[#F1F5F9] overflow-hidden">
                  <div
                    className="h-full rounded-full transition-all duration-500"
                    style={{
                      width: `${item.pct}%`,
                      backgroundColor: item.color,
                    }}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="mt-5 pt-3 border-t border-[#E2E8F0] flex items-center justify-between font-mono text-xs text-[#64748B]">
          <span>Cluster Balance Score:</span>
          <span className="text-[#10B981] font-bold">96.4% Balanced</span>
        </div>
      </div>

      {/* 02. HASH RING COVERAGE & QUORUM FAULT TOLERANCE */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between mb-4">
            <div>
              <h3 className="text-base font-bold text-[#17243A] font-mono">
                Consistent Hash Ring & Quorum Availability
              </h3>
              <p className="text-xs text-[#64748B] font-mono mt-0.5">
                Mathematical key routing & fault tolerance boundaries
              </p>
            </div>
            <span className="p-2 rounded-xl bg-emerald-50 text-[#10B981]">
              <BarChart3 className="w-4 h-4" />
            </span>
          </div>

          <div className="space-y-4 font-mono text-xs">
            {/* Visual Hash Ring Segment Bar */}
            <div className="space-y-1.5">
              <span className="text-[11px] text-[#64748B] block font-semibold">
                64-BIT HASH SPACE PARTITION SLICES (0x00000000..0xFFFFFFFF)
              </span>
              <div className="w-full h-7 rounded-xl overflow-hidden flex border border-[#DCE5F0]">
                <div
                  className="h-full bg-[#0891D5] text-white font-bold flex items-center justify-center text-[10px]"
                  style={{ width: '33.3%' }}
                  title="Shard 0: 33.3%"
                >
                  Shard 0 (33.3%)
                </div>
                <div
                  className="h-full bg-[#7965E8] text-white font-bold flex items-center justify-center text-[10px]"
                  style={{ width: '33.3%' }}
                  title="Shard 1: 33.3%"
                >
                  Shard 1 (33.3%)
                </div>
                <div
                  className="h-full bg-[#10B981] text-white font-bold flex items-center justify-center text-[10px]"
                  style={{ width: '33.4%' }}
                  title="Shard 2: 33.4%"
                >
                  Shard 2 (33.4%)
                </div>
              </div>
            </div>

            {/* Quorum Safety Guarantee Box */}
            <div className="p-3.5 rounded-xl bg-emerald-50/60 border border-emerald-200/80 space-y-2">
              <div className="flex items-center gap-2 text-emerald-800 font-bold">
                <CheckCircle2 className="w-4 h-4 text-emerald-600 shrink-0" />
                <span>FAULT TOLERANCE GUARANTEE</span>
              </div>
              <p className="text-[11px] text-emerald-700 leading-relaxed font-mono">
                With Replication Factor RF=3, each shard requires a quorum of 2 alive nodes to accept reads and writes. The cluster can tolerate any single node crash or network partition with zero downtime.
              </p>
            </div>

            {/* Shard Routing Hash Function */}
            <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] flex items-center justify-between text-[11px]">
              <span className="text-[#64748B]">Hash Routing Algorithm:</span>
              <span className="text-[#17243A] font-bold">MurmurHash3 (64-bit Modulo 3)</span>
            </div>
          </div>
        </div>

        <div className="mt-5 pt-3 border-t border-[#E2E8F0] flex items-center justify-between font-mono text-xs text-[#64748B]">
          <span>Write Quorum Threshold:</span>
          <span className="text-[#0891D5] font-bold">Majority 2 of 3</span>
        </div>
      </div>
    </div>
  );
}
