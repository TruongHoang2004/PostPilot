-- ============================================================================
-- Migration: 000002_add_rate_limits_and_webhooks (DOWN)
-- Description: Rollback bảng webhook_events và ai_generation_logs
-- ============================================================================

DROP TABLE IF EXISTS ai_generation_logs;
DROP TABLE IF EXISTS webhook_events;
