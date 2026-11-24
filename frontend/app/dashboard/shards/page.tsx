'use client';

import React from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { Layers, ShieldCheck, Database, CheckCircle2, Crown, Activity } from 'lucide-react';
import { SHARD_CONFIG } from '@/lib/topology/layout';

export default function ShardsPage() {
  return (
    <DashboardLayout activeNav="shards">
      <div className="space-y-6">
        {/* Header */}
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl sm:text-2xl font-bold text-[#17243A] tracking-tight">
                Shards & Partitions
              </h1>
              <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-xs font-semibold">
                <CheckCircle2 className="w-3.5 h-3.5" />
                ALL QUORUMS SATISFIED
              </span>
            </div>
            <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal">
              Logical database partitions, replica placement (RF=3) and shard balancing.
            </p>
          </div>

          <div className="flex items-center gap-2 font-mono text-xs">
            <span className="px-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A] font-semibold">
              3 Logical Partitions • Consistent Hash Ring
            </span>
          </div>
        </div>

        {/* Shard Placement Grid */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {SHARD_CONFIG.map((sh) => (
            <div
              key={sh.shardId}
              className="bg-white border border-[#DCE5F0] hover:border-[#CBD5E1] rounded-2xl p-6 shadow-xs hover:shadow-md transition-all flex flex-col justify-between"
            >
              <div>
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <span
                      className="w-3 h-3 rounded-full"
                      style={{ backgroundColor: sh.color }}
                    />
                    <h2 className="text-base font-bold text-[#17243A] font-mono">
                      {sh.name}
                    </h2>
                  </div>
                  <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-bold">
                    HEALTHY (3/3)
                  </span>
                </div>

                {/* Leader Box */}
                <div className="p-3.5 rounded-xl bg-[#F8FAFC] border border-[#F59E0B]/30 mb-4 flex items-center justify-between font-mono">
                  <div>
                    <span className="text-[10px] text-[#64748B] uppercase block font-semibold">
                      Current Leader
                    </span>
                    <div className="flex items-center gap-1.5 mt-0.5">
                      <Crown className="w-4 h-4 text-[#F59E0B]" />
                      <span className="text-sm font-bold text-[#D97706]">
                        {sh.defaultLeader.toUpperCase()}
                      </span>
                    </div>
                  </div>
                  <div className="text-right">
                    <span className="text-[10px] text-[#64748B] uppercase block font-semibold">
                      Term
                    </span>
                    <span className="text-sm font-bold text-[#17243A]">
                      #{sh.term}
                    </span>
                  </div>
                </div>

                {/* Replicas */}
                <div className="space-y-2 font-mono text-xs mb-4">
                  <span className="text-[10px] uppercase text-[#64748B] font-semibold tracking-wider block">
                    Replicas (RF=3)
                  </span>
                  {sh.replicas.map((rep) => {
                    const isLeader = rep === sh.defaultLeader;
                    return (
                      <div
                        key={rep}
                        className="flex items-center justify-between p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]"
                      >
                        <div className="flex items-center gap-2">
                          <span
                            className={`w-2 h-2 rounded-full ${
                              isLeader ? 'bg-[#F59E0B]' : 'bg-[#10B981]'
                            }`}
                          />
                          <span className="font-semibold text-[#17243A]">
                            {rep.toUpperCase()}
                          </span>
                        </div>
                        <span
                          className={`text-[10px] px-2 py-0.5 rounded-full font-semibold ${
                            isLeader
                              ? 'bg-amber-50 text-[#D97706] border border-amber-200'
                              : 'bg-slate-100 text-[#64748B]'
                          }`}
                        >
                          {isLeader ? 'Leader' : 'Follower'}
                        </span>
                      </div>
                    );
                  })}
                </div>
              </div>

              {/* Bottom stats */}
              <div className="pt-3 border-t border-[#E2E8F0] flex items-center justify-between text-xs font-mono text-[#64748B]">
                <span>Commit Index:</span>
                <span className="text-[#10B981] font-bold">
                  {sh.commitIndex.toLocaleString()}
                </span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </DashboardLayout>
  );
}
