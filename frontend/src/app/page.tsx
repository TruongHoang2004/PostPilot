"use client";

import React, { useState, useEffect, useCallback, useRef } from "react";
import Link from "next/link";
import { AuthGate } from "@/components/AuthGate";
import { Header, NavigationTab } from "@/components/Header";
import { StatsCards } from "@/components/StatsCards";
import { PostComposer } from "@/components/PostComposer";
import { BulkComposer } from "@/components/BulkComposer";
import { ScheduleView } from "@/components/ScheduleView";
import { LibraryManager } from "@/components/LibraryManager";
import { PageManager } from "@/components/PageManager";
import { ApprovalInbox } from "@/components/ApprovalInbox";
import { TeamAndGroupsManager } from "@/components/TeamAndGroupsManager";
import { api } from "@/lib/api";
import { Channel, Post, SystemHealth, SystemStats } from "@/lib/types";
import { useDialog } from "@/context/DialogContext";
import { useOrg } from "@/context/OrgContext";
import { ArrowUpRight } from "lucide-react";

export default function Home() {
  const { currentOrg, currentUser, loading } = useOrg();
  if (loading) return <main className="min-h-screen flex items-center justify-center">Đang xác minh tài khoản…</main>;
  if (!currentOrg || !currentUser) return <AuthGate />;
  return <Studio key={`${currentUser.id}:${currentOrg.id}`} />;
}

function Studio() {
  const { alert, toast } = useDialog();
  const { currentOrg } = useOrg();
  const [activeTab, setActiveTab] = useState<NavigationTab>("history");
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [stats, setStats] = useState<SystemStats | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [upcoming, setUpcoming] = useState<Post[]>([]);
  const [refreshToken, setRefreshToken] = useState(0);
  const [loading, setLoading] = useState<boolean>(true);
  const [dataError, setDataError] = useState<string | null>(null);
  const [autoRefresh, setAutoRefresh] = useState<boolean>(true);

  const requestEpoch = useRef(0);
  useEffect(() => () => { requestEpoch.current++; }, []);

  const fetchAllData = useCallback(async () => {
    if (!currentOrg) { setChannels([]); setUpcoming([]); setStats(null); return; }
    const epoch = ++requestEpoch.current;
    const since = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();
    try {
      const [healthData, statsData, channelsData, postsData] = await Promise.allSettled([
        api.getHealth(),
        api.getStats(),
        api.getChannels(),
        api.queryPosts({ from: since, limit: 500 }),
      ]);

      if (epoch !== requestEpoch.current) return;
      const failed = [statsData, channelsData, postsData].some(result => result.status === "rejected");
      setDataError(failed ? "Không tải được một phần dữ liệu. Thông tin đang hiển thị có thể chưa được cập nhật." : null);
      if (healthData.status === "fulfilled") setHealth(healthData.value); else setHealth(null);
      if (statsData.status === "fulfilled") setStats(statsData.value);
      if (channelsData.status === "fulfilled") setChannels(channelsData.value);
      if (postsData.status === "fulfilled") setUpcoming(postsData.value);
      setRefreshToken(token => token + 1);
    } catch (err) {
      if (epoch === requestEpoch.current) setDataError("Không tải được dữ liệu. Vui lòng thử lại.");
    } finally {
      if (epoch === requestEpoch.current) setLoading(false);
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


  const headings: Record<NavigationTab, { title: string; description: string }> = {
    history: { title: "Lịch đăng của bạn.", description: "Bấm vào một ngày trên lịch để chọn bài soạn sẵn và giờ đăng; theo dõi bài đã lên lịch và đã đăng." },
    library: { title: "Bài soạn sẵn.", description: "Soạn trước nội dung dùng lại được. Bài đặt “xoá sau khi đăng” sẽ chuyển sang mục Đã đăng để bạn quyết định giữ hay xoá." },
    studio: { title: "Một câu chuyện mới.", description: "Soạn nội dung, thêm hình ảnh và chọn thời điểm đăng." },
    bulk: { title: "Cả loạt bài, một lần xếp lịch.", description: "Soạn nhiều bài, chọn Fanpage và để hệ thống phân bổ khung giờ hợp lý." },
    approvals: { title: "Sẵn sàng để duyệt.", description: "Xem nội dung và quyết định trước khi bài được đăng." },
    pages: { title: "Kết nối với độc giả.", description: "Quản lý Fanpage và kiểm tra quyền truy cập Facebook." },
    team: { title: "Cùng nhau làm nội dung.", description: "Quản lý thành viên, nhóm Fanpage và quyền làm việc." },
  };
  return (
    <div className="dispatch-app">
      <Header health={health} pendingApprovalsCount={stats?.pending_approval_posts || 0} loading={loading}
        autoRefresh={autoRefresh} onToggleAutoRefresh={() => setAutoRefresh(!autoRefresh)} onManualRefresh={fetchAllData}
        activeTab={activeTab} onTabChange={setActiveTab} />
      <main className="dispatch-main">
        <p className="dispatch-eyebrow">{currentOrg?.name} / {activeTab === "studio" || activeTab === "bulk" || activeTab === "library" ? "Tạo nội dung" : "Không gian làm việc"}</p>
        <div className="dispatch-page-heading"><div><h1>{headings[activeTab].title}</h1><p>{headings[activeTab].description}</p></div>
          {activeTab !== "studio" && activeTab !== "bulk" && <div className="dispatch-heading-actions">{activeTab !== "library" && <button className="dispatch-button" onClick={() => setActiveTab("library")}>Bài soạn sẵn</button>}<button className="dispatch-button" onClick={() => setActiveTab("bulk")}>Lên lịch hàng loạt</button><button className="dispatch-button primary" onClick={() => setActiveTab("studio")}>Tạo bài viết<ArrowUpRight size={17} /></button></div>}
        </div>
        {dataError && <div className="dispatch-error" role="alert">{dataError}<button onClick={fetchAllData}>Thử lại</button></div>}
        {activeTab === "history" && <><StatsCards stats={stats} onNavigate={setActiveTab} /><ScheduleView channels={channels} refreshToken={refreshToken} onRefresh={fetchAllData} onCompose={() => setActiveTab("studio")} onOpenLibrary={() => setActiveTab("library")} /></>}
        {activeTab === "library" && <LibraryManager channels={channels} posts={upcoming} refreshToken={refreshToken} onChanged={fetchAllData} />}
        <section hidden={activeTab !== "studio"} aria-label="Soạn bài viết"><PostComposer channels={channels} posts={upcoming} onSuccess={fetchAllData} /></section>
        <section hidden={activeTab !== "bulk"} aria-label="Soạn hàng loạt"><BulkComposer channels={channels} onSuccess={() => { fetchAllData(); setActiveTab("history"); }} /></section>
        <div className="dispatch-legacy">
          {activeTab === "approvals" && <ApprovalInbox />}
          {activeTab === "pages" && <PageManager channels={channels} loading={loading} onRefresh={fetchAllData} />}
          {activeTab === "team" && <TeamAndGroupsManager />}
        </div>
      </main>
      <footer className="dispatch-footer"><span>PostPilot / Lên lịch. Kết nối. Chia sẻ.</span><nav aria-label="Thông tin pháp lý"><Link href="/privacy">Quyền riêng tư</Link><Link href="/terms">Điều khoản</Link><Link href="/data-deletion">Xóa dữ liệu</Link></nav></footer>
    </div>
  );
}
