package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/happy-darwin/fb-autoposter/internal/crypto"
	"github.com/happy-darwin/fb-autoposter/internal/domain"
)

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	redisErr := s.redis.Ping(ctx)
	redisMode := "redis-tcp"
	if s.redis.IsMemoryFallback() {
		redisMode = "in-memory-fallback"
	}

	fastDepth, _ := s.redis.QueueDepth(ctx, domain.QueueTierFast)
	heavyDepth, _ := s.redis.QueueDepth(ctx, domain.QueueTierHeavy)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":            "healthy",
		"service":           "fb-autoposter-api",
		"redis_mode":        redisMode,
		"redis_ok":          redisErr == nil,
		"fast_queue_depth":  fastDepth,
		"heavy_queue_depth": heavyDepth,
		"timestamp":         time.Now().UTC(),
	})
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := s.store.GetSystemStats(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	stats.FastQueueDepth, _ = s.redis.QueueDepth(ctx, domain.QueueTierFast)
	stats.HeavyQueueDepth, _ = s.redis.QueueDepth(ctx, domain.QueueTierHeavy)

	writeJSON(w, http.StatusOK, stats)
}

// ============================================================================
// 1. OAUTH2 & ĐỔI PERMANENT PAGE ACCESS TOKEN
// ============================================================================

func (s *Server) handleFBOAuthLogin(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleFBOAuthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	fbErr := r.URL.Query().Get("error")
	fbErrDesc := r.URL.Query().Get("error_description")

	targetHost := "http://localhost:3000"
	if r.Host != "" && strings.Contains(r.Host, "8080") && !strings.Contains(r.Host, "3000") {
		targetHost = "http://localhost:3000"
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

	savedPages, err := s.exchangeAndSaveToken(ctx, shortUserToken)
	if err != nil {
		http.Redirect(w, r, targetHost+"/?oauth=error&message="+url.QueryEscape(err.Error()), http.StatusTemporaryRedirect)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("%s/?oauth=success&count=%d", targetHost, len(savedPages)), http.StatusTemporaryRedirect)
}

// handleFBExchangeUserToken nhận `short_lived_user_token` (từ Graph API Explorer)
// và tự động đổi sang Long-lived User Token (60 ngày) -> Lấy toàn bộ Permanent Page Access Token
// của các Fanpage bạn quản trị -> Mã hóa AES-256-GCM và lưu vào Database.
func (s *Server) handleFBExchangeUserToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ShortLivedUserToken string `json:"short_lived_user_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ShortLivedUserToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "short_lived_user_token is required"})
		return
	}

	savedPages, err := s.exchangeAndSaveToken(r.Context(), strings.TrimSpace(body.ShortLivedUserToken))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":        "Đã đồng bộ và mã hóa Permanent Page Access Token thành công",
		"imported_count": len(savedPages),
		"pages":          savedPages,
	})
}

func (s *Server) exchangeAndSaveToken(ctx context.Context, shortUserToken string) ([]*domain.FacebookPage, error) {
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

	var savedPages []*domain.FacebookPage
	for _, mp := range managedPages {
		encToken, err := s.cipher.Encrypt(mp.AccessToken)
		if err != nil {
			continue
		}
		saved, err := s.store.UpsertPage(ctx, &domain.FacebookPage{
			PageID:               mp.PageID,
			PageName:             mp.PageName,
			Category:             mp.Category,
			EncryptedAccessToken: encToken,
			TokenStatus:          domain.TokenStatusActive,
			IsActive:             true,
		})
		if err == nil {
			saved.MaskedToken = crypto.MaskToken(mp.AccessToken)
			savedPages = append(savedPages, saved)
		}
	}
	return savedPages, nil
}

// ============================================================================
// 2. QUẢN LÝ FANPAGE & TOKEN
// ============================================================================

func (s *Server) handleListPages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pages, err := s.store.ListPages(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	for i := range pages {
		if rawTok, err := s.cipher.Decrypt(pages[i].EncryptedAccessToken); err == nil {
			pages[i].MaskedToken = crypto.MaskToken(rawTok)
		}
		if usage, err := s.redis.GetPageRateLimit(ctx, pages[i].PageID); err == nil && usage.MaxPercent() > 0 {
			pages[i].RateLimitCallCount = usage.CallCount
			pages[i].RateLimitTotalTime = usage.TotalTime
			pages[i].RateLimitTotalCPU = usage.TotalCPUTime
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"pages": pages,
	})
}

func (s *Server) handleUpsertPage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PageID      string `json:"page_id"`
		PageName    string `json:"page_name"`
		Category    string `json:"category"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	req.PageID = strings.TrimSpace(req.PageID)
	req.PageName = strings.TrimSpace(req.PageName)
	req.AccessToken = strings.TrimSpace(req.AccessToken)

	if req.PageID == "" || req.PageName == "" || req.AccessToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "page_id, page_name, and access_token are required",
		})
		return
	}

	// Nếu người dùng vô tình dán User Access Token thay vì Page Access Token,
	// tự động gọi /me/accounts để trích xuất đúng Page Access Token của Fanpage đó.
	finalToken := req.AccessToken
	if managedPages, err := s.fbClient.FetchManagedPages(r.Context(), req.AccessToken); err == nil && len(managedPages) > 0 {
		for _, mp := range managedPages {
			if mp.PageID == req.PageID && mp.AccessToken != "" {
				finalToken = mp.AccessToken
				if req.Category == "" && mp.Category != "" {
					req.Category = mp.Category
				}
				break
			}
		}
	}

	encryptedToken, err := s.cipher.Encrypt(finalToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encrypt token failed: " + err.Error()})
		return
	}

	page, err := s.store.UpsertPage(r.Context(), &domain.FacebookPage{
		PageID:               req.PageID,
		PageName:             req.PageName,
		Category:             req.Category,
		EncryptedAccessToken: encryptedToken,
		TokenStatus:          domain.TokenStatusActive,
		IsActive:             true,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	page.MaskedToken = crypto.MaskToken(finalToken)
	writeJSON(w, http.StatusCreated, page)
}

func (s *Server) handleVerifyPageToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pageUUID := r.PathValue("id")

	page, err := s.store.GetPageByUUID(ctx, pageUUID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "page not found"})
		return
	}

	rawToken, err := s.cipher.Decrypt(page.EncryptedAccessToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "decrypt token failed"})
		return
	}

	debugInfo, err := s.fbClient.DebugToken(ctx, rawToken, "")
	now := time.Now().UTC()
	if err != nil || !debugInfo.IsValid {
		_ = s.store.UpdatePageTokenStatus(ctx, page.ID, domain.TokenStatusExpired, now)
		errMsg := "token invalid"
		if err != nil {
			errMsg = err.Error()
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"page_uuid":    page.ID,
			"token_status": domain.TokenStatusExpired,
			"is_valid":     false,
			"error":        errMsg,
		})
		return
	}

	_ = s.store.UpdatePageTokenStatus(ctx, page.ID, domain.TokenStatusActive, now)
	writeJSON(w, http.StatusOK, map[string]any{
		"page_uuid":    page.ID,
		"token_status": domain.TokenStatusActive,
		"is_valid":     true,
		"debug_info":   debugInfo,
	})
}

func (s *Server) handleDeletePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pageUUID := r.PathValue("id")

	// Lấy thông tin page để dọn dẹp cache rate limit nếu có
	page, err := s.store.GetPageByUUID(ctx, pageUUID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "Không tìm thấy Fanpage cần xóa"})
		return
	}

	if err := s.store.DeletePage(ctx, pageUUID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Xóa Fanpage thất bại: " + err.Error()})
		return
	}

	// Xóa cache Rate Limit trong Redis
	if page.PageID != "" {
		_ = s.redis.DeletePageRateLimit(ctx, page.PageID)
	}

	s.logger.Info("Fanpage deleted successfully", "page_uuid", pageUUID, "page_name", page.PageName)

	writeJSON(w, http.StatusOK, map[string]any{
		"message":   "Đã xóa Fanpage thành công",
		"page_uuid": pageUUID,
	})
}

// ============================================================================
// 3. QUẢN LÝ BÀI ĐĂNG & LỊCH HẸN (POSTS & SCHEDULER)
// ============================================================================

func (s *Server) handleListPosts(w http.ResponseWriter, r *http.Request) {
	statusFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	posts, err := s.store.ListPosts(r.Context(), statusFilter, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"posts": posts,
	})
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		PageUUID    string   `json:"page_uuid"`
		Content     string   `json:"content"`
		LinkURL     string   `json:"link_url"`
		MediaType   string   `json:"media_type"`
		MediaURLs   []string `json:"media_urls"`
		ScheduledAt string   `json:"scheduled_at"` // ISO-8601 (RFC3339). Nếu rỗng -> Đăng ngay hoặc hẹn hiện tại
		PublishNow  bool     `json:"publish_now"`
		MaxRetries  int      `json:"max_retries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}

	if req.PageUUID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "page_uuid is required"})
		return
	}
	if _, err := s.store.GetPageByUUID(ctx, req.PageUUID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "page_uuid does not exist"})
		return
	}

	mediaType := strings.ToUpper(strings.TrimSpace(req.MediaType))
	if mediaType == "" {
		if req.LinkURL != "" {
			mediaType = domain.MediaTypeLink
		} else {
			mediaType = domain.MediaTypeNone
		}
	}

	schedTime := time.Now().UTC()
	if req.ScheduledAt != "" {
		if parsed, err := time.Parse(time.RFC3339, req.ScheduledAt); err == nil {
			schedTime = parsed.UTC()
		}
	}

	initialStatus := domain.PostStatusScheduled
	if req.PublishNow {
		initialStatus = domain.PostStatusQueued
		schedTime = time.Now().UTC()
	}

	post, err := s.store.CreatePost(ctx, &domain.Post{
		PageUUID:    req.PageUUID,
		Content:     req.Content,
		LinkURL:     req.LinkURL,
		MediaType:   mediaType,
		MediaURLs:   req.MediaURLs,
		Status:      initialStatus,
		ScheduledAt: schedTime,
		MaxRetries:  req.MaxRetries,
		CreatedBy:   "admin",
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if req.PublishNow {
		payload := domain.PublishJobPayload{
			PostID:     post.ID,
			PageUUID:   post.PageUUID,
			QueueTier:  post.QueueTier,
			Attempt:    1,
			EnqueuedAt: time.Now().UTC(),
		}
		_ = s.redis.EnqueuePublishJob(ctx, payload)
	}

	writeJSON(w, http.StatusCreated, post)
}

func (s *Server) handleGetPostDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID := r.PathValue("id")

	post, err := s.store.GetPostByID(ctx, postID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "post not found"})
		return
	}

	logs, _ := s.store.ListPublishLogsByPost(ctx, postID)
	writeJSON(w, http.StatusOK, map[string]any{
		"post":         post,
		"publish_logs": logs,
	})
}

func (s *Server) handlePublishPostNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID := r.PathValue("id")

	post, err := s.store.GetPostByID(ctx, postID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "post not found"})
		return
	}

	// Nếu truyền query ?sync=true thì thực thi ngay lập tức trong HTTP request và trả kết quả
	if r.URL.Query().Get("sync") == "true" && s.processor != nil {
		err := s.processor.ProcessPostByID(ctx, post.ID)
		updated, _ := s.store.GetPostByID(ctx, post.ID)
		logs, _ := s.store.ListPublishLogsByPost(ctx, post.ID)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error":        err.Error(),
				"post":         updated,
				"publish_logs": logs,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"message":      "Đã đăng bài trực tiếp thành công",
			"post":         updated,
			"publish_logs": logs,
		})
		return
	}

	payload := domain.PublishJobPayload{
		PostID:     post.ID,
		PageUUID:   post.PageUUID,
		QueueTier:  post.QueueTier,
		Attempt:    post.RetryCount + 1,
		EnqueuedAt: time.Now().UTC(),
	}
	if err := s.redis.EnqueuePublishJob(ctx, payload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"message":    "Đã đẩy bài viết vào hàng đợi ưu tiên (" + post.QueueTier + ")",
		"post_id":    post.ID,
		"queue_tier": post.QueueTier,
	})
}

func (s *Server) handleCancelPost(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if err := s.store.CancelPost(r.Context(), postID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "Đã hủy lịch đăng bài viết",
		"post_id": postID,
	})
}

// ============================================================================
// 4. MEDIA UPLOAD (DIRECT & PRESIGNED), AI CONTENT GENERATOR & WEBHOOKS
// ============================================================================

func (s *Server) handleMediaUpload(w http.ResponseWriter, r *http.Request) {
	// Giới hạn buffer RAM 50MB, tối đa kích thước file 500MB
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file too large or invalid multipart form: " + err.Error()})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing 'file' field in form: " + err.Error()})
		return
	}
	defer file.Close()

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// 1. Luôn lưu bản sao file vào thư mục local ./uploads trước tiên để tránh lỗi đóng stream
	// và hỗ trợ upload nhị phân trực tiếp (Chunked Binary Upload) lên Meta Graph API khi chạy ở localhost.
	uploadDir := "./uploads"
	_ = os.MkdirAll(uploadDir, 0755)
	cleanName := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), filepath.Base(strings.ReplaceAll(header.Filename, " ", "_")))
	localPath := filepath.Join(uploadDir, cleanName)

	dst, err := os.Create(localPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "cannot save file locally: " + err.Error()})
		return
	}
	writtenBytes, copyErr := io.Copy(dst, file)
	_ = dst.Close()
	if copyErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "cannot copy file: " + copyErr.Error()})
		return
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	publicURL := fmt.Sprintf("%s://%s/uploads/%s", scheme, r.Host, cleanName)
	storageKey := "uploads/" + cleanName

	// 2. Nếu có cấu hình S3/CDN công khai (không phải localhost), đồng bộ thêm lên S3
	if s.s3 != nil && s.s3.Endpoint != "" && !strings.Contains(s.s3.PublicBaseURL, "localhost") && !strings.Contains(s.s3.PublicBaseURL, "127.0.0.1") {
		if localFile, openErr := os.Open(localPath); openErr == nil {
			if pub, key, s3Err := s.s3.Upload(r.Context(), header.Filename, mimeType, localFile, writtenBytes); s3Err == nil {
				publicURL = pub
				storageKey = key
			} else {
				s.logger.Warn("S3 upload failed, using local storage URL", "error", s3Err)
			}
			_ = localFile.Close()
		}
	}

	asset, _ := s.store.CreateMediaAsset(r.Context(), &domain.MediaAsset{
		FileName:      header.Filename,
		StorageKey:    storageKey,
		PublicURL:     publicURL,
		MimeType:      mimeType,
		FileSizeBytes: writtenBytes,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"message":     "Upload media thành công",
		"public_url":  publicURL,
		"storage_key": storageKey,
		"file_name":   header.Filename,
		"file_size":   writtenBytes,
		"mime_type":   mimeType,
		"media_asset": asset,
	})
}

func (s *Server) handlePresignUpload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileName      string `json:"file_name"`
		MimeType      string `json:"mime_type"`
		FileSizeBytes int64  `json:"file_size_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file_name is required"})
		return
	}

	presigned, err := s.s3.GeneratePresignedPutURL(req.FileName, 900)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	asset, _ := s.store.CreateMediaAsset(r.Context(), &domain.MediaAsset{
		FileName:      req.FileName,
		StorageKey:    presigned.StorageKey,
		PublicURL:     presigned.PublicURL,
		MimeType:      req.MimeType,
		FileSizeBytes: req.FileSizeBytes,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"presigned":   presigned,
		"media_asset": asset,
	})
}

func (s *Server) handleAIGenerateContent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Topic string `json:"topic"`
		Tone  string `json:"tone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Topic) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "topic is required"})
		return
	}

	content, modelUsed, err := s.aiGen.GeneratePostContent(r.Context(), req.Topic, req.Tone)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"topic":             req.Topic,
		"tone":              req.Tone,
		"model_used":        modelUsed,
		"generated_content": content,
	})
}

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

func (s *Server) handleFBWebhookEvent(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	s.logger.Info("Received Meta Webhook event", "object", payload["object"])
	writeJSON(w, http.StatusOK, map[string]any{"received": true})
}
