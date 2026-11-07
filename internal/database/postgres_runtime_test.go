package database

import (
	"context"
	"database/sql"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Run only against an explicitly designated disposable database. Never reads DATABASE_URL.
func TestPostgresAtomicSignupAndConcurrentApproval(t *testing.T) {
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
	email := newUUID() + "@real.test"
	user := &domain.User{Email: email, FullName: "Atomic", PasswordHash: "test-hash", IsActive: true}
	if _, _, err = store.RegisterAccount(t.Context(), user, strings.Repeat("x", 300)); err == nil {
		t.Fatal("overlong workspace name should fail")
	}
	if found, _ := store.GetUserByEmail(t.Context(), email); found != nil {
		t.Fatal("failed workspace creation left user behind")
	}
	user, org, err := store.RegisterAccount(t.Context(), user, "Real test workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DELETE FROM users WHERE id=$1", user.ID)
	defer db.ExecContext(context.Background(), "DELETE FROM organizations WHERE id=$1", org.ID)
	ctx := WithTenantContext(t.Context(), &domain.TenantContext{OrganizationID: org.ID, UserID: user.ID, OrgRole: domain.OrgRoleOwner})
	ch, err := store.UpsertChannel(ctx, &domain.Channel{Name: "Test", ExternalID: newUUID(), Platform: domain.PlatformFacebook})
	if err != nil {
		t.Fatal(err)
	}
	post, err := store.CreatePost(ctx, &domain.Post{OrganizationID: org.ID, ChannelID: ch.ID, Status: domain.PostStatusPendingApproval, ScheduledAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, action := range []domain.ApprovalAction{domain.ApprovalActionApproved, domain.ApprovalActionRejected} {
		wg.Add(1)
		go func(a domain.ApprovalAction) {
			defer wg.Done()
			<-start
			_, err := store.ReviewPost(ctx, post.ID, user.ID, a, "")
			results <- err
		}(action)
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected one review winner, got %d", successes)
	}
	approvals, err := store.ListPostApprovals(ctx, post.ID)
	if err != nil || len(approvals) != 1 {
		t.Fatalf("expected one decision, got %d: %v", len(approvals), err)
	}
}
