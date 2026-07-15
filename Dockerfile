# Stage 1: Build frontend
# SLSA L2: all base images pinned by digest (see scripts/ci/check-docker-digests.sh).
# infra-035: 修正注释引用——原指向不存在的 scripts/ci/pin-digests.sh，实际脚本为
# scripts/ci/check-docker-digests.sh（验证 Dockerfile 所有 FROM 行均带 @sha256: 摘要）。
FROM node:24.1.0-alpine3.20@sha256:8fe019e0d57dbdce5f5c27c0b63d2775cf34b00e3755a7dea969802d7e0c2b25 AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go backend
FROM golang:1.26-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS go-builder
ENV GOTOOLCHAIN=auto
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Stage 3: Runtime — distroless nonroot (K8s restricted profile)
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
WORKDIR /app
COPY --from=go-builder --chown=nonroot:nonroot /server ./server
COPY --from=frontend-builder --chown=nonroot:nonroot /app/frontend/dist ./dist
COPY --chown=nonroot:nonroot backend/migrations ./migrations
USER nonroot:nonroot
ENV FRONTEND_DIR=/app/dist
EXPOSE 8080
CMD ["./server"]
