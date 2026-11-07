import { DEFAULT_TIME_SLOTS, TIME_SLOT_PATTERN } from "./schedule";

/** Per-workspace scheduling preferences kept in the browser (a convenience, never authoritative). */
export interface SchedulePrefs {
  timeSlots: string[];
  weekdays: number[];        // empty = every day
  minGapMinutes: number;
  avoidConflicts: boolean;
}

export const DEFAULT_SCHEDULE_PREFS: SchedulePrefs = { timeSlots: DEFAULT_TIME_SLOTS, weekdays: [], minGapMinutes: 30, avoidConflicts: true };
const key = (orgId: string) => `postpilot_schedule_prefs:${orgId}`;

export function loadSchedulePrefs(orgId: string): SchedulePrefs {
  if (typeof window === "undefined" || !orgId) return DEFAULT_SCHEDULE_PREFS;
  try {
    const raw = localStorage.getItem(key(orgId));
    if (!raw) return DEFAULT_SCHEDULE_PREFS;
    const parsed = JSON.parse(raw) as Partial<SchedulePrefs>;
    const slots = Array.isArray(parsed.timeSlots) ? parsed.timeSlots.filter(s => TIME_SLOT_PATTERN.test(s)) : [];
    return {
      timeSlots: slots.length ? slots : DEFAULT_TIME_SLOTS,
      weekdays: Array.isArray(parsed.weekdays) ? parsed.weekdays.filter(d => Number.isInteger(d) && d >= 0 && d <= 6) : [],
      minGapMinutes: typeof parsed.minGapMinutes === "number" && parsed.minGapMinutes >= 0 ? parsed.minGapMinutes : 30,
      avoidConflicts: parsed.avoidConflicts !== false,
    };
  } catch {
    return DEFAULT_SCHEDULE_PREFS;
  }
}

export function saveSchedulePrefs(orgId: string, prefs: SchedulePrefs): void {
  if (typeof window === "undefined" || !orgId) return;
  try { localStorage.setItem(key(orgId), JSON.stringify(prefs)); } catch { /* storage unavailable: ignore */ }
}
