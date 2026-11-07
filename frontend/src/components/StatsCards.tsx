"use client";
import { SystemStats } from "@/lib/types";
import { NavigationTab } from "./Header";

export function StatsCards({ stats, onNavigate }: { stats: SystemStats | null; onNavigate: (tab: NavigationTab) => void }) {
  const items: { label: string; value?: number; target: NavigationTab; attention?: boolean }[] = [
    { label: "bài đã lên lịch", value: stats?.scheduled_posts, target: "history" },
    { label: "bài chờ duyệt", value: stats?.pending_approval_posts, target: "approvals", attention: (stats?.pending_approval_posts || 0) > 0 },
    { label: "bài đăng thất bại", value: stats?.failed_posts, target: "history", attention: (stats?.failed_posts || 0) > 0 },
    { label: "Fanpage hoạt động", value: stats?.active_channels, target: "pages" },
  ];
  return <div className="dispatch-summary" aria-label="Tổng quan không gian làm việc">{items.map(item => <button key={item.label} className={item.attention ? "attention" : ""} onClick={() => onNavigate(item.target)}><strong>{item.value ?? "—"}</strong><span>{item.label}</span></button>)}</div>;
}
