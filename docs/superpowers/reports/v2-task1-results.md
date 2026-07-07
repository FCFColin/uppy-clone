# Task 1 审查结果汇总（Critical 资产，22 个）

> 生成日期：2026-07-08
> 4 个子代理并行产出，本文件为主代理汇总固化

---

## 1. 评分矩阵

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

**Critical 资产域均分**: v1=4.42 → v2=3.93 (Δ-0.49)

---

## 2. 发现汇总（按严重级别）

### CRITICAL（5 项，含 1 项误报）

| 发现 ID | 资产 | 轴 | 描述 | 位置 | 子代理 |
|---------|------|----|------|------|--------|
| v2-C-01 | A-066 | 可维护性 | GKE 集群未在 Terraform 中定义，ci-cd.yml 部署到手动创建的集群 | main.tf（缺失） | D |
| v2-C-02 | A-066 | 文档一致性 | ADR-013 引用 "ADR-028 (GKE multi-region)" 错误，多区域是 ADR-014 | docs/adr/013:4 | D |
| v2-C-03 | A-067 | 弹性 | HPA 依赖自定义指标 ws_connections，但全仓库无 prometheus-adapter 配置 | hpa.yaml:31-38 | D |
| v2-C-04 | A-075 | 文档一致性 | ~~v1 基线报告缺失~~ **误报**：v1 报告实际存在于 docs/superpowers/reports/2026-07-07-full-self-inspection-report.md（主代理已验证） | — | D |
| v2-C-05 | A-066 | 文档一致性 | ADR-015 状态"提议中"但代码已实现 CRDB 切换，状态过时 | docs/adr/015:5 | D |

### REQUIRED（14 项）

| 发现 ID | 资产 | 轴 | 描述 | 位置 | 子代理 |
|---------|------|----|------|------|--------|
| v2-R-01 | A-013 | 可维护性 | idempotency.go SETNX claim 路径缺单元测试（2 个 TODO） | idempotency.go:74,139 | A |
| v2-R-02 | A-022 | 弹性 | OutboxRepository.InsertOutboxEvent 缺 retry.Do 包装 | outbox_repository.go:22-32 | B |
| v2-R-03 | A-022 | 可观测性 | store 包零 slog 调用，持久化层错误无日志上下文 | 全包 | B |
| v2-R-04 | A-022 | 弹性 | 登录锁检查绕过 circuit breaker | redis_auth_session.go:69-137 | B |
| v2-R-05 | A-022 | 弹性 | CheckRateLimit fail-closed 与 ADR-029 矛盾 | redis_ratelimit.go:51-54 | B |
| v2-R-06 | A-033 | 可维护性 | 11 个 down.sql 回滚迁移零测试覆盖 | 全目录 | B |
| v2-R-07 | A-035 | 可维护性 | EntryStep 类型重复定义 | entry_flow_types.ts:3 / state_types.ts:74 | C |
| v2-R-08 | A-035 | 可观测性 | 前端无远程可观测性（无 Sentry/web-vitals/sendBeacon） | window_events.ts:72-80 | C |
| v2-R-09 | A-038 | 文档一致性 | ws-protocol.md 未记录 SNAPSHOT(0x01) 二进制布局 | docs/api/ws-protocol.md | C |
| v2-R-10 | A-038 | 文档一致性 | ws-protocol.md 未记录 RESTART_STATUS(0x07) 布局 | docs/api/ws-protocol.md | C |
| v2-R-11 | A-038 | 文档一致性 | GAME_STATE_CHANGE 未记录 endReason 字段 | docs/api/ws-protocol.md:22-32 | C |
| v2-R-12 | A-038 | 弹性 | 跨区域 421/503 流程文档有但前端未实现 | ws_connect.ts:114 | C |
| v2-R-13 | A-039 | 可维护性 | state_types.ts 导出可变单例 + store.ts 原地 Object.assign 破坏不可变语义 | state_types.ts:76 / store.ts:5-10 | C |
| v2-R-14 | A-044 | 弹性 | 所有 fetch 调用无超时（无 AbortController） | fetch.ts:10 / auth.ts:28,57,75 | C |
| v2-R-15 | A-061 | 供应链 | govulncheck/golang-migrate/go-licenses 使用 @latest 安装 | go-ci.yml:105,191,244 | D |
| v2-R-16 | A-061 | 供应链 | migration job 的 postgres:16 未 digest pin | go-ci.yml:172 | D |
| v2-R-17 | A-061 | 可观测性 | 无 CI 失败通知渠道 | go-ci.yml | D |
| v2-R-18 | A-062 | 弹性 | sed 占位符替换脆弱（自承认 fragile） | ci-cd.yml:200-207 | D |
| v2-R-19 | A-062 | 弹性 | 跨 workflow 依赖无显式触发器 | ci-cd.yml:165-177 | D |
| v2-R-20 | A-062 | 供应链 | e2e services 镜像未 digest pin | ci-cd.yml:58,71 | D |
| v2-R-21~27 | A-066 | 多轴 | Terraform 多项：无 CI validate、无 required_version、单区域、无告警 IaC、ADR-015 过时 | main.tf | D |
| v2-R-28~31 | A-067 | 文档一致性 | runbook namespace/资源类型/指标名不一致（3 处） | runbook.md | D |
| v2-R-32~33 | A-068 | 供应链 | overlays images 字段用 sed 占位符而非 Kustomize digest pinning | overlays/*/kustomization.yaml | D |
| v2-R-34~35 | A-075 | 供应链/可维护性 | baseline 需重新生成；Windows 反斜杠路径 | .secrets.baseline | D |

### OPTIONAL / FYI（略，详见各子代理原始产出）

---

## 3. 5 个新轴域级分析

| 轴 | Critical 资产均分 | Top 问题资产 | 典型模式 |
|---|---|---|---|
| 可观测性 | 3.0 | A-033(2), A-066(2), A-035/037/039/042/044(2), A-075(2) | 前端无远程遥测；迁移/ Terraform 无监控 IaC；store 零 slog |
| 可维护性 | 3.9 | A-066(2), A-013(3), A-039(3), A-068(3), A-075(3) | idempotency 测试缺失；可变单例；GKE 未 IaC 化；stub 文件堆积 |
| 供应链 | 3.5 | A-066(2), A-068(2), A-020(3), A-022(3), A-033(3), A-075(3) | @latest 工具安装；镜像未 digest pin；Terraform 无 CI validate |
| 弹性 | 3.7 | A-066(2), A-033(3), A-067(3), A-068(3), A-075(3) | sed 占位符脆弱；Redis 单副本；HPA 指标链路断裂；fetch 无超时 |
| 文档一致性 | 3.9 | A-042(2), A-066(2), A-067(3), A-068(3), A-075(3) | ws-protocol.md 缺 SNAPSHOT 布局；runbook 与代码不一致；ADR 引用错误 |

---

## 4. v1 回归检查汇总

- **22 个 Critical 资产均无代码回归**
- 子代理 D 误报 v1 报告缺失（v2-C-04），主代理确认报告存在于 `docs/superpowers/reports/2026-07-07-full-self-inspection-report.md`
- 部分资产（A-003/A-008/A-009）相比 v1 有增强（Refresh Token reuse 检测、Circuit Breaker、降级栈）
- A-022 store 的 v1 弹性/可观测性短板部分仍存在（v2-R-02~05），未回归但也未完全修复

---

## 5. 风险排名（Top 5）

| 排名 | 风险 | 资产 | 影响 |
|------|------|------|------|
| 1 | Terraform 未定义 GKE 集群 + 多区域缺失 | A-066 | 违反 IaC 原则；灾备切换不可恢复 |
| 2 | HPA ws_connections 指标链路断裂 | A-067 | WS 压力下无法自动扩容 |
| 3 | CheckRateLimit fail-closed 与 ADR 矛盾 | A-022 | Redis 故障将拒绝全站请求 |
| 4 | store 包零 slog 调用 | A-022 | 生产环境持久化层异常难以日志追踪 |
| 5 | 前端无远程可观测性 | A-035~044 | 生产环境无法定位客户端故障 |
