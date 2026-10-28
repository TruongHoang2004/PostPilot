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

	"github.com/TruongHoang2004/PostPilot/internal/api"
	"github.com/TruongHoang2004/PostPilot/internal/config"
	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/database"
	_ "github.com/TruongHoang2004/PostPilot/internal/database/pgdriver"
	"github.com/TruongHoang2004/PostPilot/internal/infra/ai"
	"github.com/TruongHoang2004/PostPilot/internal/infra/notifier"
	"github.com/TruongHoang2004/PostPilot/internal/infra/redis"
	"github.com/TruongHoang2004/PostPilot/internal/infra/storage"
	"github.com/TruongHoang2004/PostPilot/internal/platform/facebook"
	"github.com/TruongHoang2004/PostPilot/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	cipher, err := crypto.NewTokenCipher(cfg.TokenEncryptionKey)
	if err != nil {
		logger.Error("Invalid TOKEN_ENCRYPTION_KEY", "error", err)
		os.Exit(1)
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		logger.Error("Cannot open PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err = db.PingContext(context.Background()); err != nil {
		logger.Error("PostgreSQL required", "error", err)
		os.Exit(1)
	}
	if _, err = database.NewMigrator(db, logger).Up(context.Background()); err != nil {
		logger.Error("Migration failed", "error", err)
		os.Exit(1)
	}
	store := database.NewPostgresStore(db)
	redisClient := redis.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err = redisClient.Ping(context.Background()); err != nil {
		logger.Error("Redis required", "error", err)
		os.Exit(1)
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
