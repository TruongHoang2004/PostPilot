package domain

import "time"

// Enums cho trạng thái Token của Fanpage
const (
	TokenStatusActive      = "ACTIVE"
	TokenStatusExpired     = "EXPIRED"
	TokenStatusRevoked     = "REVOKED"
	TokenStatusRateLimited = "RATE_LIMITED"
)

// Enums cho loại Media của bài đăng
const (
	MediaTypeNone  = "NONE"  // Chỉ đăng chữ
	MediaTypeLink  = "LINK"  // Chữ + Link preview
	MediaTypeImage = "IMAGE" // 1 Ảnh đơn
	MediaTypeAlbum = "ALBUM" // Nhiều ảnh (Multi-photo post)
	MediaTypeVideo = "VIDEO" // Video thường (Chunked upload)
	MediaTypeReel  = "REEL"  // Facebook Reels (9:16 short video)
)

// Enums cho trạng thái bài đăng (Post Lifecycle)
const (
	PostStatusDraft      = "DRAFT"
	PostStatusScheduled  = "SCHEDULED"
	PostStatusQueued     = "QUEUED"
	PostStatusProcessing = "PROCESSING"
	PostStatusPublished  = "PUBLISHED"
	PostStatusFailed     = "FAILED"
	PostStatusCancelled  = "CANCELLED"
)

// Enums cho tầng hàng đợi (Queue Tier)
const (
	QueueTierFast  = "FAST"  // Dành cho NONE, LINK, IMAGE, ALBUM
	QueueTierHeavy = "HEAVY" // Dành cho VIDEO, REEL
)

// FacebookPage đại diện cho một Fanpage được kết nối vào hệ thống.
type FacebookPage struct {
	ID                   string     `json:"id"`
	PageID               string     `json:"page_id"`
	PageName             string     `json:"page_name"`
	Category             string     `json:"category"`
	EncryptedAccessToken string     `json:"-"` // Không bao giờ trả về qua JSON API
	MaskedToken          string     `json:"masked_token,omitempty"`
	TokenStatus          string     `json:"token_status"`
	TokenExpiresAt       *time.Time `json:"token_expires_at,omitempty"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	RateLimitCallCount   int        `json:"rate_limit_call_count"`
	RateLimitTotalTime   int        `json:"rate_limit_total_time"`
	RateLimitTotalCPU    int        `json:"rate_limit_total_cputime"`
	IsActive             bool       `json:"is_active"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// MaxRateLimitUsage trả về chỉ số % sử dụng API cao nhất trong 3 chỉ số của Meta Header X-Page-Usage.
func (p *FacebookPage) MaxRateLimitUsage() int {
	maxVal := p.RateLimitCallCount
	if p.RateLimitTotalTime > maxVal {
		maxVal = p.RateLimitTotalTime
	}
	if p.RateLimitTotalCPU > maxVal {
		maxVal = p.RateLimitTotalCPU
	}
	return maxVal
}

// MediaAsset đại diện cho file ảnh/video lưu trên S3 / MinIO / CDN.
type MediaAsset struct {
	ID              string    `json:"id"`
	FileName        string    `json:"file_name"`
	StorageKey      string    `json:"storage_key"`
	PublicURL       string    `json:"public_url"`
	MimeType        string    `json:"mime_type"`
	FileSizeBytes   int64     `json:"file_size_bytes"`
	DurationSeconds int       `json:"duration_seconds"`
	CreatedAt       time.Time `json:"created_at"`
}

// Post đại diện cho một bài viết được tạo hoặc lên lịch đăng lên Fanpage.
type Post struct {
	ID               string     `json:"id"`
	PageUUID         string     `json:"page_uuid"`
	PageName         string     `json:"page_name,omitempty"`
	FBPageID         string     `json:"fb_page_id,omitempty"`
	Content          string     `json:"content"`
	LinkURL          string     `json:"link_url,omitempty"`
	MediaType        string     `json:"media_type"`
	MediaURLs        []string   `json:"media_urls"`
	Status           string     `json:"status"`
	QueueTier        string     `json:"queue_tier"`
	ScheduledAt      time.Time  `json:"scheduled_at"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	FBPostID         string     `json:"fb_post_id,omitempty"`
	RetryCount       int        `json:"retry_count"`
	MaxRetries       int        `json:"max_retries"`
	NextRetryAt      *time.Time `json:"next_retry_at,omitempty"`
	LastErrorCode    int        `json:"last_error_code,omitempty"`
	LastErrorMessage string     `json:"last_error_message,omitempty"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// DetermineQueueTier tự động phân loại bài viết vào hàng đợi nhanh (FAST) hay nặng (HEAVY).
func DetermineQueueTier(mediaType string) string {
	switch mediaType {
	case MediaTypeVideo, MediaTypeReel:
		return QueueTierHeavy
	default:
		return QueueTierFast
	}
}

// PublishLog lưu lịch sử chi tiết từng request gọi sang Meta Graph API để phục vụ kiểm toán & debug.
type PublishLog struct {
	ID             string    `json:"id"`
	PostID         string    `json:"post_id"`
	PageUUID       string    `json:"page_uuid"`
	AttemptNumber  int       `json:"attempt_number"`
	Action         string    `json:"action"`
	HTTPStatus     int       `json:"http_status"`
	FBErrorCode    int       `json:"fb_error_code"`
	FBErrorSubcode int       `json:"fb_error_subcode"`
	RequestSummary string    `json:"request_summary"`
	ResponseBody   string    `json:"response_body"`
	DurationMs     int64     `json:"duration_ms"`
	CreatedAt      time.Time `json:"created_at"`
}

// RateLimitUsage đại diện cho dữ liệu parse từ HTTP Header `X-Page-Usage` và `X-App-Usage` của Meta.
type RateLimitUsage struct {
	CallCount    int `json:"call_count"`
	TotalTime    int `json:"total_time"`
	TotalCPUTime int `json:"total_cputime"`
}

// MaxPercent trả về mức % cao nhất trong 3 thông số giới hạn của Meta.
func (r RateLimitUsage) MaxPercent() int {
	m := r.CallCount
	if r.TotalTime > m {
		m = r.TotalTime
	}
	if r.TotalCPUTime > m {
		m = r.TotalCPUTime
	}
	return m
}

// PublishJobPayload là gói tin JSON được đẩy vào Redis Queue cho Worker xử lý.
type PublishJobPayload struct {
	PostID      string    `json:"post_id"`
	PageUUID    string    `json:"page_uuid"`
	QueueTier   string    `json:"queue_tier"`
	Attempt     int       `json:"attempt"`
	EnqueuedAt  time.Time `json:"enqueued_at"`
}

// SystemStats tổng hợp thống kê nhanh trên Dashboard.
type SystemStats struct {
	TotalPages       int `json:"total_pages"`
	ActivePages      int `json:"active_pages"`
	ScheduledPosts   int `json:"scheduled_posts"`
	QueuedPosts      int `json:"queued_posts"`
	PublishedPosts   int `json:"published_posts"`
	FailedPosts      int `json:"failed_posts"`
	FastQueueDepth   int `json:"fast_queue_depth"`
	HeavyQueueDepth  int `json:"heavy_queue_depth"`
}
