package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// handleAuthRegister creates a new user account, provisions an initial organization, and returns a signed JWT.
func (s *Server) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "email and password are required"})
		return
	}

	hashedPassword, err := crypto.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to hash password"})
		return
	}

	userEntity := &domain.User{
		Email:        req.Email,
		PasswordHash: hashedPassword,
		FullName:     req.FullName,
		IsActive:     true,
	}

	createdUser, err := s.store.CreateUser(ctx, userEntity)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}

	orgName := req.FullName
	if strings.TrimSpace(orgName) == "" {
		orgName = "My Workspace"
	} else {
		orgName = strings.TrimSpace(orgName) + "'s Team"
	}
	orgSlug := generateSlug(orgName)

	createdOrg, err := s.store.CreateOrganization(ctx, &domain.Organization{
		Name: orgName,
		Slug: orgSlug,
	}, createdUser.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create organization: " + err.Error()})
		return
	}

	claims := crypto.JWTClaims{
		UserID:         createdUser.ID,
		Email:          createdUser.Email,
		OrganizationID: createdOrg.ID,
		OrgRole:        string(domain.OrgRoleOwner),
		Exp:            time.Now().Add(7 * 24 * time.Hour).Unix(),
		Iat:            time.Now().Unix(),
	}

	token, err := crypto.SignJWT(claims, s.cfg.APIAdminKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to sign jwt token"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"token":        token,
		"user":         createdUser,
		"organization": createdOrg,
	})
}

// handleAuthLogin verifies user credentials and issues an authenticated JWT token.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	user, err := s.store.GetUserByEmail(ctx, req.Email)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid email or password"})
		return
	}

	if !crypto.VerifyPassword(req.Password, user.PasswordHash) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid email or password"})
		return
	}

	orgs, err := s.store.ListUserOrganizations(ctx, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to list organizations"})
		return
	}

	primaryOrgID := domain.DefaultOrganizationID
	primaryRole := domain.OrgRoleMember
	if len(orgs) > 0 {
		primaryOrgID = orgs[0].ID
		members, _ := s.store.ListOrganizationMembers(ctx, primaryOrgID)
		for _, m := range members {
			if m.UserID == user.ID {
				primaryRole = m.OrgRole
				break
			}
		}
	}

	claims := crypto.JWTClaims{
		UserID:         user.ID,
		Email:          user.Email,
		OrganizationID: primaryOrgID,
		OrgRole:        string(primaryRole),
		Exp:            time.Now().Add(7 * 24 * time.Hour).Unix(),
		Iat:            time.Now().Unix(),
	}

	token, err := crypto.SignJWT(claims, s.cfg.APIAdminKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to sign jwt token"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":         token,
		"user":          user,
		"organizations": orgs,
	})
}

// handleAuthMe returns current authenticated identity, scoped organization, and member details.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)

	user, err := s.store.GetUserByID(ctx, tc.UserID)
	if err != nil || user == nil {
		user = &domain.User{
			ID:       tc.UserID,
			Email:    tc.UserEmail,
			FullName: "Admin User",
			IsActive: true,
		}
	}

	org, err := s.store.GetOrganizationByID(ctx, tc.OrganizationID)
	if err != nil || org == nil {
		org = &domain.Organization{
			ID:   tc.OrganizationID,
			Name: "Default Organization",
			Slug: "default-org",
		}
	}

	var member *domain.OrganizationMember
	members, err := s.store.ListOrganizationMembers(ctx, tc.OrganizationID)
	if err == nil {
		for _, m := range members {
			if m.UserID == tc.UserID {
				member = &m
				break
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user":         user,
		"organization": org,
		"member":       member,
	})
}

// handleListOrganizations returns all organizations accessible to the authenticated user.
func (s *Server) handleListOrganizations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)

	orgs, err := s.store.ListUserOrganizations(ctx, tc.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"organizations": orgs,
	})
}

// handleCreateOrganization provisions a new organization with the current user as OWNER.
func (s *Server) handleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)

	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
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
	if req.Slug == "" {
		req.Slug = generateSlug(req.Name)
	}

	org, err := s.store.CreateOrganization(ctx, &domain.Organization{
		Name: req.Name,
		Slug: req.Slug,
	}, tc.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, org)
}

// handleListOrganizationMembers lists all members of the specified organization.
func (s *Server) handleListOrganizationMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID := r.PathValue("id")
	if orgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "organization id is required"})
		return
	}

	members, err := s.store.ListOrganizationMembers(ctx, orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"members": members,
	})
}

// handleAddOrganizationMember adds or invites a user to the organization.
func (s *Server) handleAddOrganizationMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)
	orgID := r.PathValue("id")
	if orgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "organization id is required"})
		return
	}

	// RBAC check: only OWNER or ADMIN can invite/add members to their organization
	if tc.OrganizationID == orgID && !tc.IsOrgAdminOrOwner() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: only organization owner or admin can add members"})
		return
	}

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "email is required"})
		return
	}
	role := domain.OrgRole(strings.ToUpper(strings.TrimSpace(req.Role)))
	if role != domain.OrgRoleOwner && role != domain.OrgRoleAdmin && role != domain.OrgRoleMember {
		role = domain.OrgRoleMember
	}

	existingUser, err := s.store.GetUserByEmail(ctx, req.Email)
	if err == nil && existingUser != nil {
		member, err := s.store.AddOrganizationMember(ctx, &domain.OrganizationMember{
			OrganizationID: orgID,
			UserID:         existingUser.ID,
			OrgRole:        role,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"message": "member added successfully",
			"member":  member,
		})
		return
	}

	token := generateRandomHex(16)
	invitation, err := s.store.CreateInvitation(ctx, &domain.OrganizationInvitation{
		OrganizationID: orgID,
		Email:          req.Email,
		OrgRole:        role,
		InvitedBy:      tc.UserID,
		TokenHash:      token,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"message":    "invitation created successfully",
		"invitation": invitation,
		"token":      token,
	})
}

// handleRemoveOrganizationMember removes a user membership from the organization.
func (s *Server) handleRemoveOrganizationMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)
	orgID := r.PathValue("id")
	memberID := r.PathValue("memberId")

	if tc.OrganizationID == orgID && !tc.IsOrgAdminOrOwner() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: only organization owner or admin can remove members"})
		return
	}

	if err := s.store.RemoveOrganizationMember(ctx, orgID, memberID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "member removed successfully",
	})
}

// handleAcceptInvitation processes an invitation token and links the user to the organization.
func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "token is required"})
		return
	}

	member, err := s.store.AcceptInvitation(ctx, req.Token, tc.UserID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to accept invitation: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "invitation accepted successfully",
		"member":  member,
	})
}

// handleListAuditLogs returns compliance and audit logs for the current organization.
func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := domain.TenantFromContext(ctx)

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	logs, err := s.store.ListAuditLogs(ctx, tc.OrganizationID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"audit_logs": logs,
	})
}

func generateSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var buf strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			buf.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			if buf.Len() > 0 && !strings.HasSuffix(buf.String(), "-") {
				buf.WriteRune('-')
			}
		}
	}
	res := strings.Trim(buf.String(), "-")
	if res == "" {
		res = "team-" + generateRandomHex(4)
	}
	return res
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
