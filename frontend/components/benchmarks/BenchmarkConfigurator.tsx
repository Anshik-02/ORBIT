'use client';

import React, { useState, useEffect } from 'react';
import { Play, Square, Settings, Zap, Clock, Users, Database, HardDrive } from 'lucide-react';

export interface BenchmarkConfig {
  workload: 'read_heavy' | 'write_heavy' | 'balanced' | 'pure_read';
  durationSec: number;
  concurrency: number;
  keyCount: number;
  payloadSize: '64B' | '256B' | '1KB';
}

interface BenchmarkConfiguratorProps {
  onStart: (config: BenchmarkConfig) => void;
  onStop: () => void;
  isRunning: boolean;
  progressPct: number;
  elapsedSec: number;
}

export default function BenchmarkConfigurator({
  onStart,
  onStop,
  isRunning,
  progressPct,
  elapsedSec,
}: BenchmarkConfiguratorProps) {
  const [workload, setWorkload] = useState<BenchmarkConfig['workload']>('read_heavy');
  const [durationSec, setDurationSec] = useState(15);
  const [concurrency, setConcurrency] = useState(50);
  const [keyCount, setKeyCount] = useState(25000);
  const [payloadSize, setPayloadSize] = useState<BenchmarkConfig['payloadSize']>('256B');

  const handleStart = () => {
    onStart({
      workload,
      durationSec,
      concurrency,
      keyCount,
      payloadSize,
    });
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-5 font-mono text-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-3 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
              <Zap className="w-3.5 h-3.5 text-[#0891D5]" />
            </div>
            <h3 className="text-base font-bold text-[#17243A]">
              Load Generator & Benchmark Harness
            </h3>
          </div>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Measure multi-raft write saturation, consensus latency percentiles and QPS
          </p>
        </div>

        <div className="flex items-center gap-2">
          {isRunning ? (
            <button
              onClick={onStop}
              className="px-4 py-2 rounded-xl bg-rose-600 hover:bg-rose-700 text-white font-bold flex items-center gap-2 transition-all shadow-xs"
            >
              <Square className="w-3.5 h-3.5 fill-current" />
              <span>Abort Benchmark ({durationSec - elapsedSec}s)</span>
            </button>
          ) : (
            <button
              onClick={handleStart}
              className="px-5 py-2 rounded-xl bg-[#0891D5] hover:bg-[#0284C7] text-white font-bold flex items-center gap-2 transition-all shadow-xs"
            >
              <Play className="w-3.5 h-3.5 fill-current" />
              <span>Start Benchmark Run</span>
            </button>
          )}
        </div>
      </div>

      {/* Progress Bar when running */}
      {isRunning && (
        <div className="space-y-1.5 p-3 rounded-xl bg-blue-50/50 border border-blue-100">
          <div className="flex justify-between text-[#0891D5] font-semibold text-[11px]">
            <span>Benchmark in progress: {elapsedSec}s / {durationSec}s</span>
            <span>{progressPct}%</span>
          </div>
          <div className="w-full h-2 rounded-full bg-blue-100 overflow-hidden">
            <div
              className="h-full bg-[#0891D5] rounded-full transition-all duration-300"
              style={{ width: `${progressPct}%` }}
            />
          </div>
        </div>
      )}

      {/* Configuration Controls Grid */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        {/* Workload Profile */}
        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
            WORKLOAD PROFILE
          </label>
          <select
            disabled={isRunning}
            value={workload}
            onChange={(e) => setWorkload(e.target.value as any)}
            className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
          >
            <option value="read_heavy">Read Heavy (80% / 20%)</option>
            <option value="write_heavy">Write Heavy (20% / 80%)</option>
            <option value="balanced">Balanced (50% / 50%)</option>
            <option value="pure_read">Pure Read (100% / 0%)</option>
          </select>
        </div>

        {/* Concurrency */}
        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
            CONCURRENCY (THREADS)
          </label>
          <select
            disabled={isRunning}
            value={concurrency}
            onChange={(e) => setConcurrency(parseInt(e.target.value, 10))}
            className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
          >
            <option value={10}>10 Clients</option>
            <option value={50}>50 Concurrent Clients</option>
            <option value={100}>100 Clients (High)</option>
            <option value={250}>250 Clients (Extreme)</option>
          </select>
        </div>

        {/* Duration */}
        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
            DURATION
          </label>
          <select
            disabled={isRunning}
            value={durationSec}
            onChange={(e) => setDurationSec(parseInt(e.target.value, 10))}
            className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
          >
            <option value={10}>10 Seconds (Quick)</option>
            <option value={15}>15 Seconds</option>
            <option value={30}>30 Seconds</option>
            <option value={60}>60 Seconds (Stress)</option>
          </select>
        </div>

        {/* Key Universe */}
        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
            KEY UNIVERSE
          </label>
          <select
            disabled={isRunning}
            value={keyCount}
            onChange={(e) => setKeyCount(parseInt(e.target.value, 10))}
            className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
          >
            <option value={5000}>5,000 Keys (Hotspot)</option>
            <option value={25000}>25,000 Keys</option>
            <option value={100000}>100,000 Keys (Uniform)</option>
          </select>
        </div>

        {/* Payload Size */}
        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
            PAYLOAD SIZE
          </label>
          <select
            disabled={isRunning}
            value={payloadSize}
            onChange={(e) => setPayloadSize(e.target.value as any)}
            className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
          >
            <option value="64B">64 Bytes (Tiny)</option>
            <option value="256B">256 Bytes (Standard)</option>
            <option value="1KB">1 Kilobyte (Large)</option>
          </select>
        </div>
      </div>
    </div>
  );
}
