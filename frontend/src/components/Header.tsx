"use client";

import React from "react";
import { SystemHealth } from "@/lib/types";
import { Activity, RefreshCw, Zap, Server, ShieldCheck } from "lucide-react";

interface HeaderProps {
  health: SystemHealth | null;
  loading: boolean;
  autoRefresh: boolean;
  onToggleAutoRefresh: () => void;
  onManualRefresh: () => void;
  activeTab: "studio" | "pages" | "history";
  onTabChange: (tab: "studio" | "pages" | "history") => void;
}

export function Header({
  health,
  loading,
  autoRefresh,
  onToggleAutoRefresh,
  onManualRefresh,
  activeTab,
  onTabChange,
}: HeaderProps) {
  const isHealthy = health?.status === "healthy";

  return (
    <header className="border-b border-slate-800 bg-slate-950/80 backdrop-blur-md sticky top-0 z-40 transition-all">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-16 gap-4">
          {/* Logo & Brand */}
          <div className="flex items-center gap-3">
            <div className="h-10 w-10 rounded-xl bg-gradient-to-tr from-blue-600 via-indigo-600 to-cyan-400 p-[2px] shadow-lg shadow-blue-500/20">
              <div className="h-full w-full bg-slate-950 rounded-[10px] flex items-center justify-center">
                <Zap className="h-5 w-5 text-blue-400" />
              </div>
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-extrabold text-lg tracking-tight bg-gradient-to-r from-white via-slate-100 to-slate-400 bg-clip-text text-transparent">
                  PostPilot
                </span>
                <span className="text-[10px] font-bold px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20 tracking-wider">
                  STUDIO
                </span>
              </div>
              <p className="text-xs text-slate-400 hidden sm:block">
                Hệ thống Tự động hóa Facebook Multi-Page & Content Studio
              </p>
            </div>
          </div>

          {/* Navigation Tabs */}
          <nav className="flex items-center gap-1 bg-slate-900/90 p-1 rounded-xl border border-slate-800/80">
            <button
              onClick={() => onTabChange("studio")}
              className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === "studio"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/60"
              }`}
            >
              ✨ Studio Soạn Thảo
            </button>
            <button
              onClick={() => onTabChange("pages")}
              className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === "pages"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/60"
              }`}
            >
              🛡 Quản lý Fanpage
            </button>
            <button
              onClick={() => onTabChange("history")}
              className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === "history"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/60"
              }`}
            >
              📑 Lịch Trình & Hàng Đợi
            </button>
          </nav>

          {/* System Status & Controls */}
          <div className="flex items-center gap-3">
            {/* Status Pill */}
            <div
              className={`hidden md:flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors ${
                isHealthy
                  ? "bg-emerald-950/40 border-emerald-500/30 text-emerald-400"
                  : "bg-rose-950/40 border-rose-500/30 text-rose-400"
              }`}
            >
              <span
                className={`h-2 w-2 rounded-full ${
                  isHealthy ? "bg-emerald-400 animate-pulse" : "bg-rose-400"
                }`}
              />
              <span className="truncate">
                {health
                  ? `Online (${health.redis_mode})`
                  : "Đang kết nối..."}
              </span>
            </div>

            {/* Auto refresh switch */}
            <button
              onClick={onToggleAutoRefresh}
              title="Bật/Tắt tự động cập nhật dữ liệu mỗi 5s"
              className={`flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium border transition-all ${
                autoRefresh
                  ? "bg-blue-950/40 border-blue-500/30 text-blue-400"
                  : "bg-slate-900 border-slate-800 text-slate-400 hover:text-slate-200"
              }`}
            >
              <Activity className="h-3.5 w-3.5" />
              <span className="hidden lg:inline">Auto 5s</span>
            </button>

            {/* Manual refresh button */}
            <button
              onClick={onManualRefresh}
              disabled={loading}
              title="Làm mới dữ liệu ngay lập tức"
              className="p-1.5 rounded-lg text-slate-400 hover:text-white bg-slate-900 hover:bg-slate-800 border border-slate-800 transition-all active:scale-95 disabled:opacity-50"
            >
              <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin text-blue-400" : ""}`} />
            </button>
          </div>
        </div>
      </div>
    </header>
  );
}
