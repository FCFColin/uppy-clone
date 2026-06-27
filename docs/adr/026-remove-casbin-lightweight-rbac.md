# ADR-026: 移除 Casbin，采用轻量 RBAC 策略表

## 状态: 已接受（2026-06-26）

## 上下文

`internal/rbac` 使用 Casbin v2 + `model.conf` + `policy.csv` 管理 4 角色、12 条策略。Casbin 适合大型动态策略，但本仓库策略静态且极小，引入完整引擎增加依赖与启动复杂度。

## 决策

1. 删除 Casbin 依赖与 `model.conf` 文件适配器
2. 在 `internal/rbac/permissions.go` 使用 `map[role]map[resource][]action` 表达策略
3. 保留 `Enforcer` 类型与 `Middleware(resource, action)` 签名，路由层零改动
4. `NewEnforcer()` 无文件路径参数

## 后果

- 减少 `go.mod` 依赖与镜像体积
- 策略变更需改代码并走 PR 审查（可接受：策略极少）
- 失去 Casbin 动态策略 API（本项目未使用）

## 放弃的替代方案

- 保留 Casbin：ROI 过低
- 完全内联到 middleware：失去集中策略视图
