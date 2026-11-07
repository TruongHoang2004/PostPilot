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
	"github.com/TruongHoang2004/PostPilot/internal/worker"
)

func setupTestServer(t *testing.T) (*Server, http.Handler, database.Store) {
	t.Helper()
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := database.NewInMemoryStore()
	redisClient := redis.NewInMemoryClient()
	cipher, _ := crypto.NewTokenCipher(cfg.TokenEncryptionKey)
	fbClient := testFacebookClient(cfg.FBGraphBaseURL, cfg.FBGraphVersion)
	s3 := storage.NewS3Service(cfg.S3Endpoint, cfg.S3PublicBaseURL, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey)
	aiGen := ai.NewGeminiGenerator("", cfg.GeminiModel)
	notif := notifier.NewTelegramNotifier("", "", logger)
	proc := worker.NewProcessor(cfg, store, redisClient, cipher, fbClient, notif, logger)

	srv := NewServer(cfg, store, redisClient, cipher, fbClient, s3, aiGen, notif, proc, logger)
	return srv, srv.Handler(), store
}

func TestAuthAndOrgLifecycle(t *testing.T) {
	srv, handler, _ := setupTestServer(t)

	// 1. Register a new user
	regPayload := `{"email":"alice@example.com","password":"SuperSecretPassword123","full_name":"Alice Wonderland"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(regPayload))
	rec := httptest.NewRecorder()
	serveTest(t, srv, handler, rec, req)

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
	serveTest(t, srv, handler, recLogin, reqLogin)

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
	serveTest(t, srv, handler, recMe, reqMe)

	if recMe.Code != http.StatusOK {
		t.Fatalf("expected /auth/me 200, got %d: %s", recMe.Code, recMe.Body.String())
	}

	// 4. Create second organization
	createOrgPayload := `{"name":"Acme Corp","slug":"acme-corp"}`
	reqNewOrg := httptest.NewRequest(http.MethodPost, "/api/v1/organizations", bytes.NewBufferString(createOrgPayload))
	reqNewOrg.Header.Set("Authorization", "Bearer "+loginToken)
	recNewOrg := httptest.NewRecorder()
	serveTest(t, srv, handler, recNewOrg, reqNewOrg)

	if recNewOrg.Code != http.StatusCreated {
		t.Fatalf("expected create org 201, got %d: %s", recNewOrg.Code, recNewOrg.Body.String())
	}

	// 5. List organizations
	reqListOrgs := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
	reqListOrgs.Header.Set("Authorization", "Bearer "+loginToken)
	recListOrgs := httptest.NewRecorder()
	serveTest(t, srv, handler, recListOrgs, reqListOrgs)

	if recListOrgs.Code != http.StatusOK {
		t.Fatalf("expected list orgs 200, got %d", recListOrgs.Code)
	}

	// 6. Invite member to organization
	invitePayload := `{"email":"bob@example.com","role":"MEMBER"}`
	reqInvite := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/organizations/%s/members", orgID), bytes.NewBufferString(invitePayload))
	reqInvite.Header.Set("Authorization", "Bearer "+loginToken)
	recInvite := httptest.NewRecorder()
	serveTest(t, srv, handler, recInvite, reqInvite)

	if recInvite.Code != http.StatusCreated {
		t.Fatalf("expected invite member 201, got %d: %s", recInvite.Code, recInvite.Body.String())
	}

	var inviteResp map[string]any
	_ = json.Unmarshal(recInvite.Body.Bytes(), &inviteResp)
	invToken, _ := inviteResp["token"].(string)
	if invToken == "" {
		t.Fatalf("expected invitation token")
	}

	// Recipient registers with the invited email and accepts using their own JWT.
	regBob := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"bob@example.com","password":"LongPasswordBob123","full_name":"Bob"}`))
	recBob := httptest.NewRecorder()
	handler.ServeHTTP(recBob, regBob)
	if recBob.Code != 201 {
		t.Fatalf("register Bob: %d %s", recBob.Code, recBob.Body.String())
	}
	var bobResponse map[string]any
	_ = json.Unmarshal(recBob.Body.Bytes(), &bobResponse)
	bobToken := bobResponse["token"].(string)

	// 7. Accept invitation
	acceptPayload := fmt.Sprintf(`{"token":"%s"}`, invToken)
	reqAccept := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/accept", bytes.NewBufferString(acceptPayload))
	reqAccept.Header.Set("Authorization", "Bearer "+bobToken)
	recAccept := httptest.NewRecorder()
	serveTest(t, srv, handler, recAccept, reqAccept)

	if recAccept.Code != http.StatusOK {
		t.Fatalf("expected accept invitation 200, got %d: %s", recAccept.Code, recAccept.Body.String())
	}

	// 8. List organization members
	reqListMembers := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/organizations/%s/members", orgID), nil)
	reqListMembers.Header.Set("Authorization", "Bearer "+loginToken)
	recListMembers := httptest.NewRecorder()
	serveTest(t, srv, handler, recListMembers, reqListMembers)

	if recListMembers.Code != http.StatusOK {
		t.Fatalf("expected list members 200, got %d: %s", recListMembers.Code, recListMembers.Body.String())
	}

	// 9. Query audit logs
	reqAudit := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	reqAudit.Header.Set("Authorization", "Bearer "+loginToken)
	recAudit := httptest.NewRecorder()
	serveTest(t, srv, handler, recAudit, reqAudit)

	if recAudit.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", recAudit.Code)
	}
}

func TestChannelGroupAndRBAC(t *testing.T) {
	srv, handler, store := setupTestServer(t)

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
	serveTest(t, srv, handler, recGroup, reqGroup)

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
	serveTest(t, srv, handler, recSetCh, reqSetCh)

	if recSetCh.Code != http.StatusOK {
		t.Fatalf("expected set channels 200, got %d: %s", recSetCh.Code, recSetCh.Body.String())
	}

	// 3. List Channel Groups
	reqListGroups := httptest.NewRequest(http.MethodGet, "/api/v1/channel-groups", nil)
	recListGroups := httptest.NewRecorder()
	serveTest(t, srv, handler, recListGroups, reqListGroups)

	if recListGroups.Code != http.StatusOK {
		t.Fatalf("expected list channel groups 200, got %d: %s", recListGroups.Code, recListGroups.Body.String())
	}

	// 4. Set Group Permission for an actual member.
	memberFixture, _ := store.CreateUser(ctx, &domain.User{ID: "00000000-0000-0000-0000-000000000002", Email: "member@postpilot.test", IsActive: true})
	_, _ = store.AddOrganizationMember(ctx, &domain.OrganizationMember{OrganizationID: domain.DefaultOrganizationID, UserID: memberFixture.ID, OrgRole: domain.OrgRoleMember})
	permPayload := `{"user_id":"00000000-0000-0000-0000-000000000002","role":"EDITOR"}`
	reqPerm := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/channel-groups/%s/permissions", groupID), bytes.NewBufferString(permPayload))
	recPerm := httptest.NewRecorder()
	serveTest(t, srv, handler, recPerm, reqPerm)

	if recPerm.Code != http.StatusOK {
		t.Fatalf("expected set group permission 200, got %d: %s", recPerm.Code, recPerm.Body.String())
	}

	// 5. List Permissions
	reqListPerms := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/channel-groups/%s/permissions", groupID), nil)
	recListPerms := httptest.NewRecorder()
	serveTest(t, srv, handler, recListPerms, reqListPerms)

	if recListPerms.Code != http.StatusOK {
		t.Fatalf("expected list permissions 200, got %d: %s", recListPerms.Code, recListPerms.Body.String())
	}

	// 6. Delete Permission
	reqDelPerm := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/channel-groups/%s/permissions/%s", groupID, "00000000-0000-0000-0000-000000000002"), nil)
	recDelPerm := httptest.NewRecorder()
	serveTest(t, srv, handler, recDelPerm, reqDelPerm)

	if recDelPerm.Code != http.StatusOK {
		t.Fatalf("expected delete permission 200, got %d: %s", recDelPerm.Code, recDelPerm.Body.String())
	}

	// 7. Delete Channel Group
	reqDelGroup := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/channel-groups/%s", groupID), nil)
	recDelGroup := httptest.NewRecorder()
	serveTest(t, srv, handler, recDelGroup, reqDelGroup)

	if recDelGroup.Code != http.StatusOK {
		t.Fatalf("expected delete channel group 200, got %d: %s", recDelGroup.Code, recDelGroup.Body.String())
	}
}

func TestPostPublishingApprovalAndWorkflow(t *testing.T) {
	srv, handler, store := setupTestServer(t)

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
	authenticateTestRole(t, srv, reqViewer, "VIEWER", "")
	recViewer := httptest.NewRecorder()
	serveTest(t, srv, handler, recViewer, reqViewer)

	if recViewer.Code != http.StatusForbidden {
		t.Fatalf("expected VIEWER post to return 403 Forbidden, got %d: %s", recViewer.Code, recViewer.Body.String())
	}

	// Case 2: Role = EDITOR attempting publish_now on approval-required channel -> 403 Forbidden
	editorDirectPayload := fmt.Sprintf(`{"channel_id":"%s","content":"Hello Editor Direct","publish_now":true}`, ch.ID)
	reqEditorDirect := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(editorDirectPayload))
	authenticateTestRole(t, srv, reqEditorDirect, "EDITOR", "")
	recEditorDirect := httptest.NewRecorder()
	serveTest(t, srv, handler, recEditorDirect, reqEditorDirect)

	if recEditorDirect.Code != http.StatusForbidden {
		t.Fatalf("expected EDITOR publish_now to return 403, got %d: %s", recEditorDirect.Code, recEditorDirect.Body.String())
	}

	// Case 3: Role = EDITOR authoring post on approval-required channel -> Forced PENDING_APPROVAL
	editorPostPayload := fmt.Sprintf(`{"channel_id":"%s","content":"Editor Draft For Approval"}`, ch.ID)
	reqEditor := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(editorPostPayload))
	authenticateTestRole(t, srv, reqEditor, "EDITOR", "")
	recEditor := httptest.NewRecorder()
	serveTest(t, srv, handler, recEditor, reqEditor)

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
	authenticateTestRole(t, srv, reqReviewForbidden, "EDITOR", "")
	recReviewForbidden := httptest.NewRecorder()
	serveTest(t, srv, handler, recReviewForbidden, reqReviewForbidden)

	if recReviewForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected review by EDITOR to return 403 Forbidden, got %d: %s", recReviewForbidden.Code, recReviewForbidden.Body.String())
	}

	// Case 5: APPROVER approving the post -> Status becomes SCHEDULED or QUEUED
	reqReviewSuccess := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/posts/%s/review", createdPost.ID), bytes.NewBufferString(reviewPayload))
	authenticateTestRole(t, srv, reqReviewSuccess, "APPROVER", "")
	recReviewSuccess := httptest.NewRecorder()
	serveTest(t, srv, handler, recReviewSuccess, reqReviewSuccess)

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
	serveTest(t, srv, handler, recResubmit, reqResubmit)

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
	serveTest(t, srv, handler, recFanout, reqFanout)

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

	if recFallback.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated request, got %d: %s", recFallback.Code, recFallback.Body.String())
	}
}
