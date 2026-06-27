<!--
PR 模板：强制代码与文档同步演进（doc-code unity）。
请勾选所有适用项；不适用请写「N/A + 原因」。CI 的 Docs Governance 会校验 ADR 索引、
ws-protocol/AsyncAPI 契约与 Markdown 链接。
-->

## 变更摘要

<!-- 这次改了什么、为什么（侧重 why） -->

## 关联

- ADR / Issue:
- 影响的有界上下文 / 组件:

## 代码-文档同步检查（必填）

- [ ] 若改了 **WebSocket 协议**（`backend/internal/protocol/**`）：已同步 `docs/api/ws-protocol.md` 与 `docs/api/asyncapi.yaml`
- [ ] 若改了 **REST API**：已同步 `docs/api/openapi.yaml`
- [ ] 若引入/变更 **架构决策**：已新增或更新对应 ADR，并更新 `docs/adr/README.md` 索引
- [ ] 若改了 **部署/基础设施**（`infra/**`、`deploy/**`）：已同步相关文档（架构图 / runbook / capacity-planning / topology）
- [ ] 若改了 **多区域路由/容量/SLO** 相关行为：已更新 `docs/architecture/multi-region-topology.md` / `docs/operations/slo.md` / `docs/operations/capacity-planning.md`
- [ ] 数据库 schema 变更：已提供 up/down 迁移，且 PG/CRDB 兼容（必要时更新 `docs/data/cockroachdb-migration.md`）

## 测试

- [ ] 单元/集成测试已添加或更新
- [ ] `go build ./... && go test ./... -short` 通过（后端）
- [ ] `npx tsc --noEmit` 通过（前端，如涉及）

## 风险与回滚

<!-- 部署风险、特性开关、回滚方式 -->
