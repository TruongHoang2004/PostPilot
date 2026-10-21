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

// handleCreatePost creates a new post aggregate root applying domain rules
// and automatically enqueues it to the worker queue if publish_now is true.
func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		ChannelID   string   `json:"channel_id"`
		Content     string   `json:"content"`
		LinkURL     string   `json:"link_url"`
		MediaType   string   `json:"media_type"`
		MediaURLs   []string `json:"media_urls"`
		ScheduledAt string   `json:"scheduled_at"` // ISO-8601 (RFC3339). If empty -> Immediate or current schedule
		PublishNow  bool     `json:"publish_now"`
		MaxRetries  int      `json:"max_retries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	req.ChannelID = strings.TrimSpace(req.ChannelID)
	if req.ChannelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "channel_id is required"})
		return
	}
	if _, err := s.store.GetChannelByID(ctx, req.ChannelID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "channel_id does not exist"})
		return
	}

	var schedTime time.Time
	if req.ScheduledAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.ScheduledAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid scheduled_at format, expected RFC3339 (e.g. 2026-10-21T15:04:05Z)"})
			return
		}
		schedTime = parsed.UTC()
	}

	postEntity := &domain.Post{
		ChannelID:   req.ChannelID,
		Content:     req.Content,
		LinkURL:     req.LinkURL,
		MediaType:   req.MediaType,
		MediaURLs:   req.MediaURLs,
		ScheduledAt: schedTime,
		MaxRetries:  req.MaxRetries,
		CreatedBy:   "admin",
	}
	postEntity.PrepareForCreation(req.PublishNow, time.Now().UTC())

	post, err := s.store.CreatePost(ctx, postEntity)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if req.PublishNow {
		payload := domain.PublishJobPayload{
			PostID:     post.ID,
			ChannelID:  post.ChannelID,
			QueueTier:  post.QueueTier,
			Attempt:    1,
			EnqueuedAt: time.Now().UTC(),
		}
		if err := s.redis.EnqueuePublishJob(ctx, payload); err != nil {
			s.logger.Error("Failed to enqueue publish_now job to redis", "post_id", post.ID, "error", err)
			now := time.Now().UTC()
			_ = s.store.MarkPostFailedOrRetry(ctx, post.ID, 0, &now, domain.PostStatusFailed, 0, fmt.Sprintf("failed to enqueue to redis: %v", err))
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "failed to enqueue job to message broker"})
			return
		}
	}

	writeJSON(w, http.StatusCreated, post)
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

