'use client';

import React, { useState } from 'react';
import {
  Database,
  Search,
  Eye,
  Trash2,
  Plus,
  X,
  CheckCircle2,
  Layers,
  HardDrive,
} from 'lucide-react';

export interface StoredKV {
  key: string;
  value: any;
  shardId: number;
  sizeBytes: number;
  updatedAt: string;
}

interface DataExplorerProps {
  items: StoredKV[];
  onSelectKey: (item: StoredKV) => void;
  onDeleteKey: (key: string) => void;
  onAddKey: (key: string, value: string) => void;
}

export default function DataExplorer({
  items,
  onSelectKey,
  onDeleteKey,
  onAddKey,
}: DataExplorerProps) {
  const [search, setSearch] = useState('');
  const [inspectItem, setInspectItem] = useState<StoredKV | null>(null);
  const [deleteConfirmKey, setDeleteConfirmKey] = useState<string | null>(null);
  const [showAddModal, setShowAddModal] = useState(false);
  const [newKey, setNewKey] = useState('');
  const [newVal, setNewVal] = useState('{\n  "status": "active"\n}');

  const filteredItems = items.filter(
    (item) =>
      item.key.toLowerCase().includes(search.toLowerCase()) ||
      JSON.stringify(item.value).toLowerCase().includes(search.toLowerCase())
  );

  const handleAddSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newKey.trim()) return;
    onAddKey(newKey.trim(), newVal);
    setShowAddModal(false);
    setNewKey('');
  };

  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-6 shadow-xs space-y-4 font-mono text-xs">
      {/* 01. HEADER */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 border-b border-[#DCE5F0]">
        <div>
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-[#0891D5]/10 flex items-center justify-center">
              <Database className="w-3.5 h-3.5 text-[#0891D5]" />
            </div>
            <h3 className="text-base font-bold text-[#17243A]">
              Live Key-Value Data Explorer
            </h3>
          </div>
          <p className="text-[#64748B] text-xs font-normal mt-0.5">
            Browse stored entries partitioned across the 3 Raft groups
          </p>
        </div>

        <button
          onClick={() => setShowAddModal(true)}
          className="px-3.5 py-1.5 rounded-xl bg-[#0891D5] hover:bg-[#0284C7] text-white font-bold flex items-center gap-1.5 self-start sm:self-auto transition-all shadow-xs"
        >
          <Plus className="w-3.5 h-3.5" />
          <span>New Key-Value</span>
        </button>
      </div>

      {/* 02. SEARCH BAR */}
      <div className="relative">
        <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-[#64748B]" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Filter stored database keys..."
          className="w-full pl-9 pr-3 py-1.5 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] focus:border-[#0891D5] focus:bg-white outline-none transition-all text-[#17243A]"
        />
      </div>

      {/* 03. TABLE */}
      <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="border-b border-[#E2E8F0] text-[11px] text-[#64748B] uppercase bg-[#F8FAFC]">
              <th className="py-2 px-3">Database Key</th>
              <th className="py-2 px-3">Partition</th>
              <th className="py-2 px-3">Payload Preview</th>
              <th className="py-2 px-3">Size</th>
              <th className="py-2 px-3">Last Updated</th>
              <th className="py-2 px-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9]">
            {filteredItems.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-8 text-center text-[#64748B]">
                  No stored keys found. Add keys or execute a PUT request above.
                </td>
              </tr>
            ) : (
              filteredItems.map((item) => (
                <tr key={item.key} className="hover:bg-[#F8FAFC]">
                  <td className="py-2.5 px-3 font-bold text-[#17243A]">
                    {item.key}
                  </td>
                  <td className="py-2.5 px-3">
                    <span
                      className={`px-2 py-0.5 rounded-full border text-[10px] font-bold ${
                        item.shardId === 0
                          ? 'bg-blue-50 text-[#0891D5] border-blue-200'
                          : item.shardId === 1
                          ? 'bg-purple-50 text-[#7965E8] border-purple-200'
                          : 'bg-emerald-50 text-[#10B981] border-emerald-200'
                      }`}
                    >
                      Shard {item.shardId}
                    </span>
                  </td>
                  <td className="py-2.5 px-3 text-[#64748B] max-w-xs truncate">
                    {JSON.stringify(item.value)}
                  </td>
                  <td className="py-2.5 px-3 text-[#17243A]">
                    {item.sizeBytes} B
                  </td>
                  <td className="py-2.5 px-3 text-[#64748B]">
                    {item.updatedAt}
                  </td>
                  <td className="py-2.5 px-3 text-right">
                    <div className="flex items-center justify-end gap-1.5">
                      <button
                        onClick={() => setInspectItem(item)}
                        className="p-1 rounded-lg bg-white hover:bg-slate-100 border border-[#DCE5F0] text-[#0891D5]"
                        title="Inspect JSON"
                      >
                        <Eye className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => setDeleteConfirmKey(item.key)}
                        className="p-1 rounded-lg bg-white hover:bg-rose-50 border border-[#DCE5F0] hover:border-rose-200 text-rose-600"
                        title="Delete Key"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* INSPECT VALUE MODAL */}
      {inspectItem && (
        <div className="fixed inset-0 z-50 bg-black/40 backdrop-blur-xs flex items-center justify-center p-4">
          <div className="bg-white border border-[#DCE5F0] rounded-2xl w-full max-w-lg p-6 shadow-2xl space-y-4 font-mono animate-fadeIn">
            <div className="flex items-center justify-between pb-3 border-b border-[#DCE5F0]">
              <div className="flex items-center gap-2">
                <Database className="w-4 h-4 text-[#0891D5]" />
                <h4 className="text-sm font-bold text-[#17243A]">
                  Key Inspector: {inspectItem.key}
                </h4>
              </div>
              <button
                onClick={() => setInspectItem(null)}
                className="p-1 rounded-lg text-[#64748B] hover:text-[#17243A]"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <pre className="p-4 rounded-xl bg-[#0F172A] text-slate-100 text-xs overflow-x-auto max-h-[300px]">
              <code>{JSON.stringify(inspectItem.value, null, 2)}</code>
            </pre>

            <div className="flex items-center justify-between text-xs text-[#64748B] pt-2">
              <span>Shard: {inspectItem.shardId} • Size: {inspectItem.sizeBytes} B</span>
              <button
                onClick={() => {
                  onSelectKey(inspectItem);
                  setInspectItem(null);
                }}
                className="px-3 py-1 rounded-lg bg-[#0891D5] text-white font-bold"
              >
                Load in Editor
              </button>
            </div>
          </div>
        </div>
      )}

      {/* DELETE CONFIRMATION MODAL */}
      {deleteConfirmKey && (
        <div className="fixed inset-0 z-50 bg-black/40 backdrop-blur-xs flex items-center justify-center p-4">
          <div className="bg-white border border-[#DCE5F0] rounded-2xl w-full max-w-md p-6 shadow-2xl space-y-4 font-mono animate-fadeIn">
            <div className="flex items-center gap-2 text-rose-600">
              <Trash2 className="w-5 h-5" />
              <h4 className="text-sm font-bold">Confirm Key Deletion</h4>
            </div>

            <p className="text-xs text-[#64748B] leading-relaxed">
              Are you sure you want to propose a Raft consensus delete for{' '}
              <strong className="text-[#17243A]">{deleteConfirmKey}</strong>? This action replicates a tombstone across all shard replicas.
            </p>

            <div className="flex items-center justify-end gap-2 pt-2">
              <button
                onClick={() => setDeleteConfirmKey(null)}
                className="px-3 py-1.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] text-[#64748B] font-semibold"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  onDeleteKey(deleteConfirmKey);
                  setDeleteConfirmKey(null);
                }}
                className="px-4 py-1.5 rounded-xl bg-rose-600 hover:bg-rose-700 text-white font-bold"
              >
                Confirm Delete
              </button>
            </div>
          </div>
        </div>
      )}

      {/* CREATE NEW KEY MODAL */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 bg-black/40 backdrop-blur-xs flex items-center justify-center p-4">
          <form
            onSubmit={handleAddSubmit}
            className="bg-white border border-[#DCE5F0] rounded-2xl w-full max-w-lg p-6 shadow-2xl space-y-4 font-mono animate-fadeIn"
          >
            <div className="flex items-center justify-between pb-3 border-b border-[#DCE5F0]">
              <div className="flex items-center gap-2">
                <Plus className="w-4 h-4 text-[#0891D5]" />
                <h4 className="text-sm font-bold text-[#17243A]">Create New Key-Value</h4>
              </div>
              <button
                type="button"
                onClick={() => setShowAddModal(false)}
                className="p-1 rounded-lg text-[#64748B] hover:text-[#17243A]"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="space-y-1.5 text-xs">
              <label className="text-[11px] font-semibold text-[#64748B] uppercase">
                KEY IDENTIFIER
              </label>
              <input
                type="text"
                required
                value={newKey}
                onChange={(e) => setNewKey(e.target.value)}
                placeholder="e.g. config:app_name"
                className="w-full px-3 py-2 rounded-xl bg-[#F4F7FC] border border-[#DCE5F0] focus:border-[#0891D5] focus:bg-white outline-none"
              />
            </div>

            <div className="space-y-1.5 text-xs">
              <label className="text-[11px] font-semibold text-[#64748B] uppercase">
                JSON VALUE
              </label>
              <textarea
                rows={4}
                required
                value={newVal}
                onChange={(e) => setNewVal(e.target.value)}
                className="w-full p-3 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] focus:border-[#0891D5] focus:bg-white outline-none text-xs"
              />
            </div>

            <div className="flex items-center justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setShowAddModal(false)}
                className="px-3 py-1.5 rounded-xl bg-[#F8FAFC] border border-[#DCE5F0] text-[#64748B] font-semibold"
              >
                Cancel
              </button>
              <button
                type="submit"
                className="px-4 py-1.5 rounded-xl bg-[#0891D5] hover:bg-[#0284C7] text-white font-bold"
              >
                Save via Raft PUT
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}
