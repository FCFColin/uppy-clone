# Codebase Remediation Design

> Design document for fixing findings from the 2026-07-03 self-inspection.

**Goal:** Fix all 4 CRITICAL and 17 HIGH findings, plus 25 MEDIUM/LOW findings that are cost-effective to address.

**Approach:** Organize into 4 phases by cost-effectiveness and dependency order. Each phase can be validated independently.

---

## Phase 1: Quick Wins (≤30 min each, 12 items)

Highest security/maintainability value per unit time. No architectural decisions required.

| # | Finding | Severity | What to Fix | Effort | Files |
|---|---------|----------|-------------|--------|-------|
| 1 | Task 5: bcrypt cost 10 vs ≥12 | MEDIUM | Change `bcrypt.DefaultCost` to `12` (OWASP 2026 recommendation) | 5 min | `handler/admin_password.go:29`, `cmd/migrate-passwords/main.go:105` |
| 2 | Task 3: Magic link TTL 15min | MEDIUM | Change `magicLinkTTL = 15 * time.Minute` to `10 * time.Minute` | 5 min | `config/constants.go:13` |
| 3 | Task 4: QuickPlay nickname unvalidated | MEDIUM | Add `len(nickname) >= 2 && len(nickname) <= 20 && !containsUnsafeChars(nickname)` | 15 min | `handler/auth_util.go:68-76` |
| 4 | Task 16: Silent recover() without logs | MEDIUM | Replace `recover()` with `recover()` + slog.Warn + metric counter | 10 min | `game/outbound_manager.go:99,145` |
| 5 | Task 15: WithInsecure hardcoded | MEDIUM | Add `OTLP_INSECURE` env var, default `true` for dev, `false` for production | 15 min | `telemetry/telemetry.go:55`, `config/constants.go` |
| 6 | Task 20: docker-compose hardcoded passwords | MEDIUM | Use `${POSTGRES_PASSWORD:-uppy}` pattern referencing `.env` file | 20 min | `docker-compose.yml:46,96,7,18,19,62,66` |
| 7 | Task 26: Import grouping in store/postgres.go | LOW | Add blank line between stdlib/external/internal import groups | 5 min | `store/postgres.go:4-19` |
| 8 | Task 22: Unused `data "google_project"` | INFO | Remove unused data source declaration | 5 min | `infra/terraform/main.tf:130` |
| 9 | Task 16: `fmt.Errorf` without `%w` in auth/refresh.go | MEDIUM | Add `%w` to 8+ error strings to preserve error chain | 10 min | `auth/refresh.go` |
| 10 | Task 12: Object.assign mutation in reducer | MEDIUM | Replace `Object.assign(state, ...)` with `{ ...state, ...partial }` | 10 min | `game/reducer.ts` |
| 11 | Task 13: Role not in slog context | LOW | Add `slog.String("role", role)` to logger enrichment in auth middleware | 15 min | `auth/middleware.go`, `slogctx/` |
| 12 | Task 13: Email PII in worker logs | MEDIUM | Redact email field: `slog.String("email", email)` → `slog.String("email_prefix", email[:3]+"***")` | 10 min | `worker/email_worker.go` |

**Total Phase 1 effort: ~2 hours**

---

## Phase 2: Infrastructure Security (Critical/High blockers)

The most critical items that block production deployment.

| # | Finding | Severity | What to Fix | Effort | Files |
|---|---------|----------|-------------|--------|-------|
| 13 | Task 19: No CI/CD pipeline | CRITICAL | Create GitHub Actions workflows for lint, test, security scan, build, deploy (3 workflow files) | 4 hr | `.github/workflows/` (3 new files) |
| 14 | Task 22: Cloud SQL publicly accessible | HIGH | Add `ip_configuration { ipv4_enabled = false; private_network = ... }` | 30 min | `infra/terraform/main.tf:21-32` |
| 15 | Task 25: Build-blocking compilation error `UnmarshalRoomRegistryInfo` | HIGH | Define the missing function or remove the dead reference | 30 min | `store/redis_registry.go` or wherever it's referenced |
| 16 | Task 18: No WaitGroup for worker shutdown | HIGH | Add `sync.WaitGroup` to track workers, `wg.Wait()` before db/redis close | 30 min | `server/server_lifecycle.go`, `worker/*.go` |
| 17 | Task 16: No HTTP panic recovery middleware | HIGH | Add `chi.Recoverer` or custom `recoverer` to middleware stack | 15 min | `server/routes_middleware.go`, `middleware/recovery.go` |
| 18 | Task 5: Redis TLS not configured | MEDIUM | Add `TLSConfig` to `redis.Options` when scheme is `rediss://` | 30 min | `store/redis.go:29-39`, `store/redis_addr.go` |
| 19 | Task 21: K8s missing security context | MEDIUM | Add `securityContext` to StatefulSet pod spec | 15 min | `infra/k8s/base/statefulset.yaml` |

**Total Phase 2 effort: ~6.5 hours**

---

## Phase 3: Resilience & Observability (High/Medium)

Strengthening error handling, worker resilience, and monitoring coverage.

| # | Finding | Severity | What to Fix | Effort | Files |
|---|---------|----------|-------------|--------|-------|
| 20 | Task 18: GameResultWorker no dead letter | HIGH | Add max-retry threshold and dead-letter stream after N failures | 1 hr | `worker/game_result_worker.go` |
| 21 | Task 18: GDPRCleanupWorker no retry | MEDIUM | Add retry with backoff for transient DB failures | 30 min | `worker/gdpr_cleanup.go` |
| 22 | Task 14: Missing game-specific metrics | MEDIUM | Add Prometheus gauges for active rooms, active players, tick duration, physics duration | 1 hr | `metrics/`, `game/room_tick.go`, `game/physics.go` |
| 23 | Task 14: No alert rules | MEDIUM | Create Alertmanager rules for error rate, latency p99, connection pool, disk, memory | 1 hr | `deploy/prometheus/alerts.yml` |
| 24 | Task 18: No degradation auto-detection | MEDIUM | Add `IsDegraded()` check with circuit breaker integration, expose via `/health/degraded` | 1.5 hr | `handler/degradation.go`, `server/routes_public.go` |
| 25 | Task 8: E2E auth flow (magic link) gap | HIGH | Add Playwright spec for magic link → verify → login → session | 2 hr | `tests/e2e/auth.spec.ts` |
| 26 | Task 8: E2E admin flow gap | HIGH | Add Playwright spec for admin login → config change → verify | 2 hr | `tests/e2e/admin.spec.ts` |
| 27 | Task 12: Frontend frame budget monitoring | MEDIUM | Log warning when rAF delta exceeds budget (33ms for 30fps target) | 30 min | `game/renderer.ts` |
| 28 | Task 12: Add max ripple/explosion count limit | MEDIUM | Cap ripples at 50, cap explosions at 10, cull oldest on overflow | 15 min | `game/renderer_draw_effects.ts` |
| 29 | Task 11: Redundant `time.Now()` per game tick | LOW | Cache the time value once at tick start, reuse across subsystems | 20 min | `game/room_tick.go` |

**Total Phase 3 effort: ~10 hours**

---

## Phase 4: Security Hardening & Compliance (Medium)

Defense-in-depth improvements and GDPR gap closure.

| # | Finding | Severity | What to Fix | Effort | Files |
|---|---------|----------|-------------|--------|-------|
| 30 | Task 3: HS256 → RS256 | MEDIUM | Generate key pair, update JWT sign/verify to use ECDSA, update config | 2 hr | `auth/jwt.go`, `auth/jwt_verify.go`, `config/` |
| 31 | Task 27: Missing audit log on GDPR deletion | FAIL | Add `audit.Log()` call with `Action: "gdpr_hard_delete"` in deletion path | 30 min | `handler/auth_gdpr_data.go:48` |
| 32 | Task 27: No outbox event for GDPR deletion | FAIL | Publish `UserHardDeleted` event via outbox after deletion | 30 min | `handler/auth_gdpr_data.go`, `domain/events.go` |
| 33 | Task 27: Plaintext IP in audit_logs | FAIL | Hash or encrypt `actor_ip` before writing to audit_logs | 1 hr | `audit/`, `handler/` |
| 34 | Task 16: Domain error types lacking | MEDIUM | Add `ErrNotFound`, `ErrValidation`, `ErrConflict`, `ErrUnauthorized` with `errors.Is` support | 45 min | `domain/errors.go`, update handlers |
| 35 | Task 21: K8s egress network policy | HIGH | Add egress rules: allow DNS (udp 53), DB (tcp 5432), Redis (tcp 6379), deny-all default | 30 min | `infra/k8s/global/network-policies.yaml` |
| 36 | Task 22: Redis auth_enabled + transit_encryption | MEDIUM | Add `auth_enabled = true` and `transit_encryption_enabled = true` in Terraform | 15 min | `infra/terraform/main.tf:46-52` |
| 37 | Task 3: RBAC add role to slog context | LOW | Add `slog.String("role", role)` in `rbac.Middleware` | 10 min | `rbac/middleware.go` |

**Total Phase 4 effort: ~5.5 hours**

---

## Summary

| Phase | Items | Effort | Type |
|-------|-------|--------|------|
| Phase 1: Quick Wins | 12 | ~2 hr | Security hygiene, code quality |
| Phase 2: Infrastructure Security | 7 | ~6.5 hr | Production-blockers |
| Phase 3: Resilience & Observability | 10 | ~10 hr | Error handling, monitoring, testing |
| Phase 4: Security Hardening | 8 | ~5.5 hr | Defense-in-depth, compliance |
| **Total** | **37** | **~24 hr** | |

**Not fixing (intentionally skipped):**
- Fixed window rate limit (known trade-off, sliding window adds complexity for marginal gain)
- OpenAPI/AsyncAPI full rewrite (docs are partially correct; full sync is maintenance work, not a defect)
- ADR template compliance across all 29 docs (cosmetic, high effort)
- Unused frontend exports (3 items, low impact)
- E2E multi-region test (no multi-region test infrastructure)
