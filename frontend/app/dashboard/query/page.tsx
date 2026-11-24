'use client';

import React, { useState } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { Terminal, Database, CheckCircle2, RefreshCw } from 'lucide-react';
import QueryEditor, { QueryRequest } from '@/components/query/QueryEditor';
import ResponseInspector, { QueryResponse } from '@/components/query/ResponseInspector';
import QueryHistory, { HistoryItem } from '@/components/query/QueryHistory';
import DataExplorer, { StoredKV } from '@/components/query/DataExplorer';
import QueryAnalytics from '@/components/query/QueryAnalytics';

// Initial stored keys data
const INITIAL_STORED_KEYS: StoredKV[] = [
  {
    key: 'user:1001',
    value: { name: 'Alice Cooper', tier: 'enterprise', active: true },
    shardId: 0,
    sizeBytes: 68,
    updatedAt: '14:30:20',
  },
  {
    key: 'user:1002',
    value: { name: 'Bob Dylan', tier: 'standard', active: true },
    shardId: 1,
    sizeBytes: 64,
    updatedAt: '14:28:14',
  },
  {
    key: 'session:auth_9918',
    value: { token: 'jwt_8f91a92', expiresAt: '2026-11-25T00:00:00Z' },
    shardId: 2,
    sizeBytes: 82,
    updatedAt: '14:26:01',
  },
  {
    key: 'cluster:metrics:qps',
    value: { currentQps: 1420, p99Ms: 2.1, healthy: true },
    shardId: 0,
    sizeBytes: 56,
    updatedAt: '14:30:22',
  },
];

const INITIAL_HISTORY: HistoryItem[] = [
  {
    id: 'h-1',
    timestamp: '14:30:22',
    method: 'GET',
    key: 'cluster:metrics:qps',
    statusCode: 200,
    durationMs: 1.6,
    targetNode: 'node-a',
  },
  {
    id: 'h-2',
    timestamp: '14:30:20',
    method: 'PUT',
    key: 'user:1001',
    statusCode: 200,
    durationMs: 2.4,
    targetNode: 'auto',
  },
  {
    id: 'h-3',
    timestamp: '14:28:14',
    method: 'PUT',
    key: 'user:1002',
    statusCode: 200,
    durationMs: 2.1,
    targetNode: 'node-d',
  },
];

export default function QueryConsolePage() {
  const [storedKeys, setStoredKeys] = useState<StoredKV[]>(INITIAL_STORED_KEYS);
  const [history, setHistory] = useState<HistoryItem[]>(INITIAL_HISTORY);
  const [isExecuting, setIsExecuting] = useState(false);
  const [currentResponse, setCurrentResponse] = useState<QueryResponse | null>({
    statusCode: 200,
    statusText: 'OK',
    durationMs: 2.4,
    body: {
      key: 'user:1001',
      value: { name: 'Alice Cooper', tier: 'enterprise', active: true },
      shardId: 0,
      leader: 'node-a',
      raftTerm: 3,
      commitIndex: 14820,
      linearizable: true,
    },
    targetNode: 'node-a',
    shardId: 0,
    leaderId: 'node-a',
    responseBytes: 154,
    timestamp: '14:30:20',
  });
  const [loadedQuery, setLoadedQuery] = useState<QueryRequest | undefined>(undefined);

  // Compute shard from key (Murmur3 approximation)
  const getShardForKey = (key: string): number => {
    let hash = 0;
    for (let i = 0; i < key.length; i++) {
      hash = (hash << 5) - hash + key.charCodeAt(i);
      hash |= 0;
    }
    return Math.abs(hash) % 3;
  };

  const getLeaderForShard = (shardId: number): string => {
    if (shardId === 0) return 'node-a';
    if (shardId === 1) return 'node-d';
    return 'node-b';
  };

  // Execution handler
  const handleExecute = async (req: QueryRequest) => {
    setIsExecuting(true);
    const start = performance.now();
    const shardId = getShardForKey(req.key);
    const leaderNode = getLeaderForShard(shardId);
    const effectiveNode = req.targetNode === 'auto' ? leaderNode : req.targetNode;

    // Simulate real or network execution
    setTimeout(() => {
      const end = performance.now();
      const durationMs = Math.max(1.1, end - start);

      let statusCode = 200;
      let statusText = 'OK';
      let bodyData: any = {};

      if (req.method === 'GET') {
        const found = storedKeys.find((k) => k.key === req.key);
        if (found) {
          statusCode = 200;
          statusText = 'OK';
          bodyData = {
            key: req.key,
            value: found.value,
            shardId,
            leader: leaderNode,
            linearizable: req.consistency === 'linearizable',
            cached: false,
          };
        } else {
          statusCode = 404;
          statusText = 'Key Not Found';
          bodyData = {
            error: 'KEY_NOT_FOUND',
            key: req.key,
            message: `Key "${req.key}" was not found in LSM storage on Shard ${shardId}.`,
          };
        }
      } else if (req.method === 'PUT') {
        let parsedVal = req.value;
        try {
          parsedVal = JSON.parse(req.value || '{}');
        } catch {}

        statusCode = 200;
        statusText = 'Created / Replicated';
        bodyData = {
          operation: 'PUT',
          key: req.key,
          value: parsedVal,
          shardId,
          leader: leaderNode,
          quorumAck: '3/3 nodes',
          commitIndex: 14821,
          term: 3,
        };

        // Update stored keys state
        setStoredKeys((prev) => {
          const filtered = prev.filter((k) => k.key !== req.key);
          return [
            {
              key: req.key,
              value: parsedVal,
              shardId,
              sizeBytes: JSON.stringify(parsedVal).length,
              updatedAt: new Date().toLocaleTimeString(),
            },
            ...filtered,
          ];
        });
      } else if (req.method === 'DELETE') {
        statusCode = 200;
        statusText = 'Tombstone Committed';
        bodyData = {
          operation: 'DELETE',
          key: req.key,
          shardId,
          leader: leaderNode,
          tombstoneIndex: 14822,
          quorumAck: '3/3 nodes',
        };

        // Remove from stored keys
        setStoredKeys((prev) => prev.filter((k) => k.key !== req.key));
      }

      const resObj: QueryResponse = {
        statusCode,
        statusText,
        durationMs,
        body: bodyData,
        targetNode: effectiveNode,
        shardId,
        leaderId: leaderNode,
        responseBytes: JSON.stringify(bodyData).length,
        timestamp: new Date().toLocaleTimeString(),
      };

      setCurrentResponse(resObj);
      setIsExecuting(false);

      // Append to history
      setHistory((prev) => [
        {
          id: `h-${Date.now()}`,
          timestamp: new Date().toLocaleTimeString(),
          method: req.method,
          key: req.key,
          statusCode,
          durationMs,
          targetNode: effectiveNode,
        },
        ...prev.slice(0, 19),
      ]);
    }, 320);
  };

  const handleRerunHistory = (item: HistoryItem) => {
    setLoadedQuery({
      method: item.method,
      key: item.key,
      targetNode: item.targetNode,
      consistency: 'linearizable',
      timeoutMs: 3000,
    });
  };

  const handleDeleteFromExplorer = (key: string) => {
    handleExecute({
      method: 'DELETE',
      key,
      targetNode: 'auto',
      consistency: 'linearizable',
      timeoutMs: 3000,
    });
  };

  const handleAddFromExplorer = (key: string, valueStr: string) => {
    handleExecute({
      method: 'PUT',
      key,
      value: valueStr,
      targetNode: 'auto',
      consistency: 'linearizable',
      timeoutMs: 3000,
    });
  };

  return (
    <DashboardLayout activeNav="query">
      <div className="space-y-6">
        {/* 01. PAGE HEADER */}
        <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl sm:text-2xl font-bold text-[#17243A] tracking-tight font-mono">
                Key-Value Query Console
              </h1>
              <span className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-xs font-semibold">
                <CheckCircle2 className="w-3.5 h-3.5" />
                LINEARIZABLE RAFT CONSENSUS
              </span>
            </div>
            <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal font-mono">
              Execute low-latency GET, PUT and DELETE operations directly across cluster partitions.
            </p>
          </div>

          <div className="flex items-center gap-2 font-mono text-xs">
            <span className="px-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] text-[#17243A] font-semibold">
              Automatic Leader Proxy & Failover
            </span>
          </div>
        </div>

        {/* 02. SECTION F: QUERY ANALYTICS */}
        <QueryAnalytics
          totalQueries={history.length + 45}
          successCount={history.filter((h) => h.statusCode === 200).length + 44}
          failCount={history.filter((h) => h.statusCode !== 200).length + 1}
          avgLatencyMs={
            history.length > 0
              ? history.reduce((acc, h) => acc + h.durationMs, 0) / history.length
              : 1.8
          }
          methodCounts={{
            GET: history.filter((h) => h.method === 'GET').length + 28,
            PUT: history.filter((h) => h.method === 'PUT').length + 14,
            DELETE: history.filter((h) => h.method === 'DELETE').length + 3,
          }}
        />

        {/* 03. SECTION A & C: QUERY EDITOR & RESPONSE INSPECTOR */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
          <div className="lg:col-span-6">
            <QueryEditor
              onExecute={handleExecute}
              isExecuting={isExecuting}
              initialQuery={loadedQuery}
            />
          </div>
          <div className="lg:col-span-6">
            <ResponseInspector
              response={currentResponse}
              isExecuting={isExecuting}
            />
          </div>
        </div>

        {/* 04. SECTION E: DATA EXPLORER */}
        <DataExplorer
          items={storedKeys}
          onSelectKey={(item) => {
            setLoadedQuery({
              method: 'GET',
              key: item.key,
              targetNode: 'auto',
              consistency: 'linearizable',
              timeoutMs: 3000,
            });
          }}
          onDeleteKey={handleDeleteFromExplorer}
          onAddKey={handleAddFromExplorer}
        />

        {/* 05. SECTION D: QUERY HISTORY */}
        <QueryHistory
          history={history}
          onRerun={handleRerunHistory}
          onClear={() => setHistory([])}
        />
      </div>
    </DashboardLayout>
  );
}
