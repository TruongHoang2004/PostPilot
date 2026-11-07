"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Archive, BookOpen, CalendarClock, FileText, Pencil, Plus, RotateCcw, Search, Trash2 } from "lucide-react";
import { Channel, LibraryItem, Post } from "@/lib/types";
import { api } from "@/lib/api";
import { localDateKey, mediaLabels } from "@/lib/schedule";
import { useDialog } from "@/context/DialogContext";
import { useOrg } from "@/context/OrgContext";
import { LibraryEditor } from "./LibraryEditor";
import { LibrarySchedulerDialog } from "./LibrarySchedulerDialog";

const PAGE = 50;
const formatWhen = (iso?: string) => iso ? new Date(iso).toLocaleString("vi-VN", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" }) : "";
interface LibraryManagerProps { channels: Channel[]; posts: Post[]; refreshToken: number; onChanged: () => void }

/** Part 1 of the studio: ready-made posts. "Bài soạn sẵn" holds reusable content; "Đã đăng" holds items soft-deleted after publishing. */
export function LibraryManager({ channels, posts, refreshToken, onChanged }: LibraryManagerProps) {
  const { confirm, alert, toast } = useDialog();
  const { canCreatePost } = useOrg();
  const [view, setView] = useState<"active" | "archived">("active");
  const [channel, setChannel] = useState("");
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  const [items, setItems] = useState<LibraryItem[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [appending, setAppending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [editor, setEditor] = useState<{ item: LibraryItem | null } | null>(null);
  const [scheduling, setScheduling] = useState<LibraryItem | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const epoch = useRef(0);
  const loaded = useRef(0);
  const appendingRef = useRef(false);

  useEffect(() => { const timer = setTimeout(() => setDebounced(query), 250); return () => clearTimeout(timer); }, [query]);

  /**
   * user: filters changed (spinner, first page). periodic: 5s refresh, silent and skipped while a page is being
   * appended or extra pages are loaded (they would be dropped). append: "Tải thêm". Local actions patch the list
   * in place (see patchList) so they never depend on a reload.
   */
  const load = useCallback(async (mode: "user" | "periodic" | "append" = "user") => {
    if (mode === "periodic" && (appendingRef.current || loaded.current > PAGE)) return;
    const current = ++epoch.current;
    if (mode === "user") setLoading(true);
    if (mode === "append") { appendingRef.current = true; setAppending(true); }
    try {
      const page = await api.getLibrary({ archived: view === "archived", channel_id: channel || undefined, q: debounced, limit: PAGE, offset: mode === "append" ? loaded.current : 0 });
      if (current !== epoch.current) return;
      setItems(prev => mode === "append" ? [...prev, ...page.items] : page.items);
      loaded.current = mode === "append" ? loaded.current + page.items.length : page.items.length;
      setHasMore(page.has_more);
      setError(null);
    } catch (err) {
      if (current === epoch.current) setError(err instanceof Error ? err.message : "Không tải được bài soạn sẵn");
    } finally {
      if (current === epoch.current) setLoading(false);
      if (mode === "append") { appendingRef.current = false; setAppending(false); }
    }
  }, [view, channel, debounced]);

  const filterKey = `${view}|${channel}|${debounced}`;
  const lastFilterKey = useRef("");
  const loadRef = useRef(load);
  loadRef.current = load;
  useEffect(() => {
    if (lastFilterKey.current !== filterKey) {
      lastFilterKey.current = filterKey;
      // Never show the previous view's items (with this view's actions) while the next list loads.
      loaded.current = 0; setItems([]); setHasMore(false);
      void loadRef.current("user");
      return;
    }
    void loadRef.current("periodic");
  }, [filterKey, refreshToken]);
  useEffect(() => () => { epoch.current++; }, []);

  /** Applies a local change to the loaded pages without refetching (keeps "Tải thêm" pages intact). */
  const patchList = (change: { remove?: string; upsert?: LibraryItem }) => {
    setItems(prev => {
      let next = prev;
      if (change.remove) next = next.filter(i => i.id !== change.remove);
      if (change.upsert) {
        const saved = change.upsert;
        const savedArchived = Boolean(saved.archived_at);
        if (savedArchived !== (view === "archived") || (channel && saved.channel_id !== channel)) next = next.filter(i => i.id !== saved.id);
        else next = next.some(i => i.id === saved.id) ? next.map(i => i.id === saved.id ? saved : i) : [saved, ...next];
      }
      loaded.current = next.length;
      return next;
    });
    onChanged();
  };
  const remove = async (item: LibraryItem) => {
    const ok = await confirm({ title: "Xoá vĩnh viễn", message: `Xoá hẳn “${(item.title || item.content).slice(0, 60)}”? Các bài đã lên lịch từ mục này vẫn được giữ nguyên. Hành động này không thể hoàn tác.`, confirmText: "Xoá vĩnh viễn", variant: "danger" });
    if (!ok) return;
    setBusyId(item.id);
    try { await api.deleteLibraryItem(item.id); toast("Đã xoá bài soạn sẵn.", "success"); patchList({ remove: item.id }); }
    catch (err) { alert({ title: "Không xoá được", message: err instanceof Error ? err.message : "Không xác định", variant: "danger" }); void load("user"); }
    finally { setBusyId(null); }
  };
  const restore = async (item: LibraryItem) => {
    setBusyId(item.id);
    try { await api.restoreLibraryItem(item.id); toast("Đã giữ lại bài trong Bài soạn sẵn. Các bài đang chờ đăng sẽ không xoá nó nữa.", "success"); patchList({ remove: item.id }); }
    catch (err) { alert({ title: "Không khôi phục được", message: err instanceof Error ? err.message : "Không xác định", variant: "danger" }); void load("user"); }
    finally { setBusyId(null); }
  };

  const archived = view === "archived";
  return <section aria-label="Bài soạn sẵn">
    <div className="schedule-toolbar">
      <div className="schedule-range" role="group" aria-label="Mục bài soạn sẵn"><button aria-pressed={!archived} onClick={() => setView("active")}>Bài soạn sẵn</button><button aria-pressed={archived} onClick={() => setView("archived")}>Đã đăng</button></div>
      {canCreatePost && <button className="dispatch-button primary library-new" onClick={() => setEditor({ item: null })}><Plus size={16} aria-hidden="true" />Soạn bài mới</button>}
    </div>
    <div className="library-filters">
      <label className="dispatch-field">Fanpage<select aria-label="Fanpage" value={channel} onChange={event => setChannel(event.target.value)}><option value="">Tất cả Fanpage</option>{channels.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label className="dispatch-field schedule-search">Tìm bài viết<div><Search size={16} aria-hidden="true" /><input type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="Tiêu đề hoặc nội dung…" /></div></label>
    </div>
    {archived && <p className="dispatch-note library-archive-note"><Archive size={14} aria-hidden="true" />Những bài được đặt “xoá sau khi đăng” nằm ở đây sau khi đăng thành công. Giữ lại để dùng lần nữa, hoặc xoá vĩnh viễn nếu không cần.</p>}
    {error && <div className="dispatch-error" role="alert">{error}<button onClick={() => void load("user")}>Thử lại</button></div>}
    {loading && items.length === 0 ? <div className="schedule-empty" role="status" aria-busy="true">Đang tải bài soạn sẵn…</div>
      : items.length === 0 ? <div className="schedule-empty">{archived ? <Archive size={27} aria-hidden="true" /> : <BookOpen size={27} aria-hidden="true" />}<h3>{archived ? "Chưa có bài nào trong mục Đã đăng." : "Chưa có bài soạn sẵn."}</h3><p>{archived ? "Khi bạn đặt “xoá sau khi đăng” lúc lên lịch, bài sẽ xuất hiện ở đây sau khi đăng xong." : "Soạn trước nội dung bạn hay dùng, rồi chọn bài và giờ đăng ngay trên lịch."}</p>{!archived && canCreatePost && <button className="dispatch-button" onClick={() => setEditor({ item: null })}>Soạn bài đầu tiên</button>}</div>
      : <ul className="library-grid" role="list">{items.map(item => {
        const preview = ["IMAGE", "ALBUM"].includes(item.media_type) ? item.media_urls?.[0] : undefined;
        const busy = busyId === item.id || loading;
        const itemArchived = Boolean(item.archived_at);
        const label = (item.title || item.content || mediaLabels[item.media_type] || "bài viết").slice(0, 50);
        return <li key={item.id}><article className={`library-card${itemArchived ? " archived" : ""}`}>
          <div className="library-card-media">{preview ? <img src={preview} alt="" loading="lazy" onError={event => { event.currentTarget.style.display = "none"; }} /> : <FileText size={20} aria-hidden="true" />}</div>
          <div className="library-card-body">
            <h2>{item.title || item.content.slice(0, 70) || mediaLabels[item.media_type]}</h2>
            {item.title && <p className="library-card-text">{item.content}</p>}
            {!item.title && item.content.length > 70 && <p className="library-card-text">{item.content.slice(70)}</p>}
            <p className="library-card-meta"><span className="chip static">{mediaLabels[item.media_type] || item.media_type}</span><span>{item.channel_name || "Mọi Fanpage"}</span>{item.publish_count > 0 && <span>Đã đăng {item.publish_count} lần{item.last_published_at ? ` · gần nhất ${formatWhen(item.last_published_at)}` : ""}</span>}</p>
            {itemArchived && <p className="library-card-archived">Chuyển vào Đã đăng lúc {formatWhen(item.archived_at)}</p>}
          </div>
          <div className="library-card-actions">
            {itemArchived ? <>
              {canCreatePost && <button className="dispatch-button" disabled={busy} aria-label={`Giữ lại bài ${label}`} onClick={() => restore(item)}><RotateCcw size={15} aria-hidden="true" />Giữ lại</button>}
              {canCreatePost && <button className="dispatch-button danger" disabled={busy} aria-label={`Xoá vĩnh viễn bài ${label}`} onClick={() => remove(item)}><Trash2 size={15} aria-hidden="true" />Xoá vĩnh viễn</button>}
            </> : <>
              {canCreatePost && <button className="dispatch-button primary" disabled={busy} aria-label={`Lên lịch bài ${label}`} onClick={() => setScheduling(item)}><CalendarClock size={15} aria-hidden="true" />Lên lịch</button>}
              {canCreatePost && <button className="dispatch-button" disabled={busy} aria-label={`Sửa bài ${label}`} onClick={() => setEditor({ item })}><Pencil size={15} aria-hidden="true" />Sửa</button>}
              {canCreatePost && <button className="dispatch-icon-button" disabled={busy} onClick={() => remove(item)} aria-label={`Xoá bài ${label}`}><Trash2 size={16} aria-hidden="true" /></button>}
            </>}
          </div>
        </article></li>;
      })}</ul>}
    {hasMore && <div className="schedule-more"><button className="dispatch-button" onClick={() => void load("append")} disabled={loading || appending} aria-busy={appending}>{appending ? "Đang tải…" : "Tải thêm"}</button></div>}
    {editor && <LibraryEditor item={editor.item} channels={channels} defaultChannelId={channel} onClose={() => setEditor(null)} onSaved={saved => { setEditor(null); patchList({ upsert: saved }); }} />}
    {scheduling && <LibrarySchedulerDialog dateKey={localDateKey(new Date())} channels={channels} posts={posts} initialItem={scheduling} onClose={() => setScheduling(null)} onDone={() => { setScheduling(null); onChanged(); }} />}
  </section>;
}
