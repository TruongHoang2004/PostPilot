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
	"github.com/happy-darwin/fb-autoposter/internal/domain"
	"github.com/happy-darwin/fb-autoposter/internal/facebook"
	"github.com/happy-darwin/fb-autoposter/internal/infra/ai"
	"github.com/happy-darwin/fb-autoposter/internal/infra/notifier"
	"github.com/happy-darwin/fb-autoposter/internal/infra/redis"
	"github.com/happy-darwin/fb-autoposter/internal/infra/storage"
	"github.com/happy-darwin/fb-autoposter/internal/worker"
)

// Server điều phối HTTP Router, gắn kết các Bounded Context Handlers (Publishing, Channel, Media, AI, System)
// và các Platform Adapters (Facebook OAuth/Webhook).
type Server struct {
	cfg       *config.Config
	store     database.Store
	redis     *redis.Client
	cipher    *crypto.TokenCipher
	registry  *domain.PublisherRegistry
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
	reg := domain.NewPublisherRegistry()
	if fbClient != nil {
		reg.Register(facebook.NewPublisherAdapter(fbClient))
	}
	return &Server{
		cfg:       cfg,
		store:     store,
		redis:     redisClient,
		cipher:    cipher,
		registry:  reg,
		fbClient:  fbClient,
		s3:        s3,
		aiGen:     aiGen,
		notifier:  notif,
		processor: proc,
		logger:    logger,
	}
}

// WithPublisherRegistry cho phép tiêm (inject) bộ đăng ký Adapter đa nền tảng tùy chỉnh vào API Server.
func (s *Server) WithPublisherRegistry(reg *domain.PublisherRegistry) *Server {
	if reg != nil {
		s.registry = reg
	}
	return s
}

// Handler khởi tạo HTTP Router với các module đã được phân tách theo Bounded Context.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 1. System & Observability Module (system_handler.go & ui_dashboard.go)
	mux.HandleFunc("GET /", s.handleDashboardUI)
	mux.HandleFunc("GET /healthz", s.handleHealthCheck)
	mux.HandleFunc("GET /api/v1/stats", s.handleGetStats)

	// 2. Post & Publishing Core Domain Module (publishing_handler.go)
	mux.HandleFunc("GET /api/v1/posts", s.handleListPosts)
	mux.HandleFunc("POST /api/v1/posts", s.handleCreatePost)
	mux.HandleFunc("GET /api/v1/posts/{id}", s.handleGetPostDetail)
	mux.HandleFunc("POST /api/v1/posts/{id}/publish-now", s.handlePublishPostNow)
	mux.HandleFunc("POST /api/v1/posts/{id}/cancel", s.handleCancelPost)

	// 3. Channel (Publishing Target) Module (channel_handler.go)
	mux.HandleFunc("GET /api/v1/channels", s.handleListChannels)
	mux.HandleFunc("POST /api/v1/channels", s.handleUpsertChannel)
	mux.HandleFunc("DELETE /api/v1/channels/{id}", s.handleDeleteChannel)
	mux.HandleFunc("POST /api/v1/channels/{id}/verify-token", s.handleVerifyChannelToken)

	// 4. Media Management Module (media_handler.go)
	_ = os.MkdirAll("./uploads", 0755)
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./uploads"))))
	mux.HandleFunc("POST /api/v1/media/upload", s.handleMediaUpload)
	mux.HandleFunc("POST /api/v1/media/presign", s.handlePresignUpload)

	// 5. AI Content Generation Module (ai_handler.go)
	mux.HandleFunc("POST /api/v1/ai/generate", s.handleAIGenerateContent)

	// 6. Facebook Platform Adapter Module: OAuth2 & Webhooks (facebook_handler.go)
	mux.HandleFunc("GET /api/v1/auth/facebook/login", s.handleFBOAuthLogin)
	mux.HandleFunc("GET /api/v1/auth/facebook/callback", s.handleFBOAuthCallback)
	mux.HandleFunc("POST /api/v1/auth/facebook/exchange-token", s.handleFBExchangeUserToken)
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
