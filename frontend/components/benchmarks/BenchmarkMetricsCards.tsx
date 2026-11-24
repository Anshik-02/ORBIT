'use client';

import React from 'react';
import { Zap, Clock, ShieldCheck, AlertCircle, TrendingUp } from 'lucide-react';

export interface BenchmarkMetrics {
  qps: number;
  avgLatencyMs: number;
  p50Ms: number;
  p95Ms: number;
  p99Ms: number;
  maxLatencyMs: number;
  totalRequests: number;
  errorCount: number;
}

interface BenchmarkMetricsCardsProps {
  metrics: BenchmarkMetrics;
}

export default function BenchmarkMetricsCards({ metrics }: BenchmarkMetricsCardsProps) {
  const errorRate =
    metrics.totalRequests > 0
      ? ((metrics.errorCount / metrics.totalRequests) * 100).toFixed(2)
      : '0.00';

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 font-mono">
      {/* 01. THROUGHPUT QPS */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>PEAK THROUGHPUT</span>
            <span className="p-1.5 rounded-lg bg-[#0891D5]/10 text-[#0891D5]">
              <Zap className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#17243A]">
            {metrics.qps.toLocaleString()}{' '}
            <span className="text-sm font-normal text-[#64748B]">QPS</span>
          </div>
        </div>
        <div className="h-4 w-full my-1">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 16 Q 30 6, 60 12 T 100 4"
              fill="none"
              stroke="#0891D5"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B]">
          {metrics.totalRequests.toLocaleString()} total operations executed
        </p>
      </div>

      {/* 02. P95 LATENCY */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>P95 LATENCY</span>
            <span className="p-1.5 rounded-lg bg-[#10B981]/10 text-[#10B981]">
              <Clock className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#10B981]">
            {metrics.p95Ms.toFixed(2)}{' '}
            <span className="text-sm font-normal text-[#64748B]">ms</span>
          </div>
        </div>
        <div className="h-4 w-full my-1">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 12 Q 30 14, 60 8 T 100 6"
              fill="none"
              stroke="#10B981"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B]">
          P50: {metrics.p50Ms.toFixed(2)}ms • Avg: {metrics.avgLatencyMs.toFixed(2)}ms
        </p>
      </div>

      {/* 03. P99 LATENCY */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>P99 LATENCY TAIL</span>
            <span className="p-1.5 rounded-lg bg-[#7965E8]/10 text-[#7965E8]">
              <TrendingUp className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#7965E8]">
            {metrics.p99Ms.toFixed(2)}{' '}
            <span className="text-sm font-normal text-[#64748B]">ms</span>
          </div>
        </div>
        <div className="h-4 w-full my-1">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 14 Q 40 10, 75 16 T 100 8"
              fill="none"
              stroke="#7965E8"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#64748B]">
          Max observed: {metrics.maxLatencyMs.toFixed(2)}ms
        </p>
      </div>

      {/* 04. ERROR RATE */}
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between text-xs text-[#64748B] mb-2">
            <span>QUORUM ERROR RATE</span>
            <span className="p-1.5 rounded-lg bg-emerald-50 text-[#10B981]">
              <ShieldCheck className="w-4 h-4" />
            </span>
          </div>
          <div className="text-3xl font-bold text-[#10B981]">
            {errorRate}%
          </div>
        </div>
        <div className="h-4 w-full my-1">
          <svg className="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 100 20">
            <path
              d="M 0 10 L 100 10"
              fill="none"
              stroke="#10B981"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </div>
        <p className="text-[11px] text-[#10B981] font-semibold">
          {metrics.errorCount} consensus or transport errors
        </p>
      </div>
    </div>
  );
}
