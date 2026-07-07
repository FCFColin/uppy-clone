# 修复 v2 自检发现 Spec

> change-id: fix-v2-findings
> 来源：`docs/superpowers/reports/2026-07-08-self-inspection-v2-report.md`
> 范围：全部 26 CRITICAL + 126 REQUIRED = 152 项（OPTIONAL/FYI 不纳入，列附录供按需处理）
> 验证方式：针对性回归验证（每修复编写/扩展测试 + CI 全绿 + 更新 baseline）

---

## Why

v2 自检发现 26 项 CRITICAL + 126 项 REQUIRED，其中 3 项 P0 阻断 CI/误导安全评审，9 项 P1 构成生产风险（HPA 链路断裂、Terraform 未 IaC 化、HTML 缺 CSP、告警引用不存在指标等）。需系统性修复以恢复 CI 门禁可用性、消除生产风险、消除文档-代码不一致导致的维护误导。

## What Changes

### P0 立即修复（3 项核心，~15 子项）
- 修复 CI E2E 矩阵：移除不存在的 performance.spec.ts + 补跑 5 个 critical spec
- 修正 openapi.yaml：房间码 5 位 / match 移除 deprecated / 字段名 lobbyCode / 补全 4 路由
- 修正 threat-model.md：JWT 算法 HMAC→ES256 + Admin 密钥来源修正

### P1 本周修复（9 项生产风险）
- 补 prometheus-adapter 配置恢复 HPA ws_connections 指标链路
- Terraform IaC 化 GKE 集群 + VPC（或明确标注手动管理）
- 修复 gen-frontend-constants 输出路径
- 5 个 HTML 补 CSP meta 标签
- 修正告警指标名（pgxpool_acquire_count / ws_active_connections）
- 修正 .env.example（JWT_SECRET → JWT_PRIVATE_KEY/JWT_PUBLIC_KEY）
- 补全 CI E2E 矩阵（5 个 critical spec）

### P2 本月修复（技术债务，~120 项 REQUIRED）
- Makefile 补 sync-alert-rules target + rules-configmap 修复
- CockroachDB 文档与代码对齐
- GDPR 断言 && → ||
- 前端 fetch 加 AbortController 超时
- 镜像 digest pin（balloon-game + compose + Actions）
- Redis StatefulSet 补 securityContext + PSS restricted 标签
- ADR 过时/矛盾修正（ADR-013/014/015/018/022/025）
- 后端 store/audit/worker/config/constants 修复
- 前端 ws_connection 状态收敛 + 死代码清理 + console.log 残留
- 文档一致性全量修正（architecture/runbook/db-query-analysis 等）
- 测试补全（idempotency/down.sql/cmd/metrics 端点）
- 后端 unit 覆盖率提升至 80%

### 不修复项（已修正描述，无需操作）
- v2-R-02：经 Task 5 验证实际已包装 circuit breaker，仅需更新文档说明
- v2-C-04：v1 报告缺失属历史事实，本次修复后建立新 baseline 即可

## Impact

- **Affected specs**: evolve-self-inspection-v2（自检计划本身不修改）
- **Affected code**:
  - CI: `.github/workflows/ci-cd.yml`, `go-ci.yml`, `security-scan.yml`, `docs-governance.yml`
  - 基础设施: `infra/k8s/base/`（新增 prometheus-adapter.yaml）, `infra/k8s/overlays/`, `infra/terraform/`, `Makefile`
  - 后端: `backend/internal/store/`, `audit/`, `worker/`, `config/`, `constants/`, `cmd/`, `migrations/`
  - 前端: `frontend/src/game/`（ws_connection/state/dead code）, `frontend/src/shared/game/`, `cmd/gen-frontend-constants/`, 5 个 HTML
  - 文档: `docs/api/openapi.yaml`, `docs/security/threat-model.md`, `docs/adr/013-014-015-018-022-025`, `docs/architecture/`, `docs/runbook/`, `docs/data/`, `.env.example`
  - 测试: `backend/tests/integration/`, `backend/internal/middleware/idempotency_test.go`, `backend/tests/migrations/`, E2E spec
- **Affected发现**: 26 CRITICAL + 126 REQUIRED（v2-C-01~C-41, v2-R-01~R-148）

---

## ADDED Requirements

### Requirement: CI 门禁可用性

The system SHALL provide a passing CI pipeline on all PRs, with no references to non-existent test files or Makefile targets.

#### Scenario: E2E 矩阵全部可执行
- **WHEN** CI 运行 E2E job
- **THEN** 矩阵中所有 spec 文件均存在，无 performance.spec.ts 引用（除非创建该文件）
- **AND** 5 个 critical spec（auth/admin/security/network_boundary/concurrency）均在矩阵中

#### Scenario: Makefile target 存在
- **WHEN** 运行 `make sync-alert-rules`
- **THEN** target 存在且生成 alertmanager ConfigMap（或明确标注未实现）

### Requirement: API 契约文档准确性

The system SHALL maintain openapi.yaml consistent with the actual backend implementation.

#### Scenario: 房间码一致
- **WHEN** 查阅 openapi.yaml 中房间码示例
- **THEN** 示例为 5 位字符（如 "ABC23"），与 `domain/room_code.go:11` 一致

#### Scenario: match 端点状态准确
- **WHEN** 查阅 openapi.yaml 中 `/api/v1/registry/match`
- **THEN** 无 deprecated 标记，响应字段名为 `lobbyCode`（非 `code`）

### Requirement: 安全文档准确性

The system SHALL maintain threat-model.md consistent with the actual authentication implementation.

#### Scenario: JWT 算法准确
- **WHEN** 查阅 threat-model.md
- **THEN** JWT 算法声明为 ES256（非 HMAC-SHA256）

#### Scenario: Admin JWT 密钥来源准确
- **WHEN** 查阅 threat-model.md Admin JWT 章节
- **THEN** 声明共享 ECDSA 私钥（非独立 ADMIN_JWT_SECRET）

### Requirement: HPA 指标链路完整

The system SHALL provide prometheus-adapter configuration exposing ws_connections custom metric to HPA.

#### Scenario: HPA 可基于 WS 连接数伸缩
- **WHEN** HPA 查询 ws_connections 指标
- **THEN** prometheus-adapter 通过 APIService 暴露该指标
- **AND** HPA 可基于该指标自动扩缩容

### Requirement: 前端 CSP 覆盖

The system SHALL provide Content-Security-Policy meta tags in all HTML entry points.

#### Scenario: 所有 HTML 含 CSP
- **WHEN** 检查 5 个 HTML 文件
- **THEN** 每个文件均含 `<meta http-equiv="Content-Security-Policy" ...>` 标签

### Requirement: 环境配置示例准确

The system SHALL maintain .env.example consistent with actual env var names consumed by code.

#### Scenario: JWT 配置变量名
- **WHEN** 查阅 .env.example
- **THEN** 使用 JWT_PRIVATE_KEY/JWT_PUBLIC_KEY（非 JWT_SECRET），且为 PEM 格式示例

### Requirement: 针对性回归验证

The system SHALL verify each fix via targeted tests and CI gate, without re-running full v2 inspection.

#### Scenario: 修复后验证
- **WHEN** 完成一批修复
- **THEN** 为每个修复编写/扩展测试用例
- **AND** CI 全绿
- **AND** 更新 `docs/security/self-check-baseline.txt`

---

## MODIFIED Requirements

### Requirement: 供应链安全（镜像固定）

所有 balloon-game 镜像、docker-compose 镜像、GitHub Actions SHALL 使用 digest pin（@sha256）而非浮动标签或 sed 占位符替换。

### Requirement: K8s 安全上下文

所有 StatefulSet（含 Redis）SHALL 声明 securityContext（runAsNonRoot/readOnlyRootFilesystem/capabilities drop ALL），namespace SHALL 声明 PSS restricted admission 标签。

### Requirement: 前端网络弹性

所有前端 fetch 调用 SHALL 使用 AbortController 超时（关键路径 8s），与 lifecycle.ts 连接超时对齐。

### Requirement: ADR 一致性

ADR-013/014/015/018/022/025 SHALL 与当前代码状态一致，无过时引用、无矛盾标题、无错误交叉引用。

### Requirement: 测试覆盖补全

idempotency.go SETNX claim 路径、down.sql 回滚迁移、cmd/ 代码生成器、metrics 端点 SHALL 有测试覆盖。GDPR 匿名化断言 SHALL 使用 `||`（非 `&&`）。

### Requirement: 后端可观测性补全

store 包 SHALL 添加 slog 上下文日志，audit 写入失败 SHALL 重试（非仅日志），worker SHALL 暴露处理指标。

---

## REMOVED Requirements

### Requirement: v2-R-02 修复
**Reason**: Task 5 跨层验证确认 OutboxRepository.InsertOutboxEvent 实际已包装 circuit breaker（r.cb.Execute），v2-R-02 描述基于早期版本，无需修复代码。
**Migration**: 仅在文档中更新说明（数据流文档化时一并处理）。

### Requirement: v2-C-04 修复
**Reason**: v1 基线报告缺失属历史事实，无法补回。本次修复完成后建立新 baseline 即可。
**Migration**: 修复完成后生成 `2026-07-XX-self-inspection-v2-fix-baseline.txt` 作为新基线。

---

## 附录: OPTIONAL/FYI 处理原则

OPTIONAL（~98 项）与 FYI（~94 项）不纳入本计划强制范围，按以下原则按需处理：
- OPTIONAL: 在修复相邻代码时顺手处理（如重构 ws_connection 时顺带清理 console.log）
- FYI: 信息性备注，无需操作；若涉及正面评价则忽略，若涉及潜在风险则记录 backlog
- 跨批次合并: 同一文件的 OPTIONAL 与 REQUIRED 修复合并到同一子代理任务中
