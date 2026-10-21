# 📡 Đặc tả RESTful API (API Specification)

Server mặc định lắng nghe tại địa chỉ `http://localhost:8080`. Tất cả các API trả về định dạng JSON (`application/json; charset=utf-8`).

Các HTTP Handlers trong `internal/api/` được phân tách rõ ràng theo từng Bounded Context:
- `system_handler.go`: `/healthz`, `/api/v1/stats`
- `channel_handler.go`: `/api/v1/channels`, `/api/v1/channels/{id}`, `/api/v1/channels/{id}/verify-token`
- `publishing_handler.go`: `/api/v1/posts`, `/api/v1/posts/{id}`, `/api/v1/posts/{id}/publish-now`, `/api/v1/posts/{id}/cancel`
- `facebook_handler.go`: `/api/v1/auth/facebook/*`, `/api/v1/webhooks/facebook`
- `media_handler.go`: `/api/v1/media/presign`, `/api/v1/media/upload`, `/api/v1/media`
- `ai_handler.go`: `/api/v1/ai/generate`

---

## 1. Kiểm tra Sức khỏe & Thống kê Hệ thống (`system_handler.go`)

### `GET /healthz`
Kiểm tra trạng thái kết nối của API Server, Redis Queue và độ sâu hàng đợi hiện tại.
```bash
curl -s http://localhost:8080/healthz
```

### `GET /api/v1/stats`
Lấy số liệu thống kê tổng quan (`total_channels`, `active_channels`, `scheduled_posts`, `queued_posts`, `published_posts`, `failed_posts`, `fast_queue_depth`, `heavy_queue_depth`).
```bash
curl -s http://localhost:8080/api/v1/stats
```

---

## 2. Quản lý Kênh Phát Sóng & Xác Thực Nền Tảng (`channel_handler.go` & `facebook_handler.go`)

### `POST /api/v1/channels`
Thêm hoặc cập nhật thủ công một Kênh phát sóng (`Channel`) kèm Access Token (mặc định `platform: "FACEBOOK"` nếu không truyền). Token được mã hóa AES-256-GCM trước khi lưu vào bảng `channels`.
```bash
curl -X POST http://localhost:8080/api/v1/channels \
  -H "Content-Type: application/json" \
  -d '{
    "platform": "FACEBOOK",
    "external_id": "1029384756123",
    "name": "PostPilot Tech Community",
    "category": "Software Company",
    "access_token": "EAABsbCS1iHgBO_PERMANENT_PAGE_TOKEN"
  }'
```

### `GET /api/v1/channels`
Liệt kê danh sách các Kênh phát sóng đã kết nối (Token đã được che an toàn `EAABsb...OKEN`) kèm chỉ số Rate-Limit hiện tại.
```bash
curl -s http://localhost:8080/api/v1/channels
```

### `POST /api/v1/channels/{id}/verify-token`
Kiểm tra tức thời xem Token của Kênh phát sóng (theo `channel_id` UUID) còn hiệu lực trên nền tảng đích (thông qua `PublisherPort.VerifyToken`) hay không.
```bash
curl -X POST http://localhost:8080/api/v1/channels/<CHANNEL_UUID>/verify-token
```

### `DELETE /api/v1/channels/{id}`
Ngắt kết nối và xóa vĩnh viễn Kênh phát sóng khỏi hệ thống, tự động xóa sạch các bài viết, lịch hẹn liên quan và dọn dẹp bộ nhớ đệm Rate Limit trong Redis.
```bash
curl -X DELETE http://localhost:8080/api/v1/channels/<CHANNEL_UUID>
```

### `POST /api/v1/auth/facebook/exchange-token`
Đổi `short_lived_user_token` (lấy từ Graph API Explorer) sang Long-lived Token (60 ngày), sau đó tự động kéo toàn bộ danh sách Fanpage bạn quản lý kèm **Permanent Page Access Token**, mã hóa AES-256-GCM và lưu vào bảng `channels`.
```bash
curl -X POST http://localhost:8080/api/v1/auth/facebook/exchange-token \
  -H "Content-Type: application/json" \
  -d '{
    "short_lived_user_token": "EAABsbCS1iHgBO..."
  }'
```

### `GET /api/v1/auth/facebook/login` & `GET /api/v1/auth/facebook/callback`
Khởi tạo luồng đăng nhập OAuth2 1-chạm với Facebook và tự động đồng bộ toàn bộ Fanpage vào bảng `channels`.

### `GET / POST /api/v1/webhooks/facebook`
Endpoint xác minh (`hub.challenge`) và tiếp nhận sự kiện Webhook thời gian thực từ Meta.

---

## 3. Tạo Bài Viết, Lên Lịch Hẹn & Đăng Ngay (`publishing_handler.go`)

Hỗ trợ 6 định dạng `media_type` (`internal/domain/media.go`):
* `NONE`: Bài viết chỉ có văn bản (Text) -> `FAST` queue.
* `LINK`: Bài viết kèm đường dẫn chia sẻ (`link_url`) -> `FAST` queue.
* `IMAGE`: Bài viết kèm 1 ảnh đơn (`media_urls: ["https://..."]`) -> `FAST` queue.
* `ALBUM`: Bài viết kèm nhiều ảnh gom chung vào 1 post (`media_urls: ["url1", "url2", "url3"]`) -> `FAST` queue.
* `VIDEO`: Bài viết Video thường (`media_urls: ["https://.../video.mp4"]`) -> `HEAVY` queue.
* `REEL`: Bài viết Video ngắn / Reels 9:16 (`media_urls: ["https://.../reel.mp4"]`) -> `HEAVY` queue.

### `POST /api/v1/posts` (Hẹn giờ đăng Album nhiều ảnh)
```bash
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "<CHANNEL_UUID>",
    "content": "Bộ sưu tập công nghệ mới nhất tháng 10! 🚀 #Tech #Golang",
    "media_type": "ALBUM",
    "media_urls": [
      "https://images.unsplash.com/photo-1518770660439-4636190af475",
      "https://images.unsplash.com/photo-1526374965328-7f61d4dc18c5"
    ],
    "scheduled_at": "2026-10-18T09:00:00Z",
    "publish_now": false,
    "max_retries": 3
  }'
```

### `POST /api/v1/posts` (Đăng ngay lập tức - Publish Now)
Chỉ cần đặt `"publish_now": true`, bài viết sẽ được đẩy thẳng vào Redis Queue ưu tiên (`postpilot:queue:fast` hoặc `postpilot:queue:heavy`).
```bash
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "<CHANNEL_UUID>",
    "content": "Tin nóng phát sóng trực tiếp từ hệ thống tự động!",
    "media_type": "NONE",
    "publish_now": true
  }'
```

### `GET /api/v1/posts`
Liệt kê danh sách bài viết theo bộ lọc `?status=SCHEDULED&limit=50`.
```bash
curl -s "http://localhost:8080/api/v1/posts?status=ALL&limit=50"
```

### `GET /api/v1/posts/{id}`
Xem chi tiết bài viết (`post`) và toàn bộ lịch sử `publish_logs` (`action`, `http_status`, `error_code`, `error_subcode`, `duration_ms`, `response_body`).
```bash
curl -s http://localhost:8080/api/v1/posts/<POST_UUID>
```

### `POST /api/v1/posts/{id}/publish-now?sync=true`
Kích hoạt đăng bài ngay lập tức (thêm `?sync=true` nếu muốn thực thi đồng bộ ngay trong HTTP request thông qua `PublisherPort` để nhận kết quả trả về tức thì).
```bash
curl -X POST "http://localhost:8080/api/v1/posts/<POST_UUID>/publish-now?sync=true"
```

### `POST /api/v1/posts/{id}/cancel`
Hủy lịch đăng của một bài viết đang ở trạng thái `DRAFT`, `SCHEDULED` hoặc `FAILED` (kiểm tra bất biến domain `Post.CanCancel()`).
```bash
curl -X POST http://localhost:8080/api/v1/posts/<POST_UUID>/cancel
```

---

## 4. Upload Media (`media_handler.go`) & AI Content (`ai_handler.go`)

### `POST /api/v1/media/upload`
Upload trực tiếp file ảnh/video qua `multipart/form-data` (lưu local `./uploads` và đồng bộ lên MinIO/S3).

### `POST /api/v1/media/presign`
Sinh đường dẫn Presigned PUT URL chuẩn AWS Signature V4 để Frontend/Client đẩy thẳng file ảnh hoặc video lên MinIO / AWS S3 / Cloudflare R2.
```bash
curl -X POST http://localhost:8080/api/v1/media/presign \
  -H "Content-Type: application/json" \
  -d '{
    "file_name": "promo_reel.mp4",
    "mime_type": "video/mp4",
    "file_size_bytes": 15728640
  }'
```
Sau khi nhận được `upload_url` và `public_url`, upload file bằng lệnh:
```bash
curl -X PUT "<UPLOAD_URL>" --upload-file ./promo_reel.mp4
```
Và truyền `public_url` vào trường `media_urls` khi tạo bài đăng!

### `POST /api/v1/ai/generate`
Gọi Google Gemini API để tự động viết nội dung bài đăng theo chủ đề và giọng văn.
```bash
curl -X POST http://localhost:8080/api/v1/ai/generate \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "Ra mắt giải pháp tự động hóa doanh nghiệp bằng AI",
    "tone": "chuyên nghiệp, truyền cảm hứng, kêu gọi hành động mạnh mẽ"
  }'
```
