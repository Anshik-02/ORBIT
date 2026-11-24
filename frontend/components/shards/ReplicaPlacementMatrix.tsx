'use client';

import React from 'react';
import { Crown, Check, CheckCircle2 } from 'lucide-react';

interface MatrixNode {
  id: string;
  name: string;
  httpPort: string;
  roles: { [shardId: number]: 'Leader' | 'Follower' | null };
}

interface ReplicaPlacementMatrixProps {
  nodes: MatrixNode[];
  selectedShardId: number;
  onSelectShard: (shardId: number) => void;
}

export default function ReplicaPlacementMatrix({
  nodes,
  selectedShardId,
  onSelectShard,
}: ReplicaPlacementMatrixProps) {
  const shards = [
    { id: 0, name: 'Shard 0', color: '#0891D5', label: 'Keys 0x00..0x55' },
    { id: 1, name: 'Shard 1', color: '#7965E8', label: 'Keys 0x55..0xAA' },
    { id: 2, name: 'Shard 2', color: '#10B981', label: 'Keys 0xAA..0xFF' },
  ];

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs overflow-hidden">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 mb-4">
        <div>
          <h3 className="text-base font-bold text-[#17243A] font-mono">
            Replica Placement Matrix
          </h3>
          <p className="text-xs text-[#64748B] font-mono mt-0.5">
            Cross-node allocation of 3 logical partitions with Replication Factor (RF=3)
          </p>
        </div>

        <div className="flex items-center gap-3 font-mono text-[11px] text-[#64748B]">
          <span className="flex items-center gap-1.5">
            <span className="w-2.5 h-2.5 rounded-full bg-[#F59E0B]" /> Shard Leader
          </span>
          <span className="flex items-center gap-1.5">
            <span className="w-2.5 h-2.5 rounded-full bg-[#0891D5]" /> Shard Follower
          </span>
          <span className="flex items-center gap-1.5">
            <span className="text-[#94A3B8]">—</span> Not Hosted
          </span>
        </div>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left border-collapse font-mono text-xs">
          <thead>
            <tr className="border-b border-[#DCE5F0] bg-[#F8FAFC]">
              <th className="py-3 px-4 font-semibold text-[#64748B] uppercase tracking-wider text-[11px]">
                Physical Machine
              </th>
              {shards.map((sh) => (
                <th
                  key={sh.id}
                  onClick={() => onSelectShard(sh.id)}
                  className={`py-3 px-4 font-semibold text-center cursor-pointer transition-all ${
                    selectedShardId === sh.id
                      ? 'bg-blue-50/60 text-[#0891D5] border-b-2 border-[#0891D5]'
                      : 'text-[#17243A] hover:bg-slate-100/60'
                  }`}
                >
                  <div className="flex items-center justify-center gap-1.5">
                    <span
                      className="w-2.5 h-2.5 rounded-full"
                      style={{ backgroundColor: sh.color }}
                    />
                    <span>{sh.name}</span>
                  </div>
                  <span className="text-[10px] text-[#64748B] font-normal block mt-0.5">
                    {sh.label}
                  </span>
                </th>
              ))}
              <th className="py-3 px-4 font-semibold text-right text-[#64748B] uppercase tracking-wider text-[11px]">
                Replica Load
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9]">
            {nodes.map((node) => {
              const hostedCount = Object.values(node.roles).filter(Boolean).length;
              return (
                <tr key={node.id} className="hover:bg-[#F8FAFC]/80 transition-colors">
                  <td className="py-3.5 px-4 font-bold text-[#17243A] flex items-center gap-2">
                    <span className="w-2 h-2 rounded-full bg-[#10B981]" />
                    <span>{node.name}</span>
                    <span className="text-[11px] text-[#64748B] font-normal">
                      ({node.httpPort})
                    </span>
                  </td>

                  {shards.map((sh) => {
                    const role = node.roles[sh.id];
                    const isSelectedCol = selectedShardId === sh.id;

                    return (
                      <td
                        key={sh.id}
                        onClick={() => onSelectShard(sh.id)}
                        className={`py-3.5 px-4 text-center cursor-pointer transition-colors ${
                          isSelectedCol ? 'bg-blue-50/30' : ''
                        }`}
                      >
                        {role === 'Leader' ? (
                          <span className="inline-flex items-center gap-1 px-3 py-1 rounded-full bg-amber-50 border border-amber-200 text-[#D97706] font-bold text-[11px] shadow-2xs">
                            <Crown className="w-3.5 h-3.5 text-[#F59E0B]" />
                            <span>LEADER</span>
                          </span>
                        ) : role === 'Follower' ? (
                          <span className="inline-flex items-center gap-1 px-3 py-1 rounded-full bg-slate-100 border border-slate-200 text-[#64748B] font-semibold text-[11px]">
                            <Check className="w-3 h-3 text-[#10B981]" />
                            <span>FOLLOWER</span>
                          </span>
                        ) : (
                          <span className="text-[#94A3B8] font-bold text-sm">—</span>
                        )}
                      </td>
                    );
                  })}

                  <td className="py-3.5 px-4 text-right">
                    <span className="px-2.5 py-0.5 rounded-md bg-[#F4F7FC] border border-[#DCE5F0] font-bold text-[#17243A]">
                      {hostedCount} / 3 Shards
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
          <tfoot>
            <tr className="border-t-2 border-[#DCE5F0] bg-[#F8FAFC] font-semibold">
              <td className="py-3 px-4 text-[#64748B]">Active Quorum (RF=3)</td>
              {shards.map((sh) => (
                <td key={sh.id} className="py-3 px-4 text-center text-[#10B981]">
                  <span className="inline-flex items-center gap-1 font-bold">
                    <CheckCircle2 className="w-3.5 h-3.5" />
                    3 / 3 Replicas
                  </span>
                </td>
              ))}
              <td className="py-3 px-4 text-right text-[#0891D5] font-bold">
                9 Total Replicas
              </td>
            </tr>
          </tfoot>
        </table>
      </div>
    </div>
  );
}
