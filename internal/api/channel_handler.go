package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/happy-darwin/fb-autoposter/internal/crypto"
	"github.com/happy-darwin/fb-autoposter/internal/domain"
)

// handleListChannels trả về danh sách các Kênh xuất bản đã kết nối kèm trạng thái Rate-Limit.
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

// handleUpsertChannel thêm mới hoặc cập nhật một Kênh xuất bản (Channel).
func (s *Server) handleUpsertChannel(w http.ResponseWriter, r *http.Request) {
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
			"error": "Nền tảng không hợp lệ. Các nền tảng được hỗ trợ: FACEBOOK, LINKEDIN, TWITTER, TIKTOK",
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

	// Nếu là kênh Facebook và người dùng dán User Access Token thay vì Page Access Token,
	// tự động gọi Adapter Facebook để trích xuất đúng Page Access Token của trang đó.
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

// handleVerifyChannelToken kiểm tra sức khỏe Credential của Channel thông qua `domain.PublisherRegistry`.
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

// handleDeleteChannel xóa Kênh xuất bản và dọn dẹp dữ liệu liên quan.
func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	channelID := r.PathValue("id")

	channel, err := s.store.GetChannelByID(ctx, channelID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "Không tìm thấy Kênh xuất bản cần xóa"})
		return
	}

	if err := s.store.DeleteChannel(ctx, channelID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Xóa Kênh thất bại: " + err.Error()})
		return
	}

	if channel.ExternalID != "" {
		_ = s.redis.DeleteChannelRateLimit(ctx, channel.ExternalID)
	}

	s.logger.Info("Channel deleted successfully", "channel_id", channelID, "name", channel.Name, "platform", channel.Platform)

	writeJSON(w, http.StatusOK, map[string]any{
		"message":    "Đã xóa Kênh xuất bản thành công",
		"channel_id": channelID,
	})
}

