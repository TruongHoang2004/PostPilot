package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// Store defines the storage Output Port interface for the entire system.
type Store interface {
	// Channel Management
	UpsertChannel(ctx context.Context, channel *domain.Channel) (*domain.Channel, error)
	ListChannels(ctx context.Context) ([]domain.Channel, error)
	GetChannelByID(ctx context.Context, id string) (*domain.Channel, error)
	DeleteChannel(ctx context.Context, id string) error
	UpdateChannelTokenStatus(ctx context.Context, id string, status string, checkedAt time.Time) error
	UpdateChannelRateLimit(ctx context.Context, id string, usage domain.RateLimitUsage) error

	// Post & Publishing Management (Core Domain)
	CreatePost(ctx context.Context, post *domain.Post) (*domain.Post, error)
	GetPostByID(ctx context.Context, id string) (*domain.Post, error)
	ListPosts(ctx context.Context, statusFilter string, limit int) ([]domain.Post, error)
	ClaimDuePosts(ctx context.Context, now time.Time, batchSize int) ([]domain.Post, error)
	MarkPostProcessing(ctx context.Context, postID string) error
	MarkPostPublished(ctx context.Context, postID string, externalPostID string, publishedAt time.Time) error
	MarkPostFailedOrRetry(ctx context.Context, postID string, retryCount int, nextRetryAt *time.Time, finalStatus string, errCode int, errMsg string) error
	CancelPost(ctx context.Context, postID string) error

	// Audit Logs & Media Assets
	InsertPublishLog(ctx context.Context, logEntry *domain.PublishLog) error
	ListPublishLogsByPost(ctx context.Context, postID string) ([]domain.PublishLog, error)
	CreateMediaAsset(ctx context.Context, asset *domain.MediaAsset) (*domain.MediaAsset, error)
	GetSystemStats(ctx context.Context) (*domain.SystemStats, error)
}

// PostgresStore implements Store backed by PostgreSQL.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore initializes a PostgreSQL repository.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) UpsertChannel(ctx context.Context, c *domain.Channel) (*domain.Channel, error) {
	c.Platform = domain.NormalizePlatform(c.Platform)
	query := `
	INSERT INTO channels (
		platform, external_id, name, category, encrypted_access_token, token_status, is_active, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	ON CONFLICT (platform, external_id) DO UPDATE SET
		name = EXCLUDED.name,
		category = EXCLUDED.category,
		encrypted_access_token = EXCLUDED.encrypted_access_token,
		token_status = EXCLUDED.token_status,
		is_active = EXCLUDED.is_active,
		updated_at = NOW()
	RETURNING id, created_at::text, updated_at::text;`

	var createdAtStr, updatedAtStr string
	err := s.db.QueryRowContext(
		ctx, query,
		string(c.Platform), c.ExternalID, c.Name, c.Category, c.EncryptedAccessToken, c.TokenStatus, c.IsActive,
	).Scan(&c.ID, &createdAtStr, &updatedAtStr)
	if err != nil {
		return nil, err
	}
	c.CreatedAt = parsePGTime(createdAtStr)
	c.UpdatedAt = parsePGTime(updatedAtStr)
	return c, nil
}

func (s *PostgresStore) ListChannels(ctx context.Context) ([]domain.Channel, error) {
	query := `
	SELECT id, platform, external_id, name, category, encrypted_access_token, token_status,
	       COALESCE(last_checked_at::text, ''),
	       rate_limit_call_count::text, rate_limit_total_time::text, rate_limit_total_cputime::text,
	       is_active::text, created_at::text, updated_at::text
	FROM channels
	ORDER BY created_at DESC;`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []domain.Channel
	for rows.Next() {
		var c domain.Channel
		var platformStr, lastCheckedStr, callStr, timeStr, cpuStr, activeStr, createdStr, updatedStr string
		if err := rows.Scan(
			&c.ID, &platformStr, &c.ExternalID, &c.Name, &c.Category, &c.EncryptedAccessToken, &c.TokenStatus,
			&lastCheckedStr, &callStr, &timeStr, &cpuStr, &activeStr, &createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		c.Platform = domain.NormalizePlatform(domain.PlatformType(platformStr))
		if lastCheckedStr != "" {
			t := parsePGTime(lastCheckedStr)
			c.LastCheckedAt = &t
		}
		c.RateLimitCallCount, _ = strconv.Atoi(callStr)
		c.RateLimitTotalTime, _ = strconv.Atoi(timeStr)
		c.RateLimitTotalCPU, _ = strconv.Atoi(cpuStr)
		c.IsActive = activeStr == "true" || activeStr == "t"
		c.CreatedAt = parsePGTime(createdStr)
		c.UpdatedAt = parsePGTime(updatedAtOrCreated(updatedStr, createdStr))
		channels = append(channels, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func (s *PostgresStore) GetChannelByID(ctx context.Context, id string) (*domain.Channel, error) {
	query := `
	SELECT id, platform, external_id, name, category, encrypted_access_token, token_status,
	       COALESCE(last_checked_at::text, ''),
	       rate_limit_call_count::text, rate_limit_total_time::text, rate_limit_total_cputime::text,
	       is_active::text, created_at::text, updated_at::text
	FROM channels
	WHERE id = $1;`

	var c domain.Channel
	var platformStr, lastCheckedStr, callStr, timeStr, cpuStr, activeStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &platformStr, &c.ExternalID, &c.Name, &c.Category, &c.EncryptedAccessToken, &c.TokenStatus,
		&lastCheckedStr,
		&callStr, &timeStr, &cpuStr, &activeStr, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	c.Platform = domain.NormalizePlatform(domain.PlatformType(platformStr))
	if lastCheckedStr != "" {
		t := parsePGTime(lastCheckedStr)
		c.LastCheckedAt = &t
	}
	c.RateLimitCallCount, _ = strconv.Atoi(callStr)
	c.RateLimitTotalTime, _ = strconv.Atoi(timeStr)
	c.RateLimitTotalCPU, _ = strconv.Atoi(cpuStr)
	c.IsActive = activeStr == "true" || activeStr == "t"
	c.CreatedAt = parsePGTime(createdStr)
	c.UpdatedAt = parsePGTime(updatedStr)
	return &c, nil
}

func (s *PostgresStore) DeleteChannel(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM channels WHERE id = $1", id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return errors.New("channel not found")
	}
	return nil
}

func (s *PostgresStore) UpdateChannelTokenStatus(ctx context.Context, id string, status string, checkedAt time.Time) error {
	_, err := s.db.ExecContext(
		ctx,
		"UPDATE channels SET token_status = $1, last_checked_at = $2, updated_at = NOW() WHERE id = $3",
		status, checkedAt, id,
	)
	return err
}

func (s *PostgresStore) UpdateChannelRateLimit(ctx context.Context, id string, usage domain.RateLimitUsage) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE channels
		 SET rate_limit_call_count = $1, rate_limit_total_time = $2, rate_limit_total_cputime = $3, updated_at = NOW()
		 WHERE id = $4`,
		usage.CallCount, usage.TotalTime, usage.TotalCPUTime, id,
	)
	return err
}

func (s *PostgresStore) CreatePost(ctx context.Context, post *domain.Post) (*domain.Post, error) {
	if post.MediaURLs == nil {
		post.MediaURLs = []string{}
	}
	mediaJSON, err := json.Marshal(post.MediaURLs)
	if err != nil {
		return nil, err
	}
	post.QueueTier = domain.DetermineQueueTier(post.MediaType)
	if post.MaxRetries <= 0 {
		post.MaxRetries = 3
	}

	query := `
	INSERT INTO posts (
		channel_id, content, link_url, media_type, media_urls, status, queue_tier,
		scheduled_at, max_retries, created_by
	) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10)
	RETURNING id, created_at::text, updated_at::text;`

	var createdStr, updatedStr string
	err = s.db.QueryRowContext(
		ctx, query,
		post.ChannelID, post.Content, post.LinkURL, post.MediaType, string(mediaJSON),
		post.Status, post.QueueTier, post.ScheduledAt, post.MaxRetries, post.CreatedBy,
	).Scan(&post.ID, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}
	post.CreatedAt = parsePGTime(createdStr)
	post.UpdatedAt = parsePGTime(updatedStr)
	return post, nil
}

func (s *PostgresStore) GetPostByID(ctx context.Context, id string) (*domain.Post, error) {
	query := `
	SELECT p.id, p.channel_id, COALESCE(c.name, ''), COALESCE(c.platform, 'FACEBOOK'), COALESCE(c.external_id, ''),
	       p.content, p.link_url, p.media_type, p.media_urls::text, p.status, p.queue_tier,
	       p.scheduled_at::text, COALESCE(p.published_at::text, ''), p.external_post_id,
	       p.retry_count::text, p.max_retries::text, COALESCE(p.next_retry_at::text, ''), p.last_error_code::text, p.last_error_message,
	       p.created_by, p.created_at::text, p.updated_at::text
	FROM posts p
	LEFT JOIN channels c ON c.id = p.channel_id
	WHERE p.id = $1;`

	var post domain.Post
	var platformStr, mediaStr, schedStr, pubStr, retryStr, maxRetryStr, nextRetryStr, errCodeStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&post.ID, &post.ChannelID, &post.ChannelName, &platformStr, &post.ExternalChannelID,
		&post.Content, &post.LinkURL, &post.MediaType, &mediaStr, &post.Status, &post.QueueTier,
		&schedStr, &pubStr, &post.ExternalPostID,
		&retryStr, &maxRetryStr, &nextRetryStr, &errCodeStr, &post.LastErrorMessage,
		&post.CreatedBy, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	post.Platform = domain.NormalizePlatform(domain.PlatformType(platformStr))
	_ = json.Unmarshal([]byte(mediaStr), &post.MediaURLs)
	post.ScheduledAt = parsePGTime(schedStr)
	if pubStr != "" {
		t := parsePGTime(pubStr)
		post.PublishedAt = &t
	}
	if nextRetryStr != "" {
		t := parsePGTime(nextRetryStr)
		post.NextRetryAt = &t
	}
	post.RetryCount, _ = strconv.Atoi(retryStr)
	post.MaxRetries, _ = strconv.Atoi(maxRetryStr)
	post.LastErrorCode, _ = strconv.Atoi(errCodeStr)
	post.CreatedAt = parsePGTime(createdStr)
	post.UpdatedAt = parsePGTime(updatedStr)
	return &post, nil
}

func (s *PostgresStore) ListPosts(ctx context.Context, statusFilter string, limit int) ([]domain.Post, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows *sql.Rows
	var err error

	baseQuery := `
	SELECT p.id, p.channel_id, COALESCE(c.name, ''), COALESCE(c.platform, 'FACEBOOK'), COALESCE(c.external_id, ''),
	       p.content, p.link_url, p.media_type, p.media_urls::text, p.status, p.queue_tier,
	       p.scheduled_at::text, COALESCE(p.published_at::text, ''), p.external_post_id,
	       p.retry_count::text, p.max_retries::text, COALESCE(p.next_retry_at::text, ''), p.last_error_code::text, p.last_error_message,
	       p.created_by, p.created_at::text, p.updated_at::text
	FROM posts p
	LEFT JOIN channels c ON c.id = p.channel_id `

	if statusFilter != "" {
		rows, err = s.db.QueryContext(ctx, baseQuery+"WHERE p.status = $1 ORDER BY p.scheduled_at DESC LIMIT $2", statusFilter, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, baseQuery+"ORDER BY p.scheduled_at DESC LIMIT $1", limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Post
	for rows.Next() {
		var post domain.Post
		var platformStr, mediaStr, schedStr, pubStr, retryStr, maxRetryStr, nextRetryStr, errCodeStr, createdStr, updatedStr string
		if err := rows.Scan(
			&post.ID, &post.ChannelID, &post.ChannelName, &platformStr, &post.ExternalChannelID,
			&post.Content, &post.LinkURL, &post.MediaType, &mediaStr, &post.Status, &post.QueueTier,
			&schedStr, &pubStr, &post.ExternalPostID,
			&retryStr, &maxRetryStr, &nextRetryStr, &errCodeStr, &post.LastErrorMessage,
			&post.CreatedBy, &createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		post.Platform = domain.NormalizePlatform(domain.PlatformType(platformStr))
		_ = json.Unmarshal([]byte(mediaStr), &post.MediaURLs)
		post.ScheduledAt = parsePGTime(schedStr)
		if pubStr != "" {
			t := parsePGTime(pubStr)
			post.PublishedAt = &t
		}
		if nextRetryStr != "" {
			t := parsePGTime(nextRetryStr)
			post.NextRetryAt = &t
		}
		post.RetryCount, _ = strconv.Atoi(retryStr)
		post.MaxRetries, _ = strconv.Atoi(maxRetryStr)
		post.LastErrorCode, _ = strconv.Atoi(errCodeStr)
		post.CreatedAt = parsePGTime(createdStr)
		post.UpdatedAt = parsePGTime(updatedStr)
		list = append(list, post)
	}
	return list, nil
}

// ClaimDuePosts uses PostgreSQL's `SELECT ... FOR UPDATE SKIP LOCKED`
// to ensure concurrent Scheduler/Worker instances never claim duplicate posts.
func (s *PostgresStore) ClaimDuePosts(ctx context.Context, now time.Time, batchSize int) ([]domain.Post, error) {
	query := `
	WITH due AS (
		SELECT id
		FROM posts
		WHERE (status = 'SCHEDULED' AND scheduled_at <= $1)
		   OR (status = 'FAILED' AND retry_count < max_retries AND next_retry_at IS NOT NULL AND next_retry_at <= $1)
		ORDER BY scheduled_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	)
	UPDATE posts p
	SET status = 'QUEUED', updated_at = NOW()
	FROM due
	WHERE p.id = due.id
	RETURNING p.id, p.channel_id, p.queue_tier, p.retry_count::text;`

	rows, err := s.db.QueryContext(ctx, query, now, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []domain.Post
	for rows.Next() {
		var p domain.Post
		var retryStr string
		if err := rows.Scan(&p.ID, &p.ChannelID, &p.QueueTier, &retryStr); err != nil {
			return nil, err
		}
		p.RetryCount, _ = strconv.Atoi(retryStr)
		p.Status = domain.PostStatusQueued
		claimed = append(claimed, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *PostgresStore) MarkPostProcessing(ctx context.Context, postID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE posts SET status = 'PROCESSING', updated_at = NOW() WHERE id = $1", postID)
	return err
}

func (s *PostgresStore) MarkPostPublished(ctx context.Context, postID string, externalPostID string, publishedAt time.Time) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE posts
		 SET status = 'PUBLISHED', external_post_id = $1, published_at = $2,
		     last_error_code = 0, last_error_message = '', updated_at = NOW()
		 WHERE id = $3`,
		externalPostID, publishedAt, postID,
	)
	return err
}

func (s *PostgresStore) MarkPostFailedOrRetry(ctx context.Context, postID string, retryCount int, nextRetryAt *time.Time, finalStatus string, errCode int, errMsg string) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE posts
		 SET status = $1, retry_count = $2, next_retry_at = $3,
		     last_error_code = $4, last_error_message = $5, updated_at = NOW()
		 WHERE id = $6`,
		finalStatus, retryCount, nextRetryAt, errCode, errMsg, postID,
	)
	return err
}

func (s *PostgresStore) CancelPost(ctx context.Context, postID string) error {
	res, err := s.db.ExecContext(
		ctx,
		"UPDATE posts SET status = 'CANCELLED', updated_at = NOW() WHERE id = $1 AND status IN ('DRAFT', 'SCHEDULED', 'FAILED')",
		postID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		existing, getErr := s.GetPostByID(ctx, postID)
		if getErr != nil || existing == nil {
			return fmt.Errorf("post %s not found", postID)
		}
		return fmt.Errorf("post %s cannot be cancelled (status: %s)", postID, existing.Status)
	}
	return nil
}

func (s *PostgresStore) InsertPublishLog(ctx context.Context, l *domain.PublishLog) error {
	query := `
	INSERT INTO publish_logs (
		post_id, channel_id, attempt_number, action, http_status,
		error_code, error_subcode, request_summary, response_body, duration_ms
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);`

	_, err := s.db.ExecContext(
		ctx, query,
		l.PostID, l.ChannelID, l.AttemptNumber, l.Action, l.HTTPStatus,
		l.ErrorCode, l.ErrorSubcode, l.RequestSummary, l.ResponseBody, l.DurationMs,
	)
	return err
}

func (s *PostgresStore) ListPublishLogsByPost(ctx context.Context, postID string) ([]domain.PublishLog, error) {
	query := `
	SELECT id, post_id, channel_id, attempt_number::text, action, http_status::text,
	       error_code::text, error_subcode::text, request_summary, response_body,
	       duration_ms::text, created_at::text
	FROM publish_logs
	WHERE post_id = $1
	ORDER BY created_at DESC;`

	rows, err := s.db.QueryContext(ctx, query, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []domain.PublishLog
	for rows.Next() {
		var l domain.PublishLog
		var attStr, httpStr, errStr, subStr, durStr, createdStr string
		if err := rows.Scan(
			&l.ID, &l.PostID, &l.ChannelID, &attStr, &l.Action, &httpStr,
			&errStr, &subStr, &l.RequestSummary, &l.ResponseBody, &durStr, &createdStr,
		); err != nil {
			return nil, err
		}
		l.AttemptNumber, _ = strconv.Atoi(attStr)
		l.HTTPStatus, _ = strconv.Atoi(httpStr)
		l.ErrorCode, _ = strconv.Atoi(errStr)
		l.ErrorSubcode, _ = strconv.Atoi(subStr)
		l.DurationMs, _ = strconv.ParseInt(durStr, 10, 64)
		l.CreatedAt = parsePGTime(createdStr)
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *PostgresStore) CreateMediaAsset(ctx context.Context, asset *domain.MediaAsset) (*domain.MediaAsset, error) {
	query := `
	INSERT INTO media_assets (file_name, storage_key, public_url, mime_type, file_size_bytes, duration_seconds)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id, created_at::text;`

	var createdStr string
	err := s.db.QueryRowContext(
		ctx, query,
		asset.FileName, asset.StorageKey, asset.PublicURL, asset.MimeType, asset.FileSizeBytes, asset.DurationSeconds,
	).Scan(&asset.ID, &createdStr)
	if err != nil {
		return nil, err
	}
	asset.CreatedAt = parsePGTime(createdStr)
	return asset, nil
}

func (s *PostgresStore) GetSystemStats(ctx context.Context) (*domain.SystemStats, error) {
	stats := &domain.SystemStats{}
	var totalChannels, activeChannels, sched, queued, pub, failed string
	query := `
	SELECT
		(SELECT COUNT(*)::text FROM channels),
		(SELECT COUNT(*)::text FROM channels WHERE token_status = 'ACTIVE' AND is_active = TRUE),
		(SELECT COUNT(*)::text FROM posts WHERE status = 'SCHEDULED'),
		(SELECT COUNT(*)::text FROM posts WHERE status IN ('QUEUED', 'PROCESSING')),
		(SELECT COUNT(*)::text FROM posts WHERE status = 'PUBLISHED'),
		(SELECT COUNT(*)::text FROM posts WHERE status = 'FAILED');`

	if err := s.db.QueryRowContext(ctx, query).Scan(&totalChannels, &activeChannels, &sched, &queued, &pub, &failed); err != nil {
		return stats, err
	}
	stats.TotalChannels, _ = strconv.Atoi(totalChannels)
	stats.ActiveChannels, _ = strconv.Atoi(activeChannels)
	stats.ScheduledPosts, _ = strconv.Atoi(sched)
	stats.QueuedPosts, _ = strconv.Atoi(queued)
	stats.PublishedPosts, _ = strconv.Atoi(pub)
	stats.FailedPosts, _ = strconv.Atoi(failed)
	return stats, nil
}

func updatedAtOrCreated(updated, created string) string {
	if updated != "" {
		return updated
	}
	return created
}

func parsePGTime(raw string) time.Time {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07",
		"2006-01-02 15:04:05.999999-07",
		"2006-01-02 15:04:05-07",
		"2006-01-02 15:04:05",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

// ============================================================================
// InMemoryStore: Used for unit testing and offline demo execution
// ============================================================================

type InMemoryStore struct {
	mu       sync.RWMutex
	channels map[string]*domain.Channel
	posts    map[string]*domain.Post
	logs     map[string][]domain.PublishLog
	media    map[string]*domain.MediaAsset
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		channels: make(map[string]*domain.Channel),
		posts:    make(map[string]*domain.Post),
		logs:     make(map[string][]domain.PublishLog),
		media:    make(map[string]*domain.MediaAsset),
	}
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

func (m *InMemoryStore) UpsertChannel(ctx context.Context, channel *domain.Channel) (*domain.Channel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	channel.Platform = domain.NormalizePlatform(channel.Platform)

	for _, existing := range m.channels {
		if existing.Platform == channel.Platform && existing.ExternalID == channel.ExternalID {
			existing.Name = channel.Name
			existing.Category = channel.Category
			existing.EncryptedAccessToken = channel.EncryptedAccessToken
			existing.TokenStatus = channel.TokenStatus
			existing.IsActive = channel.IsActive
			existing.UpdatedAt = time.Now().UTC()
			cp := *existing
			return &cp, nil
		}
	}

	if channel.ID == "" {
		channel.ID = newUUID()
	}
	now := time.Now().UTC()
	channel.CreatedAt = now
	channel.UpdatedAt = now
	cp := *channel
	m.channels[cp.ID] = &cp
	return &cp, nil
}

func (m *InMemoryStore) ListChannels(ctx context.Context) ([]domain.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]domain.Channel, 0, len(m.channels))
	for _, c := range m.channels {
		out = append(out, *c)
	}
	return out, nil
}

func (m *InMemoryStore) GetChannelByID(ctx context.Context, id string) (*domain.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, ok := m.channels[id]
	if !ok {
		return nil, errors.New("channel not found")
	}
	cp := *c
	return &cp, nil
}

func (m *InMemoryStore) DeleteChannel(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.channels[id]; !ok {
		return errors.New("channel not found")
	}
	delete(m.channels, id)

	for postID, post := range m.posts {
		if post.ChannelID == id {
			delete(m.posts, postID)
		}
	}
	return nil
}

func (m *InMemoryStore) UpdateChannelTokenStatus(ctx context.Context, id string, status string, checkedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.channels[id]
	if !ok {
		return errors.New("channel not found")
	}
	c.TokenStatus = status
	c.LastCheckedAt = &checkedAt
	c.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *InMemoryStore) UpdateChannelRateLimit(ctx context.Context, id string, usage domain.RateLimitUsage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.channels[id]
	if !ok {
		return errors.New("channel not found")
	}
	c.RateLimitCallCount = usage.CallCount
	c.RateLimitTotalTime = usage.TotalTime
	c.RateLimitTotalCPU = usage.TotalCPUTime
	c.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *InMemoryStore) CreatePost(ctx context.Context, post *domain.Post) (*domain.Post, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if post.ID == "" {
		post.ID = newUUID()
	}
	post.QueueTier = domain.DetermineQueueTier(post.MediaType)
	if post.MaxRetries <= 0 {
		post.MaxRetries = 3
	}
	if ch, ok := m.channels[post.ChannelID]; ok {
		post.ChannelName = ch.Name
		post.Platform = ch.Platform
		post.ExternalChannelID = ch.ExternalID
	}
	now := time.Now().UTC()
	post.CreatedAt = now
	post.UpdatedAt = now
	cp := *post
	m.posts[cp.ID] = &cp
	return &cp, nil
}

func (m *InMemoryStore) GetPostByID(ctx context.Context, id string) (*domain.Post, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.posts[id]
	if !ok {
		return nil, errors.New("post not found")
	}
	cp := *p
	if ch, exists := m.channels[cp.ChannelID]; exists {
		cp.ChannelName = ch.Name
		cp.Platform = ch.Platform
		cp.ExternalChannelID = ch.ExternalID
	}
	return &cp, nil
}

func (m *InMemoryStore) ListPosts(ctx context.Context, statusFilter string, limit int) ([]domain.Post, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []domain.Post
	for _, p := range m.posts {
		if statusFilter != "" && p.Status != statusFilter {
			continue
		}
		cp := *p
		if ch, exists := m.channels[cp.ChannelID]; exists {
			cp.ChannelName = ch.Name
			cp.Platform = ch.Platform
			cp.ExternalChannelID = ch.ExternalID
		}
		list = append(list, cp)
	}
	return list, nil
}

func (m *InMemoryStore) ClaimDuePosts(ctx context.Context, now time.Time, batchSize int) ([]domain.Post, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var claimed []domain.Post
	for _, p := range m.posts {
		if len(claimed) >= batchSize {
			break
		}
		isDueScheduled := p.Status == domain.PostStatusScheduled && !p.ScheduledAt.After(now)
		isDueRetry := p.Status == domain.PostStatusFailed && p.RetryCount < p.MaxRetries && p.NextRetryAt != nil && !p.NextRetryAt.After(now)
		if isDueScheduled || isDueRetry {
			p.Status = domain.PostStatusQueued
			p.UpdatedAt = now
			claimed = append(claimed, *p)
		}
	}
	return claimed, nil
}

func (m *InMemoryStore) MarkPostProcessing(ctx context.Context, postID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.posts[postID]
	if !ok {
		return errors.New("post not found")
	}
	p.Status = domain.PostStatusProcessing
	p.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *InMemoryStore) MarkPostPublished(ctx context.Context, postID string, externalPostID string, publishedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.posts[postID]
	if !ok {
		return errors.New("post not found")
	}
	p.Status = domain.PostStatusPublished
	p.ExternalPostID = externalPostID
	p.PublishedAt = &publishedAt
	p.LastErrorCode = 0
	p.LastErrorMessage = ""
	p.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *InMemoryStore) MarkPostFailedOrRetry(ctx context.Context, postID string, retryCount int, nextRetryAt *time.Time, finalStatus string, errCode int, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.posts[postID]
	if !ok {
		return errors.New("post not found")
	}
	p.Status = finalStatus
	p.RetryCount = retryCount
	p.NextRetryAt = nextRetryAt
	p.LastErrorCode = errCode
	p.LastErrorMessage = errMsg
	p.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *InMemoryStore) CancelPost(ctx context.Context, postID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.posts[postID]
	if !ok {
		return nil
	}
	if p.CanCancel() {
		p.Status = domain.PostStatusCancelled
		p.UpdatedAt = time.Now().UTC()
	}
	return nil
}

func (m *InMemoryStore) InsertPublishLog(ctx context.Context, l *domain.PublishLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if l.ID == "" {
		l.ID = newUUID()
	}
	l.CreatedAt = time.Now().UTC()
	m.logs[l.PostID] = append(m.logs[l.PostID], *l)
	return nil
}

func (m *InMemoryStore) ListPublishLogsByPost(ctx context.Context, postID string) ([]domain.PublishLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return append([]domain.PublishLog(nil), m.logs[postID]...), nil
}

func (m *InMemoryStore) CreateMediaAsset(ctx context.Context, asset *domain.MediaAsset) (*domain.MediaAsset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if asset.ID == "" {
		asset.ID = newUUID()
	}
	asset.CreatedAt = time.Now().UTC()
	cp := *asset
	m.media[cp.ID] = &cp
	return &cp, nil
}

func (m *InMemoryStore) GetSystemStats(ctx context.Context) (*domain.SystemStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &domain.SystemStats{
		TotalChannels: len(m.channels),
	}
	for _, c := range m.channels {
		if c.IsActive && c.TokenStatus == domain.TokenStatusActive {
			stats.ActiveChannels++
		}
	}
	for _, post := range m.posts {
		switch post.Status {
		case domain.PostStatusScheduled:
			stats.ScheduledPosts++
		case domain.PostStatusQueued, domain.PostStatusProcessing:
			stats.QueuedPosts++
		case domain.PostStatusPublished:
			stats.PublishedPosts++
		case domain.PostStatusFailed:
			stats.FailedPosts++
		}
	}
	return stats, nil
}
