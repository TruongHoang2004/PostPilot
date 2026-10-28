"use client";

import React, { useState, useEffect, useCallback } from "react";
import Link from "next/link";
import { AuthGate } from "@/components/AuthGate";
import { Header, NavigationTab } from "@/components/Header";
import { StatsCards } from "@/components/StatsCards";
import { PostComposer } from "@/components/PostComposer";
import { PostList } from "@/components/PostList";
import { PageManager } from "@/components/PageManager";
import { ApprovalInbox } from "@/components/ApprovalInbox";
import { TeamAndGroupsManager } from "@/components/TeamAndGroupsManager";
import { api } from "@/lib/api";
import { Channel, Post, SystemHealth, SystemStats } from "@/lib/types";
import { useDialog } from "@/context/DialogContext";
import { useOrg } from "@/context/OrgContext";
import { Flag, ArrowRight, Clock, ShieldCheck } from "lucide-react";

export default function Home() {
  const { alert, toast } = useDialog();
  const { currentOrg, loading: identityLoading } = useOrg();
  const [activeTab, setActiveTab] = useState<NavigationTab>("studio");
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [stats, setStats] = useState<SystemStats | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [autoRefresh, setAutoRefresh] = useState<boolean>(true);

  const fetchAllData = useCallback(async () => {
    if (!currentOrg) { setChannels([]); setPosts([]); setStats(null); return; }
    try {
      const [healthData, statsData, channelsData, postsData] = await Promise.allSettled([
        api.getHealth(),
        api.getStats(),
        api.getChannels(),
        api.getPosts("ALL", 50),
      ]);

      if (healthData.status === "fulfilled") setHealth(healthData.value);
      if (statsData.status === "fulfilled") setStats(statsData.value);
      if (channelsData.status === "fulfilled") setChannels(channelsData.value);
      if (postsData.status === "fulfilled") setPosts(postsData.value);
    } catch (err) {
      console.error("Lỗi khi tải dữ liệu:", err);
    } finally {
      setLoading(false);
    }
  }, [currentOrg]);

  // Fetch when workspace/org changes
  useEffect(() => {
    fetchAllData();
  }, [fetchAllData, currentOrg]);

  // Handle OAuth callback URL status
  useEffect(() => {
    if (typeof window === "undefined") return;
    const params = new URLSearchParams(window.location.search);
    const oauthStatus = params.get("oauth");
    if (oauthStatus === "success") {
      const count = params.get("count") || "các";
      toast(`🎉 Đăng nhập Facebook thành công! Đã tự động kết nối và đồng bộ ${count} Fanpage vào hệ thống.`, "success");
      fetchAllData();
      window.history.replaceState({}, "", window.location.pathname);
    } else if (oauthStatus === "error") {
      const msg = params.get("message") || "Không thể xác thực Facebook";
      alert({
        title: "Đăng nhập Facebook thất bại",
        message: msg,
        variant: "danger",
      });
      window.history.replaceState({}, "", window.location.pathname);
    }
  }, [fetchAllData, toast, alert]);

  // Auto-refresh timer every 5s
  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(() => {
      fetchAllData();
    }, 5000);
    return () => clearInterval(interval);
  }, [autoRefresh, fetchAllData]);

  if (identityLoading) return <main className="min-h-screen flex items-center justify-center">Đang xác minh tài khoản…</main>;
  if (!currentOrg) return <AuthGate />;

  return (
    <div className="min-h-screen flex flex-col bg-slate-50 dark:bg-[#090d16] text-slate-800 dark:text-slate-100 transition-colors duration-200">
      {/* Header bar */}
      <Header
        health={health}
        pendingApprovalsCount={stats?.pending_approval_posts || 0}
        loading={loading}
        autoRefresh={autoRefresh}
        onToggleAutoRefresh={() => setAutoRefresh(!autoRefresh)}
        onManualRefresh={fetchAllData}
        activeTab={activeTab}
        onTabChange={setActiveTab}
      />

      {/* Main Content Area */}
      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-6">
        {/* System Overview Stats Cards */}
        <StatsCards stats={stats} />

        {/* Tab 1: Studio Soạn Thảo */}
        {activeTab === "studio" && (
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
            {/* Left Column: Post Composer (7 cols) */}
            <div className="lg:col-span-7 space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="text-base font-bold text-slate-900 dark:text-white flex items-center gap-2">
                  <span>✨ Soạn Thảo Bài Đăng Mới</span>
                </h2>
                <span className="text-xs text-slate-500 dark:text-slate-400">
                  Hỗ trợ Kênh Đơn lẻ & Cụm Kênh (Batch Fan-out)
                </span>
              </div>
              <PostComposer channels={channels} onSuccess={fetchAllData} />
            </div>

            {/* Right Column: Quick Status & Recent Queue (5 cols) */}
            <div className="lg:col-span-5 space-y-6">
              {/* Connected Channels Widget */}
              <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm p-5 shadow-sm dark:shadow-xl">
                <div className="flex items-center justify-between mb-3">
                  <h3 className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-2">
                    <Flag className="h-4 w-4 text-blue-600 dark:text-blue-400" />
                    <span>Kênh Phát Sóng Đang Quản Trị ({channels.length})</span>
                  </h3>
                  <button
                    onClick={() => setActiveTab("pages")}
                    className="text-xs font-semibold text-blue-600 dark:text-blue-400 hover:text-blue-500 flex items-center gap-1 transition-colors"
                  >
                    Quản lý <ArrowRight className="h-3 w-3" />
                  </button>
                </div>

                {channels.length === 0 ? (
                  <p className="text-xs text-slate-500 py-4 text-center">
                    Chưa kết nối Kênh nào. Nhấn Quản lý để kết nối.
                  </p>
                ) : (
                  <div className="space-y-2 max-h-48 overflow-y-auto pr-1">
                    {channels.slice(0, 4).map((c) => (
                      <div
                        key={c.id}
                        className="flex items-center justify-between p-2.5 rounded-xl bg-slate-50 dark:bg-slate-950/60 border border-slate-200 dark:border-slate-800/80 text-xs"
                      >
                        <div className="flex items-center gap-2.5 truncate">
                          <span className="h-7 w-7 rounded-lg bg-blue-100 dark:bg-blue-600/20 text-blue-600 dark:text-blue-400 border border-blue-200 dark:border-blue-500/30 flex items-center justify-center text-[10px] font-bold shrink-0">
                            {c.name.charAt(0).toUpperCase()}
                          </span>
                          <div className="truncate">
                            <span className="font-semibold text-slate-800 dark:text-slate-200 block truncate">{c.name}</span>
                            <span className="text-[10px] text-slate-500">{c.platform || "FACEBOOK"} • {c.external_id}</span>
                          </div>
                        </div>
                        <span
                          className={`px-2 py-0.5 rounded text-[10px] font-bold shrink-0 ${
                            c.token_status === "ACTIVE"
                              ? "bg-emerald-50 dark:bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-500/30"
                              : "bg-rose-50 dark:bg-rose-500/15 text-rose-600 dark:text-rose-400 border border-rose-200 dark:border-rose-500/30"
                          }`}
                        >
                          {c.token_status}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Pending Approvals Quick Banner if any */}
              {(stats?.pending_approval_posts ?? 0) > 0 && (
                <div className="rounded-2xl border border-amber-300 dark:border-amber-500/30 bg-amber-50/70 dark:bg-amber-950/30 p-4 shadow-sm flex items-center justify-between gap-3">
                  <div className="flex items-center gap-3">
                    <span className="p-2 rounded-xl bg-amber-500/20 text-amber-700 dark:text-amber-400">
                      <ShieldCheck className="h-5 w-5" />
                    </span>
                    <div>
                      <p className="text-xs font-bold text-amber-900 dark:text-amber-300">
                        Có {stats?.pending_approval_posts} bài viết đang chờ phê duyệt
                      </p>
                      <p className="text-[11px] text-amber-700 dark:text-amber-400">
                        Vào Hộp thư Phê duyệt để xem xét và quyết định
                      </p>
                    </div>
                  </div>
                  <button
                    onClick={() => setActiveTab("approvals")}
                    className="px-3 py-1.5 rounded-xl text-xs font-bold text-white bg-amber-600 hover:bg-amber-500 shadow-sm shrink-0"
                  >
                    Xem ngay
                  </button>
                </div>
              )}

              {/* Recent Queue Mini List */}
              <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm p-5 shadow-sm dark:shadow-xl">
                <div className="flex items-center justify-between mb-3">
                  <h3 className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-2">
                    <Clock className="h-4 w-4 text-amber-500 dark:text-amber-400" />
                    <span>Bài Viết Gần Đây ({posts.length})</span>
                  </h3>
                  <button
                    onClick={() => setActiveTab("history")}
                    className="text-xs font-semibold text-blue-600 dark:text-blue-400 hover:text-blue-500 flex items-center gap-1 transition-colors"
                  >
                    Xem tất cả <ArrowRight className="h-3 w-3" />
                  </button>
                </div>

                {posts.length === 0 ? (
                  <p className="text-xs text-slate-500 py-6 text-center">
                    Hàng đợi trống. Hãy tạo bài viết đầu tiên bên cạnh!
                  </p>
                ) : (
                  <div className="space-y-2.5">
                    {posts.slice(0, 5).map((post) => (
                      <div
                        key={post.id}
                        className="p-3 rounded-xl bg-slate-50 dark:bg-slate-950/60 border border-slate-200 dark:border-slate-800/80 text-xs space-y-1.5 hover:border-slate-300 dark:hover:border-slate-700 transition-colors"
                      >
                        <div className="flex items-center justify-between">
                          <span className="font-bold text-slate-800 dark:text-slate-300 truncate max-w-[150px]">
                            {post.channel_name || "Kênh phát sóng"}
                          </span>
                          <span
                            className={`px-2 py-0.5 rounded-full text-[10px] font-bold ${
                              post.status === "PUBLISHED"
                                ? "bg-emerald-100 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-400"
                                : post.status === "FAILED"
                                ? "bg-rose-100 dark:bg-rose-500/20 text-rose-700 dark:text-rose-400"
                                : post.status === "PENDING_APPROVAL"
                                ? "bg-amber-100 dark:bg-amber-500/20 text-amber-700 dark:text-amber-400"
                                : post.status === "REJECTED"
                                ? "bg-rose-100 dark:bg-rose-500/20 text-rose-700 dark:text-rose-400"
                                : "bg-blue-100 dark:bg-blue-500/20 text-blue-700 dark:text-blue-400"
                            }`}
                          >
                            {post.status}
                          </span>
                        </div>
                        <p className="text-slate-600 dark:text-slate-400 line-clamp-1">{post.content || `[${post.media_type}]`}</p>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        )}

        {/* Tab 2: Hộp Thư Phê Duyệt (Approval Inbox) */}
        {activeTab === "approvals" && <ApprovalInbox />}

        {/* Tab 3: Quản lý Kênh / Fanpage */}
        {activeTab === "pages" && (
          <PageManager channels={channels} loading={loading} onRefresh={fetchAllData} />
        )}

        {/* Tab 4: Đội Ngũ & Cụm Kênh (Team & Channel Groups) */}
        {activeTab === "team" && <TeamAndGroupsManager />}

        {/* Tab 5: Lịch trình & Hàng đợi */}
        {activeTab === "history" && (
          <PostList posts={posts} loading={loading} onRefresh={fetchAllData} />
        )}
      </main>

      {/* Footer */}
      <footer className="border-t border-slate-200 dark:border-slate-800/80 bg-white/50 dark:bg-slate-950/50 py-4 mt-12 text-center text-xs text-slate-500 transition-colors">
        <div className="max-w-7xl mx-auto px-4 flex flex-col sm:flex-row items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <span className="font-semibold text-slate-700 dark:text-slate-400">PostPilot Suite</span>
            <span>•</span>
            <span>DDD Hexagonal Architecture • Golang Backend • Next.js Frontend</span>
          </div>
          <nav aria-label="Thông tin pháp lý" className="flex flex-wrap justify-center gap-x-4 gap-y-2 text-xs text-slate-600 dark:text-slate-400">
            <Link href="/privacy" className="underline-offset-4 hover:text-blue-600 hover:underline dark:hover:text-blue-300">Chính sách quyền riêng tư</Link>
            <Link href="/terms" className="underline-offset-4 hover:text-blue-600 hover:underline dark:hover:text-blue-300">Điều khoản dịch vụ</Link>
            <Link href="/data-deletion" className="underline-offset-4 hover:text-blue-600 hover:underline dark:hover:text-blue-300">Yêu cầu xóa dữ liệu</Link>
          </nav>
        </div>
      </footer>
    </div>
  );
}
