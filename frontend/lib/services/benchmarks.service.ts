/**
 * Benchmarks Service
 * Executes real Go benchmark binaries, stores historical benchmark runs, and exports reports.
 */

import { executeBenchmarkRun } from '../api/client';
import { BenchmarkConfig, BenchmarkResult, RequestOptions } from '../api/types';

const STORAGE_KEY = 'orbit_benchmark_history';

export class BenchmarksService {
  /**
   * Run real benchmark against the active database cluster.
   */
  public static async execute(config: BenchmarkConfig, options?: RequestOptions): Promise<BenchmarkResult> {
    const result = await executeBenchmarkRun(config, options);
    this.saveToHistory(result);
    return result;
  }

  /**
   * Retrieve historical benchmark runs from persistent local storage.
   */
  public static getHistory(): BenchmarkResult[] {
    if (typeof window === 'undefined') return [];
    try {
      const data = localStorage.getItem(STORAGE_KEY);
      return data ? JSON.parse(data) : [];
    } catch {
      return [];
    }
  }

  /**
   * Save benchmark run to history.
   */
  public static saveToHistory(run: BenchmarkResult) {
    if (typeof window === 'undefined') return;
    try {
      const history = this.getHistory();
      const updated = [run, ...history.filter((h) => h.timestamp !== run.timestamp)].slice(0, 50);
      localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
    } catch {
      // LocalStorage quota or unavailable
    }
  }

  /**
   * Export benchmark result to downloadable JSON.
   */
  public static exportToJson(run: BenchmarkResult, filename = 'orbit-benchmark-report.json') {
    const blob = new Blob([JSON.stringify(run, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }

  /**
   * Export benchmark history to CSV.
   */
  public static exportToCsv(runs: BenchmarkResult[], filename = 'orbit-benchmarks.csv') {
    const headers = [
      'Timestamp',
      'Scenario',
      'Target',
      'Concurrency',
      'DurationSec',
      'TotalOps',
      'ThroughputOpsSec',
      'LatencyAvgMs',
      'LatencyP50Ms',
      'LatencyP99Ms',
    ];
    const rows = runs.map((r) => [
      r.timestamp,
      r.scenario,
      r.target,
      r.concurrency,
      r.durationSeconds,
      r.totalOperations,
      r.throughputOpsPerSec,
      r.latencyAvgMs,
      r.latencyP50Ms,
      r.latencyP99Ms,
    ]);

    const csvContent = [headers.join(','), ...rows.map((r) => r.map((c) => `"${c}"`).join(','))].join('\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }
}
