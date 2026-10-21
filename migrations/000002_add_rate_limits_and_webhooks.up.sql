-- ============================================================================
-- Migration: 000002_add_rate_limits_and_webhooks (UP)
-- Description: Add webhook events table and AI generation logs table
-- ============================================================================

CREATE TABLE IF NOT EXISTS webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    object_type VARCHAR(64) NOT NULL,                  -- page, permissions, video_reels
    page_id VARCHAR(64) NOT NULL DEFAULT '',
    event_type VARCHAR(128) NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    processed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_processed ON webhook_events(processed, created_at);

CREATE TABLE IF NOT EXISTS ai_generation_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic TEXT NOT NULL,
    tone VARCHAR(64) NOT NULL DEFAULT 'professional',
    generated_content TEXT NOT NULL,
    model_used VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
