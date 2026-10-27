package database

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

func TestTenantContextFallback(t *testing.T) {
	// 1. Nil context fallback
	tc1 := TenantFromContext(nil)
	if tc1.OrganizationID != domain.DefaultOrganizationID || tc1.UserID != domain.DefaultUserID || tc1.OrgRole != domain.OrgRoleOwner {
		t.Fatalf("expected default tenant context on nil context, got: %+v", tc1)
	}

	// 2. Background context without tenant context fallback
	tc2 := TenantFromContext(context.Background())
	if tc2.OrganizationID != domain.DefaultOrganizationID || tc2.UserID != domain.DefaultUserID || tc2.OrgRole != domain.OrgRoleOwner {
		t.Fatalf("expected default tenant context on background context, got: %+v", tc2)
	}

	// 3. Explicit tenant context
	custom := &domain.TenantContext{
		UserID:         "user-123",
		UserEmail:      "user@example.com",
		OrganizationID: "org-abc",
		OrgRole:        domain.OrgRoleAdmin,
	}
	ctx := WithTenantContext(context.Background(), custom)
	tc3 := TenantFromContext(ctx)
	if tc3.OrganizationID != "org-abc" || tc3.UserID != "user-123" || tc3.OrgRole != domain.OrgRoleAdmin {
		t.Fatalf("expected custom tenant context, got: %+v", tc3)
	}
}

func TestInMemoryStore_SeededDefaults(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	// Default user exists
	user, err := store.GetUserByID(ctx, domain.DefaultUserID)
	if err != nil || user == nil {
		t.Fatalf("expected default user to be seeded, got err: %v", err)
	}
	if user.Email != "admin@postpilot.local" {
		t.Fatalf("unexpected default user email: %s", user.Email)
	}

	// Default organization exists
	org, err := store.GetOrganizationByID(ctx, domain.DefaultOrganizationID)
	if err != nil || org == nil {
		t.Fatalf("expected default organization to be seeded, got err: %v", err)
	}
	if org.Slug != "default-workspace" {
		t.Fatalf("unexpected default org slug: %s", org.Slug)
	}

	// Default member exists
	members, err := store.ListOrganizationMembers(ctx, domain.DefaultOrganizationID)
	if err != nil || len(members) == 0 {
		t.Fatalf("expected default member to be seeded, got: %v", err)
	}
	if members[0].UserID != domain.DefaultUserID || members[0].OrgRole != domain.OrgRoleOwner {
		t.Fatalf("unexpected default member role: %+v", members[0])
	}
}

func TestInMemoryStore_Users(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	// 1. CreateUser
	newUser := &domain.User{
		Email:        "editor@agency.com",
		FullName:     "Jane Editor",
		PasswordHash: "hashed_secret",
		IsActive:     true,
	}
	created, err := store.CreateUser(ctx, newUser)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected non-empty user ID")
	}

	// 2. GetUserByEmail (case insensitive)
	byEmail, err := store.GetUserByEmail(ctx, "EDITOR@AGENCY.COM")
	if err != nil {
		t.Fatalf("get user by email failed: %v", err)
	}
	if byEmail.ID != created.ID || byEmail.FullName != "Jane Editor" {
		t.Fatalf("unexpected user details: %+v", byEmail)
	}

	// 3. GetUserByID
	byID, err := store.GetUserByID(ctx, created.ID)
	if err != nil || byID.Email != "editor@agency.com" {
		t.Fatalf("get user by ID failed: %v", err)
	}

	// 4. Duplicate email fails
	_, err = store.CreateUser(ctx, &domain.User{
		Email: "editor@agency.com",
	})
	if err == nil {
		t.Fatalf("expected error on duplicate email, got nil")
	}

	// 5. Not found returns error
	_, err = store.GetUserByID(ctx, "non-existent-id")
	if err == nil {
		t.Fatalf("expected error for non-existent user, got nil")
	}
}

func TestInMemoryStore_OrganizationsAndMembers(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	// 1. Create owner user
	owner, _ := store.CreateUser(ctx, &domain.User{
		Email:    "founder@startup.io",
		FullName: "Alice Founder",
	})

	// 2. Create organization with owner
	org, err := store.CreateOrganization(ctx, &domain.Organization{
		Name: "Acme Corp",
		Slug: "acme-corp",
	}, owner.ID)
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}

	// 3. Verify owner is listed in organization members
	members, err := store.ListOrganizationMembers(ctx, org.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("expected 1 member in new org, got %d (err: %v)", len(members), err)
	}
	if members[0].UserID != owner.ID || members[0].OrgRole != domain.OrgRoleOwner {
		t.Fatalf("expected owner role, got: %+v", members[0])
	}
	if members[0].UserEmail != "founder@startup.io" || members[0].UserFullName != "Alice Founder" {
		t.Fatalf("expected member user details to be populated: %+v", members[0])
	}

	// 4. ListUserOrganizations
	userOrgs, err := store.ListUserOrganizations(ctx, owner.ID)
	if err != nil || len(userOrgs) != 1 || userOrgs[0].ID != org.ID {
		t.Fatalf("expected user to belong to Acme Corp, got: %+v", userOrgs)
	}

	// 5. Add second member
	staff, _ := store.CreateUser(ctx, &domain.User{
		Email:    "bob@startup.io",
		FullName: "Bob Staff",
	})
	addedMember, err := store.AddOrganizationMember(ctx, &domain.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         staff.ID,
		OrgRole:        domain.OrgRoleMember,
	})
	if err != nil {
		t.Fatalf("add organization member failed: %v", err)
	}
	if addedMember.UserEmail != "bob@startup.io" {
		t.Fatalf("expected user email on added member, got: %s", addedMember.UserEmail)
	}

	// 6. Update member role to ADMIN
	err = store.UpdateOrganizationMemberRole(ctx, org.ID, staff.ID, domain.OrgRoleAdmin)
	if err != nil {
		t.Fatalf("update member role failed: %v", err)
	}
	membersAfterUpdate, _ := store.ListOrganizationMembers(ctx, org.ID)
	var bobRole domain.OrgRole
	for _, m := range membersAfterUpdate {
		if m.UserID == staff.ID {
			bobRole = m.OrgRole
		}
	}
	if bobRole != domain.OrgRoleAdmin {
		t.Fatalf("expected Bob's role to be ADMIN, got %s", bobRole)
	}

	// 7. Remove member
	err = store.RemoveOrganizationMember(ctx, org.ID, staff.ID)
	if err != nil {
		t.Fatalf("remove member failed: %v", err)
	}
	membersAfterRemove, _ := store.ListOrganizationMembers(ctx, org.ID)
	if len(membersAfterRemove) != 1 {
		t.Fatalf("expected 1 member after removal, got %d", len(membersAfterRemove))
	}
}

func TestInMemoryStore_Invitations(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	inv := &domain.OrganizationInvitation{
		OrganizationID: domain.DefaultOrganizationID,
		Email:          "invitee@example.com",
		OrgRole:        domain.OrgRoleAdmin,
		InvitedBy:      domain.DefaultUserID,
		TokenHash:      "token-hash-123",
		ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
	}
	createdInv, err := store.CreateInvitation(ctx, inv)
	if err != nil {
		t.Fatalf("create invitation failed: %v", err)
	}

	// Lookup by token
	fetched, err := store.GetInvitationByToken(ctx, "token-hash-123")
	if err != nil || fetched.ID != createdInv.ID {
		t.Fatalf("get invitation by token failed: %v", err)
	}

	// Accept invitation
	newUser, _ := store.CreateUser(ctx, &domain.User{
		Email:    "invitee@example.com",
		FullName: "Invited User",
	})
	mem, err := store.AcceptInvitation(ctx, "token-hash-123", newUser.ID)
	if err != nil {
		t.Fatalf("accept invitation failed: %v", err)
	}
	if mem.OrgRole != domain.OrgRoleAdmin || mem.OrganizationID != domain.DefaultOrganizationID {
		t.Fatalf("unexpected member after accepting invite: %+v", mem)
	}

	// Re-accepting fails
	_, err = store.AcceptInvitation(ctx, "token-hash-123", newUser.ID)
	if err == nil {
		t.Fatalf("expected error on re-accepting invitation, got nil")
	}

	// Expired invitation fails
	expiredInv := &domain.OrganizationInvitation{
		OrganizationID: domain.DefaultOrganizationID,
		Email:          "expired@example.com",
		OrgRole:        domain.OrgRoleMember,
		InvitedBy:      domain.DefaultUserID,
		TokenHash:      "expired-token",
		ExpiresAt:      time.Now().UTC().Add(-1 * time.Hour),
	}
	_, _ = store.CreateInvitation(ctx, expiredInv)
	_, err = store.AcceptInvitation(ctx, "expired-token", newUser.ID)
	if err == nil {
		t.Fatalf("expected error accepting expired invitation, got nil")
	}
}

func TestInMemoryStore_ChannelGroupsAndPermissions(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	orgID := "org-media-co"
	user, _ := store.CreateUser(ctx, &domain.User{
		Email:    "manager@mediaco.com",
		FullName: "Media Manager",
	})

	// 1. Create channel group
	group, err := store.CreateChannelGroup(ctx, &domain.ChannelGroup{
		OrganizationID:  orgID,
		Name:            "Vietnam Marketing",
		Description:     "All channels for Vietnam campaigns",
		RequireApproval: true,
	})
	if err != nil {
		t.Fatalf("create channel group failed: %v", err)
	}

	// 2. Create channels and map to group
	ch1, _ := store.UpsertChannel(ctx, &domain.Channel{
		OrganizationID: orgID,
		Platform:       domain.PlatformFacebook,
		ExternalID:     "fb-vn-1",
		Name:           "VN Page 1",
		IsActive:       true,
	})
	ch2, _ := store.UpsertChannel(ctx, &domain.Channel{
		OrganizationID: orgID,
		Platform:       domain.PlatformFacebook,
		ExternalID:     "fb-vn-2",
		Name:           "VN Page 2",
		IsActive:       true,
	})

	err = store.SetChannelGroupChannels(ctx, orgID, group.ID, []string{ch1.ID, ch2.ID})
	if err != nil {
		t.Fatalf("set channel group channels failed: %v", err)
	}

	// 3. Set permission
	perm, err := store.SetChannelGroupPermission(ctx, &domain.ChannelGroupPermission{
		ChannelGroupID: group.ID,
		UserID:         user.ID,
		GroupRole:      domain.GroupRoleManager,
	})
	if err != nil {
		t.Fatalf("set channel group permission failed: %v", err)
	}
	if perm.GroupRole != domain.GroupRoleManager || perm.UserEmail != "manager@mediaco.com" {
		t.Fatalf("unexpected permission: %+v", perm)
	}

	// 4. GetChannelGroupByID
	fetchedGroup, err := store.GetChannelGroupByID(ctx, orgID, group.ID)
	if err != nil {
		t.Fatalf("get channel group failed: %v", err)
	}
	if fetchedGroup.ChannelCount != 2 || fetchedGroup.MemberCount != 1 || len(fetchedGroup.Channels) != 2 {
		t.Fatalf("expected 2 channels and 1 member in group, got count=%d, members=%d, channels=%d",
			fetchedGroup.ChannelCount, fetchedGroup.MemberCount, len(fetchedGroup.Channels))
	}

	// 5. GetUserChannelRoles
	roles, err := store.GetUserChannelRoles(ctx, orgID, user.ID, ch1.ID)
	if err != nil || len(roles) != 1 || roles[0] != domain.GroupRoleManager {
		t.Fatalf("expected MANAGER role for ch1, got: %v (err: %v)", roles, err)
	}

	// 6. Delete permission
	err = store.DeleteChannelGroupPermission(ctx, orgID, group.ID, user.ID)
	if err != nil {
		t.Fatalf("delete channel group permission failed: %v", err)
	}
	rolesAfterDel, _ := store.GetUserChannelRoles(ctx, orgID, user.ID, ch1.ID)
	if len(rolesAfterDel) != 0 {
		t.Fatalf("expected no roles after deletion, got: %v", rolesAfterDel)
	}

	// 7. Delete channel group
	err = store.DeleteChannelGroup(ctx, orgID, group.ID)
	if err != nil {
		t.Fatalf("delete channel group failed: %v", err)
	}
	_, err = store.GetChannelGroupByID(ctx, orgID, group.ID)
	if err == nil {
		t.Fatalf("expected error getting deleted group, got nil")
	}
}

func TestInMemoryStore_PostWorkflowAndApprovals(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	channel, _ := store.UpsertChannel(ctx, &domain.Channel{
		Platform:   domain.PlatformFacebook,
		ExternalID: "fb-wf-1",
		Name:       "Workflow Channel",
		IsActive:   true,
	})

	reviewer, _ := store.CreateUser(ctx, &domain.User{
		Email:    "lead@studio.com",
		FullName: "Creative Lead",
	})

	// 1. Create post initially PENDING_APPROVAL
	future := time.Now().UTC().Add(3 * time.Hour)
	post, err := store.CreatePost(ctx, &domain.Post{
		ChannelID:   channel.ID,
		Content:     "Campaign concept v1",
		Status:      domain.PostStatusPendingApproval,
		ScheduledAt: future,
	})
	if err != nil {
		t.Fatalf("create post failed: %v", err)
	}

	// 2. Reject post
	rejectedPost, err := store.ReviewPost(ctx, post.ID, reviewer.ID, domain.ApprovalActionRejected, "Needs better punchline")
	if err != nil {
		t.Fatalf("review post (reject) failed: %v", err)
	}
	if rejectedPost.Status != domain.PostStatusRejected || rejectedPost.RejectionNote != "Needs better punchline" {
		t.Fatalf("unexpected rejected post status: %+v", rejectedPost)
	}

	// Verify post approvals record
	approvals, err := store.ListPostApprovals(ctx, post.ID)
	if err != nil || len(approvals) != 1 {
		t.Fatalf("expected 1 approval log, got %d (err: %v)", len(approvals), err)
	}
	if approvals[0].Action != domain.ApprovalActionRejected || approvals[0].ReviewerName != "Creative Lead" {
		t.Fatalf("unexpected approval entry: %+v", approvals[0])
	}

	// 3. Resubmit post
	resubmittedPost, err := store.ResubmitPost(ctx, post.ID, "Campaign concept v2 (Punchline added)", future.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("resubmit post failed: %v", err)
	}
	if resubmittedPost.Status != domain.PostStatusPendingApproval || resubmittedPost.RejectionNote != "" {
		t.Fatalf("expected post to be PENDING_APPROVAL with cleared note, got: %+v", resubmittedPost)
	}

	// 4. Approve post
	approvedPost, err := store.ReviewPost(ctx, post.ID, reviewer.ID, domain.ApprovalActionApproved, "")
	if err != nil {
		t.Fatalf("review post (approve) failed: %v", err)
	}
	if approvedPost.Status != domain.PostStatusScheduled {
		t.Fatalf("expected post to be SCHEDULED after approval, got %s", approvedPost.Status)
	}

	// Verify approvals list has both records
	allApprovals, _ := store.ListPostApprovals(ctx, post.ID)
	if len(allApprovals) != 2 {
		t.Fatalf("expected 2 approval entries, got %d", len(allApprovals))
	}
}

func TestInMemoryStore_MultiTenantIsolation(t *testing.T) {
	store := NewInMemoryStore()

	ctxOrg1 := WithTenantContext(context.Background(), &domain.TenantContext{
		OrganizationID: "tenant-1",
		UserID:         "user-t1",
		OrgRole:        domain.OrgRoleOwner,
	})
	ctxOrg2 := WithTenantContext(context.Background(), &domain.TenantContext{
		OrganizationID: "tenant-2",
		UserID:         "user-t2",
		OrgRole:        domain.OrgRoleOwner,
	})

	// 1. Channel isolation
	ch1, _ := store.UpsertChannel(ctxOrg1, &domain.Channel{
		Platform:   domain.PlatformFacebook,
		ExternalID: "fb-100",
		Name:       "Tenant 1 Channel",
		IsActive:   true,
	})
	ch2, _ := store.UpsertChannel(ctxOrg2, &domain.Channel{
		Platform:   domain.PlatformFacebook,
		ExternalID: "fb-100", // Same external ID across different tenants is permitted!
		Name:       "Tenant 2 Channel",
		IsActive:   true,
	})

	// List channels in Tenant 1
	channels1, _ := store.ListChannels(ctxOrg1)
	if len(channels1) != 1 || channels1[0].ID != ch1.ID {
		t.Fatalf("tenant 1 channels leaked or wrong: %+v", channels1)
	}

	// List channels in Tenant 2
	channels2, _ := store.ListChannels(ctxOrg2)
	if len(channels2) != 1 || channels2[0].ID != ch2.ID {
		t.Fatalf("tenant 2 channels leaked or wrong: %+v", channels2)
	}

	// Cross-tenant GetChannelByID is blocked
	_, err := store.GetChannelByID(ctxOrg1, ch2.ID)
	if err == nil {
		t.Fatalf("tenant 1 must NOT be able to get tenant 2 channel by ID")
	}

	// 2. Post isolation
	post1, _ := store.CreatePost(ctxOrg1, &domain.Post{
		ChannelID: ch1.ID,
		Content:   "Secret tenant 1 content",
		Status:    domain.PostStatusScheduled,
	})
	post2, _ := store.CreatePost(ctxOrg2, &domain.Post{
		ChannelID: ch2.ID,
		Content:   "Secret tenant 2 content",
		Status:    domain.PostStatusScheduled,
	})

	posts1, _ := store.ListPosts(ctxOrg1, "", 10)
	if len(posts1) != 1 || posts1[0].ID != post1.ID {
		t.Fatalf("tenant 1 posts leaked: %+v", posts1)
	}

	// Cross-tenant GetPostByID is blocked
	_, err = store.GetPostByID(ctxOrg1, post2.ID)
	if err == nil {
		t.Fatalf("tenant 1 must NOT be able to get tenant 2 post by ID")
	}

	// System stats are tenant-isolated
	stats1, _ := store.GetSystemStats(ctxOrg1)
	if stats1.TotalChannels != 1 || stats1.ScheduledPosts != 1 {
		t.Fatalf("unexpected stats for tenant 1: %+v", stats1)
	}
}

func TestInMemoryStore_TenantFairScheduling(t *testing.T) {
	store := NewInMemoryStore()
	now := time.Now().UTC()

	// Org 1 has 5 due posts
	ctxOrg1 := WithTenantContext(context.Background(), &domain.TenantContext{
		OrganizationID: "tenant-A",
		UserID:         "user-a",
	})
	for i := 0; i < 5; i++ {
		_, _ = store.CreatePost(ctxOrg1, &domain.Post{
			ChannelID:   "ch-a",
			Content:     "Post from Org A",
			Status:      domain.PostStatusScheduled,
			ScheduledAt: now.Add(time.Duration(-10+i) * time.Minute),
		})
	}

	// Org 2 has 2 due posts
	ctxOrg2 := WithTenantContext(context.Background(), &domain.TenantContext{
		OrganizationID: "tenant-B",
		UserID:         "user-b",
	})
	for i := 0; i < 2; i++ {
		_, _ = store.CreatePost(ctxOrg2, &domain.Post{
			ChannelID:   "ch-b",
			Content:     "Post from Org B",
			Status:      domain.PostStatusScheduled,
			ScheduledAt: now.Add(time.Duration(-5+i) * time.Minute),
		})
	}

	// Batch size of 4 must interleave fairly across tenants (A, B, A, B, etc.)
	claimed, err := store.ClaimDuePosts(context.Background(), now, 4)
	if err != nil {
		t.Fatalf("claim due posts failed: %v", err)
	}
	if len(claimed) != 4 {
		t.Fatalf("expected 4 claimed posts, got %d", len(claimed))
	}

	countA := 0
	countB := 0
	for _, p := range claimed {
		if p.OrganizationID == "tenant-A" {
			countA++
		} else if p.OrganizationID == "tenant-B" {
			countB++
		}
	}

	if countA != 2 || countB != 2 {
		t.Fatalf("fair scheduling failed: expected 2 from A and 2 from B, got A=%d, B=%d", countA, countB)
	}
}

func TestInMemoryStore_AuditLogs(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	orgID := "org-security-audit"
	details, _ := json.Marshal(map[string]any{"channel_name": "Facebook Main"})

	for i := 1; i <= 5; i++ {
		_ = store.InsertAuditLog(ctx, &domain.AuditLog{
			OrganizationID: orgID,
			Action:         "CHANNEL_CREATED",
			EntityType:     "CHANNEL",
			EntityID:       "ch-audit-1",
			Details:        details,
			IPAddress:      "192.168.1.1",
		})
	}

	// List with limit 3
	logs, err := store.ListAuditLogs(ctx, orgID, 3)
	if err != nil {
		t.Fatalf("list audit logs failed: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 audit logs, got %d", len(logs))
	}
	if logs[0].OrganizationID != orgID || logs[0].Action != "CHANNEL_CREATED" {
		t.Fatalf("unexpected audit log: %+v", logs[0])
	}
}
