'use client';

import React, { useState, useEffect } from 'react';
import { Search, X } from 'lucide-react';

interface SearchInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  debounceMs?: number;
  className?: string;
}

export default function SearchInput({
  value,
  onChange,
  placeholder = 'Search...',
  debounceMs = 0,
  className = '',
}: SearchInputProps) {
  const [localValue, setLocalValue] = useState(value);

  useEffect(() => {
    setLocalValue(value);
  }, [value]);

  useEffect(() => {
    if (debounceMs <= 0) {
      onChange(localValue);
      return;
    }
    const timer = setTimeout(() => {
      onChange(localValue);
    }, debounceMs);
    return () => clearTimeout(timer);
  }, [localValue, debounceMs, onChange]);

  return (
    <div className={`relative flex items-center ${className}`}>
      <Search className="absolute left-3 h-4 w-4 text-[#94A3B8] pointer-events-none" />
      <input
        type="text"
        value={localValue}
        onChange={(e) => setLocalValue(e.target.value)}
        placeholder={placeholder}
        className="w-full rounded-xl border border-[#DCE5F0] bg-white py-2 pl-9 pr-8 text-xs font-mono text-[#17243A] placeholder-[#94A3B8] shadow-xs focus:border-[#0891D5] focus:outline-none focus:ring-1 focus:ring-[#0891D5] transition-all"
      />
      {localValue && (
        <button
          onClick={() => {
            setLocalValue('');
            onChange('');
          }}
          className="absolute right-2.5 rounded p-0.5 text-[#94A3B8] hover:text-[#17243A] transition-colors"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  );
}
