package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/config"
	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/database"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
	"github.com/TruongHoang2004/PostPilot/internal/platform/facebook"
	"github.com/TruongHoang2004/PostPilot/internal/infra/notifier"
	"github.com/TruongHoang2004/PostPilot/internal/infra/redis"
)

// Scheduler is responsible for:
// 1. Periodically scanning due posts (`SCHEDULED` or `FAILED` + `next_retry_at <= NOW()`).
// 2. Enqueuing jobs into Redis Queues (`FAST` or `HEAVY`).
// 3. Periodically verifying token health for all active Channels via `domain.PublisherRegistry`.
type Scheduler struct {
	cfg      *config.Config
	store    database.Store
	redis    *redis.Client
	cipher   *crypto.TokenCipher
	registry *domain.PublisherRegistry
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
	reg := domain.NewPublisherRegistry()
	if fbClient != nil {
		reg.Register(facebook.NewPublisherAdapter(fbClient))
	}
	return &Scheduler{
		cfg:      cfg,
		store:    store,
		redis:    redisClient,
		cipher:   cipher,
		registry: reg,
		notifier: notif,
		logger:   logger,
	}
}

// WithPublisherRegistry configures a custom multi-platform adapter registry for the Scheduler.
func (s *Scheduler) WithPublisherRegistry(reg *domain.PublisherRegistry) *Scheduler {
	if reg != nil {
		s.registry = reg
	}
	return s
}

// Start launches the background loops for due post scanning and token health checks.
func (s *Scheduler) Start(ctx context.Context) {
	go s.runDuePostScanner(ctx)
	go s.runTokenHealthChecker(ctx)
}

func (s *Scheduler) runDuePostScanner(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.SchedulerInterval)
	defer ticker.Stop()

	s.logger.Info("Post Scheduler started", "interval", s.cfg.SchedulerInterval.String())
	// Perform immediate scan upon startup
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

// ScanAndEnqueueDuePosts executes a single polling cycle claiming due posts and enqueuing them into Redis.
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
			ChannelID:  p.ChannelID,
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
			s.CheckAllChannelTokens(ctx)
		}
	}
}

// CheckAllChannelTokens verifies token health for all active channels on their respective platforms.
func (s *Scheduler) CheckAllChannelTokens(ctx context.Context) {
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		s.logger.Error("Token health check failed to list channels", "error", err)
		return
	}

	now := time.Now().UTC()
	for _, channel := range channels {
		if !channel.IsActive {
			continue
		}

		publisher, err := s.registry.Get(channel.Platform)
		if err != nil {
			continue
		}

		rawToken, err := s.cipher.Decrypt(channel.EncryptedAccessToken)
		if err != nil {
			s.logger.Error("Failed to decrypt channel token during health check", "channel_id", channel.ExternalID, "error", err)
			continue
		}

		health, err := publisher.VerifyToken(ctx, rawToken)
		if err != nil || health == nil || !health.IsValid {
			_ = s.store.UpdateChannelTokenStatus(ctx, channel.ID, domain.TokenStatusExpired, now)
			_ = s.notifier.SendAlert(
				ctx,
				"Channel Access Token Expired or Revoked",
				fmt.Sprintf("Channel: *%s* (`%s` - `%s`)\nReason: Access token is no longer valid. Please reconnect OAuth on the dashboard.", channel.Name, channel.ExternalID, channel.Platform),
			)
			continue
		}

		_ = s.store.UpdateChannelTokenStatus(ctx, channel.ID, domain.TokenStatusActive, now)
	}
}
