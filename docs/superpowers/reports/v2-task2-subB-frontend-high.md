# v2 自检 Task2 子代理 B：前端 High 资产审查

> 生成日期：2026-07-08
> 范围：5 个前端 High 资产（A-036 / A-040 / A-041 / A-045 / A-050）
> 适用轴：正确性 + 可读性 + 架构 + 性能 + 可维护性（A-045/A-050 例外见各资产）
> 重点：5 个新专业轴（可观测性 / 可维护性 / 供应链 / 弹性 / 文档一致性），原 5 轴仅标记明显回归
> 纯诊断任务，未修改任何业务代码

---

## 资产 A-036: UI 层

### 基本信息
- 路径: `frontend/src/game/ui.ts` (barrel), `ui_elements.ts`, `ui_update.ts`, `ui_utils.ts`, `ui_cooldown.ts`, `ui_wind.ts`, `renderer.ts`, `renderer_canvas.ts`, `renderer_draw.ts`, `renderer_draw_effects.ts`, `renderer_background.ts`, `renderer_background_data.ts`, `visual_helpers.ts`, `waiting_tips.ts`, `tutorial.ts`, `restart_vote_ui.ts`, `connection_ui.ts`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 性能 + 可维护性

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 4 | 模块按职责清晰拆分（barrel + elements/update/utils/cooldown/wind）；无 TODO/FIXME；测试齐全；但 `ui_update.ts` 的 `_savedNickname` 缓存与 `localStorage` 隐式耦合，跨模块状态难追踪 |
| 可观测性 | 2 | 仅 `renderer.ts:57` `console.warn` 帧预算告警 + `:94` `console.error('Render error:')`，无远程遥测；`ui_utils.ts:71` 的 fallback 错误屏只渲染本地 UI，不上报 |
| 供应链 | N/A | 无运行时依赖（仅 devDependencies） |
| 弹性 | 3 | `renderer_background_data.ts:41` 图片加载 `onerror` 自动降级到 SVG fallback，良好；`renderer.ts:67` render 函数有 try/catch 防止单帧异常崩游戏循环；但 DOM 元素引用（`ui_elements.ts`）在模块加载时立即求值 `getElementById(...)!`，若 HTML 模板缺失会直接抛错 |
| 文档一致性 | N/A | 不适用 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-44 | 可维护性 | REQUIRED | `ui_update.ts:34-45` 的 `_savedNickname` 模块级缓存与 `invalidateNicknameCache()` 形成"缓存失效"耦合，任何修改 nickname 来源的改动都需记得调用 invalidate，变更影响半径扩大 | `frontend/src/game/ui_update.ts:34` | 改为每次从 `getState().pendingNickname` + `localStorage` 直接读取，或在 store 层统一 nickname 来源 |
| v2-O-30 | 可观测性 | OPTIONAL | 无前端错误遥测，渲染异常、帧预算超限、图片加载失败等仅 console 输出，生产环境无法观测 | `frontend/src/game/renderer.ts:57,94` | 引入 `sendBeacon` 上报关键错误，或集成轻量错误上报（保持零运行时依赖可只上报到自有端点） |
| v2-O-31 | 弹性 | OPTIONAL | `ui_elements.ts:7-24` 在模块加载时用 `document.getElementById(...)!` 非空断言抓 DOM，若 HTML 模板与代码不同步会在 import 阶段直接抛错，无降级 | `frontend/src/game/ui_elements.ts:7` | 改为惰性查找函数（参考 `ui_wind.ts:15-28` 的 `ensureElements()` 模式），或在 boot 阶段统一校验 |
| v2-F-25 | 可维护性 | FYI | `visual_helpers.ts:24-26` 的 `mutateFloatingTexts` 包装函数仅做 `mutate(floatingTexts)`，无额外逻辑，抽象冗余 | `frontend/src/game/visual_helpers.ts:24` | 可直接调用，移除包装 |
| v2-F-26 | 可维护性 | FYI | `renderer_background_data.ts:34-46` 的 `loadImageEntry` 第二参数 `cacheKey` 仅用于判断是否触发 staticCacheInvalidate，命名暗示缓存语义但实际是图片名，可读性略差 | `frontend/src/game/renderer_background_data.ts:34` | 重命名为 `imageKey` 或添加注释说明 |

### 整体健康度: 🟡 3.8/5

---

## 资产 A-040: 输入与同步

### 基本信息
- 路径: `frontend/src/game/input.ts`, `state_types.ts`, `state_reset.ts`, `state_interp.ts`, `interp_buffers.ts`, `store.ts`, `reducer.ts`, `ws_handlers.ts`, `ws_handlers_snapshot.ts`, `ws_handlers_phase.ts`, `ws_handlers_events.ts`, `ws_message_queue.ts`, `ws_connection.ts`, `ws_connect.ts`, `phase_sync.ts`, `message_codec.ts`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 性能 + 可维护性

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 4 | ws_handlers 按消息类型拆分子模块（snapshot/phase/events）+ barrel 聚合，职责清晰；reducer 为纯函数；但 `ws_connection.ts` 暴露 14+ 个独立 setter（`setWs`/`setWsEverOpened`/`setReconnectTimer`/`setRoomPreChecked`...），模块状态分散，变更影响半径大 |
| 可观测性 | 2 | `ws_handlers_phase.ts:13` 残留 `console.log` 调试输出（每次阶段变更都打印）；`ws_connect.ts:110` `console.error('WebSocket error')` 无上下文；`ws_handlers.ts:35` 未知消息类型仅 `console.warn`；无 RTT/丢包/重连指标上报 |
| 供应链 | N/A | 无运行时依赖 |
| 弹性 | 4 | `ws_connection.ts:144` 指数退避重连（base 1s, max 30s, 10 次）；`ws_connection.ts:37-52` 心跳 + 超时关闭机制；`ws_message_queue.ts` 入站消息缓冲 + 帧预算 drain（每帧 8 条）；`ws_connection.ts:75-92` 出站队列重连后 flush；但所有 fetch 调用无 AbortController 超时 |
| 文档一致性 | N/A | 不适用（ADR-025 一致性问题见 A-045） |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-45 | 可观测性 | REQUIRED | `ws_handlers_phase.ts:13` 残留 `console.log('[game-state-change]...')` 调试日志，生产环境每次阶段变更都打印，污染控制台且无级别控制 | `frontend/src/game/ws_handlers_phase.ts:13` | 移除或改为 `console.debug`，或封装到可关闭的 debug logger |
| v2-R-46 | 弹性 | REQUIRED | 整个前端无 `AbortController` 使用，所有 fetch 调用（`lobby_match.ts`、`room_validate.ts`、`session.ts`、`index.ts`、`admin_config.ts`、`leaderboard.ts`）无超时，网络挂起时 UI 永久等待 | `frontend/src/game/lobby_match.ts:9`、`frontend/src/game/room_validate.ts:26` 等全量 fetch | 关键路径 fetch 加 `AbortController` + 超时（如 8s），与 `lifecycle.ts:48` 的连接超时对齐 |
| v2-R-47 | 可维护性 | REQUIRED | `ws_connection.ts` 用 14+ 个模块级 `let` 变量 + setter 函数管理连接状态（`ws`/`reconnectAttempts`/`reconnectTimer`/`wsEverOpened`/`roomPreChecked`/`heartbeatInterval`/`heartbeatTimeout`/`lastPingTime`...），状态分散难以追踪，测试需 mock 大量 setter | `frontend/src/game/ws_connection.ts:23-31` | 收敛为单一 `connectionState` 对象 + reducer，或封装为 Connection 类 |
| v2-O-32 | 可观测性 | OPTIONAL | `ws_connection.ts:23-30` 重连次数、RTT、心跳超时等关键指标仅本地使用（`updatePingDisplay` 显示在 UI），无后端遥测，无法监控全量玩家连接质量 | `frontend/src/game/ws_connection.ts:71` | 用 `sendBeacon` 周期上报 RTT/重连事件到自有指标端点 |
| v2-O-33 | 弹性 | OPTIONAL | `ws_handlers_snapshot.ts:12` 仅校验 `view.byteLength < 44` 后调用 `decodeSnapshot`，但 `decodeSnapshot` 内部 `readPlayers` 等用 `view.byteLength < o + 11` 提前 break，对截断消息静默返回部分数据，无错误计数 | `frontend/src/game/message_codec.ts:104-124` | 对截断消息记录 warn 或返回 null，便于发现协议不一致 |
| v2-F-27 | 可维护性 | FYI | `state_interp.ts:177-192` 的 `seenSeqs` 用 Set + 半量删除策略防内存增长，注释提到 "Exposed for testing"，但 `getSeenSeqsSize` 在生产代码无引用 | `frontend/src/game/state_interp.ts:199` | 测试专用函数可加 `/* @internal */` 标注或移到测试辅助文件 |

### 整体健康度: 🟡 3.6/5

---

## 资产 A-041: 匹配与房间

### 基本信息
- 路径: `frontend/src/game/lobby_match.ts`, `room_validate.ts`, `entry_flow.ts`, `entry_flow_types.ts`, `lifecycle.ts`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 性能 + 可维护性

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 4 | `entry_flow.ts` 用 `EntryStep` 状态机（connecting→nickname→waiting→handoff）+ `STEP_RANK` 单调推进，逻辑清晰；`lifecycle.ts` 作为 boot 入口聚合初始化；但 `entry_flow.ts` 单文件 394 行，混合 DOM 操作 + 状态机 + 倒计时 + 错误路由，职责略多 |
| 可观测性 | 2 | `lobby_match.ts:18` `console.error('Failed to match room:', e)` 无上报；匹配失败、房间校验失败等关键转化漏斗无埋点 |
| 供应链 | N/A | 无运行时依赖 |
| 弹性 | 3 | `room_validate.ts:41-43` 网络异常时返回 `{ ok: true, degraded: true }` 优雅降级（允许进入房间），良好；`lifecycle.ts:48-51` 连接 8s 超时硬编码；但 `lobby_match.ts` 匹配失败仅返回 null，调用方 `ws_connect.ts:62` 直接报错无重试 |
| 文档一致性 | N/A | 不适用 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-O-34 | 弹性 | OPTIONAL | `lobby_match.ts:9` 调用 `/api/v1/registry/match` 失败（非 401）直接返回 null，无重试；匹配是进入游戏的关键路径，单次失败即阻断用户 | `frontend/src/game/lobby_match.ts:9-20` | 用 `fetchWithRetry`（已存在于 `shared/network/fetch.ts`）包装，至少重试 1 次 |
| v2-O-35 | 可维护性 | OPTIONAL | `entry_flow.ts:10-17` 在模块加载时用 `document.getElementById(...)` 抓 6 个 DOM 元素，与 `ui_elements.ts` 的非空断言模式重复，且 `$loadingSpinner`/`$loadingText` 用 `?.querySelector` 可能为 null | `frontend/src/game/entry_flow.ts:10` | 统一 DOM 引用策略到 `ui_elements.ts`，或用惰性查找 |
| v2-O-36 | 可维护性 | OPTIONAL | `entry_flow.ts` 单文件 394 行，承担状态机 + DOM 操作 + 倒计时 + 错误路由 + 匹配触发 5 类职责 | `frontend/src/game/entry_flow.ts` | 可拆分 `entry_flow_state.ts`（状态机）+ `entry_flow_dom.ts`（DOM 操作），但当前可读性尚可，低优先级 |
| v2-F-28 | 可维护性 | FYI | `entry_flow.ts:291-304` 的 `startStartCountdown` 命名重复（start + Start + Countdown），可读性略差 | `frontend/src/game/entry_flow.ts:291` | 重命名为 `beginWaitingCountdown` 或 `showEntryCountdown` |
| v2-F-29 | 弹性 | FYI | `lifecycle.ts:48` 连接超时 8000ms 与 `ws_connection.ts:144` 重连退避（1s→2s→4s...）独立，无协调；若 8s 内正在重连，超时仍会触发错误屏 | `frontend/src/game/lifecycle.ts:48` | 超时检查应感知 `reconnectAttempts > 0` 状态 |

### 整体健康度: 🟡 3.8/5

---

## 资产 A-045: shared/game

### 基本信息
- 路径: `frontend/src/shared/game/constants.ts`, `protocol.ts`, `protocol.test.ts`, `types.ts`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 性能 + 可维护性 + 文档一致性 + 弹性（7 轴）

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 4 | `constants.ts` 由 `go generate` 自动生成（`gen-frontend-constants`），头部有 "DO NOT EDIT" 标注；`protocol.test.ts` 覆盖常量唯一性 + 与后端对齐；但 `protocol.ts:31-47` 的 `phaseFromCode`/`phaseToCode` 函数无任何调用方（死代码），且测试未覆盖 |
| 可观测性 | N/A | 纯类型/常量模块，无运行时逻辑 |
| 供应链 | N/A | 无运行时依赖 |
| 弹性 | N/A | 纯协议定义模块 |
| 文档一致性 | 3 | `constants.ts` 与 `backend/internal/protocol/constants.go` 完全对齐（逐字段核对一致）；`protocol.ts` 常量与 `backend/internal/constants/protocol.go` 一致；但 `protocol.test.ts:4` 注释声称对齐 `ws-protocol.md`，而 `ws-protocol.md` 未列出 EndReason 常量，`local_constants.ts:15` 的 `END_REASON` 与后端 `constants.go:46-51` 一致但未在 shared/game 集中 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-48 | 可维护性 | REQUIRED | `protocol.ts:31-47` 的 `phaseFromCode` 和 `phaseToCode` 函数无任何调用方（`message_codec.ts:9-14` 自行实现了 `phaseByCode` 映射和 `codeToPhase` 函数），是死代码；测试也未覆盖 | `frontend/src/shared/game/protocol.ts:31-47` | 删除死代码，或将 `message_codec.ts` 的 `codeToPhase` 迁移到此模块统一 |
| v2-R-49 | 文档一致性 | REQUIRED | `docs/adr/025-frontend-mutable-singleton-state.md` 与实际代码多处不一致：(1) ADR 称 `store.select(selector)` 但 `store.ts` 无 select 方法；(2) ADR 称"不再直接变异全局对象"但 `store.ts:8` 用 `Object.assign(state, newState)` 直接变异；(3) ADR 称 `localStorage` 仅存 `uppy-player-id`，实际存 `uppy-nickname`/`uppy-game-url`；(4) ADR 称 lifecycle.ts 将 `state`/`__ws` 挂到 window，实际未挂载 | `docs/adr/025-frontend-mutable-singleton-state.md:17-24`、`frontend/src/game/store.ts:8`、`frontend/src/game/lifecycle.ts:25,56` | 更新 ADR-025 反映当前实现，或按 ADR 重构代码 |
| v2-O-37 | 文档一致性 | OPTIONAL | `protocol.ts:29` 的 `import { type GamePhase } from './types.js'` 放在文件中间（第29行，函数定义之间），虽然 ES module 合法但风格怪异，易误读为函数局部引用 | `frontend/src/shared/game/protocol.ts:29` | 移到文件顶部与其他 import 一起 |
| v2-O-38 | 可维护性 | OPTIONAL | `END_REASON` 常量定义在 `frontend/src/game/local_constants.ts:15-20`，而非 `shared/game/`，与后端 `protocol/constants.go:46-51` 的 `EndReasonNone/Ground/Bird/Ghost` 跨目录对齐，易遗漏 | `frontend/src/game/local_constants.ts:15`、`backend/internal/protocol/constants.go:46` | 迁移 `END_REASON` 到 `shared/game/protocol.ts`，由 go generate 同步 |
| v2-F-30 | 文档一致性 | FYI | `protocol.test.ts:4` 注释 "Must match backend/internal/protocol/constants.go and docs/api/ws-protocol.md"，但 `ws-protocol.md` 未列出 EndReason 常量，文档不完整 | `docs/api/ws-protocol.md`、`frontend/src/shared/game/protocol.test.ts:4` | 在 `ws-protocol.md` 补充 EndReason 字段定义 |

### 整体健康度: 🟡 3.4/5

---

## 资产 A-050: 页面入口

### 基本信息
- 路径: `frontend/src/index.ts`, `verify.ts`, `admin.ts`, `admin_login.ts`, `leaderboard.ts`, `index_leaderboard.ts`, `admin_config.ts`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 性能 + 可维护性

### 轴评分
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 4 | 每个页面入口职责单一，`admin.ts` 拆分 `admin_login.ts`/`admin_config.ts` 良好；`index_leaderboard.ts` 的折叠/展开状态机清晰；但 `index.ts:114` 的 `/api/v1/registry/check/${code}` 与 `room_validate.ts` 重复实现房间校验，逻辑分散 |
| 可观测性 | 2 | 所有页面入口的错误处理仅 `catch { showError(...) }` 本地 UI，无上报；`verify.ts:29` 魔法链接验证失败无埋点，无法追踪转化漏斗 |
| 供应链 | N/A | 无运行时依赖 |
| 弹性 | 2 | `index.ts:50` `fetch('/api/v1/auth/request')`、`leaderboard.ts:45` `fetch('/api/v1/leaderboard')`、`admin_config.ts:14,32` `fetch('/api/v1/admin/config')` 等均无超时、无重试；`leaderboard.ts:46` 错误仅提示"加载失败"，无自动重试 |
| 文档一致性 | N/A | 不适用 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-50 | 弹性 | REQUIRED | `index.ts:50,80,114,126`、`verify.ts:13`、`admin_config.ts:14,32`、`leaderboard.ts:45,64`、`index_leaderboard.ts:64` 等所有 fetch 调用均无 `AbortController` 超时，网络挂起时按钮永久禁用、UI 卡死 | `frontend/src/index.ts:50` 等全量 fetch 调用 | 关键路径加 `AbortController` + 超时；至少登录/验证路径需要 |
| v2-R-51 | 可维护性 | REQUIRED | `index.ts:114` 重新实现 `/api/v1/registry/check/${code}` 调用，与 `room_validate.ts:20-44` 的 `validateRoomCode` 重复逻辑（404 处理、错误消息），变更需同步两处 | `frontend/src/index.ts:100-145`、`frontend/src/game/room_validate.ts:20` | 抽取共享 `validateRoomCode` 到 `shared/network/`，两处复用 |
| v2-O-39 | 弹性 | OPTIONAL | `leaderboard.ts:45` 排行榜加载失败仅显示"加载失败，请确认后端已启动"，无重试按钮；`index_leaderboard.ts:99-103` 同样无重试 | `frontend/src/leaderboard.ts:56-59` | 添加"重试"按钮或自动重试一次 |
| v2-O-40 | 可维护性 | OPTIONAL | `index.ts:153` 用 `document.getElementById('join-code-input')!` 非空断言，但同文件其他元素引用（如 `:9-14`）用 `as HTMLInputElement` 类型断言，DOM 引用风格不统一 | `frontend/src/index.ts:9-14,101,153` | 统一 DOM 引用模式 |
| v2-O-41 | 可观测性 | OPTIONAL | `verify.ts:5-31` 魔法链接验证流程是关键认证转化点，但无任何埋点/上报，无法追踪验证失败率 | `frontend/src/verify.ts:5` | 用 `sendBeacon` 上报验证结果到自有指标端点 |
| v2-F-31 | 可维护性 | FYI | `admin.ts:1-9` 文件头注释详细描述功能，但 `admin_login.ts:1` 和 `admin_config.ts:1` 无对应注释，注释风格不一致 | `frontend/src/admin.ts:1` | 统一注释风格，或在 `admin.ts` 统一描述 |
| v2-F-32 | 可维护性 | FYI | `leaderboard.ts:76` 的 `isSafeUrl` 检查用 `gameUrl.startsWith('/')` + `new URL(gameUrl, origin).origin === origin` 双重校验，安全意识良好，但可提取为 `shared/` 工具函数复用 | `frontend/src/leaderboard.ts:76` | 提取到 `shared/util/safe_url.ts` |

### 整体健康度: 🟡 3.4/5

---

## 汇总统计

| 资产 | 整体评分 | CRITICAL | REQUIRED | OPTIONAL | FYI |
|------|---------|---------|---------|---------|-----|
| A-036 UI 层 | 🟡 3.8/5 | 0 | 1 | 2 | 2 |
| A-040 输入与同步 | 🟡 3.6/5 | 0 | 3 | 2 | 1 |
| A-041 匹配与房间 | 🟡 3.8/5 | 0 | 0 | 3 | 2 |
| A-045 shared/game | 🟡 3.4/5 | 0 | 2 | 2 | 1 |
| A-050 页面入口 | 🟡 3.4/5 | 0 | 2 | 3 | 2 |
| **合计** | — | **0** | **8** | **12** | **8** |

## 跨资产关键发现

### 系统性问题（多资产共有）

1. **无远程遥测（可观测性）**：5 个资产均无 Sentry/web-vitals/sendBeacon 等远程错误上报，仅 console + 本地 fallback UI。生产环境无法观测前端错误率、连接质量、转化漏斗。建议引入轻量自建上报（保持零运行时依赖）。

2. **fetch 无超时（弹性）**：A-040/A-041/A-050 共 10+ 处 fetch 调用均无 `AbortController`，网络挂起时 UI 永久等待。这是跨资产系统性弹性缺陷（v2-R-46, v2-R-50）。

3. **DOM 引用风格不一（可维护性）**：`ui_elements.ts` 用 `getElementById(...)!` 非空断言，`ui_wind.ts` 用惰性 `ensureElements()`，`entry_flow.ts`/`index.ts` 混用 `as` 断言与 `!` 断言。建议统一为惰性查找 + null 安全模式。

### 良好实践

1. **WS 重连退避**（A-040）：指数退避 + 上限 30s + 最多 10 次，符合弹性最佳实践。
2. **图片降级**（A-036）：`renderer_background_data.ts:41` 自动降级到 SVG fallback。
3. **房间校验降级**（A-041）：`room_validate.ts:41` 网络异常时返回 `degraded: true` 允许进入。
4. **协议常量自动生成**（A-045）：`constants.ts` 由 `go generate` 同步，避免前后端漂移。
5. **安全 URL 校验**（A-050）：`leaderboard.ts:76` 防 open redirect。

### 与 v1 评分对比（原 5 轴无明显回归）

v1 评分继承，本次审查未发现原 5 轴（正确性/可读性/架构/性能/可维护性）的明显回归。模块拆分、测试覆盖、纯函数 reducer 等良好实践保持。
