"use client";

import React, { useState, useEffect, useCallback } from "react";
import { Post, ApprovalAction } from "@/lib/types";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { useDialog } from "@/context/DialogContext";
import {
  CheckCircle2,
  XCircle,
  Clock,
  AlertCircle,
  RefreshCw,
  Layers,
  MessageSquare,
  Send,
  Calendar,
  Flag,
  FileText,
  Image as ImageIcon,
  Film,
  Link as LinkIcon,
  ShieldCheck,
  Edit3,
} from "lucide-react";

type FilterTab = "ALL" | "PENDING_APPROVAL" | "REJECTED" | "APPROVED";

export function ApprovalInbox() {
  const { canApprovePost, currentRole, currentOrg } = useOrg();
  const { toast } = useDialog();

  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [activeTab, setActiveTab] = useState<FilterTab>("PENDING_APPROVAL");

  // Modal Review State
  const [reviewModalPost, setReviewModalPost] = useState<Post | null>(null);
  const [reviewAction, setReviewAction] = useState<ApprovalAction>("APPROVED");
  const [reviewNote, setReviewNote] = useState<string>("");
  const [applyToBatch, setApplyToBatch] = useState<boolean>(true);
  const [isSubmittingReview, setIsSubmittingReview] = useState<boolean>(false);

  // Modal Resubmit State
  const [resubmitModalPost, setResubmitModalPost] = useState<Post | null>(null);
  const [resubmitContent, setResubmitContent] = useState<string>("");
  const [resubmitScheduledAt, setResubmitScheduledAt] = useState<string>("");
  const [isSubmittingResubmit, setIsSubmittingResubmit] = useState<boolean>(false);

  const fetchPosts = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.getPosts("ALL", 100);
      setPosts(data);
    } catch (err) {
      console.error("Lỗi khi tải danh sách bài duyệt:", err);
      toast("Không thể tải danh sách bài phê duyệt", "danger");
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    fetchPosts();
  }, [fetchPosts, currentOrg]);

  // Counts
  const pendingCount = posts.filter((p) => p.status === "PENDING_APPROVAL").length;
  const rejectedCount = posts.filter((p) => p.status === "REJECTED").length;
  const approvedCount = posts.filter(
    (p) => p.status === "SCHEDULED" || p.status === "QUEUED" || p.status === "PUBLISHED"
  ).length;

  // Filtered list
  const filteredPosts = posts.filter((p) => {
    if (activeTab === "PENDING_APPROVAL") return p.status === "PENDING_APPROVAL";
    if (activeTab === "REJECTED") return p.status === "REJECTED";
    if (activeTab === "APPROVED") {
      return (
        p.status === "SCHEDULED" ||
        p.status === "QUEUED" ||
        p.status === "PUBLISHED" ||
        (p.reviewed_by && p.status !== "REJECTED" && p.status !== "PENDING_APPROVAL")
      );
    }
    return true;
  });

  // Open Review Dialog
  const openReviewModal = (post: Post, action: ApprovalAction) => {
    setReviewModalPost(post);
    setReviewAction(action);
    setReviewNote("");
    setApplyToBatch(Boolean(post.batch_id));
  };

  // Submit Review
  const handleConfirmReview = async () => {
    if (!reviewModalPost) return;
    setIsSubmittingReview(true);
    try {
      await api.reviewPost(reviewModalPost.id, {
        action: reviewAction,
        note: reviewNote.trim(),
        apply_to_batch: applyToBatch,
      });

      toast(
        reviewAction === "APPROVED"
          ? "✅ Đã phê duyệt bài đăng thành công!"
          : "⚠️ Đã gửi yêu cầu chỉnh sửa bài viết!",
        reviewAction === "APPROVED" ? "success" : "warning"
      );
      setReviewModalPost(null);
      fetchPosts();
    } catch (err: any) {
      toast(err.message || "Lỗi xử lý kiểm duyệt", "danger");
    } finally {
      setIsSubmittingReview(false);
    }
  };

  // Open Resubmit Dialog
  const openResubmitModal = (post: Post) => {
    setResubmitModalPost(post);
    setResubmitContent(post.content);
    setResubmitScheduledAt(
      post.scheduled_at ? new Date(post.scheduled_at).toISOString().slice(0, 16) : ""
    );
  };

  // Submit Resubmit
  const handleConfirmResubmit = async () => {
    if (!resubmitModalPost) return;
    if (!resubmitContent.trim()) {
      toast("Nội dung bài viết không được để trống", "warning");
      return;
    }
    setIsSubmittingResubmit(true);
    try {
      await api.submitPostForApproval(resubmitModalPost.id, {
        content: resubmitContent.trim(),
        scheduled_at: resubmitScheduledAt ? new Date(resubmitScheduledAt).toISOString() : "",
      });

      toast("🚀 Đã gửi bài viết chờ duyệt lại thành công!", "success");
      setResubmitModalPost(null);
      fetchPosts();
    } catch (err: any) {
      toast(err.message || "Lỗi gửi duyệt lại bài viết", "danger");
    } finally {
      setIsSubmittingResubmit(false);
    }
  };

  const renderMediaIcon = (type: string) => {
    switch (type) {
      case "VIDEO":
        return <Film className="h-3.5 w-3.5" />;
      case "REEL":
        return <Film className="h-3.5 w-3.5 text-pink-500" />;
      case "IMAGE":
      case "ALBUM":
        return <ImageIcon className="h-3.5 w-3.5 text-blue-500" />;
      case "LINK":
        return <LinkIcon className="h-3.5 w-3.5 text-emerald-500" />;
      default:
        return <FileText className="h-3.5 w-3.5 text-slate-400" />;
    }
  };

  return (
    <div className="space-y-6">
      {/* Top Banner & Info */}
      <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm p-5 sm:p-6 shadow-sm dark:shadow-xl space-y-4">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2.5">
              <span className="p-2 rounded-xl bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-500/20">
                <ShieldCheck className="h-5 w-5" />
              </span>
              <div>
                <h2 className="text-base font-bold text-slate-900 dark:text-white">
                  Hộp Thư Phê Duyệt Bài Đăng (Approval Inbox)
                </h2>
                <p className="text-xs text-slate-500 dark:text-slate-400">
                  Kiểm duyệt nội dung, yêu cầu chỉnh sửa và bảo vệ chất lượng phát sóng đa kênh.
                </p>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <span
              className={`px-3 py-1 rounded-full text-xs font-semibold border flex items-center gap-1.5 ${
                canApprovePost
                  ? "bg-emerald-50 dark:bg-emerald-500/10 border-emerald-500/30 text-emerald-700 dark:text-emerald-400"
                  : "bg-blue-50 dark:bg-blue-500/10 border-blue-500/30 text-blue-700 dark:text-blue-400"
              }`}
            >
              <span
                className={`h-2 w-2 rounded-full ${
                  canApprovePost ? "bg-emerald-500" : "bg-blue-500"
                }`}
              />
              Quyền hiện tại: {currentRole}
            </span>

            <button
              onClick={fetchPosts}
              disabled={loading}
              className="p-2 rounded-xl bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 transition-all disabled:opacity-50"
              title="Làm mới danh sách"
            >
              <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin text-blue-500" : ""}`} />
            </button>
          </div>
        </div>

        {/* Tab Filters */}
        <div className="flex flex-wrap items-center gap-2 pt-2 border-t border-slate-200 dark:border-slate-800">
          <button
            onClick={() => setActiveTab("PENDING_APPROVAL")}
            className={`px-3.5 py-1.5 rounded-xl text-xs font-semibold transition-all flex items-center gap-2 ${
              activeTab === "PENDING_APPROVAL"
                ? "bg-amber-500 text-white shadow-md shadow-amber-500/30"
                : "bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-700"
            }`}
          >
            <Clock className="h-3.5 w-3.5" />
            <span>Chờ Duyệt</span>
            {pendingCount > 0 && (
              <span className="px-1.5 py-0.2 rounded-full text-[10px] bg-white text-amber-700 font-bold">
                {pendingCount}
              </span>
            )}
          </button>

          <button
            onClick={() => setActiveTab("REJECTED")}
            className={`px-3.5 py-1.5 rounded-xl text-xs font-semibold transition-all flex items-center gap-2 ${
              activeTab === "REJECTED"
                ? "bg-rose-500 text-white shadow-md shadow-rose-500/30"
                : "bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-700"
            }`}
          >
            <XCircle className="h-3.5 w-3.5" />
            <span>Cần Chỉnh Sửa</span>
            {rejectedCount > 0 && (
              <span className="px-1.5 py-0.2 rounded-full text-[10px] bg-white text-rose-700 font-bold">
                {rejectedCount}
              </span>
            )}
          </button>

          <button
            onClick={() => setActiveTab("APPROVED")}
            className={`px-3.5 py-1.5 rounded-xl text-xs font-semibold transition-all flex items-center gap-2 ${
              activeTab === "APPROVED"
                ? "bg-emerald-600 text-white shadow-md shadow-emerald-600/30"
                : "bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-700"
            }`}
          >
            <CheckCircle2 className="h-3.5 w-3.5" />
            <span>Đã Duyệt / Sẵn Sàng</span>
            {approvedCount > 0 && (
              <span className="px-1.5 py-0.2 rounded-full text-[10px] bg-slate-200 dark:bg-slate-700 text-slate-800 dark:text-slate-200 font-bold">
                {approvedCount}
              </span>
            )}
          </button>

          <button
            onClick={() => setActiveTab("ALL")}
            className={`px-3.5 py-1.5 rounded-xl text-xs font-semibold transition-all ${
              activeTab === "ALL"
                ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                : "bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-700"
            }`}
          >
            Tất cả ({posts.length})
          </button>
        </div>
      </div>

      {/* Main List */}
      {loading ? (
        <div className="py-16 text-center space-y-3">
          <RefreshCw className="h-7 w-7 text-blue-500 animate-spin mx-auto" />
          <p className="text-xs text-slate-500 dark:text-slate-400">Đang tải danh sách bài viết...</p>
        </div>
      ) : filteredPosts.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-300 dark:border-slate-800 p-12 text-center space-y-2">
          <div className="p-3 w-12 h-12 mx-auto rounded-full bg-slate-100 dark:bg-slate-800 text-slate-400 flex items-center justify-center">
            <CheckCircle2 className="h-6 w-6" />
          </div>
          <h3 className="text-sm font-bold text-slate-800 dark:text-slate-200">
            Không có bài viết nào trong mục này
          </h3>
          <p className="text-xs text-slate-500 max-w-sm mx-auto">
            {activeTab === "PENDING_APPROVAL"
              ? "Tất cả bài viết đã được duyệt hoặc chưa có bài nào cần xử lý."
              : activeTab === "REJECTED"
              ? "Không có bài viết nào bị từ chối hoặc cần chỉnh sửa."
              : "Danh sách hiện tại đang trống."}
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4">
          {filteredPosts.map((post) => {
            const isPending = post.status === "PENDING_APPROVAL";
            const isRejected = post.status === "REJECTED";

            return (
              <div
                key={post.id}
                className={`rounded-2xl border p-5 transition-all bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm shadow-sm hover:shadow-md ${
                  isPending
                    ? "border-amber-400/40 dark:border-amber-500/30 ring-1 ring-amber-400/20"
                    : isRejected
                    ? "border-rose-400/40 dark:border-rose-500/30"
                    : "border-slate-200 dark:border-slate-800"
                }`}
              >
                {/* Header row */}
                <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
                  <div className="flex items-center gap-2">
                    <span
                      className={`px-2.5 py-1 rounded-full text-[10px] font-bold uppercase tracking-wider flex items-center gap-1.5 ${
                        isPending
                          ? "bg-amber-100 dark:bg-amber-500/20 text-amber-700 dark:text-amber-300 border border-amber-300 dark:border-amber-500/30"
                          : isRejected
                          ? "bg-rose-100 dark:bg-rose-500/20 text-rose-700 dark:text-rose-300 border border-rose-300 dark:border-rose-500/30"
                          : "bg-emerald-100 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 border border-emerald-300 dark:border-emerald-500/30"
                      }`}
                    >
                      {isPending && <span className="h-1.5 w-1.5 rounded-full bg-amber-500 animate-ping" />}
                      {post.status}
                    </span>

                    <span className="flex items-center gap-1 text-xs font-semibold text-slate-800 dark:text-slate-200 bg-slate-100 dark:bg-slate-800 px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-700">
                      <Flag className="h-3 w-3 text-blue-500" />
                      {post.channel_name || "Kênh phát sóng"}
                      <span className="text-[10px] text-slate-500">({post.platform || "FACEBOOK"})</span>
                    </span>

                    <span className="flex items-center gap-1 text-[11px] font-medium text-slate-600 dark:text-slate-400 bg-slate-100 dark:bg-slate-800 px-2 py-0.5 rounded-md">
                      {renderMediaIcon(post.media_type)}
                      {post.media_type}
                    </span>

                    {post.batch_id && (
                      <span className="flex items-center gap-1 text-[10px] font-semibold text-purple-600 dark:text-purple-400 bg-purple-50 dark:bg-purple-500/10 border border-purple-200 dark:border-purple-500/30 px-2 py-0.5 rounded-md">
                        <Layers className="h-3 w-3" />
                        Batch #{post.batch_id.slice(0, 8)}
                      </span>
                    )}
                  </div>

                  <div className="flex items-center gap-2 text-xs text-slate-500">
                    <Calendar className="h-3.5 w-3.5" />
                    <span>
                      Hẹn phát sóng:{" "}
                      {post.scheduled_at
                        ? new Date(post.scheduled_at).toLocaleString("vi-VN")
                        : "Phát sóng ngay sau khi duyệt"}
                    </span>
                  </div>
                </div>

                {/* Rejection Note Warning Banner */}
                {isRejected && (
                  <div className="mb-4 p-3.5 rounded-xl bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-900/60 text-xs text-rose-800 dark:text-rose-300 flex items-start gap-2.5">
                    <AlertCircle className="h-4 w-4 shrink-0 text-rose-600 dark:text-rose-400 mt-0.5" />
                    <div className="space-y-0.5">
                      <p className="font-bold">Ghi chú từ chối / Phản hồi của Người kiểm duyệt:</p>
                      <p className="text-slate-700 dark:text-rose-200 italic">
                        &quot;{post.rejection_note || "Cần điều chỉnh lại câu từ hoặc hình ảnh phù hợp định dạng"}&quot;
                      </p>
                      {post.reviewed_at && (
                        <p className="text-[10px] text-rose-500">
                          Thời gian: {new Date(post.reviewed_at).toLocaleString("vi-VN")}
                        </p>
                      )}
                    </div>
                  </div>
                )}

                {/* Content */}
                <div className="space-y-3">
                  <p className="text-xs sm:text-sm text-slate-800 dark:text-slate-200 whitespace-pre-wrap leading-relaxed bg-slate-50 dark:bg-slate-950/40 p-3.5 rounded-xl border border-slate-200/80 dark:border-slate-800/80 font-normal">
                    {post.content || <span className="italic text-slate-400">Không có văn bản</span>}
                  </p>

                  {/* Media thumbnails */}
                  {post.media_urls && post.media_urls.length > 0 && (
                    <div className="flex flex-wrap gap-2 pt-1">
                      {post.media_urls.map((url, idx) => (
                        <div
                          key={idx}
                          className="relative h-20 w-28 rounded-lg overflow-hidden border border-slate-200 dark:border-slate-800 bg-black flex items-center justify-center group"
                        >
                          {post.media_type === "VIDEO" || post.media_type === "REEL" ? (
                            <video src={url} className="h-full w-full object-cover" />
                          ) : (
                            <img src={url} alt="Media thumbnail" className="h-full w-full object-cover" />
                          )}
                          <a
                            href={url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 flex items-center justify-center text-white text-[10px] font-semibold transition-opacity"
                          >
                            Xem gốc
                          </a>
                        </div>
                      ))}
                    </div>
                  )}

                  {post.link_url && (
                    <div className="flex items-center gap-1.5 text-xs text-blue-600 dark:text-blue-400 truncate">
                      <LinkIcon className="h-3.5 w-3.5 shrink-0" />
                      <a
                        href={post.link_url}
                        target="_blank"
                        rel="noreferrer"
                        className="hover:underline truncate"
                      >
                        {post.link_url}
                      </a>
                    </div>
                  )}
                </div>

                {/* Action Bar */}
                <div className="mt-4 pt-3 border-t border-slate-200 dark:border-slate-800 flex flex-wrap items-center justify-between gap-3">
                  <div className="text-[11px] text-slate-500">
                    Mã bài: <span className="font-mono">{post.id.slice(0, 13)}...</span>
                  </div>

                  <div className="flex items-center gap-2">
                    {isPending && canApprovePost && (
                      <>
                        <button
                          type="button"
                          onClick={() => openReviewModal(post, "REJECTED")}
                          className="px-3.5 py-1.5 rounded-xl text-xs font-semibold text-rose-600 dark:text-rose-400 bg-rose-50 hover:bg-rose-100 dark:bg-rose-950/40 dark:hover:bg-rose-900/60 border border-rose-300 dark:border-rose-800/80 transition-colors flex items-center gap-1.5"
                        >
                          <XCircle className="h-3.5 w-3.5" />
                          <span>Yêu cầu sửa</span>
                        </button>

                        <button
                          type="button"
                          onClick={() => openReviewModal(post, "APPROVED")}
                          className="px-4 py-1.5 rounded-xl text-xs font-bold text-white bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 shadow-md shadow-emerald-600/25 transition-all flex items-center gap-1.5"
                        >
                          <CheckCircle2 className="h-3.5 w-3.5" />
                          <span>Phê duyệt</span>
                        </button>
                      </>
                    )}

                    {isPending && !canApprovePost && (
                      <span className="text-xs text-amber-600 dark:text-amber-400 flex items-center gap-1 bg-amber-50 dark:bg-amber-950/40 px-3 py-1 rounded-lg border border-amber-300 dark:border-amber-800/60 font-medium">
                        <Clock className="h-3 w-3" />
                        Đang chờ Quản trị viên / Người duyệt đánh giá
                      </span>
                    )}

                    {isRejected && (
                      <button
                        type="button"
                        onClick={() => openResubmitModal(post)}
                        className="px-4 py-1.5 rounded-xl text-xs font-bold text-white bg-blue-600 hover:bg-blue-500 shadow-md shadow-blue-600/25 transition-all flex items-center gap-1.5"
                      >
                        <Edit3 className="h-3.5 w-3.5" />
                        <span>Chỉnh sửa & Gửi duyệt lại</span>
                      </button>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Review Modal */}
      {reviewModalPost && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-fade-in">
          <div className="w-full max-w-lg rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-6 shadow-2xl space-y-4">
            <div className="flex items-center gap-2.5">
              <span
                className={`p-2 rounded-xl border ${
                  reviewAction === "APPROVED"
                    ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20"
                    : "bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20"
                }`}
              >
                {reviewAction === "APPROVED" ? (
                  <CheckCircle2 className="h-5 w-5" />
                ) : (
                  <XCircle className="h-5 w-5" />
                )}
              </span>
              <div>
                <h3 className="text-sm font-bold text-slate-900 dark:text-white">
                  {reviewAction === "APPROVED"
                    ? "Xác nhận Phê duyệt bài viết"
                    : "Yêu cầu chỉnh sửa bài viết"}
                </h3>
                <p className="text-xs text-slate-500">
                  Kênh: {reviewModalPost.channel_name || "Mặc định"}
                </p>
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                {reviewAction === "APPROVED"
                  ? "Ghi chú kiểm duyệt (Tùy chọn)"
                  : "Lý do từ chối / Ghi chú góp ý (Bắt buộc)*"}
              </label>
              <textarea
                rows={3}
                placeholder={
                  reviewAction === "APPROVED"
                    ? "Nội dung đạt chuẩn, duyệt phát sóng..."
                    : "Ví dụ: Cần bổ sung hashtag #PostPilot, chỉnh lại câu mở đầu cho hấp dẫn hơn..."
                }
                value={reviewNote}
                onChange={(e) => setReviewNote(e.target.value)}
                className="w-full px-3 py-2 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-slate-100 focus:outline-none focus:border-blue-500"
              />
            </div>

            {reviewModalPost.batch_id && (
              <label className="flex items-center gap-2.5 p-3 rounded-xl bg-purple-50 dark:bg-purple-950/30 border border-purple-200 dark:border-purple-800/60 cursor-pointer">
                <input
                  type="checkbox"
                  checked={applyToBatch}
                  onChange={(e) => setApplyToBatch(e.target.checked)}
                  className="rounded text-purple-600 focus:ring-purple-500 h-4 w-4"
                />
                <div className="text-xs text-purple-900 dark:text-purple-300">
                  <span className="font-bold">Áp dụng cho toàn bộ bài cùng nhóm (Batch)</span>
                  <p className="text-[11px] text-purple-700 dark:text-purple-400">
                    Tất cả bài viết được tạo chung nhóm với bài này cũng sẽ nhận quyết định tương tự.
                  </p>
                </div>
              </label>
            )}

            <div className="flex items-center justify-end gap-2.5 pt-2">
              <button
                type="button"
                onClick={() => setReviewModalPost(null)}
                disabled={isSubmittingReview}
                className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
              >
                Hủy
              </button>
              <button
                type="button"
                onClick={handleConfirmReview}
                disabled={isSubmittingReview}
                className={`px-5 py-2 rounded-xl text-xs font-bold text-white transition-all ${
                  reviewAction === "APPROVED"
                    ? "bg-emerald-600 hover:bg-emerald-500 shadow-md shadow-emerald-600/30"
                    : "bg-rose-600 hover:bg-rose-500 shadow-md shadow-rose-600/30"
                } disabled:opacity-50`}
              >
                {isSubmittingReview
                  ? "Đang lưu..."
                  : reviewAction === "APPROVED"
                  ? "Xác nhận Phê duyệt"
                  : "Gửi Yêu cầu Sửa"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Resubmit Modal */}
      {resubmitModalPost && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-fade-in">
          <div className="w-full max-w-lg rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-6 shadow-2xl space-y-4">
            <div className="flex items-center gap-2.5">
              <span className="p-2 rounded-xl bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-500/20">
                <Edit3 className="h-5 w-5" />
              </span>
              <div>
                <h3 className="text-sm font-bold text-slate-900 dark:text-white">
                  Chỉnh sửa & Gửi duyệt lại bài viết
                </h3>
                <p className="text-xs text-slate-500">
                  Cập nhật nội dung theo phản hồi và gửi lại cho người kiểm duyệt
                </p>
              </div>
            </div>

            {resubmitModalPost.rejection_note && (
              <div className="p-3 rounded-xl bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800/60 text-xs text-amber-800 dark:text-amber-300">
                <span className="font-bold">Yêu cầu sửa:</span> {resubmitModalPost.rejection_note}
              </div>
            )}

            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                Nội dung bài viết mới
              </label>
              <textarea
                rows={5}
                value={resubmitContent}
                onChange={(e) => setResubmitContent(e.target.value)}
                className="w-full px-3 py-2 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-slate-100 focus:outline-none focus:border-blue-500 leading-relaxed"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                Thời gian phát sóng
              </label>
              <input
                type="datetime-local"
                value={resubmitScheduledAt}
                onChange={(e) => setResubmitScheduledAt(e.target.value)}
                className="w-full px-3 py-2 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-slate-100 focus:outline-none focus:border-blue-500"
              />
            </div>

            <div className="flex items-center justify-end gap-2.5 pt-2">
              <button
                type="button"
                onClick={() => setResubmitModalPost(null)}
                disabled={isSubmittingResubmit}
                className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
              >
                Hủy
              </button>
              <button
                type="button"
                onClick={handleConfirmResubmit}
                disabled={isSubmittingResubmit}
                className="px-5 py-2 rounded-xl text-xs font-bold text-white bg-blue-600 hover:bg-blue-500 shadow-md shadow-blue-600/30 transition-all disabled:opacity-50 flex items-center gap-1.5"
              >
                <Send className="h-3.5 w-3.5" />
                <span>{isSubmittingResubmit ? "Đang gửi..." : "Gửi Duyệt Lại"}</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
