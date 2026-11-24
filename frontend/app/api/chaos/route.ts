import { NextRequest, NextResponse } from 'next/server';
import { execFile } from 'child_process';
import path from 'path';
import fs from 'fs';
import { promisify } from 'util';
import { ChaosExperimentReport } from '@/lib/api/types';

const execFileAsync = promisify(execFile);

export async function POST(req: NextRequest) {
  try {
    const body = await req.json();
    const faultType = body.faultType || 'wal-crash';
    const target = body.target || 'node-a';
    const durationSeconds = Number(body.durationSeconds) || 2;

    const rootDir = path.resolve(process.cwd(), '..');
    const binPath = path.join(rootDir, 'bin', 'orbit-chaos.exe');
    const fallbackBinPath = path.join(rootDir, 'orbit-chaos.exe');

    const executable = fs.existsSync(binPath)
      ? binPath
      : fs.existsSync(fallbackBinPath)
      ? fallbackBinPath
      : null;

    if (!executable) {
      return NextResponse.json(
        {
          error: 'Chaos binary not found. Please compile orbit-chaos using go build -o bin/orbit-chaos.exe ./cmd/orbit-chaos',
          code: 'CHAOS_BINARY_NOT_FOUND',
        },
        { status: 500 }
      );
    }

    const expId = `exp-${faultType}-${Date.now()}`;
    const outReport = path.join(rootDir, 'bin', `${expId}.json`);
    const args = [
      '-action=run',
      `-fault=${faultType}`,
      `-target=${target}`,
      `-duration=${durationSeconds}s`,
      `-output=${outReport}`,
    ];

    try {
      await execFileAsync(executable, args, { timeout: (durationSeconds + 10) * 1000 });

      if (fs.existsSync(outReport)) {
        const rawContent = fs.readFileSync(outReport, 'utf-8');
        try {
          fs.unlinkSync(outReport);
        } catch {
          // ignore cleanup err
        }

        const raw = JSON.parse(rawContent);
        const report: ChaosExperimentReport = {
          id: raw.id || expId,
          name: raw.name || `Fault Injection: ${faultType}`,
          faultType: raw.config?.fault_type || faultType,
          target: raw.config?.target || target,
          durationSeconds: raw.duration_seconds || durationSeconds,
          state: (raw.state as any) || 'COMPLETED',
          quorumMaintained: raw.data_consistent === true,
          dataLossDetected: raw.data_consistent === false,
          recoveryDurationMs: Math.round(raw.recovery_time_ms || 0),
          timeline: Array.isArray(raw.timeline)
            ? raw.timeline.map((t: any) => ({
                timestamp: t.timestamp ? new Date(t.timestamp).toLocaleTimeString() : new Date().toLocaleTimeString(),
                phase: t.phase || 'EXEC',
                description: t.description || '',
                details: t.details || {},
              }))
            : [],
          summary: raw.data_consistent
            ? `Successfully executed chaos fault ${faultType} against ${target || 'cluster'}. Quorum maintained and cluster remained fully consistent.`
            : `Chaos experiment ${faultType} against ${target || 'cluster'} finished with anomalies.`,
          timestamp: raw.start_time || new Date().toISOString(),
        };

        return NextResponse.json(report);
      } else {
        return NextResponse.json(
          {
            error: 'Chaos experiment executed but failed to generate report file.',
            code: 'CHAOS_REPORT_MISSING',
          },
          { status: 500 }
        );
      }
    } catch (execErr: any) {
      console.error('Failed to run orbit-chaos executable:', execErr);
      return NextResponse.json(
        {
          error: `Chaos execution failed: ${execErr.message || String(execErr)}`,
          code: 'CHAOS_EXECUTION_FAILED',
        },
        { status: 500 }
      );
    }
  } catch (error: any) {
    return NextResponse.json({ error: error.message || 'Chaos execution failed' }, { status: 500 });
  }
}
