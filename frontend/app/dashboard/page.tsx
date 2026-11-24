'use client';

import React from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import ClusterHealthCards from '@/components/dashboard/ClusterHealthCards';
import ClusterTopologyPreview from '@/components/dashboard/ClusterTopologyPreview';
import NodeStatusPanel from '@/components/dashboard/NodeStatusPanel';
import PerformanceCharts from '@/components/dashboard/PerformanceCharts';
import RaftLeadershipOverview from '@/components/dashboard/RaftLeadershipOverview';
import RecentClusterEvents from '@/components/dashboard/RecentClusterEvents';
import ClusterActivitySummary from '@/components/dashboard/ClusterActivitySummary';

export default function DashboardOverviewPage() {
  return (
    <DashboardLayout activeNav="overview">
      <div className="space-y-6">
        {/* Section 1: Cluster Health Metrics */}
        <section>
          <ClusterHealthCards />
        </section>

        {/* Section 2: Topology Preview & Fleet Node Status */}
        <section className="grid grid-cols-1 xl:grid-cols-3 gap-6">
          <div className="xl:col-span-2">
            <ClusterTopologyPreview />
          </div>
          <div>
            <NodeStatusPanel />
          </div>
        </section>

        {/* Section 3: Performance Telemetry (Throughput & Latency) */}
        <section>
          <PerformanceCharts />
        </section>

        {/* Section 4: Raft Consensus & Leadership Overview */}
        <section>
          <RaftLeadershipOverview />
        </section>

        {/* Section 5: Recent Cluster Events & 24h Activity Breakdown */}
        <section className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <RecentClusterEvents />
          <ClusterActivitySummary />
        </section>
      </div>
    </DashboardLayout>
  );
}
