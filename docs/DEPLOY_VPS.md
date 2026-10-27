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
   - `FB_REDIRECT_URI=https://api-postpilot.hexta.org/api/v1/auth/facebook/callback`
3. Khởi chạy ứng dụng (tự động chạy migration `000001` -> `000003` qua container `fbposter-migrate`):
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
