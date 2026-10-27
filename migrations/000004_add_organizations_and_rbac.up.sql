-- ============================================================================
-- Migration: 000004_add_organizations_and_rbac (UP)
-- Description: Multi-tenant team collaboration, channel groups, and RBAC schema
-- ============================================================================

-- 1. Create users table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL DEFAULT '',
    full_name VARCHAR(255) NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Create organizations table
CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. Create organization_members table
CREATE TABLE IF NOT EXISTS organization_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_role VARCHAR(32) NOT NULL DEFAULT 'MEMBER', -- OWNER, ADMIN, MEMBER
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, user_id)
);

-- 4. Create organization_invitations table
CREATE TABLE IF NOT EXISTS organization_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    org_role VARCHAR(32) NOT NULL DEFAULT 'MEMBER',
    invited_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 5. Create channel_groups table
CREATE TABLE IF NOT EXISTS channel_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    require_approval BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 6. Create channel_group_items table
CREATE TABLE IF NOT EXISTS channel_group_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_group_id UUID NOT NULL REFERENCES channel_groups(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (channel_group_id, channel_id)
);

-- 7. Create channel_group_permissions table
CREATE TABLE IF NOT EXISTS channel_group_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_group_id UUID NOT NULL REFERENCES channel_groups(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_role VARCHAR(32) NOT NULL DEFAULT 'VIEWER', -- MANAGER, APPROVER, EDITOR, VIEWER
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (channel_group_id, user_id)
);

-- 8. Seed default user, default organization, and default member
INSERT INTO users (id, email, full_name, password_hash)
VALUES ('00000000-0000-0000-0000-000000000001', 'admin@postpilot.local', 'System Admin', '')
ON CONFLICT (id) DO NOTHING;

INSERT INTO organizations (id, name, slug)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default Workspace', 'default-workspace')
ON CONFLICT (id) DO NOTHING;

INSERT INTO organization_members (organization_id, user_id, org_role)
VALUES ('00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'OWNER')
ON CONFLICT (organization_id, user_id) DO NOTHING;

-- 9. Add organization_id column to channels, posts, media_assets
ALTER TABLE channels ADD COLUMN IF NOT EXISTS organization_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001' REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE posts ADD COLUMN IF NOT EXISTS organization_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001' REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE media_assets ADD COLUMN IF NOT EXISTS organization_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001' REFERENCES organizations(id) ON DELETE CASCADE;

-- 10. Update channels uniqueness constraint for multi-tenancy
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_platform_external_id_key;
ALTER TABLE channels ADD CONSTRAINT channels_org_platform_external_id_key UNIQUE (organization_id, platform, external_id);

-- 11. Add workflow and approval columns to posts
ALTER TABLE posts ADD COLUMN IF NOT EXISTS batch_id UUID NULL;
ALTER TABLE posts ADD COLUMN IF NOT EXISTS channel_group_id UUID NULL REFERENCES channel_groups(id) ON DELETE SET NULL;
ALTER TABLE posts ADD COLUMN IF NOT EXISTS submitted_by UUID NULL REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE posts ADD COLUMN IF NOT EXISTS reviewed_by UUID NULL REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE posts ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ NULL;
ALTER TABLE posts ADD COLUMN IF NOT EXISTS rejection_note TEXT NOT NULL DEFAULT '';

-- 12. Create post_approvals table
CREATE TABLE IF NOT EXISTS post_approvals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    reviewer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action VARCHAR(32) NOT NULL, -- APPROVED, REJECTED
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 13. Create audit_logs table
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(64) NOT NULL,
    entity_type VARCHAR(64) NOT NULL,
    entity_id VARCHAR(128) NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 14. Performance and lookup indexes
CREATE INDEX IF NOT EXISTS idx_channels_org_id ON channels(organization_id);
CREATE INDEX IF NOT EXISTS idx_posts_org_id ON posts(organization_id);
CREATE INDEX IF NOT EXISTS idx_posts_batch_id ON posts(batch_id) WHERE batch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_posts_channel_group_id ON posts(channel_group_id) WHERE channel_group_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_logs_org_id ON audit_logs(organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_channel_groups_org_id ON channel_groups(organization_id);
CREATE INDEX IF NOT EXISTS idx_organization_members_user_id ON organization_members(user_id);
CREATE INDEX IF NOT EXISTS idx_post_approvals_post_id ON post_approvals(post_id);
