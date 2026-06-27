# 全仓整理清理 Checklist

跟踪扩展版全仓清理计划。完成项已勾选。

## A. 结构卫生

- [x] 删除 `run-backend.ps1`
- [x] 删除 `scripts/archive/` 并更新 layout 门禁
- [x] 删除 `backend/cmd/backfill-emails/`
- [x] 删除 `backend/cmd/migrate-passwords/`（`lib/pq` 降为 indirect）
- [x] 更新 `Makefile` `test-containers`
- [x] `k6-smoke.js` 无 phantom `ADR-V2-*`
- [x] `scripts/README.md` 同步

## B. 配置与工具链

- [x] `REDIS_URL` 支持 URL 与 `host:port`（`config.ParseRedisURL` + `.env.example`）
- [x] 根 `package.json` 使用 `docker compose`
- [x] commitlint/eslint 仅根目录；frontend 仅构建 deps
- [x] `make load-smoke` / `load-ws-soak` / `load-single-room`
- [x] `tools.go` 锁定 air/golangci；CI golangci v1.64.8

## C. CI / Makefile

- [x] `make bench` 与 CI 范围一致（protocol + game）
- [x] CI 移除重复 detect-secrets job（保留 gitleaks + pre-commit baseline）
- [x] `test:backend` 调用 `make test`
- [x] CONTRIBUTING 说明 workflow 职责
- [x] integration job 移除已删 cmd 路径

## D. 文档治理

- [x] `docs/templates/adr.md` 中文模板
- [x] ADR 008–010 中文化；001 Cloud Run 措辞；014–016 格式；018–025 状态审计
- [x] 无 `ADR-V2-*` phantom
- [x] CHANGELOG / docs/README / CONTRIBUTING 同步
- [x] ops/development 英文文档中文化（chaos/environments/profiling/coverage/postmortem）

## E. 后端生产代码

- [x] `redis_helpers.go`
- [x] `degradation.go` 合并 deps
- [x] OpenAPI 含 leaderboard/stats
- [x] hub 谓词等（roadmap Phase 4 已标记完成）

## F. 后端测试

- [x] middleware/auth/handler 合并（WIP 已落地）
- [x] store：`env_helpers_test` 并入 `redis_test`
- [x] worker：gdpr 测试并入 `email_worker_test`
- [x] game：保留 `room_contention`/`room_outbound` 等 roadmap 例外（megafile 待续）

## G. 前端

- [x] WS/state 模块拆分（`client_state_reset`、`connection_ui` 等，roadmap Phase 5）
- [x] `websocket.ts` 作为稳定 import 门面（main/input 使用）
- [x] 新模块见 CONTRIBUTING / roadmap

## H. 横切

- [x] Cloud Run 注释收敛为 GKE/反向代理
- [x] README 命名映射表
- [x] `deploy/README.md` K8s vs local 对照
- [x] coverage-policy 移除 `degradation_deps` 引用

## 门禁

- [x] `go test ./... -short`（backend）
- [ ] `make check-repo-layout`（需 bash/WSL）
- [ ] `make ci`（完整 CI 需 Linux + Docker）
