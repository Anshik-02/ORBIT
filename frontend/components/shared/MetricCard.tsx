'use client';

import React from 'react';
import { LucideIcon } from 'lucide-react';

type BadgeVariant = 'success' | 'warning' | 'error' | 'neutral' | 'accent';

interface MetricCardProps {
  label: string;
  value: string | number;
  unit?: string;
  subtext?: string;
  icon?: LucideIcon;
  iconColor?: string;
  iconBg?: string;
  badge?: {
    text: string;
    variant: BadgeVariant;
  };
  onClick?: () => void;
  className?: string;
}

export default function MetricCard({
  label,
  value,
  unit,
  subtext,
  icon: Icon,
  iconColor = 'text-[#0891D5]',
  iconBg = 'bg-[#0891D5]/10',
  badge,
  onClick,
  className = '',
}: MetricCardProps) {
  const getBadgeStyle = (variant?: BadgeVariant) => {
    switch (variant) {
      case 'success':
        return 'bg-emerald-50 border-emerald-200 text-[#10B981]';
      case 'warning':
        return 'bg-amber-50 border-amber-200 text-[#F59E0B]';
      case 'error':
        return 'bg-rose-50 border-rose-200 text-[#EF4444]';
      case 'accent':
        return 'bg-[#7965E8]/10 border-[#7965E8]/30 text-[#7965E8]';
      case 'neutral':
      default:
        return 'bg-[#F4F7FC] border-[#DCE5F0] text-[#64748B]';
    }
  };

  return (
    <div
      onClick={onClick}
      className={`bg-white border border-[#DCE5F0] rounded-2xl p-5 shadow-xs hover:shadow-md transition-all ${
        onClick ? 'cursor-pointer hover:border-[#0891D5]' : ''
      } ${className}`}
    >
      <div className="flex items-center justify-between mb-3">
        <span className="text-[11px] font-mono uppercase tracking-wider text-[#64748B] font-semibold">
          {label}
        </span>
        {Icon && (
          <div className={`w-8 h-8 rounded-lg ${iconBg} flex items-center justify-center`}>
            <Icon className={`w-4 h-4 ${iconColor}`} />
          </div>
        )}
      </div>

      <div className="flex items-baseline gap-2">
        <span className="text-3xl font-extrabold text-[#17243A] tracking-tight font-mono">
          {typeof value === 'number' && value < 10 && value >= 0
            ? String(value).padStart(2, '0')
            : value}
        </span>
        {unit && <span className="text-xs text-[#64748B] font-mono">{unit}</span>}
        {badge && (
          <span
            className={`ml-auto text-[10px] font-mono font-semibold px-2 py-0.5 rounded-full border ${getBadgeStyle(
              badge.variant
            )}`}
          >
            {badge.text}
          </span>
        )}
      </div>

      {subtext && (
        <p className="text-[11px] text-[#64748B] mt-2 border-t border-[#F1F5F9] pt-2">
          {subtext}
        </p>
      )}
    </div>
  );
}
