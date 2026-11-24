"use client";

import React from "react";
import Link from "next/link";
import { Database, ArrowRight, ShieldCheck, Terminal, Server, CheckCircle2 } from "lucide-react";
import GithubIcon from "@/components/ui/GithubIcon";

export default function Footer() {
  return (
    <footer className="relative bg-white border-t border-[#DCE5F0] overflow-hidden text-[#64748B] font-sans">
      {/* Upper CTA Banner with subtle blue radial gradient & faint network visualization */}
      <div className="relative py-24 border-b border-[#DCE5F0] bg-gradient-to-b from-[#F4F7FC] via-white to-[#EAF1FA] overflow-hidden">
        {/* Subtle radial glow & faint network mesh */}
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[46rem] h-[28rem] bg-sky-200/40 rounded-full blur-[140px] pointer-events-none" />

        {/* Faint network visualization lines */}
        <div className="absolute inset-0 opacity-15 pointer-events-none flex items-center justify-center">
          <svg viewBox="0 0 800 300" className="w-full max-w-4xl h-full">
            <line x1="100" y1="150" x2="250" y2="80" stroke="#0891D5" strokeWidth="1.5" strokeDasharray="4 4" />
            <line x1="250" y1="80" x2="400" y2="150" stroke="#0891D5" strokeWidth="1.5" />
            <line x1="400" y1="150" x2="550" y2="80" stroke="#7965E8" strokeWidth="1.5" />
            <line x1="550" y1="80" x2="700" y2="150" stroke="#7965E8" strokeWidth="1.5" strokeDasharray="4 4" />
            <line x1="250" y1="80" x2="550" y2="80" stroke="#10B981" strokeWidth="1.5" strokeDasharray="2 2" />
            <circle cx="100" cy="150" r="5" fill="#0891D5" />
            <circle cx="250" cy="80" r="6" fill="#0891D5" />
            <circle cx="400" cy="150" r="8" fill="#17243A" />
            <circle cx="550" cy="80" r="6" fill="#7965E8" />
            <circle cx="700" cy="150" r="5" fill="#7965E8" />
          </svg>
        </div>

        <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 text-center relative z-10">
          <div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full bg-white border border-[#DCE5F0] shadow-sm mb-6">
            <span className="w-2 h-2 rounded-full bg-[#10B981] animate-pulse" />
            <span className="text-xs font-mono font-semibold uppercase tracking-wider text-[#0891D5]">
              EXPLORE THE LIVE CLUSTER
            </span>
          </div>

          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-extrabold text-[#17243A] tracking-tight leading-tight">
            Understand the system. Observe the cluster.{" "}
            <span className="bg-gradient-to-r from-[#0891D5] to-[#7965E8] bg-clip-text text-transparent block sm:inline">
              Experiment with failure.
            </span>
          </h2>

          <p className="mt-4 text-base sm:text-lg text-[#64748B] max-w-2xl mx-auto font-sans leading-relaxed">
            Step inside ORBIT and observe how a distributed database behaves under load, leader crash,
            and quorum loss.
          </p>

          {/* Small Cluster Status Illustration */}
          <div className="my-8 inline-flex items-center gap-3 p-2.5 px-4 rounded-xl bg-white border border-[#DCE5F0] shadow-sm font-mono text-xs text-[#17243A]">
            <div className="flex items-center gap-1.5">
              <span className="w-2 h-2 rounded-full bg-[#10B981]" />
              <span className="font-semibold">Node A-E Online</span>
            </div>
            <span className="text-[#DCE5F0]">|</span>
            <div className="flex items-center gap-1.5">
              <ShieldCheck className="w-3.5 h-3.5 text-[#0891D5]" />
              <span>3 Raft Leaders Active</span>
            </div>
            <span className="text-[#DCE5F0]">|</span>
            <div className="flex items-center gap-1.5 text-[#7965E8]">
              <span>RF=3 In Sync</span>
            </div>
          </div>

          <div className="flex flex-wrap items-center justify-center gap-4">
            <Link
              href="/dashboard"
              className="flex items-center gap-2 px-8 py-4 text-sm font-bold text-white bg-gradient-to-r from-[#0891D5] to-[#7965E8] hover:from-[#0284c7] hover:to-[#6d28d9] rounded-xl shadow-lg shadow-sky-500/25 hover:shadow-xl hover:shadow-indigo-500/30 transition-all hover:-translate-y-0.5"
            >
              <span>ENTER ORBIT DASHBOARD</span>
              <ArrowRight className="w-4 h-4" />
            </Link>

            <a
              href="https://github.com/Anshik-02/orbit"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 px-7 py-4 text-sm font-semibold text-[#17243A] bg-white hover:bg-slate-50 rounded-xl border border-[#DCE5F0] hover:border-slate-300 shadow-sm transition-all hover:-translate-y-0.5"
            >
              <GithubIcon className="w-4 h-4 text-[#17243A]" />
              <span>View Source on GitHub</span>
            </a>
          </div>
        </div>
      </div>

      {/* Main Footer Links & Topology Grid with High Contrast */}
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-16 bg-white">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-10">
          {/* Brand Col */}
          <div className="lg:col-span-2 space-y-4">
            <div className="flex items-center gap-3">
              <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-sky-50 border border-sky-200 text-[#0891D5]">
                <Database className="w-4 h-4" />
              </div>
              <span className="font-bold text-lg text-[#17243A] tracking-wider">ORBIT</span>
              <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-sky-50 border border-sky-200 text-[#0891D5] font-semibold">
                v1.4.0
              </span>
            </div>
            <p className="text-xs text-[#64748B] leading-relaxed font-sans max-w-sm">
              A distributed, multi-shard, crash-resilient key-value store engineered around the Raft
              consensus protocol and an atomic LSM-Tree storage hierarchy.
            </p>
            <div className="flex items-center gap-4 text-xs font-mono text-[#64748B] pt-2">
              <span className="inline-flex items-center gap-1.5">
                <span className="w-2 h-2 rounded-full bg-[#10B981]" />
                <span className="font-medium text-[#17243A]">Go 1.22+ Standard Library</span>
              </span>
            </div>
          </div>

          {/* Shard Placement Table */}
          <div className="lg:col-span-1">
            <h4 className="text-xs font-mono font-bold uppercase tracking-wider text-[#17243A] mb-3">
              Shard Topology
            </h4>
            <div className="space-y-2 text-xs font-mono">
              <div className="p-2.5 rounded-lg bg-[#F4F7FC] border border-[#DCE5F0]">
                <div className="text-[#0891D5] font-bold">Shard 0</div>
                <div className="text-[11px] text-[#64748B]">Node A, B, C (RF=3)</div>
              </div>
              <div className="p-2.5 rounded-lg bg-[#F4F7FC] border border-[#DCE5F0]">
                <div className="text-[#7965E8] font-bold">Shard 1</div>
                <div className="text-[11px] text-[#64748B]">Node D, E, A (RF=3)</div>
              </div>
              <div className="p-2.5 rounded-lg bg-[#F4F7FC] border border-[#DCE5F0]">
                <div className="text-[#10B981] font-bold">Shard 2</div>
                <div className="text-[11px] text-[#64748B]">Node B, C, D (RF=3)</div>
              </div>
            </div>
          </div>

          {/* Architecture Links */}
          <div>
            <h4 className="text-xs font-mono font-bold uppercase tracking-wider text-[#17243A] mb-3">
              Architecture
            </h4>
            <ul className="space-y-2.5 text-xs font-mono text-[#64748B]">
              <li>
                <a href="#architecture" className="hover:text-[#0891D5] transition-colors">
                  LSM Storage Engine
                </a>
              </li>
              <li>
                <a href="#architecture" className="hover:text-[#0891D5] transition-colors">
                  Multi-Group Raft
                </a>
              </li>
              <li>
                <a href="#pipeline" className="hover:text-[#0891D5] transition-colors">
                  Write Pipeline Trace
                </a>
              </li>
              <li>
                <a href="#technology" className="hover:text-[#0891D5] transition-colors">
                  Consistent Hash Ring
                </a>
              </li>
              <li>
                <a href="#architecture" className="hover:text-[#0891D5] transition-colors">
                  Deterministic Recovery
                </a>
              </li>
            </ul>
          </div>

          {/* Binaries & Tools */}
          <div>
            <h4 className="text-xs font-mono font-bold uppercase tracking-wider text-[#17243A] mb-3">
              Binaries & Tools
            </h4>
            <ul className="space-y-2.5 text-xs font-mono text-[#64748B]">
              <li>
                <span className="text-[#0891D5] font-semibold">orbit</span> — Cluster Daemon
              </li>
              <li>
                <span className="text-[#7965E8] font-semibold">orbit-admin</span> — Cluster CLI
              </li>
              <li>
                <span className="text-[#10B981] font-semibold">orbit-bench</span> — Load Generator
              </li>
              <li>
                <span className="text-[#F59E0B] font-semibold">orbit-chaos</span> — Fault Injector
              </li>
            </ul>
          </div>
        </div>

        {/* Bottom copyright row */}
        <div className="mt-12 pt-8 border-t border-[#DCE5F0] flex flex-col sm:flex-row items-center justify-between gap-4 text-xs font-mono text-[#64748B]">
          <div>
            © {new Date().getFullYear()} ORBIT Project. Open-source distributed database.
          </div>
          <div className="flex items-center gap-6">
            <a
              href="https://github.com/Anshik-02/orbit"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-[#0891D5] transition-colors font-medium"
            >
              GitHub Repository
            </a>
            <Link href="/dashboard" className="hover:text-[#0891D5] transition-colors font-medium">
              Launch Dashboard
            </Link>
          </div>
        </div>
      </div>
    </footer>
  );
}
