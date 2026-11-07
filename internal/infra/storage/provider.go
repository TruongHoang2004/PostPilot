package storage

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/TruongHoang2004/PostPilot/internal/config"
)

// Supported object-storage providers (selected with STORAGE_PROVIDER).
const (
	ProviderS3 = "s3" // MinIO (default), AWS S3 or any S3-compatible endpoint configured with S3_*
	ProviderR2 = "r2" // Cloudflare R2 configured with R2_*
)

// R2Region is the signing region Cloudflare R2 expects ("auto").
const R2Region = "auto"

var r2AccountIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// R2Endpoint returns the S3-compatible API endpoint of a Cloudflare account.
func R2Endpoint(accountID string) string {
	return "https://" + strings.TrimSpace(accountID) + ".r2.cloudflarestorage.com"
}

// NewFromConfig builds the media storage service for the selected provider. It never connects and
// never fails: an incomplete configuration yields a service whose Configured() is false, so the API
// keeps starting and the upload endpoints answer 503 until the operator finishes the setup.
func NewFromConfig(cfg *config.Config) *S3Service {
	provider := strings.ToLower(strings.TrimSpace(cfg.StorageProvider))
	switch provider {
	case ProviderR2:
		endpoint := strings.TrimRight(strings.TrimSpace(cfg.R2Endpoint), "/")
		var extra []string
		if endpoint == "" && strings.TrimSpace(cfg.R2AccountID) != "" {
			if r2AccountIDPattern.MatchString(strings.TrimSpace(cfg.R2AccountID)) {
				endpoint = R2Endpoint(cfg.R2AccountID)
			} else {
				extra = append(extra, "R2_ACCOUNT_ID must be the 32-character hex Cloudflare account id")
			}
		}
		svc := NewS3Service(endpoint, cfg.R2PublicBaseURL, R2Region, cfg.R2Bucket, cfg.R2AccessKeyID, cfg.R2SecretAccessKey)
		svc.Provider = ProviderR2
		svc.extraProblems = extra
		return svc
	case "", ProviderS3:
		svc := NewS3Service(cfg.S3Endpoint, cfg.S3PublicBaseURL, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey)
		svc.Provider = ProviderS3
		return svc
	default:
		// Unknown provider: refuse to guess; report the problem through Problems().
		svc := NewS3Service("", "", "", "", "", "")
		svc.Provider = provider
		return svc
	}
}

// Configured reports whether uploads can work: every setting needed to sign and publish a URL is present.
func (s *S3Service) Configured() bool {
	return s != nil && len(s.Problems()) == 0
}

// Problems lists what is missing or unsafe in the configuration, using the environment variable names
// of the active provider. It never includes secret values.
func (s *S3Service) Problems() []string {
	if s == nil {
		return []string{"object storage is not initialised"}
	}
	var problems []string
	names := map[string]string{"endpoint": "S3_ENDPOINT", "bucket": "S3_BUCKET", "access": "S3_ACCESS_KEY", "secret": "S3_SECRET_KEY", "public": "S3_PUBLIC_BASE_URL"}
	switch s.Provider {
	case ProviderR2:
		names = map[string]string{"endpoint": "R2_ACCOUNT_ID (or R2_ENDPOINT)", "bucket": "R2_BUCKET", "access": "R2_ACCESS_KEY_ID", "secret": "R2_SECRET_ACCESS_KEY", "public": "R2_PUBLIC_BASE_URL"}
	case "", ProviderS3:
	default:
		return []string{"unsupported STORAGE_PROVIDER \"" + s.Provider + "\" (use s3 or r2)"}
	}
	for key, value := range map[string]string{"endpoint": s.Endpoint, "bucket": s.Bucket, "access": s.AccessKey, "secret": s.SecretKey, "public": s.PublicBaseURL} {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, names[key]+" is not set")
		}
	}
	problems = append(problems, s.extraProblems...)
	if s.Provider == ProviderR2 && strings.TrimSpace(s.PublicBaseURL) != "" {
		// Meta downloads media from this URL, so it must be an absolute https URL on a public domain,
		// never the private S3 API host that signs uploads.
		pub, err := url.Parse(s.PublicBaseURL)
		switch {
		case err != nil || pub.Host == "":
			problems = append(problems, "R2_PUBLIC_BASE_URL must be an absolute URL (https://...)")
		case pub.Scheme != "https" && pub.Hostname() != "127.0.0.1" && pub.Hostname() != "localhost":
			problems = append(problems, "R2_PUBLIC_BASE_URL must use https")
		case strings.HasSuffix(strings.ToLower(pub.Hostname()), ".r2.cloudflarestorage.com"):
			problems = append(problems, "R2_PUBLIC_BASE_URL must be the public bucket domain (r2.dev or custom), not the private S3 API host")
		}
	}
	if s.Provider == ProviderR2 && s.Endpoint != "" {
		// Presigned URLs carry the signature in the query string, so R2 must be reached over TLS
		// (plain HTTP is accepted only for loopback test servers).
		if u, err := url.Parse(s.Endpoint); err != nil || u.Host == "" {
			problems = append(problems, "R2 endpoint is not a valid URL")
		} else if u.Scheme != "https" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" {
			problems = append(problems, "R2 endpoint must use https")
		}
	}
	return problems
}
