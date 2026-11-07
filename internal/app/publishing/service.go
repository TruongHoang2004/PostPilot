// Package publishing is the application service for the Post & Publishing bounded context.
// It owns target resolution, RBAC/approval decisions, bulk planning and queue dispatch so HTTP
// handlers stay thin and the rules are testable without a transport.
package publishing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/database"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// JobQueue dispatches publish jobs (implemented by the Redis client).
type JobQueue interface {
	EnqueuePublishJob(ctx context.Context, payload domain.PublishJobPayload) error
}

// PermissionResolver returns the effective channel role of the current tenant context.
type PermissionResolver interface {
	ResolveChannelPermission(ctx context.Context, channelID string) (domain.GroupRole, bool)
}

// Error carries an HTTP-mappable status with a user-facing message.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

func fail(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// storageFailure logs the underlying error and returns a generic 500 so driver/schema details never reach clients.
func (s *Service) storageFailure(op string, err error) *Error {
	s.logger.Error("publishing storage failure", "op", op, "error", err)
	return fail(http.StatusInternalServerError, "storage error while %s", op)
}

// requireApproval reports whether the channel sits in at least one approval-required group.
// Storage failures surface as an error so the approval gate fails closed.
func (s *Service) requireApproval(ctx context.Context, orgID, channelID string) (bool, error) {
	set, err := s.approvalRequiredChannels(ctx, orgID)
	if err != nil {
		return false, err
	}
	return set[channelID], nil
}

// Service orchestrates post creation, bulk planning and edits.
type Service struct {
	store  database.Store
	queue  JobQueue
	perms  PermissionResolver
	logger *slog.Logger
	now    func() time.Time
}

// NewService wires the application service.
func NewService(store database.Store, queue JobQueue, perms PermissionResolver, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, queue: queue, perms: perms, logger: logger, now: func() time.Time { return time.Now().UTC() }}
}

// WithStore swaps the storage port (tests inject failing stores to prove fail-closed behaviour).
func (s *Service) WithStore(store database.Store) *Service {
	if store != nil {
		s.store = store
	}
	return s
}

// WithClock overrides the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// Targets identifies where posts go: one channel, several channels or a channel group.
type Targets struct {
	ChannelID      string
	ChannelIDs     []string
	ChannelGroupID string
}

// ResolvedTarget is a validated channel with the caller's effective role.
type ResolvedTarget struct {
	Channel         domain.Channel
	Role            domain.GroupRole
	RequireApproval bool
}

// Resolution is the outcome of target validation.
type Resolution struct {
	Targets []ResolvedTarget
	GroupID string
	FanOut  bool // more than one channel or a group was addressed
}

// ChannelIDs lists the resolved channel identifiers in request order.
func (r *Resolution) ChannelIDs() []string {
	ids := make([]string, 0, len(r.Targets))
	for _, t := range r.Targets {
		ids = append(ids, t.Channel.ID)
	}
	return ids
}

// ResolveTargets validates channels, permissions and approval requirements exactly as single
// creation always did: existence (400), permission (403), VIEWER (403), EDITOR + publish_now on an
// approval-required channel (403).
func (s *Service) ResolveTargets(ctx context.Context, t Targets, publishNow bool) (*Resolution, *Error) {
	tc := domain.TenantFromContext(ctx)
	var ids []string
	groupRequireApproval := false
	res := &Resolution{}

	switch {
	case strings.TrimSpace(t.ChannelGroupID) != "":
		cg, err := s.store.GetChannelGroupByID(ctx, tc.OrganizationID, strings.TrimSpace(t.ChannelGroupID))
		if err != nil || cg == nil {
			return nil, fail(http.StatusBadRequest, "channel_group_id does not exist")
		}
		groupRequireApproval = cg.RequireApproval
		for _, ch := range cg.Channels {
			ids = append(ids, ch.ID)
		}
		if len(ids) == 0 {
			return nil, fail(http.StatusBadRequest, "channel group contains no channels")
		}
		res.GroupID = cg.ID
		res.FanOut = true
	case len(t.ChannelIDs) > 0:
		seen := make(map[string]bool, len(t.ChannelIDs))
		for _, id := range t.ChannelIDs {
			trimmed := strings.TrimSpace(id)
			if trimmed != "" && !seen[trimmed] {
				seen[trimmed] = true
				ids = append(ids, trimmed)
			}
		}
		res.FanOut = len(ids) > 1
	case strings.TrimSpace(t.ChannelID) != "":
		ids = []string{strings.TrimSpace(t.ChannelID)}
	}
	if len(ids) == 0 {
		return nil, fail(http.StatusBadRequest, "channel_id, channel_ids, or channel_group_id is required")
	}

	approvalChannels, err := s.approvalRequiredChannels(ctx, tc.OrganizationID)
	if err != nil {
		s.logger.Error("approval lookup failed", "error", err)
		return nil, fail(http.StatusServiceUnavailable, "cannot verify approval rules right now")
	}
	for _, chID := range ids {
		channel, err := s.store.GetChannelByID(ctx, chID)
		if err != nil || channel == nil {
			return nil, fail(http.StatusBadRequest, "channel_id does not exist: %s", chID)
		}
		role, ok := s.perms.ResolveChannelPermission(ctx, chID)
		if !ok && !tc.IsOrgAdminOrOwner() {
			return nil, fail(http.StatusForbidden, "forbidden: you do not have permission to post to channel %s", chID)
		}
		if tc.IsOrgAdminOrOwner() && (!ok || role == "") {
			role = domain.GroupRoleManager
		}
		if role == domain.GroupRoleViewer {
			return nil, fail(http.StatusForbidden, "forbidden: role VIEWER cannot create posts")
		}
		requireApproval := groupRequireApproval || approvalChannels[chID]
		if role == domain.GroupRoleEditor && requireApproval && publishNow {
			return nil, fail(http.StatusForbidden, "role EDITOR cannot publish directly; please submit for approval")
		}
		res.Targets = append(res.Targets, ResolvedTarget{Channel: *channel, Role: role, RequireApproval: requireApproval})
	}
	return res, nil
}

// approvalRequiredChannels collects channel IDs that belong to at least one approval-required group.
// Any storage error is returned (never swallowed) so callers fail closed.
func (s *Service) approvalRequiredChannels(ctx context.Context, orgID string) (map[string]bool, error) {
	result := make(map[string]bool)
	groups, err := s.store.ListChannelGroups(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if !g.RequireApproval {
			continue
		}
		detail, err := s.store.GetChannelGroupByID(ctx, orgID, g.ID)
		if err != nil {
			return nil, err
		}
		if detail == nil {
			continue
		}
		for _, c := range detail.Channels {
			result[c.ID] = true
		}
	}
	return result, nil
}

// Draft is the content of one post before it is bound to channels.
type Draft struct {
	Content     string
	LinkURL     string
	MediaType   string
	MediaURLs   []string
	ScheduledAt time.Time
	MaxRetries  int
	// LibraryItemID links the post to the library item it was copied from; DeleteAfterPublish then
	// soft-deletes that item once the post is published.
	LibraryItemID      string
	DeleteAfterPublish bool
}

// Options control how the draft enters the lifecycle.
type Options struct {
	PublishNow        bool
	SubmitForApproval bool
}

// buildPost applies the approval rules shared by single and bulk creation.
func buildPost(tc *domain.TenantContext, target ResolvedTarget, draft Draft, opts Options, now time.Time, groupID string) *domain.Post {
	mustPendingApproval := opts.SubmitForApproval || (target.Role == domain.GroupRoleEditor && target.RequireApproval)
	publishNow := opts.PublishNow && !mustPendingApproval
	post := &domain.Post{
		OrganizationID: tc.OrganizationID,
		ChannelGroupID: groupID,
		ChannelID:      target.Channel.ID,
		Content:        draft.Content,
		LinkURL:        draft.LinkURL,
		MediaType:      draft.MediaType,
		MediaURLs:      append([]string{}, draft.MediaURLs...),
		ScheduledAt:    draft.ScheduledAt,
		MaxRetries:     draft.MaxRetries,
		CreatedBy:      tc.UserID,
		SubmittedBy:    tc.UserID,

		LibraryItemID:      draft.LibraryItemID,
		DeleteAfterPublish: draft.DeleteAfterPublish && draft.LibraryItemID != "",
	}
	if mustPendingApproval {
		post.PrepareForCreation(false, now)
		post.Status = domain.PostStatusPendingApproval
	} else {
		post.PrepareForCreation(publishNow, now)
	}
	return post
}

// CreateResult is the outcome of single-draft creation.
type CreateResult struct {
	BatchID string
	Posts   []*domain.Post
	FanOut  bool
}

// Create stores one draft on every resolved target and dispatches publish_now jobs.
func (s *Service) Create(ctx context.Context, targets Targets, draft Draft, opts Options) (*CreateResult, *Error) {
	tc := domain.TenantFromContext(ctx)
	content, link, mediaType, urls, err := domain.NormalizeDraft(draft.Content, draft.LinkURL, draft.MediaType, draft.MediaURLs)
	if err != nil {
		return nil, fail(http.StatusBadRequest, "%s", err.Error())
	}
	draft.Content, draft.LinkURL, draft.MediaType, draft.MediaURLs = content, link, mediaType, urls
	res, ferr := s.ResolveTargets(ctx, targets, opts.PublishNow)
	if ferr != nil {
		return nil, ferr
	}
	now := s.now()
	posts := make([]*domain.Post, 0, len(res.Targets))
	for _, target := range res.Targets {
		posts = append(posts, buildPost(tc, target, draft, opts, now, res.GroupID))
	}

	result := &CreateResult{FanOut: res.FanOut}
	if res.FanOut {
		batch := &domain.PostBatch{Kind: domain.PostBatchKindFanout}
		created, err := s.store.CreatePostsBatch(ctx, batch, posts)
		if err != nil {
			return nil, s.storageFailure("creating posts", err)
		}
		result.BatchID = batch.ID
		result.Posts = created
	} else {
		created, err := s.store.CreatePost(ctx, posts[0])
		if err != nil {
			return nil, s.storageFailure("creating post", err)
		}
		result.Posts = []*domain.Post{created}
	}
	s.audit(ctx, "POST_CREATE", "POST", result.Posts[0].ID, map[string]any{"count": len(result.Posts), "batch_id": result.BatchID, "publish_now": opts.PublishNow, "status": result.Posts[0].Status, "library_item_id": draft.LibraryItemID})

	// Dispatch publish_now jobs. A broker failure must not strand later posts in QUEUED (the scheduler
	// never reclaims QUEUED), so every remaining post is parked as a retryable FAILED before returning.
	for i, post := range result.Posts {
		if !(opts.PublishNow && post.Status == domain.PostStatusQueued) {
			continue
		}
		if ferr := s.enqueue(ctx, post, 1); ferr != nil {
			for _, rest := range result.Posts[i+1:] {
				if rest.Status == domain.PostStatusQueued {
					retryAt := s.now()
					if perr := s.store.MarkPostFailedOrRetry(ctx, rest.ID, 0, &retryAt, domain.PostStatusFailed, 0, "queue unavailable at creation; scheduler will retry"); perr != nil {
						s.logger.Error("post stranded in QUEUED after broker failure; operator reconciliation required", "post_id", rest.ID, "batch_id", result.BatchID, "error", perr)
					}
				}
			}
			ferr.Message = fmt.Sprintf("%s (batch_id=%s, post_id=%s)", ferr.Message, result.BatchID, post.ID)
			return nil, ferr
		}
	}
	return result, nil
}

func (s *Service) audit(ctx context.Context, action, entityType, entityID string, details map[string]any) {
	tc := domain.TenantFromContext(ctx)
	raw, _ := json.Marshal(details)
	if err := s.store.InsertAuditLog(ctx, &domain.AuditLog{OrganizationID: tc.OrganizationID, UserID: &tc.UserID, Action: action, EntityType: entityType, EntityID: entityID, Details: raw}); err != nil {
		s.logger.Warn("audit log write failed", "action", action, "entity_id", entityID, "error", err)
	}
}

func (s *Service) enqueue(ctx context.Context, post *domain.Post, attempt int) *Error {
	payload := domain.PublishJobPayload{
		OrganizationID: post.OrganizationID,
		PostID:         post.ID,
		ChannelID:      post.ChannelID,
		QueueTier:      post.QueueTier,
		Attempt:        attempt,
		EnqueuedAt:     s.now(),
	}
	if err := s.queue.EnqueuePublishJob(ctx, payload); err != nil {
		s.logger.Error("Failed to enqueue publish_now job to redis", "post_id", post.ID, "error", err)
		nowErr := s.now()
		if perr := s.store.MarkPostFailedOrRetry(ctx, post.ID, 0, &nowErr, domain.PostStatusFailed, 0, fmt.Sprintf("failed to enqueue to redis: %v", err)); perr != nil {
			s.logger.Error("post stranded in QUEUED after broker failure; operator reconciliation required", "post_id", post.ID, "error", perr)
		}
		return fail(http.StatusBadGateway, "failed to enqueue job to message broker")
	}
	return nil
}

// BulkItem is one post of a series.
type BulkItem struct {
	Content     string
	LinkURL     string
	MediaType   string
	MediaURLs   []string
	ScheduledAt *time.Time // explicit time; nil means "assign from the plan"
}

// BulkRequest describes a series of posts plus a distribution plan.
type BulkRequest struct {
	Name              string
	Targets           Targets
	Items             []BulkItem
	Plan              *domain.SchedulePlan
	SubmitForApproval bool
	PublishNow        bool // not supported for bulk; rejected explicitly
	MaxRetries        int
}

// BulkPlan is the computed schedule before (or at) commit.
type BulkPlan struct {
	Resolution   *Resolution
	Slots        []domain.ScheduleSlot
	Warnings     []string
	WarningCodes []domain.SlotNote
	Plan         *domain.SchedulePlan
}

// validateItem applies the shared draft rules to one bulk item, prefixing errors with its index.
func validateItem(index int, item *BulkItem) *Error {
	content, link, mediaType, urls, err := domain.NormalizeDraft(item.Content, item.LinkURL, item.MediaType, item.MediaURLs)
	if err != nil {
		return fail(http.StatusBadRequest, "items[%d]: %s", index, err.Error())
	}
	item.Content, item.LinkURL, item.MediaType, item.MediaURLs = content, link, mediaType, urls
	return nil
}

// PlanBulk validates the request, resolves targets and computes a publication time for every item.
// It never writes.
func (s *Service) PlanBulk(ctx context.Context, req *BulkRequest) (*BulkPlan, *Error) {
	if req.PublishNow {
		return nil, fail(http.StatusBadRequest, "publish_now is not supported for bulk creation; schedule the posts instead")
	}
	if len([]rune(strings.TrimSpace(req.Name))) > domain.MaxBatchNameLength {
		return nil, fail(http.StatusBadRequest, "name exceeds %d characters", domain.MaxBatchNameLength)
	}
	if len(req.Items) == 0 {
		return nil, fail(http.StatusBadRequest, "items must contain at least one post")
	}
	if len(req.Items) > domain.MaxBulkItems {
		return nil, fail(http.StatusBadRequest, "items supports at most %d posts per request", domain.MaxBulkItems)
	}
	res, ferr := s.ResolveTargets(ctx, req.Targets, false)
	if ferr != nil {
		return nil, ferr
	}
	if total := len(req.Items) * len(res.Targets); total > domain.MaxBulkPosts {
		return nil, fail(http.StatusBadRequest, "request would create %d posts; the limit is %d", total, domain.MaxBulkPosts)
	}
	for i := range req.Items {
		if ferr := validateItem(i, &req.Items[i]); ferr != nil {
			return nil, ferr
		}
	}

	now := s.now()
	channelIDs := res.ChannelIDs()
	occ := domain.Occupancy{}
	slots := make([]domain.ScheduleSlot, len(req.Items))
	needAuto := 0
	for i, item := range req.Items {
		if item.ScheduledAt == nil {
			needAuto++
			continue
		}
		at := item.ScheduledAt.UTC()
		slot := domain.ScheduleSlot{Index: i, ScheduledAt: at, Explicit: true}
		if at.Before(now) {
			n := domain.SlotNote{Code: domain.NotePastTime}
			slot.NoteCode, slot.Note = n.Code, n.String()
		}
		slots[i] = slot
		for _, ch := range channelIDs {
			occ.Add(ch, at)
		}
	}

	plan := &BulkPlan{Resolution: res, Slots: slots}
	if needAuto == 0 {
		return plan, nil
	}
	if req.Plan == nil {
		return nil, fail(http.StatusBadRequest, "schedule_plan is required when items omit scheduled_at")
	}
	req.Plan.ClampStart(now)
	if err := req.Plan.Normalize(); err != nil {
		return nil, fail(http.StatusBadRequest, "%s", err.Error())
	}
	plan.Plan = req.Plan

	if req.Plan.AvoidConflicts {
		from := now.Add(-req.Plan.MinGap())
		existing, err := s.store.QueryPosts(ctx, domain.PostQuery{ChannelIDs: channelIDs, Statuses: domain.ActivePostStatuses(), From: &from, Limit: domain.MaxPostQueryLimit, Ascending: true})
		if err != nil {
			return nil, s.storageFailure("loading existing schedule", err)
		}
		for _, p := range existing {
			occ.Add(p.ChannelID, p.ScheduledAt)
		}
		if len(existing) >= domain.MaxPostQueryLimit {
			n := domain.SlotNote{Code: domain.NoteOccupancyTruncated, Value: domain.MaxPostQueryLimit}
			plan.WarningCodes = append(plan.WarningCodes, n)
			plan.Warnings = append(plan.Warnings, n.String())
		}
	}

	times, notes, err := domain.GenerateScheduleSlots(*req.Plan, needAuto, now, channelIDs, occ)
	if err != nil {
		return nil, fail(http.StatusBadRequest, "%s", err.Error())
	}
	k := 0
	for i := range req.Items {
		if req.Items[i].ScheduledAt != nil {
			continue
		}
		slots[i] = domain.ScheduleSlot{Index: i, ScheduledAt: times[k], NoteCode: notes[k].Code, NoteValue: notes[k].Value, Note: notes[k].String()}
		k++
	}
	return plan, nil
}

// BulkResult is the committed series.
type BulkResult struct {
	Batch *domain.PostBatch
	Posts []*domain.Post
	Slots []domain.ScheduleSlot
}

// CreateBulk plans and stores the series in one transaction. Bulk never publishes immediately.
func (s *Service) CreateBulk(ctx context.Context, req *BulkRequest) (*BulkResult, *Error) {
	plan, ferr := s.PlanBulk(ctx, req)
	if ferr != nil {
		return nil, ferr
	}
	tc := domain.TenantFromContext(ctx)
	now := s.now()
	posts := make([]*domain.Post, 0, len(req.Items)*len(plan.Resolution.Targets))
	for i, item := range req.Items {
		draft := Draft{Content: item.Content, LinkURL: item.LinkURL, MediaType: item.MediaType, MediaURLs: item.MediaURLs, ScheduledAt: plan.Slots[i].ScheduledAt, MaxRetries: req.MaxRetries}
		for _, target := range plan.Resolution.Targets {
			posts = append(posts, buildPost(tc, target, draft, Options{SubmitForApproval: req.SubmitForApproval}, now, plan.Resolution.GroupID))
		}
	}
	batch := &domain.PostBatch{Name: strings.TrimSpace(req.Name), Kind: domain.PostBatchKindBulk, Plan: plan.Plan}
	created, err := s.store.CreatePostsBatch(ctx, batch, posts)
	if err != nil {
		return nil, s.storageFailure("creating bulk posts", err)
	}
	s.audit(ctx, "POST_BULK_CREATE", "POST_BATCH", batch.ID, map[string]any{"batch_id": batch.ID, "items": len(req.Items), "posts": len(created), "channels": plan.Resolution.ChannelIDs()})
	return &BulkResult{Batch: batch, Posts: created, Slots: plan.Slots}, nil
}

// Update edits an editable post after checking the author permission on its channel.
// An EDITOR (non-admin) changing text, link or media on an approval-required channel sends the post
// back to PENDING_APPROVAL so the reviewed content can never be swapped after approval; schedule-only
// changes keep the current decision.
func (s *Service) Update(ctx context.Context, postID string, patch domain.PostPatch) (*domain.Post, *Error) {
	tc := domain.TenantFromContext(ctx)
	post, err := s.store.GetPostByID(ctx, postID)
	if err != nil || post == nil {
		return nil, fail(http.StatusNotFound, "post not found")
	}
	role, ok := s.perms.ResolveChannelPermission(ctx, post.ChannelID)
	if !ok || !tc.CanCreatePost(role) {
		return nil, fail(http.StatusForbidden, "forbidden: insufficient permission to edit posts on this channel")
	}
	if patch.IsEmpty() {
		return nil, fail(http.StatusBadRequest, "nothing to update")
	}
	if !post.CanEdit() {
		return nil, fail(http.StatusConflict, "post cannot be edited in status %s", post.Status)
	}
	// Re-review applies to posts that carry (or can carry) a decision: SCHEDULED/FAILED/PENDING_APPROVAL.
	// Drafts stay drafts so editors can iterate before submitting explicitly.
	patch.RequireReview, patch.SubmittedBy = false, ""
	if patch.ChangesContent() && post.Status != domain.PostStatusDraft && !tc.IsOrgAdminOrOwner() && role == domain.GroupRoleEditor {
		required, err := s.requireApproval(ctx, tc.OrganizationID, post.ChannelID)
		if err != nil {
			s.logger.Error("approval lookup failed", "post_id", postID, "error", err)
			return nil, fail(http.StatusServiceUnavailable, "cannot verify approval rules right now")
		}
		if required {
			patch.RequireReview, patch.SubmittedBy = true, tc.UserID
		}
	}
	// Validate and normalise the merged result with the shared draft rules, then persist the normalised values.
	preview := *post
	if err := preview.ApplyPatch(patch, s.now()); err != nil {
		return nil, fail(http.StatusConflict, "%s", err.Error())
	}
	content, link, mediaType, urls, verr := domain.NormalizeDraft(preview.Content, preview.LinkURL, preview.MediaType, preview.MediaURLs)
	if verr != nil {
		return nil, fail(http.StatusBadRequest, "%s", verr.Error())
	}
	if patch.Content != nil {
		patch.Content = &content
	}
	if patch.LinkURL != nil {
		patch.LinkURL = &link
	}
	if patch.MediaType != nil {
		patch.MediaType = &mediaType
	}
	if patch.MediaURLs != nil {
		patch.MediaURLs = &urls
	}
	updated, err := s.store.UpdatePost(ctx, postID, patch)
	if err != nil {
		if errors.Is(err, domain.ErrPostNotEditable) {
			return nil, fail(http.StatusConflict, "post changed state and can no longer be edited")
		}
		return nil, s.storageFailure("updating post", err)
	}
	changed := make([]string, 0, 5)
	for name, set := range map[string]bool{"content": patch.Content != nil, "link_url": patch.LinkURL != nil, "media_type": patch.MediaType != nil, "media_urls": patch.MediaURLs != nil, "scheduled_at": patch.ScheduledAt != nil} {
		if set {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	s.audit(ctx, "POST_UPDATE", "POST", postID, map[string]any{"changed": changed, "previous_status": post.Status, "status": updated.Status, "scheduled_at": updated.ScheduledAt, "previous_scheduled_at": post.ScheduledAt, "require_review": patch.RequireReview})
	return updated, nil
}
