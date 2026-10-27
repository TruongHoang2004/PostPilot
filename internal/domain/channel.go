package domain

import "time"

// PlatformType identifies the social platform of a publishing Channel.
type PlatformType string

const (
	PlatformFacebook PlatformType = "FACEBOOK"
	PlatformLinkedIn PlatformType = "LINKEDIN"
	PlatformTwitter  PlatformType = "TWITTER"
	PlatformTikTok   PlatformType = "TIKTOK"
)

// NormalizePlatform normalizes the platform identifier, defaulting to FACEBOOK if empty.
func NormalizePlatform(p PlatformType) PlatformType {
	if p == "" {
		return PlatformFacebook
	}
	return p
}

// IsValidPlatform verifies if the platform identifier is within supported platforms.
func IsValidPlatform(p PlatformType) bool {
	switch p {
	case PlatformFacebook, PlatformLinkedIn, PlatformTwitter, PlatformTikTok:
		return true
	default:
		return false
	}
}

// Channel token and credential status constants.
const (
	TokenStatusActive      = "ACTIVE"
	TokenStatusExpired     = "EXPIRED"
	TokenStatusRevoked     = "REVOKED"
	TokenStatusRateLimited = "RATE_LIMITED"
)

// RateLimitUsage represents current API usage levels of a channel (0-100%).
type RateLimitUsage struct {
	CallCount    int `json:"call_count"`
	TotalTime    int `json:"total_time"`
	TotalCPUTime int `json:"total_cputime"`
}

// MaxPercent returns the highest usage percentage across all rate limit metrics.
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

// Channel is the Domain entity representing a publishing target (social media account)
// agnostic of the underlying platform (Facebook Page, LinkedIn Org, X Account, TikTok Profile, etc.).
type Channel struct {
	ID                   string       `json:"id"`
	OrganizationID       string       `json:"organization_id"`
	Platform             PlatformType `json:"platform"`
	ExternalID           string       `json:"external_id"`
	Name                 string       `json:"name"`
	Category             string       `json:"category"`
	EncryptedAccessToken string       `json:"-"` // Never serialized in API responses
	MaskedToken          string       `json:"masked_token,omitempty"`
	TokenStatus          string       `json:"token_status"`
	TokenExpiresAt       *time.Time   `json:"token_expires_at,omitempty"`
	LastCheckedAt        *time.Time   `json:"last_checked_at,omitempty"`
	RateLimitCallCount   int          `json:"rate_limit_call_count"`
	RateLimitTotalTime   int          `json:"rate_limit_total_time"`
	RateLimitTotalCPU    int          `json:"rate_limit_total_cputime"`
	GroupIDs             []string     `json:"group_ids,omitempty"`
	GroupNames           []string     `json:"group_names,omitempty"`
	IsActive             bool         `json:"is_active"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

// RateLimitUsage returns the RateLimitUsage value object for the Channel.
func (c *Channel) RateLimitUsage() RateLimitUsage {
	return RateLimitUsage{
		CallCount:    c.RateLimitCallCount,
		TotalTime:    c.RateLimitTotalTime,
		TotalCPUTime: c.RateLimitTotalCPU,
	}
}

// MaxRateLimitUsage returns the maximum API usage percentage for the Channel.
func (c *Channel) MaxRateLimitUsage() int {
	return c.RateLimitUsage().MaxPercent()
}

// IsUsable checks if the channel is active and has valid credentials for publishing.
func (c *Channel) IsUsable() bool {
	if !c.IsActive {
		return false
	}
	return c.TokenStatus != TokenStatusExpired && c.TokenStatus != TokenStatusRevoked
}
