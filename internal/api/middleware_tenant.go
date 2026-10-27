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

// tenantMiddleware extracts and attaches tenant and identity context to incoming requests.
func (s *Server) tenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bypass tenant extraction for CORS preflight
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		var tc *domain.TenantContext

		// 1. Check Authorization: Bearer <jwt_token>
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			if tokenStr != "" {
				claims, err := crypto.VerifyJWT(tokenStr, s.cfg.APIAdminKey)
				if err != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]any{
						"error": "invalid or expired authorization token: " + err.Error(),
					})
					return
				}

				targetOrgID := claims.OrganizationID
				if reqOrg := r.Header.Get("X-Organization-Id"); reqOrg != "" {
					targetOrgID = reqOrg
				} else if queryOrg := r.URL.Query().Get("org_id"); queryOrg != "" {
					targetOrgID = queryOrg
				}

				userOrgRole := domain.OrgRole(claims.OrgRole)
				if userOrgRole == "" {
					userOrgRole = domain.OrgRoleMember
				}

				// If requesting a different org than claims, verify membership
				if targetOrgID != "" && targetOrgID != claims.OrganizationID {
					members, err := s.store.ListOrganizationMembers(r.Context(), targetOrgID)
					if err != nil {
						writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to verify organization membership"})
						return
					}
					found := false
					for _, m := range members {
						if m.UserID == claims.UserID {
							found = true
							userOrgRole = m.OrgRole
							break
						}
					}
					if !found {
						writeJSON(w, http.StatusForbidden, map[string]any{"error": "user does not belong to requested organization"})
						return
					}
				}

				if targetOrgID == "" {
					targetOrgID = domain.DefaultOrganizationID
				}

				tc = &domain.TenantContext{
					UserID:             claims.UserID,
					UserEmail:          claims.Email,
					OrganizationID:     targetOrgID,
					OrgRole:            userOrgRole,
					ChannelPermissions: make(map[string]domain.GroupRole),
				}
			}
		}

		// 2. Fallback to X-Admin-Key if no JWT
		if tc == nil {
			adminKey := r.Header.Get("X-Admin-Key")
			if adminKey != "" && s.cfg.APIAdminKey != "" && adminKey == s.cfg.APIAdminKey {
				targetOrgID := domain.DefaultOrganizationID
				if reqOrg := r.Header.Get("X-Organization-Id"); reqOrg != "" {
					targetOrgID = reqOrg
				} else if queryOrg := r.URL.Query().Get("org_id"); queryOrg != "" {
					targetOrgID = queryOrg
				}
				tc = &domain.TenantContext{
					UserID:             domain.DefaultUserID,
					UserEmail:          "admin@postpilot.local",
					OrganizationID:     targetOrgID,
					OrgRole:            domain.OrgRoleOwner,
					ChannelPermissions: make(map[string]domain.GroupRole),
				}
			}
		}

		// 3. Fallback to DefaultTenantContext if neither JWT nor Admin-Key
		if tc == nil {
			reqOrg := strings.TrimSpace(r.Header.Get("X-Organization-Id"))
			queryOrg := strings.TrimSpace(r.URL.Query().Get("org_id"))
			if reqOrg != "" && reqOrg != domain.DefaultOrganizationID {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"error": "authentication required to access organization",
				})
				return
			}
			if queryOrg != "" && queryOrg != domain.DefaultOrganizationID {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"error": "authentication required to access organization",
				})
				return
			}

			tc = domain.DefaultTenantContext()
			tc.ChannelPermissions = make(map[string]domain.GroupRole)
		}

		// 4. Handle X-Simulate-Role and X-User-Id for testing/simulation
		if simRole := strings.TrimSpace(r.Header.Get("X-Simulate-Role")); simRole != "" {
			upperRole := strings.ToUpper(simRole)
			switch upperRole {
			case "OWNER":
				tc.OrgRole = domain.OrgRoleOwner
			case "ADMIN":
				tc.OrgRole = domain.OrgRoleAdmin
			case "MEMBER":
				tc.OrgRole = domain.OrgRoleMember
			case "MANAGER", "APPROVER", "EDITOR", "VIEWER":
				tc.OrgRole = domain.OrgRoleMember
				if tc.ChannelPermissions == nil {
					tc.ChannelPermissions = make(map[string]domain.GroupRole)
				}
				tc.ChannelPermissions["*"] = domain.GroupRole(upperRole)
			}
		}
		if simUserID := strings.TrimSpace(r.Header.Get("X-User-Id")); simUserID != "" {
			tc.UserID = simUserID
		}

		ctx := domain.WithTenantContext(r.Context(), tc)
		next.ServeHTTP(w, r.WithContext(ctx))
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
		if wildcardRole, ok := tc.ChannelPermissions["*"]; ok && wildcardRole != "" {
			return wildcardRole, true
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
