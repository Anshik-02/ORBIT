'use client';

import React, { useState } from 'react';
import {
  CheckCircle2,
  XCircle,
  Clock,
  HardDrive,
  Copy,
  Check,
  Server,
  Layers,
  ArrowRight,
} from 'lucide-react';

export interface QueryResponse {
  statusCode: number;
  statusText: string;
  durationMs: number;
  body: any;
  targetNode: string;
  shardId: number;
  leaderId: string;
  responseBytes: number;
  error?: string;
  timestamp: string;
}

interface ResponseInspectorProps {
  response: QueryResponse | null;
  isExecuting: boolean;
}

export default function ResponseInspector({
  response,
  isExecuting,
}: ResponseInspectorProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    if (!response) return;
    navigator.clipboard.writeText(JSON.stringify(response.body, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  if (isExecuting) {
    return (
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-8 shadow-xs flex flex-col items-center justify-center min-h-[300px] text-center font-mono">
        <div className="w-8 h-8 rounded-full border-2 border-[#0891D5] border-t-transparent animate-spin mb-3" />
        <span className="text-sm font-bold text-[#17243A]">
          Executing Consensus Operation...
        </span>
        <span className="text-xs text-[#64748B] mt-1">
          Waiting for Raft proposal commitment & majority acknowledgments
        </span>
      </div>
    );
  }

  if (!response) {
    return (
      <div className="bg-white border border-[#DCE5F0] rounded-2xl p-8 shadow-xs flex flex-col items-center justify-center min-h-[300px] text-center font-mono">
        <span className="p-3 rounded-2xl bg-[#F4F7FC] text-[#64748B] mb-3">
          <Server className="w-6 h-6 text-[#0891D5]" />
        </span>
        <h4 className="text-sm font-bold text-[#17243A]">No Query Executed Yet</h4>
        <p className="text-xs text-[#64748B] max-w-sm mt-1">
          Select an operation method above and press Execute to read or write keys from the cluster.
        </p>
      </div>
    );
  }

  const isSuccess = response.statusCode >= 200 && response.statusCode < 300;

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono">
      {/* 01. RESPONSE HEADER WITH STATUS & METRICS */}
      <div className="flex flex-wrap items-center justify-between gap-3 pb-3 border-b border-[#DCE5F0]">
        <div className="flex items-center gap-2">
          {isSuccess ? (
            <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-bold text-xs shadow-2xs">
              <CheckCircle2 className="w-3.5 h-3.5" />
              <span>{response.statusCode} {response.statusText}</span>
            </span>
          ) : (
            <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-rose-50 border border-rose-200 text-rose-600 font-bold text-xs shadow-2xs">
              <XCircle className="w-3.5 h-3.5" />
              <span>{response.statusCode} {response.statusText}</span>
            </span>
          )}

          <span className="text-xs text-[#64748B]">
            Shard {response.shardId} • Route: {response.targetNode.toUpperCase()} (Leader: {response.leaderId.toUpperCase()})
          </span>
        </div>

        {/* Latency & Size badges */}
        <div className="flex items-center gap-2 text-xs">
          <span className="px-2.5 py-1 rounded-lg bg-[#F8FAFC] border border-[#E2E8F0] text-[#17243A] font-semibold flex items-center gap-1">
            <Clock className="w-3 h-3 text-[#0891D5]" />
            <span>{response.durationMs.toFixed(1)} ms</span>
          </span>

          <span className="px-2.5 py-1 rounded-lg bg-[#F8FAFC] border border-[#E2E8F0] text-[#17243A] font-semibold flex items-center gap-1">
            <HardDrive className="w-3 h-3 text-[#7965E8]" />
            <span>{response.responseBytes} B</span>
          </span>

          <button
            onClick={handleCopy}
            className="p-1.5 rounded-lg bg-[#F8FAFC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#64748B] hover:text-[#0891D5] transition-colors"
            title="Copy Response JSON"
          >
            {copied ? <Check className="w-3.5 h-3.5 text-[#10B981]" /> : <Copy className="w-3.5 h-3.5" />}
          </button>
        </div>
      </div>

      {/* 02. RESPONSE BODY CODE VIEWER */}
      <div className="relative">
        <pre className="p-4 rounded-xl bg-[#0F172A] text-slate-100 font-mono text-xs overflow-x-auto leading-relaxed max-h-[350px]">
          <code>{JSON.stringify(response.body, null, 2)}</code>
        </pre>
      </div>
    </div>
  );
}
