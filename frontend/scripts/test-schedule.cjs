const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const ts = require("typescript");
const source = fs.readFileSync(path.join(__dirname, "../src/lib/schedule.ts"), "utf8");
const moduleContext = { exports: {} };
vm.runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText, moduleContext);
const { localDateKey, rangeBounds, selectSchedulePosts } = moduleContext.exports;
const post = (id, at, extra = {}) => ({id, channel_id: "a", content: "Táo đầu mùa", status: "SCHEDULED", scheduled_at: at, created_at: at, ...extra});
process.env.TZ = "Asia/Ho_Chi_Minh";
const posts = [
  post("before", "2026-10-25T16:59:59Z"),
  post("start", "2026-10-25T17:00:00Z"),
  post("end", "2026-11-01T16:59:59Z", {channel_id:"b", status:"FAILED"}),
  post("after", "2026-11-01T17:00:00Z"),
];
const filters = { date:"2026-10-28", range:"week", channel:"", status:"", query:"" };
assert.equal(selectSchedulePosts(posts, filters).map(p=>p.id).join(","), "start,end", "Monday-inclusive/Sunday-inclusive Vietnam week");
assert.equal(selectSchedulePosts(posts, {...filters, channel:"b", status:"FAILED", query:"TÁO"}).map(p=>p.id).join(","), "end", "combined filters");
assert.equal(selectSchedulePosts(posts, {...filters, status:"PUBLISHED"}).length, 0);
assert.equal(selectSchedulePosts(posts, {...filters, range:"all"}).length, 4);
assert.equal(selectSchedulePosts([post("published", "2026-10-20T00:00:00Z", {status:"PUBLISHED", published_at:"2026-10-28T01:00:00Z"})], {...filters, range:"day"}).length, 1, "published display uses actual publication date");
assert.equal(localDateKey(rangeBounds("2026-11-01", "week").start), "2026-10-26", "Sunday belongs to preceding Monday");
process.env.TZ = "America/New_York";
const dst = rangeBounds("2026-11-01", "day");
assert.equal((dst.end-dst.start)/3600000, 25, "calendar day spans DST fall-back");
assert.equal(selectSchedulePosts([post("late", "2026-11-02T04:59:59Z"), post("next", "2026-11-02T05:00:00Z")], {...filters, date:"2026-11-01", range:"day"}).map(p=>p.id).join(","), "late");
console.log("PASS: Vietnam week/day boundaries, combined filters, publication dates and DST calendar day");

// --- picker, calendar grid and suggestion helpers -------------------------------------------
process.env.TZ = "Asia/Ho_Chi_Minh";
const { toLocalInputValue, fromLocalInputValue, weekDays, groupPostsByDay, suggestSlots, hasConflict, combineDateAndTime } = moduleContext.exports;
assert.equal(toLocalInputValue(new Date(2026, 10, 10, 9, 5)), "2026-11-10T09:05");
assert.equal(fromLocalInputValue("2026-11-10T09:05").getTime(), new Date(2026, 10, 10, 9, 5).getTime());
assert.equal(fromLocalInputValue("garbage"), null);
assert.equal(weekDays("2026-11-12").join(","), "2026-11-09,2026-11-10,2026-11-11,2026-11-12,2026-11-13,2026-11-14,2026-11-15", "Monday-first week");
assert.equal(combineDateAndTime("2026-11-10", "19:30").getHours(), 19);
const grouped = groupPostsByDay([post("late", "2026-11-10T13:00:00Z"), post("early", "2026-11-10T01:00:00Z"), post("next", "2026-11-11T01:00:00Z")]);
assert.deepEqual([...grouped.keys()], ["2026-11-10", "2026-11-11"]);
assert.equal(grouped.get("2026-11-10").map(p => p.id).join(","), "early,late", "day groups sorted by time");
const now = new Date(2026, 10, 10, 10, 0); // Tue 10:00 local
const taken = [post("busy", new Date(2026, 10, 10, 19, 30).toISOString()), post("done", new Date(2026, 10, 11, 9, 0).toISOString(), { status: "PUBLISHED" }), post("other", new Date(2026, 10, 10, 12, 0).toISOString(), { channel_id: "b" })];
const suggestions = suggestSlots(taken, ["a"], ["09:00", "12:00", "19:30"], now, 3, 30);
assert.equal(suggestions.map(d => toLocalInputValue(d)).join(","), "2026-11-10T12:00,2026-11-11T09:00,2026-11-11T12:00", "skips past 09:00, the busy 19:30 on channel a, ignores published and other-channel posts");
assert.equal(suggestSlots(taken, ["a", "b"], ["12:00"], now, 1, 30)[0].getDate(), 11, "conflict on any selected channel is avoided");
assert.ok(hasConflict(taken, ["a"], new Date(2026, 10, 10, 19, 45), 30), "19:45 is within 30 minutes of the busy post");
assert.equal(hasConflict(taken, ["a"], new Date(2026, 10, 10, 20, 0), 30), null, "exactly 30 minutes apart is allowed");
console.log("PASS: picker values, week days, day grouping and free-slot suggestions");

// --- remediation helpers ---------------------------------------------------------------------
const { toggleWeekday, describeNote, validateMedia } = moduleContext.exports;
// vm-realm arrays have a foreign prototype, so compare joined values.
assert.equal(toggleWeekday([], 0).join(","), "1,2,3,4,5,6", "removing a day from 'every day' expands the set");
assert.equal(toggleWeekday([1, 2, 3, 4, 5, 6], 0).join(","), "", "re-adding the last day collapses back to 'every day'");
assert.equal(toggleWeekday([1, 3], 3).join(","), "1");
assert.equal(describeNote("CONFLICTS_SKIPPED", 2), "Đã bỏ qua 2 khung giờ trùng lịch trước bài này.");
assert.equal(describeNote("UNKNOWN", 0, "server text"), "server text", "unknown codes fall back to the server text");
assert.equal(validateMedia("IMAGE", [], "x"), "Định dạng IMAGE cần ít nhất một tệp.");
assert.equal(validateMedia("NONE", [], "  "), "Bài viết cần nội dung hoặc tệp đính kèm.");
assert.equal(validateMedia("NONE", [], "ok"), null);
const scheduledGroups = groupPostsByDay([post("pub", "2026-11-08T16:59:00Z", { status: "PUBLISHED", published_at: "2026-11-08T17:05:00Z" })], true);
assert.deepEqual([...scheduledGroups.keys()], ["2026-11-08"], "calendar cells group by scheduled_at (23:59 local on the 8th), not by the later publication instant");
console.log("PASS: weekday toggle, note codes, media rules and scheduled-day grouping");

// --- month calendar and click-to-schedule helpers --------------------------------------------
process.env.TZ = "Asia/Ho_Chi_Minh";
const { monthGrid, sameMonth, shiftMonth, isPastDay, defaultSlotForDay, statusTone, willDeleteAfterPublish } = moduleContext.exports;
{
  const grid = monthGrid("2026-11-15");
  assert.equal(grid.length, 42, "the month board always shows six weeks");
  assert.equal(grid[0], "2026-10-26", "Nov 1 2026 is a Sunday, so the Monday-first board starts on Oct 26");
  assert.equal(grid[41], "2026-12-06");
  assert.equal(grid.filter(day => sameMonth(day, "2026-11-15")).length, 30, "every day of November is on the board exactly once");
  assert.equal(monthGrid("2026-02-10")[0], "2026-01-26", "Feb 1 2026 is a Sunday as well");
  assert.equal(monthGrid("2026-12-31")[0], "2026-11-30", "year-end anchor still starts on the Monday before Dec 1");
  const { start, end } = rangeBounds("2026-11-15", "month");
  assert.equal(localDateKey(start), "2026-10-26");
  assert.equal(localDateKey(end), "2026-12-07", "range end is exclusive: the day after the last board cell");
  assert.equal(shiftMonth("2026-11-15", 1), "2026-12-01");
  assert.equal(shiftMonth("2026-01-31", -1), "2025-12-01", "month navigation never overflows into a 31st that does not exist");
  assert.equal(shiftMonth("2026-03-31", 1), "2026-04-01");
}
{
  const now = new Date(2026, 10, 10, 10, 0); // Tue 10 Nov 2026 10:00
  assert.equal(isPastDay("2026-11-09", now), true);
  assert.equal(isPastDay("2026-11-10", now), false, "today can still receive a schedule");
  const slots = ["09:00", "12:00", "19:30"];
  assert.equal(toLocalInputValue(defaultSlotForDay("2026-11-12", slots, now, [], [], 30)), "2026-11-12T09:00", "future day starts at the first slot");
  assert.equal(toLocalInputValue(defaultSlotForDay("2026-11-10", slots, now, [], [], 30)), "2026-11-10T12:00", "today skips slots that already passed");
  const busy = [post("busy", new Date(2026, 10, 10, 12, 0).toISOString(), { channel_id: "a" })];
  assert.equal(toLocalInputValue(defaultSlotForDay("2026-11-10", slots, now, busy, ["a"], 30)), "2026-11-10T19:30", "a conflicting slot on the chosen channel is skipped");
  assert.equal(toLocalInputValue(defaultSlotForDay("2026-11-10", slots, now, busy, ["b"], 30)), "2026-11-10T12:00", "another channel is unaffected");
  const late = new Date(2026, 10, 10, 21, 15);
  assert.equal(toLocalInputValue(defaultSlotForDay("2026-11-10", slots, late, [], [], 30)), "2026-11-10T22:00", "after the last slot, fall back to the next full hour");
  const nearMidnight = new Date(2026, 10, 10, 23, 30);
  assert.equal(toLocalInputValue(defaultSlotForDay("2026-11-10", slots, nearMidnight, [], [], 30)), "2026-11-10T23:35", "late evening must stay on the clicked day instead of rolling to 00:00");
  const lastMinutes = new Date(2026, 10, 10, 23, 58);
  assert.equal(toLocalInputValue(defaultSlotForDay("2026-11-10", slots, lastMinutes, [], [], 30)), "2026-11-11T09:00", "when no time is left today the first slot of the next day is used");
  assert.equal(toLocalInputValue(defaultSlotForDay("2026-11-11", [], now, [], [], 30)), "2026-11-11T09:00", "empty preferences fall back to the default slots");
}
assert.equal(statusTone("PUBLISHED"), "success");
assert.equal(statusTone("SCHEDULED"), "info");
assert.equal(statusTone("PENDING_APPROVAL"), "warning");
assert.equal(statusTone("FAILED"), "danger");
assert.equal(statusTone("CANCELLED"), "neutral");
assert.equal(willDeleteAfterPublish({ status: "SCHEDULED", library_item_id: "i", delete_after_publish: true }), true);
assert.equal(willDeleteAfterPublish({ status: "FAILED", library_item_id: "i", delete_after_publish: true }), true, "a retryable failure can still publish");
assert.equal(willDeleteAfterPublish({ status: "PUBLISHED", library_item_id: "i", delete_after_publish: true }), false, "already published: nothing left to promise");
assert.equal(willDeleteAfterPublish({ status: "CANCELLED", library_item_id: "i", delete_after_publish: true }), false);
assert.equal(willDeleteAfterPublish({ status: "SCHEDULED", library_item_id: "", delete_after_publish: true }), false, "item deleted: the flag is orphaned");
assert.equal(willDeleteAfterPublish({ status: "SCHEDULED", library_item_id: "i", delete_after_publish: false }), false);
console.log("PASS: month board geometry, month navigation, past-day rule, default slot for a clicked day and status tones");

// The same month geometry must hold in a DST timezone (US fall-back 2026-11-01 and spring-forward 2026-03-08 inside the boards).
process.env.TZ = "America/New_York";
{
  const nov = monthGrid("2026-11-15");
  assert.equal(nov.length, 42);
  assert.equal(nov[0], "2026-10-26");
  assert.equal(new Set(nov).size, 42, "no duplicated or skipped day across the 25-hour fall-back day");
  assert.equal(nov[6], "2026-11-01", "the fall-back Sunday is the 7th cell");
  const mar = monthGrid("2026-03-10");
  assert.equal(new Set(mar).size, 42, "no duplicated or skipped day across the 23-hour spring-forward day");
  assert.ok(mar.includes("2026-03-08"));
  assert.equal(localDateKey(rangeBounds("2026-11-15", "month").end), "2026-12-07");
  assert.equal(shiftMonth("2026-03-31", 1), "2026-04-01");
  const nowNY = new Date(2026, 2, 7, 20, 0);
  assert.equal(toLocalInputValue(defaultSlotForDay("2026-03-08", ["09:00"], nowNY, [], [], 30)), "2026-03-08T09:00");
}
console.log("PASS: month geometry in a DST timezone");
