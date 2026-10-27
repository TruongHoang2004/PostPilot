package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// handleListChannelGroups lists all channel groups with associated channel IDs for the current organization.
func (s *Server) handleListChannelGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)

	groups, err := s.store.ListChannelGroups(ctx, tc.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	enriched := make([]domain.ChannelGroup, 0, len(groups))
	for _, g := range groups {
		if detail, err := s.store.GetChannelGroupByID(ctx, tc.OrganizationID, g.ID); err == nil && detail != nil {
			enriched = append(enriched, *detail)
		} else {
			enriched = append(enriched, g)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"channel_groups": enriched,
	})
}

// handleCreateChannelGroup provisions a new channel group.
func (s *Server) handleCreateChannelGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)

	if !tc.IsOrgAdminOrOwner() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: only organization owner or admin can create channel groups"})
		return
	}

	var req struct {
		Name            string `json:"name"`
		Description     string `json:"description"`
		ColorCode       string `json:"color_code"`
		RequireApproval bool   `json:"require_approval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
		return
	}

	group := &domain.ChannelGroup{
		OrganizationID:  tc.OrganizationID,
		Name:            req.Name,
		Description:     req.Description,
		RequireApproval: req.RequireApproval,
	}

	created, err := s.store.CreateChannelGroup(ctx, group)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// handleDeleteChannelGroup removes an existing channel group.
func (s *Server) handleDeleteChannelGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)
	groupID := r.PathValue("id")

	if !tc.IsOrgAdminOrOwner() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: only organization owner or admin can delete channel groups"})
		return
	}

	if err := s.store.DeleteChannelGroup(ctx, tc.OrganizationID, groupID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "channel group deleted successfully",
	})
}

// handleSetChannelGroupChannels maps publishing channels into a channel group.
func (s *Server) handleSetChannelGroupChannels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)
	groupID := r.PathValue("id")

	if !tc.IsOrgAdminOrOwner() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: only organization owner or admin can configure group channels"})
		return
	}

	var req struct {
		ChannelIDs []string `json:"channel_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	if err := s.store.SetChannelGroupChannels(ctx, tc.OrganizationID, groupID, req.ChannelIDs); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":     "channels updated successfully",
		"group_id":    groupID,
		"channel_ids": req.ChannelIDs,
	})
}

// handleListChannelGroupPermissions returns assigned RBAC permissions for a channel group.
func (s *Server) handleListChannelGroupPermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)
	groupID := r.PathValue("id")

	perms, err := s.store.ListChannelGroupPermissions(ctx, tc.OrganizationID, groupID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"permissions": perms,
	})
}

// handleSetChannelGroupPermission assigns or updates a user's role on a channel group.
func (s *Server) handleSetChannelGroupPermission(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)
	groupID := r.PathValue("id")

	if !tc.IsOrgAdminOrOwner() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: only organization owner or admin can grant permissions"})
		return
	}

	var req struct {
		MemberID string `json:"member_id"`
		UserID   string `json:"user_id"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	targetUserID := strings.TrimSpace(req.UserID)
	if targetUserID == "" {
		targetUserID = strings.TrimSpace(req.MemberID)
	}
	if targetUserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "member_id or user_id is required"})
		return
	}

	role := domain.GroupRole(strings.ToUpper(strings.TrimSpace(req.Role)))
	if role != domain.GroupRoleManager && role != domain.GroupRoleApprover && role != domain.GroupRoleEditor && role != domain.GroupRoleViewer {
		role = domain.GroupRoleViewer
	}

	perm, err := s.store.SetChannelGroupPermission(ctx, &domain.ChannelGroupPermission{
		ChannelGroupID: groupID,
		UserID:         targetUserID,
		GroupRole:      role,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":    "permission set successfully",
		"permission": perm,
	})
}

// handleDeleteChannelGroupPermission removes a user's RBAC permission from a channel group.
func (s *Server) handleDeleteChannelGroupPermission(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)
	groupID := r.PathValue("id")
	memberID := r.PathValue("memberId")

	if !tc.IsOrgAdminOrOwner() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: only organization owner or admin can revoke permissions"})
		return
	}

	if err := s.store.DeleteChannelGroupPermission(ctx, tc.OrganizationID, groupID, memberID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "permission deleted successfully",
	})
}
