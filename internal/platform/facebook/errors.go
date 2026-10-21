package facebook

import (
	"fmt"
	"net/http"
)

// GraphAPIError represents an error response structure returned by Meta Graph API.
type GraphAPIError struct {
	HTTPStatus   int    `json:"http_status"`
	Message      string `json:"message"`
	Type         string `json:"type"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode"`
	FBTraceID    string `json:"fbtrace_id"`
	RawBody      string `json:"raw_body"`
}

func (e *GraphAPIError) Error() string {
	return fmt.Sprintf("meta graph api error (http=%d, code=%d, subcode=%d, type=%s, trace=%s): %s",
		e.HTTPStatus, e.Code, e.ErrorSubcode, e.Type, e.FBTraceID, e.Message)
}

// ErrorCode returns the standardized error code from the platform (implements domain.PublisherError).
func (e *GraphAPIError) ErrorCode() int {
	return e.Code
}

// Subcode returns the secondary subcode from the platform.
func (e *GraphAPIError) Subcode() int {
	return e.ErrorSubcode
}

// IsTokenInvalid checks if the error indicates an expired, revoked, or invalid access token.
// Per Meta documentation: Code 190 (OAuthException) or Code 102 (Session key invalid).
func (e *GraphAPIError) IsTokenInvalid() bool {
	return e.Code == 190 || e.Code == 102
}

// IsRateLimited checks if the error is due to hitting Meta rate limits or throttling.
// Code 4 (App rate limit), 17 (User request limit), 32 (Page request limit), 613 (Calls exceeded), 80001 (Page throttling).
func (e *GraphAPIError) IsRateLimited() bool {
	switch e.Code {
	case 4, 17, 32, 613, 80001:
		return true
	default:
		return e.HTTPStatus == http.StatusTooManyRequests
	}
}

// IsRetryable determines whether the worker should retry with exponential backoff or fail permanently.
func (e *GraphAPIError) IsRetryable() bool {
	// Token error (190), permission denied (10, 200-299), or spam block (368) -> DO NOT RETRY
	if e.IsTokenInvalid() || e.Code == 10 || (e.Code >= 200 && e.Code < 300) || e.Code == 368 {
		return false
	}
	// Rate limit -> Should retry after a backoff delay
	if e.IsRateLimited() {
		return true
	}
	// Transient errors from Meta (Code 1: Unknown, Code 2: Service temporarily unavailable) or HTTP 5xx
	if e.Code == 1 || e.Code == 2 || e.HTTPStatus >= 500 {
		return true
	}
	return false
}
