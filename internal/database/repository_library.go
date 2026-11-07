package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// ----------------------------------------------------------------------------
// Library items (reusable ready-made posts) — every statement is scoped by organization_id.
// ----------------------------------------------------------------------------

const librarySelectColumns = `l.id, l.organization_id::text, COALESCE(l.channel_id::text, ''), COALESCE(c.name, ''),
	l.title, l.content, l.link_url, l.media_type, l.media_urls::text, l.publish_count::text,
	COALESCE(l.last_published_at::text, ''), COALESCE(l.archived_at::text, ''), l.created_by, l.created_at::text, l.updated_at::text`

const librarySelectFrom = ` FROM library_items l
	LEFT JOIN channels c ON c.id = l.channel_id AND c.organization_id = l.organization_id `

func scanLibraryItem(sc rowScanner) (*domain.LibraryItem, error) {
	var item domain.LibraryItem
	var orgID, mediaStr, countStr, lastStr, archivedStr, createdStr, updatedStr string
	if err := sc.Scan(&item.ID, &orgID, &item.ChannelID, &item.ChannelName, &item.Title, &item.Content, &item.LinkURL, &item.MediaType,
		&mediaStr, &countStr, &lastStr, &archivedStr, &item.CreatedBy, &createdStr, &updatedStr); err != nil {
		return nil, err
	}
	item.OrganizationID = orgID
	_ = json.Unmarshal([]byte(mediaStr), &item.MediaURLs)
	if item.MediaURLs == nil {
		item.MediaURLs = []string{}
	}
	item.PublishCount, _ = strconv.Atoi(countStr)
	if lastStr != "" {
		t := parsePGTime(lastStr)
		item.LastPublishedAt = &t
	}
	if archivedStr != "" {
		t := parsePGTime(archivedStr)
		item.ArchivedAt = &t
	}
	item.CreatedAt = parsePGTime(createdStr)
	item.UpdatedAt = parsePGTime(updatedStr)
	return &item, nil
}

// likePattern escapes LIKE wildcards so user search text is always a literal substring.
func likePattern(raw string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(raw) + "%"
}

func (s *PostgresStore) requireLibraryChannel(ctx context.Context, channelID string) error {
	if channelID == "" {
		return nil
	}
	if _, err := s.GetChannelByID(ctx, channelID); err != nil {
		return errors.New("channel not found")
	}
	return nil
}

func (s *PostgresStore) CreateLibraryItem(ctx context.Context, item *domain.LibraryItem) (*domain.LibraryItem, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	tc := TenantFromContext(ctx)
	if item.OrganizationID != "" && item.OrganizationID != tc.OrganizationID {
		return nil, errors.New("organization mismatch")
	}
	if err := s.requireLibraryChannel(ctx, item.ChannelID); err != nil {
		return nil, err
	}
	if item.MediaURLs == nil {
		item.MediaURLs = []string{}
	}
	mediaJSON, err := json.Marshal(item.MediaURLs)
	if err != nil {
		return nil, err
	}
	createdBy := item.CreatedBy
	if createdBy == "" {
		createdBy = tc.UserID
	}
	var id string
	if err := s.db.QueryRowContext(ctx, `
	INSERT INTO library_items (organization_id, channel_id, title, content, link_url, media_type, media_urls, created_by)
	VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7::jsonb, $8)
	RETURNING id;`, tc.OrganizationID, item.ChannelID, item.Title, item.Content, item.LinkURL, item.MediaType, string(mediaJSON), createdBy).Scan(&id); err != nil {
		return nil, err
	}
	return s.GetLibraryItem(ctx, id)
}

func (s *PostgresStore) GetLibraryItem(ctx context.Context, id string) (*domain.LibraryItem, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	if !isUUID(id) {
		return nil, domain.ErrLibraryItemNotFound
	}
	item, err := scanLibraryItem(s.db.QueryRowContext(ctx, "SELECT "+librarySelectColumns+librarySelectFrom+" WHERE l.id = $1::uuid AND l.organization_id = $2;", id, TenantFromContext(ctx).OrganizationID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrLibraryItemNotFound
	}
	return item, err
}

func (s *PostgresStore) QueryLibraryItems(ctx context.Context, q domain.LibraryQuery) ([]domain.LibraryItem, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	q.Normalize()
	args := []any{TenantFromContext(ctx).OrganizationID}
	where := []string{"l.organization_id = $1"}
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if q.Archived {
		where = append(where, "l.archived_at IS NOT NULL")
	} else {
		where = append(where, "l.archived_at IS NULL")
	}
	if len(q.ChannelIDs) > 0 || q.IncludeUnbound {
		var terms []string
		ph := make([]string, 0, len(q.ChannelIDs))
		for _, id := range q.ChannelIDs {
			if isUUID(id) {
				ph = append(ph, next(id)+"::uuid")
			}
		}
		if len(ph) > 0 {
			terms = append(terms, "l.channel_id IN ("+strings.Join(ph, ", ")+")")
		}
		if q.IncludeUnbound {
			terms = append(terms, "l.channel_id IS NULL")
		}
		if len(terms) == 0 {
			return nil, nil // every requested channel id was malformed: nothing can match
		}
		where = append(where, "("+strings.Join(terms, " OR ")+")")
	}
	if q.Search != "" {
		ph := next(likePattern(q.Search))
		where = append(where, "(l.title ILIKE "+ph+" OR l.content ILIKE "+ph+")")
	}
	query := "SELECT " + librarySelectColumns + librarySelectFrom + " WHERE " + strings.Join(where, " AND ") +
		" ORDER BY l.updated_at DESC, l.id LIMIT " + next(q.Limit) + " OFFSET " + next(q.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.LibraryItem, 0)
	for rows.Next() {
		item, err := scanLibraryItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *PostgresStore) UpdateLibraryItem(ctx context.Context, id string, patch domain.LibraryPatch) (*domain.LibraryItem, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	if !isUUID(id) {
		return nil, domain.ErrLibraryItemNotFound
	}
	if patch.ChannelID != nil {
		if err := s.requireLibraryChannel(ctx, *patch.ChannelID); err != nil {
			return nil, err
		}
	}
	args := []any{id, TenantFromContext(ctx).OrganizationID}
	sets := []string{"updated_at = NOW()"}
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if patch.Title != nil {
		sets = append(sets, "title = "+next(*patch.Title))
	}
	if patch.ChannelID != nil {
		sets = append(sets, "channel_id = NULLIF("+next(*patch.ChannelID)+", '')::uuid")
	}
	if patch.Content != nil {
		sets = append(sets, "content = "+next(*patch.Content))
	}
	if patch.LinkURL != nil {
		sets = append(sets, "link_url = "+next(*patch.LinkURL))
	}
	if patch.MediaType != nil {
		sets = append(sets, "media_type = "+next(*patch.MediaType))
	}
	if patch.MediaURLs != nil {
		raw, err := json.Marshal(*patch.MediaURLs)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "media_urls = "+next(string(raw))+"::jsonb")
	}
	res, err := s.db.ExecContext(ctx, "UPDATE library_items SET "+strings.Join(sets, ", ")+" WHERE id = $1::uuid AND organization_id = $2 AND archived_at IS NULL", args...)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		existing, gerr := s.GetLibraryItem(ctx, id)
		if gerr != nil {
			return nil, gerr
		}
		if existing.Archived() {
			return nil, domain.ErrLibraryItemArchived
		}
		return nil, domain.ErrLibraryItemNotFound
	}
	return s.GetLibraryItem(ctx, id)
}

func (s *PostgresStore) DeleteLibraryItem(ctx context.Context, id string) error {
	if err := requireOrganization(ctx); err != nil {
		return err
	}
	if !isUUID(id) {
		return domain.ErrLibraryItemNotFound
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM library_items WHERE id = $1::uuid AND organization_id = $2", id, TenantFromContext(ctx).OrganizationID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrLibraryItemNotFound
	}
	return nil
}

// pendingPostStatusList renders the statuses that can still publish as a SQL IN list (constants only).
func pendingPostStatusList() string {
	quoted := make([]string, 0, len(domain.ActivePostStatuses()))
	for _, st := range domain.ActivePostStatuses() {
		quoted = append(quoted, "'"+st+"'")
	}
	return strings.Join(quoted, ",")
}

// RestoreLibraryItem brings an archived item back ("Giữ lại"). The user's decision to keep the content
// must stick, so pending posts of this item that still carry delete_after_publish drop the flag in the
// same transaction; otherwise the next one to publish would archive the item again. Lock order is
// always library item -> posts (same as DeleteLibraryItem's ON DELETE SET NULL).
func (s *PostgresStore) RestoreLibraryItem(ctx context.Context, id string) (*domain.LibraryItem, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	if !isUUID(id) {
		return nil, domain.ErrLibraryItemNotFound
	}
	orgID := TenantFromContext(ctx).OrganizationID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "UPDATE library_items SET archived_at = NULL, updated_at = NOW() WHERE id = $1::uuid AND organization_id = $2 AND archived_at IS NOT NULL", id, orgID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		if _, err := tx.ExecContext(ctx, "UPDATE posts SET delete_after_publish = FALSE, updated_at = NOW() WHERE library_item_id = $1::uuid AND organization_id = $2 AND delete_after_publish AND status IN ("+pendingPostStatusList()+")", id, orgID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetLibraryItem(ctx, id)
}

// withSavepoint runs fn and, when it fails, undoes only fn's effects so the surrounding transaction stays usable.
func withSavepoint(ctx context.Context, tx *sql.Tx, name string, fn func() error) error {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return err
	}
	if err := fn(); err != nil {
		if _, rerr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name); rerr != nil {
			return errors.Join(err, rerr)
		}
		return err
	}
	_, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name)
	return err
}

// MarkPostPublished stores the publish result and, for posts scheduled from the library, updates the
// source item in the same transaction: the publish counter always moves; the item is soft-deleted
// when the post carries delete_after_publish. A repeated call for an already published post is a no-op
// for the library so counters are never inflated.
//
// The Facebook post already exists when this runs, so recording PUBLISHED must never be lost to library
// bookkeeping: locks are taken in the order library item -> post (the order DeleteLibraryItem's
// ON DELETE SET NULL uses, so the two can not deadlock) and every library statement runs inside a
// savepoint whose failure is logged and rolled back without touching the PUBLISHED update.
func (s *PostgresStore) MarkPostPublished(ctx context.Context, postID, externalPostID string, publishedAt time.Time) error {
	if err := requireOrganization(ctx); err != nil {
		return err
	}
	orgID := TenantFromContext(ctx).OrganizationID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var hint string
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(library_item_id::text, '') FROM posts WHERE id = $1 AND organization_id = $2", postID, orgID).Scan(&hint); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("entity unavailable or state conflict")
		}
		return err
	}
	libraryLocked := false
	if hint != "" {
		if err := withSavepoint(ctx, tx, "lib_lock", func() error {
			var locked string
			err := tx.QueryRowContext(ctx, "SELECT id::text FROM library_items WHERE id = $1::uuid AND organization_id = $2 FOR NO KEY UPDATE", hint, orgID).Scan(&locked)
			if errors.Is(err, sql.ErrNoRows) {
				return nil // item deleted meanwhile: nothing to update
			}
			libraryLocked = err == nil
			return err
		}); err != nil {
			slog.Warn("library item lock failed while marking post published; library bookkeeping skipped", "post_id", postID, "library_item_id", hint, "error", err)
		}
	}
	var status, libraryID, deleteFlag string
	if err := tx.QueryRowContext(ctx, "SELECT status, COALESCE(library_item_id::text, ''), delete_after_publish::text FROM posts WHERE id = $1 AND organization_id = $2 FOR UPDATE", postID, orgID).Scan(&status, &libraryID, &deleteFlag); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("entity unavailable or state conflict")
		}
		return err
	}
	res, err := tx.ExecContext(ctx, "UPDATE posts SET status='PUBLISHED',external_post_id=$1,published_at=$2,last_error_code=0,last_error_message='',updated_at=NOW() WHERE id=$3 AND organization_id=$4", externalPostID, publishedAt, postID, orgID)
	if err := affected(res, err); err != nil {
		return err
	}
	if libraryLocked && libraryID == hint && status != domain.PostStatusPublished {
		archive := deleteFlag == "true" || deleteFlag == "t"
		if err := withSavepoint(ctx, tx, "lib_update", func() error {
			_, err := tx.ExecContext(ctx, `UPDATE library_items SET publish_count = publish_count + 1, last_published_at = $1,
				archived_at = CASE WHEN $2::boolean THEN COALESCE(archived_at, NOW()) ELSE archived_at END, updated_at = NOW()
				WHERE id = $3::uuid AND organization_id = $4`, publishedAt, archive, libraryID, orgID)
			return err
		}); err != nil {
			slog.Warn("library bookkeeping failed after publish; the post stays PUBLISHED", "post_id", postID, "library_item_id", libraryID, "error", err)
		}
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// InMemoryStore
// ---------------------------------------------------------------------------

func (m *InMemoryStore) enrichLibraryItemLocked(item *domain.LibraryItem) domain.LibraryItem {
	cp := *item
	if cp.MediaURLs == nil {
		cp.MediaURLs = []string{}
	} else {
		cp.MediaURLs = append([]string{}, cp.MediaURLs...)
	}
	cp.ChannelName = ""
	if cp.ChannelID != "" {
		if ch, ok := m.channels[cp.ChannelID]; ok && ch.OrganizationID == cp.OrganizationID {
			cp.ChannelName = ch.Name
		}
	}
	return cp
}

func (m *InMemoryStore) channelInOrgLocked(channelID, orgID string) bool {
	ch, ok := m.channels[channelID]
	return ok && ch.OrganizationID == orgID
}

func (m *InMemoryStore) CreateLibraryItem(ctx context.Context, item *domain.LibraryItem) (*domain.LibraryItem, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	tc := TenantFromContext(ctx)
	if item.OrganizationID != "" && item.OrganizationID != tc.OrganizationID {
		return nil, errors.New("organization mismatch")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if item.ChannelID != "" && !m.channelInOrgLocked(item.ChannelID, tc.OrganizationID) {
		return nil, errors.New("channel not found")
	}
	stored := *item
	stored.ID = newUUID()
	stored.OrganizationID = tc.OrganizationID
	if stored.CreatedBy == "" {
		stored.CreatedBy = tc.UserID
	}
	stored.MediaURLs = append([]string{}, item.MediaURLs...)
	stored.PublishCount, stored.LastPublishedAt, stored.ArchivedAt = 0, nil, nil
	now := time.Now().UTC()
	stored.CreatedAt, stored.UpdatedAt = now, now
	m.library[stored.ID] = &stored
	out := m.enrichLibraryItemLocked(&stored)
	return &out, nil
}

func (m *InMemoryStore) libraryItemLocked(ctx context.Context, id string) (*domain.LibraryItem, error) {
	item, ok := m.library[id]
	if !ok || item.OrganizationID != TenantFromContext(ctx).OrganizationID {
		return nil, domain.ErrLibraryItemNotFound
	}
	return item, nil
}

func (m *InMemoryStore) GetLibraryItem(ctx context.Context, id string) (*domain.LibraryItem, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, err := m.libraryItemLocked(ctx, id)
	if err != nil {
		return nil, err
	}
	out := m.enrichLibraryItemLocked(item)
	return &out, nil
}

func (m *InMemoryStore) QueryLibraryItems(ctx context.Context, q domain.LibraryQuery) ([]domain.LibraryItem, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	q.Normalize()
	orgID := TenantFromContext(ctx).OrganizationID
	m.mu.RLock()
	defer m.mu.RUnlock()
	matched := make([]domain.LibraryItem, 0)
	for _, item := range m.library {
		if item.OrganizationID != orgID || !q.Matches(item) {
			continue
		}
		matched = append(matched, m.enrichLibraryItemLocked(item))
	}
	sort.Slice(matched, func(i, j int) bool {
		if !matched[i].UpdatedAt.Equal(matched[j].UpdatedAt) {
			return matched[i].UpdatedAt.After(matched[j].UpdatedAt)
		}
		return matched[i].ID < matched[j].ID
	})
	if q.Offset >= len(matched) {
		return []domain.LibraryItem{}, nil
	}
	matched = matched[q.Offset:]
	if len(matched) > q.Limit {
		matched = matched[:q.Limit]
	}
	return matched, nil
}

func (m *InMemoryStore) UpdateLibraryItem(ctx context.Context, id string, patch domain.LibraryPatch) (*domain.LibraryItem, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item, err := m.libraryItemLocked(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.Archived() {
		return nil, domain.ErrLibraryItemArchived
	}
	if patch.ChannelID != nil && *patch.ChannelID != "" && !m.channelInOrgLocked(*patch.ChannelID, item.OrganizationID) {
		return nil, errors.New("channel not found")
	}
	if patch.Title != nil {
		item.Title = *patch.Title
	}
	if patch.ChannelID != nil {
		item.ChannelID = *patch.ChannelID
	}
	if patch.Content != nil {
		item.Content = *patch.Content
	}
	if patch.LinkURL != nil {
		item.LinkURL = *patch.LinkURL
	}
	if patch.MediaType != nil {
		item.MediaType = *patch.MediaType
	}
	if patch.MediaURLs != nil {
		item.MediaURLs = append([]string{}, (*patch.MediaURLs)...)
	}
	item.UpdatedAt = time.Now().UTC()
	out := m.enrichLibraryItemLocked(item)
	return &out, nil
}

func (m *InMemoryStore) DeleteLibraryItem(ctx context.Context, id string) error {
	if err := requireOrganization(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item, err := m.libraryItemLocked(ctx, id)
	if err != nil {
		return err
	}
	delete(m.library, item.ID)
	for _, p := range m.posts { // mirrors ON DELETE SET NULL
		if p.LibraryItemID == item.ID {
			p.LibraryItemID = ""
		}
	}
	return nil
}

func (m *InMemoryStore) RestoreLibraryItem(ctx context.Context, id string) (*domain.LibraryItem, error) {
	if err := requireOrganization(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item, err := m.libraryItemLocked(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.Archived() {
		item.ArchivedAt = nil
		item.UpdatedAt = time.Now().UTC()
		// "Giữ lại" must stick: pending posts of this item stop carrying the delete-after-publish flag.
		for _, p := range m.posts {
			if p.LibraryItemID == item.ID && p.OrganizationID == item.OrganizationID && p.DeleteAfterPublish && isPendingPostStatus(p.Status) {
				p.DeleteAfterPublish = false
				p.UpdatedAt = item.UpdatedAt
			}
		}
	}
	out := m.enrichLibraryItemLocked(item)
	return &out, nil
}

func isPendingPostStatus(status string) bool {
	for _, st := range domain.ActivePostStatuses() {
		if st == status {
			return true
		}
	}
	return false
}

func (m *InMemoryStore) MarkPostPublished(ctx context.Context, postID string, externalPostID string, publishedAt time.Time) error {
	if err := requireOrganization(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.posts[postID]
	if !ok || p.OrganizationID != TenantFromContext(ctx).OrganizationID {
		return errors.New("post not found")
	}
	wasPublished := p.Status == domain.PostStatusPublished
	p.Status = domain.PostStatusPublished
	p.ExternalPostID = externalPostID
	p.PublishedAt = &publishedAt
	p.LastErrorCode = 0
	p.LastErrorMessage = ""
	now := time.Now().UTC()
	p.UpdatedAt = now
	if p.LibraryItemID != "" && !wasPublished {
		if item, ok := m.library[p.LibraryItemID]; ok && item.OrganizationID == p.OrganizationID {
			item.PublishCount++
			at := publishedAt
			item.LastPublishedAt = &at
			if p.DeleteAfterPublish && item.ArchivedAt == nil {
				item.ArchivedAt = &now
			}
			item.UpdatedAt = now
		}
	}
	return nil
}
