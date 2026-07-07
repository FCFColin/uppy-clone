# 测试覆盖率策略

单元 80% / 集成 80% / 前端重要路径 90% / 单文件 60%（门禁脚本为准）。

## 分层

| 层级 | 命令 | 门禁 |
|------|------|------|
| 后端单元 | `make test-cover`（unit.out） | lines ≥ 80% |
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

**Go 单元 profile**（`make test-cover` / `unit.out`）：仅 `./internal/...`，不含 `./cmd/...`（CLI 入口）、`internal/testutil`（测试辅助包）与 `internal/testsecrets`（测试常量包）。

**Go per-file 排除**（`EXCLUDE_PATTERNS`）：仅保留类型定义和入口 glue：
- `.d.ts` / `_types.ts` / `vite-env.d.ts` — 无可执行代码
- `cmd/server/main.go` — 进程入口 glue
- `testutil/` — 测试辅助包
- `constants.ts` / `constants.go` — 纯常量声明

**Vitest 无额外排除**：所有含业务逻辑的文件均纳入覆盖率门禁。入口 glue（`main.ts`、`index.ts`）由 per-file 排除规则覆盖。

## 约定

- 单元测试：`go test -short`，Redis 用 miniredis（ADR-023）
- 集成测试：`//go:build integration`，testcontainers
- 每 Go 包 1–3 个 `*_test.go`（见 `.cursor/skills/code-simplification/SKILL.md`）
- **例外**：`backend/internal/game` 与 `backend/internal/handler` 在 megfile 拆分后按主题分文件（lifecycle、physics、hub 等），由 `scripts/codegen/split_go_tests.py` 维护
