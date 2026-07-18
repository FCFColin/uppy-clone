# ADR-031: 代码生成策略

## 状态: 已落地

## 上下文

前后端共享常量（物理参数、协议码、调色板颜色等）需要保持同步。此前仅 `protocol/constants.go` 中的 `@ts`-annotated 常量通过 `gen-frontend-constants` 生成到 `frontend/src/shared/game/constants.ts`，但：`domain/constants.go` 中前端需要的常量（`ReconnectGraceMs`、`MaxNicknameLen`、`MessageRateLimit`）未纳入生成；`frontend/src/game/local_constants.ts` 手写 6 个常量（`MAX_RECONNECT_ATTEMPTS`、`BASE_RECONNECT_DELAY`、`HEARTBEAT_INTERVAL_MS`、`HEARTBEAT_TIMEOUT_MS`、`MAX_SEEN_SEQS`、`MAX_PENDING_QUEUE`）与后端无同步机制；`ws_message_queue.ts` 中 `MAX_PENDING=32` 与 `local_constants.ts` 中 `MAX_PENDING_QUEUE=50` 命名混乱。

## 决策

1. **Go 代码是共享常量的唯一来源**：所有前后端共享常量必须在 Go 代码中定义，通过 `@ts <path>` 注解标注，由 `gen-frontend-constants` 自动生成
2. **禁止前端手写共享常量**：`local_constants.ts` 中与后端共享的常量必须改为从生成的 `constants.ts` 导入；仅保留纯前端本地常量（如 UI 标签文本）
3. **生成范围扩展**：纳入 `domain/constants.go` 的 `ReconnectGraceMs`/`MaxNicknameLen`/`MessageRateLimit`/`ScoreToWin`；新增连接常量到 `protocol/constants.go` 或 `domain/constants.go`：`HeartbeatIntervalMs`、`HeartbeatTimeoutMs`、`MaxReconnectAttempts`、`BaseReconnectDelay`、`MaxSeenSeqs`、`InboundQueueMax`、`OutboundQueueMax`
4. **命名规范**：前端大写+下划线（`HEARTBEAT_INTERVAL_MS`），后端驼峰（`HeartbeatIntervalMs`）；`@ts` 注解路径决定前端分组（`@ts CONNECTION.HEARTBEAT_INTERVAL_MS` → `CONNECTION.HEARTBEAT_INTERVAL_MS`）
5. **生成流程**：`make codegen`（统一入口，等价 `cd backend && go generate ./...`）；CI 校验：`make codegen` 后 `git diff --exit-code` 校验所有生成产物与提交一致

## 后果

**正面**：前后端常量零漂移；新增共享常量只需 Go 代码 + `@ts` 注解；消除 `local_constants.ts` 手写魔法数字

**负面**：生成器需扩展支持 `domain/constants.go`；前端构建依赖 Go 工具链（已有 monorepo 结构，可接受）

## 实现状态

- **生成器**：`backend/cmd/gen-frontend-constants`（源 `protocol/constants.go` + `domain/constants.go` 的 `@ts` 注解 → 产物 `frontend/src/shared/game/constants.ts`）；`scripts/codegen/generate_nicknames.go`（源 `shared/data/nicknames.json` → `pools_gen.go` + `nickname_pools_gen.ts`）。产物均带 `DO NOT EDIT` 头部
- **生成范围**：物理参数、协议码、调色板、相位码、结束原因、`ReconnectGraceMs`/`MaxNicknameLen`/`MessageRateLimit`/`ScoreToWin`、连接常量（`CONNECTION` 分组）。`local_constants.ts` 原 6 个手写共享常量已迁入生成的 `constants.ts`
- **CI 门禁**：`ci-cd.yml` → `codegen-sync` job 对所有 push/PR 跑 `make codegen` + `git diff --exit-code`，漂移则阻断合并；`docs-governance.yml` → `ws-protocol-sync` 补充语义校验
