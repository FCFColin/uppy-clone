# Decoupling Plan — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement Clean Architecture interface-driven decoupling across the full Go + TypeScript codebase

**Architecture:** Backend adopts layered architecture (Handler → Application → Domain → Infrastructure) with dependency inversion — interfaces defined by consumers, implemented by infrastructure. Frontend adopts controlled state store + explicit module boundaries.

**Tech Stack:** Go 1.22+ (chi, pgx, redis, gorilla/websocket), TypeScript 5.6+ (Vite, Vitest, Canvas 2D), no runtime framework

## Global Constraints

- Every task's commit must pass: `go vet ./...`, `golangci-lint run`, `vitest run`, `tsc --noEmit`
- All existing tests must remain green after each step
- Zero behavior change — no functional modifications, only structural refactoring
- No new runtime dependencies (Go or TypeScript)
- `server` package remains the single composition root
- Frontend: no Redux/Zustand or any state management library — simple dispatch/reducer only
- Backend: no DI framework — manual injection only

---

## Phase 1: Low-Risk Cleanup (Week 1)

### Task 1.1: Reorganize `frontend/src/shared/` into subdirectories

**Files:**
- Create: `frontend/src/shared/network/auth.ts` (from `frontend/src/shared/auth.ts`)
- Create: `frontend/src/shared/network/session.ts` (from `frontend/src/shared/session.ts`)
- Create: `frontend/src/shared/network/fetch.ts` (from `frontend/src/shared/fetch.ts`)
- Create: `frontend/src/shared/game/types.ts` (from `frontend/src/shared/types.ts`)
- Create: `frontend/src/shared/game/constants.ts` (from `frontend/src/shared/constants.ts`)
- Create: `frontend/src/shared/game/protocol.ts` (from `frontend/src/shared/protocol.ts`)
- Create: `frontend/src/shared/ui/audio.ts` (from `frontend/src/shared/audio.ts`)
- Create: `frontend/src/shared/ui/toast.ts` (from `frontend/src/shared/toast.ts`)
- Create: `frontend/src/shared/data/tutorial_cookie.ts` (from `frontend/src/shared/tutorial_cookie.ts`)
- Create: `frontend/src/shared/data/best_score_cookie.ts` (from `frontend/src/shared/best_score_cookie.ts`)
- Create: `frontend/src/shared/assets/nickname_pools_gen.ts` (from `frontend/src/shared/nickname_pools_gen.ts`)
- Remove: `frontend/src/shared/auth.ts`, `session.ts`, `fetch.ts`, `types.ts`, `constants.ts`, `protocol.ts`, `audio.ts`, `toast.ts`, `tutorial_cookie.ts`, `best_score_cookie.ts`, `nickname_pools_gen.ts`
- Modify: All files that import from the old `shared/` paths

**Interfaces:**
- Consumes: Existing `shared/` module interfaces (unchanged, just relocated)
- Produces: Same exports, new paths

- [ ] **Step 1: Create all new files**

For each file, create the directory and copy content. Example:

```bash
# Create directories
mkdir -p frontend/src/shared/network frontend/src/shared/game frontend/src/shared/ui frontend/src/shared/data frontend/src/shared/assets

# Copy files to new locations
cp frontend/src/shared/auth.ts frontend/src/shared/network/auth.ts
cp frontend/src/shared/session.ts frontend/src/shared/network/session.ts
cp frontend/src/shared/fetch.ts frontend/src/shared/network/fetch.ts
cp frontend/src/shared/types.ts frontend/src/shared/game/types.ts
cp frontend/src/shared/constants.ts frontend/src/shared/game/constants.ts
cp frontend/src/shared/protocol.ts frontend/src/shared/game/protocol.ts
cp frontend/src/shared/audio.ts frontend/src/shared/ui/audio.ts
cp frontend/src/shared/toast.ts frontend/src/shared/ui/toast.ts
cp frontend/src/shared/tutorial_cookie.ts frontend/src/shared/data/tutorial_cookie.ts
cp frontend/src/shared/best_score_cookie.ts frontend/src/shared/data/best_score_cookie.ts
cp frontend/src/shared/nickname_pools_gen.ts frontend/src/shared/assets/nickname_pools_gen.ts
```

- [ ] **Step 2: Update all imports across the codebase**

Use grep to find all files importing from `'../shared/` or `'./shared/` (with underscore variants). Update each path:

```bash
# Find all files that import from shared/
rg "../shared/" --include="*.ts" frontend/src/
rg "./shared/" --include="*.ts" frontend/src/
```

Update imports like:
```typescript
// Old
import { fetchWithRetry } from '../shared/fetch.js';
// New
import { fetchWithRetry } from '../shared/network/fetch.js';
```

Files to update include:
- `frontend/src/index.ts` — imports `shared/session`, `shared/auth`
- `frontend/src/game/main.ts` — imports `shared/auth`, `shared/audio`, `shared/session`, `shared/toast`
- `frontend/src/game/ws_connect.ts` — imports `shared/session`, `shared/auth`
- `frontend/src/game/room_validate.ts` — imports `shared/auth`
- `frontend/src/game/ui.ts` — imports `shared/audio`, `shared/toast`
- `frontend/src/game/tutorial.ts` — imports `shared/tutorial_cookie`
- `frontend/src/game/entry_flow.ts` — imports `shared/best_score_cookie`
- `frontend/src/game/constants.ts` — imports `shared/constants`, `shared/protocol`
- `frontend/src/game/message_codec.ts` — imports `shared/protocol`
- `frontend/src/game/audio.ts` — the game-local re-export of shared/audio.ts
- `frontend/src/shared/network/session.ts` — imports `shared/auth`, `shared/fetch` → `./auth.js`, `./fetch.js`
- `frontend/src/shared/data/tutorial_cookie.ts` — imports `shared/auth` → `../network/auth.js`

- [ ] **Step 3: Update tsconfig.json for path resolution (if needed)**

Check `frontend/tsconfig.json` for any path aliases that reference `shared/`:
```bash
cat frontend/tsconfig.json
```

If needed, update paths to point to the new subdirectories.

- [ ] **Step 4: Run tests and verify**

```bash
cd frontend
npx vitest run
npx tsc --noEmit
cd ..
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/shared/
git rm frontend/src/shared/auth.ts frontend/src/shared/session.ts frontend/src/shared/fetch.ts frontend/src/shared/types.ts frontend/src/shared/constants.ts frontend/src/shared/protocol.ts frontend/src/shared/audio.ts frontend/src/shared/toast.ts frontend/src/shared/tutorial_cookie.ts frontend/src/shared/best_score_cookie.ts frontend/src/shared/nickname_pools_gen.ts
git commit -m "refactor(frontend): split shared/ into subdirectories by domain"
```

---

### Task 1.2: Eliminate `game/constants.ts` re-exports

**Files:**
- Modify: `frontend/src/game/constants.ts` — remove re-exports, delete file
- Modify: All files that import from `game/constants.ts` for shared constants
- Modify: Files that import from `game/constants.ts` for palette/game-specific constants — inline or rename

**Interfaces:**
- Consumes: `shared/game/constants.ts` and `shared/game/protocol.ts`
- Produces: Direct imports from `shared/game/` instead of through `game/constants.ts`

- [ ] **Step 1: Find all imports of `game/constants.ts`**

```bash
rg "from './constants\.js'" frontend/src/game/
rg "from '../game/constants\.js'" frontend/src/game/
```

Files that import `./constants.js`:
- `input.ts` — imports `COOLDOWN`, `PHYSICS`, `TAP_COOLDOWN_MS`, `MSG_TYPE`
- `ws_connection.ts` — imports `RECONNECT_DELAY_MAX`
- `ws_handlers_snapshot.ts`
- `phase_sync.ts`
- `renderer.ts`
- `renderer_draw.ts`
- `ws_handlers_phase.ts`
- `message_codec.ts`?
- `interp_buffers.ts`?
- `entry_flow.ts`?

Check what each actually imports and which come from shared vs are local to constants.ts.

- [ ] **Step 2: Identify local-only content in `game/constants.ts`**

Read `frontend/src/game/constants.ts`:
```bash
cat frontend/src/game/constants.ts
```

It likely contains:
- Re-exports from `shared/constants.ts` and `shared/protocol.ts` → direct import
- Local color palette constants → move to a local file or their consumer
- Local timing constants → move to consumer files

- [ ] **Step 3: Update import statements**

For each consumer, change:
```typescript
// Old
import { COOLDOWN, MSG_TYPE } from './constants.js';
// New
import { COOLDOWN } from '../shared/game/constants.js';
import { MSG_TYPE } from '../shared/game/protocol.js';
```

For local-only constants (e.g., color palette), create a targeted file or inline in the single consumer.

- [ ] **Step 4: Remove or gut `game/constants.ts`**

After migrating all imports, either:
- Delete the file if everything was a re-export
- Keep only truly local constants that don't belong in shared

- [ ] **Step 5: Verify**

```bash
cd frontend
npx vitest run
npx tsc --noEmit
cd ..
```

- [ ] **Step 6: Commit**

```bash
git commit -m "refactor(frontend): eliminate game/constants.ts re-exports, use direct shared imports"
```

---

### Task 1.3: Eliminate barrel files `state.ts` and `websocket.ts`

**Files:**
- Remove: `frontend/src/game/state.ts`
- Remove: `frontend/src/game/websocket.ts`
- Modify: All files importing from `./state.js` → `./state_types.js`, `./state_interp.js`, `./state_reset.js`
- Modify: All files importing from `./websocket.js` → `./ws_connection.js`, `./ws_connect.js`

**Interfaces:**
- Consumes: `state_types.ts`, `state_interp.ts`, `state_reset.ts`, `ws_connection.ts`, `ws_connect.ts`
- Produces: Direct imports, no barrel indirection

- [ ] **Step 1: Find all imports of `./state.js`**

```bash
rg "from './state\.js'" frontend/src/game/
rg "from '../game/state\.js'" frontend/src/game/
```

Files likely importing `./state.js`:
`main.ts`, `renderer.ts`, `ui_update.ts`, `input.ts`, `entry_flow.ts`, `ws_handlers_snapshot.ts`, `ws_handlers_phase.ts`, `phase_sync.ts`, `visual_helpers.ts`, `state_interp.ts`, `state_reset.ts`

- [ ] **Step 2: Find all imports of `./websocket.js`**

```bash
rg "from './websocket\.js'" frontend/src/game/
```

Files likely importing `./websocket.js`: `main.ts`, `ws_handlers.ts`, `entry_flow.ts`

- [ ] **Step 3: Check what each consumer actually uses**

Read `state.ts` (the barrel) to see what it re-exports:
```bash
cat frontend/src/game/state.ts
```

It re-exports from `state_types`, `state_interp`, `state_reset`. For each consumer, update imports to only import what's needed:

```typescript
// Old (barrel)
import { state, ClientState, resetRoundClientState, initInterpolation } from './state.js';

// New (direct)
import { state, ClientState } from './state_types.js';
import { initInterpolation } from './state_interp.js';
import { resetRoundClientState } from './state_reset.js';
```

- [ ] **Step 4: Update `renderer.ts` specifically (likely heaviest consumer)**

```typescript
// Old
import { state, ClientState, resetInterpolation } from './state.js';
// New
import { state, ClientState } from './state_types.js';
import { resetInterpolation } from './state_interp.js';
```

- [ ] **Step 5: Remove barrel files**

```bash
git rm frontend/src/game/state.ts frontend/src/game/websocket.ts
```

- [ ] **Step 6: Verify**

```bash
cd frontend
npx vitest run
npx tsc --noEmit
cd ..
```

- [ ] **Step 7: Commit**

```bash
git commit -m "refactor(frontend): remove state.ts and websocket.ts barrel files"
```

---

### Task 1.4: Eliminate `ui.ts` ↔ `ui_update.ts` circular import

**Files:**
- Modify: `frontend/src/game/ui.ts`
- Modify: `frontend/src/game/ui_update.ts`

**Interfaces:**
- Consumes: Current `ui.ts` and `ui_update.ts` interfaces
- Produces: Clean directed dependency: `ui.ts` → `ui_update.ts` (no reverse)

- [ ] **Step 1: Read both files to understand the cycle**

```bash
cat frontend/src/game/ui.ts
cat frontend/src/game/ui_update.ts
```

Expected pattern:
- `ui_update.ts` imports `{ refreshLayout } from './ui.js'`
- `ui.ts` exports `{ updateUI } from './ui_update.js'` AND also contains implementation

- [ ] **Step 2: Decide direction**

Decision: Make `ui.ts` a pure barrel (only re-exports) OR a pure implementation (no re-exports). The cleaner choice: **move all implementation out of `ui.ts` into separate files**, let `ui.ts` be only a barrel.

- [ ] **Step 3: Extract ui.ts implementation to `ui_utils.ts`**

Move `startCountdownTimer`, `copyCode`, `showFallbackErrorScreen` from `ui.ts` to a new `ui_utils.ts`:

```typescript
// frontend/src/game/ui_utils.ts
import { pickRandomNickname } from './ui_elements.js';
import { showToast } from '../shared/ui/toast.js';
import { playCountdownTick } from '../shared/ui/audio.js';
import { resizeCanvas } from './renderer.js';

export function startCountdownTimer(seconds: number) { /* move from ui.ts */ }
export function copyCode(code: string) { /* move from ui.ts */ }
export function showFallbackErrorScreen(msg: string) { /* move from ui.ts */ }
```

- [ ] **Step 4: Convert `ui.ts` to pure barrel**

```typescript
// frontend/src/game/ui.ts
export { updateUI } from './ui_update.js';
export { startCountdownTimer, copyCode } from './ui_utils.js';
// ... other re-exports
```

- [ ] **Step 5: Fix `ui_update.ts` to not import from barrel**

```typescript
// ui_update.ts — import from ui_utils.ts directly, not ui.ts
import { resizeCanvas } from './renderer.js';
// If it needs refreshLayout, import from where it's defined
```

- [ ] **Step 6: Verify no circular dependency**

```bash
rg "from './ui\.js'" frontend/src/game/ui_update.ts
# Should show zero results now
```

- [ ] **Step 7: Tests + commit**

```bash
cd frontend
npx vitest run
npx tsc --noEmit
cd ..
git commit -m "refactor(frontend): break ui.ts circular import by extracting ui_utils.ts"
```

---

### Task 1.5: Merge `entry_flow.ts` + `entry_flow_dom.ts`

**Files:**
- Remove: `frontend/src/game/entry_flow_dom.ts`
- Modify: `frontend/src/game/entry_flow.ts` — inline DOM functions

**Interfaces:**
- Consumes: `entry_flow_dom.ts` functions
- Produces: Single `entry_flow.ts` module

- [ ] **Step 1: Read `entry_flow_dom.ts`**

```bash
cat frontend/src/game/entry_flow_dom.ts
```

- [ ] **Step 2: Inline all DOM functions into `entry_flow.ts`**

Move all exported functions from `entry_flow_dom.ts` into `entry_flow.ts`. Add the DOM imports to `entry_flow.ts`.

- [ ] **Step 3: Update importers**

Find files importing `entry_flow_dom`:
```bash
rg "entry_flow_dom" frontend/src/game/
```

Update them to only import from `entry_flow`:
```typescript
// Old
import { syncEntryOverlays } from './entry_flow_dom.js';
// New
import { syncEntryOverlays } from './entry_flow.js';
```

- [ ] **Step 4: Remove old file**

```bash
git rm frontend/src/game/entry_flow_dom.ts
```

- [ ] **Step 5: Verify**

```bash
cd frontend
npx vitest run
npx tsc --noEmit
cd ..
git commit -m "refactor(frontend): merge entry_flow_dom.ts into entry_flow.ts"
```

---

### Task 1.6: Split `config` package into sub-interfaces (Backend)

**Files:**
- Modify: `backend/internal/config/config.go`
- Create: `backend/internal/config/server.go` (optional, if splitting structs)
- Modify: All backend packages that import `config`

**Interfaces:**
- Produces: `ServerConfig`, `DBConfig`, `RedisConfig`, `AuthConfig`, `GameConfig` interfaces

- [ ] **Step 1: Read current config**

```bash
cat backend/internal/config/config.go
```

- [ ] **Step 2: Define sub-interfaces in `config/config.go`**

```go
// backend/internal/config/config.go

// ServerConfig defines server-level configuration.
type ServerConfig interface {
    Addr() string
    ReadTimeout() time.Duration
    WriteTimeout() time.Duration
}

// DBConfig defines database configuration.
type DBConfig interface {
    DatabaseURL() string
    MaxOpenConns() int
    MaxIdleConns() int
}

// RedisConfig defines Redis configuration.
type RedisConfig interface {
    RedisAddr() string
    RedisPassword() string
}

// AuthConfig defines authentication configuration.
type AuthConfig interface {
    JWTSecret() string
    AccessTokenTTL() time.Duration
    RefreshTokenTTL() time.Duration
    MagicLinkTTL() time.Duration
}

// GameConfig defines game engine configuration.
type GameConfig interface {
    MaxPlayersPerRoom() int
    GameDuration() time.Duration
    TickRate() int
}
```

- [ ] **Step 3: Make `Env` implement all interfaces**

```go
// Ensure Env struct satisfies all interfaces
var _ ServerConfig = (*Env)(nil)
var _ DBConfig = (*Env)(nil)
// etc.
```

Add any missing methods to `Env`.

- [ ] **Step 4: Update consumers to use minimal interfaces**

For each consumer package, change function signatures to accept only the interface they need:

```go
// Old: handler.go
func NewAuthHandler(cfg *config.Env) *Handler

// New: handler.go
func NewAuthHandler(cfg config.AuthConfig) *Handler
```

Update packages:
- `handler` → depends on `AuthConfig`, `ServerConfig`, `GameConfig`
- `auth` → depends on `AuthConfig`
- `store` → depends on `DBConfig`, `RedisConfig`
- `game` → depends on `GameConfig`
- `middleware` → depends on `ServerConfig`
- `server` → imports full `config.Env` (still single composition root)

- [ ] **Step 5: Verify**

```bash
cd backend
go vet ./...
golangci-lint run
cd ..
```

- [ ] **Step 6: Run backend tests**

```bash
cd backend
go test ./... -count=1 -short
cd ..
```

- [ ] **Step 7: Commit**

```bash
git add backend/internal/config/ backend/internal/handler/ backend/internal/auth/ backend/internal/store/ backend/internal/game/ backend/internal/middleware/ backend/internal/server/
git commit -m "refactor(backend): split config into sub-interfaces by domain"
```

---

## Phase 2: Core Decoupling (Week 2)

### Task 2.1: Invert `domain` → `validate` dependency

**Files:**
- Create: `backend/internal/domain/validator.go` — define validator interface
- Modify: `backend/internal/domain/nickname.go` — use interface instead of direct call
- Modify: `backend/internal/handler/*.go` — inject validator
- Modify: `backend/internal/server/main.go` — wire validator

**Interfaces:**
- Consumes: `validate.Nickname()` function
- Produces: `domain.NicknameValidator` interface

- [ ] **Step 1: Read current domain nickname validation**

```bash
cat backend/internal/domain/nickname.go
```

Expected: `domain.NewNickname(s string)` calls `validate.Nickname(s)` directly.

- [ ] **Step 2: Create `domain/validator.go` with interface**

```go
// backend/internal/domain/validator.go
package domain

// NicknameValidator validates nickname strings.
type NicknameValidator interface {
    ValidateNickname(nickname string) error
}
```

- [ ] **Step 3: Update domain to accept validator via option or parameter**

```go
// backend/internal/domain/nickname.go
package domain

// NewNickname creates a validated Nickname value object.
// It requires a NicknameValidator to perform validation.
func NewNickname(nickname string, v NicknameValidator) (Nickname, error) {
    if err := v.ValidateNickname(nickname); err != nil {
        return Nickname{}, err
    }
    return Nickname{value: nickname}, nil
}
```

Search all callers of `NewNickname` and update signatures:
```bash
rg "NewNickname" --include="*.go" backend/
```

- [ ] **Step 4: Have `validate.Nickname()` implement the interface**

```go
// backend/internal/validate/nickname.go
package validate

import "github.com/uppy-clone/backend/internal/domain"

// Ensure NicknameValidator is implemented
var _ domain.NicknameValidator = NicknameValidatorFunc(ValidateNickname)

// NicknameValidatorFunc adapts a function to the domain.NicknameValidator interface.
type NicknameValidatorFunc func(string) error

func (f NicknameValidatorFunc) ValidateNickname(nickname string) error {
    return f(nickname)
}

// ValidateNickname remains the same exported function.
func ValidateNickname(nickname string) error {
    // existing implementation
}
```

- [ ] **Step 5: Update `server` package to wire the dependency**

```go
// backend/internal/server/main.go or server.go
validator := validate.NicknameValidatorFunc(validate.ValidateNickname)
// pass validator to handlers that create nicknames
handler.NewAuthHandler(..., validator)
```

- [ ] **Step 6: Remove `validate` import from `domain`**

```go
// backend/internal/domain/nickname.go — remove import of validate
```

- [ ] **Step 7: Verify**

```bash
cd backend
go vet ./...
golangci-lint run
go test ./... -count=1 -short
cd ..
```

- [ ] **Step 8: Commit**

```bash
git commit -m "refactor(backend): invert domain→validate dependency via NicknameValidator interface"
```

---

### Task 2.2: Extract `protocol` message type constants from `metrics`

**Files:**
- Create: `backend/internal/constants/protocol_messages.go` — message type constants
- Modify: `backend/internal/protocol/constants.go` — remove or alias constants
- Modify: `backend/internal/metrics/record.go` — import from new location
- Modify: `backend/internal/game/*.go`, `handler/*.go` — update imports

**Interfaces:**
- Consumes: Existing protocol constants
- Produces: New standalone `constants` leaf package

- [ ] **Step 1: Read current constants**

```bash
cat backend/internal/protocol/constants.go
cat backend/internal/metrics/record.go
```

- [ ] **Step 2: Create `backend/internal/constants/protocol_messages.go`**

```go
// backend/internal/constants/protocol_messages.go
package constants

// Message types — originally in protocol package.
const (
    MsgTap        = 0x01
    MsgSnapshot   = 0x02
    MsgPhase      = 0x03
    MsgEvent      = 0x04
    MsgRestart    = 0x05
    // ... all message type constants
)
```

- [ ] **Step 3: Update `metrics/record.go` to import from `constants`**

```go
// backend/internal/metrics/record.go
import "github.com/uppy-clone/backend/internal/constants"

func RecordMessage(msgType byte) {
    switch msgType {
    case constants.MsgTap:
        // ...
    }
}
```

- [ ] **Step 4: Update all other imports**

Find all packages importing `protocol` for constants:
```bash
rg "\"github.com/uppy-clone/backend/internal/protocol\"" backend/ --include="*.go"
```

- `game/` — imports protocol for constants + encoding
- `handler/` — imports protocol for constants
- `metrics/` — only needs the message type constants

Update `game/` and `handler/` to import from the new `constants` package for message type constants, keeping `protocol` import only for encode/decode functions.

- [ ] **Step 5: Remove the constants from `protocol/constants.go` (or keep aliases)**

Option: Keep aliases in `protocol/constants.go` for backward compat:
```go
package protocol

import "github.com/uppy-clone/backend/internal/constants"

const (
    MsgTap      = constants.MsgTap
    MsgSnapshot = constants.MsgSnapshot
    // ...
)
```

- [ ] **Step 6: Verify**

```bash
cd backend
go vet ./...
golangci-lint run
go test ./... -count=1 -short
cd ..
```

- [ ] **Step 7: Commit**

```bash
git commit -m "refactor(backend): extract protocol message constants to standalone leaf package"
```

---

### Task 2.3: Introduce frontend `GameStore` controlled state management

**Files:**
- Create: `frontend/src/game/store.ts` — GameStore with dispatch/getState/select
- Create: `frontend/src/game/store.test.ts` — tests for store
- Create: `frontend/src/game/reducer.ts` — pure reducer function
- Modify: `frontend/src/game/state_types.ts` — no longer export mutable `state` directly
- Modify: `frontend/src/game/main.ts` — initialize store, pass to subsystems
- Modify: `frontend/src/game/entry_flow.ts` — use dispatch
- Modify: `frontend/src/game/ws_handlers_snapshot.ts` — use dispatch
- Modify: `frontend/src/game/ws_handlers_phase.ts` — use dispatch
- Modify: `frontend/src/game/ws_handlers_events.ts` — use dispatch
- Modify: `frontend/src/game/input.ts` — use dispatch
- Modify: `frontend/src/game/ui_update.ts` — use select/getState
- Modify: `frontend/src/game/phase_sync.ts` — use select/getState
- Modify: `frontend/src/game/visual_helpers.ts` — use select/getState
- Modify: `frontend/src/game/renderer.ts` — use select/getState
- Modify: `frontend/src/game/state_reset.ts` — use dispatch

**Interfaces:**
- Produces: `GameStore { dispatch(action), getState(), select(selector) }`

- [ ] **Step 1: Read current `state_types.ts`**

```bash
cat frontend/src/game/state_types.ts
```

This likely exports a mutable `state` singleton of type `ClientState`. We'll keep the type but wrap mutation.

- [ ] **Step 2: Create `reducer.ts`**

```typescript
// frontend/src/game/reducer.ts
import { ClientState } from './state_types.js';

export type GameAction =
    | { type: 'SET_PHASE'; phase: GamePhase }
    | { type: 'UPDATE_SNAPSHOT'; update: Partial<ClientState> }
    | { type: 'ADD_RIPPLE'; ripple: Ripple }
    | { type: 'UPDATE_PLAYERS'; players: PlayerState[] }
    | { type: 'UPDATE_SCORES'; scores: Record<string, number> }
    | { type: 'SET_WAITING'; waiting: boolean }
    | { type: 'RESET' }
    | { type: 'SET_CONNECTION_STATUS'; status: string }
    | { type: 'ADD_EXPLOSION'; explosion: Explosion }
    | { type: 'UPDATE_RESTART_VOTES'; votes: RestartVotes }
    | { type: 'SET_BLOCK_GAME_RENDER'; blocked: boolean };

export function gameReducer(state: ClientState, action: GameAction): ClientState {
    switch (action.type) {
        case 'SET_PHASE':
            return { ...state, phase: action.phase };
        case 'UPDATE_SNAPSHOT':
            return { ...state, ...action.update };
        case 'ADD_RIPPLE':
            return { ...state, ripples: [...state.ripples, action.ripple] };
        case 'UPDATE_PLAYERS':
            return { ...state, players: action.players };
        case 'UPDATE_SCORES':
            return { ...state, scores: action.scores };
        case 'SET_WAITING':
            return { ...state, isWaiting: action.waiting };
        case 'RESET':
            return createInitialState();
        case 'SET_CONNECTION_STATUS':
            return { ...state, connectionStatus: action.status };
        case 'ADD_EXPLOSION':
            return { ...state, explosions: [...state.explosions, action.explosion] };
        case 'UPDATE_RESTART_VOTES':
            return { ...state, restartVotes: action.votes };
        case 'SET_BLOCK_GAME_RENDER':
            return { ...state, blockGameRender: action.blocked };
        default:
            return state;
    }
}

function createInitialState(): ClientState {
    return {
        phase: 'waiting',
        players: [],
        scores: {},
        ripples: [],
        explosions: [],
        balloons: [],
        birds: [],
        ghosts: [],
        connectionStatus: 'disconnected',
        isWaiting: true,
        blockGameRender: false,
        restartVotes: { yes: 0, no: 0 },
        // ...all other initial values from the current initialization
    };
}
```

- [ ] **Step 3: Create `store.ts`**

```typescript
// frontend/src/game/store.ts
import { ClientState } from './state_types.js';
import { gameReducer, GameAction } from './reducer.js';

export interface GameStore {
    dispatch(action: GameAction): void;
    getState(): ClientState;
    select<T>(selector: (state: ClientState) => T): T;
}

export function createStore(initialState?: ClientState): GameStore {
    // For the transition period, we keep the existing state singleton
    // but wrap all writes through dispatch
    let currentState = initialState ?? createInitialState();

    return {
        dispatch(action: GameAction) {
            currentState = gameReducer(currentState, action);
        },
        getState(): ClientState {
            return currentState;
        },
        select<T>(selector: (state: ClientState) => T): T {
            return selector(currentState);
        },
    };
}
```

- [ ] **Step 4: Write store tests**

```typescript
// frontend/src/game/store.test.ts
import { describe, it, expect } from 'vitest';
import { createStore } from './store.js';

describe('GameStore', () => {
    it('should initialize with default state', () => {
        const store = createStore();
        const state = store.getState();
        expect(state.phase).toBe('waiting');
        expect(state.isWaiting).toBe(true);
    });

    it('should update state via dispatch', () => {
        const store = createStore();
        store.dispatch({ type: 'SET_PHASE', phase: 'playing' });
        expect(store.getState().phase).toBe('playing');
    });

    it('should select derived state', () => {
        const store = createStore();
        const phase = store.select(s => s.phase);
        expect(phase).toBe('waiting');
    });

    it('should reset state', () => {
        const store = createStore();
        store.dispatch({ type: 'SET_PHASE', phase: 'playing' });
        store.dispatch({ type: 'RESET' });
        expect(store.getState().phase).toBe('waiting');
    });

    it('should add ripples immutably', () => {
        const store = createStore();
        store.dispatch({ type: 'ADD_RIPPLE', ripple: { x: 0.5, y: 0.5, age: 0 } });
        expect(store.getState().ripples).toHaveLength(1);
    });

    it('should not mutate previous state', () => {
        const store = createStore();
        const prevState = store.getState();
        store.dispatch({ type: 'SET_PHASE', phase: 'playing' });
        expect(prevState.phase).toBe('waiting'); // immutability
    });
});
```

- [ ] **Step 5: Update `state_types.ts` — expose state singleton but discourage direct mutation**

Keep the `state` object but mark it as deprecated:
```typescript
// @deprecated Use store.dispatch() instead
export const state: ClientState = { /* ... */ };
```

- [ ] **Step 6: Update `main.ts` to create store and pass to subsystems**

```typescript
// frontend/src/game/main.ts
import { createStore } from './store.js';

const store = createStore();

// Pass store to all subsystems
initWebSocket(store);
initRenderer(store);
initEntryFlow(store);
initInput(store);
```

- [ ] **Step 7: Migrate one module at a time — start with `input.ts`**

```typescript
// frontend/src/game/input.ts — simplified example
import { store } from './main.js'; // or pass as parameter

export function handleTap(x: number, y: number) {
    const state = store.getState();
    if (state.phase !== 'playing') return;

    // Optimistic update
    store.dispatch({ type: 'ADD_RIPPLE', ripple: { x, y, age: 0 } });

    // Send message
    sendMessage(encodeTap(x, y));
}
```

- [ ] **Step 8: Migrate `ws_handlers_snapshot.ts`**

```typescript
// Instead of: state.balloons = decoded.balloons;
store.dispatch({
    type: 'UPDATE_SNAPSHOT',
    update: {
        balloons: decoded.balloons,
        birds: decoded.birds,
        ghosts: decoded.ghosts,
        players: decoded.players,
    },
});
```

- [ ] **Step 9: Migrate `renderer.ts`**

```typescript
// Instead of:
// if ($endedScreen && !$endedScreen.classList.contains('hidden')) return true;
function overlayBlocksGameRender(): boolean {
    return store.select(s => s.phase === 'ended' || s.blockGameRender);
}
```

- [ ] **Step 10: Run tests iteratively after each module migration**

```bash
cd frontend
npx vitest run --reporter verbose
npx tsc --noEmit
cd ..
```

- [ ] **Step 11: Commit**

```bash
git add frontend/src/game/store.ts frontend/src/game/reducer.ts frontend/src/game/store.test.ts
git add frontend/src/game/state_types.ts frontend/src/game/main.ts frontend/src/game/input.ts
git add frontend/src/game/ws_handlers_snapshot.ts frontend/src/game/renderer.ts
git commit -m "refactor(frontend): introduce GameStore controlled state management"
```

---

### Task 2.4: Clean `renderer.ts` DOM queries → store reads

**Files:**
- Modify: `frontend/src/game/renderer.ts` — replace DOM reads with store.select()

**Interfaces:**
- Consumes: `GameStore.select()` interface from Task 2.3
- Produces: Renderer no longer touches DOM for state

- [ ] **Step 1: Find all DOM queries in renderer.ts**

```bash
rg "getElementById|querySelector|classList|hidden" frontend/src/game/renderer.ts
```

Identify all lines where renderer checks DOM state:
- `if ($endedScreen && !$endedScreen.classList.contains('hidden'))`
- Any other DOM visibility checks

- [ ] **Step 2: Add `blockGameRender` to state where UI transitions set it**

In `ui_update.ts`, when showing/hiding screens, add:
```typescript
store.dispatch({ type: 'SET_BLOCK_GAME_RENDER', blocked: true }); // or false
```

- [ ] **Step 3: Replace each DOM query with store.select()**

```typescript
// Before
function overlayBlocksGameRender(): boolean {
    if ($endedScreen && !$endedScreen.classList.contains('hidden')) return true;
    if ($restartVotePanel && !$restartVotePanel.classList.contains('hidden')) return true;
    return false;
}

// After
function overlayBlocksGameRender(): boolean {
    return store.select(s => s.phase === 'ended' || s.showRestartVote);
}
```

- [ ] **Step 4: Verify**

```bash
cd frontend
npx vitest run
npx tsc --noEmit
cd ..
```

- [ ] **Step 5: Commit**

```bash
git commit -m "refactor(frontend): replace renderer DOM queries with store.select()"
```

---

### Task 2.5: Split `main.ts` event/lifecycle logic

**Files:**
- Create: `frontend/src/game/window_events.ts` — visibility, resize, online/offline, error, beforeunload
- Create: `frontend/src/game/lifecycle.ts` — app lifecycle management
- Modify: `frontend/src/game/main.ts` — delegate to new modules

**Interfaces:**
- Consumes: `GameStore` from Task 2.3
- Produces: Clean `main.ts` that only bootstraps

- [ ] **Step 1: Read main.ts**

```bash
cat frontend/src/game/main.ts
```

Identify event bindings and lifecycle code.

- [ ] **Step 2: Create `window_events.ts`**

Extract all window event listeners:

```typescript
// frontend/src/game/window_events.ts
import { GameStore } from './store.js';

export function bindWindowEvents(store: GameStore) {
    let enterPressed = false;

    window.addEventListener('resize', () => {
        // resize logic
    });

    document.addEventListener('visibilitychange', () => {
        // visibility logic
    });

    window.addEventListener('error', (e) => {
        // error handling
    });

    window.addEventListener('online', () => {
        // online logic
    });

    window.addEventListener('offline', () => {
        // offline logic
    });

    window.addEventListener('beforeunload', () => {
        // cleanup
    });

    document.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            enterPressed = true;
            // handle enter
        }
    });

    document.addEventListener('keyup', (e) => {
        if (e.key === 'Enter') {
            enterPressed = false;
        }
    });
}
```

- [ ] **Step 3: Create `lifecycle.ts`**

```typescript
// frontend/src/game/lifecycle.ts
import { GameStore } from './store.js';

export function bootSystem(store: GameStore) {
    // boot order: store → connection → entry flow → renderer
}

export function shutdownSystem(store: GameStore) {
    // graceful cleanup
}
```

- [ ] **Step 4: Update main.ts**

```typescript
// frontend/src/game/main.ts — simplified
import { createStore } from './store.js';
import { bindWindowEvents } from './window_events.js';
import { bootSystem } from './lifecycle.js';
// ... other imports

const store = createStore();
bindWindowEvents(store);
bootSystem(store);

// Not much more
```

- [ ] **Step 5: Verify**

```bash
cd frontend
npx vitest run
npx tsc --noEmit
cd ..
```

- [ ] **Step 6: Commit**

```bash
git commit -m "refactor(frontend): extract window events and lifecycle from main.ts"
```

---

## Phase 3: Deep Decoupling (Week 3)

### Task 3.1: Define interfaces in `auth` package, implement in `store`

**Files:**
- Modify: `backend/internal/auth/auth.go` — define `UserStore`, `SessionStore`, `MagicLinkStore` interfaces
- Modify: `backend/internal/auth/magiclink.go` — use interfaces instead of concrete store
- Modify: `backend/internal/auth/quickplay.go` — use interfaces
- Modify: `backend/internal/auth/revoke.go` — use interfaces
- Modify: `backend/internal/store/redis_auth_session.go` — implement `SessionStore`
- Modify: `backend/internal/store/postgres_users_crud.go` — implement `UserStore`
- Modify: `backend/internal/server/main.go` — wire interfaces

**Interfaces:**
- Produces: `auth.UserStore`, `auth.SessionStore`, `auth.MagicLinkStore`

- [ ] **Step 1: Read current auth manager struct and constructor**

```bash
cat backend/internal/auth/auth.go
```

Identify the concrete `*store.PostgresStore` and `*store.RedisStore` fields.

- [ ] **Step 2: Define interfaces in `auth/auth.go`**

```go
// backend/internal/auth/auth.go
package auth

import "context"

// UserStore defines the user persistence interface needed by auth.
type UserStore interface {
    FindOrCreate(ctx context.Context, email string) (*domain.User, error)
    GetByID(ctx context.Context, id string) (*domain.User, error)
    SoftDelete(ctx context.Context, id string) error
    GetByEmail(ctx context.Context, email string) (*domain.User, error)
}

// SessionStore defines the session persistence interface needed by auth.
type SessionStore interface {
    CreateSession(ctx context.Context, userID string, ttl time.Duration) (*domain.Session, error)
    GetSession(ctx context.Context, token string) (*domain.User, error)
    RevokeSession(ctx context.Context, token string) error
    RevokeAllUserSessions(ctx context.Context, userID string) error
}

// MagicLinkStore defines the magic link token persistence interface.
type MagicLinkStore interface {
    CreateToken(ctx context.Context, email string) (*domain.MagicLink, error)
    ConsumeToken(ctx context.Context, token string) (string, error)
}

// Manager holds the auth logic with injected store interfaces.
type Manager struct {
    users    UserStore
    sessions SessionStore
    magic    MagicLinkStore
    cfg      config.AuthConfig
    // ...
}

func NewManager(users UserStore, sessions SessionStore, magic MagicLinkStore, cfg config.AuthConfig) *Manager {
    return &Manager{users: users, sessions: sessions, magic: magic, cfg: cfg}
}
```

- [ ] **Step 3: Update auth sub-files to use interfaces**

Update `magiclink.go`:
```go
// Change from
func (m *Manager) SendMagicLink(ctx context.Context, email string) error {
    token, err := m.store.CreateMagicLinkToken(ctx, email) // direct store call
// To
func (m *Manager) SendMagicLink(ctx context.Context, email string) error {
    token, err := m.magic.CreateToken(ctx, email) // interface call
```

Update `revoke.go` and `quickplay.go` similarly.

- [ ] **Step 4: Make store implementations match interfaces**

Add adapter methods if needed to `store.PostgresStore` and `store.RedisStore`:

```go
// backend/internal/store/redis_auth_session.go
func (s *RedisStore) CreateSession(ctx context.Context, userID string, ttl time.Duration) (*domain.Session, error) {
    // existing implementation
}
```

- [ ] **Step 5: Update `server` to wire interfaces**

```go
// backend/internal/server/main.go
func NewServer(cfg *config.Env) *Server {
    pgStore := store.NewPostgresStore(cfg)
    redisStore := store.NewRedisStore(cfg)

    authMgr := auth.NewManager(
        pgStore,      // implements auth.UserStore
        redisStore,   // implements auth.SessionStore
        pgStore,      // implements auth.MagicLinkStore
        cfg,
    )
    // ...
}
```

- [ ] **Step 6: Remove `store` import from auth package**

```go
// backend/internal/auth/*.go — remove import of store
```

- [ ] **Step 7: Verify**

```bash
cd backend
go vet ./...
golangci-lint run
go test ./... -count=1 -short
cd ..
```

- [ ] **Step 8: Commit**

```bash
git commit -m "refactor(backend): invert auth→store dependency via UserStore/SessionStore/MagicLinkStore interfaces"
```

---

### Task 3.2: Define interfaces in `game` package, implement in `store`

**Files:**
- Modify: `backend/internal/game/hub.go` — define `RoomRepository`, `Broadcaster` interfaces
- Modify: `backend/internal/game/room_lifecycle.go` — use interfaces
- Modify: `backend/internal/game/room_persist_async.go` — use interfaces
- Modify: `backend/internal/game/disconnect_grace.go` — use interfaces
- Modify: `backend/internal/game/restart.go` — use interfaces
- Modify: `backend/internal/game/hub_cache.go` — use interfaces
- Modify: `backend/internal/store/redis_room_registry.go` — implement interfaces
- Modify: `backend/internal/store/postgres_room.go` — implement interfaces
- Modify: `backend/internal/server/main.go` — wire interfaces

**Interfaces:**
- Produces: `game.RoomRepository`, `game.Broadcaster`, `game.PlayerStore`

- [ ] **Step 1: Read current game hub struct**

```bash
cat backend/internal/game/hub.go
# Look for *store.RedisStore and *store.PostgresStore fields
```

- [ ] **Step 2: Define interfaces in `game/game_interfaces.go`**

```go
// backend/internal/game/game_interfaces.go
package game

import "context"

// RoomRepository defines room persistence operations.
type RoomRepository interface {
    SaveRoom(ctx context.Context, room *Room) error
    GetRoom(ctx context.Context, code string) (*Room, error)
    DeleteRoom(ctx context.Context, code string) error
    ListActiveRooms(ctx context.Context) ([]*Room, error)
}

// Broadcaster defines cross-instance message broadcasting.
type Broadcaster interface {
    PublishRoomMessage(ctx context.Context, roomCode string, msg []byte) error
    SubscribeRoomMessages(ctx context.Context, roomCode string) (<-chan []byte, error)
    UnsubscribeRoomMessages(ctx context.Context, roomCode string) error
}

// PlayerStore defines player result persistence.
type PlayerStore interface {
    SaveGameResult(ctx context.Context, result *domain.GameResult) error
    GetPlayerHistory(ctx context.Context, playerID string) ([]*domain.GameResult, error)
}
```

- [ ] **Step 3: Update Hub struct to use interfaces**

```go
// backend/internal/game/hub.go
type Hub struct {
    rooms    RoomRepository
    broadcast Broadcaster
    players  PlayerStore
    // ...
}

func NewHub(rooms RoomRepository, broadcast Broadcaster, players PlayerStore, cfg config.GameConfig) *Hub {
    return &Hub{
        rooms:     rooms,
        broadcast: broadcast,
        players:   players,
        cfg:       cfg,
    }
}
```

- [ ] **Step 4: Make store implementations match interfaces**

Ensure `store.RedisStore` and `store.PostgresStore` satisfy the new interfaces.

- [ ] **Step 5: Update `server` package**

```go
// backend/internal/server/main.go
redisStore := store.NewRedisStore(cfg)
pgStore := store.NewPostgresStore(cfg)

hub := game.NewHub(
    pgStore,      // implements game.RoomRepository
    redisStore,   // implements game.Broadcaster
    pgStore,      // implements game.PlayerStore
    cfg,
)
```

- [ ] **Step 6: Remove `store` import from game package**

```go
// backend/internal/game/*.go — remove import of store
```

- [ ] **Step 7: Verify**

```bash
cd backend
go vet ./...
golangci-lint run
go test ./... -count=1 -short
cd ..
```

- [ ] **Step 8: Commit**

```bash
git commit -m "refactor(backend): invert game→store dependency via RoomRepository/Broadcaster/PlayerStore interfaces"
```

---

### Task 3.3: Make `handler` fully interface-dependent

**Files:**
- Modify: `backend/internal/handler/*.go` — define local interfaces, inject via constructor params
- Modify: `backend/internal/server/main.go` — pass interface implementations

**Interfaces:**
- Consumes: Interfaces from Tasks 3.1 and 3.2
- Produces: Handler constructors accept only interfaces

- [ ] **Step 1: Read each handler file to see its imports**

```bash
rg "^import" backend/internal/handler/*.go
```

- [ ] **Step 2: For each handler group, define the interfaces it needs**

```go
// backend/internal/handler/auth_session.go

type UserStore interface {
    FindOrCreateByEmail(ctx, email) (*domain.User, error)
}
type SessionManager interface {
    CreateSession(ctx, user) (*domain.Session, error)
    ValidateSession(ctx, token) (*domain.User, error)
}

func NewAuthHandler(us UserStore, sm SessionManager) *AuthHandler {
    return &AuthHandler{users: us, sessions: sm}
}
```

```go
// backend/internal/handler/lobby.go

type GameService interface {
    CreateRoom(ctx) (*domain.Room, error)
    CheckRoom(ctx, code) (*domain.RoomStatus, error)
}
type LobbyStore interface {
    ListLobbies(ctx, cursor, limit) (*domain.LobbyPage, error)
}

func NewLobbyHandler(gs GameService, ls LobbyStore) *LobbyHandler {
    return &LobbyHandler{game: gs, lobbies: ls}
}
```

- [ ] **Step 3: Update `server` package**

```go
// backend/internal/server/server.go
import (
    "github.com/uppy-clone/backend/internal/handler"
    "github.com/uppy-clone/backend/internal/store"
    "github.com/uppy-clone/backend/internal/auth"
    "github.com/uppy-clone/backend/internal/game"
)

func wireHandlers(cfg *config.Env) {
    pgStore := store.NewPostgresStore(cfg)
    redisStore := store.NewRedisStore(cfg)
    authMgr := auth.NewManager(pgStore, redisStore, pgStore, cfg)
    gameHub := game.NewHub(pgStore, redisStore, pgStore, cfg)

    authH := handler.NewAuthHandler(pgStore, authMgr)
    lobbyH := handler.NewLobbyHandler(gameHub, pgStore)
    // ...
}
```

- [ ] **Step 4: Verify no handler imports `store` or `auth` directly**

```bash
rg "\"github.com/uppy-clone/backend/internal/store\"" backend/internal/handler/
rg "\"github.com/uppy-clone/backend/internal/auth\"" backend/internal/handler/
# Both should show zero results
```

- [ ] **Step 5: Verify**

```bash
cd backend
go vet ./...
golangci-lint run
go test ./... -count=1 -short
cd ..
```

- [ ] **Step 6: Commit**

```bash
git commit -m "refactor(backend): handler now depends only on interfaces, not concrete implementations"
```

---

### Task 3.4: Decouple `middleware` → `auth`

**Files:**
- Modify: `backend/internal/middleware/ratelimit.go` — accept `AuthChecker` interface
- Modify: `backend/internal/middleware/tracing.go` — accept `AuthChecker` interface
- Modify: `backend/internal/middleware/middleware_core.go` — accept interfaces via options
- Modify: `backend/internal/server/main.go` — pass auth implementations

**Interfaces:**
- Produces: `middleware.AuthChecker`, `middleware.RateLimiter`

- [ ] **Step 1: Define interface in middleware package**

```go
// backend/internal/middleware/auth_checker.go
package middleware

import "context"

// AuthChecker provides authentication checking for middleware.
type AuthChecker interface {
    IsAuthenticated(ctx context.Context) bool
    GetUserID(ctx context.Context) (string, bool)
    HasRole(ctx context.Context, role string) bool
}
```

- [ ] **Step 2: Update middleware constructors**

```go
// backend/internal/middleware/ratelimit.go — before
func RateLimit(auth *auth.Manager, store *store.RedisStore) func(http.Handler) http.Handler

// After
func RateLimit(checker AuthChecker, limiter RateLimiter) func(http.Handler) http.Handler
```

- [ ] **Step 3: Update `server` package**

```go
// In server, pass the auth manager as AuthChecker
authMiddleware := middleware.RateLimit(authMgr, redisStore)
```

- [ ] **Step 4: Verify**

```bash
cd backend
go vet ./...
golangci-lint run
go test ./... -count=1 -short
cd ..
```

- [ ] **Step 5: Commit**

```bash
git commit -m "refactor(backend): decouple middleware→auth via AuthChecker interface"
```

---

### Task 3.5: Decouple `rbac` → `auth`

**Files:**
- Modify: `backend/internal/rbac/rbac.go` — define `RoleProvider` interface
- Modify: `backend/internal/server/main.go` — wire

**Interfaces:**
- Produces: `rbac.RoleProvider`

- [ ] **Step 1: Read current rbac**

```bash
cat backend/internal/rbac/rbac.go
```

- [ ] **Step 2: Define interface**

```go
// backend/internal/rbac/rbac.go — at the top
package rbac

type RoleProvider interface {
    GetUserRoles(ctx context.Context, userID string) ([]string, error)
    HasPermission(ctx context.Context, userID string, permission string) (bool, error)
}

type Enforcer struct {
    roles RoleProvider
    // ...
}

func NewEnforcer(roles RoleProvider) *Enforcer {
    return &Enforcer{roles: roles}
}
```

- [ ] **Step 3: Have auth implement RoleProvider**

```go
// backend/internal/auth/roles.go — add adapter if needed
func (m *Manager) GetUserRoles(ctx context.Context, userID string) ([]string, error) {
    // existing logic
}
```

- [ ] **Step 4: Verify**

```bash
cd backend
go vet ./...
golangci-lint run
go test ./... -count=1 -short
cd ..
```

- [ ] **Step 5: Commit**

```bash
git commit -m "refactor(backend): decouple rbac→auth via RoleProvider interface"
```

---

## Phase 4: Verification & Cleanup

### Task 4.1: Full test suite pass

- [ ] **Step 1: Run all backend tests**

```bash
cd backend
go test ./... -count=1 -race 2>&1 | tail -20
cd ..
```

- [ ] **Step 2: Run all frontend tests**

```bash
cd frontend
npx vitest run --reporter verbose
cd ..
```

- [ ] **Step 3: Run lint**

```bash
cd backend
golangci-lint run
cd ..
cd frontend
npx tsc --noEmit
npx eslint src/
cd ..
```

### Task 4.2: Verify server composition root

- [ ] **Step 1: Read `backend/internal/server/main.go`** — confirm all wiring goes through this file

- [ ] **Step 2: Verify no other package imports `store` for types that should be behind interfaces**

```bash
# handler should no longer import store
rg "\"github.com/uppy-clone/backend/internal/store\"" backend/internal/handler/
# auth should no longer import store
rg "\"github.com/uppy-clone/backend/internal/store\"" backend/internal/auth/
# game should no longer import store
rg "\"github.com/uppy-clone/backend/internal/store\"" backend/internal/game/
```

### Task 4.3: Update ADR docs

- [ ] **Step 1: Create ADR-028 documenting interface extraction decision**

```markdown
# ADR-028: Clean Architecture Interface-Driven Decoupling

Date: 2026-07-03
Status: Accepted

## Context
The codebase had flat dependency structure where handler imported store/auth/game
directly. ...

## Decision
Adopt Clean Architecture layered approach with interfaces defined by consumers.
...
```

- [ ] **Step 2: Update ADR-025** to reflect that mutable singleton state has been replaced by controlled GameStore
