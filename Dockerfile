# Stage 1: Build frontend
# Enterprise rationale: Pin base image digests for build reproducibility.
# Tags are mutable — the same tag can point to different images over time.
# SLSA Level 2 requires reproducible builds. Trade-off: Must update digest
# manually when upgrading base image versions.
# Run scripts/pin-digests.sh to resolve current digests, then replace tags below.
#
# 企业为何需要：Tag 可被覆盖推送恶意镜像（供应链攻击）。Digest pinning 确保构建使用不可变的特定镜像版本。
# Pin digest: docker pull --quiet node:20.18.0-alpine3.20 && docker image inspect --format='{{index .RepoDigests 0}}' node:20.18.0-alpine3.20
# TODO: replace with actual digest from `docker pull <image> && docker inspect --format='{{index .RepoDigests 0}}' <image>`
# Production: pin with @sha256:<digest> for immutable, reproducible builds.
FROM node:20.18.0-alpine3.20 AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go backend
# ADR: Pin to 1.26-alpine to match go.mod directive (go 1.26.4).
# Enterprise rationale: Build reproducibility — version mismatch between
# Dockerfile and go.mod causes build failures in CI/CD pipelines.
# Pin digest for SLSA Level 2 reproducibility (see scripts/pin-digests.sh).
#
# 企业为何需要：Tag 可被覆盖推送恶意镜像（供应链攻击）。Digest pinning 确保构建使用不可变的特定镜像版本。
# Pin digest: docker pull --quiet golang:1.26.0-alpine3.20 && docker image inspect --format='{{index .RepoDigests 0}}' golang:1.26.0-alpine3.20
# TODO: replace with actual digest from `docker pull <image> && docker inspect --format='{{index .RepoDigests 0}}' <image>`
# Production: pin with @sha256:<digest> for immutable, reproducible builds.
FROM golang:1.26.0-alpine3.20 AS go-builder
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Stage 3: Runtime
# ADR: Use distroless nonroot image per least-privilege principle.
# Enterprise rationale: Distroless images ship no shell, no package manager
# and no utilities — shrinking the attack surface vs. alpine. The :nonroot
# variant runs as a built-in non-root UID (65532), satisfying K8s Pod
# Security Standards "restricted" profile. CA certificates are included in
# the image, so no apk install is needed. CGO_ENABLED=0 in Stage 2 produces
# a fully static binary compatible with distroless/static.
# Trade-off: cannot bind ports < 1024 (not needed, we use 8080); no shell
# inside the container complicates ad-hoc debugging (use `docker exec` with
# a debug image or `crictl` instead).
# Pin digest for SLSA Level 2 reproducibility (see scripts/pin-digests.sh).
#
# 企业为何需要：Tag 可被覆盖推送恶意镜像（供应链攻击）。Digest pinning 确保构建使用不可变的特定镜像版本。
# Pin digest: docker pull --quiet gcr.io/distroless/static-debian12:nonroot && docker image inspect --format='{{index .RepoDigests 0}}' gcr.io/distroless/static-debian12:nonroot
# TODO: replace with actual digest from `docker pull <image> && docker inspect --format='{{index .RepoDigests 0}}' <image>`
# Production: pin with @sha256:<digest> for immutable, reproducible builds.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=go-builder --chown=nonroot:nonroot /server ./server
COPY --from=frontend-builder --chown=nonroot:nonroot /app/frontend/dist ./dist
COPY --chown=nonroot:nonroot backend/migrations ./migrations
USER nonroot:nonroot
ENV FRONTEND_DIR=/app/dist
EXPOSE 8080
CMD ["./server"]
