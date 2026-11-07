package domain

import (
	"errors"
	"strings"
	"time"
)

// Library limits.
const (
	MaxLibraryTitleLength = 255
	MaxLibraryQueryLimit  = 200
)

// Library domain errors.
var (
	ErrLibraryItemNotFound = errors.New("library item not found")
	ErrLibraryItemArchived = errors.New("library item is archived; restore it first")
)

// LibraryItem is reusable, ready-made post content ("bài soạn sẵn"). Scheduling copies it into a
// normal Post, so later edits never change posts that are already scheduled. ArchivedAt is the soft
// delete used by "delete after publish": archived items live in the "Đã đăng" section until the user
// restores them or deletes them for good.
type LibraryItem struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organization_id"`
	ChannelID       string     `json:"channel_id,omitempty"`
	ChannelName     string     `json:"channel_name,omitempty"`
	Title           string     `json:"title"`
	Content         string     `json:"content"`
	LinkURL         string     `json:"link_url,omitempty"`
	MediaType       string     `json:"media_type"`
	MediaURLs       []string   `json:"media_urls"`
	PublishCount    int        `json:"publish_count"`
	LastPublishedAt *time.Time `json:"last_published_at,omitempty"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Archived reports whether the item was soft-deleted.
func (l *LibraryItem) Archived() bool { return l.ArchivedAt != nil }

// LibraryPatch carries optional edits for an active library item; nil fields are left untouched.
type LibraryPatch struct {
	Title     *string
	ChannelID *string // empty string clears the preferred Fanpage
	Content   *string
	LinkURL   *string
	MediaType *string
	MediaURLs *[]string
}

// IsEmpty reports whether the patch changes nothing.
func (p LibraryPatch) IsEmpty() bool {
	return p.Title == nil && p.ChannelID == nil && p.Content == nil && p.LinkURL == nil && p.MediaType == nil && p.MediaURLs == nil
}

// ChangesContent reports whether the patch touches post content (as opposed to labels only).
func (p LibraryPatch) ChangesContent() bool {
	return p.Content != nil || p.LinkURL != nil || p.MediaType != nil || p.MediaURLs != nil
}

// LibraryQuery filters a tenant-scoped library listing.
type LibraryQuery struct {
	Archived       bool     // false = active library, true = "Đã đăng" section
	ChannelIDs     []string // restrict to these preferred Fanpages (empty = no channel restriction)
	IncludeUnbound bool     // with ChannelIDs set, also match items that have no preferred Fanpage
	Search         string
	Limit          int
	Offset         int
}

// Normalize clamps paging and trims the search text.
func (q *LibraryQuery) Normalize() {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > MaxLibraryQueryLimit {
		q.Limit = MaxLibraryQueryLimit
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	q.Search = strings.TrimSpace(q.Search)
}

// Matches reports whether an item satisfies the filters (in-memory store and tests).
func (q LibraryQuery) Matches(item *LibraryItem) bool {
	if item.Archived() != q.Archived {
		return false
	}
	if len(q.ChannelIDs) > 0 || q.IncludeUnbound {
		ok := item.ChannelID == "" && q.IncludeUnbound
		for _, id := range q.ChannelIDs {
			if item.ChannelID == id {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if q.Search != "" {
		needle := strings.ToLower(q.Search)
		if !strings.Contains(strings.ToLower(item.Title), needle) && !strings.Contains(strings.ToLower(item.Content), needle) {
			return false
		}
	}
	return true
}

// NormalizeLibraryTitle trims and validates a title.
func NormalizeLibraryTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if len([]rune(title)) > MaxLibraryTitleLength {
		return "", errors.New("title exceeds 255 characters")
	}
	return title, nil
}
