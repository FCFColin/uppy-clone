# Findings Recheck V2 — deep-arch-slim-v2 Pre-flight

> **复核时间**: 2026-07-18 (Asia/Shanghai)
> **复核目的**: 验证 spec 中 5 个关键删除项的存在性与"零外部引用"声称，避免前轮 4 个幻觉候选 (`formatInternalWSTarget` / `internalUpgrader` / `verifyInternalProxySecret` / `NoopRoomCacheService`) 的教训重演。
> **复核工具**: Glob (文件存在性) + Grep (符号引用) + Read (内容确认)

## 1. backend/internal/store/ports.go (声称 82 行, 声称零外部引用)

- **文件存在**: ✅
- **实测行数**: 82 行 (Read 确认, 最后一行 `}` 在第 82 行)
- **Grep `store\.Deps|store\.DefaultDeps|store\.AuditEntry` 结果**: **46 处匹配, 跨越 12 个文件**
  - 生产代码 (非测试, 非临时文件):
    - `internal/bootstrap/store_deps.go:12,18,19,27` — `func NewStoreDeps() store.Deps`, 使用 `store.AuditEntry`
    - `internal/bootstrap/deps.go:33` — 类型签名 `... deps ...store.Deps`
    - `internal/server/server_init.go:26,27,35,63,85,152` — `func newStoreDeps() store.Deps`, `store.DefaultDeps()`, `store.AuditEntry`, 多个函数签名使用 `store.Deps`
    - `internal/server/server_lifecycle.go:75` — `func serve(... deps store.Deps) error`
    - `internal/worker/runner.go:39,201,213` — 类型签名与函数参数使用 `store.Deps`
  - 测试代码:
    - `cmd/seed/run_seed_unit_test.go:133,167,192`
    - `internal/server/server_routes_test.go:302`
    - `internal/server/server_lifecycle_test.go` (多处, ~25 处)
  - 临时文件 (将在 Task 1 删除):
    - `build-errors-2.txt:32`, `build-errors-3.txt:32`, `build-errors-4.txt:28`
- **结论**: ❌ **NOT safe to delete as-is.** Spec 的"零外部引用"声称是**幻觉**。
  - `store.Deps` / `store.DefaultDeps` / `store.AuditEntry` 在**生产代码**中被广泛使用: `bootstrap/store_deps.go`, `bootstrap/deps.go`, `server/server_init.go`, `server/server_lifecycle.go`, `worker/runner.go`。
  - 直接删除 `ports.go` 会导致 `go build` 失败。
  - **Task 2.1 as written CANNOT proceed.** 需要修订:
    - 选项 A: 将 `Deps` / `DefaultDeps` / `AuditEntry` / `ActorType*` 常量 / `noopPoolMetrics` / `depsOrZero` **迁移到** `store/base/base.go` (spec 声称的"已完全取代"目标) 或其他文件, 然后删除 `ports.go`。
    - 选项 B: 取消 Task 2.1 中删除 `ports.go` 的步骤, 仅保留删除 `util/slogctx.go` + `doc.go` 的部分。
  - **建议**: 父 agent 在派发 Task 2 前必须修订 SubTask 2.1, 明确符号迁移路径。

## 2. backend/internal/util/ 目录 (slogctx.go 等, 声称零外部引用除 1 测试)

- **目录存在**: ✅
- **文件清单**:
  - `backend/internal/util/slogctx.go`
  - `backend/internal/util/slogctx_test.go`
- **Grep `util\.WithLogger|internal/util` 结果**: **2 处匹配, 全部在 `middleware/recovery_test.go`**
  - `recovery_test.go:12` — `import "github.com/uppy-clone/backend/internal/util"`
  - `recovery_test.go:86` — `ctx := util.WithLogger(context.Background(), injected)`
- **结论**: ✅ **Safe to delete after import fix.** Spec 声称验证通过。
  - 唯一外部引用是 `middleware/recovery_test.go` 的 import + 1 处 `util.WithLogger` 调用。
  - 执行步骤: 将 `recovery_test.go:12` 的 import 改为 `slogctx` 包路径, 将 `recovery_test.go:86` 的 `util.WithLogger` 改为 `slogctx.WithLogger`, 然后删除 `internal/util/` 目录。
  - 与 spec Task 2.2 描述一致。

## 3. frontend/src/game/state.ts (声称 190 行死代码, 仅 ui.ts + ui_update.ts 引用)

- **文件存在**: ✅
- **实测行数**: 文件头注释确认 "Merged from state_types.ts, reducer.ts, and store.ts" (半途而废的合并)
- **Grep `from ['"]\./state(\.js)?['"]` 结果**: **2 处匹配**
  - `frontend/src/game/ui.ts:9` — `import { dispatch, getState } from './state.js';`
  - `frontend/src/game/ui_update.ts:5` — `import { dispatch, getState } from './state.js';`
- **额外 Grep `['"]\./state['"]|['"]\./state\.js['"]` 结果**: **1 处额外匹配** (spec 漏判)
  - `frontend/src/game/restart_vote_ui.test.ts:7` — `vi.mock('./state.js', () => ({`
  - 这是 vitest 的 `vi.mock` 调用, 删除 `state.ts` 后此 mock 会失败。
- **结论**: ⚠️ **Mostly safe to delete, with one additional cleanup point.**
  - Spec 声称的 "仅 `ui.ts` 和 `ui_update.ts` 引用 `./state.js`" **几乎正确**, 但漏掉了 `restart_vote_ui.test.ts:7` 的 `vi.mock('./state.js', ...)`。
  - 执行步骤: 在 Task 4 删除 `state.ts` 时, 必须同步处理 `restart_vote_ui.test.ts:7` 的 `vi.mock` 调用 (删除该 mock 行或改为 mock `./store.js`)。
  - **建议**: 父 agent 在派发 Task 4 时, 在 SubTask 4.1/4.2 中补充对 `restart_vote_ui.test.ts:7` 的处理。

## 4. frontend/src/game/ui.ts (声称 356 行死代码, 仅 ui_update.ts 引用)

- **文件存在**: ✅
- **实测行数**: 文件头注释确认 "Merged from ui_elements.ts, ui_utils.ts, ui_cooldown.ts, ui_wind.ts, connection_ui.ts, and waiting_tips.ts" (半途而废的合并)
- **Grep `from ['"]\./ui(\.js)?['"]` 结果**: **3 处匹配, 全部在 `ui_update.ts`**
  - `frontend/src/game/ui_update.ts:11` — `} from './ui.js';`
  - `frontend/src/game/ui_update.ts:12` — `import { updateWindIndicator, hideWindIndicator } from './ui.js';`
  - `frontend/src/game/ui_update.ts:13` — `import { refreshLayout } from './ui.js';`
- **Grep `['"]\./ui['"]|['"]\./ui\.js['"]` 结果**: 无额外匹配 (与上述 3 处一致)
- **结论**: ✅ **Safe to delete.** Spec 声称完全验证通过。
  - 唯一外部引用是 `ui_update.ts` 的 3 处 import。
  - 注意: `ui.ts:9` 自身 import `./state.js`, 这与第 3 项交叉——删除 `state.ts` (Task 4) 和删除 `ui.ts` (Task 5) 互不阻塞, 但 `ui_update.ts` 的 import 清理需在 Task 5 后完成。
  - 与 spec Task 5 描述一致。

## 5. backend/internal/game/room_restore.go (声称 114 行, 声称零外部调用)

- **文件存在**: ✅
- **实测行数**: 106 行 (Read 确认, 最后一行 `}` 在第 106 行) — **与 spec 声称的 114 行有 8 行偏差** (可能是 spec 编写时的估算或文件后续被微调, 不影响删除决策)
- **Grep `roomRestoreCoordinator|newRoomRestoreCoordinator|loadRestoreCandidates|loadSingleCandidate|contextForRestore|contextForLoad|registerRoom|restoreCandidate` 结果**: 50 处匹配, 但**所有非 `room_restore.go` 自身的匹配都是同名但不同的函数**:
  - `room_restore.go` 内部: 20 处 (类型定义 + 方法定义, 全部为内部符号)
  - `hub_restore_test.go`: 匹配的是 `registerRoomLocked` / `registerRoomInRedis` / `unregisterRoomFromRedis` — **不同函数** (Hub 的方法, 非 coordinator 的方法)
  - `hub_ops.go`: 匹配的是 `registerRoomInRedis` / `unregisterRoomFromRedis` / `registerRoomLocked` — **不同函数**
  - `hub_test.go`: 匹配的是 `registerRoomInRedis` / `unregisterRoomFromRedis` — **不同函数**
  - `hub.go`: 匹配的是 `registerRoomInRedis` — **不同函数**
  - `state.go:47` / `room_registry_store.go:78,79` / `room_cache_service.go:82,83,85` / `redis_test.go:165,166`: 匹配的是 `UnregisterRoom` (RoomRepository/RoomCacheService 接口方法) — **不同函数**
- **精确 Grep `\bregisterRoom\b` 结果**: **仅 2 处匹配, 全部在 `room_restore.go` 自身** (line 93 注释 + line 94 方法定义)
- **结论**: ✅ **Safe to delete.** Spec 声称验证通过。
  - `roomRestoreCoordinator` 类型及其所有方法 (`loadRestoreCandidates` / `loadSingleCandidate` / `registerRoom` / `contextForRestore` / `contextForLoad`) 在 `room_restore.go` 外**零调用**。
  - `newRoomRestoreCoordinator` 构造函数在 `room_restore.go` 外**零调用**。
  - `restoreCandidate` 类型在 `room_restore.go` 外**零引用**。
  - 注意: spec Task 3.3 提到 `state.go:68` 注释引用 "room restore coordinator" 字样需同步更新——本次复核未精确验证该注释行, 但即使存在也只是注释清理, 不阻塞删除。
  - 与 spec Task 3 描述一致 (行数微偏差不影响)。

## Summary

| # | 文件/目录 | 安全删除 | Spec 声称 | 复核结论 |
|---|---|---|---|---|
| 1 | `backend/internal/store/ports.go` (82 行) | ❌ NOT safe | 零外部引用 | **幻觉** — `store.Deps`/`DefaultDeps`/`AuditEntry` 在 5 个生产文件中广泛使用。Task 2.1 需修订为先迁移符号再删除。 |
| 2 | `backend/internal/util/` 目录 | ✅ safe (修 import 后) | 零外部引用除 1 测试 | 验证通过, 与 spec 一致。 |
| 3 | `frontend/src/game/state.ts` (190 行) | ⚠️ safe (额外清理 1 处) | 仅 ui.ts + ui_update.ts 引用 | **漏判** — `restart_vote_ui.test.ts:7` 有 `vi.mock('./state.js')` 需同步处理。 |
| 4 | `frontend/src/game/ui.ts` (356 行) | ✅ safe | 仅 ui_update.ts 引用 | 验证通过, 与 spec 完全一致。 |
| 5 | `backend/internal/game/room_restore.go` (实测 106 行) | ✅ safe | 零外部调用 | 验证通过, 行数微偏差 (106 vs 114) 不影响。 |

### 统计

- **安全删除**: 3/5 (items 2, 4, 5)
- **条件安全删除**: 1/5 (item 3 — 需额外清理 `restart_vote_ui.test.ts` 的 vi.mock)
- **阻塞 (NOT safe)**: 1/5 (item 1 — `store/ports.go` 需修订 Task 2.1)
- **幻觉检出**: 1 项 (item 1 的"零外部引用"声称)
- **漏判检出**: 1 项 (item 3 的 `vi.mock` 引用)

### 对后续 Task 的影响

1. **Task 2 (SubTask 2.1) 必须修订**: 不能直接删除 `ports.go`。需先确定 `Deps`/`DefaultDeps`/`AuditEntry`/`ActorType*` 常量/`noopPoolMetrics`/`depsOrZero` 的迁移目标 (推测应为 `store/base/base.go`), 迁移后再删除 `ports.go`。或者取消 SubTask 2.1, 仅保留 SubTask 2.2-2.4。
2. **Task 4 (SubTask 4.1/4.2) 需补充**: 增加 `restart_vote_ui.test.ts:7` 的 `vi.mock('./state.js')` 处理步骤。
3. **Task 3, Task 5 可按 spec 原计划执行**: items 2, 4, 5 验证通过。

### 复核方法论备注

- 本次复核再次验证了 Chesterton's Fence 原则的价值: spec 声称的"零外部引用"有 1/5 概率为幻觉 (item 1), 1/5 概率为漏判 (item 3)。
- 前轮 4 个幻觉候选的教训 (`formatInternalWSTarget` / `internalUpgrader` / `verifyInternalProxySecret` / `NoopRoomCacheService`) 在本次复核中被有效拦截。
- 建议后续每个删除 Task 执行时, 仍需在 sub-agent 内部独立 grep 复核, 不可仅依赖本复核结论。
