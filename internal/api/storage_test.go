package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/TruongHoang2004/PostPilot/internal/config"
	"github.com/TruongHoang2004/PostPilot/internal/infra/storage"
)

// Selecting Cloudflare R2 without credentials must not break the API: health reports the state and
// the upload endpoints answer 503 instead of failing or silently falling back to local disk.
func TestUnconfiguredR2DisablesUploadsWithoutBreakingAPI(t *testing.T) {
	srv, handler, _ := setupTestServer(t)
	srv.s3 = storage.NewFromConfig(&config.Config{StorageProvider: "r2", R2Bucket: "media"})

	rec := doJSON(t, srv, handler, http.MethodGet, "/healthz", "", "")
	var health map[string]any
	decode(t, rec, &health)
	if health["storage_provider"] != "r2" || health["storage_configured"] != false {
		t.Fatalf("health must report the unconfigured R2 provider: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "R2_SECRET") {
		t.Fatal("health must not leak configuration details")
	}
	if rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/media/presign", `{"file_name":"a.png"}`, ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("presign expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/media/upload", "", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("upload expected 503, got %d: %s", rec.Code, rec.Body.String())
	}

	// Once the R2 settings are complete the same endpoint hands out an R2 URL.
	srv.s3 = storage.NewFromConfig(&config.Config{StorageProvider: "r2", R2AccountID: "0123456789abcdef0123456789abcdef", R2Bucket: "media", R2AccessKeyID: "k", R2SecretAccessKey: "s", R2PublicBaseURL: "https://cdn.example.com"})
	rec = doJSON(t, srv, handler, http.MethodPost, "/api/v1/media/presign", `{"file_name":"a.png","mime_type":"image/png","file_size_bytes":10}`, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com") || !strings.Contains(rec.Body.String(), "https://cdn.example.com/uploads/") {
		t.Fatalf("configured R2 presign wrong: %d %s", rec.Code, rec.Body.String())
	}
}
