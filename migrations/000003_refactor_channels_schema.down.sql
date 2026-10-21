-- ============================================================================
-- Migration: 000003_refactor_channels_schema (DOWN)
-- Description: Rollback channels schema back to facebook_pages
-- ============================================================================

ALTER TABLE webhook_events RENAME COLUMN external_id TO page_id;

ALTER TABLE publish_logs RENAME COLUMN error_subcode TO fb_error_subcode;
ALTER TABLE publish_logs RENAME COLUMN error_code TO fb_error_code;
ALTER TABLE publish_logs RENAME COLUMN channel_id TO page_uuid;

DROP INDEX IF EXISTS idx_posts_channel_id;
ALTER TABLE posts RENAME COLUMN external_post_id TO fb_post_id;
ALTER TABLE posts RENAME COLUMN channel_id TO page_uuid;
CREATE INDEX IF NOT EXISTS idx_posts_page_uuid ON posts(page_uuid, created_at DESC);

DROP INDEX IF EXISTS idx_channels_status;
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_platform_external_id_key;
ALTER TABLE channels DROP COLUMN IF EXISTS platform;
ALTER TABLE channels RENAME COLUMN name TO page_name;
ALTER TABLE channels RENAME COLUMN external_id TO page_id;
ALTER TABLE channels ADD CONSTRAINT facebook_pages_page_id_key UNIQUE (page_id);
ALTER TABLE channels RENAME TO facebook_pages;
CREATE INDEX IF NOT EXISTS idx_facebook_pages_status ON facebook_pages(token_status, is_active);

