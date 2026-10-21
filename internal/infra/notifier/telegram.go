package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// TelegramNotifier gửi tin nhắn cảnh báo thời gian thực tới Admin khi có sự cố:
// 1. Bài đăng thất bại sau khi đã hết số lần Retry.
// 2. Page Access Token bị hết hạn hoặc thu hồi quyền (Error Code 190).
// 3. Tần suất gọi API (Rate Limit) vượt ngưỡng cảnh báo.
type TelegramNotifier struct {
	botToken   string
	chatID     string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewTelegramNotifier khởi tạo dịch vụ thông báo Telegram.
func NewTelegramNotifier(botToken, chatID string, logger *slog.Logger) *TelegramNotifier {
	return &TelegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Enabled kiểm tra xem cấu hình Telegram đã được bật hay chưa.
func (t *TelegramNotifier) Enabled() bool {
	return t.botToken != "" && t.chatID != ""
}

// SendAlert gửi cảnh báo định dạng Markdown tới nhóm/kênh Telegram của đội vận hành.
func (t *TelegramNotifier) SendAlert(ctx context.Context, title string, details string) error {
	if !t.Enabled() {
		t.logger.Debug("Telegram notifier not configured, logging alert locally", "title", title, "details", details)
		return nil
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
	body := map[string]any{
		"chat_id":    t.chatID,
		"text":       fmt.Sprintf("🚨 *[PostPilot Alert]*\n*%s*\n\n%s", title, details),
		"parse_mode": "Markdown",
	}

	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram api returned status %d", resp.StatusCode)
	}
	return nil
}
