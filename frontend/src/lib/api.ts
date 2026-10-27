import {
  AIGenerateResponse,
  ApprovalAction,
  AuditLog,
  Channel,
  ChannelGroup,
  ChannelGroupPermission,
  GroupRole,
  MediaUploadResponse,
  Organization,
  OrganizationInvitation,
  OrganizationMember,
  PlatformType,
  Post,
  PublishLog,
  SystemHealth,
  SystemStats,
  User,
  VerifyTokenResponse,
  OrgRole,
} from "./types";

// Base API URL: default empty string uses relative path (rewrites in next.config.mjs proxy to 127.0.0.1:8080)
const API_BASE = process.env.NEXT_PUBLIC_API_URL || "";

export const STORAGE_KEYS = {
  TOKEN: "postpilot_auth_token",
  ORG: "postpilot_organization_id",
  SIM_ROLE: "postpilot_simulate_role",
};

function getHeaders(customHeaders?: HeadersInit): Headers {
  const headers = new Headers(customHeaders);
  if (typeof window !== "undefined") {
    const token = localStorage.getItem(STORAGE_KEYS.TOKEN);
    if (token && !headers.has("Authorization")) {
      headers.set("Authorization", `Bearer ${token}`);
    }
    const orgId = localStorage.getItem(STORAGE_KEYS.ORG);
    if (orgId && !headers.has("X-Organization-Id")) {
      headers.set("X-Organization-Id", orgId);
    }
    const simRole = localStorage.getItem(STORAGE_KEYS.SIM_ROLE);
    if (simRole && !headers.has("X-Simulate-Role")) {
      headers.set("X-Simulate-Role", simRole);
    }
  }
  return headers;
}

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let errorMsg = `HTTP Error ${res.status}`;
    try {
      const data = await res.json();
      if (data.error) errorMsg = data.error;
    } catch {
      // ignore
    }
    throw new Error(errorMsg);
  }
  return res.json() as Promise<T>;
}

export const api = {
  // Storage & Session Helpers
  getActiveOrgId(): string | null {
    if (typeof window === "undefined") return null;
    return localStorage.getItem(STORAGE_KEYS.ORG);
  },

  setActiveOrgId(id: string | null) {
    if (typeof window === "undefined") return;
    if (id) {
      localStorage.setItem(STORAGE_KEYS.ORG, id);
    } else {
      localStorage.removeItem(STORAGE_KEYS.ORG);
    }
  },

  getToken(): string | null {
    if (typeof window === "undefined") return null;
    return localStorage.getItem(STORAGE_KEYS.TOKEN);
  },

  setToken(token: string | null) {
    if (typeof window === "undefined") return;
    if (token) {
      localStorage.setItem(STORAGE_KEYS.TOKEN, token);
    } else {
      localStorage.removeItem(STORAGE_KEYS.TOKEN);
    }
  },

  getSimulateRole(): string | null {
    if (typeof window === "undefined") return null;
    return localStorage.getItem(STORAGE_KEYS.SIM_ROLE);
  },

  setSimulateRole(role: string | null) {
    if (typeof window === "undefined") return;
    if (role) {
      localStorage.setItem(STORAGE_KEYS.SIM_ROLE, role);
    } else {
      localStorage.removeItem(STORAGE_KEYS.SIM_ROLE);
    }
  },

  clearAuth() {
    if (typeof window === "undefined") return;
    localStorage.removeItem(STORAGE_KEYS.TOKEN);
    localStorage.removeItem(STORAGE_KEYS.ORG);
    localStorage.removeItem(STORAGE_KEYS.SIM_ROLE);
  },

  // 1. Health & Stats
  async getHealth(): Promise<SystemHealth> {
    const res = await fetch(`${API_BASE}/healthz`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    return handleResponse<SystemHealth>(res);
  },

  async getStats(): Promise<SystemStats> {
    const res = await fetch(`${API_BASE}/api/v1/stats`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    return handleResponse<SystemStats>(res);
  },

  // 2. Multi-Tenant Identity & Auth
  async register(
    email: string,
    password: string,
    fullName: string
  ): Promise<{ token: string; user: User; organization: Organization }> {
    const res = await fetch(`${API_BASE}/api/v1/auth/register`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ email, password, full_name: fullName }),
    });
    const data = await handleResponse<{ token: string; user: User; organization: Organization }>(res);
    if (data.token) {
      api.setToken(data.token);
    }
    if (data.organization?.id) {
      api.setActiveOrgId(data.organization.id);
    }
    return data;
  },

  async login(
    email: string,
    password: string
  ): Promise<{ token: string; user: User; organizations: Organization[] }> {
    const res = await fetch(`${API_BASE}/api/v1/auth/login`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ email, password }),
    });
    const data = await handleResponse<{ token: string; user: User; organizations: Organization[] }>(res);
    if (data.token) {
      api.setToken(data.token);
    }
    if (data.organizations && data.organizations.length > 0) {
      api.setActiveOrgId(data.organizations[0].id);
    }
    return data;
  },

  async getMe(): Promise<{ user: User; organization: Organization; member?: OrganizationMember }> {
    const res = await fetch(`${API_BASE}/api/v1/auth/me`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    return handleResponse<{ user: User; organization: Organization; member?: OrganizationMember }>(res);
  },

  // 3. Organization Workspace Management
  async getOrganizations(): Promise<Organization[]> {
    const res = await fetch(`${API_BASE}/api/v1/organizations`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    const data = await handleResponse<{ organizations: Organization[] }>(res);
    return data.organizations || [];
  },

  async createOrganization(name: string, slug?: string): Promise<Organization> {
    const res = await fetch(`${API_BASE}/api/v1/organizations`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ name, slug: slug || "" }),
    });
    return handleResponse<Organization>(res);
  },

  async getOrgMembers(orgId: string): Promise<OrganizationMember[]> {
    const res = await fetch(`${API_BASE}/api/v1/organizations/${orgId}/members`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    const data = await handleResponse<{ members: OrganizationMember[] }>(res);
    return data.members || [];
  },

  async addOrgMember(
    orgId: string,
    email: string,
    role: OrgRole
  ): Promise<{ message: string; member?: OrganizationMember; invitation?: OrganizationInvitation; token?: string }> {
    const res = await fetch(`${API_BASE}/api/v1/organizations/${orgId}/members`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ email, role }),
    });
    return handleResponse<{ message: string; member?: OrganizationMember; invitation?: OrganizationInvitation; token?: string }>(res);
  },

  async removeOrgMember(orgId: string, memberId: string): Promise<{ message: string }> {
    const res = await fetch(`${API_BASE}/api/v1/organizations/${orgId}/members/${memberId}`, {
      method: "DELETE",
      headers: getHeaders(),
    });
    return handleResponse<{ message: string }>(res);
  },

  // 4. Channel Groups & RBAC
  async getChannelGroups(): Promise<ChannelGroup[]> {
    const res = await fetch(`${API_BASE}/api/v1/channel-groups`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    const data = await handleResponse<{ channel_groups: ChannelGroup[] }>(res);
    return data.channel_groups || [];
  },

  async createChannelGroup(group: {
    name: string;
    description?: string;
    color_code?: string;
    require_approval?: boolean;
  }): Promise<ChannelGroup> {
    const res = await fetch(`${API_BASE}/api/v1/channel-groups`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify(group),
    });
    return handleResponse<ChannelGroup>(res);
  },

  async deleteChannelGroup(id: string): Promise<{ message: string }> {
    const res = await fetch(`${API_BASE}/api/v1/channel-groups/${id}`, {
      method: "DELETE",
      headers: getHeaders(),
    });
    return handleResponse<{ message: string }>(res);
  },

  async setChannelGroupChannels(groupId: string, channelIds: string[]): Promise<{
    message: string;
    group_id: string;
    channel_ids: string[];
  }> {
    const res = await fetch(`${API_BASE}/api/v1/channel-groups/${groupId}/channels`, {
      method: "PUT",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ channel_ids: channelIds }),
    });
    return handleResponse<{ message: string; group_id: string; channel_ids: string[] }>(res);
  },

  async getChannelGroupPermissions(groupId: string): Promise<ChannelGroupPermission[]> {
    const res = await fetch(`${API_BASE}/api/v1/channel-groups/${groupId}/permissions`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    const data = await handleResponse<{ permissions: ChannelGroupPermission[] }>(res);
    return data.permissions || [];
  },

  async setChannelGroupPermission(
    groupId: string,
    memberIdOrUserId: string,
    role: GroupRole
  ): Promise<{ message: string; permission: ChannelGroupPermission }> {
    const res = await fetch(`${API_BASE}/api/v1/channel-groups/${groupId}/permissions`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ member_id: memberIdOrUserId, user_id: memberIdOrUserId, role }),
    });
    return handleResponse<{ message: string; permission: ChannelGroupPermission }>(res);
  },

  async deleteChannelGroupPermission(groupId: string, memberId: string): Promise<{ message: string }> {
    const res = await fetch(`${API_BASE}/api/v1/channel-groups/${groupId}/permissions/${memberId}`, {
      method: "DELETE",
      headers: getHeaders(),
    });
    return handleResponse<{ message: string }>(res);
  },

  // 5. Channels & Platform OAuth
  async getChannels(): Promise<Channel[]> {
    const res = await fetch(`${API_BASE}/api/v1/channels`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    const data = await handleResponse<{ channels: Channel[] }>(res);
    return data.channels || [];
  },

  async createChannel(channel: {
    platform?: PlatformType;
    external_id: string;
    name: string;
    category?: string;
    access_token: string;
  }): Promise<Channel> {
    const res = await fetch(`${API_BASE}/api/v1/channels`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify(channel),
    });
    return handleResponse<Channel>(res);
  },

  async deleteChannel(channelID: string): Promise<{ message: string; channel_id: string }> {
    const res = await fetch(`${API_BASE}/api/v1/channels/${channelID}`, {
      method: "DELETE",
      headers: getHeaders(),
    });
    return handleResponse<{ message: string; channel_id: string }>(res);
  },

  async verifyChannelToken(channelID: string): Promise<VerifyTokenResponse> {
    const res = await fetch(`${API_BASE}/api/v1/channels/${channelID}/verify-token`, {
      method: "POST",
      headers: getHeaders(),
    });
    return handleResponse<VerifyTokenResponse>(res);
  },

  async exchangeUserToken(shortLivedUserToken: string): Promise<{
    message: string;
    imported_count: number;
    channels: Channel[];
  }> {
    const res = await fetch(`${API_BASE}/api/v1/auth/facebook/exchange-token`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ short_lived_user_token: shortLivedUserToken }),
    });
    return handleResponse(res);
  },

  async getFBOAuthLoginUrl(): Promise<string> {
    const res = await fetch(`${API_BASE}/api/v1/auth/facebook/login`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    const data = await handleResponse<{ oauth_login_url: string }>(res);
    return data.oauth_login_url;
  },

  // 6. Posts, Scheduling & Approval Workflows
  async getPosts(status?: string, limit = 50): Promise<Post[]> {
    const params = new URLSearchParams();
    if (status && status !== "ALL") params.set("status", status);
    if (limit > 0) params.set("limit", limit.toString());

    const res = await fetch(`${API_BASE}/api/v1/posts?${params.toString()}`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    const data = await handleResponse<{ posts: Post[] }>(res);
    return data.posts || [];
  },

  async getPostDetail(id: string): Promise<{ post: Post; publish_logs: PublishLog[] }> {
    const res = await fetch(`${API_BASE}/api/v1/posts/${id}`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    return handleResponse<{ post: Post; publish_logs: PublishLog[] }>(res);
  },

  async createPost(payload: {
    channel_id?: string;
    channel_ids?: string[];
    channel_group_id?: string;
    content: string;
    media_type: string;
    link_url?: string;
    media_urls?: string[];
    scheduled_at?: string;
    publish_now?: boolean;
    submit_for_approval?: boolean;
    max_retries?: number;
  }): Promise<any> {
    const res = await fetch(`${API_BASE}/api/v1/posts`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify(payload),
    });
    return handleResponse<any>(res);
  },

  async publishPostNow(id: string, sync = true): Promise<any> {
    const res = await fetch(`${API_BASE}/api/v1/posts/${id}/publish-now?sync=${sync}`, {
      method: "POST",
      headers: getHeaders(),
    });
    return handleResponse<any>(res);
  },

  async cancelPost(id: string): Promise<any> {
    const res = await fetch(`${API_BASE}/api/v1/posts/${id}/cancel`, {
      method: "POST",
      headers: getHeaders(),
    });
    return handleResponse<any>(res);
  },

  async submitPostForApproval(
    id: string,
    payload?: { content?: string; scheduled_at?: string }
  ): Promise<Post> {
    const res = await fetch(`${API_BASE}/api/v1/posts/${id}/submit-approval`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify(payload || {}),
    });
    return handleResponse<Post>(res);
  },

  async reviewPost(
    id: string,
    payload: { action: ApprovalAction; note?: string; apply_to_batch?: boolean }
  ): Promise<Post> {
    const res = await fetch(`${API_BASE}/api/v1/posts/${id}/review`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({
        action: payload.action,
        note: payload.note || "",
        apply_to_batch: !!payload.apply_to_batch,
      }),
    });
    return handleResponse<Post>(res);
  },

  // 7. Media Upload (with Progress tracking & injected headers)
  uploadMedia(
    file: File,
    onProgress?: (percent: number, loadedMB: number, totalMB: number) => void
  ): Promise<MediaUploadResponse> {
    return new Promise((resolve, reject) => {
      const formData = new FormData();
      formData.append("file", file);

      const xhr = new XMLHttpRequest();
      xhr.open("POST", `${API_BASE}/api/v1/media/upload`, true);

      // Inject tenant, auth and simulation headers
      if (typeof window !== "undefined") {
        const token = localStorage.getItem(STORAGE_KEYS.TOKEN);
        if (token) xhr.setRequestHeader("Authorization", `Bearer ${token}`);
        const orgId = localStorage.getItem(STORAGE_KEYS.ORG);
        if (orgId) xhr.setRequestHeader("X-Organization-Id", orgId);
        const simRole = localStorage.getItem(STORAGE_KEYS.SIM_ROLE);
        if (simRole) xhr.setRequestHeader("X-Simulate-Role", simRole);
      }

      if (onProgress) {
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable) {
            const percent = Math.round((e.loaded / e.total) * 100);
            const loadedMB = Number((e.loaded / (1024 * 1024)).toFixed(1));
            const totalMB = Number((e.total / (1024 * 1024)).toFixed(1));
            onProgress(percent, loadedMB, totalMB);
          }
        };
      }

      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const data = JSON.parse(xhr.responseText);
            resolve(data);
          } catch (e: any) {
            reject(new Error("Lỗi giải mã phản hồi JSON: " + e.message));
          }
        } else {
          try {
            const errData = JSON.parse(xhr.responseText);
            reject(new Error(errData.error || `Upload thất bại (HTTP ${xhr.status})`));
          } catch {
            reject(new Error(`Upload thất bại (HTTP ${xhr.status}): ${xhr.responseText}`));
          }
        }
      };

      xhr.onerror = () => {
        reject(new Error("Lỗi kết nối mạng khi tải lên tập tin"));
      };

      xhr.send(formData);
    });
  },

  // 8. Audit Logs
  async getAuditLogs(limit = 50): Promise<AuditLog[]> {
    const res = await fetch(`${API_BASE}/api/v1/audit-logs?limit=${limit}`, {
      cache: "no-store",
      headers: getHeaders(),
    });
    const data = await handleResponse<{ audit_logs: AuditLog[] }>(res);
    return data.audit_logs || [];
  },

  // 9. AI Assistant
  async generateAI(
    topic: string,
    tone = "chuyên nghiệp, lôi cuốn, tạo tương tác"
  ): Promise<AIGenerateResponse> {
    const res = await fetch(`${API_BASE}/api/v1/ai/generate`, {
      method: "POST",
      headers: getHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ topic, tone }),
    });
    return handleResponse<AIGenerateResponse>(res);
  },
};
