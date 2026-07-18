# Codebase Radical Simplification Design

**Date:** 2026-07-19
**Scope:** Full codebase (Go backend + TypeScript frontend)
**Target:** 20-33% total line reduction (12,000-18,000 lines from 54,249)
**Approach:** Bottom-up cleanup → error fix → dedup → package merge → test trim → architecture

## Current State

| Metric | Value |
|--------|-------|
| Total code | 54,249 lines |
| Go production | 14,337 lines (133 files) |
| Go tests | 27,278 lines (129 files) |
| TypeScript source | ~5,690 lines (46 files) |
| TypeScript tests | ~4,155 lines (39 files) |
| Go test/source ratio | 1.93x |
| Go packages | 29 |
| `go vet` failures | 9 packages |
| ESLint errors | 4 errors + 15 warnings |

## Phase 1: Dead Code Removal (~500 lines)

### 1.1 Delete stub files

Empty files containing only `package <name>`:

| File | Lines | Reason |
|------|-------|--------|
| `internal/domain/room_directory.go` | 1 | Planned feature never implemented |
| `internal/game/hub_multiregion.go` | 1 | Multi-region stub, never implemented |
| `internal/handler/lobby_ws_proxy.go` | 1 | WS proxy stub, never implemented |
| `internal/handler/resolve.go` | 1 | Never implemented |
| `internal/crypto/bcrypt.go` | 1 | Package declaration only; bcrypt used directly in handler |
| `internal/domain/validator.go` | 5 | NicknameValidator interface; consider merging into domain |

Also delete corresponding stub test files if they exist.

### 1.2 Delete deprecated files

| File | Reason |
|------|--------|
| `frontend/src/shared/ui/toast.ts` | `@deprecated` re-export shim; update mock in `lifecycle.test.ts` |

### 1.3 Clean stale config

| File | Change |
|------|--------|
| `frontend/vitest.config.ts` | Remove coverage exclusion entries for non-existent files (`connection_ui.ts`, `waiting_tips.ts`) |

### 1.4 Clean backend artifacts

Delete non-source files in `backend/`:
- `coverage*` files (~15 files)
- `lint-output*.txt` files (~4 files)
- These are build artifacts, not source code

### Verification

```
cd backend && go build ./...
cd frontend && npm run lint
```

## Phase 2: Fix All Pre-existing Errors (~200 lines changed)

### 2.1 Backend `go vet` failures

| Package | File:Line | Error | Fix |
|---------|-----------|-------|-----|
| `game` | `hub_test.go:337-339` | Orphaned code outside function | Delete 3 lines (`_, _ = h.CreateRoom(...)` + 2 closing braces) |
| `crypto` | `aes_email_test.go:68` | `EncryptEmailForStorage` undefined | Change to `EncryptPIIForStorage` |
| `domain` | `errors_test.go:17,35` | `ErrConflict` undefined | Change to `ErrDuplicateUser` or `ErrValidation` |
| `protocol` | `decode_fuzz_test.go:35` | `DecodeSetNickname` undefined | Change to `DecodeNicknamePayload` |
| `resilience` | `retry_test.go:33` | `ExternalAPIRetry` undefined | Change to `DefaultDBRetry` |
| `telemetry` | `telemetry_test.go:25` | `isOTLPInsecure` undefined | Remove or refactor test |
| `store` | `postgres_results_test.go:52` | `EndGameAndRecordResults` undefined | Update to current method name |
| `server` | `routes_coverage_test.go:39` | `store.NewRedisClusterFromStores` undefined | Update to correct constructor |
| `metrics` | `record_extra_test.go:33` | `metrics.BeginAuth` undefined | Change to `RecordAuth` |

### 2.2 Frontend ESLint errors

**Fix in `eslint.config.js`:** Add `*_test_setup.ts` to the test-file glob pattern for `no-explicit-any` relaxation:

```javascript
// Before
{ files: ['src/**/*.test.ts', 'src/**/*.property.test.ts'] }
// After
{ files: ['src/**/*.test.ts', 'src/**/*.property.test.ts', 'src/**/*_test_setup.ts'] }
```

### 2.3 Frontend ESLint warnings

| File | Line | Fix |
|------|------|-----|
| `lifecycle.test.ts` | 11-12, 23 | Remove unused `mockUpdateUI`, `mockGenerateRandomNickname`, `mockSetupNicknameInput` |
| `window_events.test.ts` | 18-22 | Remove unused `mockUpdateUI`, `mockGenerateRandomNickname`, `mockCopyCode`, `mockRefreshLayout`, `mockShowFallbackErrorScreen` |
| `ui_update.ts` | 16 | Remove unused `syncRestartVoteUI` import |

### Verification

```
cd backend && go vet ./...
cd frontend && npm run lint
```

Expected: 0 errors, 6 warnings (only test `any` warnings remaining)

## Phase 3: Deduplicate Code (~300 lines deleted)

### 3.1 `apierror` package vs `domain/apierror.go`

**Current state:** Two character-for-character identical implementations.

**Action:** Delete `internal/apierror/` package entirely. Keep `internal/domain/apierror.go`.

**Import updates:** Grep for `apierror` imports across all Go files. Expected consumers:
- `handler/*.go` — change `"...apierror"` to `"...domain"`
- `middleware/*.go` — change `"...apierror"` to `"...domain"`

**Lines deleted:** ~76 (source) + ~55 (tests) = ~131

### 3.2 `idgen.UUID()` vs `domain.UUID()`

**Current state:** `internal/idgen/uuid.go` has identical `UUID()` function to `internal/domain/idgen.go`.

**Action:** Delete `internal/idgen/uuid.go`. Keep `domain.UUID()`.

**Import updates:** Grep for `idgen` imports. Change `idgen.UUID()` → `domain.UUID()`.

**Lines deleted:** ~17

### 3.3 Frontend leaderboard duplication

**Current state:** `leaderboard.ts:23` and `index_leaderboard.ts:29` contain near-identical DOM construction for leaderboard items.

**Action:** Extract shared `renderLeaderboardEntry()` to `shared/ui/leaderboard_utils.ts`. Update both files to import from it.

**Lines saved:** ~30 (net, after accounting for new shared file)

### 3.4 Frontend thin wrappers

**Current state:** `ui_common.ts::generateRandomNickname()` wraps `ui_elements.ts::pickRandomNickname()`. `ui_common.ts::refreshLayout()` wraps `renderer.ts::resizeCanvas()`.

**Action:** Inline these wrappers at call sites. Delete the wrapper functions.

**Lines deleted:** ~10

### Verification

```
cd backend && go build ./... && go test ./...
cd frontend && npm run lint && npm run typecheck && npm test
```

## Phase 4: Package Consolidation (~800-1200 lines deleted)

### 4.1 Merge `idgen` → `domain`

**Source:** `internal/idgen/` (17 lines + tests)
**Target:** `internal/domain/idgen.go`

**Steps:**
1. Move any remaining content from `idgen/uuid.go` to `domain/idgen.go` (if not already there)
2. Delete `internal/idgen/` directory
3. Update all imports: `"...idgen"` → `"...domain"`
4. Run `go build ./...`

### 4.2 Merge `validate` → `domain`

**Source:** `internal/validate/` (51 lines + tests)
**Target:** `internal/domain/` (create `validate.go` or merge into existing)

**Steps:**
1. Move `sanitize_nickname.go` and `nickname_adapter.go` content into `domain/`
2. Delete `internal/validate/` directory
3. Update imports
4. Run `go build ./...`

### 4.3 Merge `slogctx` → `util`

**Source:** `internal/slogctx/` (24 lines)
**Target:** `internal/util/`

**Steps:**
1. Move `slogctx.go` content into `util/`
2. Delete `internal/slogctx/` directory
3. Update imports
4. Run `go build ./...`

### 4.4 Merge `requestctx` → `middleware`

**Source:** `internal/requestctx/` (41 lines)
**Target:** `internal/middleware/` (already handles HTTP concerns)

**Steps:**
1. Move `client_ip.go` content into `middleware/`
2. Delete `internal/requestctx/` directory
3. Update imports
4. Run `go build ./...`

### 4.5 Merge `resilience` → `store`

**Source:** `internal/resilience/` (206 lines)
**Target:** `internal/store/`

**Steps:**
1. Move `circuit_breaker.go` and `retry.go` into `store/`
2. Delete `internal/resilience/` directory
3. Update imports (resilience is only used by store)
4. Run `go build ./...`

### Verification

After each merge:
```
cd backend && go build ./... && go vet ./... && go test ./...
```

## Phase 5: Test Reduction (~10,000-15,000 lines deleted)

### 5.1 Delete failed tests (9 files, ~3,000 lines)

These tests reference functions/types that no longer exist. They are dead code:

| File | Lines | Reason |
|------|-------|--------|
| `internal/crypto/aes_email_test.go` | ~150 | Tests `EncryptEmailForStorage` (renamed) |
| `internal/domain/errors_test.go` | ~80 | Tests `ErrConflict` (removed) |
| `internal/protocol/decode_fuzz_test.go` | ~100 | Tests `DecodeSetNickname` (renamed) |
| `internal/resilience/retry_test.go` | ~200 | Tests `ExternalAPIRetry` (removed) |
| `internal/telemetry/telemetry_test.go` | ~120 | Tests `isOTLPInsecure` (removed) |
| `internal/store/postgres_results_test.go` | ~300 | Tests `EndGameAndRecordResults` (removed) |
| `internal/server/routes_coverage_test.go` | ~150 | Tests `store.NewRedisClusterFromStores` (removed) |
| `internal/metrics/record_extra_test.go` | ~100 | Tests `metrics.BeginAuth` (removed) |
| `internal/game/hub_test.go` (orphaned section) | ~20 | Orphaned code fragment |

**Decision rule:** Phase 2 fixes tests that reference renamed functions (can be updated). Phase 5.1 deletes tests that reference truly removed functionality (no equivalent exists). If Phase 2 already fixed a test in this list, skip that entry here.

### 5.2 Consolidate over-split test files

Merge related small test files:

| Merge Target | Absorb | Rationale |
|-------------|--------|-----------|
| `auth/auth_test.go` | `auth/jwt_test.go`, `auth/jwt_verify_test.go` | JWT tests belong with auth tests |
| `store/postgres_repos_test.go` | `store/postgres_lobbies_query_test.go`, `store/postgres_leaderboard_test.go` | All PG query tests |
| `frontend/game/entry_flow.test.ts` | `entry_flow_dom.test.ts` | Entry flow + its DOM tests |

### 5.3 Simplify test factories

**`internal/testutil/` (416 lines):**

Review mock stores and test container setup. Simplify:
- Remove mock implementations that are only used once
- Consolidate mock store creation into a single `NewMockDeps()` function
- Remove unnecessary test helper abstractions

### 5.4 Trim repetitive test cases

For each major test file, identify test cases that test the same function from slightly different angles with high overlap. Keep:
- Happy path
- Key error paths (2-3 most important)
- Edge cases that caught real bugs

Remove:
- Redundant variations of the same error path
- Tests that only verify "function doesn't panic" (covered by `go test` anyway)
- Overly granular assertion tests (e.g., testing each field of a response separately when a snapshot test exists)

### 5.5 Evaluate property-based tests

3 frontend property-based tests (`fast-check`):
- `reducer.property.test.ts` (189 lines)
- `message_codec.property.test.ts` (86 lines)
- `snapshot_decode.property.test.ts` (123 lines)

**Decision:** If unit tests for these functions already exist and pass, consider removing the property-based tests. They add maintenance burden for marginal coverage gain.

### Target

| Metric | Before | After |
|--------|--------|-------|
| Go tests | 27,278 lines | ~15,000 lines (-45%) |
| TS tests | 4,155 lines | ~2,800 lines (-33%) |
| Total tests | 31,433 lines | ~17,800 lines (-43%) |

### Verification

```
cd backend && go test ./... -cover
cd frontend && npm test
```

Target: 70%+ coverage maintained

## Phase 6: Architecture Simplification (~500-1000 lines deleted)

### 6.1 Merge `game/hub.go` + `game/hub_ops.go`

**Current:** 795 lines across 2 files.
**Action:** Merge into single `hub.go`. The split was likely from the refactor that extracted `WSSession`.

### 6.2 Consolidate `server` route files

**Current:** `routes.go`, `routes_public.go`, `routes_admin.go`, `routes_middleware.go` (4 files, ~280 lines total).
**Action:** Merge into single `routes.go`.

### 6.3 Consolidate `middleware` files

**Current:** 12 files, some very small (18-40 lines).
**Action:** Merge small files:
- `logging.go` + `prometheus.go` + `metrics.go` → `observability.go`
- `auth_util.go` → `auth_middleware.go`
- `response_recorder.go` → keep (used by multiple middleware)

### 6.4 Simplify `config/env.go` validation

**Current:** 282 lines with extensive validation.
**Action:** Review if validation can be simplified (e.g., struct tags + validator library instead of manual checks).

### Verification

```
cd backend && go build ./... && go vet ./... && go test ./...
cd frontend && npm run lint && npm test
```

## Final Metrics

| Metric | Before | After (est.) | Reduction |
|--------|--------|-------------|-----------|
| Go production | 14,337 | ~12,500 | -13% |
| Go tests | 27,278 | ~15,000 | -45% |
| TS source | 5,690 | ~5,300 | -7% |
| TS tests | 4,155 | ~2,800 | -33% |
| **Total** | **54,249** | **~38,000** | **~30%** |

## Risk Mitigation

1. **Each phase is independently verifiable** — `go build`, `go vet`, `go test`, `npm run lint`, `npm test`
2. **Phases 1-3 are low risk** — deleting dead code, fixing errors, merging identical code
3. **Phase 4 (package merge) requires careful import updates** — use `grep` to find all imports before moving
4. **Phase 5 (test reduction) is the highest risk** — verify coverage after each batch of deletions
5. **Phase 6 is architecture-only** — no behavior changes, only file organization

## Order of Execution

Phases must be executed in order (1→2→3→4→5→6) because:
- Phase 2 fixes errors that Phase 5 might otherwise delete
- Phase 3 deduplication simplifies Phase 4 package merges
- Phase 4 package merges simplify Phase 5 test imports
- Phase 6 architecture changes are easiest after all other cleanup
