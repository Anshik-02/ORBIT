'use client';

import React from 'react';
import { AlertTriangle, X } from 'lucide-react';

interface ConfirmDialogProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: 'danger' | 'primary';
  isLoading?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export default function ConfirmDialog({
  isOpen,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  variant = 'danger',
  isLoading = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  if (!isOpen) return null;

  const isDanger = variant === 'danger';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/40 backdrop-blur-sm animate-fadeIn">
      <div className="w-full max-w-md rounded-2xl border border-[#DCE5F0] bg-white p-6 shadow-2xl transition-all">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div
              className={`flex h-10 w-10 items-center justify-center rounded-xl border ${
                isDanger ? 'border-rose-200 bg-rose-50 text-rose-500' : 'border-sky-200 bg-sky-50 text-[#0891D5]'
              }`}
            >
              <AlertTriangle className="h-5 w-5" />
            </div>
            <h3 className="text-base font-semibold text-[#17243A]">{title}</h3>
          </div>
          <button
            onClick={onCancel}
            disabled={isLoading}
            className="rounded-lg p-1 text-[#64748B] hover:bg-slate-100 hover:text-[#17243A] transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <p className="mt-3 text-xs text-[#64748B] leading-relaxed">{message}</p>

        <div className="mt-6 flex items-center justify-end gap-3 font-mono text-xs">
          <button
            onClick={onCancel}
            disabled={isLoading}
            className="rounded-xl border border-[#DCE5F0] bg-white px-4 py-2 font-medium text-[#17243A] hover:bg-slate-50 transition-colors disabled:opacity-50"
          >
            {cancelLabel}
          </button>
          <button
            onClick={onConfirm}
            disabled={isLoading}
            className={`rounded-xl px-4 py-2 font-semibold text-white shadow-sm transition-all disabled:opacity-50 ${
              isDanger
                ? 'bg-rose-600 hover:bg-rose-700 shadow-rose-500/20'
                : 'bg-[#0891D5] hover:bg-sky-600 shadow-sky-500/20'
            }`}
          >
            {isLoading ? 'Processing...' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
