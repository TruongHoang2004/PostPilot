package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/database"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
	"github.com/TruongHoang2004/PostPilot/internal/infra/redis"
	"github.com/TruongHoang2004/PostPilot/internal/platform/facebook"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// authenticateTestRole provisions real memberships/permissions and signs a JWT.
// It changes the test request, never the production middleware or its role resolution.
func authenticateTestRole(t *testing.T, s *Server, r *http.Request, role, userID string) {
	t.Helper()
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	if userID == "" && (role == "OWNER" || role == "ADMIN") {
		userID = domain.DefaultUserID
	}
	if userID == "" {
		email := "test-" + strings.ToLower(role) + "@postpilot.test"
		user, err := s.store.GetUserByEmail(ctx, email)
		if err != nil {
			user, err = s.store.CreateUser(ctx, &domain.User{Email: email, IsActive: true})
			if err != nil {
				t.Fatal(err)
			}
		}
		userID = user.ID
		_, err = s.store.AddOrganizationMember(ctx, &domain.OrganizationMember{OrganizationID: domain.DefaultOrganizationID, UserID: userID, OrgRole: domain.OrgRoleMember})
		if err != nil {
			t.Fatal(err)
		}
	}
	if role != "OWNER" && role != "ADMIN" && role != "MEMBER" {
		// A role's test group spans current channels; real API still resolves it through Store.
		groups, _ := s.store.ListChannelGroups(ctx, domain.DefaultOrganizationID)
		var group *domain.ChannelGroup
		for i := range groups {
			if groups[i].Name == "test-access-"+role {
				g := groups[i]
				group = &g
				break
			}
		}
		if group == nil {
			var err error
			group, err = s.store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "test-access-" + role})
			if err != nil {
				t.Fatal(err)
			}
		}
		channels, _ := s.store.ListChannels(ctx)
		var ids []string
		for _, ch := range channels {
			ids = append(ids, ch.ID)
		}
		if err := s.store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, group.ID, ids); err != nil {
			t.Fatal(err)
		}
		if _, err := s.store.SetChannelGroupPermission(ctx, &domain.ChannelGroupPermission{ChannelGroupID: group.ID, UserID: userID, GroupRole: domain.GroupRole(role)}); err != nil {
			t.Fatal(err)
		}
	}
	token, err := crypto.SignJWT(crypto.JWTClaims{UserID: userID, OrganizationID: domain.DefaultOrganizationID, OrgRole: role, Exp: time.Now().Add(time.Hour).Unix()}, s.cfg.APIAdminKey)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+token)
}
func serveTest(t *testing.T, s *Server, h http.Handler, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") == "" {
		authenticateTestRole(t, s, r, "OWNER", "")
	}
	h.ServeHTTP(w, r)
}
func TestUnauthenticatedAndForgedSimulationRejected(t *testing.T) {
	s, h, _ := setupTestServer(t)
	for _, path := range []string{"/api/v1/posts", "/api/v1/channels", "/api/v1/auth/me", "/api/v1/channel-groups", "/"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("X-Simulate-Role", "OWNER")
		req.Header.Set("X-User-Id", domain.DefaultUserID)
		req.Header.Set("X-Admin-Key", s.cfg.APIAdminKey)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Fatalf("anonymous %s got %d", path, rec.Code)
		}
	}
	// A valid viewer cannot elevate privileges by adding legacy headers.
	req := httptest.NewRequest("POST", "/api/v1/channel-groups", strings.NewReader(`{"name":"forged"}`))
	authenticateTestRole(t, s, req, "VIEWER", "")
	req.Header.Set("X-Simulate-Role", "OWNER")
	req.Header.Set("X-User-Id", domain.DefaultUserID)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("forged viewer got %d: %s", rec.Code, rec.Body.String())
	}
}
func TestRevokedMembershipRejectsExistingJWT(t *testing.T) {
	s, h, store := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/v1/posts", nil)
	authenticateTestRole(t, s, req, "VIEWER", "")
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	u, _ := store.GetUserByEmail(ctx, "test-viewer@postpilot.test")
	if err := store.RemoveOrganizationMember(ctx, domain.DefaultOrganizationID, u.ID); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("revoked membership got %d", rec.Code)
	}
}

type testGraphTransport struct{}

func (testGraphTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	token := r.URL.Query().Get("access_token")
	identities := map[string]string{"EAABsbCS1iHgBO1234567890": "100200300", "EAAB_fake_token_a": "fb-page-org-a-101", "tokenA": "fb-tenant-a-1"}
	var body any = map[string]any{"error": map[string]any{"message": "fixture endpoint unavailable", "code": 100}}
	code := 400
	if strings.HasSuffix(r.URL.Path, "/me/accounts") {
		body = map[string]any{"data": []any{}}
		code = 200
	}
	if strings.HasSuffix(r.URL.Path, "/me") {
		if id := identities[token]; id != "" {
			body = map[string]any{"id": id, "name": "Test Page"}
			code = 200
		}
	}
	if strings.HasSuffix(r.URL.Path, "/debug_token") {
		body = map[string]any{"data": map[string]any{"is_valid": true, "type": "PAGE"}}
		code = 200
	}
	raw, _ := json.Marshal(body)
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
}
func testFacebookClient(baseURL, version string) *facebook.Client {
	return facebook.NewClient(baseURL, version).WithHTTPClient(&http.Client{Transport: testGraphTransport{}})
}
func TestWrongPageTokenIsNotSaved(t *testing.T) {
	s, h, _ := setupTestServer(t)
	req := httptest.NewRequest("POST", "/api/v1/channels", strings.NewReader(`{"platform":"FACEBOOK","external_id":"wrong-page","name":"Wrong","access_token":"tokenA"}`))
	authenticateTestRole(t, s, req, "OWNER", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("wrong Page token got %d", rec.Code)
	}
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	channels, _ := s.store.ListChannels(ctx)
	if len(channels) != 0 {
		t.Fatal("invalid connection persisted")
	}
}

func TestLegacyWorkspaceClaimRequiresOperatorAndOnlyRunsOnce(t *testing.T) {
	s, h, store := setupTestServer(t)
	s.cfg.APIAdminKey = strings.Repeat("a", 64)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"email":"owner@real.test","password":"VeryLongPassword123","full_name":"Owner"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatal(rec.Body.String())
	}
	var account struct {
		Token string      `json:"token"`
		User  domain.User `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &account); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		key    string
		status int
	}{{"wrong", 403}, {s.cfg.APIAdminKey, 200}, {s.cfg.APIAdminKey, 409}} {
		request := httptest.NewRequest("POST", "/api/v1/auth/claim-legacy-workspace", nil)
		request.Header.Set("Authorization", "Bearer "+account.Token)
		request.Header.Set("X-Admin-Key", step.key)
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != step.status {
			t.Fatalf("claim got %d want %d: %s", response.Code, step.status, response.Body.String())
		}
	}
	members, _ := store.ListOrganizationMembers(t.Context(), domain.DefaultOrganizationID)
	if len(members) != 1 || members[0].UserID != account.User.ID {
		t.Fatal("legacy owner not transferred exactly once")
	}
}

func TestHealthRejectsUnavailableRedis(t *testing.T) {
	s, _, _ := setupTestServer(t)
	s.redis = redis.NewClient("invalid-address", "", 0)
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), `"status":"unhealthy"`) {
		t.Fatalf("unavailable queue reported healthy: %d %s", rec.Code, rec.Body.String())
	}
	if s.redis.IsMemoryFallback() {
		t.Fatal("real client silently fell back to memory")
	}
}

func TestViewerCannotCancelOrReviewRestrictedBatch(t *testing.T) {
	s, h, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := store.UpsertChannel(ctx, &domain.Channel{ExternalID: "cancel-test", Name: "Page"})
	post, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Status: domain.PostStatusScheduled})
	req := httptest.NewRequest("POST", "/api/v1/posts/"+post.ID+"/cancel", nil)
	authenticateTestRole(t, s, req, "VIEWER", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("viewer cancel got %d", rec.Code)
	}
	other, _ := store.UpsertChannel(ctx, &domain.Channel{ExternalID: "other", Name: "Other"})
	approver, _ := store.CreateUser(ctx, &domain.User{Email: "limited@real.test", IsActive: true})
	_, _ = store.AddOrganizationMember(ctx, &domain.OrganizationMember{OrganizationID: domain.DefaultOrganizationID, UserID: approver.ID, OrgRole: domain.OrgRoleMember})
	group, _ := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "Allowed"})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, group.ID, []string{ch.ID})
	_, _ = store.SetChannelGroupPermission(ctx, &domain.ChannelGroupPermission{ChannelGroupID: group.ID, UserID: approver.ID, GroupRole: domain.GroupRoleApprover})
	one, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, BatchID: "batch-permissions", Status: domain.PostStatusPendingApproval})
	two, _ := store.CreatePost(ctx, &domain.Post{ChannelID: other.ID, BatchID: "batch-permissions", Status: domain.PostStatusPendingApproval})
	request := httptest.NewRequest("POST", "/api/v1/posts/"+one.ID+"/review", strings.NewReader(`{"action":"APPROVED","apply_to_batch":true}`))
	authenticateTestRole(t, s, request, "MEMBER", approver.ID)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatalf("restricted batch got %d", response.Code)
	}
	for _, id := range []string{one.ID, two.ID} {
		p, _ := store.GetPostByID(ctx, id)
		if p.Status != domain.PostStatusPendingApproval {
			t.Fatal("unauthorized batch partially mutated")
		}
	}
}
func TestApprovalQueueOutageIsRecoverable(t *testing.T) {
	s, h, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := store.UpsertChannel(ctx, &domain.Channel{ExternalID: "page", Name: "Page"})
	post, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, Status: domain.PostStatusPendingApproval, ScheduledAt: time.Now().Add(-time.Minute)})
	s.redis = redis.NewClient("invalid-address", "", 0)
	req := httptest.NewRequest("POST", "/api/v1/posts/"+post.ID+"/review", strings.NewReader(`{"action":"APPROVED"}`))
	authenticateTestRole(t, s, req, "OWNER", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 503 {
		t.Fatalf("queue outage returned %d", rec.Code)
	}
	current, _ := store.GetPostByID(ctx, post.ID)
	if current.Status != domain.PostStatusFailed || current.NextRetryAt == nil {
		t.Fatalf("post not recoverable: %+v", current)
	}
}
func TestOAuthRequiresBrowserBindingAndSingleUse(t *testing.T) {
	s, h, store := setupTestServer(t)
	s.cfg.FBAppID = "app"
	s.cfg.FrontendURL = "https://postpilot.example.com"
	s.cfg.FBRedirectURI = s.cfg.FrontendURL + "/api/v1/auth/facebook/callback"
	req := httptest.NewRequest("GET", "/api/v1/auth/facebook/login", nil)
	authenticateTestRole(t, s, req, "OWNER", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var data map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &data)
	login, _ := url.Parse(data["oauth_login_url"])
	state := login.Query().Get("state")
	callback := httptest.NewRequest("GET", "/api/v1/auth/facebook/callback?code=test&state="+url.QueryEscape(state), nil)
	rejected := httptest.NewRecorder()
	h.ServeHTTP(rejected, callback)
	if rejected.Code != 403 {
		t.Fatal("accepted unbound OAuth callback")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatal("OAuth browser cookie missing protections")
	}
	// Consume challenge without contacting Meta; callback replay with correct cookie still fails.
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	if err := store.ConsumeOAuthState(ctx, oauthStateHash(state)); err != nil {
		t.Fatal(err)
	}
	callback.AddCookie(cookies[0])
	replayed := httptest.NewRecorder()
	h.ServeHTTP(replayed, callback)
	if replayed.Code != 403 {
		t.Fatalf("consumed state accepted: %d", replayed.Code)
	}
}
func TestRegistrationWithSameDisplayNameCreatesDistinctWorkspaces(t *testing.T) {
	_, h, _ := setupTestServer(t)
	slugs := map[string]bool{}
	for _, email := range []string{"one@real.test", "two@real.test"} {
		req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"email":"`+email+`","password":"LongRealPassword123","full_name":"Same Name"}`))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 201 {
			t.Fatal(rec.Body.String())
		}
		var result struct {
			Organization domain.Organization `json:"organization"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		if slugs[result.Organization.Slug] {
			t.Fatal("workspace slug collision")
		}
		slugs[result.Organization.Slug] = true
	}
}

type reviewConflictStore struct {
	database.Store
	failID string
}

func (s reviewConflictStore) ReviewPost(ctx context.Context, id, reviewer string, action domain.ApprovalAction, note string) (*domain.Post, error) {
	if id == s.failID {
		return nil, fmt.Errorf("concurrent review conflict")
	}
	return s.Store.ReviewPost(ctx, id, reviewer, action, note)
}
func TestBatchConflictStillDispatchesCommittedApproval(t *testing.T) {
	s, _, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := store.UpsertChannel(ctx, &domain.Channel{ExternalID: "batch-conflict", Name: "Page"})
	one, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, BatchID: "batch", Status: domain.PostStatusPendingApproval, ScheduledAt: time.Now().Add(-time.Minute)})
	two, _ := store.CreatePost(ctx, &domain.Post{ChannelID: ch.ID, BatchID: "batch", Status: domain.PostStatusPendingApproval, ScheduledAt: time.Now().Add(-time.Minute)})
	s.store = reviewConflictStore{Store: store, failID: two.ID}
	req := httptest.NewRequest("POST", "/api/v1/posts/"+one.ID+"/review", strings.NewReader(`{"action":"APPROVED","apply_to_batch":true}`))
	authenticateTestRole(t, s, req, "OWNER", "")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 409 {
		t.Fatalf("expected conflict, got %d %s", rec.Code, rec.Body.String())
	}
	job, err := s.redis.DequeuePublishJob(ctx, domain.QueueTierFast, 1)
	if err != nil || job.PostID != one.ID || job.OrganizationID != domain.DefaultOrganizationID {
		t.Fatalf("committed approval stranded: %+v %v", job, err)
	}
	current, _ := store.GetPostByID(ctx, two.ID)
	if current.Status != domain.PostStatusPendingApproval {
		t.Fatal("conflicting post mutated")
	}
}
