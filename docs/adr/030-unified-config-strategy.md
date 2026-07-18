# ADR-030: 统一配置策略

## 状态: 已落地（2026-07-16 收口）

## 上下文

代码库曾存在多套配置表示：`config.Env`（22 字段，有 `Validate()`）、`handler.Config`（15 字段扁平 struct，含 handler 不应感知的 `DatabaseURL`/`FrontendDir`）、`config/constants.go`（40+ 硬编码常量混装真正不可变常量与运行时参数）、25+ 文件裸调用 `os.Getenv()`、`server/server_config.go` 中的全局 `serverEnv` 变量。

## 决策

1. **`config.Env` 是唯一配置源**：所有运行时配置通过 struct 传递，在 `server` 包（组合根）创建一次，函数参数传递
2. **删除 `handler.Config`**：handler 改为接收自己的小 Config struct 或 `config.Env` 相关字段
3. **禁止裸 `os.Getenv()`**：所有环境变量读取必须通过 `config.Env` 字段或 `config.GetEnv`/`GetEnvInt`/`GetEnvDuration`；例外仅 `config/env.go` 的 `Load()`、`server/server_debug.go:initLogger()` 与 `worker/runner.go:initWorkerLogger()` 读取 `LOG_LEVEL`/`LOG_FORMAT`/`WORKER_HEALTH_PORT`（`config.Env` 创建前）
4. **常量分类**：`config/constants.go` 真正不可变保留 `const`（`RoomCodeLen=5`、`JWTIssuer`、`CookieMaxAge`）；运行时参数移入 `config.Env`（`MaxWSConnections`、`MaxPlayersPerRoom`）
5. **消除全局状态**：删除 `server/server_config.go` 中的 `var serverEnv *appConfig.Env`；`config.GetEnv*` 函数仅在 `Load()` 内部使用；组件通过构造函数参数接收 `*config.Env`

## 后果

**正面**：配置来源唯一（消除"改 A 没改 B"不一致）；Handler 不再感知基础设施配置；测试可注入 `config.Env` 控制配置；新增配置项只需修改 struct 与 `Load()`

**负面**：函数签名变长；需一次性迁移所有 `os.Getenv()` 调用

## 实现状态

五项决策全部落地：`config.Env` 为唯一配置源（`server/server_config.go:loadConfig()` 调 `appConfig.Load()`）；`handler.Config` 已删除；裸 `os.Getenv()` 仅存在于 `config/env.go`、测试文件及 §3 例外点；常量已分类；全局状态已消除。

**校验与默认值**：`config.Env.Validate()` 对必填项 fail-fast；`DatabaseURL` 始终必填，`JWT_PRIVATE_KEY`/`JWT_PUBLIC_KEY`/`ENCRYPTION_KEY`/`AUDIT_SECRET`/`TRUSTED_PROXY_CIDRS` 在 `ENV=production` 时必填且校验格式。运行时参数使用显式默认值，无 0 值静默回退。
