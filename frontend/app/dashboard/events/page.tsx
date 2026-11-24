'use client';

import React from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import RecentClusterEvents from '@/components/dashboard/RecentClusterEvents';

export default function EventsPage() {
  return (
    <DashboardLayout activeNav="events">
      <div className="space-y-6">
        <RecentClusterEvents />
      </div>
    </DashboardLayout>
  );
}
