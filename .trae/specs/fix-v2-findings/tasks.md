# Tasks — 修复 v2 自检发现

> change-id: fix-v2-findings
> 依赖：`docs/superpowers/reports/2026-07-08-self-inspection-v2-report.md`（数据源）
> 范围：26 CRITICAL + 126 REQUIRED = 152 项
> 原则：主次分明（P0 详细 / P1 中等 / P2 批量）+ 详略得当 + 域内并行

---

## Task 1: P0 立即修复 — CI 阻断 + 安全文档（3 项核心，~15 子项）

> 阻断所有 PR 合并 / 误导安全评审，必须最先修复
> 涉及发现：v2-C-41, v2-C-11, v2-C-12, v2-C-13, v2-C-14, v2-C-40, v2-R-53, v2-R-55, v2-R-67~74, v2-R-143

- [ ] Task 1.1: 修复 CI E2E 矩阵（v2-C-41, v2-R-143）
  - 移除 `.github/workflows/ci-cd.yml:55` 矩阵中的 `performance` 项（除非创建 `tests/e2e/performance.spec.ts`）
  - 补全矩阵：确保 auth/admin/security/network_boundary/concurrency 5 个 critical spec 均在矩阵中
  - 验证：CI E2E job 全绿

- [ ] Task 1.2: 修正 openapi.yaml（v2-C-11, v2-C-12, v2-C-13, v2-R-53, v2-R-55, v2-R-67~71）
  - 房间码示例 6 位 → 5 位（与 `domain/room_code.go:11` 一致）
  - `/api/v1/registry/match` 移除 `deprecated: true`，描述改为"已实现"
  - match 响应字段名 `code` → `lobbyCode`（与 `lobby_registry.go:204` 一致）
  - 补全 4 个已注册路由文档（参考后端路由注册代码）
  - 移除 3 处对不存在的 `/resolve` 端点的引用
  - 验证：openapi lint 通过 + 与后端路由注册逐项核对

- [ ] Task 1.3: 修正 threat-model.md（v2-C-14, v2-C-40, v2-R-72~74）
  - JWT 算法 `HMAC-SHA256` → `ES256`（与 `crypto/jwt.go` 实际一致）
  - Admin JWT 密钥来源：移除"独立 ADMIN_JWT_SECRET"，改为"共享 ECDSA 私钥"
  - 核对并修正其他认证链描述（match 限流"配置预留"等）
  - 验证：与 `crypto/jwt.go` + `admin_login.go` 逐项核对

---

## Task 2: P1 基础设施修复（4 路并行）

> 生产风险：HPA 链路断裂、Terraform 未 IaC 化、告警失效、环境配置误导
> 涉及发现：v2-C-01, v2-C-03, v2-C-15, v2-C-30~34, v2-C-36, v2-C-37, v2-C-35, v2-R-21~31, v2-R-75~81, v2-R-119~132, v2-R-148

- [ ] Task 2.1: K8s 修复（v2-C-03, v2-C-15, v2-R-77, v2-R-78, v2-R-79, v2-R-28~31）
  - 新增 `infra/k8s/base/prometheus-adapter.yaml`（APIService + Service + adapter 规则映射 ws_connections）
  - balloon-game 镜像：CI 改用 `kustomize edit set image ...@sha256:<digest>`，overlay 注入 digest
  - Redis/redis-ephemeral StatefulSet 补 `securityContext`（runAsNonRoot/runAsUser:999/readOnlyRootFilesystem/cap drop ALL）
  - namespace `balloon-game` 补 PSS restricted admission 标签（enforce/audit/warn）
  - 修正 runbook 中 HPA/Redis 相关不一致描述
  - 验证：`kustomize build` 通过 + HPA 可查询 ws_connections

- [ ] Task 2.2: Terraform IaC 化（v2-C-01, v2-C-02, v2-C-05, v2-R-21~27, v2-R-148）
  - 新增 GKE 集群 + VPC Terraform 定义（或明确标注"手动管理"并在 ADR 记录）
  - 修正 ADR-013 引用：`ADR-028` → `ADR-014`（多区域拓扑）
  - 修正 ADR-015 状态：`提议中` → `已接受`（CRDB 切换已实现）
  - 添加 `required_version` 约束 + CI `terraform validate` 门禁
  - 补 `trusted_proxy_cidrs` variable（与 K8s 层一致）
  - 验证：`terraform validate` 通过

- [ ] Task 2.3: CI 门禁 + 告警修复（v2-C-30, v2-C-31, v2-C-32, v2-C-33, v2-C-34, v2-C-36, v2-C-37, v2-R-15~20, v2-R-75, v2-R-76, v2-R-119~126）
  - Makefile 补 `sync-alert-rules` target（生成 alertmanager ConfigMap）
  - 创建缺失的 `rules-configmap.yaml` 或修正引用
  - 修正告警指标名：`pgxpool_acquire_count` → `db_pool_*` / `ws_active_connections` / `game_active_ws_connections` → `ws_connections`
  - 修正 Grafana datasource UID 不匹配
  - GitHub Actions 全仓 digest pin（@sha256）
  - 工具安装固定版本（govulncheck/golang-migrate/go-licenses 去 @latest）
  - security-scan.yml 补 `permissions: { contents: read }` + 失败通知
  - 验证：CI 全绿 + `make sync-alert-rules` 可执行

- [ ] Task 2.4: 环境配置修复（v2-C-35, v2-R-129~132）
  - `.env.example`: `JWT_SECRET` → `JWT_PRIVATE_KEY`/`JWT_PUBLIC_KEY`（PEM 格式示例）
  - 核对并修正其他脱节项（与 `config/env.go` 实际读取的变量名逐项核对）
  - 验证：按 .env.example 配置可成功启动服务

---

## Task 3: P1 前端代码修复（4 路并行）

> 涉及发现：v2-C-08, v2-C-25, v2-R-14, v2-R-39, v2-R-40, v2-R-44~50, v2-R-107~111

- [ ] Task 3.1: 生成器路径 + 常量一致性（v2-C-08, v2-R-39, v2-R-40）
  - 修复 `cmd/gen-frontend-constants` 输出路径：`shared/constants.ts` → `shared/game/constants.ts`
  - 消除 `constants/protocol.go` 与 `protocol/constants.go` 的常量重复定义（alias 合并）
  - PALETTE_COLORS / END_REASON 纳入生成器同步
  - 新增 CI 校验：前后端常量一致性（扩展 docs-governance ws-protocol-sync 覆盖 PHASE_CODE/END_REASON）
  - 验证：`go generate ./cmd/gen-frontend-constants` 后前端常量无 diff + CI 校验通过

- [ ] Task 3.2: HTML CSP（v2-C-25）
  - 5 个 HTML 文件补 `<meta http-equiv="Content-Security-Policy" ...>` 标签
  - CSP 策略与后端 CSP nonce 中间件一致
  - 验证：HTML lint + 浏览器控制台无 CSP 违规

- [ ] Task 3.3: fetch 超时 + ws_connection 状态收敛（v2-R-14, v2-R-46, v2-R-47, v2-R-50）
  - 关键路径 fetch 调用加 `AbortController` + 8s 超时（lobby_match/room_validate/session/index/admin_config/leaderboard）
  - `ws_connection.ts` 14+ 模块级 setter 收敛为单一 `connectionState` 对象 + reducer（或 Connection 类）
  - 验证：前端单测 + E2E 通过

- [ ] Task 3.4: 前端清理 + 死代码 + 文档对齐（v2-R-44, v2-R-45, v2-R-48, v2-R-49, v2-R-107~111）
  - 移除 `ws_handlers_phase.ts:13` 残留 `console.log`
  - 移除 `protocol.ts:31-47` 死代码（phaseFromCode/phaseToCode）
  - 修正 ADR-025 与代码不一致（store.select 不存在 / Object.assign 直接变异 / localStorage 键名 / window 挂载）
  - `_savedNickname` 缓存改为从 store + localStorage 直接读取
  - 修复前端外围死代码/魔法偏移（A-049/A-053/A-055 等）
  - 验证：前端单测 + `tsc --noEmit` 通过

---

## Task 4: P1 后端代码修复（4 路并行）

> 涉及发现：v2-R-01~06, v2-R-36~43, v2-R-82~84, v2-R-95~102

- [ ] Task 4.1: store/audit/弹性修复（v2-R-03, v2-R-04, v2-R-05, v2-R-36, v2-R-37）
  - store 包添加 slog 上下文日志（持久化错误含 trace_id）
  - audit `loadLastHash` 加超时 + `writeToDB` 失败重试（非仅日志丢弃）
  - 登录锁失败改 fail-closed（circuit breaker）
  - `CheckRateLimit` 数据库不可达改 fail-closed
  - 验证：单元测试覆盖 fail-closed 路径

- [ ] Task 4.2: 测试补全（v2-R-01, v2-R-06, v2-R-82）
  - `idempotency.go` SETNX claim 路径补单元测试（2 个 TODO）
  - 11 个 `down.sql` 回滚迁移补测试覆盖
  - `TestPostgresStore_AnonymizeUser` 断言 `&&` → `||`（GDPR 合规）
  - 验证：`go test ./...` 通过 + 覆盖率提升

- [ ] Task 4.3: config/constants/worker 修复（v2-R-38, v2-R-39, v2-R-40, v2-R-41, v2-R-42, v2-R-43, v2-R-83, v2-R-84）
  - `getDurationEnv` 与 `GetEnvDuration` 行为统一
  - outbox at-least-once 语义文档化（ws-protocol.md / ADR）
  - email worker 消费者 ID 去硬编码 + 加退避
  - worker 暴露处理指标（成功/失败/延迟/队列深度）
  - nicknames 中文名长度判断字节 → rune
  - slogctx 统一日志上下文
  - 验证：`go test ./...` 通过

- [ ] Task 4.4: cmd/* + 后端外围修复（v2-R-95~102）
  - `cmd/gen-frontend-constants` 补测试
  - `cmd/migrate-passwords` sslmode 绕过修复
  - cmd/* 其他外围 REQUIRED 项
  - 验证：`go test ./cmd/...` 通过

---

## Task 5: P1 文档修复（4 路并行）

> 涉及发现：v2-C-09, v2-C-10, v2-C-38, v2-C-39, v2-R-58~66, v2-R-107~110, v2-R-144~148

- [ ] Task 5.1: ADR 修正（v2-C-02, v2-C-05, v2-C-09, v2-C-10, v2-R-58~64）
  - ADR-013: 引用 `ADR-028` → `ADR-014`；状态明确（部分废弃）
  - ADR-014: README 状态 `提议中` → `已接受`（与文件一致）
  - ADR-015: 状态 `提议中` → `已接受`（CRDB 已实现）
  - ADR-018: 标注"状态管理已被 ADR-025 取代" + 修正 "Zustang" → "Zustand" + 更新测试文件数描述
  - ADR-022: 移除"RotateKey 未实现"（已实现）+ 修正行号引用 `aes.go:162-165` → `aes_email.go:38-43`
  - ADR-025: README 标题 "可变单例" → "受控状态管理" + 文件名重命名
  - 验证：ADR 交叉引用无矛盾 + 与代码核对

- [ ] Task 5.2: 架构文档修正（v2-R-65, v2-R-66）
  - `architecture.md:34` "可变单例" → "受控状态管理（ADR-025）"
  - `architecture.md:121,123,127` 房间码示例 "ABC123" → 5 位（如 "ABC23"）
  - 核对 `cmd/game-worker` 是否存在，未实现则标注"（计划中）"或移除
  - 更新"最后更新"日期 + 反映 ADR-028/029
  - 验证：与代码逐项核对

- [ ] Task 5.3: 运维/数据文档修正（v2-C-38, v2-C-39, v2-R-107~110, v2-R-144~148）
  - CockroachDB 文档：标注"未实现"或补代码实现（与文档对齐）
  - `db-query-analysis.md` 移除对已删除索引的引用
  - runbook 不一致项修正
  - room_result_async 三写并行文档化（数据流设计意图）
  - 跨层新发现（v2-R-144~148）相关文档修正
  - 验证：文档与代码逐项核对

- [ ] Task 5.4: ws-protocol.md 补全（v2-R-41, v2-R-143）
  - 补充 SNAPSHOT/RESTART_STATUS 二进制布局
  - 补充 decodeSnapshot 对超长 nickLen 的 known limitation 说明
  - 验证：与 `protocol/` 代码逐字段核对

---

## Task 6: P2 技术债务批量修复（2 路并行）

> 剩余 REQUIRED 项 + 后端覆盖率
> 涉及发现：v2-R-07~13, v2-R-32~35, v2-R-51~57, v2-R-80~81, v2-R-85~94, v2-R-103~106, v2-R-111~118, v2-R-127~128, v2-R-133~142

- [ ] Task 6.1: 后端覆盖率 + 残留 REQUIRED 批量（v2-R-05 覆盖率 + 残留后端项）
  - 后端 unit 覆盖率提升至 80%（当前 73.4%）
  - 补 metrics/tracing 端点烟雾测试
  - 补 outbox 消费/重试路径测试
  - 补 DecodeNicknamePayload fuzz 测试
  - 修正 TestRateLimiter_ConcurrentRequests 设计
  - 修复前端 property test 静默吞错（decodeSnapshot 实现）
  - 验证：`go test -cover` ≥ 80% + 前端单测通过

- [ ] Task 6.2: docker-compose + 前端外围 + 文档残留批量（v2-R-80, v2-R-81, v2-R-51~57, v2-R-85~94, v2-R-103~118, v2-R-127~128, v2-R-133~142）
  - docker-compose 7 镜像补 digest pin
  - docker-compose `name: uppy-clone` → `balloon-game`
  - testcontainers 镜像补 digest pin
  - 前端外围残留（visual_helpers/restart_vote_ui 等）
  - 文档残留项批量处理
  - 验证：`docker-compose config` 通过 + 前端单测通过

---

## Task 7: 针对性回归验证 + Baseline 更新

> 依赖：Task 1-6 全部完成
> 验证方式：针对性回归（非全量 v2 自检）

- [ ] Task 7.1: 汇总所有修复，生成修复清单映射（v2 发现 ID → 修复 commit/文件）
- [ ] Task 7.2: 运行全量 CI（go-ci + ci-cd + security-scan + docs-governance），确认全绿
- [ ] Task 7.3: 运行 `go test ./... -cover` 确认后端覆盖率 ≥ 80%
- [ ] Task 7.4: 运行 E2E 全量（含补全的 5 个 critical spec），确认全绿
- [ ] Task 7.5: 更新 `docs/security/self-check-baseline.txt` 为新基线（标注 v2 发现修复完成）
- [ ] Task 7.6: 生成修复验证报告 `docs/superpowers/reports/2026-07-XX-v2-fix-verification-report.md`
  - 内容：152 项发现 → FIXED/PARTIAL/DEFERRED 状态表
  - 每项含：修复 commit SHA + 验证证据（测试/CI）
  - 残留风险说明（如有 PARTIAL/DEFERRED）

---

# Task Dependencies

- Task 1（P0）无依赖，最先执行
- Task 2/3/4/5（P1）依赖 Task 1 完成（避免 CI 仍红时提交），4 路域间并行
- Task 6（P2）依赖 Task 2-5 完成（避免冲突），2 路并行
- Task 7（验证）依赖 Task 1-6 全部完成

# 并行执行建议

| 波次 | 任务 | 子代理数 | 说明 |
|------|------|---------|------|
| 1 | Task 1.1/1.2/1.3 | 3 | P0 并行（CI/openapi/threat-model 互不冲突） |
| 2 | Task 2.1/2.2/2.3/2.4 + Task 3.1/3.2/3.3/3.4 + Task 4.1/4.2/4.3/4.4 + Task 5.1/5.2/5.3/5.4 | 16 | P1 全并行（域间无冲突） |
| 3 | Task 6.1/6.2 | 2 | P2 并行 |
| 4 | Task 7.1-7.6 | 1 | 主代理汇总验证 |

# 产出约束

- 每个修复 commit 引用对应 v2 发现 ID（如 `fix(v2-C-41): remove non-existent performance.spec.ts from E2E matrix`）
- 不修改 v2 自检报告原始文件（历史记录保留）
- 修复验证报告独立成文，不覆盖 v2 综合报告
