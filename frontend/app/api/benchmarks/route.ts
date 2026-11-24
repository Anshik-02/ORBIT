import { NextRequest, NextResponse } from 'next/server';
import { execFile } from 'child_process';
import path from 'path';
import fs from 'fs';
import { promisify } from 'util';

const execFileAsync = promisify(execFile);

function resolveBenchmarkBinary(): string | null {
  const isWindows = process.platform === 'win32';
  const binName = isWindows ? 'orbit-bench.exe' : 'orbit-bench';

  const candidates = [
    path.resolve(process.cwd(), '..', 'bin', binName),
    path.resolve(process.cwd(), '..', binName),
    path.resolve(process.cwd(), 'bin', binName),
  ];

  for (const c of candidates) {
    if (fs.existsSync(c)) {
      return c;
    }
  }
  return null;
}

export async function POST(req: NextRequest) {
  try {
    const body = await req.json();
    const concurrency = Math.max(1, Math.min(64, Number(body.concurrency) || 4));
    const durationSeconds = Math.max(1, Math.min(60, Number(body.durationSeconds) || 3));
    const numKeys = Math.max(10, Math.min(100000, Number(body.numKeys) || 5000));
    const readRatio = Math.max(0.0, Math.min(1.0, Number(body.readRatio) ?? 0.5));
    const target = body.target || 'storage';
    const scenario = body.scenario || 'standard';
    const baseURL = body.url || 'http://127.0.0.1:2525';

    // Locate actual binary
    const executable = resolveBenchmarkBinary();
    if (!executable) {
      return NextResponse.json(
        {
          error:
            'Benchmark runner binary (orbit-bench) not found in bin/. Please compile with: go build -o bin/orbit-bench.exe ./cmd/orbit-bench',
        },
        { status: 500 }
      );
    }

    const rootDir = path.resolve(process.cwd(), '..');
    const tempOut = path.join(rootDir, 'bin', `bench-run-${Date.now()}-${Math.random().toString(36).slice(2, 6)}.json`);

    const args = [
      `-target=${target}`,
      `-scenario=${scenario}`,
      `-concurrency=${concurrency}`,
      `-duration=${durationSeconds}s`,
      `-keys=${numKeys}`,
      `-read-ratio=${readRatio}`,
      `-url=${baseURL}`,
      `-output=${tempOut}`,
    ];

    try {
      await execFileAsync(executable, args, {
        cwd: rootDir,
        timeout: (durationSeconds + 15) * 1000,
      });

      if (!fs.existsSync(tempOut)) {
        return NextResponse.json(
          { error: 'Benchmark execution completed but no output report was generated' },
          { status: 500 }
        );
      }

      const raw = fs.readFileSync(tempOut, 'utf-8');
      try {
        fs.unlinkSync(tempOut);
      } catch {}

      const parsed = JSON.parse(raw);
      const latency = parsed.latency_ms || {};

      return NextResponse.json({
        id: `bench-${Date.now()}`,
        name: parsed.config?.name || `${target}-${scenario}-${concurrency}w`,
        target: parsed.config?.target || target,
        scenario: parsed.config?.scenario || scenario,
        concurrency: parsed.config?.concurrency || concurrency,
        durationSeconds: parsed.duration_seconds || durationSeconds,
        totalOperations: parsed.total_ops || 0,
        successfulOperations: Math.max(0, (parsed.total_ops || 0) - (parsed.error_ops || 0)),
        failedOperations: parsed.error_ops || 0,
        throughputOpsPerSec: Math.round((parsed.ops_per_sec || 0) * 10) / 10,
        readThroughputOpsPerSec: Math.round((parsed.read_ops_per_sec || 0) * 10) / 10,
        writeThroughputOpsPerSec: Math.round((parsed.write_ops_per_sec || 0) * 10) / 10,
        latencyAvgMs: Math.round((latency.mean || parsed.latency_avg_ms || 0) * 100) / 100,
        latencyP50Ms: Math.round((latency.p50 || 0) * 100) / 100,
        latencyP95Ms: Math.round((latency.p95 || 0) * 100) / 100,
        latencyP99Ms: Math.round((latency.p99 || 0) * 100) / 100,
        latencyMaxMs: Math.round((parsed.max_latency_ms || latency.p999 || 0) * 100) / 100,
        timestamp: parsed.timestamp || new Date().toISOString(),
      });
    } catch (execErr: any) {
      if (fs.existsSync(tempOut)) {
        try {
          fs.unlinkSync(tempOut);
        } catch {}
      }
      return NextResponse.json(
        {
          error: `Benchmark binary execution failed: ${execErr.stderr || execErr.message || 'Process error'}`,
        },
        { status: 500 }
      );
    }
  } catch (error: any) {
    return NextResponse.json({ error: error.message || 'Benchmark request failed' }, { status: 500 });
  }
}
