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

// TelegramNotifier sends real-time alert messages to operators on issues:
// 1. Post publishing failure after exhausting retries.
// 2. Access token expiration or revocation (e.g. Error Code 190).
// 3. API rate limit exceeding warning thresholds.
type TelegramNotifier struct {
	botToken   string
	chatID     string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewTelegramNotifier initializes a new Telegram notification service.
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

// Enabled checks whether the Telegram notifier is configured.
func (t *TelegramNotifier) Enabled() bool {
	return t.botToken != "" && t.chatID != ""
}

// SendAlert sends a Markdown-formatted alert to the configured Telegram chat.
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
