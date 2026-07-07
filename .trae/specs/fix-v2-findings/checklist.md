# Checklist — 修复 v2 自检发现

> change-id: fix-v2-findings
> 用途：Task 7 验证阶段逐项核对
> 验证方式：针对性回归验证（CI 全绿 + 测试覆盖 + baseline 更新）

---

## Task 1: P0 立即修复

- [ ] ci-cd.yml E2E 矩阵无 `performance` 引用（或 performance.spec.ts 已创建）
- [ ] ci-cd.yml E2E 矩阵包含 auth/admin/security/network_boundary/concurrency 5 个 spec
- [ ] openapi.yaml 房间码示例均为 5 位
- [ ] openapi.yaml `/api/v1/registry/match` 无 `deprecated: true`
- [ ] openapi.yaml match 响应字段名为 `lobbyCode`
- [ ] openapi.yaml 补全 4 个已注册路由文档
- [ ] openapi.yaml 移除对 `/resolve` 端点的引用
- [ ] threat-model.md JWT 算法为 ES256
- [ ] threat-model.md Admin JWT 声明共享 ECDSA 私钥（非独立 ADMIN_JWT_SECRET）
- [ ] CI E2E job 全绿

## Task 2: P1 基础设施修复

- [ ] `infra/k8s/base/prometheus-adapter.yaml` 已创建（APIService + Service + ws_connections 映射）
- [ ] HPA 可查询 ws_connections 指标（`kubectl describe hpa` 无 FailedCustomMetrics）
- [ ] balloon-game 镜像使用 digest pin（非 sed 占位符）
- [ ] Redis/redis-ephemeral StatefulSet 含 securityContext
- [ ] namespace `balloon-game` 含 PSS restricted admission 标签
- [ ] Terraform 含 GKE 集群 + VPC 定义（或 ADR 明确标注手动管理）
- [ ] Terraform 含 `required_version` 约束
- [ ] CI 含 `terraform validate` 门禁
- [ ] Terraform `trusted_proxy_cidrs` variable 与 K8s 层一致
- [ ] ADR-013 引用修正为 ADR-014
- [ ] ADR-015 状态修正为"已接受"
- [ ] Makefile 含 `sync-alert-rules` target 且可执行
- [ ] `rules-configmap.yaml` 已创建或引用修正
- [ ] 告警指标名修正（pgxpool_acquire_count / ws_active_connections / game_active_ws_connections → 实际指标名）
- [ ] Grafana datasource UID 一致
- [ ] GitHub Actions 全仓 digest pin（@sha256）
- [ ] 工具安装无 @latest（govulncheck/golang-migrate/go-licenses）
- [ ] security-scan.yml 含 `permissions: { contents: read }` + 失败通知
- [ ] `.env.example` 使用 JWT_PRIVATE_KEY/JWT_PUBLIC_KEY（PEM 格式）
- [ ] `.env.example` 与 config/env.go 变量名逐项一致
- [ ] CI 全绿（go-ci + ci-cd + security-scan + docs-governance）

## Task 3: P1 前端代码修复

- [ ] `cmd/gen-frontend-constants` 输出路径为 `shared/game/constants.ts`
- [ ] `go generate` 后前端常量无 diff
- [ ] constants/protocol.go 与 protocol/constants.go 常量重复已消除
- [ ] PALETTE_COLORS / END_REASON 纳入生成器同步
- [ ] CI 含前后端常量一致性校验（覆盖 PHASE_CODE/END_REASON）
- [ ] 5 个 HTML 文件均含 CSP meta 标签
- [ ] CSP 策略与后端 nonce 中间件一致
- [ ] 关键路径 fetch 调用含 AbortController + 8s 超时
- [ ] ws_connection.ts 状态收敛为单一对象/类（无 14+ setter）
- [ ] ws_handlers_phase.ts:13 console.log 已移除
- [ ] protocol.ts:31-47 死代码已移除
- [ ] ADR-025 与代码一致（store.select/Object.assign/localStorage/window）
- [ ] _savedNickname 缓存改为直接读取
- [ ] 前端单测通过 + `tsc --noEmit` 通过

## Task 4: P1 后端代码修复

- [ ] store 包含 slog 上下文日志
- [ ] audit loadLastHash 含超时
- [ ] audit writeToDB 失败重试（非仅日志）
- [ ] 登录锁 fail-closed
- [ ] CheckRateLimit 数据库不可达 fail-closed
- [ ] idempotency.go SETNX claim 路径有单元测试
- [ ] 11 个 down.sql 回滚迁移有测试覆盖
- [ ] TestPostgresStore_AnonymizeUser 断言为 `||`（非 `&&`）
- [ ] getDurationEnv 与 GetEnvDuration 行为统一
- [ ] outbox at-least-once 语义已文档化
- [ ] email worker 消费者 ID 去硬编码 + 含退避
- [ ] worker 暴露处理指标
- [ ] nicknames 中文名长度判断用 rune（非字节）
- [ ] cmd/gen-frontend-constants 有测试
- [ ] cmd/migrate-passwords sslmode 绕过已修复
- [ ] `go test ./...` 通过

## Task 5: P1 文档修复

- [ ] ADR-013 引用修正 + 状态明确
- [ ] ADR-014 README 状态与文件一致
- [ ] ADR-015 状态修正为"已接受"
- [ ] ADR-018 标注"已被 ADR-025 取代" + Zustand 拼写修正 + 测试文件数更新
- [ ] ADR-022 移除"RotateKey 未实现" + 行号引用修正
- [ ] ADR-025 README 标题修正 + 文件重命名
- [ ] architecture.md "可变单例" → "受控状态管理"
- [ ] architecture.md 房间码示例为 5 位
- [ ] architecture.md cmd/game-worker 状态明确
- [ ] CockroachDB 文档与代码对齐（标注未实现或补实现）
- [ ] db-query-analysis.md 移除已删除索引引用
- [ ] runbook 不一致项已修正
- [ ] room_result_async 三写并行已文档化
- [ ] ws-protocol.md 含 SNAPSHOT/RESTART_STATUS 布局
- [ ] ws-protocol.md 含 decodeSnapshot known limitation 说明

## Task 6: P2 技术债务批量修复

- [ ] 后端 unit 覆盖率 ≥ 80%
- [ ] metrics/tracing 端点有烟雾测试
- [ ] outbox 消费/重试路径有测试
- [ ] DecodeNicknamePayload 有 fuzz 测试
- [ ] TestRateLimiter_ConcurrentRequests 设计修正
- [ ] 前端 decodeSnapshot 实现修复（非测试 catch 吞错）
- [ ] docker-compose 7 镜像 digest pin
- [ ] docker-compose name 为 balloon-game
- [ ] testcontainers 镜像 digest pin
- [ ] 前端外围残留项已处理
- [ ] 文档残留项已处理

## Task 7: 针对性回归验证 + Baseline 更新

- [ ] 修复清单映射表已生成（v2 发现 ID → 修复 commit/文件）
- [ ] 全量 CI 全绿（go-ci + ci-cd + security-scan + docs-governance）
- [ ] `go test ./... -cover` ≥ 80%
- [ ] E2E 全量通过（含补全的 5 个 critical spec）
- [ ] `docs/security/self-check-baseline.txt` 已更新为新基线
- [ ] 修复验证报告已生成 `docs/superpowers/reports/2026-07-XX-v2-fix-verification-report.md`
- [ ] 报告含 152 项发现 → FIXED/PARTIAL/DEFERRED 状态表
- [ ] 每项含修复 commit SHA + 验证证据
- [ ] 残留风险已说明（如有 PARTIAL/DEFERRED）

---

## 全局约束验证

- [ ] 未修改 v2 自检报告原始文件（历史记录保留）
- [ ] 每个修复 commit 引用对应 v2 发现 ID
- [ ] 修复验证报告独立成文（不覆盖 v2 综合报告）
- [ ] v2-R-02 仅文档更新（代码已正确，无需修改）
- [ ] v2-C-04 建立新 baseline（不补回 v1 报告）
