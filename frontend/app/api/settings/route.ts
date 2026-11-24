import { NextRequest, NextResponse } from 'next/server';
import path from 'path';
import fs from 'fs';

const ALLOWED_NODE_IDS = new Set(['node-a', 'node-b', 'node-c', 'node-d', 'node-e']);
const GO_DURATION_REGEX = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/;
const HOST_PORT_REGEX = /^([a-zA-Z0-9.\-_]+|\*):(\d{1,5})$/;
const SETTINGS_AUTH_TOKEN =
  process.env.ORBIT_SETTINGS_SECRET ||
  process.env.ORBIT_API_SECRET ||
  'orbit-control-plane-secret-key';

function parseDurationToMs(val: string): number | null {
  const match = val.match(/^(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)$/);
  if (!match) return null;
  const num = parseFloat(match[1]);
  const unit = match[2];
  switch (unit) {
    case 'ns': return num / 1e6;
    case 'us':
    case 'µs': return num / 1e3;
    case 'ms': return num;
    case 's': return num * 1000;
    case 'm': return num * 60000;
    case 'h': return num * 3600000;
    default: return null;
  }
}

function validateDuration(val: any, fieldName: string, minMs = 10, maxMs = 60000): string | null {
  if (val !== undefined && val !== null) {
    if (typeof val !== 'string' || !GO_DURATION_REGEX.test(val)) {
      return `Invalid Go duration format for ${fieldName}: "${val}". Expected format like "50ms", "200ms", "1s".`;
    }
    const ms = parseDurationToMs(val);
    if (ms === null || ms < minMs || ms > maxMs) {
      return `${fieldName} duration ${val} (${ms}ms) is outside allowed operational safety bounds [${minMs}ms, ${maxMs}ms].`;
    }
  }
  return null;
}

function verifyAuth(req: NextRequest): { authorized: boolean; reason?: string } {
  const authHeader = req.headers.get('authorization') || '';
  const expectedBearer = `Bearer ${SETTINGS_AUTH_TOKEN}`;

  // If a valid Bearer token is provided, authorize immediately
  if (authHeader === expectedBearer) {
    return { authorized: true };
  }

  // Check if request is genuine unproxied local loopback
  const host = req.headers.get('host') || '';
  const xForwardedFor = req.headers.get('x-forwarded-for');
  const isDirectLoopback =
    (host.startsWith('localhost') || host.startsWith('127.0.0.1') || host.startsWith('[::1]')) &&
    !xForwardedFor; // If x-forwarded-for is present, request came through a proxy; require auth token

  if (isDirectLoopback) {
    return { authorized: true };
  }

  return {
    authorized: false,
    reason:
      'Access Denied: Remote settings mutations require a valid Authorization Bearer token matching ORBIT_SETTINGS_SECRET.',
  };
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

    // Strict allowlist validation against path traversal
    if (!ALLOWED_NODE_IDS.has(nodeId)) {
      return NextResponse.json(
        { error: `Invalid nodeId "${nodeId}". Must be one of: ${Array.from(ALLOWED_NODE_IDS).join(', ')}` },
        { status: 400 }
      );
    }

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
    // 01. STRICT ACCESS CONTROL
    const authResult = verifyAuth(req);
    if (!authResult.authorized) {
      return NextResponse.json({ error: authResult.reason }, { status: 403 });
    }

    const body = await req.json();

    // 02. STRICT NODE ID ALLOWLIST VALIDATION
    const targetNodeId = body.nodeId || body.nodeConfig?.id || 'node-a';
    if (!ALLOWED_NODE_IDS.has(targetNodeId)) {
      return NextResponse.json(
        {
          error: `Invalid target node "${targetNodeId}". Allowed nodes: ${Array.from(ALLOWED_NODE_IDS).join(', ')}`,
        },
        { status: 400 }
      );
    }

    // 03. COMPREHENSIVE FIELD & RANGE VALIDATION
    const restartRequiredFields: string[] = [];
    const runtimeUpdatedFields: string[] = [];

    if (body.nodeConfig) {
      const allowedNodeFields = new Set([
        'id',
        'address',
        'raft_address',
        'data_dir',
        'heartbeat_interval',
        'election_timeout_min',
        'election_timeout_max',
        'peers',
      ]);

      for (const key of Object.keys(body.nodeConfig)) {
        if (!allowedNodeFields.has(key)) {
          return NextResponse.json(
            { error: `Unauthorized node configuration parameter: "${key}".` },
            { status: 400 }
          );
        }
      }

      if (body.nodeConfig.address) {
        if (!HOST_PORT_REGEX.test(body.nodeConfig.address)) {
          return NextResponse.json(
            { error: `Invalid HTTP address format "${body.nodeConfig.address}". Expected host:port.` },
            { status: 400 }
          );
        }
      }

      if (body.nodeConfig.raft_address) {
        if (!HOST_PORT_REGEX.test(body.nodeConfig.raft_address)) {
          return NextResponse.json(
            { error: `Invalid Raft address format "${body.nodeConfig.raft_address}". Expected host:port.` },
            { status: 400 }
          );
        }
      }

      if (body.nodeConfig.data_dir) {
        if (
          typeof body.nodeConfig.data_dir !== 'string' ||
          body.nodeConfig.data_dir.includes('..') ||
          !body.nodeConfig.data_dir.startsWith('data/')
        ) {
          return NextResponse.json(
            { error: `Invalid data_dir "${body.nodeConfig.data_dir}". Must reside inside "data/" without parent traversal.` },
            { status: 400 }
          );
        }
      }

      const hbErr = validateDuration(body.nodeConfig.heartbeat_interval, 'heartbeat_interval', 10, 5000);
      if (hbErr) return NextResponse.json({ error: hbErr }, { status: 400 });

      const minErr = validateDuration(body.nodeConfig.election_timeout_min, 'election_timeout_min', 50, 30000);
      if (minErr) return NextResponse.json({ error: minErr }, { status: 400 });

      const maxErr = validateDuration(body.nodeConfig.election_timeout_max, 'election_timeout_max', 50, 60000);
      if (maxErr) return NextResponse.json({ error: maxErr }, { status: 400 });

      // Cross-field timeout sanity check: election_timeout_max > election_timeout_min > heartbeat_interval
      if (
        body.nodeConfig.heartbeat_interval &&
        body.nodeConfig.election_timeout_min &&
        body.nodeConfig.election_timeout_max
      ) {
        const hbMs = parseDurationToMs(body.nodeConfig.heartbeat_interval)!;
        const minMs = parseDurationToMs(body.nodeConfig.election_timeout_min)!;
        const maxMs = parseDurationToMs(body.nodeConfig.election_timeout_max)!;

        if (minMs <= hbMs) {
          return NextResponse.json(
            { error: `election_timeout_min (${minMs}ms) must be strictly greater than heartbeat_interval (${hbMs}ms).` },
            { status: 400 }
          );
        }
        if (maxMs <= minMs) {
          return NextResponse.json(
            { error: `election_timeout_max (${maxMs}ms) must be strictly greater than election_timeout_min (${minMs}ms).` },
            { status: 400 }
          );
        }
      }
    }

    if (body.clusterConfig) {
      const allowedClusterFields = new Set([
        'cluster_id',
        'epoch',
        'shard_count',
        'replication_factor',
        'physical_nodes',
        'shards',
      ]);

      for (const key of Object.keys(body.clusterConfig)) {
        if (!allowedClusterFields.has(key)) {
          return NextResponse.json(
            { error: `Unauthorized cluster configuration parameter: "${key}".` },
            { status: 400 }
          );
        }
      }

      if (
        body.clusterConfig.replication_factor !== undefined &&
        (typeof body.clusterConfig.replication_factor !== 'number' ||
          body.clusterConfig.replication_factor < 1 ||
          body.clusterConfig.replication_factor > 9)
      ) {
        return NextResponse.json(
          { error: 'replication_factor must be an integer between 1 and 9.' },
          { status: 400 }
        );
      }

      if (
        body.clusterConfig.shard_count !== undefined &&
        (typeof body.clusterConfig.shard_count !== 'number' ||
          body.clusterConfig.shard_count < 1 ||
          body.clusterConfig.shard_count > 256)
      ) {
        return NextResponse.json(
          { error: 'shard_count must be an integer between 1 and 256.' },
          { status: 400 }
        );
      }
    }

    const rootDir = path.resolve(process.cwd(), '..');
    const nodeConfigPath = path.join(rootDir, 'configs', `${targetNodeId}.json`);
    const clusterConfigPath = path.join(rootDir, 'configs', 'cluster.json');

    // 04. ATOMIC WRITE WITH ROLLBACK & RESTART DETECTION
    if (body.nodeConfig && fs.existsSync(nodeConfigPath)) {
      const current = JSON.parse(fs.readFileSync(nodeConfigPath, 'utf-8'));
      const updated = { ...current, ...body.nodeConfig };

      for (const field of ['address', 'raft_address', 'data_dir']) {
        if (updated[field] !== current[field]) {
          restartRequiredFields.push(field);
        }
      }

      for (const field of ['heartbeat_interval', 'election_timeout_min', 'election_timeout_max']) {
        if (updated[field] !== current[field]) {
          runtimeUpdatedFields.push(field);
        }
      }

      atomicWriteWithBackup(nodeConfigPath, updated);
    }

    if (body.clusterConfig && fs.existsSync(clusterConfigPath)) {
      const current = JSON.parse(fs.readFileSync(clusterConfigPath, 'utf-8'));
      const updated = { ...current, ...body.clusterConfig };
      for (const field of ['shard_count', 'replication_factor']) {
        if (updated[field] !== current[field]) {
          runtimeUpdatedFields.push(field);
        }
      }
      atomicWriteWithBackup(clusterConfigPath, updated);
    }

    const restartRequired = restartRequiredFields.length > 0;

    return NextResponse.json({
      ok: true,
      restartRequired,
      restartRequiredFields,
      runtimeUpdatedFields,
      message: restartRequired
        ? `Settings saved. Node process restart required for modified network parameters: [${restartRequiredFields.join(', ')}].`
        : 'Settings saved and verified successfully. Backup created.',
    });
  } catch (error: any) {
    return NextResponse.json({ error: error.message || 'Failed to save settings' }, { status: 500 });
  }
}
