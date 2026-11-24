'use client';

import React from 'react';
import { LucideIcon, Database } from 'lucide-react';

interface EmptyStateProps {
  title: string;
  description: string;
  icon?: LucideIcon;
  action?: {
    label: string;
    onClick: () => void;
  };
}

export default function EmptyState({
  title,
  description,
  icon: Icon = Database,
  action,
}: EmptyStateProps) {
  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-8 sm:p-12 text-center flex flex-col items-center justify-center shadow-xs">
      <div className="w-12 h-12 rounded-xl bg-slate-50 border border-[#DCE5F0] flex items-center justify-center text-[#64748B] mb-3">
        <Icon className="w-6 h-6" />
      </div>
      <h3 className="text-base font-bold text-[#17243A] tracking-tight mb-1">
        {title}
      </h3>
      <p className="text-xs text-[#64748B] max-w-md mb-4 leading-relaxed">
        {description}
      </p>
      {action && (
        <button
          onClick={action.onClick}
          className="px-4 py-2 rounded-xl bg-[#0891D5] hover:bg-[#0780be] text-white font-mono text-xs font-bold transition-all shadow-xs"
        >
          {action.label}
        </button>
      )}
    </div>
  );
}
