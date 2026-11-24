'use client';

import React from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import RaftLeadershipOverview from '@/components/dashboard/RaftLeadershipOverview';

export default function RaftPage() {
  return (
    <DashboardLayout activeNav="raft">
      <div className="space-y-6">
        <RaftLeadershipOverview />
      </div>
    </DashboardLayout>
  );
}
