package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/config"
	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/database"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
	"github.com/TruongHoang2004/PostPilot/internal/platform/facebook"
	"github.com/TruongHoang2004/PostPilot/internal/infra/ai"
	"github.com/TruongHoang2004/PostPilot/internal/infra/notifier"
	"github.com/TruongHoang2004/PostPilot/internal/infra/redis"
	"github.com/TruongHoang2004/PostPilot/internal/infra/storage"
	"github.com/TruongHoang2004/PostPilot/internal/worker"
)

func TestAPIEndpointsFlow(t *testing.T) {
	t.Cleanup(func() {
		_ = os.RemoveAll("./uploads")
	})

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
	handler := srv.Handler()

	// 1. Test GET /healthz
	req1 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected /healthz 200, got %d", rec1.Code)
	}

	// 2. Test POST /api/v1/channels
	channelBody := `{"platform":"FACEBOOK","external_id":"100200300","name":"Darwin Official","category":"Tech","access_token":"EAABsbCS1iHgBO1234567890"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/channels", strings.NewReader(channelBody))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected POST /api/v1/channels 201, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var createdChannel map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &createdChannel)
	channelID, _ := createdChannel["id"].(string)
	if channelID == "" {
		t.Fatalf("expected non-empty channel ID")
	}

	// 3. Test POST /api/v1/posts
	postPayload, _ := json.Marshal(map[string]any{
		"channel_id":  channelID,
		"content":     "Hello Multi-Platform from PostPilot DDD!",
		"media_type":  "IMAGE",
		"media_urls":  []string{"https://cdn.example.com/banner.png"},
		"publish_now": true,
	})
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewReader(postPayload))
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusCreated {
		t.Fatalf("expected POST /api/v1/posts 201, got %d: %s", rec3.Code, rec3.Body.String())
	}
	var createdPostResp map[string]any
	_ = json.Unmarshal(rec3.Body.Bytes(), &createdPostResp)
	createdPostID, _ := createdPostResp["id"].(string)

	// 3a. Test GET /api/v1/posts and GET /api/v1/posts/{id}
	reqListPosts := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	recListPosts := httptest.NewRecorder()
	handler.ServeHTTP(recListPosts, reqListPosts)
	if recListPosts.Code != http.StatusOK {
		t.Fatalf("expected GET /api/v1/posts 200, got %d", recListPosts.Code)
	}

	reqGetPost := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+createdPostID, nil)
	recGetPost := httptest.NewRecorder()
	handler.ServeHTTP(recGetPost, reqGetPost)
	if recGetPost.Code != http.StatusOK {
		t.Fatalf("expected GET /api/v1/posts/%s 200, got %d", createdPostID, recGetPost.Code)
	}

	// 3b. Test POST /api/v1/posts/{id}/cancel
	reqCancel := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+createdPostID+"/cancel", nil)
	recCancel := httptest.NewRecorder()
	handler.ServeHTTP(recCancel, reqCancel)
	if recCancel.Code != http.StatusOK {
		t.Fatalf("expected POST /api/v1/posts/cancel 200, got %d", recCancel.Code)
	}

	// 3c. Test GET /api/v1/channels and POST /api/v1/channels/{id}/verify-token
	reqChannels := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	recChannels := httptest.NewRecorder()
	handler.ServeHTTP(recChannels, reqChannels)
	if recChannels.Code != http.StatusOK {
		t.Fatalf("expected GET /api/v1/channels 200, got %d", recChannels.Code)
	}

	reqVerify := httptest.NewRequest(http.MethodPost, "/api/v1/channels/"+channelID+"/verify-token", nil)
	recVerify := httptest.NewRecorder()
	handler.ServeHTTP(recVerify, reqVerify)
	if recVerify.Code != http.StatusOK {
		t.Fatalf("expected POST /api/v1/channels/verify-token 200, got %d", recVerify.Code)
	}

	// 3d. Test DELETE /api/v1/channels/{id}
	reqDelChan := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/"+channelID, nil)
	recDelChan := httptest.NewRecorder()
	handler.ServeHTTP(recDelChan, reqDelChan)
	if recDelChan.Code != http.StatusOK {
		t.Fatalf("expected DELETE /api/v1/channels 200, got %d", recDelChan.Code)
	}

	// 4. Test POST /api/v1/ai/generate (fallback template)
	req4 := httptest.NewRequest(http.MethodPost, "/api/v1/ai/generate", strings.NewReader(`{"topic":"AI Digital Transformation"}`))
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("expected POST /api/v1/ai/generate 200, got %d", rec4.Code)
	}

	// 5. Test POST /api/v1/media/presign (AWS SigV4)
	req5 := httptest.NewRequest(http.MethodPost, "/api/v1/media/presign", strings.NewReader(`{"file_name":"video.mp4","mime_type":"video/mp4"}`))
	rec5 := httptest.NewRecorder()
	handler.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusOK {
		t.Fatalf("expected POST /api/v1/media/presign 200, got %d", rec5.Code)
	}

	// 6. Test POST /api/v1/media/upload (Direct Video Multipart Upload)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "demo_reel.mp4")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	_, _ = part.Write([]byte("fake video content binary data"))
	_ = writer.Close()

	req6 := httptest.NewRequest(http.MethodPost, "/api/v1/media/upload", &body)
	req6.Header.Set("Content-Type", writer.FormDataContentType())
	rec6 := httptest.NewRecorder()
	handler.ServeHTTP(rec6, req6)
	if rec6.Code != http.StatusOK {
		t.Fatalf("expected POST /api/v1/media/upload 200, got %d: %s", rec6.Code, rec6.Body.String())
	}

	var uploadResp map[string]any
	if err := json.Unmarshal(rec6.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("unmarshal upload resp failed: %v", err)
	}
	if uploadResp["public_url"] == "" {
		t.Fatalf("expected non-empty public_url from direct media upload")
	}

	// 7. Test GET / (Dashboard UI returns HTML)
	req7 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec7 := httptest.NewRecorder()
	handler.ServeHTTP(rec7, req7)
	if rec7.Code != http.StatusOK {
		t.Fatalf("expected GET / 200, got %d", rec7.Code)
	}
	if !strings.Contains(rec7.Body.String(), "Tải Video hoặc Ảnh trực tiếp") {
		t.Fatalf("expected dashboard HTML to contain video upload section")
	}
}

func TestServer_CrossTenantAndRBACSecurity(t *testing.T) {
	_, handler, store := setupTestServer(t)

	// -------------------------------------------------------------------------
	// 1. Cross-Tenant Isolation: Org A vs Org B
	// -------------------------------------------------------------------------
	regA := `{"email":"tenant_a@pilot.com","password":"SuperSecretPasswordA1","full_name":"Tenant A Org"}`
	recA := httptest.NewRecorder()
	handler.ServeHTTP(recA, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(regA)))
	if recA.Code != http.StatusCreated {
		t.Fatalf("register A failed: %d: %s", recA.Code, recA.Body.String())
	}
	var respA map[string]any
	_ = json.Unmarshal(recA.Body.Bytes(), &respA)
	tokenA := respA["token"].(string)

	regB := `{"email":"tenant_b@pilot.com","password":"SuperSecretPasswordB1","full_name":"Tenant B Org"}`
	recB := httptest.NewRecorder()
	handler.ServeHTTP(recB, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(regB)))
	if recB.Code != http.StatusCreated {
		t.Fatalf("register B failed: %d: %s", recB.Code, recB.Body.String())
	}
	var respB map[string]any
	_ = json.Unmarshal(recB.Body.Bytes(), &respB)
	tokenB := respB["token"].(string)

	// Org A creates a Channel
	reqChA := httptest.NewRequest(http.MethodPost, "/api/v1/channels", strings.NewReader(`{"platform":"FACEBOOK","external_id":"fb-tenant-a-1","name":"Org A Channel","access_token":"tokenA"}`))
	reqChA.Header.Set("Authorization", "Bearer "+tokenA)
	recChA := httptest.NewRecorder()
	handler.ServeHTTP(recChA, reqChA)
	if recChA.Code != http.StatusCreated {
		t.Fatalf("create channel A failed: %d: %s", recChA.Code, recChA.Body.String())
	}
	var chA domain.Channel
	_ = json.Unmarshal(recChA.Body.Bytes(), &chA)

	// Org A creates a Channel Group
	reqGrA := httptest.NewRequest(http.MethodPost, "/api/v1/channel-groups", strings.NewReader(`{"name":"Org A Group","require_approval":true}`))
	reqGrA.Header.Set("Authorization", "Bearer "+tokenA)
	recGrA := httptest.NewRecorder()
	handler.ServeHTTP(recGrA, reqGrA)
	var grA domain.ChannelGroup
	_ = json.Unmarshal(recGrA.Body.Bytes(), &grA)

	// Org A creates a Post
	reqPoA := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"channel_id":"`+chA.ID+`","content":"Org A Secret"}`))
	reqPoA.Header.Set("Authorization", "Bearer "+tokenA)
	recPoA := httptest.NewRecorder()
	handler.ServeHTTP(recPoA, reqPoA)
	var poA domain.Post
	_ = json.Unmarshal(recPoA.Body.Bytes(), &poA)

	// Org B cannot read Org A's channels, posts, or groups
	reqListChB := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	reqListChB.Header.Set("Authorization", "Bearer "+tokenB)
	recListChB := httptest.NewRecorder()
	handler.ServeHTTP(recListChB, reqListChB)
	var listChB struct{ Channels []domain.Channel `json:"channels"` }
	_ = json.Unmarshal(recListChB.Body.Bytes(), &listChB)
	for _, c := range listChB.Channels {
		if c.ID == chA.ID {
			t.Fatalf("cross-tenant leak: Org B saw Org A channel")
		}
	}

	reqListPoB := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	reqListPoB.Header.Set("Authorization", "Bearer "+tokenB)
	recListPoB := httptest.NewRecorder()
	handler.ServeHTTP(recListPoB, reqListPoB)
	var listPoB struct{ Posts []domain.Post `json:"posts"` }
	_ = json.Unmarshal(recListPoB.Body.Bytes(), &listPoB)
	for _, p := range listPoB.Posts {
		if p.ID == poA.ID {
			t.Fatalf("cross-tenant leak: Org B saw Org A post")
		}
	}

	// Org B cannot get Org A post by ID (404)
	reqGetPoB := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+poA.ID, nil)
	reqGetPoB.Header.Set("Authorization", "Bearer "+tokenB)
	recGetPoB := httptest.NewRecorder()
	handler.ServeHTTP(recGetPoB, reqGetPoB)
	if recGetPoB.Code != http.StatusNotFound && recGetPoB.Code != http.StatusForbidden {
		t.Fatalf("expected 404/403 for cross-tenant post detail, got %d", recGetPoB.Code)
	}

	// Org B cannot delete Org A channel (404)
	reqDelChB := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/"+chA.ID, nil)
	reqDelChB.Header.Set("Authorization", "Bearer "+tokenB)
	recDelChB := httptest.NewRecorder()
	handler.ServeHTTP(recDelChB, reqDelChB)
	if recDelChB.Code != http.StatusNotFound && recDelChB.Code != http.StatusForbidden {
		t.Fatalf("expected 404/403 for cross-tenant delete channel, got %d", recDelChB.Code)
	}

	// Org B cannot delete Org A channel group (400/404)
	reqDelGrB := httptest.NewRequest(http.MethodDelete, "/api/v1/channel-groups/"+grA.ID, nil)
	reqDelGrB.Header.Set("Authorization", "Bearer "+tokenB)
	recDelGrB := httptest.NewRecorder()
	handler.ServeHTTP(recDelGrB, reqDelGrB)
	if recDelGrB.Code != http.StatusBadRequest && recDelGrB.Code != http.StatusNotFound {
		t.Fatalf("expected 400/404 for cross-tenant delete group, got %d", recDelGrB.Code)
	}

	// -------------------------------------------------------------------------
	// 2. RBAC Enforcement
	// -------------------------------------------------------------------------
	// Setup approval required channel in default tenant
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	chRbac, _ := store.UpsertChannel(ctx, &domain.Channel{
		Platform: domain.PlatformFacebook,
		ExternalID: "fb-rbac-ch",
		Name: "Approval Required Channel",
		EncryptedAccessToken: "token",
		TokenStatus: domain.TokenStatusActive,
		IsActive: true,
	})
	cgRbac, _ := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "Strict Approvals", RequireApproval: true})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cgRbac.ID, []string{chRbac.ID})

	// VIEWER cannot create post -> 403
	reqV := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"channel_id":"`+chRbac.ID+`","content":"Viewer post"}`))
	reqV.Header.Set("X-Simulate-Role", "VIEWER")
	recV := httptest.NewRecorder()
	handler.ServeHTTP(recV, reqV)
	if recV.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for VIEWER post creation, got %d", recV.Code)
	}

	// EDITOR creates post with publish_now -> 403 Forbidden
	reqEdDirect := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"channel_id":"`+chRbac.ID+`","content":"Editor direct","publish_now":true}`))
	reqEdDirect.Header.Set("X-Simulate-Role", "EDITOR")
	recEdDirect := httptest.NewRecorder()
	handler.ServeHTTP(recEdDirect, reqEdDirect)
	if recEdDirect.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for EDITOR publish_now on approval channel, got %d", recEdDirect.Code)
	}

	// EDITOR creates post -> forced to PENDING_APPROVAL
	reqEdPost := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"channel_id":"`+chRbac.ID+`","content":"Editor approval post"}`))
	reqEdPost.Header.Set("X-Simulate-Role", "EDITOR")
	recEdPost := httptest.NewRecorder()
	handler.ServeHTTP(recEdPost, reqEdPost)
	if recEdPost.Code != http.StatusCreated {
		t.Fatalf("expected 201 for EDITOR post, got %d", recEdPost.Code)
	}
	var edCreatedPost domain.Post
	_ = json.Unmarshal(recEdPost.Body.Bytes(), &edCreatedPost)
	if edCreatedPost.Status != domain.PostStatusPendingApproval {
		t.Fatalf("expected forced PENDING_APPROVAL, got %s", edCreatedPost.Status)
	}

	// EDITOR cannot call /review -> 403
	reqEdRev := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+edCreatedPost.ID+"/review", strings.NewReader(`{"action":"APPROVED"}`))
	reqEdRev.Header.Set("X-Simulate-Role", "EDITOR")
	recEdRev := httptest.NewRecorder()
	handler.ServeHTTP(recEdRev, reqEdRev)
	if recEdRev.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for EDITOR review, got %d", recEdRev.Code)
	}

	// -------------------------------------------------------------------------
	// 3. Approval Workflow (Reject -> Resubmit -> Approve) & Batch Approval
	// -------------------------------------------------------------------------
	// APPROVER rejects with note -> REJECTED
	reqApprRej := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+edCreatedPost.ID+"/review", strings.NewReader(`{"action":"REJECTED","note":"Fix grammar"}`))
	reqApprRej.Header.Set("X-Simulate-Role", "APPROVER")
	recApprRej := httptest.NewRecorder()
	handler.ServeHTTP(recApprRej, reqApprRej)
	if recApprRej.Code != http.StatusOK {
		t.Fatalf("expected 200 for APPROVER rejection, got %d", recApprRej.Code)
	}
	pRejected, _ := store.GetPostByID(ctx, edCreatedPost.ID)
	if pRejected.Status != domain.PostStatusRejected || pRejected.RejectionNote != "Fix grammar" {
		t.Fatalf("expected REJECTED with note, got %s, note=%s", pRejected.Status, pRejected.RejectionNote)
	}

	// EDITOR resubmits -> PENDING_APPROVAL
	reqResub := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+edCreatedPost.ID+"/submit-approval", strings.NewReader(`{"content":"Fixed grammar text"}`))
	reqResub.Header.Set("X-Simulate-Role", "EDITOR")
	recResub := httptest.NewRecorder()
	handler.ServeHTTP(recResub, reqResub)
	if recResub.Code != http.StatusOK {
		t.Fatalf("expected 200 for submit-approval, got %d", recResub.Code)
	}
	pResubmitted, _ := store.GetPostByID(ctx, edCreatedPost.ID)
	if pResubmitted.Status != domain.PostStatusPendingApproval || pResubmitted.RejectionNote != "" {
		t.Fatalf("expected PENDING_APPROVAL with cleared note, got %s", pResubmitted.Status)
	}

	// APPROVER approves -> SCHEDULED
	reqApprOk := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+edCreatedPost.ID+"/review", strings.NewReader(`{"action":"APPROVED"}`))
	reqApprOk.Header.Set("X-Simulate-Role", "APPROVER")
	recApprOk := httptest.NewRecorder()
	handler.ServeHTTP(recApprOk, reqApprOk)
	if recApprOk.Code != http.StatusOK {
		t.Fatalf("expected 200 for APPROVER approval, got %d", recApprOk.Code)
	}
	pApproved, _ := store.GetPostByID(ctx, edCreatedPost.ID)
	if pApproved.Status != domain.PostStatusScheduled && pApproved.Status != domain.PostStatusQueued {
		t.Fatalf("expected SCHEDULED or QUEUED after approval, got %s", pApproved.Status)
	}

	// Batch Fan-Out and Batch Approval via batch_id
	chRbac2, _ := store.UpsertChannel(ctx, &domain.Channel{
		Platform: domain.PlatformFacebook,
		ExternalID: "fb-rbac-ch-2",
		Name: "Approval Channel 2",
		EncryptedAccessToken: "token2",
		TokenStatus: domain.TokenStatusActive,
		IsActive: true,
	})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cgRbac.ID, []string{chRbac.ID, chRbac2.ID})

	futureSched := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	reqBatchPost := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"channel_group_id":"`+cgRbac.ID+`","content":"Batch Content","scheduled_at":"`+futureSched+`"}`))
	reqBatchPost.Header.Set("X-Simulate-Role", "EDITOR")
	recBatchPost := httptest.NewRecorder()
	handler.ServeHTTP(recBatchPost, reqBatchPost)
	if recBatchPost.Code != http.StatusCreated {
		t.Fatalf("expected 201 for batch post, got %d: %s", recBatchPost.Code, recBatchPost.Body.String())
	}
	var batchResp map[string]any
	_ = json.Unmarshal(recBatchPost.Body.Bytes(), &batchResp)
	bPosts := batchResp["posts"].([]any)
	if len(bPosts) != 2 {
		t.Fatalf("expected 2 posts created in batch, got %d", len(bPosts))
	}
	bP1ID := bPosts[0].(map[string]any)["id"].(string)
	bP2ID := bPosts[1].(map[string]any)["id"].(string)

	// Batch approval with apply_to_batch: true
	reqBatchAppr := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+bP1ID+"/review", strings.NewReader(`{"action":"APPROVED","apply_to_batch":true}`))
	reqBatchAppr.Header.Set("X-Simulate-Role", "APPROVER")
	recBatchAppr := httptest.NewRecorder()
	handler.ServeHTTP(recBatchAppr, reqBatchAppr)
	if recBatchAppr.Code != http.StatusOK {
		t.Fatalf("expected 200 for batch approval, got %d", recBatchAppr.Code)
	}
	bp1Updated, _ := store.GetPostByID(ctx, bP1ID)
	bp2Updated, _ := store.GetPostByID(ctx, bP2ID)
	if bp1Updated.Status != domain.PostStatusScheduled || bp2Updated.Status != domain.PostStatusScheduled {
		t.Fatalf("expected both posts to be SCHEDULED via batch approval, got bp1=%s, bp2=%s", bp1Updated.Status, bp2Updated.Status)
	}

	// -------------------------------------------------------------------------
	// 4. Backward Compatibility: Requests without X-Organization-Id work on Default Workspace
	// -------------------------------------------------------------------------
	reqLegacy := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	recLegacy := httptest.NewRecorder()
	handler.ServeHTTP(recLegacy, reqLegacy)
	if recLegacy.Code != http.StatusOK {
		t.Fatalf("expected 200 for legacy request without tenant header, got %d", recLegacy.Code)
	}
}

