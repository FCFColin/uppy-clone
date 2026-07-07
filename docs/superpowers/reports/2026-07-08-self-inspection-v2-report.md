# 代码库自检 v2 综合诊断报告

> 生成日期：2026-07-08
> 审查性质：纯诊断，未修改任何业务代码
> 审查方法：增量矩阵法（v1 5 轴 + v2 新增 5 轴 = 10 轴）+ 子代理并行 + 跨层交叉验证
> 资产范围：83 个资产（A-001~A-083，v1 误报 82 因 shard/shared 拼写错误）
> 执行规模：14 个子代理（4+4+4+1+1）+ 主代理汇总

---

## 0. 执行摘要

### 0.1 审查范围

| 维度 | 数值 |
|------|------|
| 资产总数 | 83（v1 误报 82，A-045 修正恢复） |
| 质量轴数 | 10（v1 原 5 轴 + v2 新增 5 轴） |
| Critical 资产 | 22 |
| High 资产 | 25 |
| Medium/Low 资产 | 36 |
| 子代理数 | 14（Task 1-3 各 4 路并行 + Task 4-5 各 1 路） |
| 跨层交叉验证主题 | 5 |
| v1 回归验证项 | 27 |

### 0.2 发现统计

| 严重级别 | Task 1 (Critical) | Task 2 (High) | Task 3 (Med/Low) | Task 5 (交叉) | **合计** |
|---------|-------------------|---------------|------------------|--------------|----------|
| CRITICAL | 5 | 8 | 11 | 2 | **26** |
| REQUIRED | 35 (14 组) | 47 | 38 | 6 | **126** |
| OPTIONAL | ~17 | 44 | 37 | 0 | **~98** |
| FYI | ~21 | 31 | 40 | 2 | **~94** |
| **合计** | **~78** | **130** | **126** | **10** | **~344** |

### 0.3 整体健康度

| 域 | 资产数 | v2 均分 | 状态 |
|---|--------|---------|------|
| Critical 资产 | 22 | 3.93/5 | 🟡 |
| High 资产 | 25 | 3.40/5 | 🟡 |
| Medium/Low 资产 | 36 | 3.40/5 | 🟡 |
| **整体加权** | **83** | **3.54/5** | **🟡** |

### 0.4 v1 回归结论

- **25 FIXED + 1 PARTIAL + 0 REGRESSION + 1 无法验证**（v1 报告缺失）
- 7 项 CRITICAL 全部 FIXED，无代码回归
- 唯一 PARTIAL：R-05 后端 unit 覆盖率 73.4% < 80% 门禁（技术债务 backlog）
- **v1 基线报告 `2026-07-07-full-self-inspection-report.md` 经 4 子代理确认不存在**（v2-C-04 非误报）

### 0.5 关键结论

1. **v1 修复无回归**：7 CRITICAL + 18/20 REQUIRED 已修复，代码无回归证据
2. **新轴暴露大量隐藏债务**：v2 新增 5 轴（可观测性/可维护性/供应链/弹性/文档一致性）贡献了大部分 CRITICAL（26 项中 21 项来自新轴视角）
3. **文档-代码一致性严重失衡**：openapi.yaml / ADR / threat-model / architecture.md 普遍与代码不一致，API 契约文档完全不可信
4. **CI 门禁存在阻断性故障**：E2E 矩阵引用不存在的 performance.spec.ts，阻塞所有 PR 合并
5. **基础设施 IaC 化不完整**：Terraform 未定义 GKE 集群/VPC，HPA 指标链路断裂，告警引用不存在的指标

---

## 1. 审查范围与方法

### 1.1 增量矩阵法

v2 在 v1 原 5 轴（正确性/可读性/架构/安全/性能）基础上新增 5 个专业轴：

| 新轴 | 关注点 | Checklist 要点数 |
|------|--------|-----------------|
| 可观测性 | metrics/tracing/日志/告警 | 8 |
| 可维护性 | 变更半径/技术债/注释/API 稳定性 | 6 |
| 供应链安全 | 依赖固定/digest pin/cosign/SBOM | 6 |
| 弹性 | 超时/重试/熔断/降级/限流/背压 | 6 |
| 文档-代码一致性 | ADR/OpenAPI/架构图/README 一致性 | 6 |

### 1.2 适用轴映射（详略得当）

| 资产类别 | 适用轴数 | 说明 |
|---------|---------|------|
| 后端核心 Critical | 10 | 全 10 轴 |
| 后端核心非 Critical | 7 | 原 5 + 可观测性 + 可维护性 |
| 前端核心 Critical | 8 | 原 5 + 可维护性 + 弹性 + 文档一致性 |
| 前端核心非 Critical | 5 | 原 5 轴 |
| 测试资产 | 5 | 正确性/可读性/可维护性/可观测性/文档一致性 |
| 基础设施 | 8 | 正确性/安全/架构/可维护性/供应链/弹性/可观测性/文档一致性 |
| 文档资产 | 3 | 正确性/可读性/文档一致性 |

### 1.3 执行流程

```
Task 0 (准备) ─┐
              ├─→ Task 1 (Critical, 4 并行) ─┐
              ├─→ Task 2 (High, 4 并行)      ─┤
              └─→ Task 3 (Med/Low, 4 并行)   ─┤
                                             ├─→ Task 4 (回归, 1) ─┐
                                             ├─→ Task 5 (交叉, 1) ─┤
                                             │                      ├─→ Task 6 (综合报告)
                                             └──────────────────────┘
```

### 1.4 固化文件索引

| 文件 | 内容 |
|------|------|
| v2-asset-inventory.md | 资产清单 + 适用轴映射 + 模板 + 新轴 checklist |
| v2-task1-results.md | 22 Critical 资产审查汇总 |
| v2-task2-results.md | 25 High 资产审查汇总（含 ID 冲突修正映射） |
| v2-task3-results.md | 36 Medium/Low 资产审查汇总 |
| v2-task4-regression.md | 27 项 v1 回归验证 |
| v2-task5-cross-validation.md | 5 主题跨层交叉验证 |
| v2-task2-subA~D-*.md | Task 2 四子代理原始产出 |
| v2-task3-subA~D-*.md | Task 3 四子代理原始产出 |

---

## 2. 全量评分矩阵

### 2.1 Critical 资产（22 个，均分 3.93）

| 资产 ID | 资产名 | v1 | 可观测性 | 可维护性 | 供应链 | 弹性 | 文档一致性 | v2 整体 | Δ | 状态 |
|---------|--------|----|---------|---------|--------|------|----------|--------|----|------|
| A-003 | auth | 4.4 | 5 | 5 | 4 | 5 | 5 | 4.8 | +0.4 | 🟢 |
| A-006 | crypto | 5.0 | 4 | 5 | 5 | 4 | 3 | 4.4 | -0.6 | 🟡 |
| A-007 | domain | 4.6 | 4 | 5 | 5 | 4 | 5 | 4.6 | 0 | 🟢 |
| A-008 | game | 4.0 | 5 | 4 | 4 | 5 | 5 | 4.6 | +0.6 | 🟢 |
| A-009 | handler | 4.0 | 5 | 5 | 4 | 5 | 5 | 4.8 | +0.8 | 🟢 |
| A-013 | middleware | 4.7 | 5 | 3 | 4 | 5 | 5 | 4.4 | -0.3 | 🟡 |
| A-016 | protocol | 4.9 | 3 | 5 | 4 | 4 | 5 | 4.6 | -0.3 | 🟢 |
| A-020 | server | 4.1 | 5 | 4 | 3 | 5 | 5 | 4.3 | +0.2 | 🟢 |
| A-022 | store | 3.3 | 3 | 4 | 3 | 3 | 4 | 3.4 | +0.1 | 🟡 |
| A-033 | migrations | 4.4 | 2 | 4 | 3 | 3 | 4 | 3.8 | -0.6 | 🟡 |
| A-035 | 主入口 | 4.4 | 2 | 4 | 4 | 4 | 4 | 4.0 | -0.4 | 🟢 |
| A-037 | 渲染引擎 | 4.2 | 2 | 4 | 4 | 4 | 4 | 3.9 | -0.3 | 🟡 |
| A-038 | WS 连接 | 4.8 | 3 | 4 | 4 | 5 | 3 | 4.3 | -0.5 | 🟢 |
| A-039 | 状态管理 | 4.4 | 2 | 3 | 4 | 4 | 4 | 3.9 | -0.5 | 🟡 |
| A-042 | 编解码 | 4.4 | 2 | 4 | 4 | 4 | 2 | 3.8 | -0.6 | 🟡 |
| A-044 | shared/network | 4.8 | 2 | 4 | 4 | 4 | 4 | 4.2 | -0.6 | 🟢 |
| A-061 | go-ci | 4.8 | 3 | 4 | 4 | 3 | 4 | 3.6 | -1.2 | 🟡 |
| A-062 | ci-cd | 4.8 | 3 | 4 | 4 | 3 | 4 | 3.6 | -1.2 | 🟡 |
| A-066 | Terraform | 4.6 | 2 | 2 | 2 | 2 | 2 | 2.0 | -2.6 | 🔴 |
| A-067 | K8s base | 4.6 | 3 | 4 | 4 | 3 | 3 | 3.4 | -1.2 | 🟡 |
| A-068 | K8s overlays | 4.6 | 3 | 3 | 2 | 3 | 3 | 2.8 | -1.8 | 🟡 |
| A-075 | 安全配置 | 4.0 | 2 | 3 | 3 | 3 | 3 | 2.8 | -1.2 | 🟡 |

**Critical 域均分**: v1=4.42 → v2=3.93 (Δ-0.49)

### 2.2 High 资产（25 个，均分 3.40）

| 资产 ID | 资产名 | v2 整体 | 状态 | 关键问题 |
|---------|--------|--------|------|---------|
| A-002 | audit | 3.3 | 🟡 | loadLastHash 无超时 + writeToDB 丢弃记录 |
| A-004 | config | 3.8 | 🟢 | getDurationEnv 与 GetEnvDuration 行为不一致 |
| A-005 | constants | 2.5 | 🔴 | 生成器输出路径错误 + 重复定义 |
| A-015 | outbox | 3.5 | 🟢 | at-least-once 语义未文档化 |
| A-017 | rbac | 3.7 | 🟢 | 审计 ActorID 用 role 而非用户 ID |
| A-019 | resilience | 4.3 | 🟢 | 弹性实现最完善 |
| A-025 | worker | 3.3 | 🟡 | email worker 消费者 ID 硬编码 + 无退避 |
| A-028 | cmd/server | 5.0 | 🟢 | 极简入口无问题 |
| A-036 | UI 层 | 3.8 | 🟡 | 无远程遥测 + DOM 引用风格不一 |
| A-040 | 输入与同步 | 3.6 | 🟡 | console.log 残留 + fetch 无超时 + 状态分散 |
| A-041 | 匹配与房间 | 3.8 | 🟡 | 无远程遥测 + 匹配失败无重试 |
| A-045 | shared/game | 3.4 | 🟡 | 死代码 + ADR-025 与代码不一致 |
| A-050 | 页面入口 | 3.4 | 🟡 | fetch 无超时 + 重复实现房间校验 |
| A-056 | E2E 测试 | 3.4 | 🟡 | waitForTimeout flaky + match 端点文档矛盾 |
| A-057 | E2E helpers | 4.0 | 🟢 | lobbyCode vs code 字段名不一致 |
| A-058 | 后端 property | 4.0 | 🟢 | rapid fail 文件已提交 |
| A-060 | 前端 property | 3.4 | 🟡 | 静默吞错"合法化"解码缺陷 |
| A-077 | ADR | 3.0 | 🟡 | ADR-013 引用错误 + ADR-022 过时 + ADR-025 矛盾 |
| A-078 | 架构文档 | 3.7 | 🟡 | "可变单例"引用过时 + 房间码示例 6 位 |
| A-079 | API 文档 | 2.3 | 🔴 | 房间码错 + match 端点错 + 字段名错 + JWT 算法错 |
| A-080 | 安全文档 | 3.3 | 🟡 | JWT 算法错 + match 限流"配置预留"错 |
| A-063 | security-scan CI | 3.6 | 🟡 | Actions 未 digest pin + 扫描覆盖窄 |
| A-069 | K8s global | 3.0 | 🔴 | HPA 链路断裂 + 镜像未 digest pin + Redis 缺 securityContext |
| A-073 | Docker | 3.5 | 🟡 | docker-compose 7 镜像未 digest pin + name 残留 |
| A-034 | 集成测试 | 3.6 | 🟡 | GDPR 断言逻辑错误（&&应为||） |

### 2.3 Medium/Low 资产（36 个，均分 3.40）

| 域 | 资产数 | 均分 | 最低分资产 | 典型问题 |
|---|--------|------|-----------|---------|
| 后端 Medium/Low | 11 | 4.3 | A-014(3.8) | 中文名长度判断用字节非 rune |
| 后端外围 cmd/* | 4 | 3.4 | A-032(2.7) | 代码生成无测试 + sslmode 绕过 |
| 前端 Medium/Low | 5 | 3.8 | A-049(2.6) | 死代码 + 魔法偏移 |
| 前端外围 | 5 | 3.3 | A-055(2.7) | CSP 缺失 + 全局可变状态 |
| 测试 Medium/Low | 1 | 3.2 | A-059(3.2) | fuzz 覆盖不足 |
| 基础设施 Medium/Low | 7 | 2.9 | A-065/A-076(2.5) | CI 必失败 + .env.example 脱节 |
| 文档 Medium/Low | 3 | 2.9 | A-083(2.5) | 引用不存在代码 + 已删除索引 |

### 2.4 最低分资产 Top 10（风险热点）

| 排名 | 资产 ID | 资产名 | v2 评分 | 主要扣分轴 |
|------|---------|--------|--------|-----------|
| 1 | A-066 | Terraform | 2.0 🔴 | 全轴低分（可维护性/供应链/弹性/文档一致性 均 2） |
| 2 | A-079 | API 文档 | 2.3 🔴 | 文档一致性 1（openapi 多处 CRITICAL 错误） |
| 3 | A-005 | constants | 2.5 🔴 | 文档一致性 2（生成器路径错）+ 可维护性 2 |
| 4 | A-065 | docs-governance CI | 2.5 🔴 | 正确性（CI 必失败） |
| 5 | A-076 | 环境配置 | 2.5 🔴 | 文档一致性（.env.example 脱节） |
| 6 | A-083 | 数据文档 | 2.5 🔴 | 文档一致性（引用已删除索引） |
| 7 | A-032 | gen-frontend-constants | 2.7 | 可维护性（无测试 + 硬编码） |
| 8 | A-052 | HTML | 2.7 | 安全（5 个 HTML 缺 CSP） |
| 9 | A-055 | vite-env.d.ts | 2.7 | 可维护性（9 个全局可变状态） |
| 10 | A-049 | test_fixtures | 2.6 | 可维护性（魔法偏移无注释） |

---

## 3. 5 个新轴专项分析

### 3.1 可观测性轴（Observability）

**整体得分分布**：Critical 均分 3.0 / High 均分 2.5 — 全场最低轴

| 评分 | 资产数 | 代表资产 |
|------|--------|---------|
| 1 | 0 | — |
| 2 | 14 | A-033, A-066, A-035, A-037, A-039, A-042, A-044, A-075, A-050, A-036, A-040, A-041, A-025, A-070 |
| 3 | 6 | A-061, A-062, A-013, A-056, A-067, A-073 |
| 4 | 2 | A-016, A-023 |
| 5 | 6 | A-003, A-008, A-009, A-020, A-019, A-012 |

**Top 3 问题资产**：
1. **A-033 migrations (2)** — 迁移过程无任何可观测性
2. **A-066 Terraform (2)** — 无监控 IaC，无告警规则
3. **A-035/A-037/A-039/A-042/A-044 前端 (均 2)** — 前端无远程遥测（无 Sentry/web-vitals/sendBeacon）

**典型发现模式**：
- 前端 5 个核心资产均无远程可观测性，生产环境无法定位客户端故障
- store 包零 slog 调用，持久化层错误无日志上下文
- worker 缺处理指标（成功/失败/延迟/队列深度）
- E2E 无 /metrics、/health/degraded、OTel span 路径覆盖
- CI 无失败通知渠道，失败仅体现在 workflow run 状态

### 3.2 可维护性轴（Maintainability）

**整体得分分布**：Critical 均分 3.9 / High 均分 3.5 — 中等

| 评分 | 资产数 | 代表资产 |
|------|--------|---------|
| 2 | 2 | A-066, A-005 |
| 3 | 8 | A-013, A-039, A-068, A-075, A-077, A-034, A-052, A-074 |
| 4 | 多数 | 大部分资产 |
| 5 | 4 | A-003, A-006, A-007, A-016 |

**Top 3 问题资产**：
1. **A-066 Terraform (2)** — GKE 未 IaC 化，stub 文件堆积
2. **A-005 constants (2)** — 常量重复定义 + 生成器路径错
3. **A-077 ADR (3)** — ADR 过时/矛盾（ADR-013 引用错误、ADR-022 过时、ADR-025 矛盾）

**典型发现模式**：
- ADR 文档过时/矛盾（ADR-013/014/018/022/025 多处不一致）
- 常量重复定义（constants/protocol.go 与 protocol/constants.go alias）
- 测试断言错误（GDPR && 应为 ||）
- 状态分散（ws_connection.ts 14+ 模块级 setter）
- 死代码（phaseFromCode/phaseToCode、drawDangerVignettes）

### 3.3 供应链安全轴（Supply Chain）

**整体得分分布**：Critical 均分 3.5 / High 均分 2.5 — 严重不足

| 评分 | 资产数 | 代表资产 |
|------|--------|---------|
| 2 | 6 | A-066, A-068, A-063, A-069, A-053, A-074 |
| 3 | 8 | A-020, A-022, A-033, A-075, A-073, A-067, A-061, A-062 |
| 4 | 4 | A-003, A-006, A-007, A-008 |
| 5 | 4 | A-009, A-013, A-016, A-019 |

**Top 3 问题资产**：
1. **A-066 Terraform (2)** — 无 CI validate，无 required_version
2. **A-068 K8s overlays (2)** — 镜像占位符 sed 替换，无 digest pin
3. **A-063 security-scan CI (2)** — GitHub Actions 全浮动标签，无 digest pin

**典型发现模式**：
- GitHub Actions 全仓用浮动标签（@v4/@v5），无 @sha256 digest pin
- 镜像未 digest pin：balloon-game（占位符 sed）、docker-compose 7 镜像、testcontainers
- 工具安装用 @latest（govulncheck/golang-migrate/go-licenses）
- Terraform 无 CI validate，无 required_version 约束
- SBOM + cosign 已实现（go-ci.yml），但仅覆盖 CI 推送镜像

### 3.4 弹性轴（Resilience）

**整体得分分布**：Critical 均分 3.7 / High 均分 2.8 — 中等偏下

| 评分 | 资产数 | 代表资产 |
|------|--------|---------|
| 2 | 2 | A-066, A-002 |
| 3 | 8 | A-033, A-067, A-068, A-075, A-069, A-050, A-061, A-062 |
| 4 | 8 | A-006, A-007, A-013, A-016, A-036, A-040, A-041, A-073 |
| 5 | 6 | A-003, A-008, A-009, A-020, A-038, A-019 |

**Top 3 问题资产**：
1. **A-066 Terraform (2)** — 单区域，无灾备
2. **A-002 audit (2)** — loadLastHash 无超时 + writeToDB 丢弃记录
3. **A-069 K8s global (3, 含 CRITICAL)** — HPA ws_connections 指标链路断裂

**典型发现模式**：
- 全前端无 AbortController，fetch 调用无超时（A-040/A-050）
- HPA 依赖自定义指标 ws_connections，但无 prometheus-adapter 配置（v2-C-03/C-15）
- sed 占位符替换脆弱（ci-cd.yml 自承认 fragile）
- audit 写入失败仅日志不重试，记录丢失
- room_result_async 三写并行，direct write 无重试

### 3.5 文档-代码一致性轴（Doc-Code Alignment）

**整体得分分布**：Critical 均分 3.9 / High 均分 2.0 — 全场问题最严重轴

| 评分 | 资产数 | 代表资产 |
|------|--------|---------|
| 1 | 1 | A-079 |
| 2 | 5 | A-042, A-066, A-005, A-077, A-080 |
| 3 | 8 | A-067, A-068, A-075, A-078, A-056, A-060, A-045, A-052 |
| 4 | 多数 | 大部分资产 |
| 5 | 6 | A-003, A-007, A-008, A-009, A-013, A-020 |

**Top 3 问题资产**：
1. **A-079 API 文档 (1)** — openapi.yaml 房间码 6 位（实际 5）、match 标记 deprecated（实际已实现）、字段名 code（实际 lobbyCode）、4 个路由缺失文档
2. **A-042 编解码 (2)** — ws-protocol.md 未记录 SNAPSHOT/RESTART_STATUS 布局
3. **A-066 Terraform (2)** — ADR-013 引用错误、ADR-015 状态过时

**典型发现模式**：
- openapi.yaml 与代码多处 CRITICAL 不一致（房间码/match/字段名/JWT 算法）
- ADR 过时/矛盾（ADR-013/014/018/022/025 交叉不一致）
- threat-model.md JWT 算法双重错误（HMAC-SHA256 → 实际 ES256）
- architecture.md 房间码示例 6 位（应 5 位）
- CockroachDB 文档说已实现但代码未实现
- .env.example 的 JWT_SECRET 应为 JWT_PRIVATE_KEY/JWT_PUBLIC_KEY

### 3.6 新轴 vs 旧轴得分对比

| 轴类别 | 轴 | Critical 均分 | High 均分 | 评价 |
|--------|----|--------------|----------|------|
| **旧轴** | 正确性 | ~4.5 | ~4.0 | 良好 |
| **旧轴** | 可读性 | ~4.3 | ~4.0 | 良好 |
| **旧轴** | 架构 | ~4.5 | ~4.0 | 良好 |
| **旧轴** | 安全 | ~4.3 | ~3.8 | 良好 |
| **旧轴** | 性能 | ~4.4 | ~3.9 | 良好 |
| **新轴** | 可观测性 | 3.0 | 2.5 | 🔴 严重不足 |
| **新轴** | 可维护性 | 3.9 | 3.5 | 🟡 中等 |
| **新轴** | 供应链 | 3.5 | 2.5 | 🔴 严重不足 |
| **新轴** | 弹性 | 3.7 | 2.8 | 🟡 中等偏下 |
| **新轴** | 文档一致性 | 3.9 | 2.0 | 🔴 严重不足 |

**结论**：旧轴（v1 已覆盖）健康度良好（~4.3）；新轴（v2 新增）健康度明显偏低（~3.0），证明 v2 增量审查的必要性 — 新轴暴露了 v1 未覆盖的隐藏债务。

---

## 4. 回归验证（v1 → v2）

### 4.1 验证基线说明

**v1 基线报告 `docs/superpowers/reports/2026-07-07-full-self-inspection-report.md` 经 4 子代理确认不存在**（reports/ 目录仅含 v2-* 文件）。

回归验证基于间接证据：
1. `docs/security/self-check-checklist.md` 子 agent 结论（2026-06-27）
2. `docs/security/self-check-baseline.txt`（2026-06-27 post-Bugbot 基线）
3. 当前代码状态（HEAD 实测）

### 4.2 回归验证结果

| 状态 | CRITICAL | REQUIRED | 合计 | 占比 |
|------|---------|----------|------|------|
| FIXED | 7 | 18 | 25 | 92.6% |
| PARTIAL | 0 | 1 | 1 | 3.7% |
| REGRESSION | 0 | 0 | 0 | 0% |
| 无法验证 | 0 | 1 | 1 | 3.7% |
| **合计** | **7** | **20** | **27** | **100%** |

### 4.3 CRITICAL 修复确认（7/7 FIXED）

| v1 发现 | 状态 | 证据 |
|---------|------|------|
| C-01: GKE 未 wire TRUSTED_PROXY_CIDRS | FIXED | service.yaml:128-133 env 注入 + region-config.yaml:15 |
| C-02: Dockerfile Go builder digest pin 回归 | FIXED | Dockerfile:11 三阶段全 digest pin |
| C-03: refresh token in localStorage | FIXED | auth.ts:5 注释 HttpOnly cookie + 全量 grep 无 localStorage |
| C-04: 无效 CIDR 静默忽略 | FIXED | env.go:106-130 validateTrustedProxyCIDRS + 测试覆盖 |
| C-05: admin lockout fail-open | FIXED | admin_login.go:64-70 改为 fail-closed (503) |
| C-06: restore 缺 Redis 注册 | FIXED | hub_restore.go:52-53 + hub_redis_registry.go:28-31 |
| C-07: 全 Pod PG restore | FIXED | hub_restore.go:37 shouldLocalMaterializeRoom 检查 |

### 4.4 唯一 PARTIAL

- **R-05 后端 unit 覆盖率 73.4% < 80% 门禁**（coverage-policy.md:9）
- self-check-baseline.txt:13 明确标记 PARTIAL（backlog）
- 未升级为 CRITICAL，归类为技术债务

### 4.5 无法验证项

- **R-20**：v1 报告缺失导致 1 项 REQUIRED 无法精确对应到代码位置

---

## 5. 跨层交叉验证（5 主题）

### 5.1 一致性状态总览

| 主题 | 涉及资产数 | ✅一致 | ⚠️部分一致 | ❌不一致 |
|------|-----------|--------|-----------|---------|
| 1: 前后端协议一致性 | 4 | 2 | 0 | 1 |
| 2: 认证链端到端 | 5 | 4 | 0 | 1 |
| 3: 数据流完整性 | 4 | 2 | 2 | 0 |
| 4: CI 门禁覆盖 | 7 | 3 | 1 | 3 |
| 5: 安全纵深一致 | 4 | 2 | 1 | 1 |

### 5.2 主题 1: 前后端协议一致性 — 大体一致

- ✅ 协议核心常量（MsgType/PhaseCode/EndReason）前后端数值完全一致
- ✅ 编解码字段顺序、字节序对齐
- ❌ **v2-C-08 生成器路径错位**：gen-frontend-constants 写入 `shared/constants.ts`（不存在），前端实际用 `shared/game/constants.ts`，生成器失效，"伪生成文件"靠手维护
- **新发现**：PALETTE_COLORS 和 END_REASON 无生成器同步、无 CI 校验

### 5.3 主题 2: 认证链端到端 — 大体一致

- ✅ 登录 → token 签发 → cookie → WS 握手认证完整链路一致
- ✅ Refresh Token 轮换 + reuse 检测 + 撤销链路完整
- ❌ **v2-C-14 threat-model.md JWT 算法双重错误**：
  1. 行 11：声明 HMAC-SHA256，实际 ES256
  2. 行 86：声明 Admin JWT 独立 ADMIN_JWT_SECRET，实际共享 ECDSA 私钥
- **新发现 (CRITICAL)**：threat-model.md:86 Admin JWT 密钥来源错误，安全评审基于错误信息

### 5.4 主题 3: 数据流完整性 — 部分一致

- ✅ 主链路完整：handler → store.InsertOutboxEvent → Publisher → Redis Stream → worker → PostgreSQL
- ✅ at-least-once 语义：Outbox Publisher + GameResultWorker + EmailWorker 均实现
- ⚠️ **v2-R-02 描述修正**：OutboxRepository.InsertOutboxEvent 实际已包装 circuit breaker（v2-R-02 描述与代码不符）
- ⚠️ **audit 写入失败仅日志不重试**（v2-R-37）
- **新发现 (REQUIRED)**：room_result_async.go 三写并行（direct write + queue + outbox），direct write 失败无重试

### 5.5 主题 4: CI 门禁覆盖 — 不一致严重

- ✅ Critical 资产 CI 覆盖：go-ci 14 job + ci-cd 5 job + docs-governance 5 job
- ❌ **新发现 (CRITICAL)**：ci-cd.yml:55 E2E 矩阵含 `performance`，但 tests/e2e/ 无 performance.spec.ts → E2E job 必失败，阻塞所有 PR
- ❌ **新发现 (REQUIRED)**：CI E2E 漏跑 5 个 critical spec（auth/admin/security/network_boundary/concurrency）
- ❌ **v2-C-31**：Makefile .PHONY 声明 sync-alert-rules 但无 target → `make sync-alert-rules` 必失败
- **v2-C-30 描述修正**：docs-governance.yml 实际不引用 rules-configmap，实际失败的是 `make sync-alert-rules` 命令本身

### 5.6 主题 5: 安全纵深一致 — 大体一致

- ✅ 应用层安全基线完整：CSP nonce + HSTS + CORS + TrustedProxy + RateLimit
- ✅ K8s 层安全基线：NetworkPolicy + balloon-game securityContext + Workload Identity
- ⚠️ **Redis securityContext 缺失**（v2-R-78）：两个 Redis StatefulSet 无安全上下文
- ⚠️ **PSS restricted 标签缺失**（v2-R-79）
- ❌ **Terraform 不参与 trusted proxy 配置**（新发现 REQUIRED）：variables.tf 无 trusted_proxy_cidrs，与 K8s 层不一致
- ❌ **GKE 集群 + VPC 未 IaC 化**（v2-C-01）：ci-cd 引用手动创建集群

---

## 6. 风险排名 Top 10

按「影响 × 涉及资产数 × 紧迫性」综合排序：

| 排名 | 风险 | 涉及资产 | 严重级别 | 影响 | 建议优先级 |
|------|------|---------|---------|------|-----------|
| 1 | CI E2E 矩阵含不存在的 performance.spec.ts | A-062, A-056 | CRITICAL | E2E job 必失败，阻塞所有 PR 合并，CI 门禁形同虚设 | **P0 立即** |
| 2 | openapi.yaml 多处 CRITICAL 不一致（房间码 5vs6 / match deprecated / 字段名 / 4 路由缺失） | A-079 | CRITICAL | API 契约文档完全不可信，客户端集成会失败 | **P0 立即** |
| 3 | threat-model.md JWT 算法 + Admin 密钥双重错误 | A-080, A-003 | CRITICAL | 安全评审基于错误信息，密钥泄露影响半径被低估 | **P0 立即** |
| 4 | HPA ws_connections 指标链路断裂（无 prometheus-adapter） | A-067, A-069 | CRITICAL | WS 压力下无法自动扩容，单实例连接超限 | **P0 立即** |
| 5 | Terraform 未定义 GKE 集群 + VPC + 多区域缺失 | A-066 | CRITICAL | 违反 IaC 原则，灾备切换不可恢复 | **P1 本周** |
| 6 | gen-frontend-constants 输出路径错误，前后端常量漂移 | A-005 | CRITICAL | 生成器失效，"伪生成文件"靠手维护，常量静默漂移 | **P1 本周** |
| 7 | 5 个 HTML 全缺 CSP meta 标签 | A-052 | CRITICAL | XSS 风险（admin + 用户昵称场景） | **P1 本周** |
| 8 | 告警引用不存在的指标（pgxpool_acquire_count / ws_active_connections） | A-070 | CRITICAL | 生产告警完全不生效 | **P1 本周** |
| 9 | .env.example JWT_SECRET 误导（应为 JWT_PRIVATE_KEY/JWT_PUBLIC_KEY） | A-076 | CRITICAL | 开发者按文档配置无法启动（ECDSA 需 PEM） | **P1 本周** |
| 10 | CI E2E 漏跑 5 个 critical spec（auth/admin/security/network_boundary/concurrency） | A-062, A-056 | REQUIRED | 认证/管理/安全场景无端到端守卫，回归风险高 | **P1 本周** |

### 6.1 次优先级风险（P2）

| 风险 | 涉及资产 | 严重级别 | 影响 |
|------|---------|---------|------|
| Makefile sync-alert-rules 无 target | A-074, A-065 | CRITICAL | 运维无法生成 alertmanager ConfigMap |
| CockroachDB 文档说已实现但代码未实现 | A-081 | CRITICAL | 误导运维进行不可行的迁移 |
| GDPR 断言 && 应为 \|\| | A-034 | REQUIRED | 合规验证形同虚设 |
| 全前端 fetch 无超时 | A-040, A-050 | REQUIRED | 网络挂起时 UI 永久卡死 |
| 镜像未 digest pin（balloon-game + compose + Actions） | A-063, A-069, A-073 | REQUIRED | 供应链劫持风险 |
| Redis securityContext 缺失 | A-069 | REQUIRED | 纵深防御缺口 |
| ADR 过时/矛盾（ADR-013/014/018/022/025） | A-077 | REQUIRED | 误导维护者与评审者 |

---

## 7. 盲区地图

### 7.1 测试覆盖盲区

| 盲区 | 涉及资产 | 风险 |
|------|---------|------|
| CI E2E 漏跑 5 个 critical spec | A-062, A-056 | auth/admin/security/network_boundary/concurrency 无端到端守卫 |
| idempotency.go SETNX claim 路径缺单元测试 | A-013 | 幂等性关键路径无测试（2 个 TODO） |
| 11 个 down.sql 回滚迁移零测试覆盖 | A-033 | 回滚迁移未验证，可能不可逆 |
| 整个 cmd/ 无测试 | A-032 | 代码生成器无测试守卫 |
| outbox 测试仅验证插入不验证消费/发布 | A-015, A-034 | outbox 消费/重试路径未验证 |
| 无 metrics/tracing 端点测试 | 全局 | 可观测性端点无烟雾测试 |
| DecodeNicknamePayload 未直接 fuzz | A-059 | 昵称解析边界未 fuzz |
| TestRateLimiter_ConcurrentRequests 设计偏弱 | A-034 | 并发限流边界未有效验证 |
| 前端 property test 静默吞错"合法化"解码缺陷 | A-060 | 真实解码缺陷被测试 catch 掩盖 |

### 7.2 文档盲区

| 盲区 | 涉及资产 | 风险 |
|------|---------|------|
| openapi.yaml 缺失 4 个已注册路由文档 | A-079 | API 消费者无法发现端点 |
| 3 份文档引用 /resolve 端点但代码未实现 | A-079 | 文档承诺不存在的端点 |
| ws-protocol.md 缺 SNAPSHOT/RESTART_STATUS 布局 | A-038 | 二进制协议无文档 |
| ADR-022 "RotateKey 未实现" 已过时 | A-077 | 误导评审者列为待修债务 |
| ADR-025 标题与内容矛盾（可变 vs 受控） | A-077 | 状态管理决策语义混乱 |
| CockroachDB 文档说已实现但代码未实现 | A-081 | 误导运维进行不可行迁移 |
| db-query-analysis.md 引用已删除索引 | A-083 | 性能分析文档失效 |
| architecture.md 房间码示例 6 位（应 5 位） | A-078 | 数据流图示例错误 |
| room_result_async 三写并行未文档化 | A-015 | 数据流设计意图不明 |

### 7.3 CI 盲区

| 盲区 | 涉及资产 | 风险 |
|------|---------|------|
| E2E performance.spec.ts 不存在 | A-062 | E2E job 必失败，阻塞所有 PR |
| 5 个 critical E2E spec 漏跑 | A-062 | 认证/管理/安全无端到端守卫 |
| Makefile sync-alert-rules 无 target | A-074 | 运维无法生成 alertmanager ConfigMap |
| security-scan 仅 npm/go CVE | A-063 | 缺 secret/SAST/容器定期重扫 |
| 无 CI 失败通知渠道 | A-061, A-062, A-063 | 失败仅体现在 workflow run 状态 |
| Terraform 无 CI validate | A-066 | IaC 变更无门禁 |
| 前端 PALETTE_COLORS/END_REASON 无 CI 校验 | A-005 | 常量漂移无守卫 |
| docs-governance ws-protocol-sync 仅校验 Msg* 前缀 | A-005 | PHASE_CODE/END_REASON 未校验 |

---

## 8. 趋势对比（v1 → v2）

### 8.1 健康度趋势

| 域 | v1 均分 | v2 均分 | Δ | 说明 |
|---|---------|---------|----|------|
| Critical 资产 | 4.42 | 3.93 | -0.49 | 新轴拉低均分（非回归） |
| High 资产 | — | 3.40 | — | v1 未提供 High 均分 |
| Medium/Low 资产 | — | 3.40 | — | v1 未提供 Med/Low 均分 |
| **整体** | **~4.4** | **3.54** | **-0.86** | 新轴暴露隐藏债务 |

**注**：v2 均分下降**非回归**，而是新轴（可观测性/供应链/文档一致性）暴露了 v1 未覆盖的隐藏债务。旧轴（正确性/可读性/架构/安全/性能）健康度仍良好（~4.3）。

### 8.2 发现数趋势

| 维度 | v1 | v2 新增 | v2 已修复 | v2 持续 |
|------|----|---------|----------|--------|
| CRITICAL | 7 | 26 (含 2 跨层) | 7 | 0 |
| REQUIRED | 20 | 126 | 18 | 1 (R-05 覆盖率) |
| **合计** | **27** | **152+** | **25** | **1** |

- **v1 27 项发现**：25 FIXED + 1 PARTIAL + 0 REGRESSION + 1 无法验证
- **v2 新增 152+ 项发现**：主要来自 5 个新轴视角（可观测性/可维护性/供应链/弹性/文档一致性）
- **0 REGRESSION**：v1 修复无代码回归

### 8.3 新轴 vs 旧轴得分对比

| 轴类别 | 平均得分 | 评价 |
|--------|---------|------|
| 旧轴（v1 已覆盖） | ~4.3 | 🟢 良好 |
| 新轴（v2 新增） | ~3.0 | 🔴 严重不足 |

**结论**：旧轴健康度良好证明 v1 修复有效；新轴健康度偏低证明 v2 增量审查的必要性 — 5 个新轴贡献了 26 项 CRITICAL 中的 21 项。

### 8.4 v1 修复成效

v1 的 7 项 CRITICAL 修复主题在 v2 审查中均无回归：

| v1 修复主题 | v2 状态 | 证据 |
|------------|---------|------|
| TRUSTED_PROXY 配置 | ✅ 无回归 | K8s env 注入链路完整 |
| admin lockout fail-closed | ✅ 无回归 | admin_login.go:64-70 503 |
| restore Redis 注册 | ✅ 无回归 | hub_redis_registry.go 完整 |
| cooldown roster 计数 | ✅ 无回归 | cooldown_contract_test.go 守卫 |
| outbound 异步化 | ✅ 无回归 | outbound_manager.go 队列+超时 |
| auth/refresh 链路 | ✅ 无回归 | refresh.go Lua 原子 + reuse 检测 |
| cosign verify | ✅ 无回归 | ci-cd.yml:179-185 |

v2 新发现（v2-R-02/04/05 等）属于 v1 修复后的**残留短板**或**新轴下的新发现**，非 v1 回归。

---

## 9. 发现清单索引

### 9.1 ID 编号方案

| 系列 | 区间 | 来源 |
|------|------|------|
| v2-C-01~C-05 | Task 1 Critical | 5 项（含 v2-C-04 v1 报告缺失，非误报） |
| v2-C-08~C-15 | Task 2 High | 8 项（C-08 subA, C-09~C-14 subC, C-15 subD） |
| v2-C-25, C-30~C-39 | Task 3 Med/Low | 11 项 |
| v2-C-40~C-41 | Task 5 交叉 | 2 项（threat-model admin JWT + E2E performance） |
| v2-R-01~R-35 | Task 1 Critical | 35 项（14 组） |
| v2-R-36~R-82 | Task 2 High | 47 项 |
| v2-R-83~R-142 | Task 3 Med/Low | 38 项（含间隔） |
| v2-R-143~R-148 | Task 5 交叉 | 6 项 |

**注**：F 系列（FYI）存在跨 Task 2/3 ID 冲突，详见各 Task 结果文件的修正映射表。最终报告引用发现时均标注来源 Task。

### 9.2 CRITICAL 发现全量清单（26 项）

| 发现 ID | 资产 | 轴 | 描述 | 来源 |
|---------|------|----|------|------|
| v2-C-01 | A-066 | 可维护性 | GKE 集群未在 Terraform 中定义 | Task 1 |
| v2-C-02 | A-066 | 文档一致性 | ADR-013 引用 ADR-028 错误（应为 ADR-014） | Task 1 |
| v2-C-03 | A-067 | 弹性 | HPA ws_connections 指标链路断裂（无 prometheus-adapter） | Task 1 |
| v2-C-04 | A-075 | 文档一致性 | v1 基线报告缺失（非误报，经 4 子代理确认） | Task 1 |
| v2-C-05 | A-066 | 文档一致性 | ADR-015 状态"提议中"但代码已实现 CRDB 切换 | Task 1 |
| v2-C-08 | A-005 | 文档一致性 | gen-frontend-constants 输出路径错误，前后端常量漂移 | Task 2 |
| v2-C-09 | A-077 | 文档一致性 | ADR-013 引用"ADR-028 GKE multi-region"错误 | Task 2 |
| v2-C-10 | A-077 | 文档一致性 | ADR-022 "RotateKey 未实现"已过时 | Task 2 |
| v2-C-11 | A-079 | 文档一致性 | openapi.yaml 房间码 6 位（实际 5 位） | Task 2 |
| v2-C-12 | A-079 | 文档一致性 | openapi.yaml match 标记 deprecated（实际已实现） | Task 2 |
| v2-C-13 | A-079 | 文档一致性 | openapi.yaml match 响应字段 code（实际 lobbyCode） | Task 2 |
| v2-C-14 | A-080 | 文档一致性 | threat-model.md JWT 算法 HMAC-SHA256（实际 ES256） | Task 2 |
| v2-C-15 | A-069 | 弹性 | HPA ws_connections 指标链路断裂（与 v2-C-03 同一问题） | Task 2 |
| v2-C-25 | A-052 | 安全 | 5 个 HTML 全缺 CSP meta 标签 | Task 3 |
| v2-C-30 | A-065 | 正确性 | docs-governance CI（实际为 make sync-alert-rules 必失败） | Task 3 |
| v2-C-31 | A-074 | 正确性 | Makefile sync-alert-rules 无 target | Task 3 |
| v2-C-32 | A-070 | 可观测性 | 告警引用不存在指标 pgxpool_acquire_count | Task 3 |
| v2-C-33 | A-071 | 正确性 | rules-configmap.yaml 不存在 | Task 3 |
| v2-C-34 | A-072 | 正确性 | Grafana datasource UID 不匹配 | Task 3 |
| v2-C-35 | A-076 | 文档一致性 | .env.example JWT_SECRET 应为 JWT_PRIVATE_KEY/JWT_PUBLIC_KEY | Task 3 |
| v2-C-36 | A-070 | 可观测性 | 告警引用 game_active_ws_connections（实际 ws_connections） | Task 3 |
| v2-C-37 | A-070 | 可观测性 | 告警引用 ws_active_connections（实际 ws_connections） | Task 3 |
| v2-C-38 | A-081 | 文档一致性 | CockroachDB 文档说已实现但代码未实现 | Task 3 |
| v2-C-39 | A-083 | 文档一致性 | db-query-analysis.md 引用已删除索引 | Task 3 |
| v2-C-40 | A-080 | 文档一致性 | threat-model.md Admin JWT 独立 ADMIN_JWT_SECRET（实际共享 ECDSA） | Task 5 |
| v2-C-41 | A-062 | 正确性 | CI E2E 矩阵含不存在的 performance.spec.ts | Task 5 |

### 9.3 跨资产重复主题

| 主题 | 涉及发现 | 影响 |
|------|---------|------|
| 房间码 5 vs 6 位 | v2-C-11, v2-R-66 | openapi 全文 + architecture 示例 vs domain 代码 + E2E |
| /api/v1/registry/match 状态 | v2-C-12, v2-R-53, v2-R-72 | openapi(deprecated) + threat-model(预留) vs 代码(已实现) + E2E(大量使用) |
| ADR-025 "可变单例 vs 受控状态" | v2-R-49, v2-R-60, v2-R-62, v2-R-65 | ADR-025 + README + ADR-018 + architecture.md 矛盾 |
| fetch 无超时 | v2-R-14, v2-R-46, v2-R-50 | 前端全量 fetch 调用无 AbortController |
| 镜像未 digest pin | v2-R-75, v2-R-77, v2-R-80 | CI Actions + K8s balloon-game + docker-compose 全未 pin |
| 前端无远程遥测 | v2-R-08 + 多个 OPTIONAL | 5 个前端资产均无 Sentry/sendBeacon |
| HPA 指标链路断裂 | v2-C-03, v2-C-15 | Task 1 与 Task 2 独立发现同一问题 |

---

## 10. v2 已知发现描述修正

Task 5 跨层交叉验证发现部分 v2 已知发现描述不准确，特此修正：

| 原发现 ID | 原描述 | 修正后描述 | 修正来源 |
|----------|--------|-----------|---------|
| v2-R-02 | OutboxRepository.InsertOutboxEvent 缺 retry.Do 包装 | 实际已包装 circuit breaker（r.cb.Execute），v2-R-02 描述基于早期版本 | Task 5 主题 3 |
| v2-C-30 | docs-governance CI 必失败（引用不存在 alertmanager rules-configmap） | docs-governance.yml 实际不引用 rules-configmap；实际失败的是 `make sync-alert-rules` 命令本身（Makefile 无 target） | Task 5 主题 4 |
| v2-C-04 | Task 1 标记为"误报，v1 报告实际存在" | 非误报，v1 报告确实缺失（经 Task 2 四子代理确认） | Task 2 汇总 |

---

## 11. 建议行动计划

### 11.1 P0 立即修复（阻断 CI / 安全评审）

1. **修复 CI E2E 矩阵**：移除 ci-cd.yml:55 的 `performance` 或创建 performance.spec.ts
2. **修正 openapi.yaml**：房间码 5 位 / match 移除 deprecated / 字段名 lobbyCode / 补全 4 个路由
3. **修正 threat-model.md**：JWT 算法 ES256 + Admin 密钥共享 ECDSA

### 11.2 P1 本周修复（生产风险）

4. **补 prometheus-adapter 配置**：恢复 HPA ws_connections 指标链路
5. **Terraform IaC 化 GKE 集群 + VPC**：或明确标注手动管理
6. **修复 gen-frontend-constants 路径**：shared/constants.ts → shared/game/constants.ts
7. **HTML 补 CSP meta 标签**：5 个 HTML 文件
8. **修正告警指标名**：pgxpool_acquire_count → db_pool_* / ws_active_connections → ws_connections
9. **修正 .env.example**：JWT_SECRET → JWT_PRIVATE_KEY/JWT_PUBLIC_KEY
10. **补全 CI E2E 矩阵**：补跑 auth/admin/security/network_boundary/concurrency

### 11.3 P2 本月修复（技术债务）

11. Makefile 补 sync-alert-rules target
12. CockroachDB 文档与代码对齐
13. GDPR 断言 && → ||
14. 前端 fetch 加 AbortController 超时
15. 镜像 digest pin（balloon-game + compose + Actions）
16. Redis StatefulSet 补 securityContext
17. ADR 过时/矛盾修正（ADR-013/014/018/022/025）
18. 后端 unit 覆盖率提升至 80%

---

## 12. 方法学局限

1. **v1 报告缺失**：无法逐项核对 v1 27 项发现的原始描述，回归验证基于间接证据
2. **纯静态审查**：未执行 go test/npm test/CI，修复实际行为未运行验证
3. **ID 冲突**：子代理并行导致 ID 冲突，已通过修正映射表解决，但 F 系列可能仍有遗漏
4. **新轴无 v1 基线**：5 个新轴无 v1 对比，趋势对比仅基于旧轴
5. **REQUIRED 项候选构造**：v1 报告缺失导致 20 项 REQUIRED 基于推断，可能与 v1 原始条目存在语义偏差

---

## 附录 A: 资产清单修正记录

| 资产 ID | v1 状态 | v2 修正 | 修正原因 |
|---------|--------|---------|---------|
| A-005 | Low | **High** | 跨前后端协议关键性 |
| A-032 | Low | **Medium** | 代码生成一致性 |
| A-045 | "目录不存在，已移除" | **存在**，路径 `frontend/src/shared/game/` | v1 拼写错误（shard vs shared） |
| A-044~A-048 | shard/* | shared/* | v1 路径命名错误 |

**v2 实际审查资产数：83 个**（v1 误报为 82）

---

## 附录 B: 固化文件清单

| 文件路径 | 内容 | 状态 |
|---------|------|------|
| .trae/specs/evolve-self-inspection-v2/spec.md | v2 自检计划规范 | 已批准 |
| .trae/specs/evolve-self-inspection-v2/tasks.md | 任务执行清单 | Task 0-6 完成 |
| .trae/specs/evolve-self-inspection-v2/checklist.md | 验证检查清单 | Task 7 待执行 |
| docs/superpowers/reports/v2-asset-inventory.md | 资产清单 + 模板 + 新轴 checklist | 已固化 |
| docs/superpowers/reports/v2-task1-results.md | 22 Critical 资产审查 | 已固化 |
| docs/superpowers/reports/v2-task2-results.md | 25 High 资产审查 | 已固化 |
| docs/superpowers/reports/v2-task3-results.md | 36 Med/Low 资产审查 | 已固化 |
| docs/superpowers/reports/v2-task4-regression.md | 27 项 v1 回归验证 | 已固化 |
| docs/superpowers/reports/v2-task5-cross-validation.md | 5 主题交叉验证 | 已固化 |
| docs/superpowers/reports/v2-task2-subA~D-*.md | Task 2 子代理原始产出 | 已固化 |
| docs/superpowers/reports/v2-task3-subA~D-*.md | Task 3 子代理原始产出 | 已固化 |
| docs/superpowers/reports/2026-07-08-self-inspection-v2-report.md | **本综合报告** | 已固化 |

---

*报告结束*
