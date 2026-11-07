package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Scheduling limits shared by the API and the application service.
const (
	MaxPlanTimeSlots   = 12
	MaxBulkItems       = 100
	MaxBulkPosts       = 500
	DefaultMinGap      = 30 * time.Minute
	MaxMinGapMinutes   = 7 * 24 * 60 // one week
	MaxMediaURLs       = 10
	MaxContentLength   = 63206 // Facebook post character limit
	MaxBatchNameLength = 255
	maxPlanHorizonDays = 400
	minLeadTime        = time.Minute
)

// Machine-readable schedule note codes (ScheduleSlot.NoteCode / plan warnings).
const (
	NoteConflictsSkipped   = "CONFLICTS_SKIPPED"   // NoteValue = number of skipped slots
	NotePastTime           = "PAST_TIME"           // explicit time already elapsed
	NoteOccupancyTruncated = "OCCUPANCY_TRUNCATED" // NoteValue = posts considered
)

// ErrPostNotEditable is returned when a post is no longer in an editable lifecycle state.
var ErrPostNotEditable = errors.New("post is not editable in its current status")

// SchedulePlan describes how publication times are distributed for a series of posts.
// Times are expressed as local wall-clock slots in Timezone and converted to UTC instants.
type SchedulePlan struct {
	StartDate      string   `json:"start_date"`      // YYYY-MM-DD in Timezone; empty = today
	Timezone       string   `json:"timezone"`        // IANA name, e.g. Asia/Ho_Chi_Minh; empty = UTC
	TimeSlots      []string `json:"time_slots"`      // HH:MM local times, at most MaxPlanTimeSlots
	Weekdays       []int    `json:"weekdays"`        // time.Weekday numbers (0 = Sunday); empty = every day
	MinGapMinutes  int      `json:"min_gap_minutes"` // minimum distance to another post on the same channel
	AvoidConflicts bool     `json:"avoid_conflicts"` // skip slots that collide with existing posts
}

// Normalize validates the plan and canonicalises slot order, weekday set and defaults.
func (p *SchedulePlan) Normalize() error {
	if p == nil {
		return errors.New("schedule_plan is required")
	}
	loc, err := p.Location()
	if err != nil {
		return err
	}
	if p.StartDate == "" {
		p.StartDate = time.Now().In(loc).Format("2006-01-02")
	}
	if _, err := time.ParseInLocation("2006-01-02", p.StartDate, loc); err != nil {
		return fmt.Errorf("schedule_plan.start_date must be YYYY-MM-DD: %w", err)
	}
	if len(p.TimeSlots) == 0 {
		return errors.New("schedule_plan.time_slots must contain at least one HH:MM value")
	}
	if len(p.TimeSlots) > MaxPlanTimeSlots {
		return fmt.Errorf("schedule_plan.time_slots supports at most %d values", MaxPlanTimeSlots)
	}
	seen := make(map[string]bool, len(p.TimeSlots))
	slots := make([]string, 0, len(p.TimeSlots))
	for _, raw := range p.TimeSlots {
		slot := strings.TrimSpace(raw)
		t, err := time.Parse("15:04", slot)
		if err != nil {
			return fmt.Errorf("schedule_plan.time_slots entry %q must be HH:MM", raw)
		}
		slot = t.Format("15:04")
		if !seen[slot] {
			seen[slot] = true
			slots = append(slots, slot)
		}
	}
	sort.Strings(slots)
	p.TimeSlots = slots

	days := make([]int, 0, len(p.Weekdays))
	seenDay := make(map[int]bool, 7)
	for _, d := range p.Weekdays {
		if d < 0 || d > 6 {
			return fmt.Errorf("schedule_plan.weekdays entry %d must be between 0 (Sunday) and 6 (Saturday)", d)
		}
		if !seenDay[d] {
			seenDay[d] = true
			days = append(days, d)
		}
	}
	sort.Ints(days)
	p.Weekdays = days

	if p.MinGapMinutes < 0 {
		return errors.New("schedule_plan.min_gap_minutes cannot be negative")
	}
	if p.MinGapMinutes == 0 {
		p.MinGapMinutes = int(DefaultMinGap / time.Minute)
	}
	if p.MinGapMinutes > MaxMinGapMinutes {
		return fmt.Errorf("schedule_plan.min_gap_minutes cannot exceed %d (one week)", MaxMinGapMinutes)
	}
	return nil
}

// ClampStart moves StartDate to today (in the plan timezone) when it lies in the past, so a stale
// remembered date still schedules from now. now is injected by the caller for determinism.
func (p *SchedulePlan) ClampStart(now time.Time) {
	loc, err := p.Location()
	if err != nil {
		return
	}
	today := now.In(loc).Format("2006-01-02")
	if p.StartDate == "" {
		p.StartDate = today
		return
	}
	// Only a well-formed past date is clamped; malformed values are left for Normalize to reject.
	if parsed, err := time.ParseInLocation("2006-01-02", p.StartDate, loc); err == nil && parsed.Format("2006-01-02") < today {
		p.StartDate = today
	}
}

// Location resolves the plan timezone, defaulting to UTC.
func (p SchedulePlan) Location() (*time.Location, error) {
	name := strings.TrimSpace(p.Timezone)
	if name == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("schedule_plan.timezone %q is not a valid IANA timezone", name)
	}
	return loc, nil
}

// MinGap returns the configured minimum gap as a duration.
func (p SchedulePlan) MinGap() time.Duration {
	return time.Duration(p.MinGapMinutes) * time.Minute
}

func (p SchedulePlan) allowsWeekday(d time.Weekday) bool {
	if len(p.Weekdays) == 0 {
		return true
	}
	for _, allowed := range p.Weekdays {
		if time.Weekday(allowed) == d {
			return true
		}
	}
	return false
}

// Occupancy records the publication instants already taken on each channel.
type Occupancy map[string][]time.Time

// Add records an instant for a channel.
func (o Occupancy) Add(channelID string, at time.Time) {
	o[channelID] = append(o[channelID], at.UTC())
}

// Conflict reports whether an existing instant on any of the channels falls within gap of at.
func (o Occupancy) Conflict(channels []string, at time.Time, gap time.Duration) bool {
	for _, ch := range channels {
		for _, existing := range o[ch] {
			delta := existing.Sub(at)
			if delta < 0 {
				delta = -delta
			}
			if delta < gap {
				return true
			}
		}
	}
	return false
}

// ScheduleSlot is one assigned publication instant for a bulk item.
type ScheduleSlot struct {
	Index       int       `json:"index"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Explicit    bool      `json:"explicit"`
	NoteCode    string    `json:"note_code,omitempty"`
	NoteValue   int       `json:"note_value,omitempty"`
	Note        string    `json:"note,omitempty"` // human-readable fallback for NoteCode
}

// SlotNote is a machine-readable annotation produced while generating slots.
type SlotNote struct {
	Code  string
	Value int
}

func (n SlotNote) String() string {
	switch n.Code {
	case NoteConflictsSkipped:
		return fmt.Sprintf("skipped %d conflicting slot(s)", n.Value)
	case NotePastTime:
		return "time is in the past; the post will publish at the next scheduler run"
	case NoteOccupancyTruncated:
		return fmt.Sprintf("only the next %d existing posts were considered for conflict detection", n.Value)
	}
	return ""
}

// GenerateScheduleSlots assigns count instants following the plan.
// It walks days from StartDate, keeps allowed weekdays and listed slots, skips instants earlier than
// now plus a short lead time and, when AvoidConflicts is set, instants within MinGap of occupied times
// on any target channel. Assigned instants are added to occ so later items keep the same gap.
func GenerateScheduleSlots(plan SchedulePlan, count int, now time.Time, channels []string, occ Occupancy) ([]time.Time, []SlotNote, error) {
	if count <= 0 {
		return nil, nil, nil
	}
	loc, err := plan.Location()
	if err != nil {
		return nil, nil, err
	}
	start, err := time.ParseInLocation("2006-01-02", plan.StartDate, loc)
	if err != nil {
		return nil, nil, fmt.Errorf("schedule_plan.start_date must be YYYY-MM-DD: %w", err)
	}
	if occ == nil {
		occ = Occupancy{}
	}
	earliest := now.UTC().Add(minLeadTime)
	gap := plan.MinGap()
	result := make([]time.Time, 0, count)
	notes := make([]SlotNote, 0, count)
	skippedConflicts := 0
	type hm struct{ h, m int }
	parsed := make([]hm, 0, len(plan.TimeSlots))
	for _, slot := range plan.TimeSlots {
		t, err := time.Parse("15:04", slot)
		if err != nil {
			return nil, nil, fmt.Errorf("schedule_plan.time_slots entry %q must be HH:MM", slot)
		}
		parsed = append(parsed, hm{t.Hour(), t.Minute()})
	}

	for day := 0; day < maxPlanHorizonDays && len(result) < count; day++ {
		date := start.AddDate(0, 0, day)
		if !plan.allowsWeekday(date.Weekday()) {
			continue
		}
		for _, slot := range parsed {
			if len(result) >= count {
				break
			}
			at := time.Date(date.Year(), date.Month(), date.Day(), slot.h, slot.m, 0, 0, loc).UTC()
			if at.Before(earliest) {
				continue
			}
			if plan.AvoidConflicts && occ.Conflict(channels, at, gap) {
				skippedConflicts++
				continue
			}
			result = append(result, at)
			note := SlotNote{}
			if skippedConflicts > 0 {
				note = SlotNote{Code: NoteConflictsSkipped, Value: skippedConflicts}
				skippedConflicts = 0
			}
			notes = append(notes, note)
			if plan.AvoidConflicts {
				for _, ch := range channels {
					occ.Add(ch, at)
				}
			}
		}
	}
	if len(result) < count {
		return nil, nil, fmt.Errorf("schedule_plan cannot place %d posts within %d days; add time slots or weekdays", count, maxPlanHorizonDays)
	}
	return result, notes, nil
}

// ActivePostStatuses lists the statuses that still occupy a publication slot.
func ActivePostStatuses() []string {
	return []string{PostStatusDraft, PostStatusPendingApproval, PostStatusScheduled, PostStatusQueued, PostStatusProcessing, PostStatusFailed}
}

// EditablePostStatuses lists the statuses in which content and schedule may still be changed.
func EditablePostStatuses() []string {
	return []string{PostStatusDraft, PostStatusScheduled, PostStatusPendingApproval, PostStatusFailed}
}

// PostQuery filters a tenant-scoped post listing.
type PostQuery struct {
	Statuses   []string
	ChannelIDs []string
	BatchID    string
	From       *time.Time // inclusive
	To         *time.Time // exclusive
	Limit      int
	Offset     int
	Ascending  bool
}

// MaxPostQueryLimit bounds any single listing.
const MaxPostQueryLimit = 500

// Normalize clamps limit/offset and upper-cases statuses.
func (q *PostQuery) Normalize() {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > MaxPostQueryLimit {
		q.Limit = MaxPostQueryLimit
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	for i, s := range q.Statuses {
		q.Statuses[i] = strings.ToUpper(strings.TrimSpace(s))
	}
}

// Matches reports whether a post satisfies the query filters (used by the in-memory store and tests).
func (q PostQuery) Matches(p *Post) bool {
	if len(q.Statuses) > 0 {
		ok := false
		for _, s := range q.Statuses {
			if p.Status == s {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(q.ChannelIDs) > 0 {
		ok := false
		for _, id := range q.ChannelIDs {
			if p.ChannelID == id {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if q.BatchID != "" && p.BatchID != q.BatchID {
		return false
	}
	if q.From != nil && p.ScheduledAt.Before(*q.From) {
		return false
	}
	if q.To != nil && !p.ScheduledAt.Before(*q.To) {
		return false
	}
	return true
}

// Batch kinds.
const (
	PostBatchKindBulk   = "BULK"   // several different posts composed together
	PostBatchKindFanout = "FANOUT" // one content fanned out to several channels
)

// PostBatch groups posts created together and keeps the plan used to schedule them.
type PostBatch struct {
	ID             string        `json:"id"`
	OrganizationID string        `json:"organization_id"`
	Name           string        `json:"name"`
	Kind           string        `json:"kind"`
	Plan           *SchedulePlan `json:"schedule_plan,omitempty"`
	ItemCount      int           `json:"item_count"`
	CreatedBy      string        `json:"created_by"`
	CreatedAt      time.Time     `json:"created_at"`
}

// NormalizeDraft trims and validates the content/media invariants shared by single, bulk and edit paths.
// It returns the normalised media type and URL list.
func NormalizeDraft(content, linkURL, mediaType string, mediaURLs []string) (string, string, string, []string, error) {
	content = strings.TrimSpace(content)
	linkURL = strings.TrimSpace(linkURL)
	mediaType = NormalizeMediaType(mediaType, linkURL)
	urls := make([]string, 0, len(mediaURLs))
	for _, u := range mediaURLs {
		if trimmed := strings.TrimSpace(u); trimmed != "" {
			urls = append(urls, trimmed)
		}
	}
	switch mediaType {
	case MediaTypeNone, MediaTypeLink, MediaTypeImage, MediaTypeAlbum, MediaTypeVideo, MediaTypeReel:
	default:
		return "", "", "", nil, fmt.Errorf("media_type %q is not supported", mediaType)
	}
	if len([]rune(content)) > MaxContentLength {
		return "", "", "", nil, fmt.Errorf("content exceeds %d characters", MaxContentLength)
	}
	if len(urls) > MaxMediaURLs {
		return "", "", "", nil, fmt.Errorf("media_urls supports at most %d entries", MaxMediaURLs)
	}
	if len(linkURL) > 2048 {
		return "", "", "", nil, errors.New("link_url is too long")
	}
	if mediaType == MediaTypeLink && linkURL == "" {
		return "", "", "", nil, errors.New("media_type LINK requires link_url")
	}
	if (mediaType == MediaTypeImage || mediaType == MediaTypeAlbum || mediaType == MediaTypeVideo || mediaType == MediaTypeReel) && len(urls) == 0 {
		return "", "", "", nil, fmt.Errorf("media_type %s requires at least one media_url", mediaType)
	}
	if content == "" && len(urls) == 0 && linkURL == "" {
		return "", "", "", nil, errors.New("post needs content, media or a link")
	}
	return content, linkURL, mediaType, urls, nil
}

// PostPatch carries optional edits for an editable post. Nil fields are left untouched.
type PostPatch struct {
	ScheduledAt *time.Time
	Content     *string
	LinkURL     *string
	MediaType   *string
	MediaURLs   *[]string
	// RequireReview sends the post back to PENDING_APPROVAL (reviewer cleared); set by the
	// application layer when an EDITOR edits content on an approval-required channel.
	RequireReview bool
	// SubmittedBy re-attributes the submission to the editing user when RequireReview fires.
	SubmittedBy string
}

// ChangesContent reports whether the patch touches what a reviewer approved (text, link or media).
func (p PostPatch) ChangesContent() bool {
	return p.Content != nil || p.LinkURL != nil || p.MediaType != nil || p.MediaURLs != nil
}

// IsEmpty reports whether the patch changes nothing.
func (p PostPatch) IsEmpty() bool {
	return p.ScheduledAt == nil && p.Content == nil && p.LinkURL == nil && p.MediaType == nil && p.MediaURLs == nil
}
