'use client';

import React from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { Settings, Save, Shield, HardDrive, Wifi } from 'lucide-react';

export default function SettingsPage() {
  return (
    <DashboardLayout activeNav="settings">
      <div className="space-y-6">
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs max-w-4xl">
          <div className="flex items-center gap-2 mb-1">
            <div className="w-8 h-8 rounded-lg bg-slate-100 flex items-center justify-center text-[#17243A]">
              <Settings className="w-4 h-4" />
            </div>
            <h1 className="text-xl font-bold text-[#17243A]">Cluster Settings & Configuration</h1>
          </div>
          <p className="text-xs text-[#64748B]">
            Configure consensus timers, compaction thresholds, forwarder failover and telemetry endpoints
          </p>

          <div className="mt-6 space-y-5 font-mono text-xs">
            <div className="p-4 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] space-y-3">
              <span className="text-[10px] uppercase font-bold text-[#17243A] block">
                Raft Consensus Timing
              </span>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label className="text-[11px] text-[#64748B] block mb-1">Heartbeat Interval</label>
                  <input
                    type="text"
                    defaultValue="50ms"
                    className="w-full bg-white border border-[#DCE5F0] rounded-lg px-3 py-1.5 text-[#17243A]"
                  />
                </div>
                <div>
                  <label className="text-[11px] text-[#64748B] block mb-1">Election Timeout Range</label>
                  <input
                    type="text"
                    defaultValue="150ms — 300ms"
                    className="w-full bg-white border border-[#DCE5F0] rounded-lg px-3 py-1.5 text-[#17243A]"
                  />
                </div>
              </div>
            </div>

            <div className="p-4 rounded-xl bg-[#F8FAFC] border border-[#E2E8F0] space-y-3">
              <span className="text-[10px] uppercase font-bold text-[#17243A] block">
                LSM Storage & Compaction
              </span>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label className="text-[11px] text-[#64748B] block mb-1">Memtable Flush Threshold</label>
                  <input
                    type="text"
                    defaultValue="4 MB"
                    className="w-full bg-white border border-[#DCE5F0] rounded-lg px-3 py-1.5 text-[#17243A]"
                  />
                </div>
                <div>
                  <label className="text-[11px] text-[#64748B] block mb-1">Bloom Filter False Positive Rate</label>
                  <input
                    type="text"
                    defaultValue="0.01 (1%)"
                    className="w-full bg-white border border-[#DCE5F0] rounded-lg px-3 py-1.5 text-[#17243A]"
                  />
                </div>
              </div>
            </div>

            <button
              onClick={() => alert('Settings saved to cluster configuration.')}
              className="px-5 py-2.5 rounded-xl bg-[#0891D5] hover:bg-[#0780be] text-white font-bold text-xs flex items-center gap-2 transition-all shadow-xs"
            >
              <Save className="w-4 h-4" />
              <span>Save Configuration</span>
            </button>
          </div>
        </div>
      </div>
    </DashboardLayout>
  );
}
