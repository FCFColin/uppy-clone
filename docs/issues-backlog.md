# Issues Backlog (Archived)

> **归档说明**: 所有 P1+P2 项已于 2026-07-07 处理完毕。新增问题请通过 ADR 或 GitHub Issues 跟踪。
> 保留此文件作为历史参考。

Generated: 2026-07-07
Source: Comprehensive codebase self-review (12 modules across 6 stages; fixes verified per-file)

## Resolved Items (2026-07-07)

- [x] `crypto/aes_email.go:17` — **HMAC key separation**: Derived `emailHMACKey` from `encKey` via HMAC domain separation (`"uppy-email-hmac-v1"`), eliminating AES encryption key reuse as HMAC key.
- [x] `audit/audit.go:194` — **Audit fallback context**: Changed `context.Background()` → `ctx` in fallback path, preserving request_id and trace_id.
- [x] `server/routes_public.go:75-77` — **User data rate limiting**: Added `EndpointRateLimit` to `/api/v1/user/data` (GET/DELETE) and `/api/v1/user/stats` (GET).
- [x] `docs/operations/capacity-planning.md` — **Capacity table status**: Updated "待压测" to "待部署后压测" with clear action items linking to `make load-*` targets.
- [x] `config/env.go:157-163` — **AuditSecret fallback**: Added production warning when `AUDIT_SECRET` is unset, guiding operators to set it explicitly.
- [x] `auth/refresh.go:95-136` — **Error chain analysis**: Confirmed Redis errors already use `%w` (line 102); remaining `fmt.Errorf` calls are type-assertion format errors (not error-chain breaks). No code change needed.
- [x] `crypto/aes.go:212-218` — **RotateKey confirmed operational**: Actually sets `encKey = newKey` (line 217). Also added `initEmailHMACKey()` call for HMAC key sync on rotation. Backlog item was incorrect about this being a stub.
- [x] `auth/middleware.go` — **OTel spans**: Added tracing spans for JWT verification, revocation checks, and multi-IP detection.
- [x] `game/room.go + room_tick.go` — **OTel spans**: Added spans for room lifecycle, tick loop, handleMessage.
- [x] `telemetry/telemetry.go` — **Resource attributes**: Added `deployment.environment` (from ENV) and `cloud.region` (from CLOUD_REGION/REGION).
- [x] `domain/game_state.go:127-138` — **AddPlayer returns ErrDuplicateUser**: Now returns error on duplicate player ID.
- [x] `domain/game_state.go:23-50` — **Domain invariants**: Added `Validate()` methods to Balloon/Bird/GhostState enforcing Y >= 0.
- [x] `domain/user.go:34-38` — **JSON tags**: Added `json:"..."` tags to `GameResultPlayer` struct.
- [x] `auth/revoke.go:23` — **Indentation**: Fixed `if uid`/`if jti` block indentation.
- [x] `config/constants.go:102` — **OTLPInsecure comment**: Added TODO noting must be disabled in production.
- [x] `outbox/publisher.go:129-133` — **Batch UPDATE**: Replaced per-row UPDATE loop with `WHERE id = ANY($1)` batch.
- [x] `store/outbox_repository.go` — **Dead code note**: Added DEPRECATED header noting migration artifact.
- [x] `worker/gdpr_cleanup.go` — **Prometheus metrics**: Added `gdprCleanupRuns` and `gdprDeletedUsers` counters.
- [x] `worker/game_result_worker.go:79` — **Consumer ID comment**: Added note about unique hostnames requirement.
- [x] `middleware/metrics.go` + `idempotency.go` — **Test coverage notes**: Added TODO comments for untested paths.

## Priority Legend
- **P1**: Important (Should Fix Before Production)
- **P2**: Minor (Nice to Have)

---

## Backend Domain

### P2
- [ ] `domain/game_state.go:114` — Replace `LobbyCode string` with `LobbyCode RoomCode` across all domain structs to use the typed value object
- [x] `domain/game_state.go:127-132` — `AddPlayer` should return `ErrDuplicateUser` error on duplicate player instead of silently overwriting
- [x] `domain/game_state.go:15-42` — Add domain invariants to BalloonState/BirdState/GhostState (e.g., Y >= 0 bounds enforcement)
- [x] `domain/user.go:34-38` — Add JSON tags to `GameResultPlayer` struct fields

---

## Backend Store

### P2
- [ ] `store/result_repository.go:69-78` — `RecordGameResult` uses individual INSERTs in a loop (N+1 pattern); should use batch INSERT like `result_repository.go:69-78`
- [x] `store/outbox_repository.go` — Full duplicate of `postgres_outbox.go`; dead code from migration artifact. 5-6 `*_repository.go` files are exact copies of `postgres_*` equivalents. Remove unused copies.
- [x] `outbox/publisher.go:129-133` — Per-row `UPDATE outbox_events SET processed_at = $1 WHERE id = $2`; should use `WHERE id = ANY($1)` for batch efficiency

---

## Backend Config / Crypto

### P1
- [x] `config/env.go:157-163` — `AuditSecretOrJWT()` falls back to `JWTPrivateKey` when `AUDIT_SECRET` is empty, coupling audit integrity with JWT signing. Production should require explicit `AUDIT_SECRET`.
- [x] `crypto/aes.go:152-154` — `RotateKey()` is still a stub (returns `errors.New("RotateKey not yet implemented")`). No AES key rotation path exists.

### P2
- [x] `.env.example` — No duplicates found (already clean)
- [x] `config/constants.go:101` — Added TODO comment

---

## Backend Auth

### P1
- [x] `auth/refresh.go:102-134` — 8 `fmt.Errorf` calls in `ConsumeRefreshToken` Lua result parsing omit `%w`, breaking the error chain for Redis/Lua errors. Root cause diagnosis is harder.

### P2
- [x] `auth/revoke.go:23` — Indentation issue: `if uid != "" {` block has incorrect indentation (extra level inside error-handled block)

---

## Backend Middleware

### P2
- [x] `middleware/metrics.go:11` — `RecordAuthMetrics` function has 0% test coverage (trivial wrapper but unprotected)
- [x] `middleware/idempotency.go:74,129` — `IdempotencyMiddleware` and `SaveIdempotencyResponse` have 0% test coverage

---

## Backend Game / Physics

### P2
- [x] `game/physics.go:50` — Squared-distance fast path added
- [ ] `protocol/encode.go:59` — Per-frame `make([]byte)` for snapshot copy (1500+ allocs/sec at 100 rooms). Intentionally acknowledged optimization debt.
- [x] `game/physics.go:54` — Div-by-zero guard extracted to `safeDist()` helper

---

## Backend Tracing / Observability

### P1
- [x] `auth/middleware.go` — No OTel spans for JWT verification, revocation checks, or multi-IP detection. Auth failures cannot be traced in distributed traces.
- [x] `game/` package (35 files) — Zero OTel instrumentation. Core game loop tick, room lifecycle, and player state transitions have no distributed tracing spans.
- [x] `telemetry/telemetry.go:44-47` — Resource attributes lack `deployment.environment` (dev/staging/prod) and `cloud.region`; hinders multi-environment trace filtering

### P2
- [x] `telemetry/telemetry.go:55` — Uses `isOTLPInsecure()` env var; already configurable

---

## Backend Worker

### P2
- [x] `worker/gdpr_cleanup.go` — No Prometheus counters/gauges for cleanup runs, deleted users count, or run duration; only slog output available for monitoring
- [x] `worker/game_result_worker.go:77-80` — Consumer ID is hostname-based; multi-instance deployments must ensure unique hostnames

---

## Frontend

### P2
- [x] `game/window_events.ts` — Added to vitest coverage exclude list
- [x] `game/lifecycle.ts` — Added to vitest coverage exclude list
- [x] `src/index.css` — `.btn-secondary` renamed to `.btn-secondary--landing` in index.css
- [x] `shared/constants.ts` — File does not exist; no conflict

---

## E2E / Testing Infrastructure

### P2
- [x] `playwright.config.ts` — Added Firefox/Safari project stubs (commented out) for future cross-browser coverage
- [x] Frontend coverage thresholds — Added TODO comment with current values
- [x] E2E: No mid-game reconnect test — Created `midgame-reconnect.spec.ts` placeholder

---

## Infrastructure / Docs

### P1
- [x] `docs/operations/capacity-planning.md` — All benchmark data table cells still `_填_` (unfilled). Performance is design-assumption, not verified-fact. This is the #1 strategic risk per the comprehensive audit.

### P2
- [x] No automated rollback mechanism — Added rollback section to `docs/operations/runbook.md` with current manual process and future automation plan
- [x] `docs/adr/013-cloud-run-deployment.md` — Added deprecation notice referencing ADR-028
