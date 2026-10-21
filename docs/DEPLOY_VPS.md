# Hướng Dẫn Triển Khai PostPilot Lên VPS (Docker Compose + Nginx + SSL)

Tài liệu này hướng dẫn chi tiết từng bước đưa hệ thống **PostPilot** (Next.js Studio + Go API + Worker + PostgreSQL + Redis + MinIO + Nginx) lên máy chủ ảo (VPS) chạy hệ điều hành Ubuntu/Debian.

---

## 1. Yêu Cầu Cấu Hình VPS Khuyến Nghị

- **HĐH**: Ubuntu 22.04 LTS / 24.04 LTS hoặc Debian 12
- **CPU**: Tối thiểu 2 Cores (Khuyến nghị 4 Cores nếu xử lý video nặng)
- **RAM**: Tối thiểu 4 GB (Khuyến nghị 6 - 8 GB vì có Next.js, Postgres, Redis, MinIO và AI Worker)
- **Ổ cứng (SSD/NVMe)**: Tối thiểu 30 GB (Khuyến nghị 50+ GB để lưu trữ media video/ảnh bài đăng)
- **Tên miền (Domain)**: 1 tên miền đã trỏ DNS (A Record) về IP của VPS (ví dụ: `postpilot.yourdomain.com`).

---

## 2. Chuẩn Bị Môi Trường Trên VPS

### 2.1. Cập nhật hệ thống & Cài đặt Docker
Đăng nhập SSH vào VPS và chạy các lệnh sau:

```bash
# Cập nhật packages
sudo apt update && sudo apt upgrade -y

# Cài đặt Docker & Docker Compose plugin chính thức
sudo apt install -y ca-certificates curl gnupg lsb-release
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# Kiểm tra phiên bản
docker --version
docker compose version
```

### 2.2. Mở Port Tường Lửa (UFW Firewall)
```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

---

## 3. Tải Mã Nguồn & Cấu Hình Môi Trường

### 3.1. Clone Repository về VPS
```bash
cd /opt
git clone <URL_REPO_CUA_BAN> postpilot
cd postpilot
```

### 3.2. Cấu hình file `.env`
Tạo file `.env` từ `.env.example`:

```bash
cp .env.example .env
nano .env
```

**Các biến quan trọng BẮT BUỘC phải đổi cho Production:**

1. **TOKEN_ENCRYPTION_KEY** (Khóa mã hóa token Facebook dài 32 bytes hoặc 64 hex chars):
   ```bash
   # Tạo key ngẫu nhiên bằng OpenSSL:
   openssl rand -hex 32
   ```
   Dán giá trị vừa tạo vào biến `TOKEN_ENCRYPTION_KEY=...`

2. **DATABASE_URL & MinIO Password**:
   - Đổi `POSTGRES_PASSWORD` thành mật khẩu bảo mật cao.
   - Đổi `MINIO_ROOT_PASSWORD` thành mật khẩu bảo mật cao.

3. **Domain & S3 Public URL**:
   - `S3_PUBLIC_BASE_URL=https://postpilot.yourdomain.com/fb-media`
   - `FB_REDIRECT_URI=https://postpilot.yourdomain.com/api/v1/auth/facebook/callback`
   - `NEXT_PUBLIC_API_URL=https://postpilot.yourdomain.com`

4. **Meta Facebook App Credentials**:
   - `FB_APP_ID=...`
   - `FB_APP_SECRET=...`
   - `FB_WEBHOOK_VERIFY_TOKEN=...`

5. **Gemini API & Telegram (nếu có)**:
   - `GEMINI_API_KEY=...`
   - `TELEGRAM_BOT_TOKEN=...`
   - `TELEGRAM_CHAT_ID=...`

---

## 4. Cấu Hình Nginx & Cấp Chứng Chỉ SSL (Let's Encrypt)

### Bước 4.1: Cấu hình tên miền trong Nginx
Mở file `nginx/conf.d/postpilot.conf`:
```bash
nano nginx/conf.d/postpilot.conf
```
- Thay thế `_` hoặc tên miền mẫu bằng tên miền thực tế của bạn (ví dụ: `postpilot.yourdomain.com`).

### Bước 4.2: Khởi động hệ thống lần đầu (Chế độ HTTP)
```bash
# Khởi động toàn bộ container
docker compose -f docker-compose.prod.yml up -d
```
Kiểm tra tình trạng container:
```bash
docker compose -f docker-compose.prod.yml ps
```
Khi này, bạn đã có thể truy cập `http://postpilot.yourdomain.com` qua HTTP.

### Bước 4.3: Yêu cầu cấp chứng chỉ SSL miễn phí từ Let's Encrypt
Chạy lệnh Certbot qua Docker:
```bash
docker compose -f docker-compose.prod.yml run --rm --entrypoint "\
  certbot certonly --webroot -w /var/www/certbot \
  -d postpilot.yourdomain.com \
  --email your-email@example.com \
  --agree-tos \
  --no-eff-email" certbot
```
*(Thay `postpilot.yourdomain.com` và `your-email@example.com` bằng thông tin của bạn)*

Khi Certbot báo **"Successfully received certificate"**, chứng chỉ đã được lưu tại `./certbot/conf/live/postpilot.yourdomain.com/`.

### Bước 4.4: Bật cấu hình HTTPS
Mở lại file `nginx/conf.d/postpilot.conf`:
```bash
nano nginx/conf.d/postpilot.conf
```

1. Ở block `server { listen 80; ... }`:
   Bỏ dấu comment (`#`) ở đoạn redirect:
   ```nginx
   location / {
       return 301 https://$host$request_uri;
   }
   ```
2. Ở block `server { listen 443 ssl http2; ... }`:
   Bỏ comment toàn bộ block này và kiểm tra tên miền trong đường dẫn file chứng chỉ:
   ```nginx
   ssl_certificate /etc/letsencrypt/live/postpilot.yourdomain.com/fullchain.pem;
   ssl_certificate_key /etc/letsencrypt/live/postpilot.yourdomain.com/privkey.pem;
   ```

3. Reload Nginx để áp dụng ngay lập tức:
   ```bash
   docker compose -f docker-compose.prod.yml exec nginx nginx -s reload
   ```

Bây giờ bạn truy cập `https://postpilot.yourdomain.com` sẽ có ổ khóa SSL màu xanh an toàn!

---

## 5. Các Lệnh Vận Hành & Bảo Trì Hàng Ngày

- **Xem log hệ thống**:
  ```bash
  # Xem tất cả các container
  docker compose -f docker-compose.prod.yml logs -f --tail=100

  # Xem riêng worker xử lý bài đăng:
  docker compose -f docker-compose.prod.yml logs -f worker

  # Xem riêng API:
  docker compose -f docker-compose.prod.yml logs -f api
  ```

- **Cập nhật phiên bản mới (CI/CD hoặc Pull image mới)**:
  ```bash
  docker compose -f docker-compose.prod.yml pull
  docker compose -f docker-compose.prod.yml up -d --remove-orphans
  ```

- **Sao lưu cơ sở dữ liệu PostgreSQL (Backup)**:
  ```bash
  docker compose -f docker-compose.prod.yml exec -T postgres pg_dump -U fbposter fb_autoposter > backup_$(date +%F).sql
  ```

- **Khôi phục cơ sở dữ liệu (Restore)**:
  ```bash
  cat backup_xxxx.sql | docker compose -f docker-compose.prod.yml exec -T postgres psql -U fbposter -d fb_autoposter
  ```

- **Truy cập MinIO Console**:
  MinIO Console (Port 9001) được bảo vệ chỉ cho phép localhost để đảm bảo an toàn. Bạn có thể mở SSH Tunnel từ máy cá nhân:
  ```bash
  ssh -L 9001:localhost:9001 user@IP_VPS
  ```
  Sau đó mở trình duyệt máy bạn vào `http://localhost:9001` (đăng nhập bằng `MINIO_ROOT_USER` và `MINIO_ROOT_PASSWORD`).
