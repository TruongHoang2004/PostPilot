import { Post, PostStatus } from "./types";

export type ScheduleRange = "day" | "week" | "all";
export const postStatusLabels: Record<PostStatus, string> = {
  DRAFT: "Bản nháp", PENDING_APPROVAL: "Chờ duyệt", SCHEDULED: "Đã lên lịch", QUEUED: "Trong hàng đợi",
  PROCESSING: "Đang đăng", PUBLISHED: "Đã đăng", FAILED: "Đăng thất bại", REJECTED: "Bị từ chối", CANCELLED: "Đã hủy",
};
export const mediaLabels: Record<string, string> = { NONE: "Văn bản", LINK: "Liên kết", IMAGE: "Ảnh", ALBUM: "Album ảnh", VIDEO: "Video", REEL: "Reel" };
export function localDateKey(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}
export function dateFromKey(value: string): Date {
  const [year, month, day] = value.split("-").map(Number);
  return new Date(year, month - 1, day);
}
export function postDate(post: Post): Date {
  return new Date(post.status === "PUBLISHED" && post.published_at ? post.published_at : post.scheduled_at || post.created_at);
}
export function rangeBounds(dateKey: string, range: ScheduleRange): { start: Date; end: Date } {
  const start = dateFromKey(dateKey);
  if (range === "week") start.setDate(start.getDate() - (start.getDay() + 6) % 7);
  const end = new Date(start);
  end.setDate(start.getDate() + (range === "week" ? 7 : 1));
  return { start, end };
}
export function selectSchedulePosts(posts: Post[], filters: { date: string; range: ScheduleRange; channel: string; status: string; query: string }): Post[] {
  const { start, end } = rangeBounds(filters.date, filters.range);
  const query = filters.query.trim().toLocaleLowerCase("vi-VN");
  return posts.filter(post => {
    if (filters.channel && post.channel_id !== filters.channel) return false;
    if (filters.status && post.status !== filters.status) return false;
    if (query && !`${post.content} ${post.channel_name || ""} ${post.id}`.toLocaleLowerCase("vi-VN").includes(query)) return false;
    if (filters.range === "all") return true;
    const date = postDate(post);
    return date >= start && date < end;
  }).sort((a, b) => (postDate(a).getTime() || 0) - (postDate(b).getTime() || 0));
}

// ---------------------------------------------------------------------------
// Time pickers, calendar grid and free-slot suggestions (pure, unit-tested)
// ---------------------------------------------------------------------------
export const DEFAULT_TIME_SLOTS = ["09:00", "12:00", "19:30"];
export const MAX_TIME_SLOTS = 12;            // mirrors domain.MaxPlanTimeSlots
export const MAX_POST_QUERY_LIMIT = 500;     // mirrors domain.MaxPostQueryLimit
export const GAP_OPTIONS = [15, 30, 60, 120, 180, 360];
export const TIME_SLOT_PATTERN = /^\d{2}:\d{2}$/;
export const EDITABLE_STATUSES: PostStatus[] = ["DRAFT", "SCHEDULED", "PENDING_APPROVAL", "FAILED"];
export const AUTHOR_ROLES = ["MANAGER", "APPROVER", "EDITOR"];

const VI = "vi-VN";
/** "T2, 10/11 09:00" style short date-time. */
export const formatDateTimeShort = (d: Date) => d.toLocaleString(VI, { weekday: "short", day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
/** "Thứ Hai, 10/11". */
export const formatDayLong = (d: Date) => d.toLocaleDateString(VI, { weekday: "long", day: "2-digit", month: "2-digit" });
/** "09:00". */
export const formatTime = (d: Date) => d.toLocaleTimeString(VI, { hour: "2-digit", minute: "2-digit" });

/** Maps server note codes (ScheduleSlot.note_code, warning_codes) to UI text; falls back to the server string. */
export function describeNote(code: string | undefined, value: number | undefined, fallback?: string): string {
  switch (code) {
    case "CONFLICTS_SKIPPED": return `Đã bỏ qua ${value ?? 0} khung giờ trùng lịch trước bài này.`;
    case "PAST_TIME": return "Giờ riêng đã qua: bài sẽ được đăng ngay ở lượt quét tiếp theo.";
    case "OCCUPANCY_TRUNCATED": return `Chỉ ${value ?? 0} bài sắp tới được dùng để phát hiện trùng lịch.`;
    default: return fallback || "";
  }
}

/** Toggles a weekday in a plan weekday set where [] means "every day". */
export function toggleWeekday(weekdays: number[], day: number): number[] {
  const base = weekdays.length === 0 ? [1, 2, 3, 4, 5, 6, 0] : weekdays;
  const next = base.includes(day) ? base.filter(d => d !== day) : [...base, day];
  return next.length === 7 || next.length === 0 ? [] : next;
}

/** Shared media/content rule mirrored from the backend (domain.NormalizeDraft); returns a message or null. */
export function validateMedia(mediaType: string, urls: string[], content: string): string | null {
  if (["IMAGE", "ALBUM", "VIDEO", "REEL"].includes(mediaType) && urls.length === 0) return `Định dạng ${mediaType} cần ít nhất một tệp.`;
  if (mediaType === "LINK" && urls.length === 0) return "Định dạng liên kết cần đường dẫn trang web.";
  if (urls.length > 10) return "Tối đa 10 tệp đính kèm cho một bài.";
  if (!content.trim() && urls.length === 0) return "Bài viết cần nội dung hoặc tệp đính kèm.";
  return null;
}
export const weekdayShort = ["CN", "T2", "T3", "T4", "T5", "T6", "T7"]; // index = Date.getDay()
export const ACTIVE_STATUSES: PostStatus[] = ["DRAFT", "PENDING_APPROVAL", "SCHEDULED", "QUEUED", "PROCESSING", "FAILED"];
const pad2 = (value: number) => String(value).padStart(2, "0");

export function isActiveStatus(status: string): boolean {
  return (ACTIVE_STATUSES as string[]).includes(status);
}

/** Formats a Date as the value of an <input type="datetime-local"> in the browser timezone. */
export function toLocalInputValue(date: Date): string {
  if (Number.isNaN(date.getTime())) return "";
  return `${localDateKey(date)}T${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
}

/** Parses a datetime-local value ("YYYY-MM-DDTHH:MM") in the browser timezone; null when invalid. */
export function fromLocalInputValue(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(value || "");
  if (!match) return null;
  const [, y, m, d, hh, mm] = match.map(Number);
  const date = new Date(y, m - 1, d, hh, mm, 0, 0);
  return Number.isNaN(date.getTime()) ? null : date;
}

/** Combines a local date key with an HH:MM slot into a local Date. */
export function combineDateAndTime(dateKey: string, slot: string): Date {
  const date = dateFromKey(dateKey);
  const [hh, mm] = slot.split(":").map(Number);
  date.setHours(hh || 0, mm || 0, 0, 0);
  return date;
}

/** The seven local date keys (Monday → Sunday) of the week containing dateKey. */
export function weekDays(dateKey: string): string[] {
  const { start } = rangeBounds(dateKey, "week");
  return Array.from({ length: 7 }, (_, i) => {
    const day = new Date(start);
    day.setDate(start.getDate() + i);
    return localDateKey(day);
  });
}

/** Groups posts by local day key, each group sorted by time. byScheduled groups on scheduled_at (calendar cells) instead of the display date. */
export function groupPostsByDay(posts: Post[], byScheduled = false): Map<string, Post[]> {
  const when = (post: Post) => byScheduled ? new Date(post.scheduled_at || post.created_at) : postDate(post);
  const groups = new Map<string, Post[]>();
  [...posts].sort((a, b) => when(a).getTime() - when(b).getTime()).forEach(post => {
    const at = when(post);
    const key = Number.isNaN(at.getTime()) ? "unknown" : localDateKey(at);
    groups.set(key, [...(groups.get(key) || []), post]);
  });
  return groups;
}

/** True when any active post on one of the channels is scheduled within gapMinutes of `at`. */
export function hasConflict(posts: Post[], channelIds: string[], at: Date, gapMinutes: number): Post | null {
  const gap = gapMinutes * 60000;
  for (const post of posts) {
    if (!isActiveStatus(post.status) || (channelIds.length > 0 && !channelIds.includes(post.channel_id))) continue;
    const when = new Date(post.scheduled_at).getTime();
    if (!Number.isNaN(when) && Math.abs(when - at.getTime()) < gap) return post;
  }
  return null;
}

/**
 * Suggests the next free publication instants for the selected channels: walks days from `now`
 * over the preferred HH:MM slots, skipping past instants and slots that collide with loaded active posts.
 */
export function suggestSlots(posts: Post[], channelIds: string[], slots: string[], now: Date, count = 3, gapMinutes = 30, horizonDays = 60): Date[] {
  const result: Date[] = [];
  const preferred = (slots.length ? slots : DEFAULT_TIME_SLOTS).slice().sort();
  const earliest = now.getTime() + 60000;
  for (let day = 0; day < horizonDays && result.length < count; day++) {
    const base = new Date(now);
    base.setDate(now.getDate() + day);
    const key = localDateKey(base);
    for (const slot of preferred) {
      if (result.length >= count) break;
      const at = combineDateAndTime(key, slot);
      if (at.getTime() < earliest) continue;
      if (hasConflict(posts, channelIds, at, gapMinutes)) continue;
      if (result.some(taken => Math.abs(taken.getTime() - at.getTime()) < gapMinutes * 60000)) continue;
      result.push(at);
    }
  }
  return result;
}
