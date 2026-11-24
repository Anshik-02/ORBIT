'use client';

import React from 'react';
import { AlertCircle, RefreshCw } from 'lucide-react';

interface ErrorStateProps {
  title?: string;
  message: string;
  code?: string;
  requestId?: string;
  onRetry?: () => void;
  className?: string;
}

export default function ErrorState({
  title = 'An error occurred',
  message,
  code,
  requestId,
  onRetry,
  className = '',
}: ErrorStateProps) {
  return (
    <div
      className={`rounded-2xl border border-rose-200 bg-rose-50/60 p-6 text-center font-sans shadow-xs ${className}`}
    >
      <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-xl border border-rose-200 bg-white text-rose-500 shadow-sm">
        <AlertCircle className="h-6 w-6" />
      </div>
      <h3 className="text-sm font-semibold text-[#17243A]">{title}</h3>
      <p className="mt-1 text-xs text-[#64748B] max-w-md mx-auto leading-relaxed">{message}</p>

      {(code || requestId) && (
        <div className="mt-3 flex items-center justify-center gap-3 font-mono text-[10px] text-[#64748B]">
          {code && <span className="bg-white/80 px-2 py-0.5 rounded border border-rose-200">CODE: {code}</span>}
          {requestId && (
            <span className="bg-white/80 px-2 py-0.5 rounded border border-rose-200">REQ: {requestId}</span>
          )}
        </div>
      )}

      {onRetry && (
        <button
          onClick={onRetry}
          className="mt-4 inline-flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-semibold text-[#17243A] border border-[#DCE5F0] shadow-sm hover:bg-slate-50 transition-colors"
        >
          <RefreshCw className="h-3.5 w-3.5 text-[#0891D5]" />
          <span>Retry Operation</span>
        </button>
      )}
    </div>
  );
}
