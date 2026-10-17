# 🛠 Sổ tay Vận hành & Đưa lên Cloud (Operations Runbook)

Tài liệu này hướng dẫn cách vận hành, giám sát, xử lý sự cố và triển khai hệ thống lên môi trường Cloud (AWS, GCP, DigitalOcean, VPS).

---

## 1. Danh sách Kiểm tra trước khi lên Production (Cloud Checklist)

1. **Sinh khóa `TOKEN_ENCRYPTION_KEY` mới (32 bytes / 64 ký tự hex):**
   ```bash
   openssl rand -hex 32
   ```
   Cập nhật giá trị này vào biến môi trường của cả `api` và `worker`. *(Lưu ý: Không làm mất khóa này vì toàn bộ Page Access Token trong DB được mã hóa bằng nó).*
2. **Lưu trữ Media công khai (Public S3 / CDN):**
   * Khi chạy trên Cloud, Meta Graph API cần tải được file ảnh/video từ các đường dẫn trong `media_urls`.
   * Do đó, hãy trỏ `S3_ENDPOINT` và `S3_PUBLIC_BASE_URL` tới **AWS S3**, **Cloudflare R2**, hoặc **MinIO có gắn tên miền HTTPS công khai** (ví dụ: `https://media.yourdomain.com/fb-media`).
3. **Bật Cảnh báo Telegram (`TELEGRAM_BOT_TOKEN` & `TELEGRAM_CHAT_ID`):**
   * Nhắn tin cho `@BotFather` trên Telegram để tạo Bot và lấy `TELEGRAM_BOT_TOKEN`.
   * Thêm Bot vào nhóm vận hành và điền `TELEGRAM_CHAT_ID` vào `.env`.

---

## 2. Bảng Tra cứu Mã lỗi Meta Graph API & Cách Hệ thống Tự Xử lý

| Mã lỗi (`code`) | Tên lỗi từ Meta | Phân loại trong Hệ thống | Hành động tự động của Worker |
| :---: | :--- | :---: | :--- |
| **`1`, `2`** | Unknown / Service temporarily unavailable | `Retryable` | Tự động thử lại theo Exponential Backoff (1p $\rightarrow$ 5p $\rightarrow$ 15p). |
| **`4`, `17`, `32`, `613`** | Application / Page request limit reached | `Retryable` (Rate Limit) | Lưu trạng thái chạm trần vào Redis, lùi lịch đăng để chờ hồi phục quota. |
| **`190`, `102`** | Invalid OAuth 2.0 Access Token | `Non-Retryable` | Đánh dấu Fanpage sang trạng thái `EXPIRED`, dừng retry bài đó và bắn cảnh báo Telegram ngay lập tức. |
| **`10`, `200`** | Permission denied (`pages_manage_posts` missing) | `Non-Retryable` | Đánh dấu bài viết `FAILED`, ghi rõ thiếu quyền trong `publish_logs`. |
| **`368`** | Temporarily blocked for policies violations | `Non-Retryable` | Fanpage bị Meta hạn chế đăng bài do vi phạm tiêu chuẩn cộng đồng. Dừng retry và báo Telegram. |

---

## 3. Mở rộng Quy mô (Scaling) khi số lượng Fanpage tăng lớn

Khi bạn quản lý từ **100 đến 1.000+ Fanpage** hoặc đăng hàng ngàn video mỗi ngày:
* **Scale ngang số lượng container Worker:**
  ```bash
  docker compose up -d --scale worker=4
  ```
  Nhờ cơ chế `SELECT ... FOR UPDATE SKIP LOCKED` trên PostgreSQL và `BRPOP` nguyên tử trên Redis, các container `worker` sẽ tự động chia tải với nhau mà không bao giờ đăng trùng bài.
* **Tăng độ song song trong mỗi Worker:**
  Điều chỉnh biến môi trường:
  ```env
  FAST_WORKER_CONCURRENCY=10
  HEAVY_WORKER_CONCURRENCY=4
  ```

---

## 4. Các Lệnh Kiểm tra & Khắc phục Sự cố Thường dùng

* **Kiểm tra trạng thái các bản Migration:**
  ```bash
  docker compose run --rm migrate /app/fb-migrate status
  ```
* **Xem log thời gian thực của Worker:**
  ```bash
  docker compose logs -f worker
  ```
* **Truy vấn trực tiếp các bài viết bị lỗi trong PostgreSQL:**
  ```bash
  docker exec -it fbposter-postgres psql -U fbposter -d fb_autoposter -c \
    "SELECT id, status, retry_count, last_error_code, last_error_message FROM posts WHERE status = 'FAILED';"
  ```
