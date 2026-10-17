package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GeminiGenerator tích hợp Google Gemini API để tự động sáng tạo nội dung bài đăng Fanpage
// theo cấu trúc chuẩn Marketing (Hook - Body - Call To Action - Hashtags).
type GeminiGenerator struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewGeminiGenerator khởi tạo dịch vụ AI sinh nội dung bài viết.
func NewGeminiGenerator(apiKey, model string) *GeminiGenerator {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &GeminiGenerator{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GeneratePostContent sinh nội dung bài đăng Facebook từ chủ đề và giọng văn mong muốn.
func (g *GeminiGenerator) GeneratePostContent(ctx context.Context, topic, tone string) (string, string, error) {
	if tone == "" {
		tone = "chuyên nghiệp, thu hút tương tác"
	}

	// Nếu người dùng chưa cấu hình GEMINI_API_KEY, trả về mẫu bài viết thông minh (Template Fallback)
	if g.apiKey == "" || g.apiKey == "your_gemini_api_key_here" {
		fallback := fmt.Sprintf(
			"🔥 %s — Xu hướng không thể bỏ lỡ!\n\n"+
				"Bạn đã sẵn sàng bứt phá hiệu suất và tối ưu quy trình làm việc cùng giải pháp mới nhất về %s chưa?\n\n"+
				"✅ Tiết kiệm 80%% thời gian vận hành\n"+
				"✅ Chuẩn hóa quy trình tự động 24/7\n"+
				"✅ Tăng trưởng tương tác tự nhiên và bền vững\n\n"+
				"💬 Hãy để lại bình luận hoặc nhắn tin ngay cho Fanpage để nhận tư vấn chi tiết nhé!\n\n"+
				"#FacebookMarketing #Automation #TangTruong #XuHuong2026",
			strings.Title(topic), topic,
		)
		return fallback, "template-fallback", nil
	}

	prompt := fmt.Sprintf(
		"Bạn là chuyên gia Content Marketing trên Facebook. Hãy viết một bài đăng Fanpage bằng tiếng Việt về chủ đề: %q.\n"+
			"Giọng văn (Tone): %s.\n"+
			"Yêu cầu cấu trúc:\n"+
			"1. Dòng đầu tiên (Hook) thật ấn tượng, có emoji phù hợp.\n"+
			"2. Phần thân (Body) ngắn gọn, chia ý rõ ràng dễ đọc trên điện thoại.\n"+
			"3. Lời kêu gọi hành động (CTA) tự nhiên.\n"+
			"4. 4-5 hashtags liên quan ở cuối bài.\n"+
			"Chỉ trả về nội dung bài đăng, không thêm lời giải thích.",
		topic, tone,
	)

	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", g.model, g.apiKey)
	reqPayload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
	}

	rawBytes, _ := json.Marshal(reqPayload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBytes))
	if err != nil {
		return "", g.model, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", g.model, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", g.model, fmt.Errorf("gemini api status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", g.model, err
	}

	if len(parsed.Candidates) > 0 && len(parsed.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(parsed.Candidates[0].Content.Parts[0].Text), g.model, nil
	}
	return "", g.model, fmt.Errorf("empty response from gemini model %s", g.model)
}
