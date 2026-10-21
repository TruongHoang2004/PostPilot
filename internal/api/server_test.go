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

	"github.com/TruongHoang2004/PostPilot/internal/config"
	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/database"
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
