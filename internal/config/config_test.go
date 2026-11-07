package config

import "testing"

func TestLoadParsesStorageProviderAndR2(t *testing.T) {
	t.Setenv("STORAGE_PROVIDER", " R2 ")
	t.Setenv("R2_ACCOUNT_ID", " acct ")
	t.Setenv("R2_ACCESS_KEY_ID", "key")
	t.Setenv("R2_SECRET_ACCESS_KEY", "secret")
	t.Setenv("R2_BUCKET", "media")
	t.Setenv("R2_PUBLIC_BASE_URL", "https://cdn.example.com/")
	t.Setenv("R2_ENDPOINT", "")
	cfg := Load()
	if cfg.StorageProvider != "r2" || cfg.R2AccountID != "acct" || cfg.R2Bucket != "media" || cfg.R2PublicBaseURL != "https://cdn.example.com" || cfg.R2Endpoint != "" {
		t.Fatalf("R2 settings not parsed: %+v", cfg)
	}
}

func TestLoadDefaultsToS3(t *testing.T) {
	t.Setenv("STORAGE_PROVIDER", "")
	if cfg := Load(); cfg.StorageProvider != "s3" {
		t.Fatalf("default provider must stay s3, got %q", cfg.StorageProvider)
	}
}
