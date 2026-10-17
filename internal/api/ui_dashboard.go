package api

import "net/http"

func (s *Server) handleDashboardUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dashboardHTML))
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="vi">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>FB Auto-Poster Control Center (Golang + Docker)</title>
  <style>
    :root {
      --bg: #0f172a;
      --card: #1e293b;
      --border: #334155;
      --text: #f8fafc;
      --muted: #94a3b8;
      --primary: #3b82f6;
      --success: #10b981;
      --warning: #f59e0b;
      --danger: #ef4444;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: var(--bg);
      color: var(--text);
      padding: 24px;
      line-height: 1.5;
    }
    .container { max-width: 1280px; margin: 0 auto; }
    header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 24px;
      padding-bottom: 16px;
      border-bottom: 1px solid var(--border);
    }
    header h1 { font-size: 1.5rem; font-weight: 700; }
    .badge {
      display: inline-block;
      padding: 4px 10px;
      border-radius: 999px;
      font-size: 0.75rem;
      font-weight: 600;
      background: rgba(59,130,246,0.2);
      color: #60a5fa;
    }
    .stats-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
      gap: 16px;
      margin-bottom: 24px;
    }
    .stat-card {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 16px;
    }
    .stat-card .label { color: var(--muted); font-size: 0.82rem; }
    .stat-card .value { font-size: 1.75rem; font-weight: 700; margin-top: 4px; }
    .main-grid {
      display: grid;
      grid-template-columns: 1fr 1.4fr;
      gap: 24px;
    }
    @media (max-width: 960px) { .main-grid { grid-template-columns: 1fr; } }
    .panel {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 20px;
      margin-bottom: 24px;
    }
    .panel h2 { font-size: 1.1rem; margin-bottom: 16px; }
    .form-group { margin-bottom: 12px; }
    label { display: block; font-size: 0.85rem; color: var(--muted); margin-bottom: 4px; }
    input, select, textarea {
      width: 100%;
      padding: 10px 12px;
      border-radius: 8px;
      border: 1px solid var(--border);
      background: #0f172a;
      color: var(--text);
      font-size: 0.9rem;
    }
    textarea { min-height: 110px; resize: vertical; }
    .btn-row { display: flex; gap: 10px; margin-top: 12px; flex-wrap: wrap; }
    button {
      cursor: pointer;
      padding: 9px 16px;
      border-radius: 8px;
      border: none;
      font-weight: 600;
      font-size: 0.88rem;
      background: var(--primary);
      color: white;
    }
    button.secondary { background: #475569; }
    button.success { background: var(--success); }
    table { width: 100%; border-collapse: collapse; font-size: 0.86rem; }
    th, td { text-align: left; padding: 10px 8px; border-bottom: 1px solid var(--border); }
    th { color: var(--muted); font-weight: 600; }
    .status-pill {
      padding: 2px 8px;
      border-radius: 6px;
      font-size: 0.75rem;
      font-weight: 600;
    }
    .status-PUBLISHED, .status-ACTIVE { background: rgba(16,185,129,0.2); color: #34d399; }
    .status-SCHEDULED, .status-QUEUED { background: rgba(245,158,11,0.2); color: #fbbf24; }
    .status-FAILED, .status-EXPIRED { background: rgba(239,68,68,0.2); color: #f87171; }
  </style>
</head>
<body>
  <div class="container">
    <header>
      <div>
        <h1>🚀 Facebook Auto-Poster Control Center</h1>
        <p style="color: var(--muted); font-size: 0.88rem;">Backend Golang • PostgreSQL Migrations • Redis Queue • AES-256-GCM Vault</p>
      </div>
      <span class="badge" id="health-badge">Đang kiểm tra kết nối...</span>
    </header>

    <div class="stats-grid">
      <div class="stat-card"><div class="label">Fanpage Hoạt Động</div><div class="value" id="st-pages">0</div></div>
      <div class="stat-card"><div class="label">Đang Hẹn Giờ</div><div class="value" id="st-scheduled">0</div></div>
      <div class="stat-card"><div class="label">Đã Đăng Thành Công</div><div class="value" id="st-published">0</div></div>
      <div class="stat-card"><div class="label">Lỗi / Đang Retry</div><div class="value" id="st-failed">0</div></div>
      <div class="stat-card"><div class="label">Fast / Heavy Queue</div><div class="value" id="st-queues">0 / 0</div></div>
    </div>

    <div class="main-grid">
      <div>
        <div class="panel">
          <h2>1. Kết nối Fanpage (Mã hóa AES-256-GCM)</h2>
          <div class="form-group">
            <label>Facebook Page ID</label>
            <input id="page-id" placeholder="Ví dụ: 1029384756123" />
          </div>
          <div class="form-group">
            <label>Tên Fanpage</label>
            <input id="page-name" placeholder="Ví dụ: Happy Darwin Official" />
          </div>
          <div class="form-group">
            <label>Page Access Token (Sẽ được mã hóa AES-256-GCM trước khi lưu DB)</label>
            <input id="page-token" type="password" placeholder="EAABsbCS1iHgBO..." />
          </div>
          <div class="btn-row">
            <button onclick="addPage()">+ Lưu Fanpage</button>
          </div>
        </div>

        <div class="panel">
          <h2>2. Tạo Bài Đăng Mới / Lên Lịch Hẹn</h2>
          <div class="form-group">
            <label>Chọn Fanpage</label>
            <select id="post-page"></select>
          </div>
          <div class="form-group">
            <label>Chủ đề gợi ý cho AI (Gemini)</label>
            <div style="display:flex; gap:8px;">
              <input id="ai-topic" placeholder="Nhập chủ đề để AI tự viết bài..." />
              <button class="secondary" onclick="generateAI()" style="white-space:nowrap;">✨ AI Viết Bài</button>
            </div>
          </div>
          <div class="form-group">
            <label>Nội dung bài viết (Caption & Hashtags)</label>
            <textarea id="post-content" placeholder="Nhập nội dung bài đăng Facebook..."></textarea>
          </div>
          <div class="form-group">
            <label>Loại bài đăng (Media Type)</label>
            <select id="post-media-type">
              <option value="NONE">Chỉ văn bản (Text Post)</option>
              <option value="LINK">Đính kèm Link Preview (Link Post)</option>
              <option value="IMAGE">1 Ảnh đơn (Single Photo)</option>
              <option value="ALBUM">Album nhiều ảnh (Multi-Photo Album)</option>
              <option value="VIDEO">Video thường (Chunked/URL Video)</option>
              <option value="REEL">Facebook Reels (Short Video 9:16)</option>
            </select>
          </div>
          <div class="form-group">
            <label>Danh sách Media URLs (Mỗi dòng 1 link S3/CDN) hoặc Link đính kèm</label>
            <input id="post-media-urls" placeholder="https://cdn.example.com/photo1.jpg, https://cdn.example.com/photo2.jpg" />
          </div>
          <div class="form-group">
            <label>Thời gian hẹn đăng (Bỏ trống nếu muốn đăng ngay)</label>
            <input id="post-schedule" type="datetime-local" />
          </div>
          <div class="btn-row">
            <button onclick="createPost(false)">📅 Lên Lịch Hẹn</button>
            <button class="success" onclick="createPost(true)">⚡ Đăng Ngay (Publish Now)</button>
          </div>
        </div>
      </div>

      <div>
        <div class="panel">
          <h2>Danh sách Fanpage & Chỉ số Rate Limit</h2>
          <table>
            <thead>
              <tr>
                <th>Fanpage</th>
                <th>Page ID</th>
                <th>Token</th>
                <th>Trạng thái</th>
                <th>Rate Limit</th>
              </tr>
            </thead>
            <tbody id="pages-tbody"></tbody>
          </table>
        </div>

        <div class="panel">
          <h2>Lịch sử & Hàng đợi Bài viết</h2>
          <table>
            <thead>
              <tr>
                <th>Fanpage</th>
                <th>Loại</th>
                <th>Nội dung</th>
                <th>Giờ hẹn</th>
                <th>Trạng thái</th>
                <th>Thao tác</th>
              </tr>
            </thead>
            <tbody id="posts-tbody"></tbody>
          </table>
        </div>
      </div>
    </div>
  </div>

  <script>
    async function refreshAll() {
      try {
        const health = await fetch('/healthz').then(r => r.json());
        document.getElementById('health-badge').textContent =
          '🟢 Hệ thống Online (' + health.redis_mode + ')';

        const stats = await fetch('/api/v1/stats').then(r => r.json());
        document.getElementById('st-pages').textContent = stats.active_pages + ' / ' + stats.total_pages;
        document.getElementById('st-scheduled').textContent = stats.scheduled_posts;
        document.getElementById('st-published').textContent = stats.published_posts;
        document.getElementById('st-failed').textContent = stats.failed_posts;
        document.getElementById('st-queues').textContent = stats.fast_queue_depth + ' / ' + stats.heavy_queue_depth;

        const pagesData = await fetch('/api/v1/pages').then(r => r.json());
        const pages = pagesData.pages || [];
        const pageSelect = document.getElementById('post-page');
        const currentSelected = pageSelect.value;
        pageSelect.innerHTML = pages.map(p =>
          '<option value="' + p.id + '">' + p.page_name + ' (' + p.page_id + ')</option>'
        ).join('');
        if (currentSelected) pageSelect.value = currentSelected;

        document.getElementById('pages-tbody').innerHTML = pages.map(p => {
          const maxRate = Math.max(p.rate_limit_call_count || 0, p.rate_limit_total_time || 0, p.rate_limit_total_cputime || 0);
          return '<tr>' +
            '<td><strong>' + p.page_name + '</strong></td>' +
            '<td><code>' + p.page_id + '</code></td>' +
            '<td><code>' + (p.masked_token || '***') + '</code></td>' +
            '<td><span class="status-pill status-' + p.token_status + '">' + p.token_status + '</span></td>' +
            '<td>' + maxRate + '%</td>' +
          '</tr>';
        }).join('') || '<tr><td colspan="5" style="color:var(--muted);">Chưa có Fanpage nào. Hãy thêm Fanpage ở cột bên trái.</td></tr>';

        const postsData = await fetch('/api/v1/posts').then(r => r.json());
        const posts = postsData.posts || [];
        document.getElementById('posts-tbody').innerHTML = posts.map(post => {
          const shortText = (post.content || '').slice(0, 48) + ((post.content || '').length > 48 ? '...' : '');
          const sched = post.scheduled_at ? new Date(post.scheduled_at).toLocaleString('vi-VN') : '-';
          return '<tr>' +
            '<td>' + (post.page_name || post.page_uuid.slice(0,8)) + '</td>' +
            '<td><code>' + post.media_type + '</code></td>' +
            '<td title="' + (post.last_error_message || '') + '">' + shortText + '</td>' +
            '<td>' + sched + '</td>' +
            '<td><span class="status-pill status-' + post.status + '">' + post.status + '</span></td>' +
            '<td><button class="secondary" style="padding:4px 8px;font-size:0.75rem;" onclick="publishNow(\'' + post.id + '\')">Đăng ngay</button></td>' +
          '</tr>';
        }).join('') || '<tr><td colspan="6" style="color:var(--muted);">Chưa có bài viết nào.</td></tr>';
      } catch (e) {
        console.error(e);
      }
    }

    async function addPage() {
      const page_id = document.getElementById('page-id').value.trim();
      const page_name = document.getElementById('page-name').value.trim();
      const access_token = document.getElementById('page-token').value.trim();
      if (!page_id || !page_name || !access_token) return alert('Vui lòng nhập đủ Page ID, Tên Fanpage và Token');
      const res = await fetch('/api/v1/pages', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ page_id, page_name, access_token })
      });
      if (!res.ok) {
        const err = await res.json();
        return alert('Lỗi: ' + err.error);
      }
      document.getElementById('page-id').value = '';
      document.getElementById('page-name').value = '';
      document.getElementById('page-token').value = '';
      refreshAll();
    }

    async function generateAI() {
      const topic = document.getElementById('ai-topic').value.trim();
      if (!topic) return alert('Nhập chủ đề trước khi gọi AI');
      const res = await fetch('/api/v1/ai/generate', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ topic, tone: 'chuyên nghiệp, thu hút tương tác' })
      }).then(r => r.json());
      if (res.generated_content) {
        document.getElementById('post-content').value = res.generated_content;
      }
    }

    async function createPost(publishNowFlag) {
      const page_uuid = document.getElementById('post-page').value;
      const content = document.getElementById('post-content').value;
      const media_type = document.getElementById('post-media-type').value;
      const rawUrls = document.getElementById('post-media-urls').value.trim();
      const schedVal = document.getElementById('post-schedule').value;

      if (!page_uuid) return alert('Hãy thêm và chọn một Fanpage trước!');
      const urls = rawUrls ? rawUrls.split(',').map(s => s.trim()).filter(Boolean) : [];
      const payload = {
        page_uuid,
        content,
        media_type,
        link_url: media_type === 'LINK' && urls.length > 0 ? urls[0] : '',
        media_urls: media_type !== 'LINK' ? urls : [],
        scheduled_at: schedVal ? new Date(schedVal).toISOString() : '',
        publish_now: publishNowFlag
      };

      const res = await fetch('/api/v1/posts', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(payload)
      });
      if (!res.ok) {
        const err = await res.json();
        return alert('Lỗi: ' + err.error);
      }
      document.getElementById('post-content').value = '';
      refreshAll();
    }

    async function publishNow(id) {
      await fetch('/api/v1/posts/' + id + '/publish-now?sync=true', { method: 'POST' });
      refreshAll();
    }

    refreshAll();
    setInterval(refreshAll, 5000);
  </script>
</body>
</html>`
