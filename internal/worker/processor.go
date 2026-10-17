package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/happy-darwin/fb-autoposter/internal/config"
	"github.com/happy-darwin/fb-autoposter/internal/crypto"
	"github.com/happy-darwin/fb-autoposter/internal/database"
	"github.com/happy-darwin/fb-autoposter/internal/domain"
	"github.com/happy-darwin/fb-autoposter/internal/facebook"
	"github.com/happy-darwin/fb-autoposter/internal/infra/notifier"
	"github.com/happy-darwin/fb-autoposter/internal/infra/redis"
)

// Processor quản lý đội ngũ Worker tiêu thụ hàng đợi `FAST` và `HEAVY`,
// thực thi đăng bài lên Facebook, kiểm soát Rate Limit và Exponential Backoff Retry.
type Processor struct {
	cfg      *config.Config
	store    database.Store
	redis    *redis.Client
	cipher   *crypto.TokenCipher
	fbClient *facebook.Client
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
	return &Processor{
		cfg:      cfg,
		store:    store,
		redis:    redisClient,
		cipher:   cipher,
		fbClient: fbClient,
		notifier: notif,
		logger:   logger,
	}
}

// Start khởi chạy các goroutine worker cho cả 2 hàng đợi FAST và HEAVY.
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
		if err := p.ProcessPostByID(ctx, job.PostID); err != nil {
			p.logger.Warn("Job execution ended with error", "worker", workerName, "post_id", job.PostID, "error", err)
		}
	}
}

// ProcessPostByID thực hiện toàn bộ quy trình đăng 1 bài viết lên Facebook:
// 1. Tải thông tin Post & Fanpage từ DB.
// 2. Kiểm tra Rate-Limit hiện tại của Fanpage trên Redis. Nếu > ngưỡng an toàn (80%), tự động hoãn 5 phút.
// 3. Giải mã Page Access Token bằng AES-256-GCM.
// 4. Gọi Meta Graph API tương ứng với MediaType.
// 5. Ghi nhận `publish_logs`, cập nhật Rate Limit và xử lý Retry (nếu lỗi tạm thời) hoặc bắn cảnh báo Telegram (nếu lỗi nghiêm trọng).
func (p *Processor) ProcessPostByID(ctx context.Context, postID string) error {
	post, err := p.store.GetPostByID(ctx, postID)
	if err != nil {
		return fmt.Errorf("load post %s: %w", postID, err)
	}

	if post.Status == domain.PostStatusPublished || post.Status == domain.PostStatusCancelled {
		return nil
	}

	page, err := p.store.GetPageByUUID(ctx, post.PageUUID)
	if err != nil {
		return fmt.Errorf("load page %s: %w", post.PageUUID, err)
	}

	if !page.IsActive || page.TokenStatus == domain.TokenStatusExpired || page.TokenStatus == domain.TokenStatusRevoked {
		errMsg := fmt.Sprintf("page %s (%s) is inactive or token status is %s", page.PageName, page.PageID, page.TokenStatus)
		_ = p.store.MarkPostFailedOrRetry(ctx, post.ID, post.RetryCount, nil, domain.PostStatusFailed, 190, errMsg)
		return errors.New(errMsg)
	}

	// Kiểm tra Rate-Limit từ Redis trước khi gọi Meta API
	cachedUsage, _ := p.redis.GetPageRateLimit(ctx, page.PageID)
	if cachedUsage.MaxPercent() >= p.cfg.RateLimitThresholdPercent {
		delayUntil := time.Now().UTC().Add(5 * time.Minute)
		msg := fmt.Sprintf("Throttled: Page rate-limit usage is %d%% (threshold %d%%), delayed 5m", cachedUsage.MaxPercent(), p.cfg.RateLimitThresholdPercent)
		_ = p.store.MarkPostFailedOrRetry(ctx, post.ID, post.RetryCount, &delayUntil, domain.PostStatusFailed, 32, msg)
		return errors.New(msg)
	}

	if err := p.store.MarkPostProcessing(ctx, post.ID); err != nil {
		return err
	}

	// Giải mã Access Token trong RAM
	accessToken, err := p.cipher.Decrypt(page.EncryptedAccessToken)
	if err != nil {
		_ = p.store.MarkPostFailedOrRetry(ctx, post.ID, post.RetryCount, nil, domain.PostStatusFailed, 0, "token decryption error: "+err.Error())
		return err
	}

	attemptNumber := post.RetryCount + 1
	outcome, pubErr := p.fbClient.PublishPost(ctx, page.PageID, accessToken, post)

	// Lưu toàn bộ các bước gọi API vào bảng publish_logs
	if outcome != nil {
		for _, step := range outcome.Steps {
			logEntry := &domain.PublishLog{
				PostID:         post.ID,
				PageUUID:       page.ID,
				AttemptNumber:  attemptNumber,
				Action:         step.Action,
				HTTPStatus:     step.Metrics.HTTPStatus,
				RequestSummary: step.RequestSummary,
				ResponseBody:   step.Metrics.RawResponse,
				DurationMs:     step.Metrics.DurationMs,
			}
			var fbErr *facebook.GraphAPIError
			if errors.As(step.Error, &fbErr) {
				logEntry.FBErrorCode = fbErr.Code
				logEntry.FBErrorSubcode = fbErr.ErrorSubcode
			}
			_ = p.store.InsertPublishLog(ctx, logEntry)
		}

		// Cập nhật thông số Rate Limit mới nhất từ Header của Meta
		if outcome.FinalRateLimit.MaxPercent() > 0 {
			_ = p.redis.SavePageRateLimit(ctx, page.PageID, outcome.FinalRateLimit)
			_ = p.store.UpdatePageRateLimit(ctx, page.ID, outcome.FinalRateLimit)
		}
	}

	if pubErr == nil && outcome != nil {
		now := time.Now().UTC()
		if err := p.store.MarkPostPublished(ctx, post.ID, outcome.FBPostID, now); err != nil {
			return err
		}
		p.logger.Info("Successfully published post to Facebook",
			"post_id", post.ID,
			"page_id", page.PageID,
			"fb_post_id", outcome.FBPostID,
		)
		return nil
	}

	// Xử lý phân loại lỗi & tính toán Exponential Backoff Retry
	return p.handlePublishFailure(ctx, post, page, attemptNumber, pubErr)
}

func (p *Processor) handlePublishFailure(ctx context.Context, post *domain.Post, page *domain.FacebookPage, attempt int, pubErr error) error {
	var fbErr *facebook.GraphAPIError
	errCode := 0
	isRetryable := true

	if errors.As(pubErr, &fbErr) {
		errCode = fbErr.Code
		isRetryable = fbErr.IsRetryable()

		if fbErr.IsTokenInvalid() {
			now := time.Now().UTC()
			_ = p.store.UpdatePageTokenStatus(ctx, page.ID, domain.TokenStatusExpired, now)
			_ = p.notifier.SendAlert(
				ctx,
				"Token Fanpage Hết Hạn Khi Đang Đăng Bài (Code 190)",
				fmt.Sprintf("Fanpage: *%s* (`%s`)\nBài viết ID: `%s`\nChi tiết: %s", page.PageName, page.PageID, post.ID, fbErr.Message),
			)
		}
	}

	// Nếu lỗi có thể thử lại (Retryable) và chưa vượt quá max_retries
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

	// Dừng hẳn (Permanent Failure) -> Cập nhật trạng thái FAILED và gửi cảnh báo Telegram
	_ = p.store.MarkPostFailedOrRetry(ctx, post.ID, attempt, nil, domain.PostStatusFailed, errCode, pubErr.Error())
	_ = p.notifier.SendAlert(
		ctx,
		"Đăng Bài Tự Động Thất Bại Vĩnh Viễn",
		fmt.Sprintf("Fanpage: *%s* (`%s`)\nBài viết ID: `%s`\nSố lần thử: `%d/%d`\nLỗi: `%s`",
			page.PageName, page.PageID, post.ID, attempt, post.MaxRetries, pubErr.Error()),
	)
	return pubErr
}

// ComputeBackoffDuration tính thời gian chờ Exponential Backoff:
// Lần 1 lỗi -> chờ 1 phút
// Lần 2 lỗi -> chờ 5 phút
// Lần 3+ lỗi -> chờ 15 phút
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
