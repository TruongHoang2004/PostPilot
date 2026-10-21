# 🏗 Kiến Trúc Tổng Thể Hệ Thống PostPilot (DDD, Hexagonal & Infrastructure Architecture)

Tài liệu này mô tả toàn diện kiến trúc **Domain-Driven Design (DDD)** và **Hexagonal Architecture (Ports & Adapters)** của dự án **PostPilot**, phân ranh giới các Bounded Contexts, mô hình dữ liệu (Database & Queue), vòng đời trạng thái bài đăng, và kiến trúc triển khai hạ tầng Production (2-Stack Docker Compose & CI/CD).

---

## 1. Cấu Trúc Thư Mục Toàn Dự Án (Repository Tree)

```text
PostPilot/
├── cmd/                                # 3 Binary Entrypoints của Golang Backend
│   ├── api/main.go                     # HTTP REST API Server (:8080) & Embedded UI
│   ├── worker/main.go                  # Background Cron Scheduler & Queue Worker
│   └── migrate/main.go                 # Database Schema Migration CLI (up/down/status)
├── internal/                           # Core Backend Packages (Zero external Go dependencies)
│   ├── domain/                         # CORE DOMAIN: Post & Publishing (Platform-agnostic)
│   │   ├── channel.go                  # Channel Entity, PlatformType, RateLimitUsage, TokenStatus
│   │   ├── media.go                    # MediaAsset, MediaAttachment, MediaType & QueueTier Routing
│   │   ├── post.go                     # Post Aggregate Root, PublishLog, PublishJobPayload & Invariants
│   │   └── ports.go                    # Outbound Ports: PublisherPort, PublisherError, PublisherRegistry
│   ├── api/                            # PRIMARY ADAPTER: Modular HTTP Handlers by Bounded Context
│   │   ├── server.go                   # Router, Dependency Injection, Middleware (CORS, Logging, Recovery)
│   │   ├── publishing_handler.go       # Publishing Context (/api/v1/posts*)
│   │   ├── channel_handler.go          # Channel Context (/api/v1/channels*)
│   │   ├── facebook_handler.go         # Facebook Auth & Webhook Adapter (/api/v1/auth/facebook/*, /api/v1/webhooks/facebook)
│   │   ├── media_handler.go            # Media Context (/api/v1/media/*)
│   │   ├── ai_handler.go               # AI Content Context (/api/v1/ai/generate)
│   │   ├── system_handler.go           # Observability Context (/healthz, /api/v1/stats)
│   │   └── ui_dashboard.go             # Embedded HTML Dashboard
│   ├── facebook/                       # SECONDARY ADAPTER: Meta Graph API v20.0 implementing PublisherPort
│   │   ├── adapter.go                  # PublisherAdapter (implements domain.PublisherPort)
│   │   ├── client.go                   # HTTP Client, Rate-Limit Header Parser (X-Page-Usage)
│   │   ├── auth.go                     # OAuth2, Token Exchange (60 ngày) & Debug Token
│   │   ├── publisher.go                # Đăng Text, Link, Photo, Album, Chunked Video & Reel
│   │   └── errors.go                   # GraphAPIError (implements domain.PublisherError)
│   ├── worker/                         # APPLICATION ORCHESTRATION: Queue Worker & Cron Scheduler
│   │   ├── scheduler.go                # Quét bài đến hạn (FOR UPDATE SKIP LOCKED) & Check Token qua PublisherRegistry
│   │   └── processor.go                # Worker Pool 2 tầng (FAST / HEAVY) gọi PublisherPort & Exponential Backoff
│   ├── database/                       # SECONDARY ADAPTER: Repository & Custom Postgres Driver
│   │   ├── repository.go               # Store interface, PostgresStore & InMemoryStore fallback
│   │   ├── migrator.go                 # Quản lý bảng schema_migrations
│   │   └── pgdriver/driver.go          # Custom PostgreSQL Wire Protocol v3.0 Driver (SCRAM-SHA-256)
│   ├── infra/                          # SECONDARY ADAPTERS: Hạ tầng kỹ thuật
│   │   ├── redis/client.go             # Custom RESP2 TCP Client (postpilot:queue:fast/heavy) + Memory Fallback
│   │   ├── storage/s3_minio.go         # Custom AWS Signature V4 Presigned PUT & Direct Upload
│   │   ├── ai/gemini.go                # Google Gemini Content Generator + Template Fallback
│   │   └── notifier/telegram.go        # Cảnh báo vận hành qua Telegram Bot API
│   ├── config/                         # Đọc cấu hình từ .env và biến môi trường
│   │   └── config.go
│   └── crypto/                         # Mã hóa Token bằng AES-256-GCM & Masking
│       └── aes.go
├── migrations/                         # SQL Migrations nhúng vào binary qua //go:embed
│   ├── embed.go
│   ├── 000001_init_schema.{up,down}.sql
│   ├── 000002_add_rate_limits_and_webhooks.{up,down}.sql
│   └── 000003_refactor_channels_schema.{up,down}.sql
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
├── .github/workflows/                  # CI/CD Pipeline
│   └── docker-publish.yml              # Tự động build & push Backend/Frontend images lên GHCR
├── Dockerfile                          # Multi-stage build cho 3 Go binaries (fb-api, fb-worker, fb-migrate)
├── docker-compose.yml                  # Môi trường phát triển Local đầy đủ
└── docker-compose.prod.yml             # Stack ứng dụng Production (/opt/postpilot trên VPS)
```

---

## 2. Sơ Đồ Kiến Trúc DDD & Hexagonal (Ports & Adapters Graph)

Trong kiến trúc mới, **`internal/domain`** là trung tâm độc lập hoàn toàn với các nền tảng mạng xã hội. `internal/worker` và `internal/api` giao tiếp với các nền tảng bên ngoài thông qua giao diện trừu tượng `domain.PublisherPort` và `domain.PublisherRegistry`. Module `internal/facebook` đóng vai trò là một **Secondary Adapter** thực thi `domain.PublisherPort`:

```mermaid
flowchart TB
    subgraph Primary ["Primary Adapters (Driving)"]
        FE["Next.js 14 Studio UI (:3000)"]
        subgraph API_Handlers ["internal/api (Modular HTTP Handlers)"]
            H_PUB["publishing_handler.go (/api/v1/posts*)"]
            H_CH["channel_handler.go (/api/v1/channels*)"]
            H_FB["facebook_handler.go (/api/v1/auth/facebook/*)"]
            H_MED["media_handler.go (/api/v1/media/*)"]
            H_AI["ai_handler.go (/api/v1/ai/generate)"]
            H_SYS["system_handler.go (/healthz, /api/v1/stats)"]
        end
        FE -->|"REST API"| API_Handlers
    end

    subgraph Core ["Core Domain & Application Layer"]
        WORKER["internal/worker (Scheduler & Processor)"]
        subgraph Domain ["internal/domain (Post & Publishing Core)"]
            ENT_POST["Post Aggregate Root & PublishLog"]
            ENT_CH["Channel Entity (Platform, ExternalID)"]
            ENT_MED["MediaAsset & MediaAttachment"]
            PORT_PUB["PublisherPort & PublisherRegistry (Outbound Port)"]
        end
        WORKER --> Domain
        API_Handlers --> Domain
        API_Handlers --> WORKER
    end

    subgraph Secondary ["Secondary Adapters (Driven)"]
        FB_ADAPT["internal/facebook (PublisherAdapter implements PublisherPort)"]
        DB_ADAPT["internal/database (PostgresStore & InMemoryStore)"]
        REDIS_ADAPT["internal/infra/redis (Fast/Heavy Queue & RateLimit Cache)"]
        S3_ADAPT["internal/infra/storage (MinIO / S3 SigV4)"]
        AI_ADAPT["internal/infra/ai (Gemini Content Generator)"]
        TELE_ADAPT["internal/infra/notifier (Telegram Alert)"]
    end

    PORT_PUB -.->|"Implemented by"| FB_ADAPT
    WORKER --> DB_ADAPT
    WORKER --> REDIS_ADAPT
    WORKER --> TELE_ADAPT
    API_Handlers --> DB_ADAPT
    API_Handlers --> REDIS_ADAPT
    API_Handlers --> S3_ADAPT
    API_Handlers --> AI_ADAPT
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
            Certbot -.->|"Gia hạn SSL định kỳ"| Nginx
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
    participant Port as PublisherPort (Facebook Adapter)
    participant Tele as Telegram Alert Bot

    rect rgb(235, 245, 255)
        Note over Admin,Port: Luồng 1: Kết nối Kênh phát sóng & Bảo mật Token (OAuth2 / Manual)
        Admin->>Nginx: Kết nối Kênh (OAuth2 hoặc POST /api/v1/channels)
        Nginx->>API: GET /api/v1/auth/facebook/callback (hoặc POST /api/v1/channels)
        API->>Port: Đổi sang Long-Lived User Token (60 ngày) & GET /me/accounts
        Port-->>API: Trả về danh sách Kênh + Access Token
        API->>API: Mã hóa Token bằng AES-256-GCM (12-byte Nonce)
        API->>DB: Upsert vào bảng channels (platform, external_id, encrypted_access_token)
        API-->>Admin: Redirect về FRONTEND_URL (?oauth=success) / Trả về MaskedToken
    end

    rect rgb(240, 255, 240)
        Note over Admin,Redis: Luồng 2: Soạn thảo AI, Upload Media & Đặt lịch bài viết
        Admin->>API: POST /api/v1/ai/generate (Chủ đề + Giọng điệu)
        API->>AI: Gọi Gemini API (hoặc Template Fallback)
        AI-->>Admin: Trả về nội dung bài viết gợi ý
        Admin->>API: POST /api/v1/media/upload (hoặc /api/v1/media/presign)
        API->>AI: Lưu ./uploads & Upload S3/MinIO qua chữ ký AWS SigV4
        API->>DB: Lưu bản ghi vào bảng media_assets
        AI-->>Admin: Trả về public_url
        Admin->>API: POST /api/v1/posts (channel_id, content, media_urls, scheduled_at, publish_now)
        API->>API: Post.PrepareForCreation(): phân loại QueueTier FAST hoặc HEAVY
        API->>DB: INSERT vào bảng posts (status = SCHEDULED hoặc QUEUED)
        opt Nếu publish_now = true
            API->>Redis: LPUSH ngay vào postpilot:queue:fast hoặc postpilot:queue:heavy
        end
    end

    rect rgb(255, 248, 235)
        Note over DB,Tele: Luồng 3: Quét lịch định kỳ, Xử lý Hàng đợi 2 tầng & Đăng bài qua PublisherPort
        loop Định kỳ mỗi 15 giây (Scheduler.ScanAndEnqueueDuePosts)
            Worker->>DB: SELECT ... FOR UPDATE SKIP LOCKED (status='SCHEDULED' hoặc 'FAILED' đến hạn retry)
            DB-->>Worker: Cập nhật nguyên tử sang status='QUEUED' & trả về danh sách bài
            Worker->>Redis: LPUSH vào postpilot:queue:fast hoặc postpilot:queue:heavy
        end

        Worker->>Redis: Worker Pool (4 FAST + 2 HEAVY goroutines) gọi BRPOP
        Worker->>Redis: Kiểm tra Rate-Limit cache (postpilot:ratelimit:channel:<id>)
        alt Rate Limit >= 80% (Ngưỡng bảo vệ Kênh)
            Worker->>DB: Hoãn bài viết 5 phút (status='FAILED', next_retry_at = NOW() + 5m)
        else Rate Limit < 80% (An toàn)
            Worker->>DB: Đánh dấu status='PROCESSING' & Giải mã AES-256-GCM Channel Token
            Worker->>Port: PublisherRegistry.Get(channel.Platform) -> Publish(ctx, PublishRequest)
            Port-->>Worker: Phản hồi PublishResult (ExternalPostID, Steps, RateLimitUsage)
            Worker->>Redis: SETEX cập nhật chỉ số Rate-Limit (TTL 900s)
            Worker->>DB: Lưu publish_logs, cập nhật rate_limit_* & đánh dấu status='PUBLISHED'
        else Lỗi tạm thời (PublisherError.IsRetryable() == true)
            Worker->>DB: Tính Exponential Backoff (1p -> 5p -> 15p) & hẹn next_retry_at
        else Lỗi vĩnh viễn (IsTokenExpired() hoặc retry_count >= max_retries)
            Worker->>DB: Đánh dấu post='FAILED' (và channel token_status='EXPIRED' nếu hết hạn token)
            Worker->>Tele: Bắn cảnh báo Markdown khẩn cấp tới Admin qua Telegram
        end
    end
```

---

## 5. Vòng Đời Trạng Thái Bài Viết (Post State Machine)

Được định nghĩa trong [`internal/domain/post.go`](../internal/domain/post.go) và điều phối bởi [`internal/worker/scheduler.go`](../internal/worker/scheduler.go) cùng [`internal/worker/processor.go`](../internal/worker/processor.go):

```mermaid
stateDiagram-v2
    [*] --> SCHEDULED : POST /api/v1/posts (publish_now = false)
    [*] --> QUEUED : POST /api/v1/posts (publish_now = true)
    SCHEDULED --> QUEUED : Scheduler quét định kỳ 15s (scheduled_at <= NOW)
    SCHEDULED --> CANCELLED : POST /api/v1/posts/{id}/cancel (Post.CanCancel)
    FAILED --> CANCELLED : POST /api/v1/posts/{id}/cancel (Post.CanCancel)
    QUEUED --> PROCESSING : Worker BRPOP từ Redis (Rate Limit < 80%)
    QUEUED --> FAILED : Rate Limit >= 80% (Tự động hẹn next_retry_at + 5 phút)
    PROCESSING --> PUBLISHED : PublisherPort trả về ExternalPostID thành công
    PROCESSING --> FAILED : Lỗi tạm thời (retry_count < max_retries, Backoff 1m/5m/15m)
    FAILED --> QUEUED : Scheduler quét lại khi next_retry_at <= NOW
    PROCESSING --> FAILED_PERM : Lỗi vĩnh viễn (Token Expired) hoặc hết lượt retry (Gửi cảnh báo Telegram)
    PUBLISHED --> [*]
    CANCELLED --> [*]
```

---

## 6. Sơ Đồ Thực Thể Cơ Sở Dữ Liệu (PostgreSQL ER Diagram)

Cơ sở dữ liệu PostgreSQL 16 gồm 7 bảng được quản lý qua các bản migration `000001`, `000002` và `000003_refactor_channels_schema`:

```mermaid
erDiagram
    channels ||--o{ posts : "channel_id (ON DELETE CASCADE)"
    channels ||--o{ publish_logs : "channel_id (ON DELETE CASCADE)"
    posts ||--o{ publish_logs : "post_id (ON DELETE CASCADE)"

    channels {
        uuid id PK
        varchar platform "FACEBOOK | LINKEDIN | TWITTER | TIKTOK"
        varchar external_id "External Platform ID (UK with platform)"
        varchar name
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
        uuid channel_id FK
        text content
        text link_url
        varchar media_type "NONE | LINK | IMAGE | ALBUM | VIDEO | REEL"
        jsonb media_urls "Mảng URL ảnh/video"
        varchar status "DRAFT | SCHEDULED | QUEUED | PROCESSING | PUBLISHED | FAILED | CANCELLED"
        varchar queue_tier "FAST | HEAVY"
        timestamptz scheduled_at
        timestamptz published_at
        varchar external_post_id
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
        uuid channel_id FK
        int attempt_number
        varchar action "FEED_POST | PHOTO_POST | VIDEO_INIT | VIDEO_TRANSFER | VIDEO_FINISH | REEL_*"
        int http_status
        int error_code
        int error_subcode
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
        varchar external_id
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

1. **DDD & Hexagonal Architecture (Ports & Adapters)**:
   - Core Domain (`internal/domain`) định nghĩa các khái niệm trừu tượng (`Channel`, `Post`, `MediaAsset`, `MediaAttachment`, `PublisherPort`, `PublisherError`).
   - Mọi tương tác mạng xã hội đều đi qua `PublisherPort`, cho phép bổ sung các adapter mới (LinkedIn, TikTok, X) mà không phải sửa đổi logic trong `internal/worker` hay `internal/domain`.
2. **Zero External Go Dependencies (`go.mod` thuần Standard Library)**:
   - **PostgreSQL Driver tự viết** (`internal/database/pgdriver/driver.go`): Cài đặt trực tiếp giao thức PostgreSQL Wire Protocol v3.0 qua TCP, hỗ trợ xác thực `SCRAM-SHA-256`, `MD5` và Simple Query Protocol.
   - **Redis RESP2 Client tự viết** (`internal/infra/redis/client.go`): Giao tiếp TCP trực tiếp bằng giao thức RESP2 (`LPUSH`, `BRPOP`, `LLEN`, `SETEX`, `GET`, `DEL`).
   - **S3 AWS Signature V4 tự viết** (`internal/infra/storage/s3_minio.go`): Tự tính toán chữ ký `AWS4-HMAC-SHA256` để tạo Presigned PUT URL và upload trực tiếp lên MinIO/AWS S3.
3. **Cơ chế Graceful In-Memory Fallback**:
   - Khi chạy local/test mà không bật PostgreSQL hoặc Redis, cả `fb-api` và `fb-worker` tự động chuyển sang `InMemoryStore` và bộ nhớ đệm trong `redis.Client`.
4. **Bảo mật Token bằng AES-256-GCM** (`internal/crypto/aes.go`):
   - Access Token của mọi `Channel` luôn được mã hóa xác thực bằng `AES-256-GCM` với Nonce 12-byte ngẫu nhiên trước khi lưu xuống DB và luôn bị ẩn (`json:"-"`) khi trả JSON qua REST API, chỉ hiển thị chuỗi che `MaskToken`.
5. **Chống trùng lặp khi Scale nhiều Worker (`SELECT ... FOR UPDATE SKIP LOCKED`)**:
   - Hàm `PostgresStore.ClaimDuePosts` sử dụng CTE nguyên tử kết hợp khóa dòng `FOR UPDATE SKIP LOCKED`, đảm bảo nhiều container `fb-worker` chạy song song không bao giờ bốc trùng một bài đăng.
6. **Hàng đợi 2 tầng (`FAST` vs `HEAVY`) & Bảo vệ Rate-Limit tự động**:
   - Tách biệt hàng đợi `postpilot:queue:fast` (4 goroutines cho Text, Link, Ảnh, Album) và `postpilot:queue:heavy` (2 goroutines cho Video Chunked 3 bước & Reels).
   - Tự động cập nhật `RateLimitUsage` từ adapter sau mỗi lần gọi API và chủ động hoãn bài đăng 5 phút nếu mức sử dụng chạm ngưỡng `80%`.
