"use client";

import React, { useState, useRef, useEffect } from "react";
import { SystemHealth } from "@/lib/types";
import {
  Activity,
  RefreshCw,
  Sun,
  Moon,
  Building2,
  ChevronDown,
  Check,
  Plus,
  FlaskConical,
  RotateCcw,
  Clock,
  X,
} from "lucide-react";
import { PostPilotLogo } from "@/components/PostPilotLogo";
import { useTheme } from "@/context/ThemeContext";
import { useOrg } from "@/context/OrgContext";
import { useDialog } from "@/context/DialogContext";

export type NavigationTab = "studio" | "approvals" | "pages" | "team" | "history";

interface HeaderProps {
  health: SystemHealth | null;
  pendingApprovalsCount?: number;
  loading: boolean;
  autoRefresh: boolean;
  onToggleAutoRefresh: () => void;
  onManualRefresh: () => void;
  activeTab: NavigationTab;
  onTabChange: (tab: NavigationTab) => void;
}

export function Header({
  health,
  pendingApprovalsCount = 0,
  loading,
  autoRefresh,
  onToggleAutoRefresh,
  onManualRefresh,
  activeTab,
  onTabChange,
}: HeaderProps) {
  const { theme, toggleTheme } = useTheme();
  const {
    currentOrg,
    organizations,
    switchOrg,
    createOrg,
    currentRole,
    actualRole,
    isSimulating,
    simulateRole,
  } = useOrg();
  const { toast } = useDialog();

  const isHealthy = health?.status === "healthy";

  // Dropdown states
  const [orgDropdownOpen, setOrgDropdownOpen] = useState<boolean>(false);
  const [roleDropdownOpen, setRoleDropdownOpen] = useState<boolean>(false);
  const [showCreateOrgModal, setShowCreateOrgModal] = useState<boolean>(false);
  const [newOrgName, setNewOrgName] = useState<string>("");
  const [isCreatingOrg, setIsCreatingOrg] = useState<boolean>(false);

  const orgRef = useRef<HTMLDivElement>(null);
  const roleRef = useRef<HTMLDivElement>(null);

  // Close dropdowns on click outside
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (orgRef.current && !orgRef.current.contains(event.target as Node)) {
        setOrgDropdownOpen(false);
      }
      if (roleRef.current && !roleRef.current.contains(event.target as Node)) {
        setRoleDropdownOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const handleCreateNewOrg = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newOrgName.trim()) return;
    setIsCreatingOrg(true);
    try {
      const created = await createOrg(newOrgName.trim());
      toast(`✨ Đã tạo Không gian làm việc "${created.name}"!`, "success");
      setNewOrgName("");
      setShowCreateOrgModal(false);
      setOrgDropdownOpen(false);
    } catch (err: any) {
      toast(err.message || "Lỗi khi tạo tổ chức", "danger");
    } finally {
      setIsCreatingOrg(false);
    }
  };

  const roleColorMap: Record<string, string> = {
    OWNER: "bg-purple-100 dark:bg-purple-500/20 text-purple-700 dark:text-purple-300 border-purple-300 dark:border-purple-500/30",
    ADMIN: "bg-blue-100 dark:bg-blue-500/20 text-blue-700 dark:text-blue-300 border-blue-300 dark:border-blue-500/30",
    MANAGER: "bg-emerald-100 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 border-emerald-300 dark:border-emerald-500/30",
    APPROVER: "bg-amber-100 dark:bg-amber-500/20 text-amber-700 dark:text-amber-300 border-amber-300 dark:border-amber-500/30",
    EDITOR: "bg-cyan-100 dark:bg-cyan-500/20 text-cyan-700 dark:text-cyan-300 border-cyan-300 dark:border-cyan-500/30",
    VIEWER: "bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border-slate-300 dark:border-slate-700",
  };

  return (
    <header className="border-b border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-950/80 backdrop-blur-md sticky top-0 z-40 transition-colors duration-200">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-16 gap-3">
          {/* Logo & Workspace Switcher */}
          <div className="flex items-center gap-3">
            <div className="h-10 w-10 flex-shrink-0 transition-transform hover:scale-105 duration-200">
              <PostPilotLogo className="h-10 w-10" />
            </div>

            {/* Brand Title */}
            <div className="hidden lg:block">
              <div className="flex items-center gap-2">
                <span className="font-extrabold text-lg tracking-tight bg-gradient-to-r from-slate-900 via-slate-800 to-slate-600 dark:from-white dark:via-slate-100 dark:to-slate-400 bg-clip-text text-transparent">
                  PostPilot
                </span>
                <span className="text-[10px] font-bold px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-500/20 tracking-wider">
                  STUDIO
                </span>
              </div>
            </div>

            {/* Workspace Switcher Dropdown */}
            <div className="relative" ref={orgRef}>
              <button
                type="button"
                onClick={() => setOrgDropdownOpen(!orgDropdownOpen)}
                className="flex items-center gap-2 px-3 py-1.5 rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800/80 text-xs font-semibold text-slate-800 dark:text-slate-200 transition-colors max-w-[200px]"
              >
                <Building2 className="h-3.5 w-3.5 text-blue-600 dark:text-blue-400 shrink-0" />
                <span className="truncate">{currentOrg?.name || "Không gian làm việc"}</span>
                <ChevronDown className="h-3 w-3 text-slate-400 shrink-0" />
              </button>

              {orgDropdownOpen && (
                <div className="absolute left-0 mt-2 w-64 rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-xl py-2 z-50 animate-fade-in text-xs">
                  <div className="px-3 py-1.5 text-[10px] font-bold uppercase tracking-wider text-slate-400">
                    Không gian làm việc ({organizations.length})
                  </div>
                  <div className="max-h-56 overflow-y-auto">
                    {organizations.map((org) => {
                      const isSelected = org.id === currentOrg?.id;
                      return (
                        <button
                          key={org.id}
                          type="button"
                          onClick={() => {
                            switchOrg(org);
                            setOrgDropdownOpen(false);
                          }}
                          className={`w-full px-3 py-2 text-left flex items-center justify-between hover:bg-slate-50 dark:hover:bg-slate-800/60 transition-colors ${
                            isSelected ? "font-bold text-blue-600 dark:text-blue-400 bg-blue-50/50 dark:bg-blue-950/20" : "text-slate-700 dark:text-slate-300"
                          }`}
                        >
                          <div className="truncate">
                            <span className="block truncate">{org.name}</span>
                            <span className="text-[10px] text-slate-400 block truncate">slug: {org.slug}</span>
                          </div>
                          {isSelected && <Check className="h-4 w-4 shrink-0 text-blue-600 dark:text-blue-400" />}
                        </button>
                      );
                    })}
                  </div>

                  <div className="border-t border-slate-100 dark:border-slate-800 mt-1 pt-1 px-1">
                    <button
                      type="button"
                      onClick={() => {
                        setShowCreateOrgModal(true);
                        setOrgDropdownOpen(false);
                      }}
                      className="w-full px-3 py-2 rounded-lg text-left text-blue-600 dark:text-blue-400 font-semibold hover:bg-blue-50 dark:hover:bg-blue-950/40 flex items-center gap-1.5 transition-colors"
                    >
                      <Plus className="h-3.5 w-3.5" />
                      <span>Tạo Không gian mới</span>
                    </button>
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* Navigation Tabs */}
          <nav className="hidden xl:flex items-center gap-1 bg-slate-100 dark:bg-slate-900/90 p-1 rounded-xl border border-slate-200 dark:border-slate-800/80 shadow-sm dark:shadow-none">
            <button
              onClick={() => onTabChange("studio")}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === "studio"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-white dark:hover:bg-slate-800/60"
              }`}
            >
              ✨ Studio Soạn Thảo
            </button>

            <button
              onClick={() => onTabChange("approvals")}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5 ${
                activeTab === "approvals"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-white dark:hover:bg-slate-800/60"
              }`}
            >
              <span>📥 Phê Duyệt</span>
              {pendingApprovalsCount > 0 && (
                <span
                  className={`px-1.5 py-0.2 rounded-full text-[10px] font-bold ${
                    activeTab === "approvals"
                      ? "bg-white text-blue-700"
                      : "bg-amber-500 text-white animate-pulse"
                  }`}
                >
                  {pendingApprovalsCount}
                </span>
              )}
            </button>

            <button
              onClick={() => onTabChange("pages")}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === "pages"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-white dark:hover:bg-slate-800/60"
              }`}
            >
              🛡 Quản lý Fanpage
            </button>

            <button
              onClick={() => onTabChange("team")}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === "team"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-white dark:hover:bg-slate-800/60"
              }`}
            >
              👥 Nhóm & Phân Quyền
            </button>

            <button
              onClick={() => onTabChange("history")}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                activeTab === "history"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-white dark:hover:bg-slate-800/60"
              }`}
            >
              📑 Hàng Đợi
            </button>
          </nav>

          {/* System Status, Role Simulator & Controls */}
          <div className="flex items-center gap-2">
            {/* Role Simulator Pill */}
            <div className="relative" ref={roleRef}>
              <button
                type="button"
                onClick={() => setRoleDropdownOpen(!roleDropdownOpen)}
                title="Mô phỏng vai trò để kiểm tra tính năng RBAC"
                className={`flex items-center gap-1.5 px-2.5 py-1.5 rounded-xl border text-xs font-bold transition-all ${
                  isSimulating
                    ? "ring-2 ring-amber-400/50 " + (roleColorMap[currentRole] || "")
                    : roleColorMap[currentRole] || "bg-slate-100 text-slate-700"
                }`}
              >
                <FlaskConical className={`h-3.5 w-3.5 ${isSimulating ? "text-amber-500 animate-bounce" : ""}`} />
                <span className="capitalize">{currentRole}</span>
                {isSimulating && <span className="text-[9px] bg-amber-500 text-white px-1 rounded">SIM</span>}
                <ChevronDown className="h-3 w-3 opacity-60" />
              </button>

              {roleDropdownOpen && (
                <div className="absolute right-0 mt-2 w-56 rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-xl py-2 z-50 animate-fade-in text-xs space-y-1">
                  <div className="px-3 py-1 text-[10px] font-bold uppercase tracking-wider text-slate-400">
                    Mô phỏng vai trò RBAC
                  </div>

                  {["OWNER", "ADMIN", "MANAGER", "APPROVER", "EDITOR", "VIEWER"].map((r) => {
                    const isCurrent = currentRole === r;
                    return (
                      <button
                        key={r}
                        type="button"
                        onClick={() => {
                          simulateRole(r);
                          setRoleDropdownOpen(false);
                          toast(`🧪 Đang mô phỏng quyền: ${r}`, "info");
                        }}
                        className={`w-full px-3 py-1.5 text-left flex items-center justify-between hover:bg-slate-50 dark:hover:bg-slate-800/60 transition-colors ${
                          isCurrent ? "font-bold text-blue-600 dark:text-blue-400 bg-blue-50/50 dark:bg-blue-950/20" : "text-slate-700 dark:text-slate-300"
                        }`}
                      >
                        <span className="capitalize">{r}</span>
                        {isCurrent && <Check className="h-3.5 w-3.5 text-blue-600 dark:text-blue-400" />}
                      </button>
                    );
                  })}

                  {isSimulating && (
                    <div className="border-t border-slate-100 dark:border-slate-800 mt-1 pt-1 px-1">
                      <button
                        type="button"
                        onClick={() => {
                          simulateRole(null);
                          setRoleDropdownOpen(false);
                          toast(`Đã hoàn tất mô phỏng, trở lại vai trò thực tế: ${actualRole}`, "success");
                        }}
                        className="w-full px-3 py-1.5 rounded-lg text-left text-rose-600 dark:text-rose-400 font-semibold hover:bg-rose-50 dark:hover:bg-rose-950/40 flex items-center gap-1.5 transition-colors"
                      >
                        <RotateCcw className="h-3.5 w-3.5" />
                        <span>Hủy mô phỏng (Về: {actualRole})</span>
                      </button>
                    </div>
                  )}
                </div>
              )}
            </div>

            {/* Status Pill */}
            <div
              className={`hidden md:flex items-center gap-2 px-2.5 py-1.5 rounded-xl text-xs font-medium border transition-colors ${
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
                {health ? `Online (${health.redis_mode})` : "Đang kết nối..."}
              </span>
            </div>

            {/* Auto refresh switch */}
            <button
              onClick={onToggleAutoRefresh}
              title="Bật/Tắt tự động cập nhật dữ liệu mỗi 5s"
              className={`hidden sm:flex items-center gap-1.5 px-2.5 py-1.5 rounded-xl text-xs font-medium border transition-all ${
                autoRefresh
                  ? "bg-blue-50 dark:bg-blue-950/40 border-blue-500/30 text-blue-600 dark:text-blue-400"
                  : "bg-slate-100 dark:bg-slate-900 border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200"
              }`}
            >
              <Activity className="h-3.5 w-3.5" />
              <span className="hidden xl:inline">Auto 5s</span>
            </button>

            {/* Theme Toggle Button (Light/Dark) */}
            <button
              onClick={toggleTheme}
              title={theme === "dark" ? "Chuyển sang giao diện Sáng" : "Chuyển sang giao diện Tối"}
              className="p-1.5 rounded-xl text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white bg-slate-100 dark:bg-slate-900 hover:bg-slate-200 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-800 transition-all active:scale-95"
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
              className="p-1.5 rounded-xl text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white bg-slate-100 dark:bg-slate-900 hover:bg-slate-200 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-800 transition-all active:scale-95 disabled:opacity-50"
            >
              <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin text-blue-500" : ""}`} />
            </button>
          </div>
        </div>

        {/* Mobile Sub-Navigation Bar */}
        <div className="xl:hidden flex items-center justify-between overflow-x-auto py-2 border-t border-slate-200 dark:border-slate-800 gap-1 text-xs">
          <button
            onClick={() => onTabChange("studio")}
            className={`px-2.5 py-1 rounded-lg shrink-0 font-medium ${
              activeTab === "studio" ? "bg-blue-600 text-white" : "text-slate-600 dark:text-slate-400"
            }`}
          >
            Studio
          </button>
          <button
            onClick={() => onTabChange("approvals")}
            className={`px-2.5 py-1 rounded-lg shrink-0 font-medium flex items-center gap-1 ${
              activeTab === "approvals" ? "bg-blue-600 text-white" : "text-slate-600 dark:text-slate-400"
            }`}
          >
            <span>Phê Duyệt</span>
            {pendingApprovalsCount > 0 && (
              <span className="px-1 py-0.2 rounded-full bg-amber-500 text-white text-[9px] font-bold">
                {pendingApprovalsCount}
              </span>
            )}
          </button>
          <button
            onClick={() => onTabChange("pages")}
            className={`px-2.5 py-1 rounded-lg shrink-0 font-medium ${
              activeTab === "pages" ? "bg-blue-600 text-white" : "text-slate-600 dark:text-slate-400"
            }`}
          >
            Fanpage
          </button>
          <button
            onClick={() => onTabChange("team")}
            className={`px-2.5 py-1 rounded-lg shrink-0 font-medium ${
              activeTab === "team" ? "bg-blue-600 text-white" : "text-slate-600 dark:text-slate-400"
            }`}
          >
            Đội Ngũ
          </button>
          <button
            onClick={() => onTabChange("history")}
            className={`px-2.5 py-1 rounded-lg shrink-0 font-medium ${
              activeTab === "history" ? "bg-blue-600 text-white" : "text-slate-600 dark:text-slate-400"
            }`}
          >
            Hàng Đợi
          </button>
        </div>
      </div>

      {/* Create Org Modal */}
      {showCreateOrgModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-fade-in">
          <div className="w-full max-w-sm rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-2">
                <Building2 className="h-4 w-4 text-blue-500" />
                <span>Tạo Không Gian Làm Việc Mới</span>
              </h3>
              <button
                type="button"
                onClick={() => setShowCreateOrgModal(false)}
                className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            <form onSubmit={handleCreateNewOrg} className="space-y-4">
              <div className="space-y-1">
                <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                  Tên Không gian / Doanh nghiệp
                </label>
                <input
                  type="text"
                  required
                  placeholder="Ví dụ: Brand Agency Alpha"
                  value={newOrgName}
                  onChange={(e) => setNewOrgName(e.target.value)}
                  className="w-full px-3 py-2 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-slate-100 focus:outline-none focus:border-blue-500"
                />
              </div>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateOrgModal(false)}
                  className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
                >
                  Hủy
                </button>
                <button
                  type="submit"
                  disabled={isCreatingOrg}
                  className="px-5 py-2 rounded-xl text-xs font-bold text-white bg-blue-600 hover:bg-blue-500 shadow-md shadow-blue-600/25 transition-all disabled:opacity-50"
                >
                  {isCreatingOrg ? "Đang tạo..." : "Tạo Mới"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </header>
  );
}
