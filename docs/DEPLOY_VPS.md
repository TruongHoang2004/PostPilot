# Hướng Dẫn Triển Khai PostPilot Lên VPS (Kiến Trúc 2-Stack Docker Compose + Nginx + SSL)

Tài liệu này hướng dẫn chi tiết từng bước triển khai và mở rộng hệ thống **PostPilot** trên máy chủ VPS Ubuntu/Debian theo kiến trúc **2 Stack Độc Lập**:
1. **Stack Hạ tầng dùng chung (`/opt/infra` — `infra/docker-compose.yml`)**: `shared-nginx`, `shared-certbot`, `shared-postgres`, `shared-redis`, `shared-minio` kết nối qua mạng Docker `infra-net`.
2. **Stack Ứng dụng PostPilot (`/opt/postpilot` — `docker-compose.prod.yml`)**: `fbposter-frontend`, `fbposter-api`, `fbposter-worker`, `fbposter-migrate`.

---

## 1. Yêu Cầu Cấu Hình VPS & Tên Miền

- **HĐH**: Ubuntu 22.04 LTS / 24.04 LTS hoặc Debian 12
- **CPU / RAM**: Tối thiểu 2 Cores, 4 GB RAM (+ 2 GB Swap), 30+ GB SSD
- **Tường lửa (UFW)**: Chỉ mở Port `22/SSH`, `80/HTTP`, `443/HTTPS`
- **Tên miền (2 Subdomain độc lập)**:
  - `postpilot.hexta.org` -> Trỏ vào **Next.js Studio UI** (`fbposter-frontend:3000`)
  - `api-postpilot.hexta.org` -> Trỏ vào **Go REST API** (`fbposter-api:8080`) và đường dẫn `/fb-media/` phục vụ file trực tiếp từ **MinIO S3** (`shared-minio:9000/fb-media/`).

---

## 2. Triển Khai Stack 1: Hạ Tầng Dùng Chung (`/opt/infra`)

Copy thư mục `infra/` trong repository lên `/opt/infra` trên VPS:

| Container Name | Network Alias | Port Mở Ngoài VPS | Port Nội Bộ (`infra-net`) | Mục đích sử dụng |
| :--- | :--- | :--- | :--- | :--- |
| **`shared-nginx`** | `nginx` | `80`, `443` | `80`, `443` | Reverse proxy, SSL termination, gateway tập trung |
| **`shared-certbot`** | `certbot` | Không | Không | Cấp và tự động gia hạn SSL Let's Encrypt |
| **`shared-postgres`** | `postgres` | `127.0.0.1:5432` | `5432` | Cung cấp Database PostgreSQL 16 |
| **`shared-redis`** | `redis` | `127.0.0.1:6379` | `6379` | Cache & Message Broker (`postpilot:queue:*`) |
| **`shared-minio`** | `minio` | `127.0.0.1:9001` (Console) | `9000` (S3 API) | S3-Compatible Object Storage |

Khởi chạy stack hạ tầng:
```bash
cd /opt/infra
cp .env.example .env
# Chỉnh sửa mật khẩu POSTGRES_PASSWORD và MINIO_ROOT_PASSWORD trong .env
docker compose up -d
```

---

## 3. Triển Khai Stack 2: Ứng Dụng PostPilot (`/opt/postpilot`)

1. Sao chép `docker-compose.prod.yml` và `.env.example` vào `/opt/postpilot`:
   ```bash
   mkdir -p /opt/postpilot && cd /opt/postpilot
   cp .env.example .env
   ```
2. Cấu hình các biến quan trọng trong `/opt/postpilot/.env`:
   - `TOKEN_ENCRYPTION_KEY`: Sinh bằng `openssl rand -hex 32`
   - `DATABASE_URL=postgres://fbposter:<PASS>@postgres:5432/fb_autoposter?sslmode=disable`
   - `REDIS_ADDR=redis:6379`
   - `S3_ENDPOINT=http://minio:9000`
   - `S3_PUBLIC_BASE_URL=https://api-postpilot.hexta.org/fb-media`
   - `FRONTEND_URL=https://postpilot.hexta.org`
   - `NEXT_PUBLIC_API_URL=https://api-postpilot.hexta.org`
   - `FB_REDIRECT_URI=https://postpilot.hexta.org/api/v1/auth/facebook/callback`
3. Khởi chạy ứng dụng (tự động chạy mọi migration còn thiếu, hiện là `000001` → `000007`, qua container `fbposter-migrate`):
   ```bash
   cd /opt/postpilot
   docker compose -f docker-compose.prod.yml pull
   docker compose -f docker-compose.prod.yml up -d
   ```

---

## 4. Cấp Chứng Chỉ SSL Let's Encrypt & Kích Hoạt HTTPS

Sau khi đã trỏ A Record cho cả 2 tên miền (`postpilot.hexta.org` và `api-postpilot.hexta.org`) về IP của VPS:

```bash
cd /opt/infra
./setup-ssl.sh your-email@example.com
```
Script `setup-ssl.sh` sẽ tự động gọi Certbot cấp chứng chỉ SSL cho cả 2 subdomain và reload `shared-nginx`.

---

## 5. Mở Rộng Hạ Tầng Cho Dự Án Mới Trên Cùng VPS

Nhờ thiết kế tách biệt `/opt/infra`, khi cần triển khai thêm dự án mới trên cùng VPS:
1. Gắn container của dự án mới vào external network `infra-net`.
2. Tạo thêm database mới trên `shared-postgres`:
   ```bash
   docker exec -it shared-postgres psql -U fbposter -d postgres -c "CREATE DATABASE new_app_db;"
   ```
3. Tạo thêm bucket mới trên `shared-minio` nếu cần:
   ```bash
   docker exec -i shared-minio-init mc mb --ignore-existing local/new-app-bucket
   ```
4. Thêm file cấu hình virtual host mới vào `/opt/infra/nginx/conf.d/` và chạy `docker compose exec nginx nginx -s reload`.

---

## 6. Sổ Tay Lệnh Vận Hành Nhanh Trên VPS

- **Xem log ứng dụng PostPilot:**
  ```bash
  cd /opt/postpilot && docker compose -f docker-compose.prod.yml logs -f --tail=100
  ```
- **Cập nhật phiên bản mới từ GitHub Container Registry (GHCR):**
  ```bash
  cd /opt/postpilot && docker compose -f docker-compose.prod.yml pull && docker compose -f docker-compose.prod.yml up -d --remove-orphans
  ```
- **Sao lưu cơ sở dữ liệu PostgreSQL:**
  ```bash
  docker exec -t shared-postgres pg_dump -U fbposter fb_autoposter > backup_$(date +%F).sql
  ```
- **Mở MinIO Web Console qua SSH Tunnel:**
  ```bash
  ssh -L 9001:localhost:9001 user@IP_VPS
  # Mở http://localhost:9001 trên trình duyệt máy cá nhân
  ```

## Cloudflare R2 (tùy chọn)

PostPilot dùng được Cloudflare R2 làm kho media thay MinIO — **chỉ bằng biến môi trường**, không cần đổi code. Mặc định không bật: nếu chưa điền thì hệ thống vẫn chạy với `STORAGE_PROVIDER=s3` (MinIO) và nếu chọn `r2` mà thiếu thông số thì API vẫn khởi động, chỉ upload trả `503` (xem `GET /healthz` → `storage_configured`).

1. Trên Cloudflare: tạo bucket R2, tạo **API token** (Object Read & Write) để lấy `Access Key ID`/`Secret Access Key`, bật **Public access** cho bucket để Meta tải được ảnh/video. Dùng **custom domain** cho production; domain `r2.dev` của Cloudflare chỉ dành cho thử nghiệm và bị giới hạn tốc độ.
2. Thêm **CORS** cho bucket nếu trình duyệt upload thẳng bằng presigned URL (`AllowedOrigins: https://postpilot.hexta.org`, `AllowedMethods: PUT`, `AllowedHeaders: *`).
3. Điền vào `/opt/postpilot/.env` rồi tạo lại container `api` để nhận biến mới (`docker restart` KHÔNG đọc lại `.env`): `docker compose -f docker-compose.prod.yml up -d api`.
   ```env
   STORAGE_PROVIDER=r2
   R2_ACCOUNT_ID=<Cloudflare account id>
   R2_ACCESS_KEY_ID=<access key>
   R2_SECRET_ACCESS_KEY=<secret key>
   R2_BUCKET=<tên bucket>
   R2_PUBLIC_BASE_URL=https://<domain công khai của bucket>
   # Tùy chọn: R2_ENDPOINT=https://<account>.eu.r2.cloudflarestorage.com (jurisdiction EU)
   ```
4. Kiểm tra: `docker compose -f docker-compose.prod.yml logs api | grep "Object storage"` (thiếu cấu hình sẽ có dòng cảnh báo liệt kê tên biến còn thiếu, không in giá trị), rồi `curl -s https://api-postpilot.hexta.org/healthz` phải có `"storage_provider":"r2","storage_configured":true`, rồi thử tải một ảnh trong tab **Bài soạn sẵn**.

Ghi chú: phần R2 đã có kiểm thử về chữ ký/URL nhưng **chưa được thử với tài khoản Cloudflare thật** — hãy thử upload một ảnh sau khi cấu hình. Endpoint mặc định `https://<account>.r2.cloudflarestorage.com`, vùng ký `auto`, kiểu path-style; URL ký sẵn bắt buộc `https`. Media cũ đã lưu theo MinIO giữ nguyên URL cũ.

## Triển khai & rollback migration 000007 (Bài soạn sẵn)

- **Thứ tự**: `migrate` chạy trước, sau đó `api`/`worker` (compose `depends_on: service_completed_successfully`). `api` cũng tự chạy migration khi khởi động. Đừng khởi động `worker` bản mới ngoài thứ tự này: `MarkPostPublished` đọc hai cột mới của `posts`.
- **Tương thích ngược**: 000007 chỉ thêm bảng/cột/chỉ mục (idempotent), nên binary cũ chạy được trên schema mới. Rollback **chỉ image** (giữ schema 000007) là an toàn nhưng bản cũ không đếm lượt đăng và không xoá mềm bài soạn sẵn; với những bài đã đăng trong khoảng đó, đối soát bằng SQL: `UPDATE library_items li SET archived_at = COALESCE(li.archived_at, NOW()) FROM posts p WHERE p.library_item_id = li.id AND p.delete_after_publish AND p.status = 'PUBLISHED';`
- **`migrate down` 000007 là phá huỷ**: xoá bảng `library_items` (toàn bộ bài soạn sẵn) và hai cột trên `posts`. Luôn `pg_dump` trước khi chạy.
