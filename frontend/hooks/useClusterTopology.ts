'use client';

import { useState, useEffect, useMemo, useCallback } from 'react';
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
  buildNetworkLinks,
  calculateClusterSummary,
  INITIAL_TOPOLOGY_EVENTS,
} from '../lib/topology/transforms';
import { subscribeToClusterEvents, fetchClusterHealth } from '../lib/topology/api';

export function useClusterTopology() {
  const [offlineNodeIds, setOfflineNodeIds] = useState<Set<string>>(new Set());
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

  // Derive Nodes
  const nodes = useMemo(() => {
    return buildNodesData(offlineNodeIds);
  }, [offlineNodeIds]);

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

  // Refresh Snapshot Handler
  const refreshSnapshot = useCallback(async () => {
    setLastRefreshed(new Date());
    const data = await fetchClusterHealth();
    if (data && data.nodes) {
      setConnectionState('connected');
      const offIds = new Set<string>();
      Object.keys(data.nodes).forEach((k) => {
        if (!data.nodes[k].online) {
          offIds.add(k);
        }
      });
      setOfflineNodeIds(offIds);
    }
  }, []);

  // Toggle Live Node Offline / Online (Chaos Simulation)
  const toggleNodeOffline = useCallback((nodeId: string) => {
    setOfflineNodeIds((prev) => {
      const next = new Set(prev);
      const isCurrentlyOffline = next.has(nodeId);
      const nowStr = new Date().toLocaleTimeString('en-US', { hour12: false });

      if (isCurrentlyOffline) {
        next.delete(nodeId);
        // Add Recovery Event
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
          ...evts.slice(0, 19),
        ]);
      } else {
        next.add(nodeId);
        // Add Disconnect Event
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
          ...evts.slice(0, 19),
        ]);
      }
      return next;
    });
  }, []);

  // WebSocket Subscription & periodic poll
  useEffect(() => {
    if (!isLiveUpdates) return;

    const unsubscribe = subscribeToClusterEvents(
      (newEvt) => {
        setEvents((prev) => [newEvt, ...prev.slice(0, 19)]);
      },
      (state) => {
        setConnectionState(state);
      }
    );

    // Initial check
    refreshSnapshot();

    return () => {
      unsubscribe();
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
  };
}
