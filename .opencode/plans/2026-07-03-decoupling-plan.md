# 解耦计划设计文档

日期：2026-07-03
状态：已批准（待执行）

## 1. 目标

对整个代码库（Go 后端 + TypeScript 前端）实施 Clean Architecture 接口驱动的解耦重构，目标：

- **降低变更影响面**：修改底层实现（store、protocol、config）不会级联影响上层包
- **提升可测试性**：通过接口注入，每个包可独立 mock 测试
- **明确模块边界**：消除前端可变全局状态的随意修改，建立受控状态管理
- **减少编译/类型检查级联**：减少单文件变更触发的大面积重编译

## 2. 范围

- **后端**：`backend/internal/` 下全部 22 个 Go 包
- **前端**：`frontend/src/` 下全部 ~50 个 TypeScript 源文件
- **排除**：`cmd/` 工具入口、`migrations/` SQL 文件、`deploy/` 基础设施配置

## 3. 后端架构设计

### 3.1 目标分层

```
┌──────────────────────────────────────────────────┐
│                Handler 层 (handler)               │
│  依赖：接口（定义在 handler 或共同接口包中）        │
│  不导入具体实现（store.Store 等）                  │
├──────────────────────────────────────────────────┤
│               Application 层 (auth/game)          │
│  业务编排，定义自己所需的存储/服务接口              │
│  auth → 定义 UserStore, SessionStore 接口         │
│  game → 定义 RoomRegistry, Broadcaster 接口       │
├──────────────────────────────────────────────────┤
│               Domain 层 (domain)                  │
│  纯领域模型，零内部依赖                           │
│  依赖 validate → 改为 domain 定义接口，注入实现    │
├──────────────────────────────────────────────────┤
│            Infrastructure 层                      │
│  store, protocol, crypto, config, validate,       │
│  metrics, audit, telemetry, resilience 等          │
│  实现 Domain 和 Application 层定义的接口           │
└──────────────────────────────────────────────────┘
         ↓ 依赖方向：外层 → 内层接口
         内层定义接口，外层实现
```

### 3.2 组合根（Dependency Injection）

`server` 包是 **唯一的组合根**——所有依赖在此组装：

```go
// server/main.go 或 server/wire.go
func NewServer(cfg *config.Env) *Server {
    db := store.NewPostgres(cfg.DatabaseURL)
    redis := store.NewRedis(cfg.RedisAddr)

    userStore := store.NewUserStore(db)           // 实现 handler.UserStore
    roomRepo := store.NewRoomRepository(db, redis)// 实现 game.RoomRepository
    sessMgr := auth.NewSessionManager(userStore, redis) // 实现 handler.SessionManager

    authHandler := handler.NewAuthHandler(sessMgr, userStore)
    gameHandler := handler.NewGameHandler(roomRepo)

    return &Server{/*...*/}
}
```

### 3.3 关键接口清单

| 消费方 | 接口 | 方法示例 | 实现方 |
|-------|------|----------|-------|
| `handler` | `UserStore` | `FindOrCreateByEmail` `GetUserByID` | `store` |
| `handler` | `SessionManager` | `CreateSession` `ValidateSession` `RevokeSession` | `auth` |
| `handler` | `GameService` | `CreateRoom` `GetRoomStatus` `ListLobbies` | `game` |
| `handler` | `LobbyStore` | `ListLobbies` `CheckRoom` | `store` |
| `handler` | `LeaderboardStore` | `GetGlobalRanking` `GetWeeklyRanking` | `store` |
| `handler` | `ConfigStore` | `LoadConfig` `SaveConfig` | `store` |
| `handler` | `AuditLogger` | `LogAction` | `audit` |
| `auth` | `UserStore` | `FindOrCreate` `GetByID` `SoftDelete` | `store` |
| `auth` | `SessionStore` | `CreateSession` `GetSession` `RevokeSession` | `store` |
| `auth` | `MagicLinkStore` | `CreateToken` `ConsumeToken` | `store` |
| `auth` | `NicknameGenerator` | `GenerateNickname` | `nicknames` |
| `auth` | `EmailSender` | `SendMagicLink` `SendNotice` | `worker` |
| `game` | `RoomRepository` | `SaveRoom` `GetRoom` `DeleteRoom` | `store` |
| `game` | `Broadcaster` | `BroadcastToRoom` `BroadcastToLobby` | `store` |
| `game` | `GameAuditLogger` | `LogGameEvent` | `audit` |
| `game` | `PlayerStore` | `SaveResult` `GetPlayerHistory` | `store` |
| `domain` | `NicknameValidator` | `ValidateNickname` | `validate` |
| `middleware` | `AuthChecker` | `IsAuthenticated` `HasRole` | `auth` |
| `middleware` | `RateLimiter` | `Allow` `Increment` | `store+auth` |
| `rbac` | `RoleProvider` | `GetUserRoles` `HasPermission` | `auth` |

### 3.4 包依赖变化示例

**handler 包（变化最大）：**

```go
// 现状
import "github.com/uppy-clone/backend/internal/store"
import "github.com/uppy-clone/backend/internal/auth"
import "github.com/uppy-clone/backend/internal/game"

func NewAuthHandler(store *store.PostgresStore, auth *auth.Manager) *Handler

// 目标
import "" // 不导入 store/auth/game 具体包

type UserStore interface {
    FindOrCreateByEmail(ctx, email) (*domain.User, error)
    GetUserByID(ctx, id) (*domain.User, error)
}
type SessionManager interface {
    CreateSession(ctx, user) (*domain.Session, error)
    ValidateSession(ctx, token) (*domain.User, error)
}

func NewAuthHandler(us UserStore, sm SessionManager) *Handler
```

**auth 包：**

```go
// 现状
type Manager struct {
    pgStore *store.PostgresStore
    redis   *store.RedisStore
}

// 目标
type UserStore interface {
    FindOrCreate(ctx, email) (*domain.User, error)
    GetByID(ctx, id) (*domain.User, error)
}
type SessionStore interface {
    CreateSession(ctx, user, ttl) (*domain.Session, error)
    GetSession(ctx, token) (*domain.User, error)
}
type Manager struct {
    users    UserStore
    sessions SessionStore
    magic    MagicLinkStore
}
```

### 3.5 具体解耦点

| 当前 | 目标 |
|------|------|
| `config` 单一包，7 个导入者 | 拆分为 `ServerConfig` `DBConfig` `RedisConfig` `AuthConfig` `GameConfig` 接口，各消费方只依赖自己需要的子集 |
| `metrics` 导入 `protocol` 取消息类型 | 消息类型常量移入 `domain` 或新建 `backend/internal/constants` 叶子包 |
| `domain` 导入 `validate` | `domain` 定义 `Validator[Nickname]` 接口，`validate` 实现并注入 |

### 3.6 不变的事项

- **无循环依赖风险**：Go 编译器强制 DAG
- **`server` 包仍然是唯一组合根**：不引入 DI 框架
- **`protocol` 包保持纯数据**：不依赖任何内部包
- **接口定义位置**：如果接口被多个消费方共享，提取到 `domain` 或新的 `backend/internal/port` 包

## 4. 前端架构设计

### 4.1 目标分层

```
┌─────────────────────────────────────────────────┐
│              Page Layer (入口层)                  │
│  main.ts / index.ts / leaderboard.ts / admin.ts  │
│  轻量引导，不包含业务逻辑                         │
├─────────────────────────────────────────────────┤
│              Game Layer (游戏逻辑层)              │
│  renderer/, input.ts, entry_flow.ts, phase_sync  │
│  读取 Store 状态，不直接修改                     │
│  通过 dispatch(action) 发起状态变更               │
├─────────────────────────────────────────────────┤
│              Store Layer (状态管理层)              │
│  store.ts — 唯一的 ClientState 修改入口           │
│  提供 dispatch(action) + getState(selector)      │
├─────────────────────────────────────────────────┤
│          Infrastructure Layer (基础设施层)        │
│  ws_connection.ts, message_codec.ts, audio.ts    │
│  auth.ts, session.ts, fetch.ts                   │
│  无业务状态，纯工具                               │
├─────────────────────────────────────────────────┤
│          Shared Constants Layer (共享常量层)      │
│  shared/types.ts, shared/constants.ts            │
│  shared/protocol.ts — 纯数据，零内部依赖           │
└─────────────────────────────────────────────────┘
```

### 4.2 受控状态管理 Store

```typescript
// src/game/store.ts  — 核心接口

// Action 类型
type GameAction =
    | { type: 'SET_PHASE'; phase: GamePhase }
    | { type: 'UPDATE_SNAPSHOT'; snapshot: ServerSnapshot }
    | { type: 'ADD_RIPPLE'; ripple: Ripple }
    | { type: 'UPDATE_PLAYERS'; players: PlayerState[] }
    | { type: 'UPDATE_SCORES'; scores: Record<string, number> }
    | { type: 'SET_WAITING'; waiting: boolean }
    | { type: 'RESET' }

// Store 接口
interface GameStore {
    dispatch(action: GameAction): void;
    getState(): ClientState;
    select<T>(selector: (state: ClientState) => T): T;
}
```

**约束：**
- **写必须通过 `dispatch()`**：没有导出 `state` 对象
- **读通过 `getState()`**：返回完整快照或通过 `select()` 选取子集
- **渲染器通过 `select()` 读取**：不再调用 `document.getElementById` 判断 UI 状态
- **没有 Redux**：使用简单的 `dispatch + reducer` 模式，不移入额外依赖

### 4.3 模块重组

**shared/ 子领域化：**

```
frontend/src/shared/
├── network/          # 网络相关
│   ├── auth.ts       # (原 shared/auth.ts)
│   ├── session.ts    # (原 shared/session.ts)
│   └── fetch.ts      # (原 shared/fetch.ts)
├── game/             # 游戏数据
│   ├── types.ts      # (原 shared/types.ts)
│   ├── constants.ts  # (原 shared/constants.ts)
│   └── protocol.ts   # (原 shared/protocol.ts)
├── ui/               # UI 工具
│   ├── audio.ts      # (原 shared/audio.ts)
│   └── toast.ts      # (原 shared/toast.ts)
├── data/             # 数据持久化
│   ├── tutorial_cookie.ts
│   └── best_score_cookie.ts
└── assets/            # 自动生成
    └── nickname_pools_gen.ts
```

**game/ 模块清理：**

| 变更 | 说明 |
|------|------|
| 消除 `state.ts` 桶文件 | 各模块直接导入 `state_types.ts` `state_interp.ts` `state_reset.ts` |
| 消除 `websocket.ts` 桶文件 | 各模块直接导入 `ws_connection.ts` `ws_connect.ts` |
| 消除 `game/constants.ts` | game 模块直接导入 `shared/game/constants.ts` 等 |
| 合并 `entry_flow.ts` + `entry_flow_dom.ts` | 不再伪分离 |
| 从 `renderer.ts` 抽取 DOM 查询 | 改为从 `store.select()` 读取 UI 状态 |
| 从 `main.ts` 抽取事件/生命周期 | 新建 `window_events.ts` `lifecycle.ts` |
| 消除 `ui.ts` ↔ `ui_update.ts` 交叉导入 | `ui.ts` 只做桶文件或只做实现，不同时担任两个角色 |

### 4.4 UI 状态 vs 渲染解耦

```typescript
// 现状: renderer.ts 直接查 DOM
function overlayBlocksGameRender(): boolean {
    if ($endedScreen && !$endedScreen.classList.contains('hidden')) return true;
    if ($restartVotePanel && !$restartVotePanel.classList.contains('hidden')) return true;
    return false;
}

// 目标: renderer.ts 读 store
function overlayBlocksGameRender(): boolean {
    return store.select(s => s.phase === 'ended' || s.showRestartVote);
}
```

UI 模块在状态转换时负责 `dispatch({ type: 'SET_PHASE', phase: 'ended' })`，同时自行操作 DOM。渲染器不再接触 DOM。

### 4.5 不变的事项

- **无运行时框架**：依然是纯 TypeScript + Canvas 2D
- **无状态管理库**：简单的 dispatch/reducer 模式
- **Vite 多页面构建不变**：5 个独立 HTML 入口
- **测试框架不变**：继续使用 Vitest + jsdom

## 5. 迁移策略

### Phase 1: 低风险、高可见性（Week 1: 3-5 天）

| # | 任务 | 领域 | 预计 |
|---|------|------|------|
| 1.1 | 前端 `shared/` 重组为子目录 | Frontend | 0.5d |
| 1.2 | 消除 `game/constants.ts` 重复导出 | Frontend | 0.5d |
| 1.3 | 消除 `state.ts` `websocket.ts` 桶文件 | Frontend | 0.5d |
| 1.4 | 消除 `ui.ts` ↔ `ui_update.ts` 交叉导入 | Frontend | 1d |
| 1.5 | 合并 `entry_flow.ts` + `entry_flow_dom.ts` | Frontend | 0.5d |
| 1.6 | 拆分 `config` 为子接口 | Backend | 1d |

### Phase 2: 核心解耦（Week 2: 5-7 天）

| # | 任务 | 领域 | 前置 | 预计 |
|---|------|------|------|------|
| 2.1 | `domain` → `validate` 依赖反转 | Backend | — | 1d |
| 2.2 | `metrics` → `protocol` 常量抽取 | Backend | — | 0.5d |
| 2.3 | 前端引入 `GameStore` 受控状态管理层 | Frontend | 1.3 | 2d |
| 2.4 | `renderer.ts` 消除 DOM 查询 | Frontend | 2.3 | 1d |
| 2.5 | `main.ts` 拆分事件/生命周期模块 | Frontend | 1.3 | 1d |

### Phase 3: 深度解耦（Week 3: 5-7 天）

| # | 任务 | 领域 | 前置 | 预计 |
|---|------|------|------|------|
| 3.1 | auth 定义 UserStore/SessionStore 接口 | Backend | 2.1 | 2d |
| 3.2 | game 定义 RoomRegistry/Broadcaster 接口 | Backend | 2.1 | 2d |
| 3.3 | handler 全面依赖接口 | Backend | 3.1, 3.2 | 1.5d |
| 3.4 | middleware → auth 解耦 | Backend | 3.1 | 0.5d |
| 3.5 | rbac → auth 解耦 | Backend | 3.1 | 0.5d |

### Phase 4: 验证与收尾（Week 3 end: 2-3 天）

| # | 任务 |
|---|------|
| 4.1 | 全量测试通过 |
| 4.2 | golangci-lint / vitest / tsc 验证 |
| 4.3 | 验证 `server` 包作为唯一组合根的正确性 |
| 4.4 | 更新 ADR 文档 |
| 4.5 | 检查 go mod 依赖缩减机会 |

### 原则

1. **每步可独立提交**：每个任务完成后测试、lint、typecheck 必须是绿色的
2. **先接口，后实现**：先在消费方定义接口并编译通过，再迁移调用方
3. **不改变行为**：解耦步骤零功能变更
4. **`server` 包集中注入**：所有依赖在 `server` 包中手动装配

## 6. 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| 接口定义不合适，后续需重构 | 中 | 中 | 用最小接口原则，宁可拆细不图大 |
| 前端 Store 引入导致回归 | 中 | 高 | Phase 2.3 先在测试覆盖高的模块试点 |
| Go 接口过多导致组合根臃肿 | 低 | 低 | 分组用 Option 模式 |
| 耗时超预期 | 中 | 低 | 每 Phase 结束后评估，必要时跳过部分任务 |
| 合并冲突（长期分支） | 中 | 中 | 每 Phase 合并到 main，不做 >1周的分支 |

## 7. 附录

### A. 后端耦合矩阵（现状）

| 包 | 导入的内部包数 | 备注 |
|----|--------------|------|
| `handler` | 12 | 最大 |
| `auth` | 11 | |
| `store` | 7 | |
| `server` | 13 | 组合根，可接受 |
| `middleware` | 7 | |
| `game` | 5 | |
| `config` | 0 | 被 7 包导入（扇入瓶颈） |

### B. 前端耦合矩阵（现状）

| 模块 | 被导入数 | 类型 |
|------|---------|------|
| `state.ts` / `state_types.ts` | ~20 | 全局可变单例 |
| `ui_elements.ts` | ~10 | DOM 引用 + 昵称逻辑 |
| `ws_connection.ts` | ~5 | WebSocket 生命周期 |
| `shared/auth.ts` | ~5 | 认证工具 |

### C. 参考文档

- ADR-000: 项目章程
- ADR-017: 限界上下文
- ADR-021: Monorepo 结构
- ADR-024: Application Service 层裁决
- ADR-025: 前端可变单例状态
- `architecture/architecture.md`: 系统架构
