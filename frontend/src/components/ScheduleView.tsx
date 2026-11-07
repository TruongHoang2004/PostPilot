"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowRight, CalendarClock, CalendarDays, ChevronLeft, ChevronRight, FileText, List, Search } from "lucide-react";
import { Channel, Post } from "@/lib/types";
import { api } from "@/lib/api";
import { dateFromKey, formatDayLong, formatTime, groupPostsByDay, localDateKey, MAX_POST_QUERY_LIMIT, mediaLabels, postDate, postStatusLabels, rangeBounds, ScheduleRange, selectSchedulePosts, shiftMonth, STATUS_QUICK_FILTERS, willDeleteAfterPublish } from "@/lib/schedule";
import { useOrg } from "@/context/OrgContext";
import { PostDetailModal } from "./PostDetailModal";
import { PostStatusBadge } from "./PostStatusBadge";
import { CalendarWeek } from "./CalendarWeek";
import { CalendarMonth } from "./CalendarMonth";
import { LibrarySchedulerDialog } from "./LibrarySchedulerDialog";

const PAGE = 200;
interface ScheduleViewProps { channels: Channel[]; refreshToken: number; onRefresh: () => void; onCompose: () => void; onOpenLibrary: () => void }

/** Schedule-first home: loads the visible range from the server (day/week) or pages through everything, with list or week-grid views. */
export function ScheduleView({ channels, refreshToken, onRefresh, onCompose, onOpenLibrary }: ScheduleViewProps) {
  const { canCreatePost } = useOrg();
  const [range, setRange] = useState<ScheduleRange>("month");
  const [scheduleDay, setScheduleDay] = useState<string | null>(null);
  const [view, setView] = useState<"list" | "grid">("grid");
  const [date, setDate] = useState(() => localDateKey(new Date()));
  const [channel, setChannel] = useState("");
  const [status, setStatus] = useState("");
  const [query, setQuery] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [truncated, setTruncated] = useState(false);
  const [appending, setAppending] = useState(false);
  const epoch = useRef(0);
  const loadedCount = useRef(0);
  const appendingRef = useRef(false);

  /**
   * mode "user": filters/range changed → show the loading state and start from the first page.
   * mode "background": periodic refresh → silent, re-fetches everything already on screen (keeps "load more" pages).
   * mode "append": "Tải thêm" → fetch the next page after what is loaded.
   */
  const load = useCallback(async (mode: "user" | "background" | "append" = "user") => {
    if (mode === "background" && appendingRef.current) return; // never cancel a page the user asked for
    const current = ++epoch.current;
    if (mode === "user") { setLoading(true); loadedCount.current = 0; }
    if (mode === "append") { appendingRef.current = true; setAppending(true); }
    try {
      if (range === "all") {
        const offset = mode === "append" ? loadedCount.current : 0;
        const limit = mode === "background" ? Math.min(Math.max(PAGE, loadedCount.current), MAX_POST_QUERY_LIMIT) : PAGE;
        const page = await api.queryPostsPage({ limit, offset, channel_id: channel || undefined, status: status || undefined });
        if (current !== epoch.current) return;
        // Background refresh at the cap keeps the pages loaded beyond 500 (deduplicated by id); otherwise the
        // server result replaces the list so cancelled/published rows disappear instead of being re-appended.
        const capped = mode === "background" && loadedCount.current > MAX_POST_QUERY_LIMIT && page.posts.length === limit;
        setPosts(prev => {
          if (mode === "append") return [...prev, ...page.posts];
          if (!capped) return page.posts;
          const seen = new Set(page.posts.map(p => p.id));
          return [...page.posts, ...prev.slice(page.posts.length).filter(p => !seen.has(p.id))];
        });
        loadedCount.current = mode === "append" ? loadedCount.current + page.posts.length : capped ? loadedCount.current : page.posts.length;
        setHasMore(page.has_more);
        setTruncated(false);
      } else {
        const { start, end } = rangeBounds(date, range);
        const page = await api.queryPostsPage({ from: start.toISOString(), to: end.toISOString(), limit: MAX_POST_QUERY_LIMIT, channel_id: channel || undefined });
        if (current !== epoch.current) return;
        setPosts(page.posts);
        loadedCount.current = page.posts.length;
        setHasMore(false);
        setTruncated(page.has_more);
      }
      setError(null);
    } catch (err) {
      if (current === epoch.current) setError(err instanceof Error ? err.message : "Không tải được lịch đăng");
    } finally {
      if (current === epoch.current) setLoading(false);
      if (mode === "append") { appendingRef.current = false; setAppending(false); }
    }
  }, [range, date, channel, status]);

  // One effect: a filter/range change is a user load; a refreshToken bump alone is a silent background load.
  const filterKey = `${range}|${date}|${channel}|${status}`;
  const lastFilterKey = useRef("");
  const loadRef = useRef(load);
  loadRef.current = load;
  useEffect(() => {
    if (lastFilterKey.current !== filterKey) { lastFilterKey.current = filterKey; void loadRef.current("user"); return; }
    void loadRef.current("background");
  }, [filterKey, refreshToken]);
  useEffect(() => () => { epoch.current++; }, []);

  const filtered = useMemo(() => {
    const rows = selectSchedulePosts(posts, { date, range: "all", channel, status, query });
    return range === "all" ? rows.reverse() : rows; // "all" lists newest first, matching the server order and the footer note
  }, [posts, date, range, channel, status, query]);
  const groups = useMemo(() => Array.from(groupPostsByDay(filtered).entries()).sort((a, b) => range === "all" ? b[0].localeCompare(a[0]) : a[0].localeCompare(b[0])), [filtered, range]);
  const moveDate = (direction: number) => {
    if (range === "month") { setDate(shiftMonth(date, direction)); return; }
    const next = dateFromKey(date); next.setDate(next.getDate() + direction * (range === "week" ? 7 : 1)); setDate(localDateKey(next));
  };
  const bounds = rangeBounds(date, range);
  const last = new Date(bounds.end); last.setDate(last.getDate() - 1);
  const title = range === "all" ? "Tất cả bài đã tải" : range === "month" ? dateFromKey(date).toLocaleDateString("vi-VN", { month: "long", year: "numeric" }) : range === "day" ? dateFromKey(date).toLocaleDateString("vi-VN", { weekday: "long", day: "numeric", month: "long", year: "numeric" }) : `${bounds.start.toLocaleDateString("vi-VN")} — ${last.toLocaleDateString("vi-VN")}`;
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const showGrid = range === "week" && view === "grid";
  const showMonth = range === "month" && view === "grid";
  const refreshAfterSchedule = () => { setScheduleDay(null); onRefresh(); void load("background"); };

  return <section aria-label="Lịch đăng bài">
    <div className="schedule-toolbar">
      <div className="schedule-range" role="group" aria-label="Phạm vi lịch">{([{ id: "month", label: "Tháng" }, { id: "week", label: "Tuần" }, { id: "day", label: "Ngày" }, { id: "all", label: "Tất cả" }] as const).map(item => <button key={item.id} aria-pressed={range === item.id} onClick={() => setRange(item.id)}>{item.label}</button>)}</div>
      {range !== "all" && <div className="schedule-date-controls"><button className="dispatch-icon-button" onClick={() => moveDate(-1)} aria-label={range === "month" ? "Tháng trước" : range === "week" ? "Tuần trước" : "Ngày trước"}><ChevronLeft size={18} /></button><label className="sr-only" htmlFor="schedule-date">Chọn ngày</label><input id="schedule-date" type="date" value={date} onChange={event => { if (event.target.value) setDate(event.target.value); }} /><button className="dispatch-icon-button" onClick={() => moveDate(1)} aria-label={range === "month" ? "Tháng sau" : range === "week" ? "Tuần sau" : "Ngày sau"}><ChevronRight size={18} /></button><button className="schedule-today" onClick={() => setDate(localDateKey(new Date()))}>Hôm nay</button></div>}
      {(range === "week" || range === "month") && <div className="schedule-range" role="group" aria-label="Kiểu hiển thị"><button aria-pressed={view === "grid"} onClick={() => setView("grid")} aria-label="Lưới lịch"><CalendarDays size={15} /></button><button aria-pressed={view === "list"} onClick={() => setView("list")} aria-label="Danh sách"><List size={15} /></button></div>}
      <span className="dispatch-muted text-xs schedule-timezone">{timezone}</span>
      {canCreatePost && <button className="dispatch-button primary" onClick={() => setScheduleDay(localDateKey(new Date()))}><CalendarClock size={16} aria-hidden="true" />Lên lịch bài soạn sẵn</button>}
    </div>
    <div className="schedule-quick-status" role="group" aria-label="Lọc nhanh theo trạng thái">{STATUS_QUICK_FILTERS.map(item => <button key={item.value || "all"} type="button" className="chip" aria-pressed={status === item.value} onClick={() => setStatus(item.value)}>{item.label}</button>)}</div>
    <div className="schedule-filters">
      <label className="dispatch-field">Fanpage<select aria-label="Fanpage" value={channel} onChange={event => setChannel(event.target.value)}><option value="">Tất cả Fanpage</option>{channels.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label className="dispatch-field">Trạng thái<select aria-label="Trạng thái" value={status} onChange={event => setStatus(event.target.value)}><option value="">Tất cả trạng thái</option>{Object.entries(postStatusLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
      <label className="dispatch-field schedule-search">Tìm bài viết<div><Search size={16} aria-hidden="true" /><input type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="Nội dung, Fanpage, lô bài hoặc mã bài…" /></div></label>
    </div>
    <div className="schedule-results-heading"><h2>{title}</h2><span className="dispatch-muted text-xs" role="status">{loading ? "Đang cập nhật…" : `${filtered.length} bài phù hợp`}</span></div>
    {error && <div className="dispatch-error" role="alert">{error}<button onClick={() => load("user")}>Thử lại</button></div>}
    {loading && posts.length === 0 ? <div className="schedule-empty" role="status">Đang tải lịch đăng…</div>
      : showMonth ? <CalendarMonth dateKey={date} posts={filtered} channels={channels} canSchedule={canCreatePost} onSelect={setSelectedId} onScheduleDay={setScheduleDay} onOpenDay={day => { setDate(day); setRange("day"); }} />
      : showGrid ? <CalendarWeek dateKey={date} posts={filtered} channels={channels} onSelect={setSelectedId} onPickDay={day => { setDate(day); setRange("day"); }} canSchedule={canCreatePost} onScheduleDay={setScheduleDay} />
      : groups.length === 0 ? <div className="schedule-empty"><FileText size={27} aria-hidden="true" /><h3>Chưa có bài phù hợp.</h3><p>Thử đổi ngày hoặc bộ lọc, hoặc tạo bài viết mới.</p><button className="dispatch-button" onClick={onCompose}>Soạn bài mới<ArrowRight size={16} /></button></div>
      : groups.map(([key, items]) => <section key={key} className="schedule-day" aria-label={key === "unknown" ? "Chưa có thời gian" : key}>
        <div className="schedule-day-title"><h3>{key === "unknown" ? "Chưa có thời gian" : formatDayLong(dateFromKey(key))}</h3><span>{items.length} bài</span></div>
        <div className="schedule-table-heading" aria-hidden="true"><span>Thời gian</span><span>Nội dung</span><span>Fanpage</span><span>Trạng thái</span><span /></div>
        {items.map(post => {
          const at = postDate(post);
          const preview = ["IMAGE", "ALBUM"].includes(post.media_type) ? post.media_urls?.[0] : undefined;
          return <article className="schedule-post" key={post.id}>
            <div className="schedule-post-time"><time dateTime={Number.isNaN(at.getTime()) ? undefined : at.toISOString()}>{Number.isNaN(at.getTime()) ? "—" : formatTime(at)}</time><small>{post.status === "PUBLISHED" ? "Đã đăng" : "Giờ dự kiến"}</small></div>
            <div className="schedule-post-content">{preview ? <img src={preview} alt="" loading="lazy" className="schedule-thumbnail" onError={event => { event.currentTarget.style.display = "none"; }} /> : <span className="schedule-thumbnail placeholder"><FileText size={18} aria-hidden="true" /></span>}<div><button className="schedule-post-title" onClick={() => setSelectedId(post.id)}>{post.content || mediaLabels[post.media_type] || "Bài viết không có văn bản"}</button><small>{mediaLabels[post.media_type] || post.media_type}{post.media_urls && post.media_urls.length > 1 ? ` · ${post.media_urls.length} tệp` : ""}{post.batch_name ? ` · Lô: ${post.batch_name}` : ""}{willDeleteAfterPublish(post) ? " · sẽ xoá mềm khỏi bài soạn sẵn sau khi đăng" : ""}</small>{post.status === "FAILED" && <p className="schedule-failure">Đăng thất bại. Mở chi tiết để xem nguyên nhân.</p>}</div></div>
            <span className="schedule-channel">{post.channel_name || channels.find(item => item.id === post.channel_id)?.name || "Fanpage"}</span>
            <div className="schedule-post-status"><PostStatusBadge status={post.status} /></div>
            <button className="dispatch-icon-button schedule-detail" onClick={() => setSelectedId(post.id)} aria-label={`Xem chi tiết bài ${post.content?.slice(0, 40) || post.id}`}><ArrowRight size={18} /></button>
          </article>;
        })}
      </section>)}
    {range === "all" && hasMore && <div className="schedule-more"><button className="dispatch-button" onClick={() => load("append")} disabled={loading || appending} aria-busy={appending}>{appending ? "Đang tải…" : "Tải thêm bài cũ hơn"}</button></div>}
    <p className="schedule-result-note">{range === "all" ? `Đã tải ${posts.length} bài, mới nhất trước.${posts.length > MAX_POST_QUERY_LIMIT ? ` Tự cập nhật chỉ làm mới ${MAX_POST_QUERY_LIMIT} bài mới nhất.` : ""}` : `Hiển thị các bài có lịch trong khoảng đã chọn, theo múi giờ trình duyệt.${truncated ? ` Khoảng này có hơn ${MAX_POST_QUERY_LIMIT} bài; thu hẹp phạm vi hoặc lọc theo Fanpage để xem đủ.` : ""}`}</p>
    {scheduleDay && <LibrarySchedulerDialog key={scheduleDay} dateKey={scheduleDay} channels={channels} posts={posts} onClose={() => setScheduleDay(null)} onDone={refreshAfterSchedule} onOpenLibrary={() => { setScheduleDay(null); onOpenLibrary(); }} />}
    {selectedId && <div className="dispatch-legacy"><PostDetailModal key={selectedId} postId={selectedId} posts={posts} onClose={() => setSelectedId(null)} onRefresh={() => { onRefresh(); void load("background"); }} /></div>}
  </section>;
}
