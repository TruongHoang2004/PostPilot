package facebook

import (
	"fmt"
	"net/http"
)

// GraphAPIError đại diện cho cấu trúc lỗi trả về từ Meta Graph API.
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

// IsTokenInvalid kiểm tra xem lỗi có phải do Page Access Token hết hạn, đổi mật khẩu hoặc bị thu hồi quyền không.
// Theo tài liệu Meta: Code 190 (OAuthException) hoặc Code 102 (Session key invalid).
func (e *GraphAPIError) IsTokenInvalid() bool {
	return e.Code == 190 || e.Code == 102
}

// IsRateLimited kiểm tra xem lỗi có phải do chạm ngưỡng giới hạn tần suất (Rate Limit) của Meta hay không.
// Code 4 (App rate limit), 17 (User request limit), 32 (Page request limit), 613 (Calls to this api have exceeded the rate limit), 80001 (Page level throttling).
func (e *GraphAPIError) IsRateLimited() bool {
	switch e.Code {
	case 4, 17, 32, 613, 80001:
		return true
	default:
		return e.HTTPStatus == http.StatusTooManyRequests
	}
}

// IsRetryable quyết định xem Worker có nên thử đăng lại (Exponential Backoff Retry) hay dừng hẳn.
func (e *GraphAPIError) IsRetryable() bool {
	// Lỗi Token (190) hoặc lỗi thiếu quyền (10, 200) hoặc bị chặn nội dung spam (368) -> KHÔNG RETRY
	if e.IsTokenInvalid() || e.Code == 10 || (e.Code >= 200 && e.Code < 300) || e.Code == 368 {
		return false
	}
	// Rate limit -> Nên Retry sau khoảng thời gian chờ dài hơn
	if e.IsRateLimited() {
		return true
	}
	// Lỗi tạm thời từ phía Meta (Code 1: Unknown, Code 2: Service temporarily unavailable) hoặc HTTP 5xx
	if e.Code == 1 || e.Code == 2 || e.HTTPStatus >= 500 {
		return true
	}
	return false
}
