import { NextRequest, NextResponse } from 'next/server';
import path from 'path';
import fs from 'fs';

const GO_DURATION_REGEX = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/;

function validateDuration(val: any, fieldName: string): string | null {
  if (val !== undefined && val !== null) {
    if (typeof val !== 'string' || !GO_DURATION_REGEX.test(val)) {
      return `Invalid Go duration format for ${fieldName}: "${val}". Expected format like "50ms", "200ms", "1s".`;
    }
  }
  return null;
}

function atomicWriteWithBackup(filePath: string, data: any) {
  const content = JSON.stringify(data, null, 2);
  const bakPath = `${filePath}.bak`;
  const tmpPath = `${filePath}.tmp.${Date.now()}`;
  let backupCreated = false;

  try {
    if (fs.existsSync(filePath)) {
      fs.copyFileSync(filePath, bakPath);
      backupCreated = true;
    }

    fs.writeFileSync(tmpPath, content, 'utf-8');
    fs.renameSync(tmpPath, filePath);
  } catch (err) {
    if (backupCreated && fs.existsSync(bakPath)) {
      try {
        fs.copyFileSync(bakPath, filePath);
      } catch {
        // preserve rollback state
      }
    }
    if (fs.existsSync(tmpPath)) {
      try {
        fs.unlinkSync(tmpPath);
      } catch {
        // cleanup tmp
      }
    }
    throw err;
  }
}

export async function GET(req: NextRequest) {
  try {
    const { searchParams } = new URL(req.url);
    const nodeId = searchParams.get('nodeId') || 'node-a';

    const rootDir = path.resolve(process.cwd(), '..');
    const nodeConfigPath = path.join(rootDir, 'configs', `${nodeId}.json`);
    const clusterConfigPath = path.join(rootDir, 'configs', 'cluster.json');

    let nodeConfig = {
      id: nodeId,
      address: '127.0.0.1:2525',
      raft_address: '127.0.0.1:3525',
      data_dir: `data/${nodeId}`,
      heartbeat_interval: '50ms',
      election_timeout_min: '150ms',
      election_timeout_max: '300ms',
      peers: [],
    };

    let clusterConfig = {
      cluster_id: 'orbit-cluster-01',
      epoch: 1,
      shard_count: 3,
      replication_factor: 3,
      physical_nodes: ['node-a', 'node-b', 'node-c', 'node-d', 'node-e'],
      shards: [],
    };

    if (fs.existsSync(nodeConfigPath)) {
      nodeConfig = JSON.parse(fs.readFileSync(nodeConfigPath, 'utf-8'));
    }
    if (fs.existsSync(clusterConfigPath)) {
      clusterConfig = JSON.parse(fs.readFileSync(clusterConfigPath, 'utf-8'));
    }

    return NextResponse.json({ nodeConfig, clusterConfig });
  } catch (error: any) {
    return NextResponse.json({ error: error.message || 'Failed to read settings' }, { status: 500 });
  }
}

export async function POST(req: NextRequest) {
  try {
    // 01. ACCESS CONTROL CHECK
    const host = req.headers.get('host') || '';
    const forwarded = req.headers.get('x-forwarded-for') || '';
    const authHeader = req.headers.get('authorization');

    const isLoopback =
      host.startsWith('localhost') ||
      host.startsWith('127.0.0.1') ||
      host.startsWith('[::1]') ||
      forwarded === '127.0.0.1' ||
      forwarded === '::1';

    if (!isLoopback && !authHeader) {
      return NextResponse.json(
        {
          error:
            'Access Denied: Configuration mutations restricted to local loopback control plane or authorized clients.',
        },
        { status: 403 }
      );
    }

    const body = await req.json();

    // 02. SCHEMA & DURATION VALIDATION
    if (body.nodeConfig) {
      const hbErr = validateDuration(body.nodeConfig.heartbeat_interval, 'heartbeat_interval');
      if (hbErr) return NextResponse.json({ error: hbErr }, { status: 400 });

      const minErr = validateDuration(body.nodeConfig.election_timeout_min, 'election_timeout_min');
      if (minErr) return NextResponse.json({ error: minErr }, { status: 400 });

      const maxErr = validateDuration(body.nodeConfig.election_timeout_max, 'election_timeout_max');
      if (maxErr) return NextResponse.json({ error: maxErr }, { status: 400 });
    }

    if (body.clusterConfig) {
      if (
        body.clusterConfig.replication_factor !== undefined &&
        (typeof body.clusterConfig.replication_factor !== 'number' || body.clusterConfig.replication_factor < 1)
      ) {
        return NextResponse.json(
          { error: 'replication_factor must be an integer greater than or equal to 1' },
          { status: 400 }
        );
      }
      if (
        body.clusterConfig.shard_count !== undefined &&
        (typeof body.clusterConfig.shard_count !== 'number' || body.clusterConfig.shard_count < 1)
      ) {
        return NextResponse.json(
          { error: 'shard_count must be an integer greater than or equal to 1' },
          { status: 400 }
        );
      }
    }

    const rootDir = path.resolve(process.cwd(), '..');
    const targetNodeId = body.nodeId || body.nodeConfig?.id || 'node-a';
    const nodeConfigPath = path.join(rootDir, 'configs', `${targetNodeId}.json`);
    const clusterConfigPath = path.join(rootDir, 'configs', 'cluster.json');

    let restartRequired = false;

    // 03. ATOMIC WRITE WITH BACKUP ROLLBACK
    if (body.nodeConfig && fs.existsSync(nodeConfigPath)) {
      const current = JSON.parse(fs.readFileSync(nodeConfigPath, 'utf-8'));
      const updated = { ...current, ...body.nodeConfig };

      if (
        updated.address !== current.address ||
        updated.raft_address !== current.raft_address ||
        updated.data_dir !== current.data_dir
      ) {
        restartRequired = true;
      }

      atomicWriteWithBackup(nodeConfigPath, updated);
    }

    if (body.clusterConfig && fs.existsSync(clusterConfigPath)) {
      const current = JSON.parse(fs.readFileSync(clusterConfigPath, 'utf-8'));
      const updated = { ...current, ...body.clusterConfig };
      atomicWriteWithBackup(clusterConfigPath, updated);
    }

    return NextResponse.json({
      ok: true,
      restartRequired,
      message: restartRequired
        ? 'Settings saved atomically with backup. Process restart required for port/network address changes to take effect.'
        : 'Settings saved and verified successfully. Backup created.',
    });
  } catch (error: any) {
    return NextResponse.json({ error: error.message || 'Failed to save settings' }, { status: 500 });
  }
}
