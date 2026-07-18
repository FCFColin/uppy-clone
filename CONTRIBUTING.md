# 贡献指南

感谢你对本项目的贡献！请遵循以下规范。

## 本地开发环境搭建

### 前置要求
- Go 1.26+
- Node.js 20+
- Docker & Docker Compose
- Git

### 启动步骤

1. 克隆仓库
```bash
git clone <repo-url>
cd 多人网页游戏
```

2. 启动依赖服务
```bash
docker compose up -d postgres redis
```

3. 启动后端
```bash
cd backend
go mod download
go run ./cmd/server
```

4. 启动前端
```bash
cd frontend
npm ci
npm run dev
```

5. 运行测试
```bash
# 推荐：与 CI 一致的全量检查
make check

# 或分步：
make test          # 后端 -race -short
make test-integration
make lint-all
```

## 目录结构

仓库布局见 [ADR-021 Monorepo 结构](docs/adr/021-monorepo-structure.md)。要点：

- **后端**：`backend/cmd/server` 为薄入口；路由与生命周期在 `backend/internal/server`
- **前端**：wire 常量在 `frontend/src/shared/game/protocol.ts`；客户端编解码在 `frontend/src/game/message_codec.ts`
- **文档**：仅 `docs/{adr,architecture,operations,development,security,data,api,templates}/`
- **基础设施**：应用清单 `infra/k8s/`，GCP `infra/terraform/`；可观测性 `deploy/`（见各目录 `README.md`）
- **脚本**：CI `scripts/ci/`；布局校验 `make check-repo-layout`
- **E2E**：`make e2e` 或 `npm run test:e2e`（Playwright，`tests/e2e/`）

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

重构与简化遵循 [代码简化技能](.cursor/skills/code-simplification/SKILL.md)：

1. **行为不变** — 只改表达方式，不改语义
2. **遵循项目约定** — 见上文测试约定与 ADR-021
3. **清晰优于 clever** — 显式代码优先
4. **保持平衡** — 不过度内联或合并无关逻辑
5. **范围可控** — 每 PR 一个主题；`refactor` 与 `feat`/`fix` 分 PR

每步运行 `make check`；阶段末 `make ci`。章程约束见 [ADR-000](docs/adr/000-project-charter.md)（不得裁剪刻意保留的企业级组件，除非按 [ADR-032 瘦身例外条款豁免](docs/adr/032-slim-exception-waiver.md) 开具定向豁免）。

Install [air](https://github.com/air-verse/air) for live reload during development:

```bash
go install github.com/air-verse/air@latest
cd backend && air
```

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

## Commit Message Validation

Commit messages are validated by:
1. **pre-commit hook** (`conventional-pre-commit`): Validates format at commit time
2. **commitlint** (`commitlint.config.js`): Provides the rules configuration

If a commit is rejected, reformat your message:
```
type(scope): description

# Example:
feat(auth): add refresh token rotation
fix(room): prevent tick loop deadlock on shutdown
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

Install pre-commit to automatically run lint and tests before each commit:

```bash
pip install pre-commit
pre-commit install
pre-commit install --hook-type commit-msg
```

This will:
- Trim trailing whitespace
- Fix end-of-file issues
- Run golangci-lint on changed files
- Validate commit message format (Conventional Commits)
- Run short tests
- Detect accidentally committed private keys
