package domain

import (
	"context"
	"testing"
	"time"
)

type stubPublisher struct {
	platform PlatformType
}

func (s stubPublisher) Platform() PlatformType { return s.platform }
func (s stubPublisher) Publish(_ context.Context, _ PublishRequest) (*PublishResult, error) {
	return &PublishResult{ExternalPostID: "ext-123"}, nil
}
func (s stubPublisher) VerifyToken(_ context.Context, _ string) (*TokenHealthResult, error) {
	return &TokenHealthResult{IsValid: true}, nil
}

func TestChannelEntityInvariants(t *testing.T) {
	ch := Channel{
		ID:                 "uuid-1",
		Platform:           PlatformFacebook,
		ExternalID:         "fb-999",
		Name:               "Darwin Channel",
		Category:           "Tech",
		TokenStatus:        TokenStatusActive,
		RateLimitCallCount: 12,
		RateLimitTotalTime: 45,
		RateLimitTotalCPU:  30,
		IsActive:           true,
	}

	if !ch.IsUsable() {
		t.Fatalf("expected active channel to be usable")
	}
	if ch.MaxRateLimitUsage() != 45 {
		t.Fatalf("expected max rate limit 45, got %d", ch.MaxRateLimitUsage())
	}

	ch.TokenStatus = TokenStatusExpired
	if ch.IsUsable() {
		t.Fatalf("expired channel must not be usable")
	}
}

func TestPostDomainInvariants(t *testing.T) {
	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	post := &Post{
		ChannelID: "channel-1",
		Content:   "Hello DDD",
		LinkURL:   "https://example.com",
		MediaType: "",
	}
	post.PrepareForCreation(false, now)

	if post.MediaType != MediaTypeLink {
		t.Fatalf("expected inferred media type LINK, got %s", post.MediaType)
	}
	if post.QueueTier != QueueTierFast {
		t.Fatalf("expected queue tier FAST, got %s", post.QueueTier)
	}
	if post.Status != PostStatusScheduled {
		t.Fatalf("expected SCHEDULED status, got %s", post.Status)
	}
	if !post.CanCancel() {
		t.Fatalf("expected scheduled post to be cancellable")
	}

	post.Status = PostStatusPublished
	if post.CanCancel() || !post.IsTerminal() {
		t.Fatalf("published post must be terminal and not cancellable")
	}
}

func TestPublisherRegistry(t *testing.T) {
	reg := NewPublisherRegistry(stubPublisher{platform: PlatformFacebook})

	pub, err := reg.Get(PlatformFacebook)
	if err != nil || pub == nil {
		t.Fatalf("expected facebook publisher from registry, got err=%v", err)
	}

	pubDefault, err := reg.Get("")
	if err != nil || pubDefault == nil {
		t.Fatalf("expected default platform publisher, got err=%v", err)
	}

	if _, err := reg.Get(PlatformLinkedIn); err == nil {
		t.Fatalf("expected error for unregistered platform LINKEDIN")
	}
}
