/**
 * Chaos Engineering Service
 * Injects deterministic faults, evaluates quorum resilience, and records recovery impact.
 */

import { executeChaosExperiment } from '../api/client';
import { ChaosExperimentConfig, ChaosExperimentReport, RequestOptions } from '../api/types';

const STORAGE_KEY = 'orbit_chaos_history';

export class ChaosService {
  /**
   * Run chaos fault injection against the cluster.
   */
  public static async execute(
    config: ChaosExperimentConfig,
    options?: RequestOptions
  ): Promise<ChaosExperimentReport> {
    const report = await executeChaosExperiment(config, options);
    this.saveToHistory(report);
    return report;
  }

  /**
   * Retrieve historical chaos experiment reports.
   */
  public static getHistory(): ChaosExperimentReport[] {
    if (typeof window === 'undefined') return [];
    try {
      const data = localStorage.getItem(STORAGE_KEY);
      return data ? JSON.parse(data) : [];
    } catch {
      return [];
    }
  }

  /**
   * Save chaos experiment report to local history.
   */
  public static saveToHistory(report: ChaosExperimentReport) {
    if (typeof window === 'undefined') return;
    try {
      const history = this.getHistory();
      const updated = [report, ...history.filter((h) => h.id !== report.id)].slice(0, 50);
      localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
    } catch {
      // LocalStorage quota or unavailable
    }
  }

  /**
   * Export chaos report to downloadable JSON.
   */
  public static exportToJson(report: ChaosExperimentReport, filename = 'orbit-chaos-report.json') {
    const blob = new Blob([JSON.stringify(report, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }
}
