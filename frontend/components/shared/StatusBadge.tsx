'use client';

import React from 'react';
import { Crown, CheckCircle2, AlertTriangle, XCircle } from 'lucide-react';

export type StatusType =
  | 'healthy'
  | 'degraded'
  | 'unavailable'
  | 'online'
  | 'offline'
  | 'leader'
  | 'follower'
  | 'stable'
  | 'recovering';

interface StatusBadgeProps {
  status: StatusType | string;
  label?: string;
  className?: string;
  showIcon?: boolean;
}

export default function StatusBadge({
  status,
  label,
  className = '',
  showIcon = true,
}: StatusBadgeProps) {
  const norm = status.toLowerCase();

  switch (norm) {
    case 'healthy':
    case 'online':
    case 'stable':
      return (
        <span
          className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-emerald-50 border border-emerald-200 text-[#10B981] font-mono text-xs font-semibold ${className}`}
        >
          {showIcon && <span className="w-1.5 h-1.5 rounded-full bg-[#10B981]" />}
          {label || status.toUpperCase()}
        </span>
      );

    case 'leader':
      return (
        <span
          className={`inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full bg-amber-50 border border-amber-200 text-[#D97706] font-mono text-xs font-bold ${className}`}
        >
          {showIcon && <Crown className="w-3 h-3 text-[#F59E0B]" />}
          {label || 'LEADER'}
        </span>
      );

    case 'follower':
      return (
        <span
          className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-slate-100 border border-slate-200 text-[#64748B] font-mono text-xs font-semibold ${className}`}
        >
          {label || 'FOLLOWER'}
        </span>
      );

    case 'degraded':
    case 'recovering':
      return (
        <span
          className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-amber-50 border border-amber-200 text-[#F59E0B] font-mono text-xs font-semibold ${className}`}
        >
          {showIcon && <AlertTriangle className="w-3 h-3" />}
          {label || status.toUpperCase()}
        </span>
      );

    case 'unavailable':
    case 'offline':
    default:
      return (
        <span
          className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-rose-50 border border-rose-200 text-[#EF4444] font-mono text-xs font-semibold ${className}`}
        >
          {showIcon && <span className="w-1.5 h-1.5 rounded-full bg-[#EF4444]" />}
          {label || status.toUpperCase()}
        </span>
      );
  }
}
