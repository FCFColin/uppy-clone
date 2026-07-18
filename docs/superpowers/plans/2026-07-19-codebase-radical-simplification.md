# Codebase Radical Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce the codebase from 54,249 lines to ~38,000 lines (~30% reduction) by removing dead code, fixing errors, deduplicating, consolidating packages, trimming tests, and simplifying architecture.

**Architecture:** Bottom-up cleanup in 6 ordered phases: dead code → error fix → dedup → package merge → test trim → architecture. Each phase is independently verifiable. Phases must execute in order because later phases depend on earlier ones.

**Tech Stack:** Go 1.22+ (backend), TypeScript/Vite/Vitest (frontend), PostgreSQL, Redis, WebSocket binary protocol

## Global Constraints

- Go backend: `go build ./...`, `go vet ./...`, `go test ./...` must pass after each phase
- TypeScript frontend: `npm run lint`, `npm run typecheck`, `npm test` must pass after each phase
- Do not change production behavior — only how code is organized
- Each commit should be a single, reviewable change
- Coverage target: 70%+ after test reduction

---

## Phase 1: Dead Code Removal

### Task 1.1: Delete empty stub files

**Files:**
- Delete: `backend/internal/domain/room_directory.go`
- Delete: `backend/internal/game/hub_multiregion.go`
- Delete: `backend/internal/handler/lobby_ws_proxy.go`
- Delete: `backend/internal/handler/resolve.go`
- Delete: `backend/internal/crypto/bcrypt.go`
- Delete: `backend/internal/crypto/bcrypt_test.go` (tests deleted function, covered by `handler/admin_test.go`)
- **Keep:** `backend/internal/domain/validator.go` — NOT a stub. It defines `NicknameValidator` interface used by `domain/nickname.go` and implemented by `validate/adapter.go`. Already wired up.

**Steps:**

- [ ] **Step 1: Verify each file is truly empty (only package declaration)**

Read each file to confirm it contains only `package <name>` and no actual code.

- [ ] **Step 2: Check no other files import these packages**

Run: `cd backend && rg "room_directory|hub_multiregion|lobby_ws_proxy|\"resolve\"" --include="*.go" -l`
Expected: Only the stub files themselves match.

- [ ] **Step 3: Delete the stub files**

```bash
cd backend
rm internal/domain/room_directory.go
rm internal/game/hub_multiregion.go
rm internal/handler/lobby_ws_proxy.go
rm internal/handler/resolve.go
rm internal/crypto/bcrypt.go
rm internal/crypto/bcrypt_test.go
```

- [ ] **Step 5: Verify build**

Run: `cd backend && go build ./...`
Expected: PASS (no output)

- [ ] **Step 6: Commit**

```bash
cd backend
git add -A
git commit -m "chore: delete empty stub files that were never implemented"
```

---

### Task 1.2: Delete deprecated frontend toast shim

**Files:**
- Delete: `frontend/src/shared/ui/toast.ts`
- Modify: `frontend/src/game/lifecycle.test.ts` (update mock path)

**Steps:**

- [ ] **Step 1: Read `frontend/src/shared/ui/toast.ts` to confirm it's a re-export shim**

Expected: ~2 lines, `@deprecated` comment, re-exports from `utils.ts`.

- [ ] **Step 2: Check who imports from `toast.ts`**

Run: `cd frontend && rg "shared/ui/toast" src/ -l`
Expected: Only `lifecycle.test.ts` (mocking it).

- [ ] **Step 3: Update `lifecycle.test.ts` mock path**

In `lifecycle.test.ts`, find the mock for `'../shared/ui/toast.js'` and either:
- Remove the mock entirely if the test doesn't use showToast, OR
- Change the mock path to `'../shared/ui/utils.js'`

- [ ] **Step 4: Delete the deprecated file**

```bash
cd frontend
rm src/shared/ui/toast.ts
```

- [ ] **Step 5: Verify**

Run: `cd frontend && npm run lint && npm run typecheck && npm test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd frontend
git add -A
git commit -m "chore: delete deprecated toast.ts re-export shim"
```

---

### Task 1.3: Clean stale vitest.config.ts entries

**Files:**
- Modify: `frontend/vitest.config.ts`

**Steps:**

- [ ] **Step 1: Read `frontend/vitest.config.ts`**

Find the coverage exclusion list.

- [ ] **Step 2: Remove entries for non-existent files**

Remove `'src/game/connection_ui.ts'` and `'src/game/waiting_tips.ts'` from the coverage exclusion array.

- [ ] **Step 3: Verify**

Run: `cd frontend && npm test`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd frontend
git add vitest.config.ts
git commit -m "chore: remove stale coverage exclusion entries from vitest config"
```

---

### Task 1.4: Clean backend build artifacts

**Files:**
- Delete: `backend/coverage*` files (~15)
- Delete: `backend/lint-output*.txt` files (~4)

**Steps:**

- [ ] **Step 1: List all artifact files**

Run: `cd backend && ls coverage* lint-output*`
Expected: List of files to delete.

- [ ] **Step 2: Delete them**

```bash
cd backend
rm -f coverage* lint-output*
```

- [ ] **Step 3: Verify `.gitignore` covers these**

Check `backend/.gitignore` or root `.gitignore` includes `coverage*` and `lint-output*`.

- [ ] **Step 4: Commit**

```bash
cd backend
git add -A
git commit -m "chore: delete build artifacts (coverage files, lint output)"
```

---

## Phase 2: Fix All Pre-existing Errors

### Task 2.1: Fix `game/hub_test.go` orphaned code

**Files:**
- Modify: `backend/internal/game/hub_test.go:337-339`

**Steps:**

- [ ] **Step 1: Read lines 330-345 of `hub_test.go`**

Identify the orphaned code outside any function.

- [ ] **Step 2: Delete the 3 orphaned lines**

Remove `_, _ = h.CreateRoom(context.Background())` and the two closing braces `}` that sit outside any function body.

- [ ] **Step 3: Verify**

Run: `cd backend && go vet ./internal/game/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd backend
git add internal/game/hub_test.go
git commit -m "fix: remove orphaned code fragment in hub_test.go"
```

---

### Task 2.2: Fix `crypto/aes_email_test.go` undefined symbol

**Files:**
- Modify: `backend/internal/crypto/aes_email_test.go:68`

**Steps:**

- [ ] **Step 1: Read the test file around line 68**

Find `EncryptEmailForStorage` reference.

- [ ] **Step 2: Check what the current function name is**

Run: `cd backend && rg "func Encrypt" internal/crypto/ --include="*.go"`
Expected: Should show `EncryptPIIForStorage`.

- [ ] **Step 3: Update the test to use the correct function name**

Replace `EncryptEmailForStorage` with `EncryptPIIForStorage` in the test.

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./internal/crypto/ -v -run TestEncrypt`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend
git add internal/crypto/aes_email_test.go
git commit -m "fix: update crypto test to use renamed EncryptPIIForStorage"
```

---

### Task 2.3: Fix `domain/errors_test.go` undefined symbol

**Files:**
- Modify: `backend/internal/domain/errors_test.go:17,35`

**Steps:**

- [ ] **Step 1: Read the test file**

Find `ErrConflict` references.

- [ ] **Step 2: Check what sentinel errors exist**

Run: `cd backend && rg "var Err" internal/domain/errors.go`
Expected: Should show `ErrDuplicateUser`, `ErrNotFound`, `ErrValidation`.

- [ ] **Step 3: Replace `ErrConflict` with appropriate existing error**

Replace `ErrConflict` with `ErrDuplicateUser` (most semantically similar).

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./internal/domain/ -v -run TestError`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend
git add internal/domain/errors_test.go
git commit -m "fix: update domain test to use existing ErrDuplicateUser sentinel"
```

---

### Task 2.4: Fix `protocol/decode_fuzz_test.go` undefined symbol

**Files:**
- Modify: `backend/internal/protocol/decode_fuzz_test.go:35`

**Steps:**

- [ ] **Step 1: Read the test file around line 35**

Find `DecodeSetNickname` reference.

- [ ] **Step 2: Check current function name**

Run: `cd backend && rg "func Decode" internal/protocol/ --include="*.go"`
Expected: Should show `DecodeNicknamePayload`.

- [ ] **Step 3: Update the test**

Replace `DecodeSetNickname` with `DecodeNicknamePayload`.

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./internal/protocol/ -v -run Fuzz`
Expected: PASS (or skip if fuzz tests need special invocation)

- [ ] **Step 5: Commit**

```bash
cd backend
git add internal/protocol/decode_fuzz_test.go
git commit -m "fix: update protocol fuzz test to use renamed DecodeNicknamePayload"
```

---

### Task 2.5: Fix `resilience/retry_test.go` undefined symbol

**Files:**
- Modify: `backend/internal/resilience/retry_test.go:33`

**Steps:**

- [ ] **Step 1: Read the test file around line 33**

Find `ExternalAPIRetry` reference.

- [ ] **Step 2: Check what retry policies exist**

Run: `cd backend && rg "func.*Retry" internal/resilience/ --include="*.go"`
Expected: Should show `DefaultDBRetry`, `DefaultRedisRetry`.

- [ ] **Step 3: Update the test**

Replace `ExternalAPIRetry` with `DefaultDBRetry` (or the most appropriate existing policy).

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./internal/resilience/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend
git add internal/resilience/retry_test.go
git commit -m "fix: update resilience test to use existing retry policy"
```

---

### Task 2.6: Fix `telemetry/telemetry_test.go` undefined symbol

**Files:**
- Modify: `backend/internal/telemetry/telemetry_test.go:25`

**Steps:**

- [ ] **Step 1: Read the test file around line 25**

Find `isOTLPInsecure` reference.

- [ ] **Step 2: Check if the function exists in production code**

Run: `cd backend && rg "isOTLPInsecure" internal/telemetry/ --include="*.go"`
Expected: Only matches in test file.

- [ ] **Step 3: Decide fix — remove the test or refactor**

If the function was removed from production code, delete the test case that references it. If the test has other valid cases, keep those.

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./internal/telemetry/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend
git add internal/telemetry/telemetry_test.go
git commit -m "fix: remove telemetry test case referencing deleted isOTLPInsecure"
```

---

### Task 2.7: Fix `store/postgres_results_test.go` undefined method

**Files:**
- Modify: `backend/internal/store/postgres_results_test.go:52`

**Steps:**

- [ ] **Step 1: Read the test file around line 52**

Find `EndGameAndRecordResults` reference.

- [ ] **Step 2: Check what methods exist on ResultRepository**

Run: `cd backend && rg "func.*ResultRepository" internal/store/ --include="*.go"`
Expected: Should show current method names.

- [ ] **Step 3: Update the test to use the correct method**

Replace `EndGameAndRecordResults` with the current method name (likely `RecordGameResult` or similar).

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./internal/store/ -v -run TestResult`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend
git add internal/store/postgres_results_test.go
git commit -m "fix: update store test to use current ResultRepository method"
```

---

### Task 2.8: Fix `server/routes_coverage_test.go` undefined function

**Files:**
- Modify: `backend/internal/server/routes_coverage_test.go:39`

**Steps:**

- [ ] **Step 1: Read the test file around line 39**

Find `store.NewRedisClusterFromStores` reference.

- [ ] **Step 2: Check what Redis constructors exist**

Run: `cd backend && rg "func New.*Redis" internal/store/ --include="*.go"`
Expected: Should show current constructor names.

- [ ] **Step 3: Update the test**

Replace with the correct constructor function.

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./internal/server/ -v -run TestRoutes`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend
git add internal/server/routes_coverage_test.go
git commit -m "fix: update server test to use correct Redis constructor"
```

---

### Task 2.9: Fix `metrics/record_extra_test.go` undefined function

**Files:**
- Modify: `backend/internal/metrics/record_extra_test.go:33`

**Steps:**

- [ ] **Step 1: Read the test file around line 33**

Find `metrics.BeginAuth` reference.

- [ ] **Step 2: Check what metrics functions exist**

Run: `cd backend && rg "func Record" internal/metrics/ --include="*.go"`
Expected: Should show `RecordAuth`.

- [ ] **Step 3: Update the test**

Replace `BeginAuth` with `RecordAuth`.

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./internal/metrics/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend
git add internal/metrics/record_extra_test.go
git commit -m "fix: update metrics test to use RecordAuth instead of removed BeginAuth"
```

---

### Task 2.10: Fix frontend ESLint config for test setup files

**Files:**
- Modify: `frontend/eslint.config.js`

**Steps:**

- [ ] **Step 1: Read `frontend/eslint.config.js`**

Find the `no-explicit-any` override for test files.

- [ ] **Step 2: Add `*_test_setup.ts` to the test-file glob**

Change:
```javascript
{ files: ['src/**/*.test.ts', 'src/**/*.property.test.ts'] }
```
To:
```javascript
{ files: ['src/**/*.test.ts', 'src/**/*.property.test.ts', 'src/**/*_test_setup.ts'] }
```

- [ ] **Step 3: Verify**

Run: `cd frontend && npm run lint`
Expected: 0 errors (warnings may remain for test `any` usage)

- [ ] **Step 4: Commit**

```bash
cd frontend
git add eslint.config.js
git commit -m "fix: include test setup files in ESLint test-file relaxation"
```

---

### Task 2.11: Fix frontend ESLint warnings

**Files:**
- Modify: `frontend/src/game/lifecycle.test.ts`
- Modify: `frontend/src/game/window_events.test.ts`
- Modify: `frontend/src/game/ui_update.ts`

**Steps:**

- [ ] **Step 1: Remove unused mocks from `lifecycle.test.ts`**

Read lines 10-25. Remove `mockUpdateUI`, `mockGenerateRandomNickname`, `mockSetupNicknameInput` declarations if unused.

- [ ] **Step 2: Remove unused mocks from `window_events.test.ts`**

Read lines 15-25. Remove `mockUpdateUI`, `mockGenerateRandomNickname`, `mockCopyCode`, `mockRefreshLayout`, `mockShowFallbackErrorScreen` declarations if unused.

- [ ] **Step 3: Remove unused import from `ui_update.ts`**

Read line 16. Remove `syncRestartVoteUI` from the import statement.

- [ ] **Step 4: Verify**

Run: `cd frontend && npm run lint`
Expected: 0 errors, only `any` warnings in test files

- [ ] **Step 5: Commit**

```bash
cd frontend
git add -A
git commit -m "fix: remove unused variables and imports flagged by ESLint"
```

---

## Phase 3: Deduplicate Code

### Task 3.1: Delete duplicate `apierror` package

**Files:**
- Delete: `backend/internal/apierror/` (entire directory)
- Modify: All files importing `"...apierror"` → `"...domain"`

**Steps:**

- [ ] **Step 1: Find all imports of `apierror` package**

Run: `cd backend && rg "\".*apierror\"" --include="*.go" -l`
Expected: List of files importing `apierror`.

- [ ] **Step 2: Verify `domain/apierror.go` has the same functions**

Run: `cd backend && rg "func " internal/domain/apierror.go`
Compare with `internal/apierror/apierror.go`.

- [ ] **Step 3: Update all imports**

For each file found in Step 1, change the import path from `"uppy-clone/internal/apierror"` to `"uppy-clone/internal/domain"`. Also update any qualified calls (e.g., `apierror.BadRequest(...)` → `domain.BadRequest(...)`).

- [ ] **Step 4: Delete the `apierror` package**

```bash
cd backend
rm -rf internal/apierror/
```

- [ ] **Step 5: Verify build**

Run: `cd backend && go build ./...`
Expected: PASS

- [ ] **Step 6: Verify tests**

Run: `cd backend && go test ./...
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd backend
git add -A
git commit -m "refactor: delete duplicate apierror package, use domain/apierror.go"
```

---

### Task 3.2: Delete duplicate `idgen` package

**Files:**
- Delete: `backend/internal/idgen/` (entire directory)
- Modify: All files importing `"...idgen"` → `"...domain"`

**Steps:**

- [ ] **Step 1: Find all imports of `idgen` package**

Run: `cd backend && rg "\".*idgen\"" --include="*.go" -l`

- [ ] **Step 2: Verify `domain/idgen.go` has `UUID()` function**

Run: `cd backend && rg "func UUID" internal/domain/idgen.go`

- [ ] **Step 3: Update all imports**

Change `"uppy-clone/internal/idgen"` → `"uppy-clone/internal/domain"`. Update qualified calls: `idgen.UUID()` → `domain.UUID()`.

- [ ] **Step 4: Delete the `idgen` package**

```bash
cd backend
rm -rf internal/idgen/
```

- [ ] **Step 5: Verify**

Run: `cd backend && go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd backend
git add -A
git commit -m "refactor: delete duplicate idgen package, use domain.UUID()"
```

---

### Task 3.3: Deduplicate frontend leaderboard rendering

**Files:**
- Create: `frontend/src/shared/ui/leaderboard_utils.ts`
- Modify: `frontend/src/leaderboard.ts`
- Modify: `frontend/src/index_leaderboard.ts`

**Steps:**

- [ ] **Step 1: Read both leaderboard files to identify the duplicated DOM construction**

Compare `appendLeaderboardItem()` in `leaderboard.ts` with `renderEntries()` in `index_leaderboard.ts`.

- [ ] **Step 2: Extract shared `renderLeaderboardEntry()` function**

Create `frontend/src/shared/ui/leaderboard_utils.ts`:
```typescript
export interface LeaderboardEntry {
  rank: number;
  score: number;
  lobbyCode: string;
  endedAt?: string;
}

export function renderLeaderboardEntry(entry: LeaderboardEntry): HTMLLIElement {
  const li = document.createElement('li');
  li.className = 'leaderboard-item';
  // ... shared DOM construction logic
  return li;
}
```

- [ ] **Step 3: Update `leaderboard.ts` to import from shared**

Replace local DOM construction with call to `renderLeaderboardEntry()`.

- [ ] **Step 4: Update `index_leaderboard.ts` to import from shared**

Replace local DOM construction with call to `renderLeaderboardEntry()`.

- [ ] **Step 5: Verify**

Run: `cd frontend && npm run lint && npm run typecheck && npm test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd frontend
git add -A
git commit -m "refactor: extract shared leaderboard entry rendering to shared/ui"
```

---

### Task 3.4: Inline frontend thin wrappers

**Files:**
- Modify: `frontend/src/game/ui_common.ts`
- Modify: Call sites of `generateRandomNickname()` and `refreshLayout()`

**Steps:**

- [ ] **Step 1: Find all callers of `generateRandomNickname()`**

Run: `cd frontend && rg "generateRandomNickname" src/ -l`

- [ ] **Step 2: Inline at each call site**

Replace `generateRandomNickname()` with `pickRandomNickname()` (from `ui_elements.ts`).

- [ ] **Step 3: Find all callers of `refreshLayout()`**

Run: `cd frontend && rg "refreshLayout" src/ -l`

- [ ] **Step 4: Inline at each call site**

Replace `refreshLayout()` with `resizeCanvas()` (from `renderer.ts`).

- [ ] **Step 5: Delete the wrapper functions from `ui_common.ts`**

Remove `generateRandomNickname()` and `refreshLayout()` function definitions.

- [ ] **Step 6: Verify**

Run: `cd frontend && npm run lint && npm run typecheck && npm test`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd frontend
git add -A
git commit -m "refactor: inline thin wrapper functions in ui_common.ts"
```

---

## Phase 4: Package Consolidation

### Task 4.1: Merge `validate` → `domain`

**Note:** `idgen` was already deleted in Task 3.2. This task merges `validate` only.

**Files:**
- Move: `backend/internal/validate/sanitize_nickname.go` → `backend/internal/domain/`
- Move: `backend/internal/validate/nickname_adapter.go` → `backend/internal/domain/`
- Delete: `backend/internal/validate/` directory
- Modify: All files importing `"...validate"`

**Steps:**

- [ ] **Step 1: Read the validate package files**

Understand what functions/types are exported.

- [ ] **Step 2: Find all imports of `validate`**

Run: `cd backend && rg "\".*validate\"" --include="*.go" -l`

- [ ] **Step 3: Move content to `domain/`**

Copy the function bodies into `domain/` (create new file or merge into existing).

- [ ] **Step 4: Update all imports**

Change `"uppy-clone/internal/validate"` → `"uppy-clone/internal/domain"`. Update qualified calls.

- [ ] **Step 5: Delete the `validate` package**

```bash
cd backend
rm -rf internal/validate/
```

- [ ] **Step 6: Verify**

Run: `cd backend && go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd backend
git add -A
git commit -m "refactor: merge validate package into domain"
```

---

### Task 4.2: Merge `slogctx` → `util`

**Files:**
- Move: `backend/internal/slogctx/slogctx.go` → `backend/internal/util/`
- Delete: `backend/internal/slogctx/` directory
- Modify: All files importing `"...slogctx"`

**Steps:**

- [ ] **Step 1: Read `slogctx.go`**

Understand what it exports.

- [ ] **Step 2: Find all imports**

Run: `cd backend && rg "\".*slogctx\"" --include="*.go" -l`

- [ ] **Step 3: Move to `util/`**

Copy content into `util/` directory.

- [ ] **Step 4: Update all imports**

Change `"uppy-clone/internal/slogctx"` → `"uppy-clone/internal/util"`.

- [ ] **Step 5: Delete the `slogctx` package**

```bash
cd backend
rm -rf internal/slogctx/
```

- [ ] **Step 6: Verify**

Run: `cd backend && go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd backend
git add -A
git commit -m "refactor: merge slogctx package into util"
```

---

### Task 4.3: Merge `requestctx` → `middleware`

**Files:**
- Move: `backend/internal/requestctx/client_ip.go` → `backend/internal/middleware/`
- Delete: `backend/internal/requestctx/` directory
- Modify: All files importing `"...requestctx"`

**Steps:**

- [ ] **Step 1: Read `client_ip.go`**

Understand what it exports.

- [ ] **Step 2: Find all imports**

Run: `cd backend && rg "\".*requestctx\"" --include="*.go" -l`

- [ ] **Step 3: Move to `middleware/`**

Copy content into `middleware/` directory.

- [ ] **Step 4: Update all imports**

Change `"uppy-clone/internal/requestctx"` → `"uppy-clone/internal/middleware"`.

- [ ] **Step 5: Delete the `requestctx` package**

```bash
cd backend
rm -rf internal/requestctx/
```

- [ ] **Step 6: Verify**

Run: `cd backend && go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd backend
git add -A
git commit -m "refactor: merge requestctx package into middleware"
```

---

### Task 4.4: Merge `resilience` → `store`

**Files:**
- Move: `backend/internal/resilience/circuit_breaker.go` → `backend/internal/store/`
- Move: `backend/internal/resilience/retry.go` → `backend/internal/store/`
- Delete: `backend/internal/resilience/` directory
- Modify: All files importing `"...resilience"`

**Steps:**

- [ ] **Step 1: Read the resilience package files**

Understand what's exported.

- [ ] **Step 2: Find all imports**

Run: `cd backend && rg "\".*resilience\"" --include="*.go" -l`
Expected: Only `store/` files import it.

- [ ] **Step 3: Move to `store/`**

Copy files into `store/` directory.

- [ ] **Step 4: Update imports**

Change `"uppy-clone/internal/resilience"` → `"uppy-clone/internal/store"`.

- [ ] **Step 5: Delete the `resilience` package**

```bash
cd backend
rm -rf internal/resilience/
```

- [ ] **Step 6: Verify**

Run: `cd backend && go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd backend
git add -A
git commit -m "refactor: merge resilience package into store (circuit breaker + retry)"
```

---

## Phase 5: Test Reduction

### Task 5.1: Delete tests referencing removed functionality

**Files:**
- Delete: `backend/internal/crypto/aes_email_test.go` (if not fixed in Phase 2)
- Delete: `backend/internal/domain/errors_test.go` (if not fixed in Phase 2)
- Delete: `backend/internal/protocol/decode_fuzz_test.go` (if not fixed in Phase 2)
- Delete: `backend/internal/resilience/retry_test.go` (if not fixed in Phase 2)
- Delete: `backend/internal/telemetry/telemetry_test.go` (if not fixed in Phase 2)
- Delete: `backend/internal/store/postgres_results_test.go` (if not fixed in Phase 2)
- Delete: `backend/internal/server/routes_coverage_test.go` (if not fixed in Phase 2)
- Delete: `backend/internal/metrics/record_extra_test.go` (if not fixed in Phase 2)

**Note:** Only delete tests that were NOT successfully fixed in Phase 2. If Phase 2 fixed them, skip this task.

**Steps:**

- [ ] **Step 1: Check which tests were fixed in Phase 2**

Run: `cd backend && go vet ./...`
If a package still has vet errors, its test may need deletion.

- [ ] **Step 2: For each unfixed test, delete it**

```bash
cd backend
# Only delete files that still have broken references
rm -f internal/crypto/aes_email_test.go  # if still broken
rm -f internal/domain/errors_test.go      # if still broken
# ... etc
```

- [ ] **Step 3: Verify**

Run: `cd backend && go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd backend
git add -A
git commit -m "test: delete tests referencing removed/renamed functions"
```

---

### Task 5.2: Trim frontend property-based tests

**Files:**
- Evaluate: `frontend/src/game/reducer.property.test.ts` (189 lines)
- Evaluate: `frontend/src/game/message_codec.property.test.ts` (86 lines)
- Evaluate: `frontend/src/game/snapshot_decode.property.test.ts` (123 lines)

**Steps:**

- [ ] **Step 1: Check if corresponding unit tests exist and pass**

Run: `cd frontend && npm test -- --reporter=verbose 2>&1 | grep -E "PASS|FAIL"`
Check if `reducer.test.ts`, `message_codec.test.ts`, `snapshot_decode.test.ts` exist and pass.

- [ ] **Step 2: If unit tests exist, delete property-based tests**

```bash
cd frontend
rm -f src/game/reducer.property.test.ts
rm -f src/game/message_codec.property.test.ts
rm -f src/game/snapshot_decode.property.test.ts
```

- [ ] **Step 3: Remove `fast-check` devDependency if no other property tests remain**

Run: `cd frontend && npm uninstall fast-check`

- [ ] **Step 4: Verify**

Run: `cd frontend && npm test`
Expected: PASS, coverage still ≥ 70%

- [ ] **Step 5: Commit**

```bash
cd frontend
git add -A
git commit -m "test: remove property-based tests where unit tests provide adequate coverage"
```

---

### Task 5.3: Consolidate over-split test files (frontend)

**Files:**
- Merge: `frontend/src/game/entry_flow_dom.test.ts` → `frontend/src/game/entry_flow.test.ts`

**Steps:**

- [ ] **Step 1: Read both test files**

Understand the test structure and identify overlap.

- [ ] **Step 2: Append `entry_flow_dom.test.ts` content to `entry_flow.test.ts`**

Combine describe blocks, ensuring no duplicate test names.

- [ ] **Step 3: Delete `entry_flow_dom.test.ts`**

```bash
cd frontend
rm src/game/entry_flow_dom.test.ts
```

- [ ] **Step 4: Verify**

Run: `cd frontend && npm test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd frontend
git add -A
git commit -m "test: merge entry_flow_dom.test.ts into entry_flow.test.ts"
```

---

### Task 5.4: Simplify testutil mocks (backend)

**Files:**
- Modify: `backend/internal/testutil/` (416 lines across 4 files)

**Steps:**

- [ ] **Step 1: Read all testutil files**

Identify mock implementations and their usage count.

- [ ] **Step 2: Find usage of each mock**

Run: `cd backend && rg "testutil\." --include="*.go" | grep -v "internal/testutil/" | cut -d: -f1 | sort | uniq -c | sort -rn`

- [ ] **Step 3: Delete mocks used only once**

Inline the mock at the single call site.

- [ ] **Step 4: Consolidate mock store creation**

Create a single `NewMockDeps()` function that returns all required mocks.

- [ ] **Step 5: Verify**

Run: `cd backend && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd backend
git add -A
git commit -m "test: simplify testutil mocks, remove single-use mock implementations"
```

---

### Task 5.5: Trim repetitive backend test cases

**Files:**
- Modify: Various test files in `backend/internal/`

**Steps:**

- [ ] **Step 1: Identify the largest test files**

Run: `cd backend && find . -name "*_test.go" -exec wc -l {} + | sort -rn | head -10`

- [ ] **Step 2: For each large test file, review for redundancy**

Look for:
- Multiple tests of the same error path with slightly different inputs
- Tests that only verify "no panic" (covered by `go test` running)
- Overly granular field-by-field assertions when snapshot tests exist

- [ ] **Step 3: Remove redundant test cases**

Keep: happy path, 2-3 key error paths, edge cases that caught real bugs.
Remove: redundant variations, "no panic" tests, duplicate assertions.

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./... -cover`
Expected: PASS, coverage ≥ 70%

- [ ] **Step 5: Commit**

```bash
cd backend
git add -A
git commit -m "test: remove redundant test cases, keep key paths and edge cases"
```

---

## Phase 6: Architecture Simplification

### Task 6.1: Merge `game/hub.go` + `game/hub_ops.go`

**Files:**
- Merge: `backend/internal/game/hub_ops.go` → `backend/internal/game/hub.go`

**Steps:**

- [ ] **Step 1: Read both files**

Understand the split (hub.go = struct + lifecycle, hub_ops.go = operations).

- [ ] **Step 2: Append `hub_ops.go` content to `hub.go`**

Combine into a single file, maintaining logical section order.

- [ ] **Step 3: Delete `hub_ops.go`**

```bash
cd backend
rm internal/game/hub_ops.go
```

- [ ] **Step 4: Verify**

Run: `cd backend && go build ./... && go test ./internal/game/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend
git add -A
git commit -m "refactor: merge hub_ops.go into hub.go"
```

---

### Task 6.2: Consolidate server route files

**Files:**
- Merge: `routes_public.go`, `routes_admin.go`, `routes_middleware.go` → `routes.go`

**Steps:**

- [ ] **Step 1: Read all 4 route files**

Understand the structure.

- [ ] **Step 2: Merge into `routes.go`**

Combine route registration into a single file with clear section comments.

- [ ] **Step 3: Delete the 3 merged files**

```bash
cd backend
rm internal/server/routes_public.go internal/server/routes_admin.go internal/server/routes_middleware.go
```

- [ ] **Step 4: Verify**

Run: `cd backend && go build ./... && go test ./internal/server/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend
git add -A
git commit -m "refactor: consolidate 4 server route files into routes.go"
```

---

### Task 6.3: Consolidate small middleware files

**Files:**
- Merge: `logging.go` + `prometheus.go` + `metrics.go` → `observability.go`
- Merge: `auth_util.go` → `auth_middleware.go`

**Steps:**

- [ ] **Step 1: Read the small middleware files**

Understand what each contains.

- [ ] **Step 2: Create `observability.go`**

Combine logging, prometheus, and metrics functions.

- [ ] **Step 3: Merge `auth_util.go` into `auth_middleware.go`**

Move utility functions.

- [ ] **Step 4: Delete old files**

```bash
cd backend
rm internal/middleware/logging.go internal/middleware/prometheus.go internal/middleware/metrics.go internal/middleware/auth_util.go
```

- [ ] **Step 5: Verify**

Run: `cd backend && go build ./... && go test ./internal/middleware/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd backend
git add -A
git commit -m "refactor: consolidate small middleware files"
```

---

### Task 6.4: Simplify `config/env.go` validation (if applicable)

**Files:**
- Modify: `backend/internal/config/env.go` (282 lines)

**Steps:**

- [ ] **Step 1: Read `env.go`**

Review the validation logic.

- [ ] **Step 2: Identify repetitive validation patterns**

Look for repeated `if x == "" { return error }` patterns that could be simplified.

- [ ] **Step 3: Simplify where possible**

Use struct tag validation or consolidate repetitive checks. Only simplify — don't change behavior.

- [ ] **Step 4: Verify**

Run: `cd backend && go test ./internal/config/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend
git add -A
git commit -m "refactor: simplify config/env.go validation logic"
```

---

## Final Verification

### Task F.1: Full build and test verification

**Steps:**

- [ ] **Step 1: Backend full verification**

```bash
cd backend
go build ./...
go vet ./...
go test ./... -cover
```

Expected: All PASS, coverage ≥ 70%

- [ ] **Step 2: Frontend full verification**

```bash
cd frontend
npm run lint
npm run typecheck
npm test
```

Expected: All PASS, coverage ≥ 70%

- [ ] **Step 3: Count final line totals**

```bash
# Go production
find backend/internal backend/cmd -name "*.go" ! -name "*_test.go" | xargs wc -l
# Go tests
find backend/internal backend/cmd -name "*_test.go" | xargs wc -l
# TypeScript
find frontend/src -name "*.ts" ! -name "*.test.ts" ! -name "*_test_setup.ts" | xargs wc -l
# TypeScript tests
find frontend/src -name "*.test.ts" -o -name "*_test_setup.ts" | xargs wc -l
```

Compare with baseline (54,249 total).

- [ ] **Step 4: Commit final state**

```bash
git add -A
git commit -m "chore: codebase simplification complete — ~30% line reduction"
```
