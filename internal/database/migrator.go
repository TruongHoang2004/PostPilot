package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	_ "github.com/happy-darwin/fb-autoposter/internal/database/pgdriver"
	"github.com/happy-darwin/fb-autoposter/migrations"
)

// MigrationRecord đại diện cho 1 bản ghi lịch sử migration đã được thực thi trong DB.
type MigrationRecord struct {
	Version   string
	Name      string
	AppliedAt string
}

// MigrationFile đại diện cho 1 cặp file .up.sql / .down.sql trong thư mục migrations/.
type MigrationFile struct {
	Version  string
	Name     string
	UpFile   string
	DownFile string
	UpSQL    string
	DownSQL  string
}

// Migrator quản lý việc nâng cấp (Up) và hạ cấp (Down) schema PostgreSQL bằng Golang.
type Migrator struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewMigrator khởi tạo bộ chạy migration từ kết nối *sql.DB.
func NewMigrator(db *sql.DB, logger *slog.Logger) *Migrator {
	return &Migrator{
		db:     db,
		logger: logger,
	}
}

func (m *Migrator) ensureMigrationsTable(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(32) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`
	_, err := m.db.ExecContext(ctx, query)
	return err
}

// LoadEmbeddedMigrations đọc tất cả file *.sql được nhúng qua embed.FS.
func LoadEmbeddedMigrations() ([]MigrationFile, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations dir: %w", err)
	}

	byVersion := make(map[string]*MigrationFile)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		if !strings.HasSuffix(fileName, ".sql") {
			continue
		}

		parts := strings.SplitN(fileName, "_", 2)
		if len(parts) < 2 {
			continue
		}
		version := parts[0]
		rest := parts[1]

		mf, exists := byVersion[version]
		if !exists {
			mf = &MigrationFile{Version: version}
			byVersion[version] = mf
		}

		rawBytes, err := fs.ReadFile(migrations.FS, fileName)
		if err != nil {
			return nil, fmt.Errorf("read migration file %s: %w", fileName, err)
		}

		if strings.HasSuffix(rest, ".up.sql") {
			mf.Name = strings.TrimSuffix(rest, ".up.sql")
			mf.UpFile = fileName
			mf.UpSQL = string(rawBytes)
		} else if strings.HasSuffix(rest, ".down.sql") {
			if mf.Name == "" {
				mf.Name = strings.TrimSuffix(rest, ".down.sql")
			}
			mf.DownFile = fileName
			mf.DownSQL = string(rawBytes)
		}
	}

	result := make([]MigrationFile, 0, len(byVersion))
	for _, mf := range byVersion {
		result = append(result, *mf)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})
	return result, nil
}

func (m *Migrator) getAppliedVersions(ctx context.Context) (map[string]MigrationRecord, error) {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return nil, err
	}

	rows, err := m.db.QueryContext(ctx, "SELECT version, name, applied_at::text FROM schema_migrations ORDER BY version ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]MigrationRecord)
	for rows.Next() {
		var rec MigrationRecord
		if err := rows.Scan(&rec.Version, &rec.Name, &rec.AppliedAt); err != nil {
			return nil, err
		}
		applied[rec.Version] = rec
	}
	return applied, nil
}

// Up chạy tất cả các bản migration chưa được áp dụng theo đúng thứ tự version tăng dần.
func (m *Migrator) Up(ctx context.Context) (int, error) {
	files, err := LoadEmbeddedMigrations()
	if err != nil {
		return 0, err
	}

	applied, err := m.getAppliedVersions(ctx)
	if err != nil {
		return 0, err
	}

	appliedCount := 0
	for _, mf := range files {
		if _, ok := applied[mf.Version]; ok {
			continue
		}

		m.logger.Info("Applying migration UP", "version", mf.Version, "name", mf.Name)
		start := time.Now()

		if _, err := m.db.ExecContext(ctx, mf.UpSQL); err != nil {
			return appliedCount, fmt.Errorf("apply migration %s (%s) failed: %w", mf.Version, mf.Name, err)
		}

		_, err = m.db.ExecContext(
			ctx,
			"INSERT INTO schema_migrations (version, name, applied_at) VALUES ($1, $2, $3)",
			mf.Version, mf.Name, time.Now().UTC(),
		)
		if err != nil {
			return appliedCount, fmt.Errorf("record migration %s failed: %w", mf.Version, err)
		}

		appliedCount++
		m.logger.Info("Migration UP completed", "version", mf.Version, "duration_ms", time.Since(start).Milliseconds())
	}

	return appliedCount, nil
}

// DownOne rollback bản migration gần nhất vừa được áp dụng.
func (m *Migrator) DownOne(ctx context.Context) (string, error) {
	files, err := LoadEmbeddedMigrations()
	if err != nil {
		return "", err
	}

	applied, err := m.getAppliedVersions(ctx)
	if err != nil {
		return "", err
	}

	for i := len(files) - 1; i >= 0; i-- {
		mf := files[i]
		if _, ok := applied[mf.Version]; !ok {
			continue
		}

		m.logger.Info("Rolling back migration DOWN", "version", mf.Version, "name", mf.Name)
		if _, err := m.db.ExecContext(ctx, mf.DownSQL); err != nil {
			return "", fmt.Errorf("rollback migration %s failed: %w", mf.Version, err)
		}

		if _, err := m.db.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = $1", mf.Version); err != nil {
			return "", fmt.Errorf("remove schema_migrations record %s failed: %w", mf.Version, err)
		}

		return mf.Version + "_" + mf.Name, nil
	}

	return "", nil
}

// Status trả về danh sách trạng thái tất cả các bản migration.
func (m *Migrator) Status(ctx context.Context) ([]map[string]string, error) {
	files, err := LoadEmbeddedMigrations()
	if err != nil {
		return nil, err
	}

	applied, err := m.getAppliedVersions(ctx)
	if err != nil {
		return nil, err
	}

	var statusList []map[string]string
	for _, mf := range files {
		state := "PENDING"
		appliedAt := "-"
		if rec, ok := applied[mf.Version]; ok {
			state = "APPLIED"
			appliedAt = rec.AppliedAt
		}
		statusList = append(statusList, map[string]string{
			"version":    mf.Version,
			"name":       mf.Name,
			"status":     state,
			"applied_at": appliedAt,
		})
	}
	return statusList, nil
}
