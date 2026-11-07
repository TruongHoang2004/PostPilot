# 📡 Đặc tả RESTful API (API Specification)

Server mặc định lắng nghe tại địa chỉ `http://localhost:8080`. Tất cả các API trả về định dạng JSON (`application/json; charset=utf-8`).

Các HTTP Handlers trong `internal/api/` được phân tách rõ ràng theo từng Bounded Context:
- `system_handler.go`: `/healthz`, `/api/v1/stats`
- `channel_handler.go`: `/api/v1/channels`, `/api/v1/channels/{id}`, `/api/v1/channels/{id}/verify-token`
- `publishing_handler.go`: `/api/v1/posts` (GET có `from/to/channel_id/batch_id/limit/offset`, POST), `/api/v1/posts/bulk`, `/api/v1/posts/bulk/preview`, `/api/v1/posts/{id}` (GET, PATCH), `/api/v1/posts/{id}/publish-now`, `/api/v1/posts/{id}/cancel`, `/api/v1/posts/{id}/submit-approval`, `/api/v1/posts/{id}/review`, `/api/v1/posts/batches/{id}`
- `library_handler.go`: `/api/v1/library` (GET, POST), `/api/v1/library/{id}` (GET, PATCH, DELETE), `/api/v1/library/{id}/restore`, `/api/v1/library/{id}/schedule`
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

Phản hồi có thêm `storage_provider` (`s3` hoặc `r2`) và `storage_configured` (`false` nghĩa là upload media đang trả 503 cho đến khi cấu hình đủ). Không trả về bất kỳ giá trị bí mật nào.

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

Dạng phản hồi của `POST /api/v1/posts`: trả **một đối tượng Post** khi chỉ giải quyết được một Fanpage (`channel_id`, hoặc `channel_ids` có một phần tử); trả **bao `{batch_id, count, posts}`** (lô `kind = FANOUT`) khi có từ hai Fanpage hoặc dùng `channel_group_id` (kể cả nhóm chỉ có một Fanpage). Bản trước luôn trả bao khi gửi `channel_ids`.

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
Liệt kê bài viết của workspace hiện tại. Tham số: `status`, `channel_id`, `batch_id`, `from`/`to` (RFC3339, khoảng nửa mở `[from, to)` theo `scheduled_at`), `limit` (mặc định 50, tối đa 500), `offset`. Có `from`/`to` thì sắp xếp tăng dần theo giờ đăng (lịch), không có thì mới nhất trước (thứ tự ổn định: `scheduled_at`, `created_at`, `id`). Mỗi bài kèm `batch_name` nếu thuộc một lô. Phản hồi trả `limit`/`offset` đã chuẩn hoá và `has_more`. Thành viên không phải OWNER/ADMIN chỉ nhận bài trên các Fanpage mình có quyền xem; giới hạn này được áp **trước** khi phân trang nên trang luôn đầy. Thay đổi so với bản cũ: `status=ALL` nghĩa là không lọc; `limit` > 200 được chấp nhận tới 500; `channel_ids` trùng lặp được gộp.
```bash
curl -s "http://localhost:8080/api/v1/posts?from=2026-11-09T17:00:00Z&to=2026-11-16T17:00:00Z&channel_id=<CHANNEL_UUID>&limit=500"
curl -s "http://localhost:8080/api/v1/posts?status=SCHEDULED&limit=200&offset=200"
```

### `POST /api/v1/posts/bulk/preview` và `POST /api/v1/posts/bulk` (Soạn & lên lịch hàng loạt)
Cùng một body cho cả hai: `preview` chỉ tính lịch và không ghi gì; `bulk` tạo toàn bộ bài trong **một transaction** (lỗi bất kỳ ⇒ không tạo bài nào). Mỗi `item` được nhân ra cho từng Fanpage đích (`channel_id` | `channel_ids` | `channel_group_id`); tối đa 100 item và 500 bài/lần. Item không có `scheduled_at` được gán giờ theo `schedule_plan`: duyệt từng ngày từ `start_date` (theo `timezone`), chỉ lấy `weekdays` cho phép, theo thứ tự `time_slots`, bỏ qua giờ đã qua và (nếu `avoid_conflicts`) giờ cách một bài đang chờ trên cùng Fanpage dưới `min_gap_minutes`. Quy tắc duyệt bài (EDITOR trên nhóm bắt buộc duyệt, `submit_for_approval`) giống `POST /api/v1/posts`; gửi `publish_now: true` cho bulk bị từ chối `400`. `name` tối đa 255 ký tự; mỗi item tối đa 10 `media_urls` và 63 206 ký tự nội dung. Tránh trùng lịch là *best-effort* (hai yêu cầu đồng thời trên cùng Fanpage vẫn có thể trùng); giờ riêng (`scheduled_at` của item) không bị kiểm tra trùng, chỉ được ghi chú nếu đã qua. Mỗi slot trả thêm `note_code` (`CONFLICTS_SKIPPED` + `note_value`, `PAST_TIME`) và preview có `warning_codes` (`OCCUPANCY_TRUNCATED`).
```bash
curl -X POST http://localhost:8080/api/v1/posts/bulk \
  -H "Authorization: Bearer <JWT>" -H "Content-Type: application/json" \
  -d '{
    "name": "Chiến dịch tháng 12",
    "channel_ids": ["<CHANNEL_A>", "<CHANNEL_B>"],
    "items": [
      {"content": "Bài 1", "media_type": "NONE"},
      {"content": "Bài 2", "media_type": "IMAGE", "media_urls": ["https://cdn.example.com/a.jpg"]},
      {"content": "Bài 3 giờ riêng", "scheduled_at": "2026-12-24T12:00:00Z"}
    ],
    "schedule_plan": {"start_date": "2026-12-01", "timezone": "Asia/Ho_Chi_Minh", "time_slots": ["09:00", "19:30"], "weekdays": [1,2,3,4,5], "min_gap_minutes": 60, "avoid_conflicts": true},
    "submit_for_approval": false,
    "max_retries": 3
  }'
```
Phản hồi `201`: `{ "batch_id", "batch": {...}, "name", "count", "posts": [...], "schedule": [{"index", "scheduled_at", "explicit", "note"}] }`. `preview` trả `200`: `{ "schedule", "warnings", "channels", "schedule_plan", "post_count" }`.

### `PATCH /api/v1/posts/{id}` (Sửa nội dung / đổi lịch)
Chỉ áp dụng cho bài `DRAFT`, `SCHEDULED`, `PENDING_APPROVAL`, `FAILED`. Bài `FAILED` chỉ quay về `SCHEDULED` (reset số lần thử) khi gửi `scheduled_at` mới; sửa mỗi nội dung thì vẫn `FAILED`. Các trường tuỳ chọn: `scheduled_at`, `content`, `link_url`, `media_type`, `media_urls`. **Bảo toàn phê duyệt**: EDITOR (không phải OWNER/ADMIN) sửa nội dung/link/media của bài trên Fanpage thuộc nhóm bắt buộc duyệt sẽ đưa bài về `PENDING_APPROVAL` và xoá quyết định duyệt cũ; chỉ đổi `scheduled_at` thì giữ nguyên. Bài `DRAFT` không bị chuyển về chờ duyệt khi sửa. `scheduled_at` mới không được kiểm tra trùng lịch hay đã qua (giao diện chỉ cảnh báo); giờ đã qua sẽ được đăng ở lượt quét kế tiếp. Trả `409` nếu bài đã ở trạng thái không sửa được (kể cả khi worker vừa nhận bài), `403` nếu không có quyền tạo bài trên Fanpage đó, `503` nếu không kiểm tra được quy tắc duyệt.
```bash
curl -X PATCH http://localhost:8080/api/v1/posts/<POST_UUID> -H "Content-Type: application/json" \
  -d '{"scheduled_at": "2026-12-05T02:30:00Z", "content": "Nội dung đã sửa"}'
```

### `GET /api/v1/posts/batches/{id}`
Thông tin lô bài (`batch`: tên, loại `BULK`/`FANOUT`, `schedule_plan`, `item_count`) và danh sách `posts` thuộc lô.

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

## 3b. Bài soạn sẵn (Content Library) & Lên lịch từ lịch (`library_handler.go`)

Bài soạn sẵn là nội dung dùng lại được, gắn tuỳ chọn với một Fanpage. **Lên lịch sao chép** nội dung thành bài đăng thường (`posts`), nên sửa/xoá bài soạn sẵn sau đó không làm đổi bài đã lên lịch. Mọi truy vấn đều giới hạn theo `organization_id`; id của tổ chức khác luôn trả `404`.

| Endpoint | Mô tả |
| --- | --- |
| `GET /api/v1/library?archived=&channel_id=&q=&limit=&offset=` | `archived=false` (mặc định) = mục **Bài soạn sẵn**; `archived=true` = mục **Đã đăng** (xoá mềm). `limit` ≤ 100. Trả `{items, limit, offset, has_more}`. |
| `POST /api/v1/library` | Tạo: `{title, channel_id?, content, link_url, media_type, media_urls}` (cùng luật nội dung như bài đăng). `201`. |
| `GET /api/v1/library/{id}` | Chi tiết. |
| `PATCH /api/v1/library/{id}` | Sửa các trường gửi lên; mục đã xoá mềm trả `409`. `channel_id: ""` bỏ gắn Fanpage. |
| `DELETE /api/v1/library/{id}` | Xoá **vĩnh viễn**. Các bài đã lên lịch từ mục này vẫn được giữ (`library_item_id` về rỗng). |
| `POST /api/v1/library/{id}/restore` | "Giữ lại": đưa mục từ **Đã đăng** về **Bài soạn sẵn** và **bỏ cờ `delete_after_publish` của các bài đang chờ đăng** thuộc mục này (cùng giao dịch), để quyết định giữ lại không bị bài khác hoàn tác. |
| `POST /api/v1/library/{id}/schedule` | Lên lịch từ bài soạn sẵn (xem dưới). |

Quyền: OWNER/ADMIN luôn được. Tạo/sửa/xoá/khôi phục cần vai trò MANAGER/APPROVER/EDITOR trên Fanpage gắn kèm (hoặc trên ít nhất một Fanpage nếu mục không gắn Fanpage); VIEWER chỉ đọc (`403` khi ghi). **Lên lịch** do quyền trên các Fanpage đích quyết định (như `POST /posts`); riêng khi đặt `delete_after_publish: true` còn cần quyền soạn bài trên Fanpage gắn kèm của mục (vì cờ này sẽ xoá mềm mục đó). Danh sách chỉ gồm mục của Fanpage người dùng được xem, cộng mục không gắn Fanpage nếu người dùng xem được ít nhất một Fanpage.

Xoá một Fanpage **không xoá** bài soạn sẵn gắn với nó: mục trở thành "mọi Fanpage" (`channel_id` về rỗng, `ON DELETE SET NULL`) và từ đó hiển thị theo quy tắc của mục không gắn Fanpage.

### `POST /api/v1/library/{id}/schedule`
```bash
curl -X POST http://localhost:8080/api/v1/library/<ITEM_ID>/schedule \
  -H "Content-Type: application/json" -H "Authorization: Bearer <TOKEN>" \
  -d '{
    "channel_ids": ["<CHANNEL_UUID>"],
    "scheduled_at": "2026-11-20T09:00:00+07:00",
    "delete_after_publish": true
  }'
```
- `scheduled_at` bắt buộc (RFC3339); `publish_now` bị từ chối (`400`). Bỏ trống mục tiêu thì dùng Fanpage gắn kèm của bài soạn sẵn.
- Dùng đúng luật của `POST /posts`: Editor trên nhóm cần duyệt → bài vào `PENDING_APPROVAL`; nhiều Fanpage/nhóm → một lô `FANOUT`.
- `delete_after_publish: true`: khi **bài đăng đầu tiên mang cờ này đăng thành công**, mục bài soạn sẵn được **xoá mềm** (`archived_at`) sang mục **Đã đăng** (cùng giao dịch với `MarkPostPublished`, đăng lại không đếm trùng). Không đặt cờ: mục vẫn ở thư viện, chỉ tăng `publish_count`/`last_published_at`.
- Phản hồi `201`: `{count, posts}` (thêm `batch_id` chỉ khi nhiều Fanpage/nhóm tạo lô `FANOUT`); mỗi post có `library_item_id` và `delete_after_publish`. Mục đã xoá mềm trả `409`.
- Khi `MarkPostPublished` ghi nhận đã đăng, việc cập nhật bài soạn sẵn chạy trong savepoint: nếu phần này lỗi thì bài **vẫn là `PUBLISHED`** (chỉ ghi cảnh báo), vì bài đã lên Facebook.

## 4. Upload Media (`media_handler.go`) & AI Content (`ai_handler.go`)

> **Nhà cung cấp lưu trữ:** `STORAGE_PROVIDER=s3` (mặc định, MinIO/AWS S3 qua `S3_*`) hoặc `r2` (Cloudflare R2 qua `R2_*`, xem [DEPLOY_VPS.md](DEPLOY_VPS.md#cloudflare-r2-tùy-chọn)). Khi chưa cấu hình đủ, cả hai endpoint dưới đây trả `503 {"error":"object storage is not configured"}` và API vẫn chạy bình thường.

### `POST /api/v1/media/upload`
Upload trực tiếp file ảnh/video qua `multipart/form-data` lên kho đối tượng (MinIO/S3/Cloudflare R2). **Không có fallback lưu local**: lỗi hoặc chưa cấu hình thì trả lỗi (`502`/`503`).

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
