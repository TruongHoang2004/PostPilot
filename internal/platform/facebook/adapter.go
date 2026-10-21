package facebook

import (
	"context"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// Ensure compile-time conformance to Core Domain Ports.
var (
	_ domain.PublisherPort  = (*PublisherAdapter)(nil)
	_ domain.PublisherError = (*GraphAPIError)(nil)
)

// PublisherAdapter acts as a Secondary Adapter (Hexagonal Architecture),
// connecting the Core Domain "Post & Publishing" to Meta Graph API.
type PublisherAdapter struct {
	client *Client
}

// NewPublisherAdapter initializes a Facebook Adapter implementing domain.PublisherPort.
func NewPublisherAdapter(client *Client) *PublisherAdapter {
	return &PublisherAdapter{client: client}
}

// Client returns the underlying Graph API HTTP Client.
func (a *PublisherAdapter) Client() *Client {
	return a.client
}

// Platform returns the FACEBOOK platform identifier.
func (a *PublisherAdapter) Platform() domain.PlatformType {
	return domain.PlatformFacebook
}

// Publish transforms generic publish requests from the Core Domain into Meta Graph API calls.
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

// VerifyToken verifies the validity and scopes of a Facebook Page Access Token via /debug_token.
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
