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
          ? "bg-white/85 backdrop-blur-md shadow-sm border-b border-[#DCE5F0] py-3.5"
          : "bg-transparent py-5 border-b border-transparent"
      }`}
    >
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between">
          {/* Left side — Brand */}
          <Link href="/" className="flex items-center gap-3 group">
            <div className="relative flex items-center justify-center w-10 h-10 rounded-xl bg-gradient-to-br from-sky-50 via-sky-100 to-indigo-50 border border-[#DCE5F0] group-hover:border-[#0891D5] group-hover:shadow-[0_0_15px_rgba(8,145,213,0.2)] transition-all">
              <Database className="w-5 h-5 text-[#0891D5] group-hover:rotate-12 transition-transform duration-300" />
            </div>
            <div className="flex flex-col">
              <div className="flex items-center gap-2">
                <span className="font-bold text-lg tracking-wider text-[#17243A]">
                  ORBIT
                </span>
                <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-sky-50 border border-sky-200 text-[#0891D5] font-semibold">
                  v1.4
                </span>
              </div>
              <span className="text-[11px] text-[#64748B] font-mono tracking-tight hidden sm:inline">
                Distributed Key-Value Database
              </span>
            </div>
          </Link>

          {/* Center Navigation Links */}
          <div className="hidden md:flex items-center gap-8">
            <a
              href="#architecture"
              className="text-sm font-medium text-[#64748B] hover:text-[#0891D5] transition-colors"
            >
              Architecture
            </a>
            <a
              href="#pipeline"
              className="text-sm font-medium text-[#64748B] hover:text-[#0891D5] transition-colors"
            >
              How It Works
            </a>
            <a
              href="#features"
              className="text-sm font-medium text-[#64748B] hover:text-[#0891D5] transition-colors"
            >
              Consensus & Sharding
            </a>
            <a
              href="#technology"
              className="text-sm font-medium text-[#64748B] hover:text-[#0891D5] transition-colors"
            >
              Technology
            </a>
          </div>

          {/* Right side — Actions */}
          <div className="hidden sm:flex items-center gap-3.5">
            <a
              href="https://github.com/Anshik-02/orbit"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 px-3.5 py-2 text-xs font-mono font-medium text-[#17243A] bg-white hover:bg-slate-50 hover:text-[#0891D5] rounded-lg border border-[#DCE5F0] hover:border-slate-300 transition-all shadow-sm"
            >
              <GithubIcon className="w-4 h-4 text-[#17243A]" />
              <span>GitHub</span>
              <ArrowUpRight className="w-3.5 h-3.5 opacity-50" />
            </a>

            <Link
              href="/dashboard"
              className="flex items-center gap-2 px-4 py-2 text-xs font-semibold text-white bg-gradient-to-r from-[#0891D5] to-[#7965E8] hover:from-[#0284c7] hover:to-[#6d28d9] rounded-lg shadow-md shadow-sky-500/20 hover:shadow-lg hover:shadow-indigo-500/30 transition-all hover:-translate-y-0.5"
            >
              <LayoutDashboard className="w-4 h-4" />
              <span>Launch Dashboard</span>
            </Link>
          </div>

          {/* Mobile hamburger menu */}
          <div className="md:hidden flex items-center gap-2">
            <Link
              href="/dashboard"
              className="p-2 text-[#0891D5] bg-sky-50 border border-sky-200 rounded-lg sm:hidden"
            >
              <LayoutDashboard className="w-4 h-4" />
            </Link>
            <button
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
              className="p-2 text-[#64748B] hover:text-[#17243A] rounded-lg hover:bg-slate-100 transition-colors"
              aria-label="Toggle menu"
            >
              {mobileMenuOpen ? <X className="w-6 h-6" /> : <Menu className="w-6 h-6" />}
            </button>
          </div>
        </div>

        {/* Mobile dropdown menu */}
        {mobileMenuOpen && (
          <div className="md:hidden mt-4 pt-4 pb-3 border-t border-[#DCE5F0] flex flex-col gap-3 bg-white/95 backdrop-blur-md rounded-xl p-4 shadow-lg border">
            <a
              href="#architecture"
              onClick={() => setMobileMenuOpen(false)}
              className="px-3 py-2 text-sm text-[#17243A] hover:text-[#0891D5] rounded-lg hover:bg-slate-50 font-medium"
            >
              Architecture
            </a>
            <a
              href="#pipeline"
              onClick={() => setMobileMenuOpen(false)}
              className="px-3 py-2 text-sm text-[#17243A] hover:text-[#0891D5] rounded-lg hover:bg-slate-50 font-medium"
            >
              How It Works
            </a>
            <a
              href="#features"
              onClick={() => setMobileMenuOpen(false)}
              className="px-3 py-2 text-sm text-[#17243A] hover:text-[#0891D5] rounded-lg hover:bg-slate-50 font-medium"
            >
              Consensus & Sharding
            </a>
            <a
              href="#technology"
              onClick={() => setMobileMenuOpen(false)}
              className="px-3 py-2 text-sm text-[#17243A] hover:text-[#0891D5] rounded-lg hover:bg-slate-50 font-medium"
            >
              Technology
            </a>
            <div className="pt-3 border-t border-[#DCE5F0] flex flex-col gap-2">
              <a
                href="https://github.com/Anshik-02/orbit"
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center justify-center gap-2 px-4 py-2.5 text-xs font-mono text-[#17243A] bg-slate-50 rounded-lg border border-[#DCE5F0]"
              >
                <GithubIcon className="w-4 h-4" />
                <span>GitHub Repository</span>
              </a>
              <Link
                href="/dashboard"
                onClick={() => setMobileMenuOpen(false)}
                className="flex items-center justify-center gap-2 px-4 py-2.5 text-xs font-semibold text-white bg-gradient-to-r from-[#0891D5] to-[#7965E8] rounded-lg shadow-md"
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
