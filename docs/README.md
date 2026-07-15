# 项目文档

本目录包含架构、运维、API 契约与开发规范。修改代码时，请同步更新对应文档（见 PR 模板与 `docs-governance` CI）。

## 入门

| 文档 | 说明 |
|------|------|
| [ADR-000 项目章程](adr/000-project-charter.md) | 项目目标、非目标与刻意保留清单 |
| [系统架构](architecture/architecture.md) | 组件概览、数据流、已实现 vs 目标架构 |

## 开发

| 文档 | 说明 |
|------|------|
| [安全自检清单](security/self-check-checklist.md) | 发布阻塞项与分层安全自检（原 repo-cleanup 条目已并入） |
| [测试覆盖率策略](development/coverage-policy.md) | 单元/集成/前端覆盖率门禁 |
| [日志策略](security/logging-policy.md) | 日志级别、结构化字段、PII 禁止项 |
| [环境配置](operations/environments.md) | 本地 / staging / 生产配置矩阵 |
| [Go microbench 基准](development/benchmarks-go-microbench.md) | CI 生成的 `go test -bench` 输出（勿手改） |
| k6 / Room SLO 基准（已移除） | 负载测试与 Room Prometheus SLO 基线（文档已删除） |

## 运维

| 文档 | 说明 |
|------|------|
| [Runbook](operations/runbook.md) | 故障排查与处置手册 |
| [SLO](operations/slo.md) | SLI/SLO/SLA 与错误预算 |
| [容量规划](operations/capacity-planning.md) | 单实例容量与扩容触发条件 |
| [混沌实验](operations/chaos-experiments.md) | 故障注入实验目录 |
| [持续 profiling](operations/continuous-profiling.md) | Pyroscope 与 pprof |

## API 契约

| 文档 | 说明 |
|------|------|
| [OpenAPI](api/openapi.yaml) | REST API 规范 |
| [WebSocket 协议](api/ws-protocol.md) | 二进制 WS 消息说明 |
| [AsyncAPI](api/asyncapi.yaml) | WS 消息机器可读规范 |

> 修改 `backend/internal/protocol/constants.go` 时，必须同步 `api/ws-protocol.md` 与 `api/asyncapi.yaml`（CI 强制校验）。

## 架构与多区域

| 文档 | 说明 |
|------|------|
| [多区域拓扑](architecture/multi-region-topology.md) | Anycast、CRDB、区域 Redis、/resolve |
| [CockroachDB 迁移](data/cockroachdb-migration.md) | PG → CRDB 迁移 runbook |
| [SQL 查询分析](data/db-query-analysis.md) | 关键查询 EXPLAIN 分析 |

## 安全

| 文档 | 说明 |
|------|------|
| [威胁模型](security/threat-model.md) | STRIDE 威胁分析 |

## 架构决策（ADR）

完整索引见 [adr/README.md](adr/README.md)。

## 模板

| 文档 | 说明 |
|------|------|
| [Postmortem 模板](templates/postmortem.md) | P0/P1 事故复盘模板 |

## 路径变更说明（旧 flat 路径 → 当前位置）

| 旧路径 | 当前路径 |
|--------|----------|
| `docs/architecture.md` | `architecture/architecture.md` |
| `docs/runbook.md` | `operations/runbook.md` |
| `docs/slo.md` | `operations/slo.md` |
| `docs/chaos-experiments.md` | `operations/chaos-experiments.md` |
| `docs/coverage-policy.md` | `development/coverage-policy.md` |
| `docs/benchmarks-v2.md` | （已删除 — 迁移至 `development/benchmarks-go-microbench.md`） |
| `docs/benchmarks.md`（flat） | `development/benchmarks-go-microbench.md` |
| `infra/base/`、`infra/main.tf` | `infra/k8s/`、`infra/terraform/` |
| `scripts/check-*.sh` | `scripts/ci/` |
| `scripts/k6/` | `scripts/load/` |
| `docker/init-scripts/` | `docker/postgres/init/` |
| `docs/environments.md` | `operations/environments.md` |
| `docs/logging-policy.md` | `security/logging-policy.md` |
| `docs/threat-model.md` | `security/threat-model.md` |
| `docs/ws-protocol.md` | `api/ws-protocol.md` |
| `docs/multi-region-topology.md` | `architecture/multi-region-topology.md` |
| `docs/cockroachdb-migration.md` | `data/cockroachdb-migration.md` |
| `docs/db-query-analysis.md` | `data/db-query-analysis.md` |
| `docs/capacity-planning.md` | `operations/capacity-planning.md` |
| `docs/continuous-profiling.md` | `operations/continuous-profiling.md` |
