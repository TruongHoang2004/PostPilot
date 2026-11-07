package worker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/config"
	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/database"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
	"github.com/TruongHoang2004/PostPilot/internal/infra/notifier"
	"github.com/TruongHoang2004/PostPilot/internal/infra/redis"
	"github.com/TruongHoang2004/PostPilot/internal/platform/facebook"
)

type mockRoundTripper struct {
	fn http.HandlerFunc
}

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.fn(rec, req)
	return rec.Result(), nil
}

func TestSchedulerAndProcessorEndToEnd(t *testing.T) {
	cfg := config.Load()
	cfg.RateLimitThresholdPercent = 80
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	store := database.NewInMemoryStore()
	redisClient := redis.NewInMemoryClient()
	cipher, _ := crypto.NewTokenCipher(cfg.TokenEncryptionKey)
	fbClient := facebook.NewClient("https://graph.facebook.com", "v20.0").WithHTTPClient(&http.Client{
		Transport: mockRoundTripper{
			fn: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Page-Usage", `{"call_count":15,"total_cputime":10,"total_time":12}`)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "998877_112233",
				})
			},
		},
	})
	notif := notifier.NewTelegramNotifier("", "", logger)

	encToken, _ := cipher.Encrypt("EAAB_permanent_page_token")
	channel, err := store.UpsertChannel(domain.WithTenantContext(context.Background(), domain.DefaultTenantContext()), &domain.Channel{
		Platform:             domain.PlatformFacebook,
		ExternalID:           "998877",
		Name:                 "Happy Darwin Tech Channel",
		Category:             "Software Company",
		EncryptedAccessToken: encToken,
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		t.Fatalf("upsert channel error: %v", err)
	}

	post, err := store.CreatePost(domain.WithTenantContext(context.Background(), domain.DefaultTenantContext()), &domain.Post{
		ChannelID:   channel.ID,
		Content:     "Automated end-to-end testing post in Go!",
		MediaType:   domain.MediaTypeNone,
		Status:      domain.PostStatusScheduled,
		ScheduledAt: time.Now().UTC().Add(-1 * time.Minute),
		MaxRetries:  3,
	})
	if err != nil {
		t.Fatalf("create post error: %v", err)
	}

	sched := NewScheduler(cfg, store, redisClient, cipher, fbClient, notif, logger)
	proc := NewProcessor(cfg, store, redisClient, cipher, fbClient, notif, logger)

	enqueued := sched.ScanAndEnqueueDuePosts(domain.WithTenantContext(context.Background(), domain.DefaultTenantContext()))
	if enqueued != 1 {
		t.Fatalf("expected 1 post enqueued, got %d", enqueued)
	}

	job, err := redisClient.DequeuePublishJob(domain.WithTenantContext(context.Background(), domain.DefaultTenantContext()), domain.QueueTierFast, 1)
	if err != nil {
		t.Fatalf("dequeue error: %v", err)
	}

	if err := proc.ProcessPostByID(domain.WithTenantContext(context.Background(), domain.DefaultTenantContext()), job.PostID); err != nil {
		t.Fatalf("process post error: %v", err)
	}

	updatedPost, _ := store.GetPostByID(domain.WithTenantContext(context.Background(), domain.DefaultTenantContext()), post.ID)
	if updatedPost.Status != domain.PostStatusPublished {
		t.Fatalf("expected post status PUBLISHED, got %s", updatedPost.Status)
	}
	if updatedPost.ExternalPostID != "998877_112233" {
		t.Fatalf("expected external_post_id 998877_112233, got %s", updatedPost.ExternalPostID)
	}

	logs, _ := store.ListPublishLogsByPost(domain.WithTenantContext(context.Background(), domain.DefaultTenantContext()), post.ID)
	if len(logs) != 1 || logs[0].Action != "PUBLISH_FEED" {
		t.Fatalf("expected 1 PUBLISH_FEED log entry, got %+v", logs)
	}
}
func TestSchedulerAndProcessorOtherOrganization(t *testing.T) {
	cfg := config.Load()
	cfg.RateLimitThresholdPercent = 80
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	store := database.NewInMemoryStore()
	_, _ = store.CreateOrganization(t.Context(), &domain.Organization{ID: "workspace-b", Name: "Workspace B"}, domain.DefaultUserID)
	redisClient := redis.NewInMemoryClient()
	cipher, _ := crypto.NewTokenCipher(cfg.TokenEncryptionKey)
	fbClient := facebook.NewClient("https://graph.facebook.com", "v20.0").WithHTTPClient(&http.Client{
		Transport: mockRoundTripper{
			fn: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Page-Usage", `{"call_count":15,"total_cputime":10,"total_time":12}`)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "998877_112233",
				})
			},
		},
	})
	notif := notifier.NewTelegramNotifier("", "", logger)

	encToken, _ := cipher.Encrypt("EAAB_permanent_page_token")
	channel, err := store.UpsertChannel(domain.WithTenantContext(context.Background(), &domain.TenantContext{OrganizationID: "workspace-b"}), &domain.Channel{
		Platform:             domain.PlatformFacebook,
		ExternalID:           "998877",
		Name:                 "Happy Darwin Tech Channel",
		Category:             "Software Company",
		EncryptedAccessToken: encToken,
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		t.Fatalf("upsert channel error: %v", err)
	}

	post, err := store.CreatePost(domain.WithTenantContext(context.Background(), &domain.TenantContext{OrganizationID: "workspace-b"}), &domain.Post{
		ChannelID:   channel.ID,
		Content:     "Automated end-to-end testing post in Go!",
		MediaType:   domain.MediaTypeNone,
		Status:      domain.PostStatusScheduled,
		ScheduledAt: time.Now().UTC().Add(-1 * time.Minute),
		MaxRetries:  3,
	})
	if err != nil {
		t.Fatalf("create post error: %v", err)
	}

	sched := NewScheduler(cfg, store, redisClient, cipher, fbClient, notif, logger)
	proc := NewProcessor(cfg, store, redisClient, cipher, fbClient, notif, logger)

	enqueued := sched.ScanAndEnqueueDuePosts(domain.WithTenantContext(context.Background(), &domain.TenantContext{OrganizationID: "workspace-b"}))
	if enqueued != 1 {
		t.Fatalf("expected 1 post enqueued, got %d", enqueued)
	}

	job, err := redisClient.DequeuePublishJob(domain.WithTenantContext(context.Background(), &domain.TenantContext{OrganizationID: "workspace-b"}), domain.QueueTierFast, 1)
	if err != nil {
		t.Fatalf("dequeue error: %v", err)
	}

	if job.OrganizationID != "workspace-b" {
		t.Fatalf("job missing tenant: %+v", job)
	}
	if err := proc.ProcessPostByID(domain.WithTenantContext(context.Background(), &domain.TenantContext{OrganizationID: "workspace-b"}), job.PostID); err != nil {
		t.Fatalf("process post error: %v", err)
	}

	updatedPost, _ := store.GetPostByID(domain.WithTenantContext(context.Background(), &domain.TenantContext{OrganizationID: "workspace-b"}), post.ID)
	if updatedPost.Status != domain.PostStatusPublished {
		t.Fatalf("expected post status PUBLISHED, got %s", updatedPost.Status)
	}
	if updatedPost.ExternalPostID != "998877_112233" {
		t.Fatalf("expected external_post_id 998877_112233, got %s", updatedPost.ExternalPostID)
	}

	logs, _ := store.ListPublishLogsByPost(domain.WithTenantContext(context.Background(), &domain.TenantContext{OrganizationID: "workspace-b"}), post.ID)
	if len(logs) != 1 || logs[0].Action != "PUBLISH_FEED" {
		t.Fatalf("expected 1 PUBLISH_FEED log entry, got %+v", logs)
	}
}

func TestComputeBackoffDuration(t *testing.T) {
	d1 := ComputeBackoffDuration(1)
	if d1 != 1*time.Minute {
		t.Fatalf("expected 1m for attempt 1, got %v", d1)
	}
	d2 := ComputeBackoffDuration(2)
	if d2 != 5*time.Minute {
		t.Fatalf("expected 5m for attempt 2, got %v", d2)
	}
	d3 := ComputeBackoffDuration(3)
	if d3 != 15*time.Minute {
		t.Fatalf("expected 15m for attempt 3, got %v", d3)
	}
	d4 := ComputeBackoffDuration(4)
	if d4 != 15*time.Minute {
		t.Fatalf("expected 15m for attempt 4, got %v", d4)
	}
}

// A post scheduled from the library with "delete after publish" must soft-delete its library item
// when the worker publishes it, while a post without the flag only bumps the publish counter.
func TestProcessorArchivesLibraryItemAfterPublish(t *testing.T) {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := database.NewInMemoryStore()
	redisClient := redis.NewInMemoryClient()
	cipher, _ := crypto.NewTokenCipher(cfg.TokenEncryptionKey)
	fbClient := facebook.NewClient("https://graph.facebook.com", "v20.0").WithHTTPClient(&http.Client{
		Transport: mockRoundTripper{fn: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "555_777"})
		}},
	})
	notif := notifier.NewTelegramNotifier("", "", logger)
	ctx := domain.WithTenantContext(context.Background(), domain.DefaultTenantContext())
	encToken, _ := cipher.Encrypt("EAAB_permanent_page_token")
	channel, err := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "555", Name: "Library channel", EncryptedAccessToken: encToken, TokenStatus: domain.TokenStatusActive, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	keepItem, _ := store.CreateLibraryItem(ctx, &domain.LibraryItem{Title: "keep", Content: "keep", MediaType: domain.MediaTypeNone})
	dropItem, _ := store.CreateLibraryItem(ctx, &domain.LibraryItem{Title: "drop", Content: "drop", MediaType: domain.MediaTypeNone})
	mk := func(item *domain.LibraryItem, flag bool) *domain.Post {
		p, err := store.CreatePost(ctx, &domain.Post{ChannelID: channel.ID, Content: item.Content, MediaType: domain.MediaTypeNone, Status: domain.PostStatusScheduled, ScheduledAt: time.Now().UTC().Add(-time.Minute), MaxRetries: 3, LibraryItemID: item.ID, DeleteAfterPublish: flag})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	keepPost, dropPost := mk(keepItem, false), mk(dropItem, true)

	proc := NewProcessor(cfg, store, redisClient, cipher, fbClient, notif, logger)
	for _, p := range []*domain.Post{keepPost, dropPost} {
		if err := proc.ProcessPostByID(ctx, p.ID); err != nil {
			t.Fatalf("process %s: %v", p.ID, err)
		}
	}
	// Re-processing a published post is a no-op and must not inflate the counter.
	if err := proc.ProcessPostByID(ctx, dropPost.ID); err != nil {
		t.Fatal(err)
	}
	kept, _ := store.GetLibraryItem(ctx, keepItem.ID)
	dropped, _ := store.GetLibraryItem(ctx, dropItem.ID)
	if kept.Archived() || kept.PublishCount != 1 {
		t.Fatalf("item without the flag must stay active and count once: %+v", kept)
	}
	if !dropped.Archived() || dropped.PublishCount != 1 {
		t.Fatalf("item with the flag must be soft-deleted after publishing: %+v", dropped)
	}
}
