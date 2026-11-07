package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

func doJSON(t *testing.T, srv *Server, h http.Handler, method, path, body, role string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	}
	if role != "" {
		authenticateTestRole(t, srv, req, role, "")
	}
	rec := httptest.NewRecorder()
	serveTest(t, srv, h, rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("invalid JSON response: %v %s", err, rec.Body.String())
	}
}

func countPosts(t *testing.T, srv *Server, h http.Handler) int {
	t.Helper()
	rec := doJSON(t, srv, h, http.MethodGet, "/api/v1/posts?limit=500", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("listing expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Posts *[]domain.Post `json:"posts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Posts == nil {
		t.Fatalf("listing must return a posts array: %v %s", err, rec.Body.String())
	}
	return len(*resp.Posts)
}

func TestBulkCreateAssignsPlanTimesTransactionally(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	chA, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "bulk-a", Name: "Bulk A", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	chB, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "bulk-b", Name: "Bulk B", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})

	start := time.Now().UTC().AddDate(0, 0, 2).Format("2006-01-02")
	explicit := time.Now().UTC().AddDate(0, 0, 10).Truncate(time.Minute)
	body := fmt.Sprintf(`{"name":"Tuần ra mắt","channel_ids":["%s","%s"],
	  "items":[{"content":"Bài 1","media_type":"NONE"},{"content":"Bài 2","media_type":"IMAGE","media_urls":["https://cdn.example.com/a.png"]},{"content":"Bài 3","scheduled_at":"%s"}],
	  "schedule_plan":{"start_date":"%s","timezone":"Asia/Ho_Chi_Minh","time_slots":["09:00","19:30"],"avoid_conflicts":true,"min_gap_minutes":60}}`,
		chA.ID, chB.ID, explicit.Format(time.RFC3339), start)

	// Preview must not write.
	preview := doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts/bulk/preview", body, "")
	if preview.Code != http.StatusOK {
		t.Fatalf("preview expected 200, got %d: %s", preview.Code, preview.Body.String())
	}
	var previewResp struct {
		Schedule  []domain.ScheduleSlot `json:"schedule"`
		PostCount int                   `json:"post_count"`
		Channels  []map[string]any      `json:"channels"`
	}
	_ = json.Unmarshal(preview.Body.Bytes(), &previewResp)
	if previewResp.PostCount != 6 || len(previewResp.Schedule) != 3 || len(previewResp.Channels) != 2 {
		t.Fatalf("preview payload wrong: %s", preview.Body.String())
	}
	if !previewResp.Schedule[2].Explicit || !previewResp.Schedule[2].ScheduledAt.Equal(explicit) {
		t.Fatalf("explicit time must be kept: %+v", previewResp.Schedule[2])
	}
	if previewResp.Schedule[0].Explicit || !previewResp.Schedule[0].ScheduledAt.Before(previewResp.Schedule[1].ScheduledAt) {
		t.Fatalf("plan times must be assigned in order: %+v", previewResp.Schedule[:2])
	}
	if n := countPosts(t, srv, handler); n != 0 {
		t.Fatalf("preview wrote %d posts", n)
	}

	created := doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts/bulk", body, "")
	if created.Code != http.StatusCreated {
		t.Fatalf("bulk create expected 201, got %d: %s", created.Code, created.Body.String())
	}
	var createdResp struct {
		BatchID string                `json:"batch_id"`
		Name    string                `json:"name"`
		Count   int                   `json:"count"`
		Posts   []domain.Post         `json:"posts"`
		Slots   []domain.ScheduleSlot `json:"schedule"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &createdResp)
	if createdResp.Count != 6 || createdResp.BatchID == "" || createdResp.Name != "Tuần ra mắt" {
		t.Fatalf("bulk response wrong: %s", created.Body.String())
	}
	perChannel := map[string]int{}
	for _, p := range createdResp.Posts {
		perChannel[p.ChannelID]++
		if p.Status != domain.PostStatusScheduled || p.BatchID != createdResp.BatchID || p.BatchName != "Tuần ra mắt" {
			t.Fatalf("post not scheduled/linked: %+v", p)
		}
	}
	if perChannel[chA.ID] != 3 || perChannel[chB.ID] != 3 {
		t.Fatalf("expected 3 posts per channel, got %v", perChannel)
	}

	// Batch read
	batchRec := doJSON(t, srv, handler, http.MethodGet, "/api/v1/posts/batches/"+createdResp.BatchID, "", "")
	if batchRec.Code != http.StatusOK {
		t.Fatalf("batch read expected 200, got %d: %s", batchRec.Code, batchRec.Body.String())
	}
	var batchResp struct {
		Batch domain.PostBatch `json:"batch"`
		Posts []domain.Post    `json:"posts"`
	}
	_ = json.Unmarshal(batchRec.Body.Bytes(), &batchResp)
	if batchResp.Batch.ItemCount != 6 || batchResp.Batch.Plan == nil || batchResp.Batch.Plan.Timezone != "Asia/Ho_Chi_Minh" || len(batchResp.Posts) != 6 {
		t.Fatalf("batch payload wrong: %s", batchRec.Body.String())
	}

	// Range listing: window around the first plan slot returns ascending order and only the window.
	first := createdResp.Slots[0].ScheduledAt
	from, to := first.Add(-time.Hour), first.Add(time.Hour)
	rangeRec := doJSON(t, srv, handler, http.MethodGet, fmt.Sprintf("/api/v1/posts?from=%s&to=%s&channel_id=%s", from.Format(time.RFC3339), to.Format(time.RFC3339), chA.ID), "", "")
	var rangeResp struct {
		Posts []domain.Post `json:"posts"`
	}
	_ = json.Unmarshal(rangeRec.Body.Bytes(), &rangeResp)
	if rangeRec.Code != http.StatusOK || len(rangeResp.Posts) != 1 || rangeResp.Posts[0].ChannelID != chA.ID {
		t.Fatalf("range listing wrong: %d %s", rangeRec.Code, rangeRec.Body.String())
	}
	if bad := doJSON(t, srv, handler, http.MethodGet, "/api/v1/posts?from=yesterday", "", ""); bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid from must be 400, got %d", bad.Code)
	}
	if bad := doJSON(t, srv, handler, http.MethodGet, fmt.Sprintf("/api/v1/posts?from=%s&to=%s", to.Format(time.RFC3339), from.Format(time.RFC3339)), "", ""); bad.Code != http.StatusBadRequest {
		t.Fatalf("inverted window must be 400, got %d", bad.Code)
	}

	// Second bulk run with conflict avoidance must not reuse the occupied slots.
	again := doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts/bulk/preview", body, "")
	var againResp struct {
		Schedule []domain.ScheduleSlot `json:"schedule"`
	}
	_ = json.Unmarshal(again.Body.Bytes(), &againResp)
	if againResp.Schedule[0].ScheduledAt.Equal(createdResp.Slots[0].ScheduledAt) {
		t.Fatalf("conflict avoidance should skip the occupied slot: %+v", againResp.Schedule[0])
	}
}

func TestBulkCreateValidationAndRBAC(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "bulk-rbac", Name: "RBAC", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	cg, _ := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "Needs approval", RequireApproval: true})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{ch.ID})
	plan := `"schedule_plan":{"start_date":"2099-01-05","timezone":"UTC","time_slots":["10:00"]}`

	cases := []struct {
		name string
		body string
		role string
		code int
	}{
		{"no items", fmt.Sprintf(`{"channel_id":"%s","items":[],%s}`, ch.ID, plan), "", http.StatusBadRequest},
		{"missing target", fmt.Sprintf(`{"items":[{"content":"x"}],%s}`, plan), "", http.StatusBadRequest},
		{"unknown channel", fmt.Sprintf(`{"channel_id":"nope","items":[{"content":"x"}],%s}`, plan), "", http.StatusBadRequest},
		{"second item invalid rolls back", fmt.Sprintf(`{"channel_id":"%s","items":[{"content":"ok"},{"content":"","media_type":"IMAGE"}],%s}`, ch.ID, plan), "", http.StatusBadRequest},
		{"plan required", fmt.Sprintf(`{"channel_id":"%s","items":[{"content":"ok"}]}`, ch.ID), "", http.StatusBadRequest},
		{"bad plan timezone", fmt.Sprintf(`{"channel_id":"%s","items":[{"content":"ok"}],"schedule_plan":{"time_slots":["10:00"],"timezone":"Nope/Nope"}}`, ch.ID), "", http.StatusBadRequest},
		{"bad explicit time", fmt.Sprintf(`{"channel_id":"%s","items":[{"content":"ok","scheduled_at":"tomorrow"}]}`, ch.ID), "", http.StatusBadRequest},
		{"viewer forbidden", fmt.Sprintf(`{"channel_id":"%s","items":[{"content":"ok"}],%s}`, ch.ID, plan), "VIEWER", http.StatusForbidden},
		{"not json", `{`, "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts/bulk", tc.body, tc.role)
		if rec.Code != tc.code {
			t.Fatalf("%s: expected %d, got %d: %s", tc.name, tc.code, rec.Code, rec.Body.String())
		}
	}
	if n := countPosts(t, srv, handler); n != 0 {
		t.Fatalf("rejected requests must create nothing, found %d posts", n)
	}

	// EDITOR on an approval-required channel: everything lands in PENDING_APPROVAL.
	editorBody := fmt.Sprintf(`{"channel_id":"%s","items":[{"content":"a"},{"content":"b"}],%s}`, ch.ID, plan)
	rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts/bulk", editorBody, "EDITOR")
	if rec.Code != http.StatusCreated {
		t.Fatalf("editor bulk expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Posts []domain.Post `json:"posts"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(resp.Posts))
	}
	for _, p := range resp.Posts {
		if p.Status != domain.PostStatusPendingApproval {
			t.Fatalf("editor posts must await approval, got %s", p.Status)
		}
	}

	// Another organization cannot see the batch.
	batchID := resp.Posts[0].BatchID
	other := httptest.NewRequest(http.MethodGet, "/api/v1/posts/batches/"+batchID, nil)
	foreignOrg, _ := store.CreateOrganization(ctx, &domain.Organization{Name: "Other", Slug: "other-" + batchID[:6]}, domain.DefaultUserID)
	other.Header.Set("X-Organization-Id", foreignOrg.ID)
	authenticateTestRole(t, srv, other, "OWNER", "")
	recOther := httptest.NewRecorder()
	handler.ServeHTTP(recOther, other)
	if recOther.Code != http.StatusNotFound {
		t.Fatalf("foreign organization must get 404, got %d: %s", recOther.Code, recOther.Body.String())
	}
}

func TestUpdatePostRescheduleRules(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "patch", Name: "Patch", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	when := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	retry := when.Add(time.Hour)
	failed, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Content: "f", Status: domain.PostStatusFailed, ScheduledAt: when, RetryCount: 3, NextRetryAt: &retry, MediaType: domain.MediaTypeNone})
	published, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Content: "p", Status: domain.PostStatusPublished, ScheduledAt: when, MediaType: domain.MediaTypeNone})

	later := when.Add(48 * time.Hour)
	rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+failed.ID, fmt.Sprintf(`{"scheduled_at":"%s","content":"đã sửa"}`, later.Format(time.RFC3339)), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("patch expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var updated domain.Post
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Status != domain.PostStatusScheduled || updated.RetryCount != 0 || !updated.ScheduledAt.Equal(later) || updated.Content != "đã sửa" {
		t.Fatalf("reschedule result wrong: %+v", updated)
	}

	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+published.ID, `{"content":"x"}`, ""); rec.Code != http.StatusConflict {
		t.Fatalf("published post must be 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+failed.ID, `{}`, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty patch must be 400, got %d", rec.Code)
	}
	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+failed.ID, `{"media_type":"IMAGE","media_urls":[]}`, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("image without urls must be 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+failed.ID, `{"scheduled_at":"soon"}`, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad time must be 400, got %d", rec.Code)
	}
	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/missing", `{"content":"x"}`, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown post must be 404, got %d", rec.Code)
	}
	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+failed.ID, `{"content":"viewer"}`, "VIEWER"); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer must be 403, got %d", rec.Code)
	}
	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+failed.ID, `{"content":"editor"}`, "EDITOR"); rec.Code != http.StatusOK {
		t.Fatalf("editor may edit, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMemberListingPagesOverVisibleChannelsOnly(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	visible, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "vis", Name: "Visible", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	hidden, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "hid", Name: "Hidden", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	base := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	for i := 0; i < 3; i++ {
		_, _ = store.CreatePost(ctx, &domain.Post{ChannelID: hidden.ID, Content: "hidden", Status: domain.PostStatusScheduled, ScheduledAt: base.Add(time.Duration(i) * time.Minute), MediaType: domain.MediaTypeNone})
		_, _ = store.CreatePost(ctx, &domain.Post{ChannelID: visible.ID, Content: "visible", Status: domain.PostStatusScheduled, ScheduledAt: base.Add(time.Duration(10+i) * time.Minute), MediaType: domain.MediaTypeNone})
	}
	// An EDITOR granted only on the visible channel.
	user, _ := store.CreateUser(ctx, &domain.User{Email: "partial@postpilot.test", IsActive: true})
	_, _ = store.AddOrganizationMember(ctx, &domain.OrganizationMember{OrganizationID: domain.DefaultOrganizationID, UserID: user.ID, OrgRole: domain.OrgRoleMember})
	group, _ := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "partial"})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, group.ID, []string{visible.ID})
	_, _ = store.SetChannelGroupPermission(ctx, &domain.ChannelGroupPermission{ChannelGroupID: group.ID, UserID: user.ID, GroupRole: domain.GroupRoleEditor})

	page := func(query string) ([]domain.Post, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?"+query, nil)
		authenticateTestRole(t, srv, req, "MEMBER", user.ID)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("listing expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Posts   []domain.Post `json:"posts"`
			Limit   int           `json:"limit"`
			Offset  int           `json:"offset"`
			HasMore bool          `json:"has_more"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		return resp.Posts, map[string]any{"limit": resp.Limit, "offset": resp.Offset, "has_more": resp.HasMore}
	}
	first, meta := page("limit=2")
	if len(first) != 2 || meta["has_more"] != true || meta["limit"] != 2 {
		t.Fatalf("first page must be full with visible posts only: %d %v", len(first), meta)
	}
	second, meta := page("limit=2&offset=2")
	if len(second) != 1 || meta["has_more"] != false {
		t.Fatalf("second page must hold the last visible post: %d %v", len(second), meta)
	}
	for _, p := range append(first, second...) {
		if p.ChannelID != visible.ID {
			t.Fatalf("hidden channel leaked: %+v", p)
		}
	}
	if rows, _ := page("channel_id=" + hidden.ID); len(rows) != 0 {
		t.Fatal("requesting a hidden channel must yield nothing")
	}
	if _, meta := page("limit=9999&offset=-3"); meta["limit"] != 500 || meta["offset"] != 0 {
		t.Fatalf("limit/offset must be echoed normalised: %v", meta)
	}
	// Exact has_more: a page that ends precisely on the last visible row must not promise more.
	if rows, meta := page("limit=3"); len(rows) != 3 || meta["has_more"] != false {
		t.Fatalf("visible total == limit must report has_more=false: %d %v", len(rows), meta)
	}
	// status and batch_id query parameters are wired through the handler.
	if rows, _ := page("status=scheduled"); len(rows) != 3 {
		t.Fatalf("status filter (lower-case) must apply: %d", len(rows))
	}
	if rows, _ := page("status=ALL"); len(rows) != 3 {
		t.Fatalf("status=ALL means no filter: %d", len(rows))
	}
	if rows, _ := page("status=PUBLISHED"); len(rows) != 0 {
		t.Fatalf("status filter must exclude other statuses: %d", len(rows))
	}
	bulk := doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts/bulk", fmt.Sprintf(`{"channel_id":"%s","items":[{"content":"b1"},{"content":"b2"}],"schedule_plan":{"start_date":"2099-01-05","timezone":"UTC","time_slots":["10:00"]}}`, visible.ID), "")
	var bulkResp struct {
		BatchID string `json:"batch_id"`
	}
	_ = json.Unmarshal(bulk.Body.Bytes(), &bulkResp)
	if rows, _ := page("batch_id=" + bulkResp.BatchID); len(rows) != 2 {
		t.Fatalf("batch_id filter must apply: %d", len(rows))
	}
	if rows, _ := page("batch_id=not-a-uuid"); len(rows) != 0 {
		t.Fatalf("malformed batch_id must match nothing: %d", len(rows))
	}
}

func TestBulkApprovalAndGroupPathsAndPublishNowRejected(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "grp", Name: "Grp", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	cg, _ := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "open group"})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{ch.ID})
	plan := `"schedule_plan":{"start_date":"2099-01-05","timezone":"UTC","time_slots":["10:00"]}`

	rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts/bulk", fmt.Sprintf(`{"channel_group_id":"%s","items":[{"content":"a"},{"content":"b"}],"submit_for_approval":true,%s}`, cg.ID, plan), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("group bulk expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Posts []domain.Post `json:"posts"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	for _, p := range resp.Posts {
		if p.Status != domain.PostStatusPendingApproval || p.ChannelGroupID != cg.ID {
			t.Fatalf("submit_for_approval via group must yield pending posts stamped with the group: %+v", p)
		}
	}
	if rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts/bulk", fmt.Sprintf(`{"channel_id":"%s","items":[{"content":"a"}],"publish_now":true,%s}`, ch.ID, plan), ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("publish_now must be rejected for bulk, got %d: %s", rec.Code, rec.Body.String())
	}
	// apply_to_batch review uses the batch query (not the legacy 100-row listing).
	first := resp.Posts[0]
	review := doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts/"+first.ID+"/review", `{"action":"APPROVED","apply_to_batch":true}`, "")
	if review.Code != http.StatusOK {
		t.Fatalf("batch review expected 200, got %d: %s", review.Code, review.Body.String())
	}
	batch := doJSON(t, srv, handler, http.MethodGet, "/api/v1/posts/batches/"+first.BatchID, "", "")
	var batchResp struct {
		Posts []domain.Post `json:"posts"`
	}
	_ = json.Unmarshal(batch.Body.Bytes(), &batchResp)
	for _, p := range batchResp.Posts {
		if p.Status == domain.PostStatusPendingApproval {
			t.Fatalf("apply_to_batch must review every post of the batch: %+v", p)
		}
	}
}

func TestPatchStatusMatrixAndApprovalPreservation(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "matrix", Name: "Matrix", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	when := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	cases := []struct {
		status string
		code   int
		after  string
	}{
		{domain.PostStatusDraft, http.StatusOK, domain.PostStatusDraft},
		{domain.PostStatusScheduled, http.StatusOK, domain.PostStatusScheduled},
		{domain.PostStatusPendingApproval, http.StatusOK, domain.PostStatusPendingApproval},
		{domain.PostStatusFailed, http.StatusOK, domain.PostStatusFailed}, // content-only edit keeps FAILED
		{domain.PostStatusQueued, http.StatusConflict, domain.PostStatusQueued},
		{domain.PostStatusProcessing, http.StatusConflict, domain.PostStatusProcessing},
		{domain.PostStatusPublished, http.StatusConflict, domain.PostStatusPublished},
		{domain.PostStatusCancelled, http.StatusConflict, domain.PostStatusCancelled},
		{domain.PostStatusRejected, http.StatusConflict, domain.PostStatusRejected},
	}
	for _, tc := range cases {
		post, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Content: "c", Status: tc.status, ScheduledAt: when, MediaType: domain.MediaTypeNone})
		rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+post.ID, `{"content":"edited"}`, "")
		if rec.Code != tc.code {
			t.Fatalf("%s: expected %d, got %d: %s", tc.status, tc.code, rec.Code, rec.Body.String())
		}
		stored, _ := store.GetPostByID(ctx, post.ID)
		if stored.Status != tc.after {
			t.Fatalf("%s: expected status %s after patch, got %s", tc.status, tc.after, stored.Status)
		}
	}

	// EDITOR on an approval-required channel: content edit of an approved post returns to review via the API;
	// a DRAFT edited by the same editor stays a draft.
	cg, _ := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "strict-api", RequireApproval: true})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{ch.ID})
	draft, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Content: "draft", Status: domain.PostStatusDraft, ScheduledAt: when, MediaType: domain.MediaTypeNone})
	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+draft.ID, `{"content":"still a draft"}`, "EDITOR"); rec.Code != http.StatusOK {
		t.Fatalf("editor draft edit expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stored, _ := store.GetPostByID(ctx, draft.ID); stored.Status != domain.PostStatusDraft {
		t.Fatalf("draft must not be auto-submitted, got %s", stored.Status)
	}
	approved, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Content: "approved", Status: domain.PostStatusScheduled, ScheduledAt: when, ReviewedBy: domain.DefaultUserID, MediaType: domain.MediaTypeNone})
	rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+approved.ID, `{"content":"sneaky"}`, "EDITOR")
	if rec.Code != http.StatusOK {
		t.Fatalf("editor edit expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var updated domain.Post
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Status != domain.PostStatusPendingApproval || updated.ReviewedBy != "" {
		t.Fatalf("editor content edit on approval channel must return to PENDING_APPROVAL: %+v", updated)
	}
	rec = doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+approved.ID, fmt.Sprintf(`{"scheduled_at":"%s"}`, when.Add(time.Hour).Format(time.RFC3339)), "APPROVER")
	if rec.Code != http.StatusOK {
		t.Fatalf("approver reschedule expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Batch metadata is hidden from members with no visible channel.
	viewerless, _ := store.CreateUser(ctx, &domain.User{Email: "nochannel@postpilot.test", IsActive: true})
	_, _ = store.AddOrganizationMember(ctx, &domain.OrganizationMember{OrganizationID: domain.DefaultOrganizationID, UserID: viewerless.ID, OrgRole: domain.OrgRoleMember})
	bulk := doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts/bulk", fmt.Sprintf(`{"channel_id":"%s","items":[{"content":"x"}],"schedule_plan":{"start_date":"2099-01-05","timezone":"UTC","time_slots":["10:00"]}}`, ch.ID), "")
	var bulkResp struct {
		BatchID string `json:"batch_id"`
	}
	_ = json.Unmarshal(bulk.Body.Bytes(), &bulkResp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/batches/"+bulkResp.BatchID, nil)
	authenticateTestRole(t, srv, req, "MEMBER", viewerless.ID)
	recBatch := httptest.NewRecorder()
	handler.ServeHTTP(recBatch, req)
	if recBatch.Code != http.StatusNotFound {
		t.Fatalf("member without channel access must get 404 for the batch, got %d: %s", recBatch.Code, recBatch.Body.String())
	}
}
