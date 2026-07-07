# Tasks — 代码库自检计划 v2（增量矩阵法）

> change-id: evolve-self-inspection-v2
> 执行原则：子代理并行 + 主代理汇总；纯诊断不修改业务代码

---

## Task 0: 准备阶段 ✅

- [x] Task 0.1: 校验 v1 资产清单的现存性
  - 遍历 A-001~A-083 的路径，确认资产目录/文件均存在
  - **v2 修正**：A-045 实际存在（路径 `frontend/src/shared/game/`，v1 因 shard/shared 拼写错误误判）；资产数 82→83
  - A-005 升级为 High；A-032 升级为 Medium
  - 产出：`docs/superpowers/reports/v2-asset-inventory.md`（资产清单 + 适用轴映射 + 模板 + 新轴 checklist）

- [x] Task 0.2: 准备统一审查模板
  - 子代理产出模板已固化在 v2-asset-inventory.md 第 5 节
  - 5 个新轴的审查 checklist 已固化在 v2-asset-inventory.md 第 6 节（每轴 6-8 个审查要点）
  - **效率优化**：v2 聚焦 5 个新轴深度审查；原 5 轴继承 v1 评分，仅标记回归

---

## Task 1: 批次 1 — Critical 资产审查（22 资产，4 子代理并行）✅

> 适用轴：后端 Critical 全 10 轴；前端 Critical 8 轴；基础设施 Critical 8 轴
> 结果固化：`docs/superpowers/reports/v2-task1-results.md`

- [x] Task 1.1: 子代理 A — 后端 Critical 资产 1（6 个）✅
  - A-003 auth (4.8) / A-006 crypto (4.4) / A-007 domain (4.6) / A-008 game (4.6) / A-009 handler (4.8) / A-013 middleware (4.4)
  - 发现：v2-R-01, v2-O-01~03, v2-F-01

- [x] Task 1.2: 子代理 B — 后端 Critical 资产 2（4 个）✅
  - A-016 protocol (4.6) / A-020 server (4.3) / A-022 store (3.4) / A-033 migrations (3.8)
  - 发现：v2-R-02~06, v2-O-04~09, v2-F-02~05

- [x] Task 1.3: 子代理 C — 前端 Critical 资产（6 个）✅
  - A-035 (4.0) / A-037 (3.9) / A-038 (4.3) / A-039 (3.9) / A-042 (3.8) / A-044 (4.2)
  - 发现：v2-R-07~14, v2-O-10~17, v2-F-06~09

- [x] Task 1.4: 子代理 D — 基础设施 Critical 资产（6 个）✅
  - A-061 (3.6) / A-062 (3.6) / A-066 (2.0🔴) / A-067 (3.4) / A-068 (2.8) / A-075 (2.8)
  - 发现：v2-C-01~05(含1误报), v2-R-15~35, v2-O-18~24, v2-F-17~21
  - **注**：v2-C-04（v1 报告缺失）为误报，主代理确认报告存在

---

## Task 2: 批次 2 — High 资产审查（25 资产，4 子代理并行）✅

> 适用轴：后端 High 7 轴；前端 High 5-8 轴；测试 5 轴；基础设施 8 轴；文档 3 轴
> 结果固化：`docs/superpowers/reports/v2-task2-results.md`（含 ID 冲突修正映射）

- [x] Task 2.1: 子代理 A — 后端 High 资产（8 个）✅
  - A-002(3.3) / A-004(3.8) / A-005(2.5🔴) / A-015(3.5) / A-017(3.7) / A-019(4.3) / A-025(3.3) / A-028(5.0)
  - 发现：v2-C-08, v2-R-36~43, v2-O-25~34, v2-F-22~28

- [x] Task 2.2: 子代理 B — 前端 High 资产（5 个）✅
  - A-036(3.8) / A-040(3.6) / A-041(3.8) / A-045(3.4，v2 恢复) / A-050(3.4)
  - 发现：v2-R-44~51, v2-O-35~46(修正后), v2-F-29~36(修正后)

- [x] Task 2.3: 子代理 C — 测试 + 文档 High 资产（8 个）✅
  - A-056(3.4) / A-057(4.0) / A-058(4.0) / A-060(3.4) / A-077(3.0) / A-078(3.7) / A-079(2.3🔴) / A-080(3.3)
  - 发现：v2-C-09~14(原C-10~15), v2-R-52~74, v2-O-47~60(修正后), v2-F-37~44(修正后)

- [x] Task 2.4: 子代理 D — 基础设施 High 资产（4 个）✅
  - A-063(3.6) / A-069(3.0🔴) / A-073(3.5) / A-034(3.6)
  - 发现：v2-C-15(原C-15→重编号), v2-R-75~82(修正后), v2-O-61~68(修正后), v2-F-45~52
  - **注**：v1 基线报告缺失经 4 子代理确认，v2-C-04 非误报

---

## Task 3: 批次 3 — Medium/Low 资产审查（36 资产，4 子代理并行）✅

> 适用轴：按资产类别映射（详见 spec.md 适用轴映射表）
> 结果固化：`docs/superpowers/reports/v2-task3-results.md`

- [x] Task 3.1: 子代理 A — 后端 Medium/Low（11 个）✅
  - 均分 4.3/5；0 CRITICAL + 2 REQUIRED（v2-R-83/84）+ 6 OPTIONAL + 19 FYI

- [x] Task 3.2: 子代理 B — 后端外围 + 前端 Medium/Low（9 个）✅
  - 0 CRITICAL + 8 REQUIRED（v2-R-95~102）+ 10 OPTIONAL + 9 FYI

- [x] Task 3.3: 子代理 C — 前端外围 + 测试 Medium/Low（6 个）✅
  - 1 CRITICAL（v2-C-25 HTML 缺 CSP）+ 5 REQUIRED + 7 OPTIONAL + 5 FYI

- [x] Task 3.4: 子代理 D — 基础设施 + 文档 Medium/Low（10 个）✅
  - 10 CRITICAL（v2-C-30~39，CI必失败+告警指标错+文档脱节）+ 23 REQUIRED + 14 OPTIONAL + 7 FYI

---

## Task 4: 旧发现回归验证（27 项，1 子代理）✅

> 依赖：Task 1-3 完成后执行（便于交叉引用新发现）
> 结果固化：`docs/superpowers/reports/v2-task4-regression.md`

- [x] Task 4.1: 子代理 E — 旧发现回归验证 ✅
  - **25 FIXED + 1 PARTIAL + 0 REGRESSION + 1 无法验证**（v1 报告缺失）
  - 7 CRITICAL 全部 FIXED
  - PARTIAL: R-05 后端 unit 覆盖率 73.4% 仍低于 80% 门禁

---

## Task 5: 跨层交叉验证（5 主题，1 子代理）✅

> 依赖：Task 1-3 完成（需要新审查结果）
> 结果固化：`docs/superpowers/reports/v2-task5-cross-validation.md`

- [x] Task 5.1: 子代理 F — 跨层交叉验证 ✅
  - 主题 1: 前后端协议一致性 — 大体一致（v2-C-08 生成器路径错）
  - 主题 2: 认证链端到端 — 大体一致（threat-model JWT 算法双重错误）
  - 主题 3: 数据流完整性 — 部分一致（room_result_async 三写并行，direct write 无重试）
  - 主题 4: CI 门禁覆盖 — 不一致严重（E2E 矩阵含不存在的 performance.spec.ts 必失败）
  - 主题 5: 安全纵深一致 — 大体一致（Terraform 未参与 trusted proxy 配置）
  - 新发现：2 CRITICAL + 6 REQUIRED + 2 FYI

---

## Task 6: 综合报告生成 ✅

> 依赖：Task 1-5 全部完成
> 结果固化：`docs/superpowers/reports/2026-07-08-self-inspection-v2-report.md`

- [x] Task 6.1: 主代理汇总所有子代理产出 ✅
  - 合并 83 资产的轴评分表（Critical 22 + High 25 + Med/Low 36）
  - 合并所有发现清单（26 CRITICAL + 126 REQUIRED + ~98 OPTIONAL + ~94 FYI）
  - 计算各域平均分：Critical 3.93 / High 3.40 / Med-Low 3.40 / 整体 3.54

- [x] Task 6.2: 生成 5 个新轴的专项分析 ✅
  - 可观测性：Critical 3.0 / High 2.5（全场最低轴，前端无远程遥测）
  - 可维护性：Critical 3.9 / High 3.5（ADR 过时/矛盾）
  - 供应链：Critical 3.5 / High 2.5（镜像/Actions 未 digest pin）
  - 弹性：Critical 3.7 / High 2.8（HPA 链路断裂 + fetch 无超时）
  - 文档一致性：Critical 3.9 / High 2.0（全场问题最严重轴，openapi 多处错误）

- [x] Task 6.3: 生成风险排名 Top 10 ✅
  - Top 1: CI E2E 矩阵含不存在的 performance.spec.ts（P0）
  - Top 2: openapi.yaml 多处 CRITICAL 不一致（P0）
  - Top 3: threat-model.md JWT 算法双重错误（P0）
  - Top 4: HPA ws_connections 指标链路断裂（P0）
  - Top 5-10: Terraform/生成器/HTML CSP/告警/.env/E2E 漏跑（P1）

- [x] Task 6.4: 生成盲区地图 ✅
  - 测试覆盖盲区：9 项（E2E 漏跑 5 spec + idempotency/down.sql/cmd/无测试）
  - 文档盲区：9 项（openapi 缺 4 路由 + /resolve 不存在 + ws-protocol 缺布局 + ADR 过时）
  - CI 盲区：8 项（performance.spec.ts + 漏跑 5 spec + sync-alert-rules + 无通知 + Terraform 无 validate）

- [x] Task 6.5: 生成趋势对比（v1 → v2）✅
  - 健康度 Δ：Critical 4.42→3.93 (-0.49，新轴拉低非回归)
  - 发现数 Δ：v1 27 项 → v2 新增 152+ 项（新轴贡献 21/26 CRITICAL）
  - 新轴 vs 旧轴：旧轴 ~4.3 良好 / 新轴 ~3.0 严重不足

- [x] Task 6.6: 写入报告文件 ✅
  - 路径：`docs/superpowers/reports/2026-07-08-self-inspection-v2-report.md`
  - 结构：执行摘要 → 全量评分矩阵 → 新轴专项 → 回归验证 → 交叉发现 → 风险排名 → 盲区地图 → 趋势对比 → 发现索引 → 修正记录 → 行动计划

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
