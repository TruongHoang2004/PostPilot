# Hướng Dẫn Quản Trị & Mở Rộng Hạ Tầng Dùng Chung (Infrastructure Stack)

Tài liệu này hướng dẫn cách sử dụng và mở rộng hệ thống hạ tầng tại thư mục `/opt/infra` trên VPS khi bạn muốn triển khai thêm các dịch vụ, ứng dụng hoặc website mới.

---

## 1. Tổng Quan Hạ Tầng (`/opt/infra`)

Tất cả các dịch vụ nền tảng đều hoạt động chung trong Docker network `infra-net`:

| Container Name | Network Aliases | Port Mở Ngoài VPS | Port Nội Bộ Trong `infra-net` | Mục đích sử dụng |
| :--- | :--- | :--- | :--- | :--- |
| **`shared-nginx`** | `nginx` | `80`, `443` | `80`, `443` | Reverse proxy, SSL termination, gateway tập trung |
| **`shared-certbot`** | `certbot` | Không | Không | Cấp và tự động gia hạn SSL Let's Encrypt |
| **`shared-postgres`** | `postgres` | `127.0.0.1:5432` | `5432` | Cung cấp Database PostgreSQL 16 |
| **`shared-redis`** | `redis` | `127.0.0.1:6379` | `6379` | Cache & Message Broker (Asynq/Celery/BullMQ) |
| **`shared-minio`** | `minio` | `127.0.0.1:9001` (Console) | `9000` (S3 API) | S3-Compatible Object Storage |

---

## 2. Hướng Dẫn Tích Hợp Dự Án / Website Mới Vào VPS

Giả sử trong tương lai bạn muốn deploy thêm một ứng dụng mới (ví dụ: `my-new-app` với tên miền `app.example.com`):

### Bước 1: Viết `docker-compose.yml` cho dự án mới
Trong file `docker-compose.yml` của dự án mới, bạn chỉ cần cấu hình để app tham gia vào network `infra-net`:

```yaml
services:
  web:
    image: my-new-app:latest
    container_name: my-new-app
    restart: unless-stopped
    environment:
      # Kết nối trực tiếp tới Postgres/Redis chung bằng alias:
      - DATABASE_URL=postgres://new_user:new_pass@postgres:5432/new_db
      - REDIS_URL=redis://redis:6379/1
    networks:
      - infra-net

networks:
  infra-net:
    name: infra-net
    external: true
```

### Bước 2: Tạo thêm Virtual Host trong Nginx
Trên VPS, tạo một file cấu hình mới tại `/opt/infra/nginx/conf.d/my-new-app.conf`:

```nginx
upstream my_new_app {
    server my-new-app:3000; # Tên container và port nội bộ của app mới
    keepalive 32;
}

server {
    listen 80;
    server_name app.example.com;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        proxy_pass http://my_new_app;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Bước 3: Reload Nginx & Cấp SSL
```bash
# 1. Reload Nginx
ssh eztech "cd /opt/infra && docker compose exec nginx nginx -s reload"

# 2. Cấp chứng chỉ SSL cho domain mới
ssh eztech "cd /opt/infra && docker compose run --rm --entrypoint '\
  certbot certonly --webroot -w /var/www/certbot \
  -d app.example.com \
  --email your-email@gmail.com \
  --agree-tos \
  --no-eff-email' certbot"
```

---

## 3. Tạo Cơ Sở Dữ Liệu Mới Trên `shared-postgres`

Khi có dự án mới cần database riêng biệt, bạn không cần cài thêm PostgreSQL:

```bash
# Mở psql console
ssh eztech "docker exec -it shared-postgres psql -U fbposter -d postgres"

# Tạo User và Database mới trong psql:
CREATE USER myapp_user WITH PASSWORD 'secure_password_here';
CREATE DATABASE myapp_db OWNER myapp_user;
GRANT ALL PRIVILEGES ON DATABASE myapp_db TO myapp_user;
\q
```

Chuỗi kết nối từ container mới:
```text
postgres://myapp_user:secure_password_here@postgres:5432/myapp_db?sslmode=disable
```

---

## 4. Tạo Bucket Mới Trên `shared-minio`

Bạn có thể tạo bucket lưu file cho dự án mới bằng lệnh `mc` (MinIO Client):

```bash
# Tạo bucket mới tên là 'myapp-storage'
ssh eztech "docker exec -i shared-minio-init mc mb --ignore-existing local/myapp-storage"

# Nếu muốn bucket này cho phép tải ảnh/video public:
ssh eztech "docker exec -i shared-minio-init mc anonymous set download local/myapp-storage"
```

---

## 5. Quản Lý & Bảo Trì Stack Hạ Tầng

- **Khởi động / Khởi động lại toàn bộ hạ tầng:**
  ```bash
  ssh eztech "cd /opt/infra && docker compose restart"
  ```
- **Kiểm tra trạng thái hạ tầng:**
  ```bash
  ssh eztech "cd /opt/infra && docker compose ps"
  ```
- **Kiểm tra log của Nginx Gateway:**
  ```bash
  ssh eztech "cd /opt/infra && docker compose logs -f --tail=100 nginx"
  ```
