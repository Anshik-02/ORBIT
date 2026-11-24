"use client";

import React, { useState, useEffect } from "react";
import {
  HardDrive,
  ShieldCheck,
  Layers,
  AlertTriangle,
  Activity,
  ArrowRight,
  CheckCircle2,
  Database,
  Cpu,
  RefreshCw,
  Play,
  RotateCcw,
} from "lucide-react";

export default function ArchitectureOverview() {
  const [activeStorageStep, setActiveStorageStep] = useState(0);
  const [raftReplicating, setRaftReplicating] = useState(false);
  const [simulatedKey, setSimulatedKey] = useState("user:4892");
  const [mappedShard, setMappedShard] = useState(1);

  // Auto-advance storage pipeline animation
  useEffect(() => {
    const timer = setInterval(() => {
      setActiveStorageStep((prev) => (prev + 1) % 4);
    }, 2400);
    return () => clearInterval(timer);
  }, []);

  const triggerRaftReplication = () => {
    setRaftReplicating(true);
    setTimeout(() => setRaftReplicating(false), 1200);
  };

  const handleHashKey = (key: string) => {
    setSimulatedKey(key);
    // Simple deterministic hash mapping simulation:
    let hash = 0;
    for (let i = 0; i < key.length; i++) {
      hash = (hash << 5) - hash + key.charCodeAt(i);
      hash |= 0;
    }
    setMappedShard(Math.abs(hash) % 3);
  };

  return (
    <section id="architecture" className="py-28 relative overflow-hidden bg-[#F4F7FC] border-t border-[#DCE5F0]">
      {/* Background ambient light radial gradient */}
      <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[52rem] h-[34rem] bg-sky-100/50 rounded-full blur-[160px] pointer-events-none" />

      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 relative z-10">
        {/* Section Header with generous breathing room */}
        <div className="text-center max-w-3xl mx-auto mb-20">
          <div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full bg-white border border-[#DCE5F0] shadow-sm mb-4">
            <Cpu className="w-4 h-4 text-[#0891D5]" />
            <span className="text-xs font-mono font-semibold uppercase tracking-wider text-[#0891D5]">
              CORE INFRASTRUCTURE
            </span>
          </div>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-extrabold text-[#17243A] tracking-tight">
            Under the hood of ORBIT.
          </h2>
          <p className="mt-4 text-base sm:text-lg text-[#64748B] font-sans">
            Explore the subsystems engineered from first principles for linearizable consensus,
            durable storage, and high-performance sharding.
          </p>
        </div>

        {/* 5 Architecture Cards Grid with lighter surfaces and generous padding */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
          {/* Card 1: Distributed Storage Engine */}
          <div className="light-card light-card-hover p-8 rounded-2xl flex flex-col justify-between transition-all">
            <div>
              <div className="flex items-center justify-between mb-5">
                <div className="p-3 rounded-xl bg-sky-50 text-[#0891D5] border border-sky-100">
                  <HardDrive className="w-6 h-6" />
                </div>
                <span className="text-[10px] font-mono font-semibold px-2.5 py-0.5 rounded-full bg-sky-50 border border-sky-200 text-[#0891D5]">
                  STORAGE ENGINE
                </span>
              </div>
              <h3 className="text-xl font-bold text-[#17243A] mb-2.5">
                Distributed Storage (LSM-Tree)
              </h3>
              <p className="text-sm text-[#64748B] leading-relaxed mb-6">
                Append-only durable write-ahead logging with in-memory Memtables, sparse-indexed SSTables, and Murmur3 Bloom filters.
              </p>

              {/* Interactive Animated Pipeline Visual */}
              <div className="p-4 bg-[#F8FAFC] rounded-xl border border-[#DCE5F0] font-mono text-xs space-y-2.5 mb-6">
                <div className="flex items-center justify-between text-[11px] text-[#64748B] pb-1 border-b border-[#DCE5F0]">
                  <span>LSM WRITE PATHWAY</span>
                  <span className="text-[#0891D5] font-semibold">STAGE {activeStorageStep + 1}/4</span>
                </div>

                {[
                  { stage: "1. WAL APPEND", desc: "CRC32 Checksummed Disk Segment", color: "text-[#0891D5]" },
                  { stage: "2. MEMTABLE", desc: "Concurrent SkipList / B-Tree Buffer", color: "text-[#7965E8]" },
                  { stage: "3. SSTABLE FLUSH", desc: "Sparse Block Index & Bloom Filter", color: "text-[#10B981]" },
                  { stage: "4. COMPACTION", desc: "Multi-Tier Leveled Merge (L0 → L2)", color: "text-amber-600" },
                ].map((item, idx) => {
                  const isActive = activeStorageStep === idx;
                  return (
                    <div
                      key={idx}
                      className={`flex items-center justify-between p-2 rounded-lg transition-all ${
                        isActive
                          ? "bg-white border border-[#CBD5E1] shadow-sm font-semibold scale-[1.02]"
                          : "opacity-60"
                      }`}
                    >
                      <div className="flex items-center gap-2">
                        <span
                          className={`w-2 h-2 rounded-full ${
                            isActive ? "bg-[#0891D5] animate-ping" : "bg-slate-300"
                          }`}
                        />
                        <span className={item.color}>{item.stage}</span>
                      </div>
                      <span className="text-[10px] text-[#64748B] truncate max-w-[140px]">
                        {item.desc}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>

            <div className="pt-4 border-t border-[#DCE5F0] flex items-center justify-between text-xs font-mono text-[#0891D5] font-semibold">
              <span>Zero-Loss WAL Recovery</span>
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>

          {/* Card 2: Raft Consensus */}
          <div className="light-card light-card-hover p-8 rounded-2xl flex flex-col justify-between transition-all">
            <div>
              <div className="flex items-center justify-between mb-5">
                <div className="p-3 rounded-xl bg-purple-50 text-[#7965E8] border border-purple-100">
                  <ShieldCheck className="w-6 h-6" />
                </div>
                <span className="text-[10px] font-mono font-semibold px-2.5 py-0.5 rounded-full bg-purple-50 border border-purple-200 text-[#7965E8]">
                  CONSENSUS PROTOCOL
                </span>
              </div>
              <h3 className="text-xl font-bold text-[#17243A] mb-2.5">
                Raft Consensus
              </h3>
              <p className="text-sm text-[#64748B] leading-relaxed mb-6">
                Maintains a single linearizable log across replicas. Guarantees quorum commitment, heartbeat health, and automated failover.
              </p>

              {/* Interactive Three Replicas Visual */}
              <div className="p-4 bg-[#F8FAFC] rounded-xl border border-[#DCE5F0] font-mono text-xs space-y-2.5 mb-6">
                <div className="flex items-center justify-between text-[11px] text-[#64748B] pb-1 border-b border-[#DCE5F0]">
                  <span>QUORUM REPLICATION (3 NODES)</span>
                  <button
                    onClick={triggerRaftReplication}
                    className="flex items-center gap-1 text-[10px] text-[#7965E8] hover:underline"
                  >
                    <Play className="w-2.5 h-2.5" /> Tick RPC
                  </button>
                </div>

                <div className="p-2.5 rounded-lg bg-amber-50/80 border border-amber-200 text-amber-900 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="w-2 h-2 rounded-full bg-amber-500" />
                    <span className="font-bold">Replica 1 (Leader)</span>
                  </div>
                  <span className="text-[11px] font-semibold">Term 4 · Commit 142</span>
                </div>

                <div
                  className={`p-2.5 rounded-lg bg-white border border-[#DCE5F0] text-[#17243A] flex items-center justify-between transition-all ${
                    raftReplicating ? "border-[#7965E8] shadow-sm bg-purple-50/30" : ""
                  }`}
                >
                  <div className="flex items-center gap-2">
                    <span className="w-2 h-2 rounded-full bg-[#10B981]" />
                    <span>Replica 2 (Follower)</span>
                  </div>
                  <span className="text-[11px] text-[#10B981] font-semibold">
                    {raftReplicating ? "Replicating..." : "In Sync (142)"}
                  </span>
                </div>

                <div
                  className={`p-2.5 rounded-lg bg-white border border-[#DCE5F0] text-[#17243A] flex items-center justify-between transition-all ${
                    raftReplicating ? "border-[#7965E8] shadow-sm bg-purple-50/30" : ""
                  }`}
                >
                  <div className="flex items-center gap-2">
                    <span className="w-2 h-2 rounded-full bg-[#10B981]" />
                    <span>Replica 3 (Follower)</span>
                  </div>
                  <span className="text-[11px] text-[#10B981] font-semibold">
                    {raftReplicating ? "Replicating..." : "In Sync (142)"}
                  </span>
                </div>
              </div>
            </div>

            <div className="pt-4 border-t border-[#DCE5F0] flex items-center justify-between text-xs font-mono text-[#7965E8] font-semibold">
              <span>Linearizable ReadIndex Protocol</span>
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>

          {/* Card 3: Consistent Hash Ring */}
          <div className="light-card light-card-hover p-8 rounded-2xl flex flex-col justify-between transition-all">
            <div>
              <div className="flex items-center justify-between mb-5">
                <div className="p-3 rounded-xl bg-emerald-50 text-[#10B981] border border-emerald-100">
                  <Layers className="w-6 h-6" />
                </div>
                <span className="text-[10px] font-mono font-semibold px-2.5 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981]">
                  DATA PARTITIONING
                </span>
              </div>
              <h3 className="text-xl font-bold text-[#17243A] mb-2.5">
                Consistent Hash Ring
              </h3>
              <p className="text-sm text-[#64748B] leading-relaxed mb-6">
                Distributes keys across logical shards with 256 virtual nodes per shard. Assigns 3 physical replicas with uniform distribution.
              </p>

              {/* Interactive Key Hasher */}
              <div className="p-4 bg-[#F8FAFC] rounded-xl border border-[#DCE5F0] font-mono text-xs space-y-3 mb-6">
                <div className="flex items-center justify-between text-[11px] text-[#64748B] pb-1 border-b border-[#DCE5F0]">
                  <span>HASH RING KEY MAPPING</span>
                  <span className="text-[#10B981] font-semibold">SHARD {mappedShard}</span>
                </div>

                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    value={simulatedKey}
                    onChange={(e) => handleHashKey(e.target.value)}
                    placeholder="Enter test key..."
                    className="flex-1 px-2.5 py-1.5 text-xs bg-white border border-[#DCE5F0] rounded-lg text-[#17243A] focus:outline-none focus:border-[#0891D5]"
                  />
                  <button
                    onClick={() => handleHashKey(`user:${Math.floor(Math.random() * 9000 + 1000)}`)}
                    className="p-1.5 bg-white border border-[#DCE5F0] hover:bg-slate-50 rounded-lg text-[#64748B]"
                    title="Generate random key"
                  >
                    <RotateCcw className="w-3.5 h-3.5" />
                  </button>
                </div>

                <div className="space-y-1.5 pt-1">
                  {[
                    { id: 0, range: "0x00000000..", nodes: "Node A, B, C", color: "bg-[#0891D5]" },
                    { id: 1, range: "0x55555555..", nodes: "Node D, E, A", color: "bg-[#7965E8]" },
                    { id: 2, range: "0xAAAAAAAA..", nodes: "Node B, C, D", color: "bg-[#10B981]" },
                  ].map((s) => {
                    const isSelected = mappedShard === s.id;
                    return (
                      <div
                        key={s.id}
                        className={`flex items-center justify-between p-2 rounded-lg text-[11px] transition-all ${
                          isSelected
                            ? "bg-white border border-[#0891D5] shadow-sm font-semibold"
                            : "opacity-60"
                        }`}
                      >
                        <div className="flex items-center gap-2">
                          <span className={`w-2 h-2 rounded-full ${s.color}`} />
                          <span>Shard {s.id}</span>
                        </div>
                        <span className="text-[#64748B] text-[10px]">{s.nodes}</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>

            <div className="pt-4 border-t border-[#DCE5F0] flex items-center justify-between text-xs font-mono text-[#10B981] font-semibold">
              <span>Deterministic Murmur3 Ring</span>
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>

          {/* Card 4: Fault Tolerance & Chaos Testing */}
          <div className="light-card light-card-hover p-8 rounded-2xl flex flex-col justify-between transition-all">
            <div>
              <div className="flex items-center justify-between mb-5">
                <div className="p-3 rounded-xl bg-amber-50 text-[#F59E0B] border border-amber-100">
                  <AlertTriangle className="w-6 h-6" />
                </div>
                <span className="text-[10px] font-mono font-semibold px-2.5 py-0.5 rounded-full bg-amber-50 border border-amber-200 text-[#F59E0B]">
                  FAULT TOLERANCE
                </span>
              </div>
              <h3 className="text-xl font-bold text-[#17243A] mb-2.5">
                Resilience & Failover
              </h3>
              <p className="text-sm text-[#64748B] leading-relaxed mb-6">
                Survives node crashes, network partitions, and abrupt leader demotions without split-brain anomalies or uncommitted state.
              </p>

              {/* Failover Status Visual */}
              <div className="p-4 bg-[#F8FAFC] rounded-xl border border-[#DCE5F0] font-mono text-xs space-y-2 mb-6">
                <div className="flex items-center justify-between text-rose-600 font-semibold p-1.5 rounded bg-rose-50/60 border border-rose-100">
                  <span>Leader (Node A)</span>
                  <span>[Simulated Crash]</span>
                </div>
                <div className="flex items-center justify-between text-amber-700 p-1.5">
                  <span>Election Timeout</span>
                  <span>150ms elapsed</span>
                </div>
                <div className="flex items-center justify-between text-emerald-700 font-semibold p-1.5 rounded bg-emerald-50/60 border border-emerald-100">
                  <span>New Leader Elected</span>
                  <span>✓ Node B (Term 5)</span>
                </div>
                <div className="flex items-center justify-between text-[#0891D5] p-1.5">
                  <span>Client Re-route</span>
                  <span>Zero Data Loss</span>
                </div>
              </div>
            </div>

            <div className="pt-4 border-t border-[#DCE5F0] flex items-center justify-between text-xs font-mono text-[#F59E0B] font-semibold">
              <span>Automatic Failover in &lt;300ms</span>
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>

          {/* Card 5: Real-Time Observability */}
          <div className="light-card light-card-hover p-8 rounded-2xl flex flex-col justify-between transition-all md:col-span-2 lg:col-span-2">
            <div>
              <div className="flex items-center justify-between mb-5">
                <div className="p-3 rounded-xl bg-sky-50 text-[#0891D5] border border-sky-100">
                  <Activity className="w-6 h-6" />
                </div>
                <span className="text-[10px] font-mono font-semibold px-2.5 py-0.5 rounded-full bg-sky-50 border border-sky-200 text-[#0891D5]">
                  REAL-TIME TELEMETRY
                </span>
              </div>
              <h3 className="text-xl font-bold text-[#17243A] mb-2.5">
                Observability & Event Bus
              </h3>
              <p className="text-sm text-[#64748B] leading-relaxed mb-6">
                Every state machine mutation, consensus term update, compaction event, and health check is broadcast over a dedicated real-time WebSocket channel.
              </p>

              {/* Clean Telemetry Tiles with High Contrast */}
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 font-mono text-xs mb-4">
                <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0]">
                  <span className="text-[10px] text-[#64748B] uppercase block">Throughput</span>
                  <span className="text-base font-bold text-[#0891D5]">12,450 op/s</span>
                </div>
                <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0]">
                  <span className="text-[10px] text-[#64748B] uppercase block">Commit Latency</span>
                  <span className="text-base font-bold text-[#10B981]">1.82 ms</span>
                </div>
                <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0]">
                  <span className="text-[10px] text-[#64748B] uppercase block">Active Replicas</span>
                  <span className="text-base font-bold text-[#7965E8]">5 / 5 Healthy</span>
                </div>
                <div className="p-3 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0]">
                  <span className="text-[10px] text-[#64748B] uppercase block">Linearizability</span>
                  <span className="text-base font-bold text-amber-600">100% Strict</span>
                </div>
              </div>
            </div>

            <div className="pt-4 border-t border-[#DCE5F0] flex items-center justify-between text-xs font-mono text-[#0891D5] font-semibold">
              <span>Streaming Event Pipeline</span>
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
