'use client';

import React from 'react';
import {
  Crown,
  ShieldCheck,
  CheckCircle2,
  Clock,
  Layers,
  HardDrive,
  Activity,
  FileText,
} from 'lucide-react';

interface RaftStateInspectorProps {
  term: number;
  leaderId: string;
  commitIndex: number;
  lastApplied: number;
  role: 'Leader' | 'Follower' | 'Candidate';
  electionState: string;
  readIndexStatus: string;
  followers: Array<{
    id: string;
    name: string;
    httpPort: string;
    online: boolean;
    matchIndex: number;
    nextIndex: number;
    replicationLag: number;
    lastAck: string;
  }>;
}

export default function RaftStateInspector({
  term,
  leaderId,
  commitIndex,
  lastApplied,
  role = 'Leader',
  electionState = 'Stable Quorum',
  readIndexStatus = 'Linearizable ReadIndex Active',
  followers,
}: RaftStateInspectorProps) {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
      {/* 01. RAFT STATE INSPECTOR PANEL (5 cols) */}
      <div className="lg:col-span-5 bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between pb-3 mb-4 border-b border-[#DCE5F0]">
            <div>
              <h3 className="text-base font-bold text-[#17243A] font-mono">
                Raft State Inspector
              </h3>
              <p className="text-xs text-[#64748B] font-mono mt-0.5">
                Consensus state machine core properties
              </p>
            </div>
            <span className="px-2.5 py-1 rounded-full bg-amber-50 border border-amber-200 text-[#D97706] font-mono text-[10px] font-bold">
              {electionState}
            </span>
          </div>

          <div className="space-y-3 font-mono text-xs">
            <div className="flex items-center justify-between p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[#64748B]">Current Raft Term</span>
              <span className="font-bold text-[#17243A] text-sm">#{term}</span>
            </div>

            <div className="flex items-center justify-between p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[#64748B]">Recognized Leader</span>
              <span className="font-bold text-[#D97706] flex items-center gap-1">
                <Crown className="w-3.5 h-3.5 text-[#F59E0B]" />
                {leaderId.toUpperCase()}
              </span>
            </div>

            <div className="flex items-center justify-between p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[#64748B]">Latest Commit Index</span>
              <span className="font-bold text-[#0891D5]">
                {commitIndex.toLocaleString()}
              </span>
            </div>

            <div className="flex items-center justify-between p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[#64748B]">State Machine Applied Index</span>
              <span className="font-bold text-[#10B981]">
                {lastApplied.toLocaleString()}
              </span>
            </div>

            <div className="flex items-center justify-between p-2.5 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0]">
              <span className="text-[#64748B]">Consensus Role</span>
              <span className="font-bold text-[#17243A]">{role}</span>
            </div>

            <div className="flex items-center justify-between p-2.5 rounded-xl bg-blue-50/50 border border-blue-100 text-[#0891D5]">
              <span>Read Consistency</span>
              <span className="font-semibold">{readIndexStatus}</span>
            </div>
          </div>
        </div>

        <div className="mt-4 pt-3 border-t border-[#E2E8F0] flex items-center justify-between font-mono text-xs text-[#64748B]">
          <span>Election Timeout Range:</span>
          <span className="text-[#17243A] font-semibold">150ms – 300ms</span>
        </div>
      </div>

      {/* 02. REPLICATION PROGRESS TABLE (7 cols) */}
      <div className="lg:col-span-7 bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs flex flex-col justify-between">
        <div>
          <div className="flex items-center justify-between pb-3 mb-4 border-b border-[#DCE5F0]">
            <div>
              <h3 className="text-base font-bold text-[#17243A] font-mono">
                Follower Replication Progress
              </h3>
              <p className="text-xs text-[#64748B] font-mono mt-0.5">
                NextIndex & MatchIndex tracking for leader log catch-up
              </p>
            </div>
            <span className="px-2.5 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-[10px] font-bold">
              QUORUM SYNCHRONIZED
            </span>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left font-mono text-xs">
              <thead>
                <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase bg-[#F8FAFC]">
                  <th className="py-2.5 px-3">Follower Node</th>
                  <th className="py-2.5 px-3">Match Index</th>
                  <th className="py-2.5 px-3">Next Index</th>
                  <th className="py-2.5 px-3">Log Lag</th>
                  <th className="py-2.5 px-3 text-right">Last Ack</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#F1F5F9]">
                {followers.map((f) => (
                  <tr key={f.id} className="hover:bg-[#F8FAFC]">
                    <td className="py-3 px-3 font-bold text-[#17243A] flex items-center gap-2">
                      <span
                        className={`w-2 h-2 rounded-full ${
                          f.online ? 'bg-[#10B981]' : 'bg-[#EF4444]'
                        }`}
                      />
                      <span>{f.name}</span>
                      <span className="text-[11px] text-[#64748B] font-normal">
                        ({f.httpPort})
                      </span>
                    </td>
                    <td className="py-3 px-3 text-[#17243A] font-semibold">
                      {f.matchIndex.toLocaleString()}
                    </td>
                    <td className="py-3 px-3 text-[#0891D5] font-semibold">
                      {f.nextIndex.toLocaleString()}
                    </td>
                    <td className="py-3 px-3">
                      {f.replicationLag === 0 ? (
                        <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-[#10B981] border border-emerald-200 text-[10px] font-bold">
                          0 LAG (SYNCED)
                        </span>
                      ) : (
                        <span className="px-2 py-0.5 rounded-full bg-amber-50 text-[#D97706] border border-amber-200 text-[10px] font-bold">
                          {f.replicationLag} BEHIND
                        </span>
                      )}
                    </td>
                    <td className="py-3 px-3 text-right text-[#64748B]">
                      {f.online ? f.lastAck : 'Timeout'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="mt-4 pt-3 border-t border-[#E2E8F0] flex items-center justify-between font-mono text-xs text-[#64748B]">
          <span>Commit Quorum Rule:</span>
          <span className="text-[#0891D5] font-semibold">
            Median MatchIndex of Majority &gt;= CommitIndex
          </span>
        </div>
      </div>
    </div>
  );
}
