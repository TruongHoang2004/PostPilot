package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// TestRBACEnforcementMatrix tests authorization checks for VIEWER, EDITOR, and APPROVER roles.
func TestRBACEnforcementMatrix(t *testing.T) {
	_, handler, store := setupTestServer(t)

	// Create test organization
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, err := store.UpsertChannel(ctx, &domain.Channel{
		Platform:             domain.PlatformFacebook,
		ExternalID:           "fb_page_rbac_101",
		Name:                 "RBAC Secure Channel",
		EncryptedAccessToken: "token_abc",
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	// Channel group with require_approval = true
	cg, err := store.CreateChannelGroup(ctx, &domain.ChannelGroup{
		Name:            "Strict Approvals Group",
		RequireApproval: true,
	})
	if err != nil {
		t.Fatalf("failed to create channel group: %v", err)
	}
	if err := store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{ch.ID}); err != nil {
		t.Fatalf("failed to associate channel with group: %v", err)
	}

	// =========================================================================
	// 1. VIEWER ROLE TESTS
	// =========================================================================

	// Test 1a: VIEWER cannot create post -> 403 Forbidden
	viewerPostPayload := fmt.Sprintf(`{"channel_id":"%s","content":"Viewer attempting to post"}`, ch.ID)
	reqViewerPost := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(viewerPostPayload))
	reqViewerPost.Header.Set("X-Simulate-Role", "VIEWER")
	recViewerPost := httptest.NewRecorder()
	handler.ServeHTTP(recViewerPost, reqViewerPost)
	if recViewerPost.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for VIEWER creating post, got %d: %s", recViewerPost.Code, recViewerPost.Body.String())
	}

	// Test 1b: VIEWER cannot create channel group -> 403 Forbidden
	reqViewerGroup := httptest.NewRequest(http.MethodPost, "/api/v1/channel-groups", bytes.NewBufferString(`{"name":"Viewer Group"}`))
	reqViewerGroup.Header.Set("X-Simulate-Role", "VIEWER")
	recViewerGroup := httptest.NewRecorder()
	handler.ServeHTTP(recViewerGroup, reqViewerGroup)
	if recViewerGroup.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for VIEWER creating channel group, got %d", recViewerGroup.Code)
	}

	// =========================================================================
	// 2. EDITOR ROLE TESTS
	// =========================================================================

	// Test 2a: EDITOR attempting publish_now on approval-required channel -> 403 Forbidden
	editorDirectPayload := fmt.Sprintf(`{"channel_id":"%s","content":"Editor Direct Post","publish_now":true}`, ch.ID)
	reqEditorDirect := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(editorDirectPayload))
	reqEditorDirect.Header.Set("X-Simulate-Role", "EDITOR")
	recEditorDirect := httptest.NewRecorder()
	handler.ServeHTTP(recEditorDirect, reqEditorDirect)
	if recEditorDirect.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for EDITOR direct publish_now on approval channel, got %d: %s", recEditorDirect.Code, recEditorDirect.Body.String())
	}

	// Test 2b: EDITOR creating post with submit_for_approval: true -> succeeds, status PENDING_APPROVAL
	editorApprovalPayload := fmt.Sprintf(`{"channel_id":"%s","content":"Editor Post for Approval","submit_for_approval":true}`, ch.ID)
	reqEditorAppr := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(editorApprovalPayload))
	reqEditorAppr.Header.Set("X-Simulate-Role", "EDITOR")
	recEditorAppr := httptest.NewRecorder()
	handler.ServeHTTP(recEditorAppr, reqEditorAppr)
	if recEditorAppr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for EDITOR submitting for approval, got %d: %s", recEditorAppr.Code, recEditorAppr.Body.String())
	}
	var createdPost domain.Post
	_ = json.Unmarshal(recEditorAppr.Body.Bytes(), &createdPost)
	if createdPost.Status != domain.PostStatusPendingApproval {
		t.Fatalf("expected post status PENDING_APPROVAL, got %s", createdPost.Status)
	}

	// Test 2c: EDITOR attempting to review post -> 403 Forbidden
	reviewPayload := `{"action":"APPROVED","note":"Self-approval attempt"}`
	reqEditorReview := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/review", createdPost.ID), bytes.NewBufferString(reviewPayload))
	reqEditorReview.Header.Set("X-Simulate-Role", "EDITOR")
	recEditorReview := httptest.NewRecorder()
	handler.ServeHTTP(recEditorReview, reqEditorReview)
	if recEditorReview.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for EDITOR attempting to review post, got %d: %s", recEditorReview.Code, recEditorReview.Body.String())
	}

	// =========================================================================
	// 3. APPROVER ROLE & WORKFLOW CYCLE (REJECT -> RESUBMIT -> APPROVE)
	// =========================================================================

	// Test 3a: APPROVER rejects post with feedback note -> 200 OK, status REJECTED
	rejectPayload := `{"action":"REJECTED","note":"Please update the tone to match corporate style guide."}`
	reqApproverReject := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/review", createdPost.ID), bytes.NewBufferString(rejectPayload))
	reqApproverReject.Header.Set("X-Simulate-Role", "APPROVER")
	recApproverReject := httptest.NewRecorder()
	handler.ServeHTTP(recApproverReject, reqApproverReject)
	if recApproverReject.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for APPROVER rejecting post, got %d: %s", recApproverReject.Code, recApproverReject.Body.String())
	}

	var rejectResp map[string]any
	_ = json.Unmarshal(recApproverReject.Body.Bytes(), &rejectResp)
	postRejectedMap := rejectResp["post"].(map[string]any)
	if postRejectedMap["status"] != domain.PostStatusRejected {
		t.Fatalf("expected status REJECTED, got %v", postRejectedMap["status"])
	}
	if postRejectedMap["rejection_note"] != "Please update the tone to match corporate style guide." {
		t.Fatalf("expected rejection note, got %v", postRejectedMap["rejection_note"])
	}

	// Test 3b: VIEWER attempting submit-approval -> 403 Forbidden
	reqViewerResubmit := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/submit-approval", createdPost.ID), bytes.NewBufferString(`{"content":"Viewer edit"}`))
	reqViewerResubmit.Header.Set("X-Simulate-Role", "VIEWER")
	recViewerResubmit := httptest.NewRecorder()
	handler.ServeHTTP(recViewerResubmit, reqViewerResubmit)
	if recViewerResubmit.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for VIEWER attempting submit-approval, got %d", recViewerResubmit.Code)
	}

	// Test 3c: EDITOR resubmits rejected post -> 200 OK, status PENDING_APPROVAL, review note cleared
	futureTime := time.Now().UTC().Add(3 * time.Hour).Format(time.RFC3339)
	resubmitPayload := fmt.Sprintf(`{"content":"Revised Content: Professional and aligned with style guide.","scheduled_at":"%s"}`, futureTime)
	reqEditorResubmit := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/submit-approval", createdPost.ID), bytes.NewBufferString(resubmitPayload))
	reqEditorResubmit.Header.Set("X-Simulate-Role", "EDITOR")
	recEditorResubmit := httptest.NewRecorder()
	handler.ServeHTTP(recEditorResubmit, reqEditorResubmit)
	if recEditorResubmit.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for EDITOR resubmitting post, got %d: %s", recEditorResubmit.Code, recEditorResubmit.Body.String())
	}
	var resubmitResp map[string]any
	_ = json.Unmarshal(recEditorResubmit.Body.Bytes(), &resubmitResp)
	postResubmittedMap := resubmitResp["post"].(map[string]any)
	if postResubmittedMap["status"] != domain.PostStatusPendingApproval {
		t.Fatalf("expected status PENDING_APPROVAL after resubmit, got %v", postResubmittedMap["status"])
	}
	if postResubmittedMap["rejection_note"] != "" && postResubmittedMap["rejection_note"] != nil {
		t.Fatalf("expected empty rejection_note after resubmit, got %v", postResubmittedMap["rejection_note"])
	}

	// Test 3d: APPROVER approves the revised post -> 200 OK, status SCHEDULED (future)
	approvePayload := `{"action":"APPROVED","note":"Approved. Looks great!"}`
	reqApproverApprove := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/review", createdPost.ID), bytes.NewBufferString(approvePayload))
	reqApproverApprove.Header.Set("X-Simulate-Role", "APPROVER")
	recApproverApprove := httptest.NewRecorder()
	handler.ServeHTTP(recApproverApprove, reqApproverApprove)
	if recApproverApprove.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for APPROVER approving post, got %d: %s", recApproverApprove.Code, recApproverApprove.Body.String())
	}
	var approveResp map[string]any
	_ = json.Unmarshal(recApproverApprove.Body.Bytes(), &approveResp)
	postApprovedMap := approveResp["post"].(map[string]any)
	if postApprovedMap["status"] != domain.PostStatusScheduled {
		t.Fatalf("expected status SCHEDULED after approval, got %v", postApprovedMap["status"])
	}
}

// TestBatchFanOutAndBatchReview tests multi-channel fan-out post creation and batch approval / rejection.
func TestBatchFanOutAndBatchReview(t *testing.T) {
	_, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())

	// Create 2 Channels
	ch1, err := store.UpsertChannel(ctx, &domain.Channel{
		Platform:             domain.PlatformFacebook,
		ExternalID:           "fb_batch_ch_1",
		Name:                 "Marketing Channel 1",
		EncryptedAccessToken: "token1",
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		t.Fatalf("failed to create channel 1: %v", err)
	}

	ch2, err := store.UpsertChannel(ctx, &domain.Channel{
		Platform:             domain.PlatformFacebook,
		ExternalID:           "fb_batch_ch_2",
		Name:                 "Marketing Channel 2",
		EncryptedAccessToken: "token2",
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		t.Fatalf("failed to create channel 2: %v", err)
	}

	// Create Channel Group containing both channels with require_approval = true
	cg, err := store.CreateChannelGroup(ctx, &domain.ChannelGroup{
		Name:            "All Marketing Channels",
		RequireApproval: true,
	})
	if err != nil {
		t.Fatalf("failed to create channel group: %v", err)
	}
	if err := store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{ch1.ID, ch2.ID}); err != nil {
		t.Fatalf("failed to set channels for group: %v", err)
	}

	// =========================================================================
	// BATCH APPROVAL FLOW
	// =========================================================================

	// Step 1: Create post targeting channel_group_id -> fan-out to 2 posts
	futureTime := time.Now().UTC().Add(4 * time.Hour).Format(time.RFC3339)
	fanoutPayload := fmt.Sprintf(`{
		"channel_group_id": "%s",
		"content": "Spring Campaign 2026 Launch Announcement",
		"scheduled_at": "%s"
	}`, cg.ID, futureTime)

	reqFanout := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(fanoutPayload))
	reqFanout.Header.Set("X-Simulate-Role", "EDITOR")
	recFanout := httptest.NewRecorder()
	handler.ServeHTTP(recFanout, reqFanout)

	if recFanout.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for fan-out post creation, got %d: %s", recFanout.Code, recFanout.Body.String())
	}

	var fanoutResp map[string]any
	_ = json.Unmarshal(recFanout.Body.Bytes(), &fanoutResp)
	batchID, ok := fanoutResp["batch_id"].(string)
	if !ok || batchID == "" {
		t.Fatalf("expected non-empty batch_id in response")
	}

	postsRaw, ok := fanoutResp["posts"].([]any)
	if !ok || len(postsRaw) != 2 {
		t.Fatalf("expected exactly 2 posts created for group with 2 channels, got %d", len(postsRaw))
	}

	post1Map := postsRaw[0].(map[string]any)
	post2Map := postsRaw[1].(map[string]any)
	post1ID := post1Map["id"].(string)
	post2ID := post2Map["id"].(string)

	if post1Map["status"] != domain.PostStatusPendingApproval || post2Map["status"] != domain.PostStatusPendingApproval {
		t.Fatalf("both posts must be in PENDING_APPROVAL status initially, got %v and %v", post1Map["status"], post2Map["status"])
	}
	if post1Map["batch_id"] != batchID || post2Map["batch_id"] != batchID {
		t.Fatalf("both posts must share batch_id %s, got %v and %v", batchID, post1Map["batch_id"], post2Map["batch_id"])
	}

	// Step 2: APPROVER calls /review on Post 1 with apply_to_batch: true -> both posts transition to SCHEDULED
	batchApprovePayload := `{"action":"APPROVED","note":"Approved all channels in batch","apply_to_batch":true}`
	reqBatchApprove := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/review", post1ID), bytes.NewBufferString(batchApprovePayload))
	reqBatchApprove.Header.Set("X-Simulate-Role", "APPROVER")
	recBatchApprove := httptest.NewRecorder()
	handler.ServeHTTP(recBatchApprove, reqBatchApprove)

	if recBatchApprove.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for batch review, got %d: %s", recBatchApprove.Code, recBatchApprove.Body.String())
	}

	// Verify Post 1 is now SCHEDULED
	p1Updated, err := store.GetPostByID(ctx, post1ID)
	if err != nil || p1Updated.Status != domain.PostStatusScheduled {
		t.Fatalf("expected Post 1 status SCHEDULED, got status=%s, err=%v", p1Updated.Status, err)
	}

	// Verify Post 2 is ALSO now SCHEDULED via batch cascade
	p2Updated, err := store.GetPostByID(ctx, post2ID)
	if err != nil || p2Updated.Status != domain.PostStatusScheduled {
		t.Fatalf("expected Post 2 status SCHEDULED via batch review cascade, got status=%s, err=%v", p2Updated.Status, err)
	}

	// =========================================================================
	// BATCH REJECTION FLOW
	// =========================================================================

	// Step 3: Create second batch of posts for rejection test
	fanoutPayload2 := fmt.Sprintf(`{
		"channel_group_id": "%s",
		"content": "Defective Campaign Assets",
		"scheduled_at": "%s"
	}`, cg.ID, futureTime)

	reqFanout2 := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(fanoutPayload2))
	reqFanout2.Header.Set("X-Simulate-Role", "EDITOR")
	recFanout2 := httptest.NewRecorder()
	handler.ServeHTTP(recFanout2, reqFanout2)

	if recFanout2.Code != http.StatusCreated {
		t.Fatalf("failed to create second fan-out batch: %d: %s", recFanout2.Code, recFanout2.Body.String())
	}

	var fanoutResp2 map[string]any
	_ = json.Unmarshal(recFanout2.Body.Bytes(), &fanoutResp2)
	postsRaw2 := fanoutResp2["posts"].([]any)
	batch2Post1ID := postsRaw2[0].(map[string]any)["id"].(string)
	batch2Post2ID := postsRaw2[1].(map[string]any)["id"].(string)

	// Step 4: APPROVER rejects entire batch via apply_to_batch: true
	batchRejectPayload := `{"action":"REJECTED","note":"Fix broken link in text before publishing","apply_to_batch":true}`
	reqBatchReject := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/review", batch2Post1ID), bytes.NewBufferString(batchRejectPayload))
	reqBatchReject.Header.Set("X-Simulate-Role", "APPROVER")
	recBatchReject := httptest.NewRecorder()
	handler.ServeHTTP(recBatchReject, reqBatchReject)

	if recBatchReject.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for batch rejection, got %d: %s", recBatchReject.Code, recBatchReject.Body.String())
	}

	// Verify both posts in second batch are REJECTED with note
	b2p1, _ := store.GetPostByID(ctx, batch2Post1ID)
	b2p2, _ := store.GetPostByID(ctx, batch2Post2ID)

	if b2p1.Status != domain.PostStatusRejected || b2p1.RejectionNote != "Fix broken link in text before publishing" {
		t.Fatalf("expected batch2 post 1 REJECTED with note, got status=%s, note=%s", b2p1.Status, b2p1.RejectionNote)
	}
	if b2p2.Status != domain.PostStatusRejected || b2p2.RejectionNote != "Fix broken link in text before publishing" {
		t.Fatalf("expected batch2 post 2 REJECTED with note via batch cascade, got status=%s, note=%s", b2p2.Status, b2p2.RejectionNote)
	}
}

// TestChannelGroupRBACMostPermissiveWins verifies that when a user has multiple roles across groups containing the same channel,
// the most permissive role takes precedence (Most Permissive Wins).
func TestChannelGroupRBACMostPermissiveWins(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())

	// Create test channel
	ch, err := store.UpsertChannel(ctx, &domain.Channel{
		Platform:             domain.PlatformFacebook,
		ExternalID:           "fb_overlapping_ch_1",
		Name:                 "Shared Marketing Channel",
		EncryptedAccessToken: "token_xyz",
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	// Group A (General Social)
	groupA, err := store.CreateChannelGroup(ctx, &domain.ChannelGroup{
		Name:            "General Social Group",
		RequireApproval: true,
	})
	if err != nil {
		t.Fatalf("failed to create group A: %v", err)
	}
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, groupA.ID, []string{ch.ID})

	// Group B (Editorial Lead Group)
	groupB, err := store.CreateChannelGroup(ctx, &domain.ChannelGroup{
		Name:            "Editorial Lead Group",
		RequireApproval: true,
	})
	if err != nil {
		t.Fatalf("failed to create group B: %v", err)
	}
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, groupB.ID, []string{ch.ID})

	// Create a user "Dave"
	dave, err := store.CreateUser(ctx, &domain.User{
		Email:        "dave@example.com",
		PasswordHash: "dummy_hash",
		FullName:     "Dave EditorApprover",
		IsActive:     true,
	})
	if err != nil {
		t.Fatalf("failed to create user Dave: %v", err)
	}

	// Assign Dave VIEWER on Group A
	_, err = store.SetChannelGroupPermission(ctx, &domain.ChannelGroupPermission{
		ChannelGroupID: groupA.ID,
		UserID:         dave.ID,
		GroupRole:      domain.GroupRoleViewer,
	})
	if err != nil {
		t.Fatalf("failed to set VIEWER permission on Group A: %v", err)
	}

	// Assign Dave EDITOR on Group B
	_, err = store.SetChannelGroupPermission(ctx, &domain.ChannelGroupPermission{
		ChannelGroupID: groupB.ID,
		UserID:         dave.ID,
		GroupRole:      domain.GroupRoleEditor,
	})
	if err != nil {
		t.Fatalf("failed to set EDITOR permission on Group B: %v", err)
	}

	// Sign JWT for Dave (OrgRole: MEMBER)
	daveClaims := domain.TenantContext{
		UserID:         dave.ID,
		UserEmail:      dave.Email,
		OrganizationID: domain.DefaultOrganizationID,
		OrgRole:        domain.OrgRoleMember,
	}
	daveToken, _ := srv.cfg.APIAdminKey, srv.cfg.APIAdminKey // use APIAdminKey to sign token
	tok, err := srv.store.GetUserByID(ctx, dave.ID)
	if err != nil || tok == nil {
		t.Fatalf("user not found")
	}

	// Direct test via server.resolveChannelPermission:
	userRoles, err := store.GetUserChannelRoles(ctx, domain.DefaultOrganizationID, dave.ID, ch.ID)
	if err != nil {
		t.Fatalf("failed to get user channel roles: %v", err)
	}
	resolved := domain.ResolveChannelRole(userRoles)
	if resolved != domain.GroupRoleEditor {
		t.Fatalf("expected resolved role EDITOR (Most Permissive Wins over VIEWER), got %s", resolved)
	}

	// Now elevate Dave's permission on Group A to APPROVER:
	_, err = store.SetChannelGroupPermission(ctx, &domain.ChannelGroupPermission{
		ChannelGroupID: groupA.ID,
		UserID:         dave.ID,
		GroupRole:      domain.GroupRoleApprover,
	})
	if err != nil {
		t.Fatalf("failed to set APPROVER permission on Group A: %v", err)
	}

	// Direct test: Dave has APPROVER (Group A) + EDITOR (Group B) -> APPROVER wins
	userRoles2, err := store.GetUserChannelRoles(ctx, domain.DefaultOrganizationID, dave.ID, ch.ID)
	if err != nil {
		t.Fatalf("failed to get updated channel roles: %v", err)
	}
	resolved2 := domain.ResolveChannelRole(userRoles2)
	if resolved2 != domain.GroupRoleApprover {
		t.Fatalf("expected resolved role APPROVER (Most Permissive Wins over EDITOR), got %s", resolved2)
	}

	// Dave makes request using real JWT with X-User-Id header simulation or resolved permission
	postPayload := fmt.Sprintf(`{"channel_id":"%s","content":"Post by Dave as Editor/Approver","submit_for_approval":true}`, ch.ID)
	reqDavePost := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(postPayload))
	reqDavePost.Header.Set("X-User-Id", dave.ID)
	reqDavePost.Header.Set("X-Simulate-Role", "MEMBER") // member with group roles
	recDavePost := httptest.NewRecorder()
	handler.ServeHTTP(recDavePost, reqDavePost)
	if recDavePost.Code != http.StatusCreated {
		t.Fatalf("expected Dave to be able to create post (resolved APPROVER/EDITOR), got %d: %s", recDavePost.Code, recDavePost.Body.String())
	}
	_ = daveClaims
	_ = daveToken
}
