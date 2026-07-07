# v1 回归验证报告（Task 4）

> 生成日期：2026-07-08
> 子代理 E 产出
> 审查性质：纯诊断，未修改任何业务代码

---

## 0. 验证范围与基线说明

**v1 基线报告 `docs/superpowers/reports/2026-07-07-full-self-inspection-report.md` 经 Task 2/3 子代理确认不存在**（reports/ 目录仅含 v2-asset-inventory.md + v2-task1-results.md + v2-task2-results.md + v2-task3-results.md + 本文件）。

因此本次回归验证基于以下间接证据：
1. `docs/security/self-check-checklist.md` 第 103-124 行"子 agent 结论"与"审查记录"（2026-06-27）
2. `docs/security/self-check-baseline.txt`（2026-06-27 post-Bugbot 基线）
3. `docs/superpowers/reports/v2-task1-results.md` 第 96-101 行 v1 回归检查汇总
4. `docs/superpowers/reports/v2-task2-results.md` 第 172-177 行 v1 回归检查汇总
5. 当前代码状态（HEAD 实测）

**影响**：v1 报告 27 项关键发现（7 CRITICAL + 20 REQUIRED）的具体描述无法逐字核对，本报告基于任务描述中给出的 v1 主题候选进行代码级验证。REQUIRED 项中可能存在与 v1 原始条目的语义偏差，已在备注列标注。

---

## 1. v1 回归验证表

### 1.1 CRITICAL（7 项）

| v1 发现 | 严重级别 | 状态 | 证据（文件:行号） | 备注 |
|---------|---------|------|------------------|------|
| C-01: GKE 未 wire TRUSTED_PROXY_CIDRS → admin lockout DoS | CRITICAL | FIXED | `infra/k8s/base/service.yaml:128-133`（env TRUSTED_PROXY_CIDRS ← ConfigMap `balloon-game-region` key `trusted-proxy-cidrs`）；`infra/k8s/base/region-config.yaml:15`（key 定义） | StatefulSet env 已注入；overlays 通过 `__TRUSTED_PROXY_CIDRS__` 占位符替换 |
| C-02: Dockerfile Go builder digest pin 回归 | CRITICAL | FIXED | `Dockerfile:11`（`golang:1.26-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648`） | 三阶段全部 digest pin（node:3、golang:11、distroless:20） |
| C-03: HEAD 未就绪：refresh token in localStorage | CRITICAL | FIXED | `frontend/src/shared/network/auth.ts:5`（注释"存储在 HttpOnly cookie（前端不可读）"）；`auth.ts:28`（`fetch('/api/v1/auth/refresh', {...})` 依赖 cookie） | 全量 grep `localStorage.*refresh` 仅命中 self-check-checklist.md，前端代码无 localStorage 存储 refresh token |
| C-04: 无效 CIDR 静默忽略 → 全员 lockout | CRITICAL | FIXED | `backend/internal/config/env.go:106-130`（`validateTrustedProxyCIDRs` 显式拒绝无效 CIDR）；`env.go:98-101`（生产强制校验）；`env_test.go:49,161,167`（拒绝/空/跳过空段测试） | 生产 `Validate()` 调用链完整 |
| C-05: admin lockout Redis 读失败 fail-open | CRITICAL | FIXED | `backend/internal/handler/admin_login.go:64-70`（`IsLoginLocked` err → 503 ServiceUnavailable + return true） | 由 fail-open 改为 fail-closed；self-check-baseline.txt:26 确认 |
| C-06: restore 缺 Redis 注册 | CRITICAL | FIXED | `backend/internal/game/hub_restore.go:52-53`（`finalizeMaterializedRoom(code)`）；`backend/internal/game/hub_redis_registry.go:28-31`（`finalizeMaterializedRoom` → `registerRoomInRedis`）；`hub_redis_registry.go:42-52`（`registerRoomInRedis` 实现） | restore 路径与 lazy-load 路径统一调用 `finalizeMaterializedRoom` |
| C-07: 全 Pod PG restore | CRITICAL | FIXED | `backend/internal/game/hub_restore.go:37`（`if !h.shouldLocalMaterializeRoom(ctx, ls.Code) { continue }`）；`hub_redis_registry.go:13-26`（`shouldLocalMaterializeRoom` 检查 `info.Instance == h.instanceID`） | 仅本实例拥有的房间被 materialize；self-check-baseline.txt:19 确认 |

### 1.2 REQUIRED（20 项候选）

> 由于 v1 报告缺失，以下 20 项为基于 `self-check-checklist.md` 子 agent 结论（R1/R2/R3/Security Review/Bugbot）推断的候选条目。

| v1 发现 | 严重级别 | 状态 | 证据（文件:行号） | 备注 |
|---------|---------|------|------------------|------|
| R-01: WS cap TOCTOU | REQUIRED | FIXED | `backend/internal/game/hub_ws_limiter.go:23-34`（`TryReserveWSConnection` CAS 循环）；`handler/lobby_ws.go:88`（upgrade 前调用） | self-check-checklist.md:107 R2 已修复 |
| R-02: room lock 阻塞（broadcast 持锁等慢客户端） | REQUIRED | FIXED | `backend/internal/game/outbound_manager.go:113-122`（`enqueueCritical` 队列满时丢弃避免持锁）；`:119`（"critical outbound queue blocked, dropping to avoid room lock hold"） | broadcast 已异步化到 outbound goroutine |
| R-03: cooldown 漂移（connected-only 计数） | REQUIRED | FIXED | `backend/internal/game/room_message.go:69`（`cooldown := CalculateCooldown(len(r.state.Players))`）；`cooldown_contract_test.go:44-61`（`TestUpdatePlayerStats_CooldownUsesRosterSize` 断言用 roster 而非 connected） | self-check-baseline.txt:20 确认 |
| R-04: outbound 死锁 | REQUIRED | FIXED | `backend/internal/game/outbound_manager.go:166-175`（`deliverCritical` 100ms 超时）；`:177-186`（`deliverNonCritical` default 非阻塞）；`:205-209`（`publishBroadcastAsync` 100ms ctx 超时） | self-check-baseline.txt:21 确认 |
| R-05: 后端 unit 覆盖率 73.4% < 100% 门禁 | REQUIRED | PARTIAL | `docs/security/self-check-baseline.txt:13`（"PARTIAL: backend unit gate 73.4% < 100% (CI go-ci.yml; backlog)"）；`docs/development/coverage-policy.md:9`（门禁实际为 lines ≥ 80%） | 覆盖率债务仍在 backlog；v2-task1-results.md 未将其列为回归，但未达标 |
| R-06: auth 测试编译 | REQUIRED | FIXED | `backend/internal/auth/` 含 8 个 `_test.go`（auth_token_test.go、auth_flow_test.go、magiclink_verify_test.go、magiclink_flow_test.go、jwt_cookies_test.go、quickplay_flow_test.go 等） | self-check-checklist.md:121 确认已修复 |
| R-07: entry_flow 倒计时 | REQUIRED | FIXED | `frontend/src/game/entry_flow.ts:285`（`startStartCountdown()`）；`:291-303`（setInterval tick + renderStartCountdownTitle）；`:195`（`renderStartCountdownTitle(remaining)`） | self-check-checklist.md:121 确认已修复 |
| R-08: index_leaderboard XSS | REQUIRED | FIXED | `frontend/src/index_leaderboard.ts:30`（`document.createElement('li')`）；`:35,39,43`（`rank.textContent` / `score.textContent` / `code.textContent`） | 全量渲染用 DOM API + textContent，无 innerHTML |
| R-09: CheckAuth 撤销检查 | REQUIRED | FIXED | `backend/internal/auth/middleware.go:78,152`（`rev.IsJWTRevoked(ctx, jti)`）；`handler/auth_test.go:255`（`TestCheckAuth_RevokedSession`） | self-check-checklist.md:109 R1 已验证 |
| R-10: GDPR 500 错误处理 | REQUIRED | FIXED | `backend/internal/handler/auth_gdpr.go:60`（deletion scheduled + sessions revoked）；`handler/auth_test.go:820,834,929`（500 状态码断言） | self-check-checklist.md:109 确认 |
| R-11: cosign verify 部署签验 | REQUIRED | FIXED | `.github/workflows/ci-cd.yml:179-185`（`cosign-installer@v3` + `cosign verify "$IMAGE"`） | self-check-checklist.md:108 R3 已修复 |
| R-12: E2E 启动 | REQUIRED | FIXED | `.github/workflows/ci-cd.yml:49-117`（e2e job matrix + `./.github/actions/e2e-setup` + playwright install + run） | self-check-checklist.md:108 R3 已修复 |
| R-13: restore/lazy-load 原子认领（TryClaimRoomRegistry） | REQUIRED | FIXED | `backend/internal/store/room_registry_store.go:25`（`TryClaimRoomRegistry`）；`store/redis_test.go:194`（`TestRoomRegistryStore_TryClaimRoomRegistry`） | self-check-baseline.txt:22 确认 |
| R-14: POST /api/v1/auth/verify 端点（避免 URL token 泄露） | REQUIRED | FIXED | `backend/internal/server/routes_public.go:69`（`r.With(...).Post("/verify", authHandler.VerifyMagicLinkPost)`） | self-check-checklist.md:62 确认 |
| R-15: metrics basic auth 生产强制 | REQUIRED | FIXED | `backend/internal/server/routes_middleware.go:36-58`（`metricsAuthMiddleware`：生产缺 METRICS_USER/PASSWORD → 403 禁用） | self-check-checklist.md:91 确认 |
| R-16: .env 不入库 | REQUIRED | FIXED | `.gitignore:11-13`（`.env` / `.env.*` 忽略；`!.env.example` 例外） | self-check-checklist.md:92 确认 |
| R-17: ALLOWED_ORIGINS 生产非 localhost 默认 | REQUIRED | FIXED | `backend/internal/config/env.go:28,56`（`AllowedOrigins` 读取）；`server/routes_middleware.go:32`（`AllowedOriginsFromEnv`） | self-check-checklist.md:83 确认 |
| R-18: EnableHSTS 生产配置 | REQUIRED | FIXED | `backend/internal/config/env.go:32,60`（`EnableHSTS` 默认 true，仅 "false" 显式关闭） | self-check-checklist.md:10 确认（HSTS=true 时强制 TRUSTED_PROXY_CIDRS） |
| R-19: cooldown 契约测试（跨语言一致） | REQUIRED | FIXED | `backend/internal/game/cooldown_contract_test.go:12-61`（`cooldownContractCases` + `TestCalculateCooldownContract` + `TestUpdatePlayerStats_CooldownUsesRosterSize`） | self-check-checklist.md:107 R2 已修复 |
| R-20: 无法精确对应（v1 报告缺失） | REQUIRED | 无法验证 | — | v1 报告缺失导致 1 项 REQUIRED 无法精确对应到代码位置；Task 1/2 汇总均声明"22 个 Critical 资产均无代码回归"，但未给出逐项映射 |

---

## 2. 统计

| 状态 | 数量 | 占比 |
|------|------|------|
| FIXED | 25 项 | 92.6% |
| PARTIAL | 1 项 | 3.7% |
| REGRESSION | 0 项 | 0% |
| 无法验证（v1 报告缺失） | 1 项 | 3.7% |
| **合计** | **27 项** | **100%** |

按级别拆分：
- **CRITICAL（7 项）**：FIXED 7 / PARTIAL 0 / REGRESSION 0 / 无法验证 0
- **REQUIRED（20 项）**：FIXED 18 / PARTIAL 1 / REGRESSION 0 / 无法验证 1

---

## 3. 关键发现

### 3.1 无代码回归
所有 7 项 CRITICAL 发现均已在代码中实施修复且无回归证据。这与 v2-task1-results.md:98"22 个 Critical 资产均无代码回归"及 v2-task2-results.md:175"v2-C-04 实际为真：v1 报告确实缺失"的结论一致。

### 3.2 唯一 PARTIAL：后端 unit 覆盖率
- **R-05** 后端 unit 覆盖率 73.4% 仍低于 `coverage-policy.md:9` 声明的 80% 门禁
- `self-check-baseline.txt:13` 明确标记为 PARTIAL（backlog）
- v2 审查未将其升级为 CRITICAL，归类为技术债务
- 注：v1 原描述为"< 100% 门禁"，但实际 `coverage-policy.md` 门禁为 80%，73.4% 仍不达标

### 3.3 无法验证项
- **R-20**：v1 报告缺失导致 1 项 REQUIRED 无法精确对应。基于 self-check-checklist.md 子 agent 结论（R1/R2/R3/Security Review/Bugbot 共约 12 项 Important/High 修复）已尽可能覆盖，但 v1 原始 20 项 REQUIRED 的精确边界无法还原。

### 3.4 与 v2 新发现的关系
v1 修复主题（TRUSTED_PROXY、admin lockout、restore、cooldown、outbound、auth/refresh）在 v2 审查中均未被列为回归。v2 新发现（如 v2-R-02 OutboxRepository 缺 retry、v2-R-04 登录锁绕过 circuit breaker、v2-R-05 CheckRateLimit fail-closed 矛盾）属于 v1 修复后的**残留短板**或**新轴（弹性/可观测性）下的新发现**，非 v1 回归。

---

## 4. 验证方法学局限

1. **v1 报告缺失**：无法逐项核对 v1 27 项发现的原始描述、严重级别、证据行号
2. **间接证据依赖**：依赖 self-check-checklist.md 子 agent 结论（2026-06-27）+ self-check-baseline.txt，二者均为修复后的状态记录，非 v1 原始发现清单
3. **REQUIRED 项候选构造**：基于任务描述中给出的 v1 主题 + self-check-checklist.md 子 agent 结论推断，可能与 v1 原始条目存在语义偏差
4. **未运行测试**：本次为纯静态代码审查，未执行 `go test`/`npm test`/CI 验证修复的实际行为

**建议**：后续回归验证应先重建 v1 基线报告（如从 git 历史恢复或重新生成），以支持精确逐项核对。
