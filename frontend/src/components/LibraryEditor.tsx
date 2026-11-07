"use client";

import { useRef, useState } from "react";
import { Save, X } from "lucide-react";
import { Channel, LibraryItem, MediaType } from "@/lib/types";
import { api } from "@/lib/api";
import { validateMedia } from "@/lib/schedule";
import { useDialog } from "@/context/DialogContext";
import { MediaUploader } from "./MediaUploader";
import { Modal } from "./Modal";

interface LibraryEditorProps {
  item: LibraryItem | null;               // null = new item
  channels: Channel[];
  defaultChannelId?: string;
  onClose: () => void;
  onSaved: (item: LibraryItem) => void;
}

/** Create/edit dialog for one ready-made post: title, optional Fanpage, content and media. */
export function LibraryEditor({ item, channels, defaultChannelId = "", onClose, onSaved }: LibraryEditorProps) {
  const { alert, confirm, toast } = useDialog();
  const [title, setTitle] = useState(item?.title || "");
  const [channelId, setChannelId] = useState(item ? item.channel_id || "" : defaultChannelId);
  const [content, setContent] = useState(item?.content || "");
  const [mediaType, setMediaType] = useState<MediaType>(item?.media_type || "NONE");
  const [urls, setUrls] = useState<string[]>(item ? (item.media_type === "LINK" ? (item.link_url ? [item.link_url] : []) : item.media_urls || []) : []);
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const initial = useRef({ title: item?.title || "", channelId: item ? item.channel_id || "" : defaultChannelId, content: item?.content || "", mediaType: item?.media_type || "NONE", urls: JSON.stringify(item ? (item.media_type === "LINK" ? (item.link_url ? [item.link_url] : []) : item.media_urls || []) : []) });
  const dirty = title !== initial.current.title || channelId !== initial.current.channelId || content !== initial.current.content || mediaType !== initial.current.mediaType || JSON.stringify(urls) !== initial.current.urls || uploading;

  /** Escape, the X button and "Huỷ" all land here: never lose a long draft silently or close mid-save. */
  const requestClose = async () => {
    if (saving) return;
    if (dirty) {
      const discard = await confirm({ title: "Bỏ thay đổi?", message: "Nội dung vừa soạn chưa được lưu và sẽ mất nếu đóng cửa sổ này.", confirmText: "Bỏ thay đổi", cancelText: "Tiếp tục soạn", variant: "danger" });
      if (!discard) return;
    }
    onClose();
  };

  const save = async () => {
    const problem = uploading ? "Tệp đang tải lên, vui lòng đợi." : validateMedia(mediaType, urls, content);
    if (problem) { alert({ title: "Chưa thể lưu", message: problem, variant: "warning" }); return; }
    setSaving(true);
    try {
      const payload = {
        title: title.trim(), channel_id: channelId, content: content.trim(), media_type: mediaType,
        link_url: mediaType === "LINK" ? urls[0] : "", media_urls: mediaType !== "LINK" ? urls : [],
      };
      const saved = item ? await api.updateLibraryItem(item.id, payload) : await api.createLibraryItem(payload);
      toast(item ? "Đã cập nhật bài soạn sẵn." : "Đã lưu vào Bài soạn sẵn.", "success");
      onSaved(saved);
    } catch (err) {
      alert({ title: "Không lưu được bài soạn sẵn", message: err instanceof Error ? err.message : "Không xác định", variant: "danger" });
    } finally { setSaving(false); }
  };

  return <Modal labelledBy="library-editor-heading" onClose={() => void requestClose()} dismissOnBackdrop={false}>
    <section className="dispatch-panel library-editor">
      <div className="dispatch-section-heading"><h2 id="library-editor-heading">{item ? "Sửa bài soạn sẵn" : "Bài soạn sẵn mới"}</h2><button type="button" className="dispatch-icon-button" onClick={() => void requestClose()} disabled={saving} aria-label="Đóng"><X size={20} /></button></div>
      <form onSubmit={event => { event.preventDefault(); void save(); }} className="library-editor-form">
        <div className="library-editor-row">
          <label className="dispatch-field">Tiêu đề (tuỳ chọn)<input value={title} maxLength={255} onChange={event => setTitle(event.target.value)} placeholder="Ví dụ: Khuyến mãi cuối tuần" /></label>
          <label className="dispatch-field">Dùng cho Fanpage<select value={channelId} onChange={event => setChannelId(event.target.value)}><option value="">Mọi Fanpage</option>{channels.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
        </div>
        <div className="composer-section">
          <div className="composer-row-head"><label className="composer-label" htmlFor="library-content">Nội dung</label><span className="dispatch-muted text-xs">{content.length} ký tự</span></div>
          <textarea id="library-content" rows={9} value={content} onChange={event => setContent(event.target.value)} placeholder="Viết sẵn nội dung, link, lời kêu gọi hành động và hashtag…" />
        </div>
        <MediaUploader idPrefix="library" mediaType={mediaType} onMediaTypeChange={setMediaType} urls={urls} onUrlsChange={setUrls} onUploadingChange={setUploading} />
        <div className="composer-actions"><button type="button" className="dispatch-button" onClick={() => void requestClose()} disabled={saving}>Huỷ</button><button className="dispatch-button primary" disabled={saving || uploading}><Save size={16} aria-hidden="true" />{saving ? "Đang lưu…" : "Lưu bài soạn sẵn"}</button></div>
      </form>
    </section>
  </Modal>;
}
