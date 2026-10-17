package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/happy-darwin/fb-autoposter/internal/config"
	"github.com/happy-darwin/fb-autoposter/internal/crypto"
	"github.com/happy-darwin/fb-autoposter/internal/database"
	_ "github.com/happy-darwin/fb-autoposter/internal/database/pgdriver"
	"github.com/happy-darwin/fb-autoposter/internal/facebook"
	"github.com/happy-darwin/fb-autoposter/internal/infra/notifier"
	"github.com/happy-darwin/fb-autoposter/internal/infra/redis"
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
	if err != nil {
		logger.Error("Failed to open postgres connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	connectedPG := false
	for attempt := 1; attempt <= 10; attempt++ {
		if err := db.Ping(); err == nil {
			connectedPG = true
			break
		}
		logger.Info("Worker waiting for PostgreSQL...", "attempt", attempt)
		time.Sleep(2 * time.Second)
	}

	if connectedPG {
		logger.Info("Worker connected to PostgreSQL")
		store = database.NewPostgresStore(db)
	} else {
		logger.Warn("Worker falling back to InMemoryStore")
		store = database.NewInMemoryStore()
	}

	redisClient := redis.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	fbClient := facebook.NewClient(cfg.FBGraphBaseURL, cfg.FBGraphVersion)
	notif := notifier.NewTelegramNotifier(cfg.TelegramBotToken, cfg.TelegramChatID, logger)

	sched := worker.NewScheduler(cfg, store, redisClient, cipher, fbClient, notif, logger)
	proc := worker.NewProcessor(cfg, store, redisClient, cipher, fbClient, notif, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	sched.Start(ctx)
	proc.Start(ctx, &wg)

	logger.Info("FB Auto-Poster Worker Service running",
		"fast_concurrency", cfg.FastWorkerConcurrency,
		"heavy_concurrency", cfg.HeavyWorkerConcurrency,
		"scheduler_interval", cfg.SchedulerInterval.String(),
	)

	<-ctx.Done()
	logger.Info("Shutting down Worker Service gracefully, waiting for active jobs...")
	wg.Wait()
	logger.Info("All workers stopped cleanly")
}
