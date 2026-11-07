-- ============================================================================
-- Migration: 000006_post_batches (UP)
-- Description: Bulk composition batches and scheduling lookup index
-- ============================================================================

CREATE TABLE IF NOT EXISTS post_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL DEFAULT '',
    kind VARCHAR(16) NOT NULL DEFAULT 'BULK' CHECK (kind IN ('BULK', 'FANOUT')), -- BULK (series) | FANOUT (one content, many channels)
    schedule_plan JSONB NULL,                           -- SchedulePlan used to distribute times, if any
    item_count INT NOT NULL DEFAULT 0,
    created_by VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_post_batches_org ON post_batches(organization_id, created_at DESC);

-- Range / per-channel lookups for calendar views and conflict detection
CREATE INDEX IF NOT EXISTS idx_posts_org_channel_sched ON posts(organization_id, channel_id, scheduled_at);

-- Backfill metadata rows for fan-out batches created before this table existed (idempotent)
INSERT INTO post_batches (id, organization_id, name, kind, item_count, created_by, created_at)
SELECT batch_id, organization_id, '', 'FANOUT', COUNT(*), MIN(created_by), MIN(created_at)
FROM posts
WHERE batch_id IS NOT NULL
GROUP BY batch_id, organization_id
ON CONFLICT (id) DO NOTHING;
