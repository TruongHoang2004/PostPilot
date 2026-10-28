package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// tenantMiddleware requires an authenticated, active user and current organization membership.
func (s *Server) tenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		public := r.Method == http.MethodOptions || (r.Method == http.MethodPost && (r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/register")) || (r.Method == http.MethodGet && (r.URL.Path == "/healthz" || r.URL.Path == "/api/v1/auth/facebook/callback")) || r.URL.Path == "/api/v1/webhooks/facebook"
		if public {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeJSON(w, 401, map[string]any{"error": "authentication required"})
			return
		}
		claims, err := crypto.VerifyJWT(strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")), s.cfg.APIAdminKey)
		if err != nil || claims.UserID == "" {
			writeJSON(w, 401, map[string]any{"error": "invalid or expired authorization token"})
			return
		}
		user, err := s.store.GetUserByID(r.Context(), claims.UserID)
		if err != nil || user == nil || !user.IsActive {
			writeJSON(w, 401, map[string]any{"error": "account unavailable"})
			return
		}
		orgID := claims.OrganizationID
		if v := r.Header.Get("X-Organization-Id"); v != "" {
			orgID = v
		} else if v := r.URL.Query().Get("org_id"); v != "" {
			orgID = v
		}
		if orgID == "" {
			writeJSON(w, 403, map[string]any{"error": "organization membership required"})
			return
		}
		members, err := s.store.ListOrganizationMembers(r.Context(), orgID)
		if err != nil {
			writeJSON(w, 503, map[string]any{"error": "unable to verify organization membership"})
			return
		}
		for _, member := range members {
			if member.UserID == user.ID {
				tc := &domain.TenantContext{UserID: user.ID, UserEmail: user.Email, OrganizationID: orgID, OrgRole: member.OrgRole, ChannelPermissions: make(map[string]domain.GroupRole)}
				next.ServeHTTP(w, r.WithContext(domain.WithTenantContext(r.Context(), tc)))
				return
			}
		}
		writeJSON(w, 403, map[string]any{"error": "user does not belong to requested organization"})
	})
}

// resolveChannelPermission determines the effective Channel Group Role for the current context.
func (s *Server) resolveChannelPermission(ctx context.Context, channelID string) (domain.GroupRole, bool) {
	tc := domain.TenantFromContext(ctx)
	if tc == nil {
		return "", false
	}
	if tc.IsOrgAdminOrOwner() {
		return domain.GroupRoleManager, true
	}
	if tc.ChannelPermissions != nil {
		if role, ok := tc.ChannelPermissions[channelID]; ok && role != "" {
			return role, true
		}
	}
	roles, err := s.store.GetUserChannelRoles(ctx, tc.OrganizationID, tc.UserID, channelID)
	if err == nil && len(roles) > 0 {
		resolved := domain.ResolveChannelRole(roles)
		return resolved, true
	}
	return "", false
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}
