package facebook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// Client encapsulates HTTP communication with Meta Graph API.
type Client struct {
	baseURL    string
	version    string
	httpClient *http.Client
}

// CallMetrics records details of a single API call to Meta to persist in publish_logs.
type CallMetrics struct {
	HTTPStatus     int
	RawResponse    string
	DurationMs     int64
	RateLimitUsage domain.RateLimitUsage
}

// NewClient initializes a Meta Graph API Client.
func NewClient(baseURL, version string) *Client {
	if baseURL == "" {
		baseURL = "https://graph.facebook.com"
	}
	if version == "" {
		version = "v20.0"
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		version: version,
		httpClient: &http.Client{
			Timeout: 300 * time.Second,
		},
	}
}

// WithHTTPClient configures a custom *http.Client or RoundTripper (useful for testing or proxies).
func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	if hc != nil {
		c.httpClient = hc
	}
	return c
}

func (c *Client) endpoint(path string) string {
	path = strings.TrimLeft(path, "/")
	base := c.baseURL
	// Per Meta Graph API specs, video upload endpoints (/{page_id}/videos)
	// must be routed to graph-video.facebook.com instead of graph.facebook.com.
	if base == "https://graph.facebook.com" && strings.HasSuffix(path, "/videos") {
		base = "https://graph-video.facebook.com"
	}
	return fmt.Sprintf("%s/%s/%s", base, c.version, path)
}

// PostForm sends an application/x-www-form-urlencoded POST request to Meta Graph API.
func (c *Client) PostForm(ctx context.Context, path string, form url.Values) (map[string]any, CallMetrics, error) {
	start := time.Now()
	fullURL := c.endpoint(path)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, CallMetrics{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return c.doRequest(req, start)
}

// PostJSON sends an application/json POST request to Meta Graph API.
func (c *Client) PostJSON(ctx context.Context, path string, payload map[string]any, extraHeaders map[string]string) (map[string]any, CallMetrics, error) {
	start := time.Now()
	fullURL := c.endpoint(path)

	rawBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, CallMetrics{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(rawBytes))
	if err != nil {
		return nil, CallMetrics{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	return c.doRequest(req, start)
}

// PostMultipartChunk uploads a single binary chunk during chunked video uploads.
func (c *Client) PostMultipartChunk(ctx context.Context, path string, fields map[string]string, fieldName, fileName string, chunkData []byte) (map[string]any, CallMetrics, error) {
	start := time.Now()
	fullURL := c.endpoint(path)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}

	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		return nil, CallMetrics{}, err
	}
	if _, err := part.Write(chunkData); err != nil {
		return nil, CallMetrics{}, err
	}
	_ = writer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, &body)
	if err != nil {
		return nil, CallMetrics{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return c.doRequest(req, start)
}

// Get sends a GET request with query params to the Meta Graph API.
func (c *Client) Get(ctx context.Context, path string, query url.Values) (map[string]any, CallMetrics, error) {
	start := time.Now()
	fullURL := c.endpoint(path)
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, CallMetrics{}, err
	}

	return c.doRequest(req, start)
}

func (c *Client) doRequest(req *http.Request, start time.Time) (map[string]any, CallMetrics, error) {
	resp, err := c.httpClient.Do(req)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return nil, CallMetrics{DurationMs: durationMs}, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	rawStr := string(respBytes)
	usage := ParseRateLimitHeaders(resp.Header)

	metrics := CallMetrics{
		HTTPStatus:     resp.StatusCode,
		RawResponse:    rawStr,
		DurationMs:     durationMs,
		RateLimitUsage: usage,
	}

	var parsed map[string]any
	_ = json.Unmarshal(respBytes, &parsed)

	if resp.StatusCode >= 400 || (parsed != nil && parsed["error"] != nil) {
		return nil, metrics, parseGraphError(resp.StatusCode, rawStr, parsed)
	}

	return parsed, metrics, nil
}

// ParseRateLimitHeaders reads rate limit usage metrics from Meta's X-Page-Usage or X-App-Usage headers.
// Example header: `{"call_count":28,"total_cputime":15,"total_time":22}`
func ParseRateLimitHeaders(h http.Header) domain.RateLimitUsage {
	var usage domain.RateLimitUsage

	pageHeader := h.Get("X-Page-Usage")
	if pageHeader != "" {
		// X-Page-Usage can be a JSON object or a map keyed by page_id
		if err := json.Unmarshal([]byte(pageHeader), &usage); err == nil && usage.MaxPercent() > 0 {
			return usage
		}
		var nested map[string]domain.RateLimitUsage
		if err := json.Unmarshal([]byte(pageHeader), &nested); err == nil {
			for _, u := range nested {
				return u
			}
		}
	}

	appHeader := h.Get("X-App-Usage")
	if appHeader != "" {
		_ = json.Unmarshal([]byte(appHeader), &usage)
	}
	return usage
}

func parseGraphError(httpStatus int, rawBody string, parsed map[string]any) *GraphAPIError {
	fbErr := &GraphAPIError{
		HTTPStatus: httpStatus,
		Message:    "unexpected error from meta graph api",
		RawBody:    rawBody,
	}

	if parsed == nil {
		return fbErr
	}

	errObj, ok := parsed["error"].(map[string]any)
	if !ok {
		return fbErr
	}

	if msg, ok := errObj["message"].(string); ok {
		fbErr.Message = msg
	}
	if typ, ok := errObj["type"].(string); ok {
		fbErr.Type = typ
	}
	if code, ok := errObj["code"].(float64); ok {
		fbErr.Code = int(code)
	}
	if subcode, ok := errObj["error_subcode"].(float64); ok {
		fbErr.ErrorSubcode = int(subcode)
	}
	if trace, ok := errObj["fbtrace_id"].(string); ok {
		fbErr.FBTraceID = trace
	}
	return fbErr
}
