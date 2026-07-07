# Task 3 审查结果汇总（Medium/Low 资产，36 个）

> 生成日期：2026-07-08
> 4 个子代理并行产出，本文件为主代理汇总固化
> 子代理原始产出：v2-task3-subA-backend-medlow.md / subB-periph-frontend.md / subC-frontend-periph-test.md / subD-infra-docs-medlow.md
> 审查性质：纯诊断，未修改任何业务代码
> 注：F 系列 ID 存在跨子代理冲突，最终报告（Task 6）将统一重编号

---

## 1. 评分矩阵（36 资产）

### 后端 Medium/Low（11 资产）— 子代理 A

| 资产 ID | 资产名 | v2 整体 | 状态 | 关键问题 |
|---------|--------|--------|------|---------|
| A-001 | apierror | 4.6 | 🟢 | 无显著问题 |
| A-010 | health | 4.3 | 🟢 | 无显著问题 |
| A-011 | idgen | 4.4 | 🟢 | 无显著问题 |
| A-012 | metrics | 4.6 | 🟢 | SLI 覆盖完整 |
| A-014 | nicknames | 3.8 | 🟡 | GenerateRandom 用字节长度非 rune 长度，中文名被拒 |
| A-018 | requestctx | 4.8 | 🟢 | 无显著问题 |
| A-021 | slogctx | 4.0 | 🟡 | CtxKey 导出但实际用未导出变量，误导调用方 |
| A-023 | telemetry | 4.4 | 🟢 | OTel 集成规范 |
| A-024 | validate | 4.4 | 🟢 | 无显著问题 |
| A-026 | testsecrets | 4.2 | 🟢 | 无显著问题 |
| A-027 | testutil | 4.0 | 🟡 | 测试辅助代码重复 |

**后端 Medium/Low 均分**: 4.3/5

### 后端外围 cmd/* + 前端 Medium/Low（9 资产）— 子代理 B

| 资产 ID | 资产名 | v2 整体 | 状态 | 关键问题 |
|---------|--------|--------|------|---------|
| A-029 | cmd/seed | 3.0 | 🟡 | sslmode 守卫可绕过 + 吞没错误仍报告成功 |
| A-030 | cmd/backfill-emails | 3.8 | 🟡 | 全量加载无分页 |
| A-031 | cmd/migrate-passwords | 4.0 | 🟢 | 无显著问题 |
| A-032 | cmd/gen-frontend-constants | 2.7 | 🟡 | 硬编码 TickRate + 无测试 |
| A-043 | game/其他 | 3.4 | 🟡 | drawDangerVignettes 死代码 |
| A-046 | shared/ui | 4.0 | 🟡 | audio 绑定 prefers-reduced-motion 语义错误 |
| A-047 | shared/data | 4.3 | 🟢 | 无显著问题 |
| A-048 | shared/assets | 4.7 | 🟢 | 无显著问题 |
| A-049 | test_fixtures | 2.6 | 🟡 | 魔法偏移无注释 |

### 前端外围 + 测试 Medium/Low（6 资产）— 子代理 C

| 资产 ID | 资产名 | v2 整体 | 状态 | 关键问题 |
|---------|--------|--------|------|---------|
| A-051 | CSS | 4.0 | 🟡 | 色板/z-index 未令牌化 |
| A-052 | HTML | 2.7 | 🟡 | 5 个 HTML 全缺 CSP meta 标签 |
| A-053 | 构建配置 | 3.3 | 🟡 | 依赖 `^` 范围未强制 npm ci |
| A-054 | 前端依赖 | 3.7 | 🟡 | 缺 engines 字段 |
| A-055 | vite-env.d.ts | 2.7 | 🔴 | 9 个全局可变状态 |
| A-059 | 后端 fuzz | 3.2 | 🟡 | DecodeNicknamePayload 未直接 fuzz |

### 基础设施 + 文档 Medium/Low（10 资产）— 子代理 D

| 资产 ID | 资产名 | v2 整体 | 状态 | 关键问题 |
|---------|--------|--------|------|---------|
| A-064 | release-please CI | 3.5 | 🟡 | Actions 未 digest pin |
| A-065 | docs-governance CI | 2.5 | 🔴 | CI 必失败（引用不存在文件）|
| A-070 | Prometheus | 2.8 | 🟡 | 告警引用不存在的指标 |
| A-071 | Alertmanager | 3.3 | 🟡 | rules-configmap.yaml 不存在 |
| A-072 | Grafana | 2.8 | 🟡 | datasource UID 不匹配 |
| A-074 | 项目配置 | 3.0 | 🟡 | pre-commit hooks 全 tag pin |
| A-076 | 环境配置 | 2.5 | 🔴 | .env.example 与代码严重脱节 |
| A-081 | 运维文档 | 2.8 | 🟡 | CockroachDB 文档说已实现但代码未实现 |
| A-082 | 开发文档 | 3.3 | 🟡 | coverage-policy 与实际不一致 |
| A-083 | 数据文档 | 2.5 | 🔴 | 引用已删除的索引 |

**Medium/Low 资产域均分**: 3.4/5（36 资产）

---

## 2. 发现汇总（按严重级别）

### CRITICAL（11 项）

| 发现 ID | 资产 | 轴 | 描述 | 子代理 |
|---------|------|----|------|--------|
| v2-C-25 | A-052 | 安全 | 5 个 HTML 全缺 CSP meta 标签（admin + 用户昵称场景风险）| C |
| v2-C-30 | A-065 | 正确性 | docs-governance CI 必失败（引用不存在的 alertmanager rules-configmap）| D |
| v2-C-31 | A-074 | 正确性 | `make sync-alert-rules` 无 target，`make ci` 必失败 | D |
| v2-C-32 | A-070 | 可观测性 | 告警引用不存在的指标 `pgxpool_acquire_count`（实际为 `db_pool_*`）| D |
| v2-C-34 | A-072 | 正确性 | Grafana datasource UID 不匹配，dashboard 加载报 "Datasource not found" | D |
| v2-C-35 | A-076 | 文档一致性 | .env.example 的 `JWT_SECRET` 实际应为 `JWT_PRIVATE_KEY`/`JWT_PUBLIC_KEY`（ECDSA PEM）| D |
| v2-C-36 | A-070 | 可观测性 | 告警引用 `game_active_ws_connections`，实际指标为 `ws_connections` | D |
| v2-C-37 | A-070 | 可观测性 | 告警引用 `ws_active_connections`，实际指标为 `ws_connections` | D |
| v2-C-38 | A-081 | 文档一致性 | CockroachDB 多区域"文档已写、代码未实现"（引用不存在的 ApplyCockroachMultiRegion）| D |
| v2-C-39 | A-083 | 文档一致性 | db-query-analysis.md 引用 migration 000008 已删除的索引 | D |
| v2-C-33 | A-071 | 正确性 | rules-configmap.yaml 不存在（CI 必失败关联）| D |

### REQUIRED（38 项）

| 发现 ID | 资产 | 描述 | 子代理 |
|---------|------|------|--------|
| v2-R-83 | A-014 | GenerateRandom 用字节长度非 rune 长度，中文名被拒 | A |
| v2-R-84 | A-021 | CtxKey 导出但用未导出变量存储，误导调用方 | A |
| v2-R-95 | A-029 | sslmode 守卫可被 `?sslmode=disable&sslmode=require` 绕过 | B |
| v2-R-96 | A-029 | 吞没插入错误仍报告"Seed completed" | B |
| v2-R-97 | A-030 | 全量加载用户无分页/游标 | B |
| v2-R-98 | A-032 | 硬编码 TickRate="15" | B |
| v2-R-99 | A-032 | 整个 cmd 无测试 | B |
| v2-R-100 | A-043 | drawDangerVignettes 死代码 | B |
| v2-R-101 | A-046 | audio 绑定 prefers-reduced-motion 语义错误 | B |
| v2-R-102 | A-049 | 魔法偏移构建 buffer 无注释 | B |
| v2-R-107 | A-052 | verify.html favicon 404 + 品牌不一致 | C |
| v2-R-108 | A-054 | 依赖 `^` 范围未强制 npm ci | C |
| v2-R-109 | A-054 | package.json 缺 engines 字段 | C |
| v2-R-110 | A-055 | vite-env.d.ts 9 个全局可变状态 | C |
| v2-R-111 | A-059 | DecodeNicknamePayload 未直接 fuzz | C |
| v2-R-119~142 | A-064~A-083 | 23 项基础设施+文档 REQUIRED（详见子代理 D 文件）| D |

### OPTIONAL / FYI

详见各子代理原始产出文件。统计：
- OPTIONAL: 37 项（A:6 + B:10 + C:7 + D:14）
- FYI: 40 项（A:19 + B:9 + C:5 + D:7）

---

## 3. 域级分析

| 域 | 资产数 | 均分 | 最低分资产 | 典型问题 |
|---|---|---|---|---|
| 后端 Medium/Low | 11 | 4.3 | A-014(3.8) | 中文名长度判断用字节非 rune |
| 后端外围 cmd/* | 4 | 3.4 | A-032(2.7) | 代码生成无测试 + sslmode 绕过 |
| 前端 Medium/Low | 5 | 3.8 | A-049(2.6) | 死代码 + 魔法偏移 |
| 前端外围 | 5 | 3.3 | A-055(2.7) | CSP 缺失 + 全局可变状态 |
| 测试 Medium/Low | 1 | 3.2 | A-059(3.2) | fuzz 覆盖不足 |
| 基础设施 Medium/Low | 7 | 2.9 | A-065/A-076(2.5) | CI 必失败 + .env.example 脱节 |
| 文档 Medium/Low | 3 | 2.9 | A-083(2.5) | 引用不存在代码 + 已删除索引 |

---

## 4. 跨资产高频根因（子代理 D 发现）

| 根因 | 涉及发现 | 影响 |
|------|---------|------|
| CI 必失败 | v2-C-30, v2-C-31 | alertmanager rules-configmap 不存在 + make sync-alert-rules 无 target |
| .env.example 与代码脱节 | v2-C-35, v2-R-129~132 | JWT_SECRET 应为 JWT_PRIVATE_KEY；6 个变量文档声明但代码零读取 |
| CockroachDB 文档已写代码未实现 | v2-C-38, v2-R-132 | 引用不存在的 ApplyCockroachMultiRegion + cockroach/001_multiregion.sql |
| 告警引用不存在的指标 | v2-C-32, v2-C-36, v2-C-37 | pgxpool_acquire_count → db_pool_*；ws_active_connections → ws_connections |
| Grafana datasource UID 不匹配 | v2-C-34 | local.yaml 未设 uid: Prometheus |
| migration 000008 删索引后文档未同步 | v2-C-39 | db-query-analysis.md 引用 idx_lobby_states_updated_at |
| 供应链 pin 不一致 | v2-R-119/120/126 | GitHub Actions + pre-commit hooks 全 tag pin |

---

## 5. 风险排名（Top 5）

| 排名 | 风险 | 资产 | 影响 |
|------|------|------|------|
| 1 | CI 必失败（alertmanager rules + make sync-alert-rules）| A-065/A-074 | CI 门禁形同虚设 |
| 2 | .env.example JWT_SECRET 误导 | A-076 | 开发者按文档配置无法启动（ECDSA 需 PEM）|
| 3 | 5 个 HTML 全缺 CSP | A-052 | XSS 风险（admin + 用户昵称场景）|
| 4 | 告警引用不存在的指标 | A-070 | 生产告警完全不生效 |
| 5 | CockroachDB 文档说已实现但代码未实现 | A-081 | 误导运维进行不可行的迁移 |
