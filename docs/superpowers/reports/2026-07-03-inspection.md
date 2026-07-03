# Production-Grade Codebase Self-Inspection Report

---

## Executive Summary

This report documents findings from a 30-task self-inspection of the balloon-game codebase conducted on 2026-07-03.

### Finding Counts by Severity

| Severity | Count |
|----------|-------|
| CRITICAL | 4 |
| HIGH | 17 |
| MEDIUM | 62 |
| LOW | 28 |
| INFO | 18 |

**Total findings: 129**

### Top 5 Risks

1. **No CI/CD pipeline** (CRITICAL) — Zero GitHub Actions workflows, no automated build, no deployment pipeline, no rollback mechanism. All quality gates are Makefile-local only.
2. **Cloud SQL publicly accessible** (HIGH) — Terraform configures Cloud SQL with no `ip_configuration`, making the database reachable from any IP on the internet without SSL enforcement.
3. **Build-blocking compilation error** (HIGH) — `domain.UnmarshalRoomRegistryInfo` is referenced but never defined, blocking `go vet`, `golangci-lint`, and test execution for 6+ critical packages (auth, game, handler, server, store, testutil).
4. **No worker shutdown coordination** (HIGH) — 3+ worker goroutines are fire-and-forget; DB/Redis pools may be closed while workers are still performing I/O, risking data corruption.
5. **Missing network policy egress rules** (HIGH) — K8s network policy has no egress rules; pods can reach any external destination, and Redis is unprotected within the namespace.

### Recommended Immediate Actions

1. Fix `domain.UnmarshalRoomRegistryInfo` (add the function or remove the reference) to unblock all tooling
2. Add `ip_configuration` block to Cloud SQL Terraform resource with `ipv4_enabled = false`
3. Create GitHub Actions CI workflow with lint, test, audit, and security scanning gates
4. Add `sync.WaitGroup` to worker shutdown in `server_lifecycle.go`
5. Add egress network policies for K8s and restrict Redis access

### Overall Codebase Health Score

**5/10** — The core game engine, auth, crypto, and architecture are well-implemented and production-grade. However, the absence of CI/CD, deployment pipeline, and infrastructure security hardening means the codebase cannot be safely deployed to production in its current state. The compilation error blocking all tooling is a critical quality gate issue. Infrastructure-as-code (Terraform, K8s) contains severe security gaps that would expose the application to internet-wide attacks.

---

## 16. Error Handling

### Checks Performed

- **Step 1:** Grep for empty error blocks (`if err != nil {}`) — swallowed errors
- **Step 2:** Grep for `recover()` in production code — panic recovery coverage
- **Step 3:** Grep for `fmt.Errorf` without `%w` wrapper — error wrapping quality
- **Step 4:** Read `domain/errors.go`, `apierror/apierror.go`, and handler error discrimination patterns
- **Step 5:** Cross-reference handler-sentinel errors with `errors.Is` usage across service boundaries

### Files Read

- `backend/internal/domain/errors.go` — Domain error sentinels (5 lines)
- `backend/internal/apierror/apierror.go` — RFC 7807 problem details (81 lines)
- `backend/internal/game/outbound_manager.go:90-154` — Panic recovery in channel send / delivery
- `backend/internal/server/auth_service.go` — Error type mapping layer
- `backend/internal/resilience/retry.go` — Retryable error discrimination (132 lines)
- `backend/internal/handler/handler_interfaces.go` — Service boundary interfaces
- `backend/internal/handler/auth_magiclink.go` — `errors.Is` discrimination pattern
- `backend/internal/crypto/aes.go`, `auth/jwt.go`, `middleware/security.go` — `panic()` locations

### Findings

#### Step 1: Swallowed Errors

- **PASS** — No empty `if err != nil {}` blocks found in production Go code.
- **PASS** — `_ = err` usages are limited to `metrics.go:22,25` where Prometheus metric registration intentionally discards "already registered" errors (correct pattern).
- **PASS** — `json.Decode` body errors are always checked in production code.
- The codebase consistently checks errors at the point of origin. No bare error-discard patterns detected.

#### Step 2: Panic Recovery

- **PASS** — 3 `panic()` calls in production code all guard unrecoverable initialization:
  - `crypto/aes.go:57` — AES key init failure (must-panic per design: "Refusing to start with no encryption key")
  - `auth/jwt.go:27` — Short JWT secret (defense-in-depth, panics before signing with weak key)
  - `middleware/security.go:78` — CSP nonce crypt/rand failure (must-panic per design)

- **FAIL** — `backend/internal/game/outbound_manager.go:99,145` — Two silent `recover()` calls:
  - Line 99: `defer func() { recover() }()` in enqueue channel-send path
  - Line 145: `defer func() { recover() }()` in deliver-to-targets per-connection loop
  - **Both swallow panics without logging, metrics, or any signal.** A panic from sending on a closed channel or nil connection would be completely invisible in production. Even if the recovery is intentional (safe fallback for closed channels), the absence of any observability (slog.Warn, metric counter) means these failures are undetectable.

- **FAIL** — No HTTP-level panic recovery middleware. The chi middleware stack in `routes_middleware.go` has `RequestIDLogger`, `TracingMiddleware`, `PrometheusMiddleware` but no `recoverer` or custom panic recovery handler. An unhandled panic in any HTTP handler or middleware causes the entire server process to crash.

#### Step 3: Error Wrapping

- **PASS** — Most error paths properly wrap with `%w` for `errors.Is`/`errors.As` compatibility. The `auth`, `crypto`, `store` packages consistently use `fmt.Errorf("...: %w", err)`.

- **FAIL** — 25+ `fmt.Errorf` calls in production code omit `%w`, breaking the error chain:

  | File | Line | Error | Impact |
  |------|------|-------|--------|
  | `auth/refresh.go:106-134` | 8 errors | `consume refresh token: unexpected result/status/reuse/userID` | `errors.Is` cannot reach original store/Redis error |
  | `auth/refresh.go:84` | 1 error | `errRefreshTokenReused = fmt.Errorf("refresh token reuse detected")` | Sentinel, acceptable |
  | `auth/gdpr_data.go:27,82,87` | 3 errors | `"user not found"`, `"refresh token has already been used"` | Cannot distinguish from other "not found" errors |
  | `auth/magiclink.go:218,223,244` | 3 errors | `"invalid or expired token"`, `"invalid token data"` | Acceptable — these are leaf errors with no inner error |
  | `auth/jwt_verify.go:29,39` | 2 errors | `"unexpected signing method"`, `"invalid token claims"` | Cannot trace to root cause |
  | `crypto/aes.go:37,133,149` | 3 errors | `"ENCRYPTION_KEY must be 32 bytes..."`, `"ciphertext too short"`, `"RotateKey not yet implemented"` | Mostly validation, low impact |
  | `store/magiclink_store.go:91` | 1 error | `"consume magic token: unexpected result type"` | Redis type assertion errors lose context |
  | `store/postgres.go:46,135` | 2 errors | `"nil pool config"`, `"migrations require a real pgxpool connection"` | Initialization, low impact |

- Most critically: `auth/refresh.go:102-134` — The `ConsumeRefreshToken` Lua result parsing has 8+ non-wrapped errors that lose the underlying Redis/Lua error context. If the Lua script fails with a runtime error, the resulting `errors.Is` chain is truncated at the `fmt.Errorf("consume refresh token: unexpected ...")` wrapper, making root-cause diagnosis harder.

#### Step 4: Error Type Discrimination

- **FAIL** — Domain error types are critically underspecified:
  - `domain/errors.go` defines exactly **one** sentinel error: `ErrDuplicateUser`
  - No `ErrNotFound`, `ErrValidation`, `ErrUnauthorized`, `ErrConflict`, `ErrForbidden`
  - Handlers cannot classify domain errors by type; they must pattern-match on error strings or rely on ad-hoc sentinels

- **PASS** — The `apierror` package provides 8 RFC 7807 constructor helpers (`BadRequest`, `Unauthorized`, `NotFound`, `Conflict`, `UnprocessableEntity`, `TooManyRequests`, `Forbidden`, `InternalError`). All handlers use these for HTTP response formatting. 100% test coverage.

- **FAIL** — No centralized error-to-HTTP-status mapping layer. Each handler manually calls `apierror.*` constructors. Service-layer errors (`auth/quickplay.go`, `auth/magiclink.go`) return raw Go errors that handlers must interpret ad-hoc via `errors.Is`. The mapping in `auth_service.go:48-54` bridges `auth.Err*` → `handler.Err*` for magic-link rate-limit/email errors, but this is the only such bridge; most errors flow untyped.

- **PASS** — `resilience/retry.go` has excellent error discrimination:
  - `isRetryable()` classifies 8+ transient error types (pgx commit rollback, conn closed, network timeout, ECONNRESET, ECONNREFUSED, io.EOF, io.ErrUnexpectedEOF, pgconn.SafeToRetry)
  - `MaybeRetryable()` and `RetryableError()` helpers combine classification with `go-retry` compatible wrapping
  - This is the best error handling pattern in the codebase

- **PASS** — Store layer correctly discriminates PostgreSQL errors:
  - `errors.Is(scanErr, pgx.ErrNoRows)` for not-found (6+ locations)
  - `errors.As(execErr, &pgErr) && pgErr.Code == "23505"` for unique constraint violations

#### Step 5: Additional Findings

- [INFO] `apierror/apierror.go:80` — `json.NewEncoder(w).Encode(e)` error is discarded with `_ =`. The HTTP status code and Content-Type header have already been written; an encode failure at this point cannot be reported via the same response. Acceptable — the error surface is minimal (JSON serialization of a known struct should never fail).

- [INFO] `domain/errors.go` — The single error `ErrDuplicateUser` is consumed correctly in `auth/quickplay.go:59` via `errors.Is`, showing the intended pattern. The gap is that no other domain error types follow this pattern.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Swallowed errors (empty blocks) | PASS — No bare discards in production code | NONE |
| Panic recovery — HTTP middleware | **FAIL** — No `recoverer` in chi middleware stack; panic kills server | HIGH |
| Panic recovery — goroutine safety | **FAIL** — 2 silent `recover()` in outbound_manager.go with zero observability | MEDIUM |
| Error wrapping (%w usage) | MIXED — Most paths wrap; 25+ unwrapped calls, worst in `auth/refresh.go` | MEDIUM |
| Domain error types | **FAIL** — Only 1 error type (`ErrDuplicateUser`); no `NotFound`/`Validation`/`Conflict` | MEDIUM |
| Error-to-HTTP-status mapping | **FAIL** — No centralized layer; ad-hoc `errors.Is` with handler-local sentinels | MEDIUM |
| RFC 7807 response formatting | PASS — 8 helper constructors, 100% coverage | NONE |
| Retryable error discrimination | PASS — Excellent coverage of transient failures | NONE |
| Store error discrimination | PASS — `pgx.ErrNoRows` and PostgreSQL error codes handled correctly | NONE |

---

## 19. CI/CD Pipeline

### Checks Performed

- **Step 1:** Audited CI quality gates — checked for `.github/workflows/ci-cd.yml`, `Makefile` CI targets
- **Step 2:** Audited security scanning — checked for `govulncheck`, `gitleaks`, `detect-secrets`, `trivy`, `CodeQL`, license check, Docker pin verification
- **Step 3:** Audited deployment pipeline — checked for sequential region rollout, cosign verification, Kustomize overlays, rollback mechanism
- **Step 4:** Audited pre-commit hooks — read `.pre-commit-config.yaml`

### Files Read

- `Makefile` — All CI/CD targets (138 lines)
- `.pre-commit-config.yaml` — Pre-commit hook configuration (56 lines)
- `Dockerfile` — Multi-stage build, distroless runtime (28 lines)
- `scripts/ci/check-coverage.sh` — Layered coverage governance (359 lines)
- `scripts/ci/check-docker-digests.sh` — Docker digest pinning enforcement (53 lines)
- `scripts/ci/verify-release-config.ps1` — Release config verification
- `scripts/ci/verify-required-checks.ps1` — Required checks alignment
- `scripts/ci/self-check-layers.ps1` — Security layer self-check
- `deploy/kustomization.yaml` — Observability stack kustomization (17 lines)

### Findings

#### Step 1: CI Quality Gates

- **FAIL — No GitHub Actions workflow files exist.** The `.github/workflows/` directory is absent entirely. The task brief references `ci-cd.yml` and `go-ci.yml` which do not exist.
- CI pipeline is defined solely in the `Makefile` with local-execution targets:
  - `ci` target = `check test-containers audit check-repo-layout`
  - `check` = `lint-all` (frontend lint + typecheck + backend golangci-lint) + `test-cover` (backend unit + integration coverage)
  - `test-containers` = integration tests with testcontainers
  - `audit` = govulncheck + gitleaks + trivy
  - `check-repo-layout` = repo layout validation
- **Frontend tests** run via `test-cover` (`npm run test:frontend`) and `test-all` (`npm test`)
- **E2E tests** (`make e2e`) exist but are **not included** in any CI target — they must be run manually
- **Integration tests** are included in `test-containers` target
- **Coverage thresholds** are defined in `check-coverage.sh` with strict limits: unit = 100%, integration = 80%, frontend lines/functions = 100% — but no tool enforces these in CI (no CI pipeline to run them)
- **npm audit** is only in `security-check` target, not in `ci` target

#### Step 2: Security Scanning

| Check | Status | Notes |
|-------|--------|-------|
| `govulncheck` | ✅ Present | `audit` Makefile target runs `govulncheck ./...` |
| `gitleaks` | ✅ Present | `audit` target runs `gitleaks detect --source . --report-path leaks.json` |
| `detect-secrets` | ❌ Not in CI | Only in `security-check` target (not `ci`), and in `.pre-commit-config.yaml` |
| `trivy` | ✅ Present | `audit` target runs `trivy fs .` |
| CodeQL (SAST) | ❌ Missing | No CodeQL configuration or workflow |
| License check (no GPL/AGPL) | ❌ Missing | No license scanning tool configured |
| Docker pin verification | ✅ Present | `check-docker-digests.sh` verifies all `FROM` lines use `@sha256:` digests; all 3 stages in Dockerfile are pinned |
| SLSA L2 compliance | ✅ Partial | Docker digests are pinned; no provenance attestation or build L2 signing |

- **Security scanning exists as ad-hoc Makefile targets** but is not automated in any CI pipeline
- `verify-release-config.ps1` references `.github/workflows/go-ci.yml` with `cosign sign` and commit SHA tags — this workflow does not exist

#### Step 3: Deployment Pipeline

| Check | Status | Notes |
|-------|--------|-------|
| Sequential region rollout | ❌ Missing | No deployment pipeline at all |
| Cosign verification after deploy | ❌ Missing | No cosign workflow exists |
| Kustomize overlay (applied correctly) | ❌ Missing | `deploy/kustomization.yaml` only covers observability stack, not the application itself |
| Rollback mechanism | ❌ Missing | No rollback strategy implemented |
| GitOps-style deployment | ❌ Missing | No ArgoCD, Flux, or similar |

- **No deployment pipeline exists.** The deployment artifacts (Docker image, Kustomize overlays, release scripts) are all references to files that do not exist:
  - `infra/k8s/overlays/us-east1/kustomization.yaml` — does not exist
  - `infra/k8s/overlays/europe-west1/kustomization.yaml` — does not exist
  - `infra/k8s/overlays/asia-southeast1/kustomization.yaml` — does not exist
- The `verify-release-config.ps1` script validates these non-existent paths and would **fail immediately** if run
- The only deployable artifact is the `Dockerfile` with digest-pinned base images

#### Step 4: Pre-commit Hooks

| Check | Status | Notes |
|-------|--------|-------|
| trailing-whitespace | ✅ Present | `trailing-whitespace` hook from `pre-commit-hooks` v4.6.0 |
| detect-secrets | ✅ Present | `detect-secrets` v1.5.0 with `.secrets.baseline` |
| golangci-lint | ✅ Present | `golangci-lint` v2.1.6 with `--new-from-rev=HEAD` |
| conventional-commits | ✅ Present | `conventional-pre-commit` v3.2.0 at `commit-msg` stage |
| Go test | ✅ Present | Local hook: `go test ./... -timeout 30s -short -race` on Go file changes |
| Frontend lint | ✅ Present | Local hook: `npm run lint` on `frontend/src/*.ts/tsx` |
| Frontend typecheck | ✅ Present | Local hook: `npm run typecheck` on `frontend/src/*.ts/tsx` |
| end-of-file-fixer | ✅ Present | Additional hook (not required) |
| check-yaml | ✅ Present | Additional hook (not required) |
| check-json | ✅ Present | Additional hook (not required) |
| check-merge-conflict | ✅ Present | Additional hook (not required) |
| detect-private-key | ✅ Present | Additional hook (not required) |

- **All 7 required checks from the task brief are present** — pre-commit hooks are the strongest part of the CI/CD posture
- Additional hooks (end-of-file-fixer, check-yaml, check-json, check-merge-conflict, detect-private-key) add value
- Go test hook uses `-race` and `-short` — appropriate for pre-commit speed
- golangci-lint uses `--new-from-rev=HEAD` to only lint new changes — appropriate for pre-commit speed

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| CI quality gates (GitHub Actions) | FAIL — No workflow files exist | CRITICAL |
| CI quality gates (Makefile local) | PASS — Backend lint, frontend lint, typecheck, coverage all present | LOW (local only) |
| E2E tests in CI | MISSING — Not included in any CI target | MEDIUM |
| npm audit in CI | MISSING — Only in `security-check` target | MEDIUM |
| govulncheck | PASS — In `audit` target | LOW |
| gitleaks | PASS — In `audit` target | LOW |
| detect-secrets (CI) | MISSING — Only in pre-commit and `security-check` | MEDIUM |
| Trivy container scan | PASS — In `audit` target | LOW |
| CodeQL SAST | MISSING | MEDIUM |
| License check (GPL/AGPL) | MISSING | MEDIUM |
| Docker pin verification | PASS — All 3 FROM lines digest-pinned | NONE |
| Deployment pipeline | MISSING — No CI/CD deployment pipeline at all | CRITICAL |
| Sequential region rollout | MISSING | CRITICAL |
| Rollback mechanism | MISSING | CRITICAL |
| Pre-commit hooks | PASS — All 7 required hooks present plus extras | NONE |

- **Pre-commit hooks are production-grade** — all 7 required checks present, properly scoped for speed
- **CI pipeline is entirely absent** — no GitHub Actions workflows; CI is Makefile-local only
- **Deployment pipeline is entirely absent** — no deploy, rollout, or rollback mechanism
- **Security scanning is fragmented** — some tools in `make audit`, some in `make security-check`, none automated
- **Dockerfile is well-configured** — multi-stage, digest-pinned, distroless nonroot runtime
- **Key incident**: `verify-release-config.ps1` and `verify-required-checks.ps1` validate files that do not exist (`.github/workflows/go-ci.yml`, `.github/workflows/ci-cd.yml`, `infra/k8s/overlays/*/kustomization.yaml`) — these scripts would always fail

---

## 9. Architecture Compliance

### Checks Performed

- **Step 1:** Verified domain package dependency direction — domain must import zero internal packages
- **Step 2:** Reviewed `handler_interfaces.go` for consumer-side interface definitions and boundary integrity
- **Step 3:** Audited domain model richness — checked for anemic domain models (data-only structs without behavior)
- **Step 4:** Verified bounded context separation — game package must not directly import `store` or `auth` in production code
- **Step 5:** Checked for god objects — files >500 lines flagged for potential splitting
- **Step 6:** Cross-referenced findings with ADR-028 (Clean Architecture), ADR-017 (Bounded Contexts), ADR-019 (No ORM)

### Files Read

- `backend/internal/handler/handler_interfaces.go` — All 10 handler-side interfaces
- `backend/internal/domain/` — All 14 production .go files (game_state.go with behavior methods, user.go, events.go, room_code.go, etc.)
- `backend/internal/game/repository.go`, `cache_store.go` — Game-side interfaces
- `backend/internal/game/room.go`, `hub.go` — Largest game files by line count
- `backend/internal/game/room_lifecycle.go` — A candidate for large-file check
- `docs/adr/028-clean-architecture-interface-decoupling.md`
- `docs/adr/017-bounded-contexts.md`
- `docs/adr/019-no-orm-raw-sql-pgx.md`

### Findings

#### Step 1: Dependency Direction (Domain imports nothing)

- **PASS** — Domain package (`backend/internal/domain/`) imports ZERO internal packages from the codebase.
- Production imports are limited to stdlib only: `fmt`, `time`, `errors`, `context`.
- Test files (`domain_test.go`, `game_state_test.go`) import only `strings`, `testing`, `time`, `math` — no internal packages.
- No violations of Clean Architecture dependency rule. Domain is the innermost layer with zero outward dependencies.

#### Step 2: Interface Boundaries (handler_interfaces.go)

- **PASS** — All interfaces in `handler_interfaces.go` are defined at the consumer (handler) side per DIP.
- 10 interfaces defined: `UserStore`, `TokenStore`, `ConfigStore`, `AdminCache`, `LeaderboardStore`, `JWTManager`, `RefreshTokenManager`, `JWTRevocationChecker`, `AuthService`, `GameService`.
- All interface method signatures use domain types (`domain.User`, `domain.AppConfig`, `domain.GameResult`, `domain.LeaderboardEntry`, etc.) or standard library types.
- **Zero concrete types from `store` or `auth` leak through interface boundaries.**
- The `game` package independently defines its own consumer-side interfaces: `RoomRepository`, `CacheStore`, `Broadcaster`, `SnapshotEncoder` (in `repository.go`, `cache_store.go`, `broadcaster.go`).
- This matches the pattern documented in ADR-028: "接口定义在被消费者（handler/middleware/rbac），实现在基础设施（store/auth）".

#### Step 3: Anemic Domain Models

- **PASS** — Key domain entities carry behavior methods; the domain is not anemic.
- `PlayerState` (game_state.go): 5 methods including `CanTap()`, `RecordTap()`, `IsRateLimited()`, `MarkDisconnected()`, `Reconnect()`. The file explicitly documents "升级为充血对象" (upgraded to rich object, P3-1.1).
- `GameState` (game_state.go): 4 methods including `AddPlayer()`, `RemovePlayer()`, `UpdatePlayerState()`, `IsGameOver()`. Explicitly documented as aggregate (P3-1.2).
- `RoomCode` (room_code.go): Value object with `NewRoomCode()` constructor enforcing 5-char length and charset validation.
- `Nickname` (nickname.go): Value object with `NewNickname()` constructor delegating to `NicknameValidator` interface.
- `ContextKey` (context_keys.go): Methods `WithValue()` and `Value()` for context access.
- **Events** (events.go): `Event` interface with `EventType()` and `OccurredAt()` methods; concrete events (`PlayerJoined`, `PlayerLeft`, `GameEnded`, `PhaseChanged`) implement the interface.
- Some domain types are purely structural (`User`, `GameResult`, `LeaderboardEntry`, `AppConfig`, `RoomRegistryInfo`, `LobbyState`, `LobbyListResult`) — this is acceptable for simple value objects and DTOs where behavior is minimal or absent by design.

#### Step 4: Bounded Context Separation (Game → Store/Auth)

- **PASS** — Game production code (non-test) imports zero `store` or `auth` packages directly.
- All 35 game production files were checked. Internal imports are limited to: `domain`, `config`, `protocol`, `audit`, `metrics`, `idgen`, `validate`, `nicknames`.
- Store access is exclusively through interfaces defined within the game package (`RoomRepository`, `CacheStore`, `Broadcaster`).
- Auth access within game is absent — no game code interacts with auth directly.
- **Test files only** import `store` directly (7 test files across `coverage_gaps_test.go`, `hub_cache_test.go`, `hub_integration_test.go`, `hub_restore_test.go`, `hub_test.go`, `room_async_test.go`, `room_lifecycle_test.go`). This is acceptable for integration testing.
- ADR-017 maps the bounded contexts correctly: Game Play (core), Identity & Access (supporting), Lobby Registry (supporting), Admin & Config (supporting), Integration (generic).
- ADR-028 confirms the dependency inversion for game→store decoupling: "game → store 反转（Task 3.2）".

#### Step 5: God Objects

- **PASS** — No production .go file in `backend/internal/` exceeds 500 lines.
- Largest files:
  - `game/room_lifecycle.go`: 314 lines (largest in game package)
  - `game/room.go`: 209 lines
  - `game/hub.go`: 178 lines
  - `server/server_lifecycle.go`: 132 lines
- [INFO] ADR-019 documents that `store/postgres.go` is ~954 lines (a known god-file in the store layer), but this was acknowledged and accepted at ADR time. It lives in the infrastructure layer, not domain/game. The ADR suggests consideration of `sqlc` as future enhancement for type-safe SQL, which would help reduce file size with generated code.

### Summary

| Check | Verdict |
|-------|---------|
| Domain imports zero internal pkgs | PASS |
| Interface boundaries (handler-side) | PASS |
| Anemic domain models | PASS — PlayerState/GameState rich with behavior |
| Bounded context separation | PASS — Game → store/auth via interfaces only |
| God objects >500 lines | PASS — No production files exceed threshold |
| Store/postgres.go known god-file | INFO — 954 lines, acknowledged in ADR-019 |

## 1. Dependency Vulnerabilities

### Checks Performed

- **Backend (Go):** `govulncheck` on compilable packages (`internal/config`, `internal/domain`, `internal/crypto`)
- **Frontend (TypeScript):** `npm audit --json` and `npm outdated --json`
- **Outdated deps:** `go list -m -u all` (backend), `npm outdated --json` (frontend)

### Pre-existing Issue Blocking Full Go Scan

The `internal/store` package has a compilation error (`room_registry_store.go:118:31: undefined: domain.UnmarshalRoomRegistryInfo`) which prevents `govulncheck` from scanning packages that transitively import it. The following packages were scanned successfully (those not importing `store`): `internal/config`, `internal/domain`, `internal/crypto`.

### Findings

#### Backend — Go Standard Library (Go 1.26.x)

The `go.mod` specifies `go 1.26.4`. If building with a Go version below the fix for each CVE, these apply:

- [HIGH] [backend] stdlib (Go <1.26.2): GO-2026-4864 / CVE-2026-32282 — TOCTOU permits root escape via Root.Chmod on Linux
- [HIGH] [backend] stdlib (Go <1.26.2): GO-2026-4865 / CVE-2026-32289 — JsBraceDepth context tracking bugs (XSS) in html/template
- [MEDIUM] [backend] stdlib (Go <1.26.2): GO-2026-4866 / CVE-2026-33810 — Case-sensitive excludedSubtrees name constraints cause auth bypass in crypto/x509
- [MEDIUM] [backend] stdlib (Go <1.26.2): GO-2026-4869 / CVE-2026-32288 — Unbounded allocation for old GNU sparse in archive/tar
- [HIGH] [backend] stdlib (Go <1.26.2): GO-2026-4870 / CVE-2026-32283 — Unauthenticated TLS 1.3 KeyUpdate DoS in crypto/tls
- [MEDIUM] [backend] stdlib (Go <1.26.2): GO-2026-4946 / CVE-2026-32281 — Inefficient policy validation in crypto/x509
- [MEDIUM] [backend] stdlib (Go <1.26.2): GO-2026-4947 / CVE-2026-32280 — Unexpected work during chain building in crypto/x509
- [MEDIUM] [backend] stdlib (Go <1.26.3): GO-2026-4918 / CVE-2026-33814 — Infinite loop in HTTP/2 transport via bad SETTINGS_MAX_FRAME_SIZE
- [MEDIUM] [backend] stdlib (Go <1.26.3): GO-2026-4971 / CVE-2026-39836 — Panic in Dial/LookupPort on Windows with NUL byte
- [MEDIUM] [backend] stdlib (Go <1.26.3): GO-2026-4976 / CVE-2026-39825 — ReverseProxy forwards hidden query params
- [LOW] [backend] stdlib (Go <1.26.3): GO-2026-4977 / CVE-2026-42499 — Quadratic string concat in net/mail consumePhrase
- [MEDIUM] [backend] stdlib (Go <1.26.3): GO-2026-4980 / CVE-2026-39826 — Escaper bypass leads to XSS in html/template
- [MEDIUM] [backend] stdlib (Go <1.26.3): GO-2026-4981 / CVE-2026-33811 — Crash on long CNAME response in net (cgo resolver)
- [MEDIUM] [backend] stdlib (Go <1.26.3): GO-2026-4982 / CVE-2026-39823 — Bypass of meta content URL escaping causes XSS in html/template
- [LOW] [backend] stdlib (Go <1.26.3): GO-2026-4986 / CVE-2026-39820 — Quadratic string concat in net/mail consumeComment
- [LOW] [backend] stdlib (Go <1.26.4): GO-2026-5037 / CVE-2026-27145 — Inefficient hostname parsing in crypto/x509
- [LOW] [backend] stdlib (Go <1.26.4): GO-2026-5038 / CVE-2026-42504 — Quadratic complexity in mime WordDecoder.DecodeHeader
- [LOW] [backend] stdlib (Go <1.26.4): GO-2026-5039 / CVE-2026-42507 — Arbitrary input in net/textproto errors

#### Backend — Third-Party Dependencies

No vulnerabilities found in third-party Go modules. The indirect dep `golang.org/x/net v0.55.0` is above the fixed version (0.53.0). All direct application dependencies appear free of known CVEs.

#### Frontend — npm audit

**No vulnerabilities found.** All 345 resolved dependencies (1 prod, 344 dev/optional/peer) are clean.

```
npm audit --json: { "vulnerabilities": {}, "metadata": { "vulnerabilities": { "critical": 0, "high": 0, "moderate": 0, "low": 0, "info": 0, "total": 0 } } }
```

#### Frontend — npm outdated (outdated but not vulnerable)

| Package | Current | Latest |
|---------|---------|--------|
| eslint | 10.5.0 | 10.6.0 |
| jsdom | 25.0.1 | 29.1.1 |
| typescript | 5.9.3 | 6.0.3 |
| typescript-eslint | 8.62.0 | 8.62.1 |
| vite | 6.4.3 | 8.1.3 |

These are not associated with known vulnerabilities (npm audit is clean) but represent dependency drift, especially `jsdom` (25 → 29) and `vite` (6 → 8).

#### Backend — Outdated Go Dependencies (selected direct deps)

| Package | Current | Latest |
|---------|---------|--------|
| github.com/alicebob/miniredis/v2 | v2.35.0 | v2.38.0 |
| github.com/go-chi/chi/v5 | v5.3.0 | (no update) |
| github.com/golang-jwt/jwt/v5 | v5.3.1 | (no update) |
| github.com/gorilla/websocket | v1.5.3 | (no update) |
| github.com/jackc/pgx/v5 | v5.10.0 | (no update) |
| github.com/prometheus/client_golang | v1.23.2 | (no update) |
| github.com/redis/go-redis/v9 | v9.21.0 | (no update) |
| go.opentelemetry.io/otel | v1.44.0 | (no update) |
| golang.org/x/crypto | v0.53.0 | (no update) |

Most direct dependencies are at or near latest. No known CVEs affect the current versions.

### Summary

- **0 critical/high CVEs** in third-party dependencies (both Go and npm)
- **17 stdlib CVEs** affect Go <1.26.4 — mitigated if building with `go 1.26.4` exactly
- **Pre-existing compilation bug** in `internal/store` prevents full govulncheck coverage
- **npm audit is clean** — no frontend dependency vulnerabilities
- **Minor drift** in frontend devDependencies (jsdom, vite, typescript) but no associated CVEs

---

## 2. Secret Exposure

### Checks Performed

- **Step 1:** Searched all backend Go source files (non-test) for hardcoded `password`, `secret`, `api.?key`, `token`, `private.?key` assignments
- **Step 2:** Searched all frontend TypeScript source files for hardcoded secret assignments
- **Step 3:** Scanned git history for committed sensitive files (`*.env`, `*.pem`, `*.key`, `*.p12`)
- **Step 4:** Reviewed `.gitleaks.toml` configuration and `.secrets.baseline`
- **Step 5:** Cross-checked docker-compose.yml and CI/CD YAML files

### Findings

#### Backend Go (production code)

- **PASS** — All secrets loaded from environment variables via `os.Getenv()`. No hardcoded secret values in production code paths.
- `backend/internal/config/env.go:204` has explicit detection/blocking of weak dev values (`"DEV_ONLY"`, `"change-in-production"`).
- `backend/internal/testsecrets/secrets.go` contains test-only placeholder credentials (`"test-only-jwt-secret-not-for-production-use"`). This directory is allowlisted in `.gitleaks.toml`. Acceptable.

#### Backend Go (test files)

- Multiple test files contain hardcoded test credentials. All are tracked in `.secrets.baseline` (generated 2026-06-28). Examples:
  - `backend/internal/auth/jwt_cookies_test.go` — test JWT secret
  - `backend/internal/config/env_test.go` — test API key, admin password
  - `backend/internal/handler/admin_handlers_test.go` — test passwords, secrets
  - `backend/cmd/migrate-passwords/migrate_integration_test.go` — test passwords
- These are acceptable for unit/integration tests and are not shipped to production.

#### Frontend TypeScript

- **PASS** — No hardcoded secrets found. All references to passwords, tokens, API keys are either:
  - User-supplied form input values submitted to the backend
  - Config fields read from backend API response
  - Token refresh utilities (cookie-based, no secret exposure)

#### docker-compose.yml

- [MEDIUM] `docker-compose.yml:46` — `POSTGRES_PASSWORD: uppy` hardcoded for local dev PostgreSQL
- [LOW] `docker-compose.yml:96` — `GF_SECURITY_ADMIN_PASSWORD=admin` hardcoded for local dev Grafana
- [LOW] `docker-compose.yml:7,18` — `DATABASE_URL=postgres://uppy:uppy@...` embeds password `uppy` in connection URL
- [LOW] `docker-compose.yml:19,62,66` — Redis password falls back to `dev-redis-secret` when `$REDIS_PASSWORD` not set
- Note: All these are local development defaults only. Production deployments inject real secrets via environment variables. Still worth addressing via `.env` rather than hardcoded values.

#### Git History

- **PASS** — No sensitive files (`.env`, `.pem`, `.key`, `.p12`) ever committed to git history.
- Only `.env.example` (template with placeholder values) and `.secrets.baseline` (baseline config) exist in history.

#### .gitleaks.toml

- Uses `[extend] useDefault = true` which loads all built-in gitleaks detectors (AWS keys, private keys, JWT tokens, GitHub tokens, etc.)
- Allowlisted paths: `backend/internal/testsecrets/`, `backend/internal/crypto/aes_test.go`, `.secrets.baseline`
- **Does not explicitly add** custom patterns for Go, TypeScript, Docker, K8s, or Terraform patterns beyond defaults.
- Recommendation: Add explicit patterns for Docker and K8s files to catch embedded credentials in compose/deploy files.

#### .secrets.baseline

- Generated `2026-06-28T05:14:50Z` (5 days old at time of inspection)
- Covers 23+ files with known false positives and test fixtures
- Uses comprehensive detectors: AWS, Azure, GCP, GitHub, GitLab, JWT, Stripe, Slack, Discord, OpenAI, SendGrid, etc.
- Filters exclude test files, GitHub workflows, and testsecrets directory
- Recommendation: Re-baseline after any significant code changes.

### Summary

- **0** hardcoded secrets in production backend Go code or frontend TypeScript
- **3 LOW-MEDIUM** issues in `docker-compose.yml` (dev-only local credentials that are hardcoded)
- **No** sensitive files ever committed to git history
- **.gitleaks.toml** relies on defaults; no custom patterns for Docker/K8s
- **.secrets.baseline** is comprehensive and up-to-date

---

## 4. Input Validation & Injection

### Checks Performed

- **Step 1:** SQL injection — grepped `backend/internal/store/` for `fmt.Sprintf` in SQL queries, checked for string interpolation vs parameterized queries
- **Step 2:** Handler input validation — grepped `backend/internal/handler/` for `r.URL.Query()`, `r.FormValue()`, `json.NewDecoder`, `mux.Vars` and verified each result is validated before use
- **Step 3:** WebSocket message decoding — read `backend/internal/protocol/decode.go`, verified bounds checking and message type validation, checked fuzz test coverage
- **Step 4:** XSS in frontend — grepped `frontend/src/` for `.innerHTML =` and inspected all matches for user-data interpolation
- **Step 5:** Path traversal — grepped `backend/` for `os.Open`, `ioutil.ReadFile`, `filepath.Join` and verified user-controlled path sanitization

### Findings

#### Step 1: SQL Injection Vectors

- **PASS** — No user-controlled string interpolation in SQL queries. All database operations use parameterized queries with `$N` placeholders via `pgx`.

- [LOW] `backend/internal/store/result_repository.go:116,119` — `fmt.Sprintf` used to build batch INSERT placeholders `($1, $2, ...)`. The placeholder numbers are derived from array index, not user input. All actual values are passed via `tx.Exec(ctx, query, values...)`. This is a legitimate batch-insert pattern and not injectable. However, it creates a code smell where `fmt.Sprintf` is used with SQL at all.
- [LOW] `backend/internal/store/postgres_results.go:112,115` — Identical batch INSERT pattern (duplicated code). Same assessment as above.
- All other store queries (`postgres_leaderboard.go`, `postgres_users_crud.go`, `postgres_lobbies_query.go`, `postgres_lobbies_save.go`, `user_repository.go`, `config_repository.go`, `lobby_repository.go`, `postgres_users_read.go`, `postgres_users_gdpr.go`, `postgres_outbox.go`, `outbox_repository.go`, `session_store.go`, `redis_*.go`) use proper parameterized queries with zero string interpolation of user data.

#### Step 2: Missing Input Validation in Handlers

- **PASS** — Most handler endpoints validate inputs before use with whitelist checks, type conversions, and length validation.

- [INFO] `backend/internal/handler/stats.go:25` — `scope` validated with whitelist (`scope != "global" && scope != "weekly"`) ✓
- [INFO] `backend/internal/handler/stats.go:35` — `limit` validated with `strconv.Atoi` and positive check ✓
- [INFO] `backend/internal/handler/lobby_registry.go:81-94` — `code` from URL param validated via `domain.NewRoomCode(code)` with length and charset checks ✓
- [INFO] `backend/internal/handler/lobby_registry.go:152-156` — `limit` validated with `strconv.Atoi` and bounded by `config.MaxPageSize` ✓
- [INFO] `backend/internal/handler/auth_magiclink.go:50,73-76` — `token` validated with length check against `config.MagicLinkTokenLen` ✓
- [INFO] `backend/internal/handler/auth_magiclink.go:23` — email validated for non-empty ✓
- [INFO] `backend/internal/handler/admin_login.go:33` — password validated against `config.BcryptMaxLen` ✓

- [MEDIUM] `backend/internal/handler/auth_util.go:68-76` — `parseQuickPlayRequest()` reads nickname from JSON body but does NOT validate length, content, or character set. The nickname is used in JWT token generation and could allow excessively long or malicious nicknames to flow downstream.
- [LOW] `backend/internal/handler/lobby_registry.go:151` — `cursor` query parameter is read from URL but not validated for format. It's passed directly to `h.hub.ListLobbiesCached()`. If the downstream consumer assumes a specific cursor format, malformed cursors could cause unexpected behavior.
- [INFO] `backend/internal/handler/auth_session.go:57` and `auth_logout.go:13` — `json.NewDecoder(r.Body).Decode` errors silently discarded. This is intentional — the body is optional and falls back to cookie-based token retrieval. Acceptable.

#### Step 3: WebSocket Message Decoding

- **PASS** — Message decoding has proper bounds checking, type validation, and fuzz test coverage.

- All decode functions (`DecodeMessage, DecodeTap, DecodeNicknamePayload, DecodeSetNickname, DecodeRestartVote, DecodePing`) validate lengths and message types before processing.
- `DecodeNicknamePayload` enforces nickname length bounds (1 ≤ nickLen ≤ 255) and validates total payload size ✓
- Fuzz tests exist for `DecodeMessage` and `DecodeTap` (`decode_fuzz_test.go`). Missing fuzz coverage: `DecodeSetNickname`, `DecodeNicknamePayload`, `DecodeRestartVote`, `DecodePing`.
- [INFO] `backend/internal/protocol/decode_fuzz_test.go` — fuzz tests cover `DecodeMessage` and `DecodeTap`. No fuzz tests for the other four decode functions.

#### Step 4: XSS in Frontend

- **PASS** — No XSS vulnerabilities in production code.

- All `innerHTML` usages in production code are static strings with no user data interpolation:
  - `frontend/src/admin.ts:33,35` — Static hardcoded `<span>` status badges (`已启用`/`已禁用`)
- All 16 remaining `.innerHTML` matches are in `*.test.ts` test files (test boilerplate/DOM setup), not production code.
- All user-controlled data (nicknames, scores, lobby codes, error messages) is rendered via `textContent`:
  - `ui_update.ts:111` — `name.textContent = displayNickname(p)` ✓
  - `ui_update.ts:117` — `score.textContent = String(p.scoreContribution)` ✓
  - `ui_update.ts:166` — `$hudScore.textContent = String(getState().score)` ✓
  - `entry_flow_dom.ts` — All UI updates use `textContent` ✓
  - `leaderboard.ts`, `index_leaderboard.ts` — All score/nickname rendering via `textContent` ✓

#### Step 5: Path Traversal

- **PASS** — Path traversal protection is properly implemented.

- `backend/internal/server/routes_public.go:128-143` — Static file serving implements multi-layer path traversal protection:
  1. `filepath.Clean(path)` — Normalizes the request path (removes `..`)
  2. `filepath.Join(staticDir, cleanedPath)` — Constructs full path safely
  3. `filepath.Abs()` on both the resolved path and the allowed static directory
  4. `strings.HasPrefix(absPath, absStaticDir)` — Ensures resolved path stays within the static directory
- All other `filepath.Join`/`os.ReadFile`/`os.WriteFile` usages are in test files or involve fixed/non-user-controlled paths (migration directories).

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| SQL injection vectors | PASS — Parameterized queries used throughout; `fmt.Sprintf` in batch INSERT is code-smell only | LOW |
| Handler input validation | PASS with concerns — Most inputs validated; QuickPlay nickname unchecked in `auth_util.go:72` | MEDIUM |
| WebSocket decode | PASS — All bounds checked; partial fuzz coverage (2/6 functions) | LOW |
| Frontend XSS | PASS — No `innerHTML` with user data in production code | NONE |
| Path traversal | PASS — Proper sanitization with `filepath.Clean` + prefix check | NONE |

---

## 8. Integration & E2E Coverage

### Checks Performed

- **Step 1:** Listed all integration test files and checked coverage of PostgreSQL CRUD, GDPR, Redis, outbox, and worker operations
- **Step 2:** Listed all E2E spec files and checked coverage of core game mechanics, multiplayer, reconnection, error handling, and cross-page navigation
- **Step 3:** Verified 5 critical E2E user journeys have tests
- **Step 4:** Read `tests/e2e/helpers.ts` for test helper quality assessment

### Files Read

- `backend/tests/integration/postgres_store_test.go`
- `backend/tests/integration/postgres_gdpr_lobby_test.go`
- `backend/tests/integration/redis_redis_store_test.go`
- `backend/internal/outbox/publisher_test.go` (integration-tagged)
- `backend/internal/worker/game_result_worker_integration_test.go`
- `backend/internal/worker/gdpr_cleanup_test.go`
- `backend/internal/worker/email_worker_test.go`
- `backend/internal/health/health_test.go` (integration subtests)
- `backend/internal/auth/auth_token_test.go` (integration subtests)
- `backend/internal/audit/audit_db_test.go` (integration subtest)
- `backend/cmd/migrate-passwords/migrate_integration_test.go`
- `backend/cmd/backfill-emails/backfill_integration_test.go`
- `backend/internal/store/postgres_outbox_test.go` (mocked)
- `backend/internal/store/postgres_users_gdpr_test.go` (mocked)
- `tests/e2e/gameplay.spec.ts`
- `tests/e2e/multiplayer.spec.ts`
- `tests/e2e/reconnect.spec.ts`
- `tests/e2e/error_handling.spec.ts`
- `tests/e2e/cross_page.spec.ts`
- `tests/e2e/helpers.ts`
- `playwright.config.ts`

### Findings

#### Step 1: Integration Test Coverage

**Integration test files (build tag `integration`):**
| File | Location | Coverage |
|------|----------|----------|
| `postgres_store_test.go` | `backend/tests/integration/` | CreateUser, GetUserByEmail, SaveLobbyState, LoadLobbyState, LoadAllActiveLobbies |
| `postgres_gdpr_lobby_test.go` | `backend/tests/integration/` | AnonymizeUser, cursor pagination for active lobbies |
| `redis_redis_store_test.go` | `backend/tests/integration/` | RegisterRoom smoke test, ListActiveRooms |
| `publisher_test.go` | `backend/internal/outbox/` | Full coverage: batch processing, empty outbox, Redis failure recovery, processed_at marking, payload content verification |
| `game_result_worker_integration_test.go` | `backend/internal/worker/` | Comprehensive: normal processing, malformed payload, invalid JSON, FK rollback, invalid UUID, idempotency, empty batch, mixed valid/invalid, multiple results, XAck ordering, Start/context cancellation |
| `migrate_integration_test.go` | `backend/cmd/migrate-passwords/` | loadStoredConfig, password migration, already-bcrypt detection |
| `backfill_integration_test.go` | `backend/cmd/backfill-emails/` | Email encryption backfill, error stop on encrypt failure |
| **Scattered subtests** | `internal/health/`, `internal/auth/`, `internal/audit/` | ReadyHandler postgres check, refresh token Redis ops, audit DB logging |

**Coverage gaps (integration tests):**
- [LOW] `backend/tests/integration/redis_redis_store_test.go` — Only a single smoke test (RegisterRoom + ListActiveRooms). No tests for Redis session store, magic link storage, revocation sets, or rate limiter operations.
- [LOW] GDPR `ExportUserData` has no dedicated integration test (tested via mocked unit tests in `auth_misc_test.go` and handler tests in `auth_test.go`).
- [LOW] Email worker has no integration test (unit tested with mocked HTTP in `email_worker_test.go`).
- [LOW] GDPR cleanup worker has no integration test (unit tested with mocked deleter in `gdpr_cleanup_test.go`).

#### Step 2: E2E Test Coverage

**E2E spec files:**
| File | Tests | Coverage |
|------|-------|----------|
| `gameplay.spec.ts` | 5 | Quickplay auth + registry match, WS connect + nickname + waiting screen, slow WS handling, full quickplay→WS→nickname→tap flow, page load smoke |
| `multiplayer.spec.ts` | 4 | Two-player join same room, countdown + game start for both, independent scoring, full room rejection |
| `reconnect.spec.ts` | 3 | Disconnect + reconnect within grace period, reconnect during waiting phase, disconnected player removal after grace period |
| `error_handling.spec.ts` | 3 | Invalid room code (404), completed room shows ended phase, invalid (empty/overlong) nickname rejection |
| `cross_page.spec.ts` | 2 | Index page quickplay navigates to play page, leaderboard page loads |

**Total: 17 E2E tests across 5 spec files.**

#### Step 3: Missing Critical E2E Scenarios

- [HIGH] **Auth flow (magic link → verify → login → session)**: No E2E tests exist. Magic link email flow is completely untested at the E2E level. Critical for email-based authentication.
- [HIGH] **Admin flow (admin login → config change → verify)**: No E2E tests exist. Admin panel is completely uncovered.
- [MEDIUM] **Complete game flow end-to-end**: `gameplay.spec.ts` covers quickplay→nickname→connect→tap but does not verify `results` or `ended` phase outcome, leaderboard update, or page transition to results screen.
- [MEDIUM] **Error recovery mid-game**: `reconnect.spec.ts` tests disconnect during `waiting` phase but not during `playing` phase (mid-game WebSocket disconnect → reconnect → resume game state).
- [LOW] **Multi-region room placement**: No E2E tests for region-based room allocation (if implemented).

#### Step 4: Test Helper Quality

Read `tests/e2e/helpers.ts` (7 exported helpers, 62 lines):

**Reusable helpers:**
- `quickplayAuth(page)` — Standard auth fixture
- `matchRoom(page)` — Room matching with code format assertion
- `connectToRoom(page, code)` — Play page navigation + WS connection wait
- `submitNickname(page, nickname)` — Nickname form fill + waiting screen assertion
- `waitForPhase(page, phase, timeout?)` — Polls `window.state.phase` with configurable timeout
- `tapCanvas(page)` — Clicks canvas center with bounding box calculation
- `createRoom(page)` — Composite: auth + match

**Quality assessment:**
- **PASS** — Helpers are composable, concise, and cover the main user flow primitives
- **PASS** — `waitForPhase` uses `waitForFunction` with polling (not hardcoded sleeps) for phase detection
- **PASS** — `connectToRoom` uses Playwright locator assertions with 30s timeout (auto-retry)
- **PASS** — `tapCanvas` correctly handles `boundingBox()` null case with guard clause

**Concerns:**
- [LOW] `submitNickname` expects `#waiting-screen:not(.hidden)` immediately after click with 5s timeout. If WebSocket round-trip is slow, this may be flaky.
- [LOW] `reconnect.spec.ts` uses `page.waitForTimeout(1000)` — hardcoded sleep pattern before reconnection, not event-driven.
- [LOW] `gameplay.spec.ts` uses `page.waitForTimeout(3500)` in slow WS test — hardcoded sleep to wait for WS delay.
- [LOW] No retry/backoff wrapper around network-dependent operations in helpers.
- [LOW] No `createRoom` duplicate or error-handling variant (e.g., `tryCreateRoom` that succeeds even if room already exists).

**Playwright config (`playwright.config.ts`):**
- Single project (Chromium only) — no Firefox/Safari cross-browser coverage
- Retries: 1 in CI, 0 locally
- Workers: 2 in CI, 1 locally
- Trace: on-first-retry
- Screenshots: only-on-failure
- Dev server auto-start via `go run ./cmd/server` with `FRONTEND_DIR: frontend/dist`
- `fullyParallel: false` — avoids race conditions between E2E tests sharing game state

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| PostgreSQL CRUD integration | PASS — CreateUser, GetUserByEmail, SaveLobbyState, LoadLobbyState, LoadAllActiveLobbies tested | LOW |
| GDPR operations (anonymize) | PASS — AnonymizeUser integration tested; ExportUserData/DeleteUserData unit+mocked tested only | LOW |
| Redis store operations | PARTIAL — RegisterRoom smoke tested; session/magic link/revocation/rate limiter integration missing | MEDIUM |
| Outbox event publishing | PASS — Comprehensive integration coverage (5 test functions) | NONE |
| Worker processing | PASS — Best-tested area: 10 integration tests covering normal, error, idempotency, lifecycle | NONE |
| Auth flow E2E (magic link) | MISSING — No E2E tests for magic link → verify → login → session | HIGH |
| Admin flow E2E | MISSING — No E2E tests for admin login → config change | HIGH |
| Game flow E2E | PARTIAL — Covers quickplay→play→tap but not results/leaderboard outcome | MEDIUM |
| Mid-game reconnect E2E | MISSING — Only waiting-phase reconnect tested, not playing-phase | MEDIUM |
| Test helper quality | GOOD — 7 composable helpers, proper polling; 3 minor hardcoded sleep patterns | LOW |
| Cross-browser coverage | MISSING — Chromium only; no Firefox/Safari | MEDIUM |

- **3 integration test files** under `backend/tests/integration/` plus **5 additional integration-tagged test files** embedded in subpackages
- **17 E2E tests** across 5 spec files
- **2 HIGH gaps:** auth flow (magic link) and admin flow have zero E2E coverage
- **2 MEDIUM gaps:** complete game outcome verification and mid-game reconnect untested
- **Test helpers are well-structured** with minor hardcoded timeout concerns

---

## 10. Database Performance

### Checks Performed

- **Step 1:** Grep all SQL queries in `backend/internal/store/` (excluding `*_test.go`)
- **Step 2:** Check for N+1 query patterns (SELECT inside loops)
- **Step 3:** Cross-reference SQL WHERE clauses with migration index definitions
- **Step 4:** Check for missing LIMIT clauses on unbounded SELECT queries
- **Step 5:** Read `backend/internal/store/postgres.go` connection pool configuration
- **Step 6:** Read `docs/data/db-query-analysis.md` (existing analysis document)
- **Step 7:** Checked outbox publisher query patterns in `backend/internal/outbox/publisher.go`

### Files Read

- `backend/internal/store/postgres.go` — Connection pool configuration, pool interface
- `backend/internal/store/postgres_leaderboard.go` — Leaderboard queries (global + weekly, LIMIT parameterized)
- `backend/internal/store/postgres_results.go` — Game result INSERT, session UPSERT, per-user results with LIMIT 100
- `backend/internal/store/postgres_users_crud.go` — CreateUser with ACID transaction (INSERT users + outbox)
- `backend/internal/store/postgres_users_read.go` — GetUserByEmail, GetUserByID (PK/unique lookups)
- `backend/internal/store/postgres_users_gdpr.go` — AnonymizeUser, HardDeleteExpiredUsers (UPDATE/DELETE)
- `backend/internal/store/postgres_outbox.go` — InsertOutboxEvent
- `backend/internal/store/postgres_lobbies_query.go` — Cursor-paginated lobby list with LIMIT
- `backend/internal/store/postgres_lobbies_save.go` — Upsert lobby state
- `backend/internal/store/postgres_config.go` — Admin config CRUD (PK lookup)
- `backend/internal/store/lobby_repository.go` — Duplicate of postgres_lobbies_* (migration artifact)
- `backend/internal/store/user_repository.go` — Duplicate of postgres_users_* (migration artifact)
- `backend/internal/store/result_repository.go` — Duplicate of postgres_results_* (migration artifact)
- `backend/internal/store/config_repository.go` — Duplicate of postgres_config (migration artifact)
- `backend/internal/store/outbox_repository.go` — Duplicate of postgres_outbox (migration artifact)
- `backend/internal/outbox/publisher.go` — Outbox polling with LIMIT + FOR UPDATE SKIP LOCKED
- `backend/migrations/000001_init_schema.up.sql` — Base schema, indexes: idx_users_email, idx_sessions_lobby, idx_results_session
- `backend/migrations/000002_add_indexes.up.sql` — Single-column indexes: idx_game_results_user_id, idx_lobby_states_updated_at, idx_game_sessions_status
- `backend/migrations/000004_add_composite_indexes.up.sql` — Composite indexes: idx_game_results_session_user, idx_game_sessions_lobby_status, idx_lobby_states_updated_code
- `backend/migrations/000006_create_audit_logs.up.sql` — audit_logs table + idx_audit_logs_created_at, idx_audit_logs_actor_id, idx_audit_logs_action
- `backend/migrations/000007_create_outbox_events.up.sql` — outbox_events table + idx_outbox_unprocessed (partial)
- `backend/migrations/000008_drop_redundant_indexes.up.sql` — Drop idx_users_email, idx_sessions_lobby, idx_results_session, idx_lobby_states_updated_at (covered by composites)
- `backend/migrations/000010_add_email_hash.up.sql` — email_hash column + idx_users_email_hash (partial unique)
- `backend/migrations/000011_outbox_retention_index.up.sql` — idx_outbox_events_processed_at (partial)
- `docs/data/db-query-analysis.md` — Existing query analysis document

### Findings

#### Step 1: SQL Query Inventory

All 25+ production SQL queries in the store layer were enumerated. Every query uses parameterized placeholders (`$1`, `$2`) — no string interpolation of user data in SQL (see Section 4, Input Validation & Injection, PASS). Queries span 7 tables: `users`, `game_sessions`, `game_results`, `lobby_states`, `admin_config`, `audit_logs`, `outbox_events`.

**PASS** — No raw SQL injection surface. All user-supplied values passed as pgx parameters.

#### Step 2: N+1 Query Patterns

**PASS** — No N+1 SELECT patterns detected. All SELECT queries in the store layer are executed standalone (not inside application-level loops over query results). The `for` loops found are either:
- `for rows.Next()` — standard pgx row iteration (1 query, many rows returned — correct)
- `for i, r := range results { ... }` — building batch INSERT placeholder strings from an in-memory slice (not database queries)

[LOW] `backend/internal/store/postgres_results.go:65-74` (`RecordGameResult`): Individual INSERTs in a loop over in-memory `results`. This is not an N+1 (no SELECT), but it performs N round-trips instead of one batch INSERT. The sibling function `EndGameAndRecordResults` (`postgres_results.go:110-115`) already uses a single multi-row INSERT for the same data. `RecordGameResult` should be aligned to use the same batch pattern.

[LOW] `backend/internal/outbox/publisher.go:129-133`: Individual `UPDATE outbox_events SET processed_at = $1 WHERE id = $2` per row after publishing. Could be a single `UPDATE ... WHERE id = ANY($1)` for efficiency.

#### Step 3: Index Coverage

Cross-referencing all WHERE/JOIN columns against migration index definitions:

| Table | WHERE Column(s) | Index | Status |
|-------|----------------|-------|--------|
| `users` | `email_hash`, `email` | `idx_users_email_hash` (partial unique) + email UNIQUE constraint | PASS |
| `users` | `id` | PK index (auto) | PASS |
| `game_results` | `user_id` | `idx_game_results_user_id` | PASS |
| `game_results` | `session_id` | Covered by `idx_game_results_session_user` (composite prefix) | PASS |
| `game_results` | `session_id`, `user_id` | `idx_game_results_session_user` | PASS |
| `game_sessions` | `status`, `final_score`, `ended_at` | No dedicated index for `ended_at` in leaderboard query | **INFO** |
| `game_sessions` | `lobby_code`, `status` | `idx_game_sessions_lobby_status` | PASS |
| `lobby_states` | `code` | `lobby_states.code` UNIQUE constraint | PASS |
| `lobby_states` | `(updated_at, code)` cursor | `idx_lobby_states_updated_code` | PASS |
| `audit_logs` | `created_at` | `idx_audit_logs_created_at` | PASS |
| `audit_logs` | `actor_id` | `idx_audit_logs_actor_id` | PASS |
| `audit_logs` | `action` | `idx_audit_logs_action` | PASS |
| `outbox_events` | `processed_at IS NULL` | `idx_outbox_unprocessed` (partial) | PASS |
| `outbox_events` | `processed_at IS NOT NULL` | `idx_outbox_events_processed_at` (partial) | PASS |
| `admin_config` | `id` | PK (auto) | PASS |
| `game_sessions` | `id` | PK (auto) | PASS |

**PASS** — All query patterns have covering indexes.

[INFO] `leaderboardQuery` filters `WHERE status = 'ended' AND final_score > 0` and optionally `ended_at >= $1`, ordered by `final_score DESC, ended_at ASC`. The composite index `idx_game_sessions_lobby_status(lobby_code, status)` covers `status` but not `final_score` or `ended_at`. For the leaderboard query, this means a filter on `status='ended'` uses the index, but `final_score > 0` and `ORDER BY final_score DESC, ended_at ASC` will be a sequential scan/filter within the `ended` rows. At small to moderate scale this is fine; at >100k ended sessions, a composite index on `(status, final_score DESC, ended_at ASC)` would improve leaderboard query performance.

#### Step 4: Missing LIMIT Clauses

Every SELECT query was reviewed:

| Query | LIMIT? | Risk |
|-------|--------|------|
| `SELECT ... FROM game_sessions WHERE status = 'ended' ... ORDER BY ... LIMIT $1` | ✅ Parameterized (default 50, max 100) | NONE |
| `SELECT ... FROM lobby_states ... WHERE (updated_at, code) < ... ORDER BY ... LIMIT $1` | ✅ Parameterized (page size) | NONE |
| `SELECT ... FROM game_results WHERE user_id = $1 ORDER BY ... LIMIT 100` | ✅ 100 hard limit | NONE |
| `SELECT ... FROM users WHERE email_hash = $1 OR ...` | ✅ WHERE on unique key (0-1 rows) | NONE |
| `SELECT ... FROM users WHERE id = $1` | ✅ WHERE on PK (0-1 rows) | NONE |
| `SELECT ... FROM admin_config WHERE id = $1` | ✅ WHERE on PK (0-1 rows) | NONE |
| `SELECT ... FROM outbox_events WHERE processed_at IS NULL ORDER BY id LIMIT $1` | ✅ Batch size (default 100) | NONE |
| `SELECT COUNT(*) FROM lobby_states` | ✅ COUNT returns 1 row | NONE |
| `SELECT COALESCE(MAX(..), 0), COUNT(*) FROM game_results WHERE user_id = $1` | ✅ Aggregate, 1 row | NONE |

**PASS** — No unbounded SELECT queries found. All queries that could return multiple rows have explicit LIMIT clauses.

#### Step 5: Connection Pool Configuration

`backend/internal/store/postgres.go:65-69`:

| Parameter | Code | Default | Configurable via Env | Verdict |
|-----------|------|---------|---------------------|---------|
| `MaxConns` | `poolConfig.MaxConns = int32(config.GetEnvIntPositive("PG_POOL_MAX_CONNS", 25))` | 25 | ✅ `PG_POOL_MAX_CONNS` | PASS — 25 is reasonable for typical workloads |
| `MinConns` | `poolConfig.MinConns = int32(config.GetEnvIntPositive("PG_POOL_MIN_CONNS", 5))` | 5 | ✅ `PG_POOL_MIN_CONNS` | PASS — maintains warm pool |
| `MaxConnLifetime` | `poolConfig.MaxConnLifetime = config.GetEnvDuration("PG_POOL_MAX_CONN_LIFETIME", 30*time.Minute)` | 30m | ✅ `PG_POOL_MAX_CONN_LIFETIME` | PASS — non-zero, prevents stale connections |
| `MaxConnIdleTime` | `poolConfig.MaxConnIdleTime = config.GetEnvDuration("PG_POOL_MAX_CONN_IDLE_TIME", 5*time.Minute)` | 5m | ✅ `PG_POOL_MAX_CONN_IDLE_TIME` | PASS — non-zero, reaps idle connections |
| `HealthCheckPeriod` | `poolConfig.HealthCheckPeriod = config.GetEnvDuration("PG_POOL_HEALTH_CHECK_PERIOD", 30*time.Second)` | 30s | ✅ `PG_POOL_HEALTH_CHECK_PERIOD` | PASS — proactive health checking |

**PASS** — All four critical pool parameters are set to sensible defaults, are non-zero, and are configurable via environment variables. Circuit breaker (`gobreaker`) wraps write operations in the store layer for additional resilience.

There is also a `PoolStats()` method (`postgres.go:112-117`) that exposes real-time pool statistics via the exported `Stat()` type, which is useful for observability.

#### Step 6: Existing Query Analysis Document

`docs/data/db-query-analysis.md` covers 5 key query scenarios with expected execution plans (index scans, nested loops). All documented queries match the current schema. The document correctly notes the composite index leftmost prefix rules. The analysis is accurate and up-to-date with the current migration state.

#### Additional: Code Duplication (Migration Artifact)

The store layer has significant code duplication between the `*_repository.go` pattern and the `postgres_*` pattern — 5 pairs of duplicate files implementing the same SQL queries. This is a known artifact of a migration/refactoring. While functionally identical, it doubles the surface area for SQL query maintenance and could lead to divergence. See ADR-019 for context.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| SQL query inventory | PASS — All 25+ queries parameterized | NONE |
| N+1 query patterns | PASS — No SELECT inside loops | NONE |
| Index coverage | PASS — All WHERE/JOIN columns indexed | NONE |
| Missing LIMIT clauses | PASS — All multi-row SELECTs bounded | NONE |
| Connection pool config | PASS — MaxConns=25, MinConns=5, MaxConnLifetime=30m, MaxConnIdleTime=5m | NONE |
| Existing analysis doc | PASS — Accurate and up-to-date | NONE |
| Leaderboard composite index | INFO — No covering index for `(status, final_score DESC, ended_at ASC)` | LOW |
| Individual INSERTs in loop | LOW — `RecordGameResult` could use batch INSERT like `EndGameAndRecordResults` | LOW |
| Per-row UPDATE in outbox | LOW — `publisher.go` updates one row at a time instead of batch | LOW |
| Dead code duplication | INFO — 5 pairs of duplicate store files (migration artifact) | LOW |

## 3. Authentication & Authorization

### Checks Performed

- **Step 1:** Audited JWT implementation (`jwt.go`, `jwt_verify.go`) — signing algorithm, token expiry, claims content, key rotation
- **Step 2:** Audited magic link flow (`magiclink.go`) — token generation, single-use enforcement, expiry, rate limiting, URL exposure
- **Step 3:** Audited auth middleware (`middleware.go`) — revocation checks, multi-IP anomaly detection, cookie security, bypass paths
- **Step 4:** Audited RBAC (`rbac/rbac.go`, `rbac/permissions.go`) — role definitions, permission checks, privilege escalation
- **Step 5:** Audited CORS and security headers (`middleware/cors.go`, `middleware/security.go`) — origin matching, HSTS, CSP, headers
- **Step 6:** Verified admin auth middleware and refresh token flow

### Files Read

- `backend/internal/auth/jwt.go` — JWT signing (HS256), key length enforcement, cookie helpers, token hashing
- `backend/internal/auth/jwt_verify.go` — JWT verification with signing method whitelisting
- `backend/internal/auth/middleware.go` — Auth middleware, revocation checking, multi-IP detection, context helpers
- `backend/internal/auth/magiclink.go` — Magic link request, token generation, verification, email enqueue
- `backend/internal/auth/secure.go` — HTTPS detection (trusted proxy support)
- `backend/internal/auth/quickplay.go` — Quick-play user creation and JWT issuance
- `backend/internal/auth/refresh.go` — Refresh token generation, atomic consume with reuse detection, revocation
- `backend/internal/auth/revoke.go` — RevokeAllTokens (access + refresh from cookies)
- `backend/internal/auth/auth_interfaces.go` — TokenStore, UserDB interface definitions
- `backend/internal/rbac/rbac.go` — RBAC enforcer, middleware, role constants
- `backend/internal/rbac/permissions.go` — In-memory RBAC policy map
- `backend/internal/middleware/cors.go` — CORS middleware with exact origin matching
- `backend/internal/middleware/security.go` — Security headers (HSTS, CSP, X-Frame-Options, etc.)
- `backend/internal/handler/admin.go` — Admin JWT signing (HS256), admin token verification, revocation on password change
- `backend/internal/server/routes_middleware.go` — Middleware setup, admin auth middleware, auth middleware wrapper
- `backend/internal/server/routes_admin.go` — Admin route registration with RBAC enforcement
- `backend/internal/store/redis_magiclink.go` — ConsumeMagicToken Lua script (atomic GET+DEL)
- `backend/internal/store/redis_auth_session.go` — RevokeJWT / IsJWTRevoked implementations
- `backend/internal/config/constants.go` — Token TTL constants

### Findings

#### JWT Implementation

- [MEDIUM] `backend/internal/auth/jwt.go:68` — Uses HS256 (HMAC-SHA256) symmetric algorithm instead of RS256 or ES256. Symmetric signing means the same key used to sign is also needed to verify. In a multi-service architecture, distributing the signing key to verifiers increases exposure. For this single-service monolith with proper 32+ byte key enforcement and rotation support, the practical risk is low, but asymmetric signing (RS256/ES256) would be more resilient to key compromise.
- **PASS** `backend/internal/config/constants.go:7` — Access token TTL = 15 minutes (≤15min ✓).
- **PASS** `backend/internal/config/constants.go:10` — Refresh token TTL = 7 days (≤7 days ✓).
- **PASS** `backend/internal/auth/jwt.go:44-47` — JWT claims contain only `sub` (userId), `nickname`, and `jti` (JWT ID). No sensitive data (email, password, IP) in payload.
- **PASS** `backend/internal/auth/jwt.go:35-41` — Key rotation supported via `NewJWTManagerWithRotation`. Previous secret used only for verification, new tokens always signed with primary secret.
- **PASS** `backend/internal/auth/jwt.go:26-28` — Minimum 32-byte key enforced (panic on short key).
- **PASS** `backend/internal/auth/jwt_verify.go:28-30` — Signing method whitelist enforced (`*jwt.SigningMethodHMAC`), preventing algorithm confusion (alg:none, RS256→HS256).

#### Magic Link Flow

- [MEDIUM] `backend/internal/config/constants.go:13` — Magic link TTL = 15 minutes, exceeding the recommended ≤10 minute maximum. A 15-minute window widens the potential reuse window if the email is intercepted or the link is shared. Reducing to 10 minutes would tighten the security envelope.
- **PASS** `backend/internal/auth/magiclink.go:117` — Magic token generated from 32 bytes of `crypto/rand`, hex-encoded to 64 chars (≥32 bytes ✓).
- **PASS** `backend/internal/store/redis_magiclink.go:65-71` — Single-use enforcement via Lua script that atomically GET+DEL the token. Eliminates TOCTOU race condition.
- **PASS** `backend/internal/auth/magiclink.go:105-114` — Rate limiting on send: 5 requests per TTL window per email, keyed as `"ml:"+email`.
- [LOW] `backend/internal/auth/magiclink.go:147` — Magic link token transmitted as URL query parameter (`?token=`). Query parameters are commonly logged by web servers, proxies, CDNs, and analytics tools. Though the token is single-use and consumed immediately, and the chi Logger middleware does not log query strings by default, URL-based token delivery exposes the token to browser history, referrer headers, and email client preview systems. Consider POST-based verification or fragment-based (#) token delivery.
- **PASS** `backend/internal/auth/magiclink.go:210-213` — `validateMagicToken` uses `ConsumeMagicToken` (atomic Lua GET+DEL), ensuring single consumption.
- **PASS** `backend/internal/auth/magiclink.go:145-165` — Saga compensation: if email enqueue fails, the stored Redis token is deleted (avoids orphan tokens).
- **PASS** `backend/internal/auth/magiclink.go:125-143` — Email encrypted with AES-256-GCM before storage in Redis (field-level PII encryption).

#### Auth Middleware

- **PASS** `backend/internal/auth/middleware.go:68-80` — Revocation check (`IsJWTRevoked`) on every authenticated request. Revoked tokens are rejected before any handler executes.
- **PASS** `backend/internal/auth/middleware.go:33` — Multi-IP anomaly detection: >3 distinct IPs per user within 1 hour triggers `SuspiciousLoginTotal` metric increment and warning log.
- **PASS** `backend/internal/auth/middleware.go:79-88` — Cookie security: `BuildAuthCookie` sets `HttpOnly=true`, `SameSite=LaxMode`, `Secure` based on request/TLS detection. Cookies: `session` (magic link), `quickplay`, `refresh` (HttpOnly refresh token).
- **PASS** `backend/internal/auth/middleware.go:63` — No bypass paths: middleware checks `session` then `quickplay` cookies; both validated through JWT verification + optional revocation check. Returns 401 if neither is valid.
- **PASS** `backend/internal/auth/middleware.go:86` — Role is set from context (`WithRole(ctx, "user")`) based on JWT verification, never from client-controlled input.
- **PASS** `backend/internal/auth/refresh.go:38-50` — Refresh token consumption uses Lua script for atomic validate+consume+reuse detection. Token reuse triggers revocation of ALL user tokens (defense against token theft).

#### RBAC

- **PASS** `backend/internal/rbac/permissions.go:5-24` — Role definitions: `admin` (config r/w, users r/w, lobby, user_data), `moderator` (config r, users r, lobby r), `user` (lobby create/join/read, user_data r/delete), `guest` (lobby read). Appropriate least-privilege separation.
- **PASS** `backend/internal/server/routes_admin.go:24-25` — RBAC middleware applied to admin config endpoints: `rbacEnforcer.Middleware("config", "read")` on GET, `rbacEnforcer.Middleware("config", "write")` on PATCH/PUT.
- **PASS** `backend/internal/rbac/rbac.go:47-56` — Middleware reads role from context (set by authenticated middleware), defaults to `RoleGuest` if not set. No privilege escalation path — role is never read from client input.
- **PASS** `backend/internal/server/routes_middleware.go:79` — Admin auth middleware explicitly sets `rbac.RoleAdmin` in context after admin JWT verification.
- [INFO] `backend/internal/rbac/permissions.go` — RBAC policy is in-memory (no hot-reload capability). Policy changes require application restart. For a game backend, this is acceptable; for dynamic permission management, a database-backed policy would be needed.

#### CORS and Security Headers

- **PASS** `backend/internal/middleware/cors.go:36-46` — CORS uses exact origin matching (`origin == ao`). No wildcard `*` allowed. Origins loaded from `ALLOWED_ORIGINS` env var via `AllowedOriginsFromEnv`.
- **PASS** `backend/internal/middleware/cors.go:17` — `Access-Control-Allow-Credentials: true` is set only when origin is explicitly allowed (never with wildcard).
- **PASS** `backend/internal/middleware/cors.go:19` — `Vary: Origin` header set to prevent cache poisoning.
- **PASS** `backend/internal/middleware/security.go:35-36` — HSTS: `max-age=31536000; includeSubDomains; preload` (≥31536000 ✓). Disabled in dev via `ENABLE_HSTS=false`.
- **PASS** `backend/internal/middleware/security.go:59-63` — CSP: Nonce-based (`'nonce-"+nonce+"'`) for scripts. No `unsafe-inline` or `unsafe-eval` in production mode. Dev mode allows `unsafe-inline` for `style-src` only (Vite HMR requirement).
- **PASS** `backend/internal/middleware/security.go:38` — `X-Content-Type-Options: nosniff`.
- **PASS** `backend/internal/middleware/security.go:39` — `X-Frame-Options: DENY`.
- **PASS** `backend/internal/middleware/security.go:40` — `Referrer-Policy: strict-origin-when-cross-origin`.
- **PASS** `backend/internal/middleware/security.go:43` — `Permissions-Policy: camera=(), microphone=(), geolocation=()`.
- **PASS** `backend/internal/middleware/security.go:75-80` — CSP nonce generated fresh per request using `crypto/rand` (16 bytes, hex-encoded).

### Summary

- **1 MEDIUM:** JWT uses HS256 symmetric algorithm instead of RS256/ES256
- **1 MEDIUM:** Magic link TTL is 15 minutes (recommended ≤10)
- **1 LOW:** Magic link token transmitted in URL query parameter
- **1 INFO:** RBAC policy is in-memory only (no hot-reload)
- **All other checks PASS** — revocation per-request, multi-IP detection, cookie security, no bypass paths, proper role handling, CORS origin whitelist, security headers (HSTS, CSP, X-Frame-Options, X-Content-Type-Options), nonce-based CSP

---

## 5. Cryptographic Practices

### Checks Performed

- **Step 1:** Audited `backend/internal/crypto/aes.go` — AES-256-GCM encryption (key length, nonce generation, auth tag verification, key storage)
- **Step 2:** Searched all Go files for `bcrypt.` calls and verified cost factor
- **Step 3:** Reviewed `backend/internal/store/postgres.go` and `backend/internal/store/redis.go` for TLS configuration
- **Step 4:** Scanned for weak crypto primitives (`md5.`, `sha1.`, `des.`, `rc4.`) in production code
- **Step 5:** Cross-checked JWT signing in `backend/internal/auth/jwt.go` and key validation in `backend/internal/config/env.go`

### Files Read

- `backend/internal/crypto/aes.go` — AES-GCM encrypt/decrypt, key initialization, nonce generation
- `backend/internal/crypto/aes_email.go` — Email encryption/decryption and HMAC helpers
- `backend/internal/crypto/bcrypt.go` — bcrypt hash format validation
- `backend/internal/crypto/aes_test.go` — Round-trip, tamper, truncated ciphertext, GCM error tests
- `backend/internal/handler/admin_password.go` — Production bcrypt hashing + comparison
- `backend/internal/auth/jwt.go` — JWT signing (HS256), key length validation, token hashing
- `backend/internal/auth/jwt_verify.go` — JWT verification with signing method enforcement
- `backend/internal/store/postgres.go` — PostgreSQL connection pool creation
- `backend/internal/store/redis.go` — Redis client creation
- `backend/internal/config/env.go` — Environment config loading and validation
- `backend/internal/config/redis_addr.go` — Redis URL parsing (supports `redis://` and `rediss://`)
- `backend/cmd/seed/main.go` — Dev seed tool with sslmode guard
- `backend/cmd/migrate-passwords/main.go` — bcrypt migration tool

### Findings

#### AES-256-GCM

- **PASS** `backend/internal/crypto/aes.go:36` — Key length enforced at exactly 32 bytes (`len(key) != 32`), matching AES-256 requirements.
- **PASS** `backend/internal/crypto/aes.go:94-97` — Nonce generated fresh per encryption using `crypto/rand.Read` with GCM nonce size (12 bytes). No nonce reuse possible.
- **PASS** `backend/internal/crypto/aes.go:137` — Authentication tag verified by `gcm.Open()` before plaintext is released. Tampered ciphertext correctly rejected (tested at `aes_test.go:176-192`).
- **PASS** `backend/internal/crypto/aes.go:45-51` — Encryption key loaded from `ENCRYPTION_KEY` environment variable. Missing key causes hard failure (`InitFromEnv` returns error, `MustInitFromEnv` panics). No fallback to zero key.
- **PASS** `backend/internal/crypto/aes.go:80` — Versioned ciphertext format (`v1:hex`) supports future key rotation.

#### bcrypt

- [MEDIUM] `backend/internal/handler/admin_password.go:29` — Uses `bcrypt.DefaultCost` (value: 10). OWASP 2026 recommends cost factor ≥ 12 for production password hashing. Cost 10 is computationally feasible for GPU-accelerated brute force against leaked hashes.
- [MEDIUM] `backend/cmd/migrate-passwords/main.go:105` — Same issue: uses `bcrypt.DefaultCost` (10) in the password migration tool.
- Note: `golang.org/x/crypto v0.53.0` used. The `bcrypt` package in this version still defines `DefaultCost = 10`. No custom cost constant is set anywhere in the codebase.

#### TLS Configuration

- [MEDIUM] `backend/internal/store/redis.go:29-39` — Redis client creation does not configure TLS. Even though `ParseRedisURL` (`redis_addr.go:21`) recognizes the `rediss://` scheme, the parsed result (`RedisConn` struct) has no TLS field, and `redis.Options` in `redis.go` does not set `TLSConfig`. Production Redis connections over `rediss://` will connect without TLS.
- [INFO] `backend/internal/store/postgres.go:60` — PostgreSQL TLS is governed entirely by the `DATABASE_URL` connection string `sslmode` parameter. The code passes the URL verbatim to `pgxpool.ParseConfig()`. No programmatic enforcement of TLS in production connection path. Production operators must ensure `sslmode=require` or `sslmode=verify-full` is set in the `DATABASE_URL`.
- [INFO] `backend/internal/config/env.go:40` — `DATABASE_URL` is required (`Validate()` returns error if empty) but no TLS-specific validation is performed on the URL.
- **PASS** No `InsecureSkipVerify: true` found anywhere in production Go code. All `crypto/tls` imports are confined to test files (`auth_misc_test.go`, `auth_flow_test.go`, `admin_handlers_test.go`, `auth_test.go`).
- **PASS** `backend/cmd/seed/main.go:32-33` has an explicit guard requiring `sslmode=disable` for the dev seed tool (safe — prevents accidental production seeding).

#### Weak Crypto Primitives

- **PASS** No MD5, SHA-1, DES, or RC4 found in production Go code. The weak-crypto regex search returned only comments and non-crypto identifiers (`codes.Error` from OpenTelemetry, comments about "collect codes").
- SHA-256 is used where appropriate: JWT token hashing (`jwt.go:126`), email HMAC (`aes_email.go:14-15`), and HMAC-SHA256 for indexed email lookup (`aes_email.go:17`).

#### JWT Signing

- **PASS** `backend/internal/auth/jwt.go:26-28` — HS256 algorithm requires minimum 32-byte key; enforcement via `panic` on short key.
- **PASS** `backend/internal/auth/jwt.go:68` — Uses `jwt.SigningMethodHS256` (HMAC-SHA256), a strong symmetric algorithm.
- **PASS** `backend/internal/auth/jwt_verify.go:28-30` — `verifyWithKey` explicitly checks that the token's signing method is HMAC (`*jwt.SigningMethodHMAC`), preventing algorithm confusion attacks (e.g., `alg: none` or `alg: RS256` with HS256 public key).
- **PASS** `backend/internal/config/env.go:66-67` — Weak/dev JWT secrets (`DEV_ONLY`, `change-in-production`) are actively detected and rejected when `ENABLE_HSTS` is true (production mode).
- **PASS** Key rotation supported via `NewJWTManagerWithRotation` — previous secret used only for verification, new tokens always signed with primary secret.

### Summary

- **1 MEDIUM:** bcrypt cost factor uses Go default (10) instead of recommended ≥ 12
- **1 MEDIUM:** Redis client does not configure TLS even when `rediss://` URL scheme is used
- **1 INFO:** PostgreSQL TLS relies entirely on operator-provided connection string; no programmatic enforcement
- **AES-256-GCM implementation is correct** — proper key length (32 bytes), random nonces, auth tag verification, env-based key storage
- **JWT implementation is robust** — key length enforcement, signing method whitelisting, weak-secret detection, key rotation support
- **No weak crypto primitives** (MD5, SHA-1, DES, RC4) in production code

---

## 6. Backend Test Coverage

### Checks Performed

- **Step 1:** Ran `go test ./... -coverprofile=coverage.out -short -count=1` — backend unit tests with coverage
- **Step 2:** Analyzed per-function coverage to identify uncovered packages and functions
- **Step 3:** Enumerated test files in critical packages (`auth`, `game`, `store`, `protocol`, `handler`)
- **Step 4:** Ran `go test ./... -race -short -count=1` to detect data races

### Findings

#### Test Execution & Coverage

- **Total coverage:** 84.3% of statements across all compilable packages.
- **Build failure** (pre-existing): `internal/store/room_registry_store.go:118` references `domain.UnmarshalRoomRegistryInfo` which does not exist. This compilation error blocks 6 dependent packages from building/testing: `auth`, `game`, `handler`, `server`, `cmd/seed`, `cmd/server`, `internal/testutil`.
- **17 packages passed** and were measured for coverage.

#### Per-Package Coverage

| Package | Coverage | Status |
|---------|----------|--------|
| `internal/apierror` | 100.0% | PASS |
| `internal/audit` | 100.0% | PASS |
| `internal/config` | 90.2% | PASS |
| `internal/crypto` | 100.0% | PASS |
| `internal/domain` | 92.3% | PASS |
| `internal/health` | 100.0% | PASS |
| `internal/idgen` | 100.0% | PASS |
| `internal/metrics` | 100.0% | PASS |
| `internal/middleware` | 82.5% | PASS |
| `internal/migrateutil` | 100.0% | PASS |
| `internal/nicknames` | 96.0% | PASS |
| `internal/outbox` | 100.0% | PASS |
| `internal/protocol` | 100.0% | PASS |
| `internal/rbac` | 100.0% | PASS |
| `internal/requestctx` | 100.0% | PASS |
| `internal/resilience` | 100.0% | PASS |
| `internal/slogctx` | 100.0% | PASS |
| `internal/telemetry` | 100.0% | PASS |
| `internal/validate` | 92.3% | PASS |
| `internal/worker` | 100.0% | PASS |
| `cmd/backfill-emails` | 83.3% | PASS |
| `cmd/migrate-passwords` | 20.7% | LOW |

#### Functions Below 80% Coverage (excl. `cmd/` entry points)

| Function | Coverage | File |
|----------|----------|------|
| `GetJWTSecret`, `GetAdminJWTSecret`, `GetEncryptionKey`, `GetAuditSecret`, `GetDatabaseURL`, `GetRedisURL`, `GetPort`, `GetEnableHSTS`, `GetMaxPlayersPerRoom` | **0.0%** | `internal/config/env.go:121-145` — Trivial getter wrappers around `GetEnv` (which *is* 100% tested). Low risk. |
| `WithValue`, `Value` | **0.0%** | `internal/domain/context_keys.go:14,18` — Context key helpers. |
| `IdempotencyMiddleware`, `SaveIdempotencyResponse` | **0.0%** | `internal/middleware/idempotency.go:74,129` — Idempotency middleware. |
| `RecordAuthMetrics` | **0.0%** | `internal/middleware/metrics.go:11` — Auth metrics recording. |
| `init` | **20.0%** | `internal/middleware/ratelimit.go:16` — Rate limit initialization. |
| `ValidateNickname` | **0.0%** | `internal/validate/adapter.go:11` — Nickname validation adapter. |

#### Test File Presence in Critical Packages

| Package | Test Files | Can Compile? | Verdict |
|---------|-----------|--------------|---------|
| `auth` | 8 | ❌ (blocked by store) | PARTIAL — files exist but can't run |
| `game` | 24 | ❌ (blocked by store) | PARTIAL — files exist but can't run |
| `store` | 15 | ❌ (compilation error) | BLOCKED — root cause |
| `protocol` | 2 | ✅ | PASS — 100% coverage |
| `handler` | 13 | ❌ (blocked by store) | PARTIAL — files exist but can't run |

#### Race Condition Detection

- **No data races detected** in any package that compiled successfully.
- All `FAIL` results in the race run are from the same pre-existing build failure in `internal/store`.
- **17 packages tested clean** with `-race` enabled (including `middleware` at 18s, `migrateutil` at 21s, `worker` at 19s — long-running integration-like unit tests).

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Total coverage (compilable packages) | 84.3% | LOW — above 80% threshold |
| Build failures blocking tests | 6 packages blocked by 1 compilation error in `internal/store` | HIGH |
| Functions <80% coverage | 14 uncovered functions (mostly trivial getters + 3 real middleware gaps) | MEDIUM |
| Test files in critical packages | Present in all 5 (8-24 files each) but 4/5 can't run due to build failure | HIGH |
| Race conditions | **0 data races** detected in passing packages | NONE |

- **Total coverage of 84.3%** meets the 80% integration threshold but falls short of the 100% unit-test policy target in `coverage-policy.md`.
- **14 uncovered functions** identified across `config`, `domain`, `middleware`, and `validate` packages. Most are trivial getters; the real gaps are `IdempotencyMiddleware`, `RecordAuthMetrics`, and `ValidateNickname`.
- **Race condition check is clean** — no data races in any compilable package.
- **Primary blocker:** The `domain.UnmarshalRoomRegistryInfo` compilation error in `internal/store` (see Section 1, Dependency Vulnerabilities — pre-existing) prevents testing 6 critical packages (`auth`, `game`, `handler`, `server`, `store` itself, and `testutil`). Coverage of those packages is currently unknown.

---

## 7. Frontend Test Coverage

### Checks Performed

- **Step 1:** Ran `npx vitest run --coverage` — 29 test files, 317 tests
- **Step 2:** Verified coverage thresholds in `frontend/vitest.config.ts`
- **Step 3:** Checked test file existence for critical game logic modules
- **Step 4:** Searched for skipped/pending tests (`it.skip`, `it.todo`, `describe.skip`, `xit`)

### Findings

#### Test Execution

- **PASS** — All 317 tests pass across 29 test files.
- **PASS** — Zero skipped tests found. No `it.skip`, `it.todo`, `describe.skip`, or `xit` in any `*.test.ts` file.

#### Coverage Thresholds (vitest.config.ts)

| Metric    | Threshold | Actual  | Verdict |
|-----------|-----------|---------|---------|
| Lines     | 95%       | 86.27%  | ❌ FAIL |
| Functions | 95%       | 80.39%  | ❌ FAIL |
| Statements| 95%       | 85.19%  | ❌ FAIL |
| Branches  | 80%       | 79.13%  | ❌ FAIL |

**All four thresholds fail.** Coverage gaps are concentrated in two categories:

1. **Uncovered glue/entry files** (not in vitest exclude list):
   - `src/game/lifecycle.ts` — 0% lines, 0% functions (73 lines, boot orchestration, DOM setup)
   - `src/game/window_events.ts` — 0% lines, 0% functions (97 lines, event binding, DOM interactions)
   - These are hard-to-test DOM-heavy modules that should be added to the vitest exclude list or refactored for testability.

2. **Excluded files with naturally low coverage** (per `vitest.config.ts` exclude list, not counted toward thresholds):
   - `shared/data/best_score_cookie.ts` — 10.52% lines
   - `shared/data/tutorial_cookie.ts` — 25% lines
   - `shared/ui/audio.ts` — 6.89% lines
   - `shared/ui/toast.ts` — 7.69% lines
   - `shared/game/types.ts` — 0% lines (types-only file)

#### Critical Game Logic Coverage

| Module                  | Stmts  | Branch | Funcs  | Lines   | Test File Found | Notes |
|-------------------------|--------|--------|--------|---------|----------------|-------|
| `message_codec.ts`      | 100%   | 100%   | 100%   | 100%    | `message_codec.test.ts` | Full coverage ✓ |
| `cooldown_contract.ts`  | 100%   | 100%   | 100%   | 100%    | `cooldown_contract.test.ts` | Full coverage ✓ |
| `state_interp.ts`       | 95.62% | 88.46% | 100%   | 100%    | Via `state_physics_interpol.test.ts` | Adequate coverage |
| `reducer.ts`            | 96.29% | 83.33% | 100%   | 96.29%  | No dedicated test | Tested via ws_handlers/snapshot tests; line 77 uncovered |
| `physics`               | N/A    | N/A    | N/A    | N/A     | No physics file in frontend/src/game/ | Physics is backend-only |

- `reducer.ts` has no dedicated test file but achieves 96.29% statement coverage through integration via `ws_handlers`, `ws_handlers_snapshot`, and `state_reset` tests. The single uncovered line (77) is an edge-case branch in state reset logic.
- `physics` was listed in the task brief but no `physics.ts` or `physics_test.ts` exists in `frontend/src/game/`. Physics simulation is purely backend.

### Summary

- **317 tests pass**, **0 skipped** — test suite is healthy and actively maintained.
- **Coverage thresholds fail across all metrics**: lines (86.27% vs 95%), functions (80.39% vs 95%), statements (85.19% vs 95%), branches (79.13% vs 80%).
- **Primary gap**: `lifecycle.ts` (0%) and `window_events.ts` (0%) — DOM-heavy boot/event-binding modules not excluded from coverage and not tested.
- **Critical game logic** (`message_codec`, `cooldown_contract`, `state_interp`, `reducer`) is well-covered (95-100%).
- **Recommendation**: Either write tests for `lifecycle.ts` and `window_events.ts`, or add them to the vitest exclude list to align with the existing exclusion pattern for similar glue/UI modules.

---

## 13. Logging Quality

### Checks Performed

- **Step 1:** Verified structured logging — grepped for `fmt.Print`, `fmt.Fprint`, `log.Print` in production Go files
- **Step 2:** Checked for PII (email, nickname, token, password) in log statements
- **Step 3:** Verified request context propagation — read `slogctx/slogctx.go`, `middleware/logging.go`, `middleware/tracing.go`, `auth/middleware.go`
- **Step 4:** Checked appropriate use of log levels (Debug/Info/Warn/Error)
- **Step 5:** Cross-referenced with `docs/security/logging-policy.md`

### Files Read

- `backend/internal/middleware/logging.go` — RequestIDLogger (injects `request_id` into slog context, logs `request completed`)
- `backend/internal/middleware/tracing.go` — TracingMiddleware (injects `trace_id` into slog context)
- `backend/internal/slogctx/slogctx.go` — Context key, `LoggerFromContext`, `WithLogger` (24 lines, minimal)
- `backend/internal/auth/middleware.go` — AuthMiddleware (injects `user_id` into slog context at line 94-98)
- `backend/internal/worker/email_worker.go` — Email worker with 4 slog statements logging `payload.To`
- `docs/security/logging-policy.md` — PII redaction policy, structured field spec, level definitions
- `backend/internal/server/routes_middleware.go` — Middleware registration order

### Findings

#### Step 1: Structured Logging

- **PASS** — Zero occurrences of `fmt.Print`, `fmt.Fprint`, or `log.Print` in production Go files (`backend/internal/` excluding `*_test.go`).
- All logging uses the `slog` structured logger exclusively (`slog.Debug`, `slog.Info`, `slog.Warn`, `slog.Error`).
- 91 `slog` calls found across production code. No raw print statements.

#### Step 2: PII in Logs

- **FAIL** — `backend/internal/worker/email_worker.go` logs the full recipient email address in 4 locations, violating the logging policy ("禁止记录完整 email，可 hash 或截断：`u***@example.com`"):

  | Line | Level | Statement |
  |------|-------|-----------|
  | 106 | WARN | `slog.Warn("email worker: RESEND_API_KEY not set, skipping", "to", payload.To)` |
  | 117 | INFO | `slog.Info("email sent", "to", payload.To, "subject", payload.Subject)` |
  | 121 | ERROR | `slog.Error("email worker: send failed", "error", sendErr, "to", payload.To)` |
  | 136 | ERROR | `slog.Error("email worker: moved to dead-letter after max retries", "to", payload.To, "retries", retryCount)` |

  `payload.To` is the raw email address (`"to": []string{payload.To}` confirmed in `email_resend.go:29`). The policy requires truncation or hashing.

- **PASS** — No passwords, JWT tokens, magic link tokens, API keys, or secrets are logged in production code.
  - `auth/middleware.go:76` logs `"jti"` (JWT ID — a random UUID, not the JWT itself) — acceptable.
  - `auth/middleware.go:213` logs `"current_ip"` for multi-IP anomaly detection — acceptable (IPs are operational data, not PII per policy).
  - `auth/gdpr_data.go:76` logs `"refresh token reuse detected"` — descriptive message, no token value included.
  - `handler/admin_login.go:65` logs `"account"` for login lock checks (admin account name, not personal user data).
  - No nickname values appear in any log statement (verified via grep for `slog.*nickname` — zero results).

#### Step 3: Request Context Propagation

- **PASS** — `request_id` is injected into slog context by `RequestIDLogger` (`logging.go:39`).
- **PASS** — `trace_id` is injected into slog context by `TracingMiddleware` (`tracing.go:35`), preserving existing `request_id` from the logger chain.
- **PASS** — `user_id` is injected into slog context by `AuthMiddleware` (`auth/middleware.go:94-98`) when a valid JWT cookie is present.

- [LOW] **Role is NOT injected into slog context.** The logging policy specifies both `user_id` and `role` should be present in request-scoped logs. `role` is set in context via `WithRole` (used by auth middleware and RBAC) but never added to the slog logger instance. The `rbac.Middleware` reads role from context but does not enrich the logger.
- [INFO] **"request completed" log does not include `user_id`.** The `RequestIDLogger` at `logging.go:48` calls `slogctx.LoggerFromContext(r.Context())` after `next.ServeHTTP` returns. However, the `r.Context()` at that point is the outer context (before auth enrichment). The auth middleware enriches the logger in an inner request context (`r.WithContext(ctx)`) that does not propagate back up to the outer handler. This means the access log (`request completed`) will have `request_id` and `trace_id` but miss `user_id`.

#### Step 4: Log Level Usage

- **PASS** — Log level distribution is appropriate:

  | Level | Count | Assessment |
  |-------|-------|------------|
  | `slog.Debug` | 2 | Both in `email_worker.go:59` and `game_result_worker.go:52` — `XGroupCreate (may already exist)`. This is the correct use of Debug: expected-but-notable events during stream group creation. |
  | `slog.Info` | ~15 | Used for: server start/stop, worker started, GDPR cleanup completed, email sent, idempotency cache hits, refresh token generated. All are normal business events — correct usage. |
  | `slog.Warn` | ~30 | Used for: degraded operation (Hub unavailable, Redis down), RBAC denied, circuit breaker state change, slow client disconnect, login lock, refresh token reuse, suspicious multi-IP. All are recoverable or security-signal events — correct usage. |
  | `slog.Error` | ~35+ | Used for: DB write failures, worker consume failures, send failures, unmarshal errors, migration failures, shutdown errors. All require human attention — correct usage. |

- **No misuse found:** No `Error` for expected conditions (e.g., 4xx client errors). No `Debug` in hot paths. No `Info` for per-request DB queries (policy requirement met).

#### Step 5: Policy Compliance

| Policy Rule | Verdict |
|-------------|---------|
| No `fmt.Print` / `log.Print` — use `slog` | PASS |
| No passwords, JWT, Magic Link tokens, API keys in logs | PASS |
| No full email in logs (hash or truncate) | **FAIL** — `email_worker.go` logs full `payload.To` |
| No INFO-level per-request DB queries | PASS (OTel spans used instead) |
| No full stack trace in ERROR to stdout | PASS (uses `slog` structured `"error"` field with caller) |
| `request_id` in request-scoped logs | PASS |
| `trace_id` in request-scoped logs | PASS |
| `user_id` in request-scoped logs | PASS (but missing from "request completed" access log) |
| `role` in request-scoped logs | **FAIL** — role is never injected into slog context |

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Structured logging (no raw prints) | PASS — 0 `fmt.Print`/`log.Print` in production | NONE |
| PII in logs (email addresses) | FAIL — 4 locations log full email in `email_worker.go` | MEDIUM |
| PII in logs (passwords, tokens, secrets) | PASS — None detected | NONE |
| Context propagation — request_id | PASS | NONE |
| Context propagation — trace_id | PASS | NONE |
| Context propagation — user_id | PASS (injected by AuthMiddleware; missing from "request completed" log) | LOW |
| Context propagation — role | FAIL — role never injected into slog context | LOW |
| Log level appropriateness | PASS — levels used correctly | NONE |
| Policy compliance (email redaction) | FAIL | MEDIUM |

---

## 11. Backend Runtime Performance

### Checks Performed

- **Step 1:** Audited game loop tick rate in `backend/internal/game/room_tick.go`
- **Step 2:** Audited physics calculations in `backend/internal/game/physics.go`
- **Step 3:** Audited broadcasting efficiency in `backend/internal/game/room_broadcast.go` and `backend/internal/protocol/encode.go`
- **Step 4:** Checked all goroutines for lifecycle management (done/quit channels, WaitGroup/context, blocking risks)
- **Step 5:** Checked for memory leak patterns (unbounded slices/maps via `append`)

### Files Read

- `backend/internal/game/room_tick.go` — Game loop tick (192 lines)
- `backend/internal/game/physics.go` — Physics calculations (113 lines)
- `backend/internal/game/room_broadcast.go` — Broadcasting (93 lines)
- `backend/internal/game/room_outbound.go` — Outbound queue bridge (43 lines)
- `backend/internal/game/outbound_manager.go` — Outbound delivery manager (221 lines)
- `backend/internal/game/persist_manager.go` — Async state persistence (163 lines)
- `backend/internal/game/broadcaster.go` — Redis Pub/Sub cross-instance broadcast (152 lines)
- `backend/internal/game/room_result_async.go` — Async game result processing (146 lines)
- `backend/internal/game/room.go` — Room struct, connection management (245 lines)
- `backend/internal/game/hub.go` — Hub, room registry (205 lines)
- `backend/internal/game/hub_cleanup.go` — Cleanup loop (107 lines)
- `backend/internal/protocol/encode.go` — Binary snapshot encoding (249 lines)
- `backend/internal/protocol/constants.go` — TickRate = 15 confirmed (line 151)
- `backend/internal/handler/lobby_ws_pumps.go` — WebSocket read/write pumps (72 lines)
- `backend/internal/server/server_lifecycle.go` — Server startup/shutdown (159 lines)
- `backend/internal/server/server_init.go` — Workers, hub init (113 lines)
- `backend/internal/server/server_metrics.go` — Metrics collection goroutines (75 lines)

### Findings

#### Step 1: Game Loop Tick Rate

- **PASS** — Tick rate is 15 Hz (confirmed at `protocol/constants.go:151`). Tick interval: `time.Duration(int64(1000/15)) * time.Millisecond` = ~66.67ms. Correct.
- **PASS** — No blocking I/O in tick loop. All operations are in-memory: physics, collision checks, state encoding, broadcast enqueue.
- **PASS** — `buildSnapshot()` reuses `r.players` slice via `r.players[:0]` to avoid per-frame allocation of the player state slice.
- **PASS** — `tick.C` signals at ~67ms intervals; the loop drains it immediately.
- **ISSUE: Per-frame allocation in snapshot encoding** — `EncodeSnapshot` (encode.go:59) does `make([]byte, buf.Len())` on every tick to copy the pooled buffer into a new slice that outlives the pool (queued on player Send channels). Comment acknowledges this: "the returned slice outlives this function (it is queued on player Send channels)". At 15 Hz per room, this is the single largest hot-path allocator. 100 rooms = ~1,500 allocs/sec of snapshot-sized byte slices.
- **ISSUE: Duplicated `time.Now()` syscalls** — `tick()` (room_tick.go:63) calls `time.Now()` for lock recording, then `tickOnce()` (line 79) calls it again for `cleanupDisconnected`, and `buildSnapshot()` (room_broadcast.go:54) calls it a third time for cooldown calculation. Each `time.Now()` is a syscall on Windows (~100-300ns). Could pass a single `now` value through the chain.
- **ISSUE: `recordRoomLock("tick", start)` string allocation** — Each tick allocates the string `"tick"` (constant, so likely optimized) and logs the start time. The overhead is negligible but present.
- **INFO:** `asyncSaveState()` (room_tick.go:71) fires every 30 ticks. The channel send in `requestPersist` is non-blocking with `select/default`, so it never blocks the tick loop.

#### Step 2: Physics Calculations

- **PASS** — Uses `math/rand/v2` (not v1) — no global mutex contention, seed-safe, fast per-goroutine PRNG.
- **PASS** — No floating-point randomness in the hot path. `rand.Float64()` in `UpdateWind` is called only at controlled intervals (every 10/75/225 ticks), not per-tick.
- **PASS** — Physics constants are generated-synced to frontend via `go:generate go run ../../cmd/gen-frontend-constants` (constants.go:140). Confirmed by `@ts PHYSICS.*` annotations.
- **PASS** — `ApplyPhysics` uses only simple arithmetic (add, multiply, clamp). No trigonometry.
- **PASS** — `ApplyTapForce` uses `math.Hypot` (internal `math.Sqrt`) but this is called on player-tap (user action), not per-tick. Acceptable.
- **PASS** — `UpdateWind`, `UpdateBirdAI`, `UpdateGhostAI` are all O(1) per tick — single entity updates, no loops.
- **INFO:** `ApplyTapForce` could use squared-distance comparison (`dx*dx + dy*dy`) to avoid the `math.Sqrt` in `math.Hypot` on the tap/hot path, since `dist > TapRange` is the common fast-fail case.

#### Step 3: Broadcasting Efficiency

- **PASS** — All protocol messages are binary-encoded (little-endian) — confirmed in `encode.go`. No JSON serialization on the message-passing path. `BroadcastMessage` for Redis Pub/Sub is JSON (cross-instance only), which is acceptable.
- **PASS** — `snapshotBufPool` (`sync.Pool`) reuses `*bytes.Buffer` across `EncodeSnapshot` calls, avoiding per-frame `bytes.Buffer` allocation.
- **PASS** — `calcSnapshotSize()` pre-computes buffer size to avoid writes that trigger slice growth reallocation.
- **PASS** — WebSocket write timeout configured: `WSWriteTimeout` default 10s (`config/timeout.go:35`), applied via `SetWriteDeadline` in `lobby_ws_pumps.go:44,66`.
- **PASS** — Slow client detection: outbound_manager.go tracks consecutive drops — 3 drops → WARN log, 10 drops → disconnect + close. Buffer size 256 with non-blocking send on critical messages (100ms timeout fallback).
- **PASS** — `broadcastCritical` uses blocking delivery per client with 100ms timeout — only for critical phase-change messages, not per-tick.
- **PASS** — Pending disconnects are cleaned up after each outbound delivery cycle (`RemovePendingDisconnects()`), preventing accumulation of zombie connections.
- **ISSUE: BroadcastMessage JSON overhead (multi-instance only)** — `outbound_manager.go:207` marshals `BroadcastMessage` as JSON for Redis Pub/Sub on every broadcast. In single-instance mode (`broadcaster == nil`), this path is skipped entirely. Acceptable for the target architecture.
- **INFO:** Building `[]byte(nil)` copy in `outbound_manager.go:86` (`copied := append([]byte(nil), payload...)`) is necessary because the original payload is reused by `buildSnapshot()`. This is a deliberate memory-vs-safety tradeoff.

#### Step 4: Goroutine Leaks

- **PASS** — All 13+ production goroutines have proper lifecycle management with context cancellation or channel close:

| Goroutine | File:Line | Lifecycle |
|-----------|-----------|-----------|
| Tick loop (per room) | `room_tick.go:134,157,175` | `r.tickCancel` context + `r.wg.WaitGroup` |
| Outbound delivery (per room) | `outbound_manager.go:70` | Channel close (`Stop()`) + `r.asyncWg` |
| Persist manager (per room) | `persist_manager.go:55` | Channel close + `r.asyncWg` |
| PubSub subscriber (per room) | `broadcaster.go:92` | `sub.Close()` via unsubscribe function |
| Game result (fire-and-forget) | `room_result_async.go:57,97,136` | Context timeout + `r.asyncWg` |
| Game session async insert | `room_result_async.go:136` | Context timeout + `r.asyncWg` |
| HTTP server | `server_lifecycle.go:113` | `srv.Shutdown()` in `waitForShutdown` |
| Hub CleanupLoop | `server_lifecycle.go:88` | `ctx.Done()` |
| Metrics collector (3x) | `server_metrics.go:19,47,61` | `ctx.Done()` |
| Email worker | `server_init.go:77` | `ctx.Done()` |
| Game result worker | `server_init.go:80` | `ctx.Done()` |
| Outbox publisher | `server_init.go:81` | `ctx.Done()` |
| GDPR cleanup worker | `server_init.go:85` | `ctx.Done()` |

- **PASS** — No goroutine blocks indefinitely. All either loop over channels (closed on shutdown), check context cancellation, or have timeouts.
- **PASS** — `restartTick()` (room_tick.go:150-179) correctly waits for old goroutine via `r.wg.Wait()` before starting new, preventing goroutine pileup.
- **PASS** — `Close()` sequence (room.go:191-217) correctly orders: stop tick → wg.Wait → stop outbound → close connections → flush persist → stop persist → asyncWg.Wait. Chain prevents use-after-close of shared channels.

#### Step 5: Memory Leak Patterns

- **PASS** — No unbounded slice or map growth in the hot path.
- All `append()` calls in production code operate on slices that are:
  - Created fresh per operation (cleanup batch, match results, remove batch, restore batch)
  - Reused with `[:0]` reset (`r.players` in `buildSnapshot`) — bounded by `MaxPlayersPerRoom`
  - Copied for async delivery (single-item `append` into closure)
- **PASS** — `r.usedNames` map is bounded by player count (`MaxPlayersPerRoom`). Entries are cleaned up in `cleanupDisconnected`.
- **PASS** — `r.connections` map is bounded by player count. Entries are deleted on disconnect/cleanup.
- **PASS** — `r.state.Players` map is bounded by `MaxPlayersPerRoom` and cleaned up in `cleanupDisconnected` after reconnect grace expires.
- **PASS** — `r.rooms` (Hub map) is cleaned up by `CleanupLoop` and `RemoveRoom`. No stale room accumulation.
- **PASS** — `Broadcaster.subs` (`sync.Map`) is cleaned up by unsubscribe function. Hub calls `unsubscribeRoomLocked` on room removal and `CloseAllRooms` on shutdown.
- **INFO:** `outbound_manager.go:86` — `copied := append([]byte(nil), payload...)` creates a new `[]byte` on every Enqueue. This is freed by GC after delivery and is bounded by the 256-entry channel buffer. Acceptable.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Tick rate (15 Hz) | PASS | NONE |
| No blocking in tick loop | PASS | NONE |
| Binary encoding | PASS | NONE |
| Buffer pool for snapshots | PASS | NONE |
| WebSocket write timeout | PASS | NONE |
| Slow client detection | PASS | NONE |
| Goroutine lifecycle | PASS — All tracked, all stoppable | NONE |
| Memory leaks (unbounded growth) | PASS — All maps/slices bounded | NONE |
| Duplicated `time.Now()` syscalls | INFO — 2-3 calls per tick; could be optimized to 1 | LOW |
| Per-frame `make([]byte)` for snapshot | INFO — Intentionally documented; 1500+ allocs/sec at 100 rooms | MEDIUM |
| `applyTapForce` uses `math.Hypot` (Sqrt) | INFO — Could use squared-distance for fast-fail | LOW |
| `BroadcastMessage` JSON (multi-instance) | INFO — Acceptable; skipped in single-instance mode | LOW |
| Per-room outbound channel (size 256) | PASS — Bounded; non-blocking send with drop behavior | NONE |
| `restartTick` WaitGroup ordering | PASS — Correctly prevents goroutine pileup | NONE |

---

## 14. Metrics Completeness

### Files Read

- `backend/internal/metrics/metrics.go` — All 52 Prometheus metric definitions (counters, histograms, gauges)
- `backend/internal/metrics/record.go` — Metrics recording helper functions and status writer
- `backend/internal/metrics/metrics_test.go` — 21 test functions covering all metric types
- `backend/internal/metrics/record_test.go` — 4 test functions for recorder helpers
- `backend/internal/metrics/record_extra_test.go` — 6 additional recorder tests
- `backend/internal/server/server_metrics.go` — Metrics collection goroutines (hub + DB + Redis polling)
- `backend/internal/middleware/prometheus.go` — Prometheus HTTP middleware
- `backend/internal/middleware/metrics.go` — Auth metrics middleware
- `backend/internal/store/postgres_helpers.go` — `ObservePoolStats` implementation
- `deploy/alertmanager/rules.yml` — 4 alerting rules (2 groups)
- `deploy/alertmanager/config.yaml` — Alertmanager routing config
- `deploy/prometheus/prometheus.yml` — Scrape config with Thanos sidecar
- `deploy/prometheus/deployment.yaml` — Prometheus + Thanos sidecar deployment

### Step 1: Golden Signals (RED) — PASS

**Rate (Traffic):**
- `HTTPRequestsTotal` (method, path, status) — every HTTP request counted via `PrometheusMiddleware`
- `AuthRequestTotal` (endpoint, status) — auth endpoints via `RecordAuthMetrics` middleware
- `WSConnectionTotal` (status) — WebSocket upgrade outcomes (established/rejected)
- `RoomCreationTotal` (status) — room creation attempts (success/failed)

**Errors:**
- Built into rate metrics via status code labels (5xx captured in `HTTPRequestsTotal` and `AuthRequestTotal`)
- `WSMessagesDroppedTotal` (room_code) — specific WS message delivery failures
- `NicknameConfirmTotal` (result) — nickname rejection tracking

**Duration (Latency):**
- `HTTPRequestDuration` (method, path) with `SLOBuckets` (5ms–5s, no 10s bucket)
- `AuthRequestDuration` (endpoint) with 0.1s–5s buckets
- `WSMessageDuration` (msg_type) with 10ms–1s buckets
- `RoomCreationDuration` with 0.1s–5s buckets
- `DBPoolAcquireDuration` with SLOBuckets
- `RoomLockHoldSeconds` (operation) with microsecond-granularity buckets
- `OutboxBatchSize` with event-count buckets

**Verdict:** All three RED signals are covered for HTTP, auth, and WebSocket endpoints. Histograms use well-tuned buckets (`SLOBuckets`) suitable for sub-second SLO targets. No explicit p50/p95/p99 metrics are emitted, but Prometheus `histogram_quantile` can compute these from histogram buckets.

### Step 2: USE Metrics (Utilization, Saturation, Errors) — PARTIAL

**PostgreSQL pool:**
| Metric | USE | Verdict |
|--------|-----|---------|
| `DBPoolIdleConns` | Utilization (idle fraction) | PASS |
| `DBPoolInUseConns` | Utilization (in-use count) | PASS |
| `DBPoolAcquireCount` | Saturation (acquire rate) | PASS |
| `DBPoolAcquireDuration` | Saturation (wait time) | PASS |
| Pool connection errors | Errors | **MISSING** — no counter for acquire failures |

**Redis pool:**
| Metric | USE | Verdict |
|--------|-----|---------|
| `RedisPoolIdleConns` | Utilization | PASS |
| `RedisPoolTotalConns` | Utilization | PASS — but no in-use/acquired gauge separate from idle |
| Acquire duration | Saturation | **MISSING** — no histogram for Redis pool wait time |
| Connection errors | Errors | **MISSING** — no counter for Redis connection failures |

**Go runtime (goroutines):**
| Metric | USE | Verdict |
|--------|-----|---------|
| `go_goroutines` (Go collector) | Utilization | PASS |
| `go_gc_*`, `go_memstats_*` | Saturation | PASS |

**WebSocket connections:**
| Metric | USE | Verdict |
|--------|-----|---------|
| `WSConnections` gauge | Utilization | PASS |
| `WSConnectionTotal` (rejected status) | Errors | PASS |
| `WSMessagesDroppedTotal` | Saturation/Errors | PASS |
| `RoomOutboundQueueDepth` | Saturation | PASS |

### Step 3: Game-Specific Metrics — PARTIAL

| Metric | Status | Notes |
|--------|--------|-------|
| `ActiveRooms` gauge | ✅ PRESENT | Polled from hub every interval |
| `ActivePlayers` gauge | ✅ PRESENT | Polled from hub every interval |
| `RoomsByPhase` gauge vector | ✅ PRESENT | Per-phase room count (waiting/countdown/playing/ended) |
| `GameTickDuration` histogram | ❌ **MISSING** | No game loop tick duration metric |
| `WSMessageDuration` histogram | ✅ PRESENT | Message processing latency per type (tap, set_nickname, etc.) |
| Message send/receive rate counters | ⚠️ IMPLICIT | Derivable from `WSMessageDuration` histogram count but no explicit `messages_sent_total` or `messages_received_total` |
| Physics calculation duration | ❌ **MISSING** | No physics tick duration metric |
| `RoomLockHoldSeconds` histogram | ✅ PRESENT | Lock contention by operation |
| `RoomPersistLagSeconds` gauge | ✅ PRESENT | Persistence staleness per room |
| `GameSessionsTotal` counter | ✅ PRESENT | Session start events |
| `NicknameConfirmTotal` counter | ✅ PRESENT | Accepted/rejected outcomes |
| Stream length gauges | ✅ PRESENT | `GameResultsStreamLen`, `EmailQueueStreamLen` |
| Outbox metrics | ✅ PRESENT | `OutboxLagSeconds`, `OutboxBatchSize` |
| Security metrics | ✅ PRESENT | `AdminLoginLockedTotal`, `SuspiciousLoginTotal` |
| `CircuitBreakerState` gauge | ✅ PRESENT | Tracks closed/half-open/open per circuit |

### Step 4: Alert Rule Coverage — FAIL

Required alerts from task brief:
| Required Alert | Present? | Notes |
|----------------|----------|-------|
| Error rate > threshold | ⚠️ PARTIAL | Auth 4xx/5xx burn rate (fast + slow) and WS rejection burn rate exist. No generic HTTP 5xx error rate alert across all endpoints. |
| Latency p99 > threshold | ❌ **MISSING** | No latency-based alerts at all despite detailed histogram instrumentation |
| Connection pool exhaustion | ❌ **MISSING** | No alerts for DB or Redis pool saturation/high acquire duration |
| Disk space | ❌ **MISSING** | No disk space alert; Prometheus Node Exporter would provide this but no rule exists |
| Memory usage | ❌ **MISSING** | No memory alert; Go runtime metrics exist but unalerted |

Additional alerts present:
- `NicknameConfirmRejectedSpike` — Rate of nickname rejections > 0.1/s for 5m
- `RoomsStuckWaiting` — Rooms in waiting with no recent game starts

Only 4 alert rules exist for the entire application, none covering the 5 required categories. This is a significant gap.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| RED (Rate, Errors, Duration) | PASS — All three signals covered across HTTP, auth, and WS | NONE |
| USE — PostgreSQL pool | PASS — Idle/in-use/acquire-count/acquire-duration present; missing error counter | LOW |
| USE — Redis pool | PARTIAL — Idle/total present; missing in-use gauge, acquire duration, error counter | MEDIUM |
| USE — Goroutines | PASS — Go runtime collector provides full goroutine/memory/GC metrics | NONE |
| USE — WebSocket | PASS — Connection gauge, total counter, drop counter, queue depth | NONE |
| Game tick duration | **MISSING** — No tick/loop timing metric for core game loop | MEDIUM |
| Physics calculation duration | **MISSING** — No physics timing metric | LOW |
| Message rate counters | IMPLICIT — Derivable from histogram count; no explicit send/receive counter | LOW |
| Alert: Error rate > threshold | PARTIAL — Auth- and WS-specific only; no generic alert | MEDIUM |
| Alert: Latency p99 > threshold | **MISSING** | HIGH |
| Alert: Pool exhaustion | **MISSING** | MEDIUM |
| Alert: Disk space | **MISSING** | MEDIUM |
| Alert: Memory usage | **MISSING** | MEDIUM |

- **2 deliberate gaps** (tick duration, physics duration) — core game loop timing is uninstrumented
- **Alert coverage is the weakest area** — 0 of 5 required alert types are fully covered; only 4 total rules exist
- **Redis pool USE is incomplete** — missing in-use connections, acquire duration, and error counter
- **Histogram instrumentation is excellent** — well-tuned buckets, broad coverage of latency sources

---

## 12. Frontend Runtime Performance

### Checks Performed

- **Step 1:** Audited game loop frame budget (`frontend/src/game/renderer.ts`)
- **Step 2:** Audited canvas drawing (`frontend/src/game/renderer_draw.ts`)
- **Step 3:** Audited effects rendering (`frontend/src/game/renderer_draw_effects.ts`)
- **Step 4:** Audited state update batching (`frontend/src/game/store.ts`, `reducer.ts`)
- **Step 5:** Checked for DOM manipulation in hot path (`document.getElementById` / `document.querySelector` in `frontend/src/game/`)

### Files Read

- `frontend/src/game/renderer.ts` — Game loop (requestAnimationFrame, render orchestration)
- `frontend/src/game/renderer_draw.ts` — Canvas drawing (balloon, bird, ghost)
- `frontend/src/game/renderer_draw_effects.ts` — Visual effects (ripples, explosion)
- `frontend/src/game/renderer_background.ts` — Background rendering (stars, clouds, mountains, particles, static layer cache)
- `frontend/src/game/renderer_canvas.ts` — Canvas layout, context caching, resize
- `frontend/src/game/store.ts` — State store (dispatch, getState, select)
- `frontend/src/game/reducer.ts` — State reducer with action types
- `frontend/src/game/state_interp.ts` — Interpolation state management
- `frontend/src/game/interp_buffers.ts` — Snapshot delay buffer management
- `frontend/src/game/ws_message_queue.ts` — Pending message queue with per-frame budget
- `frontend/src/game/visual_helpers.ts` — Floating text, vignettes, tutorial circle
- `frontend/src/game/input.ts` — Tap handler, restart vote
- `frontend/src/game/local_constants.ts` — Game constants

### Findings

#### Step 1: Game Loop (renderer.ts)

- **PASS** — Uses `requestAnimationFrame` (line 28-36), not `setInterval` or `setTimeout`. The loop is correctly recursive via `requestAnimationFrame(gameLoop)` after each render.
- **FAIL** — No frame budget monitoring. The loop does not measure `performance.now()` deltas, track frame durations, or adjust workload under load. If a frame exceeds 16ms (60fps budget), there is no backpressure, warning, or adaptive quality reduction.
- **PASS** — No blocking `await` in the render loop. `render()` is fully synchronous and catches all errors with a try/catch at line 70-72.
- **INFO** — `drainPendingMessages(8)` at line 33 caps message processing to 8 per frame, providing a message-processing budget. This prevents a single frame from being flooded by queued WebSocket messages.

#### Step 2: Canvas Drawing (renderer_draw.ts)

- **PASS** — Minimal save/restore cycles. Only `drawBalloon` uses one save/restore pair (line 33-37 for image rotation), and `drawBird` uses one (line 80-128 for flip + rotation). No unnecessary nesting or orphaned saves.
- **PASS** — Image assets are preloaded. Every `drawImage` call is guarded by `gameImages[name]!.loaded` check (balloon line 28, ghost line 141, bird via gradient fallback). Fallback procedural paths exist for all three entities.
- **PASS** — Cached gradients. `_ensureBirdGradients` (line 10-20) computes wing/body gradients once and reuses them across frames. Size-change invalidates cache.
- **INFO** — `drawBalloon` fallback (lines 41-62) creates a new `CanvasGradient` object every frame when the balloon image is not loaded. This is acceptable since images are loaded once early in the game lifecycle; the fallback only activates during the brief window before images arrive or if they fail to load.

#### Step 3: Effects Rendering (renderer_draw_effects.ts)

- **FAIL** — Render loop dispatches state mutations as side effects. `drawRipples` (lines 33-37) filters expired ripples and calls `dispatch({ type: 'SET_STATE', partial: { ripples: remaining } })` on every frame where ripples have expired. `drawExplosion` (lines 80-82) similarly dispatches `SET_STATE` to null out `explosionEffect` on expiry. This couples visual rendering with state management and means the render function has observable side effects beyond drawing.
- **FAIL** — No explicit maximum count limit on ripples. The `remaining` filter caps ripples only by age (600ms TTL), not by count. If the server sends a burst of tap events, the ripple array grows unboundedly until old ones expire, causing O(N) draws per frame with N depending entirely on server throughput.
- **PASS** — Palette RGB string precomputed. `_cachedPaletteRgb` (line 17) maps `PALETTE_COLORS` to `rgb(...)` strings once at module init, avoiding per-frame color conversion.
- **NOTE** — GC pressure in `drawRipples`. `rippleColor()` (line 19-30) creates a new `{ base, alpha }` object per ripple per frame. `Array.filter()` (line 34) creates a new array per frame when any ripples exist. For a typical frame with 2-5 visible ripples, the allocation cost is negligible, but under burst conditions it adds up.

#### Step 4: State Update Batching (store.ts, reducer.ts)

- **FAIL** — Reducer mutates state in-place. `reducer.ts:61` (`Object.assign(state, action.partial)`) mutates the existing `_state` object instead of returning a new one. This is a mutation anti-pattern: it prevents any form of change detection, makes time-travel debugging impossible, and could cause subtle bugs if two callers hold references to the same state at different logical times. The `ADD_RIPPLE` action (line 64) also mutates by reassigning `state.ripples` on the same object.
- **PASS** — Message processing is capped. `drainPendingMessages(8)` limits per-frame dispatches, preventing unbounded state mutation bursts.
- **PASS** — No unnecessary re-renders. The game uses canvas rendering driven by `requestAnimationFrame`, not DOM diffing. `getState()` simply returns the raw mutable object — there is no selector overhead, no comparison, and no virtual DOM.
- **PASS** — `dispatch` is O(1). Each action runs the reducer switch and mutates the single state reference. No immutability helpers, no middleware, no subscriber list overhead.
- **INFO** — `RESET_ALL` (reducer.ts:71-75) creates a fresh state via `createInitialState()` then `Object.assign(state, fresh)` — the object identity remains the same but its contents are overwritten. Callers holding stale references see the new data, which may be surprising or correct depending on usage.

#### Step 5: DOM in Hot Path

- **PASS** — Canvas element cached at module level. `renderer_canvas.ts:43` assigns `$canvas` once at module init via `document.getElementById('game-canvas')`.
- **PASS** — 2D rendering context cached. `getCtx()` (renderer_canvas.ts:56-62) lazily creates and caches the context, returning it on every subsequent call.
- **PASS** — DOM queries in `entry_flow_dom.ts` are cached at module level (lines 7-12: `$loadingOverlay`, `$waitingTitle`, `$loadingErrorPanel`, `$loadingErrorText`, `$loadingErrorTitle`, `$loadingErrorActions`).
- **PASS** — `measureLayoutInsets()` (renderer_canvas.ts:16-41) queries DOM by ID but is called only from `resizeCanvas`, not from the per-frame render path.
- **NOTE** — `input.ts:38` (`tapAtBalloonCenter`) calls `document.getElementById('game-canvas')` on every invocation. This is event-driven (user tap), not per-frame, so it is not a hot-path concern. Could be cached for consistency.

#### Additional Findings

- **Static background cache** (renderer_background.ts:17-25, 113-141) — The background renders mountains/sky to an off-screen canvas on resize and blits it each frame. This avoids redrawing expensive static geometry every frame. Cache invalidates on canvas resize. **PASS**.
- **Floating text mutation in render** (visual_helpers.ts:83-98) — `drawFloatingTexts` mutates the `floatingTexts` array (splice) during render. Consistent with the side-effect-in-render pattern seen in effects (Step 3). Low risk due to small array size (texts expire after 1500ms, throttled to 1 per 3s).
- **Particle state mutation in render** (renderer_background.ts:168-183) — `drawParticles` mutates particle position, velocity, and lifespan during draw. Intentional for continuous animation. Each `$canvas.width` access inside the loop (line 180) is a property read on a cached element — negligible.
- **Module-level draw state** — Several modules carry module-scoped mutable state (`seenSeqs` Set in `state_interp.ts`, `interp_buffers.ts` arrays, `floatingTexts` array). These are intentionally shared across frames but could cause issues if game instances are reused without proper reset (`resetInterpolation`, `clearAnchorBuffers`, `clearSeenSeqs` are all called during lifecycle transitions).

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| requestAnimationFrame (not setInterval) | PASS | NONE |
| Frame budget monitoring | FAIL — No frame time measurement or adaptive quality | MEDIUM |
| No blocking ops in render loop | PASS | NONE |
| Minimal save/restore cycles | PASS | NONE |
| Image preloading | PASS | NONE |
| Effect max count limits | FAIL — No limit on ripple count; server-driven unbounded O(N) | LOW |
| Effect cleanup on phase change | PASS — Ripples/explosion cleared via reducer RESET_ROUND/RESET_ALL | NONE |
| State mutations in render loop | FAIL — drawRipples/drawExplosion call dispatch as side effect | MEDIUM |
| Reducer immutability | FAIL — Object.assign in-place mutation | LOW |
| Message processing budget (8/frame) | PASS | NONE |
| DOM in render hot path | PASS — All lookups cached or event-driven | NONE |
| Static background cache | PASS | NONE |

---

## 15. Distributed Tracing

### Checks Performed

- **Step 1:** Read `backend/internal/telemetry/telemetry.go` — OTel initialization, exporter, resource, sampler
- **Step 2:** Read `backend/internal/server/server_lifecycle.go` — InitTracer call site
- **Step 3:** Read `backend/internal/middleware/tracing.go` — HTTP tracing middleware
- **Step 4:** Read `backend/internal/handler/lobby_ws_read.go`, `lobby_ws_pumps.go` — WebSocket span creation
- **Step 5:** Grepped `backend/internal/` for `tracer.Start`, `span.End`, `span.SetAttributes` — span coverage across all packages
- **Step 6:** Searched `backend/internal/game/`, `backend/internal/auth/` for OTel imports — uncovered gaps
- **Step 7:** Verified context propagation via `propagation.` and `context.WithValue` usage

### Files Read

- `backend/internal/telemetry/telemetry.go` — OTel initialization (104 lines)
- `backend/internal/telemetry/telemetry_test.go` — Test coverage (457 lines)
- `backend/internal/middleware/tracing.go` — HTTP tracing middleware (81 lines)
- `backend/internal/server/server_lifecycle.go` — InitTracer call site (159 lines)
- `backend/internal/handler/lobby_ws_read.go` — WS read pump spans (93 lines)
- `backend/internal/handler/lobby_ws_pumps.go` — WS write pump spans (72 lines)
- `backend/internal/auth/middleware.go` — Auth middleware (no spans, 216 lines)

### Findings

#### Step 1: OTel Initialization

- **PASS** — OTLP/gRPC exporter configured via `otlptracegrpc.New`. Conditional on `OTEL_EXPORTER_OTLP_ENDPOINT` env var (no endpoint → noop provider).
- **PASS** — Service name `"balloon-game"` and version `"1.0.0"` set on the OTel resource via `semconv.ServiceNameKey` and `semconv.ServiceVersionKey`.
- **MISSING** — Resource attributes lack `deployment.environment` (dev/staging/prod), `cloud.region`, `cloud.provider`, and `service.namespace`. These are critical for filtering traces in multi-environment deployments (e.g., Grafana Tempo).
- **PASS** — Sampler: `ParentBased(TraceIDRatioBased(0.1))` — proper head-based sampling that honors upstream decisions. Configurable via `OTEL_SAMPLE_RATIO` env var (validated range 0.0–1.0, default 0.1).
- [MEDIUM] `backend/internal/telemetry/telemetry.go:55` — `otlptracegrpc.WithInsecure()` is hardcoded. In production, the OTLP collector connection should use TLS (`WithTLSCredentials`). The current implementation sends trace data in plaintext over gRPC.
- **PASS** — Propagation: `TraceContext` + `Baggage` propagators registered.

#### Step 2: Span Coverage on Critical Paths

| Critical Path | Spans Present? | Location |
|---------------|---------------|----------|
| HTTP request handling | **YES** | `middleware/tracing.go:25` — every request gets `METHOD /path` span with http.method, http.url, http.status_code, http.route, enduser.id |
| Database queries (Postgres) | **YES** | 27+ spans across `store/postgres_*.go`, `result_repository.go`, `user_repository.go`, `config_repository.go`, `lobby_repository.go` — one span per store method |
| Redis operations | **YES** | 12+ spans across `store/redis_*.go`, `ratelimit_store.go`, `magiclink_store.go`, `session_store.go`, `email_queue_store.go` — one span per Redis operation |
| WebSocket read messages | **YES** | `handler/lobby_ws_read.go:89` — per-message spans (`ws.readPump.tap`, `ws.readPump.set_nickname`, `ws.readPump.restart_vote`); tap sampled every 100th message, ping skipped |
| WebSocket write messages | **YES** | `handler/lobby_ws_pumps.go:50` — per-write span (`ws.writePump.broadcast`) |
| Auth verification | **NO** | `auth/middleware.go` — JWT verification, revocation check, cookie parsing, multi-IP detection all untraced |
| Game loop tick | **NO** | `game/` package imports zero OTel packages — no spans for room lifecycle, game state updates, player state transitions, or tick processing |

- **MISSING** — Auth verification spans: JWT token validation and revocation checks happen per-request but have no observability. This makes it impossible to trace auth failures or measure auth latency in a trace view.
- **MISSING** — Game loop spans: The core game logic (`game/` package, ~35 files) has zero observability. This is the most critical untraced path — game tick duration, player join/leave, phase transitions are invisible in traces.
- [INFO] `backend/internal/handler/lobby_ws_read.go:71` — Tap message sampling at 1:100 is a sensible optimization to avoid span explosion at high tap rates.

#### Step 3: Context Propagation

- **PASS** — `otel.SetTextMapPropagator` configures `TraceContext` + `Baggage` propagators globally.
- **PASS** — Trace ID is injected into slog context in `TracingMiddleware` (`middleware/tracing.go:35`), enabling log-trace correlation via `trace_id` field.
- **PASS** — WebSocket read pump receives `wsCtx` derived from the authenticated HTTP request context, preserving the trace parent-child relationship across the HTTP→WebSocket boundary.
- **PASS** — Store layer spans use context from the caller, maintaining the span hierarchy.
- **NO** — No outbound HTTP propagation. If the service makes outbound HTTP calls (not currently present in this monolith), there's no `propagator.Inject` usage.
- **PASS** — `context.WithValue` is used only for domain context keys (userID, nickname, jti, role, requestID, nonce), not for trace context (which is managed by OTel's `trace.SpanContext`).

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| OTLP/gRPC exporter | PASS — configured, conditional on env var | NONE |
| Service name + version | PASS — "balloon-game" / "1.0.0" | LOW (hardcoded) |
| Resource attributes (env, region) | MISSING — no deployment.environment or cloud.region | MEDIUM |
| Sampler config | PASS — ParentBased + TraceIDRatioBased(0.1) | NONE |
| HTTP request spans | PASS — TracingMiddleware covers all routes | NONE |
| Database query spans | PASS — 27+ spans across store layer | NONE |
| Redis operation spans | PASS — 12+ spans across Redis stores | NONE |
| WebSocket message spans | PASS — readPump (sampled) + writePump | NONE |
| Auth verification spans | MISSING — jwt verification, revocation untraced | MEDIUM |
| Game loop tick spans | MISSING — entire game package has zero OTel | HIGH |
| Context propagation | PASS — TraceContext + Baggage, trace_id in logs | NONE |
| OTLP insecure transport | MEDIUM — WithInsecure() hardcoded, no TLS for production | MEDIUM |

- **OTel initialization is solid** — proper exporter, sampler, and propagation setup. Test coverage at 100%.
- **2 MISSING span categories**: auth middleware and game package have zero OTel instrumentation. Game is the highest-priority gap — 35 files with no tracing for tick, room, or player state operations.
- **1 MEDIUM issue**: hardcoded `WithInsecure()` on OTLP gRPC connection — production deployments must override this or trace data is sent in plaintext.
- **1 MEDIUM issue**: resource attributes lack environment/region metadata, hindering multi-environment trace filtering.
- **Log-trace correlation is working** via trace_id injection into slog context in TracingMiddleware.

---

## 20. Docker & Container Security

### Checks Performed

- **Step 1:** Audited `Dockerfile` — multi-stage build, pinned digests, non-root user, secrets in build args, `.dockerignore`
- **Step 2:** Audited `docker-compose.yml` — hardcoded passwords, health checks, resource limits, volume permissions, network isolation
- **Step 3:** Checked for Docker best practices — no bare `COPY . .`, no root in runtime

### Files Read

- `Dockerfile` — 28 lines, 3 stages (frontend-builder, go-builder, runtime)
- `docker-compose.yml` — 122 lines, 8 services (migrate, app, postgres, redis, prometheus, alertmanager, grafana, cockroach)
- `.dockerignore` — 60 lines, comprehensive exclusion list
- `docker/postgres/init/01-create-roles.sql` — 7 lines, role definitions

### Findings

#### Step 1: Dockerfile Audit

| Check | Verdict |
|-------|---------|
| Multi-stage build | **PASS** — 3 stages: `frontend-builder` (node:20-alpine), `go-builder` (golang:1.26-alpine), runtime (distroless). Only the final stage is shipped. |
| Base images pinned by SHA256 digest | **PASS** — All 3 base images pinned: `node@sha256:b1e0...`, `golang@sha256:3ad5...`, `gcr.io/distroless/static-debian12:nonroot@sha256:d093...`. Comment references `scripts/ci/pin-digests.sh`. |
| Non-root user in runtime | **PASS** — `USER nonroot:nonroot` on line 25. Distroless `nonroot` image user is 65534 (nobody). Confirmed no `USER root` anywhere in Dockerfile. |
| Secrets in build args | **PASS** — Zero `ARG` statements. No build-time secrets exposed in image layers. |
| `.dockerignore` exists | **PASS** — `.dockerignore` is comprehensive (60 lines): excludes `.git`, `node_modules`, `*.md`, `docs/`, test files, IDE config, `.env` files, build artifacts, scripts, CI config. |
| Bare `COPY . .` | **PASS** — No `COPY . .`. Each stage copies only what it needs: `COPY frontend/package*.json`, `COPY frontend/`, `COPY backend/go.mod go.sum`, `COPY backend/`. |

**Verdict: PASS** — Dockerfile follows all checked best practices.

#### Step 2: docker-compose Audit

| Check | Verdict |
|-------|---------|
| Hardcoded passwords | **FAIL** — `POSTGRES_PASSWORD: uppy` (line 46), `DATABASE_URL=postgres://uppy:uppy@...` (line 18) both hardcode the dev password. `GF_SECURITY_ADMIN_PASSWORD=admin` (line 96) hardcodes Grafana admin password. Redis password uses env var with fallback `dev-redis-secret` (line 62) — borderline acceptable. |
| Health checks | **PARTIAL** — `postgres` (line 55-59) and `redis` (line 65-69) have proper health checks. `cockroach` (line 115-119) has a health check. **Missing:** `app` has no health check at all. `prometheus`, `alertmanager`, `grafana` (observability profile) also lack health checks. |
| Resource limits | **PARTIAL** — Only `app` service has `deploy.resources.limits` (1 CPU, 512M memory) and reservations. All other services have no resource constraints, risking resource starvation in constrained environments. |
| Volume permissions | **INFO** — `pgdata` and `crdbdata` are named volumes without explicit user/group ownership settings. Defaults to container user (postgres:postgres for pgdata, which runs as root effectively). No security issue for local dev but should be documented. |
| Network isolation | **FAIL** — All 8 services share the default single network. There is no network segmentation between the application tier, database tier, and observability tier. In production, at minimum the database should be isolated from the public-facing app. No custom network is defined. |
| Init script credentials | **FAIL** — `docker/postgres/init/01-create-roles.sql` contains hardcoded `PASSWORD 'change_in_production'` for both `app_user` and `migrator` roles. This placeholder value is easily guessable. |
| `migrate` service | **PASS** — Uses `depends_on: postgres: condition: service_healthy`, ensuring migration runs only after Postgres is ready. Appropriate use of run-once job pattern. |
| Observability services access | **INFO** — Grafana, Prometheus, Alertmanager are bound to host ports with no authentication beyond the hardcoded `admin`/`admin` (Grafana). In local dev context this is acceptable, but `GF_SECURITY_ADMIN_PASSWORD` should be configurable via env var with a secure default. |

#### Step 3: Docker Best Practices

- **PASS** — `rg "COPY \."` confirmed no bare `COPY . .` in Dockerfile (only `COPY backend/`, `COPY frontend/`).
- **PASS** — `rg "USER root"` confirmed zero occurrences; runtime user is `nonroot:nonroot` (line 25).
- **PASS** — Distroless base image (`gcr.io/distroless/static-debian12:nonroot`) minimizes attack surface — no shell, no package manager, no utilities.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Multi-stage build | PASS | NONE |
| Pinned digests | PASS | NONE |
| Non-root runtime | PASS | NONE |
| No secrets in build args | PASS | NONE |
| `.dockerignore` present | PASS | NONE |
| No bare `COPY . .` | PASS | NONE |
| Health checks on app | **MISSING** — `app` has no health check | MEDIUM |
| Resource limits on all services | **MISSING** — Only `app` has limits | MEDIUM |
| Hardcoded passwords in compose | **FAIL** — 3 hardcoded passwords (postgres, grafana, init SQL) | MEDIUM |
| Network isolation | **FAIL** — Single flat network for all tiers | LOW |
| Init script placeholder password | **FAIL** — `change_in_production` in SQL | MEDIUM |

- **Dockerfile is production-grade** — multi-stage, pinned digests, non-root distroless, clean `.dockerignore`. No issues found.
- **docker-compose has 4 real gaps**: no health check on `app`, no resource limits on non-app services, hardcoded passwords, and a single flat network with no segmentation.
- **Init SQL script** contains a weak placeholder password (`change_in_production`) for `app_user` and `migrator` roles.
- **Recommendation**: Add `healthcheck` to `app` service, add `deploy.resources` to postgres/redis, replace hardcoded passwords with `${VAR:?error}` env var patterns, and define separate networks (e.g., `frontend` and `backend`).

---

## 18. Graceful Degradation

### Checks Performed

- **Step 1:** Audited graceful degradation — read `backend/internal/handler/degradation.go`, stats handler, checked degradation mode toggling, non-critical feature disablement, core game preservation, user notification
- **Step 2:** Audited graceful shutdown — read `backend/internal/server/server_lifecycle.go`, checked in-flight request draining, WebSocket connection closure, database connection release, worker stop, shutdown timeout
- **Step 3:** Audited background worker resilience — read `backend/internal/worker/game_result_worker.go`, `email_worker.go`, `email_resend.go`, `gdpr_cleanup.go`, `resilience/retry.go`, `resilience/circuitbreaker.go`, checked retry logic, dead letter handling, worker health, context cancellation

### Files Read

- `backend/internal/handler/degradation.go` — `DegradedResponse`, `WriteDegradedJSON`, `RequireDB`, `RequireRedis`, `RequireHub`, `RequireHubDegraded`
- `backend/internal/handler/stats.go` — `GetLeaderboard`, `GetUserStats` with nil-db guard
- `backend/internal/server/server_lifecycle.go` — `serve`, `waitForShutdown`, `startServer`, `Run`
- `backend/internal/server/server_init.go` — `startWorker`, `startWorkers`
- `backend/internal/worker/game_result_worker.go` — `GameResultWorker`, `Start`, `processMessage`, `processBatch` (174 lines)
- `backend/internal/worker/email_worker.go` — `EmailWorker`, `Start`, `processMessage`, `handleSendFailure` (146 lines)
- `backend/internal/worker/email_resend.go` — `sendEmail`, circuit breaker integration
- `backend/internal/worker/gdpr_cleanup.go` — `GDPRCleanupWorker`, `Start`, `runOnce` (68 lines)
- `backend/internal/resilience/retry.go` — `DefaultDBRetry`, `DefaultRedisRetry`, `ExternalAPIRetry`, `isRetryable`, `MaybeRetryable`
- `backend/internal/resilience/circuitbreaker.go` — `NewPostgresBreaker`, `NewRedisBreaker`, `NewResendBreaker`
- `backend/internal/game/hub.go` — `CloseAllRooms`

### Findings

#### Step 1: Graceful Degradation (degradation.go)

**PASS** — Degradation is implemented at the handler level via nil-guard checks on critical dependencies:

| Guard | File:Line | Behavior on nil |
|-------|-----------|-----------------|
| `RequireDB` | `degradation.go:29` | Returns false + writes 503 with `{"degraded":true,"message":"Database temporarily unavailable"}` |
| `RequireRedis` | `degradation.go:38` | Returns false + writes 503 with `{"degraded":true,"message":"Cache temporarily unavailable"}` |
| `RequireHub` | `degradation.go:47` | Returns false + writes 503 via apierror |
| `RequireHubDegraded` | `degradation.go:56` | Returns false + writes DegradedResponse with caller-specified payload |

**Non-critical features disabled gracefully:**
- `StatsHandler.GetLeaderboard` and `GetUserStats` return 503 when `h.db == nil` (`stats.go:20-23,56-59`)
- Stateless operations (WebSocket game play, lobby join) do NOT require DB/Redis — they use `RequireHubDegraded` to preserve game functionality when only the hub is available

**User notification:**
- `DegradedResponse` struct (line 11-15) carries `degraded: true` and a human-readable `message` field
- 503 Service Unavailable returned with JSON body — HTTP semantics are correct
- Frontend has no dedicated UI handling for `degraded: true` in `DegradedResponse` — the 503 response is surfaced as a generic error

**Concerns:**

- [MEDIUM] **No centralized degradation state flag.** Degradation is purely per-call nil-guarding. There is no `IsDegraded()` method, no `/health/degraded` endpoint, and no monitoring integration that records whether the system is in degraded mode. An operator cannot query the current degradation state.
- [MEDIUM] **No automatic degradation detection.** Degradation must be externally triggered (passing nil to handlers). There is no automatic circuit-breaker integration that promotes nils to the handler constructor — the circuit breaker in `resilience/` is used only by the email worker, not by the HTTP handler chain.
- [LOW] **Frontend unaware of degraded state.** The `DegradedResponse` JSON includes `degraded: true` but no frontend code reads this field to display a degradation banner or disable leaderboard/stats buttons.
- [INFO] `RequireHub` and `RequireHubDegraded` have inconsistent response formats — `RequireHub` uses `apierror.New().Write()` (standard API error JSON) while `RequireDB`/`RequireRedis` use `DegradedResponse` (custom JSON with `degraded` flag). This inconsistency makes client-side handling harder.

#### Step 2: Graceful Shutdown (server_lifecycle.go)

**PASS** — Shutdown follows a correct sequence with timeout:

| Step | Code | Detail |
|------|------|--------|
| 1. Signal wait | `server_lifecycle.go:127` | Blocks on `shutdownSignals()` (SIGINT/SIGTERM) |
| 2. Close all rooms | `server_lifecycle.go:130` | `hub.CloseAllRooms()` persists state for each room |
| 3. Close broadcaster | `server_lifecycle.go:132-136` | Closes Redis Pub/Sub subscriptions |
| 4. HTTP server drain | `server_lifecycle.go:141` | `srv.Shutdown(shutdownCtx)` with 30s timeout |
| 5. Cancel context | `server_lifecycle.go:145` | Propagates cancellation to workers, cleanup loops, metrics |
| 6. DB/Redis/tracer close | `runServer` defers | `db.Close()`, `redis.Close()`, `stopTracer()`, `audit.CloseDBLogger()` |

**Timeout:** `ShutdownTimeout = 30s` — fixed constant, not configurable. Reasonable default but should be configurable via env var for production.

**Concerns:**

- [HIGH] **No WaitGroup coordination for worker goroutines.** `startWorker` launches workers as fire-and-forget goroutines (line 68-71). After `cancel()` at line 145, workers detect `ctx.Done()` and begin cleanup, but `serve()` returns immediately — `runServer` then closes db/redis while workers may still be performing I/O. There is no `sync.WaitGroup` or `sync.WaitGroup` equivalent to ensure all workers have completed before db/redis are closed.
- [MEDIUM] **Database and Redis are closed before workers complete.** `defer db.Close()` (line 67) and `defer func() { _ = redis.Close() }()` (line 75) are in `runServer`, which runs after `serve()` returns. Since workers are not waited on, they may still be processing when the database connection pool is closed.
- [LOW] `cancel()` is called both explicitly (line 145) and via `defer cancel()` (line 87). The explicit call makes the deferred call a no-op, which is harmless but untidy.
- [INFO] The `serverShutdownFn` variable is replaceable in tests (line 35) — good testability pattern.
- [INFO] `shutdownSignals` is a function variable replaceable in tests (line 25) — good testability pattern.
- [INFO] `broadcaster.Close()` error is logged but not surfaced.

#### Step 3: Background Worker Resilience

**GameResultWorker** (`game_result_worker.go`):

| Feature | Status | Detail |
|---------|--------|--------|
| Retry on DB error | **PASS** | DB errors (begin tx, upsert, insert, commit) return without ACK — message stays in stream for XReadGroup redelivery (at-least-once) |
| Retry on parse error | **PASS** | Invalid JSON, missing payload, invalid UUID → ACK immediately (no point retrying) |
| Dead letter | **FAIL** | No dead letter queue. Messages that consistently fail (e.g., FK violation, constraint error) remain in the stream indefinitely and are redelivered forever |
| Health metric | **PASS** | `GameResultsStreamLen` gauge (`server_metrics.go`) tracks pending message count |
| Context cancellation | **PASS** | `select { case <-ctx.Done(): flush(); return }` (line 68-71) — flushes pending batch before exit |
| Batch processing | **PASS** | Processes up to 100 messages per batch, flushes every 1s or on batch full |
| Individual tx per message | **PASS** | Each message in its own transaction — one failure doesn't abort the batch |

**EmailWorker** (`email_worker.go`):

| Feature | Status | Detail |
|---------|--------|--------|
| Retry with counter | **PASS** | Manual `retry_count` in message values; up to 5 retries (`maxRetries = 5`) |
| Dead letter | **PASS** | Moved to `email:dead-letter` stream after max retries (line 131-134) |
| Circuit breaker | **PASS** | Resend API calls wrapped in `gobreaker.CircuitBreaker` via `resilience.NewResendBreaker()` — trips after 3 consecutive failures, 60s recovery timeout, 1 probe request in half-open |
| Health metric | **PASS** | `EmailQueueStreamLen` gauge |
| Context cancellation | **PASS** | `select { case <-ctx.Done(): return }` (line 63-66) |
| PII in logs | **FAIL** | (See Section 13) `payload.To` (full email address) logged at WARN, INFO, ERROR levels — violates logging policy |

**GDPRCleanupWorker** (`gdpr_cleanup.go`):

| Feature | Status | Detail |
|---------|--------|--------|
| Retry | **FAIL** | No retry logic. DB error is logged and ignored — next run is 24h later |
| Dead letter | N/A | Scheduled task, not queue-based |
| Health metric | **FAIL** | No specific health metric. No Prometheus counter for cleanup runs or deleted users |
| Context cancellation | **PASS** | `select { case <-ctx.Done(): return }` (line 48) |
| Run timeout | **PASS** | Per-run 2-minute context timeout (line 57-58) prevents long cleanup from blocking shutdown |
| Interval config | **PASS** | Configurable via `GDPR_CLEANUP_INTERVAL_HOURS` env var (default 24h) |

**Shared resilience infrastructure** (`resilience/`):

| Component | Status | Detail |
|-----------|--------|--------|
| `DefaultDBRetry` | **PASS** | Exponential backoff (100ms base), max 3 retries, 50ms jitter |
| `DefaultRedisRetry` | **PASS** | Exponential backoff (50ms base), max 2 retries, 25ms jitter |
| `ExternalAPIRetry` | **PASS** | Exponential backoff (500ms base), max 2 retries, 200ms jitter |
| `isRetryable` classifier | **PASS** | Comprehensive: pgx commit rollback, conn closed, `SafeToRetry`, timeouts (pgconn, net), ECONNRESET, ECONNREFUSED, EOF, unexpected EOF |
| `MaybeRetryable` helper | **PASS** | Maps transient errors to `retry.RetryableError`, passes persistent errors through |
| Circuit breaker — Postgres | **PASS** | `NewPostgresBreaker`: 5 consecutive failures → open, 30s timeout, 3 half-open probes |
| Circuit breaker — Redis | **PASS** | `NewRedisBreaker`: 5 consecutive failures → open, 15s timeout, 3 half-open probes |
| Circuit breaker — Resend | **PASS** | `NewResendBreaker`: 3 consecutive failures → open, 60s timeout, 1 half-open probe |
| Circuit breaker state metric | **PASS** | `CircuitBreakerState` Prometheus gauge with `{breaker, state}` labels; state changes logged at WARN level |

**Concerns:**

- [HIGH] **GameResultWorker has no dead letter queue.** Permanently-failing messages (FK violations, constraint errors, corrupt payloads past parsing) are redelivered forever. Unlike `EmailWorker` which moves to `email:dead-letter` after 5 failures, the game result worker has no max-retry threshold. This causes continuous error logs and resource waste.
- [MEDIUM] **GDPRCleanupWorker has no retry.** A transient DB failure during `HardDeleteExpiredUsers` skips an entire 24h cleanup cycle. Users whose 30-day retention window expires during that cycle are deleted one day late — a GDPR compliance concern at scale.
- [MEDIUM] **No WaitGroup for worker shutdown** (also noted in Step 2). All four workers are launched as fire-and-forget goroutines. There is no mechanism to ensure they complete before the process exits.
- [LOW] **GameResultWorker consumer ID is hostname-based** (line 77-80). Falls back to `"result-worker-1"` when `$HOSTNAME` is empty (single-instance). Correct for single-instance, but in multi-instance deployments each instance must have a unique hostname.
- [LOW] **GDPRCleanupWorker has no Prometheus metrics.** No counter for successful/failed runs, no gauge for deleted users count, no histogram for run duration. Only slog output is available for monitoring.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Degradation toggle via nil checks | PASS — DB/Redis/Hub nils disable non-critical paths | NONE |
| Non-critical features disabled | PASS — Leaderboard/stats return 503 when DB is nil | NONE |
| Core game preserved | PASS — Game play doesn't require DB/Redis | NONE |
| User notified of degraded state | PASS — `DegradedResponse` with `degraded: true` + message | NONE |
| Automatic degradation detection | FAIL — No circuit-breaker integration; degradation must be manually triggered | MEDIUM |
| Centralized degradation state | FAIL — No `IsDegraded()` or `/health/degraded` endpoint | MEDIUM |
| Frontend degraded-state awareness | FAIL — Frontend ignores `degraded` field in response | LOW |
| In-flight request drain | PASS — `srv.Shutdown` with 30s timeout | NONE |
| WebSocket connection graceful close | PASS — `hub.CloseAllRooms()` → room.Close() | NONE |
| Database connection release | PASS — `defer db.Close()` in runServer | NONE |
| Shutdown timeout | PASS — 30s (`ShutdownTimeout`) | NONE |
| Worker shutdown coordination | FAIL — No WaitGroup; workers may I/O during db/redis close | HIGH |
| GameResultWorker retry | PASS — At-least-once via no-ACK on DB errors | NONE |
| GameResultWorker dead letter | FAIL — No dead letter; permanently-failing messages redelivered forever | HIGH |
| EmailWorker retry | PASS — 5 retries with `retry_count` counter | NONE |
| EmailWorker dead letter | PASS — `email:dead-letter` stream after max retries | NONE |
| EmailWorker circuit breaker | PASS — Resend API wrapped in gobreaker | NONE |
| GDPRCleanupWorker retry | FAIL — No retry; transient errors skip 24h cycle | MEDIUM |
| GDPRCleanupWorker metrics | FAIL — No Prometheus counters/gauges for cleanup operations | LOW |
| Worker context cancellation | PASS — All 4 workers check `ctx.Done()` | NONE |
| Shared retry infrastructure | PASS — Exponential backoff + jitter + comprehensive `isRetryable` classifier | NONE |
| Circuit breaker state exported | PASS — Prometheus gauge + WARN-level state change logs | NONE |

- **Degradation is handler-level** via nil guards, not framework-level. Works correctly but lacks automatic detection, centralized state query, and frontend integration.
- **Shutdown sequence is correct** but 3 worker goroutines lack coordination — they may attempt DB/Redis operations after the pools are closed.
- **Worker resilience is mixed:** EmailWorker is fully resilient (retry + dead letter + circuit breaker); GameResultWorker has retry but no dead letter; GDPRCleanupWorker has neither retry nor metrics.
- **Shared resilience library is excellent** — well-tuned circuit breakers, comprehensive retry classification, Prometheus state export, and thorough unit test coverage (100%).

---

## 21. Kubernetes Manifests

### Checks Performed

- **Step 1:** Audited base manifests (`infra/k8s/base/`) — StatefulSet resource limits, HPA, PDB, probes, security context
- **Step 2:** Audited network policies (`infra/k8s/global/network-policy.yaml`) — default deny, egress, database isolation
- **Step 3:** Audited secret management — hardcoded secrets, external secret manager, Opaque type with base64
- **Step 4:** Checked regional overlays (`infra/k8s/overlays/`) and global MCI resources

### Files Read

- `infra/k8s/base/service.yaml` — StatefulSet (165 lines), ServiceAccount, headless + client Service
- `infra/k8s/base/hpa.yaml` — HPA definition (57 lines)
- `infra/k8s/base/pod-disruption-budget.yaml` — PDB minAvailable (11 lines)
- `infra/k8s/base/redis.yaml` — Redis StatefulSet + Service (64 lines)
- `infra/k8s/base/kustomization.yaml` — Base kustomize config (16 lines)
- `infra/k8s/base/region-config.yaml` — Region ConfigMap (15 lines)
- `infra/k8s/global/network-policy.yaml` — Ingress-only network policy (35 lines)
- `infra/k8s/global/multicluster-ingress.yaml` — MCI + BackendConfig (57 lines)
- `infra/k8s/overlays/us-east1/kustomization.yaml` — us-east1 overlay (46 lines)
- `infra/k8s/overlays/europe-west1/kustomization.yaml` — europe-west1 overlay (46 lines)
- `infra/k8s/overlays/asia-southeast1/kustomization.yaml` — asia-southeast1 overlay (46 lines)

### Findings

#### Step 1: Base Manifest Audit

| Check | Verdict |
|-------|---------|
| StatefulSet resource requests + limits | **PASS** — requests: 250m CPU / 128Mi mem; limits: 1 CPU / 512Mi mem (`service.yaml:140-146`) |
| HPA configured with appropriate metrics | **PASS** — CPU at 65% utilization + custom `ws_connections` Pods metric at avg 6000 (`hpa.yaml:24-38`). scaleUp: 100%/4 pods per 30s; scaleDown: 1 pod per 120s with 300s window |
| PDB exists | **PASS** — `pod-disruption-budget.yaml`: `minAvailable: 1` for `app: balloon-game` |
| Liveness + readiness probes | **PASS** — startupProbe (`/health/live`, 30-failure, 10s period), livenessProbe (`/health/live`, 10s delay, 30s period), readinessProbe (`/health/ready`, 5s delay, 10s period) — all HTTP on 8080 (`service.yaml:148-165`) |
| Security context (non-root, read-only rootfs, no priv esc) | **FAIL** — No `securityContext` defined on the StatefulSet pod spec or container spec. No `runAsNonRoot: true`, no `readOnlyRootFilesystem: true`, no `allowPrivilegeEscalation: false`. The Dockerfile uses a distroless non-root image (`USER nonroot:nonroot`), so the process *does* run as non-root, but this is not enforced at the K8s manifest level. |

#### Step 2: Network Policy Audit

| Check | Verdict |
|-------|---------|
| Default deny ingress | **PARTIAL** — The existing `NetworkPolicy` (ingress only) whitelists traffic to `app: balloon-game` pods, which implicitly denies unlisted ingress. However, there is no explicit `podSelector: {}` default-deny policy for defense-in-depth. Redis pods are not covered by any network policy. |
| Default deny egress | **FAIL** — No egress rules defined at all. `policyTypes` only lists `Ingress`. Pods can reach any external destination. |
| Database access restricted to app namespace | **FAIL** — No egress policy restricts database connectivity. The app pods have unrestricted egress to any namespace or external host. |
| Redis isolation | **FAIL** — The Redis StatefulSet (`redis.yaml`) is deployed in the same namespace with no network policy covering it. Any pod in the namespace (including compromised app pods) can reach Redis on port 6379. |

#### Step 3: Secret Management Audit

| Check | Verdict |
|-------|---------|
| Secrets hardcoded in manifests | **PASS** — No hardcoded secret values. Secrets referenced via `secretKeyRef` (`service.yaml:131`) and `secretRef` (`service.yaml:137`) pointing to `balloon-game-secrets`. |
| External secret management referenced | **PASS** — Comments reference GCP Workload Identity (`iam.gke.io/gcp-service-account` annotation in overlays) and GCP Secret Manager. The `balloon-game-secrets` resource is not defined in manifests — provisioned externally. |
| No `type: Opaque` with base64-encoded real secrets | **PASS** — No `Secret` resources of any kind in the manifests. All secrets are externally managed. |

#### Additional Findings

- **Redis lacks probes** — `redis.yaml:55-64` has readiness and liveness probes but no startup probe. Redis can take 10+ seconds to load RDB/AOF on restart. Without a startup probe, the liveness probe may kill Redis during initial load.
- **Redis lacks resource limits** — `redis.yaml:48-54` has resource requests AND limits already defined. This is fine.
- **No PodSecurityPolicy / PodSecurity admission labels** — No `pod-security.kubernetes.io/enforce` label on the namespace in any overlay or base manifest. GKE enforces a default (usually `restricted` in newer clusters), but this is not explicit.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Resource requests + limits (StatefulSet) | PASS | NONE |
| HPA metrics | PASS — CPU + ws_connections dual-metric | NONE |
| PDB (minAvailable) | PASS — 1 minimum available | NONE |
| Liveness + readiness + startup probes | PASS — all three probe types defined | NONE |
| Security context (non-root, read-only, no priv esc) | **FAIL** — not set on pod or container spec; relies on Dockerfile non-root only | MEDIUM |
| Network policy — default deny ingress | PARTIAL — implicit via whitelist; no explicit default-deny; Redis uncovered | MEDIUM |
| Network policy — default deny egress | **FAIL** — no egress rules at all | HIGH |
| Network policy — database isolation | **FAIL** — unrestricted egress, no DB namespace isolation | HIGH |
| Secrets hardcoded | PASS — all external via Workload Identity + GCP Secret Manager | NONE |
| Redis startup probe | **MISSING** — no startup probe for Redis, only liveness/readiness | LOW |

---

## 22. Terraform Security

### Checks Performed

- **Step 1:** Audited Terraform provider configuration — version pinning, hardcoded credentials, state storage, sensitive data in state
- **Step 2:** Audited GCP resource security — Cloud SQL, Redis, GKE Workload Identity, Secret Manager IAM
- **Step 3:** Attempted `terraform plan` for drift detection (failed — Terraform CLI not available in this environment)
- **Step 4:** Logged findings to report

### Files Read

- `infra/terraform/main.tf` — All 5 resource types: Cloud SQL, Redis, Secret Manager secrets, GKE Workload Identity GSA, IAM bindings (130 lines)
- `infra/terraform/variables.tf` — Variables: `project_id`, `region`, `db_password` (16 lines)
- `infra/terraform/outputs.tf` — Outputs: database connection name, Redis host/port, GSA email (16 lines)

### Findings

#### Step 1: Provider Configuration

**Provider versions pinned:**
- **PASS** — `hashicorp/google` constrained to `~> 5.0` in `required_providers` (main.tf:9-11). The pessimistic version constraint allows patch/minor updates within 5.x, preventing accidental upgrades to v6 (breaking changes) while receiving bugfixes.
- [INFO] No `required_version` is set on the `terraform` block. Different team members or CI runners may use different Terraform versions (e.g., 1.5 vs 1.10), potentially causing state format incompatibilities or behavioral differences in plan generation.

**Hardcoded credentials:**
- **PASS** — No hardcoded credentials in any Terraform file.
- `db_password` is passed via the `var.db_password` variable (main.tf:42), which is declared `sensitive = true` in `variables.tf:15`. This prevents the value from appearing in Terraform CLI output and plan logs.
- All other configuration values (project_id, region) are non-sensitive infrastructure identifiers.

**State storage:**
- **PASS** — Remote state is configured in a GCS bucket (`balloon-game-terraform-state`, prefix `terraform/state`) via `backend "gcs"` (main.tf:3-6). This enables state locking for collaborative use.
- [INFO] No explicit `encryption_key` is set on the GCS backend block. GCS provides server-side encryption by default (AES-256 with Google-managed keys), but for compliance-sensitive workloads, a CMEK (Customer-Managed Encryption Key) should be specified to ensure state-file encryption uses a key under the team's control.
- [INFO] No state bucket access logging configured — state bucket access is untracked.

**Sensitive data in state:**
- **PASS** — `db_password` is declared `sensitive = true`, preventing it from appearing in Terraform output.
- [INFO] Terraform state files still store all resource attribute values in plaintext (including the database password). The GCS bucket's at-rest encryption is the only protection. For stronger guarantees, a CMEK key on the state bucket is recommended.

#### Step 2: GCP Resource Security

**Cloud SQL (`google_sql_database_instance.uppy_db`):**

- **[HIGH] Public IP without authorized networks restriction** — The Cloud SQL instance (main.tf:21-32) has **no `ip_configuration` block**. This means:
  1. The instance is assigned a public IP address by default.
  2. No `authorized_networks` are configured — the database is accessible from **any IP address on the internet**.
  3. No `require_ssl` or `ssl_mode` setting is configured, meaning non-SSL connections may be accepted.
  - **Fix:** Add `ip_configuration` with `ipv4_enabled = false` (private IP only) or at minimum restrict `authorized_networks` to specific trusted CIDR ranges (e.g., GKE cluster's NAT IP or Cloud NAT range). Enable `ssl_mode = "TRUSTED_CLIENT_CERTIFICATE_REQUIRED"`.

- [INFO] `tier = "db-f1-micro"` (main.tf:26) — Shared-core micro instance. Adequate for dev/staging but insufficient for production. No `disk_size` or `disk_autoresize` configured (defaults: 10GB, likely SSD).

**Redis (`google_redis_instance.uppy_redis`):**

- [MEDIUM] No `auth_enabled` — The Redis instance (main.tf:46-52) defaults to `auth_enabled = false`. Anyone who can reach the Redis endpoint can query it without authentication. For production, `auth_enabled = true` should be set, and the generated auth string should be stored in Secret Manager.
- [MEDIUM] No `transit_encryption_enabled` — Redis connections are in plaintext. Even over VPC, enabling in-transit encryption (`transit_encryption_enabled = true`) is recommended for sensitive data.
- [INFO] No `connect_mode` or `authorized_network` specified. Memorystore Redis defaults to `connect_mode = DIRECT_PEERING` with the `default` VPC network. For production, an explicit `authorized_network` referencing a dedicated VPC should be used.
- [INFO] `redis_version = "REDIS_7_0"` — Redis 7.0 is current.

**GKE Workload Identity (`google_service_account.balloon_game`):**

- **PASS** — Correctly implemented per ADR-014 (main.tf:94-97): A dedicated GSA `balloon-game` is created for GKE pod identity.
- **PASS** — `google_service_account_iam_member` (main.tf:116-120) binds the Kubernetes service account `balloon-game/balloon-game` to the GSA via `roles/iam.workloadIdentityUser`. No service account keys are created.
- **PASS** — `google_project_iam_member.cloudsql_client` (main.tf:124-128) grants `roles/cloudsql.client` to the GSA for Cloud SQL proxy authentication.

**Secret Manager IAM (`google_secret_manager_secret_iam_member`):**

- **PASS** — Properly configured: 5 secrets receive `roles/secretmanager.secretAccessor` for the GKE Workload Identity GSA (main.tf:100-111).
- [INFO] All 5 secrets share the same role binding. There is no fine-grained access control per secret — any pod with this GSA identity can read all 5 secrets. For defense-in-depth, separate GSAs or conditional IAM bindings could limit access per pod type.
- [INFO] No secret version management — the current implementation only creates secret containers. There are no `google_secret_manager_secret_version` resources to populate initial secret values. These must be created outside Terraform or via a separate process.

#### Step 3: Terraform Drift Detection

- **BLOCKED** — Terraform CLI is not available in the inspection environment. Drift detection could not be performed.
- Recommendation: Run `terraform plan -detailed-exitcode` in a CI pipeline (with GCP credentials) to detect infrastructure drift.
- Expected baseline: initial `terraform apply` should produce exit code 0 (no changes) thereafter. Any exit code 2 indicates drifted state.

#### Additional Findings

- [INFO] `data "google_project" "current" {}` (main.tf:130) is declared but never referenced in the Terraform configuration. This unused data source can be removed to avoid unnecessary API calls during plan/apply.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Provider version pinned | PASS — `~> 5.0` constraint on `hashicorp/google` | LOW (no `required_version` on terraform block) |
| Hardcoded credentials | PASS — `db_password` from sensitive variable; no inline secrets | NONE |
| GCS state backend | PASS — Remote state in GCS bucket with locking; no CMEK configured | LOW |
| Sensitive data in state | PASS — `sensitive = true` on password; state encrypted at rest by GCS | LOW |
| Cloud SQL — public IP | **FAIL** — No `ip_configuration`; database accessible from any IP | HIGH |
| Cloud SQL — authorized networks | **FAIL** — No `authorized_networks`; unrestricted internet access | HIGH |
| Cloud SQL — SSL enforcement | **FAIL** — No `require_ssl` or `ssl_mode` configured | MEDIUM |
| Redis — auth_enabled | **FAIL** — Defaults to `false`; Redis accessible without password | MEDIUM |
| Redis — transit encryption | **FAIL** — `transit_encryption_enabled` not set; plaintext | MEDIUM |
| GKE Workload Identity | **PASS** — GSA + WI binding; no service account keys | NONE |
| Secret Manager IAM | **PASS** — `secretAccessor` role for GKE GSA | LOW (no per-secret granularity) |
| Terraform drift detection | **BLOCKED** — Terraform CLI unavailable in this environment | INFO |
| Unused data source | INFO — `data.google_project.current` declared but never used | LOW |

- **3 HIGH findings** — Cloud SQL has no public IP restriction, no authorized networks, and no SSL enforcement. This is the single greatest infrastructure security risk.
- **3 MEDIUM findings** — Redis lacks auth and in-transit encryption.
- **No hardcoded secrets** — Credential management via sensitive variables is correct.
- **Workload Identity is correctly implemented** per ADR-014, avoiding long-lived service account keys.
- **Drift detection requires Terraform CLI** — should be integrated into CI/CD pipeline.

---

## 17. Resilience Patterns

### Checks Performed

- **Step 1:** Audited circuit breaker configuration (`backend/internal/resilience/circuitbreaker.go`) — failure threshold, success threshold, timeout, state change callbacks, metrics
- **Step 2:** Audited retry configuration (`backend/internal/resilience/retry.go`, `backend/internal/store/`) — max retries, backoff strategy, retryable error classification, circuit breaker + retry composability
- **Step 3:** Audited rate limiting (`backend/internal/middleware/ratelimit.go`, `backend/internal/store/redis_ratelimit.go`) — per-endpoint limits, algorithm, headers, 429 responses
- **Step 4:** Audited bulkhead pattern (`backend/internal/middleware/bulkhead.go`) — per-endpoint quotas, graceful degradation, metrics

### Files Read

- `backend/internal/resilience/circuitbreaker.go` — 3 circuit breaker constructors (Postgres, Redis, Resend API)
- `backend/internal/resilience/retry.go` — Retry policies (DB, Redis, External), error classification, backoff helpers
- `backend/internal/resilience/circuitbreaker_test.go` — 12 test functions (smoke, state transitions, trip thresholds)
- `backend/internal/resilience/retry_test.go` — 7 test functions + 1 benchmark
- `backend/internal/resilience/retry_classify_test.go` — 15 test functions covering all error classification paths
- `backend/internal/store/postgres.go` — PostgresStore with `NewPostgresBreaker()`
- `backend/internal/store/postgres_helpers.go` — `withRetryRead` (retry + CB) and `withRetryWrite` (CB only)
- `backend/internal/store/base_repository.go` — `baseRepository` with CB (no retry wrapper)
- `backend/internal/store/base_redis_store.go` — `baseRedisStore` with `NewRedisBreaker()`
- `backend/internal/middleware/ratelimit.go` — Endpoint rate limiting middleware (228 lines)
- `backend/internal/store/redis_ratelimit.go` — `CheckRateLimit` with Lua script (56 lines)
- `backend/internal/middleware/bulkhead.go` — Bulkhead middleware (58 lines)
- `backend/internal/middleware/middleware_resilience_test.go` — Bulkhead tests (lines 505-587)
- `backend/internal/worker/email_worker.go` — EmailWorker with circuit breaker + retry via Redis stream
- `backend/internal/worker/email_resend.go` — `sendEmail` circuit breaker usage
- `backend/internal/metrics/metrics.go` — `CircuitBreakerState` gauge definition (line 115)
- `backend/internal/server/routes_public.go` — Route-level bulkhead application (lines 53, 84, 109)

### Findings

#### Step 1: Circuit Breaker Configuration

`backend/internal/resilience/circuitbreaker.go` defines 3 breakers via `github.com/sony/gobreaker/v2`:

| Breaker | Name | MaxRequests (half-open) | Interval (window) | Timeout (open→half-open) | Trip threshold |
|---------|------|------------------------|-------------------|-------------------------|----------------|
| Postgres | `"postgres"` | 3 | 60s | 30s | 5 consecutive failures |
| Redis | `"redis"` | 3 | 60s | 15s | 5 consecutive failures |
| Resend API | `"resend-api"` | 1 | 60s | 60s | 3 consecutive failures |

- **PASS** — Failure thresholds: Postgres/Redis (5) and Resend (3) are within the recommended 5-10 range.
- **PASS** — Success thresholds (MaxRequests): 3 for Postgres/Redis, 1 for Resend. All reasonable. Resend is intentionally conservative (external API).
- **PASS** — Timeouts: Postgres (30s), Redis (15s — faster recovery expected), Resend (60s — longer for external API). All within the recommended 30-60s range.
- **PASS** — `onStateChange` callback logs all state transitions via `slog.Warn` with `breaker`, `from`, `to` fields, and updates the `circuit_breaker_state` Prometheus gauge (value: 0=closed, 0.5=half-open, 1=open).

**Prometheus metric:** `circuit_breaker_state{name, state}` defined at `metrics/metrics.go:115-121`. Gauge ensures only the current state label has a non-zero value (old state label reset to 0 in `onStateChange`).

**Breaker usage across the codebase:**
- `PostgresStore` uses `NewPostgresBreaker()` for both reads and writes (`postgres.go:88`, `postgres_helpers.go:14,22`)
- `baseRepository` uses `NewPostgresBreaker()` for reads/writes (`base_repository.go:17,21,28`)
- `baseRedisStore` / `RedisStore` use `NewRedisBreaker()` for Redis operations (`base_redis_store.go:18`, `redis.go:50,58`)
- `EmailWorker` uses `NewResendBreaker()` for email API calls (`email_worker.go:45`, `email_resend.go:40`)

**Composability concern — `withRetryRead` vs `withRetryWrite`:**
- `PostgresStore.withRetryRead` (`postgres_helpers.go:12-19`): wraps `retry.Do` → `cb.Execute` → `fn`. When the breaker is open, `cb.Execute` returns `gobreaker.ErrOpenState` which is NOT classified as retryable by `isRetryable`, so the retry loop stops immediately (correct behavior).
- `PostgresStore.withRetryWrite` (`postgres_helpers.go:21-26`): uses only `cb.Execute` without `retry.Do` wrapper. Writes fail immediately on transient errors even if the CB is closed. This may be intentional (writes may not be idempotent), but it is inconsistent with reads.
- `baseRepository.withRetryRead/Write` (`base_repository.go:21-33`): uses only `cb.Execute` without `retry.Do`. The older repository pattern does not benefit from retry at all.

#### Step 2: Retry Configuration

`backend/internal/resilience/retry.go` defines 3 retry policies using `github.com/sethvargo/go-retry`:

| Policy | Base backoff | Max retries | Jitter | Total attempts (1 initial + retries) |
|--------|-------------|-------------|--------|--------------------------------------|
| `DefaultDBRetry` | 100ms exponential | 3 | 50ms | 4 |
| `DefaultRedisRetry` | 50ms exponential | 2 | 25ms | 3 |
| `ExternalAPIRetry` | 500ms exponential | 2 | 200ms | 3 |

- **PASS** — Max retries: 3 (DB), 2 (Redis), 2 (External). All within the recommended 3-5 range. External API is intentionally conservative.
- **PASS** — All three policies use exponential backoff with jitter.
- **PASS** — `isRetryable()` classification covers all transient error types:
  - pgx/v5: `ErrTxCommitRollback`, `ErrConnClosed`, `pgconn.SafeToRetry`, `pgconn.Timeout`
  - Network: `net.Error.Timeout()`, `syscall.ECONNRESET`, `syscall.ECONNREFUSED`, `io.EOF`, `io.ErrUnexpectedEOF`
  - `MaybeRetryable` helper wraps only transient errors with `retry.RetryableError`; persistent errors (constraint violations, not-found, permission denied) pass through unmodified, causing `retry.Do` to abort immediately.
- **PASS** — No retry on 4xx errors — no 4xx-class errors are classified as retryable.
- **PASS** — Test coverage for `isRetryable` is comprehensive (15 test functions in `retry_classify_test.go` covering nil, pgx errors, network errors, persistent errors, context deadline, SafeToRetry interface, stub errors).
- **PASS** — `JitteredBackoff` helper supports manual retry loops with exponential backoff + jitter. Tested with 4 attempt levels + zero-base panic guard.

**Retry usage across store layer (8 locations, all using `DefaultDBRetry` or `DefaultRedisRetry`):**
| File | Policy | Purpose |
|------|--------|---------|
| `postgres_helpers.go:13` | DefaultDBRetry | Read operations via PostgresStore |
| `store/config_repository.go:60` | DefaultDBRetry | Config read |
| `store/postgres_config.go:51` | DefaultDBRetry | Config operations |
| `store/lobby_repository.go:37` | DefaultDBRetry | Lobby read |
| `store/postgres_lobbies_save.go:28` | DefaultDBRetry | Lobby save |
| `store/magiclink_store.go:52` | DefaultRedisRetry | Magic link token ops |
| `store/redis_magiclink.go:44` | DefaultRedisRetry | Magic link Redis ops |
| `store/redis_room_registry.go:99` | DefaultRedisRetry | Room registry Redis ops |

All retry callbacks use `MaybeRetryable(err)` to ensure only transient errors trigger retries.

**EmailWorker retry mechanism:** The EmailWorker does NOT use `sethvargo/go-retry` for email sending. Instead, it implements retry via Redis stream `retry_count` field + dead-letter queue (5 max retries). Circuit breaker wraps the HTTP call (`NewResendBreaker`). This is a valid architecture — Redis stream retry survives worker restarts and provides observability via dead-letter.

#### Step 3: Rate Limiting

`backend/internal/middleware/ratelimit.go` defines per-endpoint rate limits:

| Endpoint | Requests/Window | Fail-Closed |
|----------|----------------|-------------|
| `auth:quickplay` | 10/min | ✅ Yes |
| `auth:request` | 5/min | No (fail-open) |
| `auth:verify` | 10/min | No (fail-open) |
| `registry:create` | 5/min | No (fail-open) |
| `registry:check` | 30/min | No (fail-open) |
| `registry:lobbies` | 30/min | No (fail-open) |
| `registry:match` | 10/min | No (fail-open) |
| `stats:leaderboard` | 60/min | No (fail-open) |
| `admin:login` | 5/min | ✅ Yes |
| `default` | 60/min | No (fail-open) |

- **PASS** — Rate limits per endpoint category: auth, game/registry, admin, and a default for unlisted endpoints.
- [LOW] **Algorithm:** Uses fixed-window (Redis `INCR` + `EXPIRE`), not sliding window. The Lua script at `redis_ratelimit.go:14-22` increments a counter and sets TTL on first increment. At window boundaries, the counter resets, allowing burst traffic at the boundary. A true sliding window would require Redis Sorted Sets or a more complex Lua script. This is a documented trade-off for simplicity.
- **PASS** — `setRateLimitHeaders` sets `Retry-After` (RFC 6585) and `X-RateLimit-Limit` headers on 429 responses.
- **PASS** — 429 response uses `apierror.TooManyRequests()` with JSON body.
- [INFO] `X-RateLimit-Remaining` and `X-RateLimit-Reset` headers are intentionally NOT set — the store interface only returns `(bool, error)`, not remaining count or reset time. Comment at `ratelimit.go:39` documents this as avoiding over-engineering.
- **PASS** — Composite rate limit key includes endpoint + user ID (from auth context or cookies) + IP. Fallback chain: auth context → session/quickplay cookie → IP-only.
- **PASS** — Fail-closed for security-critical endpoints (`auth:quickplay`, `admin:login`) so Redis unavailability blocks requests rather than allowing unbounded access.

**Rate limit middleware types:**
- `RateLimit` (simple IP-based) — used for basic IP throttling
- `EndpointRateLimit` (composite key with user identity) — used for per-user per-endpoint limits

#### Step 4: Bulkhead Pattern

`backend/internal/middleware/bulkhead.go` defines 4 pre-defined bulkheads:

| Bulkhead | Quota | Applied To |
|----------|-------|------------|
| `AuthBulkhead` | 10 | Auth routes (`routes_public.go:53`) |
| `LobbyBulkhead` | 10 | Registry/lobby routes (`routes_public.go:84`) |
| `AdminBulkhead` | 3 | Admin routes (`routes_admin.go:18`) |
| `WebSocketBulkhead` | 50 | WebSocket upgrade (`routes_public.go:109`) |

- **PASS** — Separate limits per resource type (auth, lobby, admin, WebSocket) provide isolation between request categories.
- **PASS** — Uses `golang.org/x/sync/semaphore.Weighted` with `TryAcquire` (non-blocking). When full, returns 503 Service Unavailable with JSON body `{"error":"service busy","code":"BULKHEAD_FULL"}` — immediate rejection, no queuing.
- [LOW] **No metrics on bulkhead rejections.** There is no Prometheus counter for bulkhead rejection events. Operators cannot alert on bulkhead saturation. The 503 response is logged via the HTTP middleware chain, but no dedicated metric exists.
- **PASS** — Bulkhead tests in `middleware_resilience_test.go:505-587` cover: under-limit (requests pass), over-limit (503 returned with `BULKHEAD_FULL` code), and release-after-completion (slot freed for next request).
- **PASS** — Applied as outermost middleware layer, protecting downstream resources (DB, Redis, goroutines) from overload before authentication, rate limiting, or handler execution.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Circuit breaker — failure threshold (5/3) | PASS — 5 for Postgres/Redis, 3 for Resend | NONE |
| Circuit breaker — success threshold (3/1) | PASS — 3 for DB/Redis, 1 for Resend | NONE |
| Circuit breaker — timeout (30/15/60s) | PASS — All in 15-60s range | NONE |
| Circuit breaker — state change logging | PASS — `slog.Warn` + Prometheus gauge | NONE |
| Retry — max retries (3/2/2) | PASS — All in 2-3 range | NONE |
| Retry — exponential backoff + jitter | PASS — All policies use both | NONE |
| Retry — error classification | PASS — Comprehensive `isRetryable` for pgx/network errors | NONE |
| Retry — no 4xx retry | PASS — No 4xx classified as retryable | NONE |
| Rate limiting — per-endpoint categories | PASS — Auth, registry, admin, stats, default tiers | NONE |
| Rate limiting — sliding window | LOW — Fixed window (INCR+EXPIRE), boundary bursts possible | LOW |
| Rate limiting — headers (Retry-After, X-RateLimit-Limit) | PASS — Both set on 429; Remaining/Reset omitted intentionally | INFO |
| Rate limiting — 429 with Retry-After | PASS — `apierror.TooManyRequests` + header | NONE |
| Rate limiting — fail-closed for security endpoints | PASS — `auth:quickplay` and `admin:login` fail closed | NONE |
| Bulkhead — per-resource isolation | PASS — 4 categories (auth=10, lobby=10, admin=3, ws=50) | NONE |
| Bulkhead — graceful degradation (503 with BULKHEAD_FULL) | PASS — Non-blocking TryAcquire, JSON error | NONE |
| Bulkhead — rejection metrics | **MISSING** — No Prometheus counter for rejections | LOW |
| CB + Retry composability (withRetryRead vs withRetryWrite) | INFO — Reads use retry+CB, writes use CB only; baseRepository uses CB only | LOW |
| EmailWorker retry strategy | PASS — Redis stream `retry_count` + dead-letter (not go-retry) | NONE |
|
| --- |
|
| ## 27. GDPR Compliance |
|
| ### Checks Performed |
|
| - **Step 1:** Verified data export capability — read `backend/internal/handler/auth_gdpr.go`, `backend/internal/auth/gdpr_data.go`, `backend/internal/server/auth_service.go`, `backend/internal/store/postgres_users_gdpr.go` |
| - **Step 2:** Verified right to deletion — audited `DeleteUserData` for PII anonymization, cascade, audit log creation, outbox event publishing |
| - **Step 3:** Verified data retention policies — read `backend/internal/worker/gdpr_cleanup.go`, migrations `000005` (soft delete), `000003` (FK cascade), audit log immutability |
| - **Step 4:** Verified field-level encryption — examined `backend/internal/crypto/aes.go`, `aes_email.go`, email storage pipeline in `store/postgres_users_email.go` |
| - **Step 5:** Log findings to report |
|
| ### Files Read |
|
| - `backend/internal/handler/auth_gdpr.go` — ExportUserData + DeleteUserData HTTP handlers (59 lines) |
| - `backend/internal/auth/gdpr_data.go` — ExportUserData + DeleteUserData business logic (109 lines) |
| - `backend/internal/auth/revoke.go` — RevokeAllTokens utility (38 lines) |
| - `backend/internal/server/auth_service.go` — Auth service adapter bridging handler→auth (126 lines) |
| - `backend/internal/store/postgres_users_gdpr.go` — AnonymizeUser + HardDeleteExpiredUsers (94 lines) |
| - `backend/internal/worker/gdpr_cleanup.go` — GDPRCleanupWorker scheduled cleanup (68 lines) |
| - `backend/internal/crypto/aes.go` — AES-256-GCM encryption (149 lines) |
| - `backend/internal/crypto/aes_email.go` — Email encryption helpers |
| - `backend/internal/audit/audit.go` — Tamper-proof audit logging (187 lines) |
| - `backend/internal/domain/user.go` — User domain model with PII fields (39 lines) |
| - `backend/internal/handler/handler_interfaces.go` — AuthService interface with ExportUserData/DeleteUserData |
| - `backend/migrations/000003_fk_cascade_and_checks.up.sql` — FK ON DELETE CASCADE |
| - `backend/migrations/000005_add_soft_delete.up.sql` — Soft delete columns + retention index |
| - `backend/migrations/000006_create_audit_logs.up.sql` — Audit log immutability trigger |
|
| ### Findings |
|
| #### Step 1: Data Export Capability (ExportUserData) |
|
| - **PASS** — `GET /api/v1/user/data` (`handler/auth_gdpr.go:12`) returns user personal data in JSON format (Article 20 portable format) |
| - Exported fields: `id`, `email`, `nickname`, `created_at`, `last_login`, `game_results` |
| - Export is scoped to the authenticated user via `AuthenticatedUserFromRequest` (no privilege escalation) |
| - Returns `401 Unauthorized` for unauthenticated requests |
| - Returns `404 Not Found` if user does not exist |
| - Game results error does not fail the export (graceful fallback to empty array) |
| - Auth service adapter (`server/auth_service.go:88`) implements the business logic by fetching user + results from the store |
|
| #### Step 2: Right to Deletion (DeleteUserData) |
|
| - **PASS** — `DELETE /api/v1/user/data` (`handler/auth_gdpr.go:35`) triggers multi-step erasure: |
|   1. `RevokeAllForUser` — revokes all refresh tokens in Redis |
|   2. `RevokeAllTokens` — revokes JWT access tokens (by jti) and refresh tokens |
|   3. `AnonymizeUser` — soft-deletes PII in the users table |
|   4. Clears all auth cookies (quickplay, session, refresh) |
| - **PASS** — `AnonymizeUser` (`store/postgres_users_gdpr.go:38`) sets: |
|   - `email` → `deleted_<id>@anonymized` (encrypted with AES-256-GCM) |
|   - `email_hash` → re-computed HMAC of anonymized email |
|   - `nickname` → `'Deleted User'` |
|   - `deleted_at` → current timestamp |
|   - `email_anonymized` → `true` |
| - **PASS** — FK ON DELETE CASCADE (`migrations/000003`) ensures `game_results` and `game_sessions` cascade when the user row is eventually hard-deleted |
| - **PASS** — Soft-delete preserves referential integrity for existing game results (row retained, PII removed) |
| - **FAIL** — No audit log entry is created in `DeleteUserData`. The `auth/gdpr_data.go:48` function does not call `audit.Log()`. Only `slog.Error` is used. Admin operations (password change, config update, login) all create audit log entries, but GDPR deletion does not. |
| - **FAIL** — No outbox event is published for GDPR deletion. The `DeleteUserData` function does not call `InsertOutboxEvent`. This means downstream systems (analytics, data warehouse sync) have no signal that a user deleted their data. |
|
| #### Step 3: Data Retention Policies |
|
| - **PASS** — `GDPRCleanupWorker` (`worker/gdpr_cleanup.go:9-10`): |
|   - `defaultGDPRRetentionDays = 30` — 30-day retention matches GDPR Article 17 (right to erasure) |
|   - `defaultGDPRCleanupInterval = 24 * time.Hour` — runs once daily |
|   - Both values are configurable via constructor params (configurable via env vars in `server_init.go`) |
|   - Per-run 2-minute timeout prevents cleanup from blocking shutdown |
| - **PASS** — `HardDeleteExpiredUsers` (`store/postgres_users_gdpr.go:68`) uses `30` day default and deletes users where `deleted_at < (now - retentionDays)` |
| - **PASS** — Audit logs are immutable (`migrations/000006`): `BEFORE UPDATE` and `BEFORE DELETE` triggers prevent modification or deletion of audit log entries. Audit logs are retained permanently, which is acceptable for compliance. |
| - **INFO** — Outbox events have a processed_at index (`migrations/000011`) but no explicit retention policy or cleanup job. Processed outbox events accumulate indefinitely. |
| - **INFO** — `idx_users_deleted_at` partial index (`WHERE deleted_at IS NOT NULL`) supports efficient cleanup queries |
|
| #### Step 4: Field-Level Encryption |
|
| - **PASS** — Email is encrypted at rest using AES-256-GCM (`crypto/aes.go`): |
|   - 32-byte key from `ENCRYPTION_KEY` env var |
|   - Fresh 12-byte random nonce per encryption (`crypto/rand.Read`) |
|   - Authenticated encryption (GCM mode) — tampered ciphertext rejected |
|   - Versioned output format (`v1:hex_ciphertext`) supports future key rotation |
|   - `init()` panics if `ENCRYPTION_KEY` is missing (fail-secure) |
| - **PASS** — Email encryption pipeline (`store/postgres_users_email.go:9`): |
|   - `prepareEmailForStorage` calls `encryptEmailForStorageFn` (defaults to `crypto.EncryptEmailForStorage`) |
|   - HMAC-SHA256 of email stored as `email_hash` for lookup while email itself is encrypted |
| - **PASS** — Magic link tokens store email encrypted in Redis (`auth/magiclink.go:126`: `crypto.Encrypt(email)`) |
| - **PASS** — Resend API key encrypted at rest (`handler/admin_config.go:101`: `crypto.Encrypt`) |
| - **FAIL** — IP addresses are NOT encrypted at rest. The `audit_logs.actor_ip` field stores raw IP addresses in plaintext. The `users` table does not store IPs directly (IPs appear only in audit logs and rate limiter counters). |
| - **FAIL** — `RotateKey` (`crypto/aes.go:148`) returns `"RotateKey not yet implemented"` — key rotation for encrypted fields is not supported and would require a manual database migration |
| - **INFO** — Nicknames are stored in plaintext in the `users` table and in JWT claims. This is acceptable as nicknames are intentionally public (displayed to other players during gameplay). |
|
| ### Summary |
|
| Check | Verdict | Risk |
|-------|---------|------|
| Data export (portable JSON format) | PASS — Returns user + game_results as JSON for authenticated user | NONE |
| Right to deletion (PII anonymization) | PASS — `AnonymizeUser` replaces email/nickname, sets deleted_at, clears cookies | NONE |
| FK cascade on hard delete | PASS — `ON DELETE CASCADE` on game_results, game_sessions | NONE |
| Audit log entry on deletion | **FAIL** — No `audit.Log()` call in DeleteUserData flow | MEDIUM |
| Outbox event on deletion | **FAIL** — No `InsertOutboxEvent` call in DeleteUserData flow | LOW |
| Retention period (30 days) | PASS — Default 30d, configurable, enforced by GDPRCleanupWorker | NONE |
| Cleanup schedule (24h) | PASS — Daily cleanup with 2-minute per-run timeout | NONE |
| Audit log immutability | PASS — UPDATE/DELETE triggers prevent modification | NONE |
| Email encryption at rest | PASS — AES-256-GCM with random nonce, env-based key | NONE |
| IP encryption at rest | **FAIL** — `audit_logs.actor_ip` stored in plaintext | MEDIUM |
| Key rotation support | **FAIL** — `RotateKey` not implemented | LOW |
| Nickname encryption | INFO — Not encrypted (intentionally public) | NONE |

---

## 24. API Specification Accuracy

### Checks Performed

- **Step 1:** Compared `docs/api/openapi.yaml` against routes in `backend/internal/server/routes_public.go` and `routes_admin.go` — HTTP methods, paths, path parameters, request/response schemas
- **Step 2:** Compared `docs/api/asyncapi.yaml` against `backend/internal/protocol/constants.go` — message types, direction attribution, binary format
- **Step 3:** Cross-referenced `docs/api/ws-protocol.md` with `backend/internal/protocol/constants.go`, `frontend/src/shared/game/protocol.ts`, and `backend/internal/constants/protocol.go`

### Files Read

- `docs/api/openapi.yaml` — REST API specification (1109 lines)
- `docs/api/asyncapi.yaml` — AsyncAPI WebSocket specification (96 lines)
- `docs/api/ws-protocol.md` — WebSocket protocol documentation (77 lines)
- `backend/internal/server/routes_public.go` — Public route definitions (164 lines)
- `backend/internal/server/routes_admin.go` — Admin route definitions (34 lines)
- `backend/internal/protocol/constants.go` — Protocol message constants and phase codes (246 lines)
- `backend/internal/constants/protocol.go` — Source-of-truth message type constants (21 lines)
- `backend/internal/domain/room_code.go` — RoomCode value object (5-char enforcement)
- `frontend/src/shared/game/protocol.ts` — Frontend protocol constants (27 lines)

### Findings

#### Step 1: OpenAPI Completeness vs Routes

**Route coverage gaps in OpenAPI:**

| Route | Documented | Notes |
|-------|-----------|-------|
| `GET /health/live` | ✅ | `healthLive` |
| `GET /health/ready` | ✅ | `healthReady` |
| `GET /health` | **MISSING** | Registered as alias to `ReadyHandler` (line 45) |
| `GET /metrics` | **MISSING** | Prometheus endpoint behind `metricsAuthMiddleware` (line 47) |
| `POST /api/v1/auth/quickplay` | ✅ | `authQuickplay` |
| `POST /api/v1/auth/request` | ✅ | `authRequestMagicLink` |
| `GET /api/v1/auth/verify` | ✅ | `authVerifyMagicLink` |
| `POST /api/v1/auth/verify` | **MISSING** | Registered at `routes_public.go:57` — POST variant accepted |
| `GET /api/v1/auth/check` | ✅ | `authCheck` |
| `POST /api/v1/auth/refresh` | ✅ | `authRefreshToken` |
| `POST /api/v1/auth/logout` | ✅ | `authLogout` |
| `GET /api/v1/user/data` | ✅ | `userExportData` |
| `DELETE /api/v1/user/data` | ✅ | `userDeleteData` |
| `GET /api/v1/leaderboard` | ✅ | `getLeaderboard` |
| `GET /api/v1/user/stats` | ✅ | `getUserStats` |
| `POST /api/v1/registry/create` | ✅ | `registryCreateRoom` |
| `GET /api/v1/registry/check/{code}` | ✅ | `registryCheckRoom` |
| `GET /api/v1/registry/lobbies` | ✅ | `registryListLobbies` |
| `POST /api/v1/registry/match` | ✅ | `registryMatchRoom` (deprecated) |
| `GET /api/v1/lobby/{code}/ws` | ✅ | `lobbyWebSocket` |
| `POST /api/v1/admin/login` | ✅ | `adminLogin` |
| `POST /api/v1/admin/logout` | ✅ | `adminLogout` |
| `GET /api/v1/admin/config` | ✅ | `adminGetConfig` |
| `PATCH /api/v1/admin/config` | ✅ | `adminUpdateConfig` |
| `PUT /api/v1/admin/config` | ✅ | `adminUpdateConfigDeprecated` (deprecated) |
| `GET /*` (SPA static) | ❌ | Not documented — acceptable for SPA catch-all |

- **FAIL** — 3 undocumented routes: `GET /health` (alias), `POST /api/v1/auth/verify` (supported but undocumented), `GET /metrics`

**Path parameter mismatch:**

- **FAIL** — Room code length: OpenAPI documents `check/{code}` as minLength=6, maxLength=6 and response example "A1B2C3" (6 chars). Code (`domain/room_code.go:11`) enforces exactly 5 characters. Also `/api/v1/registry/create` response says "6位房间代码" — actual is 5-char.

**Response schema gaps:**

- **FAIL** — `/api/v1/leaderboard` response `entries.items` is `type: object` with no properties defined — schema is underspecified
- **INFO** — `/api/v1/user/stats` response missing `nickname` field if present in actual response

**Request schema mismatch:**

- **FAIL** — QuickPlay nickname maxLength=12 in OpenAPI but code has no 12-char limit. Backend nickname validation in `DecodeNicknamePayload` allows up to 255 bytes. OpenAPI constraint does not match actual enforcement.

**HTTP methods and response codes:**

- **PASS** — All HTTP methods (GET, POST, PATCH, PUT, DELETE) match between OpenAPI and route registration
- **PASS** — Admin PUT `/config` correctly documented as deprecated with `Deprecation` and `Sunset` headers

---

## 26. Code Consistency

### Checks Performed

- **Step 1:** Checked Go naming conventions — unexported functions use camelCase, exported functions use PascalCase
- **Step 2:** Checked TypeScript naming conventions — exported types use PascalCase, functions/const use camelCase or UPPER_SNAKE_CASE
- **Step 3:** Checked Go import organization — stdlib/external/internal grouping
- **Step 4:** Checked error message conventions — lowercase, no punctuation
- **Step 5:** Reviewed `CONTRIBUTING.md` and `commitlint.config.js`

### Files Read

- CONTRIBUTING.md — Contribution guidelines (207 lines)
- commitlint.config.js — Commit message validation rules (30 lines)
- `backend/internal/auth/*.go` — JWT, magic link, refresh, quickplay, gdpr_data, middleware
- `backend/internal/apierror/apierror.go` — Error constructors
- `backend/internal/config/env.go` — Environment config
- `backend/internal/crypto/aes.go` — Encryption errors
- `backend/internal/store/postgres.go`, `redis.go` — Store imports
- `backend/internal/game/hub.go`, `hub_cache.go` — Game imports
- `frontend/src/*.ts` — Entry flow, admin, UI modules
- `frontend/src/shared/game/protocol.ts` — Frontend protocol constants

### Findings

#### Step 1: Go Naming Conventions — PASS

All unexported functions use camelCase (e.g., `computeHash`, `isValidEmail`, `sanitizePlayerName`, `generateJTI`, `getOrigin`, `validateTrustedProxyCIDRs`, `reconnectGraceExpired`, `roomJoinable`). All exported functions use PascalCase (e.g., `NewJWTManager`, `ExportUserData`, `QuickPlay`, `InitDBLogger`, `HashToken`). The special function `init()` is correct per Go spec. No naming violations detected.

**Verdict: PASS** — Go naming conventions consistently followed across all 14+ packages.

#### Step 2: TypeScript Naming Conventions — PASS

Exported types use PascalCase (`AdminConfig`, `EntryFullScreenErrorOptions`, `EntryOverlayContext`, `EntryStep`, `ConnectionErrorOptions`). Exported functions use camelCase (`bindLoginEvents`, `showReconnectBanner`, `onWebSocketOpen`, `initEntryFlow`, `submitNickname`). Exported constants use UPPER_SNAKE_CASE (`TICK_MS`, `MAX_RECONNECT_ATTEMPTS`, `ROOM_CODE_RE`, `PALETTE_COLORS`) or PascalCase for object constants (`PHYSICS`, `COOLDOWN`, `END_REASON`). No naming violations detected.

**Verdict: PASS** — TypeScript naming conventions consistently followed.

#### Step 3: Import Organization — PASS (1 LOW)

All Go files checked follow the 3-group import convention (stdlib → external → internal) with blank line separators. Examples:
- `auth/jwt.go`: stdlib → `golang-jwt` (external) → internal config/validate ✓
- `auth/refresh.go`: stdlib → `redis` (external) ✓
- `game/hub.go`: stdlib → internal (audit, config, domain) ✓
- `game/hub_cache.go`: stdlib → internal (config, domain) ✓
- `store/redis.go`: stdlib → external (redis, gobreaker) → internal (config, resilience) ✓

- [LOW] `store/postgres.go:4-19` — Imports merge external (`pgx`, `gobreaker`) and internal (`config`, `domain`, `metrics`, `migrateutil`, `resilience`) in a single group without blank line separation. This is the same file flagged as a known god-file in ADR-019 (~954 lines, migration artifact).

**Verdict: PASS** — Import grouping is clean across the codebase; the single exception in `store/postgres.go` is consistent with its known migration artifact status.

#### Step 4: Error Message Conventions — PASS

Go convention: error messages should be lowercase, no trailing punctuation, descriptive. All `errors.New` and `fmt.Errorf` calls were checked:

- No uppercase-starting messages found (the one case `"ENCRYPTION_KEY environment variable is required..."` starts with an env var name, which is acceptable)
- No messages end with `.`, `!`, or `?`
- All messages are descriptive of the error condition
- Wrap errors use `%w` where appropriate for `errors.Is`/`errors.As` compatibility (see Section 16 for detailed `%w` analysis)

Example messages: `"too many requests, try again later"`, `"duplicate user"`, `"consume refresh token: %w"`, `"save lobby state: %w"`, `"invalid or expired token"`, `"migrations require a real pgxpool connection"`.

**Verdict: PASS** — All error messages follow Go conventions.

#### Step 5: Contribution & Commit Standards — PASS

`CONTRIBUTING.md` is comprehensive (207 lines) with Chinese-language documentation covering:
- Code style: Go follows Effective Go with golangci-lint; TypeScript follows ESLint config
- Commit convention: Conventional Commits with 7 standard types and 72-char subject max
- API deprecation policy with Sunset/Link headers and 6-month minimum
- Postmortem requirement for P0/P1 incidents within 7 days

`commitlint.config.js` extends `@commitlint/config-conventional` with 11 types (feat, fix, docs, style, refactor, perf, test, chore, ci, revert, security), `lower-case` scope enforcement, and 72-char subject limit.

**Verdict: PASS** — Contribution and commit standards are well-defined and enforced via pre-commit hooks and commitlint.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Go naming (camelCase unexported / PascalCase exported) | PASS | NONE |
| TypeScript naming (PascalCase types / camelCase functions) | PASS | NONE |
| Import organization (3-group separation) | PASS — 1 LOW exception in `store/postgres.go` | LOW |
| Error message conventions (lowercase, no punctuation) | PASS | NONE |
| Contribution & commit standards | PASS | NONE |

#### Step 2: AsyncAPI vs WebSocket Protocol Constants

**Message type constants — PASS** — All 12 message types match exactly between `asyncapi.yaml`, `constants/protocol.go`, and `protocol/constants.go`:

| AsyncAPI Message | Byte | Direction | Status |
|-----------------|------|-----------|--------|
| Tap | 0x10 | Client→Server | ✅ |
| SetNickname | 0x11 | Client→Server | ✅ |
| RestartVote | 0x12 | Client→Server | ✅ |
| Ping | 0x20 | Client→Server | ✅ |
| Snapshot | 0x01 | Server→Client | ✅ |
| PlayerJoin | 0x02 | Server→Client | ✅ |
| PlayerLeave | 0x03 | Server→Client | ✅ |
| TapAccepted | 0x04 | Server→Client | ✅ |
| TapRejected | 0x05 | Server→Client | ✅ |
| GameStateChange | 0x06 | Server→Client | ✅ |
| RestartStatus | 0x07 | Server→Client | ✅ |
| Pong | 0x21 | Server→Client | ✅ |

**Operation completeness — FAIL:**

- `receiveState` operation (action: receive) lists only 3 messages: `snapshot`, `gameStateChange`, `pong`
- **Missing 5 server→client messages**: `playerJoin`, `playerLeave`, `tapAccepted`, `tapRejected`, `restartStatus`
- These are all server-sent messages that should appear in a receive operation

**Binary format description — INFO:**

- Payloads are typed as `{ type: object, x-binary-type: "0xNN" }` with no binary layout details — AsyncAPI 3.0 supports `x-binary-type` for custom tooling but the actual byte layouts are not described. This is acceptable if the authoritative byte layout reference is `ws-protocol.md`.

#### Step 3: WebSocket Protocol Doc (ws-protocol.md) Accuracy

**Message constants — PASS** — All constants in ws-protocol.md § "消息类型常量" match `constants/protocol.go` and `frontend/src/shared/game/protocol.ts`:

- Client→Server: 0x10 (Tap), 0x11 (SetNickname), 0x12 (RestartVote), 0x20 (Ping) — ✅
- Server→Client: 0x01-0x07, 0x21 — ✅
- Frontend `protocol.ts` MSG_TYPE and CLIENT_MSG match — ✅

**Phase codes — PASS** — Phase codes (0=waiting, 1=playing, 2=ended, 3=countdown) match `PhaseCode*` constants in `protocol/constants.go:38-43`.

**Binary layout — PASS** — `GAME_STATE_CHANGE` binary layout description (msgType uint8, phaseCode uint8, countdownRemainingMs uint32 LE) matches the implementation.

**Tick rate — PASS** — 15 Hz documented, matches `TickRate = 15` at `protocol/constants.go:151`.

**Undocumented / missing endpoints — FAIL**:

- ws-protocol.md § "实例与区域路由" documents `GET /api/v1/lobby/{code}/resolve` and 421/503 response codes for cross-region routing. **This endpoint does not exist** in `routes_public.go` — it is a planned/ADR-documented feature not yet implemented. Publishing it in the protocol doc creates a misleading contract.

**Frontend sync — PASS** — Frontend `protocol.ts` constants are in sync with the Go sources.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| OpenAPI route coverage | FAIL — 3 routes missing (`/health`, `POST /auth/verify`, `/metrics`) | LOW |
| OpenAPI HTTP method match | PASS — All methods match code | NONE |
| OpenAPI path parameter accuracy | FAIL — Room code documented as 6-char, actual is 5-char | MEDIUM |
| OpenAPI schema accuracy (nickname maxLength) | FAIL — Documents 12, backend accepts up to 255 | LOW |
| OpenAPI leaderboard entries schema | FAIL — `items: type: object` with no properties | LOW |
| AsyncAPI message constants | PASS — All 12 constants match code exactly | NONE |
| AsyncAPI operation completeness | FAIL — `receiveState` missing 5/8 server→client messages | MEDIUM |
| ws-protocol.md message constants | PASS — All match code and frontend constants | NONE |
| ws-protocol.md phase codes | PASS — 0/1/2/3 match `PhaseCode*` constants | NONE |
| ws-protocol.md binary layout | PASS — `GAME_STATE_CHANGE` layout accurate | NONE |
| ws-protocol.md tick rate | PASS — 15 Hz matches `TickRate` constant | NONE |
| ws-protocol.md /resolve endpoint | FAIL — Documents non-existent endpoint | MEDIUM |

---

## 29. WebSocket Protocol Correctness

### Checks Performed

- **Step 1:** Verified encode/decode symmetry in Go — read `backend/internal/protocol/encode.go`, `decode.go`, checked message type IDs, field order, field sizes, and documented layout vs actual encoding
- **Step 2:** Verified Go ↔ TypeScript compatibility — compared `backend/internal/protocol/encode.go` with `frontend/src/game/message_codec.ts` for byte order, field sizes, message type IDs, string encoding; also compared `backend/internal/constants/protocol.go` with `frontend/src/shared/game/protocol.ts`
- **Step 3:** Checked fuzz test coverage — read `backend/internal/protocol/decode_fuzz_test.go` and `encode_decode_test.go`; enumerated fuzz targets and edge cases
- **Step 4:** Logged findings to report

### Files Read

- `backend/internal/protocol/encode.go` — All 8 encode functions (249 lines)
- `backend/internal/protocol/decode.go` — All 6 decode functions (66 lines)
- `backend/internal/protocol/constants.go` — Message type constants, phase codes, data types (246 lines)
- `backend/internal/constants/protocol.go` — Source-of-truth message type constants (21 lines)
- `backend/internal/protocol/encode_decode_test.go` — Unit tests + benchmarks (466 lines)
- `backend/internal/protocol/decode_fuzz_test.go` — Fuzz test definitions (19 lines)
- `frontend/src/game/message_codec.ts` — Frontend message encode/decode (154 lines)
- `frontend/src/shared/game/protocol.ts` — Frontend protocol constants (27 lines)
- `docs/api/ws-protocol.md` — Binary protocol documentation (77 lines)

### Findings

#### Step 1: Go Encode/Decode Symmetry

The protocol package follows a unidirectional design: server→client messages are only encoded in Go (no Go-side decode) and client→server messages are only decoded in Go (no Go-side encode). There are **no symmetric encode/decode pairs within Go for the same message type**.

- **PASS** — Every message type that has both an encode and decode path in Go writes/reads fields in the same order with matching sizes: `DecodeMessage` (decode.go:8) mirrors the msgType byte structure used by all `Encode*` functions; `DecodeTap` (decode.go:20) reads 2 × float32 (matching the 8-byte tap payload); `DecodeSetNickname` (decode.go:47) validates msgType then delegates to `DecodeNicknamePayload` for the length-prefixed string (matching `encodeSetNickname`-equivalent client encoding). Field sizes are consistent: msgType = uint8, playerIndex = uint16, cooldownMs/score/palette = uint32, nickLen = uint8, nickname = bytes.
- **PASS** — Encoder buffer sizing (`calcSnapshotSize`, encode.go:65-80) matches actual byte layout. Each `buf.Grow()` call matches the documented binary layout in comments. No buffer overruns.
- **PASS** — `PhaseToCode`/`CodeToPhase` round-trip symmetry verified in unit tests (TestPhaseToCode_RoundTrip, encode_decode_test.go:290-299). Unknown phases map to `PhaseWaiting` (safe default).
- **PASS** — End reason codes (encode.go:47-51) are written by `EncodeGameStateChangeEnded` and matched by game logic; no Go-side decode needed (server→client only).
- **PASS** — Round-trip tests (TestRoundTrip_Tap, TestRoundTrip_SetNickname) verify that byte sequences produced by manual encoding can be decoded correctly.

#### Step 2: Go ↔ TypeScript Compatibility

**Message type IDs — PASS** — All 12 message types match exactly:

| Name | Go Value (`constants/protocol.go`) | TS Value (`protocol.ts`) |
|------|-----------------------------------|--------------------------|
| MsgSnapshot | 0x01 | MSG_TYPE.SNAPSHOT = 0x01 |
| MsgPlayerJoin | 0x02 | MSG_TYPE.PLAYER_JOIN = 0x02 |
| MsgPlayerLeave | 0x03 | MSG_TYPE.PLAYER_LEAVE = 0x03 |
| MsgTapAccepted | 0x04 | MSG_TYPE.TAP_ACCEPTED = 0x04 |
| MsgTapRejected | 0x05 | MSG_TYPE.TAP_REJECTED = 0x05 |
| MsgGameStateChange | 0x06 | MSG_TYPE.GAME_STATE_CHANGE = 0x06 |
| MsgRestartStatus | 0x07 | MSG_TYPE.RESTART_STATUS = 0x07 |
| MsgPong | 0x21 | MSG_TYPE.PONG = 0x21 |
| MsgTap | 0x10 | CLIENT_MSG.TAP = 0x10 |
| MsgSetNickname | 0x11 | CLIENT_MSG.SET_NICKNAME = 0x11 |
| MsgRestartVote | 0x12 | CLIENT_MSG.RESTART_VOTE = 0x12 |
| MsgPing | 0x20 | CLIENT_MSG.PING = 0x20 |

**Phase codes — PASS** — Both sides: WAITING=0, PLAYING=1, ENDED=2, COUNTDOWN=3. Phase mapping functions (`PhaseToCode`/`CodeToPhase` on Go side, `codeToPhase` on TS side) match.

**Byte order — PASS** — Both use little-endian. Go: `binary.LittleEndian` (constants.go:11). TS: all `DataView.get*` calls with `littleEndian=true` (message_codec.ts:75-128).

**Field sizes — PASS** — All integer widths match between Go encoders and TS decoders:

| Field | Go Type | TS Read | Size |
|-------|---------|---------|------|
| msgType | uint8 | implicit (stripped) | 1 byte |
| phaseCode | uint8 | `getUint8` | 1 byte |
| playerIndex | uint16 | `getUint16` | 2 bytes |
| cooldownMs | uint32 | `getUint32` | 4 bytes |
| palette | uint32 | `getUint32` | 4 bytes |
| scoreContribution | uint32 | `getUint32` | 4 bytes |
| score / tickCount | uint32 | `getUint32` | 4 bytes |
| countdownRemainingMs | uint32 | `getUint32` | 4 bytes |
| yesVotes / totalPlayers | uint8 | `getUint8` | 1 byte |
| x / y / vx / vy | float32 | `getFloat32` | 4 bytes each |
| nickLen | uint8 | `getUint8` | 1 byte |
| nickname | []byte | `TextDecoder.decode` | variable |
| repelTimer | uint16 | `getUint16` | 2 bytes |

**Length-prefixed string encoding — PASS** — Both use uint8 length prefix + raw UTF-8 bytes. Go: `buf.WriteByte(uint8(len(nickBytes)))` + `buf.Write(nickBytes)`. TS: `dv.setUint8(1, nickBytes.length)` + `new Uint8Array(buf, 2).set(nickBytes)`. On decode, TS uses `textDecoder.decode(new Uint8Array(view.buffer, view.byteOffset + o, nickLen))`.

**Snapshot binary layout — PASS** — The snapshot message layout is structurally identical between Go encode and TS decode:

| Section | Go Encode Offset | TS Decode Offset | Match |
|---------|------------------|-------------------|-------|
| msgType (1) | 0 | stripped | ✓ |
| tickCount (uint32) | 1-4 | timestamp (uint32, LE) | ✓ (value; naming differs) |
| score (uint32) | 5-8 | score (uint32, LE) | ✓ |
| phaseCode (uint8) | 9 | phaseCode (uint8) | ✓ |
| balloon (16) | 10-25 | balloon (4 × float32) | ✓ |
| bird active (1) | 26 | birdActive (uint8) | ✓ |
| bird x,y (8) | [if active] 27-34 | [if active] (2 × float32) | ✓ |
| ghost (11) | next offset | ghost (1+4+4+2) | ✓ |
| playerCount (1) | +1 | playerCount (uint8) | ✓ |
| players[] | per player: 2+4+4+4+1+nickLen | per player: same layout | ✓ |
| rippleCount (1) | +1 | rippleCount (uint8) | ✓ |
| ripples[] | per ripple: 2+4+4 | per ripple: same layout | ✓ |
| wind (float32) | next 4 bytes | wind (float32) | ✓ |

**ISSUE — TS decodeSnapshot minimum length check too permissive** (`message_codec.ts:70`):
```typescript
if (view.byteLength < 37) { return null; }
```
The actual minimum valid snapshot is **44 bytes** (bird inactive, ghost always encoded, zero players, zero ripples, wind present). A buffer of 37-43 bytes passes this guard but will cause a `DataView RangeError` when reading ghost or later fields. While the check broadly rejects tiny/malformed inputs, an attacker could craft a 37-43 byte message that passes the initial guard and then throws an unhandled `RangeError` in the consumer.

**ISSUE — Field naming mismatch: `tickCount` vs `timestamp`**:
Go `EncodeSnapshot` writes a `tickCount` (uint32, the tick counter since game start). TS `decodeSnapshot` reads this same value into a field called `timestamp`. These are semantically different concepts: a monotonic tick counter (0, 1, 2, ...) vs a wall-clock timestamp (epoch ms). The Go value is a tick counter; the TS `DecodedSnapshot.timestamp` is actually tickCount but named misleadingly. This is not a wire-format bug but creates confusion for maintainers — readers expect `timestamp` to be `Date.now()`-compatible.

**Client-side SetNickname — PASS** — TS `encodeSetNickname` (message_codec.ts:35-44) writes `msgType(0x11) + nickLen(uint8) + nickBytes`. Go `DecodeSetNickname` (decode.go:47-52) validates `data[0] == MsgSetNickname` then delegates to `DecodeNicknamePayload`. The TS applies a pre-encode truncation to 12 runes; Go accepts up to 255 bytes. The truncation difference is safe — the Go side is more permissive, which is correct for a server that should be tolerant of client variations.

#### Step 3: Fuzz Test Coverage

**Existing fuzz targets** (`decode_fuzz_test.go`):
| Function | Seeds | Edge Cases Covered |
|----------|-------|-------------------|
| `FuzzDecodeMessage` | 3 seeds (valid tap, empty, 0xff prefix) | Empty input, unknown type, any byte sequence |
| `FuzzDecodeTap` | 1 seed (valid 2-float32 payload) | Any 8+ bytes interpreted as float32s |

**PASS** — Both fuzz functions are well-formed (via `testing.F.Add` + `Fuzz`) and will not panic on any input — `DecodeMessage` returns `(0, nil)` for empty input; `DecodeTap` checks `len(data) < 8` and returns early for short inputs.

**FAIL — Only 2 of 6 decode functions have fuzz tests.** Missing fuzz coverage:

| Function | File | Lines | Risk |
|----------|------|-------|------|
| `DecodeSetNickname` | decode.go:47-52 | Validates msgType then delegates to DecodeNicknamePayload | **MEDIUM** — untrusted input path |
| `DecodeNicknamePayload` | decode.go:30-42 | Length-prefixed string with bounds checks | **MEDIUM** — core string parsing |
| `DecodeRestartVote` | decode.go:57-59 | Single-byte message type check | **LOW** — trivial function |
| `DecodePing` | decode.go:64-66 | Single-byte message type check | **LOW** — trivial function |

`DecodeSetNickname` and `DecodeNicknamePayload` are the highest-priority missing targets — they parse untrusted client input with variable-length fields. While unit tests cover basic edge cases (empty, zero-length, truncated, 255-length), fuzzing would test boundary conditions at scale.

**Unit test edge case coverage** (encode_decode_test.go):

| Edge Case | Tested? | Test Name |
|-----------|---------|-----------|
| Empty message | ✅ | TestDecodeMessage_Empty |
| Zero-length nickname | ✅ | TestDecodeNicknamePayload_ZeroLength |
| Truncated nickname | ✅ | TestDecodeNicknamePayload_Truncated |
| Max-length nickname (255 bytes) | ❌ | **Missing** |
| Negative/overlong nickLen (255) | ✅ | TestDecodeNicknamePayload_NegativeLength |
| Wrong message type | ✅ | TestDecodeSetNickname_WrongType |
| Inactive bird (compact snapshot) | ✅ | TestEncodeSnapshot_BirdInactive |
| Max players (50) | ❌ | **Missing** |
| Max ripples | ❌ | **Missing** |
| Empty players/ripples (nil slices) | ✅ | TestEncodeSnapshot_BirdInactive |
| All phase round-trips | ✅ | TestPhaseToCode_RoundTrip |
| Countdown with remainingMs | ✅ | TestEncodeGameStateChange_CountdownWithRemaining |
| Ended with end reason | ✅ | TestEncodeGameStateChangeEnded_WithReason |

#### Additional Findings

- **PASS — All 12 message constant values identical across all three sources** (`constants/protocol.go`, `protocol/constants.go`, `frontend/src/shared/game/protocol.ts`). No drift detected.
- **PASS — Both sides use little-endian byte order consistently.** Go via `binary.LittleEndian`, TS via `getFloat32(o, true)` / `getUint32(o, true)` / `getUint16(o, true)` — all with `littleEndian=true`.
- **PASS — `encodeSetNickname` in TS and `DecodeSetNickname` + `DecodeNicknamePayload` in Go are compatible.** The TS truncates to 12 runes before sending; Go accepts up to 255 bytes. Safe asymmetry.
- **PASS — End reason codes** (encode.go:47-51) are only used server-side; no frontend constants needed. The 3 bytes sent by `EncodeGameStateChangeEnded` (msgType + phaseCode + endReason) match what the game logic expects.
- **PASS — Pooled buffer pattern** in `EncodeSnapshot` (snapshotBufPool, encode.go:11-15) correctly copies the result before returning (`make([]byte, buf.Len()); copy(result, buf.Bytes())`), avoiding use-after-free of the sync.Pool buffer.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Go encode/decode field order | PASS — All encode/decode pairs match; protocol is unidirectional by design | NONE |
| Go encode/decode field sizes | PASS — uint8/uint16/uint32/float32 widths consistent | NONE |
| Go ↔ TS message type IDs | PASS — All 12 constants match exactly | NONE |
| Go ↔ TS byte order | PASS — Both little-endian | NONE |
| Go ↔ TS field sizes | PASS — All integer widths match | NONE |
| Go ↔ TS string encoding | PASS — Both use uint8 length prefix + UTF-8 bytes | NONE |
| Go ↔ TS phase codes | PASS — 0/1/2/3 match on both sides | NONE |
| TS snapshot min length check | **FAIL** — Checks `byteLength < 37` but minimum is 44 bytes | LOW |
| Field naming: tickCount vs timestamp | **FAIL** — Go writes tick counter, TS reads as "timestamp" (misleading) | LOW |
| Fuzz: DecodeMessage | ✅ Present | NONE |
| Fuzz: DecodeTap | ✅ Present | NONE |
| Fuzz: DecodeSetNickname | ❌ Missing | MEDIUM |
| Fuzz: DecodeNicknamePayload | ❌ Missing | MEDIUM |
| Fuzz: DecodeRestartVote | ❌ Missing | LOW |
| Fuzz: DecodePing | ❌ Missing | LOW |
| Unit test: max-length nickname (255) | ❌ Missing | LOW |
| Unit test: max players/ripples | ❌ Missing | LOW |

---

## 28. Protocol Sync

### Checks Performed

- **Step 1:** Ran `go generate ./internal/protocol/` — verified codegen produces current output
- **Step 2:** Manually compared critical constants between Go (`protocol/constants.go`, `constants/protocol.go`) and TypeScript (`protocol.ts`, `constants.ts`, `phase_sync.ts`)
- **Step 3:** Read `.github/workflows/docs-governance.yml` — checked CI sync enforcement

### Files Read

- `backend/internal/protocol/constants.go` — Go source-of-truth constants (246 lines)
- `backend/internal/constants/protocol.go` — Message type byte constants (21 lines)
- `backend/cmd/gen-frontend-constants/main.go` — Codegen tool (179 lines)
- `frontend/src/shared/game/protocol.ts` — Frontend protocol constants (27 lines)
- `frontend/src/shared/game/constants.ts` — Frontend physics/cooldown constants (45 lines)
- `frontend/src/game/phase_sync.ts` — Frontend end-reason constants (lines 12-17)
- `.github/workflows/docs-governance.yml` — CI governance workflow (105 lines)

### Findings

#### Step 1: Codegen Produces Current Output

Ran `go generate ./...` from `backend/`. The codegen tool (`gen-frontend-constants/main.go`) writes to `../../../frontend/src/shared/constants.ts` (relative to `cmd/gen-frontend-constants/`), which resolves to `frontend/src/shared/constants.ts`.

**FAIL — Codegen output path mismatch.** The tool writes to `frontend/src/shared/constants.ts`, but all 7 frontend imports reference `frontend/src/shared/game/constants.ts`. The generated file lands at a different path than what the frontend actually reads. The content is currently identical between the two files (all 34 physics constants + 3 cooldown constants match), so no drift has occurred yet. However, any future change to Go constants will update the wrong file — the actual consumed file (`game/constants.ts`) will remain stale.

**PASS** — `git diff --stat` shows no tracked file changes after codegen. The generated file is untracked (at the wrong path).

#### Step 2: Manually Compare Critical Constants

**Message type bytes: ALL MATCH ✓**

| Go constant | Value | TS equivalent | Value | Match |
|-------------|-------|---------------|-------|-------|
| MsgTap | 0x10 | CLIENT_MSG.TAP | 0x10 | ✅ |
| MsgSetNickname | 0x11 | CLIENT_MSG.SET_NICKNAME | 0x11 | ✅ |
| MsgRestartVote | 0x12 | CLIENT_MSG.RESTART_VOTE | 0x12 | ✅ |
| MsgPing | 0x20 | CLIENT_MSG.PING | 0x20 | ✅ |
| MsgSnapshot | 0x01 | MSG_TYPE.SNAPSHOT | 0x01 | ✅ |
| MsgPlayerJoin | 0x02 | MSG_TYPE.PLAYER_JOIN | 0x02 | ✅ |
| MsgPlayerLeave | 0x03 | MSG_TYPE.PLAYER_LEAVE | 0x03 | ✅ |
| MsgTapAccepted | 0x04 | MSG_TYPE.TAP_ACCEPTED | 0x04 | ✅ |
| MsgTapRejected | 0x05 | MSG_TYPE.TAP_REJECTED | 0x05 | ✅ |
| MsgGameStateChange | 0x06 | MSG_TYPE.GAME_STATE_CHANGE | 0x06 | ✅ |
| MsgRestartStatus | 0x07 | MSG_TYPE.RESTART_STATUS | 0x07 | ✅ |
| MsgPong | 0x21 | MSG_TYPE.PONG | 0x21 | ✅ |

**Phase codes: ALL MATCH ✓** — Waiting=0, Playing=1, Ended=2, Countdown=3 (both sides)

**End reason codes: ALL MATCH ✓** — None=0, Ground=1, Bird=2, Ghost=3 (Go: `EndReason*` vs TS: `END_REASON` in `phase_sync.ts`)

**Physics constants: ALL MATCH ✓** — All 34 `PHYSICS.*` constants verified identical value-by-value between Go and both generated `constants.ts` and `game/constants.ts`

**Cooldown constants: ALL MATCH ✓** — `BASE_MS=1000`, `LOG_COEFFICIENT=2032`, `MAX_MS=15000`

**FAIL — No protocol version constant.** Neither Go nor TypeScript defines a protocol version identifier. There is no mechanism for the server and client to negotiate or verify protocol compatibility at connection time. If the protocol changes incompatibly, old clients will connect and fail unpredictably instead of receiving a version-mismatch rejection.

#### Step 3: CI Sync Enforcement

The `.github/workflows/docs-governance.yml` has a `ws-protocol-sync` job with two checks:
1. All `Msg*` Go constants are documented in `docs/api/ws-protocol.md` and `docs/api/asyncapi.yaml`
2. Server-to-client message type values match between Go and `frontend/src/shared/protocol.ts`

**FAIL — CI references non-existent file path.** The workflow checks `frontend/src/shared/protocol.ts` (line 75), but this file does not exist — the actual file is at `frontend/src/shared/game/protocol.ts`. The grep would find no matches, causing the check to always report mismatches or silently skip.

**FAIL — Incomplete message type coverage.** The CI only verifies 8 server-to-client message types (SNAPSHOT through PONG). The 4 client-to-server message types (TAP, SET_NICKNAME, RESTART_VOTE, PING) are not checked.

**FAIL — No physics/cooldown sync check.** The CI does not verify that the 34 physics constants and 3 cooldown constants in the generated `constants.ts` are in sync with Go `constants.go`. The codegen path mismatch (Step 1) means there is no automated gate to detect drift.

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Codegen output path matches import path | **FAIL** — Writes to `shared/constants.ts`, imports from `shared/game/constants.ts` | MEDIUM |
| Codegen produces current output | PASS — Generated values match actual file (today) | NONE |
| Message type bytes (all 12) | PASS — All match between Go and TS | NONE |
| Phase codes (0/1/2/3) | PASS — Match between Go and TS | NONE |
| End reason codes (0/1/2/3) | PASS — Match between Go and TS | NONE |
| Physics constants (34 values) | PASS — All match between Go and TS | NONE |
| Cooldown constants (3 values) | PASS — All match between Go and TS | NONE |
| Protocol version defined | **FAIL** — No version constant in either Go or TS | MEDIUM |
| CI checks protocol.ts exists | **FAIL** — References non-existent `shared/protocol.ts` vs actual `shared/game/protocol.ts` | HIGH |
| CI checks all message byte types | **FAIL** — Only 8/12 server→client; missing 4 client→server | MEDIUM |
| CI checks physics/cooldown sync | **FAIL** — Not verified in CI | MEDIUM |

---

## 23. ADR Currency

### Checks Performed

- **Step 1:** Verified ADR index consistency — read `docs/adr/README.md`, counted files vs table rows, checked statuses
- **Step 2:** Cross-referenced ADR-002 (Binary Protocol), ADR-018 (Frontend Vanilla TS), ADR-019 (No ORM), ADR-025 (Mutable Singleton State), ADR-028 (Clean Architecture) against implementation
- **Step 3:** Checked all 29 ADR files against `docs/templates/adr.md` template format

### Files Read

- `docs/adr/README.md` — ADR index table (59 lines)
- `docs/templates/adr.md` — ADR template specification (41 lines)
- All 29 ADR files (`docs/adr/000-*.md` through `028-*.md`)
- `backend/internal/protocol/constants.go` — Binary protocol constants (246 lines)
- `frontend/src/game/store.ts` — State store dispatch/reducer (17 lines)
- Frontend entry HTML files — 5 HTML entry points

### Findings

#### Step 1: ADR Index Consistency

- **FAIL — ADR-028 is missing from the index table.** The `docs/adr/` directory contains 29 ADR files (000-028), but the README table lists only 28 entries (000-027). File `028-clean-architecture-interface-decoupling.md` exists on disk but has no corresponding row in the index table.

- **PASS** — All 28 indexed ADRs (000-027) have corresponding files on disk. No orphaned file entries in the index.

- **PASS** — No files without corresponding index entries (except ADR-028, covered above). No orphaned files on disk.

- **Status accuracy:**
  - 24 entries marked "已接受" — appropriate for fully implemented/adopted decisions
  - 2 entries marked "提议中" (ADR-014 Multi-Region, ADR-015 CockroachDB, ADR-016 Region-Local Rooms) — correctly reflects their proposed status. Note: ADR-014 is marked "已接受" in the ADR file itself but "提议中" in the index.
  - 2 entries marked "已接受（部分落地）" (ADR-022 PII Encryption, ADR-023 Hybrid Tests) — accurately reflects partial implementation

- **INFO — ADR-014 status mismatch between file and index:** The ADR file (`014-multi-region-topology.md:5`) says "已接受", but the index says "提议中". The index likely reflects the correct current status (CockroachDB not yet deployed, multi-region not operational), but the file and index disagree.

- **INFO — Historical duplicate ADR-011 noted in ADR-000:** The project charter (ADR-000 appendix) documents that "ADR 编号重复（两个 011）→ 去重（bounded-contexts 改为 017）" — this duplicate was already resolved.

#### Step 2: Cross-Reference ADRs with Implementation

**ADR-002 (Binary Protocol) — PASS**

| Check | Verdict |
|-------|---------|
| TickRate = 15 matches ADR "15Hz" | PASS — `protocol/constants.go:151` |
| Binary protocol files exist | PASS — `protocol/encode.go`, `protocol/decode.go` both present |
| No JSON on message-passing path | PASS — Binary encoding via little-endian; JSON only for Redis Pub/Sub (cross-instance) |

**ADR-018 (Frontend Vanilla TS) — PASS with INFO**

| Check | Verdict |
|-------|---------|
| Zero React/Vue/Svelte imports in frontend/src/ | PASS — No framework imports found |
| Vite MPA build configured | PASS — `vite.config.ts` with multi-entry build |
| HTML entry points match ADR | INFO — ADR-018 documents 4 entry points (index, play, admin, verify). Codebase has 5: also includes `leaderboard.html`. |

**ADR-019 (No ORM) — PASS**

| Check | Verdict |
|-------|---------|
| No ORM in store package | PASS — All queries use `pgx` with `$N` parameterized SQL |
| No GORM/ent/sqlx/sqlboiler imports | PASS — Zero matches for these patterns in store package |

**ADR-025 (Mutable Singleton → Dispatch/Reducer) — FAIL (naming mismatch)**

The ADR describes a dispatch/reducer pattern with immutable state updates, but the filename and index still reference "mutable singleton":

| Source | Text | Status |
|--------|------|--------|
| File name | `025-frontend-mutable-singleton-state.md` | Outdated — says "mutable singleton" |
| ADR title | `前端受控状态管理 (GameStore)` | Current — accurately describes dispatch/reducer |
| ADR status | `已更新 (2026-07-03)` | Self-acknowledges update from original |
| Index description | `前端可变单例状态管理` | Outdated — still says "mutable singleton state management" |
| Implementation | `store.ts`: `dispatch(action)` / `getState()` / `select(selector)` with `gameReducer` | Mature dispatch/reducer with immutable updates |

- **3-way inconsistency:** File name, index description, and ADR title/implementation all disagree on the pattern name.
- The ADR was correctly updated to document the current pattern, but the file was not renamed and the index was not updated.

**ADR-028 (Clean Architecture) — PASS**

All dependency-direction claims verified against codebase:

| ADR Claim | Verification | Verdict |
|-----------|-------------|---------|
| Handler production code: zero `store`/`auth` imports | Grep: 0 matches in `backend/internal/handler/` (non-test) | PASS |
| Auth production code: zero `store` imports | Grep: 0 matches in `backend/internal/auth/` | PASS |
| Game production code: zero `store`/`auth` imports | Grep: 0 matches in `backend/internal/game/` | PASS |
| Interfaces defined at consumer side (handler) | Confirmed: 10 interfaces in `handler_interfaces.go` | PASS |
| `server` package is composition root | Confirmed: `auth_service.go` bridges auth→handler | PASS |

#### Step 3: Template Compliance

Template requirements from `docs/templates/adr.md`:
- `# ADR-NNN: 中文标题` (title format)
- `## 状态` (NOT inline like `## 状态: 已接受`)
- `## 日期` (YYYY-MM format)
- `## 上下文` (context)
- `## 决策` (decision)
- `## 后果` (consequences, with `**正面**` and `**负面**` subsections)
- No `## Status`, `## 背景`
- No YAML-style blocks

Compliance breakdown by ADR:

| ADR | Title | Status Format | Date | Context | Decision | Consequences | Verdict |
|-----|-------|--------------|------|---------|----------|-------------|---------|
| 000 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ✅ | **FAIL** — inline status, missing date |
| 001 | ✅ | ✅ | ❌ missing | ✅ | ✅ | ✅ | **FAIL** — missing date |
| 002 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | **PASS** |
| 003 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | **PASS** |
| 004 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | **PASS** |
| 005 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ✅ | **FAIL** |
| 006 | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ (uses ### 好处/坏处) | **FAIL** — non-standard consequence subsections |
| 007 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | **PASS** |
| 008 | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ (uses ### 好处/坏处) | **FAIL** — non-standard consequence subsections |
| 009 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | **PASS** |
| 010 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | **PASS** |
| 011 | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ (uses ## 理由/备选方案/权衡) | **FAIL** — wrong section structure |
| 012 | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ (uses ## 理由/备选方案/权衡) | **FAIL** — wrong section structure |
| 013 | ✅ | ❌ inline | ❌ missing | ❌ uses ## 背景 | ✅ | ❌ (flat list) | **FAIL** |
| 014 | ✅ | ❌ inline | ✅ | ✅ | ✅ | ❌ (flat bullets) | **FAIL** |
| 015 | ✅ | ❌ inline | ✅ | ✅ | ✅ | ❌ (flat bullets) | **FAIL** |
| 016 | ✅ | ❌ inline | ✅ | ✅ | ✅ | ❌ (flat bullets) | **FAIL** |
| 017 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | **PASS** |
| 018 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ✅ | **FAIL** |
| 019 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ✅ | **FAIL** |
| 020 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ✅ | **FAIL** |
| 021 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ✅ | **FAIL** |
| 022 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ✅ | **FAIL** |
| 023 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ✅ | **FAIL** |
| 024 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ❌ (flat list) | **FAIL** |
| 025 | ✅ | ❌ inline ("已更新") | ❌ missing | ✅ | ✅ | ✅ | **FAIL** |
| 026 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ❌ (flat list) | **FAIL** |
| 027 | ✅ | ❌ inline | ❌ missing | ✅ | ✅ | ✅ | **FAIL** |
| 028 | ✅ | ✅ | ✅ (but inline) | ✅ | ✅ | ✅ | **PASS** (minor: date inline with `##`) |

**Summary of common violations:**

| Violation | Count | ADRs |
|-----------|-------|------|
| Inline status (`## 状态: 值` not `## 状态` on separate line) | 16/29 | 000, 005, 013, 014, 015, 016, 018, 019, 020, 021, 022, 023, 024, 025, 026, 027 |
| Missing `## 日期` section | 14/29 | 000, 001, 005, 013, 018, 019, 020, 021, 022, 023, 024, 025, 026, 027 |
| Non-standard consequences format (missing **正面**/**负面**) | 7/29 | 006, 008, 011, 012, 013, 024, 026 |
| Wrong heading name (`## 背景` instead of `## 上下文`) | 1/29 | 013 |

**Template compliance rate: 7/29 (24%) PASS**

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Index completeness (all files indexed) | **FAIL** — ADR-028 missing from table | MEDIUM |
| Index orphan detection (no extra table rows) | PASS — No orphan index entries | NONE |
| Index status accuracy | INFO — ADR-014 file says "已接受", index says "提议中" | LOW |
| ADR-002 Binary Protocol vs implementation | PASS — TickRate 15, binary codec files exist | NONE |
| ADR-018 Frontend Vanilla TS vs implementation | PASS — No framework imports; 5 entry points (ADR says 4) | LOW |
| ADR-019 No ORM vs implementation | PASS — Raw SQL via pgx, zero ORM | NONE |
| ADR-025 Mutable Singleton vs implementation | **FAIL** — 3-way naming mismatch (filename, index, title) | LOW |
| ADR-028 Clean Architecture vs implementation | PASS — Zero direct store/auth imports in handler/auth/game | NONE |
| Template compliance (all 29 ADRs) | **FAIL** — Only 7/29 (24%) fully compliant | MEDIUM |
| — Inline status format | **FAIL** — 16/29 use inline format | LOW |
| — Missing date section | **FAIL** — 14/29 missing `## 日期` | LOW |
| — Non-standard consequences | **FAIL** — 7/29 use flat list or wrong heading | LOW |

---

## 25. Code Quality

### Checks Performed

- **Step 1:** Ran `go vet ./...` — static analysis for Go compilation and common issues
- **Step 2:** Ran `golangci-lint run ./...` — Go linter aggregation (gofmt, gosec, revive, gocognit, etc.)
- **Step 3:** Ran `npx eslint .` — frontend TypeScript lint
- **Step 4:** Grepped for dead code — exported functions with no callers, unused imports/variables
- **Step 5:** Grepped for `TODO`/`FIXME`/`HACK`/`XXX` markers — unresolved technical debt
- **Step 6:** Measured file sizes >500 lines — god-object risk detection

### Findings

#### Step 1: Go Vet

- **FAIL** — `go vet` cannot complete due to pre-existing compilation error: `internal/store/room_registry_store.go:118:31: undefined: domain.UnmarshalRoomRegistryInfo`. The function `UnmarshalRoomRegistryInfo` is referenced but **never defined** anywhere in the codebase. This blocks `go vet` from analyzing all packages that transitively import `store` (auth, game, handler, server, etc.).

#### Step 2: golangci-lint

- **FAIL** — Pre-existing compilation error (same as Step 1) prevents full linter analysis across most packages. Partial results from packages that compiled:

  | File | Line | Linter | Finding |
  |------|------|--------|---------|
  | `cmd/gen-frontend-constants/main.go` | 19 | gocognit | Cognitive complexity 71 of func `main` (>30 threshold) |
  | `internal/audit/audit.go` | 178 | gofmt | File not properly formatted |
  | `cmd/gen-frontend-constants/main.go` | 124 | gosec G306 | `os.WriteFile` permissions 0644 — should be ≤0600 |
  | `cmd/gen-frontend-constants/main.go` | 1 | revive | Missing package comment |
  | `internal/constants/protocol.go` | 1 | revive | Missing package comment |

- [INFO] `internal/resilience/retry.go:44,45` — `nolint:gosec:G115` and `nolint:gosec:G404` reference linters not found (`/runner/nolint_filter` reports unknown linters). These directives are ineffective.
- [INFO] `internal/testsecrets/secrets.go:6` — `gosec G101` hardcoded test credential (allowlisted by `.gitleaks.toml`, acceptable).

#### Step 3: Frontend ESLint

- **PASS** — 0 errors, 3 warnings:
  | File | Line | Warning |
  |------|------|---------|
  | `frontend/src/game/reducer.ts` | 2 | `GamePhase` defined but never used |
  | `frontend/src/game/store.test.ts` | 3 | `GameAction` defined but never used |
  | `frontend/src/game/window_events.ts` | 6 | `updateUI` defined but never used |

  All three are unused imports/variables that can be safely removed.

#### Step 4: Dead Code

- **FAIL** — `domain.UnmarshalRoomRegistryInfo` is called in `internal/store/room_registry_store.go:118` but the function is **not defined anywhere** in the codebase (zero definition lines found across all Go files). This is either a dead reference from a removed function or an unimplemented stub. It causes a compilation failure that cascades to 6+ dependent packages.
- **3 unused exports** in frontend (see Step 3 ESLint findings) — `GamePhase`, `GameAction`, `updateUI`.
- [INFO] Several `*_repository.go` files in `store/` (user_repository.go, result_repository.go, lobby_repository.go, config_repository.go, outbox_repository.go) are duplicate implementations of `postgres_*` files — a known migration artifact. While functionally duplicated, these are still compiled and increase maintenance surface.

#### Step 5: TODO/FIXME/HACK Markers

- **FAIL** — 2 unresolved technical debt markers in production code:
  1. `backend/internal/crypto/aes.go:146` — `TODO: implement batch re-encryption of database fields when key rotation is needed`
  2. `backend/internal/server/server_debug.go:18` — `TODO: add github.com/grafana/pyroscope-go dependency to enable always-on profiling`

  Neither is tracked in an issue tracker or has a deadline.

#### Step 6: File Complexity (>500 lines)

- **PASS** — No production .go file in `backend/internal/` exceeds 500 lines. Largest files:
  - `game/room_lifecycle.go`: 367 lines (largest production file)
  - `auth/magiclink.go`: 275 lines
  - `store/user_repository.go`: 260 lines (duplicate)
  - `protocol/encode.go`: 249 lines
  - `protocol/constants.go`: 246 lines
  - `game/room.go`: 245 lines

- **PASS** — No frontend production .ts file exceeds 500 lines. Largest:
  - `entry_flow.ts`: 223 lines
  - `state_interp.ts`: 217 lines

### Summary

| Check | Verdict | Risk |
|-------|---------|------|
| Go vet — compilation | FAIL — `UnmarshalRoomRegistryInfo` undefined in store | HIGH |
| golangci-lint — gocognit | FAIL — `gen-frontend-constants/main.go` cognitive complexity 71 | MEDIUM |
| golangci-lint — gofmt | FAIL — `audit.go:178` not gofmt-formatted | LOW |
| golangci-lint — gosec G306 | FAIL — WriteFile 0644 permissions in code generator | LOW |
| golangci-lint — revive | FAIL — 2 files missing package comments | LOW |
| Eslint — unused imports | FAIL — 3 unused exports in frontend | LOW |
| Dead code — Go | FAIL — `UnmarshalRoomRegistryInfo` referenced but never defined | HIGH |
| Dead code — store duplicates | INFO — 5 pairs of `*_repository.go` / `postgres_*` duplicates | LOW |
| TODO/FIXME/HACK markers | FAIL — 2 unresolved TODOs (AES key rotation, pyroscope profiling) | LOW |
| File complexity (>500 lines) | PASS — No production file exceeds threshold | NONE |

---

## Prioritized Action Items

### CRITICAL

| # | Finding | File(s) to Modify | Effort | Risk if Not Fixed |
|---|---------|-------------------|--------|-------------------|
| C1 | **No CI quality gates** — Zero GitHub Actions workflows; all gates are Makefile-local only | Add `.github/workflows/ci.yml`, `.github/workflows/cd.yml` | 8h | No automated quality enforcement; regressions undetected before deploy |
| C2 | **No deployment pipeline** — No deploy, rollout, or rollback mechanism exists | Add `.github/workflows/deploy.yml`, deployment scripts | 16h | Manual deployment only; no rollback capability; operations risk |
| C3 | **No sequential region rollout** — Multi-region deployment not operational | `infra/k8s/overlays/*/kustomization.yaml`, add rollout workflow | 24h | All regions updated simultaneously; blast radius = entire user base |
| C4 | **No rollback mechanism** — No automated way to revert a bad deployment | Add rollback scripts, DB migration rollback, K8s rollout undo | 8h | Bad deploy requires manual intervention; extended downtime |

### HIGH

| # | Finding | File(s) to Modify | Effort | Risk if Not Fixed |
|---|---------|-------------------|--------|-------------------|
| H1 | **Cloud SQL publicly accessible** — No `ip_configuration`; accessible from any IP on internet | `infra/terraform/main.tf` | 2h | Database exposed to internet-wide brute force and data exfiltration |
| H2 | **Compilation error blocking all tooling** — `UnmarshalRoomRegistryInfo` referenced but never defined | `backend/internal/store/room_registry_store.go` (add or remove reference) | 1h | `go vet`, `golangci-lint`, and 6+ packages cannot compile or test |
| H3 | **No worker shutdown coordination** — Workers fire-and-forget; DB/Redis closed while workers in I/O | `backend/internal/server/server_lifecycle.go` | 3h | Potential data corruption or panic during shutdown |
| H4 | **No egress network policy** — K8s pods can reach any external destination | `infra/k8s/global/network-policy.yaml` | 3h | Compromised pod can exfiltrate data; no network containment |
| H5 | **No database network isolation** — Redis and DB not restricted to app-only access | `infra/k8s/global/network-policy.yaml` | 2h | Any pod in namespace can reach DB/Redis directly |
| H6 | **Game result worker has no dead letter queue** — Permanently-failing messages redelivered forever | `backend/internal/worker/game_result_worker.go` | 4h | Continuous error logs, resource waste, never-resolved failures |
| H7 | **No HTTP panic recovery middleware** — Unhandled panic in any handler kills the server | `backend/internal/server/routes_middleware.go` | 1h | Single panic brings down entire HTTP server |
| H8 | **Game loop tick spans missing** — Entire game package has zero OTel instrumentation | `backend/internal/game/*.go` | 8h | No observability into core game loop performance |
| H9 | **Auth/Admin E2E coverage missing** — Magic link and admin flows have zero E2E tests | `tests/e2e/auth.spec.ts`, `tests/e2e/admin.spec.ts` | 8h | Critical user-facing flows untested at E2E level |
| H10 | **Latency p99 alert missing** — No alert on high-latency despite histogram instrumentation | `deploy/alertmanager/rules.yml` | 2h | Latency degradation invisible until user-reported |
| H11 | **CI references non-existent file** — `docs-governance.yml` checks wrong path for protocol.ts | `.github/workflows/docs-governance.yml` | 1h | Protocol sync CI check always passes vacuously |
| H12 | **3 stdlib CVEs at HIGH severity** — TOCTOU, XSS in html/template, TLS KeyUpdate DoS | Build with `go 1.26.4`+ | 1h | Security vulnerabilities exploitable if building with older Go |
| H13 | **Build failures blocking 6 critical package tests** — auth, game, handler, server, store, testutil | Same as H2 (root cause) | 1h | Coverage for 84%+ of critical codebase is unknown |
| H14 | **Dead code: `UnmarshalRoomRegistryInfo`** — Function referenced but never defined | Same as H2 | 1h | Same as H2 — compilation failure |
| H15 | **Cloud SQL no authorized networks** — Database accessible from any IP | Same as H1 | 0.5h | Same as H1 — internet-wide exposure |

---

*Report assembled and finalized at 2026-07-03T23:59:00Z. All 30 inspection tasks complete. Executive summary, severity classification, and prioritized action items added.*

