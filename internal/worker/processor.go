package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/config"
	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/database"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
	"github.com/TruongHoang2004/PostPilot/internal/infra/notifier"
	"github.com/TruongHoang2004/PostPilot/internal/infra/redis"
	"github.com/TruongHoang2004/PostPilot/internal/platform/facebook"
)

// Processor manages worker pools consuming from `FAST` and `HEAVY` queues,
// executing post publishing via Output Port `domain.PublisherRegistry`,
// and enforcing rate limiting and exponential backoff retries across platforms.
type Processor struct {
	cfg      *config.Config
	store    database.Store
	redis    *redis.Client
	cipher   *crypto.TokenCipher
	registry *domain.PublisherRegistry
	notifier *notifier.TelegramNotifier
	logger   *slog.Logger
}

func NewProcessor(
	cfg *config.Config,
	store database.Store,
	redisClient *redis.Client,
	cipher *crypto.TokenCipher,
	fbClient *facebook.Client,
	notif *notifier.TelegramNotifier,
	logger *slog.Logger,
) *Processor {
	reg := domain.NewPublisherRegistry()
	if fbClient != nil {
		reg.Register(facebook.NewPublisherAdapter(fbClient))
	}
	return &Processor{
		cfg:      cfg,
		store:    store,
		redis:    redisClient,
		cipher:   cipher,
		registry: reg,
		notifier: notif,
		logger:   logger,
	}
}

// WithPublisherRegistry allows injecting a custom multi-platform adapter registry.
func (p *Processor) WithPublisherRegistry(reg *domain.PublisherRegistry) *Processor {
	if reg != nil {
		p.registry = reg
	}
	return p
}

// Start launches worker goroutines for both FAST and HEAVY queues.
func (p *Processor) Start(ctx context.Context, wg *sync.WaitGroup) {
	for i := 1; i <= p.cfg.FastWorkerConcurrency; i++ {
		wg.Add(1)
		workerName := fmt.Sprintf("fast-worker-%d", i)
		go func(name string) {
			defer wg.Done()
			p.workerLoop(ctx, name, domain.QueueTierFast)
		}(workerName)
	}

	for i := 1; i <= p.cfg.HeavyWorkerConcurrency; i++ {
		wg.Add(1)
		workerName := fmt.Sprintf("heavy-worker-%d", i)
		go func(name string) {
			defer wg.Done()
			p.workerLoop(ctx, name, domain.QueueTierHeavy)
		}(workerName)
	}
}

func (p *Processor) workerLoop(ctx context.Context, workerName string, tier string) {
	p.logger.Info("Worker started", "worker", workerName, "tier", tier)
	for {
		select {
		case <-ctx.Done():
			p.logger.Info("Worker stopping", "worker", workerName)
			return
		default:
		}

		job, err := p.redis.DequeuePublishJob(ctx, tier, 2)
		if err != nil {
			if errors.Is(err, redis.ErrQueueEmpty) || errors.Is(err, context.Canceled) {
				continue
			}
			p.logger.Warn("Queue dequeue error", "worker", workerName, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		p.logger.Info("Worker picked up job", "worker", workerName, "post_id", job.PostID, "attempt", job.Attempt)
		if job.OrganizationID == "" {
			p.logger.Error("Legacy job missing organization; operator reconciliation required", "post_id", job.PostID)
			continue
		}
		jobCtx := domain.WithTenantContext(ctx, &domain.TenantContext{OrganizationID: job.OrganizationID})
		if err := p.ProcessPostByID(jobCtx, job.PostID); err != nil {
			p.logger.Warn("Job execution ended with error", "worker", workerName, "post_id", job.PostID, "error", err)
		}
	}
}

// ProcessPostByID executes the end-to-end publishing flow for a post to its target channel:
// 1. Load Post & Channel from database.
// 2. Check channel rate limits on Redis. If > threshold (e.g. 80%), postpone for 5 minutes.
// 3. Decrypt Access Token in memory via AES-256-GCM.
// 4. Resolve the platform adapter from `PublisherRegistry` and invoke `Publish`.
// 5. Persist `publish_logs`, update rate limits, and handle retries or Telegram alerts.
func (p *Processor) ProcessPostByID(ctx context.Context, postID string) error {
	post, err := p.store.GetPostByID(ctx, postID)
	if err != nil {
		return fmt.Errorf("load post %s: %w", postID, err)
	}

	if post.IsTerminal() {
		return nil
	}

	channel, err := p.store.GetChannelByID(ctx, post.ChannelID)
	if err != nil {
		return fmt.Errorf("load channel %s: %w", post.ChannelID, err)
	}

	if !channel.IsUsable() {
		errMsg := fmt.Sprintf("channel %s (%s) is inactive or token status is %s", channel.Name, channel.ExternalID, channel.TokenStatus)
		_ = p.store.MarkPostFailedOrRetry(ctx, post.ID, post.RetryCount, nil, domain.PostStatusFailed, 190, errMsg)
		return errors.New(errMsg)
	}

	// Check rate limit cache on Redis before calling the adapter
	cachedUsage, _ := p.redis.GetChannelRateLimit(ctx, channel.ExternalID)
	if cachedUsage.MaxPercent() >= p.cfg.RateLimitThresholdPercent {
		delayUntil := time.Now().UTC().Add(5 * time.Minute)
		msg := fmt.Sprintf("Throttled: Channel rate-limit usage is %d%% (threshold %d%%), delayed 5m", cachedUsage.MaxPercent(), p.cfg.RateLimitThresholdPercent)
		_ = p.store.MarkPostFailedOrRetry(ctx, post.ID, post.RetryCount, &delayUntil, domain.PostStatusFailed, 32, msg)
		return errors.New(msg)
	}

	publisher, err := p.registry.Get(channel.Platform)
	if err != nil {
		_ = p.store.MarkPostFailedOrRetry(ctx, post.ID, post.RetryCount, nil, domain.PostStatusFailed, 0, err.Error())
		return err
	}

	if err := p.store.MarkPostProcessing(ctx, post.ID); err != nil {
		return err
	}

	// Decrypt Access Token in memory
	accessToken, err := p.cipher.Decrypt(channel.EncryptedAccessToken)
	if err != nil {
		_ = p.store.MarkPostFailedOrRetry(ctx, post.ID, post.RetryCount, nil, domain.PostStatusFailed, 0, "token decryption error: "+err.Error())
		return err
	}

	attemptNumber := post.RetryCount + 1
	outcome, pubErr := publisher.Publish(ctx, domain.PublishRequest{
		Channel:     *channel,
		AccessToken: accessToken,
		Post:        post,
	})

	// Record all API execution steps into publish_logs table
	if outcome != nil {
		for _, step := range outcome.Steps {
			logEntry := &domain.PublishLog{
				PostID:         post.ID,
				ChannelID:      channel.ID,
				AttemptNumber:  attemptNumber,
				Action:         step.Action,
				HTTPStatus:     step.HTTPStatus,
				RequestSummary: step.RequestSummary,
				ResponseBody:   step.RawResponse,
				DurationMs:     step.DurationMs,
			}
			var platErr domain.PublisherError
			if errors.As(step.Error, &platErr) {
				logEntry.ErrorCode = platErr.ErrorCode()
				logEntry.ErrorSubcode = platErr.Subcode()
			}
			_ = p.store.InsertPublishLog(ctx, logEntry)
		}

		// Update latest rate limit metrics from the adapter
		if outcome.FinalRateLimit.MaxPercent() > 0 {
			_ = p.redis.SaveChannelRateLimit(ctx, channel.ExternalID, outcome.FinalRateLimit)
			_ = p.store.UpdateChannelRateLimit(ctx, channel.ID, outcome.FinalRateLimit)
		}
	}

	if pubErr == nil && outcome != nil {
		now := time.Now().UTC()
		if err := p.store.MarkPostPublished(ctx, post.ID, outcome.ExternalPostID, now); err != nil {
			return err
		}
		p.logger.Info("Successfully published post to channel",
			"platform", channel.Platform,
			"post_id", post.ID,
			"channel_id", channel.ExternalID,
			"external_post_id", outcome.ExternalPostID,
		)
		return nil
	}

	// Handle error classification and exponential backoff retry
	return p.handlePublishFailure(ctx, post, *channel, attemptNumber, pubErr)
}

func (p *Processor) handlePublishFailure(ctx context.Context, post *domain.Post, channel domain.Channel, attempt int, pubErr error) error {
	if pubErr == nil {
		pubErr = errors.New("publisher returned empty result without error")
	}

	var platErr domain.PublisherError
	errCode := 0
	isRetryable := false

	if errors.As(pubErr, &platErr) {
		errCode = platErr.ErrorCode()
		isRetryable = platErr.IsRetryable()

		if platErr.IsTokenInvalid() {
			now := time.Now().UTC()
			_ = p.store.UpdateChannelTokenStatus(ctx, channel.ID, domain.TokenStatusExpired, now)
			_ = p.notifier.SendAlert(
				ctx,
				"Channel Access Token Expired During Publishing",
				fmt.Sprintf("Channel: *%s* (`%s` - `%s`)\nPost ID: `%s`\nDetails: %s", channel.Name, channel.ExternalID, channel.Platform, post.ID, platErr.Error()),
			)
		}
	}

	// If retryable error and attempt count has not exceeded max_retries
	if isRetryable && attempt < post.MaxRetries {
		backoffDuration := ComputeBackoffDuration(attempt)
		nextRetry := time.Now().UTC().Add(backoffDuration)
		_ = p.store.MarkPostFailedOrRetry(ctx, post.ID, attempt, &nextRetry, domain.PostStatusFailed, errCode, pubErr.Error())
		p.logger.Warn("Scheduled post for retry with exponential backoff",
			"post_id", post.ID,
			"attempt", attempt,
			"max_retries", post.MaxRetries,
			"next_retry_in", backoffDuration.String(),
		)
		return pubErr
	}

	// Permanent Failure -> Mark post FAILED and send Telegram alert
	_ = p.store.MarkPostFailedOrRetry(ctx, post.ID, attempt, nil, domain.PostStatusFailed, errCode, pubErr.Error())
	_ = p.notifier.SendAlert(
		ctx,
		"Auto-Publishing Failed Permanently",
		fmt.Sprintf("Channel: *%s* (`%s`)\nPost ID: `%s`\nAttempt: `%d/%d`\nError: `%s`",
			channel.Name, channel.ExternalID, post.ID, attempt, post.MaxRetries, pubErr.Error()),
	)
	return pubErr
}

// ComputeBackoffDuration calculates exponential backoff duration:
// Attempt 1 -> 1 minute
// Attempt 2 -> 5 minutes
// Attempt 3+ -> 15 minutes
func ComputeBackoffDuration(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 1 * time.Minute
	case 2:
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}
