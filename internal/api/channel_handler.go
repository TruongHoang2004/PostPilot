package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/crypto"
	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// handleListChannels returns connected publishing channels along with their rate limit usage.
func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	for i := range channels {
		if rawTok, err := s.cipher.Decrypt(channels[i].EncryptedAccessToken); err == nil {
			channels[i].MaskedToken = crypto.MaskToken(rawTok)
		}
		if usage, err := s.redis.GetChannelRateLimit(ctx, channels[i].ExternalID); err == nil && usage.MaxPercent() > 0 {
			channels[i].RateLimitCallCount = usage.CallCount
			channels[i].RateLimitTotalTime = usage.TotalTime
			channels[i].RateLimitTotalCPU = usage.TotalCPUTime
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"channels": channels,
	})
}

// handleUpsertChannel creates or updates a publishing Channel.
func (s *Server) handleUpsertChannel(w http.ResponseWriter, r *http.Request) {
	tc := domain.TenantFromContext(r.Context())
	if !tc.IsOrgAdminOrOwner() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "only organization owners and admins can configure channels"})
		return
	}

	var req struct {
		Platform    string `json:"platform"`
		ExternalID  string `json:"external_id"`
		Name        string `json:"name"`
		Category    string `json:"category"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	platform := domain.NormalizePlatform(domain.PlatformType(strings.ToUpper(strings.TrimSpace(req.Platform))))
	if !domain.IsValidPlatform(platform) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "Invalid platform. Supported platforms: FACEBOOK, LINKEDIN, TWITTER, TIKTOK",
		})
		return
	}
	req.ExternalID = strings.TrimSpace(req.ExternalID)
	req.Name = strings.TrimSpace(req.Name)
	req.AccessToken = strings.TrimSpace(req.AccessToken)

	if req.ExternalID == "" || req.Name == "" || req.AccessToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "external_id, name, and access_token are required",
		})
		return
	}

	// For Facebook channels where a User Access Token is provided instead of Page Access Token,
	// automatically invoke the Facebook adapter to extract the correct Page Access Token.
	finalToken := req.AccessToken
	if platform == domain.PlatformFacebook && s.fbClient != nil {
		if managedPages, err := s.fbClient.FetchManagedPages(r.Context(), req.AccessToken); err == nil && len(managedPages) > 0 {
			for _, mp := range managedPages {
				if mp.PageID == req.ExternalID && mp.AccessToken != "" {
					finalToken = mp.AccessToken
					if req.Category == "" && mp.Category != "" {
						req.Category = mp.Category
					}
					break
				}
			}
		}
	}

	encryptedToken, err := s.cipher.Encrypt(finalToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encrypt token failed: " + err.Error()})
		return
	}

	channel, err := s.store.UpsertChannel(r.Context(), &domain.Channel{
		Platform:             platform,
		ExternalID:           req.ExternalID,
		Name:                 req.Name,
		Category:             req.Category,
		EncryptedAccessToken: encryptedToken,
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	channel.MaskedToken = crypto.MaskToken(finalToken)
	writeJSON(w, http.StatusCreated, channel)
}

// handleVerifyChannelToken verifies credential health for a Channel via `domain.PublisherRegistry`.
func (s *Server) handleVerifyChannelToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	channelID := r.PathValue("id")

	channel, err := s.store.GetChannelByID(ctx, channelID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "channel not found"})
		return
	}

	rawToken, err := s.cipher.Decrypt(channel.EncryptedAccessToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "decrypt token failed"})
		return
	}

	publisher, err := s.registry.Get(channel.Platform)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	health, err := publisher.VerifyToken(ctx, rawToken)
	now := time.Now().UTC()
	if err != nil || health == nil || !health.IsValid {
		_ = s.store.UpdateChannelTokenStatus(ctx, channel.ID, domain.TokenStatusExpired, now)
		errMsg := "token invalid"
		if err != nil {
			errMsg = err.Error()
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"channel_id":   channel.ID,
			"token_status": domain.TokenStatusExpired,
			"is_valid":     false,
			"error":        errMsg,
		})
		return
	}

	_ = s.store.UpdateChannelTokenStatus(ctx, channel.ID, domain.TokenStatusActive, now)
	writeJSON(w, http.StatusOK, map[string]any{
		"channel_id":   channel.ID,
		"token_status": domain.TokenStatusActive,
		"is_valid":     true,
		"debug_info":   health.RawInfo,
	})
}

// handleDeleteChannel removes a channel and purges related cached data.
func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	channelID := r.PathValue("id")

	tc := domain.TenantFromContext(ctx)
	if !tc.IsOrgAdminOrOwner() {
		role, hasExplicit := s.resolveChannelPermission(ctx, channelID)
		if !hasExplicit || !tc.CanManageChannel(role) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "insufficient permissions to delete channel"})
			return
		}
	}

	channel, err := s.store.GetChannelByID(ctx, channelID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "channel not found"})
		return
	}

	if err := s.store.DeleteChannel(ctx, channelID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "delete channel failed: " + err.Error()})
		return
	}

	if channel.ExternalID != "" {
		_ = s.redis.DeleteChannelRateLimit(ctx, channel.ExternalID)
	}

	s.logger.Info("Channel deleted successfully", "channel_id", channelID, "name", channel.Name, "platform", channel.Platform)

	writeJSON(w, http.StatusOK, map[string]any{
		"message":    "Channel deleted successfully",
		"channel_id": channelID,
	})
}

