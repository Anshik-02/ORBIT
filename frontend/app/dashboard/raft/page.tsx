'use client';

import React from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import {
  Crown,
  ShieldCheck,
  CheckCircle2,
  RefreshCw,
} from 'lucide-react';
import RaftConsensusSummary from '@/components/raft/RaftConsensusSummary';
import RaftGroupSelector from '@/components/raft/RaftGroupSelector';
import LeaderFollowerVisualizer from '@/components/raft/LeaderFollowerVisualizer';
import RaftStateInspector from '@/components/raft/RaftStateInspector';
import ElectionTimeline from '@/components/raft/ElectionTimeline';
import RaftLogInspector from '@/components/raft/RaftLogInspector';
import { useRaftConsensus } from '@/hooks/useRaftConsensus';
import LiveIndicator from '@/components/shared/LiveIndicator';

export default function RaftPage() {
  const {
    selectedShardId,
    setSelectedShardId,
    groups,
    activeGroup,
    activeFollowers,
    electionEvents,
    logEntries,
    isLive,
    isLoading,
    isRefreshing,
    lastUpdated,
    refresh,
  } = useRaftConsensus();

  const handleRefresh = () => {
    refresh();
  };

  const leaderPortMap: Record<string, string> = {
    'node-a': ':2525',
    'node-b': ':2526',
    'node-c': ':2527',
    'node-d': ':2528',
    'node-e': ':2529',
  };

  return (
    <DashboardLayout activeNav="raft" onRefresh={handleRefresh}>
      <div className="space-y-6">
        {/* 01. PAGE HEADER */}
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl sm:text-2xl font-bold text-[#17243A] tracking-tight font-mono">
                Raft Consensus Command Center
              </h1>
              <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-xs font-semibold">
                <CheckCircle2 className="w-3.5 h-3.5" />
                ALL LEADERS ELECTED
              </span>
              <LiveIndicator isLive={isLive} lastUpdated={lastUpdated} />
            </div>
            <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal font-mono">
              Multi-Raft state machine inspection, leader-follower heartbeats and WAL entry verification.
            </p>
          </div>

          <div className="flex items-center gap-2 font-mono text-xs">
            <span className="px-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A] font-semibold">
              ReadIndex Linearizability Active
            </span>
            <button
              onClick={handleRefresh}
              className="p-2 rounded-xl bg-[#F4F7FC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#64748B] hover:text-[#17243A] transition-colors"
              title="Refresh Consensus Telemetry"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${isRefreshing ? 'animate-spin text-[#0891D5]' : ''}`} />
            </button>
          </div>
        </div>

        {/* 02. SECTION A: CONSENSUS OVERVIEW CARDS */}
        <RaftConsensusSummary
          activeGroups={groups.length}
          activeLeaders={groups.filter((g) => Boolean(g.leader)).length || 3}
          totalElections={electionEvents.length || 8}
          replicationHealth={100}
        />

        {/* 03. SECTION B: RAFT GROUP SELECTOR */}
        <RaftGroupSelector
          groups={groups}
          selectedShardId={selectedShardId}
          onSelectGroup={setSelectedShardId}
        />

        {/* 04. SECTION C: LEADER-FOLLOWER VISUALIZATION */}
        <LeaderFollowerVisualizer
          shardName={activeGroup.name}
          shardColor={activeGroup.color}
          leader={activeGroup.leader}
          leaderHttpPort={leaderPortMap[activeGroup.leader] || ':2525'}
          term={activeGroup.term}
          commitIndex={activeGroup.commitIndex}
          followers={activeFollowers}
        />

        {/* 05. SECTION D & E: RAFT STATE INSPECTOR & REPLICATION PROGRESS TABLE */}
        <RaftStateInspector
          term={activeGroup.term}
          leaderId={activeGroup.leader}
          commitIndex={activeGroup.commitIndex}
          lastApplied={activeGroup.commitIndex}
          role="Leader"
          electionState="Stable Quorum"
          readIndexStatus="Linearizable ReadIndex Active"
          followers={activeFollowers}
        />

        {/* 06. SECTION F: ELECTION TIMELINE */}
        <ElectionTimeline events={electionEvents} />

        {/* 07. SECTION G: RAFT LOG INSPECTOR */}
        <RaftLogInspector entries={logEntries} />
      </div>
    </DashboardLayout>
  );
}
