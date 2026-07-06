# 代码简化 TODO 清单

## 优先级：高（High）

### TODO-1: 统一 `getAuthenticatedUser` 函数
- **文件:** `middleware/auth_util.go:16`, `handler/auth_util.go:54`, `auth/middleware.go:144`
- **模式:** 重复逻辑
- **描述:** 三处完全相同的函数从 context 读取 userID 和 nickname
- **建议:** 提取到 `domain` 或 `requestctx` 包，其余两处删除并导入
- **工作量:** 小

### TODO-2: 统一 `buildAuthCookie` 函数
- **文件:** `handler/auth_util.go:24`, `auth/jwt.go:94`
- **模式:** 重复逻辑
- **描述:** 两处构建相同格式的 HttpOnly cookie
- **建议:** 统一到 `auth` 包（已有），`handler` 包调用 `auth.BuildAuthCookie`
- **工作量:** 小

### TODO-3: 统一 `refreshTokenFromRequest` 函数
- **文件:** `handler/auth_util.go:46`, `auth/jwt.go:119`
- **模式:** 重复逻辑
- **描述:** 两处完全相同的函数读取 refresh cookie
- **建议:** 统一到 `auth` 包
- **工作量:** 小

### TODO-4: 统一哨兵错误 `ErrTooManyRequests` / `ErrInvalidEmail`
- **文件:** `auth/magiclink.go:27-28`, `handler/auth_util.go:16-17`
- **模式:** 重复逻辑
- **描述:** 两处定义了相同的哨兵错误
- **建议:** 统一到 `handler` 包（或新建 `autherrors` 包）
- **工作量:** 小

## 优先级：中（Medium）

### TODO-5: 提取 panic recovery 辅助函数
- **文件:** `game/outbound_manager.go:98-114`, `game/outbound_manager.go:156-167`
- **模式:** 重复逻辑
- **描述:** 两处 `recover()` 闭包完全相同
- **建议:** 提取 `safeSend(ch, msg, logger)` 函数
- **工作量:** 小

### TODO-6: 提取 cookie 清零辅助函数
- **文件:** `handler/auth_gdpr.go:56-58`, `handler/auth_logout.go:28-30`
- **模式:** 重复逻辑
- **描述:** 两处清除 quickplay/session/refresh cookie 的模式相同
- **建议:** 提取 `clearAuthCookies(w, r)` 函数
- **工作量:** 小

### TODO-7: 提取 `tickOnce` 碰撞检查
- **文件:** `game/room_tick.go:104-118`
- **模式:** 重复逻辑
- **描述:** Ghost碰撞、Bird碰撞检查模式重复（check → EndGameWithReason → return）
- **建议:** 提取 `checkCollisions() (hit bool, reason uint8)` 方法
- **工作量:** 小

### TODO-8: 拆分 `enqueueGameResultAsync`
- **文件:** `game/room_result_async.go:35-102`
- **模式:** 长函数
- **描述:** 同时包含直写 PG 和 Redis 入队两条路径，逻辑交织
- **建议:** 拆分为 `recordGameResultDirect()` 和 `enqueueGameResultAsync()` 两个方法
- **工作量:** 中

### TODO-9: 精简过度注释（"企业为何需要"类）
- **文件:** 全库多处
- **模式:** 命名与可读性 - 注释太长
- **描述:** 大量 `// 企业为何需要：` 注释描述背景，增加了阅读负担但内容有价值
- **建议:** 将背景信息移到 ADR 文档，源文件仅保留必要的技术注释
- **工作量:** 中

### TODO-10: 提取 `startRedisSpan` 辅助函数
- **文件:** `store/redis_auth_session.go:16-20,34-38` 等多处
- **模式:** 重复逻辑
- **描述:** 每个方法都以相同模式创建 OpenTelemetry span
- **建议:** 提取 `startRedisSpan(ctx, name, op)` 辅助函数
- **工作量:** 小

## 优先级：低（Low）

### TODO-11: 统一可替换函数模式命名
- **文件:** 全库15+处
- **模式:** 代码一致性
- **描述:** `var ...Fn = ...` 模式用于测试注入，命名不统一（有的 `Fn` 后缀，有的 `Fn` 前缀）
- **建议:** 统一为 `SetXxxHook(…) (restore func())` 模式
- **工作量:** 大

### TODO-12: 提取 `setEndGameAlarm` 闭包为命名方法
- **文件:** `game/room_lifecycle.go:293-311`
- **模式:** 长函数 / 嵌套闭包
- **描述:** `time.AfterFunc` 中的闭包包含版本检查和阶段分支逻辑
- **建议:** 提取为 `endGameAlarmHandler()` 命名方法
- **工作量:** 小

### TODO-13: 精简哨兵 error 类型命名
- **文件:** `game/hub_ws_limiter.go:10-14`, `game/room.go:233-235`
- **模式:** 命名一致性
- **描述:** `wsConnectionLimitError` 和 `roomFullError` 的 `Error()` 方法前缀冗余
- **建议:** 保持哨兵模式但统一命名风格
- **工作量:** 小