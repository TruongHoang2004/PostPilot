package database

import "testing"

func TestLoadEmbeddedMigrations(t *testing.T) {
	files, err := LoadEmbeddedMigrations()
	if err != nil {
		t.Fatalf("failed to load embedded migrations: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("expected at least 2 migrations, got %d", len(files))
	}
	if files[0].Version != "000001" || files[0].UpSQL == "" || files[0].DownSQL == "" {
		t.Fatalf("unexpected first migration metadata: %+v", files[0])
	}
	if files[1].Version != "000002" || files[1].UpSQL == "" || files[1].DownSQL == "" {
		t.Fatalf("unexpected second migration metadata: %+v", files[1])
	}
}
