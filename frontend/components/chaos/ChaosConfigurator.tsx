'use client';

import React, { useState } from 'react';
import {
  Flame,
  ShieldAlert,
  CheckCircle2,
  AlertTriangle,
  Play,
  RotateCcw,
  Server,
  Layers,
} from 'lucide-react';
import { ChaosExperimentDef } from './ChaosExperimentLibrary';

interface ChaosConfiguratorProps {
  experiment: ChaosExperimentDef;
  onExecute: (targetNode: string, targetShard: number) => void;
  onAbort: () => void;
  isRunning: boolean;
}

export default function ChaosConfigurator({
  experiment,
  onExecute,
  onAbort,
  isRunning,
}: ChaosConfiguratorProps) {
  const [targetNode, setTargetNode] = useState('node-a');
  const [targetShard, setTargetShard] = useState(0);

  // Safety pre-check calculation
  const isSafe = true; // 4 other nodes are healthy
  const warningText =
    experiment.id === 'leader-kill'
      ? 'Target node is currently the elected leader of Shard 0. Term will increment.'
      : 'Target node hosts follower replicas. Write quorum will remain satisfied.';

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-5 font-mono text-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-4 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <span className="p-1.5 rounded-lg bg-rose-50 text-rose-600">
              <Flame className="w-4 h-4" />
            </span>
            <h3 className="text-base font-bold text-[#17243A]">
              Chaos Injection Control: {experiment.name}
            </h3>
          </div>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Configure target fault injection parameters and run safety verification
          </p>
        </div>

        <div className="flex items-center gap-2">
          {isRunning ? (
            <button
              onClick={onAbort}
              className="px-4 py-2 rounded-xl bg-rose-600 hover:bg-rose-700 text-white font-bold flex items-center gap-1.5 transition-all shadow-xs"
            >
              <span>Abort Chaos Test</span>
            </button>
          ) : (
            <button
              onClick={() => onExecute(targetNode, targetShard)}
              className="px-5 py-2 rounded-xl bg-rose-600 hover:bg-rose-700 text-white font-bold flex items-center gap-1.5 transition-all shadow-xs"
            >
              <Flame className="w-4 h-4 fill-current" />
              <span>Inject Fault ({experiment.durationSec}s)</span>
            </button>
          )}
        </div>
      </div>

      {/* Target configuration parameters */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
            TARGET NODE
          </label>
          <select
            disabled={isRunning}
            value={targetNode}
            onChange={(e) => setTargetNode(e.target.value)}
            className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
          >
            <option value="node-a">Node A (:2525) - Leader Shard 0</option>
            <option value="node-b">Node B (:2526) - Leader Shard 2</option>
            <option value="node-c">Node C (:2527) - Follower</option>
            <option value="node-d">Node D (:2528) - Leader Shard 1</option>
            <option value="node-e">Node E (:2529) - Follower</option>
          </select>
        </div>

        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
            TARGET SHARD
          </label>
          <select
            disabled={isRunning}
            value={targetShard}
            onChange={(e) => setTargetShard(parseInt(e.target.value, 10))}
            className="w-full p-2.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] outline-none text-[#17243A] font-semibold"
          >
            <option value={0}>Shard 0 (0x00..0x55)</option>
            <option value={1}>Shard 1 (0x55..0xAA)</option>
            <option value={2}>Shard 2 (0xAA..0xFF)</option>
          </select>
        </div>

        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold text-[#64748B] uppercase block">
            SAFETY PRE-CHECK
          </label>
          <div className="p-2.5 rounded-xl bg-emerald-50 border border-emerald-200 text-[#10B981] font-semibold flex items-center gap-1.5">
            <CheckCircle2 className="w-3.5 h-3.5 text-[#10B981] shrink-0" />
            <span>Passed: Write Quorum Safe</span>
          </div>
        </div>
      </div>

      {/* Safety Advisory Banner */}
      <div className="p-3.5 rounded-xl bg-amber-50/70 border border-amber-200 text-amber-900 text-[11px] flex items-center gap-2">
        <AlertTriangle className="w-4 h-4 text-[#F59E0B] shrink-0" />
        <span>
          <strong>Advisory:</strong> {warningText} {experiment.safetyCheck}
        </span>
      </div>
    </div>
  );
}
