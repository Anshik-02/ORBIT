"use client";

import React, { useState, useEffect } from "react";
import {
  Play,
  Pause,
  RotateCcw,
  ChevronRight,
  ChevronLeft,
  CheckCircle,
  Database,
  Send,
  Cpu,
  Layers,
  ShieldCheck,
  HardDrive,
  CheckCircle2,
  Clock,
  Code2,
} from "lucide-react";

interface PipelineStep {
  id: number;
  name: string;
  short: string;
  icon: any;
  color: string;
  summary: string;
  detail: string;
  protocolLog: string;
}

const STEPS: PipelineStep[] = [
  {
    id: 1,
    name: "CLIENT INITIATION",
    short: "CLIENT",
    icon: Send,
    color: "#38bdf8",
    summary: "Client issues HTTP PUT /kv/user:101 with JSON payload",
    detail:
      "A client application submits a write request with optional X-Idempotency-Key and X-Request-ID headers. The request hits any available physical node in the cluster.",
    protocolLog: `PUT /kv/user:101 HTTP/1.1
Host: 127.0.0.1:8000
Content-Type: application/json
X-Request-ID: 7a8f90bc12
X-Idempotency-Key: idemp-write-9912

{"value": "{\\"tier\\":\\"premium\\",\\"status\\":\\"active\\"}"}`,
  },
  {
    id: 2,
    name: "API ROUTER INGRESS",
    short: "API ROUTER",
    icon: Cpu,
    color: "#818cf8",
    summary: "Gateway parses payload and checks idempotency cache",
    detail:
      "The HTTP gateway validates payload size limits (16MB), checks whether the idempotency key was previously committed to prevent duplicate operations, and establishes a context timeout.",
    protocolLog: `[API ROUTER] Validated key="user:101", len(val)=42 bytes
[API ROUTER] Idempotency cache MISS for key="idemp-write-9912"
[API ROUTER] Initializing forwardContext deadline=5000ms`,
  },
  {
    id: 3,
    name: "SHARD ROUTING",
    short: "SHARD ROUTING",
    icon: Layers,
    color: "#c084fc",
    summary: "Consistent Hash Ring maps key to Shard 1 (Leader: Node D)",
    detail:
      "The 256-vnode consistent hash ring hashes user:101 via Murmur3 to token 0x82A1B7F0, assigning it to Shard 1. If the current node is not the leader, forwardWithFailover routes the request.",
    protocolLog: `[ROUTER] Murmur3("user:101") = 0x82A1B7F0
[ROUTER] Mapped to Logical Shard 1 (Replicas: Node D, Node E, Node A)
[ROUTER] Target Shard 1 Active Leader = Node D (127.0.0.1:8003)`,
  },
  {
    id: 4,
    name: "RAFT LEADER PROPOSE",
    short: "RAFT LEADER",
    icon: ShieldCheck,
    color: "#f59e0b",
    summary: "Shard Leader appends command to local Raft log & WAL",
    detail:
      "Node D verifies it holds a valid leadership lease/term. It allocates LogIndex 142 in Term 4, writes an append-only entry to disk storage, and notifies the consensus replication worker.",
    protocolLog: `[RAFT LEADER] Propose(op="SET", key="user:101", term=4)
[RAFT LEADER] Assigned LogIndex=142
[WAL] Durably appended LogEntry{Index: 142, Term: 4, Op: SET} CRC32=0x9FA821`,
  },
  {
    id: 5,
    name: "LOG REPLICATION",
    short: "LOG REPLICATION",
    icon: Send,
    color: "#a855f7",
    summary: "Leader dispatches AppendEntries RPCs in parallel to followers",
    detail:
      "Node D sends AppendEntries RPCs containing log entry 142 to followers Node E and Node A over dedicated TCP Raft transport channels.",
    protocolLog: `[RPC BROADCAST] AppendEntries -> Node E (127.0.0.1:9004) [Index: 142]
[RPC BROADCAST] AppendEntries -> Node A (127.0.0.1:9000) [Index: 142]
[PEER RECV] Node E acknowledged matchIndex=142 in 0.8ms`,
  },
  {
    id: 6,
    name: "QUORUM COMMIT",
    short: "QUORUM COMMIT",
    icon: CheckCircle2,
    color: "#10b981",
    summary: "Majority quorum (2 of 3 replicas) acknowledged write",
    detail:
      "Node D receives match confirmation from Node E. Having achieved majority quorum (2/3 replicas), entry 142 is officially committed and commitIndex advances to 142.",
    protocolLog: `[CONSENSUS] Quorum achieved: 2/3 replicas (Node D, Node E)
[CONSENSUS] commitIndex advanced from 141 -> 142
[CONSENSUS] State transition: UNCOMMITTED -> COMMITTED`,
  },
  {
    id: 7,
    name: "STORAGE ENGINE APPLY",
    short: "STORAGE ENGINE",
    icon: HardDrive,
    color: "#38bdf8",
    summary: "Operation is applied to active in-memory Memtable",
    detail:
      "The state machine executes the committed entry. The key-value pair is inserted into the active Memtable and marked durable. lastApplied is updated to 142.",
    protocolLog: `[STATE MACHINE] Applying LogEntry Index=142 to LSM Engine
[MEMTABLE] Put key="user:101" size=42 bytes
[OBSERVABILITY] Emitted cluster event: ENTRY_COMMITTED`,
  },
  {
    id: 8,
    name: "CLIENT RESPONSE",
    short: "RESPONSE",
    icon: CheckCircle,
    color: "#22c55e",
    summary: "HTTP 200 OK returned with linearizable read guarantee",
    detail:
      "The leader writes the HTTP response back to the client with execution latency, assigned shard ID, and updated commit index.",
    protocolLog: `HTTP/1.1 200 OK
Content-Type: application/json
X-Request-ID: 7a8f90bc12
X-Consensus-Term: 4
X-Commit-Index: 142

{
  "status": "ok",
  "key": "user:101",
  "shard_id": 1,
  "index": 142
}`,
  },
];

export default function WritePipeline() {
  const [currentStep, setCurrentStep] = useState<number>(1);
  const [isPlaying, setIsPlaying] = useState<boolean>(true);

  useEffect(() => {
    if (!isPlaying) return;
    const interval = setInterval(() => {
      setCurrentStep((prev) => (prev >= STEPS.length ? 1 : prev + 1));
    }, 2800);
    return () => clearInterval(interval);
  }, [isPlaying]);

  const active = STEPS.find((s) => s.id === currentStep)!;

  return (
    <section id="pipeline" className="py-24 relative overflow-hidden bg-tech-grid border-t border-slate-900">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 relative z-10">
        {/* Section Header */}
        <div className="text-center max-w-3xl mx-auto mb-16">
          <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-sky-950/70 border border-sky-500/30 mb-4">
            <Clock className="w-3.5 h-3.5 text-sky-400" />
            <span className="text-xs font-mono font-semibold uppercase tracking-wider text-sky-300">
              LIFECYCLE OF A WRITE
            </span>
          </div>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-extrabold text-white tracking-tight">
            From a single write to a replicated operation.
          </h2>
          <p className="mt-4 text-base sm:text-lg text-slate-400 font-sans">
            Follow the journey of a client write request as it traverses API routing, hash ring
            assignment, Raft consensus replication, quorum commitment, and LSM persistence.
          </p>
        </div>

        {/* Playback Controls & Step Progress Bar */}
        <div className="flex flex-wrap items-center justify-between gap-4 p-4 glass-panel rounded-2xl border border-sky-500/20 mb-8">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setIsPlaying(!isPlaying)}
              className="flex items-center gap-2 px-3.5 py-1.5 rounded-lg bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold shadow-md shadow-sky-600/30 transition-all"
            >
              {isPlaying ? <Pause className="w-3.5 h-3.5" /> : <Play className="w-3.5 h-3.5" />}
              <span>{isPlaying ? "Pause Simulation" : "Play Simulation"}</span>
            </button>
            <button
              onClick={() => {
                setCurrentStep(1);
                setIsPlaying(false);
              }}
              className="p-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs transition-colors"
              title="Reset to Step 1"
            >
              <RotateCcw className="w-4 h-4" />
            </button>
          </div>

          <div className="flex items-center gap-2">
            <button
              onClick={() => {
                setIsPlaying(false);
                setCurrentStep((prev) => (prev <= 1 ? STEPS.length : prev - 1));
              }}
              className="p-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <span className="text-xs font-mono text-slate-300 px-2">
              Step <span className="text-sky-400 font-bold">{currentStep}</span> of{" "}
              <span>{STEPS.length}</span>
            </span>
            <button
              onClick={() => {
                setIsPlaying(false);
                setCurrentStep((prev) => (prev >= STEPS.length ? 1 : prev + 1));
              }}
              className="p-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Horizontal Pipeline Steps Track (Scrollable on mobile) */}
        <div className="overflow-x-auto pb-4 mb-8">
          <div className="flex items-center min-w-[880px] justify-between relative px-2">
            {/* Background connecting track */}
            <div className="absolute top-1/2 left-6 right-6 -translate-y-1/2 h-1 bg-slate-800 z-0" />
            {/* Active illuminated track */}
            <div
              className="absolute top-1/2 left-6 -translate-y-1/2 h-1 bg-gradient-to-r from-sky-500 via-indigo-500 to-emerald-400 transition-all duration-500 z-0"
              style={{
                width: `${((currentStep - 1) / (STEPS.length - 1)) * 95}%`,
              }}
            />

            {STEPS.map((step) => {
              const Icon = step.icon;
              const isPast = step.id <= currentStep;
              const isCurrent = step.id === currentStep;

              return (
                <div
                  key={step.id}
                  onClick={() => {
                    setIsPlaying(false);
                    setCurrentStep(step.id);
                  }}
                  className="flex flex-col items-center cursor-pointer group z-10"
                >
                  <div
                    className={`w-11 h-11 rounded-xl flex items-center justify-center transition-all duration-300 ${
                      isCurrent
                        ? "bg-sky-500 text-white shadow-[0_0_25px_rgba(56,189,248,0.7)] scale-110 border-2 border-white"
                        : isPast
                        ? "bg-slate-800 text-sky-400 border border-sky-500/40"
                        : "bg-slate-900 text-slate-500 border border-slate-800 hover:border-slate-600"
                    }`}
                  >
                    <Icon className="w-5 h-5" />
                  </div>
                  <span
                    className={`mt-2 text-[11px] font-mono tracking-tight font-semibold transition-colors text-center ${
                      isCurrent
                        ? "text-sky-300"
                        : isPast
                        ? "text-slate-300"
                        : "text-slate-500 group-hover:text-slate-400"
                    }`}
                  >
                    {step.short}
                  </span>
                </div>
              );
            })}
          </div>
        </div>

        {/* Selected Step Deep Dive Card */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 glass-panel p-6 sm:p-8 rounded-2xl border border-sky-500/30 shadow-2xl">
          {/* Step description */}
          <div className="lg:col-span-7 flex flex-col justify-between">
            <div>
              <div className="flex items-center gap-2 mb-3">
                <span className="text-xs font-mono font-bold px-2.5 py-1 rounded-md bg-sky-500/20 text-sky-300 border border-sky-500/40">
                  PHASE {active.id}
                </span>
                <span className="text-xs font-mono text-slate-400 uppercase tracking-wider">
                  {active.name}
                </span>
              </div>
              <h3 className="text-xl sm:text-2xl font-bold text-white mb-3">{active.summary}</h3>
              <p className="text-sm text-slate-300 leading-relaxed font-sans mb-6">{active.detail}</p>
            </div>

            <div className="flex items-center gap-4 text-xs font-mono text-slate-400 pt-4 border-t border-slate-800">
              <div className="flex items-center gap-1.5 text-emerald-400">
                <CheckCircle2 className="w-4 h-4" />
                <span>Linearizable Write Guarantee</span>
              </div>
              <span>•</span>
              <div>Quorum Strict Consensus</div>
            </div>
          </div>

          {/* Step Protocol Telemetry / Wire Log */}
          <div className="lg:col-span-5 bg-slate-950 p-4 rounded-xl border border-slate-800 font-mono text-xs flex flex-col">
            <div className="flex items-center justify-between pb-2 mb-3 border-b border-slate-800 text-[11px] text-slate-400">
              <div className="flex items-center gap-2">
                <Code2 className="w-3.5 h-3.5 text-sky-400" />
                <span>Live Wire Protocol Trace</span>
              </div>
              <span className="text-[10px] text-emerald-400 font-bold">200 OK</span>
            </div>
            <pre className="text-slate-300 text-[11px] leading-relaxed overflow-x-auto whitespace-pre-wrap flex-1">
              {active.protocolLog}
            </pre>
          </div>
        </div>
      </div>
    </section>
  );
}
