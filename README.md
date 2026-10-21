<p align="center">
  <img src="docs/logo-dark.jpg" alt="PostPilot Logo (Dark Mode)" width="150" style="border-radius: 26px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); margin: 8px;" />
  &nbsp;&nbsp;&nbsp;&nbsp;
  <img src="docs/logo-light.jpg" alt="PostPilot Logo (Light Mode)" width="150" style="border-radius: 26px; box-shadow: 0 10px 30px rgba(0,0,0,0.1); margin: 8px;" />
</p>

<h1 align="center">✈️ PostPilot - Hệ Thống Quản Trị & Đăng Bài Đa Kênh (DDD Architecture)</h1>

<p align="center">
  <b>Hạ tầng Go 1.26 + DDD / Hexagonal Architecture + Redis Queue + Next.js Studio + AI Content Studio</b>
</p>

---

Hệ thống **PostPilot** chuẩn Production được xây dựng theo kiến trúc **Domain-Driven Design (DDD)** và **Hexagonal Architecture (Ports & Adapters)** bằng **Golang (Go 1.26)** kết hợp **Next.js 14 Studio UI**. Trong đó **Post & Publishing** đóng vai trò là **Core Domain** độc lập nền tảng (với các thực thể trừu tượng `Channel`, `Post`, `MediaAsset`, `MediaAttachment` và cổng `PublisherPort`), còn **Facebook (Meta Graph API v20.0)** đóng vai trò là một **Secondary Adapter** thực thi `PublisherPort`, sẵn sàng mở rộng sang LinkedIn, TikTok, X/Twitter.

---

## 📚 Mục lục Tài liệu Hệ thống

Để nắm bắt toàn bộ thiết kế và cách vận hành, bạn có thể đọc theo thứ tự các tài liệu chi tiết trong thư mục [`docs/`](docs/):

1. **[Kiến trúc Hệ thống & Cơ sở dữ liệu (`docs/ARCHITECTURE.md`)](docs/ARCHITECTURE.md)**: Sơ đồ DDD / Hexagonal Architecture, phân ranh giới các Bounded Contexts, thiết kế bảng PostgreSQL (`channels`, `posts`, `publish_logs`, `media_assets`), cơ chế khóa dòng chống trùng lặp (`SELECT ... FOR UPDATE SKIP LOCKED`), thuật toán Exponential Backoff Retry và kiểm soát Rate-Limit.
2. **[Đặc tả RESTful API & Ví dụ cURL (`docs/API_SPEC.md`)](docs/API_SPEC.md)**: Chi tiết toàn bộ các API quản lý Kênh phát sóng (`/api/v1/channels`), tạo lịch đăng bài (`/api/v1/posts`), tạo Presigned URL upload ảnh/video (`/api/v1/media/*`) và gọi AI viết bài (`/api/v1/ai/generate`).
3. **[Hướng dẫn Triển khai VPS 2-Stack (`docs/DEPLOY_VPS.md`)](docs/DEPLOY_VPS.md)**: Kiến trúc triển khai tách biệt Stack Hạ tầng dùng chung (`/opt/infra`) và Stack Ứng dụng (`/opt/postpilot`), cấu hình Nginx Gateway và cấp chứng chỉ SSL Let's Encrypt.
4. **[Hướng dẫn Setup Meta App & Lấy Token Vĩnh Viễn (`docs/META_SETUP_GUIDE.md`)](docs/META_SETUP_GUIDE.md)**: Từng bước cấu hình trên `developers.facebook.com` và cách dùng API của hệ thống để đổi sang **Permanent Page Access Token** (không bao giờ hết hạn).
5. **[Sổ tay Vận hành & Đưa lên Cloud (`docs/OPERATIONS_RUNBOOK.md`)](docs/OPERATIONS_RUNBOOK.md)**: Hướng dẫn vận hành, cách scale nhiều Worker, bảng tra cứu mã lỗi và cảnh báo Telegram.

---

## 🗂 Cấu trúc Thư mục Dự án

```text
PostPilot/
├── Dockerfile                         # Multi-stage Dockerfile đóng gói 3 binary: fb-api, fb-worker, fb-migrate
├── docker-compose.yml                 # Hạ tầng phát triển Local: PostgreSQL 16, Redis 7, MinIO (S3), Migrator, API, Worker
├── docker-compose.prod.yml            # Stack ứng dụng Production (/opt/postpilot trên VPS)
├── infra/                             # Stack hạ tầng dùng chung trên VPS (/opt/infra: Nginx, Certbot, Postgres, Redis, MinIO)
├── Makefile                           # Các lệnh tắt: build, test, migrate-up, migrate-status, docker-up
├── .env.example                       # Cấu hình mẫu đầy đủ các biến môi trường
├── migrations/                        # Quản lý Database Schema Migration (nhúng vào binary qua embed.FS)
│   ├── embed.go                       # Nhúng *.sql vào binary Golang
│   ├── 000001_init_schema.{up,down}.sql             # Khởi tạo schema ban đầu
│   ├── 000002_add_rate_limits_and_webhooks.{up,down}.sql # Bổ sung webhook_events & ai_generation_logs
│   └── 000003_refactor_channels_schema.{up,down}.sql     # Refactor sang mô hình đa kênh (channels, channel_id, external_post_id)
├── cmd/                               # Các Entrypoint thực thi của hệ thống
│   ├── api/main.go                    # HTTP REST API Server + Giao diện Web Admin Dashboard (Port 8080)
│   ├── worker/main.go                 # Background Cron Scheduler + Fast/Heavy Queue Workers + Token Health Checker
│   └── migrate/main.go                # Công cụ CLI chạy Database Migration bằng Golang (up, down, status)
├── frontend/                          # Giao diện Next.js 14 Studio UI (Port 3000)
└── internal/                          # Các tầng nghiệp vụ cốt lõi (DDD & Hexagonal Architecture)
    ├── config/config.go               # Đọc cấu hình từ .env và biến môi trường
    ├── crypto/aes.go                  # Mã hóa & giải mã Access Token bằng AES-256-GCM
    ├── domain/                        # Core Domain: Post & Publishing (Độc lập với nền tảng MXH)
    │   ├── channel.go                 # Thực thể Channel (PlatformType, ExternalID, RateLimitUsage, TokenStatus)
    │   ├── media.go                   # Thực thể MediaAsset, MediaAttachment & phân loại QueueTier (FAST/HEAVY)
    │   ├── post.go                    # Aggregate Root Post, PublishLog, PublishJobPayload & Domain Invariants
    │   └── ports.go                   # Outbound Port PublisherPort, PublisherError & PublisherRegistry
    ├── database/
    │   ├── pgdriver/driver.go         # Native PostgreSQL v3 Wire Protocol driver (SCRAM-SHA-256, MD5, Simple Query)
    │   ├── migrator.go                # Engine thực thi SQL Migration tự động bằng Golang
    │   └── repository.go              # Repository thao tác PostgreSQL + InMemoryStore fallback
    ├── infra/
    │   ├── redis/client.go            # Redis Queue Client (postpilot:queue:fast/heavy) + Channel Rate-Limit Cache
    │   ├── storage/s3_minio.go        # Ký AWS Signature V4 Presigned PUT URL cho S3 / MinIO / Cloudflare R2
    │   ├── notifier/telegram.go       # Gửi cảnh báo thời gian thực qua Telegram Bot
    │   └── ai/gemini.go               # Tích hợp Google Gemini API tự động viết nội dung bài đăng
    ├── platform/                      # Secondary Platform Adapters thực thi domain.PublisherPort
    │   └── facebook/                  # Facebook Platform Adapter
    │       ├── adapter.go             # PublisherAdapter cầu nối giữa Core Domain và Meta Graph API
    │       ├── client.go              # HTTP Client gọi Meta Graph API v20.0 & đọc Header X-Page-Usage
    │       ├── auth.go                # Luồng OAuth2 & đổi Short-lived -> Long-lived -> Permanent Page Token
    │       ├── publisher.go           # Engine đăng bài: Text, Link, Single Photo, Multi-Photo Album, Chunked Video, Reels
    │       └── errors.go              # Phân loại mã lỗi Meta thực thi domain.PublisherError
    ├── worker/
    │   ├── scheduler.go               # Quét bài đến hạn đăng & kiểm tra sức khỏe Token qua PublisherRegistry
    │   └── processor.go               # Xử lý hàng đợi FAST/HEAVY qua PublisherPort, điều tiết Rate-Limit & Backoff
    └── api/                           # Primary HTTP Adapter (Tách biệt Handler theo Bounded Context)
        ├── server.go                  # HTTP Router, Dependency Injection & Middleware (CORS, Logging, Recovery)
        ├── publishing_handler.go      # Handler quản lý vòng đời bài viết (/api/v1/posts*)
        ├── channel_handler.go         # Handler quản lý Kênh phát sóng (/api/v1/channels*)
        ├── facebook_handler.go        # Handler tích hợp OAuth2 & Webhook Facebook (/api/v1/auth/facebook/*, /api/v1/webhooks/facebook)
        ├── media_handler.go           # Handler tải lên & Presign S3 Media (/api/v1/media/*)
        ├── ai_handler.go              # Handler sáng tạo nội dung AI (/api/v1/ai/generate)
        ├── system_handler.go          # Handler Healthcheck & Thống kê hệ thống (/healthz, /api/v1/stats)
        └── ui_dashboard.go            # Giao diện Web Control Center tích hợp sẵn tại http://localhost:8080
```

---

## ⚡ Hướng dẫn Khởi chạy Nhanh (Quickstart)

### Cách 1: Chạy toàn bộ hạ tầng bằng Docker Compose (Khuyên dùng)

1. Tạo file `.env` từ file mẫu:
   ```bash
   cp .env.example .env
   ```
2. Khởi động toàn bộ cụm dịch vụ (PostgreSQL, Redis, MinIO, Golang Migrator, API Server, Background Worker):
   ```bash
   make docker-up
   # Hoặc chạy trực tiếp:
   docker compose up -d --build
   ```
3. Truy cập các giao diện quản trị:
   * **Web Control Center & API Server:** `http://localhost:8080`
   * **Health Check API:** `http://localhost:8080/healthz`
   * **MinIO Console (Quản lý kho Ảnh/Video S3):** `http://localhost:9001` (User: `minioadmin` / Pass: `minioadmin123`)

---

### Cách 2: Chạy và Kiểm thử trực tiếp bằng Golang trên máy Local

Hệ thống được thiết kế **Zero External Dependency Blocker**: ngay cả khi bạn chưa bật Docker, API Server vẫn có thể khởi chạy ngay lập tức với chế độ `InMemoryStore` fallback để bạn trải nghiệm giao diện và test luồng API.

1. **Chạy toàn bộ Unit Test & End-to-End Test:**
   ```bash
   make test
   # Hoặc: go test -v ./...
   ```
2. **Biên dịch 3 file thực thi (`bin/fb-api`, `bin/fb-worker`, `bin/fb-migrate`):**
   ```bash
   make build
   ```
3. **Quản lý Database Migration bằng Golang (khi đã bật PostgreSQL):**
   ```bash
   make migrate-status   # Xem trạng thái các bản migration
   make migrate-up       # Áp dụng các bản migration mới nhất
   make migrate-down     # Rollback 1 phiên bản migration gần nhất
   ```
4. **Chạy API Server, Worker và Next.js Studio:**
   ```bash
   make run-api          # Chạy Backend API Server (Port 8080)
   make run-worker       # Chạy Background Worker & Scheduler
   make frontend-dev     # Chạy Next.js Studio UI (Port 3000)
   ```

5. **Truy cập ứng dụng:**
   * **Next.js Studio UI (Khuyên dùng):** `http://localhost:3000`
   * **Golang API & Embedded Dashboard:** `http://localhost:8080`
   * **Healthcheck & Queue Stats:** `http://localhost:8080/healthz`
   * **MinIO Console (S3 Storage):** `http://localhost:9001`
