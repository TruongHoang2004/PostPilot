package domain

import (
	"context"
	"encoding/json"
	"time"
)

// Default multi-tenant identity constants for single-tenant backward compatibility.
const (
	DefaultOrganizationID = "00000000-0000-0000-0000-000000000001"
	DefaultUserID         = "00000000-0000-0000-0000-000000000001"
)

// Organization Role type and levels.
type OrgRole string

const (
	OrgRoleOwner  OrgRole = "OWNER"
	OrgRoleAdmin  OrgRole = "ADMIN"
	OrgRoleMember OrgRole = "MEMBER"
)

// Channel Group Role type and levels (RBAC hierarchy: MANAGER > APPROVER > EDITOR > VIEWER).
type GroupRole string

const (
	GroupRoleManager  GroupRole = "MANAGER"
	GroupRoleApprover GroupRole = "APPROVER"
	GroupRoleEditor   GroupRole = "EDITOR"
	GroupRoleViewer   GroupRole = "VIEWER"
)

// Approval Action types.
type ApprovalAction string

const (
	ApprovalActionApproved ApprovalAction = "APPROVED"
	ApprovalActionRejected ApprovalAction = "REJECTED"
)

// User represents an authenticated identity in PostPilot.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	FullName     string    `json:"full_name"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Organization is an isolated multi-tenant team / workspace.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrganizationMember links a user to an organization with an assigned role.
type OrganizationMember struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	UserID         string    `json:"user_id"`
	OrgRole        OrgRole   `json:"org_role"`
	UserEmail      string    `json:"user_email,omitempty"`
	UserFullName   string    `json:"user_full_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// OrganizationInvitation stores pending email invitations to join an organization.
type OrganizationInvitation struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	Email          string     `json:"email"`
	OrgRole        OrgRole    `json:"org_role"`
	InvitedBy      string     `json:"invited_by"`
	TokenHash      string     `json:"-"`
	ExpiresAt      time.Time  `json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ChannelGroup groups channels together with approval workflows and RBAC permissions.
type ChannelGroup struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organization_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	RequireApproval bool      `json:"require_approval"`
	ChannelCount    int       `json:"channel_count,omitempty"`
	MemberCount     int       `json:"member_count,omitempty"`
	Channels        []Channel `json:"channels,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ChannelGroupItem maps channels into channel groups.
type ChannelGroupItem struct {
	ID             string       `json:"id"`
	ChannelGroupID string       `json:"channel_group_id"`
	ChannelID      string       `json:"channel_id"`
	ChannelName    string       `json:"channel_name,omitempty"`
	Platform       PlatformType `json:"platform,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
}

// ChannelGroupPermission maps users to channel groups with a specific group role.
type ChannelGroupPermission struct {
	ID             string    `json:"id"`
	ChannelGroupID string    `json:"channel_group_id"`
	UserID         string    `json:"user_id"`
	GroupRole      GroupRole `json:"group_role"`
	UserEmail      string    `json:"user_email,omitempty"`
	UserFullName   string    `json:"user_full_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// PostApproval records editorial decisions (APPROVED or REJECTED) on posts.
type PostApproval struct {
	ID           string         `json:"id"`
	PostID       string         `json:"post_id"`
	ReviewerID   string         `json:"reviewer_id"`
	ReviewerName string         `json:"reviewer_name,omitempty"`
	Action       ApprovalAction `json:"action"`
	Note         string         `json:"note,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// AuditLog records security, administrative, and publishing events within an organization.
type AuditLog struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	UserID         *string         `json:"user_id,omitempty"`
	Action         string          `json:"action"`
	EntityType     string          `json:"entity_type"`
	EntityID       string          `json:"entity_id"`
	Details        json.RawMessage `json:"details"`
	IPAddress      string          `json:"ip_address,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// TenantContext carries authenticated identity, organization scope, and resolved channel permissions.
type TenantContext struct {
	UserID             string               `json:"user_id"`
	UserEmail          string               `json:"user_email"`
	OrganizationID     string               `json:"organization_id"`
	OrgRole            OrgRole              `json:"org_role"`
	ChannelPermissions map[string]GroupRole `json:"channel_permissions,omitempty"`
}

type tenantCtxKey struct{}

// WithTenantContext attaches a TenantContext to the given context.
func WithTenantContext(ctx context.Context, tc *TenantContext) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tc)
}

// DefaultTenantContext returns a default tenant context pointing to DefaultOrganizationID, DefaultUserID, and OrgRoleOwner.
func DefaultTenantContext() *TenantContext {
	return &TenantContext{
		UserID:         DefaultUserID,
		UserEmail:      "admin@postpilot.local",
		OrganizationID: DefaultOrganizationID,
		OrgRole:        OrgRoleOwner,
	}
}

// TenantFromContext extracts the TenantContext from ctx, or returns a default tenant context if none is present.
func TenantFromContext(ctx context.Context) *TenantContext {
	if ctx != nil {
		if tc, ok := ctx.Value(tenantCtxKey{}).(*TenantContext); ok && tc != nil {
			return tc
		}
	}
	return DefaultTenantContext()
}

// IsOrgAdminOrOwner checks if the tenant context user has organization-wide administrative authority.
func (tc *TenantContext) IsOrgAdminOrOwner() bool {
	if tc == nil {
		return false
	}
	return tc.OrgRole == OrgRoleOwner || tc.OrgRole == OrgRoleAdmin
}

// groupRolePriority returns integer rank for comparing permission tiers.
func groupRolePriority(r GroupRole) int {
	switch r {
	case GroupRoleManager:
		return 4
	case GroupRoleApprover:
		return 3
	case GroupRoleEditor:
		return 2
	case GroupRoleViewer:
		return 1
	default:
		return 0
	}
}

// ResolveChannelRole resolves the effective channel role from multiple group roles
// using the "Most Permissive Wins" rule: MANAGER > APPROVER > EDITOR > VIEWER.
func ResolveChannelRole(roles []GroupRole) GroupRole {
	var best GroupRole
	bestPri := 0
	for _, r := range roles {
		if p := groupRolePriority(r); p > bestPri {
			best = r
			bestPri = p
		}
	}
	return best
}

// ResolveChannelRole resolves the effective channel role for this context,
// granting MANAGER if the user is an Org Owner/Admin.
func (tc *TenantContext) ResolveChannelRole(roles []GroupRole) GroupRole {
	if tc != nil && tc.IsOrgAdminOrOwner() {
		return GroupRoleManager
	}
	return ResolveChannelRole(roles)
}

// CanManageChannel checks if a role allows channel configuration & credentials management (MANAGER only).
func CanManageChannel(role GroupRole) bool {
	return role == GroupRoleManager
}

// CanManageChannel checks if the tenant context user can manage the channel.
func (tc *TenantContext) CanManageChannel(role GroupRole) bool {
	if tc != nil && tc.IsOrgAdminOrOwner() {
		return true
	}
	return CanManageChannel(role)
}

// CanCreatePost checks if a role allows authoring posts (MANAGER, APPROVER, EDITOR).
func CanCreatePost(role GroupRole) bool {
	switch role {
	case GroupRoleManager, GroupRoleApprover, GroupRoleEditor:
		return true
	default:
		return false
	}
}

// CanCreatePost checks if the tenant context user can author posts.
func (tc *TenantContext) CanCreatePost(role GroupRole) bool {
	if tc != nil && tc.IsOrgAdminOrOwner() {
		return true
	}
	return CanCreatePost(role)
}

// CanPublishDirectly checks if a role allows publishing without going through approval.
// If requireApproval is true, only MANAGER and APPROVER can bypass approval; EDITOR cannot.
// If requireApproval is false, MANAGER, APPROVER, and EDITOR can all publish directly.
// VIEWER cannot publish in either case.
func CanPublishDirectly(role GroupRole, requireApproval bool) bool {
	switch role {
	case GroupRoleManager, GroupRoleApprover:
		return true
	case GroupRoleEditor:
		return !requireApproval
	default:
		return false
	}
}

// CanPublishDirectly checks if the tenant context user can publish without approval.
func (tc *TenantContext) CanPublishDirectly(role GroupRole, requireApproval bool) bool {
	if tc != nil && tc.IsOrgAdminOrOwner() {
		return true
	}
	return CanPublishDirectly(role, requireApproval)
}

// CanApprovePost checks if a role allows approving or rejecting posts (MANAGER, APPROVER).
func CanApprovePost(role GroupRole) bool {
	return role == GroupRoleManager || role == GroupRoleApprover
}

// CanApprovePost checks if the tenant context user can approve posts.
func (tc *TenantContext) CanApprovePost(role GroupRole) bool {
	if tc != nil && tc.IsOrgAdminOrOwner() {
		return true
	}
	return CanApprovePost(role)
}

// CanViewChannel checks if a role allows viewing channels and their posts (MANAGER, APPROVER, EDITOR, VIEWER).
func CanViewChannel(role GroupRole) bool {
	switch role {
	case GroupRoleManager, GroupRoleApprover, GroupRoleEditor, GroupRoleViewer:
		return true
	default:
		return false
	}
}

// CanViewChannel checks if the tenant context user can view the channel.
func (tc *TenantContext) CanViewChannel(role GroupRole) bool {
	if tc != nil && tc.IsOrgAdminOrOwner() {
		return true
	}
	return CanViewChannel(role)
}

// ChannelRole returns the effective group role for the given channel ID from the context.
func (tc *TenantContext) ChannelRole(channelID string) GroupRole {
	if tc == nil {
		return ""
	}
	if tc.IsOrgAdminOrOwner() {
		return GroupRoleManager
	}
	if tc.ChannelPermissions == nil {
		return ""
	}
	return tc.ChannelPermissions[channelID]
}
