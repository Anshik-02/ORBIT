'use client';

import React from 'react';
import { Server, Crown, Eye, Power } from 'lucide-react';
import { PhysicalNode } from '../../lib/topology/types';

interface NodeInventoryProps {
  nodes: PhysicalNode[];
  selectedNodeId: string | null;
  onSelectNode: (nodeId: string) => void;
  onToggleOffline: (nodeId: string) => void;
}

export default function NodeInventory({
  nodes,
  selectedNodeId,
  onSelectNode,
  onToggleOffline,
}: NodeInventoryProps) {
  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs overflow-hidden">
      <div className="flex items-center justify-between pb-4 mb-4 border-b border-[#DCE5F0]">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
            <Server className="w-4 h-4 text-[#0891D5]" />
          </div>
          <div>
            <h3 className="text-base font-bold text-[#17243A] tracking-tight">
              Physical Node Inventory
            </h3>
            <p className="text-xs text-[#64748B]">
              Registered physical machines, network addresses & multi-raft participation
            </p>
          </div>
        </div>

        <span className="text-xs font-mono text-[#64748B] px-2.5 py-1 rounded-lg bg-[#F8FAFC] border border-[#E2E8F0]">
          {nodes.length} machines registered
        </span>
      </div>

      {/* Structured Table */}
      <div className="overflow-x-auto">
        <table className="w-full text-left font-mono text-xs">
          <thead>
            <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase tracking-wider">
              <th className="pb-3 font-semibold">Node</th>
              <th className="pb-3 font-semibold">Status</th>
              <th className="pb-3 font-semibold">HTTP Address</th>
              <th className="pb-3 font-semibold">Hosted Shards</th>
              <th className="pb-3 font-semibold">Raft Groups</th>
              <th className="pb-3 font-semibold">Last Activity</th>
              <th className="pb-3 font-semibold text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9]">
            {nodes.map((node) => {
              const isSelected = selectedNodeId === node.id;

              return (
                <tr
                  key={node.id}
                  onClick={() => onSelectNode(node.id)}
                  className={`cursor-pointer transition-all ${
                    isSelected
                      ? 'bg-sky-50/80 font-medium'
                      : 'hover:bg-[#F8FAFC]'
                  }`}
                >
                  {/* Node */}
                  <td className="py-3.5 pr-4">
                    <div className="flex items-center gap-2">
                      <span
                        className={`w-2 h-2 rounded-full ${
                          node.online ? 'bg-[#10B981]' : 'bg-[#EF4444]'
                        }`}
                      />
                      <span className="font-bold text-[#17243A] text-sm">
                        {node.name}
                      </span>
                      <span className="text-[11px] text-[#64748B]">
                        ({node.id})
                      </span>
                    </div>
                  </td>

                  {/* Status */}
                  <td className="py-3.5 pr-4">
                    {node.online ? (
                      <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-semibold text-[11px]">
                        <span className="w-1.5 h-1.5 rounded-full bg-[#10B981]" />
                        Online
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-rose-50 border border-rose-200 text-[#EF4444] font-semibold text-[11px]">
                        <span className="w-1.5 h-1.5 rounded-full bg-[#EF4444]" />
                        Offline
                      </span>
                    )}
                  </td>

                  {/* HTTP Address */}
                  <td className="py-3.5 pr-4 text-[#0891D5] font-semibold">
                    {node.httpAddress}
                  </td>

                  {/* Hosted Shards */}
                  <td className="py-3.5 pr-4">
                    <div className="flex items-center gap-1.5 flex-wrap">
                      {node.hostedShards.map((sh) => (
                        <span
                          key={sh.shardId}
                          className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] border"
                          style={{
                            backgroundColor: `${sh.color}15`,
                            borderColor: `${sh.color}40`,
                            color: sh.color,
                          }}
                        >
                          {sh.shardName}
                          {sh.role === 'Leader' && (
                            <Crown className="w-3 h-3 text-[#F59E0B]" />
                          )}
                        </span>
                      ))}
                    </div>
                  </td>

                  {/* Raft Groups */}
                  <td className="py-3.5 pr-4 text-[#17243A]">
                    {node.raftGroupCount} groups{' '}
                    <span className="text-[#64748B] text-[11px]">
                      ({node.leaderGroupCount}L, {node.followerGroupCount}F)
                    </span>
                  </td>

                  {/* Last Activity */}
                  <td className="py-3.5 pr-4 text-[#64748B]">
                    {node.lastObservedActivity}
                  </td>

                  {/* Actions */}
                  <td className="py-3.5 text-right">
                    <div
                      className="inline-flex items-center gap-2"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <button
                        onClick={() => onSelectNode(node.id)}
                        className="p-1.5 rounded-lg bg-white hover:bg-slate-100 border border-[#DCE5F0] text-[#0891D5]"
                        title="Inspect Node"
                      >
                        <Eye className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => onToggleOffline(node.id)}
                        className={`p-1.5 rounded-lg border text-xs ${
                          node.online
                            ? 'bg-rose-50 hover:bg-rose-100/80 border-rose-200 text-rose-600'
                            : 'bg-emerald-50 hover:bg-emerald-100/80 border-emerald-200 text-emerald-700'
                        }`}
                        title={node.online ? 'Simulate Crash' : 'Restart'}
                      >
                        <Power className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
