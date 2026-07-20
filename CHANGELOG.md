# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Removed
- **BREAKING**: 多区域路由层全删——`backend/internal/handler/resolve.go`、`backend/internal/handler/lobby_ws_proxy.go`、`backend/internal/game/hub_multiregion.go`、`backend/internal/domain/room_directory.go`（ADR-032 豁免裁剪；owner 反向代理 `RouteLocal`/`RouteProxy` 与 `/resolve` 端点、`room_directory` 表、`ClaimRoomOwnership` 租约接管一并移除）
- **BREAKING**: HMAC 审计链验证器删除——`backend/internal/audit/audit_chain_verifier.go`（ADR-032 豁免裁剪；`VerifyAuditChain` 从未被业务消费方读取，`audit.go` HMAC 链待 ADR-033 简化）
- **BREAKING**: Idempotency 中间件删除——`backend/internal/middleware/idempotency*.go`（ADR-032 豁免裁剪；balloon 游戏客户端无 POST 创建付费/订单类操作）
- **BREAKING**: Bulkhead 中间件删除——`backend/internal/middleware/bulkhead.go`（ADR-032 豁免裁剪；rate limiter + WSLimiter 已足够）
- **BREAKING**: outbox 独立 worker runner 删除——`backend/internal/worker/runner.go`（ADR-032 豁免裁剪；`EnableEmbeddedWorkers` 默认 true 即证据，独立 worker 进程从未被部署）
- OpenAPI 一致性测试删除（`handler/openapi_consistency_test.go` + `handler/openapi_admin_consistency_test.go`）—— ADR-032 第 6 项授权豁免，ADR-033 再次确认；Task C1 已执行，保留 `swagger-cli validate` CI job 守门
- **BREAKING**: outbox `publisher.go` 与 `InsertOutboxEvent` 调用链删除（ADR-033 决策 2；游戏结果改为直接写 PG，无下游消费者）
- **BREAKING**: `audit.go` HMAC 链简化为直接 INSERT（ADR-033 决策 1；删除 `computeHash`/`lastHash`/`prevHash`/`writeDeadLetter`/`loadLastHash`，失去密码学防篡改能力，改为依赖 PostgreSQL 只追加约束）
- **BREAKING**: `hub.go` `instanceAddress()` 与 Redis registry `Address` 字段删除（ADR-033 决策 4；多区域路由层连带死代码清理）
- **BREAKING**: `OutboundSource` 接口删除，消费方改为直接接收 `*Room`（ADR-033 决策 6；接口形同虚设，所有实现都返回 `*Room`）
- `test_seams.go` `newRedisStoreFn` 删除（ADR-033 决策 5；死代码，无调用方）
- `infra/terraform` 删除 `gke_regions`/`gke_cluster_name_prefix` 变量与多区域输出（G6；-82 行，ADR-014 已废弃）
- `deploy/` 删除 `sync-alert-rules.sh` 与 `render-overlays.sh` 多区域专用脚本（G7；-149 行，无人调用）
- `Makefile`/`go-ci.yml` 移除 `./internal/outbox/...` 引用，删除空目录 `backend/internal/outbox/`（G12）
- **BREAKING**: 保守路径瘦身 spec 执行——`server_debug.go`/`server_metrics.go` 合入 `server_lifecycle.go`、`server_deps.go` 合入 `server_init.go`（B 类）；移除 `RoomHandle`/`TokenSigner`/`userHardDeleter` 接口（C 类）；`internal/domain/` 碎片化整合（D-002）；`internal/store/base` 合入 `internal/store`（B-001）；测试表驱动合并与样板消除（E/F 类）；详见 `execute-conservative-slim-path` spec

### Added
- RBAC middleware coverage for user/lobby/registry routes (T18)
- Admin token revocation via jti + Redis blacklist (T19)
- `POST /api/v1/admin/logout` endpoint for admin session termination
- Admin password change auto-revokes current admin token
- `.secrets.baseline` for detect-secrets pre-commit hook (T5)
- `scripts/ci/check-docker-digests.sh` CI gate for digest pinning enforcement (T12)
- ADR index consistency CI check script (T14)

### Fixed
- Retry mechanism now properly wraps errors with `retry.RetryableError` — previously configured retries were no-ops (T1)
- EmailWorker HTTP client now uses configured timeouts instead of `http.DefaultClient` (T2)
- EndpointRateLimit applied to auth/registry/admin endpoints — previously used IP-only rate limiter (T3)
- Admin login lockout now uses real client IP from `X-Forwarded-For` instead of `r.RemoteAddr` (T4)
- CORS now allows PATCH method (T6)
- Audit log auto-extracts `request_id` and `trace_id` from context (T7)
- OpenTelemetry sampler configured with `ParentBased(TraceIDRatioBased)` — previously `AlwaysSample` (T9)
- `DBPoolAcquireDuration` metric now observed via delta sampling (T10)
- `LOG_FORMAT=text` env var now switches logger to text format for local dev (T11)
- Dropped 4 redundant database indexes (T13)
- Deploy job uses `environment: production` for approval gating (T21)

### Changed
- 全仓整理：清理 `run-backend.ps1` 内联 env 读取改为复用 `_bootstrap-env.ps1`；删除 `scripts/archive/`、一次性 `cmd/`、构建产物、重复脚本；统一 ADR 中文模板与状态审计
- ADR 索引覆盖 000–027；移除 phantom `ADR-V2-*` 引用
- `REDIS_URL` 支持 URL 与 host:port；`make load-*` 与 CI/golangci 版本对齐
- ADR README index rewritten to include ADR-000 through ADR-027
- `pin-digests.sh` updated with exact image tags matching Dockerfile (T12)

### Security
- Non-root container execution
- .dockerignore prevents sensitive file leakage
- govulncheck + Trivy in CI pipeline
- Encrypted storage for admin API keys

## [1.0.0] - 2026-06-24

### Added
- Enterprise-grade observability: structured JSON logs, Prometheus metrics, OpenTelemetry tracing
- Circuit breaker protection for PostgreSQL, Redis, and Resend API
- Retry with exponential backoff + jitter for transient failures
- Layered timeout configuration (PG/Redis/HTTP/WS independently configurable)
- JWT refresh token mechanism with Redis-backed revocation
- AES-256-GCM encryption for sensitive config fields (API keys)
- RFC 7807 unified error responses (application/problem+json)
- API versioning (/api/v1/) with backward-compatible 301 redirects
- Health probes (/health/live, /health/ready) for K8s/Cloud Run
- Go backend CI pipeline (test, lint, vet, govulncheck, container scan)
- golangci-lint configuration (10 linters)
- Database index optimization (user_id, updated_at, status)
- Connection pool tuning (MaxConns=25, MinConns=5)
- Architecture documentation with ADR records
- STRIDE threat model
- On-call runbook
- OpenAPI/Swagger specification
- Transactional Outbox pattern for reliable event publishing (ADR-009)
- Tamper-proof audit log with HMAC chain (ADR-008)
- Async email sending via Redis Stream + Worker (ADR-010)
- Redis Stream message queue for game results (ADR-007)
- PR template with shift-left checklist (T17)
- release-please workflow for automated changelog (T20)
- Branch protection rules via `.github/settings.yml` (T21)

### Changed
- Dockerfile: Go version 1.22 → 1.26 (match go.mod)
- Container runs as non-root user (appuser)
- Access token TTL reduced from 7 days to 15 minutes
- All API routes moved to /api/v1/ prefix
