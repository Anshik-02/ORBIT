'use client';

import React, { useState } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import TopologyHeader from '@/components/topology/TopologyHeader';
import TopologySummary from '@/components/topology/TopologySummary';
import TopologyControls, { CameraPreset } from '@/components/topology/TopologyControls';
import ClusterScene from '@/components/topology/ClusterScene';
import NodeInspector from '@/components/topology/NodeInspector';
import NodeInventory from '@/components/topology/NodeInventory';
import NetworkActivity from '@/components/topology/NetworkActivity';
import { useClusterTopology } from '@/hooks/useClusterTopology';

export default function TopologyPage() {
  const {
    nodes,
    filteredNodes,
    links,
    summary,
    events,
    selectedNodeId,
    selectedNode,
    hoveredLinkId,
    nodeFilter,
    shardFilter,
    viewMode,
    networkMode,
    isLiveUpdates,
    isLiveBackend,
    isSimulationMode,
    connectionState,
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
  } = useClusterTopology();

  const [cameraPreset, setCameraPreset] = useState<CameraPreset | null>(null);

  const handleCameraPreset = (preset: CameraPreset) => {
    setCameraPreset(preset);
    if (preset === 'reset') {
      setSelectedNodeId(null);
    }
  };

  return (
    <DashboardLayout activeNav="topology" onRefresh={refreshSnapshot}>
      <div className="space-y-6">
        {/* 01. PAGE HEADER */}
        <TopologyHeader
          connectionState={connectionState}
          nodeFilter={nodeFilter}
          shardFilter={shardFilter}
          isLiveUpdates={isLiveUpdates}
          onNodeFilterChange={setNodeFilter}
          onShardFilterChange={setShardFilter}
          onToggleLiveUpdates={() => setIsLiveUpdates(!isLiveUpdates)}
          onRefresh={refreshSnapshot}
        />

        {/* 02. CLUSTER SUMMARY CARDS */}
        <TopologySummary summary={summary} />

        {/* 03. INTERACTIVE 3D/2D CLUSTER EXPLORER & NODE INSPECTOR */}
        <div className="space-y-3">
          {/* Controls Bar with Camera Presets and Live/Demo Mode Switcher */}
          <TopologyControls
            viewMode={viewMode}
            networkMode={networkMode}
            isLiveBackend={isLiveBackend}
            isSimulationMode={isSimulationMode}
            onViewModeChange={setViewMode}
            onNetworkModeChange={setNetworkMode}
            onCameraPreset={handleCameraPreset}
            onToggleSimulationMode={toggleSimulationMode}
          />

          {/* 3D Visualizer Canvas & Node Inspector */}
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
            <div className={selectedNode ? 'lg:col-span-8' : 'lg:col-span-12'}>
              <ClusterScene
                nodes={filteredNodes}
                links={links}
                viewMode={viewMode}
                networkMode={networkMode}
                selectedNodeId={selectedNodeId}
                selectedShardId={shardFilter === 'all' ? 'all' : parseInt(shardFilter, 10)}
                cameraPreset={cameraPreset}
                onSelectNode={setSelectedNodeId}
                hoveredLinkId={hoveredLinkId}
                onHoverLink={setHoveredLinkId}
              />
            </div>

            {selectedNode && (
              <div className="lg:col-span-4">
                <NodeInspector
                  node={selectedNode}
                  onClose={() => setSelectedNodeId(null)}
                  onToggleOffline={toggleNodeOffline}
                />
              </div>
            )}
          </div>
        </div>

        {/* 04. NODE INVENTORY TABLE */}
        <NodeInventory
          nodes={nodes}
          selectedNodeId={selectedNodeId}
          onSelectNode={(id) => setSelectedNodeId(id === selectedNodeId ? null : id)}
          onToggleOffline={toggleNodeOffline}
        />

        {/* 05. NETWORK & CONSENSUS ACTIVITY AUDIT STREAM */}
        <NetworkActivity events={events} />
      </div>
    </DashboardLayout>
  );
}
