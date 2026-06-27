# ADR-021: Monorepo 结构（Go + TS + infra + docs 同仓）

## 状态: 提议中（审计草稿，2026-06-26）

## 上下文

本项目是端到端学习工程，涵盖：
- Go 后端（`backend/`）
- TypeScript 前端（`frontend/`）
- 基础设施即代码（`infra/` Kustomize + Terraform）
- 可观测性部署（`deploy/` Prometheus/Grafana/Thanos）
- 架构文档与 ADR（`docs/`）
- E2E 测试（根 `tests/` + Playwright）

二进制 WS 协议常量必须在前后端保持同步（`protocol/constants.go` ↔ `shared/protocol.ts`）。

## 决策

采用 **单 Git 仓库 Monorepo** 结构：
```
/
├── backend/          # Go module (github.com/uppy-clone/backend)
├── frontend/         # npm package (零 runtime deps)
├── infra/            # Kustomize base + overlays
├── deploy/           # Prometheus/Grafana/Alertmanager/Thanos
├── docs/             # README hub, adr/, architecture/, operations/, api/, ...
├── scripts/          # 负载测试、工具脚本
├── tests/            # Playwright E2E
├── docker-compose.yml
├── Dockerfile        # 三阶段构建
└── Makefile          # 统一 dev/test/lint 入口
```

根 `package.json` 管理 dev 工具（Playwright、concurrently）；Go 开发工具通过 [`backend/tools.go`](../../backend/tools.go) 与主 `go.mod` pin（air、golangci-lint、migrate 等），由 `make dev` / `make audit` 构建到 `backend/bin/`。

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
