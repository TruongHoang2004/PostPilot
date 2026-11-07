package database

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// Exercises the real SQL of QueryPosts / CreatePostsBatch / UpdatePost / GetPostBatch against a
// disposable PostgreSQL (POSTPILOT_TEST_DATABASE_URL). Skipped otherwise; never reads DATABASE_URL.
func TestPostgresPostQueriesBatchesAndEdits(t *testing.T) {
	dsn := os.Getenv("POSTPILOT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable PostgreSQL not configured; set POSTPILOT_TEST_DATABASE_URL")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := NewMigrator(db, slog.New(slog.NewTextHandler(io.Discard, nil))).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresStore(db)
	user := &domain.User{Email: newUUID() + "@posts.test", FullName: "Posts", PasswordHash: "h", IsActive: true}
	user, org, err := store.RegisterAccount(t.Context(), user, "Posts workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DELETE FROM users WHERE id=$1", user.ID)
	defer db.ExecContext(context.Background(), "DELETE FROM organizations WHERE id=$1", org.ID)
	ctx := WithTenantContext(t.Context(), &domain.TenantContext{OrganizationID: org.ID, UserID: user.ID, OrgRole: domain.OrgRoleOwner})
	chA, err := store.UpsertChannel(ctx, &domain.Channel{Name: "A", ExternalID: newUUID(), Platform: domain.PlatformFacebook, EncryptedAccessToken: "x", TokenStatus: domain.TokenStatusActive, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	chB, err := store.UpsertChannel(ctx, &domain.Channel{Name: "B", ExternalID: newUUID(), Platform: domain.PlatformFacebook, EncryptedAccessToken: "x", TokenStatus: domain.TokenStatusActive, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2030, 1, 10, 9, 0, 0, 0, time.UTC)

	// Transaction rollback: the second post references a channel of another tenant / unknown id.
	_, err = store.CreatePostsBatch(ctx, &domain.PostBatch{Name: "broken"}, []*domain.Post{
		{ChannelID: chA.ID, Content: "ok", Status: domain.PostStatusScheduled, ScheduledAt: base},
		{ChannelID: newUUID(), Content: "bad", Status: domain.PostStatusScheduled, ScheduledAt: base},
	})
	if err == nil {
		t.Fatal("batch with unknown channel must fail")
	}
	if rows, _ := store.QueryPosts(ctx, domain.PostQuery{}); len(rows) != 0 {
		t.Fatalf("failed batch must not leave rows, got %d", len(rows))
	}
	var batches int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM post_batches WHERE organization_id=$1", org.ID).Scan(&batches)
	if batches != 0 {
		t.Fatalf("failed batch must not leave a post_batches row, got %d", batches)
	}

	// In-transaction rollback: the batch row and the first post are written before the second post
	// violates posts.status VARCHAR(32); nothing may survive.
	_, err = store.CreatePostsBatch(ctx, &domain.PostBatch{Name: "half"}, []*domain.Post{
		{ChannelID: chA.ID, Content: "ok", Status: domain.PostStatusScheduled, ScheduledAt: base},
		{ChannelID: chA.ID, Content: "bad", Status: "THIS_STATUS_IS_FAR_TOO_LONG_FOR_THE_COLUMN", ScheduledAt: base},
	})
	if err == nil {
		t.Fatal("oversized status must fail inside the transaction")
	}
	if rows, _ := store.QueryPosts(ctx, domain.PostQuery{}); len(rows) != 0 {
		t.Fatalf("in-transaction failure must roll back every post, got %d", len(rows))
	}
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM post_batches WHERE organization_id=$1", org.ID).Scan(&batches)
	if batches != 0 {
		t.Fatalf("in-transaction failure must roll back the batch row, got %d", batches)
	}

	plan := &domain.SchedulePlan{StartDate: "2030-01-10", Timezone: "Asia/Ho_Chi_Minh", TimeSlots: []string{"09:00", "19:30"}, Weekdays: []int{1, 2, 3, 4, 5}, MinGapMinutes: 60, AvoidConflicts: true}
	batch := &domain.PostBatch{Name: "Tháng 1", Plan: plan}
	created, err := store.CreatePostsBatch(ctx, batch, []*domain.Post{
		{ChannelID: chA.ID, Content: "a1", Status: domain.PostStatusScheduled, ScheduledAt: base, MediaType: domain.MediaTypeNone},
		{ChannelID: chB.ID, Content: "b1", Status: domain.PostStatusScheduled, ScheduledAt: base.Add(time.Hour), MediaType: domain.MediaTypeImage, MediaURLs: []string{"https://x/a.png"}},
		{ChannelID: chA.ID, Content: "a2", Status: domain.PostStatusPendingApproval, ScheduledAt: base.Add(48 * time.Hour), MediaType: domain.MediaTypeNone},
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.ID == "" || batch.ItemCount != 3 || batch.CreatedAt.IsZero() {
		t.Fatalf("batch metadata wrong: %+v", batch)
	}
	got, err := store.GetPostBatch(ctx, batch.ID)
	if err != nil || got.Plan == nil || got.Plan.Timezone != "Asia/Ho_Chi_Minh" || len(got.Plan.Weekdays) != 5 || got.Kind != domain.PostBatchKindBulk {
		t.Fatalf("GetPostBatch round trip wrong: %+v %v", got, err)
	}
	one, err := store.GetPostByID(ctx, created[0].ID)
	if err != nil || one.BatchName != "Tháng 1" || one.BatchID != batch.ID || one.ChannelName != "A" {
		t.Fatalf("batch name must be joined on reads: %+v %v", one, err)
	}
	if created[0].ChannelName != "A" || created[1].Platform != domain.PlatformFacebook {
		t.Fatalf("created posts must carry channel display fields (store parity): %+v", created[0])
	}
	if _, err := store.GetPostByID(ctx, "not-a-uuid"); err == nil {
		t.Fatal("malformed id must be not found, not a driver error")
	}
	if _, err := store.GetPostBatch(ctx, "not-a-uuid"); err == nil {
		t.Fatal("malformed batch id must be not found")
	}
	if rows, err := store.QueryPosts(ctx, domain.PostQuery{BatchID: "nope"}); err != nil || len(rows) != 0 {
		t.Fatalf("malformed batch filter must match nothing: %v %v", rows, err)
	}
	from, to := base, base.Add(24*time.Hour)
	day, err := store.QueryPosts(ctx, domain.PostQuery{From: &from, To: &to, Ascending: true})
	if err != nil || len(day) != 2 || day[0].Content != "a1" || day[1].Content != "b1" {
		t.Fatalf("range query wrong: %+v %v", day, err)
	}
	onlyB, err := store.QueryPosts(ctx, domain.PostQuery{ChannelIDs: []string{chB.ID, "not-a-uuid"}, Statuses: domain.ActivePostStatuses()})
	if err != nil || len(onlyB) != 1 || onlyB[0].ChannelID != chB.ID {
		t.Fatalf("channel filter must accept non-uuid text safely: %+v %v", onlyB, err)
	}
	byBatch, err := store.QueryPosts(ctx, domain.PostQuery{BatchID: batch.ID, Limit: 2, Offset: 1, Ascending: true})
	if err != nil || len(byBatch) != 2 || byBatch[0].Content != "b1" {
		t.Fatalf("batch paging wrong: %+v %v", byBatch, err)
	}
	legacy, err := store.ListPosts(ctx, domain.PostStatusScheduled, 50)
	if err != nil || len(legacy) != 2 || legacy[0].Content != "b1" {
		t.Fatalf("ListPosts wrapper newest-first wrong: %+v %v", legacy, err)
	}
	// Stable paging over fully tied rows (same scheduled_at and created_at within one transaction).
	ties := make([]*domain.Post, 0, 4)
	for i := 0; i < 4; i++ {
		ties = append(ties, &domain.Post{ChannelID: chB.ID, Content: "tie", Status: domain.PostStatusDraft, ScheduledAt: base.Add(240 * time.Hour), MediaType: domain.MediaTypeNone})
	}
	if _, err := store.CreatePostsBatch(ctx, nil, ties); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for offset := 0; offset < 4; offset++ {
		rows, err := store.QueryPosts(ctx, domain.PostQuery{Statuses: []string{domain.PostStatusDraft}, Limit: 1, Offset: offset, Ascending: true})
		if err != nil || len(rows) != 1 {
			t.Fatalf("tie page %d: %v %v", offset, rows, err)
		}
		seen[rows[0].ID]++
	}
	if len(seen) != 4 {
		t.Fatalf("tied rows must page without repeats, saw %d distinct", len(seen))
	}

	// UpdatePost: content + reschedule on SCHEDULED; CAS blocks once the worker claimed it.
	later := base.Add(72 * time.Hour)
	text := "a1 edited"
	updated, err := store.UpdatePost(ctx, created[0].ID, domain.PostPatch{Content: &text, ScheduledAt: &later})
	if err != nil || updated.Content != "a1 edited" || !updated.ScheduledAt.Equal(later) || updated.Status != domain.PostStatusScheduled {
		t.Fatalf("UpdatePost wrong: %+v %v", updated, err)
	}
	if err := store.MarkPostProcessing(ctx, created[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePost(ctx, created[0].ID, domain.PostPatch{Content: &text}); !errors.Is(err, domain.ErrPostNotEditable) {
		t.Fatalf("processing post must not be editable, got %v", err)
	}
	// FAILED → SCHEDULED with retry reset.
	retryAt := base.Add(time.Hour)
	if err := store.MarkPostFailedOrRetry(ctx, created[1].ID, 2, &retryAt, domain.PostStatusFailed, 10, "boom"); err != nil {
		t.Fatal(err)
	}
	re, err := store.UpdatePost(ctx, created[1].ID, domain.PostPatch{ScheduledAt: &later})
	if err != nil || re.Status != domain.PostStatusScheduled || re.RetryCount != 0 || re.NextRetryAt != nil || re.LastErrorMessage != "" {
		t.Fatalf("failed reschedule wrong: %+v %v", re, err)
	}

	// RequireReview clears the previous decision in SQL.
	reviewedAt := base
	if _, err := db.ExecContext(ctx, "UPDATE posts SET reviewed_by=$1, reviewed_at=$2, rejection_note='old' WHERE id=$3", user.ID, reviewedAt, created[2].ID); err != nil {
		t.Fatal(err)
	}
	back, err := store.UpdatePost(ctx, created[2].ID, domain.PostPatch{Content: &text, RequireReview: true, SubmittedBy: user.ID})
	if err != nil || back.Status != domain.PostStatusPendingApproval || back.ReviewedBy != "" || back.ReviewedAt != nil || back.RejectionNote != "" || back.SubmittedBy != user.ID {
		t.Fatalf("RequireReview must reset the decision columns and re-attribute the submitter: %+v %v", back, err)
	}

	// Tenant isolation on every new method.
	other := WithTenantContext(t.Context(), &domain.TenantContext{OrganizationID: domain.DefaultOrganizationID, UserID: domain.DefaultUserID, OrgRole: domain.OrgRoleOwner})
	if _, err := store.GetPostBatch(other, batch.ID); err == nil {
		t.Fatal("batch visible across tenants")
	}
	if rows, _ := store.QueryPosts(other, domain.PostQuery{BatchID: batch.ID}); len(rows) != 0 {
		t.Fatal("batch posts visible across tenants")
	}
	if _, err := store.UpdatePost(other, created[2].ID, domain.PostPatch{Content: &text}); err == nil {
		t.Fatal("post editable across tenants")
	}
	if _, err := store.CreatePostsBatch(other, nil, []*domain.Post{{ChannelID: chA.ID, Content: "x", Status: domain.PostStatusScheduled, ScheduledAt: base}}); err == nil {
		t.Fatal("foreign tenant wrote to another tenant's channel")
	}
}
