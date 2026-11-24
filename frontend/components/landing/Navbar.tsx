"use client";

import React, { useState, useEffect } from "react";
import Link from "next/link";
import { Database, LayoutDashboard, Menu, X, ArrowUpRight } from "lucide-react";
import GithubIcon from "@/components/ui/GithubIcon";

export default function Navbar() {
  const [scrolled, setScrolled] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  useEffect(() => {
    const handleScroll = () => {
      setScrolled(window.scrollY > 20);
    };
    window.addEventListener("scroll", handleScroll);
    return () => window.removeEventListener("scroll", handleScroll);
  }, []);

  return (
    <nav
      className={`fixed top-0 left-0 right-0 z-50 transition-all duration-300 ${
        scrolled
          ? "glass-panel bg-[#030712]/85 shadow-lg shadow-sky-950/20 py-3 border-b border-sky-500/20"
          : "bg-transparent py-5 border-b border-transparent"
      }`}
    >
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between">
          {/* Left side — Brand */}
          <Link href="/" className="flex items-center gap-3 group">
            <div className="relative flex items-center justify-center w-10 h-10 rounded-xl bg-gradient-to-br from-sky-500/20 via-indigo-500/10 to-purple-500/20 border border-sky-500/30 group-hover:border-sky-400 group-hover:shadow-[0_0_20px_rgba(56,189,248,0.4)] transition-all">
              <Database className="w-5 h-5 text-sky-400 group-hover:rotate-12 transition-transform duration-300" />
              <div className="absolute inset-0 rounded-xl border border-sky-400/40 animate-ping opacity-25 pointer-events-none" />
            </div>
            <div className="flex flex-col">
              <div className="flex items-center gap-2">
                <span className="font-bold text-lg tracking-wider bg-gradient-to-r from-white via-slate-100 to-sky-300 bg-clip-text text-transparent">
                  ORBIT
                </span>
                <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-sky-950/80 border border-sky-500/30 text-sky-400">
                  v1.4
                </span>
              </div>
              <span className="text-[11px] text-slate-400 font-mono tracking-tight hidden sm:inline">
                Distributed Key-Value Database
              </span>
            </div>
          </Link>

          {/* Center Navigation Links */}
          <div className="hidden md:flex items-center gap-8">
            <a
              href="#architecture"
              className="text-sm font-medium text-slate-300 hover:text-sky-400 transition-colors"
            >
              Architecture
            </a>
            <a
              href="#pipeline"
              className="text-sm font-medium text-slate-300 hover:text-sky-400 transition-colors"
            >
              How It Works
            </a>
            <a
              href="#features"
              className="text-sm font-medium text-slate-300 hover:text-sky-400 transition-colors"
            >
              Consensus & Sharding
            </a>
            <a
              href="#technology"
              className="text-sm font-medium text-slate-300 hover:text-sky-400 transition-colors"
            >
              Technology
            </a>
          </div>

          {/* Right side — Actions */}
          <div className="hidden sm:flex items-center gap-4">
            <a
              href="https://github.com/Anshik-02/orbit"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 px-3.5 py-2 text-xs font-mono font-medium text-slate-300 bg-slate-900/80 hover:bg-slate-800/90 hover:text-white rounded-lg border border-slate-700/60 hover:border-slate-600 transition-all shadow-sm"
            >
              <GithubIcon className="w-4 h-4 text-slate-300" />
              <span>GitHub</span>
              <ArrowUpRight className="w-3.5 h-3.5 opacity-60" />
            </a>

            <Link
              href="/dashboard"
              className="flex items-center gap-2 px-4 py-2 text-xs font-semibold text-white bg-gradient-to-r from-sky-600 via-sky-500 to-indigo-600 hover:from-sky-500 hover:to-indigo-500 rounded-lg shadow-md shadow-sky-600/30 hover:shadow-sky-500/50 border border-sky-400/30 transition-all hover:-translate-y-0.5"
            >
              <LayoutDashboard className="w-4 h-4" />
              <span>Launch Dashboard</span>
            </Link>
          </div>

          {/* Mobile hamburger menu */}
          <div className="md:hidden flex items-center gap-2">
            <Link
              href="/dashboard"
              className="p-2 text-sky-400 bg-sky-950/60 border border-sky-500/30 rounded-lg sm:hidden"
            >
              <LayoutDashboard className="w-4 h-4" />
            </Link>
            <button
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
              className="p-2 text-slate-400 hover:text-white rounded-lg hover:bg-slate-800/60 transition-colors"
              aria-label="Toggle menu"
            >
              {mobileMenuOpen ? <X className="w-6 h-6" /> : <Menu className="w-6 h-6" />}
            </button>
          </div>
        </div>

        {/* Mobile dropdown menu */}
        {mobileMenuOpen && (
          <div className="md:hidden mt-4 pt-4 pb-3 border-t border-slate-800/80 flex flex-col gap-3 animate-fadeIn">
            <a
              href="#architecture"
              onClick={() => setMobileMenuOpen(false)}
              className="px-3 py-2 text-sm text-slate-300 hover:text-sky-400 rounded-lg hover:bg-slate-900/50"
            >
              Architecture
            </a>
            <a
              href="#pipeline"
              onClick={() => setMobileMenuOpen(false)}
              className="px-3 py-2 text-sm text-slate-300 hover:text-sky-400 rounded-lg hover:bg-slate-900/50"
            >
              How It Works
            </a>
            <a
              href="#features"
              onClick={() => setMobileMenuOpen(false)}
              className="px-3 py-2 text-sm text-slate-300 hover:text-sky-400 rounded-lg hover:bg-slate-900/50"
            >
              Consensus & Sharding
            </a>
            <a
              href="#technology"
              onClick={() => setMobileMenuOpen(false)}
              className="px-3 py-2 text-sm text-slate-300 hover:text-sky-400 rounded-lg hover:bg-slate-900/50"
            >
              Technology
            </a>
            <div className="pt-3 border-t border-slate-800/80 flex flex-col gap-2">
              <a
                href="https://github.com/Anshik-02/orbit"
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center justify-center gap-2 px-4 py-2.5 text-xs font-mono text-slate-200 bg-slate-900 rounded-lg border border-slate-700"
              >
                <GithubIcon className="w-4 h-4" />
                <span>GitHub Repository</span>
              </a>
              <Link
                href="/dashboard"
                onClick={() => setMobileMenuOpen(false)}
                className="flex items-center justify-center gap-2 px-4 py-2.5 text-xs font-semibold text-white bg-sky-600 hover:bg-sky-500 rounded-lg shadow-md shadow-sky-600/30"
              >
                <LayoutDashboard className="w-4 h-4" />
                <span>Enter Live Dashboard</span>
              </Link>
            </div>
          </div>
        )}
      </div>
    </nav>
  );
}
