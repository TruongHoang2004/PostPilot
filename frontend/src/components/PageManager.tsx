"use client";

import React, { useState } from "react";
import { Channel, VerifyTokenResponse } from "@/lib/types";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import {
  ShieldCheck,
  ShieldAlert,
  Key,
  Copy,
  Check,
  Plus,
  RefreshCw,
  Sparkles,
  Zap,
  Trash2,
} from "lucide-react";
import { useDialog } from "@/context/DialogContext";

interface PageManagerProps {
  channels: Channel[];
  loading: boolean;
  onRefresh: () => void;
}

export function PageManager({ channels, loading, onRefresh }: PageManagerProps) {
  const { confirm, alert, toast } = useDialog();
  const [activeSubTab, setActiveSubTab] = useState<"list" | "manual" | "exchange">("list");

  // Form thêm thủ công
  const [externalId, setExternalId] = useState("");
  const [channelName, setChannelName] = useState("");
  const [category, setCategory] = useState("");
  const [accessToken, setAccessToken] = useState("");
  const [addingManual, setAddingManual] = useState(false);

  // Form exchange token
  const [userToken, setUserToken] = useState("");
  const [exchanging, setExchanging] = useState(false);
  const [exchangeResult, setExchangeResult] = useState<string | null>(null);

  // Verify token state
  const [verifyingId, setVerifyingId] = useState<string | null>(null);
  const [verifyModalData, setVerifyModalData] = useState<VerifyTokenResponse | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  // Copied indicator
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const handleDeleteChannel = async (ch: Channel) => {
    const isConfirmed = await confirm({
      title: "Xác nhận xóa Kênh phát sóng",
      message: `Bạn có chắc chắn muốn ngắt kết nối và xóa kênh "${ch.name}" (ID: ${ch.external_id}) khỏi hệ thống? Tất cả bài viết và lịch hẹn liên quan sẽ bị xóa.`,
      confirmText: "Xóa Kênh",
      cancelText: "Hủy bỏ",
      variant: "danger",
    });

    if (!isConfirmed) return;

    setDeletingId(ch.id);
    try {
      await api.deleteChannel(ch.id);
      toast(`✅ Đã xóa thành công kênh "${ch.name}"`, "success");
      onRefresh();
    } catch (err: any) {
      alert({
        title: "Lỗi khi xóa Kênh",
        message: err.message || "Không thể xóa Kênh. Vui lòng thử lại sau.",
        variant: "danger",
      });
    } finally {
      setDeletingId(null);
    }
  };

  const handleManualAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!externalId.trim() || !channelName.trim() || !accessToken.trim()) {
      alert({
        title: "Thiếu thông tin",
        message: "Vui lòng điền đầy đủ External ID (Page ID), Tên Kênh và Access Token!",
        variant: "warning",
      });
      return;
    }
    setAddingManual(true);
    try {
      await api.createChannel({
        platform: "FACEBOOK",
        external_id: externalId.trim(),
        name: channelName.trim(),
        category: category.trim() || "Social Media",
        access_token: accessToken.trim(),
      });
      setExternalId("");
      setChannelName("");
      setCategory("");
      setAccessToken("");
      setActiveSubTab("list");
      onRefresh();
      toast("✅ Đã lưu Kênh và mã hóa AES-256-GCM an toàn!", "success");
    } catch (err: any) {
      alert({
        title: "Lỗi thêm Kênh",
        message: err.message,
        variant: "danger",
      });
    } finally {
      setAddingManual(false);
    }
  };

  const handleExchangeToken = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!userToken.trim()) {
      alert({
        title: "Thiếu User Token",
        message: "Vui lòng nhập Short-lived User Token từ Facebook Graph API Explorer!",
        variant: "warning",
      });
      return;
    }
    setExchanging(true);
    setExchangeResult(null);
    try {
      const res = await api.exchangeUserToken(userToken.trim());
      setUserToken("");
      setExchangeResult(
        `🎉 Thành công! Đã tự động đổi và đồng bộ ${res.imported_count} Fanpage với Permanent Token không bao giờ hết hạn.`
      );
      onRefresh();
      toast("🎉 Đã đổi và lưu Token vĩnh viễn thành công!", "success");
    } catch (err: any) {
      alert({
        title: "Lỗi khi đổi Token",
        message: err.message,
        variant: "danger",
      });
    } finally {
      setExchanging(false);
    }
  };

  const handleVerifyToken = async (channelID: string) => {
    setVerifyingId(channelID);
    try {
      const res = await api.verifyChannelToken(channelID);
      setVerifyModalData(res);
      onRefresh();
    } catch (err: any) {
      alert({
        title: "Lỗi kiểm tra token",
        message: err.message,
        variant: "danger",
      });
    } finally {
      setVerifyingId(null);
    }
  };

  const handleOAuthLogin = async () => {
    try {
      const loginUrl = await api.getFBOAuthLoginUrl();
      window.location.href = loginUrl;
    } catch (err: any) {
      alert({
        title: "Lỗi OAuth Login",
        message: err.message || "Chưa cấu hình FB_APP_ID trong file .env",
        variant: "danger",
      });
    }
  };

  return (
    <div className="space-y-6">
      {/* Facebook OAuth 1-Click Connection Hero Banner */}
      <div className="relative overflow-hidden rounded-2xl border border-blue-500/30 bg-gradient-to-br from-blue-950/60 via-slate-900/80 to-indigo-950/50 p-6 shadow-2xl backdrop-blur-xl">
        <div className="absolute -right-10 -bottom-10 w-44 h-44 rounded-full bg-blue-600/15 blur-3xl pointer-events-none" />
        <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-6 relative z-10">
          <div className="max-w-xl">
            <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-blue-500/20 border border-blue-500/30 text-blue-300 text-xs font-semibold mb-3">
              <Sparkles className="w-3.5 h-3.5 text-blue-400" />
              <span>Kết Nối Tự Động 1-Chạm (Recommended)</span>
            </div>
            <h2 className="text-xl font-bold text-white tracking-tight">
              Đăng nhập Facebook để tự động kết nối tất cả Fanpage
            </h2>
            <p className="text-xs sm:text-sm text-slate-300 mt-1.5 leading-relaxed">
              Bạn không cần phải tìm Page ID hay tự copy Access Token phức tạp. Chỉ cần đăng nhập tài khoản Facebook của bạn, hệ thống sẽ tự động quét danh sách Fanpage bạn quản lý, tạo <strong>Token vĩnh viễn không bao giờ hết hạn</strong> và hiển thị sẵn sàng để bạn đăng bài ngay!
            </p>
          </div>

          <button
            type="button"
            onClick={handleOAuthLogin}
            className="shrink-0 px-6 py-3.5 rounded-xl font-bold text-white bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 shadow-xl shadow-blue-600/30 border border-blue-400/40 flex items-center gap-3 transition-all transform hover:scale-[1.03] active:scale-[0.98] group"
          >
            <div className="w-6 h-6 rounded-lg bg-white/20 flex items-center justify-center p-1 group-hover:scale-110 transition-transform">
              <svg className="w-full h-full fill-white" viewBox="0 0 24 24">
                <path d="M24 12.073c0-6.627-5.373-12-12-12s-12 5.373-12 12c0 5.99 4.388 10.954 10.125 11.854v-8.385H7.078v-3.47h3.047V9.43c0-3.007 1.792-4.669 4.533-4.669 1.312 0 2.686.235 2.686.235v2.953H15.83c-1.491 0-1.956.925-1.956 1.874v2.25h3.328l-.532 3.47h-2.796v8.385C19.612 23.027 24 18.062 24 12.073z" />
              </svg>
            </div>
            <span className="text-sm tracking-wide">Đăng nhập bằng Facebook</span>
          </button>
        </div>
      </div>

      {/* Sub Tabs Navigation */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 dark:border-slate-800 pb-3">
        <div className="flex items-center gap-2">
          <button
            onClick={() => setActiveSubTab("list")}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
              activeSubTab === "list"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800/60"
            }`}
          >
            📋 Danh sách Kênh ({channels.length})
          </button>
          <button
            onClick={() => setActiveSubTab("manual")}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
              activeSubTab === "manual"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800/60"
            }`}
          >
            ➕ Thêm Kênh Thủ Công
          </button>
          <button
            onClick={() => setActiveSubTab("exchange")}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
              activeSubTab === "exchange"
                ? "bg-purple-600 text-white shadow-sm"
                : "text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800/60"
            }`}
          >
            ✨ Đổi Permanent Token Tự Động
          </button>
        </div>
      </div>

      {/* Sub Tab: Danh sách Kênh */}
      {activeSubTab === "list" && (
        <div className="space-y-4">
          {channels.length === 0 ? (
            <div className="p-12 text-center rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/70 dark:bg-slate-900/40 shadow-sm">
              <Key className="h-10 w-10 mx-auto mb-3 text-slate-400 dark:text-slate-600" />
              <h3 className="text-base font-bold text-slate-800 dark:text-slate-200">Chưa có Kênh phát sóng nào được kết nối</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 max-w-md mx-auto">
                Hãy thêm Fanpage cùng Token hoặc sử dụng tính năng Đổi Token Vĩnh Viễn để quản lý nhiều kênh cùng lúc.
              </p>
              <div className="flex justify-center gap-3 mt-4">
                <button
                  onClick={() => setActiveSubTab("manual")}
                  className="px-4 py-2 rounded-xl text-xs font-semibold bg-blue-600 hover:bg-blue-500 text-white shadow-md shadow-blue-600/20"
                >
                  + Thêm Kênh ngay
                </button>
                <button
                  onClick={() => setActiveSubTab("exchange")}
                  className="px-4 py-2 rounded-xl text-xs font-semibold bg-purple-600 hover:bg-purple-500 text-white"
                >
                  ✨ Đổi Token tự động
                </button>
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {channels.map((ch) => {
                const maxRate = Math.max(
                  ch.rate_limit_call_count || 0,
                  ch.rate_limit_total_time || 0,
                  ch.rate_limit_total_cputime || 0
                );
                const isVerifying = verifyingId === ch.id;
                const isActive = ch.token_status === "ACTIVE";

                return (
                  <div
                    key={ch.id}
                    className="relative rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm p-5 hover:border-slate-300 dark:hover:border-slate-700 transition-all flex flex-col justify-between shadow-sm dark:shadow-lg"
                  >
                    <div>
                      {/* Channel Header */}
                      <div className="flex items-start justify-between gap-2 mb-3">
                        <div className="flex items-center gap-3">
                          <div className="h-10 w-10 rounded-xl bg-gradient-to-tr from-blue-600 to-indigo-600 text-white font-black text-sm flex items-center justify-center shadow-md shadow-blue-500/20 shrink-0">
                            {ch.name.charAt(0).toUpperCase()}
                          </div>
                          <div>
                            <h4 className="font-bold text-sm text-slate-900 dark:text-white line-clamp-1" title={ch.name}>
                              {ch.name}
                            </h4>
                            <span className="text-[11px] text-slate-500 dark:text-slate-400 block">
                              {ch.platform || "FACEBOOK"} • {ch.category || "Fanpage"}
                            </span>
                          </div>
                        </div>

                        <span
                          className={`px-2 py-0.5 rounded-full text-[10px] font-bold border ${
                            isActive
                              ? "bg-emerald-50 dark:bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-200 dark:border-emerald-500/30"
                              : "bg-rose-50 dark:bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-200 dark:border-rose-500/30"
                          }`}
                        >
                          {ch.token_status}
                        </span>
                      </div>

                      {/* Info rows */}
                      <div className="space-y-2 text-xs bg-slate-50 dark:bg-slate-950/50 p-3 rounded-xl border border-slate-200 dark:border-slate-800/80 mb-3">
                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">External ID:</span>
                          <div className="flex items-center gap-1 font-mono text-slate-800 dark:text-slate-300">
                            <span>{ch.external_id}</span>
                            <button
                              onClick={() => copyToClipboard(ch.external_id, ch.id)}
                              className="p-1 hover:text-blue-600 dark:hover:text-white transition-colors"
                              title="Sao chép External ID"
                            >
                              {copiedId === ch.id ? (
                                <Check className="h-3 w-3 text-emerald-500 dark:text-emerald-400" />
                              ) : (
                                <Copy className="h-3 w-3" />
                              )}
                            </button>
                          </div>
                        </div>

                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">AES Vault Token:</span>
                          <code className="font-mono text-slate-600 dark:text-slate-400 text-[11px]">
                            {ch.masked_token || "EAABsb...OKEN"}
                          </code>
                        </div>
                      </div>

                      {/* Rate Limit Gauge */}
                      <div className="space-y-1 mb-4">
                        <div className="flex items-center justify-between text-[11px]">
                          <span className="text-slate-600 dark:text-slate-400 flex items-center gap-1">
                            <Zap className="h-3 w-3 text-amber-500 dark:text-amber-400" /> Rate Limit Quota:
                          </span>
                          <span
                            className={`font-bold ${
                              maxRate > 80
                                ? "text-rose-600 dark:text-rose-400"
                                : maxRate > 50
                                ? "text-amber-600 dark:text-amber-400"
                                : "text-emerald-600 dark:text-emerald-400"
                            }`}
                          >
                            {maxRate}%
                          </span>
                        </div>
                        <div className="w-full h-1.5 rounded-full bg-slate-200 dark:bg-slate-800 overflow-hidden">
                          <div
                            className={`h-full transition-all duration-300 ${
                              maxRate > 80
                                ? "bg-rose-500"
                                : maxRate > 50
                                ? "bg-amber-500"
                                : "bg-emerald-500"
                            }`}
                            style={{ width: `${Math.min(maxRate, 100)}%` }}
                          />
                        </div>
                        <div className="flex justify-between text-[10px] text-slate-400 dark:text-slate-500 pt-0.5">
                          <span>Calls: {ch.rate_limit_call_count || 0}%</span>
                          <span>CPU: {ch.rate_limit_total_cputime || 0}%</span>
                          <span>Time: {ch.rate_limit_total_time || 0}%</span>
                        </div>
                      </div>
                    </div>

                    {/* Card Actions */}
                    <div className="pt-2 border-t border-slate-200 dark:border-slate-800/80 flex items-center justify-between gap-2">
                      <span className="text-[10px] text-slate-400 dark:text-slate-500 truncate">
                        {ch.updated_at ? `Cập nhật ${formatDate(ch.updated_at)}` : ""}
                      </span>
                      <div className="flex items-center gap-1.5 shrink-0">
                        <button
                          onClick={() => handleVerifyToken(ch.id)}
                          disabled={isVerifying || deletingId === ch.id}
                          className="px-2.5 py-1 rounded-lg text-xs font-semibold bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 border border-slate-200 dark:border-slate-700 transition-all flex items-center gap-1 disabled:opacity-50"
                          title="Kiểm tra trạng thái & hạn dùng của Token"
                        >
                          <RefreshCw className={`h-3 w-3 ${isVerifying ? "animate-spin" : ""}`} />
                          <span>{isVerifying ? "Đang kiểm tra..." : "Kiểm tra"}</span>
                        </button>
                        <button
                          onClick={() => handleDeleteChannel(ch)}
                          disabled={deletingId === ch.id || isVerifying}
                          className="p-1.5 rounded-lg text-xs font-semibold bg-rose-50 hover:bg-rose-100 dark:bg-rose-500/10 dark:hover:bg-rose-500/20 text-rose-600 dark:text-rose-400 border border-rose-200 dark:border-rose-500/30 transition-all flex items-center justify-center disabled:opacity-50"
                          title="Xóa Kênh khỏi hệ thống"
                        >
                          <Trash2 className={`h-3.5 w-3.5 ${deletingId === ch.id ? "animate-pulse" : ""}`} />
                        </button>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* Sub Tab: Thêm thủ công */}
      {activeSubTab === "manual" && (
        <div className="max-w-xl mx-auto rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm p-6 shadow-sm dark:shadow-xl">
          <div className="flex items-center gap-3 mb-4">
            <div className="p-2 rounded-xl bg-blue-100 dark:bg-blue-600/20 text-blue-600 dark:text-blue-400 border border-blue-200 dark:border-blue-500/30">
              <Plus className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-slate-900 dark:text-white">Thêm Kênh & Mã Hóa Token</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400">Token được mã hóa bằng AES-256-GCM trước khi lưu vào Database</p>
            </div>
          </div>

          <form onSubmit={handleManualAdd} className="space-y-4 text-xs">
            <div>
              <label className="block text-slate-700 dark:text-slate-300 font-semibold mb-1">External ID (Facebook Page ID) *</label>
              <input
                type="text"
                required
                placeholder="Ví dụ: 102938475612345"
                value={externalId}
                onChange={(e) => setExternalId(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-slate-900 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-blue-500"
              />
            </div>

            <div>
              <label className="block text-slate-700 dark:text-slate-300 font-semibold mb-1">Tên Kênh / Fanpage *</label>
              <input
                type="text"
                required
                placeholder="Ví dụ: PostPilot Official Community"
                value={channelName}
                onChange={(e) => setChannelName(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-slate-900 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-blue-500"
              />
            </div>

            <div>
              <label className="block text-slate-700 dark:text-slate-300 font-semibold mb-1">Danh mục (Category)</label>
              <input
                type="text"
                placeholder="Ví dụ: Công nghệ & Phần mềm"
                value={category}
                onChange={(e) => setCategory(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-slate-900 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-blue-500"
              />
            </div>

            <div>
              <label className="block text-slate-700 dark:text-slate-300 font-semibold mb-1">Access Token *</label>
              <textarea
                required
                rows={3}
                placeholder="Dán Page Access Token (EAABsbCS1iHgBO...)"
                value={accessToken}
                onChange={(e) => setAccessToken(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-slate-900 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-blue-500 font-mono text-[11px]"
              />
            </div>

            <div className="pt-2 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setActiveSubTab("list")}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 transition-colors"
              >
                Hủy
              </button>
              <button
                type="submit"
                disabled={addingManual}
                className="px-5 py-2 rounded-xl text-xs font-semibold bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-600/30 transition-all disabled:opacity-50"
              >
                {addingManual ? "Đang mã hóa & lưu..." : "Lưu & Mã Hóa Token"}
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Sub Tab: Tự động đổi Permanent Page Token */}
      {activeSubTab === "exchange" && (
        <div className="max-w-xl mx-auto rounded-2xl border border-purple-200 dark:border-purple-500/30 bg-purple-50/50 dark:bg-purple-950/20 backdrop-blur-sm p-6 shadow-sm dark:shadow-xl">
          <div className="flex items-center gap-3 mb-4">
            <div className="p-2 rounded-xl bg-purple-100 dark:bg-purple-600/20 text-purple-600 dark:text-purple-400 border border-purple-200 dark:border-purple-500/30">
              <Sparkles className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-slate-900 dark:text-white">Đổi Sang Permanent Page Access Token</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400">
                Tự động lấy tất cả các Fanpage bạn đang quản lý kèm Token không bao giờ hết hạn
              </p>
            </div>
          </div>

          <div className="p-3.5 rounded-xl bg-purple-100/60 dark:bg-purple-900/20 border border-purple-200 dark:border-purple-500/20 text-xs text-purple-900 dark:text-purple-200 mb-4 space-y-1">
            <p className="font-semibold">💡 Cách lấy Short-lived User Token trong 10 giây:</p>
            <ol className="list-decimal pl-4 space-y-0.5 text-slate-600 dark:text-slate-300 text-[11px]">
              <li>Vào Facebook Graph API Explorer (developers.facebook.com/tools/explorer).</li>
              <li>Chọn Facebook App của bạn và cấp các quyền: <code>pages_manage_posts</code>, <code>pages_read_engagement</code>, <code>pages_show_list</code>.</li>
              <li>Bấm <strong>Generate Access Token</strong> và copy đoạn mã token dán vào ô bên dưới.</li>
            </ol>
          </div>

          <form onSubmit={handleExchangeToken} className="space-y-4 text-xs">
            <div>
              <label className="block text-slate-700 dark:text-slate-300 font-semibold mb-1">Short-lived User Token</label>
              <textarea
                required
                rows={3}
                placeholder="Dán token bắt đầu bằng EAABsbCS..."
                value={userToken}
                onChange={(e) => setUserToken(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-xl bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-slate-900 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-purple-500 font-mono text-[11px]"
              />
            </div>

            {exchangeResult && (
              <div className="p-3 bg-emerald-50 dark:bg-emerald-500/10 border border-emerald-200 dark:border-emerald-500/30 rounded-xl text-emerald-700 dark:text-emerald-300 text-xs">
                {exchangeResult}
              </div>
            )}

            <div className="pt-2 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setActiveSubTab("list")}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 transition-colors"
              >
                Đóng
              </button>
              <button
                type="submit"
                disabled={exchanging}
                className="px-5 py-2 rounded-xl text-xs font-semibold bg-purple-600 hover:bg-purple-500 text-white shadow-lg shadow-purple-600/30 transition-all disabled:opacity-50 flex items-center gap-1.5"
              >
                <Sparkles className="h-4 w-4" />
                <span>{exchanging ? "Đang đồng bộ..." : "Tự Động Kéo Tất Cả Fanpage"}</span>
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Verify Token Modal */}
      {verifyModalData && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-in fade-in duration-150">
          <div className="relative w-full max-w-md bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-2xl shadow-2xl p-6 space-y-4">
            <div className="flex items-center justify-between border-b border-slate-200 dark:border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                {verifyModalData.is_valid ? (
                  <ShieldCheck className="h-5 w-5 text-emerald-500 dark:text-emerald-400" />
                ) : (
                  <ShieldAlert className="h-5 w-5 text-rose-500 dark:text-rose-400" />
                )}
                <span className="font-bold text-slate-900 dark:text-white text-sm">Kết Quả Kiểm Tra Token</span>
              </div>
              <button
                onClick={() => setVerifyModalData(null)}
                className="text-slate-400 hover:text-slate-700 dark:hover:text-white p-1 rounded-lg"
              >
                ✕
              </button>
            </div>

            <div className="space-y-3 text-xs">
              <div className="flex justify-between items-center bg-slate-50 dark:bg-slate-950 p-2.5 rounded-xl border border-slate-200 dark:border-slate-800">
                <span className="text-slate-500 dark:text-slate-400">Trạng thái:</span>
                <span
                  className={`font-bold px-2 py-0.5 rounded text-[11px] ${
                    verifyModalData.is_valid
                      ? "bg-emerald-50 dark:bg-emerald-500/20 text-emerald-600 dark:text-emerald-400"
                      : "bg-rose-50 dark:bg-rose-500/20 text-rose-600 dark:text-rose-400"
                  }`}
                >
                  {verifyModalData.token_status} {verifyModalData.is_valid ? "(HỢP LỆ)" : "(HẾT HẠN)"}
                </span>
              </div>

              {verifyModalData.debug_info ? (
                <div className="space-y-2 bg-slate-50 dark:bg-slate-950/60 p-3 rounded-xl border border-slate-200 dark:border-slate-800/80">
                  <div className="flex justify-between">
                    <span className="text-slate-500">App ID / Application:</span>
                    <span className="text-slate-800 dark:text-slate-200 font-semibold">{verifyModalData.debug_info.application || verifyModalData.debug_info.app_id}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-slate-500">Hạn sử dụng:</span>
                    <span className="text-emerald-600 dark:text-emerald-400 font-semibold">
                      {verifyModalData.debug_info.expires_at === 0
                        ? "Vĩnh Viễn (Never Expires)"
                        : verifyModalData.debug_info.expires_at
                        ? new Date(verifyModalData.debug_info.expires_at * 1000).toLocaleString("vi-VN")
                        : "Không xác định"}
                    </span>
                  </div>
                  <div>
                    <span className="text-slate-500 block mb-1">Quyền hạn đã cấp (Scopes):</span>
                    <div className="flex flex-wrap gap-1">
                      {verifyModalData.debug_info.scopes?.map((sc) => (
                        <span key={sc} className="px-1.5 py-0.5 rounded bg-blue-50 dark:bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-200 dark:border-blue-500/20 text-[10px]">
                          {sc}
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
              ) : (
                verifyModalData.error && (
                  <div className="p-3 bg-rose-50 dark:bg-rose-500/10 border border-rose-200 dark:border-rose-500/20 rounded-xl text-rose-600 dark:text-rose-300">
                    {verifyModalData.error}
                  </div>
                )
              )}
            </div>

            <button
              onClick={() => setVerifyModalData(null)}
              className="w-full py-2 rounded-xl text-xs font-semibold bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-800 dark:text-white transition-colors"
            >
              Đóng
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
