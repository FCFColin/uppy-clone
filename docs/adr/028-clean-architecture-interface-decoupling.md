# ADR-028: Clean Architecture Interface-Driven Decoupling

## 状态: 已接受

## 日期: 2026-07-03

## 上下文

代码库存在扁平的依赖结构，其中 `handler`、`middleware`、`rbac` 等上层包直接导入 `store`（PostgreSQL/Redis 实现）和 `auth`（认证逻辑）。这导致：
- 循环导入风险（已有 `domain` ↔ `validate` 循环）
- 单元测试必须携带完整基础设施（数据库、Redis）
- 违反依赖反转原则（DIP）：高层策略不应依赖低层细节
- `server` 包不是唯一的组合根 — 依赖散布在各包间

## 决策

采用 Clean Architecture 分层方法，由消费者定义接口：

### 模式

**接口定义在被消费者（handler/middleware/rbac），实现在基础设施（store/auth）**。

```
handler → [UserStore, TokenStore, AuthService, GameService, ...] ← store/auth
middleware → [JWTManager, RateLimiterStore] ← store/auth
rbac → 使用 domain.ContextKeyRole 读取角色
game → [RoomRepository, CacheStore, Broadcaster] ← store
auth → [UserDB, TokenStore] ← store
```

### 实施步骤（按依赖方向）

1. **config 拆分**（Phase 1.6）：`Env` 结构体实现 `ServerConfig`、`DBConfig`、`RedisConfig` 等子接口
2. **domain → validate 反转**（Task 2.1）：`domain.NicknameValidator` 接口，`validate` 包实现
3. **auth → store 反转**（Task 3.1）：`auth.UserDB`、`auth.TokenStore` 接口
4. **game → store 反转**（Task 3.2）：`game.RoomRepository`、`game.CacheStore`、`game.Broadcaster`
5. **handler 全接口化**（Task 3.3）：Handler 仅依赖 `domain`、`config`、`game` 和本地接口；零 `store`/`auth` 导入
6. **middleware → auth 解耦**（Task 3.4）：本地 `JWTManager` 接口 + `domain.ContextKey*` 读上下文
7. **rbac → auth 解耦**（Task 3.5）：使用 `domain.ContextKeyRole.Value()` 替代 `auth.RoleFromContext()`
8. **game → auth 解耦**（Phase 4）：内联 `GameEndedOutboxPayload` 到 `game` 包

### 共享上下文键

`domain.ContextKey` 类型（`internal/domain/context_keys.go`）被 `auth`、`handler`、`middleware` 共同使用，存储/读取认证信息。避免 Go 中不同包的 `type contextKey string` 被视为不同类型的问题。

### 适配器模式

`handler/default_auth_service.go` 实现 `handler.AuthService`，将 `auth` 包的函数式 API 适配为接口方法。
`server` 包是唯一的组合根，导入所有具体实现并完成依赖注入。

## 后果

**正面**
- 消除所有循环导入风险
- Handler 测试可用接口替身（stub/mock），无需真实数据库
- 每个包仅导入所需的最小接口集
- 依赖方向清晰：从上层 → 下层
- `server` 包成为唯一的组合根

**负面**
- 接口定义与实现分离，增加少量间接层
- 跨包共享类型（`domain.ContextKey`）需谨慎维护
- 需要适配器（`handler/default_auth_service.go`）桥接函数式 API 和接口

**验证结果**
- `go vet ./...` 零警告
- `go build ./...` 零错误
- 全部测试通过（仅 `TestValidateMagicToken_DeleteError` 和 `TestListLobbies_SuccessWithETag` 为预存不稳定测试）
- Handler 非测试文件零 `store`/`auth` 导入
- Auth 非测试文件零 `store` 导入
- Game 非测试文件零 `store`/`auth` 导入
- Middleware 非测试文件零 `auth` 导入
- RBAC 非测试文件零 `auth` 导入
