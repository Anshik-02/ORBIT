'use client';

import React, { useState, useEffect } from 'react';
import DashboardLayout from '@/components/dashboard/DashboardLayout';
import { Terminal, Database, CheckCircle2, RefreshCw } from 'lucide-react';
import QueryEditor, { QueryRequest } from '@/components/query/QueryEditor';
import ResponseInspector, { QueryResponse } from '@/components/query/ResponseInspector';
import QueryHistory, { HistoryItem } from '@/components/query/QueryHistory';
import DataExplorer, { StoredKV } from '@/components/query/DataExplorer';
import QueryAnalytics from '@/components/query/QueryAnalytics';
import LiveIndicator from '@/components/shared/LiveIndicator';
import { QueryService } from '@/lib/services/query.service';
import { ClusterService } from '@/lib/services/cluster.service';
import {
  executeGetKV,
  executePutKV,
  executeDeleteKV,
  executeScanKV,
  getActiveNodeEndpoint,
} from '@/lib/api/client';

export default function QueryConsolePage() {
  const [storedKeys, setStoredKeys] = useState<StoredKV[]>([]);
  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [isExecuting, setIsExecuting] = useState(false);
  const [isLive, setIsLive] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<Date>(new Date());
  const [currentResponse, setCurrentResponse] = useState<QueryResponse | null>(null);
  const [loadedQuery, setLoadedQuery] = useState<QueryRequest | undefined>(undefined);
  const [shardLeaders, setShardLeaders] = useState<Record<number, string>>({
    0: 'node-a',
    1: 'node-d',
    2: 'node-b',
  });

  // Compute shard from key using 32-bit hash matching ORBIT router
  const getShardForKey = (key: string): number => {
    let hash = 0;
    for (let i = 0; i < key.length; i++) {
      hash = (hash << 5) - hash + key.charCodeAt(i);
      hash |= 0;
    }
    return Math.abs(hash) % 3;
  };

  const getLeaderForShard = (shardId: number): string => {
    return shardLeaders[shardId] || (shardId === 0 ? 'node-a' : shardId === 1 ? 'node-d' : 'node-b');
  };

  // Load persistent query history from localStorage
  useEffect(() => {
    try {
      const saved = localStorage.getItem('orbit_query_history');
      if (saved) {
        const parsed = JSON.parse(saved);
        if (Array.isArray(parsed)) {
          setHistory(parsed);
        }
      }
    } catch {}
  }, []);

  // Poll scan of live cluster keys and shard leaders on mount
  useEffect(() => {
    async function loadLiveClusterData() {
      try {
        const endpoint = await getActiveNodeEndpoint();
        if (endpoint) {
          setIsLive(true);

          // 1. Fetch live shard leaders from health telemetry
          const health = await ClusterService.getHealth({ timeoutMs: 3000 });
          if (health?.data?.shards) {
            const leaders: Record<number, string> = {};
            for (const [sId, sData] of Object.entries(health.data.shards)) {
              if (sData.current_leader) {
                leaders[Number(sId)] = sData.current_leader;
              }
            }
            if (Object.keys(leaders).length > 0) {
              setShardLeaders((prev) => ({ ...prev, ...leaders }));
            }
          }

          // 2. Fetch live keys via cluster scan
          const scanRes = await executeScanKV({ limit: 50 });
          if (scanRes && scanRes.items && scanRes.items.length > 0) {
            const liveItems: StoredKV[] = scanRes.items.map((item) => {
              let parsedVal: any = item.value;
              try {
                parsedVal = JSON.parse(item.value);
              } catch {
                // Raw string
              }
              const sId = getShardForKey(item.key);
              return {
                key: item.key,
                value: parsedVal,
                shardId: sId,
                sizeBytes: item.value.length,
                updatedAt: 'Live',
              };
            });
            setStoredKeys(liveItems);
          }
        } else {
          setIsLive(false);
        }
      } catch {
        setIsLive(false);
      } finally {
        setLastUpdated(new Date());
      }
    }
    loadLiveClusterData();
  }, []);

  // Execution handler
  const handleExecute = async (req: QueryRequest) => {
    setIsExecuting(true);
    const shardId = getShardForKey(req.key);
    const leaderNode = getLeaderForShard(shardId);
    const effectiveNode = req.targetNode === 'auto' ? leaderNode : req.targetNode;

    let statusCode = 200;
    let statusText = 'OK';
    let durationMs = 1.8;
    let bodyData: any = {};

    try {
      if (req.method === 'GET') {
        const res = await executeGetKV(req.key);
        durationMs = res.latencyMs;
        statusCode = res.status;
        statusText = res.ok ? 'OK' : 'Error';
        bodyData = res.ok ? res.data : { error: res.error, key: req.key };
      } else if (req.method === 'PUT') {
        const res = await executePutKV(req.key, req.value || '');
        durationMs = res.latencyMs;
        statusCode = res.status;
        statusText = res.ok ? 'Created / Replicated' : 'Error';
        bodyData = res.ok ? res.data : { error: res.error, key: req.key };

        // ONLY update explorer when mutation is confirmed successful by the cluster backend
        if (res.ok) {
          let parsedVal: any = req.value;
          try {
            parsedVal = JSON.parse(req.value || '{}');
          } catch {}

          setStoredKeys((prev) => {
            const filtered = prev.filter((k) => k.key !== req.key);
            return [
              {
                key: req.key,
                value: parsedVal,
                shardId: res.data?.shard_id ?? shardId,
                sizeBytes: (req.value || '').length,
                updatedAt: new Date().toLocaleTimeString(),
              },
              ...filtered,
            ];
          });
        }
      } else if (req.method === 'DELETE') {
        const res = await executeDeleteKV(req.key);
        durationMs = res.latencyMs;
        statusCode = res.status;
        statusText = res.ok ? 'Tombstone Committed' : 'Error';
        bodyData = res.ok ? res.data : { error: res.error, key: req.key };

        // ONLY remove from explorer if backend confirmed deletion
        if (res.ok) {
          setStoredKeys((prev) => prev.filter((k) => k.key !== req.key));
        }
      } else if (req.method === 'SCAN') {
        const start = performance.now();
        // Support prefix scan if query ends with : or *
        const isPrefix = req.key && (req.key.endsWith(':') || req.key.endsWith('*'));
        const cleanPrefix = req.key ? req.key.replace(/\*$/, '') : '';

        const res = isPrefix
          ? await QueryService.prefixScan(cleanPrefix, 50)
          : await QueryService.scan({ start: req.key || undefined, limit: 50 });

        durationMs = Math.round((performance.now() - start) * 10) / 10;
        statusCode = res ? 200 : 500;
        statusText = res ? `Scan Completed (${res.items?.length || 0} items)` : 'Scan Failed';
        bodyData = res || { error: 'Failed to scan keys' };

        // Sync scanned items into storedKeys
        if (res && res.items && res.items.length > 0) {
          const scannedList: StoredKV[] = res.items.map((item) => {
            let pVal: any = item.value;
            try {
              pVal = JSON.parse(item.value);
            } catch {}
            return {
              key: item.key,
              value: pVal,
              shardId: getShardForKey(item.key),
              sizeBytes: item.value.length,
              updatedAt: new Date().toLocaleTimeString(),
            };
          });

          setStoredKeys((prev) => {
            const keyMap = new Map(prev.map((k) => [k.key, k]));
            for (const item of scannedList) {
              keyMap.set(item.key, item);
            }
            return Array.from(keyMap.values());
          });
        }
      } else if (req.method === 'BATCH') {
        const start = performance.now();
        let ops = [];
        try {
          ops = JSON.parse(req.value || '[]');
        } catch {
          ops = [];
        }
        const res = await QueryService.batchWrite(ops);
        durationMs = Math.round((performance.now() - start) * 10) / 10;
        const committed = res.results?.filter((r) => r.status === 'COMMITTED').length || 0;
        const failed = res.results?.filter((r) => r.status === 'ERROR').length || 0;
        statusCode = failed === 0 ? 200 : 207;
        statusText = `Batch Executed (${committed} committed, ${failed} failed)`;
        bodyData = res;
      }
    } catch (err: any) {
      statusCode = 500;
      statusText = 'Internal Failure';
      bodyData = { error: err.message || 'Execution failed' };
    } finally {
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

      // Append to persistent query history
      const newHistItem: HistoryItem = {
        id: `h-${Date.now()}`,
        timestamp: new Date().toLocaleTimeString(),
        method: req.method,
        key: req.key,
        statusCode,
        durationMs,
        targetNode: effectiveNode,
      };

      setHistory((prev) => {
        const next = [newHistItem, ...prev.slice(0, 49)];
        try {
          localStorage.setItem('orbit_query_history', JSON.stringify(next));
        } catch {}
        return next;
      });
    }
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

  // Query analytics computed strictly from actual history
  const totalQueries = history.length;
  const successCount = history.filter((h) => h.statusCode >= 200 && h.statusCode < 300).length;
  const failCount = history.filter((h) => h.statusCode >= 400).length;
  const avgLatencyMs =
    history.length > 0
      ? Math.round((history.reduce((acc, h) => acc + h.durationMs, 0) / history.length) * 10) / 10
      : 0;
  const methodCounts = {
    GET: history.filter((h) => h.method === 'GET').length,
    PUT: history.filter((h) => h.method === 'PUT').length,
    DELETE: history.filter((h) => h.method === 'DELETE').length,
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
              <LiveIndicator isLive={isLive} lastUpdated={lastUpdated} />
            </div>
            <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal font-mono">
              Execute low-latency GET, PUT, DELETE, SCAN and BATCH operations directly across cluster partitions.
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
          totalQueries={totalQueries}
          successCount={successCount}
          failCount={failCount}
          avgLatencyMs={avgLatencyMs}
          methodCounts={methodCounts}
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
          onClear={() => {
            setHistory([]);
            try {
              localStorage.removeItem('orbit_query_history');
            } catch {}
          }}
        />
      </div>
    </DashboardLayout>
  );
}
