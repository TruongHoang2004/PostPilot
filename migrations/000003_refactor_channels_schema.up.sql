-- ============================================================================
-- Migration: 000003_refactor_channels_schema (UP)
-- Description: Chuyển đổi mô hình dữ liệu sang kiến trúc DDD đa nền tảng (Channel & Publishing Core)
-- ============================================================================

-- 1. Chuyển đổi bảng facebook_pages -> channels
ALTER TABLE facebook_pages RENAME TO channels;

ALTER TABLE channels RENAME COLUMN page_id TO external_id;
ALTER TABLE channels RENAME COLUMN page_name TO name;
ALTER TABLE channels ADD COLUMN IF NOT EXISTS platform VARCHAR(32) NOT NULL DEFAULT 'FACEBOOK';

ALTER TABLE channels DROP CONSTRAINT IF EXISTS facebook_pages_page_id_key;
ALTER TABLE channels ADD CONSTRAINT channels_platform_external_id_key UNIQUE (platform, external_id);

DROP INDEX IF EXISTS idx_facebook_pages_status;
CREATE INDEX IF NOT EXISTS idx_channels_status ON channels(platform, token_status, is_active);

-- 2. Chuyển đổi bảng posts sang tham chiếu channel_id & external_post_id
ALTER TABLE posts RENAME COLUMN page_uuid TO channel_id;
ALTER TABLE posts RENAME COLUMN fb_post_id TO external_post_id;

DROP INDEX IF EXISTS idx_posts_page_uuid;
CREATE INDEX IF NOT EXISTS idx_posts_channel_id ON posts(channel_id, created_at DESC);

-- 3. Chuyển đổi bảng publish_logs sang tham chiếu channel_id & error_code trừu tượng
ALTER TABLE publish_logs RENAME COLUMN page_uuid TO channel_id;
ALTER TABLE publish_logs RENAME COLUMN fb_error_code TO error_code;
ALTER TABLE publish_logs RENAME COLUMN fb_error_subcode TO error_subcode;

-- 4. Chuyển đổi bảng webhook_events
ALTER TABLE webhook_events RENAME COLUMN page_id TO external_id;

