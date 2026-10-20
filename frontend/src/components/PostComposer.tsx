"use client";

import React, { useState, useRef } from "react";
import { FacebookPage, MediaType } from "@/lib/types";
import { api } from "@/lib/api";
import {
  Sparkles,
  Upload,
  Calendar,
  Send,
  Film,
  Video,
  Image as ImageIcon,
  Link as LinkIcon,
  FileText,
  X,
  Clock,
  Layers,
  AlertCircle,
  CheckCircle2,
} from "lucide-react";
import { useDialog } from "@/context/DialogContext";

interface PostComposerProps {
  pages: FacebookPage[];
  onSuccess: () => void;
}

export function PostComposer({ pages, onSuccess }: PostComposerProps) {
  const { confirm, alert, toast } = useDialog();
  const [selectedPageUUID, setSelectedPageUUID] = useState<string>(
    pages.length > 0 ? pages[0].id : ""
  );

  // Content & Type
  const [content, setContent] = useState<string>("");
  const [mediaType, setMediaType] = useState<MediaType>("NONE");
  const [mediaUrls, setMediaUrls] = useState<string>("");
  const [scheduledAt, setScheduledAt] = useState<string>("");
  const [maxRetries, setMaxRetries] = useState<number>(3);

  // AI Assistant State
  const [aiTopic, setAiTopic] = useState<string>("");
  const [aiTone, setAiTone] = useState<string>("chuyên nghiệp, lôi cuốn, tạo tương tác cao");
  const [isGeneratingAI, setIsGeneratingAI] = useState<boolean>(false);

  // Upload State
  const [isUploading, setIsUploading] = useState<boolean>(false);
  const [uploadProgress, setUploadProgress] = useState<number>(0);
  const [uploadText, setUploadText] = useState<string>("");
  const [uploadedFile, setUploadedFile] = useState<{
    name: string;
    sizeMB: string;
    isVideo: boolean;
    previewUrl: string;
  } | null>(null);

  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);
  const [dragOver, setDragOver] = useState<boolean>(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Sync selected page if empty
  React.useEffect(() => {
    if (!selectedPageUUID && pages.length > 0) {
      setSelectedPageUUID(pages[0].id);
    }
  }, [pages, selectedPageUUID]);

  // AI Generate Handler
  const handleGenerateAI = async () => {
    if (!aiTopic.trim()) {
      alert({
        title: "Thiếu chủ đề",
        message: "Vui lòng nhập chủ đề gợi ý để AI Gemini sáng tạo nội dung bài viết!",
        variant: "warning",
      });
      return;
    }
    setIsGeneratingAI(true);
    try {
      const res = await api.generateAI(aiTopic.trim(), aiTone);
      if (res.generated_content) {
        setContent(res.generated_content);
        toast("✨ AI đã viết xong nội dung bài đăng!", "success");
      }
    } catch (err: any) {
      alert({
        title: "Lỗi AI Gemini",
        message: err.message || "Không thể kết nối đến AI API",
        variant: "danger",
      });
    } finally {
      setIsGeneratingAI(false);
    }
  };

  // Upload File Handler
  const handleFile = async (file: File) => {
    const isVideo = file.type.startsWith("video/") || /\.(mp4|mov|webm|m4v)$/i.test(file.name);
    const isImage = file.type.startsWith("image/") || /\.(jpg|jpeg|png|webp|gif)$/i.test(file.name);
    const sizeMB = (file.size / (1024 * 1024)).toFixed(1);
    const previewUrl = URL.createObjectURL(file);

    setUploadedFile({
      name: file.name,
      sizeMB,
      isVideo,
      previewUrl,
    });

    if (isVideo && mediaType !== "REEL") {
      setMediaType("VIDEO");
    } else if (isImage && mediaType === "NONE") {
      setMediaType("IMAGE");
    }

    setIsUploading(true);
    setUploadProgress(0);
    setUploadText(`Đang tải lên: 0% (0 MB / ${sizeMB} MB)`);

    try {
      const res = await api.uploadMedia(file, (percent, loadedMB, totalMB) => {
        setUploadProgress(percent);
        setUploadText(`Đang tải lên: ${percent}% (${loadedMB} MB / ${totalMB} MB)`);
      });

      if (res.public_url) {
        setMediaUrls((prev) => (prev ? `${prev}, ${res.public_url}` : res.public_url));
        toast("📁 Đã tải tệp lên máy chủ thành công!", "success");
      }
    } catch (err: any) {
      alert({
        title: "Tải file thất bại",
        message: err.message,
        variant: "danger",
      });
      clearUploadedMedia();
    } finally {
      setIsUploading(false);
    }
  };

  const clearUploadedMedia = () => {
    if (uploadedFile?.previewUrl) {
      URL.revokeObjectURL(uploadedFile.previewUrl);
    }
    setUploadedFile(null);
    setMediaUrls("");
    setMediaType("NONE");
    if (fileInputRef.current) fileInputRef.current.value = "";
  };

  // Quick schedule helpers
  const setQuickSchedule = (minutesFromNow: number) => {
    const d = new Date(Date.now() + minutesFromNow * 60 * 1000);
    // Format to YYYY-MM-DDTHH:mm for datetime-local
    const pad = (n: number) => n.toString().padStart(2, "0");
    const formatted = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    setScheduledAt(formatted);
  };

  // Submit Post
  const handleSubmit = async (publishNow: boolean) => {
    if (!selectedPageUUID) {
      alert({
        title: "Chưa chọn Fanpage",
        message: "Vui lòng chọn Fanpage đích để đăng bài viết!",
        variant: "warning",
      });
      return;
    }
    if (isUploading) {
      alert({
        title: "Tập tin đang xử lý",
        message: "Tập tin media đang được tải lên máy chủ, vui lòng đợi giây lát!",
        variant: "warning",
      });
      return;
    }

    const urls = mediaUrls
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);

    if ((mediaType === "VIDEO" || mediaType === "REEL") && urls.length === 0) {
      alert({
        title: "Thiếu Video",
        message: "Loại bài viết Video / Reel bắt buộc phải có ít nhất 1 video đính kèm!",
        variant: "warning",
      });
      return;
    }

    if (!content.trim() && urls.length === 0) {
      alert({
        title: "Bài viết trống",
        message: "Bài đăng cần có nội dung văn bản hoặc media đính kèm!",
        variant: "warning",
      });
      return;
    }

    if (publishNow) {
      const ok = await confirm({
        title: "Xác nhận đăng bài ngay",
        message: "Bài viết sẽ được đưa vào hàng đợi ưu tiên để phát sóng trực tiếp lên Facebook ngay. Tiếp tục?",
        confirmText: "Đăng ngay bây giờ",
        variant: "info",
      });
      if (!ok) return;
    }

    setIsSubmitting(true);
    try {
      await api.createPost({
        page_uuid: selectedPageUUID,
        content: content.trim(),
        media_type: mediaType,
        link_url: mediaType === "LINK" && urls.length > 0 ? urls[0] : "",
        media_urls: mediaType !== "LINK" ? urls : [],
        scheduled_at: scheduledAt ? new Date(scheduledAt).toISOString() : "",
        publish_now: publishNow,
        max_retries: maxRetries,
      });

      // Reset form
      setContent("");
      clearUploadedMedia();
      setScheduledAt("");
      onSuccess();
      toast(
        publishNow
          ? "🚀 Đã kích hoạt đăng bài thành công lên Facebook!"
          : "📅 Đã lưu lịch hẹn đăng bài tự động!",
        "success"
      );
    } catch (err: any) {
      alert({
        title: "Lỗi tạo bài đăng",
        message: err.message || "Không thể tạo bài đăng",
        variant: "danger",
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  const mediaTypes: { type: MediaType; label: string; icon: any }[] = [
    { type: "NONE", label: "Văn bản thuần", icon: FileText },
    { type: "LINK", label: "Link Preview", icon: LinkIcon },
    { type: "IMAGE", label: "1 Ảnh", icon: ImageIcon },
    { type: "ALBUM", label: "Album nhiều ảnh", icon: Layers },
    { type: "VIDEO", label: "Video thường", icon: Video },
    { type: "REEL", label: "Facebook Reel", icon: Film },
  ];

  const toneOptions = [
    { label: "Chuyên nghiệp", value: "chuyên nghiệp, lôi cuốn, tạo tương tác cao" },
    { label: "Bắt trend / Hài hước", value: "hài hước, dí dỏm, bắt trend giới trẻ" },
    { label: "Bán hàng / Giảm giá", value: "bán hàng hấp dẫn, kêu gọi mua ngay, khan hiếm" },
    { label: "Truyền cảm hứng", value: "truyền cảm hứng sâu sắc, triết lý kinh doanh" },
  ];

  return (
    <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm p-5 sm:p-6 shadow-sm dark:shadow-xl space-y-5 transition-colors">
      {/* Target Fanpage Selection */}
      <div>
        <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 uppercase tracking-wider mb-2">
          Chọn Fanpage phát sóng
        </label>
        {pages.length === 0 ? (
          <div className="p-3 bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-500/30 rounded-xl text-amber-700 dark:text-amber-300 text-xs flex items-center gap-2">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>Chưa có Fanpage nào được kết nối. Hãy thêm Fanpage trong tab Quản lý Fanpage trước.</span>
          </div>
        ) : (
          <select
            value={selectedPageUUID}
            onChange={(e) => setSelectedPageUUID(e.target.value)}
            className="w-full px-3.5 py-2.5 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-slate-100 font-semibold focus:outline-none focus:border-blue-500"
          >
            {pages.map((p) => (
              <option key={p.id} value={p.id}>
                {p.page_name} ({p.page_id}) — {p.token_status}
              </option>
            ))}
          </select>
        )}
      </div>

      {/* AI Assistant Generator */}
      <div className="rounded-xl border border-blue-200 dark:border-blue-500/20 bg-blue-50/60 dark:bg-blue-950/20 p-4 space-y-3">
        <div className="flex items-center justify-between">
          <span className="text-xs font-bold text-blue-700 dark:text-blue-300 flex items-center gap-1.5">
            <Sparkles className="h-3.5 w-3.5 text-blue-500 dark:text-blue-400" /> Trợ Lý AI Gemini Sáng Tạo Nội Dung
          </span>
          <span className="text-[10px] text-blue-700 dark:text-blue-400/80 bg-blue-100 dark:bg-blue-500/10 px-2 py-0.5 rounded-md border border-blue-200 dark:border-blue-500/20">
            Gemini Flash
          </span>
        </div>

        <div className="flex flex-col sm:flex-row gap-2">
          <input
            type="text"
            placeholder="Ví dụ: Giới thiệu tính năng đăng video Reel tự động cho kinh doanh online..."
            value={aiTopic}
            onChange={(e) => setAiTopic(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleGenerateAI()}
            className="flex-1 px-3 py-2 rounded-lg bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-blue-500"
          />
          <select
            value={aiTone}
            onChange={(e) => setAiTone(e.target.value)}
            className="px-3 py-2 rounded-lg bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-700 dark:text-slate-300 focus:outline-none focus:border-blue-500"
          >
            {toneOptions.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
          <button
            type="button"
            onClick={handleGenerateAI}
            disabled={isGeneratingAI}
            className="px-4 py-2 rounded-lg text-xs font-semibold bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 text-white shadow-md shadow-blue-600/20 transition-all disabled:opacity-50 flex items-center justify-center gap-1.5 shrink-0"
          >
            <Sparkles className={`h-3.5 w-3.5 ${isGeneratingAI ? "animate-spin" : ""}`} />
            <span>{isGeneratingAI ? "Đang viết..." : "Viết bài"}</span>
          </button>
        </div>
      </div>

      {/* Post Caption / Content */}
      <div>
        <div className="flex justify-between items-center mb-1.5">
          <label className="text-xs font-semibold text-slate-700 dark:text-slate-300 uppercase tracking-wider">
            Nội dung bài viết (Caption & Hashtags)
          </label>
          <span className="text-[11px] text-slate-400 dark:text-slate-500">{content.length} ký tự</span>
        </div>
        <textarea
          rows={5}
          placeholder="Nhập nội dung bài đăng Facebook, thông tin chương trình, link, lời kêu gọi hành động, kèm hashtags..."
          value={content}
          onChange={(e) => setContent(e.target.value)}
          className="w-full px-3.5 py-2.5 rounded-xl bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 focus:outline-none focus:border-blue-500 leading-relaxed resize-y"
        />
      </div>

      {/* Media Type Selector */}
      <div>
        <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 uppercase tracking-wider mb-2">
          Định dạng bài đăng (Media Type)
        </label>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-2">
          {mediaTypes.map((item) => {
            const Icon = item.icon;
            const isSelected = mediaType === item.type;
            return (
              <button
                key={item.type}
                type="button"
                onClick={() => setMediaType(item.type)}
                className={`flex items-center gap-2 p-2.5 rounded-xl border text-xs font-semibold transition-all ${
                  isSelected
                    ? "bg-blue-50 dark:bg-blue-600/15 border-blue-500 text-blue-600 dark:text-blue-400 shadow-sm"
                    : "bg-slate-50 dark:bg-slate-950/60 border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 hover:border-slate-300 dark:hover:border-slate-700"
                }`}
              >
                <Icon className="h-4 w-4 shrink-0" />
                <span className="truncate">{item.label}</span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Media Upload Drag & Drop Area */}
      <div>
        <div className="flex justify-between items-center mb-1.5">
          <label className="text-xs font-semibold text-slate-700 dark:text-slate-300 uppercase tracking-wider">
            Tải Video hoặc Ảnh trực tiếp từ máy tính
          </label>
          {uploadedFile && (
            <button
              type="button"
              onClick={clearUploadedMedia}
              className="text-[11px] text-rose-500 dark:text-rose-400 hover:underline flex items-center gap-1"
            >
              <X className="h-3 w-3" /> Bỏ chọn tập tin
            </button>
          )}
        </div>

        {!uploadedFile ? (
          <div
            onDragEnter={(e) => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragOver={(e) => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={(e) => {
              e.preventDefault();
              setDragOver(false);
              const files = e.dataTransfer.files;
              if (files && files.length > 0) handleFile(files[0]);
            }}
            onClick={() => fileInputRef.current?.click()}
            className={`border-2 border-dashed rounded-2xl p-6 text-center cursor-pointer transition-all ${
              dragOver
                ? "border-blue-500 bg-blue-500/10"
                : "border-slate-300 dark:border-slate-800 bg-slate-50/70 dark:bg-slate-950/40 hover:border-slate-400 dark:hover:border-slate-700 hover:bg-slate-100/70 dark:hover:bg-slate-950/60"
            }`}
          >
            <input
              ref={fileInputRef}
              type="file"
              className="hidden"
              accept="video/*,image/*"
              onChange={(e) => {
                if (e.target.files && e.target.files.length > 0) {
                  handleFile(e.target.files[0]);
                }
              }}
            />
            <div className="flex flex-col items-center gap-2">
              <div className="p-3 rounded-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-blue-500 dark:text-blue-400 shadow-sm">
                <Upload className="h-5 w-5" />
              </div>
              <p className="text-xs font-semibold text-slate-800 dark:text-slate-200">
                Kéo thả Video / Ảnh vào đây hoặc nhấn để duyệt file
              </p>
              <p className="text-[11px] text-slate-500 dark:text-slate-500">
                Hỗ trợ MP4, MOV, WebM (Reels / Video lên tới 500MB) hoặc PNG, JPG, WebP
              </p>
            </div>
          </div>
        ) : (
          <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950 p-4 space-y-3">
            <div className="flex items-center justify-between text-xs">
              <span className="font-semibold text-slate-800 dark:text-slate-200 truncate max-w-sm">
                📁 {uploadedFile.name} ({uploadedFile.sizeMB} MB)
              </span>
              <span className="text-emerald-600 dark:text-emerald-400 font-medium flex items-center gap-1">
                <CheckCircle2 className="h-3.5 w-3.5" /> Sẵn sàng
              </span>
            </div>

            {/* Upload Progress Bar if uploading */}
            {isUploading && (
              <div className="space-y-1">
                <div className="flex justify-between text-[11px] text-blue-500 dark:text-blue-400">
                  <span>{uploadText}</span>
                </div>
                <div className="w-full h-1.5 rounded-full bg-slate-200 dark:bg-slate-800 overflow-hidden">
                  <div
                    className="h-full bg-gradient-to-r from-blue-500 to-emerald-400 transition-all duration-150"
                    style={{ width: `${uploadProgress}%` }}
                  />
                </div>
              </div>
            )}

            {/* Media Preview Box */}
            <div className="rounded-lg overflow-hidden border border-slate-200 dark:border-slate-800 bg-black flex items-center justify-center max-h-56">
              {uploadedFile.isVideo ? (
                <video src={uploadedFile.previewUrl} controls className="max-h-56 max-w-full" />
              ) : (
                <img src={uploadedFile.previewUrl} alt="Preview" className="max-h-56 object-contain" />
              )}
            </div>
          </div>
        )}
      </div>

      {/* Media URLs manual input */}
      <div>
        <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 uppercase tracking-wider mb-1">
          Media URLs (Tự động điền sau khi upload hoặc dán link CDN ngoài)
        </label>
        <input
          type="text"
          placeholder="https://cdn.example.com/video.mp4"
          value={mediaUrls}
          onChange={(e) => setMediaUrls(e.target.value)}
          className="w-full px-3.5 py-2 rounded-xl bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-800 dark:text-slate-300 placeholder-slate-400 dark:placeholder-slate-600 focus:outline-none focus:border-blue-500 font-mono text-[11px]"
        />
      </div>

      {/* Schedule Picker & Helpers */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-2 border-t border-slate-200 dark:border-slate-800/80">
        <div>
          <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 uppercase tracking-wider mb-1.5">
            Thời gian hẹn giờ phát sóng
          </label>
          <input
            type="datetime-local"
            value={scheduledAt}
            onChange={(e) => setScheduledAt(e.target.value)}
            className="w-full px-3.5 py-2 rounded-xl bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-white focus:outline-none focus:border-blue-500"
          />
          <div className="flex flex-wrap gap-1.5 mt-2">
            <button
              type="button"
              onClick={() => setQuickSchedule(15)}
              className="px-2 py-0.5 rounded bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 text-[10px] transition-colors"
            >
              +15 phút
            </button>
            <button
              type="button"
              onClick={() => setQuickSchedule(60)}
              className="px-2 py-0.5 rounded bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 text-[10px] transition-colors"
            >
              +1 giờ
            </button>
            <button
              type="button"
              onClick={() => setQuickSchedule(24 * 60)}
              className="px-2 py-0.5 rounded bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 text-[10px] transition-colors"
            >
              +24 giờ
            </button>
            {scheduledAt && (
              <button
                type="button"
                onClick={() => setScheduledAt("")}
                className="px-2 py-0.5 rounded bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-rose-500 dark:text-rose-400 text-[10px] transition-colors"
              >
                Xóa giờ hẹn
              </button>
            )}
          </div>
        </div>

        <div>
          <label className="block text-xs font-semibold text-slate-700 dark:text-slate-300 uppercase tracking-wider mb-1.5">
            Số lần thử lại khi lỗi (Exponential Backoff)
          </label>
          <select
            value={maxRetries}
            onChange={(e) => setMaxRetries(Number(e.target.value))}
            className="w-full px-3.5 py-2 rounded-xl bg-white dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-white focus:outline-none focus:border-blue-500"
          >
            <option value={1}>1 lần (Không thử lại nhiều)</option>
            <option value={3}>3 lần (Tiêu chuẩn khuyên dùng)</option>
            <option value={5}>5 lần (Độ tin cậy cao)</option>
          </select>
          <p className="text-[10px] text-slate-500 mt-2">
            Hệ thống tự động hoãn (backoff) theo lũy thừa 2s, 4s, 8s nếu gặp lỗi mạng hoặc Meta Rate-Limit.
          </p>
        </div>
      </div>

      {/* Action Buttons */}
      <div className="pt-3 border-t border-slate-200 dark:border-slate-800 flex flex-col sm:flex-row items-center justify-end gap-3">
        <button
          type="button"
          onClick={() => handleSubmit(false)}
          disabled={isSubmitting || isUploading}
          className="w-full sm:w-auto px-5 py-2.5 rounded-xl text-xs font-semibold bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 border border-slate-200 dark:border-slate-700 transition-all disabled:opacity-50 flex items-center justify-center gap-1.5"
        >
          <Calendar className="h-4 w-4 text-amber-500 dark:text-amber-400" />
          <span>Lên Lịch Hẹn Đăng Bài</span>
        </button>

        <button
          type="button"
          onClick={() => handleSubmit(true)}
          disabled={isSubmitting || isUploading}
          className="w-full sm:w-auto px-6 py-2.5 rounded-xl text-xs font-bold bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-white shadow-lg shadow-emerald-600/25 transition-all disabled:opacity-50 flex items-center justify-center gap-1.5"
        >
          <Send className="h-4 w-4" />
          <span>{isSubmitting ? "Đang xử lý..." : "Đăng Ngay (Publish Now)"}</span>
        </button>
      </div>
    </div>
  );
}
