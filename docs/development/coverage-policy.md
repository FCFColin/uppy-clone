# 测试覆盖率策略

单元 100% / 集成 80% / 前端重要路径 90% / 单文件 60%（门禁脚本为准）。

## 分层

| 层级 | 命令 | 门禁 |
|------|------|------|
| 后端单元 | `make test-cover`（unit.out） | lines/branches/functions ≥ 100% |
| 后端集成 | integration profile | lines ≥ 80% |
| 前端 Vitest | `npm run test:frontend` | 见 `scripts/ci/check-coverage.sh` |

治理脚本：[`scripts/ci/check-coverage.sh`](../../scripts/ci/check-coverage.sh)

```bash
make test-cover
bash scripts/ci/check-coverage.sh unit backend/unit.out
bash scripts/ci/check-coverage.sh integration backend/int.out
bash scripts/ci/check-coverage.sh frontend
```

## 排除规则

纯类型、常量、入口 glue、renderer/UI 视觉代码等见脚本内 `EXCLUDE_PATTERNS`。修改排除列表需同步本文件与 CI。

## 约定

- 单元测试：`go test -short`，Redis 用 miniredis（ADR-023）
- 集成测试：`//go:build integration`，testcontainers
- 每 Go 包 1–3 个 `*_test.go`（见 [code-simplification-roadmap.md](code-simplification-roadmap.md)）
