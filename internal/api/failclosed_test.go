package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/database"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// faultyStore fails selected reads to prove the API fails closed instead of open.
type faultyStore struct {
	database.Store
	failGroups   bool
	failChannels bool
}

func (f *faultyStore) ListChannelGroups(ctx context.Context, orgID string) ([]domain.ChannelGroup, error) {
	if f.failGroups {
		return nil, errors.New("groups unavailable")
	}
	return f.Store.ListChannelGroups(ctx, orgID)
}

func (f *faultyStore) ListChannels(ctx context.Context) ([]domain.Channel, error) {
	if f.failChannels {
		return nil, errors.New("channels unavailable")
	}
	return f.Store.ListChannels(ctx)
}

func TestApprovalLookupFailsClosedAndListingReportsStorageErrors(t *testing.T) {
	srv, _, base := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := base.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "faulty", Name: "Faulty", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	cg, _ := base.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "strict", RequireApproval: true})
	_ = base.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{ch.ID})
	when := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	approved, _ := base.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Content: "approved", Status: domain.PostStatusScheduled, ScheduledAt: when, ReviewedBy: domain.DefaultUserID, MediaType: domain.MediaTypeNone})

	faulty := &faultyStore{Store: base}
	srv.store = faulty
	srv.publishing = srv.publishing.WithStore(faulty)
	handler := srv.Handler()

	// Groups unreadable: an EDITOR content edit and an EDITOR creation must be refused (503), not persisted as unreviewed.
	faulty.failGroups = true
	rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/posts/"+approved.ID, `{"content":"sneaky"}`, "EDITOR")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("editor edit with unreadable approval rules must be 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if stored, _ := base.GetPostByID(ctx, approved.ID); stored.Content != "approved" || stored.Status != domain.PostStatusScheduled {
		t.Fatalf("post must be untouched after fail-closed refusal: %+v", stored)
	}
	rec = doJSON(t, srv, handler, http.MethodPost, "/api/v1/posts", fmt.Sprintf(`{"channel_id":"%s","content":"direct"}`, ch.ID), "EDITOR")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("editor create with unreadable approval rules must be 503, got %d: %s", rec.Code, rec.Body.String())
	}
	faulty.failGroups = false

	// Channel listing unreadable: a member listing must be a 500, never an empty 200 page.
	faulty.failChannels = true
	member, _ := base.CreateUser(ctx, &domain.User{Email: "faulty-member@postpilot.test", IsActive: true})
	_, _ = base.AddOrganizationMember(ctx, &domain.OrganizationMember{OrganizationID: domain.DefaultOrganizationID, UserID: member.ID, OrgRole: domain.OrgRoleMember})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?limit=10", nil)
	authenticateTestRole(t, srv, req, "MEMBER", member.ID)
	recList := httptest.NewRecorder()
	handler.ServeHTTP(recList, req)
	if recList.Code != http.StatusInternalServerError {
		t.Fatalf("member listing with unreadable channels must be 500, got %d: %s", recList.Code, recList.Body.String())
	}
}

func TestHasMoreIsExactAtTheMaximumPageSize(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "cap", Name: "Cap", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	base := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	posts := make([]*domain.Post, 0, domain.MaxPostQueryLimit+2)
	for i := 0; i < domain.MaxPostQueryLimit+2; i++ {
		posts = append(posts, &domain.Post{ChannelID: ch.ID, Content: "p", Status: domain.PostStatusScheduled, ScheduledAt: base.Add(time.Duration(i) * time.Minute), MediaType: domain.MediaTypeNone})
	}
	if _, err := store.CreatePostsBatch(ctx, nil, posts); err != nil {
		t.Fatal(err)
	}
	page := func(query string) (int, bool) {
		rec := doJSON(t, srv, handler, http.MethodGet, "/api/v1/posts?"+query, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Posts   []domain.Post `json:"posts"`
			HasMore bool          `json:"has_more"`
		}
		decode(t, rec, &resp)
		return len(resp.Posts), resp.HasMore
	}
	if n, more := page("limit=500"); n != 500 || !more {
		t.Fatalf("first capped page must be full with has_more=true, got %d %v", n, more)
	}
	if n, more := page("limit=500&offset=500"); n != 2 || more {
		t.Fatalf("second page must hold the remainder with has_more=false, got %d %v", n, more)
	}
	if n, more := page("limit=500&offset=502"); n != 0 || more {
		t.Fatalf("page beyond the end must be empty, got %d %v", n, more)
	}
}
