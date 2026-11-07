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

// runLibraryStoreContract is shared by the in-memory and PostgreSQL stores so both keep identical
// library semantics: tenant isolation, soft delete via the publish hook, idempotent counters.
func runLibraryStoreContract(t *testing.T, store Store, ctxA, ctxB context.Context, chA, chB *domain.Channel) {
	t.Helper()
	bound, err := store.CreateLibraryItem(ctxA, &domain.LibraryItem{Title: "Khuyến mãi", ChannelID: chA.ID, Content: "unique-sale content", MediaType: domain.MediaTypeNone})
	if err != nil {
		t.Fatal(err)
	}
	if bound.ID == "" || bound.ChannelName != chA.Name || bound.PublishCount != 0 || bound.Archived() || bound.MediaURLs == nil {
		t.Fatalf("created item wrong: %+v", bound)
	}
	unbound, err := store.CreateLibraryItem(ctxA, &domain.LibraryItem{Title: "Chung", Content: "evergreen", LinkURL: "https://example.com", MediaType: domain.MediaTypeLink})
	if err != nil {
		t.Fatal(err)
	}

	// Tenant isolation: organization B can neither see nor touch A's items, nor bind A's channel.
	if _, err := store.GetLibraryItem(ctxB, bound.ID); !errors.Is(err, domain.ErrLibraryItemNotFound) {
		t.Fatalf("cross-tenant get must be not found, got %v", err)
	}
	title := "hijack"
	if _, err := store.UpdateLibraryItem(ctxB, bound.ID, domain.LibraryPatch{Title: &title}); !errors.Is(err, domain.ErrLibraryItemNotFound) {
		t.Fatalf("cross-tenant update must be not found, got %v", err)
	}
	if err := store.DeleteLibraryItem(ctxB, bound.ID); !errors.Is(err, domain.ErrLibraryItemNotFound) {
		t.Fatalf("cross-tenant delete must be not found, got %v", err)
	}
	if _, err := store.RestoreLibraryItem(ctxB, bound.ID); !errors.Is(err, domain.ErrLibraryItemNotFound) {
		t.Fatalf("cross-tenant restore must be not found, got %v", err)
	}
	if _, err := store.CreateLibraryItem(ctxB, &domain.LibraryItem{Title: "x", ChannelID: chA.ID, Content: "x", MediaType: domain.MediaTypeNone}); err == nil {
		t.Fatal("binding another tenant's channel must fail")
	}
	if items, err := store.QueryLibraryItems(ctxB, domain.LibraryQuery{}); err != nil || len(items) != 0 {
		t.Fatalf("tenant B must see nothing: %v %+v", err, items)
	}

	// Listing filters.
	all, err := store.QueryLibraryItems(ctxA, domain.LibraryQuery{})
	if err != nil || len(all) != 2 {
		t.Fatalf("active list wrong: %v %+v", err, all)
	}
	if hit, _ := store.QueryLibraryItems(ctxA, domain.LibraryQuery{Search: "UNIQUE-sale"}); len(hit) != 1 || hit[0].ID != bound.ID {
		t.Fatalf("search must be case-insensitive on content: %+v", hit)
	}
	if hit, _ := store.QueryLibraryItems(ctxA, domain.LibraryQuery{Search: "100%_"}); len(hit) != 0 {
		t.Fatalf("LIKE wildcards must be literal: %+v", hit)
	}
	if hit, _ := store.QueryLibraryItems(ctxA, domain.LibraryQuery{ChannelIDs: []string{chA.ID}}); len(hit) != 1 || hit[0].ID != bound.ID {
		t.Fatalf("channel filter wrong: %+v", hit)
	}
	if hit, _ := store.QueryLibraryItems(ctxA, domain.LibraryQuery{ChannelIDs: []string{chA.ID}, IncludeUnbound: true}); len(hit) != 2 {
		t.Fatalf("channel + unbound filter wrong: %+v", hit)
	}
	if hit, _ := store.QueryLibraryItems(ctxA, domain.LibraryQuery{IncludeUnbound: true}); len(hit) != 1 || hit[0].ID != unbound.ID {
		t.Fatalf("unbound-only filter wrong: %+v", hit)
	}
	if page, _ := store.QueryLibraryItems(ctxA, domain.LibraryQuery{Limit: 1, Offset: 1}); len(page) != 1 {
		t.Fatalf("paging wrong: %+v", page)
	}

	// Edit clears the preferred Fanpage and changes content.
	content, empty := "edited", ""
	edited, err := store.UpdateLibraryItem(ctxA, bound.ID, domain.LibraryPatch{Content: &content, ChannelID: &empty})
	if err != nil || edited.Content != "edited" || edited.ChannelID != "" || edited.ChannelName != "" {
		t.Fatalf("update wrong: %v %+v", err, edited)
	}
	if _, err := store.UpdateLibraryItem(ctxA, bound.ID, domain.LibraryPatch{ChannelID: &chA.ID}); err != nil {
		t.Fatal(err)
	}

	// Publish hook: counter always moves, the flag soft-deletes, a repeat never double counts.
	at := time.Now().UTC().Add(time.Hour)
	mk := func(flag bool) *domain.Post {
		p, err := store.CreatePost(ctxA, &domain.Post{ChannelID: chA.ID, Content: "from library", Status: domain.PostStatusScheduled, ScheduledAt: at, MediaType: domain.MediaTypeNone, LibraryItemID: bound.ID, DeleteAfterPublish: flag})
		if err != nil {
			t.Fatal(err)
		}
		got, err := store.GetPostByID(ctxA, p.ID)
		if err != nil || got.LibraryItemID != bound.ID || got.DeleteAfterPublish != flag {
			t.Fatalf("post must round-trip library link and flag: %v %+v", err, got)
		}
		return p
	}
	keep, drop := mk(false), mk(true)
	published := time.Now().UTC()
	if err := store.MarkPostPublished(ctxA, keep.ID, "fb_1", published); err != nil {
		t.Fatal(err)
	}
	item, _ := store.GetLibraryItem(ctxA, bound.ID)
	if item.PublishCount != 1 || item.Archived() || item.LastPublishedAt == nil {
		t.Fatalf("publish without the flag must only count: %+v", item)
	}
	if err := store.MarkPostPublished(ctxA, keep.ID, "fb_1", published); err != nil {
		t.Fatal(err)
	}
	if item, _ = store.GetLibraryItem(ctxA, bound.ID); item.PublishCount != 1 {
		t.Fatalf("repeated publish must not double count: %+v", item)
	}
	if err := store.MarkPostPublished(ctxA, drop.ID, "fb_2", published); err != nil {
		t.Fatal(err)
	}
	if item, _ = store.GetLibraryItem(ctxA, bound.ID); item.PublishCount != 2 || !item.Archived() {
		t.Fatalf("publish with the flag must soft-delete: %+v", item)
	}
	if hit, _ := store.QueryLibraryItems(ctxA, domain.LibraryQuery{}); len(hit) != 1 || hit[0].ID != unbound.ID {
		t.Fatalf("archived item must leave the active list: %+v", hit)
	}
	if hit, _ := store.QueryLibraryItems(ctxA, domain.LibraryQuery{Archived: true}); len(hit) != 1 || hit[0].ID != bound.ID {
		t.Fatalf("archived item must be listed in the archive: %+v", hit)
	}
	if _, err := store.UpdateLibraryItem(ctxA, bound.ID, domain.LibraryPatch{Content: &content}); !errors.Is(err, domain.ErrLibraryItemArchived) {
		t.Fatalf("archived item must not be editable, got %v", err)
	}

	// Restore keeps the content ("Giữ lại"); the restore-then-sibling-publish rule has its own contract below.
	restored, err := store.RestoreLibraryItem(ctxA, bound.ID)
	if err != nil || restored.Archived() || restored.PublishCount != 2 {
		t.Fatalf("restore wrong: %v %+v", err, restored)
	}
	// Foreign tenant publish must never touch the item.
	foreign, err := store.CreatePost(ctxB, &domain.Post{ChannelID: chB.ID, Content: "b", Status: domain.PostStatusScheduled, ScheduledAt: at, MediaType: domain.MediaTypeNone})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPostPublished(ctxA, foreign.ID, "x", published); err == nil {
		t.Fatal("publishing another tenant's post must fail")
	}

	// Permanent delete detaches posts instead of deleting them.
	if err := store.DeleteLibraryItem(ctxA, bound.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetLibraryItem(ctxA, bound.ID); !errors.Is(err, domain.ErrLibraryItemNotFound) {
		t.Fatalf("deleted item must be gone, got %v", err)
	}
	if got, err := store.GetPostByID(ctxA, keep.ID); err != nil || got.LibraryItemID != "" || got.Status != domain.PostStatusPublished {
		t.Fatalf("deleting the item must keep its posts: %v %+v", err, got)
	}
}

func TestInMemoryStoreLibraryContract(t *testing.T) {
	store := NewInMemoryStore()
	ctxA := WithTenantContext(context.Background(), domain.DefaultTenantContext())
	ctxB := WithTenantContext(context.Background(), &domain.TenantContext{OrganizationID: newUUID(), UserID: "other", OrgRole: domain.OrgRoleOwner})
	chA := seedChannel(t, store, ctxA, "lib-a")
	chB, err := store.UpsertChannel(ctxB, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "lib-b", Name: "Page B", EncryptedAccessToken: "enc", TokenStatus: domain.TokenStatusActive, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	runLibraryStoreContract(t, store, ctxA, ctxB, chA, chB)
	runLibraryRestoreAndOwnershipContract(t, store, ctxA, ctxB, chA, chB)
	runLibrarySearchContract(t, store, ctxA)
}

// Exercises the real SQL (migration 000007, transactional publish hook) against a disposable PostgreSQL.
func TestPostgresStoreLibraryContract(t *testing.T) {
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
	newTenant := func(name string) (context.Context, *domain.Channel) {
		user := &domain.User{Email: newUUID() + "@library.test", FullName: name, PasswordHash: "h", IsActive: true}
		user, org, err := store.RegisterAccount(t.Context(), user, name+" workspace")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			db.ExecContext(context.Background(), "DELETE FROM organizations WHERE id=$1", org.ID)
			db.ExecContext(context.Background(), "DELETE FROM users WHERE id=$1", user.ID)
		})
		ctx := WithTenantContext(t.Context(), &domain.TenantContext{OrganizationID: org.ID, UserID: user.ID, OrgRole: domain.OrgRoleOwner})
		ch, err := store.UpsertChannel(ctx, &domain.Channel{Name: "Page " + name, ExternalID: newUUID(), Platform: domain.PlatformFacebook, EncryptedAccessToken: "x", TokenStatus: domain.TokenStatusActive, IsActive: true})
		if err != nil {
			t.Fatal(err)
		}
		return ctx, ch
	}
	ctxA, chA := newTenant("A")
	ctxB, chB := newTenant("B")
	runLibraryStoreContract(t, store, ctxA, ctxB, chA, chB)
}

// runLibraryRestoreAndOwnershipContract pins down the rules reviewers asked to make explicit:
// "Giữ lại" is durable, foreign library ids are never linked, deleting a Fanpage unbinds its items.
func runLibraryRestoreAndOwnershipContract(t *testing.T, store Store, ctxA, ctxB context.Context, chA, chB *domain.Channel) {
	t.Helper()
	item, err := store.CreateLibraryItem(ctxA, &domain.LibraryItem{Title: "durable", Content: "durable content", MediaType: domain.MediaTypeNone})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(2 * time.Hour)
	mk := func(flag bool) *domain.Post {
		p, err := store.CreatePost(ctxA, &domain.Post{ChannelID: chA.ID, Content: "x", Status: domain.PostStatusScheduled, ScheduledAt: at, MediaType: domain.MediaTypeNone, LibraryItemID: item.ID, DeleteAfterPublish: flag})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	first, second, plain := mk(true), mk(true), mk(false)
	now := time.Now().UTC()
	if err := store.MarkPostPublished(ctxA, first.ID, "fb_a", now); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetLibraryItem(ctxA, item.ID); !got.Archived() {
		t.Fatalf("first flagged publish must archive: %+v", got)
	}
	if _, err := store.RestoreLibraryItem(ctxA, item.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetPostByID(ctxA, second.ID); got.DeleteAfterPublish {
		t.Fatal("restoring ('Giữ lại') must drop the delete-after-publish flag of pending sibling posts")
	}
	if got, _ := store.GetPostByID(ctxA, first.ID); got.Status != domain.PostStatusPublished || !got.DeleteAfterPublish {
		t.Fatalf("a published post keeps its history: %+v", got)
	}
	if err := store.MarkPostPublished(ctxA, second.ID, "fb_b", now); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPostPublished(ctxA, plain.ID, "fb_c", now); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetLibraryItem(ctxA, item.ID); got.Archived() || got.PublishCount != 3 {
		t.Fatalf("after 'Giữ lại' a sibling publish must not archive again and must still count: %+v", got)
	}
	// Restoring an item that was never archived must not touch pending posts.
	third, _ := store.CreatePost(ctxA, &domain.Post{ChannelID: chA.ID, Content: "x", Status: domain.PostStatusScheduled, ScheduledAt: at, MediaType: domain.MediaTypeNone, LibraryItemID: item.ID, DeleteAfterPublish: true})
	if _, err := store.RestoreLibraryItem(ctxA, item.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetPostByID(ctxA, third.ID); !got.DeleteAfterPublish {
		t.Fatal("restoring an active item is a no-op for pending posts")
	}

	// A library id of another tenant (or an unknown one) is stored unlinked, never cross-linked.
	foreign, err := store.CreatePost(ctxB, &domain.Post{ChannelID: chB.ID, Content: "b", Status: domain.PostStatusScheduled, ScheduledAt: at, MediaType: domain.MediaTypeNone, LibraryItemID: item.ID, DeleteAfterPublish: true})
	if err != nil {
		t.Fatal(err)
	}
	if foreign.LibraryItemID != "" || foreign.DeleteAfterPublish {
		t.Fatalf("a foreign library id must not be linked: %+v", foreign)
	}
	if stored, _ := store.GetPostByID(ctxB, foreign.ID); stored.LibraryItemID != "" || stored.DeleteAfterPublish {
		t.Fatalf("a foreign library id must not be stored: %+v", stored)
	}
	if err := store.MarkPostPublished(ctxB, foreign.ID, "fb_x", now); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetLibraryItem(ctxA, item.ID); got.PublishCount != 3 {
		t.Fatalf("another tenant's publish must not touch the item: %+v", got)
	}
	unknown, err := store.CreatePost(ctxA, &domain.Post{ChannelID: chA.ID, Content: "u", Status: domain.PostStatusScheduled, ScheduledAt: at, MediaType: domain.MediaTypeNone, LibraryItemID: newUUID(), DeleteAfterPublish: true})
	if err != nil || unknown.LibraryItemID != "" {
		t.Fatalf("an unknown library id must be stored unlinked: %v %+v", err, unknown)
	}

	// Deleting a Fanpage unbinds (never deletes) the library items tied to it.
	gone, err := store.UpsertChannel(ctxA, &domain.Channel{Name: "Gone", ExternalID: newUUID(), Platform: domain.PlatformFacebook, EncryptedAccessToken: "x", TokenStatus: domain.TokenStatusActive, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := store.CreateLibraryItem(ctxA, &domain.LibraryItem{Title: "bound", ChannelID: gone.ID, Content: "bound", MediaType: domain.MediaTypeNone})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteChannel(ctxA, gone.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetLibraryItem(ctxA, bound.ID); err != nil || got.ChannelID != "" || got.ChannelName != "" {
		t.Fatalf("deleting a Fanpage must unbind its library items: %v %+v", err, got)
	}
}

// runLibrarySearchContract: wildcards are literal, search is case-insensitive (incl. Vietnamese) and
// title/content are matched separately, identically on both stores.
func runLibrarySearchContract(t *testing.T, store Store, ctx context.Context) {
	t.Helper()
	for _, spec := range [][2]string{{"cotton", "100% cotton"}, {"plain", "100 cotton"}, {"under", "a_b"}, {"cross", "axb"}, {"slash", `path\to`}, {"viet", "Táo xanh đầu mùa"}, {"Alpha", "beta"}} {
		if _, err := store.CreateLibraryItem(ctx, &domain.LibraryItem{Title: spec[0], Content: spec[1], MediaType: domain.MediaTypeNone}); err != nil {
			t.Fatal(err)
		}
	}
	contents := func(q string) []string {
		items, err := store.QueryLibraryItems(ctx, domain.LibraryQuery{Search: q, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, it.Content)
		}
		return out
	}
	for q, want := range map[string]string{"100%": "100% cotton", "a_b": "a_b", `\to`: `path\to`, "TÁO": "Táo xanh đầu mùa", "ĐẦU MÙA": "Táo xanh đầu mùa"} {
		if got := contents(q); len(got) != 1 || got[0] != want {
			t.Fatalf("search %q must match only %q, got %v", q, want, got)
		}
	}
	if got := contents("alpha beta"); len(got) != 0 {
		t.Fatalf("title and content are matched separately, a needle spanning both must not match: %v", got)
	}
	if got := contents("ALPHA"); len(got) != 1 || got[0] != "beta" {
		t.Fatalf("title search must be case-insensitive: %v", got)
	}
}

// A failing library statement must never undo PUBLISHED: the Facebook post already exists.
func TestPostgresPublishSurvivesLibraryBookkeepingFailure(t *testing.T) {
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
	user, org, err := store.RegisterAccount(t.Context(), &domain.User{Email: newUUID() + "@bookkeeping.test", FullName: "B", PasswordHash: "h", IsActive: true}, "Bookkeeping workspace")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(context.Background(), "DELETE FROM organizations WHERE id=$1", org.ID)
		db.ExecContext(context.Background(), "DELETE FROM users WHERE id=$1", user.ID)
	})
	ctx := WithTenantContext(t.Context(), &domain.TenantContext{OrganizationID: org.ID, UserID: user.ID, OrgRole: domain.OrgRoleOwner})
	ch, err := store.UpsertChannel(ctx, &domain.Channel{Name: "P", ExternalID: newUUID(), Platform: domain.PlatformFacebook, EncryptedAccessToken: "x", TokenStatus: domain.TokenStatusActive, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.CreateLibraryItem(ctx, &domain.LibraryItem{Title: "overflow", Content: "c", MediaType: domain.MediaTypeNone})
	if err != nil {
		t.Fatal(err)
	}
	// publish_count is an INT: at its maximum the "+ 1" in the publish hook raises "integer out of range".
	if _, err := db.ExecContext(ctx, "UPDATE library_items SET publish_count = 2147483647 WHERE id = $1::uuid", item.ID); err != nil {
		t.Fatal(err)
	}
	post, err := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Content: "x", Status: domain.PostStatusProcessing, ScheduledAt: time.Now().UTC(), MediaType: domain.MediaTypeNone, LibraryItemID: item.ID, DeleteAfterPublish: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPostPublished(ctx, post.ID, "fb_1", time.Now().UTC()); err != nil {
		t.Fatalf("library bookkeeping failure must not fail the publish record: %v", err)
	}
	got, _ := store.GetPostByID(ctx, post.ID)
	if got.Status != domain.PostStatusPublished || got.ExternalPostID != "fb_1" {
		t.Fatalf("the post must stay PUBLISHED when the library side effect fails: %+v", got)
	}
	if after, _ := store.GetLibraryItem(ctx, item.ID); after.Archived() {
		t.Fatalf("the failed library update must have been rolled back: %+v", after)
	}

	// A concurrent permanent delete and publish of a post of that item must both succeed (lock order items -> posts).
	for i := 0; i < 20; i++ {
		item2, err := store.CreateLibraryItem(ctx, &domain.LibraryItem{Title: "race", Content: "c", MediaType: domain.MediaTypeNone})
		if err != nil {
			t.Fatal(err)
		}
		p, err := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Content: "x", Status: domain.PostStatusProcessing, ScheduledAt: time.Now().UTC(), MediaType: domain.MediaTypeNone, LibraryItemID: item2.ID, DeleteAfterPublish: true})
		if err != nil {
			t.Fatal(err)
		}
		errs := make(chan error, 2)
		go func() { errs <- store.MarkPostPublished(ctx, p.ID, "fb_race", time.Now().UTC()) }()
		go func() { errs <- store.DeleteLibraryItem(ctx, item2.ID) }()
		for j := 0; j < 2; j++ {
			if err := <-errs; err != nil && !errors.Is(err, domain.ErrLibraryItemNotFound) {
				t.Fatalf("concurrent publish/delete iteration %d failed: %v", i, err)
			}
		}
		if got, _ := store.GetPostByID(ctx, p.ID); got.Status != domain.PostStatusPublished {
			t.Fatalf("iteration %d: post must be PUBLISHED, got %s", i, got.Status)
		}
	}
}
