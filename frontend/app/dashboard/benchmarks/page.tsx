'use client';

import React from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import PerformanceCharts from '@/components/dashboard/PerformanceCharts';
import ClusterActivitySummary from '@/components/dashboard/ClusterActivitySummary';
import { Zap } from 'lucide-react';

export default function BenchmarksPage() {
  return (
    <DashboardLayout activeNav="benchmarks">
      <div className="space-y-6">
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <div className="w-8 h-8 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
                <Zap className="w-4 h-4 text-[#0891D5]" />
              </div>
              <h1 className="text-xl font-bold text-[#17243A]">Cluster Performance & Benchmarks</h1>
            </div>
            <p className="text-xs text-[#64748B]">
              Real-time throughput (RPS), percentiles (P50/P95/P99) and linearizable read/write latencies
            </p>
          </div>
        </div>

        <PerformanceCharts />
        <ClusterActivitySummary />
      </div>
    </DashboardLayout>
  );
}
