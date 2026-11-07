package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// handleFBOAuthLogin initializes the OAuth2 authorization URL for the Facebook Adapter.
func (s *Server) handleFBOAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.fbClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "facebook client is not configured",
		})
		return
	}
	if s.cfg.FBAppID == "" || s.cfg.FBAppID == "your_facebook_app_id" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "FB_APP_ID is not configured in .env",
		})
		return
	}

	front, frontErr := url.Parse(s.cfg.FrontendURL)
	callback, callbackErr := url.Parse(s.cfg.FBRedirectURI)
	if frontErr != nil || callbackErr != nil || front.Host == "" || callback.Host != front.Host || callback.Scheme != front.Scheme {
		writeJSON(w, 400, map[string]any{"error": "FB_REDIRECT_URI must use the frontend origin and /api/v1/auth/facebook/callback so the browser can bind OAuth securely"})
		return
	}

	tc := domain.TenantFromContext(r.Context())
	state := crypto.SignOAuthState(tc.OrganizationID, tc.UserID, s.cfg.APIAdminKey)
	if err := s.store.CreateOAuthState(r.Context(), oauthStateHash(state), time.Now().Add(10*time.Minute)); err != nil {
		writeJSON(w, 503, map[string]any{"error": "cannot start secure OAuth session"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthCookieName(s.cfg.FrontendURL), Value: state, Path: "/", MaxAge: 600, HttpOnly: true, Secure: front.Scheme == "https", SameSite: http.SameSiteLaxMode})
	loginURL := s.fbClient.BuildOAuthLoginURL(s.cfg.FBAppID, s.cfg.FBRedirectURI, state)
	writeJSON(w, http.StatusOK, map[string]any{
		"oauth_login_url": loginURL,
	})
}

// handleFBOAuthCallback processes OAuth2 callback from Meta and maps Fanpages to `domain.Channel`.
func (s *Server) handleFBOAuthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	fbErr := r.URL.Query().Get("error")
	fbErrDesc := r.URL.Query().Get("error_description")

	targetHost := s.cfg.FrontendURL
	if targetHost == "" {
		targetHost = "http://localhost:3000"
	}

	cookie, cookieErr := r.Cookie(oauthCookieName(s.cfg.FrontendURL))
	http.SetCookie(w, &http.Cookie{Name: oauthCookieName(s.cfg.FrontendURL), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: strings.HasPrefix(s.cfg.FrontendURL, "https://"), SameSite: http.SameSiteLaxMode})
	if cookieErr != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		http.Error(w, "OAuth browser session mismatch; reconnect from PostPilot", 403)
		return
	}

	if s.fbClient == nil {
		http.Redirect(w, r, targetHost+"/?oauth=error&message="+url.QueryEscape("facebook client is not configured"), http.StatusTemporaryRedirect)
		return
	}

	if fbErr != "" {
		http.Redirect(w, r, targetHost+"/?oauth=error&message="+url.QueryEscape(fbErrDesc), http.StatusTemporaryRedirect)
		return
	}
	if code == "" {
		http.Redirect(w, r, targetHost+"/?oauth=error&message="+url.QueryEscape("missing oauth code parameter"), http.StatusTemporaryRedirect)
		return
	}

	if state == "" {
		http.Redirect(w, r, targetHost+"/?oauth=error&message="+url.QueryEscape("missing oauth state parameter"), http.StatusTemporaryRedirect)
		return
	}
	orgID, userID, err := crypto.VerifyOAuthState(state, s.cfg.APIAdminKey)
	if err != nil {
		http.Redirect(w, r, targetHost+"/?oauth=error&message="+url.QueryEscape("invalid or expired oauth state: "+err.Error()), http.StatusTemporaryRedirect)
		return
	}

	ctx = domain.WithTenantContext(ctx, &domain.TenantContext{OrganizationID: orgID, UserID: userID})
	if err := s.store.ConsumeOAuthState(ctx, oauthStateHash(state)); err != nil {
		http.Error(w, "OAuth session expired or already used", 403)
		return
	}

	user, userErr := s.store.GetUserByID(ctx, userID)
	members, memberErr := s.store.ListOrganizationMembers(ctx, orgID)
	allowed := false
	for _, member := range members {
		if member.UserID == userID && (member.OrgRole == domain.OrgRoleOwner || member.OrgRole == domain.OrgRoleAdmin) {
			allowed = true
		}
	}
	if userErr != nil || user == nil || !user.IsActive || memberErr != nil || !allowed {
		http.Error(w, "OAuth workspace access revoked", http.StatusForbidden)
		return
	}
	ctx = domain.WithTenantContext(ctx, &domain.TenantContext{UserID: userID, OrganizationID: orgID})

	shortUserToken, err := s.fbClient.ExchangeCodeForUserToken(
		ctx, s.cfg.FBAppID, s.cfg.FBAppSecret, s.cfg.FBRedirectURI, code,
	)
	if err != nil {
		http.Redirect(w, r, targetHost+"/?oauth=error&message="+url.QueryEscape("Facebook token exchange failed; verify app configuration and reconnect"), http.StatusTemporaryRedirect)
		return
	}

	savedChannels, err := s.exchangeAndSaveFacebookChannels(ctx, shortUserToken, orgID)
	if err != nil {
		http.Redirect(w, r, targetHost+"/?oauth=error&message="+url.QueryEscape(err.Error()), http.StatusTemporaryRedirect)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("%s/?oauth=success&count=%d", targetHost, len(savedChannels)), http.StatusTemporaryRedirect)
}

// handleFBExchangeUserToken receives `short_lived_user_token` and exchanges it for Permanent Page Access Tokens,
// mapping managed pages into `domain.Channel` with `Platform = FACEBOOK`.
func (s *Server) handleFBExchangeUserToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ShortLivedUserToken string `json:"short_lived_user_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ShortLivedUserToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "short_lived_user_token is required"})
		return
	}

	tc := domain.TenantFromContext(r.Context())
	if !tc.IsOrgAdminOrOwner() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "only organization owners and admins can connect channels"})
		return
	}

	savedChannels, err := s.exchangeAndSaveFacebookChannels(r.Context(), strings.TrimSpace(body.ShortLivedUserToken), tc.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":        "Permanent Channel Access Tokens synchronized and encrypted successfully",
		"imported_count": len(savedChannels),
		"channels":       savedChannels,
	})
}

func (s *Server) exchangeAndSaveFacebookChannels(ctx context.Context, shortUserToken, orgID string) ([]*domain.Channel, error) {
	if s.fbClient == nil {
		return nil, errors.New("facebook client is not configured")
	}
	if orgID == "" {
		return nil, errors.New("organization is required")
	}
	longUserToken := shortUserToken

	if s.cfg.FBAppID != "" && s.cfg.FBAppSecret != "" && s.cfg.FBAppID != "your_facebook_app_id" {
		exchanged, err := s.fbClient.ExchangeShortToLongLivedUserToken(ctx, s.cfg.FBAppID, s.cfg.FBAppSecret, shortUserToken)
		if err != nil {
			return nil, errors.New("Facebook token exchange failed; verify the app configuration and reconnect")
		}
		longUserToken = exchanged
	}

	managedPages, err := s.fbClient.FetchManagedPages(ctx, longUserToken)
	if err != nil {
		return nil, errors.New("cannot load Facebook Pages; reconnect and verify Page permissions")
	}

	var savedChannels []*domain.Channel
	for _, mp := range managedPages {
		encToken, err := s.cipher.Encrypt(mp.AccessToken)
		if err != nil {
			return savedChannels, fmt.Errorf("connected %d Pages; cannot encrypt credential for Page %s", len(savedChannels), mp.PageID)
		}
		saved, err := s.store.UpsertChannel(ctx, &domain.Channel{
			OrganizationID:       orgID,
			Platform:             domain.PlatformFacebook,
			ExternalID:           mp.PageID,
			Name:                 mp.PageName,
			Category:             mp.Category,
			EncryptedAccessToken: encToken,
			TokenStatus:          domain.TokenStatusActive,
			IsActive:             true,
		})
		if err != nil {
			return savedChannels, fmt.Errorf("connected %d Pages; could not save Page %s; retry after storage is available", len(savedChannels), mp.PageID)
		}
		saved.MaskedToken = crypto.MaskToken(mp.AccessToken)
		savedChannels = append(savedChannels, saved)
	}
	return savedChannels, nil
}

// handleFBWebhookVerify verifies Meta Webhook challenge (`hub.challenge`).
func (s *Server) handleFBWebhookVerify(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == s.cfg.FBWebhookVerifyToken {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(challenge))
		return
	}
	w.WriteHeader(http.StatusForbidden)
}

// handleFBWebhookEvent receives webhook event payloads from Meta.
func (s *Server) handleFBWebhookEvent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "webhook event processing is not available; scheduled publishing uses the worker"})
}

func oauthStateHash(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}

func oauthCookieName(frontendURL string) string {
	if strings.HasPrefix(frontendURL, "https://") {
		return "__Host-postpilot-oauth-state"
	}
	return "postpilot_oauth_state"
}
