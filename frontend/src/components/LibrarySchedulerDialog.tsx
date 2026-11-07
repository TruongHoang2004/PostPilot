"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { BookOpen, CalendarClock, FileText, Search, Trash2, X } from "lucide-react";
import { Channel, LibraryItem, Post } from "@/lib/types";
import { api } from "@/lib/api";
import { dateFromKey, defaultSlotForDay, formatDayLong, fromLocalInputValue, mediaLabels, toLocalInputValue } from "@/lib/schedule";
import { loadSchedulePrefs, saveSchedulePrefs, SchedulePrefs } from "@/lib/preferences";
import { useDialog } from "@/context/DialogContext";
import { useOrg } from "@/context/OrgContext";
import { Modal } from "./Modal";
import { SchedulePicker } from "./SchedulePicker";
import { TargetPicker, TargetSelection, defaultTarget, reconcileTarget, targetChannelIds, targetIsComplete, targetPayload } from "./TargetPicker";

interface LibrarySchedulerDialogProps {
  dateKey: string;                        // the calendar day that was clicked
  channels: Channel[];
  posts: Post[];                          // loaded posts: free-slot suggestions and conflict hints
  initialItem?: LibraryItem | null;       // pre-selected item (opened from the library tab)
  onClose: () => void;
  onDone: () => void;
  onOpenLibrary?: () => void;
}

const snippet = (item: LibraryItem) => (item.title || item.content || mediaLabels[item.media_type] || "Bài viết").trim();

/** Calendar click flow: pick a library post, confirm the time and Fanpages, optionally soft-delete it after publishing. */
export function LibrarySchedulerDialog({ dateKey, channels, posts, initialItem, onClose, onDone, onOpenLibrary }: LibrarySchedulerDialogProps) {
  const { alert, toast } = useDialog();
  const { channelGroups, canCreatePost, channelRoles, canManageOrg, currentOrg } = useOrg();
  const orgId = currentOrg?.id || "";
  const [items, setItems] = useState<LibraryItem[]>(initialItem ? [initialItem] : []);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<LibraryItem | null>(initialItem || null);   // kept apart from the search results
  const [target, setTarget] = useState<TargetSelection>(() => {
    const base = defaultTarget(channels, channelGroups);
    return initialItem?.channel_id && channels.some(c => c.id === initialItem.channel_id) ? { ...base, mode: "CHANNEL", channelId: initialItem.channel_id } : base;
  });
  const [prefs, setPrefs] = useState<SchedulePrefs>(() => loadSchedulePrefs(orgId));
  const [scheduledAt, setScheduledAt] = useState(() => {
    const p = loadSchedulePrefs(orgId);
    const ids = initialItem?.channel_id ? [initialItem.channel_id] : [];
    return toLocalInputValue(defaultSlotForDay(dateKey, p.timeSlots, new Date(), posts, ids, p.minGapMinutes));
  });
  const timeTouched = useRef(false);   // once the user edits the time, a changed target no longer re-suggests it
  const [deleteAfter, setDeleteAfter] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => { setTarget(prev => reconcileTarget(prev, channels, channelGroups)); }, [channels, channelGroups]);
  // Server-side search (debounced); the initial item stays pinned so the library tab can open any item.
  useEffect(() => {
    let cancelled = false;
    const timer = setTimeout(async () => {
      setLoading(true);
      try {
        const page = await api.getLibrary({ q: search, limit: 100 });
        if (cancelled) return;
        setItems(initialItem && !search.trim() && !page.items.some(i => i.id === initialItem.id) ? [initialItem, ...page.items] : page.items);
        setHasMore(page.has_more);
        setLoadError(null);
      } catch (err) {
        if (!cancelled) setLoadError(err instanceof Error ? err.message : "Không tải được bài soạn sẵn");
      } finally { if (!cancelled) setLoading(false); }
    }, search ? 250 : 0);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [search, initialItem]);

  const channelIds = useMemo(() => targetChannelIds(target, channels, channelGroups), [target, channels, channelGroups]);
  const selectedGroup = channelGroups.find(g => g.id === target.groupId);
  const approvalChannel = (id: string) => channelGroups.some(g => g.require_approval && g.channels?.some(c => c.id === id));
  const isEditorOnApprovalGroup = !canManageOrg && channelIds.some(id => channelRoles[id] === "EDITOR" && ((target.mode === "GROUP" && selectedGroup?.require_approval) || approvalChannel(id)));
  const channelKey = channelIds.join(",");
  useEffect(() => {
    if (timeTouched.current) return;
    setScheduledAt(toLocalInputValue(defaultSlotForDay(dateKey, prefs.timeSlots, new Date(), posts, channelIds, prefs.minGapMinutes)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelKey, dateKey]);
  const when = fromLocalInputValue(scheduledAt);
  const requestClose = () => { if (!submitting) onClose(); };   // never hide an in-flight request (double scheduling)
  const updatePrefs = (next: SchedulePrefs) => { setPrefs(next); saveSchedulePrefs(orgId, next); };

  const choose = (item: LibraryItem) => {
    setSelected(item);
    if (item.channel_id && channels.some(c => c.id === item.channel_id)) setTarget(prev => ({ ...prev, mode: "CHANNEL", channelId: item.channel_id! }));
  };

  const submit = async () => {
    const problem = !canCreatePost ? "Vai trò hiện tại chỉ được xem, không lên lịch bài."
      : !selected ? "Chọn một bài soạn sẵn để lên lịch."
      : !targetIsComplete(target) ? "Chọn Fanpage hoặc nhóm Fanpage để đăng."
      : !when ? "Chọn ngày và giờ đăng."
      : when.getTime() < Date.now() ? "Giờ đăng đã qua. Hãy chọn một thời điểm trong tương lai."
      : null;
    if (problem || !selected || !when) { alert({ title: "Chưa thể lên lịch", message: problem || "Thiếu thông tin", variant: "warning" }); return; }
    setSubmitting(true);
    try {
      const res = await api.scheduleLibraryItem(selected.id, { ...targetPayload(target), scheduled_at: when.toISOString(), delete_after_publish: deleteAfter, submit_for_approval: isEditorOnApprovalGroup });
      const at = when.toLocaleString("vi-VN", { weekday: "short", day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
      toast(isEditorOnApprovalGroup ? `Đã gửi ${res.count} bài vào hộp phê duyệt.` : `Đã lên lịch ${res.count} bài lúc ${at}.${deleteAfter ? " Sau khi đăng, bài soạn sẵn sẽ chuyển sang mục Đã đăng." : ""}`, "success");
      onDone();
    } catch (err) {
      alert({ title: "Không lên lịch được", message: err instanceof Error ? err.message : "Không xác định", variant: "danger" });
    } finally { setSubmitting(false); }
  };

  return <Modal labelledBy="library-schedule-heading" onClose={requestClose} dismissOnBackdrop={false}>
    <section className="dispatch-panel library-dialog">
      <div className="dispatch-section-heading"><div><h2 id="library-schedule-heading">Lên lịch bài soạn sẵn</h2><p className="dispatch-muted text-sm mt-1">Ngày {formatDayLong(dateFromKey(dateKey))} · chọn bài, giờ đăng và Fanpage.</p></div><button type="button" className="dispatch-icon-button" onClick={requestClose} disabled={submitting} aria-label="Đóng"><X size={20} /></button></div>
      <div className="library-dialog-body">
        <div className="library-picker">
          <label className="dispatch-field library-search">Tìm bài soạn sẵn<div><Search size={16} aria-hidden="true" /><input type="search" value={search} onChange={event => setSearch(event.target.value)} placeholder="Tiêu đề hoặc nội dung…" /></div></label>
          {loadError && <p className="dispatch-error" role="alert">{loadError}</p>}
          {loading && items.length === 0 && <p className="dispatch-muted text-sm" role="status">Đang tải…</p>}
          {!loading && items.length === 0 && !loadError && <div className="library-options-empty"><BookOpen size={22} aria-hidden="true" /><p>{search ? "Không có bài phù hợp." : "Chưa có bài soạn sẵn nào."}</p>{!search && onOpenLibrary && <button type="button" className="dispatch-button" onClick={onOpenLibrary}>Soạn bài đầu tiên</button>}</div>}
          {items.length > 0 && <ul className="library-options" aria-label="Bài soạn sẵn">
            {items.map(item => <li key={item.id}><button type="button" aria-pressed={item.id === selected?.id} className="library-option" onClick={() => choose(item)}>
              <strong>{snippet(item).slice(0, 80)}</strong>
              <span>{(item.title ? item.content : "").slice(0, 100)}</span>
              <small>{mediaLabels[item.media_type] || item.media_type} · {item.channel_name || "Mọi Fanpage"}{item.publish_count > 0 ? ` · đã đăng ${item.publish_count} lần` : ""}</small>
            </button></li>)}
          </ul>}
          {hasMore && <p className="dispatch-note">Chỉ hiển thị 100 bài đầu tiên — hãy gõ từ khoá để thu hẹp danh sách.</p>}
        </div>
        <form className="library-dialog-form" onSubmit={event => { event.preventDefault(); void submit(); }} aria-label="Thiết lập lịch đăng">
          {selected ? <div className="library-preview"><p className="dispatch-eyebrow">Bài đã chọn</p><p className="library-preview-content">{selected.content || mediaLabels[selected.media_type]}</p>
            {["IMAGE", "ALBUM"].includes(selected.media_type) && selected.media_urls?.[0] && <img src={selected.media_urls[0]} alt="" className="library-preview-media" loading="lazy" onError={event => { event.currentTarget.style.display = "none"; }} />}</div>
            : <p className="dispatch-note"><FileText size={14} aria-hidden="true" />Chọn một bài trong danh sách.</p>}
          <TargetPicker channels={channels} channelGroups={channelGroups} value={target} onChange={setTarget} idPrefix="library-schedule" />
          <SchedulePicker id="library-schedule-time" label="Giờ đăng" pastNote="Giờ đã qua. Hãy chọn một thời điểm trong tương lai." value={scheduledAt} onChange={v => { timeTouched.current = true; setScheduledAt(v); }} posts={posts} channelIds={channelIds} prefs={prefs} onPrefsChange={updatePrefs} allowEmpty={false} />
          <label className="library-delete-option"><input type="checkbox" checked={deleteAfter} onChange={event => setDeleteAfter(event.target.checked)} /><span><strong><Trash2 size={14} aria-hidden="true" />Xoá sau khi đăng</strong><small>Xoá mềm: sau khi đăng thành công, bài soạn sẵn chuyển sang mục “Đã đăng” để bạn quyết định giữ lại hay xoá hẳn.</small></span></label>
          {isEditorOnApprovalGroup && <p className="dispatch-note warning">Fanpage này cần phê duyệt: bài sẽ được gửi vào hộp phê duyệt thay vì lên lịch ngay.</p>}
          <div className="composer-actions"><button type="button" className="dispatch-button" onClick={requestClose} disabled={submitting}>Huỷ</button><button className="dispatch-button primary" disabled={submitting || !selected || !canCreatePost}><CalendarClock size={16} aria-hidden="true" />{submitting ? "Đang lên lịch…" : isEditorOnApprovalGroup ? "Gửi duyệt" : "Lên lịch"}</button></div>
        </form>
      </div>
    </section>
  </Modal>;
}
