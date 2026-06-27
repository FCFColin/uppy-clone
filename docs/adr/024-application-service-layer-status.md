# ADR-024: Application Service 层状态与裁决需求

## 状态: 已接受（方案 A，2026-06-26）

## 上下文

ADR-017（限界上下文）规划了 handler → service → store 的分层架构，并引入了 CQRS 模式的 `QueryService` 和 `CommandService`。代码库中 `internal/service` 包包含五个服务类型：

| 服务 | 文件 | 生产代码引用 |
|------|------|-------------|
| `AuthService` | `auth_service.go` | **部分**：仅 `RefreshSession` 和 `DeleteUserData`（`handler/auth.go:211,300`） |
| `QueryService` | `query_service.go` | **零**（仅 `_test.go`） |
| `CommandService` | `command_service.go` | **零**（仅 `_test.go`） |
| `LobbyService` | `lobby_service.go` | **零**（仅 `_test.go`）；TODO 承认未迁移（`:10`） |
| `AdminService` | `admin_service.go` | **零**（仅 `_test.go`）；骨架 `VerifyLogin` 返回 `true`（`:44`） |

实际运行时路径：
- Auth：`handler` → `auth` 包函数 + 直接 `store` 调用（`auth.go:46,76,127,161,257-272`）
- Lobby：`handler` → `game.Hub` 直接（`lobby.go:27,73`）
- Admin：`handler` → `store` 直接（`admin.go:35-38`）

2026-06-26 全维度审计（AUDIT.md C-01）将此列为最大结构性风险。

## 决策

**团队已选择方案 A：删除死代码，拥抱当前实际架构。**

1. 删除 `QueryService`、`CommandService`、`LobbyService`、`AdminService` 及其测试
2. 将 `AuthService` 中已用的方法（`RefreshSession`、`DeleteUserData`）迁移至 `auth` 包
3. 删除整个 `internal/service` 包
4. 更新 ADR-017 为"handler → domain(auth/game) → store"的务实分层
5. 在 architecture.md 中移除 CQRS 描述

## 后果

- 消除架构模糊性，新人不再困惑"代码该放哪"
- 损失 CQRS 练习价值（但当前也未真正练习到）
- 一次性清理 ~200 行死代码

**放弃的替代方案**
- 方案 B（完成迁移至 service 层）：需 3-5 天迁移 + 全面回归测试
- 维持现状不做决策：最差选项
