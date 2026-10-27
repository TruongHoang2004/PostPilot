-- ============================================================================
-- Migration: 000004_add_organizations_and_rbac (DOWN)
-- Description: Rollback multi-tenant team collaboration, channel groups, and RBAC
-- ============================================================================

-- 1. Drop indexes
DROP INDEX IF EXISTS idx_post_approvals_post_id;
DROP INDEX IF EXISTS idx_organization_members_user_id;
DROP INDEX IF EXISTS idx_channel_groups_org_id;
DROP INDEX IF EXISTS idx_audit_logs_org_id;
DROP INDEX IF EXISTS idx_posts_channel_group_id;
DROP INDEX IF EXISTS idx_posts_batch_id;
DROP INDEX IF EXISTS idx_posts_org_id;
DROP INDEX IF EXISTS idx_channels_org_id;

-- 2. Drop columns from posts
ALTER TABLE posts DROP COLUMN IF EXISTS rejection_note;
ALTER TABLE posts DROP COLUMN IF EXISTS reviewed_at;
ALTER TABLE posts DROP COLUMN IF EXISTS reviewed_by;
ALTER TABLE posts DROP COLUMN IF EXISTS submitted_by;
ALTER TABLE posts DROP COLUMN IF EXISTS channel_group_id;
ALTER TABLE posts DROP COLUMN IF EXISTS batch_id;
ALTER TABLE posts DROP COLUMN IF EXISTS organization_id;

-- 3. Restore channels uniqueness constraint and drop organization_id
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_org_platform_external_id_key;
ALTER TABLE channels ADD CONSTRAINT channels_platform_external_id_key UNIQUE (platform, external_id);
ALTER TABLE channels DROP COLUMN IF EXISTS organization_id;

-- 4. Drop organization_id from media_assets
ALTER TABLE media_assets DROP COLUMN IF EXISTS organization_id;

-- 5. Drop new tables in reverse order of foreign key dependency
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS post_approvals;
DROP TABLE IF EXISTS channel_group_permissions;
DROP TABLE IF EXISTS channel_group_items;
DROP TABLE IF EXISTS channel_groups;
DROP TABLE IF EXISTS organization_invitations;
DROP TABLE IF EXISTS organization_members;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS users;
