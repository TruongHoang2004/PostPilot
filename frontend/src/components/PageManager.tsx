"use client";

import React, { useState } from "react";
import { FacebookPage, VerifyTokenResponse } from "@/lib/types";
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
  ExternalLink,
  Layers,
  Sparkles,
  Lock,
  Cpu,
  Clock,
  Zap,
} from "lucide-react";

interface PageManagerProps {
  pages: FacebookPage[];
  loading: boolean;
  onRefresh: () => void;
}

export function PageManager({ pages, loading, onRefresh }: PageManagerProps) {
  const [activeSubTab, setActiveSubTab] = useState<"list" | "manual" | "exchange">("list");

  // Form thêm thủ công
  const [pageId, setPageId] = useState("");
  const [pageName, setPageName] = useState("");
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

  // Copied indicator
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const handleManualAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!pageId.trim() || !pageName.trim() || !accessToken.trim()) {
      alert("Vui lòng điền đầy đủ Page ID, Tên Fanpage và Access Token!");
      return;
    }
    setAddingManual(true);
    try {
      await api.createPage({
        page_id: pageId.trim(),
        page_name: pageName.trim(),
        category: category.trim() || "Social Media",
        access_token: accessToken.trim(),
      });
      setPageId("");
      setPageName("");
      setCategory("");
      setAccessToken("");
      setActiveSubTab("list");
      onRefresh();
      alert("✅ Đã thêm Fanpage và mã hóa AES-256-GCM Token an toàn!");
    } catch (err: any) {
      alert("Lỗi khi thêm Fanpage: " + err.message);
    } finally {
      setAddingManual(false);
    }
  };

  const handleExchangeToken = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!userToken.trim()) {
      alert("Vui lòng nhập Short-lived User Token từ Facebook Graph API Explorer!");
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
    } catch (err: any) {
      alert("Lỗi khi đổi Token: " + err.message);
    } finally {
      setExchanging(false);
    }
  };

  const handleVerifyToken = async (uuid: string) => {
    setVerifyingId(uuid);
    try {
      const res = await api.verifyPageToken(uuid);
      setVerifyModalData(res);
      onRefresh();
    } catch (err: any) {
      alert("Lỗi khi kiểm tra token: " + err.message);
    } finally {
      setVerifyingId(null);
    }
  };

  const handleOAuthLogin = async () => {
    try {
      const loginUrl = await api.getFBOAuthLoginUrl();
      window.location.href = loginUrl;
    } catch (err: any) {
      alert("Lỗi OAuth Login: " + (err.message || "Chưa cấu hình FB_APP_ID trong file .env"));
    }
  };

  return (
    <div className="space-y-6">
      {/* Sub Tabs Navigation */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-800 pb-3">
        <div className="flex items-center gap-2">
          <button
            onClick={() => setActiveSubTab("list")}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
              activeSubTab === "list"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/60"
            }`}
          >
            📋 Danh sách Fanpage ({pages.length})
          </button>
          <button
            onClick={() => setActiveSubTab("manual")}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
              activeSubTab === "manual"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/60"
            }`}
          >
            ➕ Thêm Fanpage Thủ Công
          </button>
          <button
            onClick={() => setActiveSubTab("exchange")}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
              activeSubTab === "exchange"
                ? "bg-purple-600 text-white shadow-sm"
                : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/60"
            }`}
          >
            ✨ Đổi Permanent Token Tự Động
          </button>
        </div>

        <button
          onClick={handleOAuthLogin}
          className="px-3 py-1.5 rounded-lg text-xs font-semibold bg-slate-900 hover:bg-slate-800 border border-slate-700 text-blue-400 hover:text-blue-300 flex items-center gap-1.5 transition-colors"
        >
          <Lock className="h-3.5 w-3.5" />
          <span>Đăng nhập Meta OAuth</span>
        </button>
      </div>

      {/* Sub Tab: Danh sách Fanpage */}
      {activeSubTab === "list" && (
        <div className="space-y-4">
          {pages.length === 0 ? (
            <div className="p-12 text-center rounded-2xl border border-slate-800 bg-slate-900/40">
              <Key className="h-10 w-10 mx-auto mb-3 text-slate-600" />
              <h3 className="text-base font-bold text-slate-200">Chưa có Fanpage nào được kết nối</h3>
              <p className="text-xs text-slate-400 mt-1 max-w-md mx-auto">
                Hãy thêm Fanpage cùng Token hoặc sử dụng tính năng Đổi Token Vĩnh Viễn để quản lý nhiều Fanpage cùng lúc.
              </p>
              <div className="flex justify-center gap-3 mt-4">
                <button
                  onClick={() => setActiveSubTab("manual")}
                  className="px-4 py-2 rounded-xl text-xs font-semibold bg-blue-600 hover:bg-blue-500 text-white shadow-md shadow-blue-600/20"
                >
                  + Thêm Fanpage ngay
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
              {pages.map((p) => {
                const maxRate = Math.max(
                  p.rate_limit_call_count || 0,
                  p.rate_limit_total_time || 0,
                  p.rate_limit_total_cputime || 0
                );
                const isVerifying = verifyingId === p.id;
                const isActive = p.token_status === "ACTIVE";

                return (
                  <div
                    key={p.id}
                    className="relative rounded-2xl border border-slate-800 bg-slate-900/60 backdrop-blur-sm p-5 hover:border-slate-700 transition-all flex flex-col justify-between shadow-lg"
                  >
                    <div>
                      {/* Page Header */}
                      <div className="flex items-start justify-between gap-2 mb-3">
                        <div className="flex items-center gap-3">
                          <div className="h-10 w-10 rounded-xl bg-gradient-to-tr from-blue-600 to-indigo-600 text-white font-black text-sm flex items-center justify-center shadow-md shadow-blue-500/20 shrink-0">
                            {p.page_name.charAt(0).toUpperCase()}
                          </div>
                          <div>
                            <h4 className="font-bold text-sm text-white line-clamp-1" title={p.page_name}>
                              {p.page_name}
                            </h4>
                            <span className="text-[11px] text-slate-400 block">{p.category || "Fanpage"}</span>
                          </div>
                        </div>

                        <span
                          className={`px-2 py-0.5 rounded-full text-[10px] font-bold border ${
                            isActive
                              ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/30"
                              : "bg-rose-500/10 text-rose-400 border-rose-500/30"
                          }`}
                        >
                          {p.token_status}
                        </span>
                      </div>

                      {/* Info rows */}
                      <div className="space-y-2 text-xs bg-slate-950/50 p-3 rounded-xl border border-slate-800/80 mb-3">
                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">Page ID:</span>
                          <div className="flex items-center gap-1 font-mono text-slate-300">
                            <span>{p.page_id}</span>
                            <button
                              onClick={() => copyToClipboard(p.page_id, p.id)}
                              className="p-1 hover:text-white"
                              title="Sao chép Page ID"
                            >
                              {copiedId === p.id ? (
                                <Check className="h-3 w-3 text-emerald-400" />
                              ) : (
                                <Copy className="h-3 w-3" />
                              )}
                            </button>
                          </div>
                        </div>

                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">AES Vault Token:</span>
                          <code className="font-mono text-slate-400 text-[11px]">
                            {p.masked_token || "EAABsb...OKEN"}
                          </code>
                        </div>
                      </div>

                      {/* Rate Limit Gauge */}
                      <div className="space-y-1 mb-4">
                        <div className="flex items-center justify-between text-[11px]">
                          <span className="text-slate-400 flex items-center gap-1">
                            <Zap className="h-3 w-3 text-amber-400" /> Rate Limit Meta:
                          </span>
                          <span
                            className={`font-bold ${
                              maxRate > 80
                                ? "text-rose-400"
                                : maxRate > 50
                                ? "text-amber-400"
                                : "text-emerald-400"
                            }`}
                          >
                            {maxRate}%
                          </span>
                        </div>
                        <div className="w-full h-1.5 rounded-full bg-slate-800 overflow-hidden">
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
                        <div className="flex justify-between text-[10px] text-slate-500 pt-0.5">
                          <span>Calls: {p.rate_limit_call_count || 0}%</span>
                          <span>CPU: {p.rate_limit_total_cputime || 0}%</span>
                          <span>Time: {p.rate_limit_total_time || 0}%</span>
                        </div>
                      </div>
                    </div>

                    {/* Card Actions */}
                    <div className="pt-2 border-t border-slate-800/80 flex items-center justify-between">
                      <span className="text-[10px] text-slate-500">
                        {p.updated_at ? `Cập nhật ${formatDate(p.updated_at)}` : ""}
                      </span>
                      <button
                        onClick={() => handleVerifyToken(p.id)}
                        disabled={isVerifying}
                        className="px-2.5 py-1 rounded-lg text-xs font-semibold bg-slate-800 hover:bg-slate-700 text-blue-400 hover:text-blue-300 border border-slate-700 transition-all flex items-center gap-1 disabled:opacity-50"
                      >
                        <RefreshCw className={`h-3 w-3 ${isVerifying ? "animate-spin" : ""}`} />
                        <span>{isVerifying ? "Đang kiểm tra..." : "Kiểm tra Token"}</span>
                      </button>
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
        <div className="max-w-xl mx-auto rounded-2xl border border-slate-800 bg-slate-900/60 backdrop-blur-sm p-6 shadow-xl">
          <div className="flex items-center gap-3 mb-4">
            <div className="p-2 rounded-xl bg-blue-600/20 text-blue-400 border border-blue-500/30">
              <Plus className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-white">Thêm Fanpage & Mã Hóa Token</h3>
              <p className="text-xs text-slate-400">Token được mã hóa bằng AES-256-GCM trước khi lưu vào Database</p>
            </div>
          </div>

          <form onSubmit={handleManualAdd} className="space-y-4 text-xs">
            <div>
              <label className="block text-slate-300 font-semibold mb-1">Facebook Page ID *</label>
              <input
                type="text"
                required
                placeholder="Ví dụ: 102938475612345"
                value={pageId}
                onChange={(e) => setPageId(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-xl bg-slate-950 border border-slate-800 text-slate-100 placeholder-slate-500 focus:outline-none focus:border-blue-500"
              />
            </div>

            <div>
              <label className="block text-slate-300 font-semibold mb-1">Tên Fanpage *</label>
              <input
                type="text"
                required
                placeholder="Ví dụ: PostPilot Official Community"
                value={pageName}
                onChange={(e) => setPageName(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-xl bg-slate-950 border border-slate-800 text-slate-100 placeholder-slate-500 focus:outline-none focus:border-blue-500"
              />
            </div>

            <div>
              <label className="block text-slate-300 font-semibold mb-1">Danh mục (Category)</label>
              <input
                type="text"
                placeholder="Ví dụ: Công nghệ & Phần mềm"
                value={category}
                onChange={(e) => setCategory(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-xl bg-slate-950 border border-slate-800 text-slate-100 placeholder-slate-500 focus:outline-none focus:border-blue-500"
              />
            </div>

            <div>
              <label className="block text-slate-300 font-semibold mb-1">Page Access Token *</label>
              <textarea
                required
                rows={3}
                placeholder="Dán Page Access Token (EAABsbCS1iHgBO...)"
                value={accessToken}
                onChange={(e) => setAccessToken(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-xl bg-slate-950 border border-slate-800 text-slate-100 placeholder-slate-500 focus:outline-none focus:border-blue-500 font-mono text-[11px]"
              />
            </div>

            <div className="pt-2 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setActiveSubTab("list")}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
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
        <div className="max-w-xl mx-auto rounded-2xl border border-purple-500/30 bg-purple-950/20 backdrop-blur-sm p-6 shadow-xl">
          <div className="flex items-center gap-3 mb-4">
            <div className="p-2 rounded-xl bg-purple-600/20 text-purple-400 border border-purple-500/30">
              <Sparkles className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-white">Đổi Sang Permanent Page Access Token</h3>
              <p className="text-xs text-slate-400">
                Tự động lấy tất cả các Fanpage bạn đang quản lý kèm Token không bao giờ hết hạn
              </p>
            </div>
          </div>

          <div className="p-3.5 rounded-xl bg-purple-900/20 border border-purple-500/20 text-xs text-purple-200 mb-4 space-y-1">
            <p className="font-semibold">💡 Cách lấy Short-lived User Token trong 10 giây:</p>
            <ol className="list-decimal pl-4 space-y-0.5 text-slate-300 text-[11px]">
              <li>Vào Facebook Graph API Explorer (developers.facebook.com/tools/explorer).</li>
              <li>Chọn Facebook App của bạn và cấp các quyền: <code>pages_manage_posts</code>, <code>pages_read_engagement</code>, <code>pages_show_list</code>.</li>
              <li>Bấm <strong>Generate Access Token</strong> và copy đoạn mã token dán vào ô bên dưới.</li>
            </ol>
          </div>

          <form onSubmit={handleExchangeToken} className="space-y-4 text-xs">
            <div>
              <label className="block text-slate-300 font-semibold mb-1">Short-lived User Token</label>
              <textarea
                required
                rows={3}
                placeholder="Dán token bắt đầu bằng EAABsbCS..."
                value={userToken}
                onChange={(e) => setUserToken(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-xl bg-slate-950 border border-slate-800 text-slate-100 placeholder-slate-500 focus:outline-none focus:border-purple-500 font-mono text-[11px]"
              />
            </div>

            {exchangeResult && (
              <div className="p-3 bg-emerald-500/10 border border-emerald-500/30 rounded-xl text-emerald-300 text-xs">
                {exchangeResult}
              </div>
            )}

            <div className="pt-2 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setActiveSubTab("list")}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
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
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm animate-in fade-in duration-150">
          <div className="relative w-full max-w-md bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl p-6 space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                {verifyModalData.is_valid ? (
                  <ShieldCheck className="h-5 w-5 text-emerald-400" />
                ) : (
                  <ShieldAlert className="h-5 w-5 text-rose-400" />
                )}
                <span className="font-bold text-white text-sm">Kết Quả Kiểm Tra Token</span>
              </div>
              <button
                onClick={() => setVerifyModalData(null)}
                className="text-slate-400 hover:text-white p-1 rounded-lg"
              >
                ✕
              </button>
            </div>

            <div className="space-y-3 text-xs">
              <div className="flex justify-between items-center bg-slate-950 p-2.5 rounded-xl border border-slate-800">
                <span className="text-slate-400">Trạng thái:</span>
                <span
                  className={`font-bold px-2 py-0.5 rounded text-[11px] ${
                    verifyModalData.is_valid
                      ? "bg-emerald-500/20 text-emerald-400"
                      : "bg-rose-500/20 text-rose-400"
                  }`}
                >
                  {verifyModalData.token_status} {verifyModalData.is_valid ? "(HỢP LỆ)" : "(HẾT HẠN)"}
                </span>
              </div>

              {verifyModalData.debug_info ? (
                <div className="space-y-2 bg-slate-950/60 p-3 rounded-xl border border-slate-800/80">
                  <div className="flex justify-between">
                    <span className="text-slate-500">Facebook App:</span>
                    <span className="text-slate-200 font-semibold">{verifyModalData.debug_info.application}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-slate-500">Hạn sử dụng:</span>
                    <span className="text-emerald-400 font-semibold">
                      {verifyModalData.debug_info.expires_at === 0
                        ? "Vĩnh Viễn (Never Expires)"
                        : new Date(verifyModalData.debug_info.expires_at * 1000).toLocaleString("vi-VN")}
                    </span>
                  </div>
                  <div>
                    <span className="text-slate-500 block mb-1">Quyền hạn đã cấp (Scopes):</span>
                    <div className="flex flex-wrap gap-1">
                      {verifyModalData.debug_info.scopes?.map((sc) => (
                        <span key={sc} className="px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20 text-[10px]">
                          {sc}
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
              ) : (
                verifyModalData.error && (
                  <div className="p-3 bg-rose-500/10 border border-rose-500/20 rounded-xl text-rose-300">
                    {verifyModalData.error}
                  </div>
                )
              )}
            </div>

            <button
              onClick={() => setVerifyModalData(null)}
              className="w-full py-2 rounded-xl text-xs font-semibold bg-slate-800 hover:bg-slate-700 text-white transition-colors"
            >
              Đóng
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
