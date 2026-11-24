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
  AlertTriangle,
  RefreshCw,
} from "lucide-react";

interface PipelineStep {
  id: number;
  name: string;
  short: string;
  icon: any;
  summary: string;
  detail: string;
  protocolLog: string;
}

const NORMAL_STEPS: PipelineStep[] = [
  {
    id: 1,
    name: "CLIENT INITIATION",
    short: "CLIENT",
    icon: Send,
    summary: "Client issues HTTP PUT /kv/user:101 with JSON payload",
    detail:
      "A client submits a key-value mutation with optional X-Idempotency-Key and X-Request-ID headers. The request hits any available physical node in the cluster.",
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
    short: "REPLICATION",
    icon: Send,
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
    short: "QUORUM",
    icon: CheckCircle2,
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
    short: "STORAGE",
    icon: HardDrive,
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

const FAILURE_STEPS: PipelineStep[] = [
  {
    id: 1,
    name: "CLIENT INITIATION",
    short: "CLIENT",
    icon: Send,
    summary: "Client issues HTTP PUT /kv/order:992 to Node A gateway",
    detail:
      "Client submits write request mapped to Shard 1. Gateway on Node A attempts to proxy the proposal to known leader Node D.",
    protocolLog: `PUT /kv/order:992 HTTP/1.1
Host: 127.0.0.1:8000
X-Request-ID: failover-sim-8812

{"value": "{\\"amount\\":450,\\"state\\":\\"paid\\"}"}`,
  },
  {
    id: 2,
    name: "LEADER OUTAGE DETECTED",
    short: "CRASH / TIMEOUT",
    icon: AlertTriangle,
    summary: "Leader Node D becomes unreachable; RPC connection fails",
    detail:
      "Node D suffers an abrupt network partition or process termination. Heartbeat ticks stop and gateway connection times out with 503.",
    protocolLog: `[ROUTER] Forwarding PUT to Shard 1 leader Node D (127.0.0.1:8003)
[TRANSPORT] dial tcp 127.0.0.1:8003: connect: connection refused
[ROUTER] Shard 1 leader unreachable; invalidating cached leader`,
  },
  {
    id: 3,
    name: "ELECTION TIMER EXPIRES",
    short: "ELECTION TRIGGER",
    icon: Clock,
    summary: "Follower Node E election timer expires (180ms without heartbeat)",
    detail:
      "Surviving follower Node E detects absence of leader heartbeats. It transitions to Candidate state, increments term to 5, and votes for itself.",
    protocolLog: `[RAFT FOLLOWER] Node E: heartbeat timeout (180ms elapsed)
[RAFT STATE] Node E: Follower -> Candidate (Term 5)
[RAFT CONSENSUS] Voted for self (Node E) in Term 5`,
  },
  {
    id: 4,
    name: "REQUEST VOTE RPC",
    short: "VOTE GATHER",
    icon: Send,
    summary: "Candidate Node E requests votes from surviving replica Node A",
    detail:
      "Candidate Node E broadcasts RequestVote RPC to Node A containing its lastLogIndex and lastLogTerm to ensure log up-to-dateness.",
    protocolLog: `[RPC BROADCAST] RequestVote -> Node A (Term: 5, LastLogIndex: 141)
[PEER RECV] Node A grants vote: Candidate log is at least as up-to-date`,
  },
  {
    id: 5,
    name: "LEADER ELECTED",
    short: "NEW LEADER",
    icon: ShieldCheck,
    summary: "Candidate Node E wins majority quorum (2/2 votes) and becomes Leader",
    detail:
      "Node E receives votes from self and Node A (2 of 3 replicas). It ascends to Leader for Term 5 and broadcasts initial heartbeats.",
    protocolLog: `[CONSENSUS] Quorum achieved: 2/3 votes (Node E, Node A)
[RAFT STATE] Node E: Candidate -> Leader (Term 5)
[RAFT BROADCAST] Initial empty AppendEntries heartbeat sent to Node A`,
  },
  {
    id: 6,
    name: "DISCOVERY & RETRY",
    short: "DISCOVERY",
    icon: RefreshCw,
    summary: "API Router detects new leader via bounded failover backoff",
    detail:
      "The client gateway retries with exponential jitter (50ms). It queries cluster shard metadata, discovers Node E as the new Shard 1 leader, and forwards the write.",
    protocolLog: `[API ROUTER] Refreshing Shard 1 leader tracker
[API ROUTER] Discovered new Shard 1 leader: Node E (127.0.0.1:8004)
[API ROUTER] Retrying forward to Node E with original Idempotency-Key`,
  },
  {
    id: 7,
    name: "REPLICATION ON TERM 5",
    short: "QUORUM COMMIT",
    icon: CheckCircle2,
    summary: "New leader appends and commits write with majority on Term 5",
    detail:
      "Node E durably appends key order:992 at LogIndex 142 in Term 5. Node A replicates and acknowledges. Quorum is achieved.",
    protocolLog: `[RAFT LEADER] Propose(op="SET", key="order:992", term=5) -> LogIndex 142
[WAL] Durably appended to Node E disk
[RPC] Replicated to Node A -> Quorum confirmed (commitIndex=142)`,
  },
  {
    id: 8,
    name: "SUCCESSFUL COMMIT",
    short: "RECOVERY OK",
    icon: CheckCircle,
    summary: "HTTP 200 OK returned with zero data loss & seamless failover",
    detail:
      "Client receives confirmation of durability. Linearizable consistency was strictly preserved throughout the leader transition.",
    protocolLog: `HTTP/1.1 200 OK
Content-Type: application/json
X-Consensus-Term: 5
X-Commit-Index: 142
X-Failover-Hops: 1

{
  "status": "ok",
  "key": "order:992",
  "shard_id": 1,
  "index": 142,
  "leader": "node-e"
}`,
  },
];

export default function WritePipeline() {
  const [mode, setMode] = useState<"normal" | "failure">("normal");
  const [currentStep, setCurrentStep] = useState<number>(1);
  const [isPlaying, setIsPlaying] = useState<boolean>(true);

  const steps = mode === "normal" ? NORMAL_STEPS : FAILURE_STEPS;

  useEffect(() => {
    if (!isPlaying) return;
    const interval = setInterval(() => {
      setCurrentStep((prev) => (prev >= steps.length ? 1 : prev + 1));
    }, 2800);
    return () => clearInterval(interval);
  }, [isPlaying, steps.length]);

  const active = steps.find((s) => s.id === currentStep) || steps[0];

  return (
    <section id="pipeline" className="py-28 relative overflow-hidden bg-white border-t border-[#DCE5F0]">
      {/* Subtle background tech grid in light infrastructure style */}
      <div className="absolute inset-0 bg-tech-grid opacity-60 pointer-events-none" />

      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 relative z-10">
        {/* Section Header */}
        <div className="text-center max-w-3xl mx-auto mb-16">
          <div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full bg-[#F4F7FC] border border-[#DCE5F0] shadow-sm mb-4">
            <Clock className="w-4 h-4 text-[#0891D5]" />
            <span className="text-xs font-mono font-semibold uppercase tracking-wider text-[#0891D5]">
              LIFECYCLE OF A WRITE
            </span>
          </div>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-extrabold text-[#17243A] tracking-tight">
            From a single write to a replicated operation.
          </h2>
          <p className="mt-4 text-base sm:text-lg text-[#64748B] font-sans">
            Follow the journey of a client write request as it traverses API routing, hash ring
            assignment, Raft consensus replication, quorum commitment, and LSM persistence.
          </p>
        </div>

        {/* Mode Selector Toggle: NORMAL OPERATION | LEADER FAILURE */}
        <div className="flex justify-center mb-8">
          <div className="inline-flex p-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] shadow-sm font-mono text-xs">
            <button
              onClick={() => {
                setMode("normal");
                setCurrentStep(1);
              }}
              className={`flex items-center gap-2 px-4 py-2 rounded-lg font-bold transition-all ${
                mode === "normal"
                  ? "bg-white text-[#0891D5] border border-[#DCE5F0] shadow-sm"
                  : "text-[#64748B] hover:text-[#17243A]"
              }`}
            >
              <CheckCircle2 className="w-3.5 h-3.5 text-[#10B981]" />
              <span>NORMAL OPERATION</span>
            </button>

            <button
              onClick={() => {
                setMode("failure");
                setCurrentStep(1);
              }}
              className={`flex items-center gap-2 px-4 py-2 rounded-lg font-bold transition-all ${
                mode === "failure"
                  ? "bg-white text-rose-600 border border-rose-200 shadow-sm"
                  : "text-[#64748B] hover:text-rose-600"
              }`}
            >
              <AlertTriangle className="w-3.5 h-3.5 text-rose-500" />
              <span>LEADER FAILURE & RE-ELECTION</span>
            </button>
          </div>
        </div>

        {/* Playback Controls & Step Progress Bar */}
        <div className="flex flex-wrap items-center justify-between gap-4 p-4 light-card rounded-2xl mb-8">
          <div className="flex items-center gap-2.5">
            <button
              onClick={() => setIsPlaying(!isPlaying)}
              className="flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-to-r from-[#0891D5] to-[#7965E8] hover:from-[#0284c7] hover:to-[#6d28d9] text-white text-xs font-semibold shadow-md shadow-sky-500/20 transition-all"
            >
              {isPlaying ? <Pause className="w-3.5 h-3.5" /> : <Play className="w-3.5 h-3.5" />}
              <span>{isPlaying ? "Pause Simulation" : "Play Simulation"}</span>
            </button>
            <button
              onClick={() => {
                setCurrentStep(1);
                setIsPlaying(false);
              }}
              className="p-2 rounded-lg bg-[#F4F7FC] hover:bg-slate-100 text-[#17243A] border border-[#DCE5F0] transition-colors"
              title="Reset to Step 1"
            >
              <RotateCcw className="w-4 h-4 text-[#64748B]" />
            </button>
          </div>

          <div className="flex items-center gap-2 font-mono text-xs">
            <button
              onClick={() => {
                setIsPlaying(false);
                setCurrentStep((prev) => (prev <= 1 ? steps.length : prev - 1));
              }}
              className="p-2 rounded-lg bg-[#F4F7FC] hover:bg-slate-100 text-[#17243A] border border-[#DCE5F0] transition-colors"
            >
              <ChevronLeft className="w-4 h-4 text-[#64748B]" />
            </button>
            <span className="text-xs text-[#17243A] px-2 font-semibold">
              Step <span className="text-[#0891D5] font-bold">{currentStep}</span> of{" "}
              <span>{steps.length}</span>
            </span>
            <button
              onClick={() => {
                setIsPlaying(false);
                setCurrentStep((prev) => (prev >= steps.length ? 1 : prev + 1));
              }}
              className="p-2 rounded-lg bg-[#F4F7FC] hover:bg-slate-100 text-[#17243A] border border-[#DCE5F0] transition-colors"
            >
              <ChevronRight className="w-4 h-4 text-[#64748B]" />
            </button>
          </div>
        </div>

        {/* Horizontal Pipeline Steps Track with Blue-to-Purple Gradient Track */}
        <div className="overflow-x-auto pb-4 mb-8">
          <div className="flex items-center min-w-[880px] justify-between relative px-4 py-2">
            {/* Background connecting track */}
            <div className="absolute top-1/2 left-8 right-8 -translate-y-1/2 h-1.5 bg-[#E2E8F0] z-0 rounded-full" />

            {/* Active illuminated track with Blue-to-Purple gradient */}
            <div
              className="absolute top-1/2 left-8 -translate-y-1/2 h-1.5 bg-gradient-to-r from-[#0891D5] via-[#4f46e5] to-[#7965E8] transition-all duration-500 z-0 rounded-full"
              style={{
                width: `${((currentStep - 1) / (steps.length - 1)) * 92}%`,
              }}
            />

            {steps.map((step) => {
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
                    className={`w-12 h-12 rounded-xl flex items-center justify-center transition-all duration-300 ${
                      isCurrent
                        ? "bg-gradient-to-r from-[#0891D5] to-[#7965E8] text-white shadow-[0_8px_25px_rgba(8,145,213,0.35)] scale-110 border-2 border-white ring-2 ring-[#0891D5]/40"
                        : isPast
                        ? "bg-white text-[#0891D5] border border-[#CBD5E1] shadow-sm"
                        : "bg-[#F8FAFC] text-[#94A3B8] border border-[#DCE5F0] hover:border-slate-400"
                    }`}
                  >
                    <Icon className="w-5 h-5" />
                  </div>
                  <span
                    className={`mt-2 text-[11px] font-mono tracking-tight font-semibold transition-colors text-center ${
                      isCurrent
                        ? "text-[#0891D5]"
                        : isPast
                        ? "text-[#17243A]"
                        : "text-[#94A3B8] group-hover:text-[#64748B]"
                    }`}
                  >
                    {step.short}
                  </span>
                </div>
              );
            })}
          </div>
        </div>

        {/* Selected Step Deep Dive Card: Light Content + Modern Dark Developer Terminal */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 light-card p-8 sm:p-10 rounded-2xl shadow-xl">
          {/* Step description */}
          <div className="lg:col-span-7 flex flex-col justify-between">
            <div>
              <div className="flex items-center gap-2 mb-3">
                <span className="text-xs font-mono font-bold px-3 py-1 rounded-full bg-sky-50 text-[#0891D5] border border-sky-200">
                  PHASE {active.id}
                </span>
                <span className="text-xs font-mono text-[#64748B] uppercase tracking-wider">
                  {active.name}
                </span>
              </div>
              <h3 className="text-2xl sm:text-3xl font-extrabold text-[#17243A] mb-3">
                {active.summary}
              </h3>
              <p className="text-base text-[#64748B] leading-relaxed font-sans mb-6">
                {active.detail}
              </p>
            </div>

            <div className="flex items-center gap-4 text-xs font-mono text-[#64748B] pt-4 border-t border-[#DCE5F0]">
              <div className="flex items-center gap-1.5 text-[#10B981] font-semibold">
                <CheckCircle2 className="w-4 h-4" />
                <span>Strict Linearizable Ordering</span>
              </div>
              <span>•</span>
              <div>Zero Inconsistent Reads</div>
            </div>
          </div>

          {/* Modern Dark Developer Terminal for Wire Protocol Trace (Soft Hybrid) */}
          <div className="lg:col-span-5 bg-[#0B1120] p-5 rounded-xl border border-[#1E293B] shadow-inner font-mono text-xs flex flex-col text-[#E2E8F0]">
            <div className="flex items-center justify-between pb-3 mb-3 border-b border-[#1E293B] text-[11px] text-slate-400">
              <div className="flex items-center gap-2">
                <div className="flex gap-1.5">
                  <span className="w-2.5 h-2.5 rounded-full bg-rose-500/80 inline-block" />
                  <span className="w-2.5 h-2.5 rounded-full bg-amber-500/80 inline-block" />
                  <span className="w-2.5 h-2.5 rounded-full bg-emerald-500/80 inline-block" />
                </div>
                <span className="ml-2 font-mono text-slate-300">protocol-trace.log</span>
              </div>
              <span className="text-[10px] text-emerald-400 font-bold px-1.5 py-0.5 rounded bg-emerald-950/60 border border-emerald-500/30">
                200 OK
              </span>
            </div>
            <pre className="text-slate-300 text-[11px] leading-relaxed overflow-x-auto whitespace-pre-wrap flex-1 font-mono">
              {active.protocolLog}
            </pre>
          </div>
        </div>
      </div>
    </section>
  );
}
