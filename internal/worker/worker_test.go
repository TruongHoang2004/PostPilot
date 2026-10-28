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
	channel, err := store.UpsertChannel(context.Background(), &domain.Channel{
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

	post, err := store.CreatePost(context.Background(), &domain.Post{
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

	enqueued := sched.ScanAndEnqueueDuePosts(context.Background())
	if enqueued != 1 {
		t.Fatalf("expected 1 post enqueued, got %d", enqueued)
	}

	job, err := redisClient.DequeuePublishJob(context.Background(), domain.QueueTierFast, 1)
	if err != nil {
		t.Fatalf("dequeue error: %v", err)
	}

	if err := proc.ProcessPostByID(context.Background(), job.PostID); err != nil {
		t.Fatalf("process post error: %v", err)
	}

	updatedPost, _ := store.GetPostByID(context.Background(), post.ID)
	if updatedPost.Status != domain.PostStatusPublished {
		t.Fatalf("expected post status PUBLISHED, got %s", updatedPost.Status)
	}
	if updatedPost.ExternalPostID != "998877_112233" {
		t.Fatalf("expected external_post_id 998877_112233, got %s", updatedPost.ExternalPostID)
	}

	logs, _ := store.ListPublishLogsByPost(context.Background(), post.ID)
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
