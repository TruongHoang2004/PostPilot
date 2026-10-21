package api

import (
	"net/http"
	"time"

	"github.com/TruongHoang2004/PostPilot/internal/domain"
)

// handleHealthCheck checks health and connectivity for API Server and Redis queues.
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	redisErr := s.redis.Ping(ctx)
	redisMode := "redis-tcp"
	if s.redis.IsMemoryFallback() {
		redisMode = "in-memory-fallback"
	}

	fastDepth, _ := s.redis.QueueDepth(ctx, domain.QueueTierFast)
	heavyDepth, _ := s.redis.QueueDepth(ctx, domain.QueueTierHeavy)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":            "healthy",
		"service":           "postpilot-api",
		"redis_mode":        redisMode,
		"redis_ok":          redisErr == nil,
		"fast_queue_depth":  fastDepth,
		"heavy_queue_depth": heavyDepth,
		"timestamp":         time.Now().UTC(),
	})
}

// handleGetStats returns aggregated system statistics.
func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := s.store.GetSystemStats(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	stats.FastQueueDepth, _ = s.redis.QueueDepth(ctx, domain.QueueTierFast)
	stats.HeavyQueueDepth, _ = s.redis.QueueDepth(ctx, domain.QueueTierHeavy)

	writeJSON(w, http.StatusOK, stats)
}

