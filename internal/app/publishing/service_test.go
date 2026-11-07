package publishing

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/database"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

type fakeQueue struct {
	jobs []domain.PublishJobPayload
	fail bool
}

func (q *fakeQueue) EnqueuePublishJob(_ context.Context, p domain.PublishJobPayload) error {
	if q.fail {
		return errors.New("redis down")
	}
	q.jobs = append(q.jobs, p)
	return nil
}

type fakePerms struct{ roles map[string]domain.GroupRole }

func (f fakePerms) ResolveChannelPermission(_ context.Context, channelID string) (domain.GroupRole, bool) {
	r, ok := f.roles[channelID]
	return r, ok
}

func newFixture(t *testing.T, role domain.OrgRole) (*Service, *database.InMemoryStore, *fakeQueue, context.Context, []string) {
	return newFixtureN(t, role, 2)
}

func newFixtureN(t *testing.T, role domain.OrgRole, channels int) (*Service, *database.InMemoryStore, *fakeQueue, context.Context, []string) {
	t.Helper()
	store := database.NewInMemoryStore()
	ctx := domain.WithTenantContext(context.Background(), &domain.TenantContext{UserID: domain.DefaultUserID, OrganizationID: domain.DefaultOrganizationID, OrgRole: role})
	var ids []string
	exts := make([]string, 0, channels)
	for i := 0; i < channels; i++ {
		exts = append(exts, "svc-"+string(rune('a'+i)))
	}
	for _, ext := range exts {
		ch, err := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: ext, Name: ext, EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, ch.ID)
	}
	queue := &fakeQueue{}
	roles := map[string]domain.GroupRole{}
	for _, id := range ids {
		roles[id] = domain.GroupRoleEditor
	}
	perms := fakePerms{roles: roles}
	svc := NewService(store, queue, perms, slog.New(slog.NewTextHandler(io.Discard, nil))).WithClock(func() time.Time { return time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC) })
	return svc, store, queue, ctx, ids
}

func TestResolveTargetsRules(t *testing.T) {
	svc, store, _, ctx, ids := newFixture(t, domain.OrgRoleMember)
	if _, err := svc.ResolveTargets(ctx, Targets{}, false); err == nil || err.Status != http.StatusBadRequest {
		t.Fatalf("missing target must be 400, got %v", err)
	}
	if _, err := svc.ResolveTargets(ctx, Targets{ChannelID: "ghost"}, false); err == nil || err.Status != http.StatusBadRequest {
		t.Fatalf("unknown channel must be 400, got %v", err)
	}
	res, err := svc.ResolveTargets(ctx, Targets{ChannelIDs: []string{ids[0], " ", ids[0], ids[1]}}, false)
	if err != nil || len(res.Targets) != 2 || !res.FanOut {
		t.Fatalf("duplicates/blanks must collapse: %+v %v", res, err)
	}
	// Approval-required group makes EDITOR + publish_now forbidden, otherwise allowed.
	cg, _ := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "strict", RequireApproval: true})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{ids[0]})
	if _, err := svc.ResolveTargets(ctx, Targets{ChannelID: ids[0]}, true); err == nil || err.Status != http.StatusForbidden {
		t.Fatalf("editor publish_now on approval channel must be 403, got %v", err)
	}
	res, err = svc.ResolveTargets(ctx, Targets{ChannelID: ids[0]}, false)
	if err != nil || !res.Targets[0].RequireApproval {
		t.Fatalf("approval flag must be derived from group membership: %+v %v", res, err)
	}
	// Viewer is rejected; unknown permission for a plain member is rejected; admin falls back to MANAGER.
	svc.perms = fakePerms{roles: map[string]domain.GroupRole{ids[0]: domain.GroupRoleViewer}}
	if _, err := svc.ResolveTargets(ctx, Targets{ChannelID: ids[0]}, false); err == nil || err.Status != http.StatusForbidden {
		t.Fatalf("viewer must be 403, got %v", err)
	}
	if _, err := svc.ResolveTargets(ctx, Targets{ChannelID: ids[1]}, false); err == nil || err.Status != http.StatusForbidden {
		t.Fatalf("member without grant must be 403, got %v", err)
	}
	adminCtx := domain.WithTenantContext(context.Background(), &domain.TenantContext{UserID: domain.DefaultUserID, OrganizationID: domain.DefaultOrganizationID, OrgRole: domain.OrgRoleOwner})
	res, err = svc.ResolveTargets(adminCtx, Targets{ChannelID: ids[1]}, true)
	if err != nil || res.Targets[0].Role != domain.GroupRoleManager {
		t.Fatalf("owner must resolve to MANAGER: %+v %v", res, err)
	}
}

func TestCreateFanOutIsTransactionalAndEnqueues(t *testing.T) {
	svc, store, queue, ctx, ids := newFixture(t, domain.OrgRoleOwner)
	res, err := svc.Create(ctx, Targets{ChannelIDs: ids}, Draft{Content: "hi"}, Options{PublishNow: true})
	if err != nil || len(res.Posts) != 2 || res.BatchID == "" {
		t.Fatalf("fan-out create failed: %+v %v", res, err)
	}
	if len(queue.jobs) != 2 || queue.jobs[0].OrganizationID != domain.DefaultOrganizationID {
		t.Fatalf("publish_now must enqueue one tenant-scoped job per post: %+v", queue.jobs)
	}
	batch, berr := store.GetPostBatch(ctx, res.BatchID)
	if berr != nil || batch.Kind != domain.PostBatchKindFanout {
		t.Fatalf("fan-out must record a FANOUT batch: %+v %v", batch, berr)
	}
	queue.fail = true
	if _, err := svc.Create(ctx, Targets{ChannelID: ids[0]}, Draft{Content: "x"}, Options{PublishNow: true}); err == nil || err.Status != http.StatusBadGateway {
		t.Fatalf("queue outage must be 502, got %v", err)
	}
	failed, _ := store.QueryPosts(ctx, domain.PostQuery{Statuses: []string{domain.PostStatusFailed}})
	if len(failed) != 1 || failed[0].NextRetryAt == nil {
		t.Fatalf("queue outage must leave a retryable FAILED post: %+v", failed)
	}
}

func TestPlanBulkAssignsAndRespectsExistingPosts(t *testing.T) {
	svc, store, _, ctx, ids := newFixture(t, domain.OrgRoleOwner)
	occupied := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC) // 09:00 Ho Chi Minh on the 10th
	if _, err := store.CreatePost(ctx, &domain.Post{ChannelID: ids[1], Content: "busy", Status: domain.PostStatusScheduled, ScheduledAt: occupied, MediaType: domain.MediaTypeNone}); err != nil {
		t.Fatal(err)
	}
	req := &BulkRequest{
		Targets: Targets{ChannelIDs: ids},
		Items:   []BulkItem{{Content: "one"}, {Content: "two", MediaType: "IMAGE", MediaURLs: []string{"https://x/a.png"}}, {Content: "three"}},
		Plan:    &domain.SchedulePlan{StartDate: "2026-11-10", Timezone: "Asia/Ho_Chi_Minh", TimeSlots: []string{"09:00", "19:30"}, AvoidConflicts: true, MinGapMinutes: 30},
	}
	plan, err := svc.PlanBulk(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	want := []time.Time{time.Date(2026, 11, 10, 19, 30, 0, 0, loc), time.Date(2026, 11, 11, 9, 0, 0, 0, loc), time.Date(2026, 11, 11, 19, 30, 0, 0, loc)}
	for i, slot := range plan.Slots {
		if !slot.ScheduledAt.Equal(want[i]) {
			t.Fatalf("slot %d: got %s want %s", i, slot.ScheduledAt.In(loc), want[i])
		}
	}
	if all, _ := store.QueryPosts(ctx, domain.PostQuery{}); len(all) != 1 {
		t.Fatalf("PlanBulk must not write, found %d posts", len(all))
	}
	if req.Items[1].MediaType != domain.MediaTypeImage {
		t.Fatal("media type must be normalised in place")
	}

	// Validation failures
	bad := []BulkRequest{
		{Targets: Targets{ChannelID: ids[0]}, Items: nil},
		{Targets: Targets{ChannelID: ids[0]}, Items: []BulkItem{{Content: "", MediaType: "IMAGE"}}, Plan: req.Plan},
		{Targets: Targets{ChannelID: ids[0]}, Items: []BulkItem{{Content: "x", MediaType: "LINK"}}, Plan: req.Plan},
		{Targets: Targets{ChannelID: ids[0]}, Items: []BulkItem{{Content: "x", MediaType: "HOLOGRAM"}}, Plan: req.Plan},
		{Targets: Targets{ChannelID: ids[0]}, Items: []BulkItem{{Content: "x"}}},
		{Targets: Targets{ChannelID: ids[0]}, Items: make([]BulkItem, domain.MaxBulkItems+1)},
	}
	for i := range bad {
		if _, err := svc.PlanBulk(ctx, &bad[i]); err == nil || err.Status != http.StatusBadRequest {
			t.Fatalf("case %d expected 400, got %v", i, err)
		}
	}
	// Posts cap: 100 items × 6 channels = 600 > 500.
	svc6, _, _, ctx6, ids6 := newFixtureN(t, domain.OrgRoleOwner, 6)
	many := &BulkRequest{Targets: Targets{ChannelIDs: ids6}, Items: make([]BulkItem, domain.MaxBulkItems), Plan: &domain.SchedulePlan{StartDate: "2026-11-10", TimeSlots: []string{"09:00"}}}
	for i := range many.Items {
		many.Items[i].Content = "x"
	}
	if _, err := svc6.PlanBulk(ctx6, many); err == nil || err.Status != http.StatusBadRequest || !strings.Contains(err.Message, "limit is 500") {
		t.Fatalf("exceeding MaxBulkPosts must be 400, got %v", err)
	}
	if _, err := svc.PlanBulk(ctx, &BulkRequest{Targets: Targets{ChannelID: ids[0]}, Items: []BulkItem{{Content: "x"}}, Plan: req.Plan, PublishNow: true}); err == nil || err.Status != http.StatusBadRequest {
		t.Fatalf("publish_now must be rejected for bulk, got %v", err)
	}
	past := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC) // before the injected clock (2026-11-09)
	pastPlan, err := svc.PlanBulk(ctx, &BulkRequest{Targets: Targets{ChannelID: ids[0]}, Items: []BulkItem{{Content: "late", ScheduledAt: &past}}})
	if err != nil || pastPlan.Slots[0].NoteCode != domain.NotePastTime || !pastPlan.Slots[0].Explicit || pastPlan.Slots[0].Note == "" {
		t.Fatalf("explicit past time must be annotated PAST_TIME: %+v %v", pastPlan, err)
	}
	if _, err := svc.PlanBulk(ctx, &BulkRequest{Name: strings.Repeat("n", domain.MaxBatchNameLength+1), Targets: Targets{ChannelID: ids[0]}, Items: []BulkItem{{Content: "x"}}, Plan: req.Plan}); err == nil || err.Status != http.StatusBadRequest {
		t.Fatalf("overlong name must be 400, got %v", err)
	}

	created, err := svc.CreateBulk(ctx, req)
	if err != nil || len(created.Posts) != 6 || created.Batch.Plan == nil {
		t.Fatalf("CreateBulk failed: %+v %v", created, err)
	}
	for _, p := range created.Posts {
		if p.Status != domain.PostStatusScheduled || p.BatchID != created.Batch.ID {
			t.Fatalf("bulk post wrong: %+v", p)
		}
	}
	logs, _ := store.ListAuditLogs(ctx, domain.DefaultOrganizationID, 10)
	if len(logs) == 0 || logs[0].Action != "POST_BULK_CREATE" {
		t.Fatalf("bulk create must be audited: %+v", logs)
	}
}

func TestUpdateRules(t *testing.T) {
	svc, store, _, ctx, ids := newFixture(t, domain.OrgRoleOwner)
	when := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	post, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ids[0], Content: "a", Status: domain.PostStatusScheduled, ScheduledAt: when, MediaType: domain.MediaTypeNone})
	if _, err := svc.Update(ctx, post.ID, domain.PostPatch{}); err == nil || err.Status != http.StatusBadRequest {
		t.Fatalf("empty patch must be 400, got %v", err)
	}
	empty := ""
	if _, err := svc.Update(ctx, post.ID, domain.PostPatch{Content: &empty}); err == nil || err.Status != http.StatusBadRequest {
		t.Fatalf("blank post must be 400, got %v", err)
	}
	later := when.Add(time.Hour)
	updated, err := svc.Update(ctx, post.ID, domain.PostPatch{ScheduledAt: &later})
	if err != nil || !updated.ScheduledAt.Equal(later) {
		t.Fatalf("reschedule failed: %+v %v", updated, err)
	}
	_ = store.MarkPostProcessing(ctx, post.ID)
	if _, err := svc.Update(ctx, post.ID, domain.PostPatch{ScheduledAt: &later}); err == nil || err.Status != http.StatusConflict {
		t.Fatalf("processing post must be 409, got %v", err)
	}
	if _, err := svc.Update(ctx, "missing", domain.PostPatch{ScheduledAt: &later}); err == nil || err.Status != http.StatusNotFound {
		t.Fatalf("missing post must be 404, got %v", err)
	}
}

type failSecondQueue struct {
	calls int
	jobs  []domain.PublishJobPayload
}

func (q *failSecondQueue) EnqueuePublishJob(_ context.Context, p domain.PublishJobPayload) error {
	q.calls++
	if q.calls == 2 {
		return errors.New("redis blip")
	}
	q.jobs = append(q.jobs, p)
	return nil
}

func TestCreateFanOutEnqueueFailureParksRemainingPosts(t *testing.T) {
	svc, store, _, ctx, ids := newFixtureN(t, domain.OrgRoleOwner, 3)
	q := &failSecondQueue{}
	svc.queue = q
	_, err := svc.Create(ctx, Targets{ChannelIDs: ids}, Draft{Content: "hi"}, Options{PublishNow: true})
	if err == nil || err.Status != http.StatusBadGateway {
		t.Fatalf("expected 502, got %v", err)
	}
	queued, _ := store.QueryPosts(ctx, domain.PostQuery{Statuses: []string{domain.PostStatusQueued}})
	if len(queued) != 1 {
		t.Fatalf("only the successfully enqueued post may stay QUEUED, got %d", len(queued))
	}
	failed, _ := store.QueryPosts(ctx, domain.PostQuery{Statuses: []string{domain.PostStatusFailed}})
	if len(failed) != 2 {
		t.Fatalf("the failed and the never-enqueued posts must be retryable FAILED, got %d", len(failed))
	}
	for _, p := range failed {
		if p.NextRetryAt == nil {
			t.Fatalf("parked post must carry next_retry_at: %+v", p)
		}
	}
}

func TestCreateSingleChannelIDsIsNotFanOut(t *testing.T) {
	svc, store, _, ctx, ids := newFixture(t, domain.OrgRoleOwner)
	res, err := svc.Create(ctx, Targets{ChannelIDs: []string{ids[0]}}, Draft{Content: "solo"}, Options{})
	if err != nil || res.FanOut || res.BatchID != "" {
		t.Fatalf("one channel id must behave like channel_id: %+v %v", res, err)
	}
	if _, err := svc.Create(ctx, Targets{ChannelID: ids[0]}, Draft{Content: "   ", MediaType: "IMAGE"}, Options{}); err == nil || err.Status != http.StatusBadRequest {
		t.Fatalf("single create must apply the shared draft rules, got %v", err)
	}
	logs, _ := store.ListAuditLogs(ctx, domain.DefaultOrganizationID, 10)
	if len(logs) == 0 || logs[0].Action != "POST_CREATE" {
		t.Fatalf("single create must be audited: %+v", logs)
	}
}

func TestUpdateEditorOnApprovalChannelReturnsToReview(t *testing.T) {
	svc, store, _, ctx, ids := newFixture(t, domain.OrgRoleMember)
	cg, _ := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "strict", RequireApproval: true})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{ids[0]})
	when := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	reviewed := when.Add(-time.Hour)
	approved, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ids[0], Content: "approved text", Status: domain.PostStatusScheduled, ScheduledAt: when, ReviewedBy: "approver", ReviewedAt: &reviewed, MediaType: domain.MediaTypeNone})
	later := when.Add(time.Hour)

	// Schedule-only change keeps the approval.
	updated, err := svc.Update(ctx, approved.ID, domain.PostPatch{ScheduledAt: &later})
	if err != nil || updated.Status != domain.PostStatusScheduled || updated.ReviewedBy != "approver" {
		t.Fatalf("schedule-only edit must keep approval: %+v %v", updated, err)
	}
	// Content change by an EDITOR goes back to review.
	content := "swapped text"
	updated, err = svc.Update(ctx, approved.ID, domain.PostPatch{Content: &content})
	if err != nil || updated.Status != domain.PostStatusPendingApproval || updated.ReviewedBy != "" || updated.ReviewedAt != nil {
		t.Fatalf("editor content edit must require re-approval: %+v %v", updated, err)
	}
	// Same edit on a channel without approval requirement stays SCHEDULED.
	free, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ids[1], Content: "free", Status: domain.PostStatusScheduled, ScheduledAt: when, MediaType: domain.MediaTypeNone})
	updated, err = svc.Update(ctx, free.ID, domain.PostPatch{Content: &content})
	if err != nil || updated.Status != domain.PostStatusScheduled {
		t.Fatalf("editor edit on an open channel stays scheduled: %+v %v", updated, err)
	}
	// Owner editing the approval channel keeps the approval.
	ownerCtx := domain.WithTenantContext(context.Background(), &domain.TenantContext{UserID: domain.DefaultUserID, OrganizationID: domain.DefaultOrganizationID, OrgRole: domain.OrgRoleOwner})
	svc.perms = fakePerms{roles: map[string]domain.GroupRole{ids[0]: domain.GroupRoleManager}}
	second, _ := store.CreatePost(ownerCtx, &domain.Post{ChannelID: ids[0], Content: "owner text", Status: domain.PostStatusScheduled, ScheduledAt: when, ReviewedBy: "approver", ReviewedAt: &reviewed, MediaType: domain.MediaTypeNone})
	updated, err = svc.Update(ownerCtx, second.ID, domain.PostPatch{Content: &content})
	if err != nil || updated.Status != domain.PostStatusScheduled || updated.ReviewedBy != "approver" {
		t.Fatalf("owner edit must not require re-approval: %+v %v", updated, err)
	}
	// DRAFT edits by an editor stay drafts (no implicit submission); submitter is re-attributed when review is required.
	svc.perms = fakePerms{roles: map[string]domain.GroupRole{ids[0]: domain.GroupRoleEditor}}
	draft, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ids[0], Content: "draft", Status: domain.PostStatusDraft, ScheduledAt: when, MediaType: domain.MediaTypeNone})
	updated, err = svc.Update(ctx, draft.ID, domain.PostPatch{Content: &content})
	if err != nil || updated.Status != domain.PostStatusDraft {
		t.Fatalf("draft must stay draft: %+v %v", updated, err)
	}
	if pending, _ := store.GetPostByID(ctx, approved.ID); pending.SubmittedBy != domain.DefaultUserID {
		t.Fatalf("submitter must be the editing user after re-review, got %q", pending.SubmittedBy)
	}
	svc.perms = fakePerms{roles: map[string]domain.GroupRole{ids[0]: domain.GroupRoleManager}}
	// Content is persisted trimmed.
	padded := "  padded  "
	updated, _ = svc.Update(ownerCtx, second.ID, domain.PostPatch{Content: &padded})
	if updated.Content != "padded" {
		t.Fatalf("content must be stored normalised, got %q", updated.Content)
	}
}
