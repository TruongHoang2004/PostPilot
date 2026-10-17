package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/happy-darwin/fb-autoposter/internal/api"
	"github.com/happy-darwin/fb-autoposter/internal/config"
	"github.com/happy-darwin/fb-autoposter/internal/crypto"
	"github.com/happy-darwin/fb-autoposter/internal/database"
	_ "github.com/happy-darwin/fb-autoposter/internal/database/pgdriver"
	"github.com/happy-darwin/fb-autoposter/internal/facebook"
	"github.com/happy-darwin/fb-autoposter/internal/infra/ai"
	"github.com/happy-darwin/fb-autoposter/internal/infra/notifier"
	"github.com/happy-darwin/fb-autoposter/internal/infra/redis"
	"github.com/happy-darwin/fb-autoposter/internal/infra/storage"
	"github.com/happy-darwin/fb-autoposter/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	cipher, err := crypto.NewTokenCipher(cfg.TokenEncryptionKey)
	if err != nil {
		logger.Error("Invalid TOKEN_ENCRYPTION_KEY", "error", err)
		os.Exit(1)
	}

	var store database.Store
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err == nil && db.Ping() == nil {
		logger.Info("Connected to PostgreSQL database")
		migrator := database.NewMigrator(db, logger)
		if applied, mErr := migrator.Up(context.Background()); mErr != nil {
			logger.Warn("Auto-migration warning", "error", mErr)
		} else if applied > 0 {
			logger.Info("Applied pending database migrations on startup", "count", applied)
		}
		store = database.NewPostgresStore(db)
	} else {
		logger.Warn("PostgreSQL not reachable, using InMemoryStore fallback for local preview", "error", err)
		store = database.NewInMemoryStore()
	}

	redisClient := redis.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if redisClient.IsMemoryFallback() {
		logger.Warn("Redis not reachable, using in-memory queue fallback")
	} else {
		logger.Info("Connected to Redis queue broker", "addr", cfg.RedisAddr)
	}

	fbClient := facebook.NewClient(cfg.FBGraphBaseURL, cfg.FBGraphVersion)
	s3Service := storage.NewS3Service(cfg.S3Endpoint, cfg.S3PublicBaseURL, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey)
	aiGen := ai.NewGeminiGenerator(cfg.GeminiAPIKey, cfg.GeminiModel)
	notif := notifier.NewTelegramNotifier(cfg.TelegramBotToken, cfg.TelegramChatID, logger)
	proc := worker.NewProcessor(cfg, store, redisClient, cipher, fbClient, notif, logger)

	apiServer := api.NewServer(cfg, store, redisClient, cipher, fbClient, s3Service, aiGen, notif, proc, logger)

	httpSrv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      apiServer.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("FB Auto-Poster API & Dashboard listening", "url", "http://localhost:"+cfg.HTTPPort)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down API server gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}
