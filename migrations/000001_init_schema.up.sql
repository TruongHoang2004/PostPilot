-- ============================================================================
-- Migration: 000001_init_schema (UP)
-- Description: Khởi tạo các bảng cốt lõi cho hệ thống đăng bài tự động Facebook
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 1. Bảng quản lý danh sách Facebook Fanpage & Token đã mã hóa AES-256-GCM
CREATE TABLE IF NOT EXISTS facebook_pages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    page_id VARCHAR(64) NOT NULL UNIQUE,
    page_name VARCHAR(255) NOT NULL,
    category VARCHAR(128) DEFAULT '',
    encrypted_access_token TEXT NOT NULL,
    token_status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE', -- ACTIVE, EXPIRED, REVOKED, RATE_LIMITED
    token_expires_at TIMESTAMPTZ NULL,                 -- NULL nếu là Permanent Page Token
    last_checked_at TIMESTAMPTZ NULL,
    rate_limit_call_count INT NOT NULL DEFAULT 0,
    rate_limit_total_time INT NOT NULL DEFAULT 0,
    rate_limit_total_cputime INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_facebook_pages_status ON facebook_pages(token_status, is_active);

-- 2. Bảng quản lý tài nguyên Media (Ảnh, Video, Reels trên S3 / MinIO / CDN)
CREATE TABLE IF NOT EXISTS media_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_name VARCHAR(255) NOT NULL,
    storage_key VARCHAR(512) NOT NULL UNIQUE,
    public_url TEXT NOT NULL,
    mime_type VARCHAR(128) NOT NULL,
    file_size_bytes BIGINT NOT NULL DEFAULT 0,
    duration_seconds INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. Bảng quản lý Bài viết & Lịch đăng (Posts & Schedules)
CREATE TABLE IF NOT EXISTS posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    page_uuid UUID NOT NULL REFERENCES facebook_pages(id) ON DELETE CASCADE,
    content TEXT NOT NULL DEFAULT '',
    link_url TEXT NOT NULL DEFAULT '',
    media_type VARCHAR(32) NOT NULL DEFAULT 'NONE',    -- NONE, LINK, IMAGE, ALBUM, VIDEO, REEL
    media_urls JSONB NOT NULL DEFAULT '[]'::jsonb,     -- Mảng các URL ảnh/video trên S3/CDN
    status VARCHAR(32) NOT NULL DEFAULT 'DRAFT',       -- DRAFT, SCHEDULED, QUEUED, PROCESSING, PUBLISHED, FAILED, CANCELLED
    queue_tier VARCHAR(16) NOT NULL DEFAULT 'FAST',    -- FAST (Text/Link/Photo) hoặc HEAVY (Video/Reel)
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ NULL,
    fb_post_id VARCHAR(128) NOT NULL DEFAULT '',       -- ID bài viết do Meta Graph API trả về
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    next_retry_at TIMESTAMPTZ NULL,
    last_error_code INT NOT NULL DEFAULT 0,
    last_error_message TEXT NOT NULL DEFAULT '',
    created_by VARCHAR(128) NOT NULL DEFAULT 'system',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index phục vụ Cron Scheduler quét bài đến giờ đăng cực nhanh (SELECT ... FOR UPDATE SKIP LOCKED)
CREATE INDEX IF NOT EXISTS idx_posts_scheduler_scan
    ON posts(status, scheduled_at)
    WHERE status IN ('SCHEDULED', 'FAILED');

CREATE INDEX IF NOT EXISTS idx_posts_page_uuid ON posts(page_uuid, created_at DESC);

-- 4. Bảng lưu lịch sử gọi API sang Meta Graph (Publish Audit & Debug Logs)
CREATE TABLE IF NOT EXISTS publish_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    page_uuid UUID NOT NULL REFERENCES facebook_pages(id) ON DELETE CASCADE,
    attempt_number INT NOT NULL DEFAULT 1,
    action VARCHAR(64) NOT NULL,                       -- PUBLISH_FEED, UPLOAD_PHOTO, CHUNK_VIDEO_START, CHUNK_VIDEO_FINISH, etc.
    http_status INT NOT NULL DEFAULT 0,
    fb_error_code INT NOT NULL DEFAULT 0,
    fb_error_subcode INT NOT NULL DEFAULT 0,
    request_summary TEXT NOT NULL DEFAULT '',
    response_body TEXT NOT NULL DEFAULT '',
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_publish_logs_post_id ON publish_logs(post_id, created_at DESC);
