import {
  AIGenerateResponse,
  FacebookPage,
  MediaUploadResponse,
  Post,
  PublishLog,
  SystemHealth,
  SystemStats,
  VerifyTokenResponse,
} from "./types";

// Base API URL: default empty string uses relative path (rewrites in next.config.mjs proxy to 127.0.0.1:8080)
const API_BASE = process.env.NEXT_PUBLIC_API_URL || "";

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
  // 1. Health & Stats
  async getHealth(): Promise<SystemHealth> {
    const res = await fetch(`${API_BASE}/healthz`, { cache: "no-store" });
    return handleResponse<SystemHealth>(res);
  },

  async getStats(): Promise<SystemStats> {
    const res = await fetch(`${API_BASE}/api/v1/stats`, { cache: "no-store" });
    return handleResponse<SystemStats>(res);
  },

  // 2. Facebook Pages & OAuth
  async getPages(): Promise<FacebookPage[]> {
    const res = await fetch(`${API_BASE}/api/v1/pages`, { cache: "no-store" });
    const data = await handleResponse<{ pages: FacebookPage[] }>(res);
    return data.pages || [];
  },

  async createPage(page: {
    page_id: string;
    page_name: string;
    category?: string;
    access_token: string;
  }): Promise<FacebookPage> {
    const res = await fetch(`${API_BASE}/api/v1/pages`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(page),
    });
    return handleResponse<FacebookPage>(res);
  },

  async deletePage(pageUUID: string): Promise<{ message: string; page_uuid: string }> {
    const res = await fetch(`${API_BASE}/api/v1/pages/${pageUUID}`, {
      method: "DELETE",
    });
    return handleResponse<{ message: string; page_uuid: string }>(res);
  },

  async verifyPageToken(pageUUID: string): Promise<VerifyTokenResponse> {
    const res = await fetch(`${API_BASE}/api/v1/pages/${pageUUID}/verify-token`, {
      method: "POST",
    });
    return handleResponse<VerifyTokenResponse>(res);
  },

  async exchangeUserToken(shortLivedUserToken: string): Promise<{
    message: string;
    imported_count: number;
    pages: FacebookPage[];
  }> {
    const res = await fetch(`${API_BASE}/api/v1/auth/facebook/exchange-token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ short_lived_user_token: shortLivedUserToken }),
    });
    return handleResponse(res);
  },

  async getFBOAuthLoginUrl(): Promise<string> {
    const res = await fetch(`${API_BASE}/api/v1/auth/facebook/login`, {
      cache: "no-store",
    });
    const data = await handleResponse<{ oauth_login_url: string }>(res);
    return data.oauth_login_url;
  },

  // 3. Posts & Scheduling
  async getPosts(status?: string, limit = 50): Promise<Post[]> {
    const params = new URLSearchParams();
    if (status && status !== "ALL") params.set("status", status);
    if (limit > 0) params.set("limit", limit.toString());

    const res = await fetch(`${API_BASE}/api/v1/posts?${params.toString()}`, {
      cache: "no-store",
    });
    const data = await handleResponse<{ posts: Post[] }>(res);
    return data.posts || [];
  },

  async getPostDetail(id: string): Promise<{ post: Post; publish_logs: PublishLog[] }> {
    const res = await fetch(`${API_BASE}/api/v1/posts/${id}`, {
      cache: "no-store",
    });
    return handleResponse<{ post: Post; publish_logs: PublishLog[] }>(res);
  },

  async createPost(payload: {
    page_uuid: string;
    content: string;
    media_type: string;
    link_url?: string;
    media_urls?: string[];
    scheduled_at?: string;
    publish_now?: boolean;
    max_retries?: number;
  }): Promise<Post> {
    const res = await fetch(`${API_BASE}/api/v1/posts`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    return handleResponse<Post>(res);
  },

  async publishPostNow(id: string, sync = true): Promise<any> {
    const res = await fetch(`${API_BASE}/api/v1/posts/${id}/publish-now?sync=${sync}`, {
      method: "POST",
    });
    return handleResponse<any>(res);
  },

  async cancelPost(id: string): Promise<any> {
    const res = await fetch(`${API_BASE}/api/v1/posts/${id}/cancel`, {
      method: "POST",
    });
    return handleResponse<any>(res);
  },

  // 4. Media Upload (with Progress tracking)
  uploadMedia(
    file: File,
    onProgress?: (percent: number, loadedMB: number, totalMB: number) => void
  ): Promise<MediaUploadResponse> {
    return new Promise((resolve, reject) => {
      const formData = new FormData();
      formData.append("file", file);

      const xhr = new XMLHttpRequest();
      xhr.open("POST", `${API_BASE}/api/v1/media/upload`, true);

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

  // 5. AI Assistant
  async generateAI(topic: string, tone = "chuyên nghiệp, lôi cuốn, tạo tương tác"): Promise<AIGenerateResponse> {
    const res = await fetch(`${API_BASE}/api/v1/ai/generate`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ topic, tone }),
    });
    return handleResponse<AIGenerateResponse>(res);
  },
};
