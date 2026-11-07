package storage

import (
	"net/url"
	"strings"
	"testing"

	"github.com/TruongHoang2004/PostPilot/internal/config"
)

func r2Config() *config.Config {
	return &config.Config{
		StorageProvider: "r2", R2AccountID: "0123456789abcdef0123456789abcdef", R2Bucket: "postpilot-media",
		R2AccessKeyID: "r2-access", R2SecretAccessKey: "r2-secret", R2PublicBaseURL: "https://media.example.com",
		S3Endpoint: "http://localhost:9000", S3PublicBaseURL: "http://localhost:9000/fb-media", S3Region: "us-east-1", S3Bucket: "fb-media", S3AccessKey: "minioadmin", S3SecretKey: "minioadmin123",
	}
}

func TestR2ServiceSignsForCloudflare(t *testing.T) {
	svc := NewFromConfig(r2Config())
	if svc.Provider != ProviderR2 || !svc.Configured() || len(svc.Problems()) != 0 {
		t.Fatalf("complete R2 settings must be configured: %+v problems=%v", svc, svc.Problems())
	}
	if svc.Endpoint != "https://0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com" || svc.Region != "auto" {
		t.Fatalf("endpoint/region wrong: %s %s", svc.Endpoint, svc.Region)
	}
	signed, err := svc.GeneratePresignedPutURL("ảnh sản phẩm.png", 600)
	if err != nil {
		t.Fatal(err)
	}
	upload, err := url.Parse(signed.UploadURL)
	if err != nil {
		t.Fatal(err)
	}
	if upload.Host != "0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com" || !strings.HasPrefix(upload.Path, "/postpilot-media/uploads/") {
		t.Fatalf("path-style R2 URL expected, got %s", signed.UploadURL)
	}
	q := upload.Query()
	if !strings.HasSuffix(q.Get("X-Amz-Credential"), "/auto/s3/aws4_request") || !strings.HasPrefix(q.Get("X-Amz-Credential"), "r2-access/") {
		t.Fatalf("credential scope must use region auto: %s", q.Get("X-Amz-Credential"))
	}
	if q.Get("X-Amz-Signature") == "" || q.Get("X-Amz-Expires") != "600" || q.Get("X-Amz-SignedHeaders") != "host" {
		t.Fatalf("presign parameters wrong: %v", q)
	}
	if !strings.HasPrefix(signed.PublicURL, "https://media.example.com/uploads/") || strings.Contains(signed.PublicURL, "r2.cloudflarestorage.com") {
		t.Fatalf("public URL must use the public bucket domain, never the signing endpoint: %s", signed.PublicURL)
	}
	if strings.Contains(signed.UploadURL, "r2-secret") || strings.Contains(signed.PublicURL, "r2-secret") {
		t.Fatal("secret key must never appear in URLs")
	}
}

func TestIncompleteR2NeverBlocksStartup(t *testing.T) {
	cfg := r2Config()
	cfg.R2AccountID, cfg.R2SecretAccessKey, cfg.R2PublicBaseURL = "", "", ""
	svc := NewFromConfig(cfg) // must not panic or fail
	if svc.Configured() {
		t.Fatal("R2 without account/secret/public URL must report not configured")
	}
	joined := strings.Join(svc.Problems(), "; ")
	for _, want := range []string{"R2_ACCOUNT_ID", "R2_SECRET_ACCESS_KEY", "R2_PUBLIC_BASE_URL"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("problems must name %s: %s", want, joined)
		}
	}
	if strings.Contains(joined, "r2-access") {
		t.Fatal("problems must not echo credentials")
	}
}

func TestR2EndpointOverrideAndTransportSafety(t *testing.T) {
	cfg := r2Config()
	cfg.R2Endpoint = "https://0123.eu.r2.cloudflarestorage.com/"
	if svc := NewFromConfig(cfg); svc.Endpoint != "https://0123.eu.r2.cloudflarestorage.com" || !svc.Configured() {
		t.Fatalf("jurisdiction override must win and be trimmed: %+v", svc)
	}
	cfg.R2Endpoint = "http://203.0.113.9"
	if svc := NewFromConfig(cfg); svc.Configured() {
		t.Fatal("plain HTTP R2 endpoint must be rejected because presigned URLs carry the signature")
	}
	cfg.R2Endpoint = "http://127.0.0.1:9999"
	if svc := NewFromConfig(cfg); !svc.Configured() {
		t.Fatal("loopback HTTP is allowed for test servers")
	}
}

func TestDefaultProviderStaysS3AndUnknownIsRefused(t *testing.T) {
	cfg := r2Config()
	cfg.StorageProvider = ""
	svc := NewFromConfig(cfg)
	if svc.Provider != ProviderS3 || svc.Endpoint != "http://localhost:9000" || svc.Region != "us-east-1" || svc.Bucket != "fb-media" || !svc.Configured() {
		t.Fatalf("default behaviour must stay MinIO/S3: %+v", svc)
	}
	cfg.StorageProvider = "gcs"
	if bad := NewFromConfig(cfg); bad.Configured() || !strings.Contains(strings.Join(bad.Problems(), ""), "unsupported STORAGE_PROVIDER") {
		t.Fatalf("unknown provider must be refused: %+v", bad.Problems())
	}
	var nilSvc *S3Service
	if nilSvc.Configured() {
		t.Fatal("nil service must be unconfigured")
	}
}

func TestR2RejectsBadPublicURLAndAccountID(t *testing.T) {
	cases := map[string]func(*config.Config){
		"scheme-less public url": func(c *config.Config) { c.R2PublicBaseURL = "media.example.com" },
		"http public url":        func(c *config.Config) { c.R2PublicBaseURL = "http://media.example.com" },
		"private api host":       func(c *config.Config) { c.R2PublicBaseURL = "https://0123.r2.cloudflarestorage.com/bucket" },
		"account id not hex":     func(c *config.Config) { c.R2AccountID = "foo/bar" },
		"account id is a url":    func(c *config.Config) { c.R2AccountID = "https://abc.r2.cloudflarestorage.com" },
	}
	for name, mutate := range cases {
		cfg := r2Config()
		mutate(cfg)
		svc := NewFromConfig(cfg)
		if svc.Configured() {
			t.Fatalf("%s must not count as configured", name)
		}
		if svc.Endpoint == "https://foo/bar.r2.cloudflarestorage.com" {
			t.Fatalf("%s: a malformed account id must never become a host", name)
		}
	}
	cfg := r2Config()
	cfg.R2PublicBaseURL = "https://pub-0123456789abcdef.r2.dev"
	if !NewFromConfig(cfg).Configured() {
		t.Fatal("an r2.dev public domain is valid")
	}
}

func TestUploadErrorsNeverContainThePresignedURL(t *testing.T) {
	svc := NewS3Service("http://127.0.0.1:1", "https://cdn.example.com", "auto", "media", "AKIDEXAMPLE", "very-secret-key")
	_, _, err := svc.Upload(t.Context(), "a.png", "image/png", strings.NewReader("x"), 1)
	if err == nil {
		t.Fatal("an unreachable endpoint must fail")
	}
	for _, leaked := range []string{"X-Amz-Signature", "X-Amz-Credential", "AKIDEXAMPLE", "very-secret-key"} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("upload error leaks %q: %v", leaked, err)
		}
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Fatalf("the host should stay visible for diagnosis: %v", err)
	}
}
