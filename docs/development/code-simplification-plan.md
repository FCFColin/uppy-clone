# 全项目代码简化计划

执行状态跟踪文档。原则见 [CONTRIBUTING.md](../../CONTRIBUTING.md)「Code Simplification」小节。章程约束见 [ADR-000](../adr/000-project-charter.md)。

## 五原则摘要

1. **行为不变** — 只改表达方式，不改输入/输出/副作用/错误行为
2. **遵循项目约定** — CONTRIBUTING、ADR-021、coverage-policy
3. **清晰优于 clever** — 显式代码优先
4. **保持平衡** — 不过度内联或合并无关逻辑
5. **范围可控** — 每 PR 一个主题；refactor 与 feat/fix 分 PR

## 进度

| Phase | 状态 | 说明 |
|-------|------|------|
| 0 基线 | 完成 | Redis healthcheck、roadmap |
| 1 卫生/CI | 完成 | CI 去重、deploy 镜像等待、load-smoke、legacy scripts 清理 |
| 2 testutil | 完成 | integration build tag |
| 3 后端测试 | 完成 | helpers + middleware/auth/handler 合并 |
| 4 后端生产 | 完成 | hub 谓词、lifecycle、config、magiclink、email 加密 |
| 5 前端 | 完成 | client_state_reset、connection_ui、ui_update、ws_connect 单路由 |
| 6 文档 | 完成 | CONTRIBUTING、docs/README |
| 7 game grace/restart | 完成 | `reconnectGraceExpired`、restart 计数 dedupe、nickname 路径 |
| 8 game helpers | 完成 | outbound/persist/cache helper 提取 |
| 9 前端 entry/phase | 完成 | `syncEntryOverlays`、phase hook 表 |
| 10 前端 snapshot/errors | 完成 | 纯 `decodeSnapshot` + `applySnapshot`、`routeConnectionError` |

## 验证清单

- [x] 后端 `go test ./... -short` 通过
- [x] 前端 Vitest 全量通过
- [x] `make check` 每步门禁
- [x] middleware 2 / auth 3 / handler 3 测试文件
- [x] integration build tag
- [x] Email 加密 store 接线（legacy 行兼容）

## 例外记录

- `backend/internal/metrics/record_test.go` — `package metrics_test`
- `backend/internal/game/room_outbound_test.go`、`room_contention_test.go` — 独立主题
- `backend/internal/game/player_test.go` — HandleSetNickname 单元测试（lifecycle 保留 msg 集成测试）
- `backend/internal/middleware/` — 2 测试文件（core + resilience）
