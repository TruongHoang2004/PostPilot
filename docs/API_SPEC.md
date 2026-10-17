# 📡 Đặc tả RESTful API (API Specification)

Server mặc định lắng nghe tại địa chỉ `http://localhost:8080`. Tất cả các API trả về định dạng JSON (`application/json; charset=utf-8`).

---

## 1. Kiểm tra Sức khỏe & Thống kê Hệ thống

### `GET /healthz`
Kiểm tra trạng thái kết nối của API Server, Redis Queue và độ sâu hàng đợi hiện tại.
```bash
curl -s http://localhost:8080/healthz
```

### `GET /api/v1/stats`
Lấy số liệu thống kê tổng quan (Số Fanpage hoạt động, số bài đang hẹn giờ, đã đăng, bị lỗi, số job trong Fast/Heavy Queue).
```bash
curl -s http://localhost:8080/api/v1/stats
```

---

## 2. Quản lý Fanpage & Đổi Token Vĩnh Viễn

### `POST /api/v1/auth/facebook/exchange-token`
Đổi `short_lived_user_token` (lấy từ Graph API Explorer) sang Long-lived Token (60 ngày), sau đó tự động kéo toàn bộ danh sách Fanpage bạn quản lý kèm **Permanent Page Access Token**, mã hóa AES-256-GCM và lưu vào DB.
```bash
curl -X POST http://localhost:8080/api/v1/auth/facebook/exchange-token \
  -H "Content-Type: application/json" \
  -d '{
    "short_lived_user_token": "EAABsbCS1iHgBO..."
  }'
```

### `POST /api/v1/pages`
Thêm hoặc cập nhật thủ công một Fanpage cùng Page Access Token.
```bash
curl -X POST http://localhost:8080/api/v1/pages \
  -H "Content-Type: application/json" \
  -d '{
    "page_id": "1029384756123",
    "page_name": "Happy Darwin Tech",
    "category": "Software Company",
    "access_token": "EAABsbCS1iHgBO_PERMANENT_PAGE_TOKEN"
  }'
```

### `GET /api/v1/pages`
Liệt kê danh sách các Fanpage đã kết nối (Token đã được che an toàn `EAABsb...OKEN`) kèm chỉ số Rate-Limit hiện tại.
```bash
curl -s http://localhost:8080/api/v1/pages
```

### `POST /api/v1/pages/{id}/verify-token`
Kiểm tra tức thời xem Token của Fanpage (theo UUID) còn sống trên Meta Graph API hay không.
```bash
curl -X POST http://localhost:8080/api/v1/pages/<PAGE_UUID>/verify-token
```

---

## 3. Tạo Bài Viết, Lên Lịch Hẹn & Đăng Ngay

Hỗ trợ 6 định dạng `media_type`:
* `NONE`: Bài viết chỉ có văn bản (Text).
* `LINK`: Bài viết kèm đường dẫn chia sẻ (`link_url`).
* `IMAGE`: Bài viết kèm 1 ảnh đơn (`media_urls: ["https://..."]`).
* `ALBUM`: Bài viết kèm nhiều ảnh gom chung vào 1 post (`media_urls: ["url1", "url2", "url3"]`).
* `VIDEO`: Bài viết Video thường (`media_urls: ["https://.../video.mp4"]`).
* `REEL`: Bài viết Facebook Reels 9:16 (`media_urls: ["https://.../reel.mp4"]`).

### `POST /api/v1/posts` (Hẹn giờ đăng Album nhiều ảnh)
```bash
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "page_uuid": "<PAGE_UUID>",
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
Chỉ cần đặt `"publish_now": true`, bài viết sẽ được đẩy thẳng vào Redis Queue ưu tiên.
```bash
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "page_uuid": "<PAGE_UUID>",
    "content": "Tin nóng phát sóng trực tiếp từ hệ thống tự động!",
    "media_type": "NONE",
    "publish_now": true
  }'
```

### `GET /api/v1/posts/{id}`
Xem chi tiết bài viết và toàn bộ lịch sử `publish_logs` (mã HTTP, thời gian phản hồi từ Facebook, mã lỗi nếu có).
```bash
curl -s http://localhost:8080/api/v1/posts/<POST_UUID>
```

### `POST /api/v1/posts/{id}/publish-now?sync=true`
Kích hoạt đăng bài ngay lập tức (thêm `?sync=true` nếu muốn thực thi đồng bộ ngay trong HTTP request để nhận kết quả trả về tức thì).
```bash
curl -X POST "http://localhost:8080/api/v1/posts/<POST_UUID>/publish-now?sync=true"
```

### `POST /api/v1/posts/{id}/cancel`
Hủy lịch đăng của một bài viết đang ở trạng thái `SCHEDULED` hoặc `FAILED`.
```bash
curl -X POST http://localhost:8080/api/v1/posts/<POST_UUID>/cancel
```

---

## 4. Upload Media (S3 / MinIO Presigned URL) & AI Content

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
Gọi Google Gemini API để tự động viết nội dung bài đăng Facebook theo chủ đề và giọng văn.
```bash
curl -X POST http://localhost:8080/api/v1/ai/generate \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "Ra mắt giải pháp tự động hóa doanh nghiệp bằng AI",
    "tone": "chuyên nghiệp, truyền cảm hứng, kêu gọi hành động mạnh mẽ"
  }'
```
