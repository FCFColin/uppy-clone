# ADR-021: Monorepo 结构（Go + TS + infra + docs 同仓）

## 状态: 已接受（2026-06-27）

## 上下文

本项目是端到端学习工程，涵盖：
- Go 后端（`backend/`）
- TypeScript 前端（`frontend/`）
- 基础设施即代码（`infra/terraform/` + `infra/k8s/`）
- 可观测性部署（`deploy/` Prometheus/Grafana/Thanos）
- 架构文档与 ADR（`docs/`）
- E2E 测试（根 `tests/e2e/` + Playwright）

二进制 WS 协议常量必须在前后端保持同步（`protocol/constants.go` ↔ `shared/protocol.ts`）。

## 决策

采用 **单 Git 仓库 Monorepo** 结构：
```
/
├── backend/
│   ├── cmd/server/           # 薄 main，调用 internal/server
│   ├── internal/             # 业务包 + server 组合根
│   ├── migrations/
│   └── tests/integration/
├── frontend/
│   ├── src/game/             # message_codec.ts（客户端编解码）
│   ├── src/shared/           # protocol.ts（wire 常量）+ fetch.ts
│   └── src/styles/
├── infra/
│   ├── terraform/            # GCP SQL、Redis、GSA
│   └── k8s/                  # base + overlays + global
├── deploy/                   # observability Kustomize + local/ 开发配置
├── docs/                     # adr/, architecture/, operations/, ...
├── scripts/
│   ├── ci/                   # coverage、digest 校验
│   ├── load/                 # k6
│   └── archive/
├── docker/postgres/init/     # DB 角色初始化 SQL
├── tests/e2e/                # Playwright
├── docker-compose.yml
├── Dockerfile
└── Makefile                  # make dev / check / ci / e2e
```

根 `package.json` 管理 dev 工具（Playwright、concurrently）；Go 开发工具通过 [`backend/tools.go`](../../backend/tools.go) 与主 `go.mod` pin（air、golangci-lint、migrate 等），由 `make dev` / `make audit` 构建到 `backend/bin/`。

### 布局约束（2026-06-27 目录整理后）

- **`docs/`**：禁止在根目录放置 flat 文档（`docs/README.md` 除外）；新文档必须归入 `adr/`、`architecture/`、`operations/` 等子目录。
- **`infra/`**：禁止在根目录放置 YAML/Terraform；应用清单仅允许 `k8s/`，GCP 资源仅允许 `terraform/`。
- **`backend/cmd/server/`**：仅保留薄 `main.go`；路由与生命周期代码位于 `internal/server/`。
- **`scripts/`**：CI 与负载脚本分别位于 `ci/`、`load/`；禁止在 `scripts/` 根目录放置 `.sh`/`.py`。
- **门禁**：`make check-repo-layout` 在 CI 中校验上述约束。

## 后果

**正面**
- 协议常量同步只需一个 PR
- ADR 与代码变更原子提交
- `make dev` / `make test` 统一入口
- CI 可在一个 workflow 中跑全栈检查

**负面**
- 仓库体积大（含 infra YAML、文档、lint 产物）
- 前后端 CI 耦合（前端 lint 失败阻塞后端发布）
- 权限粒度粗（无法对 infra 单独设 CODEOWNERS 生效域，除非配置 path-based rules）
- Clone 成本高

**放弃的替代方案**
- 前后端分仓：协议同步需跨仓 PR + 版本 pinning
- Polyrepo + git submodule： Submodule 管理痛苦
- Bazel/Nx 统一构建：对当前规模过度
