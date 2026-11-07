"use client";

import { useMemo } from "react";
import { AlertTriangle, Clock, Sparkles, X } from "lucide-react";
import { Post } from "@/lib/types";
import { combineDateAndTime, formatDateTimeShort, fromLocalInputValue, hasConflict, localDateKey, suggestSlots, toLocalInputValue } from "@/lib/schedule";
import { SchedulePrefs } from "@/lib/preferences";
import { TimeSlotEditor } from "./TimeSlotEditor";

interface SchedulePickerProps {
  id: string;
  value: string;                       // datetime-local value or "" (no schedule)
  onChange: (value: string) => void;
  posts: Post[];                       // loaded posts used for free-slot suggestions and conflict warnings
  channelIds: string[];
  prefs: SchedulePrefs;
  onPrefsChange?: (prefs: SchedulePrefs) => void;
  allowEmpty?: boolean;
  emptyHint?: string;
  label?: string;
}

/** Date/time picker with presets, preferred time slots and next-free-slot suggestions for the selected Fanpages. */
export function SchedulePicker({ id, value, onChange, posts, channelIds, prefs, onPrefsChange, allowEmpty = true, emptyHint, label = "Thời gian đăng" }: SchedulePickerProps) {
  const now = new Date();
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const selected = fromLocalInputValue(value);
  const suggestions = useMemo(() => suggestSlots(posts, channelIds, prefs.timeSlots, new Date(), 3, prefs.minGapMinutes), [posts, channelIds, prefs.timeSlots, prefs.minGapMinutes]);
  const conflict = selected ? hasConflict(posts, channelIds, selected, prefs.minGapMinutes) : null;
  const isPast = selected ? selected.getTime() < now.getTime() : false;
  const pick = (d: Date) => onChange(toLocalInputValue(d));
  const tonight = combineDateAndTime(localDateKey(now), prefs.timeSlots[prefs.timeSlots.length - 1] || "19:30");
  if (tonight.getTime() <= now.getTime()) tonight.setDate(tonight.getDate() + 1);
  const tomorrowMorning = combineDateAndTime(localDateKey(now), prefs.timeSlots[0] || "09:00");
  tomorrowMorning.setDate(tomorrowMorning.getDate() + 1);

  return <div className="schedule-picker">
    <div className="composer-row-head"><label className="composer-label" htmlFor={id}>{label}</label><span className="dispatch-muted text-xs">{timezone}</span></div>
    <div className="picker-main">
      <input id={id} type="datetime-local" value={value} min={toLocalInputValue(now).slice(0, 16)} onChange={e => onChange(e.target.value)} />
      {allowEmpty && value && <button type="button" className="dispatch-icon-button" aria-label="Bỏ hẹn giờ" onClick={() => onChange("")}><X size={16} /></button>}
    </div>
    <div className="chip-row" role="group" aria-label="Chọn nhanh">
      <button type="button" className="chip" onClick={() => pick(new Date(now.getTime() + 60 * 60000))}><Clock size={13} aria-hidden="true" />+1 giờ</button>
      <button type="button" className="chip" onClick={() => pick(tonight)}>{tonight.getDate() === now.getDate() ? "Tối nay" : "Tối mai"} {prefs.timeSlots[prefs.timeSlots.length - 1] || "19:30"}</button>
      <button type="button" className="chip" onClick={() => pick(tomorrowMorning)}>Sáng mai {prefs.timeSlots[0] || "09:00"}</button>
    </div>
    {suggestions.length > 0 && <div className="picker-suggest"><span className="dispatch-muted text-xs"><Sparkles size={13} aria-hidden="true" />Giờ trống gần nhất{channelIds.length ? " cho Fanpage đã chọn" : ""}:</span>
      <div className="chip-row" role="group" aria-label="Giờ trống gợi ý">{suggestions.map(d => <button key={d.getTime()} type="button" className="chip accent" aria-pressed={selected?.getTime() === d.getTime()} onClick={() => pick(d)}>{formatDateTimeShort(d)}</button>)}</div></div>}
    {selected && isPast && <p className="dispatch-note warning"><AlertTriangle size={14} aria-hidden="true" />Thời gian đã qua: bài sẽ được đăng ngay ở lượt quét tiếp theo.</p>}
    {selected && !isPast && conflict && <p className="dispatch-note warning"><AlertTriangle size={14} aria-hidden="true" />Cách bài “{(conflict.content || "không có văn bản").slice(0, 40)}” dưới {prefs.minGapMinutes} phút trên cùng Fanpage.</p>}
    {!selected && allowEmpty && <p className="dispatch-note">{emptyHint || "Chưa hẹn giờ: “Lên lịch” sẽ dùng thời điểm hiện tại, “Đăng ngay” đưa bài vào hàng đợi lập tức."}</p>}
    {onPrefsChange && <details className="picker-prefs"><summary>Khung giờ ưa thích ({prefs.timeSlots.join(", ")})</summary>
      <TimeSlotEditor idPrefix={id} prefs={prefs} onChange={onPrefsChange} />
    </details>}
  </div>;
}
