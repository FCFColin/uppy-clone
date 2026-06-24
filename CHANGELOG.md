# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- RBAC middleware coverage for user/lobby/registry routes (T18)
- Admin token revocation via jti + Redis blacklist (T19)
- `POST /api/v1/admin/logout` endpoint for admin session termination
- Admin password change auto-revokes current admin token
- `.secrets.baseline` for detect-secrets pre-commit hook (T5)
- `scripts/check-docker-digests.sh` CI gate for digest pinning enforcement (T12)
- ADR index consistency CI check script (T14)

### Fixed
- Retry mechanism now properly wraps errors with `retry.RetryableError` — previously configured retries were no-ops (T1)
- EmailWorker HTTP client now uses configured timeouts instead of `http.DefaultClient` (T2)
- EndpointRateLimit applied to auth/registry/admin endpoints — previously used IP-only rate limiter (T3)
- Admin login lockout now uses real client IP from `X-Forwarded-For` instead of `r.RemoteAddr` (T4)
- CORS now allows PATCH method and `Idempotency-Key` header (T6)
- Audit log auto-extracts `request_id` and `trace_id` from context (T7)
- OpenTelemetry sampler configured with `ParentBased(TraceIDRatioBased)` — previously `AlwaysSample` (T9)
- `DBPoolAcquireDuration` metric now observed via delta sampling (T10)
- `LOG_FORMAT=text` env var now switches logger to text format for local dev (T11)
- Dropped 4 redundant database indexes (T13)
- Deploy job uses `environment: production` for approval gating (T21)

### Changed
- ADR-005 status updated from "提议中" to "已接受（部分实施）" (T15)
- `architecture.md` tick rate corrected from 60fps to 15Hz (T15)
- `architecture.md` "无消息队列" updated to reflect Redis Stream implementation (T15)
- ADR README index rewritten to include ADR-001 through ADR-010 (T14)
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
