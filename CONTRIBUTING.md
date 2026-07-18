# 贡献指南

感谢你对本项目的贡献！请遵循以下规范。

## 本地开发环境搭建

详见 [README.md 快速开始](README.md#快速开始)（`make dev` / `make check` / `make test-all` / `make e2e`）。

## 目录结构

仓库布局详见 [ADR-021 Monorepo 结构](docs/adr/021-monorepo-structure.md)。

## 测试约定

- 优先每包 1–3 个 `*_test.go`，使用 `t.Run` 表驱动子测试
- 禁止为通过 `funlen` 将单函数拆成 `{funcName}_test.go` 机械碎片
- 共享 testcontainers 辅助代码放在 `backend/internal/testutil/`
- **单元测试**（`make test`，带 `-short`）：使用 miniredis，无需 Docker
- **集成测试**（`make test-integration`，带 `-tags=integration`）：使用 testcontainers，需要 Docker
- Redis 策略：单元 → miniredis；集成 → testcontainers（`testutil.SetupRedisStore` / `SetupRedisClient`）
- 本地与 CI 一致：`make check`
- 完整 CI parity：`make ci`（含 testcontainers、audit、布局校验）；GitHub `go-ci` workflow 另含 golangci-lint、integration、安全扫描

## Code Simplification

重构与简化遵循 [代码简化技能](.cursor/skills/code-simplification/SKILL.md)。章程约束参见 [ADR-000](docs/adr/000-project-charter.md)（不得裁剪刻意保留的企业级组件，除非按 [ADR-032 瘦身例外条款豁免](docs/adr/032-slim-exception-waiver.md) 开具定向豁免）。每步运行 `make check`；阶段末 `make ci`。

## 代码风格

- Go: 遵循 [Effective Go](https://go.dev/doc/effective_go)，使用 golangci-lint 检查
- TypeScript: 遵循项目 ESLint 配置
- 运行 lint: `cd backend && golangci-lint run`
- 日志级别策略: 见 [docs/security/logging-policy.md](docs/security/logging-policy.md)

## Commit 规范

使用 [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

类型:
- feat: 新功能
- fix: 修复 bug
- docs: 文档变更
- style: 代码格式（不影响逻辑）
- refactor: 重构
- test: 测试
- chore: 构建/工具变更

示例:
```
feat(auth): add refresh token rotation
fix(room): prevent tick loop deadlock on shutdown
docs(adr): add circuit breaker decision record
```

## PR 提交规范

1. 从 main 创建 feature 分支: `feat/auth-refresh`
2. 确保所有测试通过: `go test -race ./...`
3. 确保 lint 通过: `golangci-lint run`
4. PR 描述包含: 变更内容、原因、测试方法
5. 至少 1 人 review 通过后方可合并

## 分支保护

- main: 禁止直接推送，必须通过 PR
- PR 必须通过 CI (test + lint + security)
- PR 必须 1 人 approve

## Pre-commit Hooks

提交前自动检查（密钥检测、golangci-lint、前端 lint 等）由 [.pre-commit-config.yaml](.pre-commit-config.yaml) 定义。
