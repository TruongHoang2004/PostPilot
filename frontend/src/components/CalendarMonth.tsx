"use client";

import { FileText, Plus, Trash2 } from "lucide-react";
import { Channel, Post } from "@/lib/types";
import { dateFromKey, formatTime, groupPostsByDay, isPastDay, localDateKey, monthGrid, postStatusLabels, sameMonth, statusTone, weekdayShort, willDeleteAfterPublish } from "@/lib/schedule";

const MAX_CHIPS = 3;
interface CalendarMonthProps {
  dateKey: string;
  posts: Post[];
  channels: Channel[];
  canSchedule: boolean;
  onSelect: (postId: string) => void;
  onScheduleDay: (dateKey: string) => void;   // click a day → choose a library post + time
  onOpenDay: (dateKey: string) => void;       // "+N bài khác" → day view
}

/** Month board (Monday first). Click a non-past day to schedule a library post; chips open the post detail. */
export function CalendarMonth({ dateKey, posts, channels, canSchedule, onSelect, onScheduleDay, onOpenDay }: CalendarMonthProps) {
  const days = monthGrid(dateKey);
  const groups = groupPostsByDay(posts, true);
  const now = new Date();
  const today = localDateKey(now);
  const weekdayHeads = [1, 2, 3, 4, 5, 6, 0].map(i => weekdayShort[i]);
  return <div className="calendar-month" role="group" aria-label="Lịch tháng">
    <div className="calendar-month-heads" aria-hidden="true">{weekdayHeads.map(label => <span key={label}>{label}</span>)}</div>
    <div className="calendar-month-grid">
      {days.map(day => {
        const date = dateFromKey(day);
        const items = groups.get(day) || [];
        const past = isPastDay(day, now);
        const clickable = canSchedule && !past;
        const label = date.toLocaleDateString("vi-VN", { weekday: "long", day: "2-digit", month: "2-digit" });
        return <div key={day} role="group" className={`calendar-month-cell${sameMonth(day, dateKey) ? "" : " outside"}${day === today ? " today" : ""}${past ? " past" : ""}${clickable ? " clickable" : ""}`} aria-label={`${label}${items.length ? `, ${items.length} bài` : ""}`}
          onClick={event => { if (clickable && event.target === event.currentTarget) onScheduleDay(day); }}>
          <div className="calendar-month-head">
            <span className="calendar-month-num">{date.getDate()}</span>
            {clickable && <button type="button" className="calendar-month-add" onClick={() => onScheduleDay(day)} aria-label={`Lên lịch bài soạn sẵn vào ${label}`} title="Lên lịch bài soạn sẵn"><Plus size={14} aria-hidden="true" /></button>}
          </div>
          <ul className="calendar-month-posts" role="list" onClick={event => { if (clickable && event.target === event.currentTarget) onScheduleDay(day); }}>
            {items.slice(0, MAX_CHIPS).map(post => {
              const at = new Date(post.scheduled_at || post.created_at);
              const channelName = post.channel_name || channels.find(c => c.id === post.channel_id)?.name || "Fanpage";
              const state = postStatusLabels[post.status] || post.status;
              return <li key={post.id}><button type="button" className={`calendar-month-post ${statusTone(post.status)}`} onClick={() => onSelect(post.id)}
                title={`${state} · ${channelName}\n${post.content || post.media_type}${willDeleteAfterPublish(post) ? "\nSẽ xoá mềm khỏi bài soạn sẵn sau khi đăng" : ""}`} aria-label={`${state}, ${Number.isNaN(at.getTime()) ? "" : formatTime(at)}, ${post.content?.slice(0, 60) || post.media_type}`}>
                <span className="dot" aria-hidden="true" />
                <time>{Number.isNaN(at.getTime()) ? "—" : formatTime(at)}</time>
                <span className="calendar-month-text">{post.content?.trim() || <FileText size={11} aria-hidden="true" />}</span>
                {willDeleteAfterPublish(post) && <Trash2 size={11} className="calendar-month-flag" aria-hidden="true" />}
              </button></li>;
            })}
            {items.length > MAX_CHIPS && <li><button type="button" className="calendar-month-more" onClick={() => onOpenDay(day)}>+{items.length - MAX_CHIPS} bài khác</button></li>}
          </ul>
        </div>;
      })}
    </div>
    <ul className="calendar-legend" aria-label="Chú thích trạng thái">{(["SCHEDULED", "PUBLISHED", "PENDING_APPROVAL", "FAILED"] as const).map(status => <li key={status}><span className={`dot ${statusTone(status)}`} aria-hidden="true" />{postStatusLabels[status]}</li>)}<li><Trash2 size={12} aria-hidden="true" />Sẽ xoá mềm khỏi bài soạn sẵn sau khi đăng</li></ul>
  </div>;
}
