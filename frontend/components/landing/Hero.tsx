"use client";

import React from "react";
import Link from "next/link";
import { ArrowRight, ChevronDown, ShieldCheck, Layers, HardDrive } from "lucide-react";
import ClusterVisualization3D from "./ClusterVisualization3D";

export default function Hero() {
  return (
    <section className="relative min-h-screen pt-32 pb-20 flex items-center justify-center overflow-hidden bg-tech-grid">
      {/* Subtle ambient light gradient orbs */}
      <div className="absolute top-1/4 left-1/4 -translate-x-1/2 -translate-y-1/2 w-[30rem] h-[30rem] bg-sky-200/40 rounded-full blur-[140px] pointer-events-none" />
      <div className="absolute bottom-1/4 right-1/4 translate-x-1/3 translate-y-1/3 w-[32rem] h-[32rem] bg-indigo-100/50 rounded-full blur-[150px] pointer-events-none" />

      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 w-full z-10">
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-12 lg:gap-10 items-center">
          {/* Left Column — Core Content */}
          <div className="lg:col-span-6 flex flex-col items-start space-y-6 text-left">
            {/* Small Badge */}
            <div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full bg-white border border-[#DCE5F0] shadow-sm">
              <span className="w-2 h-2 rounded-full bg-[#0891D5] animate-pulse" />
              <span className="text-[11px] font-mono font-semibold uppercase tracking-widest text-[#0891D5]">
                DISTRIBUTED SYSTEMS PROJECT
              </span>
            </div>

            {/* Main Heading */}
            <h1 className="text-4xl sm:text-5xl lg:text-[3.75rem] font-extrabold tracking-tight text-[#17243A] leading-[1.12]">
              A distributed database.{" "}
              <span className="bg-gradient-to-r from-[#0891D5] via-[#2563eb] to-[#7965E8] bg-clip-text text-transparent block sm:inline">
                Built to understand failure.
              </span>
            </h1>

            {/* Supporting Description — Narrower for breathing room */}
            <p className="text-base sm:text-lg text-[#64748B] leading-relaxed font-sans max-w-lg">
              ORBIT is a distributed key-value database built around Raft consensus, replicated
              storage, sharding, and fault tolerance. Engineered from first principles with zero
              third-party dependencies.
            </p>

            {/* Two Action Buttons */}
            <div className="flex flex-wrap items-center gap-4 pt-1 w-full sm:w-auto">
              <Link
                href="/dashboard"
                className="flex items-center justify-center gap-2 px-6 py-3.5 text-sm font-semibold text-white bg-gradient-to-r from-[#0891D5] to-[#7965E8] hover:from-[#0284c7] hover:to-[#6d28d9] rounded-xl shadow-md shadow-sky-500/20 hover:shadow-lg hover:shadow-indigo-500/30 transition-all hover:-translate-y-0.5 active:translate-y-0"
              >
                <span>Launch Dashboard</span>
                <ArrowRight className="w-4 h-4" />
              </Link>

              <a
                href="#architecture"
                className="flex items-center justify-center gap-2 px-6 py-3.5 text-sm font-semibold text-[#17243A] bg-white hover:bg-slate-50 rounded-xl border border-[#DCE5F0] hover:border-slate-300 shadow-sm transition-all hover:-translate-y-0.5"
              >
                <span>Explore ORBIT</span>
                <ChevronDown className="w-4 h-4 text-[#64748B]" />
              </a>
            </div>

            {/* Three Technical Feature Cards Underneath Hero */}
            <div className="pt-6 border-t border-[#DCE5F0] w-full grid grid-cols-1 sm:grid-cols-3 gap-3.5">
              <div className="light-card light-card-hover p-4 rounded-xl flex items-start gap-3">
                <div className="p-2 rounded-lg bg-sky-50 text-[#0891D5] border border-sky-100 shrink-0">
                  <ShieldCheck className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-xs font-semibold text-[#17243A] font-mono">
                    Raft Consensus
                  </h4>
                  <p className="text-[11px] text-[#64748B] leading-relaxed mt-1">
                    Quorum replicated state machine with linearizable ReadIndex
                  </p>
                </div>
              </div>

              <div className="light-card light-card-hover p-4 rounded-xl flex items-start gap-3">
                <div className="p-2 rounded-lg bg-purple-50 text-[#7965E8] border border-purple-100 shrink-0">
                  <Layers className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-xs font-semibold text-[#17243A] font-mono">
                    Multi-Shard
                  </h4>
                  <p className="text-[11px] text-[#64748B] leading-relaxed mt-1">
                    Consistent hash ring with Murmur3 virtual nodes
                  </p>
                </div>
              </div>

              <div className="light-card light-card-hover p-4 rounded-xl flex items-start gap-3">
                <div className="p-2 rounded-lg bg-emerald-50 text-[#10B981] border border-emerald-100 shrink-0">
                  <HardDrive className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-xs font-semibold text-[#17243A] font-mono">
                    LSM Storage
                  </h4>
                  <p className="text-[11px] text-[#64748B] leading-relaxed mt-1">
                    Durable WAL, Sparse SSTables & Bloom filters
                  </p>
                </div>
              </div>
            </div>
          </div>

          {/* Right Column — 3D Centerpiece Cluster */}
          <div className="lg:col-span-6 relative w-full">
            <ClusterVisualization3D />
          </div>
        </div>
      </div>
    </section>
  );
}
