# 测试覆盖率重构设计方案

## 目标

将整体测试覆盖率提升到各维度 ≥80%，重要路径 ≥90%，个别文件 ≥60%，并建立完善的集成测试、属性测试和 E2E 测试体系。禁止"为测试而测试"，所有测试必须以对抗性思维发现问题。

---

## 1. 测试命名与文件组织

### Go 后端

```
internal/<package>/
  <package>_test.go              # 核心单元测试（所有导出函数）
  <package>_property_test.go     # 属性测试（不变量 invariant）
  <package>_fuzz_test.go         # 模糊测试
  <component>_<scenario>_test.go # 大型包按模块拆分
```

### TypeScript 前端

```
src/<module>/
  <module>.test.ts               # 核心测试
  <module>.property.test.ts      # 属性测试（fast-check）
  <module>.adversarial.test.ts   # 对抗性测试（错误注入/竞态）
```

### 函数/用例命名

- Go: `Test<Component>_<Scenario>`（如 `TestRoom_Tick_AppliesPhysics`）
- TS: `describe('<Module>') > describe('<Function>') > it('<scenario>_<outcome>')`
- 禁止使用 `Test1`、`Test2` 等无描述性的命名

### 大型文件拆分

- `server_lifecycle_test.go`（1208行）→ 按功能拆分为 3-4 个文件
- `handler/admin_handlers_test.go`（764行）→ 按 handler 拆分（已有基础，细化分组）

---

## 2. 测试基础设施修复

### 2.1 JWT PEM 问题

**涉及文件**：
- `backend/internal/auth/auth_flow_test.go:301`
- `backend/internal/handler/admin_handlers_test.go:167,508`
- `backend/internal/handler/admin_test.go:24,27,62,162,189`
- `backend/internal/middleware/middleware_resilience_test.go:109`

**修复方案**：所有 `NewJWTManager("test-secret-...")` 替换为 `NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)`。删除 `admin_test.go` 中的 `testJWTSecret` 常量。

### 2.2 Server 编译修复

**涉及文件**：`backend/internal/server/server_lifecycle_test.go`、`server_routes_test.go`

**修复方案**：
- 删除 `server_lifecycle_test.go` 中 4 处残留的 `serverEnv.`（lines 384, 546, 1068, 1145）
- `server_lifecycle_test.go:753` 补全 `startWorkers(ctx, &wg, cfg, redisStore, db, appConfig.DefaultTimeoutConfig())`
- `server_routes_test.go:34,288,289` 删除已移除的 `JWTSecret`/`AdminJWTSecret` 字段

### 2.3 Store pgxmock 修复

**涉及文件**：`backend/internal/store/postgres_users_gdpr_test.go:38-48`

**修复方案**：`TestAnonymizeUser_Success` 中补加 `mock.ExpectBegin()` 和 `mock.ExpectCommit()`。

---

## 3. 单元测试覆盖率提升

### 3.1 后端低覆盖包

| 包 | 当前 | 目标 | 主要缺失场景 |
|---|---|---|---|
| `internal/config` | 89.7% | 95% | 空环境变量、格式错误 URL、缺失必需字段、边界值 |
| `internal/domain` | 80.0% | 95% | 序列化反序列化、nil 接收者、空 ID、边界昵称 |
| `internal/validate` | 92.3% | 95% | 超长/空/Unicode/控制字符/HTML/XSS 注入 |

### 3.2 前端低覆盖文件

| 文件 | 当前 | 目标 | 主要缺失场景 |
|---|---|---|---|
| `best_score_cookie.ts` | 10.52% | 95% | Cookie 不存在、解析失败、过期、篡改 |
| `tutorial_cookie.ts` | 25% | 95% | 版本兼容、多次切换、损坏数据 |
| `audio.ts` | 6.89% | 95% | AudioContext 创建失败、播放错误、并发 |
| `toast.ts` | 7.69% | 95% | 多种类型、自动关闭、叠加消息、超长文本 |
| `lifecycle.ts` | 0% | 80% | DOMContentLoaded、可见性变化、页面关闭 |
| `window_events.ts` | 0% | 80% | resize 节流、focus/blur、visibility |

### 3.3 前端 vitest.config.ts 调整

- 从 `exclude` 列表中移除 `best_score_cookie.ts`、`tutorial_cookie.ts`、`audio.ts`、`toast.ts`、`state.ts`、`websocket.ts`、`state_interp.ts`
- 全局阈值调为 lines 85% / functions 85% / branches 80% / statements 85%
- 关键路径（`game/`、`shared/network/`、`shared/game/`）单独用 `thresholds` per-file 约束在 95%

---

## 4. 集成测试方案

### 新增集成测试文件

| 文件 | 路径 | 说明 |
|---|---|---|
| `game_room_lifecycle_test.go` | `tests/integration/` | 房间创建→加入→开始→结束→清理 完整流程 |
| `game_multiplayer_test.go` | `tests/integration/` | 多客户端同时交互 (2-8人) |
| `auth_full_flow_test.go` | `tests/integration/` | quickplay→session→refresh→logout 链条 |
| `ws_handler_test.go` | `tests/integration/` | WebSocket 升级→收发→断连→重连 |
| `admin_api_test.go` | `tests/integration/` | 管理后台 CRUD + 权限 |
| `rate_limiter_test.go` | `tests/integration/` | 限流阈值 + 恢复 |
| `outbox_test.go` | `tests/integration/` | outbox 发布 + worker 消费 + 死信 |

### 对抗性集成场景

- 并发房间创建/加入（竞态条件）
- DB 连接池耗尽时的降级行为
- Redis 宕机回退路径
- WebSocket 并发消息乱序
- 慢客户端检测 + 踢出
- 同时 GDPR 删除 + 活跃游戏

---

## 5. 属性测试（Property-Based Testing）

### 5.1 后端（Go）

Physics 不变量（`internal/game/physics_property_test.go`）：
- `TestPhysics_EnergyConservation` — 无外力时总能量守恒
- `TestPhysics_DragOpposesVelocity` — 阻力方向总与速度相反
- `TestPhysics_WindWithinBounds` — 风速始终在合理范围内
- `TestPhysics_BalloonCountConstant` — 气球总数守恒

Protocol 编解码对称性（`internal/protocol/property_test.go`）：
- `TestProtocol_EncodeDecodeRoundtrip` — 任意合法状态编码后解码等于原值
- `TestProtocol_NoPanicOnCorruptInput` — 损坏输入不 panic

State 不变量（`internal/game/state_property_test.go`）：
- `TestState_ScoresNonNegative` — 分数永不小于 0
- `TestState_PlayerCountInRange` — 玩家人数始终在 [2, maxPlayers]
- `TestState_PhaseTransitionValid` — 阶段转换图合法
- `TestState_DecodeRandom` — 随机状态帧解码不崩溃

Fuzz 测试扩展（`internal/protocol/decode_fuzz_test.go`）：
- 增加字段类型覆盖、边界大小（0 / 最大帧）、空输入

### 5.2 前端（TypeScript）

依赖：`npm install -D fast-check`

Protocol 编解码（`message_codec.property.test.ts`）：
- 任意帧结构 → encode → decode → 等于原值

State 不变量（`reducer.property.test.ts` + `store.property.test.ts`）：
- 任意合法 action 序列 → state 满足所有不变量

SnapShot 解码（`snapshot_decode.property.test.ts`）：
- 任意二进制缓冲区 → 解码不 panic、结果在合理范围

---

## 6. E2E 测试扩展

### 新增 Playwright 测试文件

| 文件 | 说明 |
|---|---|
| `tests/e2e/network_boundary.spec.ts` | 断网重连、超大帧、多 tab 同步、高延迟 |
| `tests/e2e/concurrency.spec.ts` | 8 人同时进入、同时 tap、结束时加入 |
| `tests/e2e/slow_client.spec.ts` | 低帧率、心跳超时、踢出重连 |
| `tests/e2e/security.spec.ts` | 篡改 cookie、无效 JWT、XSS、房间遍历 |

### E2E 测试基础设施改进

- 添加 `beforeAll` hooks 统一创建测试用户和房间
- 添加失败重试策略（flake 保护）
- 添加断言超时和等待策略的统一封装

---

## 7. 对抗性测试策略

所有测试必须遵循以下思维准则：

### 7.1 错误注入
- DB 操作：超时、连接断开、唯一约束冲突
- Redis：连接失败、OOM、key 过期
- WebSocket：立即关闭、损坏帧、大帧、空帧
- JWT：过期、篡改 payload、错误签名算法

### 7.2 竞态条件
- 并发房间创建 + 同时查询
- 同一玩家多 tab 同时操作
- GDPR 删除 + 活跃游戏
- outbox 发布 + 消费 + 重新投递

### 7.3 边界输入
- Go：nil map、零值 struct、空 slice
- TS：undefined、null、NaN、超大整数
- Protocol：40B ~ 64KB 任意二进制
- 昵称：含 null byte 的字符串、最长的合法输入、最短的合法输入

### 7.4 确定性模拟
- 利用已有 `deterministic.go` 的确定性 RNG
- 录制并重放 tick 序列验证状态一致性
- 1000x 加速模拟验证无死锁和内存泄漏

---

## 8. 覆盖率门禁调整

### 后端（`scripts/ci/check-coverage.sh`）

| 门禁 | 当前值 | 调整后 |
|---|---|---|
| 单元 total | 100% | 85%（语句覆盖率） |
| 单元 per-file | 60% | 60% |
| 单元 important paths | 90% | 90% |
| 集成 total | 80% | 85% |

### 前端（`frontend/vitest.config.ts`）

| 门禁 | 当前值 | 调整后 |
|---|---|---|
| lines | 95% | 85%（全局）/ 95%（关键路径）|
| functions | 95% | 85%（全局）/ 95%（关键路径）|
| branches | 80% | 80% |
| statements | 95% | 85%（全局）/ 95%（关键路径）|

### 新增门禁

- 属性测试：CI 中运行 `make test-property`
- E2E 测试：CI 中运行 `make e2e`（标记为 non-blocking）

---

## 9. 实现顺序

```
Step 1  — 修复测试基础设施（JWT PEM、server编译、store mock）
Step 2  — 调整覆盖率配置和门禁
Step 3  — 后端单元覆盖补齐（config/domain/validate）
Step 4  — 前端低覆盖文件补齐（cookie/audio/toast/lifecycle）
Step 5  — 后端集成测试扩展（6+ 新文件）
Step 6  — 属性测试（后端：physics/protocol/state，安装 fast-check）
Step 7  — 前端属性测试（protocol/reducer/input）
Step 8  — E2E 测试扩展（network_boundary/concurrency/security）
Step 9  — 对抗性测试整合（错误注入/竞态/确定性模拟）
Step 10 — CI 集成 + 最终门禁验证
```

---

## 10. 最终目标指标

| 指标 | 当前 | 目标 |
|---|---|---|
| 后端语句覆盖率 | ~83% | ≥85% |
| 后端重要路径 | ~90-96% | ≥90% |
| 前端行覆盖率 | 86.15% | ≥85% |
| 前端分支覆盖率 | 79.5% | ≥80% |
| 前端函数覆盖率 | 80.39% | ≥85% |
| 前端语句覆盖率 | 85.03% | ≥85% |
| 集成测试文件数 | 3 | ≥9 |
| 属性测试数 | 1 (fuzz) | ≥15 |
| E2E spec 数 | 7 | ≥12 |
| 测试总用例数 | ~516 | ≥800+ |
| 所有包测试通过 | ❌（4包失败） | ✅ |
