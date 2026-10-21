package facebook

import (
	"context"

	"github.com/happy-darwin/fb-autoposter/internal/domain"
)

// Ensure compile-time conformance to Core Domain Ports.
var (
	_ domain.PublisherPort  = (*PublisherAdapter)(nil)
	_ domain.PublisherError = (*GraphAPIError)(nil)
)

// PublisherAdapter đóng vai trò Secondary Adapter (Hexagonal Architecture),
// kết nối Core Domain "Post & Publishing" với Meta Graph API.
type PublisherAdapter struct {
	client *Client
}

// NewPublisherAdapter khởi tạo Facebook Adapter triển khai domain.PublisherPort.
func NewPublisherAdapter(client *Client) *PublisherAdapter {
	return &PublisherAdapter{client: client}
}

// Client trả về Graph API HTTP Client bên dưới (phục vụ các luồng đặc thù của Facebook như OAuth2).
func (a *PublisherAdapter) Client() *Client {
	return a.client
}

// Platform trả về định danh nền tảng FACEBOOK.
func (a *PublisherAdapter) Platform() domain.PlatformType {
	return domain.PlatformFacebook
}

// Publish chuyển đổi yêu cầu xuất bản trừu tượng từ Core Domain thành các lệnh gọi Meta Graph API.
func (a *PublisherAdapter) Publish(ctx context.Context, req domain.PublishRequest) (*domain.PublishResult, error) {
	outcome, err := a.client.PublishPost(ctx, req.Channel.ExternalID, req.AccessToken, req.Post)
	if outcome == nil {
		return nil, err
	}

	steps := make([]domain.PublishStepOutcome, 0, len(outcome.Steps))
	for _, s := range outcome.Steps {
		steps = append(steps, domain.PublishStepOutcome{
			Action:         s.Action,
			RequestSummary: s.RequestSummary,
			HTTPStatus:     s.Metrics.HTTPStatus,
			RawResponse:    s.Metrics.RawResponse,
			DurationMs:     s.Metrics.DurationMs,
			Error:          s.Error,
		})
	}

	return &domain.PublishResult{
		ExternalPostID: outcome.FBPostID,
		Steps:          steps,
		FinalRateLimit: outcome.FinalRateLimit,
	}, err
}

// VerifyToken kiểm tra sức khỏe Access Token của Kênh Facebook thông qua endpoint /debug_token.
func (a *PublisherAdapter) VerifyToken(ctx context.Context, accessToken string) (*domain.TokenHealthResult, error) {
	info, err := a.client.DebugToken(ctx, accessToken, "")
	if err != nil {
		return nil, err
	}
	return &domain.TokenHealthResult{
		IsValid:   info.IsValid,
		ExpiresAt: info.ExpiresAt,
		Scopes:    info.Scopes,
		RawInfo:   info,
	}, nil
}
