package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

func seedChannel(t *testing.T, store *InMemoryStore, ctx context.Context, externalID string) *domain.Channel {
	t.Helper()
	ch, err := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: externalID, Name: "Page " + externalID, EncryptedAccessToken: "enc", TokenStatus: domain.TokenStatusActive, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func TestInMemoryStore_QueryPostsFiltersAndOrder(t *testing.T) {
	store := NewInMemoryStore()
	ctx := WithTenantContext(context.Background(), domain.DefaultTenantContext())
	a := seedChannel(t, store, ctx, "qa")
	b := seedChannel(t, store, ctx, "qb")
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	for i, spec := range []struct {
		ch     string
		status string
		at     time.Time
	}{
		{a.ID, domain.PostStatusScheduled, base},
		{a.ID, domain.PostStatusPublished, base.Add(2 * time.Hour)},
		{b.ID, domain.PostStatusScheduled, base.Add(26 * time.Hour)},
		{b.ID, domain.PostStatusFailed, base.Add(-time.Hour)},
	} {
		if _, err := store.CreatePost(ctx, &domain.Post{ChannelID: spec.ch, Content: "p", Status: spec.status, ScheduledAt: spec.at, MediaType: domain.MediaTypeNone}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	from, to := base, base.Add(24*time.Hour)
	day, err := store.QueryPosts(ctx, domain.PostQuery{From: &from, To: &to, Ascending: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(day) != 2 || !day[0].ScheduledAt.Equal(base) || !day[1].ScheduledAt.Equal(base.Add(2*time.Hour)) {
		t.Fatalf("range query wrong: %+v", day)
	}
	active, err := store.QueryPosts(ctx, domain.PostQuery{ChannelIDs: []string{b.ID}, Statuses: domain.ActivePostStatuses(), Ascending: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 || active[0].Status != domain.PostStatusFailed || active[1].ChannelName != "Page qb" {
		t.Fatalf("channel/status query wrong: %+v", active)
	}
	page, err := store.QueryPosts(ctx, domain.PostQuery{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || !page[0].ScheduledAt.Equal(base.Add(2*time.Hour)) {
		t.Fatalf("offset paging wrong (expected newest-first skipping one): %+v", page)
	}
	if beyond, _ := store.QueryPosts(ctx, domain.PostQuery{Offset: 99}); len(beyond) != 0 {
		t.Fatal("offset beyond range must be empty")
	}
	legacy, err := store.ListPosts(ctx, domain.PostStatusScheduled, 0)
	if err != nil || len(legacy) != 2 || legacy[0].ScheduledAt.Before(legacy[1].ScheduledAt) {
		t.Fatalf("ListPosts wrapper must keep newest-first status filtering: %v %+v", err, legacy)
	}
}

func TestInMemoryStore_CreatePostsBatchIsAtomicAndScoped(t *testing.T) {
	store := NewInMemoryStore()
	ctx := WithTenantContext(context.Background(), domain.DefaultTenantContext())
	ch := seedChannel(t, store, ctx, "batch")
	when := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

	posts := []*domain.Post{
		{ChannelID: ch.ID, Content: "one", Status: domain.PostStatusScheduled, ScheduledAt: when},
		{ChannelID: "missing-channel", Content: "two", Status: domain.PostStatusScheduled, ScheduledAt: when},
	}
	if _, err := store.CreatePostsBatch(ctx, &domain.PostBatch{Name: "x"}, posts); err == nil {
		t.Fatal("expected failure for unknown channel")
	}
	if all, _ := store.QueryPosts(ctx, domain.PostQuery{}); len(all) != 0 {
		t.Fatalf("all-or-nothing violated: %d posts stored", len(all))
	}

	plan := &domain.SchedulePlan{StartDate: "2026-11-12", TimeSlots: []string{"10:00"}}
	batch := &domain.PostBatch{Name: "Tuần lễ ra mắt", Plan: plan}
	good := []*domain.Post{
		{ChannelID: ch.ID, Content: "one", Status: domain.PostStatusScheduled, ScheduledAt: when},
		{ChannelID: ch.ID, Content: "two", Status: domain.PostStatusScheduled, ScheduledAt: when.Add(time.Hour)},
	}
	created, err := store.CreatePostsBatch(ctx, batch, good)
	if err != nil {
		t.Fatal(err)
	}
	if batch.ID == "" || batch.ItemCount != 2 || batch.Kind != domain.PostBatchKindBulk || batch.OrganizationID != domain.DefaultOrganizationID {
		t.Fatalf("batch metadata wrong: %+v", batch)
	}
	for _, p := range created {
		if p.BatchID != batch.ID || p.ID == "" || p.ChannelName != "Page batch" {
			t.Fatalf("post not linked/enriched: %+v", p)
		}
	}
	fetched, err := store.GetPostByID(ctx, created[0].ID)
	if err != nil || fetched.BatchName != "Tuần lễ ra mắt" {
		t.Fatalf("batch name must be surfaced on reads: %v %+v", err, fetched)
	}
	got, err := store.GetPostBatch(ctx, batch.ID)
	if err != nil || got.Plan == nil || got.Plan.StartDate != "2026-11-12" {
		t.Fatalf("GetPostBatch wrong: %v %+v", err, got)
	}
	got.Plan.StartDate = "mutated"
	if again, _ := store.GetPostBatch(ctx, batch.ID); again.Plan.StartDate != "2026-11-12" {
		t.Fatal("GetPostBatch must return a copy")
	}

	other := WithTenantContext(context.Background(), &domain.TenantContext{UserID: "u2", OrganizationID: "org-other", OrgRole: domain.OrgRoleOwner})
	if _, err := store.GetPostBatch(other, batch.ID); err == nil {
		t.Fatal("batch must be invisible to another organization")
	}
	if rows, _ := store.QueryPosts(other, domain.PostQuery{BatchID: batch.ID}); len(rows) != 0 {
		t.Fatal("batch posts must be invisible to another organization")
	}
	if _, err := store.CreatePostsBatch(other, nil, []*domain.Post{{ChannelID: ch.ID, Content: "x", Status: domain.PostStatusScheduled}}); err == nil {
		t.Fatal("foreign organization cannot write to another tenant's channel")
	}
	if _, err := store.CreatePostsBatch(ctx, nil, nil); err == nil {
		t.Fatal("empty batch rejected")
	}
}

func TestInMemoryStore_UpdatePostConditional(t *testing.T) {
	store := NewInMemoryStore()
	ctx := WithTenantContext(context.Background(), domain.DefaultTenantContext())
	ch := seedChannel(t, store, ctx, "upd")
	when := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	retry := when.Add(time.Hour)
	failed, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Content: "f", Status: domain.PostStatusFailed, ScheduledAt: when, RetryCount: 2, NextRetryAt: &retry, MediaType: domain.MediaTypeNone})
	published, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Content: "p", Status: domain.PostStatusPublished, ScheduledAt: when, MediaType: domain.MediaTypeNone})

	later := when.Add(48 * time.Hour)
	content := "fixed"
	updated, err := store.UpdatePost(ctx, failed.ID, domain.PostPatch{ScheduledAt: &later, Content: &content})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.PostStatusScheduled || updated.RetryCount != 0 || updated.NextRetryAt != nil || !updated.ScheduledAt.Equal(later) || updated.Content != "fixed" {
		t.Fatalf("failed post reschedule wrong: %+v", updated)
	}
	if _, err := store.UpdatePost(ctx, published.ID, domain.PostPatch{Content: &content}); !errors.Is(err, domain.ErrPostNotEditable) {
		t.Fatalf("published post must not be editable, got %v", err)
	}
	if _, err := store.UpdatePost(ctx, "nope", domain.PostPatch{Content: &content}); err == nil {
		t.Fatal("unknown post must error")
	}
	other := WithTenantContext(context.Background(), &domain.TenantContext{UserID: "u2", OrganizationID: "org-other", OrgRole: domain.OrgRoleOwner})
	if _, err := store.UpdatePost(other, failed.ID, domain.PostPatch{Content: &content}); err == nil {
		t.Fatal("another organization cannot edit the post")
	}
	stored, _ := store.GetPostByID(ctx, failed.ID)
	if stored.Content != "fixed" {
		t.Fatal("update must persist")
	}
}

func TestInMemoryStore_QueryPostsStablePagingOnTies(t *testing.T) {
	store := NewInMemoryStore()
	ctx := WithTenantContext(context.Background(), domain.DefaultTenantContext())
	ch := seedChannel(t, store, ctx, "ties")
	when := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	posts := make([]*domain.Post, 0, 5)
	for i := 0; i < 5; i++ {
		posts = append(posts, &domain.Post{ChannelID: ch.ID, Content: "tie", Status: domain.PostStatusScheduled, ScheduledAt: when, MediaType: domain.MediaTypeNone})
	}
	if _, err := store.CreatePostsBatch(ctx, &domain.PostBatch{Name: "ties"}, posts); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for offset := 0; offset < 5; offset += 2 {
		rows, err := store.QueryPosts(ctx, domain.PostQuery{Limit: 2, Offset: offset, Ascending: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			seen[r.ID]++
		}
	}
	if len(seen) != 5 {
		t.Fatalf("paging over tied rows must visit every row exactly once, saw %d distinct", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("row %s returned %d times", id, n)
		}
	}
	dup := []*domain.Post{{ID: "same", ChannelID: ch.ID, Content: "a", Status: domain.PostStatusScheduled, ScheduledAt: when}, {ID: "same", ChannelID: ch.ID, Content: "b", Status: domain.PostStatusScheduled, ScheduledAt: when}}
	before, _ := store.QueryPosts(ctx, domain.PostQuery{})
	if _, err := store.CreatePostsBatch(ctx, nil, dup); err == nil {
		t.Fatal("duplicate ids inside one batch must be rejected")
	}
	after, _ := store.QueryPosts(ctx, domain.PostQuery{})
	if len(after) != len(before) {
		t.Fatal("rejected batch must leave the store untouched")
	}
}
