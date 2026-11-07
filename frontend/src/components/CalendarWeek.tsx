"use client";

import { FileText } from "lucide-react";
import { Channel, Post } from "@/lib/types";
import { dateFromKey, formatTime, groupPostsByDay, localDateKey, postStatusLabels, weekDays } from "@/lib/schedule";

interface CalendarWeekProps { dateKey: string; posts: Post[]; channels: Channel[]; onSelect: (postId: string) => void; onPickDay: (dateKey: string) => void }

const tone = (status: string) => status === "FAILED" || status === "REJECTED" ? "danger" : status === "PUBLISHED" ? "success" : status === "PENDING_APPROVAL" ? "warning" : ["SCHEDULED", "QUEUED", "PROCESSING"].includes(status) ? "info" : "neutral";

/** Seven-day board (Monday first) with posts stacked by scheduled time; collapses to a stacked list on narrow screens. */
export function CalendarWeek({ dateKey, posts, channels, onSelect, onPickDay }: CalendarWeekProps) {
  const days = weekDays(dateKey);
  const groups = groupPostsByDay(posts, true);
  const today = localDateKey(new Date());
  return <div className="calendar-week" role="group" aria-label="Lưới tuần">
    {days.map(day => {
      const date = dateFromKey(day);
      const items = groups.get(day) || [];
      const headingId = `calendar-day-${day}`;
      return <section key={day} className={`calendar-day${day === today ? " today" : ""}`} aria-labelledby={headingId}>
        <button type="button" id={headingId} className="calendar-day-head" onClick={() => onPickDay(day)} aria-label={`${day === today ? "Hôm nay, " : ""}${date.toLocaleDateString("vi-VN", { weekday: "long", day: "2-digit", month: "2-digit" })}${items.length ? `, ${items.length} bài` : ""}. Mở xem theo ngày`}>
          <span>{date.toLocaleDateString("vi-VN", { weekday: "short" })}</span><strong>{date.getDate()}</strong><small>{day === today ? "hôm nay" : ""}{items.length ? `${day === today ? " · " : ""}${items.length} bài` : ""}</small>
        </button>
        <ul className="calendar-day-body" role="list">
          {items.length === 0 && <li className="calendar-empty" aria-hidden="true">·</li>}
          {items.map(post => {
            const at = new Date(post.scheduled_at || post.created_at);
            const thumb = ["IMAGE", "ALBUM"].includes(post.media_type) ? post.media_urls?.[0] : undefined;
            return <li key={post.id}><button type="button" className={`calendar-post ${tone(post.status)}`} onClick={() => onSelect(post.id)} title={post.content || post.media_type}>
              <span className="calendar-post-time"><span className="dot" aria-hidden="true" />{Number.isNaN(at.getTime()) ? "—" : formatTime(at)}{post.status !== "SCHEDULED" && <em className="calendar-post-state">{postStatusLabels[post.status] || post.status}</em>}</span>
              <span className="calendar-post-text">{thumb ? <img src={thumb} alt="" loading="lazy" /> : <FileText size={12} aria-hidden="true" />}{post.content?.trim() || post.media_type}</span>
              <small>{post.channel_name || channels.find(c => c.id === post.channel_id)?.name || "Fanpage"}{post.batch_name ? ` · ${post.batch_name}` : ""}</small>
            </button></li>;
          })}
        </ul>
      </section>;
    })}
  </div>;
}
