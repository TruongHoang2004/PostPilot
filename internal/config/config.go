package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config chứa toàn bộ cấu hình của hệ thống Facebook Auto-Poster.
type Config struct {
	AppEnv   string
	HTTPPort string
	LogLevel string

	// Database & Redis
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Security & Encryption
	TokenEncryptionKey string
	APIAdminKey        string

	// Meta / Facebook Graph API
	FBGraphVersion       string
	FBGraphBaseURL       string
	FBAppID              string
	FBAppSecret          string
	FBRedirectURI        string
	FBWebhookVerifyToken string

	// S3 / MinIO Media Storage
	S3Endpoint      string
	S3PublicBaseURL string
	S3Region        string
	S3Bucket        string
	S3AccessKey     string
	S3SecretKey     string

	// Worker & Scheduler
	SchedulerInterval          time.Duration
	TokenCheckInterval         time.Duration
	FastWorkerConcurrency      int
	HeavyWorkerConcurrency     int
	RateLimitThresholdPercent  int
	VideoChunkSizeBytes        int

	// AI (Gemini) & Alerting (Telegram)
	GeminiAPIKey     string
	GeminiModel      string
	TelegramBotToken string
	TelegramChatID   string
}

// Load nạp file .env (nếu có) và đọc các biến môi trường với giá trị mặc định an toàn.
func Load() *Config {
	loadDotEnv(".env")

	return &Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPPort: getEnv("HTTP_PORT", "8080"),
		LogLevel: getEnv("LOG_LEVEL", "info"),

		DatabaseURL:   getEnv("DATABASE_URL", "postgres://fbposter:fbposter_secret@localhost:5432/fb_autoposter?sslmode=disable"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		TokenEncryptionKey: getEnv("TOKEN_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
		APIAdminKey:        getEnv("API_ADMIN_KEY", "dev-admin-secret-key-change-me"),

		FBGraphVersion:       getEnv("FB_GRAPH_VERSION", "v20.0"),
		FBGraphBaseURL:       strings.TrimRight(getEnv("FB_GRAPH_BASE_URL", "https://graph.facebook.com"), "/"),
		FBAppID:              getEnv("FB_APP_ID", ""),
		FBAppSecret:          getEnv("FB_APP_SECRET", ""),
		FBRedirectURI:        getEnv("FB_REDIRECT_URI", "http://localhost:8080/api/v1/auth/facebook/callback"),
		FBWebhookVerifyToken: getEnv("FB_WEBHOOK_VERIFY_TOKEN", "my_secure_webhook_verify_token"),

		S3Endpoint:      strings.TrimRight(getEnv("S3_ENDPOINT", "http://localhost:9000"), "/"),
		S3PublicBaseURL: strings.TrimRight(getEnv("S3_PUBLIC_BASE_URL", "http://localhost:9000/fb-media"), "/"),
		S3Region:        getEnv("S3_REGION", "us-east-1"),
		S3Bucket:        getEnv("S3_BUCKET", "fb-media"),
		S3AccessKey:     getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:     getEnv("S3_SECRET_KEY", "minioadmin123"),

		SchedulerInterval:         time.Duration(getEnvInt("SCHEDULER_INTERVAL_SEC", 15)) * time.Second,
		TokenCheckInterval:        time.Duration(getEnvInt("TOKEN_CHECK_INTERVAL_HOURS", 12)) * time.Hour,
		FastWorkerConcurrency:     getEnvInt("FAST_WORKER_CONCURRENCY", 4),
		HeavyWorkerConcurrency:    getEnvInt("HEAVY_WORKER_CONCURRENCY", 2),
		RateLimitThresholdPercent: getEnvInt("RATE_LIMIT_THRESHOLD_PERCENT", 80),
		VideoChunkSizeBytes:       getEnvInt("VIDEO_CHUNK_SIZE_BYTES", 4*1024*1024),

		GeminiAPIKey:     getEnv("GEMINI_API_KEY", ""),
		GeminiModel:      getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),
	}
}

func loadDotEnv(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return fallback
}
