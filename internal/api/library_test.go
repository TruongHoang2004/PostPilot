package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/database"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

type libraryListResponse struct {
	Items   []domain.LibraryItem `json:"items"`
	HasMore bool                 `json:"has_more"`
}

func listLibrary(t *testing.T, srv *Server, h http.Handler, query, role string) libraryListResponse {
	t.Helper()
	rec := doJSON(t, srv, h, http.MethodGet, "/api/v1/library"+query, "", role)
	if rec.Code != http.StatusOK {
		t.Fatalf("library list expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp libraryListResponse
	decode(t, rec, &resp)
	if resp.Items == nil {
		t.Fatalf("items must be an array, got null: %s", rec.Body.String())
	}
	return resp
}

func createLibraryItem(t *testing.T, srv *Server, h http.Handler, body, role string) domain.LibraryItem {
	t.Helper()
	rec := doJSON(t, srv, h, http.MethodPost, "/api/v1/library", body, role)
	if rec.Code != http.StatusCreated {
		t.Fatalf("library create expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var item domain.LibraryItem
	decode(t, rec, &item)
	return item
}

func TestLibraryLifecycleScheduleAndSoftDelete(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "lib-ch", Name: "Thư viện", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})

	bound := createLibraryItem(t, srv, handler, fmt.Sprintf(`{"title":"Khuyến mãi","channel_id":"%s","content":"Giảm 20%% cuối tuần","media_type":"NONE"}`, ch.ID), "")
	if bound.ChannelName != "Thư viện" || bound.PublishCount != 0 || bound.MediaURLs == nil {
		t.Fatalf("created item wrong: %+v", bound)
	}
	createLibraryItem(t, srv, handler, `{"title":"Chung","content":"evergreen","media_type":"LINK","link_url":"https://example.com/a"}`, "")

	for name, body := range map[string]string{
		"empty":         `{"title":"x"}`,
		"image no url":  `{"content":"x","media_type":"IMAGE"}`,
		"unknown page":  `{"content":"x","channel_id":"nope"}`,
		"title too big": fmt.Sprintf(`{"content":"x","title":"%s"}`, string(bytes.Repeat([]byte("a"), 300))),
		"not json":      `{`,
	} {
		if rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/library", body, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", name, rec.Code, rec.Body.String())
		}
	}
	if got := listLibrary(t, srv, handler, "", ""); len(got.Items) != 2 {
		t.Fatalf("expected 2 active items, got %d", len(got.Items))
	}
	if got := listLibrary(t, srv, handler, "?channel_id="+ch.ID, ""); len(got.Items) != 1 || got.Items[0].ID != bound.ID {
		t.Fatalf("channel filter wrong: %+v", got.Items)
	}
	if got := listLibrary(t, srv, handler, "?q=giảm", ""); len(got.Items) != 1 {
		t.Fatalf("search wrong: %+v", got.Items)
	}
	if got := listLibrary(t, srv, handler, "?limit=1", ""); len(got.Items) != 1 || !got.HasMore {
		t.Fatalf("paging must report has_more: %+v", got)
	}

	// Edit
	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/library/"+bound.ID, `{"content":"Giảm 30% cuối tuần"}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("patch expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/library/"+bound.ID, `{}`, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty patch expected 400, got %d", rec.Code)
	}

	// Schedule with validation failures first.
	at := time.Now().UTC().AddDate(0, 0, 3).Truncate(time.Minute)
	schedule := func(body string) *httptest.ResponseRecorder {
		return doJSON(t, srv, handler, http.MethodPost, "/api/v1/library/"+bound.ID+"/schedule", body, "")
	}
	for name, body := range map[string]string{
		"no time":     fmt.Sprintf(`{"channel_id":"%s"}`, ch.ID),
		"bad time":    fmt.Sprintf(`{"channel_id":"%s","scheduled_at":"tomorrow"}`, ch.ID),
		"publish now": fmt.Sprintf(`{"channel_id":"%s","scheduled_at":"%s","publish_now":true}`, ch.ID, at.Format(time.RFC3339)),
		"bad channel": fmt.Sprintf(`{"channel_id":"nope","scheduled_at":"%s"}`, at.Format(time.RFC3339)),
	} {
		if rec := schedule(body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", name, rec.Code, rec.Body.String())
		}
	}
	if n := countPosts(t, srv, handler); n != 0 {
		t.Fatalf("rejected schedules must create nothing, found %d", n)
	}
	// Defaults to the item's own Fanpage when no target is sent.
	rec := schedule(fmt.Sprintf(`{"scheduled_at":"%s","delete_after_publish":true}`, at.Format(time.RFC3339)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("schedule expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var scheduled struct {
		Count int           `json:"count"`
		Posts []domain.Post `json:"posts"`
	}
	decode(t, rec, &scheduled)
	if scheduled.Count != 1 || len(scheduled.Posts) != 1 {
		t.Fatalf("expected one post: %s", rec.Body.String())
	}
	post := scheduled.Posts[0]
	if post.Status != domain.PostStatusScheduled || post.LibraryItemID != bound.ID || !post.DeleteAfterPublish || post.Content != "Giảm 30% cuối tuần" || !post.ScheduledAt.Equal(at) {
		t.Fatalf("scheduled post wrong: %+v", post)
	}
	// The post is a copy: editing the library item afterwards must not change it.
	doJSON(t, srv, handler, http.MethodPatch, "/api/v1/library/"+bound.ID, `{"content":"Nội dung mới"}`, "")
	if got, _ := store.GetPostByID(ctx, post.ID); got.Content != "Giảm 30% cuối tuần" {
		t.Fatalf("scheduled post must keep its snapshot, got %q", got.Content)
	}

	// Publishing soft-deletes the item into the "Đã đăng" section.
	if err := store.MarkPostPublished(ctx, post.ID, "fb_123", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if got := listLibrary(t, srv, handler, "", ""); len(got.Items) != 1 || got.Items[0].ID == bound.ID {
		t.Fatalf("archived item must leave the active library: %+v", got.Items)
	}
	archived := listLibrary(t, srv, handler, "?archived=true", "")
	if len(archived.Items) != 1 || archived.Items[0].ID != bound.ID || archived.Items[0].PublishCount != 1 || archived.Items[0].ArchivedAt == nil {
		t.Fatalf("archive wrong: %+v", archived.Items)
	}
	if rec := schedule(fmt.Sprintf(`{"scheduled_at":"%s"}`, at.Format(time.RFC3339))); rec.Code != http.StatusConflict {
		t.Fatalf("scheduling an archived item expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, srv, handler, http.MethodPatch, "/api/v1/library/"+bound.ID, `{"title":"x"}`, ""); rec.Code != http.StatusConflict {
		t.Fatalf("editing an archived item expected 409, got %d", rec.Code)
	}

	// "Giữ lại": restore keeps the content and the counters.
	rec = doJSON(t, srv, handler, http.MethodPost, "/api/v1/library/"+bound.ID+"/restore", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("restore expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := listLibrary(t, srv, handler, "", ""); len(got.Items) != 2 {
		t.Fatalf("restored item must be active again: %+v", got.Items)
	}

	// Permanent delete keeps the published post.
	if rec := doJSON(t, srv, handler, http.MethodDelete, "/api/v1/library/"+bound.ID, "", ""); rec.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, srv, handler, http.MethodGet, "/api/v1/library/"+bound.ID, "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted item expected 404, got %d", rec.Code)
	}
	if got, err := store.GetPostByID(ctx, post.ID); err != nil || got.Status != domain.PostStatusPublished {
		t.Fatalf("published post must survive item deletion: %v %+v", err, got)
	}
}

func TestLibraryRBACAndApprovalRules(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	ch, _ := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "lib-rbac", Name: "RBAC", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	owned := createLibraryItem(t, srv, handler, fmt.Sprintf(`{"title":"Có chủ","channel_id":"%s","content":"owner item"}`, ch.ID), "")
	at := time.Now().UTC().AddDate(0, 0, 2).Format(time.RFC3339)

	// A viewer can read but never author or schedule.
	if got := listLibrary(t, srv, handler, "", "VIEWER"); len(got.Items) != 1 {
		t.Fatalf("viewer must see items of Fanpages they can view: %+v", got.Items)
	}
	if rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/library", `{"content":"viewer"}`, "VIEWER"); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer create expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	for method, path := range map[string]string{
		http.MethodPatch:  "/api/v1/library/" + owned.ID,
		http.MethodDelete: "/api/v1/library/" + owned.ID,
	} {
		if rec := doJSON(t, srv, handler, method, path, `{"title":"x"}`, "VIEWER"); rec.Code != http.StatusForbidden {
			t.Fatalf("viewer %s expected 403, got %d: %s", method, rec.Code, rec.Body.String())
		}
	}
	if rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/library/"+owned.ID+"/restore", "", "VIEWER"); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer restore expected 403, got %d", rec.Code)
	}
	if rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/library/"+owned.ID+"/schedule", fmt.Sprintf(`{"scheduled_at":"%s"}`, at), "VIEWER"); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer schedule expected 403, got %d: %s", rec.Code, rec.Body.String())
	}

	// An editor authors, but on an approval-required Fanpage the scheduled post waits for review.
	cg, _ := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "Needs approval", RequireApproval: true})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{ch.ID})
	if rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/library", fmt.Sprintf(`{"channel_id":"%s","content":"editor draft"}`, ch.ID), "EDITOR"); rec.Code != http.StatusCreated {
		t.Fatalf("editor create expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	rec := doJSON(t, srv, handler, http.MethodPost, "/api/v1/library/"+owned.ID+"/schedule", fmt.Sprintf(`{"scheduled_at":"%s","delete_after_publish":true}`, at), "EDITOR")
	if rec.Code != http.StatusCreated {
		t.Fatalf("editor schedule expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Posts []domain.Post `json:"posts"`
	}
	decode(t, rec, &resp)
	if len(resp.Posts) != 1 || resp.Posts[0].Status != domain.PostStatusPendingApproval {
		t.Fatalf("library scheduling must not bypass approval: %+v", resp.Posts)
	}
}

func authRequest(t *testing.T, srv *Server, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	serveTest(t, srv, h, rec, req)
	return rec
}

func registerTenant(t *testing.T, srv *Server, h http.Handler, email string) (token, orgID string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(fmt.Sprintf(`{"email":"%s","password":"Password!123456","full_name":"Tenant"}`, email)))
	rec := httptest.NewRecorder()
	serveTest(t, srv, h, rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: %d %s", email, rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return resp["token"].(string), resp["organization"].(map[string]any)["id"].(string)
}

func TestLibraryTenantIsolation(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	alice, aliceOrg := registerTenant(t, srv, handler, "alice@library-a.test")
	bob, _ := registerTenant(t, srv, handler, "bob@library-b.test")

	aliceCtx := domain.WithTenantContext(t.Context(), &domain.TenantContext{OrganizationID: aliceOrg, OrgRole: domain.OrgRoleOwner})
	channel, err := store.UpsertChannel(aliceCtx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: "lib-iso-a", Name: "Alice Page", EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	rec := authRequest(t, srv, handler, http.MethodPost, "/api/v1/library", fmt.Sprintf(`{"title":"Secret","channel_id":"%s","content":"alice only"}`, channel.ID), alice)
	if rec.Code != http.StatusCreated {
		t.Fatalf("alice item: %d %s", rec.Code, rec.Body.String())
	}
	var item domain.LibraryItem
	decode(t, rec, &item)
	at := time.Now().UTC().AddDate(0, 0, 2).Format(time.RFC3339)

	for name, call := range map[string]func() *httptest.ResponseRecorder{
		"get": func() *httptest.ResponseRecorder {
			return authRequest(t, srv, handler, http.MethodGet, "/api/v1/library/"+item.ID, "", bob)
		},
		"patch": func() *httptest.ResponseRecorder {
			return authRequest(t, srv, handler, http.MethodPatch, "/api/v1/library/"+item.ID, `{"title":"x"}`, bob)
		},
		"delete": func() *httptest.ResponseRecorder {
			return authRequest(t, srv, handler, http.MethodDelete, "/api/v1/library/"+item.ID, "", bob)
		},
		"restore": func() *httptest.ResponseRecorder {
			return authRequest(t, srv, handler, http.MethodPost, "/api/v1/library/"+item.ID+"/restore", "", bob)
		},
		"schedule": func() *httptest.ResponseRecorder {
			return authRequest(t, srv, handler, http.MethodPost, "/api/v1/library/"+item.ID+"/schedule", fmt.Sprintf(`{"channel_id":"%s","scheduled_at":"%s"}`, channel.ID, at), bob)
		},
	} {
		if got := call(); got.Code != http.StatusNotFound {
			t.Fatalf("%s across tenants expected 404, got %d: %s", name, got.Code, got.Body.String())
		}
	}
	rec = authRequest(t, srv, handler, http.MethodGet, "/api/v1/library", "", bob)
	var list libraryListResponse
	decode(t, rec, &list)
	if rec.Code != http.StatusOK || len(list.Items) != 0 {
		t.Fatalf("bob must see an empty library: %d %s", rec.Code, rec.Body.String())
	}
	// Bob cannot bind a library item to Alice's Fanpage.
	if rec := authRequest(t, srv, handler, http.MethodPost, "/api/v1/library", fmt.Sprintf(`{"channel_id":"%s","content":"x"}`, channel.ID), bob); rec.Code != http.StatusBadRequest {
		t.Fatalf("binding a foreign channel expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	// Alice's item is intact.
	if rec := authRequest(t, srv, handler, http.MethodGet, "/api/v1/library/"+item.ID, "", alice); rec.Code != http.StatusOK {
		t.Fatalf("alice item must survive: %d", rec.Code)
	}
}

// memberWithRoles creates a MEMBER of the default organization with one role per channel and returns a bearer token.
func memberWithRoles(t *testing.T, srv *Server, store database.Store, email string, roles map[string]domain.GroupRole) string {
	t.Helper()
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	user, err := store.CreateUser(ctx, &domain.User{Email: email, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddOrganizationMember(ctx, &domain.OrganizationMember{OrganizationID: domain.DefaultOrganizationID, UserID: user.ID, OrgRole: domain.OrgRoleMember}); err != nil {
		t.Fatal(err)
	}
	for channelID, role := range roles {
		group, err := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: fmt.Sprintf("%s-%s", email, channelID[:8])})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, group.ID, []string{channelID}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.SetChannelGroupPermission(ctx, &domain.ChannelGroupPermission{ChannelGroupID: group.ID, UserID: user.ID, GroupRole: role}); err != nil {
			t.Fatal(err)
		}
	}
	token, err := crypto.SignJWT(crypto.JWTClaims{UserID: user.ID, OrganizationID: domain.DefaultOrganizationID, OrgRole: "MEMBER", Exp: time.Now().Add(time.Hour).Unix()}, srv.cfg.APIAdminKey)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestLibraryPerChannelRBACFanOutAndDeleteFlag(t *testing.T) {
	srv, handler, store := setupTestServer(t)
	ctx := domain.WithTenantContext(t.Context(), domain.DefaultTenantContext())
	mkChannel := func(ext string) *domain.Channel {
		ch, err := store.UpsertChannel(ctx, &domain.Channel{Platform: domain.PlatformFacebook, ExternalID: ext, Name: "Page " + ext, EncryptedAccessToken: "t", TokenStatus: domain.TokenStatusActive, IsActive: true})
		if err != nil {
			t.Fatal(err)
		}
		return ch
	}
	chA, chB, chC := mkChannel("rbac-a"), mkChannel("rbac-b"), mkChannel("rbac-c")
	at := time.Now().UTC().AddDate(0, 0, 2).Format(time.RFC3339)
	itemA := createLibraryItem(t, srv, handler, fmt.Sprintf(`{"title":"A","channel_id":"%s","content":"for A"}`, chA.ID), "")
	itemB := createLibraryItem(t, srv, handler, fmt.Sprintf(`{"title":"B","channel_id":"%s","content":"for B"}`, chB.ID), "")
	unbound := createLibraryItem(t, srv, handler, `{"title":"Chung","content":"anyone"}`, "")

	// Editor on A only; viewer on B; nothing on C.
	editorA := memberWithRoles(t, srv, store, "editor-a@library.test", map[string]domain.GroupRole{chA.ID: domain.GroupRoleEditor, chB.ID: domain.GroupRoleViewer})
	call := func(token, method, path, body string) *httptest.ResponseRecorder {
		return authRequest(t, srv, handler, method, path, body, token)
	}
	// The list holds only items of Fanpages the member may view, plus unbound ones.
	rec := call(editorA, http.MethodGet, "/api/v1/library", "")
	var list libraryListResponse
	decode(t, rec, &list)
	seen := map[string]bool{}
	for _, it := range list.Items {
		seen[it.ID] = true
	}
	if !seen[itemA.ID] || !seen[itemB.ID] || !seen[unbound.ID] {
		t.Fatalf("editor must see A, viewable B and the unbound item: %+v", list.Items)
	}
	itemC := createLibraryItem(t, srv, handler, fmt.Sprintf(`{"title":"C","channel_id":"%s","content":"for C"}`, chC.ID), "")
	rec = call(editorA, http.MethodGet, "/api/v1/library", "")
	decode(t, rec, &list)
	for _, it := range list.Items {
		if it.ID == itemC.ID {
			t.Fatalf("an item of a Fanpage the member cannot view must stay hidden")
		}
	}
	if rec := call(editorA, http.MethodGet, "/api/v1/library/"+itemC.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("hidden item expected 404, got %d", rec.Code)
	}

	// Author on A: may edit A's item, not B's (viewer) nor C's (hidden).
	if rec := call(editorA, http.MethodPatch, "/api/v1/library/"+itemA.ID, `{"title":"A2"}`); rec.Code != http.StatusOK {
		t.Fatalf("editor patch own-channel item expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"patch viewer-channel item":  call(editorA, http.MethodPatch, "/api/v1/library/"+itemB.ID, `{"title":"x"}`),
		"delete viewer-channel item": call(editorA, http.MethodDelete, "/api/v1/library/"+itemB.ID, ""),
		"restore viewer item":        call(editorA, http.MethodPost, "/api/v1/library/"+itemB.ID+"/restore", ""),
		"create bound to viewer ch":  call(editorA, http.MethodPost, "/api/v1/library", fmt.Sprintf(`{"channel_id":"%s","content":"x"}`, chB.ID)),
		"create bound to hidden ch":  call(editorA, http.MethodPost, "/api/v1/library", fmt.Sprintf(`{"channel_id":"%s","content":"x"}`, chC.ID)),
	} {
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s expected 403, got %d: %s", name, rec.Code, rec.Body.String())
		}
	}
	// Unbound items need an author role on at least one Fanpage: the editor has one.
	if rec := call(editorA, http.MethodPost, "/api/v1/library", `{"title":"mine","content":"unbound by editor"}`); rec.Code != http.StatusCreated {
		t.Fatalf("editor create unbound expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	// A pure viewer (no authoring anywhere) cannot create an unbound item, and a member with no role sees nothing.
	viewerOnly := memberWithRoles(t, srv, store, "viewer-only@library.test", map[string]domain.GroupRole{chA.ID: domain.GroupRoleViewer})
	if rec := call(viewerOnly, http.MethodPost, "/api/v1/library", `{"content":"x"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer create unbound expected 403, got %d", rec.Code)
	}
	nobody := memberWithRoles(t, srv, store, "nobody@library.test", nil)
	rec = call(nobody, http.MethodGet, "/api/v1/library", "")
	decode(t, rec, &list)
	if rec.Code != http.StatusOK || len(list.Items) != 0 {
		t.Fatalf("a member without any Fanpage role must get an empty library (not even unbound items): %d %+v", rec.Code, list.Items)
	}

	// SEC-001: delete_after_publish archives the item later, so it needs authoring rights on the ITEM's Fanpage.
	editorAB := memberWithRoles(t, srv, store, "editor-a-viewer-b@library.test", map[string]domain.GroupRole{chA.ID: domain.GroupRoleEditor, chB.ID: domain.GroupRoleViewer})
	flagged := fmt.Sprintf(`{"channel_id":"%s","scheduled_at":"%s","delete_after_publish":true}`, chA.ID, at)
	if rec := call(editorAB, http.MethodPost, "/api/v1/library/"+itemB.ID+"/schedule", flagged); rec.Code != http.StatusForbidden {
		t.Fatalf("flagging another Fanpage's item expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	plain := fmt.Sprintf(`{"channel_id":"%s","scheduled_at":"%s"}`, chA.ID, at)
	rec = call(editorAB, http.MethodPost, "/api/v1/library/"+itemB.ID+"/schedule", plain)
	if rec.Code != http.StatusCreated {
		t.Fatalf("scheduling B's item onto A without the flag expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if n := countPosts(t, srv, handler); n != 1 {
		t.Fatalf("only the allowed schedule may create a post, found %d", n)
	}

	// Fan-out through the library: several channels and a group create one batch, every post linked and flagged.
	multi := fmt.Sprintf(`{"channel_ids":["%s","%s"],"scheduled_at":"%s","delete_after_publish":true}`, chA.ID, chC.ID, at)
	rec = doJSON(t, srv, handler, http.MethodPost, "/api/v1/library/"+unbound.ID+"/schedule", multi, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("fan-out expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var fan struct {
		BatchID string        `json:"batch_id"`
		Count   int           `json:"count"`
		Posts   []domain.Post `json:"posts"`
	}
	decode(t, rec, &fan)
	if fan.Count != 2 || fan.BatchID == "" || len(fan.Posts) != 2 {
		t.Fatalf("fan-out must report a batch and two posts: %s", rec.Body.String())
	}
	for _, p := range fan.Posts {
		if p.LibraryItemID != unbound.ID || !p.DeleteAfterPublish || p.BatchID != fan.BatchID || p.Status != domain.PostStatusScheduled {
			t.Fatalf("fan-out post wrong: %+v", p)
		}
	}
	cg, _ := store.CreateChannelGroup(ctx, &domain.ChannelGroup{Name: "Library group"})
	_ = store.SetChannelGroupChannels(ctx, domain.DefaultOrganizationID, cg.ID, []string{chA.ID, chB.ID, chC.ID})
	rec = doJSON(t, srv, handler, http.MethodPost, "/api/v1/library/"+unbound.ID+"/schedule", fmt.Sprintf(`{"channel_group_id":"%s","scheduled_at":"%s"}`, cg.ID, at), "")
	decode(t, rec, &fan)
	if rec.Code != http.StatusCreated || fan.Count != 3 || fan.BatchID == "" {
		t.Fatalf("group schedule must create 3 posts in a batch: %d %s", rec.Code, rec.Body.String())
	}
	// A single target response carries no batch_id (SA-5).
	rec = doJSON(t, srv, handler, http.MethodPost, "/api/v1/library/"+itemA.ID+"/schedule", fmt.Sprintf(`{"scheduled_at":"%s"}`, at), "")
	if rec.Code != http.StatusCreated || strings.Contains(rec.Body.String(), "batch_id") {
		t.Fatalf("single-target schedule must omit batch_id: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLibraryListPagingBoundaries(t *testing.T) {
	srv, handler, _ := setupTestServer(t)
	for i := 0; i < 3; i++ {
		createLibraryItem(t, srv, handler, fmt.Sprintf(`{"title":"item %d","content":"page me %d"}`, i, i), "")
	}
	for query, want := range map[string]struct {
		n       int
		hasMore bool
	}{
		"?limit=3":            {3, false}, // exactly the item count: no phantom next page
		"?limit=2":            {2, true},
		"?limit=2&offset=2":   {1, false},
		"?limit=2&offset=3":   {0, false},
		"?limit=5000":         {3, false}, // clamped to the API maximum, never an error
		"?limit=0":            {3, false}, // default page
		"?limit=-4&offset=-1": {3, false},
	} {
		got := listLibrary(t, srv, handler, query, "")
		if len(got.Items) != want.n || got.HasMore != want.hasMore {
			t.Fatalf("%s: want %d items has_more=%v, got %d has_more=%v", query, want.n, want.hasMore, len(got.Items), got.HasMore)
		}
	}
	if got := listLibrary(t, srv, handler, "?q=PAGE%20ME%201", ""); len(got.Items) != 1 {
		t.Fatalf("case-insensitive search wrong: %+v", got.Items)
	}
}
