package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// ----------------------------------------------------------------------------
// Shared post row mapping (PostgresStore)
// ----------------------------------------------------------------------------

// postSelectColumns is the single column list used by every post read so the scan order stays in one place.
const postSelectColumns = `p.id, p.organization_id::text, p.channel_id, COALESCE(c.name, ''), COALESCE(c.platform, 'FACEBOOK'), COALESCE(c.external_id, ''),
	       COALESCE(p.channel_group_id::text, ''), COALESCE(p.batch_id::text, ''), COALESCE(b.name, ''),
	       p.content, p.link_url, p.media_type, p.media_urls::text, p.status, p.queue_tier,
	       p.scheduled_at::text, COALESCE(p.published_at::text, ''), p.external_post_id,
	       COALESCE(p.submitted_by::text, ''), COALESCE(p.reviewed_by::text, ''), COALESCE(p.reviewed_at::text, ''), p.rejection_note,
	       p.retry_count::text, p.max_retries::text, COALESCE(p.next_retry_at::text, ''), p.last_error_code::text, p.last_error_message,
	       p.created_by, p.created_at::text, p.updated_at::text`

const postSelectFrom = ` FROM posts p
	LEFT JOIN channels c ON c.id = p.channel_id
	LEFT JOIN post_batches b ON b.id = p.batch_id `

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isUUID guards uuid-typed predicates so indexes are used natively and malformed ids simply match nothing.
func isUUID(v string) bool { return uuidPattern.MatchString(v) }

// enrichPostChannel copies display fields from the channel onto the post.
func enrichPostChannel(post *domain.Post, ch *domain.Channel) {
	if ch == nil {
		return
	}
	post.ChannelName = ch.Name
	post.Platform = ch.Platform
	post.ExternalChannelID = ch.ExternalID
}

type rowScanner interface {
	Scan(dest ...any) error
}

type sqlRunner interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func scanPost(sc rowScanner) (*domain.Post, error) {
	var post domain.Post
	var orgIDStr, platformStr, groupIDStr, batchIDStr, mediaStr, schedStr, pubStr string
	var subStr, revStr, revAtStr, retryStr, maxRetryStr, nextRetryStr, errCodeStr, createdStr, updatedStr string
	if err := sc.Scan(
		&post.ID, &orgIDStr, &post.ChannelID, &post.ChannelName, &platformStr, &post.ExternalChannelID,
		&groupIDStr, &batchIDStr, &post.BatchName,
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
	if post.MediaURLs == nil {
		post.MediaURLs = []string{}
	}
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

// insertPost writes one prepared aggregate using the given runner (plain DB or transaction).
func insertPost(ctx context.Context, run sqlRunner, post *domain.Post) error {
	if post.MediaURLs == nil {
		post.MediaURLs = []string{}
	}
	mediaJSON, err := json.Marshal(post.MediaURLs)
	if err != nil {
		return err
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
	if err := run.QueryRowContext(
		ctx, query,
		post.OrganizationID, post.ChannelID, post.ChannelGroupID, post.BatchID,
		post.Content, post.LinkURL, post.MediaType, string(mediaJSON),
		post.Status, post.QueueTier, post.ScheduledAt, post.MaxRetries, post.CreatedBy, post.SubmittedBy,
	).Scan(&post.ID, &createdStr, &updatedStr); err != nil {
		return err
	}
	post.CreatedAt = parsePGTime(createdStr)
	post.UpdatedAt = parsePGTime(updatedStr)
	return nil
}

// QueryPosts lists tenant posts filtered by status, channel, batch and scheduled time window.
func (s *PostgresStore) QueryPosts(ctx context.Context, q domain.PostQuery) ([]domain.Post, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	q.Normalize()
	args := []any{TenantFromContext(ctx).OrganizationID}
	where := []string{"p.organization_id = $1"}
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if len(q.Statuses) > 0 {
		ph := make([]string, 0, len(q.Statuses))
		for _, st := range q.Statuses {
			ph = append(ph, next(st))
		}
		where = append(where, "p.status IN ("+strings.Join(ph, ", ")+")")
	}
	if len(q.ChannelIDs) > 0 {
		ph := make([]string, 0, len(q.ChannelIDs))
		for _, id := range q.ChannelIDs {
			if isUUID(id) {
				ph = append(ph, next(id)+"::uuid")
			}
		}
		if len(ph) == 0 {
			return nil, nil // every requested channel id is malformed: nothing can match
		}
		where = append(where, "p.channel_id IN ("+strings.Join(ph, ", ")+")")
	}
	if q.BatchID != "" {
		if !isUUID(q.BatchID) {
			return nil, nil
		}
		where = append(where, "p.batch_id = "+next(q.BatchID)+"::uuid")
	}
	if q.From != nil {
		where = append(where, "p.scheduled_at >= "+next(q.From.UTC()))
	}
	if q.To != nil {
		where = append(where, "p.scheduled_at < "+next(q.To.UTC()))
	}
	order := "DESC"
	if q.Ascending {
		order = "ASC"
	}
	query := "SELECT " + postSelectColumns + postSelectFrom +
		" WHERE " + strings.Join(where, " AND ") +
		" ORDER BY p.scheduled_at " + order + ", p.created_at " + order + ", p.id ASC" +
		" LIMIT " + next(q.Limit) + " OFFSET " + next(q.Offset) + ";"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Post
	for rows.Next() {
		post, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *post)
	}
	return list, rows.Err()
}

// CreatePostsBatch inserts an optional batch record and all posts in one transaction (all-or-nothing).
func (s *PostgresStore) CreatePostsBatch(ctx context.Context, batch *domain.PostBatch, posts []*domain.Post) ([]*domain.Post, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return nil, errors.New("no posts to create")
	}
	tc := TenantFromContext(ctx)
	if batch != nil && batch.OrganizationID != "" && batch.OrganizationID != tc.OrganizationID {
		return nil, errors.New("organization mismatch")
	}
	channels := make(map[string]*domain.Channel)
	for _, post := range posts {
		if post.OrganizationID != "" && post.OrganizationID != tc.OrganizationID {
			return nil, errors.New("organization mismatch")
		}
		if _, ok := channels[post.ChannelID]; !ok {
			ch, err := s.GetChannelByID(ctx, post.ChannelID)
			if err != nil {
				return nil, err
			}
			channels[post.ChannelID] = ch
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if batch != nil {
		if batch.ID == "" {
			batch.ID = newUUID()
		}
		batch.OrganizationID = tc.OrganizationID
		if batch.Kind == "" {
			batch.Kind = domain.PostBatchKindBulk
		}
		if batch.CreatedBy == "" {
			batch.CreatedBy = tc.UserID
		}
		batch.ItemCount = len(posts)
		planJSON := ""
		if batch.Plan != nil {
			raw, err := json.Marshal(batch.Plan)
			if err != nil {
				return nil, err
			}
			planJSON = string(raw)
		}
		var createdStr string
		if err := tx.QueryRowContext(ctx, `
		INSERT INTO post_batches (id, organization_id, name, kind, schedule_plan, item_count, created_by)
		VALUES ($1::uuid, $2, $3, $4, NULLIF($5, '')::jsonb, $6, $7)
		RETURNING created_at::text;`, batch.ID, batch.OrganizationID, batch.Name, batch.Kind, planJSON, batch.ItemCount, batch.CreatedBy).Scan(&createdStr); err != nil {
			return nil, err
		}
		batch.CreatedAt = parsePGTime(createdStr)
	}

	for _, post := range posts {
		if post.OrganizationID == "" {
			post.OrganizationID = tc.OrganizationID
		}
		if post.CreatedBy == "" {
			post.CreatedBy = tc.UserID
		}
		if batch != nil {
			post.BatchID = batch.ID
			post.BatchName = batch.Name
		}
		if err := insertPost(ctx, tx, post); err != nil {
			return nil, err
		}
		enrichPostChannel(post, channels[post.ChannelID])
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return posts, nil
}

// UpdatePost applies a patch to an editable post with a compare-and-set on its current status.
func (s *PostgresStore) UpdatePost(ctx context.Context, postID string, patch domain.PostPatch) (*domain.Post, error) {
	post, err := s.GetPostByID(ctx, postID)
	if err != nil {
		return nil, err
	}
	prevStatus := post.Status
	if err := post.ApplyPatch(patch, time.Now().UTC()); err != nil {
		return nil, err
	}
	mediaJSON, err := json.Marshal(post.MediaURLs)
	if err != nil {
		return nil, err
	}
	clearReview := 0
	if patch.RequireReview {
		clearReview = 1
	}
	res, err := s.db.ExecContext(ctx, `
	UPDATE posts SET content = $1, link_url = $2, media_type = $3, media_urls = $4::jsonb, queue_tier = $5,
	       scheduled_at = $6, status = $7, retry_count = $8,
	       next_retry_at = CASE WHEN $7 = 'FAILED' THEN next_retry_at ELSE NULL END,
	       last_error_code = $9, last_error_message = $10,
	       reviewed_by = CASE WHEN $14 = 1 THEN NULL ELSE reviewed_by END,
	       reviewed_at = CASE WHEN $14 = 1 THEN NULL ELSE reviewed_at END,
	       rejection_note = CASE WHEN $14 = 1 THEN '' ELSE rejection_note END,
	       submitted_by = CASE WHEN $14 = 1 AND NULLIF($15, '') IS NOT NULL THEN NULLIF($15, '')::uuid ELSE submitted_by END,
	       updated_at = NOW()
	WHERE id = $11::uuid AND organization_id = $12 AND status = $13;`,
		post.Content, post.LinkURL, post.MediaType, string(mediaJSON), post.QueueTier,
		post.ScheduledAt, post.Status, post.RetryCount, post.LastErrorCode, post.LastErrorMessage,
		postID, TenantFromContext(ctx).OrganizationID, prevStatus, clearReview, submittedBy(patch))
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, domain.ErrPostNotEditable
	}
	return s.GetPostByID(ctx, postID)
}

// submittedBy returns the re-attribution id only when it is a well-formed uuid (the column is uuid-typed).
func submittedBy(patch domain.PostPatch) string {
	if patch.RequireReview && isUUID(patch.SubmittedBy) {
		return patch.SubmittedBy
	}
	return ""
}

// GetPostBatch returns a tenant-scoped batch record.
func (s *PostgresStore) GetPostBatch(ctx context.Context, id string) (*domain.PostBatch, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	if !isUUID(id) {
		return nil, errors.New("batch not found")
	}
	var b domain.PostBatch
	var planStr, countStr, createdStr string
	err := s.db.QueryRowContext(ctx, `
	SELECT id::text, organization_id::text, name, kind, COALESCE(schedule_plan::text, ''), item_count::text, created_by, created_at::text
	FROM post_batches WHERE id = $1::uuid AND organization_id = $2;`, id, TenantFromContext(ctx).OrganizationID).
		Scan(&b.ID, &b.OrganizationID, &b.Name, &b.Kind, &planStr, &countStr, &b.CreatedBy, &createdStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("batch not found")
		}
		return nil, err
	}
	if planStr != "" {
		var plan domain.SchedulePlan
		if json.Unmarshal([]byte(planStr), &plan) == nil {
			b.Plan = &plan
		}
	}
	b.ItemCount, _ = strconv.Atoi(countStr)
	b.CreatedAt = parsePGTime(createdStr)
	return &b, nil
}

// ----------------------------------------------------------------------------
// InMemoryStore parity
// ----------------------------------------------------------------------------

func (m *InMemoryStore) enrichPost(p *domain.Post) domain.Post {
	cp := *p
	if ch, exists := m.channels[cp.ChannelID]; exists {
		cp.ChannelName = ch.Name
		cp.Platform = ch.Platform
		cp.ExternalChannelID = ch.ExternalID
	}
	if cp.BatchID != "" {
		if b, ok := m.batches[cp.BatchID]; ok {
			cp.BatchName = b.Name
		}
	}
	if cp.MediaURLs == nil {
		cp.MediaURLs = []string{}
	}
	return cp
}

// insertPostLocked stores a prepared aggregate; the caller holds m.mu.
func (m *InMemoryStore) insertPostLocked(post *domain.Post, tc *domain.TenantContext, now time.Time) (*domain.Post, error) {
	if post.OrganizationID == "" {
		post.OrganizationID = tc.OrganizationID
	}
	if post.CreatedBy == "" {
		post.CreatedBy = tc.UserID
	}
	if post.ID != "" {
		if _, exists := m.posts[post.ID]; exists {
			return nil, errors.New("entity ID already exists")
		}
	} else {
		post.ID = newUUID()
	}
	post.QueueTier = domain.DetermineQueueTier(post.MediaType)
	if post.MaxRetries <= 0 {
		post.MaxRetries = 3
	}
	if post.MediaURLs == nil {
		post.MediaURLs = []string{}
	}
	post.CreatedAt = now
	post.UpdatedAt = now
	stored := *post
	m.posts[stored.ID] = &stored
	enriched := m.enrichPost(&stored)
	*post = enriched
	return post, nil
}

// QueryPosts lists tenant posts filtered by status, channel, batch and scheduled window.
func (m *InMemoryStore) QueryPosts(ctx context.Context, q domain.PostQuery) ([]domain.Post, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	q.Normalize()
	m.mu.RLock()
	defer m.mu.RUnlock()

	orgID := TenantFromContext(ctx).OrganizationID
	var list []domain.Post
	for _, p := range m.posts {
		if p.OrganizationID != orgID || !q.Matches(p) {
			continue
		}
		list = append(list, m.enrichPost(p))
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.ScheduledAt.Equal(b.ScheduledAt) {
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.ID < b.ID
			}
			if q.Ascending {
				return a.CreatedAt.Before(b.CreatedAt)
			}
			return a.CreatedAt.After(b.CreatedAt)
		}
		if q.Ascending {
			return a.ScheduledAt.Before(b.ScheduledAt)
		}
		return a.ScheduledAt.After(b.ScheduledAt)
	})
	if q.Offset >= len(list) {
		return nil, nil
	}
	list = list[q.Offset:]
	if len(list) > q.Limit {
		list = list[:q.Limit]
	}
	return list, nil
}

// CreatePostsBatch stores an optional batch and all posts atomically under one lock.
func (m *InMemoryStore) CreatePostsBatch(ctx context.Context, batch *domain.PostBatch, posts []*domain.Post) ([]*domain.Post, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return nil, errors.New("no posts to create")
	}
	tc := TenantFromContext(ctx)
	if batch != nil && batch.OrganizationID != "" && batch.OrganizationID != tc.OrganizationID {
		return nil, errors.New("organization mismatch")
	}
	seenChannel := make(map[string]bool)
	for _, post := range posts {
		if post.OrganizationID != "" && post.OrganizationID != tc.OrganizationID {
			return nil, errors.New("organization mismatch")
		}
		if !seenChannel[post.ChannelID] {
			if _, err := m.GetChannelByID(ctx, post.ChannelID); err != nil {
				return nil, err
			}
			seenChannel[post.ChannelID] = true
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	incoming := make(map[string]bool, len(posts))
	for _, post := range posts {
		if post.ID != "" {
			if _, exists := m.posts[post.ID]; exists || incoming[post.ID] {
				return nil, errors.New("entity ID already exists")
			}
			incoming[post.ID] = true
		}
	}
	now := time.Now().UTC()
	if batch != nil {
		if batch.ID == "" {
			batch.ID = newUUID()
		}
		batch.OrganizationID = tc.OrganizationID
		if batch.Kind == "" {
			batch.Kind = domain.PostBatchKindBulk
		}
		if batch.CreatedBy == "" {
			batch.CreatedBy = tc.UserID
		}
		batch.ItemCount = len(posts)
		batch.CreatedAt = now
		stored := *batch
		if batch.Plan != nil {
			plan := *batch.Plan
			stored.Plan = &plan
		}
		m.batches[stored.ID] = &stored
	}
	for _, post := range posts {
		if batch != nil {
			post.BatchID = batch.ID
		}
		if _, err := m.insertPostLocked(post, tc, now); err != nil {
			return nil, err
		}
	}
	return posts, nil
}

// UpdatePost applies a patch to an editable post.
func (m *InMemoryStore) UpdatePost(ctx context.Context, postID string, patch domain.PostPatch) (*domain.Post, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.posts[postID]
	if !ok || p.OrganizationID != TenantFromContext(ctx).OrganizationID {
		return nil, errors.New("post not found")
	}
	updated := *p
	updated.MediaURLs = append([]string{}, p.MediaURLs...)
	if err := updated.ApplyPatch(patch, time.Now().UTC()); err != nil {
		return nil, err
	}
	m.posts[postID] = &updated
	enriched := m.enrichPost(&updated)
	return &enriched, nil
}

// GetPostBatch returns a tenant-scoped batch record.
func (m *InMemoryStore) GetPostBatch(ctx context.Context, id string) (*domain.PostBatch, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.batches[id]
	if !ok || b.OrganizationID != TenantFromContext(ctx).OrganizationID {
		return nil, errors.New("batch not found")
	}
	cp := *b
	if b.Plan != nil {
		plan := *b.Plan
		cp.Plan = &plan
	}
	return &cp, nil
}
