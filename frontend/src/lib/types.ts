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
  total_pages: number;
  active_pages: number;
  total_posts: number;
  scheduled_posts: number;
  queued_posts: number;
  published_posts: number;
  failed_posts: number;
  fast_queue_depth?: number;
  heavy_queue_depth?: number;
}

export type TokenStatus = 'ACTIVE' | 'EXPIRED' | 'REVOKED' | 'WARNING';

export interface FacebookPage {
  id: string;
  page_id: string;
  page_name: string;
  category?: string;
  masked_token?: string;
  token_status: TokenStatus;
  token_expires_at?: string;
  is_active: boolean;
  rate_limit_call_count?: number;
  rate_limit_total_time?: number;
  rate_limit_total_cputime?: number;
  created_at: string;
  updated_at: string;
}

export type MediaType = 'NONE' | 'LINK' | 'IMAGE' | 'ALBUM' | 'VIDEO' | 'REEL';

export type PostStatus = 'DRAFT' | 'SCHEDULED' | 'QUEUED' | 'PROCESSING' | 'PUBLISHED' | 'FAILED' | 'CANCELLED';

export interface Post {
  id: string;
  page_uuid: string;
  page_name?: string;
  fb_post_id?: string;
  content: string;
  link_url?: string;
  media_type: MediaType;
  media_urls?: string[];
  status: PostStatus;
  scheduled_at: string;
  published_at?: string;
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
  attempt_number: number;
  fb_post_id?: string;
  http_status: number;
  error_code?: number;
  error_subcode?: number;
  error_message?: string;
  duration_ms: number;
  executed_at: string;
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
  type: string;
  application: string;
  data_access_expires_at: number;
  expires_at: number;
  is_valid: boolean;
  issued_at: number;
  scopes: string[];
  user_id: string;
}

export interface VerifyTokenResponse {
  page_uuid: string;
  token_status: TokenStatus;
  is_valid: boolean;
  debug_info?: TokenDebugInfo;
  error?: string;
}
