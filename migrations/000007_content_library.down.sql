-- ============================================================================
-- Migration: 000007_content_library (DOWN)
-- ============================================================================
DROP INDEX IF EXISTS idx_posts_library_item;
ALTER TABLE posts DROP COLUMN IF EXISTS delete_after_publish;
ALTER TABLE posts DROP COLUMN IF EXISTS library_item_id;
DROP INDEX IF EXISTS idx_library_items_org_channel;
DROP INDEX IF EXISTS idx_library_items_org_state;
DROP TABLE IF EXISTS library_items;
