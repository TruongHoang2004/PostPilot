package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/happy-darwin/fb-autoposter/internal/crypto"
	"github.com/happy-darwin/fb-autoposter/internal/domain"
)

// handleFBOAuthLogin khởi tạo URL đăng nhập OAuth2 cho Facebook Adapter.
func (s *Server) handleFBOAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.fbClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "facebook client is not configured",
		})
		return
	}
	if s.cfg.FBAppID == "" || s.cfg.FBAppID == "your_facebook_app_id" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "FB_APP_ID chưa được cấu hình trong file .env",
		})
		return
	}
	loginURL := s.fbClient.BuildOAuthLoginURL(s.cfg.FBAppID, s.cfg.FBRedirectURI, "fbposter_state")
	writeJSON(w, http.StatusOK, map[string]any{
		"oauth_login_url": loginURL,
	})
}

// handleFBOAuthCallback xử lý OAuth2 callback từ Meta và lưu các Fanpage thành các `domain.Channel`.
func (s *Server) handleFBOAuthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	fbErr := r.URL.Query().Get("error")
	fbErrDesc := r.URL.Query().Get("error_description")

	targetHost := s.cfg.FrontendURL
	if targetHost == "" {
		targetHost = "http://localhost:3000"
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

	shortUserToken, err := s.fbClient.ExchangeCodeForUserToken(
		ctx, s.cfg.FBAppID, s.cfg.FBAppSecret, s.cfg.FBRedirectURI, code,
	)
	if err != nil {
		http.Redirect(w, r, targetHost+"/?oauth=error&message="+url.QueryEscape("exchange code failed: "+err.Error()), http.StatusTemporaryRedirect)
		return
	}

	savedChannels, err := s.exchangeAndSaveFacebookChannels(ctx, shortUserToken)
	if err != nil {
		http.Redirect(w, r, targetHost+"/?oauth=error&message="+url.QueryEscape(err.Error()), http.StatusTemporaryRedirect)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("%s/?oauth=success&count=%d", targetHost, len(savedChannels)), http.StatusTemporaryRedirect)
}

// handleFBExchangeUserToken nhận `short_lived_user_token` và đổi sang Permanent Page Access Token,
// ánh xạ các trang quản trị thành các `domain.Channel` có `Platform = FACEBOOK`.
func (s *Server) handleFBExchangeUserToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ShortLivedUserToken string `json:"short_lived_user_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ShortLivedUserToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "short_lived_user_token is required"})
		return
	}

	savedChannels, err := s.exchangeAndSaveFacebookChannels(r.Context(), strings.TrimSpace(body.ShortLivedUserToken))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":        "Đã đồng bộ và mã hóa Permanent Channel Access Token thành công",
		"imported_count": len(savedChannels),
		"channels":       savedChannels,
	})
}

func (s *Server) exchangeAndSaveFacebookChannels(ctx context.Context, shortUserToken string) ([]*domain.Channel, error) {
	if s.fbClient == nil {
		return nil, errors.New("facebook client is not configured")
	}
	longUserToken := shortUserToken

	if s.cfg.FBAppID != "" && s.cfg.FBAppSecret != "" && s.cfg.FBAppID != "your_facebook_app_id" {
		exchanged, err := s.fbClient.ExchangeShortToLongLivedUserToken(ctx, s.cfg.FBAppID, s.cfg.FBAppSecret, shortUserToken)
		if err != nil {
			return nil, fmt.Errorf("exchange short-to-long-lived token: %w", err)
		}
		longUserToken = exchanged
	}

	managedPages, err := s.fbClient.FetchManagedPages(ctx, longUserToken)
	if err != nil {
		return nil, fmt.Errorf("fetch /me/accounts: %w", err)
	}

	var savedChannels []*domain.Channel
	for _, mp := range managedPages {
		encToken, err := s.cipher.Encrypt(mp.AccessToken)
		if err != nil {
			continue
		}
		saved, err := s.store.UpsertChannel(ctx, &domain.Channel{
			Platform:             domain.PlatformFacebook,
			ExternalID:           mp.PageID,
			Name:                 mp.PageName,
			Category:             mp.Category,
			EncryptedAccessToken: encToken,
			TokenStatus:          domain.TokenStatusActive,
			IsActive:             true,
		})
		if err == nil {
			saved.MaskedToken = crypto.MaskToken(mp.AccessToken)
			savedChannels = append(savedChannels, saved)
		}
	}
	return savedChannels, nil
}

// handleFBWebhookVerify xác thực Meta Webhook (`hub.challenge`).
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

// handleFBWebhookEvent tiếp nhận sự kiện Webhook từ Meta.
func (s *Server) handleFBWebhookEvent(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	s.logger.Info("Received Meta Webhook event", "object", payload["object"])
	writeJSON(w, http.StatusOK, map[string]any{"received": true})
}

