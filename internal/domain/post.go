package domain

import "time"

// Enums cho trạng thái vòng đời bài đăng (Post Lifecycle)
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

// Post là Aggregate Root cốt lõi của Bounded Context "Post & Publishing".
type Post struct {
	ID                string       `json:"id"`
	ChannelID         string       `json:"channel_id"`
	ChannelName       string       `json:"channel_name,omitempty"`
	Platform          PlatformType `json:"platform,omitempty"`
	ExternalChannelID string       `json:"external_channel_id,omitempty"`
	Content           string       `json:"content"`
	LinkURL           string       `json:"link_url,omitempty"`
	MediaType         string       `json:"media_type"`
	MediaURLs         []string     `json:"media_urls"`
	Status            string       `json:"status"`
	QueueTier         string       `json:"queue_tier"`
	ScheduledAt       time.Time    `json:"scheduled_at"`
	PublishedAt       *time.Time   `json:"published_at,omitempty"`
	ExternalPostID    string       `json:"external_post_id,omitempty"`
	RetryCount        int          `json:"retry_count"`
	MaxRetries        int          `json:"max_retries"`
	NextRetryAt       *time.Time   `json:"next_retry_at,omitempty"`
	LastErrorCode     int          `json:"last_error_code,omitempty"`
	LastErrorMessage  string       `json:"last_error_message,omitempty"`
	CreatedBy         string       `json:"created_by"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// DetermineQueueTier tự động phân loại bài viết vào hàng đợi nhanh (FAST) hay nặng (HEAVY).
func DetermineQueueTier(mediaType string) string {
	if IsHeavyMedia(mediaType) {
		return QueueTierHeavy
	}
	return QueueTierFast
}

// IsTerminal trả về true nếu bài viết đã ở trạng thái kết thúc (PUBLISHED hoặc CANCELLED).
func (p *Post) IsTerminal() bool {
	return p.Status == PostStatusPublished || p.Status == PostStatusCancelled
}

// CanCancel kiểm tra quy tắc nghiệp vụ xem bài viết có được phép hủy lịch hay không.
func (p *Post) CanCancel() bool {
	return p.Status == PostStatusDraft || p.Status == PostStatusScheduled || p.Status == PostStatusFailed
}

// PrepareForCreation áp dụng các quy tắc khởi tạo mặc định cho Aggregate Root Post.
func (p *Post) PrepareForCreation(publishNow bool, now time.Time) {
	p.MediaType = NormalizeMediaType(p.MediaType, p.LinkURL)
	p.QueueTier = DetermineQueueTier(p.MediaType)
	if p.ScheduledAt.IsZero() || publishNow {
		p.ScheduledAt = now.UTC()
	} else {
		p.ScheduledAt = p.ScheduledAt.UTC()
	}
	if publishNow {
		p.Status = PostStatusQueued
	} else if p.Status == "" {
		p.Status = PostStatusScheduled
	}
	if p.MaxRetries <= 0 {
		p.MaxRetries = 3
	}
	if p.CreatedBy == "" {
		p.CreatedBy = "admin"
	}
}

// MediaAttachments trả về danh sách các tệp đính kèm dưới dạng Value Objects chuẩn hóa.
func (p *Post) MediaAttachments() []MediaAttachment {
	items := make([]MediaAttachment, 0, len(p.MediaURLs))
	for i, u := range p.MediaURLs {
		items = append(items, MediaAttachment{
			URL:      u,
			Format:   p.MediaType,
			Position: i + 1,
		})
	}
	return items
}

// PublishLog lưu lịch sử chi tiết từng bước gọi sang nền tảng xuất bản để phục vụ kiểm toán & debug.
type PublishLog struct {
	ID             string    `json:"id"`
	PostID         string    `json:"post_id"`
	ChannelID      string    `json:"channel_id"`
	AttemptNumber  int       `json:"attempt_number"`
	Action         string    `json:"action"`
	HTTPStatus     int       `json:"http_status"`
	ErrorCode      int       `json:"error_code"`
	ErrorSubcode   int       `json:"error_subcode"`
	RequestSummary string    `json:"request_summary"`
	ResponseBody   string    `json:"response_body"`
	DurationMs     int64     `json:"duration_ms"`
	CreatedAt      time.Time `json:"created_at"`
}

// PublishJobPayload là gói tin JSON được đẩy vào Redis Queue cho Worker xử lý.
type PublishJobPayload struct {
	PostID     string    `json:"post_id"`
	ChannelID  string    `json:"channel_id"`
	QueueTier  string    `json:"queue_tier"`
	Attempt    int       `json:"attempt"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

// SystemStats tổng hợp thống kê nhanh trên Dashboard.
type SystemStats struct {
	TotalChannels   int `json:"total_channels"`
	ActiveChannels  int `json:"active_channels"`
	ScheduledPosts  int `json:"scheduled_posts"`
	QueuedPosts     int `json:"queued_posts"`
	PublishedPosts  int `json:"published_posts"`
	FailedPosts     int `json:"failed_posts"`
	FastQueueDepth  int `json:"fast_queue_depth"`
	HeavyQueueDepth int `json:"heavy_queue_depth"`
}
