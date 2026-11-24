'use client';

import React, { useState } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import {
  Crown,
  ShieldCheck,
  CheckCircle2,
  RefreshCw,
  Zap,
} from 'lucide-react';
import RaftConsensusSummary from '@/components/raft/RaftConsensusSummary';
import RaftGroupSelector, { RaftGroupInfo } from '@/components/raft/RaftGroupSelector';
import LeaderFollowerVisualizer from '@/components/raft/LeaderFollowerVisualizer';
import RaftStateInspector from '@/components/raft/RaftStateInspector';
import ElectionTimeline, { ElectionEvent } from '@/components/raft/ElectionTimeline';
import RaftLogInspector, { RaftLogEntry } from '@/components/raft/RaftLogInspector';

const RAFT_GROUPS: RaftGroupInfo[] = [
  {
    shardId: 0,
    name: 'Shard 0 (0x00..0x55)',
    color: '#0891D5',
    leader: 'node-a',
    term: 3,
    commitIndex: 14820,
    replicas: ['node-a', 'node-b', 'node-c'],
  },
  {
    shardId: 1,
    name: 'Shard 1 (0x55..0xAA)',
    color: '#7965E8',
    leader: 'node-d',
    term: 2,
    commitIndex: 14818,
    replicas: ['node-d', 'node-e', 'node-a'],
  },
  {
    shardId: 2,
    name: 'Shard 2 (0xAA..0xFF)',
    color: '#10B981',
    leader: 'node-b',
    term: 4,
    commitIndex: 14822,
    replicas: ['node-b', 'node-c', 'node-d'],
  },
];

const FOLLOWER_DATA: { [shardId: number]: any[] } = {
  0: [
    {
      id: 'node-b',
      name: 'Node B',
      httpPort: ':2526',
      online: true,
      matchIndex: 14820,
      nextIndex: 14821,
      replicationLag: 0,
      latency: '0.7ms',
      lastAck: '12ms ago',
      lastHeartbeat: '42ms ago',
    },
    {
      id: 'node-c',
      name: 'Node C',
      httpPort: ':2527',
      online: true,
      matchIndex: 14820,
      nextIndex: 14821,
      replicationLag: 0,
      latency: '0.9ms',
      lastAck: '15ms ago',
      lastHeartbeat: '48ms ago',
    },
  ],
  1: [
    {
      id: 'node-e',
      name: 'Node E',
      httpPort: ':2529',
      online: true,
      matchIndex: 14818,
      nextIndex: 14819,
      replicationLag: 0,
      latency: '1.1ms',
      lastAck: '22ms ago',
      lastHeartbeat: '51ms ago',
    },
    {
      id: 'node-a',
      name: 'Node A',
      httpPort: ':2525',
      online: true,
      matchIndex: 14818,
      nextIndex: 14819,
      replicationLag: 0,
      latency: '0.8ms',
      lastAck: '16ms ago',
      lastHeartbeat: '45ms ago',
    },
  ],
  2: [
    {
      id: 'node-c',
      name: 'Node C',
      httpPort: ':2527',
      online: true,
      matchIndex: 14822,
      nextIndex: 14823,
      replicationLag: 0,
      latency: '0.6ms',
      lastAck: '11ms ago',
      lastHeartbeat: '39ms ago',
    },
    {
      id: 'node-d',
      name: 'Node D',
      httpPort: ':2528',
      online: true,
      matchIndex: 14822,
      nextIndex: 14823,
      replicationLag: 0,
      latency: '0.8ms',
      lastAck: '14ms ago',
      lastHeartbeat: '43ms ago',
    },
  ],
};

const INITIAL_EVENTS: ElectionEvent[] = [
  {
    id: 'evt-1',
    timestamp: '14:30:16',
    type: 'leader_elected',
    nodeId: 'node-a',
    term: 3,
    description: 'Node A won majority quorum (2/3 votes) and transitioned to LEADER for Shard 0.',
  },
  {
    id: 'evt-2',
    timestamp: '14:30:15',
    type: 'vote_granted',
    nodeId: 'node-b',
    term: 3,
    description: 'Node B granted RequestVote to candidate Node A (candidate log is up-to-date).',
  },
  {
    id: 'evt-3',
    timestamp: '14:30:14',
    type: 'election_started',
    nodeId: 'node-a',
    term: 3,
    description: 'Node A election timer expired (210ms). Incremented term to #3 and broadcast RequestVote.',
  },
  {
    id: 'evt-4',
    timestamp: '14:30:08',
    type: 'term_updated',
    nodeId: 'node-d',
    term: 2,
    description: 'Shard 1 stabilized on Term #2 under Leader Node D with 3 active replicas.',
  },
];

const INITIAL_LOG_ENTRIES: RaftLogEntry[] = [
  {
    index: 14820,
    term: 3,
    type: 'KV_PUT',
    payloadSummary: 'PUT key="user:9982" (len: 64B)',
    committed: true,
    applied: true,
    timestamp: '14:30:22',
  },
  {
    index: 14819,
    term: 3,
    type: 'KV_PUT',
    payloadSummary: 'PUT key="session:0a12" (len: 128B)',
    committed: true,
    applied: true,
    timestamp: '14:30:20',
  },
  {
    index: 14818,
    term: 3,
    type: 'NO_OP',
    payloadSummary: 'Leader Election Barrier (Term #3)',
    committed: true,
    applied: true,
    timestamp: '14:30:16',
  },
  {
    index: 14817,
    term: 2,
    type: 'KV_DELETE',
    payloadSummary: 'DELETE key="temp:expired_lock"',
    committed: true,
    applied: true,
    timestamp: '14:28:44',
  },
  {
    index: 14816,
    term: 2,
    type: 'CONF_CHANGE',
    payloadSummary: 'Joint Consensus Placement Confirmed',
    committed: true,
    applied: true,
    timestamp: '14:25:10',
  },
];

export default function RaftPage() {
  const [selectedShardId, setSelectedShardId] = useState<number>(0);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const selectedGroup =
    RAFT_GROUPS.find((g) => g.shardId === selectedShardId) || RAFT_GROUPS[0];
  const followers = FOLLOWER_DATA[selectedShardId] || FOLLOWER_DATA[0];

  const handleRefresh = () => {
    setIsRefreshing(true);
    setTimeout(() => {
      setIsRefreshing(false);
    }, 400);
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
          activeGroups={3}
          activeLeaders={3}
          totalElections={8}
          replicationHealth={100}
        />

        {/* 03. SECTION B: RAFT GROUP SELECTOR */}
        <RaftGroupSelector
          groups={RAFT_GROUPS}
          selectedShardId={selectedShardId}
          onSelectGroup={setSelectedShardId}
        />

        {/* 04. SECTION C: LEADER-FOLLOWER VISUALIZATION */}
        <LeaderFollowerVisualizer
          shardName={selectedGroup.name}
          shardColor={selectedGroup.color}
          leader={selectedGroup.leader}
          leaderHttpPort={selectedGroup.leader === 'node-a' ? ':2525' : selectedGroup.leader === 'node-b' ? ':2526' : ':2528'}
          term={selectedGroup.term}
          commitIndex={selectedGroup.commitIndex}
          followers={followers}
        />

        {/* 05. SECTION D & E: RAFT STATE INSPECTOR & REPLICATION PROGRESS TABLE */}
        <RaftStateInspector
          term={selectedGroup.term}
          leaderId={selectedGroup.leader}
          commitIndex={selectedGroup.commitIndex}
          lastApplied={selectedGroup.commitIndex}
          role="Leader"
          electionState="Stable Quorum"
          readIndexStatus="Linearizable ReadIndex Active"
          followers={followers}
        />

        {/* 06. SECTION F: ELECTION TIMELINE */}
        <ElectionTimeline events={INITIAL_EVENTS} />

        {/* 07. SECTION G: RAFT LOG INSPECTOR */}
        <RaftLogInspector entries={INITIAL_LOG_ENTRIES} />
      </div>
    </DashboardLayout>
  );
}
