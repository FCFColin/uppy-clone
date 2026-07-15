# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 1.0.0 (2026-07-15)


### Features

* feat:  ([54e0e36](https://github.com/FCFColin/uppy-clone/commit/54e0e36661de93931f95a876f2c6d1cd872fc30c))
* add CI/CD pipeline with quality gates, build, and security scans ([86ef398](https://github.com/FCFColin/uppy-clone/commit/86ef398d682c2f95ff536deab46460a60a5ac635))
* add degradation auto-detection via circuit breaker state ([b67a262](https://github.com/FCFColin/uppy-clone/commit/b67a2627818616c6013045b92a3b835056006490))
* add frame budget monitoring warning when fps drops below 30 ([1ea9ee0](https://github.com/FCFColin/uppy-clone/commit/1ea9ee0046b14d99469f29fb2e061cbdfd823b22))
* add game-specific Prometheus metrics (rooms, players, tick duration) ([67b84cb](https://github.com/FCFColin/uppy-clone/commit/67b84cba28b1e41eb1f1c171cd18390a0a702602))
* add Prometheus alert rules for error rate, latency, pool, disk, memory ([b9b2ccf](https://github.com/FCFColin/uppy-clone/commit/b9b2ccf463fd11da3c58955b505e48c431c905f1))
* add role attribute to request-scoped slog context ([e4ded47](https://github.com/FCFColin/uppy-clone/commit/e4ded471e4348780212603551220b74be5f5f6c2))
* **frontend:** CSP安全头、admin模块重构、WebSocket连接优化 ([8d12e03](https://github.com/FCFColin/uppy-clone/commit/8d12e036c6d64cdf7eae986e983ef5a991e98866))
* GDPR audit log, outbox event, IP hashing, and domain error types ([3d03ff9](https://github.com/FCFColin/uppy-clone/commit/3d03ff9e1b5f7a9baf5c1b86581a5ff37ed801ea))
* horizontal scaling via Redis Pub/Sub + CI/CD fixes + git setup ([a1519ac](https://github.com/FCFColin/uppy-clone/commit/a1519ac6163d6e2291bd233efcc306551e5a3b02))
* migrate JWT from HS256 to ES256 (ECDSA P-256), inject deterministic RNG into game ticks, add WaitGroup for worker shutdown ([68cefd2](https://github.com/FCFColin/uppy-clone/commit/68cefd2ce93225600b16cfcc36c9239010c82ff8))


### Bug Fixes

* fix:  ([f3f74d7](https://github.com/FCFColin/uppy-clone/commit/f3f74d7ff2d222b825d9904e504cc2aa143f3a5e))
* fix:  ([501f189](https://github.com/FCFColin/uppy-clone/commit/501f189d55175a9cc4678ddf33c0d991ffb1e4da))
* fix:  ([78b9636](https://github.com/FCFColin/uppy-clone/commit/78b9636e78a2f7ea8055635cf82b9fd2e89d0907))
* fix:  ([fc79b94](https://github.com/FCFColin/uppy-clone/commit/fc79b9407aafaece023956cb45f6d5a13ea65746))
* add adversarial auth integration tests and rename WS rate limit test ([4d8a991](https://github.com/FCFColin/uppy-clone/commit/4d8a9911d406bb8e4d8db0f391efdca549e006e0))
* add dead-letter queue to GameResultWorker after 5 retries ([d8db51c](https://github.com/FCFColin/uppy-clone/commit/d8db51c0ddead50b846025797f4e94fa94d2b4c8))
* add HTTP panic recovery middleware to prevent server crashes ([1e94e11](https://github.com/FCFColin/uppy-clone/commit/1e94e11703d7197c7c6d1dbba288f16363b5f513))
* add HTTP panic recovery middleware to prevent server crashes ([109368d](https://github.com/FCFColin/uppy-clone/commit/109368d3937e734f0cd2e7d989c7e2875da104a2))
* add K8s egress network policy (deny-all with allowlist for DNS, DB, Redis) ([b84bc69](https://github.com/FCFColin/uppy-clone/commit/b84bc69dece1338317d2ce93d9629c0bcdcb7dba))
* add retry with exponential backoff to GDPRCleanupWorker ([4f0b6a7](https://github.com/FCFColin/uppy-clone/commit/4f0b6a7eb9d9f76e26029bd36fd81e191d3e9b10))
* add security context to K8s StatefulSet (non-root, read-only rootfs) ([62ee387](https://github.com/FCFColin/uppy-clone/commit/62ee387c248f0ce60e7f3f5aaff8ae8d83e522b5))
* cap ripple (50) and explosion (10) counts to prevent unbounded growth ([c6cf5e5](https://github.com/FCFColin/uppy-clone/commit/c6cf5e5a70d75a6147ee2438601f32d2f9f3fc4e))
* define missing UnmarshalRoomRegistryInfo function ([5a78ad1](https://github.com/FCFColin/uppy-clone/commit/5a78ad1a19b34f153dd30af6d3513353b4d29e95))
* enable Redis auth and transit encryption in Terraform ([a6db250](https://github.com/FCFColin/uppy-clone/commit/a6db2502ce54a087637819963997efeb877efe51))
* enable TLS for Redis connections when rediss:// scheme is used ([109368d](https://github.com/FCFColin/uppy-clone/commit/109368d3937e734f0cd2e7d989c7e2875da104a2))
* **frontend:** harden cookie security with Secure flag, add room code validation ([372920e](https://github.com/FCFColin/uppy-clone/commit/372920e84816e0f22650f3484b0135abce92c4a0))
* handle decodeSnapshot known limitation in property tests (oversized nickname buffers) ([805ecdb](https://github.com/FCFColin/uppy-clone/commit/805ecdbe5f325addc4491852bf14cda7054b7b8d))
* redact email PII in worker logs ([6ea4837](https://github.com/FCFColin/uppy-clone/commit/6ea4837a40df1abce7522b0a82cdd0419e812528))
* replace hardcoded docker-compose passwords with env var references ([b2d4996](https://github.com/FCFColin/uppy-clone/commit/b2d49963257267cd169236831d2ec47ea7c80c2f))
* replace Object.assign with spread syntax in reducer ([6dfeb51](https://github.com/FCFColin/uppy-clone/commit/6dfeb51dbf8d32aa2e26c55b223e8cadfa5df8af))
* restrict Cloud SQL to private network, disable public IP, enforce SSL ([367d885](https://github.com/FCFColin/uppy-clone/commit/367d8851332588a3c082758665336a4744781f0b))
* revert unintended behavioral changes in audio.ts, message_codec.ts, session.ts, snapshot_decode.test.ts ([8c3be71](https://github.com/FCFColin/uppy-clone/commit/8c3be713697827153fbd1ce65ff5b66720e95779))
* server test compilation and store pgxmock expectation order ([68f7c9d](https://github.com/FCFColin/uppy-clone/commit/68f7c9d60c055d942820f05261d648726cbee0fa))
* tighten gitignore coverage patterns to avoid ignoring recovery.go ([74acd12](https://github.com/FCFColin/uppy-clone/commit/74acd123c97df031b58fbb66daed68cbd297fcca))
* use PEM-encoded ECDSA key in auth/handler/middleware tests (ES256 migration) ([011bebe](https://github.com/FCFColin/uppy-clone/commit/011bebed2a993566bd4d6d42f7affef1a7de756a))


### Performance Improvements

* cache time.Now() once per game tick to reduce syscalls ([96dd47f](https://github.com/FCFColin/uppy-clone/commit/96dd47f65b80f842c1c63f6e1bed3ea87edf878c))

## [Unreleased]

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
- CORS now allows PATCH method and `Idempotency-Key` header (T6)
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
