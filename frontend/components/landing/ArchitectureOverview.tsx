"use client";

import React, { useState } from "react";
import {
  HardDrive,
  ShieldCheck,
  Layers,
  AlertTriangle,
  Activity,
  ArrowRight,
  CheckCircle2,
  Database,
  Terminal,
  Zap,
  RefreshCw,
  Cpu,
} from "lucide-react";

export default function ArchitectureOverview() {
  const [selectedCard, setSelectedCard] = useState<number>(0);

  return (
    <section id="architecture" className="py-24 relative overflow-hidden bg-[#030712] border-t border-slate-900">
      {/* Background radial glow */}
      <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[48rem] h-[30rem] bg-indigo-500/10 rounded-full blur-[160px] pointer-events-none" />

      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 relative z-10">
        {/* Section Header */}
        <div className="text-center max-w-3xl mx-auto mb-16">
          <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-sky-950/70 border border-sky-500/30 mb-4">
            <Cpu className="w-3.5 h-3.5 text-sky-400" />
            <span className="text-xs font-mono font-semibold uppercase tracking-wider text-sky-300">
              CORE INFRASTRUCTURE
            </span>
          </div>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-extrabold text-white tracking-tight">
            Under the hood of ORBIT.
          </h2>
          <p className="mt-4 text-base sm:text-lg text-slate-400 font-sans">
            Explore the components that make distributed storage, consensus, and fault tolerance
            possible.
          </p>
        </div>

        {/* 5 Architecture Cards Grid */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {/* Card 1: Distributed Storage */}
          <div
            onClick={() => setSelectedCard(0)}
            className={`glass-panel glass-panel-hover p-6 rounded-2xl flex flex-col justify-between cursor-pointer border transition-all ${
              selectedCard === 0
                ? "border-sky-400/60 bg-slate-900/90 shadow-[0_0_30px_rgba(56,189,248,0.25)]"
                : "border-slate-800/80 hover:border-sky-500/30"
            }`}
          >
            <div>
              <div className="flex items-center justify-between mb-4">
                <div className="p-2.5 rounded-xl bg-sky-500/10 text-sky-400 border border-sky-500/20">
                  <HardDrive className="w-6 h-6" />
                </div>
                <span className="text-[10px] font-mono font-semibold px-2 py-0.5 rounded bg-sky-950 border border-sky-500/30 text-sky-300">
                  STORAGE ENGINE
                </span>
              </div>
              <h3 className="text-lg font-bold text-white mb-2">Distributed Storage (LSM-Tree)</h3>
              <p className="text-xs text-slate-400 leading-relaxed mb-4">
                Combines append-only durable write-ahead logging with in-memory Memtables, sparse-indexed SSTables, and Murmur3 Bloom filters.
              </p>

              {/* Mini Pipeline Visual */}
              <div className="p-3 bg-slate-950/80 rounded-xl border border-slate-800 font-mono text-[11px] space-y-1.5 mb-4">
                <div className="flex items-center justify-between text-slate-300">
                  <span className="text-sky-400 font-bold">1. WRITE</span>
                  <span className="text-slate-500">→</span>
                  <span className="text-amber-400">WAL (CRC32)</span>
                </div>
                <div className="flex items-center justify-between text-slate-300">
                  <span className="text-amber-400">2. MEMTABLE</span>
                  <span className="text-slate-500">→</span>
                  <span className="text-purple-400">SkipList / BTree</span>
                </div>
                <div className="flex items-center justify-between text-slate-300">
                  <span className="text-purple-400">3. SSTABLES</span>
                  <span className="text-slate-500">→</span>
                  <span className="text-emerald-400">Sparse Index & Bloom</span>
                </div>
                <div className="flex items-center justify-between text-slate-300">
                  <span className="text-emerald-400">4. COMPACTION</span>
                  <span className="text-slate-500">→</span>
                  <span className="text-sky-300">Two-Way Merge</span>
                </div>
              </div>
            </div>

            <div className="pt-3 border-t border-slate-800/80 flex items-center justify-between text-xs font-mono text-sky-400">
              <span>Inspect LSM Pipeline</span>
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>

          {/* Card 2: Raft Consensus */}
          <div
            onClick={() => setSelectedCard(1)}
            className={`glass-panel glass-panel-hover p-6 rounded-2xl flex flex-col justify-between cursor-pointer border transition-all ${
              selectedCard === 1
                ? "border-sky-400/60 bg-slate-900/90 shadow-[0_0_30px_rgba(56,189,248,0.25)]"
                : "border-slate-800/80 hover:border-sky-500/30"
            }`}
          >
            <div>
              <div className="flex items-center justify-between mb-4">
                <div className="p-2.5 rounded-xl bg-purple-500/10 text-purple-400 border border-purple-500/20">
                  <ShieldCheck className="w-6 h-6" />
                </div>
                <span className="text-[10px] font-mono font-semibold px-2 py-0.5 rounded bg-purple-950 border border-purple-500/30 text-purple-300">
                  CONSENSUS PROTOCOL
                </span>
              </div>
              <h3 className="text-lg font-bold text-white mb-2">Raft Consensus</h3>
              <p className="text-xs text-slate-400 leading-relaxed mb-4">
                Maintains a single consistent, replicated log across nodes. Ensures linearizability, quorum commitment, and automated failover.
              </p>

              {/* Three Replicas Visual */}
              <div className="p-3 bg-slate-950/80 rounded-xl border border-slate-800 font-mono text-xs space-y-2 mb-4">
                <div className="flex items-center justify-between p-1.5 rounded bg-amber-500/10 border border-amber-500/30 text-amber-300">
                  <span className="font-bold">Replica 1 (Leader)</span>
                  <span className="text-[10px]">Term 4 • Commit: 142</span>
                </div>
                <div className="flex items-center justify-between p-1.5 rounded bg-slate-900 border border-slate-800 text-slate-300">
                  <span>Replica 2 (Follower)</span>
                  <span className="text-[10px] text-emerald-400">In Sync (142)</span>
                </div>
                <div className="flex items-center justify-between p-1.5 rounded bg-slate-900 border border-slate-800 text-slate-300">
                  <span>Replica 3 (Follower)</span>
                  <span className="text-[10px] text-emerald-400">In Sync (142)</span>
                </div>
              </div>
            </div>

            <div className="pt-3 border-t border-slate-800/80 flex items-center justify-between text-xs font-mono text-purple-400">
              <span>Inspect Quorum Logic</span>
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>

          {/* Card 3: Sharding */}
          <div
            onClick={() => setSelectedCard(2)}
            className={`glass-panel glass-panel-hover p-6 rounded-2xl flex flex-col justify-between cursor-pointer border transition-all ${
              selectedCard === 2
                ? "border-sky-400/60 bg-slate-900/90 shadow-[0_0_30px_rgba(56,189,248,0.25)]"
                : "border-slate-800/80 hover:border-sky-500/30"
            }`}
          >
            <div>
              <div className="flex items-center justify-between mb-4">
                <div className="p-2.5 rounded-xl bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                  <Layers className="w-6 h-6" />
                </div>
                <span className="text-[10px] font-mono font-semibold px-2 py-0.5 rounded bg-emerald-950 border border-emerald-500/30 text-emerald-300">
                  DATA PARTITIONING
                </span>
              </div>
              <h3 className="text-lg font-bold text-white mb-2">Consistent Hash Ring</h3>
              <p className="text-xs text-slate-400 leading-relaxed mb-4">
                Distributes keys across logical shards with 256 virtual nodes per shard. Assigns 3 physical replicas with zero hotspotting.
              </p>

              {/* Shard Distribution Visual */}
              <div className="p-3 bg-slate-950/80 rounded-xl border border-slate-800 font-mono text-[11px] space-y-1.5 mb-4">
                <div className="flex items-center justify-between">
                  <span className="text-sky-400 font-semibold">Shard 0 (0-33%)</span>
                  <span className="text-slate-400">A, B, C (RF=3)</span>
                </div>
                <div className="w-full bg-slate-800 h-1.5 rounded-full overflow-hidden">
                  <div className="bg-sky-400 h-full w-[33%]" />
                </div>
                <div className="flex items-center justify-between pt-1">
                  <span className="text-purple-400 font-semibold">Shard 1 (34-66%)</span>
                  <span className="text-slate-400">D, E, A (RF=3)</span>
                </div>
                <div className="w-full bg-slate-800 h-1.5 rounded-full overflow-hidden">
                  <div className="bg-purple-400 h-full w-[66%]" />
                </div>
                <div className="flex items-center justify-between pt-1">
                  <span className="text-emerald-400 font-semibold">Shard 2 (67-100%)</span>
                  <span className="text-slate-400">B, C, D (RF=3)</span>
                </div>
                <div className="w-full bg-slate-800 h-1.5 rounded-full overflow-hidden">
                  <div className="bg-emerald-400 h-full w-[100%]" />
                </div>
              </div>
            </div>

            <div className="pt-3 border-t border-slate-800/80 flex items-center justify-between text-xs font-mono text-emerald-400">
              <span>Inspect Hash Ring</span>
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>

          {/* Card 4: Fault Tolerance */}
          <div
            onClick={() => setSelectedCard(3)}
            className={`glass-panel glass-panel-hover p-6 rounded-2xl flex flex-col justify-between cursor-pointer border transition-all ${
              selectedCard === 3
                ? "border-sky-400/60 bg-slate-900/90 shadow-[0_0_30px_rgba(56,189,248,0.25)]"
                : "border-slate-800/80 hover:border-sky-500/30"
            }`}
          >
            <div>
              <div className="flex items-center justify-between mb-4">
                <div className="p-2.5 rounded-xl bg-amber-500/10 text-amber-400 border border-amber-500/20">
                  <AlertTriangle className="w-6 h-6" />
                </div>
                <span className="text-[10px] font-mono font-semibold px-2 py-0.5 rounded bg-amber-950 border border-amber-500/30 text-amber-300">
                  FAULT TOLERANCE
                </span>
              </div>
              <h3 className="text-lg font-bold text-white mb-2">Resilience & Failover</h3>
              <p className="text-xs text-slate-400 leading-relaxed mb-4">
                Survives node crashes, network partitions, and leader failures without split-brain anomalies or uncommitted dirty reads.
              </p>

              {/* Failover Status Visual */}
              <div className="p-3 bg-slate-950/80 rounded-xl border border-slate-800 font-mono text-[11px] space-y-1.5 mb-4">
                <div className="flex items-center justify-between text-red-400">
                  <span>Leader (Node A) [CRASHED]</span>
                  <span>✖ Offline</span>
                </div>
                <div className="flex items-center justify-between text-amber-400">
                  <span>Heartbeat timeout</span>
                  <span>150ms elapsed</span>
                </div>
                <div className="flex items-center justify-between text-emerald-400 font-semibold">
                  <span>New Leader Elected (Node B)</span>
                  <span>✓ Term 5</span>
                </div>
                <div className="flex items-center justify-between text-sky-400">
                  <span>Client Re-route</span>
                  <span>Zero Data Loss</span>
                </div>
              </div>
            </div>

            <div className="pt-3 border-t border-slate-800/80 flex items-center justify-between text-xs font-mono text-amber-400">
              <span>View Chaos Testing</span>
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>

          {/* Card 5: Observability */}
          <div
            onClick={() => setSelectedCard(4)}
            className={`glass-panel glass-panel-hover p-6 rounded-2xl flex flex-col justify-between cursor-pointer border transition-all md:col-span-2 lg:col-span-2 ${
              selectedCard === 4
                ? "border-sky-400/60 bg-slate-900/90 shadow-[0_0_30px_rgba(56,189,248,0.25)]"
                : "border-slate-800/80 hover:border-sky-500/30"
            }`}
          >
            <div>
              <div className="flex items-center justify-between mb-4">
                <div className="p-2.5 rounded-xl bg-sky-500/10 text-sky-400 border border-sky-500/20">
                  <Activity className="w-6 h-6" />
                </div>
                <span className="text-[10px] font-mono font-semibold px-2 py-0.5 rounded bg-sky-950 border border-sky-500/30 text-sky-300">
                  REAL-TIME TELEMETRY
                </span>
              </div>
              <h3 className="text-lg font-bold text-white mb-2">Observability & WebSocket Bus</h3>
              <p className="text-xs text-slate-400 leading-relaxed mb-4">
                Every state machine event, log commit, leader transition, and heartbeat is broadcast over a dedicated real-time WebSocket channel with deduplication.
              </p>

              {/* Metrics visual */}
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 font-mono text-xs">
                <div className="p-2.5 rounded-xl bg-slate-950/80 border border-slate-800">
                  <span className="text-[10px] text-slate-400 block">Throughput</span>
                  <span className="text-sm font-bold text-sky-400">12,450 op/s</span>
                </div>
                <div className="p-2.5 rounded-xl bg-slate-950/80 border border-slate-800">
                  <span className="text-[10px] text-slate-400 block">Commit Latency</span>
                  <span className="text-sm font-bold text-emerald-400">1.82 ms</span>
                </div>
                <div className="p-2.5 rounded-xl bg-slate-950/80 border border-slate-800">
                  <span className="text-[10px] text-slate-400 block">Active Replicas</span>
                  <span className="text-sm font-bold text-purple-400">5 / 5 Healthy</span>
                </div>
                <div className="p-2.5 rounded-xl bg-slate-950/80 border border-slate-800">
                  <span className="text-[10px] text-slate-400 block">Deduplicated</span>
                  <span className="text-sm font-bold text-amber-400">100% Exact</span>
                </div>
              </div>
            </div>

            <div className="pt-4 mt-4 border-t border-slate-800/80 flex items-center justify-between text-xs font-mono text-sky-400">
              <span>Stream Live Cluster Metrics</span>
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
