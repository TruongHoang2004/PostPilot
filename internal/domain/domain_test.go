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

func TestPostApprovalFSMTransitions(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	future := now.Add(2 * time.Hour)
	past := now.Add(-30 * time.Minute)

	// FSM 1: DRAFT -> PENDING_APPROVAL -> SCHEDULED (future) -> CANCELLED
	post1 := &Post{
		ID:          "fsm-post-1",
		ChannelID:   "ch-1",
		Content:     "FSM Flow 1",
		Status:      PostStatusDraft,
		ScheduledAt: future,
	}
	if post1.CanReview() {
		t.Fatalf("draft post must not be reviewable")
	}

	// Submit for approval -> PENDING_APPROVAL
	post1.Resubmit(future)
	if post1.Status != PostStatusPendingApproval {
		t.Fatalf("expected PENDING_APPROVAL, got %s", post1.Status)
	}
	if !post1.CanReview() {
		t.Fatalf("pending approval post must be reviewable")
	}

	// Approve future post -> SCHEDULED
	post1.Approve("reviewer-alice", now)
	if post1.Status != PostStatusScheduled {
		t.Fatalf("expected SCHEDULED, got %s", post1.Status)
	}
	if post1.ReviewedBy != "reviewer-alice" || post1.ReviewedAt == nil {
		t.Fatalf("expected reviewer metadata to be populated")
	}
	if post1.CanReview() {
		t.Fatalf("approved post must not be reviewable")
	}
	if !post1.CanCancel() {
		t.Fatalf("scheduled post must be cancellable")
	}

	// Cancel scheduled post -> CANCELLED
	post1.Status = PostStatusCancelled
	if post1.CanCancel() || !post1.IsTerminal() {
		t.Fatalf("cancelled post must be terminal and not cancellable")
	}

	// FSM 2: PENDING_APPROVAL -> REJECTED -> RESUBMIT -> QUEUED (immediate/past)
	post2 := &Post{
		ID:          "fsm-post-2",
		ChannelID:   "ch-1",
		Content:     "FSM Flow 2",
		Status:      PostStatusPendingApproval,
		ScheduledAt: future,
	}
	post2.Reject("reviewer-bob", "Tone is too casual", now)
	if post2.Status != PostStatusRejected {
		t.Fatalf("expected REJECTED, got %s", post2.Status)
	}
	if post2.RejectionNote != "Tone is too casual" {
		t.Fatalf("expected rejection note, got %s", post2.RejectionNote)
	}
	if !post2.CanCancel() {
		t.Fatalf("rejected post must be cancellable")
	}

	// Resubmit with immediate/past schedule
	post2.Resubmit(now)
	post2.ScheduledAt = past
	if post2.Status != PostStatusPendingApproval {
		t.Fatalf("expected PENDING_APPROVAL after resubmit, got %s", post2.Status)
	}
	if post2.RejectionNote != "" || post2.ReviewedBy != "" {
		t.Fatalf("expected cleared review fields upon resubmit")
	}

	// Approve immediate/past post -> QUEUED
	post2.Approve("reviewer-bob", now)
	if post2.Status != PostStatusQueued {
		t.Fatalf("expected QUEUED for past/immediate post, got %s", post2.Status)
	}

	// FSM 3: Terminal states (PUBLISHED and CANCELLED)
	terminalStatuses := []string{PostStatusPublished, PostStatusCancelled}
	for _, st := range terminalStatuses {
		termPost := &Post{Status: st}
		if !termPost.IsTerminal() {
			t.Fatalf("expected status %s to be terminal", st)
		}
		if termPost.CanReview() {
			t.Fatalf("terminal post %s must not be reviewable", st)
		}
		if termPost.CanCancel() {
			t.Fatalf("terminal post %s must not be cancellable", st)
		}
	}

	// Failed post is non-terminal and cancellable/retryable
	failedPost := &Post{Status: PostStatusFailed}
	if failedPost.IsTerminal() {
		t.Fatalf("failed post should not be terminal")
	}
	if !failedPost.CanCancel() {
		t.Fatalf("failed post should be cancellable")
	}
}

func TestRBACPermissionMatrixResolution(t *testing.T) {
	// Table-driven verification of Most Permissive Wins resolution
	cases := []struct {
		name            string
		roles           []GroupRole
		expectedRole    GroupRole
		canCreate       bool
		canPublishReq   bool // Can publish when approval required
		canPublishNoReq bool // Can publish when approval not required
		canApprove      bool
		canManage       bool
	}{
		{
			name:            "Viewer only",
			roles:           []GroupRole{GroupRoleViewer},
			expectedRole:    GroupRoleViewer,
			canCreate:       false,
			canPublishReq:   false,
			canPublishNoReq: false,
			canApprove:      false,
			canManage:       false,
		},
		{
			name:            "Viewer + Editor -> Editor wins",
			roles:           []GroupRole{GroupRoleViewer, GroupRoleEditor},
			expectedRole:    GroupRoleEditor,
			canCreate:       true,
			canPublishReq:   false, // Editor cannot publish directly if approval required
			canPublishNoReq: true,  // Editor CAN publish directly if approval NOT required
			canApprove:      false,
			canManage:       false,
		},
		{
			name:            "Editor + Approver -> Approver wins",
			roles:           []GroupRole{GroupRoleEditor, GroupRoleApprover},
			expectedRole:    GroupRoleApprover,
			canCreate:       true,
			canPublishReq:   true,
			canPublishNoReq: true,
			canApprove:      true,
			canManage:       false,
		},
		{
			name:            "Viewer + Approver + Manager -> Manager wins",
			roles:           []GroupRole{GroupRoleViewer, GroupRoleApprover, GroupRoleManager},
			expectedRole:    GroupRoleManager,
			canCreate:       true,
			canPublishReq:   true,
			canPublishNoReq: true,
			canApprove:      true,
			canManage:       true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved := ResolveChannelRole(tc.roles)
			if resolved != tc.expectedRole {
				t.Fatalf("expected resolved role %s, got %s", tc.expectedRole, resolved)
			}
			if CanCreatePost(resolved) != tc.canCreate {
				t.Fatalf("CanCreatePost mismatch: expected %v, got %v", tc.canCreate, CanCreatePost(resolved))
			}
			if CanPublishDirectly(resolved, true) != tc.canPublishReq {
				t.Fatalf("CanPublishDirectly(req=true) mismatch: expected %v, got %v", tc.canPublishReq, CanPublishDirectly(resolved, true))
			}
			if CanPublishDirectly(resolved, false) != tc.canPublishNoReq {
				t.Fatalf("CanPublishDirectly(req=false) mismatch: expected %v, got %v", tc.canPublishNoReq, CanPublishDirectly(resolved, false))
			}
			if CanApprovePost(resolved) != tc.canApprove {
				t.Fatalf("CanApprovePost mismatch: expected %v, got %v", tc.canApprove, CanApprovePost(resolved))
			}
			if CanManageChannel(resolved) != tc.canManage {
				t.Fatalf("CanManageChannel mismatch: expected %v, got %v", tc.canManage, CanManageChannel(resolved))
			}
		})
	}
}
