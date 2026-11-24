"use client";

import React from "react";
import Link from "next/link";
import { ArrowRight, ChevronDown, Cpu, Layers, HardDrive, ShieldCheck, Zap } from "lucide-react";
import ClusterVisualization3D from "./ClusterVisualization3D";

export default function Hero() {
  return (
    <section className="relative min-h-screen pt-28 pb-16 flex items-center justify-center overflow-hidden bg-tech-grid">
      {/* Ambient background glow orbs */}
      <div className="absolute top-1/4 left-1/4 -translate-x-1/2 -translate-y-1/2 w-96 h-96 bg-sky-500/15 rounded-full blur-[128px] pointer-events-none" />
      <div className="absolute bottom-1/4 right-1/4 translate-x-1/3 translate-y-1/3 w-[32rem] h-[32rem] bg-purple-600/15 rounded-full blur-[140px] pointer-events-none" />

      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 w-full z-10">
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-12 lg:gap-8 items-center">
          {/* Left Column — Core Content */}
          <div className="lg:col-span-6 flex flex-col items-start space-y-6 text-left">
            {/* Small Badge */}
            <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-sky-950/70 border border-sky-500/30 shadow-inner">
              <span className="w-2 h-2 rounded-full bg-sky-400 animate-pulse" />
              <span className="text-[11px] font-mono font-semibold uppercase tracking-widest text-sky-300">
                DISTRIBUTED SYSTEMS PROJECT
              </span>
            </div>

            {/* Main Heading */}
            <h1 className="text-4xl sm:text-5xl lg:text-6xl font-extrabold tracking-tight text-white leading-[1.12]">
              A distributed database.{" "}
              <span className="bg-gradient-to-r from-sky-400 via-sky-300 to-indigo-400 bg-clip-text text-transparent">
                Built to understand failure.
              </span>
            </h1>

            {/* Supporting Description */}
            <p className="text-base sm:text-lg text-slate-300 leading-relaxed font-sans max-w-xl">
              ORBIT is a distributed key-value database built around Raft consensus, replicated
              storage, sharding, and fault tolerance. Engineered from first principles with zero
              third-party dependencies.
            </p>

            {/* Two Action Buttons */}
            <div className="flex flex-wrap items-center gap-4 pt-2 w-full sm:w-auto">
              <Link
                href="/dashboard"
                className="flex items-center justify-center gap-2 px-6 py-3.5 text-sm font-semibold text-white bg-gradient-to-r from-sky-600 via-sky-500 to-indigo-600 hover:from-sky-500 hover:to-indigo-500 rounded-xl shadow-lg shadow-sky-600/35 hover:shadow-sky-500/50 border border-sky-400/30 transition-all hover:-translate-y-0.5 active:translate-y-0"
              >
                <span>Launch Dashboard</span>
                <ArrowRight className="w-4 h-4" />
              </Link>

              <a
                href="#architecture"
                className="flex items-center justify-center gap-2 px-6 py-3.5 text-sm font-semibold text-slate-200 bg-slate-900/80 hover:bg-slate-800/90 rounded-xl border border-slate-700 hover:border-slate-500 transition-all hover:-translate-y-0.5"
              >
                <span>Explore ORBIT</span>
                <ChevronDown className="w-4 h-4 text-slate-400" />
              </a>
            </div>

            {/* Three Technical Highlights */}
            <div className="pt-6 border-t border-slate-800/80 w-full grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div className="glass-panel p-3 rounded-xl border border-sky-500/15 flex items-start gap-2.5">
                <div className="p-1.5 rounded-lg bg-sky-500/10 text-sky-400 border border-sky-500/20">
                  <ShieldCheck className="w-4 h-4" />
                </div>
                <div>
                  <h4 className="text-xs font-semibold text-white font-mono">Raft Consensus</h4>
                  <p className="text-[11px] text-slate-400 leading-tight mt-0.5">
                    Quorum replicated state machine with ReadIndex
                  </p>
                </div>
              </div>

              <div className="glass-panel p-3 rounded-xl border border-purple-500/15 flex items-start gap-2.5">
                <div className="p-1.5 rounded-lg bg-purple-500/10 text-purple-400 border border-purple-500/20">
                  <Layers className="w-4 h-4" />
                </div>
                <div>
                  <h4 className="text-xs font-semibold text-white font-mono">Multi-Shard</h4>
                  <p className="text-[11px] text-slate-400 leading-tight mt-0.5">
                    Consistent hash ring with Murmur3 virtual nodes
                  </p>
                </div>
              </div>

              <div className="glass-panel p-3 rounded-xl border border-emerald-500/15 flex items-start gap-2.5">
                <div className="p-1.5 rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                  <HardDrive className="w-4 h-4" />
                </div>
                <div>
                  <h4 className="text-xs font-semibold text-white font-mono">LSM Storage</h4>
                  <p className="text-[11px] text-slate-400 leading-tight mt-0.5">
                    Durable WAL, Sparse SSTables & Bloom filters
                  </p>
                </div>
              </div>
            </div>
          </div>

          {/* Right Column — 3D Centerpiece Cluster */}
          <div className="lg:col-span-6 relative w-full">
            <div className="relative rounded-2xl glass-panel p-2 border border-sky-500/25 shadow-2xl shadow-sky-950/40">
              <ClusterVisualization3D />
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
