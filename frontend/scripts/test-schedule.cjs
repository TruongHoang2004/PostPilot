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
