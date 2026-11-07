package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// TestMultiTenantDataIsolation verifies strict data segregation between organizations.
func TestMultiTenantDataIsolation(t *testing.T) {
	srv, handler, _ := setupTestServer(t)

	// Step 1: Provision Tenant A (Alice)
	regA := `{"email":"alice@company-a.com","password":"PasswordA!123456","full_name":"Alice OrgA"}`
	reqA := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(regA))
	recA := httptest.NewRecorder()
	serveTest(t, srv, handler, recA, reqA)
	if recA.Code != http.StatusCreated {
		t.Fatalf("failed to register Org A: %d: %s", recA.Code, recA.Body.String())
	}
	var respA map[string]any
	_ = json.Unmarshal(recA.Body.Bytes(), &respA)
	tokenA := respA["token"].(string)
	orgA := respA["organization"].(map[string]any)
	orgAID := orgA["id"].(string)

	// Step 2: Provision Tenant B (Bob)
	regB := `{"email":"bob@company-b.com","password":"PasswordB!123456","full_name":"Bob OrgB"}`
	reqB := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(regB))
	recB := httptest.NewRecorder()
	serveTest(t, srv, handler, recB, reqB)
	if recB.Code != http.StatusCreated {
		t.Fatalf("failed to register Org B: %d: %s", recB.Code, recB.Body.String())
	}
	var respB map[string]any
	_ = json.Unmarshal(recB.Body.Bytes(), &respB)
	tokenB := respB["token"].(string)
	orgB := respB["organization"].(map[string]any)
	orgBID := orgB["id"].(string)

	if orgAID == orgBID {
		t.Fatalf("expected distinct organization IDs, got same: %s", orgAID)
	}

	// Step 3: Org A creates a Channel
	chanPayloadA := `{"platform":"FACEBOOK","external_id":"fb-page-org-a-101","name":"Org A Confidential Channel","category":"Brand","access_token":"EAAB_fake_token_a"}`
	reqChanA := httptest.NewRequest(http.MethodPost, "/api/v1/channels", bytes.NewBufferString(chanPayloadA))
	reqChanA.Header.Set("Authorization", "Bearer "+tokenA)
	recChanA := httptest.NewRecorder()
	serveTest(t, srv, handler, recChanA, reqChanA)
	if recChanA.Code != http.StatusCreated {
		t.Fatalf("failed to create Org A channel: %d: %s", recChanA.Code, recChanA.Body.String())
	}
	var chanA map[string]any
	_ = json.Unmarshal(recChanA.Body.Bytes(), &chanA)
	chanAID := chanA["id"].(string)

	// Step 4: Org A creates a Post
	postPayloadA := fmt.Sprintf(`{"channel_id":"%s","content":"Secret Org A Q4 Strategy Roadmap","publish_now":false}`, chanAID)
	reqPostA := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(postPayloadA))
	reqPostA.Header.Set("Authorization", "Bearer "+tokenA)
	recPostA := httptest.NewRecorder()
	serveTest(t, srv, handler, recPostA, reqPostA)
	if recPostA.Code != http.StatusCreated {
		t.Fatalf("failed to create Org A post: %d: %s", recPostA.Code, recPostA.Body.String())
	}
	var postA map[string]any
	_ = json.Unmarshal(recPostA.Body.Bytes(), &postA)
	postAID := postA["id"].(string)

	// Step 5: Org A creates a Channel Group
	groupPayloadA := `{"name":"Org A Executive Group","description":"Confidential C-suite announcements","require_approval":true}`
	reqGroupA := httptest.NewRequest(http.MethodPost, "/api/v1/channel-groups", bytes.NewBufferString(groupPayloadA))
	reqGroupA.Header.Set("Authorization", "Bearer "+tokenA)
	recGroupA := httptest.NewRecorder()
	serveTest(t, srv, handler, recGroupA, reqGroupA)
	if recGroupA.Code != http.StatusCreated {
		t.Fatalf("failed to create Org A group: %d: %s", recGroupA.Code, recGroupA.Body.String())
	}
	var groupA map[string]any
	_ = json.Unmarshal(recGroupA.Body.Bytes(), &groupA)
	groupAID := groupA["id"].(string)

	// =========================================================================
	// ISOLATION VERIFICATION: Org B queries its own resources
	// =========================================================================

	// Test 1: Org B GET /api/v1/channels -> must NOT see Org A's channel
	reqListChanB := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	reqListChanB.Header.Set("Authorization", "Bearer "+tokenB)
	recListChanB := httptest.NewRecorder()
	serveTest(t, srv, handler, recListChanB, reqListChanB)
	if recListChanB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Org B channels query, got: %d", recListChanB.Code)
	}
	var listChanB struct {
		Channels []domain.Channel `json:"channels"`
	}
	_ = json.Unmarshal(recListChanB.Body.Bytes(), &listChanB)
	for _, ch := range listChanB.Channels {
		if ch.ID == chanAID || ch.Name == "Org A Confidential Channel" {
			t.Fatalf("CRITICAL SECURITY LEAK: Org B can see Org A channel: %+v", ch)
		}
	}

	// Test 2: Org B GET /api/v1/posts -> must NOT see Org A's post
	reqListPostB := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	reqListPostB.Header.Set("Authorization", "Bearer "+tokenB)
	recListPostB := httptest.NewRecorder()
	serveTest(t, srv, handler, recListPostB, reqListPostB)
	if recListPostB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Org B posts query, got: %d", recListPostB.Code)
	}
	var listPostB struct {
		Posts []domain.Post `json:"posts"`
	}
	_ = json.Unmarshal(recListPostB.Body.Bytes(), &listPostB)
	for _, p := range listPostB.Posts {
		if p.ID == postAID || p.Content == "Secret Org A Q4 Strategy Roadmap" {
			t.Fatalf("CRITICAL SECURITY LEAK: Org B can see Org A post: %+v", p)
		}
	}

	// Test 3: Org B GET /api/v1/channel-groups -> must NOT see Org A's channel group
	reqListGroupsB := httptest.NewRequest(http.MethodGet, "/api/v1/channel-groups", nil)
	reqListGroupsB.Header.Set("Authorization", "Bearer "+tokenB)
	recListGroupsB := httptest.NewRecorder()
	serveTest(t, srv, handler, recListGroupsB, reqListGroupsB)
	if recListGroupsB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Org B channel-groups query, got: %d", recListGroupsB.Code)
	}
	var listGroupsB struct {
		ChannelGroups []domain.ChannelGroup `json:"channel_groups"`
	}
	_ = json.Unmarshal(recListGroupsB.Body.Bytes(), &listGroupsB)
	for _, g := range listGroupsB.ChannelGroups {
		if g.ID == groupAID || g.Name == "Org A Executive Group" {
			t.Fatalf("CRITICAL SECURITY LEAK: Org B can see Org A channel group: %+v", g)
		}
	}

	// =========================================================================
	// ISOLATION VERIFICATION: Org B direct cross-tenant access attempts
	// =========================================================================

	// Test 4: Org B attempts GET /api/v1/posts/{Org_A_Post_ID} -> 404 or 403
	reqDirectPostB := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+postAID, nil)
	reqDirectPostB.Header.Set("Authorization", "Bearer "+tokenB)
	recDirectPostB := httptest.NewRecorder()
	serveTest(t, srv, handler, recDirectPostB, reqDirectPostB)
	if recDirectPostB.Code != http.StatusNotFound && recDirectPostB.Code != http.StatusForbidden {
		t.Fatalf("expected 404 or 403 when Org B fetches Org A post directly, got: %d: %s", recDirectPostB.Code, recDirectPostB.Body.String())
	}

	// Test 5: Org B attempts DELETE /api/v1/channels/{Org_A_Channel_ID} -> 404 or 403
	reqDelChanB := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/"+chanAID, nil)
	reqDelChanB.Header.Set("Authorization", "Bearer "+tokenB)
	recDelChanB := httptest.NewRecorder()
	serveTest(t, srv, handler, recDelChanB, reqDelChanB)
	if recDelChanB.Code != http.StatusNotFound && recDelChanB.Code != http.StatusForbidden {
		t.Fatalf("expected 404 or 403 when Org B deletes Org A channel, got: %d: %s", recDelChanB.Code, recDelChanB.Body.String())
	}

	// Verify Org A channel was NOT deleted
	reqVerifyChanA := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	reqVerifyChanA.Header.Set("Authorization", "Bearer "+tokenA)
	recVerifyChanA := httptest.NewRecorder()
	serveTest(t, srv, handler, recVerifyChanA, reqVerifyChanA)
	var verifyChanList struct {
		Channels []domain.Channel `json:"channels"`
	}
	_ = json.Unmarshal(recVerifyChanA.Body.Bytes(), &verifyChanList)
	foundChanA := false
	for _, ch := range verifyChanList.Channels {
		if ch.ID == chanAID {
			foundChanA = true
			break
		}
	}
	if !foundChanA {
		t.Fatalf("Org A channel was incorrectly deleted by Org B unauthorized request!")
	}

	// Test 6: Org B attempts DELETE /api/v1/channel-groups/{Org_A_Group_ID} -> 400 or 404 or 403
	reqDelGroupB := httptest.NewRequest(http.MethodDelete, "/api/v1/channel-groups/"+groupAID, nil)
	reqDelGroupB.Header.Set("Authorization", "Bearer "+tokenB)
	recDelGroupB := httptest.NewRecorder()
	serveTest(t, srv, handler, recDelGroupB, reqDelGroupB)
	if recDelGroupB.Code != http.StatusBadRequest && recDelGroupB.Code != http.StatusNotFound && recDelGroupB.Code != http.StatusForbidden {
		t.Fatalf("expected 400/404/403 when Org B deletes Org A group, got: %d: %s", recDelGroupB.Code, recDelGroupB.Body.String())
	}

	// Test 7: Org B attempts POST /api/v1/posts referencing Org A's channel -> 400 or 403
	tamperedPostPayload := fmt.Sprintf(`{"channel_id":"%s","content":"Attacker Injected Post"}`, chanAID)
	reqTamperedPost := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewBufferString(tamperedPostPayload))
	reqTamperedPost.Header.Set("Authorization", "Bearer "+tokenB)
	recTamperedPost := httptest.NewRecorder()
	serveTest(t, srv, handler, recTamperedPost, reqTamperedPost)
	if recTamperedPost.Code != http.StatusBadRequest && recTamperedPost.Code != http.StatusForbidden {
		t.Fatalf("expected 400 or 403 when Org B attempts to publish to Org A channel, got: %d: %s", recTamperedPost.Code, recTamperedPost.Body.String())
	}

	// Test 8: Org B attempts POST /api/v1/posts/{Org_A_Post_ID}/review -> 404 or 403
	reqReviewOrgB := httptest.NewRequest(http.MethodPost, "/api/v1/posts/"+postAID+"/review", bytes.NewBufferString(`{"action":"APPROVED"}`))
	reqReviewOrgB.Header.Set("Authorization", "Bearer "+tokenB)
	recReviewOrgB := httptest.NewRecorder()
	serveTest(t, srv, handler, recReviewOrgB, reqReviewOrgB)
	if recReviewOrgB.Code != http.StatusNotFound && recReviewOrgB.Code != http.StatusForbidden {
		t.Fatalf("expected 404 or 403 when Org B attempts to review Org A post, got: %d: %s", recReviewOrgB.Code, recReviewOrgB.Body.String())
	}

	// =========================================================================
	// HEADER SPOOFING & MEMBERSHIP VALIDATION
	// =========================================================================

	// Test 9: User A attempts to switch to Org B via X-Organization-Id without membership -> 403 Forbidden
	reqSpoof := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	reqSpoof.Header.Set("Authorization", "Bearer "+tokenA)
	reqSpoof.Header.Set("X-Organization-Id", orgBID)
	recSpoof := httptest.NewRecorder()
	serveTest(t, srv, handler, recSpoof, reqSpoof)
	if recSpoof.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when accessing unassociated Org via X-Organization-Id header, got: %d", recSpoof.Code)
	}

	// Test 10: Multi-Tenant Org Switch: Existing user Bob is added as member to Org A
	addMemberPayload := `{"email":"bob@company-b.com","role":"MEMBER"}`
	reqAddMember := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/organizations/%s/members", orgAID), bytes.NewBufferString(addMemberPayload))
	reqAddMember.Header.Set("Authorization", "Bearer "+tokenA)
	recAddMember := httptest.NewRecorder()
	serveTest(t, srv, handler, recAddMember, reqAddMember)
	if recAddMember.Code != http.StatusCreated {
		t.Fatalf("failed to add Bob to Org A: %d: %s", recAddMember.Code, recAddMember.Body.String())
	}

	// Now Bob requests Org A channels with X-Organization-Id header -> 200 OK
	reqBobInOrgA := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	reqBobInOrgA.Header.Set("Authorization", "Bearer "+tokenB)
	reqBobInOrgA.Header.Set("X-Organization-Id", orgAID)
	recBobInOrgA := httptest.NewRecorder()
	serveTest(t, srv, handler, recBobInOrgA, reqBobInOrgA)
	if recBobInOrgA.Code != http.StatusOK {
		t.Fatalf("Bob should now be allowed to view Org A with valid membership, got: %d: %s", recBobInOrgA.Code, recBobInOrgA.Body.String())
	}

	// Verify Bob can see Org A channel when explicitly operating in Org A
	var bobOrgAChans struct {
		Channels []domain.Channel `json:"channels"`
	}
	_ = json.Unmarshal(recBobInOrgA.Body.Bytes(), &bobOrgAChans)
	foundOrgAChan := false
	for _, c := range bobOrgAChans.Channels {
		if c.ID == chanAID {
			foundOrgAChan = true
			break
		}
	}
	if foundOrgAChan {
		t.Fatalf("membership alone must not reveal a channel without group permissions: %s", orgAID)
	}

	// Test 11: Unregistered user invitation and accept workflow
	invitePayload := `{"email":"charlie-unregistered@company-c.com","role":"MEMBER"}`
	reqInvite := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/organizations/%s/members", orgAID), bytes.NewBufferString(invitePayload))
	reqInvite.Header.Set("Authorization", "Bearer "+tokenA)
	recInvite := httptest.NewRecorder()
	serveTest(t, srv, handler, recInvite, reqInvite)
	if recInvite.Code != http.StatusCreated {
		t.Fatalf("failed to invite unregistered Charlie to Org A: %d: %s", recInvite.Code, recInvite.Body.String())
	}
	var inviteResp map[string]any
	_ = json.Unmarshal(recInvite.Body.Bytes(), &inviteResp)
	invToken, _ := inviteResp["token"].(string)
	if invToken == "" {
		t.Fatalf("expected invitation token for unregistered user")
	}

	// Charlie registers
	regCharlie := `{"email":"charlie-unregistered@company-c.com","password":"PasswordC!123456","full_name":"Charlie C"}`
	reqRegCharlie := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(regCharlie))
	recRegCharlie := httptest.NewRecorder()
	serveTest(t, srv, handler, recRegCharlie, reqRegCharlie)
	if recRegCharlie.Code != http.StatusCreated {
		t.Fatalf("failed to register Charlie: %d: %s", recRegCharlie.Code, recRegCharlie.Body.String())
	}
	var respC map[string]any
	_ = json.Unmarshal(recRegCharlie.Body.Bytes(), &respC)
	tokenC := respC["token"].(string)

	// Charlie accepts invitation
	reqAccept := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/accept", bytes.NewBufferString(fmt.Sprintf(`{"token":"%s"}`, invToken)))
	reqAccept.Header.Set("Authorization", "Bearer "+tokenC)
	recAccept := httptest.NewRecorder()
	serveTest(t, srv, handler, recAccept, reqAccept)
	if recAccept.Code != http.StatusOK {
		t.Fatalf("Charlie failed to accept invitation: %d: %s", recAccept.Code, recAccept.Body.String())
	}
}

// Legacy anonymous owner behavior is deliberately removed.
func TestBackwardCompatibility(t *testing.T) {
	srv, handler, _ := setupTestServer(t)
	for _, path := range []string{"/api/v1/channels", "/api/v1/posts"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Admin-Key", srv.cfg.APIAdminKey)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("legacy anonymous request should be rejected, got %d", rec.Code)
		}
	}
}
