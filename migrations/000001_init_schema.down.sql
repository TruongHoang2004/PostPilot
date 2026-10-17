-- ============================================================================
-- Migration: 000001_init_schema (DOWN)
-- Description: Rollback các bảng cốt lõi
-- ============================================================================

DROP TABLE IF EXISTS publish_logs;
DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS media_assets;
DROP TABLE IF EXISTS facebook_pages;
