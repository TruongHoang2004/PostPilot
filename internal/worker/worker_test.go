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

	"github.com/happy-darwin/fb-autoposter/internal/config"
	"github.com/happy-darwin/fb-autoposter/internal/crypto"
	"github.com/happy-darwin/fb-autoposter/internal/database"
	"github.com/happy-darwin/fb-autoposter/internal/domain"
	"github.com/happy-darwin/fb-autoposter/internal/facebook"
	"github.com/happy-darwin/fb-autoposter/internal/infra/notifier"
	"github.com/happy-darwin/fb-autoposter/internal/infra/redis"
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
	page, err := store.UpsertPage(context.Background(), &domain.FacebookPage{
		PageID:               "998877",
		PageName:             "Happy Darwin Tech Page",
		Category:             "Software Company",
		EncryptedAccessToken: encToken,
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		t.Fatalf("upsert page error: %v", err)
	}

	post, err := store.CreatePost(context.Background(), &domain.Post{
		PageUUID:    page.ID,
		Content:     "Bài đăng tự động kiểm thử End-to-End bằng Golang!",
		MediaType:   domain.MediaTypeNone,
		Status:      domain.PostStatusScheduled,
		ScheduledAt: time.Now().UTC().Add(-1 * time.Minute), // Đã đến giờ đăng
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
	if updatedPost.FBPostID != "998877_112233" {
		t.Fatalf("expected fb_post_id 998877_112233, got %s", updatedPost.FBPostID)
	}

	logs, _ := store.ListPublishLogsByPost(context.Background(), post.ID)
	if len(logs) != 1 {
		t.Fatalf("expected 1 publish log entry, got %d", len(logs))
	}
}
