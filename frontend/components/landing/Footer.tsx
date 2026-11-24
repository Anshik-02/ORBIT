"use client";

import React from "react";
import Link from "next/link";
import { Database, ArrowRight, Shield, Terminal, BookOpen, Layers } from "lucide-react";
import GithubIcon from "@/components/ui/GithubIcon";

export default function Footer() {
  return (
    <footer className="relative bg-[#02050e] border-t border-slate-900 overflow-hidden text-slate-400 font-sans">
      {/* Upper CTA Banner */}
      <div className="relative py-20 border-b border-slate-900/80 bg-radial-gradient">
        <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 text-center">
          <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-sky-950/70 border border-sky-500/30 mb-6">
            <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
            <span className="text-xs font-mono font-semibold uppercase tracking-wider text-sky-300">
              EXPLORE THE LIVE CLUSTER
            </span>
          </div>

          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-extrabold text-white tracking-tight leading-tight">
            Understand the system. Observe the cluster.{" "}
            <span className="bg-gradient-to-r from-sky-400 via-sky-300 to-indigo-400 bg-clip-text text-transparent">
              Experiment with failure.
            </span>
          </h2>

          <p className="mt-4 text-base sm:text-lg text-slate-300 max-w-2xl mx-auto font-sans leading-relaxed">
            Step inside ORBIT and explore how a distributed database behaves under load, leader crash,
            and quorum loss.
          </p>

          <div className="mt-8 flex flex-wrap items-center justify-center gap-4">
            <Link
              href="/dashboard"
              className="flex items-center gap-2 px-8 py-4 text-sm font-bold text-white bg-gradient-to-r from-sky-600 via-sky-500 to-indigo-600 hover:from-sky-500 hover:to-indigo-500 rounded-xl shadow-xl shadow-sky-600/30 hover:shadow-sky-500/50 border border-sky-400/40 transition-all hover:-translate-y-0.5 active:translate-y-0"
            >
              <span>ENTER ORBIT DASHBOARD</span>
              <ArrowRight className="w-4 h-4" />
            </Link>

            <a
              href="https://github.com/Anshik-02/orbit"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 px-6 py-4 text-sm font-semibold text-slate-300 bg-slate-900/90 hover:bg-slate-800 rounded-xl border border-slate-700/80 hover:border-slate-500 transition-all"
            >
              <GithubIcon className="w-4 h-4" />
              <span>View Source on GitHub</span>
            </a>
          </div>
        </div>
      </div>

      {/* Main Footer Links & Topology Grid */}
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-16">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-10">
          {/* Brand Col */}
          <div className="lg:col-span-2 space-y-4">
            <div className="flex items-center gap-3">
              <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-sky-500/10 border border-sky-500/30 text-sky-400">
                <Database className="w-4 h-4" />
              </div>
              <span className="font-bold text-lg text-white tracking-wider">ORBIT</span>
              <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-sky-950 border border-sky-500/30 text-sky-300">
                v1.4.0
              </span>
            </div>
            <p className="text-xs text-slate-400 leading-relaxed font-sans max-w-sm">
              A distributed, multi-shard, crash-resilient key-value store engineered around the Raft
              consensus protocol and an atomic LSM-Tree storage hierarchy.
            </p>
            <div className="flex items-center gap-4 text-xs font-mono text-slate-400 pt-2">
              <span className="inline-flex items-center gap-1.5">
                <span className="w-2 h-2 rounded-full bg-emerald-400" />
                <span>Go 1.22+ Standard Library</span>
              </span>
            </div>
          </div>

          {/* Shard Placement Table */}
          <div className="lg:col-span-1">
            <h4 className="text-xs font-mono font-bold uppercase tracking-wider text-slate-200 mb-3">
              Shard Topology
            </h4>
            <div className="space-y-2 text-xs font-mono">
              <div className="p-2 rounded bg-slate-900/60 border border-slate-800">
                <div className="text-sky-400 font-semibold">Shard 0</div>
                <div className="text-[11px] text-slate-400">Node A, B, C (RF=3)</div>
              </div>
              <div className="p-2 rounded bg-slate-900/60 border border-slate-800">
                <div className="text-purple-400 font-semibold">Shard 1</div>
                <div className="text-[11px] text-slate-400">Node D, E, A (RF=3)</div>
              </div>
              <div className="p-2 rounded bg-slate-900/60 border border-slate-800">
                <div className="text-emerald-400 font-semibold">Shard 2</div>
                <div className="text-[11px] text-slate-400">Node B, C, D (RF=3)</div>
              </div>
            </div>
          </div>

          {/* Architecture Links */}
          <div>
            <h4 className="text-xs font-mono font-bold uppercase tracking-wider text-slate-200 mb-3">
              Architecture
            </h4>
            <ul className="space-y-2 text-xs font-mono">
              <li>
                <a href="#architecture" className="hover:text-sky-400 transition-colors">
                  LSM Storage Engine
                </a>
              </li>
              <li>
                <a href="#architecture" className="hover:text-sky-400 transition-colors">
                  Multi-Group Raft
                </a>
              </li>
              <li>
                <a href="#pipeline" className="hover:text-sky-400 transition-colors">
                  Write Pipeline Trace
                </a>
              </li>
              <li>
                <a href="#technology" className="hover:text-sky-400 transition-colors">
                  Consistent Hash Ring
                </a>
              </li>
              <li>
                <a href="#architecture" className="hover:text-sky-400 transition-colors">
                  Deterministic Recovery
                </a>
              </li>
            </ul>
          </div>

          {/* Resources & CLI */}
          <div>
            <h4 className="text-xs font-mono font-bold uppercase tracking-wider text-slate-200 mb-3">
              Binaries & Tools
            </h4>
            <ul className="space-y-2 text-xs font-mono">
              <li>
                <span className="text-sky-400 font-semibold">orbit</span> — Cluster Daemon
              </li>
              <li>
                <span className="text-purple-400 font-semibold">orbit-admin</span> — Cluster CLI
              </li>
              <li>
                <span className="text-emerald-400 font-semibold">orbit-bench</span> — Load Generator
              </li>
              <li>
                <span className="text-amber-400 font-semibold">orbit-chaos</span> — Fault Injector
              </li>
            </ul>
          </div>
        </div>

        {/* Bottom copyright row */}
        <div className="mt-12 pt-8 border-t border-slate-900/80 flex flex-col sm:flex-row items-center justify-between gap-4 text-xs font-mono text-slate-500">
          <div>
            © {new Date().getFullYear()} ORBIT Project. Open-source distributed database.
          </div>
          <div className="flex items-center gap-6">
            <a
              href="https://github.com/Anshik-02/orbit"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-sky-400 transition-colors"
            >
              GitHub Repository
            </a>
            <Link href="/dashboard" className="hover:text-sky-400 transition-colors">
              Launch Dashboard
            </Link>
          </div>
        </div>
      </div>
    </footer>
  );
}
