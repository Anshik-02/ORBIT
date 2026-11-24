'use client';

import React, { useState } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { Terminal, Play, ArrowRight, CornerDownLeft } from 'lucide-react';

export default function QueryConsolePage() {
  const [key, setKey] = useState('user:1001');
  const [val, setVal] = useState('{"name":"Alice","tier":"enterprise"}');
  const [method, setMethod] = useState<'GET' | 'PUT' | 'DELETE'>('GET');
  const [responseOutput, setResponseOutput] = useState<string>(
    JSON.stringify(
      {
        status: 200,
        key: 'user:1001',
        value: '{"name":"Alice","tier":"enterprise"}',
        shard: 0,
        leader: 'node-a',
        linearizable: true,
        rtt: '2.4ms',
      },
      null,
      2
    )
  );

  const handleExecute = () => {
    setResponseOutput(
      JSON.stringify(
        {
          status: 200,
          operation: method,
          key,
          value: method === 'DELETE' ? undefined : val,
          timestamp: new Date().toISOString(),
          shard: 0,
          leader: 'node-a',
          raftTerm: 4,
          quorumAck: '3/3 nodes',
        },
        null,
        2
      )
    );
  };

  return (
    <DashboardLayout activeNav="query">
      <div className="space-y-6">
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs">
          <div className="flex items-center gap-2 mb-1">
            <div className="w-8 h-8 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
              <Terminal className="w-4 h-4 text-[#0891D5]" />
            </div>
            <h1 className="text-xl font-bold text-[#17243A]">Interactive Query Console</h1>
          </div>
          <p className="text-xs text-[#64748B]">
            Execute linearizable key-value operations across consensus multi-raft groups
          </p>

          <div className="mt-5 grid grid-cols-1 lg:grid-cols-12 gap-4">
            <div className="lg:col-span-8 space-y-4">
              <div className="flex gap-2">
                <select
                  value={method}
                  onChange={(e) => setMethod(e.target.value as any)}
                  className="bg-[#F8FAFC] border border-[#DCE5F0] rounded-xl px-3 py-2 text-xs font-mono font-bold text-[#17243A]"
                >
                  <option value="GET">GET</option>
                  <option value="PUT">PUT</option>
                  <option value="DELETE">DELETE</option>
                </select>
                <input
                  type="text"
                  value={key}
                  onChange={(e) => setKey(e.target.value)}
                  placeholder="/kv/{key}"
                  className="flex-1 bg-[#F8FAFC] border border-[#DCE5F0] rounded-xl px-4 py-2 text-xs font-mono text-[#17243A] outline-hidden focus:border-[#0891D5]"
                />
                <button
                  onClick={handleExecute}
                  className="px-4 py-2 rounded-xl bg-[#0891D5] hover:bg-[#0780be] text-white font-mono text-xs font-bold flex items-center gap-1.5 transition-all shadow-xs"
                >
                  <Play className="w-3.5 h-3.5" />
                  <span>Execute</span>
                </button>
              </div>

              {method === 'PUT' && (
                <div>
                  <label className="text-[10px] uppercase font-mono text-[#64748B] block mb-1">
                    Value Payload (JSON / String)
                  </label>
                  <textarea
                    value={val}
                    onChange={(e) => setVal(e.target.value)}
                    rows={4}
                    className="w-full bg-[#F8FAFC] border border-[#DCE5F0] rounded-xl p-3 text-xs font-mono text-[#17243A] outline-hidden focus:border-[#0891D5]"
                  />
                </div>
              )}

              <div>
                <label className="text-[10px] uppercase font-mono text-[#64748B] block mb-1">
                  Response Telemetry
                </label>
                <pre className="bg-[#0B1120] text-emerald-400 p-4 rounded-xl text-xs font-mono overflow-x-auto shadow-inner border border-slate-800">
                  {responseOutput}
                </pre>
              </div>
            </div>

            <div className="lg:col-span-4 bg-[#F8FAFC] border border-[#E2E8F0] rounded-xl p-4 font-mono text-xs space-y-3">
              <span className="text-[10px] uppercase font-semibold text-[#64748B] block">
                Cluster Wire Directives
              </span>
              <p className="text-[#64748B] text-[11px] leading-relaxed">
                Client requests are routed through consistent hashing. Any node can forward writes to the active shard leader with zero data loss.
              </p>
              <div className="border-t border-[#E2E8F0] pt-2 space-y-1 text-[11px]">
                <div className="flex justify-between">
                  <span className="text-[#64748B]">Linearizable Read:</span>
                  <span className="text-[#10B981] font-semibold">Active (ReadIndex)</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-[#64748B]">Idempotency:</span>
                  <span className="text-[#0891D5] font-semibold">Deduplicated</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </DashboardLayout>
  );
}
