"use client";

import React from "react";
import {
  Cpu,
  ShieldCheck,
  FileText,
  Database,
  Filter,
  Container,
  LayoutTemplate,
  Box,
  Layers,
} from "lucide-react";

interface TechItem {
  name: string;
  category: string;
  icon: any;
  color: string;
  description: string;
  badge: string;
}

const TECHNOLOGIES: TechItem[] = [
  {
    name: "Go Standard Library",
    category: "Core Engine",
    icon: Cpu,
    color: "#0891D5",
    description: "Pure Go implementation with zero third-party packages in go.mod. High-concurrency goroutines and sync primitives.",
    badge: "Zero Dependencies",
  },
  {
    name: "Raft Consensus",
    category: "Replication",
    icon: ShieldCheck,
    color: "#7965E8",
    description: "Multi-group Raft protocol with randomized election timers, empty heartbeat suppression, and linearizable ReadIndex.",
    badge: "Strict Consistency",
  },
  {
    name: "Write-Ahead Log (WAL)",
    category: "Durability",
    icon: FileText,
    color: "#0891D5",
    description: "Append-only write journal with 32-bit CRC checksums, partial record truncation recovery, and checkpoint flushing.",
    badge: "Crash-Consistent",
  },
  {
    name: "SSTables & Sparse Index",
    category: "On-Disk Storage",
    icon: Database,
    color: "#10B981",
    description: "Immutable Sorted String Tables on disk with sparse in-memory index boundaries and atomic manifest recovery.",
    badge: "LSM Hierarchy",
  },
  {
    name: "Murmur3 Bloom Filters",
    category: "Lookup Acceleration",
    icon: Filter,
    color: "#F59E0B",
    description: "Probabilistic bit arrays built into each SSTable block to eliminate unnecessary disk seeks for non-existent keys.",
    badge: "O(1) Negative Lookups",
  },
  {
    name: "5-Node Topology",
    category: "Orchestration",
    icon: Container,
    color: "#0891D5",
    description: "5 physical nodes, 3 logical shards with RF=3 replication, automated leader discovery, and failover routing.",
    badge: "Multi-Process Mesh",
  },
  {
    name: "Next.js & React 18",
    category: "Frontend UI",
    icon: LayoutTemplate,
    color: "#17243A",
    description: "Server-side rendering, real-time WebSocket telemetry ingestion, and low-latency interactive visualization.",
    badge: "App Router",
  },
  {
    name: "Three.js & WebGL",
    category: "3D Visualization",
    icon: Box,
    color: "#7965E8",
    description: "Interactive hardware-accelerated 3D constellation representing live nodes, replicas, and traveling consensus photons.",
    badge: "Hardware Accelerated",
  },
];

export default function TechStack() {
  return (
    <section id="technology" className="py-28 relative overflow-hidden bg-[#F4F7FC] border-t border-[#DCE5F0]">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 relative z-10">
        {/* Section Header */}
        <div className="text-center max-w-3xl mx-auto mb-20">
          <div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full bg-white border border-[#DCE5F0] shadow-sm mb-4">
            <Layers className="w-4 h-4 text-[#0891D5]" />
            <span className="text-xs font-mono font-semibold uppercase tracking-wider text-[#0891D5]">
              ARCHITECTURE & TOOLING
            </span>
          </div>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-extrabold text-[#17243A] tracking-tight">
            Engineered with Go. Designed around distributed systems.
          </h2>
          <p className="mt-4 text-base sm:text-lg text-[#64748B] font-sans">
            Every layer of ORBIT is built with rigor—from atomic disk manifests to multi-group consensus
            and interactive real-time telemetry.
          </p>
        </div>

        {/* 8 Technology Cards Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
          {TECHNOLOGIES.map((tech) => {
            const Icon = tech.icon;
            return (
              <div
                key={tech.name}
                className="light-card light-card-hover p-6 rounded-2xl flex flex-col justify-between group transition-all"
              >
                <div>
                  <div className="flex items-center justify-between mb-5">
                    <div
                      className="p-3 rounded-xl border transition-transform duration-300 group-hover:scale-105"
                      style={{
                        backgroundColor: `${tech.color}15`,
                        borderColor: `${tech.color}35`,
                        color: tech.color,
                      }}
                    >
                      <Icon className="w-5 h-5" />
                    </div>
                    <span className="text-[10px] font-mono font-semibold px-2 py-0.5 rounded-full bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A]">
                      {tech.badge}
                    </span>
                  </div>

                  <h3 className="text-base font-bold text-[#17243A] mb-1 font-mono tracking-tight group-hover:text-[#0891D5] transition-colors">
                    {tech.name}
                  </h3>
                  <span className="text-[11px] font-mono text-[#64748B] block mb-3">
                    {tech.category}
                  </span>
                  <p className="text-xs text-[#64748B] leading-relaxed font-sans">
                    {tech.description}
                  </p>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
