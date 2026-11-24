"use client";

import React, { useState } from "react";
import Link from "next/link";
import {
  Database,
  ArrowLeft,
  Activity,
  Server,
  Layers,
  ShieldCheck,
  Send,
  Terminal,
  Zap,
  Play,
  Flame,
  Radio,
  CheckCircle2,
  Clock,
  Code2,
  RefreshCw,
  Search,
} from "lucide-react";

interface NodeState {
  id: string;
  name: string;
  role: "Leader" | "Follower";
  status: "Online" | "Offline" | "Recovering";
  term: number;
  commitIndex: number;
  apiPort: number;
  raftPort: number;
  shards: number[];
}

export default function DashboardPage() {
  const [nodes, setNodes] = useState<NodeState[]>([
    {
      id: "node-a",
      name: "Node A",
      role: "Leader",
      status: "Online",
      term: 4,
      commitIndex: 142,
      apiPort: 8000,
      raftPort: 9000,
      shards: [0, 1],
    },
    {
      id: "node-b",
      name: "Node B",
      role: "Leader",
      status: "Online",
      term: 4,
      commitIndex: 142,
      apiPort: 8001,
      raftPort: 9001,
      shards: [0, 2],
    },
    {
      id: "node-c",
      name: "Node C",
      role: "Follower",
      status: "Online",
      term: 4,
      commitIndex: 142,
      apiPort: 8002,
      raftPort: 9002,
      shards: [0, 2],
    },
    {
      id: "node-d",
      name: "Node D",
      role: "Leader",
      status: "Online",
      term: 4,
      commitIndex: 142,
      apiPort: 8003,
      raftPort: 9003,
      shards: [1, 2],
    },
    {
      id: "node-e",
      name: "Node E",
      role: "Follower",
      status: "Online",
      term: 4,
      commitIndex: 142,
      apiPort: 8004,
      raftPort: 9004,
      shards: [1],
    },
  ]);

  // Query Console State
  const [method, setMethod] = useState<"GET" | "PUT" | "DELETE">("PUT");
  const [key, setKey] = useState<string>("user:101");
  const [value, setValue] = useState<string>('{"name": "Alice Cooper", "role": "admin"}');
  const [responseLog, setResponseLog] = useState<string | null>(
    `HTTP/1.1 200 OK
Content-Type: application/json
X-Request-ID: req-7a8f90bc
X-Consensus-Term: 4
X-Commit-Index: 142

{
  "key": "user:101",
  "value": "{\\"name\\": \\"Alice Cooper\\", \\"role\\": \\"admin\\"}",
  "shard_id": 1,
  "status": "ok"
}`
  );
  const [isExecuting, setIsExecuting] = useState<boolean>(false);
  const [activeTab, setActiveTab] = useState<"cluster" | "console" | "events">("cluster");

  // Simulated query execution
  const executeQuery = () => {
    setIsExecuting(true);
    setTimeout(() => {
      setIsExecuting(false);
      // Murmur3 hash shard calculation simulation
      const shardId = (key.length * 7 + 1) % 3;
      if (method === "PUT") {
        setResponseLog(
          `HTTP/1.1 200 OK\nContent-Type: application/json\nX-Request-ID: req-${Math.random()
            .toString(36)
            .substring(2, 10)}\nX-Consensus-Term: 4\nX-Commit-Index: 143\n\n{\n  "key": "${key}",\n  "value": ${JSON.stringify(
            value
          )},\n  "shard_id": ${shardId},\n  "status": "ok"\n}`
        );
      } else if (method === "GET") {
        setResponseLog(
          `HTTP/1.1 200 OK\nContent-Type: application/json\nX-Request-ID: req-${Math.random()
            .toString(36)
            .substring(2, 10)}\nX-Consensus-Term: 4\nLinearizable-Read: true\n\n{\n  "key": "${key}",\n  "value": ${JSON.stringify(
            value
          )},\n  "shard_id": ${shardId}\n}`
        );
      } else {
        setResponseLog(
          `HTTP/1.1 200 OK\nContent-Type: application/json\nX-Request-ID: req-${Math.random()
            .toString(36)
            .substring(2, 10)}\nX-Consensus-Term: 4\n\n{\n  "key": "${key}",\n  "deleted": true,\n  "shard_id": ${shardId}\n}`
        );
      }
    }, 180);
  };

  return (
    <div className="min-h-screen bg-[#030712] text-slate-100 flex flex-col font-sans">
      {/* Top Dashboard Header */}
      <header className="border-b border-slate-800/80 bg-[#060b19]/90 backdrop-blur-md sticky top-0 z-40 px-4 sm:px-6 py-3.5">
        <div className="max-w-7xl mx-auto flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-4">
            <Link
              href="/"
              className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-slate-900 hover:bg-slate-800 text-slate-300 hover:text-white border border-slate-800 text-xs font-mono transition-colors"
            >
              <ArrowLeft className="w-3.5 h-3.5" />
              <span>Landing Page</span>
            </Link>

            <div className="flex items-center gap-2.5">
              <div className="w-8 h-8 rounded-lg bg-sky-500/10 border border-sky-500/30 flex items-center justify-center text-sky-400">
                <Database className="w-4 h-4" />
              </div>
              <div>
                <span className="font-bold text-sm tracking-wider text-white">ORBIT</span>
                <span className="text-[10px] font-mono text-slate-400 ml-2">Cluster Telemetry</span>
              </div>
            </div>
          </div>

          {/* Quick Status Badges */}
          <div className="flex items-center gap-3 font-mono text-xs">
            <div className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-950/60 border border-emerald-500/30 text-emerald-400">
              <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
              <span>5/5 Nodes Online</span>
            </div>
            <div className="hidden sm:flex items-center gap-1.5 px-3 py-1 rounded-full bg-sky-950/60 border border-sky-500/30 text-sky-400">
              <Layers className="w-3.5 h-3.5" />
              <span>3 Shards (RF=3)</span>
            </div>
            <div className="hidden md:flex items-center gap-1.5 px-3 py-1 rounded-full bg-purple-950/60 border border-purple-500/30 text-purple-400">
              <ShieldCheck className="w-3.5 h-3.5" />
              <span>Quorum Stable</span>
            </div>
          </div>
        </div>
      </header>

      {/* Main Dashboard Body */}
      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8 w-full flex-1">
        {/* Navigation Tabs */}
        <div className="flex items-center gap-2 pb-4 mb-6 border-b border-slate-800 text-xs font-mono">
          <button
            onClick={() => setActiveTab("cluster")}
            className={`flex items-center gap-2 px-4 py-2 rounded-xl transition-all ${
              activeTab === "cluster"
                ? "bg-sky-500/20 text-sky-300 border border-sky-500/40 font-bold"
                : "text-slate-400 hover:text-slate-200"
            }`}
          >
            <Server className="w-4 h-4" />
            <span>Cluster Topology</span>
          </button>
          <button
            onClick={() => setActiveTab("console")}
            className={`flex items-center gap-2 px-4 py-2 rounded-xl transition-all ${
              activeTab === "console"
                ? "bg-sky-500/20 text-sky-300 border border-sky-500/40 font-bold"
                : "text-slate-400 hover:text-slate-200"
            }`}
          >
            <Terminal className="w-4 h-4" />
            <span>KV Query Console</span>
          </button>
          <button
            onClick={() => setActiveTab("events")}
            className={`flex items-center gap-2 px-4 py-2 rounded-xl transition-all ${
              activeTab === "events"
                ? "bg-sky-500/20 text-sky-300 border border-sky-500/40 font-bold"
                : "text-slate-400 hover:text-slate-200"
            }`}
          >
            <Activity className="w-4 h-4" />
            <span>Event Stream</span>
          </button>
        </div>

        {/* Tab 1: Cluster Topology */}
        {activeTab === "cluster" && (
          <div className="space-y-6 animate-fadeIn">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
              {nodes.map((node) => (
                <div
                  key={node.id}
                  className="glass-panel p-5 rounded-2xl border border-slate-800/80 hover:border-sky-500/30 flex flex-col justify-between"
                >
                  <div>
                    <div className="flex items-center justify-between mb-3">
                      <div className="flex items-center gap-2">
                        <span className="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse" />
                        <span className="font-bold font-mono text-white text-base">{node.name}</span>
                        <span className="text-[11px] font-mono text-slate-400">({node.id})</span>
                      </div>
                      <span
                        className={`text-[10px] font-mono font-semibold px-2 py-0.5 rounded-full ${
                          node.role === "Leader"
                            ? "bg-amber-500/20 text-amber-300 border border-amber-500/40"
                            : "bg-sky-500/20 text-sky-300 border border-sky-500/40"
                        }`}
                      >
                        {node.role}
                      </span>
                    </div>

                    <div className="space-y-2 text-xs font-mono mb-4">
                      <div className="flex items-center justify-between text-slate-300 bg-slate-900/50 p-2 rounded-lg border border-slate-800">
                        <span className="text-slate-400">HTTP Port:</span>
                        <span className="text-sky-400 font-semibold">{node.apiPort}</span>
                      </div>
                      <div className="flex items-center justify-between text-slate-300 bg-slate-900/50 p-2 rounded-lg border border-slate-800">
                        <span className="text-slate-400">Raft Port:</span>
                        <span className="text-purple-400 font-semibold">{node.raftPort}</span>
                      </div>
                      <div className="flex items-center justify-between text-slate-300 bg-slate-900/50 p-2 rounded-lg border border-slate-800">
                        <span className="text-slate-400">Term / Commit:</span>
                        <span className="text-emerald-400 font-semibold">
                          T:{node.term} / #{node.commitIndex}
                        </span>
                      </div>
                    </div>

                    <div className="flex items-center justify-between text-xs font-mono bg-slate-950 p-2 rounded-lg border border-slate-800/80">
                      <span className="text-slate-400">Hosted Shards:</span>
                      <div className="flex gap-1.5">
                        {node.shards.map((sid) => (
                          <span
                            key={sid}
                            className="px-2 py-0.5 rounded bg-sky-950/80 border border-sky-500/30 text-sky-300 font-bold text-[10px]"
                          >
                            Shard {sid}
                          </span>
                        ))}
                      </div>
                    </div>
                  </div>

                  <div className="mt-4 pt-3 border-t border-slate-800/80 flex items-center justify-between text-[11px] font-mono text-slate-400">
                    <span className="text-emerald-400">Healthy & Synchronized</span>
                    <span>99.99% Uptime</span>
                  </div>
                </div>
              ))}
            </div>

            {/* Quick Shard Summary Matrix */}
            <div className="glass-panel p-6 rounded-2xl border border-sky-500/20">
              <h3 className="text-sm font-mono font-bold uppercase tracking-wider text-slate-300 mb-4 flex items-center gap-2">
                <Layers className="w-4 h-4 text-sky-400" />
                <span>Replica Placement & Leader Matrix</span>
              </h3>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4 font-mono text-xs">
                <div className="p-4 rounded-xl bg-slate-900/80 border border-sky-500/30">
                  <div className="flex items-center justify-between mb-2">
                    <span className="font-bold text-sky-400 text-sm">Shard 0</span>
                    <span className="text-[10px] px-2 py-0.5 rounded bg-sky-950 text-sky-300 border border-sky-500/40">
                      Leader: Node A
                    </span>
                  </div>
                  <p className="text-slate-400 text-[11px] mb-2">Replicas: Node A, Node B, Node C</p>
                  <div className="text-[11px] text-emerald-400">3/3 Quorum Active</div>
                </div>

                <div className="p-4 rounded-xl bg-slate-900/80 border border-purple-500/30">
                  <div className="flex items-center justify-between mb-2">
                    <span className="font-bold text-purple-400 text-sm">Shard 1</span>
                    <span className="text-[10px] px-2 py-0.5 rounded bg-purple-950 text-purple-300 border border-purple-500/40">
                      Leader: Node D
                    </span>
                  </div>
                  <p className="text-slate-400 text-[11px] mb-2">Replicas: Node D, Node E, Node A</p>
                  <div className="text-[11px] text-emerald-400">3/3 Quorum Active</div>
                </div>

                <div className="p-4 rounded-xl bg-slate-900/80 border border-emerald-500/30">
                  <div className="flex items-center justify-between mb-2">
                    <span className="font-bold text-emerald-400 text-sm">Shard 2</span>
                    <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 border border-emerald-500/40">
                      Leader: Node B
                    </span>
                  </div>
                  <p className="text-slate-400 text-[11px] mb-2">Replicas: Node B, Node C, Node D</p>
                  <div className="text-[11px] text-emerald-400">3/3 Quorum Active</div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Tab 2: Key-Value Query Console */}
        {activeTab === "console" && (
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 animate-fadeIn">
            {/* Request Builder */}
            <div className="lg:col-span-6 glass-panel p-6 rounded-2xl border border-sky-500/25">
              <h3 className="text-sm font-mono font-bold uppercase tracking-wider text-slate-300 mb-4 flex items-center gap-2">
                <Terminal className="w-4 h-4 text-sky-400" />
                <span>Execute Cluster Query</span>
              </h3>

              <div className="space-y-4 font-mono text-xs">
                <div>
                  <label className="text-slate-400 text-[11px] block mb-1">HTTP Method</label>
                  <div className="flex gap-2">
                    {(["PUT", "GET", "DELETE"] as const).map((m) => (
                      <button
                        key={m}
                        onClick={() => setMethod(m)}
                        className={`px-4 py-2 rounded-lg font-bold transition-all ${
                          method === m
                            ? m === "PUT"
                              ? "bg-sky-600 text-white"
                              : m === "GET"
                              ? "bg-emerald-600 text-white"
                              : "bg-red-600 text-white"
                            : "bg-slate-900 text-slate-400 border border-slate-800 hover:border-slate-700"
                        }`}
                      >
                        {m}
                      </button>
                    ))}
                  </div>
                </div>

                <div>
                  <label className="text-slate-400 text-[11px] block mb-1">Key Name</label>
                  <input
                    type="text"
                    value={key}
                    onChange={(e) => setKey(e.target.value)}
                    className="w-full bg-slate-950 p-2.5 rounded-lg border border-slate-800 text-slate-200 focus:outline-none focus:border-sky-500"
                  />
                </div>

                {method === "PUT" && (
                  <div>
                    <label className="text-slate-400 text-[11px] block mb-1">Value Payload (JSON or String)</label>
                    <textarea
                      value={value}
                      onChange={(e) => setValue(e.target.value)}
                      rows={4}
                      className="w-full bg-slate-950 p-2.5 rounded-lg border border-slate-800 text-slate-200 focus:outline-none focus:border-sky-500"
                    />
                  </div>
                )}

                <button
                  onClick={executeQuery}
                  disabled={isExecuting}
                  className="w-full py-3 rounded-xl bg-gradient-to-r from-sky-600 to-indigo-600 hover:from-sky-500 hover:to-indigo-500 text-white font-bold flex items-center justify-center gap-2 shadow-lg shadow-sky-600/30 transition-all disabled:opacity-50"
                >
                  {isExecuting ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Send className="w-4 h-4" />}
                  <span>{isExecuting ? "Executing Consensus..." : "Submit to Cluster"}</span>
                </button>
              </div>
            </div>

            {/* Response Console */}
            <div className="lg:col-span-6 bg-slate-950 p-6 rounded-2xl border border-slate-800 font-mono text-xs flex flex-col justify-between">
              <div>
                <div className="flex items-center justify-between pb-3 mb-4 border-b border-slate-800 text-slate-400 text-[11px]">
                  <div className="flex items-center gap-2">
                    <Code2 className="w-4 h-4 text-sky-400" />
                    <span>Response Output</span>
                  </div>
                  <span className="text-emerald-400 font-bold">Latency: 1.4ms</span>
                </div>

                <pre className="text-slate-300 text-[11px] leading-relaxed overflow-x-auto whitespace-pre-wrap">
                  {responseLog}
                </pre>
              </div>

              <div className="pt-4 mt-6 border-t border-slate-800 text-[11px] text-slate-400 flex items-center justify-between">
                <span>Consensus Verification: 2/3 Quorum Match</span>
                <span className="text-sky-400">LSM Memtable Committed</span>
              </div>
            </div>
          </div>
        )}

        {/* Tab 3: Event Stream */}
        {activeTab === "events" && (
          <div className="glass-panel p-6 rounded-2xl border border-sky-500/20 font-mono text-xs space-y-3 animate-fadeIn">
            <div className="flex items-center justify-between pb-3 border-b border-slate-800">
              <div className="flex items-center gap-2 text-slate-300">
                <Radio className="w-4 h-4 text-emerald-400 animate-pulse" />
                <span className="font-bold">Live WebSocket Event Relay (/ws/events)</span>
              </div>
              <span className="text-[10px] text-slate-400">Auto-scrolling stream</span>
            </div>

            <div className="space-y-2 text-[11px]">
              <div className="p-2.5 rounded-lg bg-slate-950/80 border border-slate-800 flex items-center justify-between text-slate-300">
                <span className="text-emerald-400">[ENTRY_COMMITTED]</span>
                <span>Key: user:101 | Shard 1 | Index: 142</span>
                <span className="text-slate-500">12ms ago</span>
              </div>
              <div className="p-2.5 rounded-lg bg-slate-950/80 border border-slate-800 flex items-center justify-between text-slate-300">
                <span className="text-sky-400">[HEARTBEAT_ACK]</span>
                <span>Shard 1 Leader (Node D) acknowledged by Node E</span>
                <span className="text-slate-500">25ms ago</span>
              </div>
              <div className="p-2.5 rounded-lg bg-slate-950/80 border border-slate-800 flex items-center justify-between text-slate-300">
                <span className="text-purple-400">[LEADER_CONFIRMED]</span>
                <span>Shard 0: Node A | Shard 1: Node D | Shard 2: Node B</span>
                <span className="text-slate-500">80ms ago</span>
              </div>
              <div className="p-2.5 rounded-lg bg-slate-950/80 border border-slate-800 flex items-center justify-between text-slate-300">
                <span className="text-amber-400">[MEMTABLE_ROTATION]</span>
                <span>Active Memtable exceeded 64MB &rarr; rotated to ImmutableMemtable</span>
                <span className="text-slate-500">240ms ago</span>
              </div>
              <div className="p-2.5 rounded-lg bg-slate-950/80 border border-slate-800 flex items-center justify-between text-slate-300">
                <span className="text-emerald-400">[SSTABLE_FLUSH]</span>
                <span>Flushed SSTable level 0 file 000012.sst with Sparse Index & Bloom</span>
                <span className="text-slate-500">510ms ago</span>
              </div>
            </div>
          </div>
        )}
      </main>
    </div>
  );
}
