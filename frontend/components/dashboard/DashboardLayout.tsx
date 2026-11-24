"use client";

import React, { useState, useEffect } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Database,
  LayoutDashboard,
  Share2,
  Layers,
  ShieldCheck,
  Terminal,
  Activity,
  HardDrive,
  Zap,
  Flame,
  Settings,
  Bell,
  RefreshCw,
  Menu,
  X,
  ChevronRight,
  Radio,
  CheckCircle2,
  Info,
  ExternalLink,
} from "lucide-react";

interface DashboardLayoutProps {
  children: React.ReactNode;
  activeNav?: string;
  onRefresh?: () => void;
  isRefreshing?: boolean;
}

const NAV_ITEMS = [
  { id: "overview", label: "OVERVIEW", href: "/dashboard", icon: LayoutDashboard },
  { id: "topology", label: "TOPOLOGY", href: "/dashboard#topology", icon: Share2 },
  { id: "shards", label: "SHARDS", href: "/dashboard#shards", icon: Layers },
  { id: "raft", label: "RAFT", href: "/dashboard#raft", icon: ShieldCheck },
  { id: "query", label: "QUERY", href: "/dashboard#query", icon: Terminal },
  { id: "events", label: "EVENTS", href: "/dashboard#events", icon: Activity },
  { id: "storage", label: "STORAGE", href: "/dashboard#storage", icon: HardDrive },
  { id: "benchmarks", label: "BENCHMARKS", href: "/dashboard#benchmarks", icon: Zap },
  { id: "chaos", label: "CHAOS LAB", href: "/dashboard#chaos", icon: Flame },
  { id: "settings", label: "SETTINGS", href: "/dashboard#settings", icon: Settings },
];

export default function DashboardLayout({
  children,
  activeNav = "overview",
  onRefresh,
  isRefreshing = false,
}: DashboardLayoutProps) {
  const pathname = usePathname();
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const [lastUpdated, setLastUpdated] = useState("Just now");
  const [statusModalOpen, setStatusModalOpen] = useState(false);
  const [notificationsOpen, setNotificationsOpen] = useState(false);

  useEffect(() => {
    const updateTime = () => {
      const now = new Date();
      setLastUpdated(now.toLocaleTimeString("en-US", { hour12: false }));
    };
    updateTime();
    const interval = setInterval(updateTime, 1000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="min-h-screen bg-[#030712] text-slate-100 flex flex-col font-sans select-none antialiased">
      {/* Top Universal Command Center Header */}
      <header className="border-b border-slate-800/90 bg-[#050a17]/95 backdrop-blur-md sticky top-0 z-40 px-4 sm:px-6 py-2.5">
        <div className="flex items-center justify-between gap-4">
          {/* Left Side — Logo, Page Title, Breadcrumbs */}
          <div className="flex items-center gap-4">
            <button
              onClick={() => setMobileSidebarOpen(!mobileSidebarOpen)}
              className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 lg:hidden"
              aria-label="Toggle Navigation"
            >
              {mobileSidebarOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
            </button>

            <Link href="/" className="flex items-center gap-2.5 group">
              <div className="w-8 h-8 rounded-lg bg-sky-500/15 border border-sky-500/40 flex items-center justify-center text-sky-400 group-hover:border-sky-400 group-hover:shadow-[0_0_15px_rgba(56,189,248,0.35)] transition-all">
                <Database className="w-4 h-4 group-hover:rotate-12 transition-transform duration-300" />
              </div>
              <span className="font-bold text-base tracking-wider bg-gradient-to-r from-white via-slate-100 to-sky-300 bg-clip-text text-transparent">
                ORBIT
              </span>
            </Link>

            <div className="hidden sm:flex items-center text-xs font-mono text-slate-400 border-l border-slate-800 pl-4 space-x-1.5">
              <span>Cluster</span>
              <ChevronRight className="w-3.5 h-3.5 text-slate-600" />
              <span className="text-sky-400 font-semibold">Overview</span>
            </div>
          </div>

          {/* Right Side — Connection Status, Last Updated, Actions */}
          <div className="flex items-center gap-3">
            {/* Connection Status Badge (Clickable for health modal) */}
            <div className="relative">
              <button
                onClick={() => setStatusModalOpen(!statusModalOpen)}
                className="flex items-center gap-2 px-3 py-1 rounded-full bg-emerald-950/80 border border-emerald-500/40 text-emerald-400 hover:bg-emerald-900/60 transition-all font-mono text-xs shadow-sm shadow-emerald-950/40"
              >
                <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
                <span className="font-semibold tracking-wide">CLUSTER ONLINE</span>
              </button>

              {/* Status Popover */}
              {statusModalOpen && (
                <div className="absolute right-0 mt-2 w-72 glass-panel p-4 rounded-xl border border-emerald-500/30 shadow-2xl z-50 animate-fadeIn text-xs font-mono">
                  <div className="flex items-center justify-between pb-2 mb-2 border-b border-slate-800">
                    <span className="font-bold text-white uppercase tracking-wider text-[11px]">
                      Cluster Health Status
                    </span>
                    <span className="text-[10px] text-emerald-400 font-bold">100% OPERATIONAL</span>
                  </div>
                  <div className="space-y-1.5 text-slate-300 text-[11px]">
                    <div className="flex justify-between">
                      <span className="text-slate-400">Physical Nodes:</span>
                      <span className="text-emerald-400 font-semibold">5 / 5 Connected</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-slate-400">Logical Shards:</span>
                      <span className="text-sky-400 font-semibold">3 / 3 Active</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-slate-400">Raft Quorum:</span>
                      <span className="text-purple-400 font-semibold">Stable (3 Groups)</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-slate-400">Consensus Term:</span>
                      <span className="text-slate-200">Term 4</span>
                    </div>
                  </div>
                  <div className="mt-3 pt-2 border-t border-slate-800 text-[10px] text-slate-400 text-center">
                    All nodes communicating over TCP raft transport.
                  </div>
                </div>
              )}
            </div>

            {/* Last Updated Timestamp */}
            <div className="hidden md:flex items-center gap-2 px-2.5 py-1 rounded-lg bg-slate-900/80 border border-slate-800 text-[11px] font-mono text-slate-400">
              <span className="w-1.5 h-1.5 rounded-full bg-sky-400" />
              <span>Updated:</span>
              <span className="text-slate-200 font-semibold">{lastUpdated}</span>
            </div>

            {/* Notifications Bell */}
            <div className="relative">
              <button
                onClick={() => setNotificationsOpen(!notificationsOpen)}
                className="p-1.5 rounded-lg bg-slate-900 hover:bg-slate-800 text-slate-400 hover:text-white border border-slate-800 transition-colors relative"
                aria-label="Notifications"
              >
                <Bell className="w-4 h-4" />
                <span className="absolute top-1 right-1 w-2 h-2 rounded-full bg-sky-400 animate-pulse" />
              </button>

              {notificationsOpen && (
                <div className="absolute right-0 mt-2 w-80 glass-panel p-4 rounded-xl border border-sky-500/30 shadow-2xl z-50 animate-fadeIn text-xs font-mono">
                  <div className="flex items-center justify-between pb-2 mb-2 border-b border-slate-800">
                    <span className="font-bold text-white uppercase text-[11px]">System Alerts</span>
                    <span className="text-[10px] text-sky-400">0 Critical</span>
                  </div>
                  <div className="space-y-2 text-[11px]">
                    <div className="p-2 rounded bg-slate-900/70 border border-slate-800 flex items-start gap-2">
                      <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400 mt-0.5 shrink-0" />
                      <div>
                        <div className="text-slate-200 font-semibold">Cluster Initialized</div>
                        <div className="text-[10px] text-slate-400">3 Raft shards elected independent leaders.</div>
                      </div>
                    </div>
                    <div className="p-2 rounded bg-slate-900/70 border border-slate-800 flex items-start gap-2">
                      <Radio className="w-3.5 h-3.5 text-sky-400 mt-0.5 shrink-0" />
                      <div>
                        <div className="text-slate-200 font-semibold">WebSocket Bus Active</div>
                        <div className="text-[10px] text-slate-400">Streaming real-time consensus telemetry.</div>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>

            {/* Manual Refresh Trigger */}
            <button
              onClick={onRefresh}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 hover:bg-slate-800 text-slate-300 hover:text-white border border-slate-800 hover:border-slate-700 font-mono text-xs transition-colors"
              title="Refresh Cluster State"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${isRefreshing ? "animate-spin text-sky-400" : ""}`} />
              <span className="hidden sm:inline">Refresh</span>
            </button>
          </div>
        </div>
      </header>

      {/* Main Body Shell: Sidebar + Content */}
      <div className="flex flex-1 relative overflow-hidden">
        {/* Left Sticky Infrastructure Sidebar */}
        <aside
          className={`w-56 shrink-0 bg-[#040816]/95 border-r border-slate-800/90 flex flex-col justify-between py-5 px-3 z-30 transition-all duration-300 fixed lg:static inset-y-0 left-0 ${
            mobileSidebarOpen ? "translate-x-0 shadow-2xl" : "-translate-x-full lg:translate-x-0"
          }`}
        >
          <div className="space-y-1">
            <div className="px-3 pb-3 mb-2 border-b border-slate-800/60">
              <span className="text-[10px] font-mono uppercase tracking-widest text-slate-500 font-bold block">
                Command Navigation
              </span>
            </div>

            {NAV_ITEMS.map((item) => {
              const Icon = item.icon;
              const isActive = activeNav === item.id;

              return (
                <Link
                  key={item.id}
                  href={item.href}
                  onClick={() => setMobileSidebarOpen(false)}
                  className={`flex items-center gap-3 px-3 py-2.5 rounded-xl font-mono text-xs font-semibold tracking-wider transition-all ${
                    isActive
                      ? "bg-sky-500/20 text-sky-300 border border-sky-500/40 shadow-sm shadow-sky-950/40"
                      : "text-slate-400 hover:text-slate-200 hover:bg-slate-900/60 border border-transparent"
                  }`}
                >
                  <Icon
                    className={`w-4 h-4 ${
                      isActive ? "text-sky-400" : "text-slate-500 group-hover:text-slate-400"
                    }`}
                  />
                  <span>{item.label}</span>
                  {isActive && <div className="ml-auto w-1.5 h-1.5 rounded-full bg-sky-400" />}
                </Link>
              );
            })}
          </div>

          {/* Sidebar Footer Info */}
          <div className="pt-4 border-t border-slate-800/80 px-2 space-y-2 font-mono text-[11px] text-slate-400">
            <div className="flex items-center justify-between">
              <span className="text-slate-500">Hash Epoch:</span>
              <span className="text-sky-400 font-bold">1</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-slate-500">Virtual Nodes:</span>
              <span className="text-purple-400 font-bold">256 / shard</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-slate-500">Replica Factor:</span>
              <span className="text-emerald-400 font-bold">3</span>
            </div>
          </div>
        </aside>

        {/* Mobile backdrop overlay */}
        {mobileSidebarOpen && (
          <div
            onClick={() => setMobileSidebarOpen(false)}
            className="fixed inset-0 bg-black/60 backdrop-blur-sm z-20 lg:hidden"
          />
        )}

        {/* Main Workspace Content Area */}
        <main className="flex-1 overflow-y-auto min-w-0 bg-[#030712] p-4 sm:p-6 lg:p-8">
          <div className="max-w-[1560px] mx-auto space-y-6">{children}</div>
        </main>
      </div>
    </div>
  );
}
