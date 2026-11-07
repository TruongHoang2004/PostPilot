"use client";

import { useState } from "react";
import { SystemHealth } from "@/lib/types";
import { ArrowUpRight, Check, ChevronDown, LogOut, Moon, Plus, RefreshCw, Sun, X } from "lucide-react";
import { useTheme } from "@/context/ThemeContext";
import { useOrg } from "@/context/OrgContext";
import { Modal } from "./Modal";
import { useDialog } from "@/context/DialogContext";

export type NavigationTab = "studio" | "bulk" | "approvals" | "pages" | "team" | "history";
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

export function Header({ health, pendingApprovalsCount = 0, loading, autoRefresh, onToggleAutoRefresh, onManualRefresh, activeTab, onTabChange }: HeaderProps) {
  const { theme, toggleTheme } = useTheme();
  const { currentOrg, currentUser, organizations, switchOrg, createOrg, currentRole, logout } = useOrg();
  const { toast } = useDialog();
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [busy, setBusy] = useState(false);
  const [createError, setCreateError] = useState("");
  const tabs: { id: NavigationTab; name: string }[] = [
    { id: "history", name: "Lịch đăng" }, { id: "studio", name: "Soạn bài" }, { id: "bulk", name: "Hàng loạt" },
    { id: "approvals", name: "Phê duyệt" }, { id: "pages", name: "Fanpage" }, { id: "team", name: "Đội nhóm" },
  ];
  const roleLabel: Record<string, string> = { OWNER: "Chủ sở hữu", ADMIN: "Quản trị viên", MEMBER: "Thành viên" };
  return <header className="dispatch-header">
    <div className="dispatch-header-top">
      <button className="dispatch-brand" onClick={() => onTabChange("history")} aria-label="PostPilot — về lịch đăng">POSTPILOT<span>↗</span></button>
      <div className="dispatch-workspace">
        <label className="sr-only" htmlFor="workspace-select">Không gian làm việc</label>
        <select id="workspace-select" value={currentOrg?.id || ""} onChange={event => {
          const org = organizations.find(item => item.id === event.target.value);
          if (org) switchOrg(org);
        }}>{organizations.map(org => <option key={org.id} value={org.id}>{org.name}</option>)}</select>
      </div>
      <div className="dispatch-account-area">
        <span className={`dispatch-connection ${health?.status === "healthy" ? "connected" : ""}`}><span aria-hidden="true" />{health?.status === "healthy" ? "Đã kết nối" : "Chưa xác nhận kết nối"}</span>
        <button className="dispatch-icon-button" onClick={onManualRefresh} disabled={loading} aria-label="Làm mới dữ liệu"><RefreshCw size={17} className={loading ? "animate-spin" : ""} /></button>
        <details className="dispatch-account">
          <summary aria-label={`Tài khoản: ${currentUser?.full_name || currentUser?.email || "người dùng"}`}><span className="dispatch-avatar">{(currentUser?.full_name || currentUser?.email || "?").slice(0, 1).toUpperCase()}</span><ChevronDown size={14} aria-hidden="true" /></summary>
          <div className="dispatch-account-menu">
            <p className="font-medium break-words">{currentUser?.full_name || currentUser?.email}</p><p className="dispatch-muted text-sm mb-3">{roleLabel[currentRole] || currentRole}</p>
            <button onClick={toggleTheme}>{theme === "light" ? <Moon size={16} /> : <Sun size={16} />}Giao diện {theme === "light" ? "tối" : "sáng"}</button>
            <button onClick={onToggleAutoRefresh} aria-pressed={autoRefresh}><Check aria-hidden="true" size={16} style={{ opacity: autoRefresh ? 1 : 0 }} />Tự cập nhật mỗi 5 giây</button>
            <button onClick={() => { setCreateError(""); setCreating(true); }}><Plus size={16} />Tạo không gian mới</button>
            <button onClick={logout}><LogOut size={16} />Đăng xuất</button>
          </div>
        </details>
      </div>
    </div>
    <nav className="dispatch-navigation" aria-label="Điều hướng chính">{tabs.map(tab => <button key={tab.id} aria-current={activeTab === tab.id ? "page" : undefined} onClick={() => onTabChange(tab.id)}>{tab.name}{tab.id === "approvals" && pendingApprovalsCount > 0 && <span className="dispatch-count">{pendingApprovalsCount}</span>}</button>)}</nav>
    {creating && <Modal labelledBy="create-workspace-heading" onClose={() => setCreating(false)}><section className="dispatch-panel dispatch-create-org">
      <div className="dispatch-section-heading"><h2 id="create-workspace-heading">Không gian mới</h2><button className="dispatch-icon-button" onClick={() => setCreating(false)} aria-label="Đóng"><X size={20} /></button></div>
      <form onSubmit={async event => {
        event.preventDefault(); setBusy(true); setCreateError("");
        try { await createOrg(newName.trim()); setCreating(false); setNewName(""); toast("Đã tạo không gian làm việc.", "success"); }
        catch (error) { setCreateError(error instanceof Error ? error.message : "Không thể tạo không gian"); }
        finally { setBusy(false); }
      }}>{createError && <p role="alert" className="dispatch-error">{createError}</p>}<label className="dispatch-field">Tên không gian<input autoFocus required value={newName} onChange={event => setNewName(event.target.value)} placeholder="Tên doanh nghiệp hoặc đội nhóm" /></label><button disabled={busy || !newName.trim()} className="dispatch-button primary mt-5">{busy ? "Đang tạo…" : "Tạo không gian"}<ArrowUpRight size={16} /></button></form>
    </section></Modal>}
  </header>;
}
