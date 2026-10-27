#!/bin/bash
set -e

# ==============================================================================
# Script tự động cấp chứng chỉ SSL Let's Encrypt cho PostPilot
# Sử dụng: ./setup-ssl.sh your-email@example.com
# ==============================================================================

EMAIL="$1"
DOMAINS=("postpilot.hexta.org" "api-postpilot.hexta.org")
RSA_KEY_SIZE=4096
DATA_PATH="./certbot"

if [ -z "$EMAIL" ]; then
    echo "❌ Lỗi: Vui lòng cung cấp email đăng ký Let's Encrypt."
    echo "👉 Cách dùng: $0 your-email@example.com"
    exit 1
fi

echo "🚀 Bắt đầu thiết lập SSL cho các tên miền: ${DOMAINS[*]}"

# Tạo thư mục certbot nếu chưa có
mkdir -p "$DATA_PATH/conf/live"
mkdir -p "$DATA_PATH/www"

# 1. Tạo chứng chỉ giả (dummy certificates) nếu chưa có để Nginx có thể khởi động
for domain in "${DOMAINS[@]}"; do
    CERT_DIR="$DATA_PATH/conf/live/$domain"
    if [ ! -f "$CERT_DIR/fullchain.pem" ]; then
        echo "🔐 Khởi tạo dummy certificate tạm thời cho $domain..."
        mkdir -p "$CERT_DIR"
        openssl req -x509 -nodes -newkey rsa:2048 -days 1 \
            -keyout "$CERT_DIR/privkey.pem" \
            -out "$CERT_DIR/fullchain.pem" \
            -subj "/CN=localhost" 2>/dev/null
    fi
done

# 2. Khởi động Nginx để mở cổng 80 cho ACME challenge
echo "🌐 Khởi động Nginx..."
docker compose up -d nginx

# 3. Yêu cầu cấp chứng chỉ thực từ Let's Encrypt cho từng domain
for domain in "${DOMAINS[@]}"; do
    echo "📜 Đang xin cấp SSL chính thức cho $domain..."
    docker compose run --rm certbot certonly \
        --webroot \
        --webroot-path=/var/www/certbot \
        --email "$EMAIL" \
        -d "$domain" \
        --rsa-key-size "$RSA_KEY_SIZE" \
        --agree-tos \
        --force-renewal \
        --non-interactive
done

# 4. Reload Nginx để áp dụng chứng chỉ thật
echo "🔄 Reload Nginx với chứng chỉ SSL mới..."
docker compose exec nginx nginx -s reload

echo "✅ Hoàn tất cấp SSL thành công cho:"
for domain in "${DOMAINS[@]}"; do
    echo "   🔗 https://$domain"
done
