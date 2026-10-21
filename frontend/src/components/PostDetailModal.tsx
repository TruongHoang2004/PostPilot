"use client";

import React, { useEffect, useState } from "react";
import { Post, PublishLog } from "@/lib/types";
import { api } from "@/lib/api";
import { formatDate, formatBytes, getStatusBadgeClass } from "@/lib/utils";
import { X, ExternalLink, Clock, AlertCircle, CheckCircle2, ShieldAlert, Film, Image as ImageIcon } from "lucide-react";
import { useDialog } from "@/context/DialogContext";

interface PostDetailModalProps {
  postId: string | null;
  onClose: () => void;
  onRefresh: () => void;
}

export function PostDetailModal({ postId, onClose, onRefresh }: PostDetailModalProps) {
  const { confirm, alert, toast } = useDialog();
  const [loading, setLoading] = useState(true);
  const [post, setPost] = useState<Post | null>(null);
  const [logs, setLogs] = useState<PublishLog[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [publishing, setPublishing] = useState(false);

  useEffect(() => {
    if (!postId) return;
    setLoading(true);
    setError(null);
    api
      .getPostDetail(postId)
      .then((data) => {
        setPost(data.post);
        setLogs(data.publish_logs || []);
      })
      .catch((err) => {
        setError(err.message || "Không thể tải chi tiết bài viết");
      })
      .finally(() => {
        setLoading(false);
      });
  }, [postId]);

  if (!postId) return null;

  const handlePublishNow = async (sync: boolean) => {
    if (!post) return;
    const ok = await confirm({
      title: "Xác nhận phát sóng ngay",
      message: "Hệ thống sẽ đẩy bài viết này lên Facebook ngay lập tức. Tiếp tục?",
      confirmText: "Đăng ngay",
      variant: "info",
    });
    if (!ok) return;

    setPublishing(true);
    try {
      await api.publishPostNow(post.id, sync);
      const updated = await api.getPostDetail(post.id);
      setPost(updated.post);
      setLogs(updated.publish_logs || []);
      toast("🚀 Đã kích hoạt đăng bài thành công!", "success");
      onRefresh();
    } catch (err: any) {
      alert({
        title: "Lỗi khi đăng bài",
        message: err.message || "Không xác định",
        variant: "danger",
      });
    } finally {
      setPublishing(false);
    }
  };

  const handleCancel = async () => {
    if (!post) return;
    const ok = await confirm({
      title: "Hủy bài viết",
      message: "Bạn có chắc chắn muốn hủy bài viết này không? Trạng thái sẽ chuyển sang CANCELLED.",
      confirmText: "Hủy bài viết",
      variant: "danger",
    });
    if (!ok) return;

    try {
      await api.cancelPost(post.id);
      const updated = await api.getPostDetail(post.id);
      setPost(updated.post);
      toast("Đã hủy bài đăng thành công", "success");
      onRefresh();
    } catch (err: any) {
      alert({
        title: "Lỗi khi hủy bài",
        message: err.message,
        variant: "danger",
      });
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-in fade-in duration-150">
      <div
        className="relative w-full max-w-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950/50">
          <div className="flex items-center gap-3">
            <span className="text-base font-bold text-slate-900 dark:text-white">Chi tiết Bài Viết & Nhật Ký Meta API</span>
            {post && (
              <span className={`text-[11px] font-semibold px-2.5 py-0.5 rounded-full ${getStatusBadgeClass(post.status)}`}>
                {post.status}
              </span>
            )}
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-lg text-slate-400 hover:text-slate-700 dark:hover:text-white hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Content Body */}
        <div className="p-6 overflow-y-auto space-y-6 flex-1 text-slate-800 dark:text-slate-100">
          {loading ? (
            <div className="flex flex-col items-center justify-center py-12 text-slate-400 gap-3">
              <div className="h-8 w-8 rounded-full border-2 border-blue-500 border-t-transparent animate-spin" />
              <p className="text-sm">Đang tải chi tiết bài viết và lịch sử log...</p>
            </div>
          ) : error ? (
            <div className="p-4 rounded-xl bg-rose-50 dark:bg-rose-500/10 border border-rose-200 dark:border-rose-500/30 text-rose-600 dark:text-rose-300 text-sm flex items-center gap-2">
              <ShieldAlert className="h-5 w-5 shrink-0" />
              <span>{error}</span>
            </div>
          ) : post ? (
            <>
              {/* Post Meta Info */}
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 bg-slate-50 dark:bg-slate-950/60 p-4 rounded-xl border border-slate-200 dark:border-slate-800/80 text-xs">
                <div>
                  <span className="text-slate-500 block">Định dạng</span>
                  <span className="font-semibold text-slate-800 dark:text-slate-200 mt-0.5 block">{post.media_type}</span>
                </div>
                <div>
                  <span className="text-slate-500 block">Hàng đợi</span>
                  <span className="font-semibold text-purple-600 dark:text-purple-400 mt-0.5 block">{post.queue_tier} Queue</span>
                </div>
                <div>
                  <span className="text-slate-500 block">Số lần retry</span>
                  <span className="font-semibold text-slate-800 dark:text-slate-200 mt-0.5 block">{post.retry_count} / {post.max_retries}</span>
                </div>
                <div>
                  <span className="text-slate-500 block">Thời gian hẹn</span>
                  <span className="font-semibold text-amber-600 dark:text-amber-400 mt-0.5 block">{formatDate(post.scheduled_at)}</span>
                </div>
              </div>

              {/* Published External Link if exists */}
              {post.external_post_id && (
                <div className="p-3 bg-blue-50 dark:bg-blue-500/10 border border-blue-200 dark:border-blue-500/30 rounded-xl flex items-center justify-between">
                  <div className="flex items-center gap-2 text-sm text-blue-700 dark:text-blue-300">
                    <CheckCircle2 className="h-4 w-4 text-emerald-500 dark:text-emerald-400" />
                    <span>External Post ID: <code className="font-mono font-bold text-slate-900 dark:text-white">{post.external_post_id}</code></span>
                  </div>
                  <a
                    href={`https://facebook.com/${post.external_post_id}`}
                    target="_blank"
                    rel="noreferrer"
                    className="flex items-center gap-1 text-xs font-semibold px-2.5 py-1 bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors"
                  >
                    Xem bài đăng <ExternalLink className="h-3 w-3" />
                  </a>
                </div>
              )}

              {/* Error message banner if any */}
              {post.last_error_message && (
                <div className="p-3 bg-rose-50 dark:bg-rose-500/10 border border-rose-200 dark:border-rose-500/30 rounded-xl text-xs text-rose-700 dark:text-rose-300 flex items-start gap-2">
                  <AlertCircle className="h-4 w-4 text-rose-500 dark:text-rose-400 shrink-0 mt-0.5" />
                  <div>
                    <span className="font-semibold">Mã lỗi Meta: {post.last_error_code || "N/A"}</span>
                    <p className="mt-0.5 text-rose-600 dark:text-rose-200/90">{post.last_error_message}</p>
                  </div>
                </div>
              )}

              {/* Post Caption / Content */}
              <div>
                <label className="text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider block mb-2">
                  Nội dung bài viết
                </label>
                <div className="p-4 rounded-xl bg-slate-50 dark:bg-slate-950/80 border border-slate-200 dark:border-slate-800 text-sm text-slate-800 dark:text-slate-200 whitespace-pre-wrap leading-relaxed max-h-56 overflow-y-auto">
                  {post.content || <span className="text-slate-400 dark:text-slate-500 italic">Không có văn bản</span>}
                </div>
              </div>

              {/* Media Previews */}
              {post.media_urls && post.media_urls.length > 0 && (
                <div>
                  <label className="text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider block mb-2">
                    Media đính kèm ({post.media_urls.length})
                  </label>
                  <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                    {post.media_urls.map((url, i) => {
                      const isVideo = url.match(/\.(mp4|mov|webm|m4v)/i) || post.media_type === "VIDEO" || post.media_type === "REEL";
                      return (
                        <div key={i} className="relative rounded-lg overflow-hidden border border-slate-200 dark:border-slate-800 bg-black aspect-video flex items-center justify-center group">
                          {isVideo ? (
                            <video src={url} controls className="max-h-full max-w-full" />
                          ) : (
                            <img src={url} alt={`Media ${i}`} className="object-cover w-full h-full" />
                          )}
                          <a
                            href={url}
                            target="_blank"
                            rel="noreferrer"
                            className="absolute top-1 right-1 p-1 rounded bg-black/60 text-white opacity-0 group-hover:opacity-100 transition-opacity"
                          >
                            <ExternalLink className="h-3 w-3" />
                          </a>
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}

              {/* Publish Logs History */}
              <div>
                <label className="text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase tracking-wider block mb-2">
                  Lịch sử thực thi Meta Graph API ({logs.length})
                </label>
                {logs.length === 0 ? (
                  <p className="text-xs text-slate-500 p-3 bg-slate-50 dark:bg-slate-950/40 rounded-xl border border-slate-200 dark:border-slate-800/60">
                    Chưa có nhật ký thực thi nào. Bài viết sẽ được Worker tự động xử lý khi đến giờ hẹn hoặc khi bạn nhấn Đăng Ngay.
                  </p>
                ) : (
                  <div className="space-y-2">
                    {logs.map((log) => (
                      <div
                        key={log.id}
                        className="p-3 rounded-xl bg-slate-50 dark:bg-slate-950/60 border border-slate-200 dark:border-slate-800/80 text-xs flex flex-col gap-1.5"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2">
                            <span className="font-bold text-slate-800 dark:text-slate-200">Lần thử #{log.attempt_number}</span>
                            <span
                              className={`px-1.5 py-0.5 rounded text-[10px] font-bold ${
                                log.http_status >= 200 && log.http_status < 300
                                  ? "bg-emerald-100 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-400"
                                  : "bg-rose-100 dark:bg-rose-500/20 text-rose-700 dark:text-rose-400"
                              }`}
                            >
                              HTTP {log.http_status || "ERR"}
                            </span>
                          </div>
                          <div className="flex items-center gap-2 text-slate-400 dark:text-slate-500">
                            <Clock className="h-3 w-3" />
                            <span>{log.duration_ms}ms</span>
                            <span>•</span>
                            <span>{formatDate(log.created_at)}</span>
                          </div>
                        </div>

                        {log.error_code ? (
                          <div className="text-rose-600 dark:text-rose-400 bg-rose-50 dark:bg-rose-500/5 p-2 rounded-lg border border-rose-200 dark:border-rose-500/20 font-mono text-[11px] break-all">
                            {`[Code ${log.error_code}] `}
                            {log.response_body || log.request_summary}
                          </div>
                        ) : log.response_body ? (
                          <div className="text-emerald-600 dark:text-emerald-400 font-mono text-[11px] break-all">
                            {log.action ? `[${log.action}] ` : ""}
                            {log.response_body}
                          </div>
                        ) : null}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </>
          ) : null}
        </div>

        {/* Footer Actions */}
        {post && (
          <div className="px-6 py-4 border-t border-slate-200 dark:border-slate-800 bg-slate-50/90 dark:bg-slate-950/80 flex items-center justify-between gap-3 flex-wrap">
            <div className="flex items-center gap-2">
              {(post.status === "SCHEDULED" || post.status === "FAILED") && (
                <button
                  onClick={handleCancel}
                  className="px-3 py-1.5 rounded-lg text-xs font-semibold bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-rose-600 dark:text-rose-400 border border-slate-200 dark:border-slate-700 hover:border-rose-500/30 transition-all"
                >
                  ✕ Hủy Lịch Đăng
                </button>
              )}
            </div>

            <div className="flex items-center gap-2">
              <button
                onClick={() => handlePublishNow(false)}
                disabled={publishing}
                className="px-3.5 py-1.5 rounded-lg text-xs font-semibold bg-purple-600 hover:bg-purple-500 text-white transition-all disabled:opacity-50"
              >
                {publishing ? "Đang đẩy..." : "⚡ Đẩy vào Queue"}
              </button>
              <button
                onClick={() => handlePublishNow(true)}
                disabled={publishing}
                className="px-3.5 py-1.5 rounded-lg text-xs font-semibold bg-emerald-600 hover:bg-emerald-500 text-white shadow-lg shadow-emerald-600/20 transition-all disabled:opacity-50 flex items-center gap-1.5"
              >
                {publishing ? "Đang xử lý..." : "🚀 Đăng Ngay Đồng Bộ"}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
