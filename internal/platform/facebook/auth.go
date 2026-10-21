package facebook

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// ManagedPageInfo contains Page metadata and Page Access Token returned by `/me/accounts`.
type ManagedPageInfo struct {
	PageID      string `json:"page_id"`
	PageName    string `json:"page_name"`
	Category    string `json:"category"`
	AccessToken string `json:"access_token"`
}

// TokenDebugInfo contains token inspection results from `/debug_token`.
type TokenDebugInfo struct {
	IsValid   bool     `json:"is_valid"`
	AppID     string   `json:"app_id"`
	Type      string   `json:"type"`
	ExpiresAt int64    `json:"expires_at"` // 0 indicates never expires (Permanent Token)
	Scopes    []string `json:"scopes"`
}

// BuildOAuthLoginURL constructs the Facebook OAuth2 login dialog URL with page management scopes.
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

// ExchangeCodeForUserToken exchanges an OAuth authorization code for a short-lived User Access Token.
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

// ExchangeShortToLongLivedUserToken exchanges a short-lived User Token (1-2 hours) for a long-lived User Token (60 days).
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

// FetchManagedPages queries `/me/accounts` with a long-lived User Token to fetch pages
// along with Permanent Page Access Tokens (which never expire).
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

// DebugToken inspects the health and scopes of a target access token.
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
