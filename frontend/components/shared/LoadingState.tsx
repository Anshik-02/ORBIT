'use client';

import React from 'react';
import { Loader2 } from 'lucide-react';

interface LoadingStateProps {
  message?: string;
  className?: string;
}

export default function LoadingState({
  message = 'Fetching live cluster telemetry...',
  className = '',
}: LoadingStateProps) {
  return (
    <div
      className={`bg-white border border-[#DCE5F0] rounded-2xl p-12 text-center flex flex-col items-center justify-center shadow-xs ${className}`}
    >
      <Loader2 className="w-8 h-8 text-[#0891D5] animate-spin mb-3" />
      <p className="text-xs font-mono text-[#64748B]">{message}</p>
    </div>
  );
}
