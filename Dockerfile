# Stage 1: Build frontend
# SLSA L2: all base images pinned by digest (see scripts/ci/pin-digests.sh).
FROM node:20.18.0-alpine3.20@sha256:b1e0880c3af955867bc2f1944b49d20187beb7afa3f30173e15a97149ab7f5f1 AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go backend
FROM golang:1.26-alpine AS go-builder
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
