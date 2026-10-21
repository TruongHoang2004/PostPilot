package facebook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

type handlerRoundTripper struct {
	handler http.HandlerFunc
}

func (hrt handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	hrt.handler(rec, req)
	return rec.Result(), nil
}

func newMockClient(fn http.HandlerFunc) *Client {
	return NewClient("https://graph.facebook.com", "v20.0").WithHTTPClient(&http.Client{
		Transport: handlerRoundTripper{handler: fn},
	})
}

func TestPublishPhotoAlbumAndRateLimitParsing(t *testing.T) {
	var photoUploads int32

	client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Page-Usage", `{"call_count":35,"total_cputime":12,"total_time":24}`)

		if strings.HasSuffix(r.URL.Path, "/123456/photos") {
			count := atomic.AddInt32(&photoUploads, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "photo_fbid_" + string(rune('0'+count)),
			})
			return
		}

		if strings.HasSuffix(r.URL.Path, "/123456/feed") {
			_ = r.ParseForm()
			attached := r.FormValue("attached_media")
			if !strings.Contains(attached, "photo_fbid_1") || !strings.Contains(attached, "photo_fbid_2") {
				t.Errorf("expected attached_media to contain uploaded photo IDs, got %s", attached)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "123456_999888777",
			})
			return
		}

		http.NotFound(w, r)
	})

	post := &domain.Post{
		MediaType: domain.MediaTypeAlbum,
		Content:   "Autumn 2026 Collection",
		MediaURLs: []string{
			"https://cdn.example.com/img1.jpg",
			"https://cdn.example.com/img2.jpg",
		},
	}

	outcome, err := client.PublishPost(context.Background(), "123456", "EAAB_test_token", post)
	if err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}

	if outcome.FBPostID != "123456_999888777" {
		t.Fatalf("expected FBPostID 123456_999888777, got %s", outcome.FBPostID)
	}
	if len(outcome.Steps) != 3 {
		t.Fatalf("expected 3 steps (2 photo uploads + 1 feed post), got %d", len(outcome.Steps))
	}
	if outcome.FinalRateLimit.CallCount != 35 {
		t.Fatalf("expected rate limit call_count=35, got %d", outcome.FinalRateLimit.CallCount)
	}
}

func TestPublishVideoChunked(t *testing.T) {
	var transferCount int32

	client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			atomic.AddInt32(&transferCount, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"start_offset": "0",
				"end_offset":   "10",
			})
			return
		}

		_ = r.ParseForm()
		phase := r.FormValue("upload_phase")
		switch phase {
		case "start":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"upload_session_id": "sess_777",
				"video_id":          "vid_888",
			})
		case "finish":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"post_id": "123456_vid_888",
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	dummyVideo := []byte("0123456789ABCDEFGHIJ") // 20 bytes, chunkSize = 8 -> 3 chunks

	outcome, err := client.PublishVideoChunked(context.Background(), "123456", "EAAB_token", "Video test", dummyVideo, 8)
	if err != nil {
		t.Fatalf("unexpected chunked video error: %v", err)
	}

	if atomic.LoadInt32(&transferCount) != 3 {
		t.Fatalf("expected 3 chunk transfers, got %d", transferCount)
	}
	if outcome.FBPostID != "123456_vid_888" {
		t.Fatalf("expected FBPostID 123456_vid_888, got %s", outcome.FBPostID)
	}
}

func TestGraphAPIErrorClassification(t *testing.T) {
	tokenErr := &GraphAPIError{HTTPStatus: 400, Code: 190, ErrorSubcode: 463, Message: "Session has expired"}
	if !tokenErr.IsTokenInvalid() || tokenErr.IsRetryable() {
		t.Fatalf("code 190 must be classified as token invalid and non-retryable")
	}

	rateErr := &GraphAPIError{HTTPStatus: 400, Code: 32, Message: "Page request limit reached"}
	if !rateErr.IsRateLimited() || !rateErr.IsRetryable() {
		t.Fatalf("code 32 must be classified as rate-limited and retryable")
	}
}

func TestPublisherAdapter(t *testing.T) {
	client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/debug_token") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"app_id":     "123456",
					"is_valid":   true,
					"expires_at": 1799999999,
					"scopes":     []string{"pages_manage_posts"},
				},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/feed") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "123456_fb_feed_999",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	adapter := NewPublisherAdapter(client)
	if adapter.Platform() != domain.PlatformFacebook {
		t.Fatalf("expected platform FACEBOOK, got %s", adapter.Platform())
	}

	// Verify Token
	health, err := adapter.VerifyToken(context.Background(), "mock_access_token")
	if err != nil {
		t.Fatalf("unexpected verify token error: %v", err)
	}
	if !health.IsValid || health.ExpiresAt != 1799999999 {
		t.Fatalf("unexpected token health: %+v", health)
	}

	// Publish Post via Adapter
	res, err := adapter.Publish(context.Background(), domain.PublishRequest{
		Channel: domain.Channel{
			Platform:   domain.PlatformFacebook,
			ExternalID: "123456",
		},
		AccessToken: "mock_token",
		Post: &domain.Post{
			Content:   "Testing DDD PublisherAdapter",
			MediaType: domain.MediaTypeNone,
		},
	})
	if err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}
	if res.ExternalPostID != "123456_fb_feed_999" {
		t.Fatalf("expected external post ID 123456_fb_feed_999, got %s", res.ExternalPostID)
	}
}
