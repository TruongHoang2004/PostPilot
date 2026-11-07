-- ============================================================================
-- Migration: 000006_post_batches (DOWN)
-- ============================================================================
DROP INDEX IF EXISTS idx_posts_org_channel_sched;
DROP INDEX IF EXISTS idx_post_batches_org;
DROP TABLE IF EXISTS post_batches;
