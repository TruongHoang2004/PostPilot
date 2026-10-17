# ============================================================================
# Stage 1: Build Golang Binaries (API, Worker, Migrator)
# ============================================================================
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Copy go.mod và source code
COPY go.mod ./
COPY migrations ./migrations
COPY internal ./internal
COPY cmd ./cmd

# Build 3 static binaries tối ưu dung lượng
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/fb-api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/fb-worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/fb-migrate ./cmd/migrate

# ============================================================================
# Stage 2: Minimal Production Runtime Image
# ============================================================================
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

COPY --from=builder /out/fb-api /app/fb-api
COPY --from=builder /out/fb-worker /app/fb-worker
COPY --from=builder /out/fb-migrate /app/fb-migrate

EXPOSE 8080

CMD ["/app/fb-api"]
