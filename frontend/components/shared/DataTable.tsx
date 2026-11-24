'use client';

import React, { useState, useMemo } from 'react';
import { ChevronUp, ChevronDown, ChevronLeft, ChevronRight } from 'lucide-react';

export interface Column<T> {
  key: keyof T | string;
  header: string;
  render?: (row: T, index: number) => React.ReactNode;
  sortable?: boolean;
  align?: 'left' | 'center' | 'right';
  width?: string;
}

interface DataTableProps<T> {
  data: T[];
  columns: Column<T>[];
  pageSize?: number;
  emptyMessage?: string;
  onRowClick?: (row: T) => void;
  className?: string;
  keyExtractor?: (row: T, index: number) => string;
}

export default function DataTable<T extends Record<string, any>>({
  data,
  columns,
  pageSize = 10,
  emptyMessage = 'No records found',
  onRowClick,
  className = '',
  keyExtractor,
}: DataTableProps<T>) {
  const [sortKey, setSortKey] = useState<string | null>(null);
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('asc');
  const [currentPage, setCurrentPage] = useState(1);

  const handleSort = (key: string) => {
    if (sortKey === key) {
      if (sortOrder === 'asc') {
        setSortOrder('desc');
      } else {
        setSortKey(null);
        setSortOrder('asc');
      }
    } else {
      setSortKey(key);
      setSortOrder('asc');
    }
  };

  const sortedData = useMemo(() => {
    if (!sortKey) return data;
    return [...data].sort((a, b) => {
      const valA = a[sortKey];
      const valB = b[sortKey];
      if (valA === valB) return 0;
      if (valA === null || valA === undefined) return 1;
      if (valB === null || valB === undefined) return -1;
      if (typeof valA === 'number' && typeof valB === 'number') {
        return sortOrder === 'asc' ? valA - valB : valB - valA;
      }
      return sortOrder === 'asc'
        ? String(valA).localeCompare(String(valB))
        : String(valB).localeCompare(String(valA));
    });
  }, [data, sortKey, sortOrder]);

  const totalPages = Math.ceil(sortedData.length / pageSize) || 1;
  const paginatedData = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return sortedData.slice(start, start + pageSize);
  }, [sortedData, currentPage, pageSize]);

  return (
    <div className={`w-full overflow-hidden rounded-2xl border border-[#DCE5F0] bg-white shadow-xs ${className}`}>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-xs font-mono">
          <thead className="bg-[#F8FAFC] border-b border-[#DCE5F0] text-[11px] uppercase tracking-wider text-[#64748B]">
            <tr>
              {columns.map((col) => {
                const isSorted = sortKey === String(col.key);
                return (
                  <th
                    key={String(col.key)}
                    style={{ width: col.width }}
                    onClick={() => col.sortable && handleSort(String(col.key))}
                    className={`py-3 px-4 font-semibold ${
                      col.sortable ? 'cursor-pointer select-none hover:text-[#17243A]' : ''
                    } ${
                      col.align === 'right'
                        ? 'text-right'
                        : col.align === 'center'
                        ? 'text-center'
                        : 'text-left'
                    }`}
                  >
                    <div
                      className={`inline-flex items-center gap-1.5 ${
                        col.align === 'right' ? 'justify-end' : col.align === 'center' ? 'justify-center' : 'justify-start'
                      }`}
                    >
                      <span>{col.header}</span>
                      {col.sortable && (
                        <span className="text-[#94A3B8]">
                          {isSorted ? (
                            sortOrder === 'asc' ? (
                              <ChevronUp className="w-3.5 h-3.5 text-[#0891D5]" />
                            ) : (
                              <ChevronDown className="w-3.5 h-3.5 text-[#0891D5]" />
                            )
                          ) : (
                            <ChevronUp className="w-3.5 h-3.5 opacity-40 hover:opacity-100" />
                          )}
                        </span>
                      )}
                    </div>
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody className="divide-y divide-[#F1F5F9] text-[#17243A]">
            {paginatedData.length === 0 ? (
              <tr>
                <td colSpan={columns.length} className="py-8 text-center text-[#64748B]">
                  {emptyMessage}
                </td>
              </tr>
            ) : (
              paginatedData.map((row, idx) => {
                const key = keyExtractor ? keyExtractor(row, idx) : (row.id ? String(row.id) : String(idx));
                return (
                  <tr
                    key={key}
                    onClick={() => onRowClick?.(row)}
                    className={`transition-colors ${
                      onRowClick ? 'cursor-pointer hover:bg-sky-50/50' : 'hover:bg-slate-50/60'
                    }`}
                  >
                    {columns.map((col) => (
                      <td
                        key={String(col.key)}
                        className={`py-3 px-4 ${
                          col.align === 'right'
                            ? 'text-right'
                            : col.align === 'center'
                            ? 'text-center'
                            : 'text-left'
                        }`}
                      >
                        {col.render ? col.render(row, idx) : String(row[col.key as keyof T] ?? '—')}
                      </td>
                    ))}
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between border-t border-[#DCE5F0] bg-[#F8FAFC] px-4 py-2.5 text-xs text-[#64748B]">
          <div>
            Showing {(currentPage - 1) * pageSize + 1} to{' '}
            {Math.min(currentPage * pageSize, sortedData.length)} of {sortedData.length} records
          </div>
          <div className="flex items-center gap-1.5 font-mono">
            <button
              onClick={() => setCurrentPage((p) => Math.max(p - 1, 1))}
              disabled={currentPage === 1}
              className="p-1 rounded-lg border border-[#DCE5F0] bg-white disabled:opacity-40 hover:bg-slate-50 transition-colors"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <span className="px-2 font-semibold text-[#17243A]">
              Page {currentPage} of {totalPages}
            </span>
            <button
              onClick={() => setCurrentPage((p) => Math.min(p + 1, totalPages))}
              disabled={currentPage === totalPages}
              className="p-1 rounded-lg border border-[#DCE5F0] bg-white disabled:opacity-40 hover:bg-slate-50 transition-colors"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
