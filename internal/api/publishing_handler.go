package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/app/publishing"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// writePublishingError maps an application-service error to a JSON response.
func writePublishingError(w http.ResponseWriter, err *publishing.Error) {
	writeJSON(w, err.Status, map[string]any{"error": err.Message})
}

// ResolveChannelPermission exposes the effective channel role to the application service.
func (s *Server) ResolveChannelPermission(ctx context.Context, channelID string) (domain.GroupRole, bool) {
	return s.resolveChannelPermission(ctx, channelID)
}

func parseRFC3339Param(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	u := t.UTC()
	return &u, nil
}

// handleListPosts returns tenant posts filtered by status, channel, batch and scheduled window.
// Query: status, channel_id, batch_id, from, to (RFC3339, half-open), limit (<=500), offset.
// A time window sorts ascending (calendar order); otherwise newest scheduled first.
func (s *Server) handleListPosts(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	query := domain.PostQuery{ChannelIDs: nil, BatchID: strings.TrimSpace(qs.Get("batch_id"))}
	if status := strings.ToUpper(strings.TrimSpace(qs.Get("status"))); status != "" && status != "ALL" {
		query.Statuses = []string{status}
	}
	if ch := strings.TrimSpace(qs.Get("channel_id")); ch != "" {
		query.ChannelIDs = []string{ch}
	}
	query.Limit, _ = strconv.Atoi(qs.Get("limit"))
	query.Offset, _ = strconv.Atoi(qs.Get("offset"))
	from, err := parseRFC3339Param(qs.Get("from"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid from, expected RFC3339"})
		return
	}
	to, err := parseRFC3339Param(qs.Get("to"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid to, expected RFC3339"})
		return
	}
	if from != nil && to != nil && !to.After(*from) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "to must be after from"})
		return
	}
	query.From, query.To = from, to
	query.Ascending = from != nil || to != nil
	query.Normalize()

	// Restrict the query itself to channels the caller may view so LIMIT/OFFSET page over the visible set.
	allowed, restricted, verr := s.viewableChannelIDs(r.Context())
	if verr != nil {
		s.logger.Error("channel visibility lookup failed", "error", verr)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "storage error while listing posts"})
		return
	}
	if restricted {
		if len(query.ChannelIDs) > 0 {
			query.ChannelIDs = intersect(query.ChannelIDs, allowed)
		} else {
			query.ChannelIDs = allowed
		}
		if len(query.ChannelIDs) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"posts": []domain.Post{}, "limit": query.Limit, "offset": query.Offset, "has_more": false})
			return
		}
	}
	// has_more is exact: below the cap probe one extra row (trimmed before serialisation); at the cap the
	// stores clamp the probe back, so peek at the next page with a one-row query instead.
	probe := query
	var hasMore bool
	var posts []domain.Post
	var qerr error
	if query.Limit < domain.MaxPostQueryLimit {
		probe.Limit = query.Limit + 1
		posts, qerr = s.store.QueryPosts(r.Context(), probe)
		if qerr == nil && len(posts) > query.Limit {
			hasMore = true
			posts = posts[:query.Limit]
		}
	} else {
		posts, qerr = s.store.QueryPosts(r.Context(), query)
		if qerr == nil && len(posts) == query.Limit {
			next := query
			next.Limit, next.Offset = 1, query.Offset+query.Limit
			peek, perr := s.store.QueryPosts(r.Context(), next)
			qerr = perr
			hasMore = len(peek) > 0
		}
	}
	if qerr != nil {
		s.logger.Error("list posts failed", "error", qerr)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "storage error while listing posts"})
		return
	}
	if posts == nil {
		posts = []domain.Post{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"posts":    posts,
		"limit":    query.Limit,
		"offset":   query.Offset,
		"has_more": hasMore,
	})
}

// viewableChannelIDs returns the channels the caller may view. restricted is false for org admins/owners,
// who see every channel of the workspace.
func (s *Server) viewableChannelIDs(ctx context.Context) ([]string, bool, error) {
	tc := domain.TenantFromContext(ctx)
	if tc.IsOrgAdminOrOwner() {
		return nil, false, nil
	}
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, true, err
	}
	allowed := make([]string, 0, len(channels))
	for _, ch := range channels {
		if role, ok := s.resolveChannelPermission(ctx, ch.ID); ok && domain.CanViewChannel(role) {
			allowed = append(allowed, ch.ID)
		}
	}
	return allowed, true, nil
}

func intersect(requested, allowed []string) []string {
	set := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		set[id] = true
	}
	out := make([]string, 0, len(requested))
	for _, id := range requested {
		if set[id] {
			out = append(out, id)
		}
	}
	return out
}

// visiblePosts keeps only posts on channels the caller may view, resolving each channel once.
func (s *Server) visiblePosts(ctx context.Context, posts []domain.Post) []domain.Post {
	visible := make([]domain.Post, 0, len(posts))
	cache := make(map[string]bool)
	for _, post := range posts {
		allowed, seen := cache[post.ChannelID]
		if !seen {
			role, ok := s.resolveChannelPermission(ctx, post.ChannelID)
			allowed = ok && domain.CanViewChannel(role)
			cache[post.ChannelID] = allowed
		}
		if allowed {
			visible = append(visible, post)
		}
	}
	return visible
}

// handleCreatePost creates one draft on one or many channels through the publishing service.
func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChannelID         string   `json:"channel_id"`
		ChannelIDs        []string `json:"channel_ids"`
		ChannelGroupID    string   `json:"channel_group_id"`
		Content           string   `json:"content"`
		LinkURL           string   `json:"link_url"`
		MediaType         string   `json:"media_type"`
		MediaURLs         []string `json:"media_urls"`
		ScheduledAt       string   `json:"scheduled_at"` // ISO-8601 (RFC3339). If empty -> Immediate or current schedule
		PublishNow        bool     `json:"publish_now"`
		SubmitForApproval bool     `json:"submit_for_approval"`
		MaxRetries        int      `json:"max_retries"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	targets := publishing.Targets{ChannelID: req.ChannelID, ChannelIDs: req.ChannelIDs, ChannelGroupID: req.ChannelGroupID}

	var schedTime time.Time
	if req.ScheduledAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.ScheduledAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid scheduled_at format, expected RFC3339 (e.g. 2026-10-21T15:04:05Z)"})
			return
		}
		schedTime = parsed.UTC()
	}

	result, ferr := s.publishing.Create(r.Context(), targets,
		publishing.Draft{Content: req.Content, LinkURL: req.LinkURL, MediaType: req.MediaType, MediaURLs: req.MediaURLs, ScheduledAt: schedTime, MaxRetries: req.MaxRetries},
		publishing.Options{PublishNow: req.PublishNow, SubmitForApproval: req.SubmitForApproval})
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	if !result.FanOut {
		writeJSON(w, http.StatusCreated, result.Posts[0])
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"batch_id": result.BatchID,
		"count":    len(result.Posts),
		"posts":    result.Posts,
	})
}

// bulkRequestBody is the JSON contract shared by bulk preview and bulk create.
type bulkRequestBody struct {
	Name              string               `json:"name"`
	ChannelID         string               `json:"channel_id"`
	ChannelIDs        []string             `json:"channel_ids"`
	ChannelGroupID    string               `json:"channel_group_id"`
	Items             []bulkItemBody       `json:"items"`
	SchedulePlan      *domain.SchedulePlan `json:"schedule_plan"`
	SubmitForApproval bool                 `json:"submit_for_approval"`
	PublishNow        bool                 `json:"publish_now"`
	MaxRetries        int                  `json:"max_retries"`
}

type bulkItemBody struct {
	Content     string   `json:"content"`
	LinkURL     string   `json:"link_url"`
	MediaType   string   `json:"media_type"`
	MediaURLs   []string `json:"media_urls"`
	ScheduledAt string   `json:"scheduled_at"`
}

func decodeBulkRequest(w http.ResponseWriter, r *http.Request) (*publishing.BulkRequest, *publishing.Error) {
	var body bulkRequestBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&body); err != nil {
		return nil, &publishing.Error{Status: http.StatusBadRequest, Message: "invalid json body"}
	}
	req := &publishing.BulkRequest{
		Name:              body.Name,
		Targets:           publishing.Targets{ChannelID: body.ChannelID, ChannelIDs: body.ChannelIDs, ChannelGroupID: body.ChannelGroupID},
		Plan:              body.SchedulePlan,
		SubmitForApproval: body.SubmitForApproval,
		PublishNow:        body.PublishNow,
		MaxRetries:        body.MaxRetries,
	}
	for i, item := range body.Items {
		bi := publishing.BulkItem{Content: item.Content, LinkURL: item.LinkURL, MediaType: item.MediaType, MediaURLs: item.MediaURLs}
		if strings.TrimSpace(item.ScheduledAt) != "" {
			parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(item.ScheduledAt))
			if err != nil {
				return nil, &publishing.Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("items[%d].scheduled_at must be RFC3339", i)}
			}
			at := parsed.UTC()
			bi.ScheduledAt = &at
		}
		req.Items = append(req.Items, bi)
	}
	return req, nil
}

func bulkChannelsPayload(res *publishing.Resolution) []map[string]any {
	out := make([]map[string]any, 0, len(res.Targets))
	for _, t := range res.Targets {
		out = append(out, map[string]any{"id": t.Channel.ID, "name": t.Channel.Name, "platform": t.Channel.Platform, "require_approval": t.RequireApproval, "role": t.Role})
	}
	return out
}

// handleBulkPreviewPosts computes publication times for a series without writing anything.
func (s *Server) handleBulkPreviewPosts(w http.ResponseWriter, r *http.Request) {
	req, ferr := decodeBulkRequest(w, r)
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	plan, ferr := s.publishing.PlanBulk(r.Context(), req)
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	warningCodes := make([]map[string]any, 0, len(plan.WarningCodes))
	for _, n := range plan.WarningCodes {
		warningCodes = append(warningCodes, map[string]any{"code": n.Code, "value": n.Value})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schedule":      plan.Slots,
		"warnings":      plan.Warnings,
		"warning_codes": warningCodes,
		"channels":      bulkChannelsPayload(plan.Resolution),
		"schedule_plan": plan.Plan,
		"post_count":    len(req.Items) * len(plan.Resolution.Targets),
	})
}

// handleBulkCreatePosts stores a series of posts on the resolved channels in one transaction.
func (s *Server) handleBulkCreatePosts(w http.ResponseWriter, r *http.Request) {
	req, ferr := decodeBulkRequest(w, r)
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	result, ferr := s.publishing.CreateBulk(r.Context(), req)
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"batch_id": result.Batch.ID,
		"batch":    result.Batch,
		"name":     result.Batch.Name,
		"count":    len(result.Posts),
		"posts":    result.Posts,
		"schedule": result.Slots,
	})
}

// handleUpdatePost edits content or schedule of an editable post (DRAFT, SCHEDULED, PENDING_APPROVAL, FAILED).
func (s *Server) handleUpdatePost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ScheduledAt *string   `json:"scheduled_at"`
		Content     *string   `json:"content"`
		LinkURL     *string   `json:"link_url"`
		MediaType   *string   `json:"media_type"`
		MediaURLs   *[]string `json:"media_urls"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	patch := domain.PostPatch{Content: body.Content, LinkURL: body.LinkURL, MediaType: body.MediaType, MediaURLs: body.MediaURLs}
	if body.ScheduledAt != nil {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*body.ScheduledAt))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid scheduled_at format, expected RFC3339"})
			return
		}
		at := parsed.UTC()
		patch.ScheduledAt = &at
	}
	updated, ferr := s.publishing.Update(r.Context(), r.PathValue("id"), patch)
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleGetPostBatch returns a batch and its posts visible to the caller.
func (s *Server) handleGetPostBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batch, err := s.store.GetPostBatch(ctx, r.PathValue("id"))
	if err != nil || batch == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "batch not found"})
		return
	}
	posts, err := s.store.QueryPosts(ctx, domain.PostQuery{BatchID: batch.ID, Limit: domain.MaxPostQueryLimit, Ascending: true})
	if err != nil {
		s.logger.Error("batch posts lookup failed", "batch_id", batch.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "storage error while loading batch"})
		return
	}
	visible := s.visiblePosts(ctx, posts)
	if len(visible) == 0 && batch.ItemCount > 0 {
		// Members without view permission on any channel of the batch must not learn its metadata.
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "batch not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"batch": batch, "posts": visible})
}

// handlePostSubmitApproval resubmits a draft or rejected post for editorial approval.
func (s *Server) handlePostSubmitApproval(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)
	postID := r.PathValue("id")

	post, err := s.store.GetPostByID(ctx, postID)
	if err != nil || post == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "post not found"})
		return
	}

	role, _ := s.resolveChannelPermission(ctx, post.ChannelID)
	if !tc.IsOrgAdminOrOwner() && !domain.CanCreatePost(role) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: insufficient permission to submit post for approval"})
		return
	}

	if post.Status != domain.PostStatusDraft && post.Status != domain.PostStatusRejected {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("only draft or rejected posts can be submitted for approval (status: %s)", post.Status)})
		return
	}

	var req struct {
		Content     string `json:"content"`
		ScheduledAt string `json:"scheduled_at"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	if len([]rune(req.Content)) > domain.MaxContentLength {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("content exceeds %d characters", domain.MaxContentLength)})
		return
	}

	var schedTime time.Time
	if req.ScheduledAt != "" {
		if parsed, err := time.Parse(time.RFC3339, req.ScheduledAt); err == nil {
			schedTime = parsed.UTC()
		}
	}

	content := req.Content
	if content == "" {
		content = post.Content
	}

	updated, err := s.store.ResubmitPost(ctx, postID, content, schedTime)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	_ = s.store.InsertAuditLog(ctx, &domain.AuditLog{
		OrganizationID: tc.OrganizationID,
		UserID:         &tc.UserID,
		Action:         "POST_SUBMIT_APPROVAL",
		EntityType:     "POST",
		EntityID:       postID,
		Details:        json.RawMessage(`{"status":"PENDING_APPROVAL"}`),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "post submitted for approval successfully",
		"post":    updated,
	})
}

// handleReviewPost evaluates an approval decision on a pending post (APPROVED or REJECTED).
func (s *Server) handleReviewPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)
	postID := r.PathValue("id")

	post, err := s.store.GetPostByID(ctx, postID)
	if err != nil || post == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "post not found"})
		return
	}

	role, _ := s.resolveChannelPermission(ctx, post.ChannelID)
	if !tc.IsOrgAdminOrOwner() && !domain.CanApprovePost(role) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: insufficient permission to review posts"})
		return
	}

	var req struct {
		Action       string `json:"action"`
		Note         string `json:"note"`
		ApplyToBatch bool   `json:"apply_to_batch"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	action := domain.ApprovalAction(strings.ToUpper(strings.TrimSpace(req.Action)))
	if action != domain.ApprovalActionApproved && action != domain.ApprovalActionRejected {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "action must be APPROVED or REJECTED"})
		return
	}

	targets := []domain.Post{*post}
	if req.ApplyToBatch && post.BatchID != "" {
		pending, err := s.store.QueryPosts(ctx, domain.PostQuery{BatchID: post.BatchID, Statuses: []string{domain.PostStatusPendingApproval}, Limit: domain.MaxPostQueryLimit, Ascending: true})
		if err != nil {
			writeJSON(w, 503, map[string]any{"error": "cannot load batch"})
			return
		}
		if len(pending) >= domain.MaxPostQueryLimit {
			writeJSON(w, http.StatusConflict, map[string]any{"error": fmt.Sprintf("batch has %d or more pending posts; review it in smaller parts", domain.MaxPostQueryLimit)})
			return
		}
		for _, other := range pending {
			if other.ID != post.ID {
				otherRole, ok := s.resolveChannelPermission(ctx, other.ChannelID)
				if !ok || !domain.CanApprovePost(otherRole) {
					writeJSON(w, 403, map[string]any{"error": "approval permission is required for every channel in this batch"})
					return
				}
				targets = append(targets, other)
			}
		}
	}

	reviewedPost, err := s.store.ReviewPost(ctx, post.ID, tc.UserID, action, req.Note)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	outcomes := []*domain.Post{reviewedPost}
	batchConflict := false
	for _, target := range targets[1:] {
		other, err := s.store.ReviewPost(ctx, target.ID, tc.UserID, action, req.Note)
		if err != nil {
			batchConflict = true
			break
		}
		outcomes = append(outcomes, other)
	}
	queueFailed := false
	for _, reviewed := range outcomes {
		if action == domain.ApprovalActionApproved && reviewed.Status == domain.PostStatusQueued {
			err := s.redis.EnqueuePublishJob(ctx, domain.PublishJobPayload{OrganizationID: reviewed.OrganizationID, PostID: reviewed.ID, ChannelID: reviewed.ChannelID, QueueTier: reviewed.QueueTier, Attempt: 1, EnqueuedAt: time.Now().UTC()})
			if err != nil {
				queueFailed = true
				retryAt := time.Now().UTC().Add(30 * time.Second)
				if persistErr := s.store.MarkPostFailedOrRetry(ctx, reviewed.ID, 0, &retryAt, domain.PostStatusFailed, 0, "queue unavailable after approval"); persistErr != nil {
					s.logger.Error("Unable to persist approval recovery", "post_id", reviewed.ID, "error", persistErr)
				}
			}
		}
	}
	if queueFailed {
		writeJSON(w, 503, map[string]any{"error": "approval saved but queue unavailable; recoverable posts will be retried by scheduler"})
		return
	}

	if batchConflict {
		writeJSON(w, 409, map[string]any{"error": "batch review partially committed; committed approvals were dispatched or scheduled for recovery; refresh before retrying"})
		return
	}

	details, _ := json.Marshal(map[string]any{
		"action":         string(action),
		"note":           req.Note,
		"apply_to_batch": req.ApplyToBatch,
		"batch_id":       post.BatchID,
	})
	_ = s.store.InsertAuditLog(ctx, &domain.AuditLog{
		OrganizationID: tc.OrganizationID,
		UserID:         &tc.UserID,
		Action:         "POST_REVIEW_" + string(action),
		EntityType:     "POST",
		EntityID:       post.ID,
		Details:        details,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "post review recorded successfully",
		"post":    reviewedPost,
	})
}

// handleGetPostDetail returns detailed post information and publish audit logs.
func (s *Server) handleGetPostDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID := r.PathValue("id")

	post, err := s.store.GetPostByID(ctx, postID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "post not found"})
		return
	}

	if role, ok := s.resolveChannelPermission(ctx, post.ChannelID); !ok || !domain.CanViewChannel(role) {
		writeJSON(w, 403, map[string]any{"error": "no permission to view this channel"})
		return
	}

	logs, _ := s.store.ListPublishLogsByPost(ctx, postID)
	writeJSON(w, http.StatusOK, map[string]any{
		"post":         post,
		"publish_logs": logs,
	})
}

// handlePublishPostNow triggers immediate publishing (synchronous via `?sync=true` or async via Queue).
func (s *Server) handlePublishPostNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID := r.PathValue("id")

	post, err := s.store.GetPostByID(ctx, postID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "post not found"})
		return
	}

	if post.Status == domain.PostStatusPendingApproval || post.Status == domain.PostStatusRejected {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "post status is " + post.Status + "; cannot be published directly without approval",
		})
		return
	}
	if post.IsTerminal() {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "post is already in terminal state (" + post.Status + ")",
		})
		return
	}

	tc := domain.TenantFromContext(ctx)
	if !tc.IsOrgAdminOrOwner() {
		role, hasExplicit := s.resolveChannelPermission(ctx, post.ChannelID)
		if !hasExplicit || !tc.CanPublishDirectly(role, true) {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error": "insufficient permissions to publish post directly",
			})
			return
		}
	}

	if r.URL.Query().Get("sync") == "true" && s.processor != nil {
		err := s.processor.ProcessPostByID(ctx, post.ID)
		updated, _ := s.store.GetPostByID(ctx, post.ID)
		logs, _ := s.store.ListPublishLogsByPost(ctx, post.ID)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error":        err.Error(),
				"post":         updated,
				"publish_logs": logs,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"message":      "Post published directly successfully",
			"post":         updated,
			"publish_logs": logs,
		})
		return
	}

	payload := domain.PublishJobPayload{
		OrganizationID: post.OrganizationID,
		PostID:         post.ID,
		ChannelID:      post.ChannelID,
		QueueTier:      post.QueueTier,
		Attempt:        post.RetryCount + 1,
		EnqueuedAt:     time.Now().UTC(),
	}
	if err := s.redis.EnqueuePublishJob(ctx, payload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"message":    "Post enqueued to priority queue (" + post.QueueTier + ")",
		"post_id":    post.ID,
		"queue_tier": post.QueueTier,
	})
}

// handleCancelPost cancels scheduled publishing for a non-terminal post.
func (s *Server) handleCancelPost(w http.ResponseWriter, r *http.Request) {
	post, err := s.store.GetPostByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, 404, map[string]any{"error": "post not found"})
		return
	}
	role, ok := s.resolveChannelPermission(r.Context(), post.ChannelID)
	if !ok || !domain.CanCreatePost(role) {
		writeJSON(w, 403, map[string]any{"error": "no permission to cancel posts on this channel"})
		return
	}
	if err = s.store.CancelPost(r.Context(), post.ID); err != nil {
		writeJSON(w, 409, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"message": "Post schedule cancelled successfully", "post_id": post.ID})
}
