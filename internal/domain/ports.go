package domain

import (
	"context"
	"fmt"
	"sync"
)

// PublisherError trừu tượng hóa lỗi trả về từ bất kỳ Adapter mạng xã hội nào (Facebook, LinkedIn, X, TikTok,...),
// giúp Core Domain & Worker quyết định Retry hay vô hiệu hóa Token mà không cần biết chi tiết nền tảng.
type PublisherError interface {
	error
	ErrorCode() int
	Subcode() int
	IsRetryable() bool
	IsTokenInvalid() bool
	IsRateLimited() bool
}

// PublishStepOutcome ghi lại kết quả thực thi của một bước gọi API lên nền tảng đích.
type PublishStepOutcome struct {
	Action         string
	RequestSummary string
	HTTPStatus     int
	RawResponse    string
	DurationMs     int64
	Error          error
}

// PublishRequest chứa thông tin chuẩn hóa để Adapter thực hiện đăng bài.
type PublishRequest struct {
	Channel     Channel
	AccessToken string
	Post        *Post
}

// PublishResult chứa kết quả trả về sau khi Adapter xuất bản bài viết lên nền tảng đích.
type PublishResult struct {
	ExternalPostID string
	Steps          []PublishStepOutcome
	FinalRateLimit RateLimitUsage
}

// TokenHealthResult phản ánh trạng thái sống/chết của Credential trên nền tảng đích.
type TokenHealthResult struct {
	IsValid   bool
	ExpiresAt int64
	Scopes    []string
	RawInfo   any
}

// PublisherPort là Output Port (Contract) mà mọi Social Platform Adapter phải triển khai.
type PublisherPort interface {
	// Platform trả về định danh nền tảng mà Adapter này phục vụ (VD: FACEBOOK).
	Platform() PlatformType

	// Publish thực thi xuất bản bài viết lên Kênh đích.
	Publish(ctx context.Context, req PublishRequest) (*PublishResult, error)

	// VerifyToken kiểm tra tính hợp lệ của Access Token trên nền tảng đích.
	VerifyToken(ctx context.Context, accessToken string) (*TokenHealthResult, error)
}

// PublisherRegistry quản lý danh sách các PublisherPort theo từng nền tảng (Multi-platform Adapter Registry).
type PublisherRegistry struct {
	mu         sync.RWMutex
	publishers map[PlatformType]PublisherPort
}

// NewPublisherRegistry khởi tạo bộ đăng ký Adapter xuất bản.
func NewPublisherRegistry(adapters ...PublisherPort) *PublisherRegistry {
	r := &PublisherRegistry{
		publishers: make(map[PlatformType]PublisherPort),
	}
	for _, a := range adapters {
		if a != nil {
			r.Register(a)
		}
	}
	return r
}

// Register đăng ký một Adapter cho nền tảng tương ứng.
func (r *PublisherRegistry) Register(adapter PublisherPort) {
	if adapter == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publishers[NormalizePlatform(adapter.Platform())] = adapter
}

// Get trả về Adapter tương ứng với nền tảng của Channel.
func (r *PublisherRegistry) Get(platform PlatformType) (PublisherPort, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	norm := NormalizePlatform(platform)
	adapter, ok := r.publishers[norm]
	if !ok {
		return nil, fmt.Errorf("no publisher adapter registered for platform %s", norm)
	}
	return adapter, nil
}
