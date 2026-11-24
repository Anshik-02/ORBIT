import { NextRequest, NextResponse } from 'next/server';
import path from 'path';
import fs from 'fs';

export async function GET() {
  try {
    const rootDir = path.resolve(process.cwd(), '..');
    const nodeConfigPath = path.join(rootDir, 'configs', 'node-a.json');
    const clusterConfigPath = path.join(rootDir, 'configs', 'cluster.json');

    let nodeConfig = {
      id: 'node-a',
      address: '127.0.0.1:2525',
      raft_address: '127.0.0.1:3525',
      data_dir: 'data/node-a',
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
    const body = await req.json();
    const rootDir = path.resolve(process.cwd(), '..');
    const nodeConfigPath = path.join(rootDir, 'configs', 'node-a.json');
    const clusterConfigPath = path.join(rootDir, 'configs', 'cluster.json');

    let restartRequired = false;

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

      fs.writeFileSync(nodeConfigPath, JSON.stringify(updated, null, 2), 'utf-8');
    }

    if (body.clusterConfig && fs.existsSync(clusterConfigPath)) {
      const current = JSON.parse(fs.readFileSync(clusterConfigPath, 'utf-8'));
      const updated = { ...current, ...body.clusterConfig };
      fs.writeFileSync(clusterConfigPath, JSON.stringify(updated, null, 2), 'utf-8');
    }

    return NextResponse.json({
      ok: true,
      restartRequired,
      message: restartRequired
        ? 'Settings saved. Process restart required for port/network address changes to take effect.'
        : 'Settings saved and applied successfully.',
    });
  } catch (error: any) {
    return NextResponse.json({ error: error.message || 'Failed to save settings' }, { status: 500 });
  }
}
