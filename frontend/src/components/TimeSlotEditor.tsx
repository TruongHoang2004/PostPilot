"use client";

import { useState } from "react";
import { Plus, X } from "lucide-react";
import { GAP_OPTIONS, MAX_TIME_SLOTS, TIME_SLOT_PATTERN } from "@/lib/schedule";
import { SchedulePrefs } from "@/lib/preferences";

interface TimeSlotEditorProps { prefs: SchedulePrefs; onChange: (prefs: SchedulePrefs) => void; idPrefix: string; showGap?: boolean }

/** Preferred HH:MM slots (add/remove) and the minimum gap, shared by the single composer and the bulk plan. */
export function TimeSlotEditor({ prefs, onChange, idPrefix, showGap = true }: TimeSlotEditorProps) {
  const [value, setValue] = useState("");
  const add = () => {
    if (!TIME_SLOT_PATTERN.test(value) || prefs.timeSlots.includes(value) || prefs.timeSlots.length >= MAX_TIME_SLOTS) return;
    onChange({ ...prefs, timeSlots: [...prefs.timeSlots, value].sort() }); setValue("");
  };
  return <div className="slot-editor">
    <div className="chip-row" role="group" aria-labelledby={`${idPrefix}-slots-label`}>
      <span id={`${idPrefix}-slots-label`} className="sr-only">Khung giờ ưa thích</span>
      {prefs.timeSlots.map(slot => <span key={slot} className="chip static">{slot}<button type="button" aria-label={`Bỏ khung giờ ${slot}`} disabled={prefs.timeSlots.length <= 1} onClick={() => onChange({ ...prefs, timeSlots: prefs.timeSlots.filter(s => s !== slot) })}><X size={12} /></button></span>)}
      <span className="picker-add"><input type="time" aria-label="Khung giờ mới" value={value} onChange={e => setValue(e.target.value)} onKeyDown={e => { if (e.key === "Enter") { e.preventDefault(); add(); } }} /><button type="button" className="dispatch-icon-button" aria-label="Thêm khung giờ" onClick={add} disabled={prefs.timeSlots.length >= MAX_TIME_SLOTS}><Plus size={16} /></button></span>
    </div>
    {showGap && <GapSelect prefs={prefs} onChange={onChange} label="Cách nhau tối thiểu giữa hai bài trên cùng Fanpage" className="picker-gap" />}
  </div>;
}

/** Minimum-gap selector; merges a stored non-standard value into the option list so it stays visible. */
export function GapSelect({ prefs, onChange, label, className = "dispatch-field" }: { prefs: SchedulePrefs; onChange: (prefs: SchedulePrefs) => void; label: string; className?: string }) {
  const options = GAP_OPTIONS.includes(prefs.minGapMinutes) ? GAP_OPTIONS : [...GAP_OPTIONS, prefs.minGapMinutes].sort((a, b) => a - b);
  const text = (m: number) => m < 60 ? `${m} phút` : m % 60 === 0 ? `${m / 60} giờ` : `${Math.floor(m / 60)} giờ ${m % 60} phút`;
  return <label className={className}>{label}<select value={prefs.minGapMinutes} onChange={e => onChange({ ...prefs, minGapMinutes: Number(e.target.value) })}>{options.map(m => <option key={m} value={m}>{text(m)}</option>)}</select></label>;
}

/** Retry budget selector shared by the composers. */
export function RetrySelect({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  return <label className="dispatch-field composer-retries">Thử lại khi lỗi<select value={value} onChange={e => onChange(Number(e.target.value))}><option value={1}>1 lần</option><option value={3}>3 lần (khuyên dùng)</option><option value={5}>5 lần</option></select></label>;
}
