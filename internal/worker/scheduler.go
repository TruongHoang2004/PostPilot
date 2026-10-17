package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/happy-darwin/fb-autoposter/internal/config"
	"github.com/happy-darwin/fb-autoposter/internal/crypto"
	"github.com/happy-darwin/fb-autoposter/internal/database"
	"github.com/happy-darwin/fb-autoposter/internal/domain"
	"github.com/happy-darwin/fb-autoposter/internal/facebook"
	"github.com/happy-darwin/fb-autoposter/internal/infra/notifier"
	"github.com/happy-darwin/fb-autoposter/internal/infra/redis"
)

// Scheduler chịu trách nhiệm:
// 1. Quét định kỳ các bài viết đã đến giờ hẹn (`SCHEDULED`) hoặc đến giờ thử lại (`FAILED` + `next_retry_at <= NOW()`).
// 2. Đẩy job vào Redis Queue (`FAST` hoặc `HEAVY`).
// 3. Kiểm tra sức khỏe định kỳ của toàn bộ Page Access Token (`Token Health Check`).
type Scheduler struct {
	cfg      *config.Config
	store    database.Store
	redis    *redis.Client
	cipher   *crypto.TokenCipher
	fbClient *facebook.Client
	notifier *notifier.TelegramNotifier
	logger   *slog.Logger
}

func NewScheduler(
	cfg *config.Config,
	store database.Store,
	redisClient *redis.Client,
	cipher *crypto.TokenCipher,
	fbClient *facebook.Client,
	notif *notifier.TelegramNotifier,
	logger *slog.Logger,
) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		store:    store,
		redis:    redisClient,
		cipher:   cipher,
		fbClient: fbClient,
		notifier: notif,
		logger:   logger,
	}
}

// Start khởi chạy vòng lặp quét lịch hẹn và vòng lặp kiểm tra sức khỏe Token.
func (s *Scheduler) Start(ctx context.Context) {
	go s.runDuePostScanner(ctx)
	go s.runTokenHealthChecker(ctx)
}

func (s *Scheduler) runDuePostScanner(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.SchedulerInterval)
	defer ticker.Stop()

	s.logger.Info("Post Scheduler started", "interval", s.cfg.SchedulerInterval.String())
	// Quét ngay lần đầu khi khởi động
	s.ScanAndEnqueueDuePosts(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Post Scheduler stopping")
			return
		case <-ticker.C:
			s.ScanAndEnqueueDuePosts(ctx)
		}
	}
}

// ScanAndEnqueueDuePosts thực thi 1 chu kỳ quét DB và đẩy các bài đến hạn vào Redis Queue.
func (s *Scheduler) ScanAndEnqueueDuePosts(ctx context.Context) int {
	now := time.Now().UTC()
	claimedPosts, err := s.store.ClaimDuePosts(ctx, now, 50)
	if err != nil {
		s.logger.Error("Failed to claim due posts", "error", err)
		return 0
	}

	enqueued := 0
	for _, p := range claimedPosts {
		payload := domain.PublishJobPayload{
			PostID:     p.ID,
			PageUUID:   p.PageUUID,
			QueueTier:  p.QueueTier,
			Attempt:    p.RetryCount + 1,
			EnqueuedAt: now,
		}
		if err := s.redis.EnqueuePublishJob(ctx, payload); err != nil {
			s.logger.Error("Failed to enqueue job to Redis, reverting post to retry", "post_id", p.ID, "error", err)
			retryAt := now.Add(30 * time.Second)
			_ = s.store.MarkPostFailedOrRetry(ctx, p.ID, p.RetryCount, &retryAt, domain.PostStatusFailed, 0, "redis enqueue error: "+err.Error())
			continue
		}
		enqueued++
		s.logger.Info("Enqueued scheduled post", "post_id", p.ID, "queue_tier", p.QueueTier, "attempt", payload.Attempt)
	}
	return enqueued
}

func (s *Scheduler) runTokenHealthChecker(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.TokenCheckInterval)
	defer ticker.Stop()

	s.logger.Info("Token Health Checker started", "interval", s.cfg.TokenCheckInterval.String())
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.CheckAllPageTokens(ctx)
		}
	}
}

// CheckAllPageTokens kiểm tra toàn bộ các Fanpage đang hoạt động xem Token còn hợp lệ trên Meta hay không.
func (s *Scheduler) CheckAllPageTokens(ctx context.Context) {
	pages, err := s.store.ListPages(ctx)
	if err != nil {
		s.logger.Error("Token health check failed to list pages", "error", err)
		return
	}

	now := time.Now().UTC()
	for _, page := range pages {
		if !page.IsActive {
			continue
		}

		rawToken, err := s.cipher.Decrypt(page.EncryptedAccessToken)
		if err != nil {
			s.logger.Error("Failed to decrypt page token during health check", "page_id", page.PageID, "error", err)
			continue
		}

		debugInfo, err := s.fbClient.DebugToken(ctx, rawToken, "")
		if err != nil || !debugInfo.IsValid {
			_ = s.store.UpdatePageTokenStatus(ctx, page.ID, domain.TokenStatusExpired, now)
			_ = s.notifier.SendAlert(
				ctx,
				"Page Access Token Hết Hạn / Mất Quyền",
				fmt.Sprintf("Fanpage: *%s* (`%s`)\nLý do: Token không còn hợp lệ. Vui lòng kết nối lại OAuth trên Dashboard.", page.PageName, page.PageID),
			)
			continue
		}

		_ = s.store.UpdatePageTokenStatus(ctx, page.ID, domain.TokenStatusActive, now)
	}
}
