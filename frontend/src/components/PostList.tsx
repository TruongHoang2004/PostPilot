"use client";

import React, { useState } from "react";
import { Post, PostStatus } from "@/lib/types";
import { api } from "@/lib/api";
import { formatDate, getStatusBadgeClass } from "@/lib/utils";
import { PostDetailModal } from "./PostDetailModal";
import {
  Search,
  Filter,
  Play,
  Zap,
  XCircle,
  FileText,
  Video,
  Image as ImageIcon,
  Share2,
  Film,
  ExternalLink,
  Info,
} from "lucide-react";

interface PostListProps {
  posts: Post[];
  loading: boolean;
  onRefresh: () => void;
}

export function PostList({ posts, loading, onRefresh }: PostListProps) {
  const [filterStatus, setFilterStatus] = useState<string>("ALL");
  const [searchQuery, setSearchQuery] = useState<string>("");
  const [selectedPostId, setSelectedPostId] = useState<string | null>(null);
  const [publishingId, setPublishingId] = useState<string | null>(null);

  const statuses: { label: string; value: string }[] = [
    { label: "Tất cả", value: "ALL" },
    { label: "Hẹn giờ", value: "SCHEDULED" },
    { label: "Hàng đợi", value: "QUEUED" },
    { label: "Đang xử lý", value: "PROCESSING" },
    { label: "Đã đăng", value: "PUBLISHED" },
    { label: "Lỗi", value: "FAILED" },
    { label: "Đã hủy", value: "CANCELLED" },
  ];

  const filteredPosts = posts.filter((post) => {
    if (filterStatus !== "ALL" && post.status !== filterStatus) return false;
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      const matchContent = post.content?.toLowerCase().includes(q);
      const matchPage = post.page_name?.toLowerCase().includes(q);
      const matchId = post.id.toLowerCase().includes(q);
      return matchContent || matchPage || matchId;
    }
    return true;
  });

  const handlePublishNow = async (id: string, sync: boolean) => {
    setPublishingId(id);
    try {
      await api.publishPostNow(id, sync);
      onRefresh();
    } catch (err: any) {
      alert("Lỗi khi đăng bài: " + (err.message || "Không xác định"));
    } finally {
      setPublishingId(null);
    }
  };

  const handleCancel = async (id: string) => {
    if (!confirm("Bạn có chắc chắn muốn hủy bài đăng này?")) return;
    try {
      await api.cancelPost(id);
      onRefresh();
    } catch (err: any) {
      alert("Lỗi khi hủy: " + err.message);
    }
  };

  const renderMediaTypeIcon = (type: string) => {
    switch (type) {
      case "VIDEO":
        return <Video className="h-3.5 w-3.5 text-blue-400" />;
      case "REEL":
        return <Film className="h-3.5 w-3.5 text-purple-400" />;
      case "IMAGE":
      case "ALBUM":
        return <ImageIcon className="h-3.5 w-3.5 text-emerald-400" />;
      case "LINK":
        return <Share2 className="h-3.5 w-3.5 text-cyan-400" />;
      default:
        return <FileText className="h-3.5 w-3.5 text-slate-400" />;
    }
  };

  return (
    <div className="space-y-4">
      {/* Filters and Search Bar */}
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 bg-slate-900/60 p-3 rounded-xl border border-slate-800">
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0">
          {statuses.map((s) => (
            <button
              key={s.value}
              onClick={() => setFilterStatus(s.value)}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold whitespace-nowrap transition-all ${
                filterStatus === s.value
                  ? "bg-blue-600 text-white shadow-sm"
                  : "text-slate-400 hover:text-slate-200 hover:bg-slate-800"
              }`}
            >
              {s.label}
              {s.value !== "ALL" && (
                <span className="ml-1.5 opacity-60">
                  ({posts.filter((p) => p.status === s.value).length})
                </span>
              )}
            </button>
          ))}
        </div>

        <div className="relative min-w-[240px]">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-slate-400" />
          <input
            type="text"
            placeholder="Tìm theo nội dung, page..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-9 pr-3 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-blue-500 transition-colors"
          />
        </div>
      </div>

      {/* Posts Table */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/40 backdrop-blur-sm overflow-hidden shadow-xl">
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse text-xs">
            <thead>
              <tr className="border-b border-slate-800 bg-slate-950/60 text-slate-400 uppercase font-semibold tracking-wider text-[11px]">
                <th className="py-3.5 px-4">Fanpage</th>
                <th className="py-3.5 px-3">Loại</th>
                <th className="py-3.5 px-4">Nội dung bài viết</th>
                <th className="py-3.5 px-3">Thời gian hẹn / đăng</th>
                <th className="py-3.5 px-3">Trạng thái</th>
                <th className="py-3.5 px-4 text-right">Thao tác</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60">
              {loading && posts.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-12 text-center text-slate-400">
                    <div className="inline-block h-6 w-6 rounded-full border-2 border-blue-500 border-t-transparent animate-spin mb-2" />
                    <p>Đang tải dữ liệu bài viết...</p>
                  </td>
                </tr>
              ) : filteredPosts.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-12 text-center text-slate-500">
                    <FileText className="h-8 w-8 mx-auto mb-2 text-slate-600" />
                    <p className="font-medium text-slate-400">Chưa có bài viết nào phù hợp bộ lọc.</p>
                    <p className="text-[11px] mt-0.5">Hãy tạo bài đăng mới ở Studio Soạn Thảo!</p>
                  </td>
                </tr>
              ) : (
                filteredPosts.map((post) => {
                  const isProcessing = publishingId === post.id;
                  const canCancel = post.status === "SCHEDULED" || post.status === "FAILED";

                  return (
                    <tr
                      key={post.id}
                      className="hover:bg-slate-800/30 transition-colors group cursor-pointer"
                      onClick={() => setSelectedPostId(post.id)}
                    >
                      {/* Fanpage */}
                      <td className="py-3 px-4 font-semibold text-slate-200 whitespace-nowrap">
                        <div className="flex items-center gap-2">
                          <span className="h-6 w-6 rounded-md bg-blue-600/20 text-blue-400 border border-blue-500/30 flex items-center justify-center text-[10px] font-bold">
                            {(post.page_name || "P").charAt(0).toUpperCase()}
                          </span>
                          <span className="max-w-[140px] truncate" title={post.page_name || post.page_uuid}>
                            {post.page_name || post.page_uuid.slice(0, 8)}
                          </span>
                        </div>
                      </td>

                      {/* Media Type */}
                      <td className="py-3 px-3 whitespace-nowrap">
                        <div className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded bg-slate-800 border border-slate-700/60 text-[10px] font-semibold text-slate-300">
                          {renderMediaTypeIcon(post.media_type)}
                          <span>{post.media_type}</span>
                        </div>
                      </td>

                      {/* Content Preview */}
                      <td className="py-3 px-4 max-w-[280px]">
                        <p className="line-clamp-2 text-slate-300 group-hover:text-white transition-colors">
                          {post.content || <span className="text-slate-500 italic">Không có văn bản</span>}
                        </p>
                        {post.last_error_message && (
                          <p className="text-[10px] text-rose-400 truncate mt-0.5" title={post.last_error_message}>
                            ⚠️ Lỗi: {post.last_error_message}
                          </p>
                        )}
                      </td>

                      {/* Scheduled At */}
                      <td className="py-3 px-3 whitespace-nowrap text-slate-400">
                        {post.published_at ? (
                          <div>
                            <span className="text-emerald-400 font-medium">Đã phát</span>
                            <div className="text-[10px] text-slate-500">{formatDate(post.published_at)}</div>
                          </div>
                        ) : (
                          <div>
                            <span className="text-amber-400 font-medium">{formatDate(post.scheduled_at)}</span>
                            <div className="text-[10px] text-slate-500">{post.queue_tier} Queue</div>
                          </div>
                        )}
                      </td>

                      {/* Status */}
                      <td className="py-3 px-3 whitespace-nowrap">
                        <span className={`px-2.5 py-0.5 rounded-full text-[10px] font-bold ${getStatusBadgeClass(post.status)}`}>
                          {post.status}
                        </span>
                      </td>

                      {/* Actions */}
                      <td className="py-3 px-4 text-right whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                        <div className="flex items-center justify-end gap-1.5">
                          <button
                            onClick={() => setSelectedPostId(post.id)}
                            title="Xem chi tiết & Nhật ký Meta API"
                            className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 border border-transparent hover:border-slate-700 transition-all"
                          >
                            <Info className="h-4 w-4" />
                          </button>

                          {post.status !== "PUBLISHED" && (
                            <button
                              onClick={() => handlePublishNow(post.id, true)}
                              disabled={isProcessing}
                              title="Đăng ngay lập tức (Sync)"
                              className="px-2.5 py-1 rounded-lg text-[11px] font-semibold bg-emerald-600/80 hover:bg-emerald-600 text-white shadow-sm transition-all disabled:opacity-50 flex items-center gap-1"
                            >
                              <Play className="h-3 w-3 fill-current" />
                              <span>{isProcessing ? "Đang..." : "Đăng ngay"}</span>
                            </button>
                          )}

                          {canCancel && (
                            <button
                              onClick={() => handleCancel(post.id)}
                              title="Hủy lịch đăng"
                              className="p-1.5 rounded-lg text-slate-500 hover:text-rose-400 hover:bg-rose-500/10 border border-transparent hover:border-rose-500/20 transition-all"
                            >
                              <XCircle className="h-4 w-4" />
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Post Detail & Logs Modal */}
      {selectedPostId && (
        <PostDetailModal
          postId={selectedPostId}
          onClose={() => setSelectedPostId(null)}
          onRefresh={onRefresh}
        />
      )}
    </div>
  );
}
