# 全项目代码简化计划

执行状态跟踪文档。原则见 [CONTRIBUTING.md](../../CONTRIBUTING.md) 测试约定。

## 进度

| Phase | 状态 | 说明 |
|-------|------|------|
| 0 基线与卫生 | 完成 | 构建产物清理、ADR-021 目录布局、`check-repo-layout` 门禁 |
| 1 email 加密 | 待办 | store 接线 + backfill |
| 2 测试基础设施 | 待办 | testutil 统一 + integration tag |
| 3 测试文件整理 | 部分完成 | `internal/game` 已合并为 3 文件；其余包待续 |
| 4 生产代码 | 待办 | config/hub/protocol/auth |
| 5 前端与文档 | 部分完成 | `message_codec.ts`、coverage 脚本路径、`benchmarks-*` 文档命名 |

## 约定

- 每包 1–3 个 `*_test.go`，表驱动 `t.Run`
- 共享 testcontainers 辅助代码：`backend/internal/testutil/`
- **禁止**为通过 `funlen` 机械拆测试文件
- 单元测试（`-short`）→ miniredis；集成测试 → testcontainers + `//go:build integration`
- 每步门禁：`make check`；阶段末：`make ci`

## 验证清单

- [ ] `make ci` 通过
- [ ] 所有测试无需改断言（Phase 1 加密路径除外）
- [ ] golangci-lint + frontend lint/typecheck 零新增告警
- [x] 无构建产物进 git
- [ ] integration 测试有 build tag，`-short` 不依赖 Docker
- [x] CONTRIBUTING 与本文档已更新（ADR-021 布局、`scripts/ci/`、`benchmarks-*` 命名）
- [ ] 每包测试文件数符合 1–3 约定（或记录合理例外）
- [x] `make check-repo-layout` 通过（ADR-021 目录门禁）

## 例外记录

- `backend/internal/metrics/record_test.go` 使用 `package metrics_test`（black-box 测试，有意保留）
