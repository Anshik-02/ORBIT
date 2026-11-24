'use client';

import React from 'react';

interface PageHeaderProps {
  title: string;
  subtitle?: string;
  badge?: React.ReactNode;
  children?: React.ReactNode;
}

export default function PageHeader({
  title,
  subtitle,
  badge,
  children,
}: PageHeaderProps) {
  return (
    <div className="bg-white border border-[#DCE5F0] rounded-2xl p-5 sm:p-6 shadow-xs">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-3 flex-wrap">
            <h1 className="text-xl sm:text-2xl font-bold text-[#17243A] tracking-tight">
              {title}
            </h1>
            {badge}
          </div>
          {subtitle && (
            <p className="text-xs sm:text-sm text-[#64748B] mt-1 font-normal">
              {subtitle}
            </p>
          )}
        </div>

        {children && (
          <div className="flex flex-wrap items-center gap-2.5">
            {children}
          </div>
        )}
      </div>
    </div>
  );
}
