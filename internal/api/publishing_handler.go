package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// handleListPosts returns posts (Post Aggregate) matching the status filter.
func (s *Server) handleListPosts(w http.ResponseWriter, r *http.Request) {
	statusFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	posts, err := s.store.ListPosts(r.Context(), statusFilter, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"posts": posts,
	})
}

// handleCreatePost creates a new post aggregate root applying domain rules, multi-channel fan-out,
// and approval workflows, enqueuing to Redis if publish_now is true and permitted.
func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)

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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	// 1. Resolve target channel IDs
	var targetChannelIDs []string
	groupRequireApproval := false

	if req.ChannelGroupID != "" {
		cg, err := s.store.GetChannelGroupByID(ctx, tc.OrganizationID, req.ChannelGroupID)
		if err != nil || cg == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "channel_group_id does not exist"})
			return
		}
		groupRequireApproval = cg.RequireApproval
		for _, ch := range cg.Channels {
			targetChannelIDs = append(targetChannelIDs, ch.ID)
		}
		if len(targetChannelIDs) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "channel group contains no channels"})
			return
		}
	} else if len(req.ChannelIDs) > 0 {
		for _, id := range req.ChannelIDs {
			trimmed := strings.TrimSpace(id)
			if trimmed != "" {
				targetChannelIDs = append(targetChannelIDs, trimmed)
			}
		}
	} else if strings.TrimSpace(req.ChannelID) != "" {
		targetChannelIDs = []string{strings.TrimSpace(req.ChannelID)}
	}

	if len(targetChannelIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "channel_id, channel_ids, or channel_group_id is required"})
		return
	}

	// 2. Parse scheduled time
	var schedTime time.Time
	if req.ScheduledAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.ScheduledAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid scheduled_at format, expected RFC3339 (e.g. 2026-10-21T15:04:05Z)"})
			return
		}
		schedTime = parsed.UTC()
	}

	// 3. Batch ID if multiple channels or group
	var batchID string
	if len(targetChannelIDs) > 1 || req.ChannelGroupID != "" {
		batchID = newUUID()
	}

	// 4. Pre-validate channels and RBAC permissions
	type validatedChannel struct {
		channelID       string
		role            domain.GroupRole
		requireApproval bool
	}
	validated := make([]validatedChannel, 0, len(targetChannelIDs))

	for _, chID := range targetChannelIDs {
		if _, err := s.store.GetChannelByID(ctx, chID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "channel_id does not exist: " + chID})
			return
		}

		role, ok := s.resolveChannelPermission(ctx, chID)
		if !ok && !tc.IsOrgAdminOrOwner() {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: you do not have permission to post to channel " + chID})
			return
		}
		if tc.IsOrgAdminOrOwner() && (!ok || role == "") {
			role = domain.GroupRoleManager
		}

		if role == domain.GroupRoleViewer {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: role VIEWER cannot create posts"})
			return
		}

		chRequireApproval := groupRequireApproval
		if !chRequireApproval {
			if groups, err := s.store.ListChannelGroups(ctx, tc.OrganizationID); err == nil {
				for _, g := range groups {
					if g.RequireApproval {
						if gDetail, err := s.store.GetChannelGroupByID(ctx, tc.OrganizationID, g.ID); err == nil {
							for _, c := range gDetail.Channels {
								if c.ID == chID {
									chRequireApproval = true
									break
								}
							}
						}
					}
					if chRequireApproval {
						break
					}
				}
			}
		}

		if role == domain.GroupRoleEditor && chRequireApproval && req.PublishNow {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "role EDITOR cannot publish directly; please submit for approval"})
			return
		}

		validated = append(validated, validatedChannel{
			channelID:       chID,
			role:            role,
			requireApproval: chRequireApproval,
		})
	}

	// 5. Create posts for each channel
	now := time.Now().UTC()
	createdPosts := make([]*domain.Post, 0, len(validated))

	for _, vc := range validated {
		mustPendingApproval := req.SubmitForApproval || (vc.role == domain.GroupRoleEditor && vc.requireApproval)
		publishNow := req.PublishNow && !mustPendingApproval

		postEntity := &domain.Post{
			OrganizationID: tc.OrganizationID,
			BatchID:        batchID,
			ChannelGroupID: req.ChannelGroupID,
			ChannelID:      vc.channelID,
			Content:        req.Content,
			LinkURL:        req.LinkURL,
			MediaType:      req.MediaType,
			MediaURLs:      req.MediaURLs,
			ScheduledAt:    schedTime,
			MaxRetries:     req.MaxRetries,
			CreatedBy:      tc.UserID,
			SubmittedBy:    tc.UserID,
		}

		if mustPendingApproval {
			postEntity.PrepareForCreation(false, now)
			postEntity.Status = domain.PostStatusPendingApproval
		} else {
			postEntity.PrepareForCreation(publishNow, now)
		}

		post, err := s.store.CreatePost(ctx, postEntity)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		if publishNow && post.Status == domain.PostStatusQueued {
			payload := domain.PublishJobPayload{
				PostID:     post.ID,
				ChannelID:  post.ChannelID,
				QueueTier:  post.QueueTier,
				Attempt:    1,
				EnqueuedAt: time.Now().UTC(),
			}
			if err := s.redis.EnqueuePublishJob(ctx, payload); err != nil {
				s.logger.Error("Failed to enqueue publish_now job to redis", "post_id", post.ID, "error", err)
				nowErr := time.Now().UTC()
				_ = s.store.MarkPostFailedOrRetry(ctx, post.ID, 0, &nowErr, domain.PostStatusFailed, 0, fmt.Sprintf("failed to enqueue to redis: %v", err))
				writeJSON(w, http.StatusBadGateway, map[string]any{"error": "failed to enqueue job to message broker"})
				return
			}
		}

		createdPosts = append(createdPosts, post)
	}

	if len(createdPosts) == 1 && req.ChannelGroupID == "" && len(req.ChannelIDs) == 0 {
		writeJSON(w, http.StatusCreated, createdPosts[0])
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"batch_id": batchID,
		"count":    len(createdPosts),
		"posts":    createdPosts,
	})
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
	_ = json.NewDecoder(r.Body).Decode(&req)

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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	action := domain.ApprovalAction(strings.ToUpper(strings.TrimSpace(req.Action)))
	if action != domain.ApprovalActionApproved && action != domain.ApprovalActionRejected {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "action must be APPROVED or REJECTED"})
		return
	}

	reviewedPost, err := s.store.ReviewPost(ctx, post.ID, tc.UserID, action, req.Note)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// If approved and queued, enqueue to Redis
	if action == domain.ApprovalActionApproved && reviewedPost.Status == domain.PostStatusQueued {
		payload := domain.PublishJobPayload{
			PostID:     reviewedPost.ID,
			ChannelID:  reviewedPost.ChannelID,
			QueueTier:  reviewedPost.QueueTier,
			Attempt:    1,
			EnqueuedAt: time.Now().UTC(),
		}
		_ = s.redis.EnqueuePublishJob(ctx, payload)
	}

	// Apply decision to whole batch if requested
	if req.ApplyToBatch && post.BatchID != "" {
		pendingPosts, _ := s.store.ListPosts(ctx, domain.PostStatusPendingApproval, 100)
		for _, p := range pendingPosts {
			if p.BatchID == post.BatchID && p.ID != post.ID {
				otherReviewed, err := s.store.ReviewPost(ctx, p.ID, tc.UserID, action, req.Note)
				if err == nil && action == domain.ApprovalActionApproved && otherReviewed.Status == domain.PostStatusQueued {
					_ = s.redis.EnqueuePublishJob(ctx, domain.PublishJobPayload{
						PostID:     otherReviewed.ID,
						ChannelID:  otherReviewed.ChannelID,
						QueueTier:  otherReviewed.QueueTier,
						Attempt:    1,
						EnqueuedAt: time.Now().UTC(),
					})
				}
			}
		}
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
		PostID:     post.ID,
		ChannelID:  post.ChannelID,
		QueueTier:  post.QueueTier,
		Attempt:    post.RetryCount + 1,
		EnqueuedAt: time.Now().UTC(),
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
	postID := r.PathValue("id")
	if err := s.store.CancelPost(r.Context(), postID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "Post schedule cancelled successfully",
		"post_id": postID,
	})
}
