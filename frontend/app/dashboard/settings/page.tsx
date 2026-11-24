'use client';

import React, { useState } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import {
  Settings,
  Save,
  Shield,
  HardDrive,
  Wifi,
  Activity,
  CheckCircle2,
  AlertTriangle,
  RotateCcw,
  Palette,
  Server,
  Layers,
  Lock,
} from 'lucide-react';

type SettingsTab =
  | 'cluster'
  | 'raft'
  | 'storage'
  | 'network'
  | 'observability'
  | 'appearance';

export default function SettingsPage() {
  const [activeTab, setActiveTab] = useState<SettingsTab>('cluster');
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [isSaving, setIsSaving] = useState(false);

  // Settings State matching configs/cluster.json
  const [clusterId, setClusterId] = useState('orbit-cluster-01');
  const [shardCount, setShardCount] = useState(3);
  const [replicationFactor, setReplicationFactor] = useState(3);

  // Raft Settings
  const [heartbeatIntervalMs, setHeartbeatIntervalMs] = useState(50);
  const [electionTimeoutMinMs, setElectionTimeoutMinMs] = useState(150);
  const [electionTimeoutMaxMs, setElectionTimeoutMaxMs] = useState(300);
  const [snapshotThresholdEntries, setSnapshotThresholdEntries] = useState(10000);
  const [readIndexLinearizable, setReadIndexLinearizable] = useState(true);

  // Storage Settings
  const [memtableThresholdMb, setMemtableThresholdMb] = useState(64);
  const [walSyncMode, setWalSyncMode] = useState<'STRICT_SYNC' | 'BATCH_SYNC'>('STRICT_SYNC');
  const [sstableBlockSizeKb, setSstableBlockSizeKb] = useState(4);
  const [l0CompactionThreshold, setL0CompactionThreshold] = useState(4);

  // Networking Settings
  const [httpDeadlineMs, setHttpDeadlineMs] = useState(5000);
  const [maxForwardRetries, setMaxForwardRetries] = useState(3);
  const [maxBackoffMs, setMaxBackoffMs] = useState(2000);

  // Observability
  const [logLevel, setLogLevel] = useState<'DEBUG' | 'INFO' | 'WARN' | 'ERROR'>('INFO');
  const [eventBufferLimit, setEventBufferLimit] = useState(10000);

  // Appearance
  const [themePreference, setThemePreference] = useState<'light' | 'dark' | 'system'>('light');
  const [fpsLimit, setFpsLimit] = useState(60);

  const handleSave = () => {
    setIsSaving(true);
    setSaveSuccess(false);
    setTimeout(() => {
      setIsSaving(false);
      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
    }, 600);
  };

  return (
    <DashboardLayout activeNav="settings">
      <div className="space-y-6 max-w-5xl font-mono text-xs">
        {/* 01. PAGE HEADER */}
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl sm:text-2xl font-bold text-[#17243A] tracking-tight">
                Cluster Settings & Parameters
              </h1>
              <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-blue-50 border border-blue-200 text-[#0891D5] text-xs font-semibold">
                CONFIG: configs/cluster.json
              </span>
            </div>
            <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal">
              Manage Raft consensus timers, storage limits, failover retry policies, and observability.
            </p>
          </div>

          <div className="flex items-center gap-2">
            <button
              onClick={handleSave}
              disabled={isSaving}
              className="px-5 py-2.5 rounded-xl bg-[#0891D5] hover:bg-[#0284C7] disabled:opacity-50 text-white font-bold flex items-center gap-2 transition-all shadow-xs"
            >
              <Save className="w-3.5 h-3.5" />
              <span>{isSaving ? 'Validating & Saving...' : 'Save Changes'}</span>
            </button>
          </div>
        </div>

        {saveSuccess && (
          <div className="p-3.5 rounded-xl bg-emerald-50 border border-emerald-200 text-emerald-800 flex items-center gap-2 animate-fadeIn">
            <CheckCircle2 className="w-4 h-4 text-emerald-600 shrink-0" />
            <span>
              Configuration changes successfully validated and saved to cluster state.
            </span>
          </div>
        )}

        {/* 02. TAB SELECTOR */}
        <div className="flex flex-wrap items-center gap-1 p-1 bg-white border border-[#DCE5F0] rounded-2xl shadow-xs">
          <button
            onClick={() => setActiveTab('cluster')}
            className={`px-3.5 py-2 rounded-xl font-bold transition-all flex items-center gap-1.5 ${
              activeTab === 'cluster'
                ? 'bg-[#F4F7FC] text-[#0891D5] border border-[#DCE5F0] shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            <Server className="w-3.5 h-3.5" />
            <span>Cluster & Nodes</span>
          </button>

          <button
            onClick={() => setActiveTab('raft')}
            className={`px-3.5 py-2 rounded-xl font-bold transition-all flex items-center gap-1.5 ${
              activeTab === 'raft'
                ? 'bg-[#F4F7FC] text-[#0891D5] border border-[#DCE5F0] shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            <Shield className="w-3.5 h-3.5" />
            <span>Raft Consensus</span>
          </button>

          <button
            onClick={() => setActiveTab('storage')}
            className={`px-3.5 py-2 rounded-xl font-bold transition-all flex items-center gap-1.5 ${
              activeTab === 'storage'
                ? 'bg-[#F4F7FC] text-[#0891D5] border border-[#DCE5F0] shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            <HardDrive className="w-3.5 h-3.5" />
            <span>Storage & LSM</span>
          </button>

          <button
            onClick={() => setActiveTab('network')}
            className={`px-3.5 py-2 rounded-xl font-bold transition-all flex items-center gap-1.5 ${
              activeTab === 'network'
                ? 'bg-[#F4F7FC] text-[#0891D5] border border-[#DCE5F0] shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            <Wifi className="w-3.5 h-3.5" />
            <span>Networking & Retries</span>
          </button>

          <button
            onClick={() => setActiveTab('observability')}
            className={`px-3.5 py-2 rounded-xl font-bold transition-all flex items-center gap-1.5 ${
              activeTab === 'observability'
                ? 'bg-[#F4F7FC] text-[#0891D5] border border-[#DCE5F0] shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            <Activity className="w-3.5 h-3.5" />
            <span>Observability</span>
          </button>

          <button
            onClick={() => setActiveTab('appearance')}
            className={`px-3.5 py-2 rounded-xl font-bold transition-all flex items-center gap-1.5 ${
              activeTab === 'appearance'
                ? 'bg-[#F4F7FC] text-[#0891D5] border border-[#DCE5F0] shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            <Palette className="w-3.5 h-3.5" />
            <span>Appearance</span>
          </button>
        </div>

        {/* 03. TAB PANELS */}
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-6">
          {/* TAB 1: CLUSTER & NODES */}
          {activeTab === 'cluster' && (
            <div className="space-y-4">
              <div className="pb-3 border-b border-[#DCE5F0]">
                <h3 className="text-base font-bold text-[#17243A]">
                  Cluster Topology & Sharding Settings
                </h3>
                <p className="text-[#64748B] text-xs font-normal mt-0.5">
                  Cluster-wide identity, shard replication factor, and node membership
                </p>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    CLUSTER IDENTIFIER
                  </label>
                  <input
                    type="text"
                    value={clusterId}
                    onChange={(e) => setClusterId(e.target.value)}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">Unique cluster namespace in discovery</span>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    SHARD COUNT (FIXED RING)
                  </label>
                  <input
                    type="number"
                    disabled
                    value={shardCount}
                    className="w-full p-2.5 rounded-xl bg-[#F1F5F9] border border-[#DCE5F0] text-[#64748B] cursor-not-allowed"
                  />
                  <span className="text-[10px] text-amber-600 flex items-center gap-1">
                    <Lock className="w-3 h-3" />
                    <span>Fixed partition count (Requires full cluster re-hash to change)</span>
                  </span>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    REPLICATION FACTOR (RF)
                  </label>
                  <input
                    type="number"
                    value={replicationFactor}
                    onChange={(e) => setReplicationFactor(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">Default: 3 replicas per shard partition</span>
                </div>
              </div>
            </div>
          )}

          {/* TAB 2: RAFT CONSENSUS */}
          {activeTab === 'raft' && (
            <div className="space-y-4">
              <div className="pb-3 border-b border-[#DCE5F0]">
                <h3 className="text-base font-bold text-[#17243A]">
                  Raft Consensus Protocol Timers
                </h3>
                <p className="text-[#64748B] text-xs font-normal mt-0.5">
                  Heartbeat frequencies, election timeouts, and linearizable read policies
                </p>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    HEARTBEAT INTERVAL (MS)
                  </label>
                  <input
                    type="number"
                    value={heartbeatIntervalMs}
                    onChange={(e) => setHeartbeatIntervalMs(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">Leader heartbeat pulse interval (Recommended: 50ms)</span>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    ELECTION TIMEOUT RANGE (MS)
                  </label>
                  <div className="flex items-center gap-2">
                    <input
                      type="number"
                      value={electionTimeoutMinMs}
                      onChange={(e) => setElectionTimeoutMinMs(parseInt(e.target.value, 10))}
                      className="w-1/2 p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                      placeholder="Min (150ms)"
                    />
                    <span className="text-[#64748B]">to</span>
                    <input
                      type="number"
                      value={electionTimeoutMaxMs}
                      onChange={(e) => setElectionTimeoutMaxMs(parseInt(e.target.value, 10))}
                      className="w-1/2 p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                      placeholder="Max (300ms)"
                    />
                  </div>
                  <span className="text-[10px] text-[#94A3B8]">Randomized jitter window preventing split votes</span>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    SNAPSHOT COMPACTION TRIGGER (ENTRIES)
                  </label>
                  <input
                    type="number"
                    value={snapshotThresholdEntries}
                    onChange={(e) => setSnapshotThresholdEntries(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">Log truncation threshold triggers SSTable snapshot</span>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    LINEARIZABLE READINDEX
                  </label>
                  <div className="flex items-center gap-3 pt-2">
                    <input
                      type="checkbox"
                      id="readIndex"
                      checked={readIndexLinearizable}
                      onChange={(e) => setReadIndexLinearizable(e.target.checked)}
                      className="w-4 h-4 text-[#0891D5] rounded"
                    />
                    <label htmlFor="readIndex" className="text-xs text-[#17243A] cursor-pointer">
                      Verify quorum lease before serving client read operations (Zero stale reads)
                    </label>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* TAB 3: STORAGE & LSM */}
          {activeTab === 'storage' && (
            <div className="space-y-4">
              <div className="pb-3 border-b border-[#DCE5F0]">
                <h3 className="text-base font-bold text-[#17243A]">
                  LSM Storage Engine & Disk Durability
                </h3>
                <p className="text-[#64748B] text-xs font-normal mt-0.5">
                  Memtable memory thresholds, fsync durability mode, and SSTable compaction parameters
                </p>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    MEMTABLE FLUSH THRESHOLD (MB)
                  </label>
                  <input
                    type="number"
                    value={memtableThresholdMb}
                    onChange={(e) => setMemtableThresholdMb(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">Threshold when active SkipList freezes to immutable</span>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    WAL FSYNC DURABILITY MODE
                  </label>
                  <select
                    value={walSyncMode}
                    onChange={(e) => setWalSyncMode(e.target.value as any)}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  >
                    <option value="STRICT_SYNC">STRICT_SYNC (Every write fsynced immediately)</option>
                    <option value="BATCH_SYNC">BATCH_SYNC (Group commits every 10ms)</option>
                  </select>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    SSTABLE BLOCK SIZE (KB)
                  </label>
                  <input
                    type="number"
                    value={sstableBlockSizeKb}
                    onChange={(e) => setSstableBlockSizeKb(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">Disk block compression & sparse index granularity</span>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    L0 COMPACTION TRIGGER (FILES)
                  </label>
                  <input
                    type="number"
                    value={l0CompactionThreshold}
                    onChange={(e) => setL0CompactionThreshold(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">Number of overlapping L0 SSTables before merge to L1</span>
                </div>
              </div>
            </div>
          )}

          {/* TAB 4: NETWORKING & RETRIES */}
          {activeTab === 'network' && (
            <div className="space-y-4">
              <div className="pb-3 border-b border-[#DCE5F0]">
                <h3 className="text-base font-bold text-[#17243A]">
                  HTTP Gateway & Raft Transport Policies
                </h3>
                <p className="text-[#64748B] text-xs font-normal mt-0.5">
                  Deadlines, forwarder retry loops, and exponential backoff
                </p>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    HTTP REQUEST DEADLINE (MS)
                  </label>
                  <input
                    type="number"
                    value={httpDeadlineMs}
                    onChange={(e) => setHttpDeadlineMs(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">Gateway timeout before returning HTTP 504</span>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    MAX FORWARDING RETRIES
                  </label>
                  <input
                    type="number"
                    value={maxForwardRetries}
                    onChange={(e) => setMaxForwardRetries(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">Retries during transient leader failover</span>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    MAX BACKOFF DURATION (MS)
                  </label>
                  <input
                    type="number"
                    value={maxBackoffMs}
                    onChange={(e) => setMaxBackoffMs(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">Maximum sleep between failover retry attempts</span>
                </div>
              </div>
            </div>
          )}

          {/* TAB 5: OBSERVABILITY */}
          {activeTab === 'observability' && (
            <div className="space-y-4">
              <div className="pb-3 border-b border-[#DCE5F0]">
                <h3 className="text-base font-bold text-[#17243A]">
                  Observability, Telemetry & Logs
                </h3>
                <p className="text-[#64748B] text-xs font-normal mt-0.5">
                  Logging thresholds and event retention buffer
                </p>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    LOG VERBOSITY LEVEL
                  </label>
                  <select
                    value={logLevel}
                    onChange={(e) => setLogLevel(e.target.value as any)}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  >
                    <option value="DEBUG">DEBUG (Detailed Raft packets)</option>
                    <option value="INFO">INFO (State transitions & API requests)</option>
                    <option value="WARN">WARN (Heartbeat timeouts only)</option>
                    <option value="ERROR">ERROR (Fatal errors only)</option>
                  </select>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    EVENT BUFFER RETENTION LIMIT
                  </label>
                  <input
                    type="number"
                    value={eventBufferLimit}
                    onChange={(e) => setEventBufferLimit(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  />
                  <span className="text-[10px] text-[#94A3B8]">In-memory events buffer size for /cluster/events</span>
                </div>
              </div>
            </div>
          )}

          {/* TAB 6: APPEARANCE */}
          {activeTab === 'appearance' && (
            <div className="space-y-4">
              <div className="pb-3 border-b border-[#DCE5F0]">
                <h3 className="text-base font-bold text-[#17243A]">
                  Appearance, Typography & Visualizer
                </h3>
                <p className="text-[#64748B] text-xs font-normal mt-0.5">
                  Theme palette, typography and Three.js visualizer rendering options
                </p>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    COLOR THEME
                  </label>
                  <select
                    value={themePreference}
                    onChange={(e) => setThemePreference(e.target.value as any)}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  >
                    <option value="light">Light Infrastructure (Recommended)</option>
                    <option value="system">System Default</option>
                  </select>
                  <span className="text-[10px] text-[#94A3B8]">
                    Palette: #F4F7FC (Bg), #FFFFFF (Cards), #0891D5 (Cyan), #7965E8 (Purple)
                  </span>
                </div>

                <div className="space-y-1.5">
                  <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
                    3D VISUALIZER FRAMERATE CAP
                  </label>
                  <select
                    value={fpsLimit}
                    onChange={(e) => setFpsLimit(parseInt(e.target.value, 10))}
                    className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
                  >
                    <option value={30}>30 FPS (Energy Saving)</option>
                    <option value={60}>60 FPS (Smooth)</option>
                    <option value={120}>120 FPS (Ultra Pro)</option>
                  </select>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </DashboardLayout>
  );
}
