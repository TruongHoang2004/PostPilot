package facebook

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// ManagedPageInfo chứa thông tin Fanpage và Permanent Page Access Token trả về từ `/me/accounts`.
type ManagedPageInfo struct {
	PageID      string `json:"page_id"`
	PageName    string `json:"page_name"`
	Category    string `json:"category"`
	AccessToken string `json:"access_token"`
}

// TokenDebugInfo chứa kết quả kiểm tra sức khỏe của Access Token từ `/debug_token`.
type TokenDebugInfo struct {
	IsValid   bool     `json:"is_valid"`
	AppID     string   `json:"app_id"`
	Type      string   `json:"type"`
	ExpiresAt int64    `json:"expires_at"` // 0 nghĩa là Never Expires (Vĩnh viễn)
	Scopes    []string `json:"scopes"`
}

// BuildOAuthLoginURL tạo đường dẫn đăng nhập Facebook OAuth2 với đầy đủ quyền quản lý Fanpage.
func (c *Client) BuildOAuthLoginURL(appID, redirectURI, state string) string {
	scopes := []string{
		"pages_manage_posts",
		"pages_read_engagement",
		"pages_show_list",
	}
	q := url.Values{}
	q.Set("client_id", appID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("scope", strings.Join(scopes, ","))
	q.Set("response_type", "code")

	return fmt.Sprintf("https://www.facebook.com/%s/dialog/oauth?%s", c.version, q.Encode())
}

// ExchangeCodeForUserToken đổi OAuth authorization `code` sang Short-lived User Access Token.
func (c *Client) ExchangeCodeForUserToken(ctx context.Context, appID, appSecret, redirectURI, code string) (string, error) {
	q := url.Values{}
	q.Set("client_id", appID)
	q.Set("client_secret", appSecret)
	q.Set("redirect_uri", redirectURI)
	q.Set("code", code)

	res, _, err := c.Get(ctx, "oauth/access_token", q)
	if err != nil {
		return "", err
	}
	token, _ := res["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("missing access_token in oauth code exchange response")
	}
	return token, nil
}

// ExchangeShortToLongLivedUserToken đổi Short-lived User Token (1-2h) sang Long-lived User Token (60 ngày).
func (c *Client) ExchangeShortToLongLivedUserToken(ctx context.Context, appID, appSecret, shortLivedUserToken string) (string, error) {
	q := url.Values{}
	q.Set("grant_type", "fb_exchange_token")
	q.Set("client_id", appID)
	q.Set("client_secret", appSecret)
	q.Set("fb_exchange_token", shortLivedUserToken)

	res, _, err := c.Get(ctx, "oauth/access_token", q)
	if err != nil {
		return "", err
	}
	token, _ := res["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("missing access_token in fb_exchange_token response")
	}
	return token, nil
}

// FetchManagedPages gọi `/me/accounts` bằng Long-lived User Token để lấy danh sách Fanpage
// kèm theo **Permanent Page Access Token** (Token vĩnh viễn không hết hạn).
func (c *Client) FetchManagedPages(ctx context.Context, longLivedUserToken string) ([]ManagedPageInfo, error) {
	q := url.Values{}
	q.Set("fields", "id,name,category,access_token")
	q.Set("access_token", longLivedUserToken)

	res, _, err := c.Get(ctx, "me/accounts", q)
	if err != nil {
		return nil, err
	}

	rawData, ok := res["data"].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected /me/accounts response format")
	}

	pages := make([]ManagedPageInfo, 0, len(rawData))
	for _, item := range rawData {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		name, _ := m["name"].(string)
		cat, _ := m["category"].(string)
		tok, _ := m["access_token"].(string)
		if id != "" && tok != "" {
			pages = append(pages, ManagedPageInfo{
				PageID:      id,
				PageName:    name,
				Category:    cat,
				AccessToken: tok,
			})
		}
	}
	return pages, nil
}

// DebugToken kiểm tra trạng thái sống/chết của 1 Page Access Token.
func (c *Client) DebugToken(ctx context.Context, targetToken, appAccessToken string) (*TokenDebugInfo, error) {
	if appAccessToken == "" {
		appAccessToken = targetToken
	}
	q := url.Values{}
	q.Set("input_token", targetToken)
	q.Set("access_token", appAccessToken)

	res, _, err := c.Get(ctx, "debug_token", q)
	if err != nil {
		return nil, err
	}

	data, ok := res["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing data field in debug_token response")
	}

	info := &TokenDebugInfo{}
	if v, ok := data["is_valid"].(bool); ok {
		info.IsValid = v
	}
	if v, ok := data["app_id"].(string); ok {
		info.AppID = v
	}
	if v, ok := data["type"].(string); ok {
		info.Type = v
	}
	if v, ok := data["expires_at"].(float64); ok {
		info.ExpiresAt = int64(v)
	}
	if rawScopes, ok := data["scopes"].([]any); ok {
		for _, s := range rawScopes {
			if str, ok := s.(string); ok {
				info.Scopes = append(info.Scopes, str)
			}
		}
	}
	return info, nil
}
