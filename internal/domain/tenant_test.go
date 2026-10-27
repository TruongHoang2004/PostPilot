package domain

import (
	"testing"
	"time"
)

func TestTenantContext_OrgRoleChecks(t *testing.T) {
	ownerCtx := &TenantContext{
		UserID:         "user-owner",
		UserEmail:      "owner@example.com",
		OrganizationID: "org-1",
		OrgRole:        OrgRoleOwner,
	}
	if !ownerCtx.IsOrgAdminOrOwner() {
		t.Fatalf("expected owner to be org admin or owner")
	}

	adminCtx := &TenantContext{
		UserID:         "user-admin",
		UserEmail:      "admin@example.com",
		OrganizationID: "org-1",
		OrgRole:        OrgRoleAdmin,
	}
	if !adminCtx.IsOrgAdminOrOwner() {
		t.Fatalf("expected admin to be org admin or owner")
	}

	memberCtx := &TenantContext{
		UserID:         "user-member",
		UserEmail:      "member@example.com",
		OrganizationID: "org-1",
		OrgRole:        OrgRoleMember,
	}
	if memberCtx.IsOrgAdminOrOwner() {
		t.Fatalf("expected member NOT to be org admin or owner")
	}

	var nilCtx *TenantContext
	if nilCtx.IsOrgAdminOrOwner() {
		t.Fatalf("expected nil context NOT to be org admin or owner")
	}
}

func TestResolveChannelRole_MostPermissiveWins(t *testing.T) {
	cases := []struct {
		name     string
		roles    []GroupRole
		expected GroupRole
	}{
		{
			name:     "single viewer",
			roles:    []GroupRole{GroupRoleViewer},
			expected: GroupRoleViewer,
		},
		{
			name:     "viewer and editor",
			roles:    []GroupRole{GroupRoleViewer, GroupRoleEditor},
			expected: GroupRoleEditor,
		},
		{
			name:     "editor and approver",
			roles:    []GroupRole{GroupRoleEditor, GroupRoleApprover},
			expected: GroupRoleApprover,
		},
		{
			name:     "viewer, editor, and manager",
			roles:    []GroupRole{GroupRoleViewer, GroupRoleEditor, GroupRoleManager},
			expected: GroupRoleManager,
		},
		{
			name:     "approver and manager",
			roles:    []GroupRole{GroupRoleApprover, GroupRoleManager},
			expected: GroupRoleManager,
		},
		{
			name:     "empty roles",
			roles:    []GroupRole{},
			expected: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved := ResolveChannelRole(tc.roles)
			if resolved != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, resolved)
			}
		})
	}
}

func TestTenantContext_PermissionsAndOverrides(t *testing.T) {
	// Member with specific channel permissions
	memberCtx := &TenantContext{
		UserID:         "user-editor",
		UserEmail:      "editor@example.com",
		OrganizationID: "org-1",
		OrgRole:        OrgRoleMember,
		ChannelPermissions: map[string]GroupRole{
			"ch-1": GroupRoleEditor,
			"ch-2": GroupRoleViewer,
			"ch-3": GroupRoleManager,
			"ch-4": GroupRoleApprover,
		},
	}

	// 1. CanManageChannel
	if !CanManageChannel(GroupRoleManager) {
		t.Fatalf("manager must be able to manage channel")
	}
	if CanManageChannel(GroupRoleApprover) || CanManageChannel(GroupRoleEditor) || CanManageChannel(GroupRoleViewer) {
		t.Fatalf("non-manager cannot manage channel")
	}
	if memberCtx.CanManageChannel(memberCtx.ChannelRole("ch-1")) {
		t.Fatalf("editor must not manage channel")
	}
	if !memberCtx.CanManageChannel(memberCtx.ChannelRole("ch-3")) {
		t.Fatalf("manager must be able to manage channel via context")
	}

	// 2. CanCreatePost
	if !CanCreatePost(GroupRoleManager) || !CanCreatePost(GroupRoleApprover) || !CanCreatePost(GroupRoleEditor) {
		t.Fatalf("manager, approver, and editor must be able to create posts")
	}
	if CanCreatePost(GroupRoleViewer) {
		t.Fatalf("viewer must not be able to create posts")
	}

	// 3. CanPublishDirectly
	// When requireApproval == true:
	if !CanPublishDirectly(GroupRoleManager, true) || !CanPublishDirectly(GroupRoleApprover, true) {
		t.Fatalf("manager and approver can publish directly even when requireApproval is true")
	}
	if CanPublishDirectly(GroupRoleEditor, true) {
		t.Fatalf("editor must NOT publish directly when requireApproval is true")
	}
	if CanPublishDirectly(GroupRoleViewer, true) {
		t.Fatalf("viewer must NOT publish directly")
	}

	// When requireApproval == false:
	if !CanPublishDirectly(GroupRoleEditor, false) {
		t.Fatalf("editor CAN publish directly when requireApproval is false")
	}
	if CanPublishDirectly(GroupRoleViewer, false) {
		t.Fatalf("viewer must NOT publish directly even when requireApproval is false")
	}

	// 4. CanApprovePost
	if !CanApprovePost(GroupRoleManager) || !CanApprovePost(GroupRoleApprover) {
		t.Fatalf("manager and approver can approve posts")
	}
	if CanApprovePost(GroupRoleEditor) || CanApprovePost(GroupRoleViewer) {
		t.Fatalf("editor and viewer cannot approve posts")
	}

	// 5. CanViewChannel
	if !CanViewChannel(GroupRoleManager) || !CanViewChannel(GroupRoleApprover) ||
		!CanViewChannel(GroupRoleEditor) || !CanViewChannel(GroupRoleViewer) {
		t.Fatalf("all roles can view channel")
	}

	// 6. Org Admin / Owner override
	adminCtx := &TenantContext{
		UserID:         "admin-1",
		UserEmail:      "admin@example.com",
		OrganizationID: "org-1",
		OrgRole:        OrgRoleAdmin,
	}
	if !adminCtx.CanManageChannel(GroupRoleViewer) {
		t.Fatalf("org admin should have management access regardless of channel role")
	}
	if !adminCtx.CanCreatePost(GroupRoleViewer) {
		t.Fatalf("org admin should have post creation access regardless of channel role")
	}
	if !adminCtx.CanPublishDirectly(GroupRoleViewer, true) {
		t.Fatalf("org admin should be able to publish directly regardless of approval setting")
	}
	if !adminCtx.CanApprovePost(GroupRoleViewer) {
		t.Fatalf("org admin should be able to approve posts regardless of channel role")
	}
	if adminCtx.ResolveChannelRole([]GroupRole{GroupRoleViewer}) != GroupRoleManager {
		t.Fatalf("org admin resolved role must be MANAGER")
	}
}

func TestPostApprovalWorkflowMethods(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	future := now.Add(2 * time.Hour)

	post := &Post{
		ID:          "post-100",
		ChannelID:   "ch-1",
		Content:     "Needs approval",
		Status:      PostStatusPendingApproval,
		ScheduledAt: future,
	}

	if !post.CanReview() {
		t.Fatalf("expected pending approval post to be reviewable")
	}

	// 1. Approve for future schedule
	post.Approve("reviewer-1", now)
	if post.Status != PostStatusScheduled {
		t.Fatalf("expected status SCHEDULED for future post, got: %s", post.Status)
	}
	if post.ReviewedBy != "reviewer-1" || post.ReviewedAt == nil {
		t.Fatalf("expected reviewer details to be set")
	}
	if post.CanReview() {
		t.Fatalf("approved post must not be reviewable")
	}

	// 2. Reject post
	post.Status = PostStatusPendingApproval
	post.Reject("reviewer-2", "Violates brand guidelines", now)
	if post.Status != PostStatusRejected {
		t.Fatalf("expected status REJECTED, got: %s", post.Status)
	}
	if post.RejectionNote != "Violates brand guidelines" {
		t.Fatalf("expected rejection note, got: %s", post.RejectionNote)
	}
	if !post.CanCancel() {
		t.Fatalf("rejected post must be cancellable")
	}

	// 3. Resubmit post
	post.Resubmit(now.Add(10 * time.Minute))
	if post.Status != PostStatusPendingApproval {
		t.Fatalf("expected status PENDING_APPROVAL after resubmit, got: %s", post.Status)
	}
	if post.RejectionNote != "" || post.ReviewedBy != "" || post.ReviewedAt != nil {
		t.Fatalf("resubmit should clear review fields")
	}

	// 4. Approve for past / immediate schedule -> QUEUED
	post.ScheduledAt = now.Add(-10 * time.Minute)
	post.Approve("reviewer-3", now)
	if post.Status != PostStatusQueued {
		t.Fatalf("expected status QUEUED for past/immediate post, got: %s", post.Status)
	}

	// 5. Default organization id on creation
	newPost := &Post{
		ChannelID: "ch-2",
		Content:   "Default org test",
	}
	newPost.PrepareForCreation(false, now)
	if newPost.OrganizationID != DefaultOrganizationID {
		t.Fatalf("expected DefaultOrganizationID, got: %s", newPost.OrganizationID)
	}
}
