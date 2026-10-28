package domain

import "time"

// Post lifecycle status constants.
const (
	PostStatusDraft           = "DRAFT"
	PostStatusPendingApproval = "PENDING_APPROVAL"
	PostStatusScheduled       = "SCHEDULED"
	PostStatusQueued          = "QUEUED"
	PostStatusProcessing      = "PROCESSING"
	PostStatusPublished       = "PUBLISHED"
	PostStatusFailed          = "FAILED"
	PostStatusCancelled       = "CANCELLED"
	PostStatusRejected        = "REJECTED"
)

// Queue tier constants.
const (
	QueueTierFast  = "FAST"  // For NONE, LINK, IMAGE, ALBUM
	QueueTierHeavy = "HEAVY" // For VIDEO, REEL
)

// Post is the core Aggregate Root of the "Post & Publishing" bounded context.
type Post struct {
	ID                string       `json:"id"`
	OrganizationID    string       `json:"organization_id"`
	BatchID           string       `json:"batch_id,omitempty"`
	ChannelGroupID    string       `json:"channel_group_id,omitempty"`
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
	SubmittedBy       string       `json:"submitted_by,omitempty"`
	ReviewedBy        string       `json:"reviewed_by,omitempty"`
	ReviewedAt        *time.Time   `json:"reviewed_at,omitempty"`
	RejectionNote     string       `json:"rejection_note,omitempty"`
	RetryCount        int          `json:"retry_count"`
	MaxRetries        int          `json:"max_retries"`
	NextRetryAt       *time.Time   `json:"next_retry_at,omitempty"`
	LastErrorCode     int          `json:"last_error_code,omitempty"`
	LastErrorMessage  string       `json:"last_error_message,omitempty"`
	CreatedBy         string       `json:"created_by"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// DetermineQueueTier automatically routes the post to FAST or HEAVY queue based on media type.
func DetermineQueueTier(mediaType string) string {
	if IsHeavyMedia(mediaType) {
		return QueueTierHeavy
	}
	return QueueTierFast
}

// IsTerminal returns true if the post is in a final state (PUBLISHED or CANCELLED).
func (p *Post) IsTerminal() bool {
	return p.Status == PostStatusPublished || p.Status == PostStatusCancelled
}

// CanCancel checks business invariants to determine if a post can be cancelled.
func (p *Post) CanCancel() bool {
	return p.Status == PostStatusDraft ||
		p.Status == PostStatusScheduled ||
		p.Status == PostStatusQueued ||
		p.Status == PostStatusFailed ||
		p.Status == PostStatusPendingApproval ||
		p.Status == PostStatusRejected
}

// CanReview checks if a post is in a reviewable state (PENDING_APPROVAL).
func (p *Post) CanReview() bool {
	return p.Status == PostStatusPendingApproval
}

// Approve transitions a pending post to APPROVED and sets status to QUEUED (if due) or SCHEDULED.
func (p *Post) Approve(reviewerID string, now time.Time) {
	t := now.UTC()
	p.ReviewedBy = reviewerID
	p.ReviewedAt = &t
	p.RejectionNote = ""
	p.UpdatedAt = t
	if p.ScheduledAt.Before(t) || p.ScheduledAt.Equal(t) {
		p.Status = PostStatusQueued
	} else {
		p.Status = PostStatusScheduled
	}
}

// Reject transitions a pending post to REJECTED with a reviewer explanation note.
func (p *Post) Reject(reviewerID, note string, now time.Time) {
	t := now.UTC()
	p.Status = PostStatusRejected
	p.ReviewedBy = reviewerID
	p.RejectionNote = note
	p.ReviewedAt = &t
	p.UpdatedAt = t
}

// Resubmit resets a rejected or draft post back to PENDING_APPROVAL.
func (p *Post) Resubmit(now time.Time) {
	t := now.UTC()
	p.Status = PostStatusPendingApproval
	p.RejectionNote = ""
	p.ReviewedBy = ""
	p.ReviewedAt = nil
	p.UpdatedAt = t
}

// PrepareForCreation applies domain defaults when creating a new Post aggregate.
func (p *Post) PrepareForCreation(publishNow bool, now time.Time) {
	if p.OrganizationID == "" {
		p.OrganizationID = DefaultOrganizationID
	}
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

// MediaAttachments returns normalized MediaAttachment value objects.
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

// PublishLog records audit details for each external platform publishing step.
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

// PublishJobPayload is the JSON message enqueued in Redis for worker processing.
type PublishJobPayload struct {
	OrganizationID string    `json:"organization_id"`
	PostID         string    `json:"post_id"`
	ChannelID      string    `json:"channel_id"`
	QueueTier      string    `json:"queue_tier"`
	Attempt        int       `json:"attempt"`
	EnqueuedAt     time.Time `json:"enqueued_at"`
}

// SystemStats aggregates high-level dashboard metrics.
type SystemStats struct {
	TotalChannels        int `json:"total_channels"`
	ActiveChannels       int `json:"active_channels"`
	ScheduledPosts       int `json:"scheduled_posts"`
	QueuedPosts          int `json:"queued_posts"`
	PublishedPosts       int `json:"published_posts"`
	FailedPosts          int `json:"failed_posts"`
	PendingApprovalPosts int `json:"pending_approval_posts"`
	RejectedPosts        int `json:"rejected_posts"`
	FastQueueDepth       int `json:"fast_queue_depth"`
	HeavyQueueDepth      int `json:"heavy_queue_depth"`
}
