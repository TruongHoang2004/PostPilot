package domain

import (
	"context"
	"fmt"
	"sync"
)

// PublisherError abstracts errors returned by any social platform adapter (Facebook, LinkedIn, X, TikTok,...),
// enabling Core Domain & Worker to decide retry strategies or token invalidation without platform coupling.
type PublisherError interface {
	error
	ErrorCode() int
	Subcode() int
	IsRetryable() bool
	IsTokenInvalid() bool
	IsRateLimited() bool
}

// PublishStepOutcome records execution metrics and results of an individual API call to the target platform.
type PublishStepOutcome struct {
	Action         string
	RequestSummary string
	HTTPStatus     int
	RawResponse    string
	DurationMs     int64
	Error          error
}

// PublishRequest contains standardized parameters for an adapter to execute publishing.
type PublishRequest struct {
	Channel     Channel
	AccessToken string
	Post        *Post
}

// PublishResult contains the outcome after an adapter publishes a post to the target platform.
type PublishResult struct {
	ExternalPostID string
	Steps          []PublishStepOutcome
	FinalRateLimit RateLimitUsage
}

// TokenHealthResult reflects credential validity and expiration on the target platform.
type TokenHealthResult struct {
	IsValid   bool
	ExpiresAt int64
	Scopes    []string
	RawInfo   any
}

// PublisherPort is the Outbound Port contract implemented by each social platform adapter.
type PublisherPort interface {
	// Platform returns the platform identifier serviced by this adapter (e.g. FACEBOOK).
	Platform() PlatformType

	// Publish executes post publishing to the target channel.
	Publish(ctx context.Context, req PublishRequest) (*PublishResult, error)

	// VerifyToken checks the validity and scopes of the access token on the target platform.
	VerifyToken(ctx context.Context, accessToken string) (*TokenHealthResult, error)
}

// PublisherRegistry manages multi-platform publishing adapters.
type PublisherRegistry struct {
	mu         sync.RWMutex
	publishers map[PlatformType]PublisherPort
}

// NewPublisherRegistry initializes an adapter registry.
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

// Register registers an adapter for its supported platform.
func (r *PublisherRegistry) Register(adapter PublisherPort) {
	if adapter == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publishers[NormalizePlatform(adapter.Platform())] = adapter
}

// Get returns the adapter registered for the given platform.
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
