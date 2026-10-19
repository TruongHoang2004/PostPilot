package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

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

// Server quản lý toàn bộ RESTful API và Giao diện Quản trị Web (Admin Dashboard).
type Server struct {
	cfg       *config.Config
	store     database.Store
	redis     *redis.Client
	cipher    *crypto.TokenCipher
	fbClient  *facebook.Client
	s3        *storage.S3Service
	aiGen     *ai.GeminiGenerator
	notifier  *notifier.TelegramNotifier
	processor *worker.Processor
	logger    *slog.Logger
}

func NewServer(
	cfg *config.Config,
	store database.Store,
	redisClient *redis.Client,
	cipher *crypto.TokenCipher,
	fbClient *facebook.Client,
	s3 *storage.S3Service,
	aiGen *ai.GeminiGenerator,
	notif *notifier.TelegramNotifier,
	proc *worker.Processor,
	logger *slog.Logger,
) *Server {
	return &Server{
		cfg:       cfg,
		store:     store,
		redis:     redisClient,
		cipher:    cipher,
		fbClient:  fbClient,
		s3:        s3,
		aiGen:     aiGen,
		notifier:  notif,
		processor: proc,
		logger:    logger,
	}
}

// Handler khởi tạo HTTP Router với đầy đủ các endpoint chuẩn RESTful API v1.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 1. Giao diện Dashboard & Healthcheck
	mux.HandleFunc("GET /", s.handleDashboardUI)
	mux.HandleFunc("GET /healthz", s.handleHealthCheck)
	mux.HandleFunc("GET /api/v1/stats", s.handleGetStats)

	// 2. Luồng OAuth2 & Tự động đổi Permanent Page Token
	mux.HandleFunc("GET /api/v1/auth/facebook/login", s.handleFBOAuthLogin)
	mux.HandleFunc("GET /api/v1/auth/facebook/callback", s.handleFBOAuthCallback)
	mux.HandleFunc("POST /api/v1/auth/facebook/exchange-token", s.handleFBExchangeUserToken)

	// 3. Quản lý Fanpage & Sức khỏe Token
	mux.HandleFunc("GET /api/v1/pages", s.handleListPages)
	mux.HandleFunc("POST /api/v1/pages", s.handleUpsertPage)
	mux.HandleFunc("POST /api/v1/pages/{id}/verify-token", s.handleVerifyPageToken)

	// 4. Quản lý Bài viết, Lên lịch & Đăng ngay (Publish Now)
	mux.HandleFunc("GET /api/v1/posts", s.handleListPosts)
	mux.HandleFunc("POST /api/v1/posts", s.handleCreatePost)
	mux.HandleFunc("GET /api/v1/posts/{id}", s.handleGetPostDetail)
	mux.HandleFunc("POST /api/v1/posts/{id}/publish-now", s.handlePublishPostNow)
	mux.HandleFunc("POST /api/v1/posts/{id}/cancel", s.handleCancelPost)

	// 5. Media Upload (Direct & Presigned), AI Content Generator & Meta Webhook
	_ = os.MkdirAll("./uploads", 0755)
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./uploads"))))
	mux.HandleFunc("POST /api/v1/media/upload", s.handleMediaUpload)
	mux.HandleFunc("POST /api/v1/media/presign", s.handlePresignUpload)
	mux.HandleFunc("POST /api/v1/ai/generate", s.handleAIGenerateContent)
	mux.HandleFunc("GET /api/v1/webhooks/facebook", s.handleFBWebhookVerify)
	mux.HandleFunc("POST /api/v1/webhooks/facebook", s.handleFBWebhookEvent)

	return s.withMiddlewares(mux)
}

func (s *Server) withMiddlewares(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("Recovered from HTTP panic", "panic", rec, "path", r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"error": "internal server error",
				})
			}
		}()

		next.ServeHTTP(w, r)

		s.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
