-- ============================================================================
-- Migration: 000002_add_rate_limits_and_webhooks (DOWN)
-- Description: Rollback webhook_events and ai_generation_logs tables
-- ============================================================================

DROP TABLE IF EXISTS ai_generation_logs;
DROP TABLE IF EXISTS webhook_events;
