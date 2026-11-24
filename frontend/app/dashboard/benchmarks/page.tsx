'use client';

import React, { useState, useEffect, useRef } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { Zap, CheckCircle2, RefreshCw } from 'lucide-react';
import BenchmarkConfigurator, { BenchmarkConfig } from '@/components/benchmarks/BenchmarkConfigurator';
import BenchmarkMetricsCards, { BenchmarkMetrics } from '@/components/benchmarks/BenchmarkMetricsCards';
import BenchmarkCharts from '@/components/benchmarks/BenchmarkCharts';
import BenchmarkHistory, { BenchmarkRunRecord } from '@/components/benchmarks/BenchmarkHistory';
import { executeBenchmarkRun } from '@/lib/api/client';
import LiveIndicator from '@/components/shared/LiveIndicator';

const EMPTY_METRICS: BenchmarkMetrics = {
  qps: 0,
  avgLatencyMs: 0,
  p50Ms: 0,
  p95Ms: 0,
  p99Ms: 0,
  maxLatencyMs: 0,
  totalRequests: 0,
  errorCount: 0,
};

export default function BenchmarksPage() {
  const [metrics, setMetrics] = useState<BenchmarkMetrics>(EMPTY_METRICS);
  const [history, setHistory] = useState<BenchmarkRunRecord[]>([]);
  const [isRunning, setIsRunning] = useState(false);
  const [progressPct, setProgressPct] = useState(0);
  const [elapsedSec, setElapsedSec] = useState(0);
  const [throughputHistory, setThroughputHistory] = useState<number[]>([]);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const timerRef = useRef<NodeJS.Timeout | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  // Load persistent history from localStorage
  useEffect(() => {
    try {
      const saved = localStorage.getItem('orbit_benchmark_history');
      if (saved) {
        const parsed = JSON.parse(saved);
        if (Array.isArray(parsed) && parsed.length > 0) {
          setHistory(parsed);
          setMetrics(parsed[0].metrics);
          setThroughputHistory(parsed.map((r: BenchmarkRunRecord) => r.metrics.qps).reverse());
        }
      }
    } catch {}
  }, []);

  const handleStartBenchmark = async (config: BenchmarkConfig) => {
    setErrorMessage(null);
    setIsRunning(true);
    setProgressPct(0);
    setElapsedSec(0);

    abortControllerRef.current = new AbortController();

    const totalDuration = config.durationSec;
    let currentElapsed = 0;

    timerRef.current = setInterval(() => {
      currentElapsed += 1;
      setElapsedSec(currentElapsed);
      const pct = Math.min(98, Math.round((currentElapsed / totalDuration) * 100));
      setProgressPct(pct);
    }, 1000);

    try {
      const readRatio =
        config.workload === 'read_heavy' ? 0.8 : config.workload === 'write_heavy' ? 0.2 : 0.5;

      const result = await executeBenchmarkRun(
        {
          target: 'storage',
          scenario: 'standard',
          concurrency: config.concurrency,
          durationSeconds: config.durationSec,
          numKeys: config.keyCount,
          valueSize: parseInt(config.payloadSize) || 256,
          readRatio,
        },
        { signal: abortControllerRef.current.signal }
      );

      if (timerRef.current) clearInterval(timerRef.current);
      setProgressPct(100);
      setIsRunning(false);

      const finalMetrics: BenchmarkMetrics = {
        qps: result.throughputOpsPerSec,
        avgLatencyMs: result.latencyAvgMs,
        p50Ms: result.latencyP50Ms,
        p95Ms: result.latencyP95Ms,
        p99Ms: result.latencyP99Ms,
        maxLatencyMs: result.latencyMaxMs || result.latencyP99Ms,
        totalRequests: result.totalOperations,
        errorCount: result.failedOperations,
      };

      setMetrics(finalMetrics);
      setThroughputHistory((prev) => [...prev, finalMetrics.qps]);

      const newRun: BenchmarkRunRecord = {
        id: `run-${Date.now()}`,
        timestamp: new Date().toLocaleTimeString(),
        config,
        metrics: finalMetrics,
      };

      setHistory((prev) => {
        const updated = [newRun, ...prev];
        try {
          localStorage.setItem('orbit_benchmark_history', JSON.stringify(updated));
        } catch {}
        return updated;
      });
    } catch (err: any) {
      if (timerRef.current) clearInterval(timerRef.current);
      setIsRunning(false);
      setProgressPct(0);
      if (err?.name === 'AbortError' || err?.message?.includes('aborted')) {
        setErrorMessage('Benchmark run cancelled by operator.');
      } else {
        setErrorMessage(err?.message || 'Benchmark execution failed.');
      }
    }
  };

  const handleStopBenchmark = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    if (timerRef.current) clearInterval(timerRef.current);
    setIsRunning(false);
    setProgressPct(0);
  };

  const handleExportReport = (run: BenchmarkRunRecord) => {
    const blob = new Blob([JSON.stringify(run, null, 2)], {
      type: 'application/json',
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `orbit_benchmark_${run.id}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleExportCsv = (run: BenchmarkRunRecord) => {
    const headers = [
      'id',
      'timestamp',
      'workload',
      'durationSec',
      'concurrency',
      'keyCount',
      'payloadSize',
      'qps',
      'avgLatencyMs',
      'p50Ms',
      'p95Ms',
      'p99Ms',
      'maxLatencyMs',
      'totalRequests',
      'errorCount',
    ];
    const row = [
      run.id,
      `"${run.timestamp}"`,
      run.config.workload,
      run.config.durationSec,
      run.config.concurrency,
      run.config.keyCount,
      run.config.payloadSize,
      run.metrics.qps,
      run.metrics.avgLatencyMs,
      run.metrics.p50Ms,
      run.metrics.p95Ms,
      run.metrics.p99Ms,
      run.metrics.maxLatencyMs,
      run.metrics.totalRequests,
      run.metrics.errorCount,
    ];
    const csvContent = headers.join(',') + '\n' + row.join(',');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `orbit_benchmark_${run.id}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <DashboardLayout activeNav="benchmarks">
      <div className="space-y-6">
        {/* 01. PAGE HEADER */}
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-4 font-mono">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl sm:text-2xl font-bold text-[#17243A] tracking-tight">
                Cluster Performance & Benchmarking
              </h1>
              <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] text-xs font-semibold">
                <CheckCircle2 className="w-3.5 h-3.5" />
                MULTI-RAFT ENGINE OPTIMIZED
              </span>
            </div>
            <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal">
              Stress-test linearizable read index and asynchronous Raft commit saturation.
            </p>
          </div>

          <div className="flex items-center gap-2 text-xs">
            <span className="px-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A] font-semibold">
              Harness: Go orbit-bench Engine
            </span>
          </div>
        </div>

        {/* ERROR / CANCELLATION BANNER */}
        {errorMessage && (
          <div className="bg-amber-50 border border-amber-200 rounded-xl p-3.5 flex items-center justify-between text-xs font-mono text-amber-900">
            <span>{errorMessage}</span>
            <button
              onClick={() => setErrorMessage(null)}
              className="text-amber-600 hover:text-amber-800 font-bold ml-4"
            >
              ✕
            </button>
          </div>
        )}

        {/* 02. SECTION A & B: BENCHMARK CONFIGURATOR */}
        <BenchmarkConfigurator
          onStart={handleStartBenchmark}
          onStop={handleStopBenchmark}
          isRunning={isRunning}
          progressPct={progressPct}
          elapsedSec={elapsedSec}
        />

        {/* 03. SECTION C: METRICS DASHBOARD */}
        <BenchmarkMetricsCards metrics={metrics} />

        {/* 04. SECTION D: PERFORMANCE CHARTS */}
        <BenchmarkCharts
          throughputHistory={throughputHistory.length > 0 ? throughputHistory : [0]}
          readQps={Math.round(metrics.qps * 0.8)}
          writeQps={Math.round(metrics.qps * 0.2)}
          p50={metrics.p50Ms}
          p90={Math.round((metrics.p50Ms * 0.35 + metrics.p95Ms * 0.65) * 100) / 100}
          p95={metrics.p95Ms}
          p99={metrics.p99Ms}
        />

        {/* 05. SECTION E & F: BENCHMARK HISTORY & EXPORTS */}
        <BenchmarkHistory
          runs={history}
          onRerun={handleStartBenchmark}
          onExport={handleExportReport}
          onExportCsv={handleExportCsv}
        />
      </div>
    </DashboardLayout>
  );
}
