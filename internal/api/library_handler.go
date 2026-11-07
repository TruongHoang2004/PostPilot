package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/app/publishing"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// maxLibraryPage keeps the has_more probe (limit+1) below the store cap.
const maxLibraryPage = 100

// libraryBody is the JSON contract for creating and (with pointer semantics) editing library items.
type libraryBody struct {
	Title     *string   `json:"title"`
	ChannelID *string   `json:"channel_id"`
	Content   *string   `json:"content"`
	LinkURL   *string   `json:"link_url"`
	MediaType *string   `json:"media_type"`
	MediaURLs *[]string `json:"media_urls"`
}

func (b libraryBody) patch() domain.LibraryPatch {
	return domain.LibraryPatch{Title: b.Title, ChannelID: b.ChannelID, Content: b.Content, LinkURL: b.LinkURL, MediaType: b.MediaType, MediaURLs: b.MediaURLs}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// handleListLibrary lists the active library (default) or the "Đã đăng" archive (archived=true).
// Query: archived, channel_id, q, limit (<=100), offset. Members only see items tied to Fanpages they
// can view, plus unbound items when they can view at least one Fanpage.
func (s *Server) handleListLibrary(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	query := domain.LibraryQuery{Archived: qs.Get("archived") == "true", Search: qs.Get("q")}
	query.Limit, _ = strconv.Atoi(qs.Get("limit"))
	query.Offset, _ = strconv.Atoi(qs.Get("offset"))
	if query.Limit <= 0 || query.Limit > maxLibraryPage {
		query.Limit = maxLibraryPage
		if v, _ := strconv.Atoi(qs.Get("limit")); v > 0 && v < maxLibraryPage {
			query.Limit = v
		}
	}
	explicit := strings.TrimSpace(qs.Get("channel_id"))
	if explicit != "" {
		query.ChannelIDs = []string{explicit}
	}
	allowed, restricted, err := s.viewableChannelIDs(r.Context())
	if err != nil {
		s.logger.Error("channel visibility lookup failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "storage error while listing library"})
		return
	}
	if restricted {
		if len(allowed) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"items": []domain.LibraryItem{}, "limit": query.Limit, "offset": query.Offset, "has_more": false})
			return
		}
		if explicit != "" {
			query.ChannelIDs = intersect(query.ChannelIDs, allowed)
			if len(query.ChannelIDs) == 0 {
				writeJSON(w, http.StatusOK, map[string]any{"items": []domain.LibraryItem{}, "limit": query.Limit, "offset": query.Offset, "has_more": false})
				return
			}
		} else {
			query.ChannelIDs, query.IncludeUnbound = allowed, true
		}
	}
	limit := query.Limit
	probe := query
	probe.Limit = limit + 1
	items, err := s.store.QueryLibraryItems(r.Context(), probe)
	if err != nil {
		s.logger.Error("list library failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "storage error while listing library"})
		return
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	if items == nil {
		items = []domain.LibraryItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "limit": limit, "offset": query.Offset, "has_more": hasMore})
}

func (s *Server) handleCreateLibraryItem(w http.ResponseWriter, r *http.Request) {
	var body libraryBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	in := publishing.LibraryInput{Title: deref(body.Title), ChannelID: deref(body.ChannelID), Content: deref(body.Content), LinkURL: deref(body.LinkURL), MediaType: deref(body.MediaType)}
	if body.MediaURLs != nil {
		in.MediaURLs = *body.MediaURLs
	}
	item, ferr := s.publishing.CreateLibraryItem(r.Context(), in)
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleGetLibraryItem(w http.ResponseWriter, r *http.Request) {
	item, ferr := s.publishing.GetLibraryItem(r.Context(), r.PathValue("id"))
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleUpdateLibraryItem(w http.ResponseWriter, r *http.Request) {
	var body libraryBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	item, ferr := s.publishing.UpdateLibraryItem(r.Context(), r.PathValue("id"), body.patch())
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleDeleteLibraryItem(w http.ResponseWriter, r *http.Request) {
	if ferr := s.publishing.DeleteLibraryItem(r.Context(), r.PathValue("id")); ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "library item deleted", "id": r.PathValue("id")})
}

func (s *Server) handleRestoreLibraryItem(w http.ResponseWriter, r *http.Request) {
	item, ferr := s.publishing.RestoreLibraryItem(r.Context(), r.PathValue("id"))
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// handleScheduleLibraryItem copies a library item into scheduled posts (calendar click flow).
func (s *Server) handleScheduleLibraryItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChannelID          string   `json:"channel_id"`
		ChannelIDs         []string `json:"channel_ids"`
		ChannelGroupID     string   `json:"channel_group_id"`
		ScheduledAt        string   `json:"scheduled_at"`
		DeleteAfterPublish bool     `json:"delete_after_publish"`
		SubmitForApproval  bool     `json:"submit_for_approval"`
		PublishNow         bool     `json:"publish_now"`
		MaxRetries         int      `json:"max_retries"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	if req.PublishNow {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "publish_now is not supported for library scheduling; choose a time instead"})
		return
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ScheduledAt))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "scheduled_at is required, expected RFC3339 (e.g. 2026-11-20T09:00:00+07:00)"})
		return
	}
	result, ferr := s.publishing.ScheduleLibraryItem(r.Context(), r.PathValue("id"),
		publishing.Targets{ChannelID: req.ChannelID, ChannelIDs: req.ChannelIDs, ChannelGroupID: req.ChannelGroupID},
		publishing.LibrarySchedule{ScheduledAt: at.UTC(), DeleteAfterPublish: req.DeleteAfterPublish, SubmitForApproval: req.SubmitForApproval, MaxRetries: req.MaxRetries})
	if ferr != nil {
		writePublishingError(w, ferr)
		return
	}
	resp := map[string]any{"count": len(result.Posts), "posts": result.Posts}
	if result.BatchID != "" {
		resp["batch_id"] = result.BatchID // only fan-out creates a batch
	}
	writeJSON(w, http.StatusCreated, resp)
}
