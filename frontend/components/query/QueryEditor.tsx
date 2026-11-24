'use client';

import React, { useState, useEffect } from 'react';
import {
  Play,
  Trash2,
  CornerDownLeft,
  Server,
  ShieldCheck,
  Clock,
  AlertCircle,
  CheckCircle2,
} from 'lucide-react';

export type QueryMethod = 'GET' | 'PUT' | 'DELETE' | 'SCAN' | 'BATCH';
export type ConsistencyLevel = 'linearizable' | 'eventual';

export interface QueryRequest {
  method: QueryMethod;
  key: string;
  value?: string;
  targetNode: string;
  consistency: ConsistencyLevel;
  timeoutMs: number;
}

interface QueryEditorProps {
  onExecute: (req: QueryRequest) => void;
  isExecuting: boolean;
  initialQuery?: QueryRequest;
}

export default function QueryEditor({
  onExecute,
  isExecuting,
  initialQuery,
}: QueryEditorProps) {
  const [method, setMethod] = useState<QueryMethod>(initialQuery?.method || 'GET');
  const [key, setKey] = useState(initialQuery?.key || 'user:1001');
  const [value, setValue] = useState(
    initialQuery?.value || '{\n  "name": "Alice Cooper",\n  "tier": "enterprise",\n  "active": true\n}'
  );
  const [targetNode, setTargetNode] = useState(initialQuery?.targetNode || 'auto');
  const [consistency, setConsistency] = useState<ConsistencyLevel>(
    initialQuery?.consistency || 'linearizable'
  );
  const [timeoutMs, setTimeoutMs] = useState(initialQuery?.timeoutMs || 3000);
  const [jsonError, setJsonError] = useState<string | null>(null);

  useEffect(() => {
    if (initialQuery) {
      setMethod(initialQuery.method);
      setKey(initialQuery.key);
      if (initialQuery.value) setValue(initialQuery.value);
      if (initialQuery.targetNode) setTargetNode(initialQuery.targetNode);
      if (initialQuery.consistency) setConsistency(initialQuery.consistency);
      if (initialQuery.timeoutMs) setTimeoutMs(initialQuery.timeoutMs);
    }
  }, [initialQuery]);

  // Validate JSON on PUT and BATCH
  useEffect(() => {
    if (method === 'PUT' || method === 'BATCH') {
      try {
        JSON.parse(value);
        setJsonError(null);
      } catch (err: any) {
        setJsonError(err.message || 'Invalid JSON syntax');
      }
    } else {
      setJsonError(null);
    }
  }, [value, method]);

  // Keyboard shortcut Ctrl+Enter / Cmd+Enter
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        handleSubmit();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  });

  const handleSubmit = () => {
    if (!key.trim()) return;
    if (method === 'PUT' && jsonError) return;

    onExecute({
      method,
      key: key.trim(),
      value: method === 'PUT' ? value : undefined,
      targetNode,
      consistency,
      timeoutMs,
    });
  };

  const handleClear = () => {
    setKey('');
    setValue('{}');
    setJsonError(null);
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-5">
      {/* 01. METHOD & TARGET CONFIGURATION BAR */}
      <div className="flex flex-wrap items-center justify-between gap-3 pb-4 border-b border-[#DCE5F0]">
        {/* Method selector */}
        <div className="flex items-center gap-1 p-1 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] font-mono text-xs">
          <button
            onClick={() => setMethod('GET')}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all ${
              method === 'GET'
                ? 'bg-blue-50 text-[#0891D5] border border-blue-200 shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            GET
          </button>
          <button
            onClick={() => setMethod('PUT')}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all ${
              method === 'PUT'
                ? 'bg-emerald-50 text-[#10B981] border border-emerald-200 shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            PUT
          </button>
          <button
            onClick={() => setMethod('DELETE')}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all cursor-pointer ${
              method === 'DELETE'
                ? 'bg-rose-50 text-rose-600 border border-rose-200 shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            DELETE
          </button>
          <button
            onClick={() => {
              setMethod('SCAN');
              if (!key) setKey('user:');
            }}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all cursor-pointer ${
              method === 'SCAN'
                ? 'bg-purple-50 text-[#7965E8] border border-purple-200 shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            SCAN
          </button>
          <button
            onClick={() => {
              setMethod('BATCH');
              setKey('batch:ops');
              setValue('[\n  { "type": "SET", "key": "user:2001", "value": "{\\"tier\\":\\"pro\\"}" },\n  { "type": "SET", "key": "user:2002", "value": "{\\"tier\\":\\"free\\"}" }\n]');
            }}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all cursor-pointer ${
              method === 'BATCH'
                ? 'bg-amber-50 text-[#D97706] border border-amber-200 shadow-2xs'
                : 'text-[#64748B] hover:text-[#17243A]'
            }`}
          >
            BATCH
          </button>
        </div>

        {/* Target Node & Consistency */}
        <div className="flex flex-wrap items-center gap-2 font-mono text-xs">
          {/* Node Selector */}
          <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A]">
            <Server className="w-3.5 h-3.5 text-[#0891D5]" />
            <span className="text-[#64748B]">Target:</span>
            <select
              value={targetNode}
              onChange={(e) => setTargetNode(e.target.value)}
              className="bg-transparent font-semibold outline-none cursor-pointer"
            >
              <option value="auto">Auto (Leader Route)</option>
              <option value="node-a">Node A (:2525)</option>
              <option value="node-b">Node B (:2526)</option>
              <option value="node-c">Node C (:2527)</option>
              <option value="node-d">Node D (:2528)</option>
              <option value="node-e">Node E (:2529)</option>
            </select>
          </div>

          {/* Consistency Toggle */}
          <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A]">
            <ShieldCheck className="w-3.5 h-3.5 text-[#10B981]" />
            <select
              value={consistency}
              onChange={(e) => setConsistency(e.target.value as ConsistencyLevel)}
              className="bg-transparent font-semibold outline-none cursor-pointer"
            >
              <option value="linearizable">Linearizable (ReadIndex)</option>
              <option value="eventual">Eventual (Local Lease)</option>
            </select>
          </div>
        </div>
      </div>

      {/* 02. KEY INPUT */}
      <div className="space-y-1.5 font-mono text-xs">
        <label className="text-[11px] font-semibold text-[#64748B] uppercase tracking-wider block">
          DATABASE KEY
        </label>
        <div className="flex items-center gap-2">
          <input
            type="text"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="e.g. user:1001, session:token, metric:qps"
            className="flex-1 px-3.5 py-2 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] focus:border-[#0891D5] focus:bg-white outline-none transition-all text-[#17243A] font-semibold text-sm placeholder-[#94A3B8]"
          />
          <button
            onClick={handleClear}
            className="p-2 rounded-xl bg-[#F4F7FC] hover:bg-[#EAF1FA] border border-[#DCE5F0] text-[#64748B] hover:text-[#17243A] transition-colors"
            title="Clear Inputs"
          >
            <Trash2 className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* 03. JSON VALUE EDITOR (FOR PUT & BATCH) */}
      {(method === 'PUT' || method === 'BATCH') && (
        <div className="space-y-1.5 font-mono text-xs">
          <div className="flex items-center justify-between">
            <label className="text-[11px] font-semibold text-[#64748B] uppercase tracking-wider">
              {method === 'BATCH' ? 'BATCH OPERATIONS ARRAY' : 'JSON PAYLOAD VALUE'}
            </label>
            {jsonError ? (
              <span className="flex items-center gap-1 text-[11px] text-rose-600 font-semibold">
                <AlertCircle className="w-3.5 h-3.5" />
                {jsonError}
              </span>
            ) : (
              <span className="flex items-center gap-1 text-[11px] text-[#10B981] font-semibold">
                <CheckCircle2 className="w-3.5 h-3.5" />
                Valid JSON Syntax
              </span>
            )}
          </div>

          <textarea
            rows={5}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder='{\n  "property": "value"\n}'
            className="w-full p-3 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] focus:bg-white outline-none font-mono text-xs text-[#17243A] transition-all leading-relaxed"
          />
        </div>
      )}

      {/* 04. EXECUTE BUTTONS & SHORTCUT HINT */}
      <div className="flex items-center justify-between pt-2">
        <span className="text-[11px] font-mono text-[#64748B] hidden sm:inline">
          Pro-tip: Press <kbd className="px-1.5 py-0.5 rounded bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A] font-bold">Ctrl</kbd> + <kbd className="px-1.5 py-0.5 rounded bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A] font-bold">Enter</kbd> to execute
        </span>

        <button
          onClick={handleSubmit}
          disabled={isExecuting || (method === 'PUT' && !!jsonError)}
          className="w-full sm:w-auto px-6 py-2.5 rounded-xl bg-[#0891D5] hover:bg-[#0284C7] disabled:opacity-50 text-white font-mono text-xs font-bold flex items-center justify-center gap-2 transition-all shadow-xs"
        >
          <Play className="w-4 h-4 fill-current" />
          <span>{isExecuting ? 'Executing Consensus...' : `Execute ${method} Request`}</span>
        </button>
      </div>
    </div>
  );
}
