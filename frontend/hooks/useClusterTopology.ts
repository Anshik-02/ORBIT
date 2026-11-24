'use client';

import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import {
  PhysicalNode,
  NetworkLink,
  ClusterSummaryData,
  TopologyEvent,
  NodeFilterOption,
  ShardFilterOption,
  ViewMode,
  NetworkMode,
  ConnectionState,
} from '../lib/topology/types';
import {
  buildNodesData,
  transformLiveBackendToNodes,
  buildNetworkLinks,
  calculateClusterSummary,
  INITIAL_TOPOLOGY_EVENTS,
} from '../lib/topology/transforms';
import { subscribeToClusterEvents, fetchLiveClusterData } from '../lib/topology/api';

export function useClusterTopology() {
  // Mode: Real Go Backend vs Interactive Demo Simulation
  const [isLiveBackend, setIsLiveBackend] = useState<boolean>(false);
  const [isSimulationMode, setIsSimulationMode] = useState<boolean>(false);

  // Real backend payload state
  const [liveBackendData, setLiveBackendData] = useState<{
    health: any | null;
    shards: any | null;
    nodes: any | null;
  }>({ health: null, shards: null, nodes: null });

  // Simulation mode overrides
  const [simulationOfflineNodeIds, setSimulationOfflineNodeIds] = useState<Set<string>>(new Set());

  // Selection & UI state
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [hoveredLinkId, setHoveredLinkId] = useState<string | null>(null);

  // Filters & Modes
  const [nodeFilter, setNodeFilter] = useState<NodeFilterOption>('all');
  const [shardFilter, setShardFilter] = useState<ShardFilterOption>('all');
  const [viewMode, setViewMode] = useState<ViewMode>('3d');
  const [networkMode, setNetworkMode] = useState<NetworkMode>('physical');

  // Controls & Live Updates
  const [isLiveUpdates, setIsLiveUpdates] = useState<boolean>(true);
  const [connectionState, setConnectionState] = useState<ConnectionState>('connected');
  const [events, setEvents] = useState<TopologyEvent[]>(INITIAL_TOPOLOGY_EVENTS);
  const [lastRefreshed, setLastRefreshed] = useState<Date>(new Date());

  // Derive Nodes: from live backend if available & not in forced simulation mode, else local model
  const nodes = useMemo(() => {
    if (isLiveBackend && !isSimulationMode && liveBackendData.health) {
      return transformLiveBackendToNodes(
        liveBackendData.health,
        liveBackendData.shards,
        liveBackendData.nodes
      );
    }
    return buildNodesData(simulationOfflineNodeIds);
  }, [isLiveBackend, isSimulationMode, liveBackendData, simulationOfflineNodeIds]);

  // Derive Links
  const links = useMemo(() => {
    const selectedShard = shardFilter === 'all' ? 'all' : parseInt(shardFilter, 10);
    return buildNetworkLinks(nodes, networkMode, selectedShard);
  }, [nodes, networkMode, shardFilter]);

  // Derive Summary
  const summary: ClusterSummaryData = useMemo(() => {
    return calculateClusterSummary(nodes);
  }, [nodes]);

  // Filtered Nodes for Display
  const filteredNodes = useMemo(() => {
    return nodes.filter((node) => {
      // Shard filter
      if (shardFilter !== 'all') {
        const sid = parseInt(shardFilter, 10);
        const hostsShard = node.hostedShards.some((s) => s.shardId === sid);
        if (!hostsShard) return false;
      }

      // Node filter
      if (nodeFilter === 'online') return node.online;
      if (nodeFilter === 'offline') return !node.online;
      if (nodeFilter === 'leaders') return node.hostedShards.some((s) => s.role === 'Leader');
      if (nodeFilter === 'followers') return node.hostedShards.some((s) => s.role === 'Follower');

      return true;
    });
  }, [nodes, nodeFilter, shardFilter]);

  // Selected Node Object
  const selectedNode = useMemo(() => {
    if (!selectedNodeId) return null;
    return nodes.find((n) => n.id === selectedNodeId) || null;
  }, [nodes, selectedNodeId]);

  // Selected Link Object
  const hoveredLink = useMemo(() => {
    if (!hoveredLinkId) return null;
    return links.find((l) => l.id === hoveredLinkId) || null;
  }, [links, hoveredLinkId]);

  // Refresh Snapshot from Real Backend
  const refreshSnapshot = useCallback(async () => {
    setLastRefreshed(new Date());
    const data = await fetchLiveClusterData();

    if (data.isLive && data.health) {
      setIsLiveBackend(true);
      setConnectionState('connected');
      setLiveBackendData({
        health: data.health,
        shards: data.shards,
        nodes: data.nodes,
      });
      if (data.events && data.events.length > 0) {
        setEvents((prev) => {
          const existingIds = new Set(prev.map((e) => e.id));
          const newUnique = data.events!.filter((e) => !existingIds.has(e.id));
          return [...newUnique, ...prev].slice(0, 30);
        });
      }
    } else {
      // Real backend not reachable
      if (!isSimulationMode) {
        setIsLiveBackend(false);
      }
    }
  }, [isSimulationMode]);

  // Toggle Live Node Offline / Online (Simulation Mode)
  const toggleNodeOffline = useCallback((nodeId: string) => {
    setIsSimulationMode(true);
    setSimulationOfflineNodeIds((prev) => {
      const next = new Set(prev);
      const isCurrentlyOffline = next.has(nodeId);
      const nowStr = new Date().toLocaleTimeString('en-US', { hour12: false });

      if (isCurrentlyOffline) {
        next.delete(nodeId);
        setEvents((evts) => [
          {
            id: `chaos-rec-${Date.now()}`,
            timestamp: nowStr,
            nodeId,
            type: 'NODE_RECOVERED',
            category: 'CLUSTER',
            description: `Physical node ${nodeId} reconnected and rejoined peer mesh.`,
            level: 'success',
          },
          ...evts.slice(0, 24),
        ]);
      } else {
        next.add(nodeId);
        setEvents((evts) => [
          {
            id: `chaos-disc-${Date.now()}`,
            timestamp: nowStr,
            nodeId,
            type: 'NODE_DISCONNECTED',
            category: 'NETWORK',
            description: `Heartbeat lost: physical node ${nodeId} transitioned to OFFLINE.`,
            level: 'error',
          },
          ...evts.slice(0, 24),
        ]);
      }
      return next;
    });
  }, []);

  const toggleSimulationMode = useCallback(() => {
    setIsSimulationMode((prev) => {
      const next = !prev;
      if (!next) {
        setSimulationOfflineNodeIds(new Set());
        refreshSnapshot();
      }
      return next;
    });
  }, [refreshSnapshot]);

  // WebSocket Subscription & periodic poll
  useEffect(() => {
    if (!isLiveUpdates) return;

    refreshSnapshot();

    const unsubscribe = subscribeToClusterEvents(
      (newEvt) => {
        setEvents((prev) => [newEvt, ...prev.slice(0, 24)]);
        // When a cluster event arrives, refresh health snapshot
        refreshSnapshot();
      },
      (state) => {
        setConnectionState(state);
      }
    );

    // Periodic poll every 2.5s for live status
    const pollInterval = setInterval(() => {
      refreshSnapshot();
    }, 2500);

    return () => {
      unsubscribe();
      clearInterval(pollInterval);
    };
  }, [isLiveUpdates, refreshSnapshot]);

  return {
    nodes,
    filteredNodes,
    links,
    summary,
    events,
    selectedNodeId,
    selectedNode,
    hoveredLinkId,
    hoveredLink,
    nodeFilter,
    shardFilter,
    viewMode,
    networkMode,
    isLiveUpdates,
    isLiveBackend,
    isSimulationMode,
    connectionState,
    lastRefreshed,
    setSelectedNodeId,
    setHoveredLinkId,
    setNodeFilter,
    setShardFilter,
    setViewMode,
    setNetworkMode,
    setIsLiveUpdates,
    refreshSnapshot,
    toggleNodeOffline,
    toggleSimulationMode,
  };
}
