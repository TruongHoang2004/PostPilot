package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/config"
	"github.com/TruongHoang2004/PostPilot/internal/database"
	_ "github.com/TruongHoang2004/PostPilot/internal/database/pgdriver"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		logger.Error("Failed to open postgres connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Wait for PostgreSQL readiness when starting up in Docker Compose
	for attempt := 1; attempt <= 15; attempt++ {
		if err := db.PingContext(ctx); err == nil {
			break
		} else if attempt == 15 {
			logger.Error("PostgreSQL is not reachable after 15 attempts", "error", err)
			os.Exit(1)
		}
		logger.Info("Waiting for PostgreSQL to be ready...", "attempt", attempt)
		time.Sleep(2 * time.Second)
	}

	migrator := database.NewMigrator(db, logger)

	switch command {
	case "up":
		applied, err := migrator.Up(ctx)
		if err != nil {
			logger.Error("Migration UP failed", "error", err)
			os.Exit(1)
		}
		logger.Info("Migration UP finished successfully", "applied_count", applied)

	case "down":
		rolledBack, err := migrator.DownOne(ctx)
		if err != nil {
			logger.Error("Migration DOWN failed", "error", err)
			os.Exit(1)
		}
		if rolledBack == "" {
			logger.Info("No applied migrations to roll back")
		} else {
			logger.Info("Rolled back migration successfully", "migration", rolledBack)
		}

	case "status":
		statuses, err := migrator.Status(ctx)
		if err != nil {
			logger.Error("Migration STATUS failed", "error", err)
			os.Exit(1)
		}
		fmt.Printf("%-10s | %-36s | %-10s | %s\n", "VERSION", "NAME", "STATUS", "APPLIED AT")
		fmt.Println("----------------------------------------------------------------------------------------")
		for _, st := range statuses {
			fmt.Printf("%-10s | %-36s | %-10s | %s\n", st["version"], st["name"], st["status"], st["applied_at"])
		}

	default:
		fmt.Fprintf(os.Stderr, "Usage: %s [up|down|status]\n", os.Args[0])
		os.Exit(1)
	}
}
