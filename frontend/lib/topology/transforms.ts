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

// Helper to format ISO timestamp into human relative string without inventing fake metrics
export function formatRelativeTime(isoString?: string): string {
  if (!isoString) return 'Active';
  try {
    const d = new Date(isoString);
    const now = Date.now();
    const diffMs = now - d.getTime();
    if (diffMs < 0 || diffMs < 1000) return 'Just now';
    if (diffMs < 60000) return `${Math.floor(diffMs / 1000)}s ago`;
    if (diffMs < 3600000) return `${Math.floor(diffMs / 60000)}m ago`;
    return d.toLocaleTimeString('en-US', { hour12: false });
  } catch {
    return 'Active';
  }
}

// Transform real live Go backend /cluster/health response into PhysicalNode[]
export function transformLiveBackendToNodes(
  health: any,
  shardsData?: any,
  nodesData?: any
): PhysicalNode[] {
  const nodeIds =
    nodesData?.nodes ||
    (health?.nodes ? Object.keys(health.nodes) : ['node-a', 'node-b', 'node-c', 'node-d', 'node-e']);

  const shardsList = shardsData?.shards || [];
  const shardsMap = health?.shards || {};

  return nodeIds.map((id: string) => {
    const cfg = NODE_NETWORK_CONFIG[id];
    const backendNode = health?.nodes ? health.nodes[id] : null;

    const isOnline = backendNode ? backendNode.online : true;
    const hostedShardIds: number[] = backendNode?.hosted_shard_ids || [];

    // Map hosted shards
    const hostedShards: HostedShardInfo[] = [];

    // If backend gave hosted_shard_ids, use them; otherwise check shardsList or SHARD_CONFIG
    const targetShardIds =
      hostedShardIds.length > 0
        ? hostedShardIds
        : SHARD_CONFIG.filter((s) => s.replicas.includes(id)).map((s) => s.shardId);

    targetShardIds.forEach((sid: number) => {
      const defaultCfg = SHARD_CONFIG.find((s) => s.shardId === sid);
      const shardDetail = shardsList.find((s: any) => s.id === sid);
      const liveShardStatus = shardsMap[sid];

      // Determine real leader
      let leaderId = liveShardStatus?.current_leader || shardDetail?.leader || defaultCfg?.defaultLeader || '';
      const isLeader = isOnline && leaderId === id;

      hostedShards.push({
        shardId: sid,
        shardName: defaultCfg?.name || `Shard ${sid}`,
        role: isLeader ? 'Leader' : 'Follower',
        term: liveShardStatus?.current_term || defaultCfg?.term || 1,
        commitIndex: liveShardStatus?.commit_index || defaultCfg?.commitIndex || 0,
        appliedIndex: liveShardStatus?.last_applied_index || defaultCfg?.commitIndex || 0,
        color: defaultCfg?.color || '#0891D5',
      });
    });

    const leaderGroups = hostedShards.filter((s) => s.role === 'Leader').length;
    const followerGroups = hostedShards.filter((s) => s.role === 'Follower').length;

    return {
      id,
      name: cfg?.name || id,
      status: isOnline ? 'online' : 'offline',
      online: isOnline,
      httpAddress: `127.0.0.1:${cfg?.httpPort || 2525}`,
      raftAddress: `127.0.0.1:${cfg?.raftPort || 3525}`,
      hostedShards,
      raftGroupCount: hostedShards.length,
      leaderGroupCount: leaderGroups,
      followerGroupCount: followerGroups,
      lastObservedActivity: isOnline
        ? formatRelativeTime(backendNode?.last_observed_activity)
        : 'Heartbeat lost (Offline)',
      lastHeartbeatMs: 0,
      position3D: DETERMINISTIC_3D_POSITIONS[id] || [0, 0, 0],
      position2D: DETERMINISTIC_2D_POSITIONS[id] || { x: 50, y: 50 },
    };
  });
}

// Build node objects for local simulation mode
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
      httpAddress: `127.0.0.1:${cfg?.httpPort || 2525}`,
      raftAddress: `127.0.0.1:${cfg?.raftPort || 3525}`,
      hostedShards,
      raftGroupCount: hostedShards.length,
      leaderGroupCount: leaderGroups,
      followerGroupCount: followerGroups,
      lastObservedActivity: isOnline ? 'Active (Demo)' : 'Disconnected',
      lastHeartbeatMs: 0,
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
    return PHYSICAL_NETWORK_PEERS.map((peer) => {
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
        lastContact: bothOnline ? 'Connected' : 'Connection unreachable',
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

    // Determine current leader from node hosted shard definitions
    let currentLeader = sh.defaultLeader;
    nodes.forEach((n) => {
      const matching = n.hostedShards.find((s) => s.shardId === sh.shardId);
      if (matching && matching.role === 'Leader' && n.online) {
        currentLeader = n.id;
      }
    });

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
        lastContact: healthy ? 'Active AppendEntries stream' : 'Heartbeat lost',
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
    timestamp: '14:30:21',
    nodeId: 'node-a',
    type: 'HEARTBEAT_ACK',
    category: 'NETWORK',
    description: 'Heartbeat ack received across TCP peer transport mesh',
    level: 'info',
  },
  {
    id: 'evt-2',
    timestamp: '14:30:19',
    nodeId: 'node-d',
    type: 'ENTRY_COMMITTED',
    category: 'RAFT',
    description: 'Shard 1 committed log index across majority quorum (RF=3)',
    level: 'success',
  },
  {
    id: 'evt-3',
    timestamp: '14:30:16',
    nodeId: 'node-b',
    type: 'LEADER_ELECTED',
    category: 'RAFT',
    description: 'Shard 2 leader election confirmed',
    level: 'info',
  },
  {
    id: 'evt-4',
    timestamp: '14:30:12',
    nodeId: 'node-e',
    type: 'REPLICA_CATCHUP_COMPLETED',
    category: 'STORAGE',
    description: 'Node E catch-up stream synchronized from leader Node D',
    level: 'success',
  },
  {
    id: 'evt-5',
    timestamp: '14:30:08',
    nodeId: 'node-c',
    type: 'NODE_JOINED',
    category: 'CLUSTER',
    description: 'Physical node Node C membership confirmed on hash ring',
    level: 'info',
  },
];
