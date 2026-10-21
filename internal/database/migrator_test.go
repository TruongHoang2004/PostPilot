package database

import "testing"

func TestLoadEmbeddedMigrations(t *testing.T) {
	files, err := LoadEmbeddedMigrations()
	if err != nil {
		t.Fatalf("failed to load embedded migrations: %v", err)
	}
	if len(files) < 3 {
		t.Fatalf("expected at least 3 migrations, got %d", len(files))
	}
	if files[0].Version != "000001" || files[0].UpSQL == "" || files[0].DownSQL == "" {
		t.Fatalf("unexpected first migration metadata: %+v", files[0])
	}
	if files[1].Version != "000002" || files[1].UpSQL == "" || files[1].DownSQL == "" {
		t.Fatalf("unexpected second migration metadata: %+v", files[1])
	}
	if files[2].Version != "000003" || files[2].UpSQL == "" || files[2].DownSQL == "" {
		t.Fatalf("unexpected third migration metadata: %+v", files[2])
	}
}
