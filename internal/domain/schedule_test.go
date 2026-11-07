package domain

import (
	"strings"
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestSchedulePlanNormalize(t *testing.T) {
	p := SchedulePlan{StartDate: "2026-11-10", Timezone: "Asia/Ho_Chi_Minh", TimeSlots: []string{"19:30", "09:00", "9:00", " 12:00 "}, Weekdays: []int{5, 1, 1}}
	if err := p.Normalize(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.TimeSlots, ",") != "09:00,12:00,19:30" {
		t.Fatalf("slots not canonical: %v", p.TimeSlots)
	}
	if len(p.Weekdays) != 2 || p.Weekdays[0] != 1 || p.Weekdays[1] != 5 {
		t.Fatalf("weekdays not canonical: %v", p.Weekdays)
	}
	if p.MinGapMinutes != 30 {
		t.Fatalf("expected default gap 30, got %d", p.MinGapMinutes)
	}
	huge := SchedulePlan{StartDate: "2026-11-10", TimeSlots: []string{"09:00"}, MinGapMinutes: MaxMinGapMinutes + 1}
	if err := huge.Normalize(); err == nil {
		t.Fatal("gap above one week must be rejected")
	}

	bad := []SchedulePlan{
		{StartDate: "10/11/2026", TimeSlots: []string{"09:00"}},
		{StartDate: "2026-11-10", TimeSlots: []string{"25:00"}},
		{StartDate: "2026-11-10", TimeSlots: []string{"09:00"}, Weekdays: []int{7}},
		{StartDate: "2026-11-10", TimeSlots: []string{"09:00"}, Timezone: "Mars/Olympus"},
		{StartDate: "2026-11-10"},
		{StartDate: "2026-11-10", TimeSlots: []string{"09:00"}, MinGapMinutes: -1},
	}
	for i, plan := range bad {
		if err := plan.Normalize(); err == nil {
			t.Fatalf("case %d expected validation error", i)
		}
	}
	var nilPlan *SchedulePlan
	if err := nilPlan.Normalize(); err == nil {
		t.Fatal("nil plan must be rejected")
	}
}

func TestGenerateScheduleSlotsRespectsWeekdaysSlotsAndPast(t *testing.T) {
	loc := mustLoc(t, "Asia/Ho_Chi_Minh")
	plan := SchedulePlan{StartDate: "2026-11-09", Timezone: "Asia/Ho_Chi_Minh", TimeSlots: []string{"09:00", "19:30"}, Weekdays: []int{1, 3}} // Mon, Wed
	if err := plan.Normalize(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, loc) // Monday 10:00 local: 09:00 slot already past
	got, notes, err := GenerateScheduleSlots(plan, 4, now, []string{"ch1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Time{
		time.Date(2026, 11, 9, 19, 30, 0, 0, loc),
		time.Date(2026, 11, 11, 9, 0, 0, 0, loc),
		time.Date(2026, 11, 11, 19, 30, 0, 0, loc),
		time.Date(2026, 11, 16, 9, 0, 0, 0, loc),
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("slot %d: got %s want %s", i, got[i].In(loc), want[i])
		}
		if got[i].Location() != time.UTC {
			t.Fatalf("slot %d must be UTC", i)
		}
	}
	if len(notes) != 4 {
		t.Fatalf("expected one note per slot, got %d", len(notes))
	}
}

func TestGenerateScheduleSlotsAvoidsConflictsAndSelfGap(t *testing.T) {
	loc := mustLoc(t, "UTC")
	plan := SchedulePlan{StartDate: "2026-11-10", TimeSlots: []string{"09:00", "09:10", "12:00"}, MinGapMinutes: 30, AvoidConflicts: true}
	if err := plan.Normalize(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, loc)
	occ := Occupancy{}
	occ.Add("a", time.Date(2026, 11, 10, 11, 50, 0, 0, loc)) // blocks 12:00 on channel a
	got, notes, err := GenerateScheduleSlots(plan, 3, now, []string{"a", "b"}, occ)
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Time{
		time.Date(2026, 11, 10, 9, 0, 0, 0, loc),
		time.Date(2026, 11, 11, 9, 0, 0, 0, loc),  // 09:10 (self gap) and 12:00 (occupied) skipped on the 10th
		time.Date(2026, 11, 11, 12, 0, 0, 0, loc), // 09:10 self gap skipped, 12:00 free on the 11th
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("slot %d: got %s want %s", i, got[i], want[i])
		}
	}
	if notes[1].Code != NoteConflictsSkipped || notes[1].Value != 2 || notes[2].Value != 1 || notes[0].Code != "" {
		t.Fatalf("expected conflict notes, got %+v %+v %+v", notes[0], notes[1], notes[2])
	}
	if !strings.Contains(notes[1].String(), "skipped 2") {
		t.Fatalf("human fallback wrong: %q", notes[1].String())
	}
	if len(occ["b"]) != 3 {
		t.Fatalf("assigned slots must be recorded for every channel, got %d", len(occ["b"]))
	}

	plan.AvoidConflicts = false
	got, _, err = GenerateScheduleSlots(plan, 2, now, []string{"a"}, Occupancy{"a": {time.Date(2026, 11, 10, 9, 0, 0, 0, loc)}})
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].Equal(time.Date(2026, 11, 10, 9, 0, 0, 0, loc)) || !got[1].Equal(time.Date(2026, 11, 10, 9, 10, 0, 0, loc)) {
		t.Fatalf("without conflict avoidance slots are taken verbatim: %v", got)
	}
}

func TestGenerateScheduleSlotsExhaustion(t *testing.T) {
	plan := SchedulePlan{StartDate: "2026-11-10", TimeSlots: []string{"09:00"}, Weekdays: []int{0}}
	if err := plan.Normalize(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	if _, _, err := GenerateScheduleSlots(plan, 80, now, []string{"a"}, nil); err == nil {
		t.Fatal("expected exhaustion error for 80 Sundays within the horizon")
	}
	if got, _, err := GenerateScheduleSlots(plan, 0, now, nil, nil); err != nil || got != nil {
		t.Fatalf("zero count returns nothing: %v %v", got, err)
	}
}

func TestGenerateScheduleSlotsTimezoneDateBoundary(t *testing.T) {
	// 01:00 in Ho Chi Minh on the 10th is 18:00 UTC on the 9th; the UTC date must not shift the day.
	loc := mustLoc(t, "Asia/Ho_Chi_Minh")
	plan := SchedulePlan{StartDate: "2026-11-10", Timezone: "Asia/Ho_Chi_Minh", TimeSlots: []string{"01:00"}}
	if err := plan.Normalize(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	got, _, err := GenerateScheduleSlots(plan, 1, now, []string{"a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 10, 1, 0, 0, 0, loc); !got[0].Equal(want) {
		t.Fatalf("got %s want %s", got[0], want)
	}
	if got[0].UTC().Day() != 9 {
		t.Fatalf("expected UTC instant on the 9th, got %s", got[0].UTC())
	}
}

func TestPostQueryMatchesAndNormalize(t *testing.T) {
	from := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	q := PostQuery{Statuses: []string{"scheduled"}, ChannelIDs: []string{"c1"}, BatchID: "b1", From: &from, To: &to, Limit: 9999, Offset: -5}
	q.Normalize()
	if q.Limit != MaxPostQueryLimit || q.Offset != 0 || q.Statuses[0] != "SCHEDULED" {
		t.Fatalf("normalize failed: %+v", q)
	}
	ok := &Post{Status: PostStatusScheduled, ChannelID: "c1", BatchID: "b1", ScheduledAt: from}
	if !q.Matches(ok) {
		t.Fatal("expected match")
	}
	for _, bad := range []*Post{
		{Status: PostStatusPublished, ChannelID: "c1", BatchID: "b1", ScheduledAt: from},
		{Status: PostStatusScheduled, ChannelID: "c2", BatchID: "b1", ScheduledAt: from},
		{Status: PostStatusScheduled, ChannelID: "c1", BatchID: "b2", ScheduledAt: from},
		{Status: PostStatusScheduled, ChannelID: "c1", BatchID: "b1", ScheduledAt: to},
		{Status: PostStatusScheduled, ChannelID: "c1", BatchID: "b1", ScheduledAt: from.Add(-time.Second)},
	} {
		if q.Matches(bad) {
			t.Fatalf("unexpected match: %+v", bad)
		}
	}
	empty := PostQuery{}
	empty.Normalize()
	if empty.Limit != 50 || !empty.Matches(ok) {
		t.Fatal("empty query defaults to 50 and matches everything")
	}
}

func TestPostApplyPatch(t *testing.T) {
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	retry := now.Add(time.Hour)
	p := &Post{Status: PostStatusFailed, MediaType: MediaTypeImage, MediaURLs: []string{"a"}, RetryCount: 2, NextRetryAt: &retry, LastErrorMessage: "x", QueueTier: QueueTierFast}
	content := "new"
	when := now.Add(2 * time.Hour)
	video := MediaTypeVideo
	urls := []string{"v.mp4"}
	if err := p.ApplyPatch(PostPatch{Content: &content, ScheduledAt: &when, MediaType: &video, MediaURLs: &urls}, now); err != nil {
		t.Fatal(err)
	}
	if p.Status != PostStatusScheduled || p.RetryCount != 0 || p.NextRetryAt != nil || p.LastErrorMessage != "" {
		t.Fatalf("failed post must reset for a fresh attempt: %+v", p)
	}
	if p.QueueTier != QueueTierHeavy || p.Content != "new" || !p.ScheduledAt.Equal(when) {
		t.Fatalf("patch not applied: %+v", p)
	}
	published := &Post{Status: PostStatusPublished}
	if err := published.ApplyPatch(PostPatch{Content: &content}, now); err != ErrPostNotEditable {
		t.Fatalf("expected ErrPostNotEditable, got %v", err)
	}
	pending := &Post{Status: PostStatusPendingApproval}
	if err := pending.ApplyPatch(PostPatch{}, now); err != nil || pending.Status != PostStatusPendingApproval {
		t.Fatalf("pending stays pending: %v %s", err, pending.Status)
	}
	if !(PostPatch{}).IsEmpty() || (PostPatch{Content: &content}).IsEmpty() {
		t.Fatal("IsEmpty wrong")
	}
}

func TestSchedulePlanClampStart(t *testing.T) {
	loc := mustLoc(t, "Asia/Ho_Chi_Minh")
	now := time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC) // 03:00 on the 10th in Ho Chi Minh
	p := SchedulePlan{StartDate: "2025-01-01", Timezone: "Asia/Ho_Chi_Minh", TimeSlots: []string{"09:00"}}
	p.ClampStart(now)
	if p.StartDate != "2026-11-10" {
		t.Fatalf("stale start must clamp to today in plan tz, got %s", p.StartDate)
	}
	future := SchedulePlan{StartDate: "2026-12-01", Timezone: "Asia/Ho_Chi_Minh", TimeSlots: []string{"09:00"}}
	future.ClampStart(now)
	if future.StartDate != "2026-12-01" {
		t.Fatal("future start must be kept")
	}
	empty := SchedulePlan{Timezone: "Asia/Ho_Chi_Minh", TimeSlots: []string{"09:00"}}
	empty.ClampStart(now)
	if empty.StartDate != now.In(loc).Format("2006-01-02") {
		t.Fatalf("empty start must become today, got %s", empty.StartDate)
	}
	malformed := SchedulePlan{StartDate: "1/12/2026", Timezone: "Asia/Ho_Chi_Minh", TimeSlots: []string{"09:00"}}
	malformed.ClampStart(now)
	if malformed.StartDate != "1/12/2026" {
		t.Fatalf("malformed start must be left for Normalize to reject, got %s", malformed.StartDate)
	}
	if err := malformed.Normalize(); err == nil {
		t.Fatal("malformed start must be rejected")
	}
}

func TestGenerateScheduleSlotsBoundaries(t *testing.T) {
	plan := SchedulePlan{StartDate: "2026-11-10", TimeSlots: []string{"09:00", "09:30", "10:00"}, MinGapMinutes: 30, AvoidConflicts: true}
	if err := plan.Normalize(); err != nil {
		t.Fatal(err)
	}
	// Lead time: a slot exactly 60s ahead is kept, 59s ahead is skipped.
	now := time.Date(2026, 11, 10, 8, 59, 0, 0, time.UTC)
	got, _, err := GenerateScheduleSlots(plan, 1, now, []string{"a"}, nil)
	if err != nil || !got[0].Equal(time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("slot at now+60s must be kept: %v %v", got, err)
	}
	got, _, _ = GenerateScheduleSlots(plan, 1, now.Add(time.Second), []string{"a"}, nil)
	if !got[0].Equal(time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC)) {
		t.Fatalf("slot at now+59s must be skipped: %v", got)
	}
	// Gap: exactly MinGap apart is allowed, one second less conflicts.
	occ := Occupancy{"a": {time.Date(2026, 11, 10, 8, 30, 0, 0, time.UTC)}}
	got, _, _ = GenerateScheduleSlots(plan, 1, time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), []string{"a"}, occ)
	if !got[0].Equal(time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("exactly 30 minutes apart must be allowed: %v", got)
	}
	occ = Occupancy{"a": {time.Date(2026, 11, 10, 8, 30, 1, 0, time.UTC)}}
	got, _, _ = GenerateScheduleSlots(plan, 1, time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), []string{"a"}, occ)
	if !got[0].Equal(time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC)) {
		t.Fatalf("29m59s apart must conflict: %v", got)
	}
}

func TestGenerateScheduleSlotsDST(t *testing.T) {
	loc := mustLoc(t, "Europe/Berlin")
	// 2026-03-29: clocks jump 02:00 -> 03:00; 02:30 does not exist. 2026-10-25: 02:30 happens twice.
	plan := SchedulePlan{StartDate: "2026-03-29", Timezone: "Europe/Berlin", TimeSlots: []string{"02:30", "12:00"}}
	if err := plan.Normalize(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC)
	got, _, err := GenerateScheduleSlots(plan, 2, now, []string{"a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].In(loc).Hour() != 3 || got[0].In(loc).Minute() != 30 {
		t.Fatalf("nonexistent 02:30 must normalise forward to 03:30 local, got %s", got[0].In(loc))
	}
	if !got[1].Equal(time.Date(2026, 3, 29, 12, 0, 0, 0, loc)) || got[1].Sub(got[0]) != 8*time.Hour+30*time.Minute {
		t.Fatalf("noon slot must follow the spring-forward gap: %s (delta %s)", got[1].In(loc), got[1].Sub(got[0]))
	}
	fall := SchedulePlan{StartDate: "2026-10-25", Timezone: "Europe/Berlin", TimeSlots: []string{"02:30", "03:00"}}
	if err := fall.Normalize(); err != nil {
		t.Fatal(err)
	}
	got, _, err = GenerateScheduleSlots(fall, 2, time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC), []string{"a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// time.Date resolves the repeated 02:30 to the post-transition offset (CET, UTC+1 → 01:30Z) in the Go
	// runtime used here; pin that instant so any change in resolution (and therefore gap arithmetic) is noticed.
	if want := time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC); !got[0].Equal(want) {
		t.Fatalf("ambiguous 02:30 must resolve to %s, got %s", want, got[0])
	}
	if got[1].Sub(got[0]) != 30*time.Minute {
		t.Fatalf("03:00 CET follows 02:30 CET by 30 minutes, got %s", got[1].Sub(got[0]))
	}
}

func TestNormalizeDraft(t *testing.T) {
	content, link, mediaType, urls, err := NormalizeDraft("  hi  ", "", "", []string{" https://x/a.png ", ""})
	if err != nil || content != "hi" || link != "" || mediaType != MediaTypeNone || len(urls) != 1 {
		t.Fatalf("normalisation wrong: %q %q %q %v %v", content, link, mediaType, urls, err)
	}
	if _, _, mt, _, err := NormalizeDraft("", "https://site", "", nil); err != nil || mt != MediaTypeLink {
		t.Fatalf("link_url alone must infer LINK: %s %v", mt, err)
	}
	bad := []struct {
		c, l, m string
		u       []string
	}{
		{"", "", "", nil},
		{"x", "", "IMAGE", nil},
		{"x", "", "LINK", nil},
		{"x", "", "HOLOGRAM", nil},
		{strings.Repeat("a", MaxContentLength+1), "", "", nil},
		{"x", "", "ALBUM", make([]string, MaxMediaURLs+1)},
	}
	for i, b := range bad {
		for j := range b.u {
			b.u[j] = "https://x/" + string(rune('a'+j))
		}
		if _, _, _, _, err := NormalizeDraft(b.c, b.l, b.m, b.u); err == nil {
			t.Fatalf("case %d must fail", i)
		}
	}
}

func TestApplyPatchContentOnlyFailedAndReview(t *testing.T) {
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	retry := now.Add(time.Hour)
	content := "fixed text"
	failed := &Post{Status: PostStatusFailed, RetryCount: 3, NextRetryAt: &retry, LastErrorMessage: "boom", MediaURLs: []string{}}
	if err := failed.ApplyPatch(PostPatch{Content: &content}, now); err != nil {
		t.Fatal(err)
	}
	if failed.Status != PostStatusFailed || failed.RetryCount != 3 || failed.NextRetryAt == nil {
		t.Fatalf("content-only edit must not re-arm a failed post: %+v", failed)
	}
	when := now.Add(2 * time.Hour)
	approved := &Post{Status: PostStatusScheduled, ReviewedBy: "approver", ReviewedAt: &now, MediaURLs: []string{}}
	if err := approved.ApplyPatch(PostPatch{Content: &content, ScheduledAt: &when, RequireReview: true}, now); err != nil {
		t.Fatal(err)
	}
	if approved.Status != PostStatusPendingApproval || approved.ReviewedBy != "" || approved.ReviewedAt != nil {
		t.Fatalf("review-required patch must clear the previous decision: %+v", approved)
	}
	if !(PostPatch{Content: &content}).ChangesContent() || (PostPatch{ScheduledAt: &when}).ChangesContent() {
		t.Fatal("ChangesContent wrong")
	}
}
