package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TruongHoang2004/PostPilot/internal/config"
	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/database"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
	"github.com/TruongHoang2004/PostPilot/internal/infra/ai"
	"github.com/TruongHoang2004/PostPilot/internal/infra/notifier"
	"github.com/TruongHoang2004/PostPilot/internal/infra/redis"
	"github.com/TruongHoang2004/PostPilot/internal/infra/storage"
	"github.com/TruongHoang2004/PostPilot/internal/platform/facebook"
	"github.com/TruongHoang2004/PostPilot/internal/worker"
)

func setupTestServer(t *testing.T) (*Server, http.Handler, database.Store) {
	t.Helper()
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := database.NewInMemoryStore()
	redisClient := redis.NewInMemoryClient()
	cipher, _ := crypto.NewTokenCipher(cfg.TokenEncryptionKey)
	fbClient := facebook.NewClient(cfg.FBGraphBaseURL, cfg.FBGraphVersion)
	s3 := storage.NewS3Service(cfg.S3Endpoint, cfg.S3PublicBaseURL, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey)
	aiGen := ai.NewGeminiGenerator("", cfg.GeminiModel)
	notif := notifier.NewTelegramNotifier("", "", logger)
	proc := worker.NewProcessor(cfg, store, redisClient, cipher, fbClient, notif, logger)

	srv := NewServer(cfg, store, redisClient, cipher, fbClient, s3, aiGen, notif, proc, logger)
	return srv, srv.Handler(), store
}

func TestAuthAndOrgLifecycle(t *testing.T) {
	_, handler, _ := setupTestServer(t)

	// 1. Register a new user
	regPayload := `{"email":"alice@example.com","password":"SuperSecretPassword123","full_name":"Alice Wonderland"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(regPayload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var regResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &regResp); err != nil {
		t.Fatalf("failed to decode register response: %v", err)
	}

	token, ok := regResp["token"].(string)
	if !ok || token == "" {
		t.Fatalf("expected non-empty token from register")
	}

	userMap, _ := regResp["user"].(map[string]any)
	userID, _ := userMap["id"].(string)
	if userID == "" {
		t.Fatalf("expected user id")
	}

	orgMap, _ := regResp["organization"].(map[string]any)
	orgID, _ := orgMap["id"].(string)
	if orgID == "" {
		t.Fatalf("expected organization id")
	}

	// 2. Login
	loginPayload := `{"email":"alice@example.com","password":"SuperSecretPassword123"}`
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(loginPayload))
	recLogin := httptest.NewRecorder()
	handler.ServeHTTP(recLogin, reqLogin)

	if recLogin.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", recLogin.Code, recLogin.Body.String())
	}

	var loginResp map[string]any
	_ = json.Unmarshal(recLogin.Body.Bytes(), &loginResp)
	loginToken := loginResp["token"].(string)
	if loginToken == "" {
		t.Fatalf("expected valid login token")
	}

	// 3. GET /api/v1/auth/me
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+loginToken)
	recMe := httptest.NewRecorder()
	handler.ServeHTTP(recMe, reqMe)

	if recMe.Code != http.StatusOK {
		t.Fatalf("expected /auth/me 200, got %d: %s", recMe.Code, recMe.Body.String())
	}

	// 4. Create second organization
	createOrgPayload := `{"name":"Acme Corp","slug":"acme-corp"}`
	reqNewOrg := httptest.NewRequest(http.MethodPost, "/api/v1/organizations", bytes.NewBufferString(createOrgPayload))
	reqNewOrg.Header.Set("Authorization", "Bearer "+loginToken)
	recNewOrg := httptest.NewRecorder()
	handler.ServeHTTP(recNewOrg, reqNewOrg)

	if recNewOrg.Code != http.StatusCreated {
		t.Fatalf("expected create org 201, got %d: %s", recNewOrg.Code, recNewOrg.Body.String())
	}

	// 5. List organizations
	reqListOrgs := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
	reqListOrgs.Header.Set("Authorization", "Bearer "+loginToken)
	recListOrgs := httptest.NewRecorder()
	handler.ServeHTTP(recListOrgs, reqListOrgs)

	if recListOrgs.Code != http.StatusOK {
		t.Fatalf("expected list orgs 200, got %d", recListOrgs.Code)
	}

	// 6. Invite member to organization
	invitePayload := `{"email":"bob@example.com","role":"MEMBER"}`
	reqInvite := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/organizations/%s/members", orgID), bytes.NewBufferString(invitePayload))
	reqInvite.Header.Set("Authorization", "Bearer "+loginToken)
	recInvite := httptest.NewRecorder()
	handler.ServeHTTP(recInvite, reqInvite)

	if recInvite.Code != http.StatusCreated {
		t.Fatalf("expected invite member 201, got %d: %s", recInvite.Code, recInvite.Body.String())
	}

	var inviteResp map[string]any
	_ = json.Unmarshal(recInvite.Body.Bytes(), &inviteResp)
	invToken, _ := inviteResp["token"].(string)
	if invToken == "" {
		t.Fatalf("expected invitation token")
	}

	// 7. Accept invitation
	acceptPayload := fmt.Sprintf(`{"token":"%s"}`, invToken)
	reqAccept := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/accept", bytes.NewBufferString(acceptPayload))
	reqAccept.Header.Set("Authorization", "Bearer "+loginToken)
	recAccept := httptest.NewRecorder()
	handler.ServeHTTP(recAccept, reqAccept)

	if recAccept.Code != http.StatusOK {
		t.Fatalf("expected accept invitation 200, got %d: %s", recAccept.Code, recAccept.Body.String())
	}

	// 8. List organization members
	reqListMembers := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/organizations/%s/members", orgID), nil)
	reqListMembers.Header.Set("Authorization", "Bearer "+loginToken)
	recListMembers := httptest.NewRecorder()
	handler.ServeHTTP(recListMembers, reqListMembers)

	if recListMembers.Code != http.StatusOK {
		t.Fatalf("expected list members 200, got %d: %s", recListMembers.Code, recListMembers.Body.String())
	}

	// 9. Query audit logs
	reqAudit := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	reqAudit.Header.Set("Authorization", "Bearer "+loginToken)
	recAudit := httptest.NewRecorder()
	handler.ServeHTTP(recAudit, reqAudit)

	if recAudit.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", recAudit.Code)
	}
}

func TestChannelGroupAndRBAC(t *testing.T) {
	_, handler, store := setupTestServer(t)

	// Create test channel
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, err := store.UpsertChannel(ctx, &domain.Channel{
		Platform:             domain.PlatformFacebook,
		ExternalID:           "test_fb_page_100",
		Name:                 "Marketing Global",
		EncryptedAccessToken: "dummy",
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	// 1. Create Channel Group with require_approval = true
	groupPayload := `{"name":"Marketing Campaigns","description":"Social Media Team Group","require_approval":true}`
	reqGroup := httptest.NewRequest(http.MethodPost, "/api/v1/channel-groups", bytes.NewBufferString(groupPayload))
	recGroup := httptest.NewRecorder()
	handler.ServeHTTP(recGroup, reqGroup)

	if recGroup.Code != http.StatusCreated {
		t.Fatalf("expected create channel group 201, got %d: %s", recGroup.Code, recGroup.Body.String())
	}

	var groupResp map[string]any
	_ = json.Unmarshal(recGroup.Body.Bytes(), &groupResp)
	groupID := groupResp["id"].(string)

	// 2. Set Channels for Group
	setChPayload := fmt.Sprintf(`{"channel_ids":["%s"]}`, ch.ID)
	reqSetCh := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/channel-groups/%s/channels", groupID), bytes.NewBufferString(setChPayload))
	recSetCh := httptest.NewRecorder()
	handler.ServeHTTP(recSetCh, reqSetCh)

	if recSetCh.Code != http.StatusOK {
		t.Fatalf("expected set channels 200, got %d: %s", recSetCh.Code, recSetCh.Body.String())
	}

	// 3. List Channel Groups
	reqListGroups := httptest.NewRequest(http.MethodGet, "/api/v1/channel-groups", nil)
	recListGroups := httptest.NewRecorder()
	handler.ServeHTTP(recListGroups, reqListGroups)

	if recListGroups.Code != http.StatusOK {
		t.Fatalf("expected list channel groups 200, got %d: %s", recListGroups.Code, recListGroups.Body.String())
	}

	// 4. Set Group Permission for Member
	permPayload := `{"user_id":"00000000-0000-0000-0000-000000000002","role":"EDITOR"}`
	reqPerm := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/channel-groups/%s/permissions", groupID), bytes.NewBufferString(permPayload))
	recPerm := httptest.NewRecorder()
	handler.ServeHTTP(recPerm, reqPerm)

	if recPerm.Code != http.StatusOK {
		t.Fatalf("expected set group permission 200, got %d: %s", recPerm.Code, recPerm.Body.String())
	}

	// 5. List Permissions
	reqListPerms := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/channel-groups/%s/permissions", groupID), nil)
	recListPerms := httptest.NewRecorder()
	handler.ServeHTTP(recListPerms, reqListPerms)

	if recListPerms.Code != http.StatusOK {
		t.Fatalf("expected list permissions 200, got %d: %s", recListPerms.Code, recListPerms.Body.String())
	}

	// 6. Delete Permission
	reqDelPerm := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/channel-groups/%s/permissions/%s", groupID, "00000000-0000-0000-0000-000000000002"), nil)
	recDelPerm := httptest.NewRecorder()
	handler.ServeHTTP(recDelPerm, reqDelPerm)

	if recDelPerm.Code != http.StatusOK {
		t.Fatalf("expected delete permission 200, got %d: %s", recDelPerm.Code, recDelPerm.Body.String())
	}

	// 7. Delete Channel Group
	reqDelGroup := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/channel-groups/%s", groupID), nil)
	recDelGroup := httptest.NewRecorder()
	handler.ServeHTTP(recDelGroup, reqDelGroup)

	if recDelGroup.Code != http.StatusOK {
		t.Fatalf("expected delete channel group 200, got %d: %s", recDelGroup.Code, recDelGroup.Body.String())
	}
}

func TestPostPublishingApprovalAndWorkflow(t *testing.T) {
	_, handler, store := setupTestServer(t)

	// Create channel and channel group with approval required
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, err := store.UpsertChannel(ctx, &domain.Channel{
		Platform:             domain.PlatformFacebook,
		ExternalID:           "page_approval_test",
		Name:                 "Approval Test Channel",
		EncryptedAccessToken: "token_abc",
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	cg, err := store.CreateChannelGroup(ctx, &domain.ChannelGroup{
		Name:            "Strict Approval Group",
		RequireApproval: true,
	})
	if err != nil {
		t.Fatalf("failed to create channel group: %v", err)
	}

	if err := store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{ch.ID}); err != nil {
		t.Fatalf("failed to map channel to group: %v", err)
	}

	// Case 1: Role = VIEWER -> Rejected with 403 Forbidden
	viewerPostPayload := fmt.Sprintf(`{"channel_id":"%s","content":"Hello Viewer"}`, ch.ID)
	reqViewer := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(viewerPostPayload))
	reqViewer.Header.Set("X-Simulate-Role", "VIEWER")
	recViewer := httptest.NewRecorder()
	handler.ServeHTTP(recViewer, reqViewer)

	if recViewer.Code != http.StatusForbidden {
		t.Fatalf("expected VIEWER post to return 403 Forbidden, got %d: %s", recViewer.Code, recViewer.Body.String())
	}

	// Case 2: Role = EDITOR attempting publish_now on approval-required channel -> 403 Forbidden
	editorDirectPayload := fmt.Sprintf(`{"channel_id":"%s","content":"Hello Editor Direct","publish_now":true}`, ch.ID)
	reqEditorDirect := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(editorDirectPayload))
	reqEditorDirect.Header.Set("X-Simulate-Role", "EDITOR")
	recEditorDirect := httptest.NewRecorder()
	handler.ServeHTTP(recEditorDirect, reqEditorDirect)

	if recEditorDirect.Code != http.StatusForbidden {
		t.Fatalf("expected EDITOR publish_now to return 403, got %d: %s", recEditorDirect.Code, recEditorDirect.Body.String())
	}

	// Case 3: Role = EDITOR authoring post on approval-required channel -> Forced PENDING_APPROVAL
	editorPostPayload := fmt.Sprintf(`{"channel_id":"%s","content":"Editor Draft For Approval"}`, ch.ID)
	reqEditor := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(editorPostPayload))
	reqEditor.Header.Set("X-Simulate-Role", "EDITOR")
	recEditor := httptest.NewRecorder()
	handler.ServeHTTP(recEditor, reqEditor)

	if recEditor.Code != http.StatusCreated {
		t.Fatalf("expected EDITOR post to be created 201, got %d: %s", recEditor.Code, recEditor.Body.String())
	}

	var createdPost domain.Post
	_ = json.Unmarshal(recEditor.Body.Bytes(), &createdPost)
	if createdPost.Status != domain.PostStatusPendingApproval {
		t.Fatalf("expected forced status PENDING_APPROVAL for EDITOR on approval channel, got %s", createdPost.Status)
	}

	// Case 4: EDITOR attempting to review post -> 403 Forbidden
	reviewPayload := `{"action":"APPROVED","note":"Approved by myself"}`
	reqReviewForbidden := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/review", createdPost.ID), bytes.NewBufferString(reviewPayload))
	reqReviewForbidden.Header.Set("X-Simulate-Role", "EDITOR")
	recReviewForbidden := httptest.NewRecorder()
	handler.ServeHTTP(recReviewForbidden, reqReviewForbidden)

	if recReviewForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected review by EDITOR to return 403 Forbidden, got %d: %s", recReviewForbidden.Code, recReviewForbidden.Body.String())
	}

	// Case 5: APPROVER approving the post -> Status becomes SCHEDULED or QUEUED
	reqReviewSuccess := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/review", createdPost.ID), bytes.NewBufferString(reviewPayload))
	reqReviewSuccess.Header.Set("X-Simulate-Role", "APPROVER")
	recReviewSuccess := httptest.NewRecorder()
	handler.ServeHTTP(recReviewSuccess, reqReviewSuccess)

	if recReviewSuccess.Code != http.StatusOK {
		t.Fatalf("expected review by APPROVER to return 200 OK, got %d: %s", recReviewSuccess.Code, recReviewSuccess.Body.String())
	}

	var reviewResp map[string]any
	_ = json.Unmarshal(recReviewSuccess.Body.Bytes(), &reviewResp)
	revPostMap := reviewResp["post"].(map[string]any)
	if revPostMap["status"] != domain.PostStatusScheduled && revPostMap["status"] != domain.PostStatusQueued {
		t.Fatalf("expected approved post status SCHEDULED or QUEUED, got %v", revPostMap["status"])
	}

	// Case 6: Submit for approval
	// Create a rejected post to test submit-approval
	rejectedPost, err := store.CreatePost(ctx, &domain.Post{
		ChannelID: ch.ID,
		Content:   "Draft to resubmit",
		Status:    domain.PostStatusRejected,
	})
	if err != nil {
		t.Fatalf("failed to create rejected post: %v", err)
	}

	resubmitPayload := `{"content":"Updated content for approval"}`
	reqResubmit := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/submit-approval", rejectedPost.ID), bytes.NewBufferString(resubmitPayload))
	recResubmit := httptest.NewRecorder()
	handler.ServeHTTP(recResubmit, reqResubmit)

	if recResubmit.Code != http.StatusOK {
		t.Fatalf("expected submit approval 200 OK, got %d: %s", recResubmit.Code, recResubmit.Body.String())
	}

	var resubmitResp map[string]any
	_ = json.Unmarshal(recResubmit.Body.Bytes(), &resubmitResp)
	resubmitPostMap := resubmitResp["post"].(map[string]any)
	if resubmitPostMap["status"] != domain.PostStatusPendingApproval {
		t.Fatalf("expected status PENDING_APPROVAL after submit-approval, got %v", resubmitPostMap["status"])
	}

	// Case 7: Multi-channel fan-out via channel_group_id
	fanoutPayload := fmt.Sprintf(`{"channel_group_id":"%s","content":"Fanout Announcement"}`, cg.ID)
	reqFanout := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(fanoutPayload))
	recFanout := httptest.NewRecorder()
	handler.ServeHTTP(recFanout, reqFanout)

	if recFanout.Code != http.StatusCreated {
		t.Fatalf("expected fanout post creation 201, got %d: %s", recFanout.Code, recFanout.Body.String())
	}

	var fanoutResp map[string]any
	_ = json.Unmarshal(recFanout.Body.Bytes(), &fanoutResp)
	if fanoutResp["batch_id"] == nil || fanoutResp["batch_id"] == "" {
		t.Fatalf("expected batch_id in fanout response")
	}
}

func TestTenantContextMiddlewareValidation(t *testing.T) {
	_, handler, _ := setupTestServer(t)

	// 1. Invalid JWT returns 401 Unauthorized
	reqInvalid := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	reqInvalid.Header.Set("Authorization", "Bearer invalid.jwt.token")
	recInvalid := httptest.NewRecorder()
	handler.ServeHTTP(recInvalid, reqInvalid)

	if recInvalid.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid JWT, got %d: %s", recInvalid.Code, recInvalid.Body.String())
	}

	// 2. Fallback to DefaultTenantContext when no header is supplied
	reqFallback := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	recFallback := httptest.NewRecorder()
	handler.ServeHTTP(recFallback, reqFallback)

	if recFallback.Code != http.StatusOK {
		t.Fatalf("expected 200 for unauthenticated fallback request, got %d: %s", recFallback.Code, recFallback.Body.String())
	}
}
