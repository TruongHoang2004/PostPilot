"use client";

import { useEffect, useRef, useState } from "react";
import { Film, FileText, Image as ImageIcon, Layers, Link as LinkIcon, Upload, Video, X } from "lucide-react";
import { MediaType } from "@/lib/types";
import { api } from "@/lib/api";
import { useDialog } from "@/context/DialogContext";

export const mediaTypeOptions: { type: MediaType; label: string; icon: typeof FileText }[] = [
  { type: "NONE", label: "Văn bản", icon: FileText }, { type: "LINK", label: "Liên kết", icon: LinkIcon }, { type: "IMAGE", label: "Ảnh", icon: ImageIcon },
  { type: "ALBUM", label: "Album", icon: Layers }, { type: "VIDEO", label: "Video", icon: Video }, { type: "REEL", label: "Reel", icon: Film },
];

interface MediaUploaderProps {
  mediaType: MediaType; onMediaTypeChange: (type: MediaType) => void;
  urls: string[]; onUrlsChange: (urls: string[]) => void;
  idPrefix: string; compact?: boolean;
  onUploadingChange?: (uploading: boolean) => void;
}

/** Media type chips, drag-and-drop upload with progress, URL entry and previews. Shared by every composer. */
export function MediaUploader({ mediaType, onMediaTypeChange, urls, onUrlsChange, idPrefix, compact, onUploadingChange }: MediaUploaderProps) {
  const { alert, toast } = useDialog();
  const fileInput = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState("");
  const [dragOver, setDragOver] = useState(false);
  const [urlText, setUrlText] = useState("");
  // Uploads resolve asynchronously; read the latest list instead of the closure captured at drop time.
  const latestUrls = useRef(urls);
  useEffect(() => { latestUrls.current = urls; }, [urls]);

  const setUploadingState = (state: boolean) => { setUploading(state); onUploadingChange?.(state); };
  const handleFile = async (file: File) => {
    const isVideo = file.type.startsWith("video/") || /\.(mp4|mov|webm|m4v)$/i.test(file.name);
    const isImage = file.type.startsWith("image/") || /\.(jpg|jpeg|png|webp|gif)$/i.test(file.name);
    if (!isVideo && !isImage) { if (fileInput.current) fileInput.current.value = ""; alert({ title: "Tệp không hỗ trợ", message: "Chỉ nhận ảnh (PNG, JPG, WebP, GIF) hoặc video (MP4, MOV, WebM).", variant: "warning" }); return; }
    if (uploading) { if (fileInput.current) fileInput.current.value = ""; return; }
    const current = latestUrls.current;
    if (isVideo && mediaType !== "REEL") onMediaTypeChange("VIDEO");
    else if (isImage && (mediaType === "NONE" || mediaType === "LINK")) onMediaTypeChange(current.length >= 1 ? "ALBUM" : "IMAGE");
    else if (isImage && mediaType === "IMAGE" && current.length >= 1) onMediaTypeChange("ALBUM");
    setUploadingState(true); setProgress("Đang tải lên 0%");
    try {
      const res = await api.uploadMedia(file, (percent, loaded, total) => setProgress(`Đang tải lên ${percent}% (${loaded}/${total} MB)`));
      if (res.public_url) { onUrlsChange([...latestUrls.current, res.public_url]); toast("Đã tải tệp lên máy chủ.", "success"); }
    } catch (err) {
      alert({ title: "Tải tệp thất bại", message: err instanceof Error ? err.message : "Không xác định", variant: "danger" });
    } finally { setUploadingState(false); setProgress(""); if (fileInput.current) fileInput.current.value = ""; }
  };
  const addUrls = () => {
    const parts = urlText.split(/[\s,]+/).map(s => s.trim()).filter(Boolean);
    if (parts.length === 0) return;
    onUrlsChange(mediaType === "LINK" ? [parts[0]] : [...urls, ...parts]);
    setUrlText("");
  };
  const removeUrl = (index: number) => onUrlsChange(urls.filter((_, i) => i !== index));
  const isLink = mediaType === "LINK";
  const needsMedia = ["IMAGE", "ALBUM", "VIDEO", "REEL"].includes(mediaType);

  return <div className={`media-uploader${compact ? " compact" : ""}`}>
    <div className="composer-row-head"><span className="composer-label" id={`${idPrefix}-format-label`}>Định dạng</span>
      <div className="chip-row" role="group" aria-labelledby={`${idPrefix}-format-label`}>{mediaTypeOptions.map(opt => { const Icon = opt.icon; return <button key={opt.type} type="button" className="chip" aria-pressed={mediaType === opt.type} onClick={() => onMediaTypeChange(opt.type)}><Icon size={14} aria-hidden="true" />{opt.label}</button>; })}</div>
    </div>
    {mediaType !== "NONE" && <>
      {!isLink && <div className={`dropzone${dragOver ? " active" : ""}${uploading ? " busy" : ""}`} role="button" tabIndex={0} aria-label="Tải ảnh hoặc video lên" aria-disabled={uploading} aria-busy={uploading}
        onClick={() => { if (!uploading) fileInput.current?.click(); }} onKeyDown={e => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); if (!uploading) fileInput.current?.click(); } }}
        onDragEnter={e => { e.preventDefault(); setDragOver(true); }} onDragOver={e => { e.preventDefault(); setDragOver(true); }} onDragLeave={() => setDragOver(false)}
        onDrop={e => { e.preventDefault(); setDragOver(false); const file = e.dataTransfer.files?.[0]; if (file) handleFile(file); }}>
        <input ref={fileInput} type="file" className="hidden" accept="video/*,image/*" onChange={e => { const file = e.target.files?.[0]; if (file) handleFile(file); }} />
        <Upload size={18} aria-hidden="true" />
        <span>{uploading ? progress : compact ? "Kéo thả hoặc nhấn để chọn tệp" : "Kéo thả ảnh/video vào đây hoặc nhấn để chọn tệp"}</span>
        {!compact && <small>PNG, JPG, WebP, GIF hoặc MP4, MOV, WebM (video tới 500MB)</small>}
      </div>}
      <div className="url-entry">
        <label className="dispatch-field"><span className="sr-only">{isLink ? "Đường dẫn trang web" : "Đường dẫn ảnh hoặc video"}</span>
          <input id={`${idPrefix}-url`} type="url" inputMode="url" value={urlText} placeholder={isLink ? "https://trang-web.vn/bai-viet" : "https://cdn.example.com/anh.jpg"} onChange={e => setUrlText(e.target.value)} onKeyDown={e => { if (e.key === "Enter") { e.preventDefault(); addUrls(); } }} /></label>
        <button type="button" className="dispatch-button" onClick={addUrls} disabled={!urlText.trim()}>{isLink ? "Dùng liên kết" : "Thêm đường dẫn"}</button>
      </div>
      {urls.length > 0 && <ul className="media-grid" aria-label="Tệp đính kèm">{urls.map((url, i) => {
        const video = ["VIDEO", "REEL"].includes(mediaType) || /\.(mp4|mov|webm|m4v)(\?|$)/i.test(url);
        return <li key={`${url}-${i}`} className="media-thumb">
          {isLink ? <span className="media-link"><LinkIcon size={14} aria-hidden="true" />{url}</span> : video ? <video src={url} preload="metadata" muted /> : <img src={url} alt={`Tệp ${i + 1}`} loading="lazy" onError={e => { e.currentTarget.classList.add("broken"); }} />}
          <button type="button" className="media-remove" aria-label={`Bỏ tệp ${i + 1}`} onClick={() => removeUrl(i)}><X size={14} /></button>
        </li>;
      })}</ul>}
      {needsMedia && urls.length === 0 && !uploading && <p className="dispatch-note warning">Định dạng này cần ít nhất một tệp ảnh/video.</p>}
      {isLink && urls.length === 0 && <p className="dispatch-note warning">Dán đường dẫn trang web cần chia sẻ.</p>}
    </>}
  </div>;
}
