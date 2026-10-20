package facebook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/happy-darwin/fb-autoposter/internal/domain"
)

func isLocalOrInternalURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.Contains(lower, "localhost") ||
		strings.Contains(lower, "127.0.0.1") ||
		strings.Contains(lower, "minio:") ||
		strings.Contains(lower, "api:") ||
		strings.Contains(lower, "/uploads/")
}

// resolveLocalMediaBytes kiểm tra xem mediaURL có trỏ tới file trong ./uploads/ hoặc dịch vụ nội bộ (api/minio/localhost) hay không.
// Nếu có, đọc trực tiếp dữ liệu nhị phân để đẩy thẳng lên Meta Graph API (Chunked/Multipart)
// thay vì gửi link localhost (do máy chủ Meta trên Internet không thể tải file từ localhost của người dùng).
func resolveLocalMediaBytes(mediaURL string) ([]byte, string) {
	fileName := filepath.Base(mediaURL)
	if qIdx := strings.Index(fileName, "?"); qIdx != -1 {
		fileName = fileName[:qIdx]
	}
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = "media.mp4"
	}

	// 1. Đọc trực tiếp từ ổ đĩa ./uploads/<fileName> nếu có
	if idx := strings.Index(mediaURL, "/uploads/"); idx != -1 {
		relName := mediaURL[idx+len("/uploads/"):]
		if qIdx := strings.Index(relName, "?"); qIdx != -1 {
			relName = relName[:qIdx]
		}
		baseName := filepath.Base(relName)
		localPath := filepath.Join("./uploads", baseName)
		if data, err := os.ReadFile(localPath); err == nil && len(data) > 0 {
			return data, baseName
		}
	}

	// 2. Nếu là URL nội bộ (localhost / 127.0.0.1 / minio / api), tải qua HTTP nội bộ giữa các container
	if isLocalOrInternalURL(mediaURL) && (strings.HasPrefix(mediaURL, "http://") || strings.HasPrefix(mediaURL, "https://")) {
		candidates := []string{mediaURL}
		if strings.Contains(mediaURL, "localhost:8080") || strings.Contains(mediaURL, "127.0.0.1:8080") {
			r1 := strings.ReplaceAll(strings.ReplaceAll(mediaURL, "localhost:8080", "api:8080"), "127.0.0.1:8080", "api:8080")
			candidates = append([]string{r1}, candidates...)
		}
		if strings.Contains(mediaURL, "localhost:9000") || strings.Contains(mediaURL, "127.0.0.1:9000") {
			r2 := strings.ReplaceAll(strings.ReplaceAll(mediaURL, "localhost:9000", "minio:9000"), "127.0.0.1:9000", "minio:9000")
			candidates = append([]string{r2}, candidates...)
		}
		hc := &http.Client{Timeout: 60 * time.Second}
		for _, cand := range candidates {
			resp, err := hc.Get(cand)
			if err == nil && resp.StatusCode == http.StatusOK {
				data, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if readErr == nil && len(data) > 0 {
					return data, fileName
				}
			} else if resp != nil {
				_ = resp.Body.Close()
			}
		}
	}

	return nil, ""
}

// PublishOutcome chứa kết quả sau khi thực thi đăng bài lên Facebook Fanpage.
type PublishOutcome struct {
	FBPostID       string
	Steps          []StepExecutionLog
	FinalRateLimit domain.RateLimitUsage
}

// StepExecutionLog ghi lại từng bước gọi API (ví dụ: upload từng ảnh trong album rồi gom vào feed).
type StepExecutionLog struct {
	Action         string
	RequestSummary string
	Metrics        CallMetrics
	Error          error
}

// PublishPost tự động điều phối cách đăng bài dựa trên `post.MediaType`:
// - NONE / LINK  -> POST /{page_id}/feed
// - IMAGE        -> POST /{page_id}/photos
// - ALBUM        -> Upload từng ảnh `published=false` + POST /{page_id}/feed `attached_media`
// - VIDEO        -> POST /{page_id}/videos (tự động dùng Chunked Binary Upload nếu là file local)
// - REEL         -> Luồng 3 bước chuẩn Facebook Reels (`/{page_id}/video_reels`)
func (c *Client) PublishPost(ctx context.Context, fbPageID, pageAccessToken string, post *domain.Post) (*PublishOutcome, error) {
	switch post.MediaType {
	case domain.MediaTypeNone, domain.MediaTypeLink:
		return c.PublishTextOrLink(ctx, fbPageID, pageAccessToken, post.Content, post.LinkURL)
	case domain.MediaTypeImage:
		if len(post.MediaURLs) == 0 {
			return nil, errors.New("IMAGE post requires at least 1 url in media_urls")
		}
		if imgBytes, fileName := resolveLocalMediaBytes(post.MediaURLs[0]); len(imgBytes) > 0 {
			return c.PublishSinglePhotoBytes(ctx, fbPageID, pageAccessToken, post.Content, fileName, imgBytes)
		}
		if isLocalOrInternalURL(post.MediaURLs[0]) {
			return nil, fmt.Errorf("không tìm thấy file ảnh local (%s); vui lòng tải lại file lên từ giao diện", post.MediaURLs[0])
		}
		return c.PublishSinglePhoto(ctx, fbPageID, pageAccessToken, post.Content, post.MediaURLs[0])
	case domain.MediaTypeAlbum:
		if len(post.MediaURLs) == 0 {
			return nil, errors.New("ALBUM post requires media_urls")
		}
		if len(post.MediaURLs) == 1 {
			if imgBytes, fileName := resolveLocalMediaBytes(post.MediaURLs[0]); len(imgBytes) > 0 {
				return c.PublishSinglePhotoBytes(ctx, fbPageID, pageAccessToken, post.Content, fileName, imgBytes)
			}
			return c.PublishSinglePhoto(ctx, fbPageID, pageAccessToken, post.Content, post.MediaURLs[0])
		}
		return c.PublishPhotoAlbum(ctx, fbPageID, pageAccessToken, post.Content, post.MediaURLs)
	case domain.MediaTypeVideo:
		if len(post.MediaURLs) == 0 {
			return nil, errors.New("VIDEO post requires video url in media_urls")
		}
		if videoBytes, _ := resolveLocalMediaBytes(post.MediaURLs[0]); len(videoBytes) > 0 {
			return c.PublishVideoChunked(ctx, fbPageID, pageAccessToken, post.Content, videoBytes, 4*1024*1024)
		}
		if isLocalOrInternalURL(post.MediaURLs[0]) {
			return nil, fmt.Errorf("không tìm thấy file video local (%s); vui lòng chọn tải lại video từ máy tính", post.MediaURLs[0])
		}
		return c.PublishVideoByURL(ctx, fbPageID, pageAccessToken, post.Content, post.MediaURLs[0])
	case domain.MediaTypeReel:
		if len(post.MediaURLs) == 0 {
			return nil, errors.New("REEL post requires video url in media_urls")
		}
		if videoBytes, _ := resolveLocalMediaBytes(post.MediaURLs[0]); len(videoBytes) > 0 {
			return c.PublishVideoChunked(ctx, fbPageID, pageAccessToken, post.Content, videoBytes, 4*1024*1024)
		}
		if isLocalOrInternalURL(post.MediaURLs[0]) {
			return nil, fmt.Errorf("không tìm thấy file video local (%s); vui lòng chọn tải lại video từ máy tính", post.MediaURLs[0])
		}
		return c.PublishReelByURL(ctx, fbPageID, pageAccessToken, post.Content, post.MediaURLs[0])
	default:
		return nil, fmt.Errorf("unsupported media_type: %s", post.MediaType)
	}
}

// PublishTextOrLink đăng bài viết dạng chữ hoặc kèm đường dẫn (Link Preview) qua `POST /{page_id}/feed`.
func (c *Client) PublishTextOrLink(ctx context.Context, pageID, token, message, link string) (*PublishOutcome, error) {
	form := url.Values{}
	form.Set("message", message)
	form.Set("access_token", token)
	if link != "" {
		form.Set("link", link)
	}

	path := fmt.Sprintf("%s/feed", pageID)
	res, metrics, err := c.PostForm(ctx, path, form)

	outcome := &PublishOutcome{
		FinalRateLimit: metrics.RateLimitUsage,
		Steps: []StepExecutionLog{
			{
				Action:         "PUBLISH_FEED",
				RequestSummary: fmt.Sprintf("POST /%s/feed (has_link=%v)", pageID, link != ""),
				Metrics:        metrics,
				Error:          err,
			},
		},
	}
	if err != nil {
		return outcome, err
	}

	outcome.FBPostID = extractFBID(res)
	return outcome, nil
}

// PublishSinglePhoto đăng 1 ảnh đơn kèm caption qua `POST /{page_id}/photos`.
func (c *Client) PublishSinglePhoto(ctx context.Context, pageID, token, caption, imageURL string) (*PublishOutcome, error) {
	form := url.Values{}
	form.Set("url", imageURL)
	form.Set("caption", caption)
	form.Set("published", "true")
	form.Set("access_token", token)

	path := fmt.Sprintf("%s/photos", pageID)
	res, metrics, err := c.PostForm(ctx, path, form)

	outcome := &PublishOutcome{
		FinalRateLimit: metrics.RateLimitUsage,
		Steps: []StepExecutionLog{
			{
				Action:         "PUBLISH_SINGLE_PHOTO",
				RequestSummary: fmt.Sprintf("POST /%s/photos url=%s", pageID, imageURL),
				Metrics:        metrics,
				Error:          err,
			},
		},
	}
	if err != nil {
		return outcome, err
	}

	outcome.FBPostID = extractFBID(res)
	return outcome, nil
}

// PublishSinglePhotoBytes đăng 1 ảnh từ dữ liệu nhị phân local qua `POST /{page_id}/photos` (multipart).
func (c *Client) PublishSinglePhotoBytes(ctx context.Context, pageID, token, caption, fileName string, imageBytes []byte) (*PublishOutcome, error) {
	path := fmt.Sprintf("%s/photos", pageID)
	fields := map[string]string{
		"caption":      caption,
		"published":    "true",
		"access_token": token,
	}
	res, metrics, err := c.PostMultipartChunk(ctx, path, fields, "source", fileName, imageBytes)
	outcome := &PublishOutcome{
		FinalRateLimit: metrics.RateLimitUsage,
		Steps: []StepExecutionLog{
			{
				Action:         "PUBLISH_SINGLE_PHOTO_BINARY",
				RequestSummary: fmt.Sprintf("POST /%s/photos file=%s size=%d", pageID, fileName, len(imageBytes)),
				Metrics:        metrics,
				Error:          err,
			},
		},
	}
	if err != nil {
		return outcome, err
	}
	outcome.FBPostID = extractFBID(res)
	return outcome, nil
}

// PublishPhotoAlbum đăng nhiều ảnh vào chung 1 bài viết (Multi-Photo Post):
// Bước 1: Upload từng ảnh với `published=false` để lấy `media_fbid`.
// Bước 2: Gọi `POST /{page_id}/feed` với mảng `attached_media=[{"media_fbid":"..."}]`.
func (c *Client) PublishPhotoAlbum(ctx context.Context, pageID, token, message string, imageURLs []string) (*PublishOutcome, error) {
	outcome := &PublishOutcome{}
	attachedMedia := make([]map[string]string, 0, len(imageURLs))

	photoPath := fmt.Sprintf("%s/photos", pageID)
	for idx, imgURL := range imageURLs {
		var res map[string]any
		var metrics CallMetrics
		var err error

		if imgBytes, fileName := resolveLocalMediaBytes(imgURL); len(imgBytes) > 0 {
			fields := map[string]string{
				"published":    "false",
				"access_token": token,
			}
			res, metrics, err = c.PostMultipartChunk(ctx, photoPath, fields, "source", fileName, imgBytes)
		} else {
			form := url.Values{}
			form.Set("url", imgURL)
			form.Set("published", "false")
			form.Set("access_token", token)
			res, metrics, err = c.PostForm(ctx, photoPath, form)
		}
		outcome.FinalRateLimit = metrics.RateLimitUsage
		outcome.Steps = append(outcome.Steps, StepExecutionLog{
			Action:         fmt.Sprintf("UPLOAD_ALBUM_PHOTO_%d", idx+1),
			RequestSummary: fmt.Sprintf("POST /%s/photos (published=false) url=%s", pageID, imgURL),
			Metrics:        metrics,
			Error:          err,
		})
		if err != nil {
			return outcome, err
		}

		mediaID := extractFBID(res)
		if mediaID == "" {
			return outcome, errors.New("missing photo id from unpublished photo upload")
		}
		attachedMedia = append(attachedMedia, map[string]string{"media_fbid": mediaID})
	}

	attachedJSON, _ := json.Marshal(attachedMedia)
	feedForm := url.Values{}
	feedForm.Set("message", message)
	feedForm.Set("attached_media", string(attachedJSON))
	feedForm.Set("access_token", token)

	feedPath := fmt.Sprintf("%s/feed", pageID)
	res, metrics, err := c.PostForm(ctx, feedPath, feedForm)
	outcome.FinalRateLimit = metrics.RateLimitUsage
	outcome.Steps = append(outcome.Steps, StepExecutionLog{
		Action:         "PUBLISH_ALBUM_FEED",
		RequestSummary: fmt.Sprintf("POST /%s/feed attached_media_count=%d", pageID, len(attachedMedia)),
		Metrics:        metrics,
		Error:          err,
	})
	if err != nil {
		return outcome, err
	}

	outcome.FBPostID = extractFBID(res)
	return outcome, nil
}

// PublishVideoByURL đăng video từ đường dẫn CDN/S3 qua `POST /{page_id}/videos`.
func (c *Client) PublishVideoByURL(ctx context.Context, pageID, token, description, fileURL string) (*PublishOutcome, error) {
	form := url.Values{}
	form.Set("file_url", fileURL)
	form.Set("description", description)
	form.Set("access_token", token)

	path := fmt.Sprintf("%s/videos", pageID)
	res, metrics, err := c.PostForm(ctx, path, form)

	outcome := &PublishOutcome{
		FinalRateLimit: metrics.RateLimitUsage,
		Steps: []StepExecutionLog{
			{
				Action:         "PUBLISH_VIDEO_URL",
				RequestSummary: fmt.Sprintf("POST /%s/videos file_url=%s", pageID, fileURL),
				Metrics:        metrics,
				Error:          err,
			},
		},
	}
	if err != nil {
		return outcome, err
	}

	outcome.FBPostID = extractFBID(res)
	return outcome, nil
}

// PublishVideoChunked thực thi giao thức Chunked Upload 3 pha của Meta cho các file Video dung lượng lớn:
// Pha 1: `upload_phase=start` -> nhận `upload_session_id`, `start_offset`, `end_offset`
// Pha 2: `upload_phase=transfer` -> cắt nhỏ byte buffer và gửi tuần tự từng chunk
// Pha 3: `upload_phase=finish` -> hoàn tất và xuất bản video kèm caption
func (c *Client) PublishVideoChunked(ctx context.Context, pageID, token, description string, videoBytes []byte, chunkSize int) (*PublishOutcome, error) {
	if chunkSize <= 0 {
		chunkSize = 4 * 1024 * 1024
	}
	outcome := &PublishOutcome{}
	videoPath := fmt.Sprintf("%s/videos", pageID)

	// Pha 1: Start
	startForm := url.Values{}
	startForm.Set("upload_phase", "start")
	startForm.Set("file_size", strconv.Itoa(len(videoBytes)))
	startForm.Set("access_token", token)

	startRes, startMetrics, err := c.PostForm(ctx, videoPath, startForm)
	outcome.FinalRateLimit = startMetrics.RateLimitUsage
	outcome.Steps = append(outcome.Steps, StepExecutionLog{
		Action:         "CHUNK_VIDEO_START",
		RequestSummary: fmt.Sprintf("POST /%s/videos upload_phase=start size=%d", pageID, len(videoBytes)),
		Metrics:        startMetrics,
		Error:          err,
	})
	if err != nil {
		return outcome, err
	}

	sessionID, _ := startRes["upload_session_id"].(string)
	videoID, _ := startRes["video_id"].(string)
	if sessionID == "" {
		return outcome, errors.New("missing upload_session_id in video start phase")
	}

	// Pha 2: Transfer từng chunk (tôn trọng start_offset & end_offset do Meta trả về nếu có)
	totalSize := len(videoBytes)
	offset := parseOffset(startRes["start_offset"], 0)
	nextEnd := parseOffset(startRes["end_offset"], 0)
	chunkIndex := 1
	for offset < totalSize {
		end := offset + chunkSize
		if nextEnd > offset {
			end = nextEnd
		}
		if end > totalSize {
			end = totalSize
		}
		chunk := videoBytes[offset:end]

		fields := map[string]string{
			"upload_phase":      "transfer",
			"upload_session_id": sessionID,
			"start_offset":      strconv.Itoa(offset),
			"access_token":      token,
		}
		transferRes, transferMetrics, err := c.PostMultipartChunk(ctx, videoPath, fields, "video_file_chunk", "chunk.mp4", chunk)
		outcome.FinalRateLimit = transferMetrics.RateLimitUsage
		outcome.Steps = append(outcome.Steps, StepExecutionLog{
			Action:         fmt.Sprintf("CHUNK_VIDEO_TRANSFER_%d", chunkIndex),
			RequestSummary: fmt.Sprintf("POST /%s/videos transfer offset=%d..%d", pageID, offset, end),
			Metrics:        transferMetrics,
			Error:          err,
		})
		if err != nil {
			return outcome, err
		}
		newStart := parseOffset(transferRes["start_offset"], -1)
		newEnd := parseOffset(transferRes["end_offset"], -1)
		if newStart >= 0 && newEnd >= newStart && (newStart > offset || newStart == totalSize) {
			offset = newStart
			nextEnd = newEnd
		} else {
			offset = end
			nextEnd = 0
		}
		chunkIndex++
	}

	// Pha 3: Finish
	finishForm := url.Values{}
	finishForm.Set("upload_phase", "finish")
	finishForm.Set("upload_session_id", sessionID)
	finishForm.Set("description", description)
	finishForm.Set("published", "true")
	finishForm.Set("video_state", "PUBLISHED")
	finishForm.Set("access_token", token)

	finishRes, finishMetrics, err := c.PostForm(ctx, videoPath, finishForm)
	outcome.FinalRateLimit = finishMetrics.RateLimitUsage
	outcome.Steps = append(outcome.Steps, StepExecutionLog{
		Action:         "CHUNK_VIDEO_FINISH",
		RequestSummary: fmt.Sprintf("POST /%s/videos upload_phase=finish session=%s", pageID, sessionID),
		Metrics:        finishMetrics,
		Error:          err,
	})
	if err != nil {
		return outcome, err
	}

	outcome.FBPostID = extractFBID(finishRes)
	if outcome.FBPostID == "" {
		outcome.FBPostID = videoID
	}
	return outcome, nil
}

func parseOffset(val any, fallback int) int {
	switch v := val.(type) {
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	case float64:
		return int(v)
	case int:
		return v
	}
	return fallback
}

// PublishReelByURL thực hiện quy trình 3 bước đăng Facebook Reels qua `/{page_id}/video_reels`:
// Bước 1: `upload_phase=start` -> lấy `video_id`
// Bước 2: Gửi header `file_url` lên `/{video_id}` để Meta kéo video từ S3/CDN
// Bước 3: `upload_phase=finish` với `video_state=PUBLISHED` và `description`
func (c *Client) PublishReelByURL(ctx context.Context, pageID, token, description, videoURL string) (*PublishOutcome, error) {
	outcome := &PublishOutcome{}
	reelsPath := fmt.Sprintf("%s/video_reels", pageID)

	// Bước 1: Khởi tạo phiên đăng Reel
	startForm := url.Values{}
	startForm.Set("upload_phase", "start")
	startForm.Set("access_token", token)

	startRes, startMetrics, err := c.PostForm(ctx, reelsPath, startForm)
	outcome.FinalRateLimit = startMetrics.RateLimitUsage
	outcome.Steps = append(outcome.Steps, StepExecutionLog{
		Action:         "REEL_UPLOAD_START",
		RequestSummary: fmt.Sprintf("POST /%s/video_reels upload_phase=start", pageID),
		Metrics:        startMetrics,
		Error:          err,
	})
	if err != nil {
		return outcome, err
	}

	videoID, _ := startRes["video_id"].(string)
	if videoID == "" {
		return outcome, errors.New("missing video_id in reel start response")
	}

	// Bước 2: Yêu cầu Meta tải file video từ URL
	headers := map[string]string{
		"Authorization": "OAuth " + token,
		"file_url":      videoURL,
	}
	_, transferMetrics, err := c.PostJSON(ctx, videoID, map[string]any{}, headers)
	outcome.FinalRateLimit = transferMetrics.RateLimitUsage
	outcome.Steps = append(outcome.Steps, StepExecutionLog{
		Action:         "REEL_UPLOAD_TRANSFER",
		RequestSummary: fmt.Sprintf("POST /%s (header file_url=%s)", videoID, videoURL),
		Metrics:        transferMetrics,
		Error:          err,
	})
	if err != nil {
		return outcome, err
	}

	// Bước 3: Xuất bản Reel
	finishForm := url.Values{}
	finishForm.Set("upload_phase", "finish")
	finishForm.Set("video_id", videoID)
	finishForm.Set("video_state", "PUBLISHED")
	finishForm.Set("description", description)
	finishForm.Set("access_token", token)

	finishRes, finishMetrics, err := c.PostForm(ctx, reelsPath, finishForm)
	outcome.FinalRateLimit = finishMetrics.RateLimitUsage
	outcome.Steps = append(outcome.Steps, StepExecutionLog{
		Action:         "REEL_UPLOAD_FINISH",
		RequestSummary: fmt.Sprintf("POST /%s/video_reels upload_phase=finish video_id=%s", pageID, videoID),
		Metrics:        finishMetrics,
		Error:          err,
	})
	if err != nil {
		return outcome, err
	}

	outcome.FBPostID = extractFBID(finishRes)
	if outcome.FBPostID == "" {
		outcome.FBPostID = videoID
	}
	return outcome, nil
}

func extractFBID(res map[string]any) string {
	if res == nil {
		return ""
	}
	if postID, ok := res["post_id"].(string); ok && postID != "" {
		return postID
	}
	if id, ok := res["id"].(string); ok && id != "" {
		return id
	}
	return ""
}
