export interface SystemHealth {
  status: string;
  service: string;
  redis_mode: string;
  redis_ok: boolean;
  fast_queue_depth: number;
  heavy_queue_depth: number;
  timestamp: string;
}

export interface SystemStats {
  total_channels: number;
  active_channels: number;
  scheduled_posts: number;
  queued_posts: number;
  published_posts: number;
  failed_posts: number;
  pending_approval_posts?: number;
  rejected_posts?: number;
  fast_queue_depth?: number;
  heavy_queue_depth?: number;
}

export type TokenStatus = 'ACTIVE' | 'EXPIRED' | 'REVOKED' | 'RATE_LIMITED' | 'WARNING';

export type PlatformType = 'FACEBOOK' | 'LINKEDIN' | 'TWITTER' | 'TIKTOK';

export interface Channel {
  id: string;
  platform: PlatformType;
  external_id: string;
  name: string;
  category?: string;
  masked_token?: string;
  token_status: TokenStatus;
  token_expires_at?: string;
  is_active: boolean;
  rate_limit_call_count?: number;
  rate_limit_total_time?: number;
  rate_limit_total_cputime?: number;
  last_checked_at?: string;
  created_at: string;
  updated_at: string;
}

export type MediaType = 'NONE' | 'LINK' | 'IMAGE' | 'ALBUM' | 'VIDEO' | 'REEL';

export type PostStatus =
  | 'DRAFT'
  | 'PENDING_APPROVAL'
  | 'SCHEDULED'
  | 'QUEUED'
  | 'PROCESSING'
  | 'PUBLISHED'
  | 'FAILED'
  | 'REJECTED'
  | 'CANCELLED';

export interface Post {
  id: string;
  organization_id?: string;
  batch_id?: string;
  channel_group_id?: string;
  channel_id: string;
  channel_name?: string;
  platform?: PlatformType;
  external_channel_id?: string;
  external_post_id?: string;
  content: string;
  link_url?: string;
  media_type: MediaType;
  media_urls?: string[];
  status: PostStatus;
  scheduled_at: string;
  published_at?: string;
  submitted_by?: string;
  reviewed_by?: string;
  reviewed_at?: string;
  rejection_note?: string;
  retry_count: number;
  max_retries: number;
  last_error_code?: number;
  last_error_message?: string;
  queue_tier: 'FAST' | 'HEAVY';
  created_at: string;
  updated_at: string;
}

export interface PublishLog {
  id: string;
  post_id: string;
  channel_id?: string;
  attempt_number: number;
  action?: string;
  http_status: number;
  error_code?: number;
  error_subcode?: number;
  request_summary?: string;
  response_body?: string;
  duration_ms: number;
  created_at: string;
}

export interface MediaUploadResponse {
  message: string;
  public_url: string;
  storage_key: string;
  file_name: string;
  file_size: number;
  mime_type: string;
}

export interface AIGenerateResponse {
  topic: string;
  tone: string;
  model_used: string;
  generated_content: string;
}

export interface TokenDebugInfo {
  app_id: string;
  type?: string;
  application?: string;
  data_access_expires_at?: number;
  expires_at?: number;
  is_valid: boolean;
  issued_at?: number;
  scopes?: string[];
  user_id?: string;
}

export interface VerifyTokenResponse {
  channel_id: string;
  token_status: TokenStatus;
  is_valid: boolean;
  debug_info?: TokenDebugInfo;
  error?: string;
}

// Multi-Tenant, RBAC & Approval Types
export type OrgRole = 'OWNER' | 'ADMIN' | 'MEMBER';
export type GroupRole = 'MANAGER' | 'APPROVER' | 'EDITOR' | 'VIEWER';
export type ApprovalAction = 'APPROVED' | 'REJECTED';

export interface User {
  id: string;
  email: string;
  full_name: string;
  avatar_url?: string;
  is_active: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  created_at: string;
  updated_at: string;
}

export interface OrganizationMember {
  id: string;
  organization_id: string;
  user_id: string;
  org_role: OrgRole;
  user_email?: string;
  user_full_name?: string;
  created_at: string;
  updated_at?: string;
}

export interface OrganizationInvitation {
  id: string;
  organization_id: string;
  email: string;
  org_role: OrgRole;
  invited_by: string;
  expires_at: string;
  accepted_at?: string;
  created_at: string;
}

export interface ChannelGroup {
  id: string;
  organization_id: string;
  name: string;
  description?: string;
  color_code?: string;
  require_approval: boolean;
  channel_count?: number;
  member_count?: number;
  channels?: Channel[];
  created_at: string;
  updated_at: string;
}

export interface ChannelGroupPermission {
  id: string;
  channel_group_id: string;
  user_id: string;
  group_role: GroupRole;
  user_email?: string;
  user_full_name?: string;
  created_at: string;
  updated_at?: string;
}

export interface PostApproval {
  id: string;
  post_id: string;
  reviewer_id: string;
  reviewer_name?: string;
  action: ApprovalAction;
  note?: string;
  created_at: string;
}

export interface AuditLog {
  id: string;
  organization_id: string;
  user_id?: string;
  action: string;
  entity_type: string;
  entity_id: string;
  details?: any;
  ip_address?: string;
  created_at: string;
}
