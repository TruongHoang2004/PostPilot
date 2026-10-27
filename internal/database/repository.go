package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// WithTenantContext attaches a TenantContext to the given context.
func WithTenantContext(ctx context.Context, tc *domain.TenantContext) context.Context {
	return domain.WithTenantContext(ctx, tc)
}

// TenantFromContext retrieves the TenantContext from ctx, defaulting to DefaultTenantContext if absent.
func TenantFromContext(ctx context.Context) *domain.TenantContext {
	return domain.TenantFromContext(ctx)
}

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

	// Users
	CreateUser(ctx context.Context, u *domain.User) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByID(ctx context.Context, id string) (*domain.User, error)

	// Organizations & Members
	CreateOrganization(ctx context.Context, org *domain.Organization, ownerUserID string) (*domain.Organization, error)
	GetOrganizationByID(ctx context.Context, id string) (*domain.Organization, error)
	ListUserOrganizations(ctx context.Context, userID string) ([]domain.Organization, error)
	ListOrganizationMembers(ctx context.Context, orgID string) ([]domain.OrganizationMember, error)
	AddOrganizationMember(ctx context.Context, m *domain.OrganizationMember) (*domain.OrganizationMember, error)
	UpdateOrganizationMemberRole(ctx context.Context, orgID string, memberID string, newRole domain.OrgRole) error
	RemoveOrganizationMember(ctx context.Context, orgID string, memberID string) error
	CreateInvitation(ctx context.Context, inv *domain.OrganizationInvitation) (*domain.OrganizationInvitation, error)
	GetInvitationByToken(ctx context.Context, token string) (*domain.OrganizationInvitation, error)
	AcceptInvitation(ctx context.Context, token string, userID string) (*domain.OrganizationMember, error)

	// Channel Groups & Permissions
	CreateChannelGroup(ctx context.Context, g *domain.ChannelGroup) (*domain.ChannelGroup, error)
	ListChannelGroups(ctx context.Context, orgID string) ([]domain.ChannelGroup, error)
	GetChannelGroupByID(ctx context.Context, orgID string, groupID string) (*domain.ChannelGroup, error)
	DeleteChannelGroup(ctx context.Context, orgID string, groupID string) error
	SetChannelGroupChannels(ctx context.Context, orgID string, groupID string, channelIDs []string) error
	SetChannelGroupPermission(ctx context.Context, p *domain.ChannelGroupPermission) (*domain.ChannelGroupPermission, error)
	DeleteChannelGroupPermission(ctx context.Context, orgID string, groupID string, memberID string) error
	ListChannelGroupPermissions(ctx context.Context, orgID string, groupID string) ([]domain.ChannelGroupPermission, error)
	GetUserChannelRoles(ctx context.Context, orgID string, userID string, channelID string) ([]domain.GroupRole, error)

	// Post Workflow & Approvals
	ReviewPost(ctx context.Context, postID string, reviewerID string, action domain.ApprovalAction, feedbackNote string) (*domain.Post, error)
	ResubmitPost(ctx context.Context, postID string, content string, scheduledAt time.Time) (*domain.Post, error)
	ListPostApprovals(ctx context.Context, postID string) ([]domain.PostApproval, error)

	// Audit Logs & Media Assets
	InsertAuditLog(ctx context.Context, log *domain.AuditLog) error
	ListAuditLogs(ctx context.Context, orgID string, limit int) ([]domain.AuditLog, error)
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
	if c.OrganizationID == "" {
		c.OrganizationID = TenantFromContext(ctx).OrganizationID
	}
	if c.OrganizationID == "" {
		c.OrganizationID = domain.DefaultOrganizationID
	}

	query := `
	INSERT INTO channels (
		organization_id, platform, external_id, name, category, encrypted_access_token, token_status, is_active, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	ON CONFLICT (organization_id, platform, external_id) DO UPDATE SET
		name = EXCLUDED.name,
		category = EXCLUDED.category,
		encrypted_access_token = EXCLUDED.encrypted_access_token,
		token_status = EXCLUDED.token_status,
		is_active = EXCLUDED.is_active,
		updated_at = NOW()
	RETURNING id, organization_id::text, created_at::text, updated_at::text;`

	var orgIDStr, createdAtStr, updatedAtStr string
	err := s.db.QueryRowContext(
		ctx, query,
		c.OrganizationID, string(c.Platform), c.ExternalID, c.Name, c.Category, c.EncryptedAccessToken, c.TokenStatus, c.IsActive,
	).Scan(&c.ID, &orgIDStr, &createdAtStr, &updatedAtStr)
	if err != nil {
		return nil, err
	}
	c.OrganizationID = orgIDStr
	c.CreatedAt = parsePGTime(createdAtStr)
	c.UpdatedAt = parsePGTime(updatedAtStr)
	return c, nil
}

func (s *PostgresStore) ListChannels(ctx context.Context) ([]domain.Channel, error) {
	orgID := TenantFromContext(ctx).OrganizationID

	query := `
	SELECT id, organization_id::text, platform, external_id, name, category, encrypted_access_token, token_status,
	       COALESCE(last_checked_at::text, ''),
	       rate_limit_call_count::text, rate_limit_total_time::text, rate_limit_total_cputime::text,
	       is_active::text, created_at::text, updated_at::text
	FROM channels
	WHERE organization_id = $1
	ORDER BY created_at DESC;`

	rows, err := s.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []domain.Channel
	for rows.Next() {
		var c domain.Channel
		var orgIDStr, platformStr, lastCheckedStr, callStr, timeStr, cpuStr, activeStr, createdStr, updatedStr string
		if err := rows.Scan(
			&c.ID, &orgIDStr, &platformStr, &c.ExternalID, &c.Name, &c.Category, &c.EncryptedAccessToken, &c.TokenStatus,
			&lastCheckedStr, &callStr, &timeStr, &cpuStr, &activeStr, &createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		c.OrganizationID = orgIDStr
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
	SELECT id, organization_id::text, platform, external_id, name, category, encrypted_access_token, token_status,
	       COALESCE(last_checked_at::text, ''),
	       rate_limit_call_count::text, rate_limit_total_time::text, rate_limit_total_cputime::text,
	       is_active::text, created_at::text, updated_at::text
	FROM channels
	WHERE id = $1;`

	var c domain.Channel
	var orgIDStr, platformStr, lastCheckedStr, callStr, timeStr, cpuStr, activeStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &orgIDStr, &platformStr, &c.ExternalID, &c.Name, &c.Category, &c.EncryptedAccessToken, &c.TokenStatus,
		&lastCheckedStr,
		&callStr, &timeStr, &cpuStr, &activeStr, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	c.OrganizationID = orgIDStr
	tc := TenantFromContext(ctx)
	if tc != nil && tc.OrganizationID != "" && c.OrganizationID != tc.OrganizationID {
		return nil, errors.New("channel not found")
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
	tc := TenantFromContext(ctx)
	var res sql.Result
	var err error
	if tc != nil && tc.OrganizationID != "" {
		res, err = s.db.ExecContext(ctx, "DELETE FROM channels WHERE id = $1 AND organization_id = $2", id, tc.OrganizationID)
	} else {
		res, err = s.db.ExecContext(ctx, "DELETE FROM channels WHERE id = $1", id)
	}
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
	tc := TenantFromContext(ctx)
	if post.OrganizationID == "" {
		post.OrganizationID = tc.OrganizationID
	}
	if post.OrganizationID == "" {
		post.OrganizationID = domain.DefaultOrganizationID
	}
	if post.CreatedBy == "" {
		post.CreatedBy = tc.UserID
	}

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
		organization_id, channel_id, channel_group_id, batch_id, content, link_url, media_type, media_urls,
		status, queue_tier, scheduled_at, max_retries, created_by, submitted_by
	) VALUES (
		$1, $2, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, $5, $6, $7, $8::jsonb,
		$9, $10, $11, $12, $13, NULLIF($14, '')::uuid
	)
	RETURNING id, created_at::text, updated_at::text;`

	var createdStr, updatedStr string
	err = s.db.QueryRowContext(
		ctx, query,
		post.OrganizationID, post.ChannelID, post.ChannelGroupID, post.BatchID,
		post.Content, post.LinkURL, post.MediaType, string(mediaJSON),
		post.Status, post.QueueTier, post.ScheduledAt, post.MaxRetries, post.CreatedBy, post.SubmittedBy,
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
	SELECT p.id, p.organization_id::text, p.channel_id, COALESCE(c.name, ''), COALESCE(c.platform, 'FACEBOOK'), COALESCE(c.external_id, ''),
	       COALESCE(p.channel_group_id::text, ''), COALESCE(p.batch_id::text, ''),
	       p.content, p.link_url, p.media_type, p.media_urls::text, p.status, p.queue_tier,
	       p.scheduled_at::text, COALESCE(p.published_at::text, ''), p.external_post_id,
	       COALESCE(p.submitted_by::text, ''), COALESCE(p.reviewed_by::text, ''), COALESCE(p.reviewed_at::text, ''), p.rejection_note,
	       p.retry_count::text, p.max_retries::text, COALESCE(p.next_retry_at::text, ''), p.last_error_code::text, p.last_error_message,
	       p.created_by, p.created_at::text, p.updated_at::text
	FROM posts p
	LEFT JOIN channels c ON c.id = p.channel_id
	WHERE p.id = $1;`

	var post domain.Post
	var orgIDStr, platformStr, groupIDStr, batchIDStr, mediaStr, schedStr, pubStr string
	var subStr, revStr, revAtStr, retryStr, maxRetryStr, nextRetryStr, errCodeStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&post.ID, &orgIDStr, &post.ChannelID, &post.ChannelName, &platformStr, &post.ExternalChannelID,
		&groupIDStr, &batchIDStr,
		&post.Content, &post.LinkURL, &post.MediaType, &mediaStr, &post.Status, &post.QueueTier,
		&schedStr, &pubStr, &post.ExternalPostID,
		&subStr, &revStr, &revAtStr, &post.RejectionNote,
		&retryStr, &maxRetryStr, &nextRetryStr, &errCodeStr, &post.LastErrorMessage,
		&post.CreatedBy, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	post.OrganizationID = orgIDStr
	tc := TenantFromContext(ctx)
	if tc != nil && tc.OrganizationID != "" && post.OrganizationID != tc.OrganizationID {
		return nil, errors.New("post not found")
	}

	post.ChannelGroupID = groupIDStr
	post.BatchID = batchIDStr
	post.SubmittedBy = subStr
	post.ReviewedBy = revStr
	if revAtStr != "" {
		t := parsePGTime(revAtStr)
		post.ReviewedAt = &t
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
	orgID := TenantFromContext(ctx).OrganizationID
	var rows *sql.Rows
	var err error

	baseQuery := `
	SELECT p.id, p.organization_id::text, p.channel_id, COALESCE(c.name, ''), COALESCE(c.platform, 'FACEBOOK'), COALESCE(c.external_id, ''),
	       COALESCE(p.channel_group_id::text, ''), COALESCE(p.batch_id::text, ''),
	       p.content, p.link_url, p.media_type, p.media_urls::text, p.status, p.queue_tier,
	       p.scheduled_at::text, COALESCE(p.published_at::text, ''), p.external_post_id,
	       COALESCE(p.submitted_by::text, ''), COALESCE(p.reviewed_by::text, ''), COALESCE(p.reviewed_at::text, ''), p.rejection_note,
	       p.retry_count::text, p.max_retries::text, COALESCE(p.next_retry_at::text, ''), p.last_error_code::text, p.last_error_message,
	       p.created_by, p.created_at::text, p.updated_at::text
	FROM posts p
	LEFT JOIN channels c ON c.id = p.channel_id `

	if statusFilter != "" {
		query := baseQuery + `WHERE p.organization_id = $1 AND p.status = $2 ORDER BY p.scheduled_at DESC LIMIT $3;`
		rows, err = s.db.QueryContext(ctx, query, orgID, statusFilter, limit)
	} else {
		query := baseQuery + `WHERE p.organization_id = $1 ORDER BY p.scheduled_at DESC LIMIT $2;`
		rows, err = s.db.QueryContext(ctx, query, orgID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Post
	for rows.Next() {
		var post domain.Post
		var orgIDStr, platformStr, groupIDStr, batchIDStr, mediaStr, schedStr, pubStr string
		var subStr, revStr, revAtStr, retryStr, maxRetryStr, nextRetryStr, errCodeStr, createdStr, updatedStr string
		if err := rows.Scan(
			&post.ID, &orgIDStr, &post.ChannelID, &post.ChannelName, &platformStr, &post.ExternalChannelID,
			&groupIDStr, &batchIDStr,
			&post.Content, &post.LinkURL, &post.MediaType, &mediaStr, &post.Status, &post.QueueTier,
			&schedStr, &pubStr, &post.ExternalPostID,
			&subStr, &revStr, &revAtStr, &post.RejectionNote,
			&retryStr, &maxRetryStr, &nextRetryStr, &errCodeStr, &post.LastErrorMessage,
			&post.CreatedBy, &createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		post.OrganizationID = orgIDStr
		post.ChannelGroupID = groupIDStr
		post.BatchID = batchIDStr
		post.SubmittedBy = subStr
		post.ReviewedBy = revStr
		if revAtStr != "" {
			t := parsePGTime(revAtStr)
			post.ReviewedAt = &t
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

// ClaimDuePosts implements Tenant-Fair Scheduling using:
// ROW_NUMBER() OVER (PARTITION BY organization_id ORDER BY scheduled_at ASC)
// combined with FOR UPDATE SKIP LOCKED.
func (s *PostgresStore) ClaimDuePosts(ctx context.Context, now time.Time, batchSize int) ([]domain.Post, error) {
	query := `
	WITH due_candidates AS (
		SELECT id, organization_id,
		       ROW_NUMBER() OVER (PARTITION BY organization_id ORDER BY scheduled_at ASC) as rnum
		FROM posts
		WHERE (status = 'SCHEDULED' AND scheduled_at <= $1)
		   OR (status = 'FAILED' AND retry_count < max_retries AND next_retry_at IS NOT NULL AND next_retry_at <= $1)
	),
	selected AS (
		SELECT p.id
		FROM posts p
		JOIN due_candidates dc ON dc.id = p.id
		ORDER BY dc.rnum ASC, p.scheduled_at ASC
		LIMIT $2
		FOR UPDATE OF p SKIP LOCKED
	)
	UPDATE posts p
	SET status = 'QUEUED', updated_at = NOW()
	FROM selected
	WHERE p.id = selected.id
	RETURNING p.id, p.organization_id::text, p.channel_id, p.queue_tier, p.retry_count::text;`

	rows, err := s.db.QueryContext(ctx, query, now, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []domain.Post
	for rows.Next() {
		var p domain.Post
		var orgIDStr, retryStr string
		if err := rows.Scan(&p.ID, &orgIDStr, &p.ChannelID, &p.QueueTier, &retryStr); err != nil {
			return nil, err
		}
		p.OrganizationID = orgIDStr
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
	tc := TenantFromContext(ctx)
	var res sql.Result
	var err error

	allowedStatuses := "('DRAFT', 'SCHEDULED', 'QUEUED', 'FAILED', 'PENDING_APPROVAL', 'REJECTED')"
	if tc != nil && tc.OrganizationID != "" {
		res, err = s.db.ExecContext(
			ctx,
			"UPDATE posts SET status = 'CANCELLED', updated_at = NOW() WHERE id = $1 AND organization_id = $2 AND status IN "+allowedStatuses,
			postID, tc.OrganizationID,
		)
	} else {
		res, err = s.db.ExecContext(
			ctx,
			"UPDATE posts SET status = 'CANCELLED', updated_at = NOW() WHERE id = $1 AND status IN "+allowedStatuses,
			postID,
		)
	}
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

// ----------------------------------------------------------------------------
// Users Implementation (Postgres)
// ----------------------------------------------------------------------------

func (s *PostgresStore) CreateUser(ctx context.Context, u *domain.User) (*domain.User, error) {
	query := `
	INSERT INTO users (id, email, password_hash, full_name, avatar_url, is_active, created_at, updated_at)
	VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, NOW(), NOW())
	RETURNING id, created_at::text, updated_at::text;`

	var createdStr, updatedStr string
	err := s.db.QueryRowContext(
		ctx, query,
		u.ID, u.Email, u.PasswordHash, u.FullName, u.AvatarURL, u.IsActive,
	).Scan(&u.ID, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}
	u.CreatedAt = parsePGTime(createdStr)
	u.UpdatedAt = parsePGTime(updatedStr)
	return u, nil
}

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
	SELECT id, email, password_hash, full_name, avatar_url, is_active::text, created_at::text, updated_at::text
	FROM users
	WHERE LOWER(email) = LOWER($1);`

	var u domain.User
	var activeStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.AvatarURL, &activeStr, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	u.IsActive = activeStr == "true" || activeStr == "t"
	u.CreatedAt = parsePGTime(createdStr)
	u.UpdatedAt = parsePGTime(updatedStr)
	return &u, nil
}

func (s *PostgresStore) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	query := `
	SELECT id, email, password_hash, full_name, avatar_url, is_active::text, created_at::text, updated_at::text
	FROM users
	WHERE id = $1;`

	var u domain.User
	var activeStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.AvatarURL, &activeStr, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	u.IsActive = activeStr == "true" || activeStr == "t"
	u.CreatedAt = parsePGTime(createdStr)
	u.UpdatedAt = parsePGTime(updatedStr)
	return &u, nil
}

// ----------------------------------------------------------------------------
// Organizations & Members Implementation (Postgres)
// ----------------------------------------------------------------------------

func (s *PostgresStore) CreateOrganization(ctx context.Context, org *domain.Organization, ownerUserID string) (*domain.Organization, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	query := `
	INSERT INTO organizations (id, name, slug, created_at, updated_at)
	VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, NOW(), NOW())
	RETURNING id, created_at::text, updated_at::text;`

	var createdStr, updatedStr string
	if err := tx.QueryRowContext(ctx, query, org.ID, org.Name, org.Slug).Scan(&org.ID, &createdStr, &updatedStr); err != nil {
		return nil, err
	}
	org.CreatedAt = parsePGTime(createdStr)
	org.UpdatedAt = parsePGTime(updatedStr)

	if ownerUserID != "" {
		memberQuery := `
		INSERT INTO organization_members (organization_id, user_id, org_role, created_at, updated_at)
		VALUES ($1, $2, 'OWNER', NOW(), NOW())
		ON CONFLICT (organization_id, user_id) DO NOTHING;`
		if _, err := tx.ExecContext(ctx, memberQuery, org.ID, ownerUserID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *PostgresStore) GetOrganizationByID(ctx context.Context, id string) (*domain.Organization, error) {
	query := `
	SELECT id, name, slug, created_at::text, updated_at::text
	FROM organizations
	WHERE id = $1;`

	var org domain.Organization
	var createdStr, updatedStr string
	if err := s.db.QueryRowContext(ctx, query, id).Scan(&org.ID, &org.Name, &org.Slug, &createdStr, &updatedStr); err != nil {
		return nil, err
	}
	org.CreatedAt = parsePGTime(createdStr)
	org.UpdatedAt = parsePGTime(updatedStr)
	return &org, nil
}

func (s *PostgresStore) ListUserOrganizations(ctx context.Context, userID string) ([]domain.Organization, error) {
	query := `
	SELECT o.id, o.name, o.slug, o.created_at::text, o.updated_at::text
	FROM organizations o
	JOIN organization_members om ON om.organization_id = o.id
	WHERE om.user_id = $1
	ORDER BY o.name ASC;`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Organization
	for rows.Next() {
		var o domain.Organization
		var createdStr, updatedStr string
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		o.CreatedAt = parsePGTime(createdStr)
		o.UpdatedAt = parsePGTime(updatedStr)
		list = append(list, o)
	}
	return list, nil
}

func (s *PostgresStore) ListOrganizationMembers(ctx context.Context, orgID string) ([]domain.OrganizationMember, error) {
	query := `
	SELECT om.id, om.organization_id::text, om.user_id::text, om.org_role,
	       COALESCE(u.email, ''), COALESCE(u.full_name, ''),
	       om.created_at::text, om.updated_at::text
	FROM organization_members om
	LEFT JOIN users u ON u.id = om.user_id
	WHERE om.organization_id = $1
	ORDER BY om.created_at ASC;`

	rows, err := s.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []domain.OrganizationMember
	for rows.Next() {
		var m domain.OrganizationMember
		var roleStr, createdStr, updatedStr string
		if err := rows.Scan(
			&m.ID, &m.OrganizationID, &m.UserID, &roleStr,
			&m.UserEmail, &m.UserFullName,
			&createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		m.OrgRole = domain.OrgRole(roleStr)
		m.CreatedAt = parsePGTime(createdStr)
		m.UpdatedAt = parsePGTime(updatedStr)
		members = append(members, m)
	}
	return members, nil
}

func (s *PostgresStore) AddOrganizationMember(ctx context.Context, m *domain.OrganizationMember) (*domain.OrganizationMember, error) {
	query := `
	INSERT INTO organization_members (organization_id, user_id, org_role, created_at, updated_at)
	VALUES ($1, $2, $3, NOW(), NOW())
	ON CONFLICT (organization_id, user_id) DO UPDATE SET
		org_role = EXCLUDED.org_role,
		updated_at = NOW()
	RETURNING id, created_at::text, updated_at::text;`

	var createdStr, updatedStr string
	err := s.db.QueryRowContext(
		ctx, query,
		m.OrganizationID, m.UserID, string(m.OrgRole),
	).Scan(&m.ID, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}
	m.CreatedAt = parsePGTime(createdStr)
	m.UpdatedAt = parsePGTime(updatedStr)

	// Fetch user details if not set
	if m.UserEmail == "" {
		_ = s.db.QueryRowContext(ctx, "SELECT email, full_name FROM users WHERE id = $1", m.UserID).Scan(&m.UserEmail, &m.UserFullName)
	}
	return m, nil
}

func (s *PostgresStore) UpdateOrganizationMemberRole(ctx context.Context, orgID string, memberID string, newRole domain.OrgRole) error {
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE organization_members
		 SET org_role = $1, updated_at = NOW()
		 WHERE organization_id = $2 AND (id::text = $3 OR user_id::text = $3)`,
		string(newRole), orgID, memberID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return errors.New("organization member not found")
	}
	return nil
}

func (s *PostgresStore) RemoveOrganizationMember(ctx context.Context, orgID string, memberID string) error {
	res, err := s.db.ExecContext(
		ctx,
		`DELETE FROM organization_members
		 WHERE organization_id = $1 AND (id::text = $2 OR user_id::text = $2)`,
		orgID, memberID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return errors.New("organization member not found")
	}
	return nil
}

func (s *PostgresStore) CreateInvitation(ctx context.Context, inv *domain.OrganizationInvitation) (*domain.OrganizationInvitation, error) {
	if inv.OrganizationID == "" {
		inv.OrganizationID = TenantFromContext(ctx).OrganizationID
	}
	if inv.ExpiresAt.IsZero() {
		inv.ExpiresAt = time.Now().UTC().Add(7 * 24 * time.Hour)
	}

	query := `
	INSERT INTO organization_invitations (organization_id, email, org_role, invited_by, token_hash, expires_at, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, NOW())
	RETURNING id, created_at::text;`

	var createdStr string
	err := s.db.QueryRowContext(
		ctx, query,
		inv.OrganizationID, inv.Email, string(inv.OrgRole), inv.InvitedBy, inv.TokenHash, inv.ExpiresAt,
	).Scan(&inv.ID, &createdStr)
	if err != nil {
		return nil, err
	}
	inv.CreatedAt = parsePGTime(createdStr)
	return inv, nil
}

func (s *PostgresStore) GetInvitationByToken(ctx context.Context, token string) (*domain.OrganizationInvitation, error) {
	query := `
	SELECT id, organization_id::text, email, org_role, invited_by::text, token_hash,
	       expires_at::text, COALESCE(accepted_at::text, ''), created_at::text
	FROM organization_invitations
	WHERE token_hash = $1;`

	var inv domain.OrganizationInvitation
	var roleStr, expStr, accStr, createdStr string
	err := s.db.QueryRowContext(ctx, query, token).Scan(
		&inv.ID, &inv.OrganizationID, &inv.Email, &roleStr, &inv.InvitedBy, &inv.TokenHash,
		&expStr, &accStr, &createdStr,
	)
	if err != nil {
		return nil, err
	}
	inv.OrgRole = domain.OrgRole(roleStr)
	inv.ExpiresAt = parsePGTime(expStr)
	if accStr != "" {
		t := parsePGTime(accStr)
		inv.AcceptedAt = &t
	}
	inv.CreatedAt = parsePGTime(createdStr)
	return &inv, nil
}

func (s *PostgresStore) AcceptInvitation(ctx context.Context, token string, userID string) (*domain.OrganizationMember, error) {
	inv, err := s.GetInvitationByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("invitation not found: %w", err)
	}
	if inv.AcceptedAt != nil {
		return nil, errors.New("invitation has already been accepted")
	}
	if inv.ExpiresAt.Before(time.Now().UTC()) {
		return nil, errors.New("invitation has expired")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "UPDATE organization_invitations SET accepted_at = NOW() WHERE id = $1", inv.ID); err != nil {
		return nil, err
	}

	memberQuery := `
	INSERT INTO organization_members (organization_id, user_id, org_role, created_at, updated_at)
	VALUES ($1, $2, $3, NOW(), NOW())
	ON CONFLICT (organization_id, user_id) DO UPDATE SET
		org_role = EXCLUDED.org_role,
		updated_at = NOW()
	RETURNING id, created_at::text, updated_at::text;`

	var mem domain.OrganizationMember
	mem.OrganizationID = inv.OrganizationID
	mem.UserID = userID
	mem.OrgRole = inv.OrgRole
	var createdStr, updatedStr string
	if err := tx.QueryRowContext(ctx, memberQuery, mem.OrganizationID, mem.UserID, string(mem.OrgRole)).Scan(&mem.ID, &createdStr, &updatedStr); err != nil {
		return nil, err
	}
	mem.CreatedAt = parsePGTime(createdStr)
	mem.UpdatedAt = parsePGTime(updatedStr)

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &mem, nil
}

// ----------------------------------------------------------------------------
// Channel Groups & Permissions Implementation (Postgres)
// ----------------------------------------------------------------------------

func (s *PostgresStore) CreateChannelGroup(ctx context.Context, g *domain.ChannelGroup) (*domain.ChannelGroup, error) {
	if g.OrganizationID == "" {
		g.OrganizationID = TenantFromContext(ctx).OrganizationID
	}
	if g.OrganizationID == "" {
		g.OrganizationID = domain.DefaultOrganizationID
	}

	query := `
	INSERT INTO channel_groups (organization_id, name, description, require_approval, created_at, updated_at)
	VALUES ($1, $2, $3, $4, NOW(), NOW())
	RETURNING id, created_at::text, updated_at::text;`

	var createdStr, updatedStr string
	err := s.db.QueryRowContext(
		ctx, query,
		g.OrganizationID, g.Name, g.Description, g.RequireApproval,
	).Scan(&g.ID, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}
	g.CreatedAt = parsePGTime(createdStr)
	g.UpdatedAt = parsePGTime(updatedStr)
	return g, nil
}

func (s *PostgresStore) ListChannelGroups(ctx context.Context, orgID string) ([]domain.ChannelGroup, error) {
	query := `
	SELECT cg.id, cg.organization_id::text, cg.name, cg.description, cg.require_approval::text,
	       (SELECT COUNT(*) FROM channel_group_items cgi WHERE cgi.channel_group_id = cg.id)::text AS channel_count,
	       (SELECT COUNT(*) FROM channel_group_permissions cgp WHERE cgp.channel_group_id = cg.id)::text AS member_count,
	       cg.created_at::text, cg.updated_at::text
	FROM channel_groups cg
	WHERE cg.organization_id = $1
	ORDER BY cg.created_at DESC;`

	rows, err := s.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []domain.ChannelGroup
	for rows.Next() {
		var g domain.ChannelGroup
		var reqAppStr, chCountStr, memCountStr, createdStr, updatedStr string
		if err := rows.Scan(
			&g.ID, &g.OrganizationID, &g.Name, &g.Description, &reqAppStr,
			&chCountStr, &memCountStr,
			&createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		g.RequireApproval = reqAppStr == "true" || reqAppStr == "t"
		g.ChannelCount, _ = strconv.Atoi(chCountStr)
		g.MemberCount, _ = strconv.Atoi(memCountStr)
		g.CreatedAt = parsePGTime(createdStr)
		g.UpdatedAt = parsePGTime(updatedStr)
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *PostgresStore) GetChannelGroupByID(ctx context.Context, orgID string, groupID string) (*domain.ChannelGroup, error) {
	query := `
	SELECT cg.id, cg.organization_id::text, cg.name, cg.description, cg.require_approval::text,
	       (SELECT COUNT(*) FROM channel_group_items cgi WHERE cgi.channel_group_id = cg.id)::text AS channel_count,
	       (SELECT COUNT(*) FROM channel_group_permissions cgp WHERE cgp.channel_group_id = cg.id)::text AS member_count,
	       cg.created_at::text, cg.updated_at::text
	FROM channel_groups cg
	WHERE cg.id = $1 AND cg.organization_id = $2;`

	var g domain.ChannelGroup
	var reqAppStr, chCountStr, memCountStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, groupID, orgID).Scan(
		&g.ID, &g.OrganizationID, &g.Name, &g.Description, &reqAppStr,
		&chCountStr, &memCountStr,
		&createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	g.RequireApproval = reqAppStr == "true" || reqAppStr == "t"
	g.ChannelCount, _ = strconv.Atoi(chCountStr)
	g.MemberCount, _ = strconv.Atoi(memCountStr)
	g.CreatedAt = parsePGTime(createdStr)
	g.UpdatedAt = parsePGTime(updatedStr)

	// Fetch associated channels
	chQuery := `
	SELECT c.id, c.organization_id::text, c.platform, c.external_id, c.name, c.category, c.token_status, c.is_active::text
	FROM channels c
	JOIN channel_group_items cgi ON cgi.channel_id = c.id
	WHERE cgi.channel_group_id = $1;`

	chRows, err := s.db.QueryContext(ctx, chQuery, groupID)
	if err == nil {
		defer chRows.Close()
		for chRows.Next() {
			var ch domain.Channel
			var platStr, actStr string
			if err := chRows.Scan(&ch.ID, &ch.OrganizationID, &platStr, &ch.ExternalID, &ch.Name, &ch.Category, &ch.TokenStatus, &actStr); err == nil {
				ch.Platform = domain.NormalizePlatform(domain.PlatformType(platStr))
				ch.IsActive = actStr == "true" || actStr == "t"
				g.Channels = append(g.Channels, ch)
			}
		}
	}
	return &g, nil
}

func (s *PostgresStore) DeleteChannelGroup(ctx context.Context, orgID string, groupID string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM channel_groups WHERE id = $1 AND organization_id = $2", groupID, orgID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return errors.New("channel group not found")
	}
	return nil
}

func (s *PostgresStore) SetChannelGroupChannels(ctx context.Context, orgID string, groupID string, channelIDs []string) error {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM channel_groups WHERE id = $1 AND organization_id = $2", groupID, orgID).Scan(&count)
	if err != nil || count == 0 {
		return errors.New("channel group not found")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM channel_group_items WHERE channel_group_id = $1", groupID); err != nil {
		return err
	}

	for _, chID := range channelIDs {
		if strings.TrimSpace(chID) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO channel_group_items (channel_group_id, channel_id, created_at) VALUES ($1, $2, NOW()) ON CONFLICT DO NOTHING", groupID, chID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) SetChannelGroupPermission(ctx context.Context, p *domain.ChannelGroupPermission) (*domain.ChannelGroupPermission, error) {
	query := `
	INSERT INTO channel_group_permissions (channel_group_id, user_id, group_role, created_at, updated_at)
	VALUES ($1, $2, $3, NOW(), NOW())
	ON CONFLICT (channel_group_id, user_id) DO UPDATE SET
		group_role = EXCLUDED.group_role,
		updated_at = NOW()
	RETURNING id, created_at::text, updated_at::text;`

	var createdStr, updatedStr string
	err := s.db.QueryRowContext(
		ctx, query,
		p.ChannelGroupID, p.UserID, string(p.GroupRole),
	).Scan(&p.ID, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}
	p.CreatedAt = parsePGTime(createdStr)
	p.UpdatedAt = parsePGTime(updatedStr)

	if p.UserEmail == "" {
		_ = s.db.QueryRowContext(ctx, "SELECT email, full_name FROM users WHERE id = $1", p.UserID).Scan(&p.UserEmail, &p.UserFullName)
	}
	return p, nil
}

func (s *PostgresStore) DeleteChannelGroupPermission(ctx context.Context, orgID string, groupID string, memberID string) error {
	query := `
	DELETE FROM channel_group_permissions cgp
	USING channel_groups cg
	WHERE cgp.channel_group_id = cg.id
	  AND cg.organization_id = $1
	  AND cg.id = $2
	  AND (cgp.id::text = $3 OR cgp.user_id::text = $3);`

	res, err := s.db.ExecContext(ctx, query, orgID, groupID, memberID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return errors.New("channel group permission not found")
	}
	return nil
}

func (s *PostgresStore) ListChannelGroupPermissions(ctx context.Context, orgID string, groupID string) ([]domain.ChannelGroupPermission, error) {
	query := `
	SELECT cgp.id, cgp.channel_group_id::text, cgp.user_id::text, cgp.group_role,
	       COALESCE(u.email, ''), COALESCE(u.full_name, ''),
	       cgp.created_at::text, cgp.updated_at::text
	FROM channel_group_permissions cgp
	JOIN channel_groups cg ON cg.id = cgp.channel_group_id
	LEFT JOIN users u ON u.id = cgp.user_id
	WHERE cgp.channel_group_id = $1 AND cg.organization_id = $2
	ORDER BY cgp.created_at ASC;`

	rows, err := s.db.QueryContext(ctx, query, groupID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []domain.ChannelGroupPermission
	for rows.Next() {
		var p domain.ChannelGroupPermission
		var roleStr, createdStr, updatedStr string
		if err := rows.Scan(
			&p.ID, &p.ChannelGroupID, &p.UserID, &roleStr,
			&p.UserEmail, &p.UserFullName,
			&createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		p.GroupRole = domain.GroupRole(roleStr)
		p.CreatedAt = parsePGTime(createdStr)
		p.UpdatedAt = parsePGTime(updatedStr)
		perms = append(perms, p)
	}
	return perms, nil
}

func (s *PostgresStore) GetUserChannelRoles(ctx context.Context, orgID string, userID string, channelID string) ([]domain.GroupRole, error) {
	query := `
	SELECT cgp.group_role
	FROM channel_group_permissions cgp
	JOIN channel_group_items cgi ON cgi.channel_group_id = cgp.channel_group_id
	JOIN channel_groups cg ON cg.id = cgp.channel_group_id
	WHERE cg.organization_id = $1 AND cgp.user_id = $2 AND cgi.channel_id = $3;`

	rows, err := s.db.QueryContext(ctx, query, orgID, userID, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []domain.GroupRole
	for rows.Next() {
		var roleStr string
		if err := rows.Scan(&roleStr); err != nil {
			return nil, err
		}
		roles = append(roles, domain.GroupRole(roleStr))
	}
	return roles, nil
}

// ----------------------------------------------------------------------------
// Post Workflow & Approvals Implementation (Postgres)
// ----------------------------------------------------------------------------

func (s *PostgresStore) ReviewPost(ctx context.Context, postID string, reviewerID string, action domain.ApprovalAction, feedbackNote string) (*domain.Post, error) {
	post, err := s.GetPostByID(ctx, postID)
	if err != nil {
		return nil, err
	}
	if !post.CanReview() {
		return nil, fmt.Errorf("post %s is not in reviewable state (status: %s)", postID, post.Status)
	}

	now := time.Now().UTC()
	if action == domain.ApprovalActionApproved {
		post.Approve(reviewerID, now)
	} else if action == domain.ApprovalActionRejected {
		post.Reject(reviewerID, feedbackNote, now)
	} else {
		return nil, fmt.Errorf("invalid approval action: %s", action)
	}
	newStatus := post.Status

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	approvalQuery := `
	INSERT INTO post_approvals (post_id, reviewer_id, action, note, created_at)
	VALUES ($1, $2, $3, $4, NOW());`
	if _, err := tx.ExecContext(ctx, approvalQuery, postID, reviewerID, string(action), feedbackNote); err != nil {
		return nil, err
	}

	postQuery := `
	UPDATE posts
	SET status = $1, reviewed_by = $2, reviewed_at = NOW(), rejection_note = $3, updated_at = NOW()
	WHERE id = $4;`
	if _, err := tx.ExecContext(ctx, postQuery, newStatus, reviewerID, feedbackNote, postID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetPostByID(ctx, postID)
}

func (s *PostgresStore) ResubmitPost(ctx context.Context, postID string, content string, scheduledAt time.Time) (*domain.Post, error) {
	post, err := s.GetPostByID(ctx, postID)
	if err != nil {
		return nil, err
	}
	if post.Status != domain.PostStatusRejected && post.Status != domain.PostStatusDraft {
		return nil, fmt.Errorf("only rejected or draft posts can be resubmitted (status: %s)", post.Status)
	}

	var schedArg any
	if !scheduledAt.IsZero() {
		schedArg = scheduledAt.UTC()
	}

	query := `
	UPDATE posts
	SET status = 'PENDING_APPROVAL',
	    content = CASE WHEN $1 <> '' THEN $1 ELSE content END,
	    scheduled_at = CASE WHEN $2::timestamptz IS NOT NULL THEN $2::timestamptz ELSE scheduled_at END,
	    rejection_note = '',
	    reviewed_by = NULL,
	    reviewed_at = NULL,
	    updated_at = NOW()
	WHERE id = $3;`

	if _, err := s.db.ExecContext(ctx, query, content, schedArg, postID); err != nil {
		return nil, err
	}

	return s.GetPostByID(ctx, postID)
}

func (s *PostgresStore) ListPostApprovals(ctx context.Context, postID string) ([]domain.PostApproval, error) {
	query := `
	SELECT pa.id, pa.post_id::text, pa.reviewer_id::text, COALESCE(u.full_name, COALESCE(u.email, '')),
	       pa.action, pa.note, pa.created_at::text
	FROM post_approvals pa
	LEFT JOIN users u ON u.id = pa.reviewer_id
	WHERE pa.post_id = $1
	ORDER BY pa.created_at ASC;`

	rows, err := s.db.QueryContext(ctx, query, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.PostApproval
	for rows.Next() {
		var a domain.PostApproval
		var actionStr, createdStr string
		if err := rows.Scan(
			&a.ID, &a.PostID, &a.ReviewerID, &a.ReviewerName,
			&actionStr, &a.Note, &createdStr,
		); err != nil {
			return nil, err
		}
		a.Action = domain.ApprovalAction(actionStr)
		a.CreatedAt = parsePGTime(createdStr)
		list = append(list, a)
	}
	return list, nil
}

// ----------------------------------------------------------------------------
// Audit Logs & Media Assets Implementation (Postgres)
// ----------------------------------------------------------------------------

func (s *PostgresStore) InsertAuditLog(ctx context.Context, log *domain.AuditLog) error {
	if log.OrganizationID == "" {
		log.OrganizationID = TenantFromContext(ctx).OrganizationID
	}
	if log.OrganizationID == "" {
		log.OrganizationID = domain.DefaultOrganizationID
	}
	if len(log.Details) == 0 {
		log.Details = json.RawMessage("{}")
	}

	var userIDArg any
	if log.UserID != nil && *log.UserID != "" {
		userIDArg = *log.UserID
	}

	query := `
	INSERT INTO audit_logs (id, organization_id, user_id, action, entity_type, entity_id, details, ip_address, created_at)
	VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7::jsonb, $8, NOW())
	RETURNING id, created_at::text;`

	var createdStr string
	err := s.db.QueryRowContext(
		ctx, query,
		log.ID, log.OrganizationID, userIDArg, log.Action, log.EntityType, log.EntityID, string(log.Details), log.IPAddress,
	).Scan(&log.ID, &createdStr)
	if err != nil {
		return err
	}
	log.CreatedAt = parsePGTime(createdStr)
	return nil
}

func (s *PostgresStore) ListAuditLogs(ctx context.Context, orgID string, limit int) ([]domain.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	query := `
	SELECT id, organization_id::text, COALESCE(user_id::text, ''), action, entity_type, entity_id, details::text, ip_address, created_at::text
	FROM audit_logs
	WHERE organization_id = $1
	ORDER BY created_at DESC
	LIMIT $2;`

	rows, err := s.db.QueryContext(ctx, query, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []domain.AuditLog
	for rows.Next() {
		var l domain.AuditLog
		var userIDStr, detailsStr, createdStr string
		if err := rows.Scan(
			&l.ID, &l.OrganizationID, &userIDStr, &l.Action, &l.EntityType, &l.EntityID,
			&detailsStr, &l.IPAddress, &createdStr,
		); err != nil {
			return nil, err
		}
		if userIDStr != "" {
			l.UserID = &userIDStr
		}
		l.Details = json.RawMessage(detailsStr)
		l.CreatedAt = parsePGTime(createdStr)
		logs = append(logs, l)
	}
	return logs, nil
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
	if asset.OrganizationID == "" {
		asset.OrganizationID = TenantFromContext(ctx).OrganizationID
	}
	if asset.OrganizationID == "" {
		asset.OrganizationID = domain.DefaultOrganizationID
	}

	query := `
	INSERT INTO media_assets (organization_id, file_name, storage_key, public_url, mime_type, file_size_bytes, duration_seconds)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	RETURNING id, created_at::text;`

	var createdStr string
	err := s.db.QueryRowContext(
		ctx, query,
		asset.OrganizationID, asset.FileName, asset.StorageKey, asset.PublicURL, asset.MimeType, asset.FileSizeBytes, asset.DurationSeconds,
	).Scan(&asset.ID, &createdStr)
	if err != nil {
		return nil, err
	}
	asset.CreatedAt = parsePGTime(createdStr)
	return asset, nil
}

func (s *PostgresStore) GetSystemStats(ctx context.Context) (*domain.SystemStats, error) {
	stats := &domain.SystemStats{}
	orgID := TenantFromContext(ctx).OrganizationID

	var totalChannels, activeChannels, sched, queued, pub, failed, pending, rejected string
	query := `
	SELECT
		(SELECT COUNT(*)::text FROM channels WHERE organization_id = $1),
		(SELECT COUNT(*)::text FROM channels WHERE organization_id = $1 AND token_status = 'ACTIVE' AND is_active = TRUE),
		(SELECT COUNT(*)::text FROM posts WHERE organization_id = $1 AND status = 'SCHEDULED'),
		(SELECT COUNT(*)::text FROM posts WHERE organization_id = $1 AND status IN ('QUEUED', 'PROCESSING')),
		(SELECT COUNT(*)::text FROM posts WHERE organization_id = $1 AND status = 'PUBLISHED'),
		(SELECT COUNT(*)::text FROM posts WHERE organization_id = $1 AND status = 'FAILED'),
		(SELECT COUNT(*)::text FROM posts WHERE organization_id = $1 AND status = 'PENDING_APPROVAL'),
		(SELECT COUNT(*)::text FROM posts WHERE organization_id = $1 AND status = 'REJECTED');`

	if err := s.db.QueryRowContext(ctx, query, orgID).Scan(&totalChannels, &activeChannels, &sched, &queued, &pub, &failed, &pending, &rejected); err != nil {
		return stats, err
	}
	stats.TotalChannels, _ = strconv.Atoi(totalChannels)
	stats.ActiveChannels, _ = strconv.Atoi(activeChannels)
	stats.ScheduledPosts, _ = strconv.Atoi(sched)
	stats.QueuedPosts, _ = strconv.Atoi(queued)
	stats.PublishedPosts, _ = strconv.Atoi(pub)
	stats.FailedPosts, _ = strconv.Atoi(failed)
	stats.PendingApprovalPosts, _ = strconv.Atoi(pending)
	stats.RejectedPosts, _ = strconv.Atoi(rejected)
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
// InMemoryStore: Used for unit testing and offline execution
// ============================================================================

type InMemoryStore struct {
	mu                      sync.RWMutex
	users                   map[string]*domain.User
	usersByEmail            map[string]string // email -> user.ID
	organizations           map[string]*domain.Organization
	members                 map[string]*domain.OrganizationMember // member.ID -> member
	invitations             map[string]*domain.OrganizationInvitation // token_hash -> invitation
	channelGroups           map[string]*domain.ChannelGroup // group.ID -> group
	channelGroupItems       map[string][]string // group.ID -> []channel.ID
	channelGroupPermissions map[string]*domain.ChannelGroupPermission // perm.ID -> perm
	channels                map[string]*domain.Channel
	posts                   map[string]*domain.Post
	postApprovals           map[string][]domain.PostApproval // post.ID -> []PostApproval
	logs                    map[string][]domain.PublishLog
	media                   map[string]*domain.MediaAsset
	auditLogs               map[string][]domain.AuditLog // orgID -> []AuditLog
}

func NewInMemoryStore() *InMemoryStore {
	now := time.Now().UTC()
	s := &InMemoryStore{
		users:                   make(map[string]*domain.User),
		usersByEmail:            make(map[string]string),
		organizations:           make(map[string]*domain.Organization),
		members:                 make(map[string]*domain.OrganizationMember),
		invitations:             make(map[string]*domain.OrganizationInvitation),
		channelGroups:           make(map[string]*domain.ChannelGroup),
		channelGroupItems:       make(map[string][]string),
		channelGroupPermissions: make(map[string]*domain.ChannelGroupPermission),
		channels:                make(map[string]*domain.Channel),
		posts:                   make(map[string]*domain.Post),
		postApprovals:           make(map[string][]domain.PostApproval),
		logs:                    make(map[string][]domain.PublishLog),
		media:                   make(map[string]*domain.MediaAsset),
		auditLogs:               make(map[string][]domain.AuditLog),
	}

	// Seed default user
	defaultUser := &domain.User{
		ID:        domain.DefaultUserID,
		Email:     "admin@postpilot.local",
		FullName:  "System Admin",
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.users[defaultUser.ID] = defaultUser
	s.usersByEmail[defaultUser.Email] = defaultUser.ID

	// Seed default workspace / organization
	defaultOrg := &domain.Organization{
		ID:        domain.DefaultOrganizationID,
		Name:      "Default Workspace",
		Slug:      "default-workspace",
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.organizations[defaultOrg.ID] = defaultOrg

	// Seed default organization member (Owner)
	defaultMember := &domain.OrganizationMember{
		ID:             domain.DefaultUserID,
		OrganizationID: domain.DefaultOrganizationID,
		UserID:         domain.DefaultUserID,
		OrgRole:        domain.OrgRoleOwner,
		UserEmail:      defaultUser.Email,
		UserFullName:   defaultUser.FullName,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.members[defaultMember.ID] = defaultMember

	return s
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

func (m *InMemoryStore) enrichChannelGroups(c *domain.Channel) {
	c.GroupIDs = nil
	c.GroupNames = nil
	for groupID, channelIDs := range m.channelGroupItems {
		for _, chID := range channelIDs {
			if chID == c.ID {
				if g, ok := m.channelGroups[groupID]; ok {
					c.GroupIDs = append(c.GroupIDs, g.ID)
					c.GroupNames = append(c.GroupNames, g.Name)
				}
				break
			}
		}
	}
}

func (m *InMemoryStore) UpsertChannel(ctx context.Context, channel *domain.Channel) (*domain.Channel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	channel.Platform = domain.NormalizePlatform(channel.Platform)
	if channel.OrganizationID == "" {
		channel.OrganizationID = TenantFromContext(ctx).OrganizationID
	}
	if channel.OrganizationID == "" {
		channel.OrganizationID = domain.DefaultOrganizationID
	}

	for _, existing := range m.channels {
		if existing.OrganizationID == channel.OrganizationID &&
			existing.Platform == channel.Platform &&
			existing.ExternalID == channel.ExternalID {
			existing.Name = channel.Name
			existing.Category = channel.Category
			existing.EncryptedAccessToken = channel.EncryptedAccessToken
			existing.TokenStatus = channel.TokenStatus
			existing.IsActive = channel.IsActive
			existing.UpdatedAt = time.Now().UTC()
			cp := *existing
			m.enrichChannelGroups(&cp)
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
	m.enrichChannelGroups(&cp)
	return &cp, nil
}

func (m *InMemoryStore) ListChannels(ctx context.Context) ([]domain.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	orgID := TenantFromContext(ctx).OrganizationID
	out := make([]domain.Channel, 0)
	for _, c := range m.channels {
		if c.OrganizationID == orgID {
			cp := *c
			m.enrichChannelGroups(&cp)
			out = append(out, cp)
		}
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
	tc := TenantFromContext(ctx)
	if tc != nil && tc.OrganizationID != "" && c.OrganizationID != tc.OrganizationID {
		return nil, errors.New("channel not found")
	}
	cp := *c
	m.enrichChannelGroups(&cp)
	return &cp, nil
}

func (m *InMemoryStore) DeleteChannel(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.channels[id]
	if !ok {
		return errors.New("channel not found")
	}
	tc := TenantFromContext(ctx)
	if tc != nil && tc.OrganizationID != "" && c.OrganizationID != tc.OrganizationID {
		return errors.New("channel not found")
	}

	delete(m.channels, id)

	for postID, post := range m.posts {
		if post.ChannelID == id {
			delete(m.posts, postID)
		}
	}
	for groupID, items := range m.channelGroupItems {
		var filtered []string
		for _, item := range items {
			if item != id {
				filtered = append(filtered, item)
			}
		}
		m.channelGroupItems[groupID] = filtered
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

	tc := TenantFromContext(ctx)
	if post.OrganizationID == "" {
		post.OrganizationID = tc.OrganizationID
	}
	if post.OrganizationID == "" {
		post.OrganizationID = domain.DefaultOrganizationID
	}
	if post.CreatedBy == "" {
		post.CreatedBy = tc.UserID
	}

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
	tc := TenantFromContext(ctx)
	if tc != nil && tc.OrganizationID != "" && p.OrganizationID != tc.OrganizationID {
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

	orgID := TenantFromContext(ctx).OrganizationID
	var list []domain.Post
	for _, p := range m.posts {
		if p.OrganizationID != orgID {
			continue
		}
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

	sort.Slice(list, func(i, j int) bool {
		return list[i].ScheduledAt.After(list[j].ScheduledAt)
	})

	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

// ClaimDuePosts implements Tenant-Fair Scheduling for InMemoryStore.
func (m *InMemoryStore) ClaimDuePosts(ctx context.Context, now time.Time, batchSize int) ([]domain.Post, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	byOrg := make(map[string][]*domain.Post)
	for _, p := range m.posts {
		isDueScheduled := p.Status == domain.PostStatusScheduled && !p.ScheduledAt.After(now)
		isDueRetry := p.Status == domain.PostStatusFailed && p.RetryCount < p.MaxRetries && p.NextRetryAt != nil && !p.NextRetryAt.After(now)
		if isDueScheduled || isDueRetry {
			byOrg[p.OrganizationID] = append(byOrg[p.OrganizationID], p)
		}
	}

	// Sort each tenant's posts by ScheduledAt ascending
	orgKeys := make([]string, 0, len(byOrg))
	for orgID, list := range byOrg {
		orgKeys = append(orgKeys, orgID)
		sort.Slice(list, func(i, j int) bool {
			return list[i].ScheduledAt.Before(list[j].ScheduledAt)
		})
	}
	sort.Strings(orgKeys)

	var claimed []domain.Post
	maxLen := 0
	for _, list := range byOrg {
		if len(list) > maxLen {
			maxLen = len(list)
		}
	}

	// Interleave round-robin across tenants
	for idx := 0; idx < maxLen && len(claimed) < batchSize; idx++ {
		for _, orgID := range orgKeys {
			list := byOrg[orgID]
			if idx < len(list) && len(claimed) < batchSize {
				p := list[idx]
				p.Status = domain.PostStatusQueued
				p.UpdatedAt = now
				claimed = append(claimed, *p)
			}
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
		return errors.New("post not found")
	}
	tc := TenantFromContext(ctx)
	if tc != nil && tc.OrganizationID != "" && p.OrganizationID != tc.OrganizationID {
		return errors.New("post not found")
	}

	if p.CanCancel() {
		p.Status = domain.PostStatusCancelled
		p.UpdatedAt = time.Now().UTC()
		return nil
	}
	return fmt.Errorf("post %s cannot be cancelled (status: %s)", postID, p.Status)
}

// ----------------------------------------------------------------------------
// Users Implementation (InMemory)
// ----------------------------------------------------------------------------

func (m *InMemoryStore) CreateUser(ctx context.Context, u *domain.User) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	lower := strings.ToLower(u.Email)
	if _, exists := m.usersByEmail[lower]; exists {
		return nil, fmt.Errorf("user with email %s already exists", u.Email)
	}
	if u.ID == "" {
		u.ID = newUUID()
	}
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	cp := *u
	m.users[cp.ID] = &cp
	m.usersByEmail[lower] = cp.ID
	return &cp, nil
}

func (m *InMemoryStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.usersByEmail[strings.ToLower(email)]
	if !ok {
		return nil, errors.New("user not found")
	}
	cp := *m.users[id]
	return &cp, nil
}

func (m *InMemoryStore) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, ok := m.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	cp := *u
	return &cp, nil
}

// ----------------------------------------------------------------------------
// Organizations & Members Implementation (InMemory)
// ----------------------------------------------------------------------------

func (m *InMemoryStore) CreateOrganization(ctx context.Context, org *domain.Organization, ownerUserID string) (*domain.Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if org.ID == "" {
		org.ID = newUUID()
	}
	now := time.Now().UTC()
	org.CreatedAt = now
	org.UpdatedAt = now
	cp := *org
	m.organizations[cp.ID] = &cp

	if ownerUserID != "" {
		mem := &domain.OrganizationMember{
			ID:             newUUID(),
			OrganizationID: cp.ID,
			UserID:         ownerUserID,
			OrgRole:        domain.OrgRoleOwner,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if u, ok := m.users[ownerUserID]; ok {
			mem.UserEmail = u.Email
			mem.UserFullName = u.FullName
		}
		m.members[mem.ID] = mem
	}

	return &cp, nil
}

func (m *InMemoryStore) GetOrganizationByID(ctx context.Context, id string) (*domain.Organization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	org, ok := m.organizations[id]
	if !ok {
		return nil, errors.New("organization not found")
	}
	cp := *org
	return &cp, nil
}

func (m *InMemoryStore) ListUserOrganizations(ctx context.Context, userID string) ([]domain.Organization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	seen := make(map[string]bool)
	var out []domain.Organization
	for _, mem := range m.members {
		if mem.UserID == userID {
			if org, ok := m.organizations[mem.OrganizationID]; ok && !seen[org.ID] {
				seen[org.ID] = true
				out = append(out, *org)
			}
		}
	}
	return out, nil
}

func (m *InMemoryStore) ListOrganizationMembers(ctx context.Context, orgID string) ([]domain.OrganizationMember, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []domain.OrganizationMember
	for _, mem := range m.members {
		if mem.OrganizationID == orgID {
			cp := *mem
			if u, ok := m.users[cp.UserID]; ok {
				cp.UserEmail = u.Email
				cp.UserFullName = u.FullName
			}
			out = append(out, cp)
		}
	}
	return out, nil
}

func (m *InMemoryStore) AddOrganizationMember(ctx context.Context, mem *domain.OrganizationMember) (*domain.OrganizationMember, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	for _, existing := range m.members {
		if existing.OrganizationID == mem.OrganizationID && existing.UserID == mem.UserID {
			existing.OrgRole = mem.OrgRole
			existing.UpdatedAt = now
			cp := *existing
			if u, ok := m.users[cp.UserID]; ok {
				cp.UserEmail = u.Email
				cp.UserFullName = u.FullName
			}
			return &cp, nil
		}
	}

	if mem.ID == "" {
		mem.ID = newUUID()
	}
	mem.CreatedAt = now
	mem.UpdatedAt = now
	cp := *mem
	if u, ok := m.users[cp.UserID]; ok {
		cp.UserEmail = u.Email
		cp.UserFullName = u.FullName
	}
	m.members[cp.ID] = &cp
	return &cp, nil
}

func (m *InMemoryStore) UpdateOrganizationMemberRole(ctx context.Context, orgID string, memberID string, newRole domain.OrgRole) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, mem := range m.members {
		if mem.OrganizationID == orgID && (mem.ID == memberID || mem.UserID == memberID) {
			mem.OrgRole = newRole
			mem.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return errors.New("organization member not found")
}

func (m *InMemoryStore) RemoveOrganizationMember(ctx context.Context, orgID string, memberID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, mem := range m.members {
		if mem.OrganizationID == orgID && (mem.ID == memberID || mem.UserID == memberID) {
			delete(m.members, id)
			return nil
		}
	}
	return errors.New("organization member not found")
}

func (m *InMemoryStore) CreateInvitation(ctx context.Context, inv *domain.OrganizationInvitation) (*domain.OrganizationInvitation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if inv.OrganizationID == "" {
		inv.OrganizationID = TenantFromContext(ctx).OrganizationID
	}
	if inv.ID == "" {
		inv.ID = newUUID()
	}
	if inv.ExpiresAt.IsZero() {
		inv.ExpiresAt = time.Now().UTC().Add(7 * 24 * time.Hour)
	}
	inv.CreatedAt = time.Now().UTC()
	cp := *inv
	m.invitations[cp.TokenHash] = &cp
	return &cp, nil
}

func (m *InMemoryStore) GetInvitationByToken(ctx context.Context, token string) (*domain.OrganizationInvitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	inv, ok := m.invitations[token]
	if !ok {
		return nil, errors.New("invitation not found")
	}
	cp := *inv
	return &cp, nil
}

func (m *InMemoryStore) AcceptInvitation(ctx context.Context, token string, userID string) (*domain.OrganizationMember, error) {
	m.mu.Lock()
	inv, ok := m.invitations[token]
	if !ok {
		m.mu.Unlock()
		return nil, errors.New("invitation not found")
	}
	if inv.AcceptedAt != nil {
		m.mu.Unlock()
		return nil, errors.New("invitation has already been accepted")
	}
	if inv.ExpiresAt.Before(time.Now().UTC()) {
		m.mu.Unlock()
		return nil, errors.New("invitation has expired")
	}

	now := time.Now().UTC()
	inv.AcceptedAt = &now
	orgID := inv.OrganizationID
	role := inv.OrgRole
	m.mu.Unlock()

	return m.AddOrganizationMember(ctx, &domain.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		OrgRole:        role,
	})
}

// ----------------------------------------------------------------------------
// Channel Groups & Permissions Implementation (InMemory)
// ----------------------------------------------------------------------------

func (m *InMemoryStore) CreateChannelGroup(ctx context.Context, g *domain.ChannelGroup) (*domain.ChannelGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if g.OrganizationID == "" {
		g.OrganizationID = TenantFromContext(ctx).OrganizationID
	}
	if g.OrganizationID == "" {
		g.OrganizationID = domain.DefaultOrganizationID
	}
	if g.ID == "" {
		g.ID = newUUID()
	}
	now := time.Now().UTC()
	g.CreatedAt = now
	g.UpdatedAt = now
	cp := *g
	m.channelGroups[cp.ID] = &cp
	return &cp, nil
}

func (m *InMemoryStore) ListChannelGroups(ctx context.Context, orgID string) ([]domain.ChannelGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []domain.ChannelGroup
	for _, g := range m.channelGroups {
		if g.OrganizationID == orgID {
			cp := *g
			cp.ChannelCount = len(m.channelGroupItems[g.ID])
			permCount := 0
			for _, p := range m.channelGroupPermissions {
				if p.ChannelGroupID == g.ID {
					permCount++
				}
			}
			cp.MemberCount = permCount
			out = append(out, cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (m *InMemoryStore) GetChannelGroupByID(ctx context.Context, orgID string, groupID string) (*domain.ChannelGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	g, ok := m.channelGroups[groupID]
	if !ok || g.OrganizationID != orgID {
		return nil, errors.New("channel group not found")
	}
	cp := *g
	chIDs := m.channelGroupItems[groupID]
	for _, chID := range chIDs {
		if ch, exists := m.channels[chID]; exists {
			chCopy := *ch
			m.enrichChannelGroups(&chCopy)
			cp.Channels = append(cp.Channels, chCopy)
		}
	}
	cp.ChannelCount = len(cp.Channels)
	permCount := 0
	for _, p := range m.channelGroupPermissions {
		if p.ChannelGroupID == groupID {
			permCount++
		}
	}
	cp.MemberCount = permCount
	return &cp, nil
}

func (m *InMemoryStore) DeleteChannelGroup(ctx context.Context, orgID string, groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.channelGroups[groupID]
	if !ok || g.OrganizationID != orgID {
		return errors.New("channel group not found")
	}
	delete(m.channelGroups, groupID)
	delete(m.channelGroupItems, groupID)
	for pID, p := range m.channelGroupPermissions {
		if p.ChannelGroupID == groupID {
			delete(m.channelGroupPermissions, pID)
		}
	}
	return nil
}

func (m *InMemoryStore) SetChannelGroupChannels(ctx context.Context, orgID string, groupID string, channelIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.channelGroups[groupID]
	if !ok || g.OrganizationID != orgID {
		return errors.New("channel group not found")
	}
	m.channelGroupItems[groupID] = append([]string(nil), channelIDs...)
	return nil
}

func (m *InMemoryStore) SetChannelGroupPermission(ctx context.Context, p *domain.ChannelGroupPermission) (*domain.ChannelGroupPermission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	for _, existing := range m.channelGroupPermissions {
		if existing.ChannelGroupID == p.ChannelGroupID && existing.UserID == p.UserID {
			existing.GroupRole = p.GroupRole
			existing.UpdatedAt = now
			cp := *existing
			if u, ok := m.users[cp.UserID]; ok {
				cp.UserEmail = u.Email
				cp.UserFullName = u.FullName
			}
			return &cp, nil
		}
	}

	if p.ID == "" {
		p.ID = newUUID()
	}
	p.CreatedAt = now
	p.UpdatedAt = now
	cp := *p
	if u, ok := m.users[cp.UserID]; ok {
		cp.UserEmail = u.Email
		cp.UserFullName = u.FullName
	}
	m.channelGroupPermissions[cp.ID] = &cp
	return &cp, nil
}

func (m *InMemoryStore) DeleteChannelGroupPermission(ctx context.Context, orgID string, groupID string, memberID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.channelGroups[groupID]
	if !ok || g.OrganizationID != orgID {
		return errors.New("channel group not found")
	}

	for pID, perm := range m.channelGroupPermissions {
		if perm.ChannelGroupID == groupID && (perm.ID == memberID || perm.UserID == memberID) {
			delete(m.channelGroupPermissions, pID)
			return nil
		}
	}
	return errors.New("channel group permission not found")
}

func (m *InMemoryStore) ListChannelGroupPermissions(ctx context.Context, orgID string, groupID string) ([]domain.ChannelGroupPermission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	g, ok := m.channelGroups[groupID]
	if !ok || g.OrganizationID != orgID {
		return nil, errors.New("channel group not found")
	}

	var out []domain.ChannelGroupPermission
	for _, p := range m.channelGroupPermissions {
		if p.ChannelGroupID == groupID {
			cp := *p
			if u, ok := m.users[cp.UserID]; ok {
				cp.UserEmail = u.Email
				cp.UserFullName = u.FullName
			}
			out = append(out, cp)
		}
	}
	return out, nil
}

func (m *InMemoryStore) GetUserChannelRoles(ctx context.Context, orgID string, userID string, channelID string) ([]domain.GroupRole, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var roles []domain.GroupRole
	for groupID, chList := range m.channelGroupItems {
		g, ok := m.channelGroups[groupID]
		if !ok || g.OrganizationID != orgID {
			continue
		}
		hasChannel := false
		for _, chID := range chList {
			if chID == channelID {
				hasChannel = true
				break
			}
		}
		if !hasChannel {
			continue
		}

		for _, perm := range m.channelGroupPermissions {
			if perm.ChannelGroupID == groupID && perm.UserID == userID {
				roles = append(roles, perm.GroupRole)
			}
		}
	}
	return roles, nil
}

// ----------------------------------------------------------------------------
// Post Workflow & Approvals Implementation (InMemory)
// ----------------------------------------------------------------------------

func (m *InMemoryStore) ReviewPost(ctx context.Context, postID string, reviewerID string, action domain.ApprovalAction, feedbackNote string) (*domain.Post, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.posts[postID]
	if !ok {
		return nil, errors.New("post not found")
	}
	if !p.CanReview() {
		return nil, fmt.Errorf("post %s is not in reviewable state (status: %s)", postID, p.Status)
	}

	now := time.Now().UTC()
	if action == domain.ApprovalActionApproved {
		p.Approve(reviewerID, now)
	} else if action == domain.ApprovalActionRejected {
		p.Reject(reviewerID, feedbackNote, now)
	} else {
		return nil, fmt.Errorf("invalid approval action: %s", action)
	}
	p.UpdatedAt = now

	approval := domain.PostApproval{
		ID:         newUUID(),
		PostID:     postID,
		ReviewerID: reviewerID,
		Action:     action,
		Note:       feedbackNote,
		CreatedAt:  now,
	}
	if u, ok := m.users[reviewerID]; ok {
		approval.ReviewerName = u.FullName
		if approval.ReviewerName == "" {
			approval.ReviewerName = u.Email
		}
	}
	m.postApprovals[postID] = append(m.postApprovals[postID], approval)

	cp := *p
	if ch, exists := m.channels[cp.ChannelID]; exists {
		cp.ChannelName = ch.Name
		cp.Platform = ch.Platform
		cp.ExternalChannelID = ch.ExternalID
	}
	return &cp, nil
}

func (m *InMemoryStore) ResubmitPost(ctx context.Context, postID string, content string, scheduledAt time.Time) (*domain.Post, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.posts[postID]
	if !ok {
		return nil, errors.New("post not found")
	}
	if p.Status != domain.PostStatusRejected && p.Status != domain.PostStatusDraft {
		return nil, fmt.Errorf("only rejected or draft posts can be resubmitted (status: %s)", p.Status)
	}

	now := time.Now().UTC()
	p.Status = domain.PostStatusPendingApproval
	if content != "" {
		p.Content = content
	}
	if !scheduledAt.IsZero() {
		p.ScheduledAt = scheduledAt.UTC()
	}
	p.RejectionNote = ""
	p.ReviewedBy = ""
	p.ReviewedAt = nil
	p.UpdatedAt = now

	cp := *p
	if ch, exists := m.channels[cp.ChannelID]; exists {
		cp.ChannelName = ch.Name
		cp.Platform = ch.Platform
		cp.ExternalChannelID = ch.ExternalID
	}
	return &cp, nil
}

func (m *InMemoryStore) ListPostApprovals(ctx context.Context, postID string) ([]domain.PostApproval, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := m.postApprovals[postID]
	out := make([]domain.PostApproval, len(list))
	for i, pa := range list {
		out[i] = pa
		if u, ok := m.users[pa.ReviewerID]; ok {
			out[i].ReviewerName = u.FullName
			if out[i].ReviewerName == "" {
				out[i].ReviewerName = u.Email
			}
		}
	}
	return out, nil
}

// ----------------------------------------------------------------------------
// Audit Logs & Media Assets Implementation (InMemory)
// ----------------------------------------------------------------------------

func (m *InMemoryStore) InsertAuditLog(ctx context.Context, log *domain.AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if log.OrganizationID == "" {
		log.OrganizationID = TenantFromContext(ctx).OrganizationID
	}
	if log.OrganizationID == "" {
		log.OrganizationID = domain.DefaultOrganizationID
	}
	if log.ID == "" {
		log.ID = newUUID()
	}
	if len(log.Details) == 0 {
		log.Details = json.RawMessage("{}")
	}
	log.CreatedAt = time.Now().UTC()
	m.auditLogs[log.OrganizationID] = append(m.auditLogs[log.OrganizationID], *log)
	return nil
}

func (m *InMemoryStore) ListAuditLogs(ctx context.Context, orgID string, limit int) ([]domain.AuditLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > 500 {
		limit = 50
	}
	logs := m.auditLogs[orgID]
	n := len(logs)
	out := make([]domain.AuditLog, 0, limit)
	for i := n - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, logs[i])
	}
	return out, nil
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

	if asset.OrganizationID == "" {
		asset.OrganizationID = TenantFromContext(ctx).OrganizationID
	}
	if asset.OrganizationID == "" {
		asset.OrganizationID = domain.DefaultOrganizationID
	}
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

	orgID := TenantFromContext(ctx).OrganizationID
	stats := &domain.SystemStats{}
	for _, c := range m.channels {
		if c.OrganizationID == orgID {
			stats.TotalChannels++
			if c.IsActive && c.TokenStatus == domain.TokenStatusActive {
				stats.ActiveChannels++
			}
		}
	}
	for _, post := range m.posts {
		if post.OrganizationID != orgID {
			continue
		}
		switch post.Status {
		case domain.PostStatusScheduled:
			stats.ScheduledPosts++
		case domain.PostStatusQueued, domain.PostStatusProcessing:
			stats.QueuedPosts++
		case domain.PostStatusPublished:
			stats.PublishedPosts++
		case domain.PostStatusFailed:
			stats.FailedPosts++
		case domain.PostStatusPendingApproval:
			stats.PendingApprovalPosts++
		case domain.PostStatusRejected:
			stats.RejectedPosts++
		}
	}
	return stats, nil
}
