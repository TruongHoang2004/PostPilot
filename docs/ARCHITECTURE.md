# 🏗 Kiến Trúc Tổng Thể Hệ Thống PostPilot (Codebase & Infrastructure Architecture)

Tài liệu này mô tả toàn diện kiến trúc mã nguồn, sự phụ thuộc giữa các package (Go Backend & Next.js Frontend), mô hình dữ liệu (Database & Queue), vòng đời trạng thái bài đăng, và kiến trúc triển khai hạ tầng Production (2-Stack Docker Compose & CI/CD).

---

## 1. Cấu Trúc Thư Mục Toàn Dự Án (Repository Tree)

```text
PostPilot/
├── cmd/                                # 3 Binary Entrypoints của Golang Backend
│   ├── api/main.go                     # HTTP REST API Server (:8080) & Embedded UI
│   ├── worker/main.go                  # Background Cron Scheduler & Queue Worker
│   └── migrate/main.go                 # Database Schema Migration CLI (up/down/status)
├── internal/                           # Core Backend Packages (Zero external Go dependencies)
│   ├── api/                            # HTTP Router, Middleware, REST v1 Handlers, Embedded HTML
│   │   ├── server.go
│   │   ├── handlers.go
│   │   └── ui_dashboard.go
│   ├── config/                         # Đọc cấu hình từ .env và biến môi trường
│   │   └── config.go
│   ├── crypto/                         # Mã hóa Token bằng AES-256-GCM & Masking
│   │   └── aes.go
│   ├── database/                       # Repository Pattern, Migrator & Custom Postgres Driver
│   │   ├── repository.go               # Store interface, PostgresStore & InMemoryStore fallback
│   │   ├── migrator.go                 # Quản lý bảng schema_migrations
│   │   └── pgdriver/driver.go          # Custom PostgreSQL Wire Protocol v3.0 Driver (SCRAM-SHA-256)
│   ├── domain/                         # Domain Entities, Enums & DTOs
│   │   └── models.go
│   ├── facebook/                       # Tích hợp Meta Graph API v20.0
│   │   ├── client.go                   # HTTP Client, Rate-Limit Header Parser (X-Page-Usage)
│   │   ├── auth.go                     # OAuth2, Token Exchange (60 ngày) & Debug Token
│   │   ├── publisher.go                # Đăng Text, Link, Photo, Album, Chunked Video & Reel
│   │   └── errors.go                   # Phân loại lỗi GraphAPIError (Retryable vs Token Invalid)
│   ├── infra/                          # Các Adapter kết nối dịch vụ hạ tầng
│   │   ├── redis/client.go             # Custom RESP2 TCP Client (LPUSH/BRPOP/SETEX) + Memory Fallback
│   │   ├── storage/s3_minio.go         # Custom AWS Signature V4 Presigned PUT & Direct Upload
│   │   ├── ai/gemini.go                # Google Gemini Content Generator + Template Fallback
│   │   └── notifier/telegram.go        # Cảnh báo vận hành qua Telegram Bot API
│   └── worker/                         # Điều phối & Xử lý hàng đợi bất đồng bộ
│       ├── scheduler.go                # Quét bài đến hạn (FOR UPDATE SKIP LOCKED) & Check Token
│       └── processor.go                # Worker Pool 2 tầng (FAST / HEAVY) & Exponential Backoff
├── migrations/                         # SQL Migrations nhúng vào binary qua //go:embed
│   ├── embed.go
│   ├── 000001_init_schema.{up,down}.sql
│   └── 000002_add_rate_limits_and_webhooks.{up,down}.sql
├── frontend/                           # Next.js 14 App Router Studio UI (:3000)
│   └── src/
│       ├── app/                        # RootLayout (layout.tsx) & Main Studio Page (page.tsx)
│       ├── components/                 # Header, StatsCards, PostComposer, PostList, PageManager, PostDetailModal
│       ├── context/                    # ThemeContext (Dark/Light) & DialogContext (Modal/Toast)
│       └── lib/                        # API Client (api.ts), TypeScript Interfaces (types.ts), Utils
├── infra/                              # Shared Infrastructure Stack (/opt/infra trên VPS)
│   ├── docker-compose.yml              # shared-nginx, shared-certbot, shared-postgres, shared-redis, shared-minio
│   └── nginx/                          # Cấu hình Gateway Reverse Proxy tách 2 tên miền
│       ├── nginx.conf
│       └── conf.d/{frontend,backend}.conf
├── nginx/                              # Cấu hình Nginx mẫu tại repo root
├── .github/workflows/                  # CI/CD Pipeline
│   └── docker-publish.yml              # Tự động build & push Backend/Frontend images lên GHCR
├── Dockerfile                          # Multi-stage build cho 3 Go binaries (fb-api, fb-worker, fb-migrate)
├── docker-compose.yml                  # Môi trường phát triển Local đầy đủ
└── docker-compose.prod.yml             # Stack ứng dụng Production (/opt/postpilot trên VPS)
```

---

## 2. Sơ Đồ Kiến Trúc Module & Dependency (Codebase Component Graph)

Sơ đồ dưới đây thể hiện mối quan hệ giữa **Frontend Next.js**, **3 Go Executables (`cmd/*`)**, các **Internal Packages (`internal/*`)** và các hệ thống bên ngoài:

```mermaid
flowchart TB
    subgraph Frontend ["Frontend Studio (Next.js 14 - Port 3000)"]
        direction TB
        FE_Page["app/page.tsx (Home)"]
        FE_Comp["components/* (PostComposer, PageManager, PostList, StatsCards, PostDetailModal)"]
        FE_Ctx["context/* (ThemeContext, DialogContext)"]
        FE_API["lib/api.ts & next.config.mjs Rewrites"]
        FE_Page --> FE_Comp
        FE_Page --> FE_Ctx
        FE_Comp --> FE_API
    end

    subgraph Cmd ["Go Entrypoints (cmd/*)"]
        CMD_API["cmd/api/main.go (fb-api :8080)"]
        CMD_WORKER["cmd/worker/main.go (fb-worker)"]
        CMD_MIGRATE["cmd/migrate/main.go (fb-migrate)"]
    end

    subgraph Core ["Application & Domain Layer (internal/*)"]
        API_PKG["internal/api (Server, Middleware, Handlers, Embedded UI)"]
        WORKER_PKG["internal/worker (Scheduler & Processor)"]
        FB_PKG["internal/facebook (Client, Auth, Publisher, Errors)"]
        CRYPTO_PKG["internal/crypto (TokenCipher AES-256-GCM)"]
        CFG_PKG["internal/config (Config Loader)"]
        DOMAIN_PKG["internal/domain (Models & Constants)"]
    end

    subgraph DataInfra ["Data & Infrastructure Adapters (Zero-Dependency)"]
        DB_PKG["internal/database (Store, PostgresStore, InMemoryStore, Migrator)"]
        PG_DRV["internal/database/pgdriver (Wire Protocol v3.0 Driver)"]
        REDIS_PKG["internal/infra/redis (RESP2 TCP Client + Mem Fallback)"]
        S3_PKG["internal/infra/storage (AWS SigV4 S3/MinIO Client)"]
        AI_PKG["internal/infra/ai (GeminiGenerator)"]
        TELE_PKG["internal/infra/notifier (TelegramNotifier)"]
        MIG_FS["migrations/embed.go (//go:embed *.sql)"]
    end

    subgraph External ["External Services & Storage"]
        PG_DB[("PostgreSQL 16")]
        REDIS_SRV[("Redis 7")]
        MINIO_SRV[("MinIO S3 / Local ./uploads")]
        META_API["Meta Graph API v20.0"]
        GEMINI_API["Google Gemini API"]
        TELE_API["Telegram Bot API"]
    end

    FE_API -->|"REST API /api/v1/* & /healthz"| API_PKG

    CMD_API --> API_PKG
    CMD_API --> WORKER_PKG
    CMD_WORKER --> WORKER_PKG
    CMD_MIGRATE --> DB_PKG

    API_PKG --> WORKER_PKG
    API_PKG --> FB_PKG
    API_PKG --> CRYPTO_PKG
    API_PKG --> DB_PKG
    API_PKG --> REDIS_PKG
    API_PKG --> S3_PKG
    API_PKG --> AI_PKG
    API_PKG --> TELE_PKG
    API_PKG --> DOMAIN_PKG
    API_PKG --> CFG_PKG

    WORKER_PKG --> FB_PKG
    WORKER_PKG --> CRYPTO_PKG
    WORKER_PKG --> DB_PKG
    WORKER_PKG --> REDIS_PKG
    WORKER_PKG --> TELE_PKG
    WORKER_PKG --> DOMAIN_PKG
    WORKER_PKG --> CFG_PKG

    DB_PKG --> PG_DRV
    DB_PKG --> MIG_FS
    DB_PKG --> DOMAIN_PKG

    PG_DRV -->|"TCP :5432"| PG_DB
    REDIS_PKG -->|"RESP2 TCP :6379"| REDIS_SRV
    S3_PKG -->|"HTTP PUT SigV4 :9000"| MINIO_SRV
    FB_PKG -->|"HTTPS graph.facebook.com"| META_API
    AI_PKG -->|"HTTPS generativelanguage.googleapis.com"| GEMINI_API
    TELE_PKG -->|"HTTPS api.telegram.org"| TELE_API
```

---

## 3. Kiến Trúc Hạ Tầng Triển Khai & CI/CD (2-Stack Production Topology)

Hệ thống Production trên VPS được phân tách thành **2 Docker Compose Stack độc lập** kết nối qua mạng nội bộ `infra-net`, kết hợp với luồng tự động build image qua **GitHub Actions (GHCR)**:

```mermaid
flowchart LR
    subgraph CICD ["GitHub Actions CI/CD (.github/workflows/docker-publish.yml)"]
        GitPush["Git Push (main / tags)"] --> Buildx["Docker Buildx"]
        Buildx --> GHCR_BE["ghcr.io/.../postpilot-backend"]
        Buildx --> GHCR_FE["ghcr.io/.../postpilot-frontend"]
    end

    subgraph Users ["Clients & External Actors"]
        Browser["Người vận hành (Trình duyệt)"]
        FBCrawler["Meta Graph API / Media Crawler"]
    end

    subgraph VPS ["Máy Chủ VPS (Docker Network: infra-net)"]
        subgraph InfraStack ["Stack 1: Hạ Tầng Dùng Chung (/opt/infra)"]
            Nginx["shared-nginx (:80, :443)"]
            Certbot["shared-certbot (Let's Encrypt SSL)"]
            Postgres[("shared-postgres (PostgreSQL 16 :5432)")]
            Redis[("shared-redis (Redis 7 :6379)")]
            MinIO[("shared-minio (S3 Storage :9000)")]
            Certbot -.->|"Gia hạn SSL định kỳ 12d"| Nginx
        end

        subgraph AppStack ["Stack 2: Ứng Dụng PostPilot (/opt/postpilot)"]
            FE_Container["fbposter-frontend (Next.js :3000)"]
            API_Container["fbposter-api (Go API :8080)"]
            Worker_Container["fbposter-worker (Go Scheduler & Worker)"]
            Migrate_Container["fbposter-migrate (Go DB Migrator)"]
            UploadsVol[("Volume: uploads_data")]
        end
    end

    GHCR_FE -.->|"docker compose pull"| FE_Container
    GHCR_BE -.->|"docker compose pull"| API_Container
    GHCR_BE -.->|"docker compose pull"| Worker_Container
    GHCR_BE -.->|"docker compose pull"| Migrate_Container

    Browser -->|"postpilot.nhavuonngoclam.com"| Nginx
    Browser -->|"api-postpilot.nhavuonngoclam.com"| Nginx
    FBCrawler -->|"api-postpilot.nhavuonngoclam.com/fb-media/*"| Nginx

    Nginx -->|"Proxy pass :3000"| FE_Container
    Nginx -->|"Proxy pass :8080 (/api/v1/*, /healthz)"| API_Container
    Nginx -->|"Proxy pass :9000 (/fb-media/*)"| MinIO

    FE_Container -->|"SSR / Rewrite Proxy"| API_Container
    Migrate_Container -->|"1. Chạy SQL Migrations"| Postgres
    API_Container -->|"2. Đọc/Ghi dữ liệu"| Postgres
    API_Container -->|"LPUSH Queue & Rate-Limit"| Redis
    API_Container -->|"Presign / Upload"| MinIO
    API_Container --- UploadsVol
    Worker_Container -->|"ClaimDuePosts & Logs"| Postgres
    Worker_Container -->|"BRPOP Queue & Rate-Limit"| Redis
    Worker_Container --- UploadsVol
```

---

## 4. Sơ Đồ Luồng Xử Lý Nghiệp Vụ End-to-End (Sequence Diagram)

```mermaid
sequenceDiagram
    autonumber
    actor Admin as Người Vận Hành (Next.js Studio)
    participant Nginx as Nginx Gateway
    participant API as Go API Server (fb-api)
    participant AI as Gemini AI / S3 MinIO
    participant DB as PostgreSQL 16
    participant Redis as Redis 7 (FAST & HEAVY Queues)
    participant Worker as Go Worker & Scheduler (fb-worker)
    participant Meta as Meta Graph API v20.0
    participant Tele as Telegram Alert Bot

    rect rgb(235, 245, 255)
        Note over Admin,Meta: Luồng 1: Kết nối Fanpage & Bảo mật Token (OAuth2 / Manual)
        Admin->>Nginx: Kết nối Fanpage (OAuth2 hoặc nhập User/Page Token)
        Nginx->>API: GET /api/v1/auth/facebook/callback (hoặc POST /api/v1/pages)
        API->>Meta: Đổi sang Long-Lived User Token (60 ngày) & GET /me/accounts
        Meta-->>API: Trả về danh sách Fanpage + Page Access Token
        API->>API: Mã hóa Token bằng AES-256-GCM (12-byte Nonce)
        API->>DB: Upsert vào bảng facebook_pages (encrypted_access_token)
        API-->>Admin: Redirect về FRONTEND_URL (?oauth=success) / Trả về MaskedToken
    end

    rect rgb(240, 255, 240)
        Note over Admin,Redis: Luồng 2: Soạn thảo AI, Upload Media & Đặt lịch bài viết
        Admin->>API: POST /api/v1/ai/generate (Chủ đề + Giọng điệu)
        API->>AI: Gọi Gemini API (hoặc Template Fallback nếu chưa cấu hình key)
        AI-->>Admin: Trả về nội dung bài viết gợi ý
        Admin->>API: POST /api/v1/media/upload (hoặc /api/v1/media/presign)
        API->>AI: Lưu ./uploads & Upload S3/MinIO qua chữ ký AWS SigV4
        API->>DB: Lưu bản ghi vào bảng media_assets
        AI-->>Admin: Trả về public_url (https://api-postpilot.../fb-media/...)
        Admin->>API: POST /api/v1/posts (page_uuid, content, media_urls, scheduled_at, publish_now)
        API->>API: Phân loại QueueTier: FAST (Text/Link/Image/Album) hoặc HEAVY (Video/Reel)
        API->>DB: INSERT vào bảng posts (status = SCHEDULED hoặc QUEUED)
        opt Nếu publish_now = true
            API->>Redis: LPUSH ngay vào fbposter:queue:fast hoặc fbposter:queue:heavy
        end
    end

    rect rgb(255, 248, 235)
        Note over DB,Tele: Luồng 3: Quét lịch định kỳ, Xử lý Hàng đợi 2 tầng & Đăng bài
        loop Định kỳ mỗi 15 giây (Scheduler.ScanAndEnqueueDuePosts)
            Worker->>DB: SELECT ... FOR UPDATE SKIP LOCKED (status='SCHEDULED' hoặc 'FAILED' đến hạn retry)
            DB-->>Worker: Cập nhật nguyên tử sang status='QUEUED' & trả về danh sách bài
            Worker->>Redis: LPUSH vào fbposter:queue:fast hoặc fbposter:queue:heavy
        end

        Worker->>Redis: Worker Pool (4 FAST + 2 HEAVY goroutines) gọi BRPOP
        Worker->>Redis: Kiểm tra Rate-Limit cache (fbposter:ratelimit:page:<id>)
        alt Rate Limit >= 80% (Ngưỡng bảo vệ Fanpage)
            Worker->>DB: Hoãn bài viết 5 phút (status='FAILED', next_retry_at = NOW() + 5m)
        else Rate Limit < 80% (An toàn)
            Worker->>DB: Đánh dấu status='PROCESSING' & Giải mã AES-256-GCM Page Token
            Worker->>Meta: Gọi Graph API (/feed, /photos, /videos chunked, /video_reels)
            Meta-->>Worker: Phản hồi fb_post_id + Header X-Page-Usage / X-App-Usage
            Worker->>Redis: SETEX cập nhật chỉ số Rate-Limit (TTL 900s)
            Worker->>DB: Lưu publish_logs, cập nhật rate_limit_* & đánh dấu status='PUBLISHED'
        else Lỗi tạm thời (HTTP 5xx, Network, Rate Limit Code 4/17/32/613)
            Worker->>DB: Tính Exponential Backoff (1p -> 5p -> 15p) & hẹn next_retry_at
        else Lỗi vĩnh viễn (Code 190/102 Token hết hạn hoặc retry_count >= max_retries)
            Worker->>DB: Đánh dấu post='FAILED' (và page token_status='EXPIRED' nếu lỗi 190)
            Worker->>Tele: Bắn cảnh báo Markdown khẩn cấp tới Admin qua Telegram
        end
    end
```

---

## 5. Vòng Đời Trạng Thái Bài Viết (Post State Machine)

Được định nghĩa trong [`internal/domain/models.go`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/domain/models.go#L19-L27) và điều phối bởi [`internal/worker/scheduler.go`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/worker/scheduler.go#L78-L105) cùng [`internal/worker/processor.go`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/worker/processor.go#L106-L238):

```mermaid
stateDiagram-v2
    [*] --> SCHEDULED : POST /api/v1/posts (publish_now = false)
    [*] --> QUEUED : POST /api/v1/posts (publish_now = true)
    SCHEDULED --> QUEUED : Scheduler quét định kỳ 15s (scheduled_at <= NOW)
    SCHEDULED --> CANCELLED : POST /api/v1/posts/{id}/cancel
    FAILED --> CANCELLED : POST /api/v1/posts/{id}/cancel
    QUEUED --> PROCESSING : Worker BRPOP từ Redis (Rate Limit < 80%)
    QUEUED --> FAILED : Rate Limit >= 80% (Tự động hẹn next_retry_at + 5 phút)
    PROCESSING --> PUBLISHED : Meta Graph API trả về fb_post_id thành công
    PROCESSING --> FAILED : Lỗi tạm thời (retry_count < max_retries, Backoff 1m/5m/15m)
    FAILED --> QUEUED : Scheduler quét lại khi next_retry_at <= NOW
    PROCESSING --> FAILED_PERM : Lỗi vĩnh viễn (Code 190) hoặc hết lượt retry (Gửi cảnh báo Telegram)
    PUBLISHED --> [*]
    CANCELLED --> [*]
```

---

## 6. Sơ Đồ Thực Thể Cơ Sở Dữ Liệu (PostgreSQL ER Diagram)

Cơ sở dữ liệu PostgreSQL 16 gồm 7 bảng được khởi tạo tự động qua [`migrations/000001_init_schema.up.sql`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/migrations/000001_init_schema.up.sql) và [`migrations/000002_add_rate_limits_and_webhooks.up.sql`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/migrations/000002_add_rate_limits_and_webhooks.up.sql):

```mermaid
erDiagram
    facebook_pages ||--o{ posts : "page_uuid (ON DELETE CASCADE)"
    facebook_pages ||--o{ publish_logs : "page_uuid (ON DELETE CASCADE)"
    posts ||--o{ publish_logs : "post_id (ON DELETE CASCADE)"

    facebook_pages {
        uuid id PK
        varchar page_id UK "Facebook Page ID"
        varchar page_name
        varchar category
        text encrypted_access_token "AES-256-GCM Base64"
        varchar token_status "ACTIVE | EXPIRED | REVOKED"
        timestamptz token_expires_at
        timestamptz last_checked_at
        int rate_limit_call_count
        int rate_limit_total_time
        int rate_limit_total_cputime
        boolean is_active
        timestamptz created_at
        timestamptz updated_at
    }

    posts {
        uuid id PK
        uuid page_uuid FK
        text content
        text link_url
        varchar media_type "NONE | LINK | IMAGE | ALBUM | VIDEO | REEL"
        jsonb media_urls "Mảng URL ảnh/video"
        varchar status "DRAFT | SCHEDULED | QUEUED | PROCESSING | PUBLISHED | FAILED | CANCELLED"
        varchar queue_tier "FAST | HEAVY"
        timestamptz scheduled_at
        timestamptz published_at
        varchar fb_post_id
        int retry_count
        int max_retries
        timestamptz next_retry_at
        int last_error_code
        text last_error_message
        varchar created_by
        timestamptz created_at
        timestamptz updated_at
    }

    publish_logs {
        uuid id PK
        uuid post_id FK
        uuid page_uuid FK
        int attempt_number
        varchar action "FEED_POST | PHOTO_POST | VIDEO_INIT | VIDEO_TRANSFER | VIDEO_FINISH | REEL_*"
        int http_status
        int fb_error_code
        int fb_error_subcode
        text request_summary
        text response_body
        bigint duration_ms
        timestamptz created_at
    }

    media_assets {
        uuid id PK
        varchar file_name
        varchar storage_key UK
        text public_url
        varchar mime_type
        bigint file_size_bytes
        int duration_seconds
        timestamptz created_at
    }

    schema_migrations {
        varchar version PK
        varchar name
        timestamptz applied_at
    }

    webhook_events {
        uuid id PK
        varchar object_type
        varchar page_id
        varchar event_type
        jsonb payload
        boolean processed
        timestamptz created_at
    }

    ai_generation_logs {
        uuid id PK
        text topic
        varchar tone
        text generated_content
        varchar model_used
        timestamptz created_at
    }
```

---

## 7. Các Quyết Định Thiết Kế Kỹ Thuật Đặc Trưng Của Codebase

1. **Zero External Go Dependencies (`go.mod` thuần Standard Library)**:
   - **PostgreSQL Driver tự viết** ([`internal/database/pgdriver/driver.go`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/database/pgdriver/driver.go)): Cài đặt trực tiếp giao thức PostgreSQL Wire Protocol v3.0 qua TCP, hỗ trợ xác thực `SCRAM-SHA-256`, `MD5` và Simple Query Protocol.
   - **Redis RESP2 Client tự viết** ([`internal/infra/redis/client.go`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/infra/redis/client.go)): Giao tiếp TCP trực tiếp bằng giao thức RESP2 (`LPUSH`, `BRPOP`, `LLEN`, `SETEX`, `GET`, `DEL`).
   - **S3 AWS Signature V4 tự viết** ([`internal/infra/storage/s3_minio.go`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/infra/storage/s3_minio.go)): Tự tính toán chữ ký `AWS4-HMAC-SHA256` để tạo Presigned PUT URL và upload trực tiếp lên MinIO/AWS S3.
2. **Cơ chế Graceful In-Memory Fallback**:
   - Khi chạy local/test mà không bật PostgreSQL hoặc Redis, cả `fb-api` và `fb-worker` tự động chuyển sang [`InMemoryStore`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/database/repository.go#L497-L503) và bộ nhớ đệm trong [`redis.Client`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/infra/redis/client.go#L30-L39). Ngoài ra `fb-api` nhúng sẵn một [`worker.Processor`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/cmd/api/main.go#L64) để hỗ trợ đăng đồng bộ (`?sync=true`).
3. **Bảo mật Token bằng AES-256-GCM** ([`internal/crypto/aes.go`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/crypto/aes.go#L17-L105)):
   - Page Access Token luôn được mã hóa xác thực bằng `AES-256-GCM` với Nonce 12-byte ngẫu nhiên trước khi lưu xuống DB và luôn bị ẩn (`json:"-"`) khi trả JSON qua REST API, chỉ hiển thị chuỗi che [`MaskToken`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/crypto/aes.go#L107-L116).
4. **Chống trùng lặp khi Scale nhiều Worker (`SELECT ... FOR UPDATE SKIP LOCKED`)**:
   - Hàm [`PostgresStore.ClaimDuePosts`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/database/repository.go#L308-L343) sử dụng CTE nguyên tử kết hợp khóa dòng `FOR UPDATE SKIP LOCKED`, đảm bảo nhiều container `fb-worker` chạy song song không bao giờ bốc trùng một bài đăng.
5. **Hàng đợi 2 tầng (`FAST` vs `HEAVY`) & Bảo vệ Rate-Limit tự động**:
   - Tách biệt hàng đợi `fbposter:queue:fast` (4 goroutines cho Text, Link, Ảnh, Album) và `fbposter:queue:heavy` (2 goroutines cho Video Chunked 3 bước & Reels) để các tác vụ upload video nặng không gây nghẽn bài đăng thông thường.
   - Tự động đọc header `X-Page-Usage` và `X-App-Usage` sau mỗi lần gọi Meta API ([`ParseRateLimitHeaders`](file:///Users/truonghoang/Documents/dev/personal/PostPilot/internal/facebook/client.go#L182-L204)) và chủ động hoãn bài đăng 5 phút nếu mức sử dụng chạm ngưỡng `80%`.
