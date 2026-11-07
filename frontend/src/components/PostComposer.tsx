"use client";

import { useEffect, useMemo, useState } from "react";
import { BookmarkPlus, Calendar, Clock, Send, Sparkles } from "lucide-react";
import { Channel, MediaType, Post } from "@/lib/types";
import { api } from "@/lib/api";
import { fromLocalInputValue, validateMedia } from "@/lib/schedule";
import { loadSchedulePrefs, saveSchedulePrefs, SchedulePrefs } from "@/lib/preferences";
import { useDialog } from "@/context/DialogContext";
import { useOrg } from "@/context/OrgContext";
import { TargetPicker, TargetSelection, defaultTarget, reconcileTarget, targetChannelIds, targetIsComplete, targetLabel, targetPayload } from "./TargetPicker";
import { MediaUploader } from "./MediaUploader";
import { SchedulePicker } from "./SchedulePicker";
import { RetrySelect } from "./TimeSlotEditor";

interface PostComposerProps { channels: Channel[]; posts: Post[]; onSuccess: () => void }

const toneOptions = [
  { label: "Chuyên nghiệp", value: "chuyên nghiệp, lôi cuốn, tạo tương tác cao" },
  { label: "Bắt trend / Hài hước", value: "hài hước, dí dỏm, bắt trend giới trẻ" },
  { label: "Bán hàng / Giảm giá", value: "bán hàng hấp dẫn, kêu gọi mua ngay, khan hiếm" },
  { label: "Truyền cảm hứng", value: "truyền cảm hứng sâu sắc, triết lý kinh doanh" },
];

/** Single-post composer: target, content with AI assist, media, schedule picker and live preview. */
export function PostComposer({ channels, posts, onSuccess }: PostComposerProps) {
  const { confirm, alert, toast } = useDialog();
  const { channelGroups, canCreatePost, channelRoles, canManageOrg, currentOrg } = useOrg();
  const orgId = currentOrg?.id || "";
  const [target, setTarget] = useState<TargetSelection>(() => defaultTarget(channels, channelGroups));
  const [content, setContent] = useState("");
  const [mediaType, setMediaType] = useState<MediaType>("NONE");
  const [urls, setUrls] = useState<string[]>([]);
  const [scheduledAt, setScheduledAt] = useState("");
  const [maxRetries, setMaxRetries] = useState(3);
  const [prefs, setPrefs] = useState<SchedulePrefs>(() => loadSchedulePrefs(orgId));
  const [aiTopic, setAiTopic] = useState("");
  const [aiTone, setAiTone] = useState(toneOptions[0].value);
  const [generating, setGenerating] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => { setPrefs(loadSchedulePrefs(orgId)); }, [orgId]);
  useEffect(() => { setTarget(prev => reconcileTarget(prev, channels, channelGroups)); }, [channels, channelGroups]);
  const updatePrefs = (next: SchedulePrefs) => { setPrefs(next); saveSchedulePrefs(orgId, next); };

  const channelIds = useMemo(() => targetChannelIds(target, channels, channelGroups), [target, channels, channelGroups]);
  const selectedGroup = channelGroups.find(g => g.id === target.groupId);
  const approvalChannel = (id: string) => channelGroups.some(g => g.require_approval && g.channels?.some(c => c.id === id));
  const isEditorOnApprovalGroup = !canManageOrg && channelIds.some(id => channelRoles[id] === "EDITOR" && ((target.mode === "GROUP" && selectedGroup?.require_approval) || approvalChannel(id)));

  const handleGenerateAI = async () => {
    if (!aiTopic.trim()) { alert({ title: "Thiếu chủ đề", message: "Nhập chủ đề để AI gợi ý nội dung.", variant: "warning" }); return; }
    setGenerating(true);
    try { const res = await api.generateAI(aiTopic.trim(), aiTone); if (res.generated_content) { setContent(res.generated_content); toast("AI đã viết xong nội dung.", "success"); } }
    catch (err) { alert({ title: "Lỗi AI", message: err instanceof Error ? err.message : "Không thể kết nối AI", variant: "danger" }); }
    finally { setGenerating(false); }
  };

  const validate = (): string | null => {
    if (!canCreatePost) return "Vai trò hiện tại chỉ được xem, không tạo bài.";
    if (!targetIsComplete(target)) return "Chọn Fanpage hoặc nhóm Fanpage để đăng.";
    if (uploading) return "Tệp đang tải lên, vui lòng đợi.";
    return validateMedia(mediaType, urls, content);
  };

  const handleSubmit = async (publishNow: boolean, submitForApproval = false) => {
    const problem = validate();
    if (problem) { alert({ title: "Chưa thể gửi", message: problem, variant: "warning" }); return; }
    if (publishNow && !submitForApproval) {
      const ok = await confirm({ title: "Đăng ngay", message: `Bài viết sẽ vào hàng đợi và đăng ngay lên ${channelIds.length} Fanpage. Tiếp tục?`, confirmText: "Đăng ngay", variant: "info" });
      if (!ok) return;
    }
    const when = fromLocalInputValue(scheduledAt);
    setSubmitting(true);
    try {
      await api.createPost({
        ...targetPayload(target), content: content.trim(), media_type: mediaType,
        link_url: mediaType === "LINK" ? urls[0] : "", media_urls: mediaType !== "LINK" ? urls : [],
        scheduled_at: when ? when.toISOString() : "", publish_now: publishNow,
        submit_for_approval: submitForApproval || isEditorOnApprovalGroup, max_retries: maxRetries,
      });
      setContent(""); setUrls([]); setMediaType("NONE"); setScheduledAt("");
      onSuccess();
      toast(submitForApproval || isEditorOnApprovalGroup ? "Đã gửi bài vào hộp phê duyệt." : publishNow ? "Đã đưa bài vào hàng đợi. Theo dõi trạng thái trong Lịch đăng." : "Đã lên lịch đăng bài.", "success");
    } catch (err) {
      alert({ title: "Không tạo được bài", message: err instanceof Error ? err.message : "Không xác định", variant: "danger" });
    } finally { setSubmitting(false); }
  };

  /** Keeps the current content as a reusable library post (no Fanpage binding unless exactly one is chosen). */
  const saveToLibrary = async () => {
    if (!canCreatePost) { alert({ title: "Chưa thể lưu", message: "Vai trò hiện tại chỉ được xem, không tạo bài.", variant: "warning" }); return; }
    const problem = uploading ? "Tệp đang tải lên, vui lòng đợi." : validateMedia(mediaType, urls, content);
    if (problem) { alert({ title: "Chưa thể lưu", message: problem, variant: "warning" }); return; }
    setSubmitting(true);
    try {
      await api.createLibraryItem({ channel_id: target.mode === "CHANNEL" ? target.channelId : "", content: content.trim(), media_type: mediaType, link_url: mediaType === "LINK" ? urls[0] : "", media_urls: mediaType !== "LINK" ? urls : [] });
      const boundTo = target.mode === "CHANNEL" ? channels.find(c => c.id === target.channelId)?.name : "";
      toast(`Đã lưu vào Bài soạn sẵn (${boundTo ? `dành cho Fanpage ${boundTo}` : "dùng cho mọi Fanpage"}). Bấm vào một ngày trên lịch để lên lịch.`, "success");
    } catch (err) {
      alert({ title: "Không lưu được bài soạn sẵn", message: err instanceof Error ? err.message : "Không xác định", variant: "danger" });
    } finally { setSubmitting(false); }
  };

  const previewName = targetLabel(target, channels, channelGroups);
  const when = fromLocalInputValue(scheduledAt);
  const disabled = submitting || uploading || !canCreatePost;
  return <div className="composer-layout">
    <form className="dispatch-panel composer-form" onSubmit={e => e.preventDefault()} aria-label="Soạn một bài viết">
      {!canCreatePost && <p className="dispatch-note warning">Bạn chỉ có quyền xem. Liên hệ quản trị viên để được cấp quyền tạo bài.</p>}
      <TargetPicker channels={channels} channelGroups={channelGroups} value={target} onChange={setTarget} idPrefix="compose" />
      <section className="composer-section" aria-label="Soạn nội dung">
        <div className="composer-row-head"><label className="composer-label" htmlFor="compose-content">Nội dung</label><span className="dispatch-muted text-xs">{content.length} ký tự</span></div>
        <textarea id="compose-content" rows={8} value={content} onChange={e => setContent(e.target.value)} placeholder="Viết nội dung, thêm link, lời kêu gọi hành động và hashtag…" />
        <div className="ai-row">
          <input type="text" aria-label="Chủ đề để AI gợi ý" placeholder="Chủ đề để AI viết nháp, ví dụ: ra mắt bộ sưu tập thu đông" value={aiTopic} onChange={e => setAiTopic(e.target.value)} onKeyDown={e => { if (e.key === "Enter") { e.preventDefault(); handleGenerateAI(); } }} />
          <select aria-label="Giọng văn" value={aiTone} onChange={e => setAiTone(e.target.value)}>{toneOptions.map(t => <option key={t.value} value={t.value}>{t.label}</option>)}</select>
          <button type="button" className="dispatch-button" onClick={handleGenerateAI} disabled={generating}><Sparkles size={15} className={generating ? "animate-spin" : ""} />{generating ? "Đang viết…" : "AI viết nháp"}</button>
        </div>
      </section>
      <MediaUploader idPrefix="compose" mediaType={mediaType} onMediaTypeChange={setMediaType} urls={urls} onUrlsChange={setUrls} onUploadingChange={setUploading} />
      <section className="composer-section composer-schedule" aria-label="Thời gian">
        <SchedulePicker id="compose-schedule" value={scheduledAt} onChange={setScheduledAt} posts={posts} channelIds={channelIds} prefs={prefs} onPrefsChange={updatePrefs} />
        <RetrySelect value={maxRetries} onChange={setMaxRetries} />
      </section>
      <div className="composer-actions">
        <button type="button" className="dispatch-button" disabled={disabled} onClick={saveToLibrary}><BookmarkPlus size={16} />Lưu bài soạn sẵn</button>
        {isEditorOnApprovalGroup ? <button type="button" className="dispatch-button primary" disabled={disabled} onClick={() => handleSubmit(false, true)}><Clock size={16} />{submitting ? "Đang gửi…" : "Gửi duyệt"}</button> : <>
          <button type="button" className="dispatch-button" disabled={disabled} onClick={() => handleSubmit(false, true)}><Clock size={16} />Gửi duyệt</button>
          <button type="button" className="dispatch-button" disabled={disabled} onClick={() => handleSubmit(false, false)}><Calendar size={16} />{when ? "Lên lịch" : "Lưu lịch (giờ hiện tại)"}</button>
          <button type="button" className="dispatch-button primary" disabled={disabled} onClick={() => handleSubmit(true, false)}><Send size={16} />{submitting ? "Đang xử lý…" : "Đăng ngay"}</button>
        </>}
      </div>
    </form>
    <aside className="dispatch-panel composer-preview" aria-label="Xem trước bài viết">
      <p className="dispatch-eyebrow">Xem trước</p>
      <div className="flex items-center gap-3 mt-6"><span className="dispatch-avatar">{previewName.slice(0, 1).toUpperCase()}</span><div className="min-w-0"><h2 className="text-sm font-semibold break-words">{previewName}</h2><p className="dispatch-muted text-xs mt-1">{channelIds.length > 1 ? `${channelIds.length} Fanpage` : "Facebook · Bài viết"}</p></div></div>
      <p className="composer-preview-content">{content || "Nội dung bạn viết sẽ xuất hiện tại đây."}</p>
      {["IMAGE", "ALBUM"].includes(mediaType) && urls.map(url => <img key={url} className="composer-preview-media" src={url} alt="Ảnh đính kèm" />)}
      {["VIDEO", "REEL"].includes(mediaType) && urls[0] && <video className="composer-preview-media" src={urls[0]} controls preload="metadata" />}
      {mediaType === "LINK" && urls[0] && <p className="dispatch-muted text-xs mt-4 break-all">{urls[0]}</p>}
      <div className="composer-preview-foot"><p className="text-xs dispatch-muted">{when ? `Dự kiến: ${when.toLocaleString("vi-VN")}` : "Chưa chọn thời gian đăng"}</p><p className="text-xs dispatch-muted mt-3">Bản xem trước nội dung. Hiển thị thực tế trên Facebook có thể khác.</p></div>
    </aside>
  </div>;
}
