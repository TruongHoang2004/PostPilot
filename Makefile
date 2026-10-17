.PHONY: build test migrate-up migrate-down migrate-status run-api run-worker docker-up docker-down

# Build tất cả các binary Golang vào thư mục ./bin
build:
	@mkdir -p bin
	go build -o bin/fb-api ./cmd/api
	go build -o bin/fb-worker ./cmd/worker
	go build -o bin/fb-migrate ./cmd/migrate

# Chạy toàn bộ Unit Test & End-to-End Test
test:
	go test -v ./...

# Chạy Database Migration UP bằng Golang
migrate-up:
	go run ./cmd/migrate up

# Rollback 1 bản Database Migration gần nhất
migrate-down:
	go run ./cmd/migrate down

# Xem trạng thái các bản Database Migration
migrate-status:
	go run ./cmd/migrate status

# Chạy API Server & Web Dashboard ở local (Port 8080)
run-api:
	go run ./cmd/api

# Chạy Background Worker & Scheduler ở local
run-worker:
	go run ./cmd/worker

# Khởi động toàn bộ hạ tầng Docker (Postgres, Redis, MinIO, Migrator, API, Worker)
docker-up:
	docker compose up -d --build

# Tắt hạ tầng Docker
docker-down:
	docker compose down
