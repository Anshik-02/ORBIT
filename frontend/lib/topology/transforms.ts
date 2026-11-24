import {
  PhysicalNode,
  NetworkLink,
  ClusterSummaryData,
  HostedShardInfo,
  NetworkMode,
  TopologyEvent,
} from './types';
import {
  DETERMINISTIC_3D_POSITIONS,
  DETERMINISTIC_2D_POSITIONS,
  NODE_NETWORK_CONFIG,
  SHARD_CONFIG,
  PHYSICAL_NETWORK_PEERS,
} from './layout';

export function buildNodesData(
  offlineNodeIds: Set<string> = new Set()
): PhysicalNode[] {
  const nodeIds = ['node-a', 'node-b', 'node-c', 'node-d', 'node-e'];

  return nodeIds.map((id) => {
    const isOnline = !offlineNodeIds.has(id);
    const cfg = NODE_NETWORK_CONFIG[id];

    // Find shards hosted on this node
    const hostedShards: HostedShardInfo[] = [];
    SHARD_CONFIG.forEach((sh) => {
      if (sh.replicas.includes(id)) {
        // If the default leader is offline, failover to first surviving replica
        let currentLeader = sh.defaultLeader;
        if (offlineNodeIds.has(currentLeader)) {
          const survivor = sh.replicas.find((r) => !offlineNodeIds.has(r));
          if (survivor) {
            currentLeader = survivor;
          }
        }

        const isLeader = isOnline && currentLeader === id;
        hostedShards.push({
          shardId: sh.shardId,
          shardName: sh.name,
          role: isLeader ? 'Leader' : 'Follower',
          term: isLeader ? sh.term + (currentLeader !== sh.defaultLeader ? 1 : 0) : sh.term,
          commitIndex: sh.commitIndex,
          appliedIndex: sh.commitIndex,
          color: sh.color,
        });
      }
    });

    const leaderGroups = hostedShards.filter((s) => s.role === 'Leader').length;
    const followerGroups = hostedShards.filter((s) => s.role === 'Follower').length;

    return {
      id,
      name: cfg?.name || id,
      status: isOnline ? 'online' : 'offline',
      online: isOnline,
      httpAddress: `127.0.0.1:${cfg?.httpPort || 8000}`,
      raftAddress: `127.0.0.1:${cfg?.raftPort || 9000}`,
      hostedShards,
      raftGroupCount: hostedShards.length,
      leaderGroupCount: leaderGroups,
      followerGroupCount: followerGroups,
      lastObservedActivity: isOnline ? '42 ms ago' : 'Disconnected',
      lastHeartbeatMs: isOnline ? 42 : 15000,
      position3D: DETERMINISTIC_3D_POSITIONS[id] || [0, 0, 0],
      position2D: DETERMINISTIC_2D_POSITIONS[id] || { x: 50, y: 50 },
    };
  });
}

export function buildNetworkLinks(
  nodes: PhysicalNode[],
  networkMode: NetworkMode,
  selectedShardId?: number | 'all'
): NetworkLink[] {
  const nodeMap = new Map(nodes.map((n) => [n.id, n]));

  if (networkMode === 'physical') {
    return PHYSICAL_NETWORK_PEERS.map((peer, idx) => {
      const srcNode = nodeMap.get(peer.source);
      const dstNode = nodeMap.get(peer.destination);
      const bothOnline = srcNode?.online && dstNode?.online;
      const oneOffline = !srcNode?.online || !dstNode?.online;

      return {
        id: `phys-${peer.source}-${peer.destination}`,
        source: peer.source,
        destination: peer.destination,
        type: 'physical',
        status: bothOnline ? 'healthy' : oneOffline ? 'disconnected' : 'idle',
        latencyMs: bothOnline ? peer.latencyMs : undefined,
        lastContact: bothOnline ? `${Math.round(peer.latencyMs * 15)} ms ago` : 'Disconnected',
        activeTraffic: bothOnline ? true : false,
      };
    });
  }

  // Raft relationships mode
  const links: NetworkLink[] = [];
  SHARD_CONFIG.forEach((sh) => {
    if (selectedShardId !== undefined && selectedShardId !== 'all' && selectedShardId !== sh.shardId) {
      return;
    }

    // Determine current leader
    let currentLeader = sh.defaultLeader;
    const leaderNode = nodeMap.get(currentLeader);
    if (!leaderNode?.online) {
      const survivor = sh.replicas.find((r) => nodeMap.get(r)?.online);
      if (survivor) {
        currentLeader = survivor;
      }
    }

    const liveLeader = nodeMap.get(currentLeader);

    sh.replicas.forEach((replicaId) => {
      if (replicaId === currentLeader) return;
      const replicaNode = nodeMap.get(replicaId);
      const healthy = liveLeader?.online && replicaNode?.online;

      links.push({
        id: `raft-sh${sh.shardId}-${currentLeader}-${replicaId}`,
        source: currentLeader,
        destination: replicaId,
        type: 'raft',
        shardId: sh.shardId,
        status: healthy ? 'healthy' : 'disconnected',
        latencyMs: healthy ? 1.4 : undefined,
        lastContact: healthy ? '18 ms ago (AppendEntries heartbeat)' : 'Heartbeat lost',
        activeTraffic: healthy ? true : false,
      });
    });
  });

  return links;
}

export function calculateClusterSummary(nodes: PhysicalNode[]): ClusterSummaryData {
  const totalNodes = nodes.length;
  const availableNodes = nodes.filter((n) => n.online).length;
  const offlineNodes = totalNodes - availableNodes;

  // Shard health: each shard has RF=3, needs >=2 alive replicas for majority quorum
  let healthyShards = 0;
  let leaderlessShards = 0;

  SHARD_CONFIG.forEach((sh) => {
    const aliveReplicas = sh.replicas.filter((rid) => {
      const node = nodes.find((n) => n.id === rid);
      return node?.online;
    }).length;

    if (aliveReplicas >= 2) {
      healthyShards++;
    } else {
      leaderlessShards++;
    }
  });

  let replicationHealth: 'healthy' | 'degraded' | 'unavailable' = 'healthy';
  if (offlineNodes > 0 && healthyShards === 3) {
    replicationHealth = 'degraded';
  } else if (healthyShards < 3) {
    replicationHealth = 'unavailable';
  }

  return {
    totalNodes,
    availableNodes,
    offlineNodes,
    totalShards: 3,
    healthyShards,
    leaderlessShards,
    replicationHealth,
  };
}

export const INITIAL_TOPOLOGY_EVENTS: TopologyEvent[] = [
  {
    id: 'evt-1',
    timestamp: '07:30:21',
    nodeId: 'node-a',
    type: 'HEARTBEAT_ACK',
    category: 'NETWORK',
    description: 'Heartbeat ack received from Node B and Node C over TCP/HTTP mesh',
    level: 'info',
  },
  {
    id: 'evt-2',
    timestamp: '07:30:19',
    nodeId: 'node-d',
    type: 'ENTRY_COMMITTED',
    category: 'RAFT',
    description: 'Shard 1 committed log index 12450 across majority quorum (RF=3)',
    level: 'success',
  },
  {
    id: 'evt-3',
    timestamp: '07:30:16',
    nodeId: 'node-b',
    type: 'LEADER_ELECTED',
    category: 'RAFT',
    description: 'Shard 2 leader heartbeat confirmed for Term 5',
    level: 'info',
  },
  {
    id: 'evt-4',
    timestamp: '07:30:12',
    nodeId: 'node-e',
    type: 'REPLICA_CATCHUP_COMPLETED',
    category: 'STORAGE',
    description: 'Node E catch-up stream synchronized 4 entries from leader Node D',
    level: 'success',
  },
  {
    id: 'evt-5',
    timestamp: '07:30:08',
    nodeId: 'node-c',
    type: 'NODE_JOINED',
    category: 'CLUSTER',
    description: 'Physical node Node C membership verified on consistent hash ring',
    level: 'info',
  },
];
