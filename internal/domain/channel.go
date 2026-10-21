package domain

import "time"

// PlatformType định danh nền tảng mạng xã hội của một Kênh xuất bản (Publishing Channel).
type PlatformType string

const (
	PlatformFacebook PlatformType = "FACEBOOK"
	PlatformLinkedIn PlatformType = "LINKEDIN"
	PlatformTwitter  PlatformType = "TWITTER"
	PlatformTikTok   PlatformType = "TIKTOK"
)

// NormalizePlatform chuẩn hóa tên nền tảng, mặc định là FACEBOOK nếu để trống.
func NormalizePlatform(p PlatformType) PlatformType {
	if p == "" {
		return PlatformFacebook
	}
	return p
}

// IsValidPlatform kiểm tra xem tên nền tảng có nằm trong danh mục hỗ trợ hay không.
func IsValidPlatform(p PlatformType) bool {
	switch p {
	case PlatformFacebook, PlatformLinkedIn, PlatformTwitter, PlatformTikTok:
		return true
	default:
		return false
	}
}

// Enums cho trạng thái Token / Credential của Kênh xuất bản
const (
	TokenStatusActive      = "ACTIVE"
	TokenStatusExpired     = "EXPIRED"
	TokenStatusRevoked     = "REVOKED"
	TokenStatusRateLimited = "RATE_LIMITED"
)

// RateLimitUsage đại diện cho mức tải trọng API hiện tại của Kênh xuất bản (0-100%).
type RateLimitUsage struct {
	CallCount    int `json:"call_count"`
	TotalTime    int `json:"total_time"`
	TotalCPUTime int `json:"total_cputime"`
}

// MaxPercent trả về mức % cao nhất trong các thông số giới hạn API.
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

// Channel là thực thể Domain đại diện cho một Đích xuất bản (Publishing Target / Social Account)
// độc lập với nền tảng mạng xã hội cụ thể (Facebook Page, LinkedIn Organization, X Account, TikTok Profile,...).
type Channel struct {
	ID                   string       `json:"id"`
	Platform             PlatformType `json:"platform"`
	ExternalID           string       `json:"external_id"`
	Name                 string       `json:"name"`
	Category             string       `json:"category"`
	EncryptedAccessToken string       `json:"-"` // Không bao giờ trả về qua JSON API
	MaskedToken          string       `json:"masked_token,omitempty"`
	TokenStatus          string       `json:"token_status"`
	TokenExpiresAt       *time.Time   `json:"token_expires_at,omitempty"`
	LastCheckedAt        *time.Time   `json:"last_checked_at,omitempty"`
	RateLimitCallCount   int          `json:"rate_limit_call_count"`
	RateLimitTotalTime   int          `json:"rate_limit_total_time"`
	RateLimitTotalCPU    int          `json:"rate_limit_total_cputime"`
	IsActive             bool         `json:"is_active"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

// RateLimitUsage trả về Value Object RateLimitUsage của Channel.
func (c *Channel) RateLimitUsage() RateLimitUsage {
	return RateLimitUsage{
		CallCount:    c.RateLimitCallCount,
		TotalTime:    c.RateLimitTotalTime,
		TotalCPUTime: c.RateLimitTotalCPU,
	}
}

// MaxRateLimitUsage trả về chỉ số % sử dụng API cao nhất của Channel.
func (c *Channel) MaxRateLimitUsage() int {
	return c.RateLimitUsage().MaxPercent()
}

// IsUsable kiểm tra xem Kênh có đang hoạt động và có Credential hợp lệ để đăng bài hay không.
func (c *Channel) IsUsable() bool {
	if !c.IsActive {
		return false
	}
	return c.TokenStatus != TokenStatusExpired && c.TokenStatus != TokenStatusRevoked
}
