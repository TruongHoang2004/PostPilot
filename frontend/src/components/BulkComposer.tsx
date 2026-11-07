"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, ArrowDown, ArrowUp, ChevronDown, ChevronUp, Clock, Copy, ListPlus, Plus, Trash2, Wand2, X } from "lucide-react";
import { BulkItemInput, BulkPreview, BulkRequest, Channel, MediaType, ScheduleSlot } from "@/lib/types";
import { api } from "@/lib/api";
import { describeNote, formatDateTimeShort, formatDayLong, formatTime, fromLocalInputValue, localDateKey, toggleWeekday, validateMedia, weekdayShort } from "@/lib/schedule";
import { loadSchedulePrefs, saveSchedulePrefs, SchedulePrefs } from "@/lib/preferences";
import { useDialog } from "@/context/DialogContext";
import { useOrg } from "@/context/OrgContext";
import { TargetPicker, TargetSelection, defaultTarget, reconcileTarget, targetChannelIds, targetIsComplete, targetPayload } from "./TargetPicker";
import { MediaUploader } from "./MediaUploader";
import { Modal } from "./Modal";
import { GapSelect, RetrySelect, TimeSlotEditor } from "./TimeSlotEditor";

interface BulkItem { key: string; content: string; mediaType: MediaType; urls: string[]; explicitAt: string; open: boolean }
interface BulkComposerProps { channels: Channel[]; onSuccess: () => void }
type TimelineRow = ScheduleSlot & { item: BulkItem | undefined };

let keySeed = 0;
const newItem = (content = ""): BulkItem => ({ key: `item-${Date.now()}-${keySeed++}`, content, mediaType: "NONE", urls: [], explicitAt: "", open: true });
const itemReady = (item: BulkItem) => item.content.trim() !== "" || item.urls.length > 0;
const splitChunks = (text: string) => text.split(/\n\s*\n/).map(s => s.trim()).filter(Boolean);

/** Bulk composer: a series of posts, shared targets, a distribution plan and a server-computed preview before committing. */
export function BulkComposer({ channels, onSuccess }: BulkComposerProps) {
  const { confirm, alert, toast } = useDialog();
  const { channelGroups, canCreatePost, currentOrg } = useOrg();
  const orgId = currentOrg?.id || "";
  const [name, setName] = useState("");
  const [target, setTarget] = useState<TargetSelection>(() => reconcileTarget(defaultTarget([], channelGroups), channels, channelGroups, true));
  const [items, setItems] = useState<BulkItem[]>(() => [newItem(), newItem()]);
  const [prefs, setPrefs] = useState<SchedulePrefs>(() => loadSchedulePrefs(orgId));
  const [startDate, setStartDate] = useState(() => { const d = new Date(); d.setDate(d.getDate() + 1); return localDateKey(d); });
  const [maxRetries, setMaxRetries] = useState(3);
  const [preview, setPreview] = useState<{ loading: boolean; data: BulkPreview | null; error: string | null }>({ loading: false, data: null, error: null });
  const [submitting, setSubmitting] = useState(false);
  const [uploadingKeys, setUploadingKeys] = useState<Set<string>>(() => new Set());
  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState("");
  const previewEpoch = useRef(0);
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;

  useEffect(() => { setPrefs(loadSchedulePrefs(orgId)); }, [orgId]);
  useEffect(() => { setTarget(prev => reconcileTarget(prev, channels, channelGroups, true)); }, [channels, channelGroups]);
  const updatePrefs = (next: SchedulePrefs) => { setPrefs(next); saveSchedulePrefs(orgId, next); };
  const channelIds = useMemo(() => targetChannelIds(target, channels, channelGroups), [target, channels, channelGroups]);
  const setUploading = (key: string, up: boolean) => setUploadingKeys(prev => { const next = new Set(prev); if (up) next.add(key); else next.delete(key); return next; });

  const updateItem = (key: string, patch: Partial<BulkItem>) => setItems(prev => prev.map(item => item.key === key ? { ...item, ...patch } : item));
  const removeItem = (key: string) => { setItems(prev => prev.length > 1 ? prev.filter(item => item.key !== key) : prev); setUploading(key, false); };
  const duplicateItem = (key: string) => setItems(prev => { const i = prev.findIndex(item => item.key === key); if (i < 0) return prev; const copy = { ...newItem(prev[i].content), mediaType: prev[i].mediaType, urls: [...prev[i].urls] }; return [...prev.slice(0, i + 1), copy, ...prev.slice(i + 1)]; });
  const moveItem = (key: string, dir: -1 | 1) => setItems(prev => { const i = prev.findIndex(item => item.key === key); const j = i + dir; if (i < 0 || j < 0 || j >= prev.length) return prev; const next = [...prev]; [next[i], next[j]] = [next[j], next[i]]; return next; });
  const importItems = () => {
    const chunks = splitChunks(importText);
    if (chunks.length === 0) return;
    setItems(prev => [...prev.filter(itemReady), ...chunks.map(chunk => newItem(chunk))]);
    setImportText(""); setImportOpen(false);
    toast(`Đã thêm ${chunks.length} bài từ văn bản.`, "success");
  };

  const buildPayload = useCallback((): BulkRequest | null => {
    const ready = items.filter(itemReady);
    if (ready.length === 0 || !targetIsComplete(target)) return null;
    const payloadItems: BulkItemInput[] = ready.map(item => {
      const when = fromLocalInputValue(item.explicitAt);
      return { content: item.content.trim(), media_type: item.mediaType, link_url: item.mediaType === "LINK" ? item.urls[0] || "" : "", media_urls: item.mediaType !== "LINK" ? item.urls : [], scheduled_at: when ? when.toISOString() : undefined };
    });
    return {
      name: name.trim() || undefined, ...targetPayload(target), items: payloadItems, max_retries: maxRetries,
      schedule_plan: { start_date: startDate, timezone, time_slots: prefs.timeSlots, weekdays: prefs.weekdays, min_gap_minutes: prefs.minGapMinutes, avoid_conflicts: prefs.avoidConflicts },
    };
  }, [items, target, name, maxRetries, startDate, timezone, prefs]);

  // Server preview, debounced and keyed on the serialised payload so unrelated re-renders (auto-refresh) never re-fire it.
  const payloadKey = useMemo(() => JSON.stringify(buildPayload()), [buildPayload]);
  useEffect(() => {
    const epoch = ++previewEpoch.current;
    if (payloadKey === "null") { setPreview({ loading: false, data: null, error: null }); return; }
    setPreview(prev => ({ ...prev, loading: true }));
    const timer = setTimeout(async () => {
      try { const data = await api.previewBulk(JSON.parse(payloadKey) as BulkRequest); if (epoch === previewEpoch.current) setPreview({ loading: false, data, error: null }); }
      catch (err) { if (epoch === previewEpoch.current) setPreview({ loading: false, data: null, error: err instanceof Error ? err.message : "Không tính được lịch" }); }
    }, 600);
    return () => clearTimeout(timer);
  }, [payloadKey]);

  const readyItems = items.filter(itemReady);
  const slotFor = (index: number) => preview.data?.schedule.find(s => s.index === index);
  const submit = async (submitForApproval: boolean) => {
    const payload = buildPayload();
    if (!canCreatePost) { alert({ title: "Chưa thể gửi", message: "Vai trò hiện tại không được tạo bài.", variant: "warning" }); return; }
    if (!payload) { alert({ title: "Chưa thể gửi", message: "Cần ít nhất một bài có nội dung và một Fanpage đích.", variant: "warning" }); return; }
    if (uploadingKeys.size > 0) { alert({ title: "Tệp đang tải lên", message: "Đợi tải lên xong rồi gửi.", variant: "warning" }); return; }
    if (readyItems.length > 100) { alert({ title: "Quá nhiều bài", message: "Tối đa 100 bài mỗi lần.", variant: "warning" }); return; }
    for (const item of readyItems) {
      const problem = validateMedia(item.mediaType, item.urls, item.content);
      if (problem) { alert({ title: `Bài ${items.indexOf(item) + 1} chưa hợp lệ`, message: problem, variant: "warning" }); return; }
    }
    const total = readyItems.length * Math.max(channelIds.length, 1);
    const ok = await confirm({ title: submitForApproval ? "Gửi duyệt hàng loạt" : "Lên lịch hàng loạt", message: `Tạo ${total} bài (${readyItems.length} nội dung × ${Math.max(channelIds.length, 1)} Fanpage) theo lịch đã xem trước?`, confirmText: submitForApproval ? "Gửi duyệt" : "Lên lịch", variant: "info" });
    if (!ok) return;
    setSubmitting(true);
    try {
      const res = await api.createBulk({ ...payload, submit_for_approval: submitForApproval });
      toast(`Đã tạo ${res.count} bài${res.name ? ` trong lô “${res.name}”` : ""}.`, "success");
      setItems([newItem(), newItem()]); setName(""); setUploadingKeys(new Set());
      onSuccess();
    } catch (err) {
      alert({ title: "Không tạo được lô bài", message: err instanceof Error ? err.message : "Không xác định", variant: "danger" });
    } finally { setSubmitting(false); }
  };

  const timeline = useMemo((): [string, TimelineRow[]][] => {
    const rows: TimelineRow[] = (preview.data?.schedule || []).map(slot => ({ ...slot, item: readyItems[slot.index] })).sort((a, b) => new Date(a.scheduled_at).getTime() - new Date(b.scheduled_at).getTime());
    const groups = new Map<string, TimelineRow[]>();
    rows.forEach(row => { const d = new Date(row.scheduled_at); const key = Number.isNaN(d.getTime()) ? "?" : formatDayLong(d); groups.set(key, [...(groups.get(key) || []), row]); });
    return Array.from(groups.entries());
  }, [preview.data, readyItems]);
  const busy = submitting || !canCreatePost || !preview.data || preview.loading;
  const warnings = (preview.data?.warning_codes?.length
    ? preview.data.warning_codes.map((w, i) => describeNote(w.code, w.value, preview.data?.warnings?.[i]))
    : (preview.data?.warnings || [])).filter(Boolean);

  return <div className="bulk-layout">
    <div className="bulk-items">
      {!canCreatePost && <p className="dispatch-note warning">Bạn chỉ có quyền xem. Liên hệ quản trị viên để được cấp quyền tạo bài.</p>}
      <div className="dispatch-panel bulk-head">
        <label className="dispatch-field">Tên lô bài (tuỳ chọn)<input value={name} onChange={e => setName(e.target.value)} placeholder="Ví dụ: Chiến dịch tháng 12" maxLength={255} /></label>
        <div className="bulk-head-actions">
          <button type="button" className="dispatch-button" onClick={() => setImportOpen(true)}><ListPlus size={16} />Tách từ văn bản</button>
          <button type="button" className="dispatch-button" onClick={() => setItems(prev => [...prev, newItem()])}><Plus size={16} />Thêm bài</button>
        </div>
      </div>
      <ol className="bulk-list" aria-label="Danh sách bài viết">{items.map((item, index) => <BulkItemCard key={item.key} item={item} index={index} total={items.length} slot={itemReady(item) ? slotFor(readyItems.indexOf(item)) : undefined} previewLoading={preview.loading}
        onChange={patch => updateItem(item.key, patch)} onMove={dir => moveItem(item.key, dir)} onDuplicate={() => duplicateItem(item.key)} onRemove={() => removeItem(item.key)} onUploading={up => setUploading(item.key, up)} />)}</ol>
    </div>
    <aside className="bulk-sidebar">
      <section className="dispatch-panel bulk-card" aria-label="Fanpage đích"><TargetPicker channels={channels} channelGroups={channelGroups} value={target} onChange={setTarget} idPrefix="bulk" /></section>
      <section className="dispatch-panel bulk-card" aria-label="Kế hoạch thời gian">
        <div className="composer-row-head"><span className="composer-label"><Wand2 size={15} aria-hidden="true" />Phân bổ thời gian</span><span className="dispatch-muted text-xs">{timezone}</span></div>
        <label className="dispatch-field">Bắt đầu từ ngày<input type="date" value={startDate} min={localDateKey(new Date())} onChange={e => { if (e.target.value) setStartDate(e.target.value); }} /></label>
        <div className="composer-row-head"><span className="composer-label" id="bulk-weekdays-label">Ngày trong tuần</span><button type="button" className="text-xs dispatch-muted underline" onClick={() => updatePrefs({ ...prefs, weekdays: [] })}>Mọi ngày</button></div>
        <div className="chip-row" role="group" aria-labelledby="bulk-weekdays-label">{[1, 2, 3, 4, 5, 6, 0].map(day => { const active = prefs.weekdays.length === 0 || prefs.weekdays.includes(day); return <button key={day} type="button" className="chip" aria-pressed={active} onClick={() => updatePrefs({ ...prefs, weekdays: toggleWeekday(prefs.weekdays, day) })}>{weekdayShort[day]}</button>; })}</div>
        <div className="composer-row-head"><span className="composer-label">Khung giờ mỗi ngày</span></div>
        <TimeSlotEditor idPrefix="bulk" prefs={prefs} onChange={updatePrefs} showGap={false} />
        <div className="bulk-plan-row">
          <GapSelect prefs={prefs} onChange={updatePrefs} label="Cách nhau tối thiểu" />
          <label className="check-item inline"><input type="checkbox" checked={prefs.avoidConflicts} onChange={e => updatePrefs({ ...prefs, avoidConflicts: e.target.checked })} /><span>Tránh trùng lịch đã có</span></label>
        </div>
        <RetrySelect value={maxRetries} onChange={setMaxRetries} />
      </section>
      <section className="dispatch-panel bulk-card bulk-timeline" aria-label="Lịch dự kiến">
        <div className="composer-row-head"><span className="composer-label">Lịch dự kiến</span><span className="dispatch-muted text-xs" aria-live="polite">{preview.loading ? "Đang tính…" : preview.data ? `${preview.data.post_count} bài` : ""}</span></div>
        {preview.error && <p className="dispatch-note warning" role="alert"><AlertTriangle size={14} aria-hidden="true" />{preview.error}</p>}
        {!preview.error && !preview.data && !preview.loading && <p className="dispatch-note">Thêm nội dung và chọn Fanpage để xem lịch.</p>}
        {warnings.map((w, i) => <p key={`${i}-${w}`} className="dispatch-note warning"><AlertTriangle size={14} aria-hidden="true" />{w}</p>)}
        {timeline.map(([day, rows]) => <div key={day} className="timeline-day"><h3>{day}</h3>{rows.map(row => <div key={row.index} className="timeline-row"><time>{formatTime(new Date(row.scheduled_at))}</time><span>Bài {row.item ? items.indexOf(row.item) + 1 : row.index + 1}{row.explicit ? " · giờ riêng" : ""}</span><small>{row.item?.content.trim().slice(0, 36) || "tệp đính kèm"}</small></div>)}</div>)}
        {preview.data && preview.data.channels.some(c => c.require_approval && c.role === "EDITOR") && <p className="dispatch-note warning">Một số Fanpage yêu cầu duyệt với vai trò của bạn: các bài đó sẽ vào hộp phê duyệt.</p>}
      </section>
      <div className="composer-actions">
        <button type="button" className="dispatch-button" disabled={busy} onClick={() => submit(true)}><Clock size={16} />Gửi duyệt tất cả</button>
        <button type="button" className="dispatch-button primary" disabled={busy} onClick={() => submit(false)}>{submitting ? "Đang tạo…" : `Lên lịch ${preview.data ? preview.data.post_count : ""} bài`}</button>
      </div>
    </aside>
    {importOpen && <Modal labelledBy="bulk-import-heading" onClose={() => setImportOpen(false)}><section className="dispatch-panel bulk-import">
      <div className="dispatch-section-heading"><h2 id="bulk-import-heading">Tách bài từ văn bản</h2><button className="dispatch-icon-button" onClick={() => setImportOpen(false)} aria-label="Đóng"><X size={20} /></button></div>
      <p className="dispatch-muted text-sm">Dán nhiều bài, mỗi bài cách nhau một dòng trống. Mỗi đoạn sẽ thành một bài trong danh sách.</p>
      <textarea rows={10} value={importText} onChange={e => setImportText(e.target.value)} aria-label="Văn bản chứa nhiều bài" placeholder={"Bài thứ nhất…\n\nBài thứ hai…"} />
      <div className="composer-actions"><button type="button" className="dispatch-button" onClick={() => setImportOpen(false)}>Huỷ</button><button type="button" className="dispatch-button primary" disabled={!importText.trim()} onClick={importItems}>Thêm {splitChunks(importText).length || ""} bài</button></div>
    </section></Modal>}
  </div>;
}

interface BulkItemCardProps {
  item: BulkItem; index: number; total: number; slot: ScheduleSlot | undefined; previewLoading: boolean;
  onChange: (patch: Partial<BulkItem>) => void; onMove: (dir: -1 | 1) => void; onDuplicate: () => void; onRemove: () => void; onUploading: (up: boolean) => void;
}

function BulkItemCard({ item, index, total, slot, previewLoading, onChange, onMove, onDuplicate, onRemove, onUploading }: BulkItemCardProps) {
  const label = `bài ${index + 1}`;
  const ready = itemReady(item);
  return <li className={`dispatch-panel bulk-item${ready ? "" : " empty"}`}>
    <div className="bulk-item-head">
      <button type="button" className="bulk-item-toggle" aria-expanded={item.open} onClick={() => onChange({ open: !item.open })}>{item.open ? <ChevronUp size={16} /> : <ChevronDown size={16} />}<strong>Bài {index + 1}</strong><span className="bulk-item-snippet">{item.content.trim().split("\n")[0]?.slice(0, 60) || (item.urls.length ? `${item.urls.length} tệp` : "Chưa có nội dung")}</span></button>
      <span className={`bulk-item-time${slot?.explicit ? " explicit" : ""}`}><Clock size={13} aria-hidden="true" />{slot ? formatDateTimeShort(new Date(slot.scheduled_at)) : ready ? (previewLoading ? "đang tính…" : "—") : "bỏ qua"}</span>
      <div className="bulk-item-tools">
        <button type="button" className="dispatch-icon-button" aria-label={`Chuyển ${label} lên`} disabled={index === 0} onClick={() => onMove(-1)}><ArrowUp size={15} /></button>
        <button type="button" className="dispatch-icon-button" aria-label={`Chuyển ${label} xuống`} disabled={index === total - 1} onClick={() => onMove(1)}><ArrowDown size={15} /></button>
        <button type="button" className="dispatch-icon-button" aria-label={`Nhân đôi ${label}`} onClick={onDuplicate}><Copy size={15} /></button>
        <button type="button" className="dispatch-icon-button" aria-label={`Xoá ${label}`} disabled={total <= 1} onClick={onRemove}><Trash2 size={15} /></button>
      </div>
    </div>
    {item.open && <div className="bulk-item-body">
      <textarea aria-label={`Nội dung bài ${index + 1}`} rows={4} value={item.content} onChange={e => onChange({ content: e.target.value })} placeholder="Nội dung bài viết…" />
      <MediaUploader idPrefix={`bulk-${item.key}`} compact mediaType={item.mediaType} onMediaTypeChange={type => onChange({ mediaType: type })} urls={item.urls} onUrlsChange={urls => onChange({ urls })} onUploadingChange={onUploading} />
      <div className="bulk-item-when">
        <label className="dispatch-field">Giờ riêng cho bài này<input type="datetime-local" value={item.explicitAt} onChange={e => onChange({ explicitAt: e.target.value })} /></label>
        {item.explicitAt ? <button type="button" className="dispatch-button" onClick={() => onChange({ explicitAt: "" })}><X size={14} />Theo kế hoạch chung</button> : <span className="dispatch-muted text-xs">Để trống để xếp theo khung giờ chung bên phải.</span>}
      </div>
      {slot?.note_code && <p className="dispatch-note warning"><AlertTriangle size={14} aria-hidden="true" />{describeNote(slot.note_code, slot.note_value, slot.note)}</p>}
    </div>}
  </li>;
}
