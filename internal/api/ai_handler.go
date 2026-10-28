package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleAIGenerateContent generates post text and captions automatically via AI (Google Gemini).
func (s *Server) handleAIGenerateContent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Topic string `json:"topic"`
		Tone  string `json:"tone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Topic) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "topic is required"})
		return
	}

	content, modelUsed, err := s.aiGen.GeneratePostContent(r.Context(), req.Topic, req.Tone)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"topic":             req.Topic,
		"tone":              req.Tone,
		"model_used":        modelUsed,
		"generated_content": content,
	})
}
