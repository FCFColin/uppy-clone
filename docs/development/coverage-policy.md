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

**Go 单元 profile**（`make test-cover` / `unit.out`）：仅 `./internal/...`，不含 `./cmd/...`（CLI 入口）与 `internal/testutil`（测试辅助包）。

**Go per-file 额外排除**（`EXCLUDE_PATTERNS`）：`cmd/server/main.go`、`constants.go`、`testutil/`、`degradation_deps.go` 等。

**Vitest 额外排除**（入口 glue / 纯 UI / 浏览器 API 封装，不计入 100% 门禁）：

- `src/game/main.ts`
- `src/admin.ts`, `src/admin_config.ts`, `src/admin_login.ts`
- `src/leaderboard.ts`, `src/index_leaderboard.ts`
- `src/game/connection_ui.ts`, `src/game/tutorial.ts`, `src/game/waiting_tips.ts`, `src/game/visual_helpers.ts`
- `src/shared/audio.ts`, `src/shared/toast.ts`, `src/shared/best_score_cookie.ts`, `src/shared/tutorial_cookie.ts`
- `src/verify.ts`（magic link 验证页入口 glue）
- `src/game/restart_vote_ui.ts`（重启投票 DOM 同步，纯 UI）
- `src/game/input.ts`（Canvas 点击 / 重启按钮 DOM 交互）
- `src/game/entry_flow.ts`（进房 DOM overlay；纯函数仍由 `entry_flow.test.ts` 覆盖）
- `src/game/ws_message_queue.ts`（WS 帧队列；行为由 `ws_handlers` 集成测试覆盖）
- `src/game/ws_connect.ts`（WebSocket 连接编排 / 重连 glue）
- `src/game/state_interp.ts`（渲染插值；由 `state_physics_interpol.test.ts` 覆盖）
- `src/game/ws_handlers_phase.ts`（阶段切换 + 结束屏 DOM）
- `src/game/ws_connection.ts`（心跳 / 重连 / 待发队列 glue）
- `src/shared/session.ts`（认证 cookie 流程；由 `session.test.ts` 覆盖）

## 约定

- 单元测试：`go test -short`，Redis 用 miniredis（ADR-023）
- 集成测试：`//go:build integration`，testcontainers
- 每 Go 包 1–3 个 `*_test.go`（见 [code-simplification-plan.md](code-simplification-plan.md)）
- **例外**：`backend/internal/game` 与 `backend/internal/handler` 在 megfile 拆分后按主题分文件（lifecycle、physics、hub 等），由 `scripts/codegen/split_go_tests.py` 维护
