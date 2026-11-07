package facebook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// PublishOutcome contains the execution results of publishing to a Facebook Page.
type PublishOutcome struct {
	FBPostID       string
	Steps          []StepExecutionLog
	FinalRateLimit domain.RateLimitUsage
}

// StepExecutionLog records an individual API call step (e.g. uploading an individual photo before creating an album feed post).
type StepExecutionLog struct {
	Action         string
	RequestSummary string
	Metrics        CallMetrics
	Error          error
}

// PublishPost orchestrates publishing according to `post.MediaType`:
// - NONE / LINK  -> POST /{page_id}/feed
// - IMAGE        -> POST /{page_id}/photos
// - ALBUM        -> Upload individual photos `published=false` + POST /{page_id}/feed `attached_media`
// - VIDEO        -> POST /{page_id}/videos (uses Chunked Binary Upload if local file)
// - REEL         -> 3-phase flow for Facebook Reels (`/{page_id}/video_reels`)
func (c *Client) PublishPost(ctx context.Context, pageID, token string, post *domain.Post) (*PublishOutcome, error) {
	if post.MediaType == domain.MediaTypeNone || post.MediaType == domain.MediaTypeLink {
		return c.PublishTextOrLink(ctx, pageID, token, post.Content, post.LinkURL)
	}
	if len(post.MediaURLs) == 0 {
		return nil, errors.New("media URL is required")
	}
	for _, raw := range post.MediaURLs {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			return nil, errors.New("media must use a public HTTPS URL; re-upload legacy local media")
		}
	}
	switch post.MediaType {
	case domain.MediaTypeImage:
		return c.PublishSinglePhoto(ctx, pageID, token, post.Content, post.MediaURLs[0])
	case domain.MediaTypeAlbum:
		return c.PublishPhotoAlbum(ctx, pageID, token, post.Content, post.MediaURLs)
	case domain.MediaTypeVideo:
		return c.PublishVideoByURL(ctx, pageID, token, post.Content, post.MediaURLs[0])
	case domain.MediaTypeReel:
		return c.PublishReelByURL(ctx, pageID, token, post.Content, post.MediaURLs[0])
	default:
		return nil, fmt.Errorf("unsupported media type: %s", post.MediaType)
	}
}

// PublishTextOrLink publishes a text post or link preview post via `POST /{page_id}/feed`.
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

// PublishSinglePhoto publishes a single photo with caption via `POST /{page_id}/photos`.
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

// PublishSinglePhotoBytes uploads a photo from local binary data via `POST /{page_id}/photos` (multipart).
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

// PublishPhotoAlbum publishes multiple photos in a single feed post (Multi-Photo Post):
// Step 1: Upload each photo with `published=false` to obtain `media_fbid`.
// Step 2: Call `POST /{page_id}/feed` with `attached_media=[{"media_fbid":"..."}]`.
func (c *Client) PublishPhotoAlbum(ctx context.Context, pageID, token, message string, imageURLs []string) (*PublishOutcome, error) {
	outcome := &PublishOutcome{}
	attachedMedia := make([]map[string]string, 0, len(imageURLs))

	photoPath := fmt.Sprintf("%s/photos", pageID)
	for idx, imgURL := range imageURLs {
		var res map[string]any
		var metrics CallMetrics
		var err error

		form := url.Values{}
		form.Set("url", imgURL)
		form.Set("published", "false")
		form.Set("access_token", token)
		res, metrics, err = c.PostForm(ctx, photoPath, form)

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

// PublishVideoByURL publishes a video from a CDN/S3 URL via `POST /{page_id}/videos`.
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

// PublishVideoChunked executes Meta's 3-phase chunked upload protocol for large video files:
// Phase 1: `upload_phase=start` -> receives `upload_session_id`, `start_offset`, `end_offset`
// Phase 2: `upload_phase=transfer` -> splits byte buffer and transfers each chunk sequentially
// Phase 3: `upload_phase=finish` -> completes and publishes the video with description
func (c *Client) PublishVideoChunked(ctx context.Context, pageID, token, description string, videoBytes []byte, chunkSize int) (*PublishOutcome, error) {
	if chunkSize <= 0 {
		chunkSize = 4 * 1024 * 1024
	}
	outcome := &PublishOutcome{}
	videoPath := fmt.Sprintf("%s/videos", pageID)

	// Phase 1: Start
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

	// Phase 2: Transfer each chunk (respecting start_offset & end_offset returned by Meta)
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

// PublishReelByURL executes the 3-step publishing flow for Facebook Reels via `/{page_id}/video_reels`:
// Step 1: `upload_phase=start` -> get `video_id`
// Step 2: Send `file_url` header to `/{video_id}` for Meta to pull video from S3/CDN
// Step 3: `upload_phase=finish` with `video_state=PUBLISHED` and `description`
func (c *Client) PublishReelByURL(ctx context.Context, pageID, token, description, videoURL string) (*PublishOutcome, error) {
	outcome := &PublishOutcome{}
	reelsPath := fmt.Sprintf("%s/video_reels", pageID)

	// Step 1: Initialize Reel upload session
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

	// Hosted Reel uploads use the upload URL returned by Meta, not the Graph video node.
	uploadURL, _ := startRes["upload_url"].(string)
	parsed, parseErr := url.Parse(uploadURL)
	if parseErr != nil || parsed.Scheme != "https" || parsed.Host != "rupload.facebook.com" || parsed.User != nil {
		return outcome, errors.New("Meta returned an invalid Reel upload URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, http.NoBody)
	if err != nil {
		return outcome, err
	}
	request.Header.Set("Authorization", "OAuth "+token)
	request.Header.Set("file_url", videoURL)
	uploadClient := *c.httpClient
	uploadClient.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	uploadAdapter := *c
	uploadAdapter.httpClient = &uploadClient
	_, transferMetrics, err := uploadAdapter.doRequest(request, time.Now())

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

	// Step 3: Publish Reel
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
