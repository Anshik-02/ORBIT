import { PhysicalNode } from './types';

export const DETERMINISTIC_3D_POSITIONS: Record<string, [number, number, number]> = {
  'node-a': [0, 0, -4.5],
  'node-b': [-4.2, 0, -1.3],
  'node-e': [4.2, 0, -1.3],
  'node-c': [-2.6, 0, 3.4],
  'node-d': [2.6, 0, 3.4],
};

export const DETERMINISTIC_2D_POSITIONS: Record<string, { x: number; y: number }> = {
  'node-a': { x: 50, y: 15 },
  'node-b': { x: 20, y: 42 },
  'node-e': { x: 80, y: 42 },
  'node-c': { x: 32, y: 82 },
  'node-d': { x: 68, y: 82 },
};

export const NODE_NETWORK_CONFIG: Record<
  string,
  { name: string; httpPort: number; raftPort: number }
> = {
  'node-a': { name: 'Node A', httpPort: 2525, raftPort: 3525 },
  'node-b': { name: 'Node B', httpPort: 2526, raftPort: 3526 },
  'node-c': { name: 'Node C', httpPort: 2527, raftPort: 3527 },
  'node-d': { name: 'Node D', httpPort: 2528, raftPort: 3528 },
  'node-e': { name: 'Node E', httpPort: 2529, raftPort: 3529 },
};

export const SHARD_CONFIG = [
  {
    shardId: 0,
    name: 'Shard 0',
    color: '#0891D5', // Cyan
    replicas: ['node-a', 'node-b', 'node-c'],
    defaultLeader: 'node-a',
    term: 4,
    commitIndex: 14892,
  },
  {
    shardId: 1,
    name: 'Shard 1',
    color: '#7965E8', // Purple
    replicas: ['node-d', 'node-e', 'node-a'],
    defaultLeader: 'node-d',
    term: 6,
    commitIndex: 12450,
  },
  {
    shardId: 2,
    name: 'Shard 2',
    color: '#10B981', // Emerald
    replicas: ['node-b', 'node-c', 'node-d'],
    defaultLeader: 'node-b',
    term: 5,
    commitIndex: 13910,
  },
];

export const PHYSICAL_NETWORK_PEERS = [
  { source: 'node-a', destination: 'node-b', latencyMs: 1.2 },
  { source: 'node-a', destination: 'node-e', latencyMs: 1.4 },
  { source: 'node-b', destination: 'node-c', latencyMs: 1.1 },
  { source: 'node-c', destination: 'node-d', latencyMs: 1.8 },
  { source: 'node-d', destination: 'node-e', latencyMs: 1.3 },
  { source: 'node-b', destination: 'node-e', latencyMs: 2.1 },
  { source: 'node-a', destination: 'node-c', latencyMs: 1.9 },
  { source: 'node-a', destination: 'node-d', latencyMs: 2.0 },
];
