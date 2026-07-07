# 代码库自检计划 v2（增量矩阵法）Spec

> change-id: evolve-self-inspection-v2
> 生成日期：2026-07-08
> 前置基线：`docs/superpowers/reports/2026-07-07-full-self-inspection-report.md`（v1，82 资产 × 5 轴，7 CRITICAL + 20 REQUIRED）
> 定位：纯诊断计划，不修改任何业务代码

---

## Why

2026-07-07 完成的首版矩阵法自检（v1）覆盖正确性/可读性/架构/安全/性能 5 轴，发现 7 个 CRITICAL + 20 个 REQUIRED 问题，但缺少**生产成熟度**维度（可观测性、可维护性、供应链安全、弹性、文档-代码一致性）。本计划在不废弃 v1 资产清单的前提下，**增量补充 5 个专业轴**并对全栈 82 资产重审，同时**回归验证** v1 的 27 项关键发现，为下一轮修复迭代提供 v2 基线。

## What Changes

- 在 v1 的 5 轴基础上**新增 5 个专业轴**：
  1. **可观测性（Observability）** — metrics / tracing / 结构化日志 / 告警规则
  2. **可维护性（Maintainability）** — 变更影响半径 / 测试稳定性 / 技术债 / 注释质量
  3. **供应链安全（Supply Chain）** — 依赖固定 / SBOM / 签名验证 / 镜像 digest pinning
  4. **弹性（Resilience）** — 超时 / 重试 / 熔断 / 降级 / 限流 / 背压
  5. **文档-代码一致性（Doc-Code Alignment）** — ADR / OpenAPI / 架构图 / README 与代码对齐
- 复用 v1 资产清单（A-001~A-083，剔除不存在的 A-045，实际 **82 资产**）
- 每个资产按"适用轴"矩阵审查，避免全量 10 轴（详略得当）
- **回归验证**：v1 的 7 CRITICAL + 20 REQUIRED 发现逐项三态验证（FIXED / PARTIAL / REGRESSION）
- **新增交叉验证**：5 项跨资产主题（前后端协议、认证链、数据流、CI 门禁、安全纵深）
- 执行方式：**子代理并行**（v1 已验证 4 路并行可行）+ 工具脚本辅助
- 产出：单一综合诊断报告 `docs/superpowers/reports/2026-07-08-self-inspection-v2-report.md`
- **BREAKING**：不修改业务代码；v1 报告保留为历史基线；新报告独立存放

## Impact

- **Affected specs**：无（纯诊断，不修改代码）
- **Affected code（仅读取）**：
  - `backend/internal/**` — Go 包（27 核心 + 7 外围）
  - `backend/cmd/**` — 命令行入口
  - `backend/migrations/**` — 数据库迁移
  - `backend/tests/integration/**` — 集成测试
  - `frontend/src/**` — TypeScript 源码
  - `infra/**` — K8s / Terraform
  - `deploy/**` — Prometheus / Alertmanager / Grafana
  - `.github/workflows/**` — CI/CD
  - `docs/**` — ADR / 架构 / API / 安全文档

---

## ADDED Requirements

### Requirement: 新增 5 个专业质量轴

系统 SHALL 在 v1 的 5 轴基础上，为适用资产审查以下 5 个新专业轴，每轴按统一标准评分（1-5）并记录发现。

#### 轴 6: 可观测性（Observability）
审查要点：
- **metrics**：是否暴露 Prometheus metrics？关键业务指标（在线房间数、活跃玩家、tick 延迟）是否覆盖？
- **tracing**：是否使用 OpenTelemetry？跨服务/跨 goroutine trace 上下文是否传播？
- **logging**：是否使用 `slog` 结构化日志？日志级别是否合理？敏感字段（email/token）是否脱敏？
- **alerting**：是否有告警规则？告警是否基于 SLO burn-rate？是否有通知渠道？

#### 轴 7: 可维护性（Maintainability）
审查要点：
- **变更影响半径**：修改一个功能需触及多少文件？是否有过度耦合？
- **测试稳定性**：测试是否 flaky？是否依赖外部状态（时间/网络/全局变量）？
- **技术债标记**：TODO/FIXME/HACK 数量与跟踪状态
- **注释质量**：注释是否解释"为什么"而非"做什么"？是否有误导性注释？

#### 轴 8: 供应链安全（Supply Chain）
审查要点：
- **依赖固定**：`go.sum` / `package-lock.json` 是否锁定？Go module 是否使用特定版本？
- **镜像 digest pinning**：Docker/K8s 镜像是否使用 `@sha256:` digest 而非 tag？
- **签名验证**：是否使用 cosign 签名镜像？是否验证签名？
- **SBOM**：是否生成 SBOM？是否在 CI 中校验？
- **依赖审计**：是否定期运行 `govulncheck` / `npm audit`？CI 是否阻塞？

#### 轴 9: 弹性（Resilience）
审查要点：
- **超时**：HTTP / DB / Redis 调用是否设置超时？超时是否合理？
- **重试**：是否使用 `internal/resilience/retry.go`？重试策略是否含指数退避？
- **熔断**：是否有 circuit breaker？是否覆盖关键依赖？
- **降级**：Redis / DB 故障时是否优雅降级（fail-open vs fail-closed 是否合理）？
- **限流**：是否有限流？是否区分用户/IP/全局？
- **背压**：WS / goroutine 是否有背压机制？是否有 worker pool？

#### 轴 10: 文档-代码一致性（Doc-Code Alignment）
审查要点：
- **ADR 一致性**：ADR 描述的决策是否在代码中体现？代码是否偏离 ADR？
- **OpenAPI 一致性**：`docs/api/openapi.yaml` 是否与实际 API 路由/请求/响应一致？
- **架构图一致性**：`docs/architecture/architecture.md` 描述的组件是否与代码一致？
- **README 一致性**：README 描述的构建/部署流程是否与 `Makefile` / CI 一致？
- **注释 vs 实现**：函数注释是否与实现一致？是否有误导性文档？

#### Scenario: 新轴审查完成
- **WHEN** 子代理完成对资产 A-NNN 的可观测性轴审查
- **THEN** 必须输出该轴 1-5 评分 + 发现清单（CRITICAL/REQUIRED/OPTIONAL/FYI）

#### Scenario: 新轴发现可追溯
- **WHEN** 子代理记录新轴发现
- **THEN** 必须包含：发现 ID、资产 ID、轴、严重级别、描述、位置（文件:行号）、建议

### Requirement: 适用轴映射（详略得当）

系统 SHALL 为每个资产维护"适用轴"列表，避免对所有资产审查全部 10 轴。映射规则：

| 资产类别 | 适用轴 | 轴数 |
|---------|-------|------|
| 后端核心 Critical（auth/game/handler/middleware/store/server/protocol 等） | 全部 10 轴 | 10 |
| 后端核心非 Critical | 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 可观测性 | 7 |
| 后端外围 cmd/* | 正确性 + 安全 + 可维护性 + 弹性 | 4 |
| 后端外围 migrations | 正确性 + 架构 + 可维护性 + 文档一致性 | 4 |
| 前端核心 Critical | 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 弹性 + 文档一致性 | 8 |
| 前端核心非 Critical | 正确性 + 可读性 + 架构 + 性能 + 可维护性 | 5 |
| 前端外围 | 正确性 + 安全 + 可维护性 | 3 |
| 测试资产 | 正确性 + 可读性 + 可维护性 + 可观测性 + 文档一致性 | 5 |
| 基础设施（K8s/Terraform/CI） | 正确性 + 安全 + 架构 + 可维护性 + 供应链 + 弹性 + 可观测性 + 文档一致性 | 8 |
| 监控配置（Prometheus/Alertmanager/Grafana） | 正确性 + 可观测性 + 可维护性 + 文档一致性 | 4 |
| Docker / 项目配置 | 正确性 + 安全 + 供应链 + 可维护性 | 4 |
| 文档资产 | 正确性 + 可读性 + 文档一致性 | 3 |

#### Scenario: 资产审查范围合理
- **WHEN** 子代理开始审查资产 A-NNN
- **THEN** 仅对该资产"适用轴"列表中的轴评分，不超界

### Requirement: 旧发现回归验证

系统 SHALL 对 v1 报告中的 27 项关键发现（7 CRITICAL + 20 REQUIRED）逐项验证修复状态，输出三态结果：

| 状态 | 含义 | 判据 |
|------|------|------|
| **FIXED** | 已完全修复 | 代码中已实施修复且无回归 |
| **PARTIAL** | 部分修复 | 修复不完整或存在残留风险 |
| **REGRESSION** | 未修复或回归 | 代码中仍存在原问题，或修复后被回退 |

#### Scenario: 回归验证完成
- **WHEN** 所有 27 项旧发现完成验证
- **THEN** 输出回归验证表（发现 ID → 状态 → 证据：文件:行号 → 备注）

#### Scenario: 回归验证可追溯
- **WHEN** 一项发现被标记为 FIXED
- **THEN** 必须引用具体修复 commit 或代码位置作为证据

### Requirement: 子代理并行执行

系统 SHALL 通过子代理并行执行审查，主代理负责汇总。批次划分：

| 批次 | 资产范围 | 数量 | 子代理数 | 备注 |
|------|---------|------|---------|------|
| 批次 1 | Critical 资产 | 22 | 4 | 后端 10 + 前端 6 + 基础设施 6 |
| 批次 2 | High 资产 | 24 | 4 | 后端 8 + 前端 5 + 测试 4 + 基础设施 4 + 文档 3 |
| 批次 3 | Medium/Low 资产 | 36 | 4 | 按域均分 |
| 批次 4 | 旧发现回归验证 | 27 项 | 1 | 串行验证 |
| 批次 5 | 跨层交叉验证 | 5 主题 | 1 | 依赖批次 1-3 完成 |

#### Scenario: 子代理产出标准化
- **WHEN** 子代理完成审查
- **THEN** 必须按统一模板输出：
  ```
  ## 资产 A-NNN: [名称]
  ### 适用轴评分
  | 轴 | 评分 | 关键发现 |
  |----|------|---------|
  ### 发现清单
  | 发现 ID | 轴 | 严重级别 | 描述 | 位置 | 建议 |
  ### 整体健康度: 🟢/🟡/🔴 X.X/5
  ```

#### Scenario: 子代理失败可恢复
- **WHEN** 某子代理审查失败或产出不符合模板
- **THEN** 主代理重新分发该资产给新子代理，不阻塞其他资产

### Requirement: 跨层交叉验证

系统 SHALL 执行 5 项跨资产交叉验证主题：

| 主题 | 涉及资产 | 验证内容 |
|------|---------|---------|
| 前后端协议一致性 | A-005, A-016, A-042, A-045 | 协议常量、字段顺序、编解码对齐 |
| 认证链端到端 | A-003, A-009, A-044, A-038 | 登录 → token → WS 握手完整链路 |
| 数据流完整性 | A-022, A-025, A-002, A-015 | 写路径：handler → store → outbox → worker → audit |
| CI 门禁覆盖 | A-061~A-065, A-033, A-056 | CI 是否覆盖所有 Critical 资产 |
| 安全纵深一致 | A-075, A-066, A-067, A-013 | 安全基线在 Terraform/K8s/中间件层是否一致 |

#### Scenario: 交叉验证完成
- **WHEN** 5 项交叉验证全部完成
- **THEN** 输出交叉验证表（主题 → 涉及资产 → 一致性状态 → 发现）

### Requirement: 综合诊断报告

系统 SHALL 输出单一综合报告至 `docs/superpowers/reports/2026-07-08-self-inspection-v2-report.md`，包含：

1. **执行摘要**：审查范围、发现统计、健康度趋势 vs v1
2. **全量评分矩阵**：82 资产 × 适用轴（最多 10 轴）
3. **新增 5 轴专项分析**：每轴的整体得分分布、Top 3 问题资产
4. **旧发现回归验证表**：27 项 → FIXED/PARTIAL/REGRESSION
5. **跨层交叉发现**：5 主题的发现清单
6. **风险排名**：Top 10 风险（含影响、涉及资产、建议优先级）
7. **盲区地图**：未被覆盖的领域、未验证的假设
8. **趋势对比**：v1 → v2 的健康度变化、发现数变化

#### Scenario: 报告可对比
- **WHEN** 报告生成完成
- **THEN** 必须包含与 v1 的趋势对比列（健康度 Δ、发现数 Δ、新增/解决发现数）

#### Scenario: 报告可追溯
- **WHEN** 报告中引用某发现
- **THEN** 必须包含资产 ID + 文件:行号，便于跳转验证

---

## MODIFIED Requirements

### Requirement: 资产清单（继承自 v1）
v1 资产清单 A-001~A-083 继承，仅以下调整：
- **A-045 (shard/game)**：v1 确认目录不存在，**永久移除**，实际 82 资产
- **A-005 (constants)**：v1 标记为 Low，本计划因其跨前后端协议关键性，**升级为 High**
- **A-032 (gen-frontend-constants)**：因涉及代码生成一致性，**升级为 Medium**

---

## REMOVED Requirements

### Requirement: 修复补丁产出
**Reason**：用户明确选择"诊断报告"产出物，不修改业务代码
**Migration**：发现的 CRITICAL/REQUIRED 问题将作为下一轮 spec 的输入（如 `fix-critical-findings-v2`）

### Requirement: CI 集成与门禁规则
**Reason**：用户未选择"CI 集成"和"门禁规则"产出物
**Migration**：扫描脚本可保留为一次性工具，未来如需 CI 化可单独发起 spec
