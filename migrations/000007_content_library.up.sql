-- ============================================================================
-- Migration: 000007_content_library (UP)
-- Description: Reusable "ready-made" posts (library) and the link from a scheduled post back to
--              its library item, including the "delete after publish" (soft delete) option.
-- ============================================================================

CREATE TABLE IF NOT EXISTS library_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_id UUID NULL REFERENCES channels(id) ON DELETE SET NULL, -- optional preferred Fanpage
    title VARCHAR(255) NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    link_url TEXT NOT NULL DEFAULT '',
    media_type VARCHAR(16) NOT NULL DEFAULT 'NONE',
    media_urls JSONB NOT NULL DEFAULT '[]'::jsonb,
    publish_count INT NOT NULL DEFAULT 0,
    last_published_at TIMESTAMPTZ NULL,
    archived_at TIMESTAMPTZ NULL,                                 -- soft delete ("Đã đăng" section)
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_library_items_org_state ON library_items(organization_id, archived_at, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_library_items_org_channel ON library_items(organization_id, channel_id);

ALTER TABLE posts ADD COLUMN IF NOT EXISTS library_item_id UUID NULL REFERENCES library_items(id) ON DELETE SET NULL;
ALTER TABLE posts ADD COLUMN IF NOT EXISTS delete_after_publish BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_posts_library_item ON posts(library_item_id) WHERE library_item_id IS NOT NULL;
