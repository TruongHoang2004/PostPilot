package publishing

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// LibraryInput is the author-editable content of a library item.
type LibraryInput struct {
	Title     string
	ChannelID string
	Content   string
	LinkURL   string
	MediaType string
	MediaURLs []string
}

// libraryFailure maps store sentinels to HTTP errors; anything else is an opaque 500.
func (s *Service) libraryFailure(op string, err error) *Error {
	switch {
	case errors.Is(err, domain.ErrLibraryItemNotFound):
		return fail(http.StatusNotFound, "library item not found")
	case errors.Is(err, domain.ErrLibraryItemArchived):
		return fail(http.StatusConflict, "%s", err.Error())
	}
	return s.storageFailure(op, err)
}

// canAuthorOn reports whether the caller may author on the channel (or on any channel when empty).
func (s *Service) canAuthorOn(ctx context.Context, channelID string) (bool, error) {
	tc := domain.TenantFromContext(ctx)
	if tc.IsOrgAdminOrOwner() {
		return true, nil
	}
	if channelID != "" {
		role, ok := s.perms.ResolveChannelPermission(ctx, channelID)
		return ok && tc.CanCreatePost(role), nil
	}
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return false, err
	}
	for _, ch := range channels {
		if role, ok := s.perms.ResolveChannelPermission(ctx, ch.ID); ok && tc.CanCreatePost(role) {
			return true, nil
		}
	}
	return false, nil
}

// requireAuthor enforces the library authoring rule: org OWNER/ADMIN, an author role on the bound
// channel, or (for unbound items) an author role on at least one channel.
func (s *Service) requireAuthor(ctx context.Context, channelID string) *Error {
	ok, err := s.canAuthorOn(ctx, channelID)
	if err != nil {
		return s.storageFailure("checking library permission", err)
	}
	if !ok {
		return fail(http.StatusForbidden, "forbidden: insufficient permission to manage library posts")
	}
	return nil
}

// canView applies the read rule: channel-bound items need view permission on the channel; unbound
// items are visible to anyone who can view at least one channel.
func (s *Service) canView(ctx context.Context, item *domain.LibraryItem) (bool, error) {
	tc := domain.TenantFromContext(ctx)
	if tc.IsOrgAdminOrOwner() {
		return true, nil
	}
	if item.ChannelID != "" {
		role, ok := s.perms.ResolveChannelPermission(ctx, item.ChannelID)
		return ok && domain.CanViewChannel(role), nil
	}
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return false, err
	}
	for _, ch := range channels {
		if role, ok := s.perms.ResolveChannelPermission(ctx, ch.ID); ok && domain.CanViewChannel(role) {
			return true, nil
		}
	}
	return false, nil
}

// loadVisible fetches an item of the current tenant; items the caller may not view look like 404.
func (s *Service) loadVisible(ctx context.Context, id string) (*domain.LibraryItem, *Error) {
	item, err := s.store.GetLibraryItem(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, s.libraryFailure("loading library item", err)
	}
	ok, err := s.canView(ctx, item)
	if err != nil {
		return nil, s.storageFailure("checking library permission", err)
	}
	if !ok {
		return nil, fail(http.StatusNotFound, "library item not found")
	}
	return item, nil
}

// requireChannel makes sure a preferred Fanpage belongs to the current workspace.
func (s *Service) requireChannel(ctx context.Context, channelID string) *Error {
	if channelID == "" {
		return nil
	}
	if ch, err := s.store.GetChannelByID(ctx, channelID); err != nil || ch == nil {
		return fail(http.StatusBadRequest, "channel_id does not exist: %s", channelID)
	}
	return nil
}

// GetLibraryItem returns one visible library item.
func (s *Service) GetLibraryItem(ctx context.Context, id string) (*domain.LibraryItem, *Error) {
	return s.loadVisible(ctx, id)
}

// CreateLibraryItem stores reusable content after validating it with the shared draft rules.
func (s *Service) CreateLibraryItem(ctx context.Context, in LibraryInput) (*domain.LibraryItem, *Error) {
	title, err := domain.NormalizeLibraryTitle(in.Title)
	if err != nil {
		return nil, fail(http.StatusBadRequest, "%s", err.Error())
	}
	content, link, mediaType, urls, err := domain.NormalizeDraft(in.Content, in.LinkURL, in.MediaType, in.MediaURLs)
	if err != nil {
		return nil, fail(http.StatusBadRequest, "%s", err.Error())
	}
	channelID := strings.TrimSpace(in.ChannelID)
	if ferr := s.requireAuthor(ctx, channelID); ferr != nil {
		return nil, ferr
	}
	if ferr := s.requireChannel(ctx, channelID); ferr != nil {
		return nil, ferr
	}
	tc := domain.TenantFromContext(ctx)
	created, serr := s.store.CreateLibraryItem(ctx, &domain.LibraryItem{ChannelID: channelID, Title: title, Content: content, LinkURL: link, MediaType: mediaType, MediaURLs: urls, CreatedBy: tc.UserID})
	if serr != nil {
		return nil, s.libraryFailure("creating library item", serr)
	}
	s.audit(ctx, "LIBRARY_CREATE", "LIBRARY_ITEM", created.ID, map[string]any{"channel_id": channelID})
	return created, nil
}

// UpdateLibraryItem edits an active item. Content changes are validated on the merged result.
func (s *Service) UpdateLibraryItem(ctx context.Context, id string, patch domain.LibraryPatch) (*domain.LibraryItem, *Error) {
	item, ferr := s.loadVisible(ctx, id)
	if ferr != nil {
		return nil, ferr
	}
	if patch.IsEmpty() {
		return nil, fail(http.StatusBadRequest, "nothing to update")
	}
	if item.Archived() {
		return nil, fail(http.StatusConflict, "%s", domain.ErrLibraryItemArchived.Error())
	}
	if ferr := s.requireAuthor(ctx, item.ChannelID); ferr != nil {
		return nil, ferr
	}
	if patch.ChannelID != nil {
		trimmed := strings.TrimSpace(*patch.ChannelID)
		patch.ChannelID = &trimmed
		if ferr := s.requireAuthor(ctx, trimmed); ferr != nil {
			return nil, ferr
		}
		if ferr := s.requireChannel(ctx, trimmed); ferr != nil {
			return nil, ferr
		}
	}
	if patch.Title != nil {
		title, err := domain.NormalizeLibraryTitle(*patch.Title)
		if err != nil {
			return nil, fail(http.StatusBadRequest, "%s", err.Error())
		}
		patch.Title = &title
	}
	merged := *item
	if patch.Content != nil {
		merged.Content = *patch.Content
	}
	if patch.LinkURL != nil {
		merged.LinkURL = *patch.LinkURL
	}
	if patch.MediaType != nil {
		merged.MediaType = *patch.MediaType
	}
	if patch.MediaURLs != nil {
		merged.MediaURLs = *patch.MediaURLs
	}
	content, link, mediaType, urls, err := domain.NormalizeDraft(merged.Content, merged.LinkURL, merged.MediaType, merged.MediaURLs)
	if err != nil {
		return nil, fail(http.StatusBadRequest, "%s", err.Error())
	}
	if patch.ChangesContent() {
		patch.Content, patch.LinkURL, patch.MediaType, patch.MediaURLs = &content, &link, &mediaType, &urls
	}
	updated, serr := s.store.UpdateLibraryItem(ctx, item.ID, patch)
	if serr != nil {
		return nil, s.libraryFailure("updating library item", serr)
	}
	s.audit(ctx, "LIBRARY_UPDATE", "LIBRARY_ITEM", item.ID, map[string]any{"content_changed": patch.ChangesContent()})
	return updated, nil
}

// DeleteLibraryItem removes an item permanently (posts already scheduled from it are kept).
func (s *Service) DeleteLibraryItem(ctx context.Context, id string) *Error {
	item, ferr := s.loadVisible(ctx, id)
	if ferr != nil {
		return ferr
	}
	if ferr := s.requireAuthor(ctx, item.ChannelID); ferr != nil {
		return ferr
	}
	if err := s.store.DeleteLibraryItem(ctx, item.ID); err != nil {
		return s.libraryFailure("deleting library item", err)
	}
	s.audit(ctx, "LIBRARY_DELETE", "LIBRARY_ITEM", item.ID, map[string]any{"was_archived": item.Archived()})
	return nil
}

// RestoreLibraryItem brings an archived item back to the active library ("Giữ lại").
func (s *Service) RestoreLibraryItem(ctx context.Context, id string) (*domain.LibraryItem, *Error) {
	item, ferr := s.loadVisible(ctx, id)
	if ferr != nil {
		return nil, ferr
	}
	if ferr := s.requireAuthor(ctx, item.ChannelID); ferr != nil {
		return nil, ferr
	}
	restored, err := s.store.RestoreLibraryItem(ctx, item.ID)
	if err != nil {
		return nil, s.libraryFailure("restoring library item", err)
	}
	s.audit(ctx, "LIBRARY_RESTORE", "LIBRARY_ITEM", item.ID, nil)
	return restored, nil
}

// LibrarySchedule describes when and how a library item is published.
type LibrarySchedule struct {
	ScheduledAt        time.Time
	DeleteAfterPublish bool
	SubmitForApproval  bool
	MaxRetries         int
}

// ScheduleLibraryItem copies an active library item into normal posts on the resolved targets. All
// RBAC and approval rules are those of Create, so a library post can never bypass review. With no
// explicit target the item's preferred Fanpage is used.
func (s *Service) ScheduleLibraryItem(ctx context.Context, id string, targets Targets, sched LibrarySchedule) (*CreateResult, *Error) {
	item, ferr := s.loadVisible(ctx, id)
	if ferr != nil {
		return nil, ferr
	}
	if item.Archived() {
		return nil, fail(http.StatusConflict, "%s", domain.ErrLibraryItemArchived.Error())
	}
	if sched.ScheduledAt.IsZero() {
		return nil, fail(http.StatusBadRequest, "scheduled_at is required")
	}
	// delete_after_publish archives the item later, so it needs the same right as deleting it by hand;
	// otherwise a member who may only post elsewhere could hide another Fanpage's library item.
	if sched.DeleteAfterPublish {
		if ferr := s.requireAuthor(ctx, item.ChannelID); ferr != nil {
			return nil, ferr
		}
	}
	if strings.TrimSpace(targets.ChannelID) == "" && len(targets.ChannelIDs) == 0 && strings.TrimSpace(targets.ChannelGroupID) == "" {
		targets.ChannelID = item.ChannelID
	}
	draft := Draft{
		Content: item.Content, LinkURL: item.LinkURL, MediaType: item.MediaType, MediaURLs: item.MediaURLs,
		ScheduledAt: sched.ScheduledAt.UTC(), MaxRetries: sched.MaxRetries,
		LibraryItemID: item.ID, DeleteAfterPublish: sched.DeleteAfterPublish,
	}
	result, ferr := s.Create(ctx, targets, draft, Options{SubmitForApproval: sched.SubmitForApproval})
	if ferr != nil {
		return nil, ferr
	}
	s.audit(ctx, "LIBRARY_SCHEDULE", "LIBRARY_ITEM", item.ID, map[string]any{"posts": len(result.Posts), "batch_id": result.BatchID, "scheduled_at": draft.ScheduledAt, "delete_after_publish": sched.DeleteAfterPublish})
	return result, nil
}
