"use client";

import React from "react";
import { SystemHealth } from "@/lib/types";
import { Activity, RefreshCw, Sun, Moon } from "lucide-react";
import { PostPilotLogo } from "@/components/PostPilotLogo";
import { useTheme } from "@/context/ThemeContext";

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
  const { theme, toggleTheme } = useTheme();
  const isHealthy = health?.status === "healthy";

  return (
    <header className="border-b border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-950/80 backdrop-blur-md sticky top-0 z-40 transition-colors duration-200">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-16 gap-4">
          {/* Logo & Brand */}
          <div className="flex items-center gap-3">
            <div className="h-10 w-10 flex-shrink-0 transition-transform hover:scale-105 duration-200">
              <PostPilotLogo className="h-10 w-10" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-extrabold text-lg tracking-tight bg-gradient-to-r from-slate-900 via-slate-800 to-slate-600 dark:from-white dark:via-slate-100 dark:to-slate-400 bg-clip-text text-transparent">
                  PostPilot
                </span>
                <span className="text-[10px] font-bold px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-500/20 tracking-wider">
                  STUDIO
                </span>
              </div>
              <p className="text-xs text-slate-500 dark:text-slate-400 hidden sm:block">
                Hệ thống Tự động hóa Facebook Multi-Page & Content Studio
              </p>
            </div>
          </div>

          {/* Navigation Tabs */}
          <nav className="flex items-center gap-1 bg-slate-100 dark:bg-slate-900/90 p-1 rounded-xl border border-slate-200 dark:border-slate-800/80 shadow-sm dark:shadow-none">
            <button
              onClick={() => onTabChange("studio")}
              className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === "studio"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-white dark:hover:bg-slate-800/60"
              }`}
            >
              ✨ Studio Soạn Thảo
            </button>
            <button
              onClick={() => onTabChange("pages")}
              className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === "pages"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-white dark:hover:bg-slate-800/60"
              }`}
            >
              🛡 Quản lý Fanpage
            </button>
            <button
              onClick={() => onTabChange("history")}
              className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === "history"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-white dark:hover:bg-slate-800/60"
              }`}
            >
              📑 Lịch Trình & Hàng Đợi
            </button>
          </nav>

          {/* System Status & Controls */}
          <div className="flex items-center gap-2 sm:gap-3">
            {/* Status Pill */}
            <div
              className={`hidden md:flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors ${
                isHealthy
                  ? "bg-emerald-50 dark:bg-emerald-950/40 border-emerald-500/30 text-emerald-700 dark:text-emerald-400"
                  : "bg-rose-50 dark:bg-rose-950/40 border-rose-500/30 text-rose-700 dark:text-rose-400"
              }`}
            >
              <span
                className={`h-2 w-2 rounded-full ${
                  isHealthy ? "bg-emerald-500 dark:bg-emerald-400 animate-pulse" : "bg-rose-500 dark:bg-rose-400"
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
                  ? "bg-blue-50 dark:bg-blue-950/40 border-blue-500/30 text-blue-600 dark:text-blue-400"
                  : "bg-slate-100 dark:bg-slate-900 border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200"
              }`}
            >
              <Activity className="h-3.5 w-3.5" />
              <span className="hidden lg:inline">Auto 5s</span>
            </button>

            {/* Theme Toggle Button (Light/Dark) */}
            <button
              onClick={toggleTheme}
              title={theme === "dark" ? "Chuyển sang giao diện Sáng" : "Chuyển sang giao diện Tối"}
              className="p-1.5 rounded-lg text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white bg-slate-100 dark:bg-slate-900 hover:bg-slate-200 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-800 transition-all active:scale-95"
            >
              {theme === "dark" ? (
                <Sun className="h-4 w-4 text-amber-400" />
              ) : (
                <Moon className="h-4 w-4 text-indigo-600" />
              )}
            </button>

            {/* Manual refresh button */}
            <button
              onClick={onManualRefresh}
              disabled={loading}
              title="Làm mới dữ liệu ngay lập tức"
              className="p-1.5 rounded-lg text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white bg-slate-100 dark:bg-slate-900 hover:bg-slate-200 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-800 transition-all active:scale-95 disabled:opacity-50"
            >
              <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin text-blue-500" : ""}`} />
            </button>
          </div>
        </div>
      </div>
    </header>
  );
}

