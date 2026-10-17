package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/happy-darwin/fb-autoposter/internal/config"
	"github.com/happy-darwin/fb-autoposter/internal/crypto"
	"github.com/happy-darwin/fb-autoposter/internal/database"
	"github.com/happy-darwin/fb-autoposter/internal/facebook"
	"github.com/happy-darwin/fb-autoposter/internal/infra/ai"
	"github.com/happy-darwin/fb-autoposter/internal/infra/notifier"
	"github.com/happy-darwin/fb-autoposter/internal/infra/redis"
	"github.com/happy-darwin/fb-autoposter/internal/infra/storage"
	"github.com/happy-darwin/fb-autoposter/internal/worker"
)

func TestAPIEndpointsFlow(t *testing.T) {
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

	// 2. Test POST /api/v1/pages
	pageBody := `{"page_id":"100200300","page_name":"Darwin Official","category":"Tech","access_token":"EAABsbCS1iHgBO1234567890"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/pages", strings.NewReader(pageBody))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected POST /api/v1/pages 201, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var createdPage map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &createdPage)
	pageUUID, _ := createdPage["id"].(string)
	if pageUUID == "" {
		t.Fatalf("expected non-empty page UUID")
	}

	// 3. Test POST /api/v1/posts
	postPayload, _ := json.Marshal(map[string]any{
		"page_uuid":   pageUUID,
		"content":     "Hello Facebook from Golang Auto-Poster!",
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

	// 4. Test POST /api/v1/ai/generate (fallback template)
	req4 := httptest.NewRequest(http.MethodPost, "/api/v1/ai/generate", strings.NewReader(`{"topic":"Chuyển đổi số AI"}`))
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
}
