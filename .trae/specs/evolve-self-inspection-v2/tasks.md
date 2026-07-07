# Tasks — 代码库自检计划 v2（增量矩阵法）

> change-id: evolve-self-inspection-v2
> 执行原则：子代理并行 + 主代理汇总；纯诊断不修改业务代码

---

## Task 0: 准备阶段

- [ ] Task 0.1: 校验 v1 资产清单的现存性
  - 遍历 A-001~A-083 的路径，确认 82 个资产目录/文件均存在
  - 应用 v2 调整：移除 A-045；A-005 升级为 High；A-032 升级为 Medium
  - 产出：`docs/superpowers/reports/v2-asset-inventory.md`（资产清单 + 适用轴映射）

- [ ] Task 0.2: 准备统一审查模板
  - 在主代理上下文中固化子代理产出模板（资产 ID、轴评分表、发现清单、健康度）
  - 准备 5 个新轴的审查 checklist（每轴 5-8 个审查要点）

---

## Task 1: 批次 1 — Critical 资产审查（22 资产，4 子代理并行）

> 适用轴：后端 Critical 全 10 轴；前端 Critical 8 轴；基础设施 Critical 8 轴

- [ ] Task 1.1: 子代理 A — 后端 Critical 资产 1（6 个）
  - A-003 auth / A-006 crypto / A-007 domain / A-008 game / A-009 handler / A-013 middleware
  - 适用 10 轴：正确性/可读性/架构/安全/性能 + 可观测性/可维护性/供应链/弹性/文档一致性

- [ ] Task 1.2: 子代理 B — 后端 Critical 资产 2（4 个）
  - A-016 protocol / A-020 server / A-022 store / A-033 migrations
  - A-022 适用 10 轴；A-016/A-020 适用 10 轴；A-033 适用 4 轴（正确性/架构/可维护性/文档一致性）

- [ ] Task 1.3: 子代理 C — 前端 Critical 资产（6 个）
  - A-035 主入口 / A-037 渲染引擎 / A-038 WS 连接 / A-039 状态管理 / A-042 编解码 / A-044 shard/network
  - 适用 8 轴：正确性/可读性/架构/安全/性能 + 可维护性/弹性/文档一致性

- [ ] Task 1.4: 子代理 D — 基础设施 Critical 资产（6 个）
  - A-061 go-ci / A-062 ci-cd / A-066 Terraform / A-067 K8s base / A-068 K8s overlays / A-075 安全配置
  - 适用 8 轴：正确性/安全/架构 + 可维护性/供应链/弹性/可观测性/文档一致性

---

## Task 2: 批次 2 — High 资产审查（24 资产，4 子代理并行）

> 适用轴：后端 High 7 轴；前端 High 5-8 轴；测试 5 轴；基础设施 8 轴；文档 3 轴

- [ ] Task 2.1: 子代理 A — 后端 High 资产（8 个）
  - A-002 audit / A-004 config / A-005 constants（升级为 High）/ A-015 outbox / A-017 rbac / A-019 resilience / A-025 worker / A-028 cmd/server
  - A-005 适用 7 轴（含文档一致性，因跨前后端协议）

- [ ] Task 2.2: 子代理 B — 前端 High 资产（5 个）
  - A-036 UI 层 / A-040 输入与同步 / A-041 匹配与房间 / A-045 shard/game（已移除，跳过）/ A-050 页面入口
  - 实际审查 4 个资产

- [ ] Task 2.3: 子代理 C — 测试 + 文档 High 资产（7 个）
  - A-056 E2E / A-057 E2E helpers / A-058 后端 property / A-060 前端 property
  - A-077 ADR / A-078 架构文档 / A-079 API 文档 / A-080 安全文档
  - 共 8 个（测试 4 + 文档 4）

- [ ] Task 2.4: 子代理 D — 基础设施 High 资产（4 个）
  - A-063 security-scan / A-069 K8s global / A-073 Docker / A-034 集成测试
  - A-034 测试资产适用 5 轴；其余基础设施适用 8 轴

---

## Task 3: 批次 3 — Medium/Low 资产审查（36 资产，4 子代理并行）

> 适用轴：按资产类别映射（详见 spec.md 适用轴映射表）

- [ ] Task 3.1: 子代理 A — 后端 Medium/Low（11 个）
  - A-001 apierror / A-005（已升级，跳过）/ A-010 health / A-011 idgen / A-012 metrics / A-014 nicknames / A-018 requestctx / A-021 slogctx / A-023 telemetry / A-024 validate / A-026 testsecrets / A-027 testutil
  - 实际审查 11 个

- [ ] Task 3.2: 子代理 B — 后端外围 Medium/Low（4 个）+ 前端 Medium/Low（5 个）
  - 后端：A-029 cmd/seed / A-030 cmd/backfill-emails / A-031 cmd/migrate-passwords / A-032 cmd/gen-frontend-constants（升级为 Medium）
  - 前端：A-043 game/其他 / A-046 shard/ui / A-047 shard/data / A-048 shard/assets / A-049 test_fixtures
  - 共 9 个

- [ ] Task 3.3: 子代理 C — 前端外围 + 测试 Medium/Low（6 个）
  - A-051 CSS / A-052 HTML / A-053 构建配置 / A-054 前端依赖 / A-055 vite-env.d.ts
  - A-059 后端 fuzz
  - 共 6 个

- [ ] Task 3.4: 子代理 D — 基础设施 + 文档 Medium/Low（10 个）
  - 基础设施：A-064 release-please / A-065 docs-governance / A-070 Prometheus / A-071 Alertmanager / A-072 Grafana / A-074 项目配置 / A-076 环境配置
  - 文档：A-081 运维文档 / A-082 开发文档 / A-083 数据文档
  - 共 10 个

---

## Task 4: 旧发现回归验证（27 项，1 子代理）

> 依赖：Task 1-3 完成后执行（便于交叉引用新发现）

- [ ] Task 4.1: 子代理 E — 旧发现回归验证
  - 验证 v1 的 7 CRITICAL（C-01~C-07）+ 20 REQUIRED（R-01~R-20）
  - 逐项输出三态：FIXED / PARTIAL / REGRESSION
  - 每项必须引用证据（文件:行号 或 commit SHA）
  - 产出：回归验证表

---

## Task 5: 跨层交叉验证（5 主题，1 子代理）

> 依赖：Task 1-3 完成（需要新审查结果）

- [ ] Task 5.1: 子代理 F — 跨层交叉验证
  - 主题 1: 前后端协议一致性（A-005, A-016, A-042）
  - 主题 2: 认证链端到端（A-003, A-009, A-044, A-038）
  - 主题 3: 数据流完整性（A-022, A-025, A-002, A-015）
  - 主题 4: CI 门禁覆盖（A-061~A-065, A-033, A-056）
  - 主题 5: 安全纵深一致（A-075, A-066, A-067, A-013）
  - 产出：交叉验证表（主题 → 涉及资产 → 一致性状态 → 发现）

---

## Task 6: 综合报告生成

> 依赖：Task 1-5 全部完成

- [ ] Task 6.1: 主代理汇总所有子代理产出
  - 合并 82 资产的轴评分表
  - 合并所有发现清单（统一编号：v2-C-NN / v2-R-NN / v2-O-NN / v2-F-NN）
  - 计算各域平均分、整体健康度

- [ ] Task 6.2: 生成 5 个新轴的专项分析
  - 每轴的整体得分分布（直方图）
  - 每轴的 Top 3 问题资产
  - 每轴的典型发现模式

- [ ] Task 6.3: 生成风险排名 Top 10
  - 按影响 × 涉及资产数 × 修复成本综合排序
  - 每项含：风险描述、涉及资产、影响、建议优先级

- [ ] Task 6.4: 生成盲区地图
  - 测试覆盖盲区（未被测试覆盖的关键路径）
  - 文档盲区（有代码无文档的接口）
  - CI 盲区（未被 CI 门禁覆盖的资产）

- [ ] Task 6.5: 生成趋势对比（v1 → v2）
  - 健康度 Δ（整体 + 各域）
  - 发现数 Δ（新增 / 解决 / 持续）
  - 新轴 vs 旧轴得分对比

- [ ] Task 6.6: 写入报告文件
  - 路径：`docs/superpowers/reports/2026-07-08-self-inspection-v2-report.md`
  - 结构：执行摘要 → 全量评分矩阵 → 新轴专项 → 回归验证 → 交叉发现 → 风险排名 → 盲区地图 → 趋势对比

---

# Task Dependencies

```
Task 0 (准备) ─┐
              ├─→ Task 1 (Critical) ─┐
              ├─→ Task 2 (High)     ─┤
              └─→ Task 3 (Med/Low)  ─┤
                                     ├─→ Task 4 (回归验证) ─┐
                                     ├─→ Task 5 (交叉验证) ─┤
                                     │                      ├─→ Task 6 (综合报告)
                                     └──────────────────────┘
```

- **Task 1, 2, 3 可并行**（不同资产集，无依赖）
- **Task 4, 5 依赖 Task 1-3**（需要新审查结果作为对比基线）
- **Task 6 依赖 Task 1-5 全部完成**

---

# 执行规模估算

| 阶段 | 资产数 | 子代理数 | 预计产出 |
|------|--------|---------|---------|
| Task 1 (Critical) | 22 | 4 | 22 个资产审查报告 |
| Task 2 (High) | 24 | 4 | 24 个资产审查报告 |
| Task 3 (Med/Low) | 36 | 4 | 36 个资产审查报告 |
| Task 4 (回归) | 27 项 | 1 | 回归验证表 |
| Task 5 (交叉) | 5 主题 | 1 | 交叉验证表 |
| Task 6 (报告) | — | 主代理 | 综合报告 1 份 |
| **合计** | **82 资产** | **最多 4 并行** | **1 份综合报告** |
