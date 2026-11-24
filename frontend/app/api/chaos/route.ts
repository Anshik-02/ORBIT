import { NextRequest, NextResponse } from 'next/server';
import { execFile } from 'child_process';
import path from 'path';
import fs from 'fs';
import { promisify } from 'util';
import { ChaosExperimentReport } from '@/lib/api/types';

const execFileAsync = promisify(execFile);

function resolveChaosBinary(): string | null {
  const isWindows = process.platform === 'win32';
  const binName = isWindows ? 'orbit-chaos.exe' : 'orbit-chaos';
  const altName = isWindows ? 'orbit-chaos' : 'orbit-chaos.exe';

  const candidates = [
    path.resolve(process.cwd(), '..', 'bin', binName),
    path.resolve(process.cwd(), '..', binName),
    path.resolve(process.cwd(), 'bin', binName),
    path.resolve(process.cwd(), '..', 'bin', altName),
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
    const faultType = body.faultType || 'wal-crash';
    const target = body.target || 'node-a';
    const durationSeconds = Number(body.durationSeconds) || 2;

    const rootDir = path.resolve(process.cwd(), '..');
    const executable = resolveChaosBinary();

    if (!executable) {
      return NextResponse.json(
        {
          error:
            'Chaos binary not found in bin/. Please compile orbit-chaos using go build -o bin/orbit-chaos.exe ./cmd/orbit-chaos',
          code: 'CHAOS_BINARY_NOT_FOUND',
        },
        { status: 500 }
      );
    }

    // Resolve file/directory targets for disk & WAL faults
    let resolvedTarget = target;
    const allowedNodes = ['node-a', 'node-b', 'node-c', 'node-d', 'node-e'];
    if (allowedNodes.includes(target)) {
      if (faultType === 'wal-crash') {
        const candidateWal = path.join(rootDir, 'data', target, 'shard-0', 'wal.log');
        const fallbackWal = path.join(rootDir, 'data', target, 'wal.log');
        resolvedTarget = fs.existsSync(candidateWal) ? candidateWal : fallbackWal;
      } else if (faultType === 'flush-crash' || faultType === 'compaction-crash') {
        const shard0Dir = path.join(rootDir, 'data', target, 'shard-0');
        resolvedTarget = fs.existsSync(shard0Dir) ? shard0Dir : path.join(rootDir, 'data', target);
      } else if (faultType === 'corrupt-sstable') {
        const sstDir = path.join(rootDir, 'data', target, 'shard-0', 'sstables');
        if (fs.existsSync(sstDir)) {
          const files = fs.readdirSync(sstDir).filter((f) => f.endsWith('.sst'));
          if (files.length > 0) {
            resolvedTarget = path.join(sstDir, files[0]);
          }
        }
      }
    }

    const expId = `exp-${faultType}-${Date.now()}`;
    const outReport = path.join(rootDir, 'bin', `${expId}.json`);
    const args = [
      '-action=run',
      `-fault=${faultType}`,
      `-target=${resolvedTarget}`,
      `-duration=${durationSeconds}s`,
      `-output=${outReport}`,
    ];

    try {
      await execFileAsync(executable, args, {
        cwd: rootDir,
        timeout: (durationSeconds + 12) * 1000,
      });

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
                timestamp: t.timestamp
                  ? new Date(t.timestamp).toLocaleTimeString()
                  : new Date().toLocaleTimeString(),
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
      if (fs.existsSync(outReport)) {
        try {
          fs.unlinkSync(outReport);
        } catch {}
      }
      return NextResponse.json(
        {
          error: `Chaos execution failed: ${execErr.stderr || execErr.message || String(execErr)}`,
          code: 'CHAOS_EXECUTION_FAILED',
        },
        { status: 500 }
      );
    }
  } catch (error: any) {
    return NextResponse.json({ error: error.message || 'Chaos request failed' }, { status: 500 });
  }
}
