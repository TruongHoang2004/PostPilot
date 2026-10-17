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

	"github.com/happy-darwin/fb-autoposter/internal/domain"
)

// Store định nghĩa interface lưu trữ dữ liệu cho hệ thống.
type Store interface {
	// Quản lý Fanpage
	UpsertPage(ctx context.Context, page *domain.FacebookPage) (*domain.FacebookPage, error)
	ListPages(ctx context.Context) ([]domain.FacebookPage, error)
	GetPageByUUID(ctx context.Context, uuid string) (*domain.FacebookPage, error)
	UpdatePageTokenStatus(ctx context.Context, uuid string, status string, checkedAt time.Time) error
	UpdatePageRateLimit(ctx context.Context, uuid string, usage domain.RateLimitUsage) error

	// Quản lý Bài viết & Lập lịch
	CreatePost(ctx context.Context, post *domain.Post) (*domain.Post, error)
	GetPostByID(ctx context.Context, id string) (*domain.Post, error)
	ListPosts(ctx context.Context, statusFilter string, limit int) ([]domain.Post, error)
	ClaimDuePosts(ctx context.Context, now time.Time, batchSize int) ([]domain.Post, error)
	MarkPostProcessing(ctx context.Context, postID string) error
	MarkPostPublished(ctx context.Context, postID string, fbPostID string, publishedAt time.Time) error
	MarkPostFailedOrRetry(ctx context.Context, postID string, retryCount int, nextRetryAt *time.Time, finalStatus string, errCode int, errMsg string) error
	CancelPost(ctx context.Context, postID string) error

	// Quản lý Audit Log & Media
	InsertPublishLog(ctx context.Context, logEntry *domain.PublishLog) error
	ListPublishLogsByPost(ctx context.Context, postID string) ([]domain.PublishLog, error)
	CreateMediaAsset(ctx context.Context, asset *domain.MediaAsset) (*domain.MediaAsset, error)
	GetSystemStats(ctx context.Context) (*domain.SystemStats, error)
}

// PostgresStore triển khai Store sử dụng PostgreSQL.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore khởi tạo repository kết nối tới PostgreSQL.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) UpsertPage(ctx context.Context, p *domain.FacebookPage) (*domain.FacebookPage, error) {
	query := `
	INSERT INTO facebook_pages (
		page_id, page_name, category, encrypted_access_token, token_status, is_active, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, NOW())
	ON CONFLICT (page_id) DO UPDATE SET
		page_name = EXCLUDED.page_name,
		category = EXCLUDED.category,
		encrypted_access_token = EXCLUDED.encrypted_access_token,
		token_status = EXCLUDED.token_status,
		is_active = EXCLUDED.is_active,
		updated_at = NOW()
	RETURNING id, created_at::text, updated_at::text;`

	var createdAtStr, updatedAtStr string
	err := s.db.QueryRowContext(
		ctx, query,
		p.PageID, p.PageName, p.Category, p.EncryptedAccessToken, p.TokenStatus, p.IsActive,
	).Scan(&p.ID, &createdAtStr, &updatedAtStr)
	if err != nil {
		return nil, err
	}
	p.CreatedAt = parsePGTime(createdAtStr)
	p.UpdatedAt = parsePGTime(updatedAtStr)
	return p, nil
}

func (s *PostgresStore) ListPages(ctx context.Context) ([]domain.FacebookPage, error) {
	query := `
	SELECT id, page_id, page_name, category, encrypted_access_token, token_status,
	       COALESCE(last_checked_at::text, ''),
	       rate_limit_call_count::text, rate_limit_total_time::text, rate_limit_total_cputime::text,
	       is_active::text, created_at::text, updated_at::text
	FROM facebook_pages
	ORDER BY created_at DESC;`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []domain.FacebookPage
	for rows.Next() {
		var p domain.FacebookPage
		var lastCheckedStr, callStr, timeStr, cpuStr, activeStr, createdStr, updatedStr string
		if err := rows.Scan(
			&p.ID, &p.PageID, &p.PageName, &p.Category, &p.EncryptedAccessToken, &p.TokenStatus,
			&lastCheckedStr, &callStr, &timeStr, &cpuStr, &activeStr, &createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		if lastCheckedStr != "" {
			t := parsePGTime(lastCheckedStr)
			p.LastCheckedAt = &t
		}
		p.RateLimitCallCount, _ = strconv.Atoi(callStr)
		p.RateLimitTotalTime, _ = strconv.Atoi(timeStr)
		p.RateLimitTotalCPU, _ = strconv.Atoi(cpuStr)
		p.IsActive = activeStr == "true" || activeStr == "t"
		p.CreatedAt = parsePGTime(createdStr)
		p.UpdatedAt = parsePGTime(updatedStr)
		pages = append(pages, p)
	}
	return pages, nil
}

func (s *PostgresStore) GetPageByUUID(ctx context.Context, uuid string) (*domain.FacebookPage, error) {
	query := `
	SELECT id, page_id, page_name, category, encrypted_access_token, token_status,
	       rate_limit_call_count::text, rate_limit_total_time::text, rate_limit_total_cputime::text,
	       is_active::text, created_at::text, updated_at::text
	FROM facebook_pages
	WHERE id = $1;`

	var p domain.FacebookPage
	var callStr, timeStr, cpuStr, activeStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, uuid).Scan(
		&p.ID, &p.PageID, &p.PageName, &p.Category, &p.EncryptedAccessToken, &p.TokenStatus,
		&callStr, &timeStr, &cpuStr, &activeStr, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	p.RateLimitCallCount, _ = strconv.Atoi(callStr)
	p.RateLimitTotalTime, _ = strconv.Atoi(timeStr)
	p.RateLimitTotalCPU, _ = strconv.Atoi(cpuStr)
	p.IsActive = activeStr == "true" || activeStr == "t"
	p.CreatedAt = parsePGTime(createdStr)
	p.UpdatedAt = parsePGTime(updatedStr)
	return &p, nil
}

func (s *PostgresStore) UpdatePageTokenStatus(ctx context.Context, uuid string, status string, checkedAt time.Time) error {
	_, err := s.db.ExecContext(
		ctx,
		"UPDATE facebook_pages SET token_status = $1, last_checked_at = $2, updated_at = NOW() WHERE id = $3",
		status, checkedAt, uuid,
	)
	return err
}

func (s *PostgresStore) UpdatePageRateLimit(ctx context.Context, uuid string, usage domain.RateLimitUsage) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE facebook_pages
		 SET rate_limit_call_count = $1, rate_limit_total_time = $2, rate_limit_total_cputime = $3, updated_at = NOW()
		 WHERE id = $4`,
		usage.CallCount, usage.TotalTime, usage.TotalCPUTime, uuid,
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
		page_uuid, content, link_url, media_type, media_urls, status, queue_tier,
		scheduled_at, max_retries, created_by
	) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10)
	RETURNING id, created_at::text, updated_at::text;`

	var createdStr, updatedStr string
	err = s.db.QueryRowContext(
		ctx, query,
		post.PageUUID, post.Content, post.LinkURL, post.MediaType, string(mediaJSON),
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
	SELECT p.id, p.page_uuid, COALESCE(fp.page_name, ''), COALESCE(fp.page_id, ''),
	       p.content, p.link_url, p.media_type, p.media_urls::text, p.status, p.queue_tier,
	       p.scheduled_at::text, COALESCE(p.published_at::text, ''), p.fb_post_id,
	       p.retry_count::text, p.max_retries::text, p.last_error_code::text, p.last_error_message,
	       p.created_by, p.created_at::text, p.updated_at::text
	FROM posts p
	LEFT JOIN facebook_pages fp ON fp.id = p.page_uuid
	WHERE p.id = $1;`

	var post domain.Post
	var mediaStr, schedStr, pubStr, retryStr, maxRetryStr, errCodeStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&post.ID, &post.PageUUID, &post.PageName, &post.FBPageID,
		&post.Content, &post.LinkURL, &post.MediaType, &mediaStr, &post.Status, &post.QueueTier,
		&schedStr, &pubStr, &post.FBPostID,
		&retryStr, &maxRetryStr, &errCodeStr, &post.LastErrorMessage,
		&post.CreatedBy, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(mediaStr), &post.MediaURLs)
	post.ScheduledAt = parsePGTime(schedStr)
	if pubStr != "" {
		t := parsePGTime(pubStr)
		post.PublishedAt = &t
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
	SELECT p.id, p.page_uuid, COALESCE(fp.page_name, ''), COALESCE(fp.page_id, ''),
	       p.content, p.link_url, p.media_type, p.media_urls::text, p.status, p.queue_tier,
	       p.scheduled_at::text, COALESCE(p.published_at::text, ''), p.fb_post_id,
	       p.retry_count::text, p.max_retries::text, p.last_error_code::text, p.last_error_message,
	       p.created_by, p.created_at::text, p.updated_at::text
	FROM posts p
	LEFT JOIN facebook_pages fp ON fp.id = p.page_uuid `

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
		var mediaStr, schedStr, pubStr, retryStr, maxRetryStr, errCodeStr, createdStr, updatedStr string
		if err := rows.Scan(
			&post.ID, &post.PageUUID, &post.PageName, &post.FBPageID,
			&post.Content, &post.LinkURL, &post.MediaType, &mediaStr, &post.Status, &post.QueueTier,
			&schedStr, &pubStr, &post.FBPostID,
			&retryStr, &maxRetryStr, &errCodeStr, &post.LastErrorMessage,
			&post.CreatedBy, &createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(mediaStr), &post.MediaURLs)
		post.ScheduledAt = parsePGTime(schedStr)
		if pubStr != "" {
			t := parsePGTime(pubStr)
			post.PublishedAt = &t
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

// ClaimDuePosts sử dụng cơ chế `SELECT ... FOR UPDATE SKIP LOCKED` của PostgreSQL
// để đảm bảo khi chạy nhiều instance Scheduler/Worker song song không bao giờ lấy trùng 1 bài đăng.
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
	RETURNING p.id, p.page_uuid, p.queue_tier, p.retry_count::text;`

	rows, err := s.db.QueryContext(ctx, query, now, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []domain.Post
	for rows.Next() {
		var p domain.Post
		var retryStr string
		if err := rows.Scan(&p.ID, &p.PageUUID, &p.QueueTier, &retryStr); err != nil {
			return nil, err
		}
		p.RetryCount, _ = strconv.Atoi(retryStr)
		p.Status = domain.PostStatusQueued
		claimed = append(claimed, p)
	}
	return claimed, nil
}

func (s *PostgresStore) MarkPostProcessing(ctx context.Context, postID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE posts SET status = 'PROCESSING', updated_at = NOW() WHERE id = $1", postID)
	return err
}

func (s *PostgresStore) MarkPostPublished(ctx context.Context, postID string, fbPostID string, publishedAt time.Time) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE posts
		 SET status = 'PUBLISHED', fb_post_id = $1, published_at = $2,
		     last_error_code = 0, last_error_message = '', updated_at = NOW()
		 WHERE id = $3`,
		fbPostID, publishedAt, postID,
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
	_, err := s.db.ExecContext(
		ctx,
		"UPDATE posts SET status = 'CANCELLED', updated_at = NOW() WHERE id = $1 AND status IN ('DRAFT', 'SCHEDULED', 'FAILED')",
		postID,
	)
	return err
}

func (s *PostgresStore) InsertPublishLog(ctx context.Context, l *domain.PublishLog) error {
	query := `
	INSERT INTO publish_logs (
		post_id, page_uuid, attempt_number, action, http_status,
		fb_error_code, fb_error_subcode, request_summary, response_body, duration_ms
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);`

	_, err := s.db.ExecContext(
		ctx, query,
		l.PostID, l.PageUUID, l.AttemptNumber, l.Action, l.HTTPStatus,
		l.FBErrorCode, l.FBErrorSubcode, l.RequestSummary, l.ResponseBody, l.DurationMs,
	)
	return err
}

func (s *PostgresStore) ListPublishLogsByPost(ctx context.Context, postID string) ([]domain.PublishLog, error) {
	query := `
	SELECT id, post_id, page_uuid, attempt_number::text, action, http_status::text,
	       fb_error_code::text, fb_error_subcode::text, request_summary, response_body,
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
			&l.ID, &l.PostID, &l.PageUUID, &attStr, &l.Action, &httpStr,
			&errStr, &subStr, &l.RequestSummary, &l.ResponseBody, &durStr, &createdStr,
		); err != nil {
			return nil, err
		}
		l.AttemptNumber, _ = strconv.Atoi(attStr)
		l.HTTPStatus, _ = strconv.Atoi(httpStr)
		l.FBErrorCode, _ = strconv.Atoi(errStr)
		l.FBErrorSubcode, _ = strconv.Atoi(subStr)
		l.DurationMs, _ = strconv.ParseInt(durStr, 10, 64)
		l.CreatedAt = parsePGTime(createdStr)
		logs = append(logs, l)
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
	var totalPages, activePages, sched, queued, pub, failed string
	query := `
	SELECT
		(SELECT COUNT(*)::text FROM facebook_pages),
		(SELECT COUNT(*)::text FROM facebook_pages WHERE token_status = 'ACTIVE' AND is_active = TRUE),
		(SELECT COUNT(*)::text FROM posts WHERE status = 'SCHEDULED'),
		(SELECT COUNT(*)::text FROM posts WHERE status IN ('QUEUED', 'PROCESSING')),
		(SELECT COUNT(*)::text FROM posts WHERE status = 'PUBLISHED'),
		(SELECT COUNT(*)::text FROM posts WHERE status = 'FAILED');`

	if err := s.db.QueryRowContext(ctx, query).Scan(&totalPages, &activePages, &sched, &queued, &pub, &failed); err != nil {
		return stats, err
	}
	stats.TotalPages, _ = strconv.Atoi(totalPages)
	stats.ActivePages, _ = strconv.Atoi(activePages)
	stats.ScheduledPosts, _ = strconv.Atoi(sched)
	stats.QueuedPosts, _ = strconv.Atoi(queued)
	stats.PublishedPosts, _ = strconv.Atoi(pub)
	stats.FailedPosts, _ = strconv.Atoi(failed)
	return stats, nil
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
// InMemoryStore: Dùng cho Unit Test hoặc chạy Demo độc lập khi chưa bật Docker
// ============================================================================

type InMemoryStore struct {
	mu    sync.RWMutex
	pages map[string]*domain.FacebookPage
	posts map[string]*domain.Post
	logs  map[string][]domain.PublishLog
	media map[string]*domain.MediaAsset
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		pages: make(map[string]*domain.FacebookPage),
		posts: make(map[string]*domain.Post),
		logs:  make(map[string][]domain.PublishLog),
		media: make(map[string]*domain.MediaAsset),
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

func (m *InMemoryStore) UpsertPage(ctx context.Context, page *domain.FacebookPage) (*domain.FacebookPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, existing := range m.pages {
		if existing.PageID == page.PageID {
			existing.PageName = page.PageName
			existing.Category = page.Category
			existing.EncryptedAccessToken = page.EncryptedAccessToken
			existing.TokenStatus = page.TokenStatus
			existing.IsActive = page.IsActive
			existing.UpdatedAt = time.Now().UTC()
			cp := *existing
			return &cp, nil
		}
	}

	if page.ID == "" {
		page.ID = newUUID()
	}
	now := time.Now().UTC()
	page.CreatedAt = now
	page.UpdatedAt = now
	cp := *page
	m.pages[cp.ID] = &cp
	return &cp, nil
}

func (m *InMemoryStore) ListPages(ctx context.Context) ([]domain.FacebookPage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]domain.FacebookPage, 0, len(m.pages))
	for _, p := range m.pages {
		out = append(out, *p)
	}
	return out, nil
}

func (m *InMemoryStore) GetPageByUUID(ctx context.Context, uuid string) (*domain.FacebookPage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.pages[uuid]
	if !ok {
		return nil, errors.New("page not found")
	}
	cp := *p
	return &cp, nil
}

func (m *InMemoryStore) UpdatePageTokenStatus(ctx context.Context, uuid string, status string, checkedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.pages[uuid]
	if !ok {
		return errors.New("page not found")
	}
	p.TokenStatus = status
	p.LastCheckedAt = &checkedAt
	p.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *InMemoryStore) UpdatePageRateLimit(ctx context.Context, uuid string, usage domain.RateLimitUsage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.pages[uuid]
	if !ok {
		return errors.New("page not found")
	}
	p.RateLimitCallCount = usage.CallCount
	p.RateLimitTotalTime = usage.TotalTime
	p.RateLimitTotalCPU = usage.TotalCPUTime
	p.UpdatedAt = time.Now().UTC()
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
	if p, ok := m.pages[post.PageUUID]; ok {
		post.PageName = p.PageName
		post.FBPageID = p.PageID
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
	if page, exists := m.pages[cp.PageUUID]; exists {
		cp.PageName = page.PageName
		cp.FBPageID = page.PageID
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
		if page, exists := m.pages[cp.PageUUID]; exists {
			cp.PageName = page.PageName
			cp.FBPageID = page.PageID
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

func (m *InMemoryStore) MarkPostPublished(ctx context.Context, postID string, fbPostID string, publishedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.posts[postID]
	if !ok {
		return errors.New("post not found")
	}
	p.Status = domain.PostStatusPublished
	p.FBPostID = fbPostID
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
		return errors.New("post not found")
	}
	p.Status = domain.PostStatusCancelled
	p.UpdatedAt = time.Now().UTC()
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
		TotalPages: len(m.pages),
	}
	for _, p := range m.pages {
		if p.IsActive && p.TokenStatus == domain.TokenStatusActive {
			stats.ActivePages++
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
