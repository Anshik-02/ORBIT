'use client';

import React from 'react';

export type TimeRange = '1m' | '5m' | '15m' | '1h' | '24h';

interface TimeRangeSelectorProps {
  value: TimeRange;
  onChange: (range: TimeRange) => void;
  options?: TimeRange[];
  className?: string;
}

export default function TimeRangeSelector({
  value,
  onChange,
  options = ['1m', '5m', '15m', '1h', '24h'],
  className = '',
}: TimeRangeSelectorProps) {
  return (
    <div
      className={`inline-flex items-center gap-1 rounded-xl border border-[#DCE5F0] bg-[#F4F7FC] p-1 font-mono text-xs ${className}`}
    >
      {options.map((opt) => (
        <button
          key={opt}
          onClick={() => onChange(opt)}
          className={`rounded-lg px-2.5 py-1 font-medium transition-all ${
            value === opt
              ? 'bg-white font-semibold text-[#0891D5] shadow-xs border border-[#DCE5F0]'
              : 'text-[#64748B] hover:text-[#17243A]'
          }`}
        >
          {opt}
        </button>
      ))}
    </div>
  );
}
