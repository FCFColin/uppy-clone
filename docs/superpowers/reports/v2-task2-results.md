# Task 2 审查结果汇总（High 资产，25 个）

> 生成日期：2026-07-08
> 4 个子代理并行产出，本文件为主代理汇总固化
> 子代理原始产出：v2-task2-subA-backend-high.md / subB-frontend-high.md / subC-test-docs-high.md / subD-infra-high.md
> 审查性质：纯诊断，未修改任何业务代码

---

## 0. ID 冲突修正说明

子代理并行审查导致部分发现 ID 冲突。本汇总采用以下修正映射，后续报告统一使用修正后 ID：

| 原始 ID（子代理文件） | 修正后 ID | 来源 | 说明 |
|----------------------|----------|------|------|
| subB v2-O-30~O-34 | v2-O-35~O-39 | 前端 | 与 subA O-30~34 冲突 |
| subB v2-O-35~O-41 | v2-O-40~O-46 | 前端 | 顺延 |
| subB v2-F-25~F-28 | v2-F-29~F-32 | 前端 | 与 subA F-25~28 冲突 |
| subB v2-F-29~F-32 | v2-F-33~F-36 | 前端 | 顺延 |
| subC v2-O-35~O-48 | v2-O-47~O-60 | 测试文档 | 与 subB 修正后冲突 |
| subC v2-F-30~F-37 | v2-F-37~F-44 | 测试文档 | 与 subB 修正后冲突 |
| subD v2-C-15 | **v2-C-16** | 基础设施 | 与 subC C-15 冲突 |
| subD v2-R-60~R-67 | **v2-R-75~R-82** | 基础设施 | 与 subC R-60~R-67 冲突 |
| subD v2-O-45~O-52 | **v2-O-61~O-68** | 基础设施 | 与 subC 修正后冲突 |

**统一后 ID 区间**：
- CRITICAL: v2-C-08（A）+ v2-C-09~C-14（C，原 C-10~C-15）+ v2-C-15（D，原 C-15）
- REQUIRED: v2-R-36~R-43（A）+ v2-R-44~R-51（B）+ v2-R-52~R-74（C）+ v2-R-75~R-82（D）
- OPTIONAL: v2-O-25~O-34（A）+ v2-O-35~O-46（B）+ v2-O-47~O-60（C）+ v2-O-61~O-68（D）
- FYI: v2-F-22~F-28（A）+ v2-F-29~F-36（B）+ v2-F-37~F-44（C）+ v2-F-45~F-52（D）

---

## 1. 评分矩阵（25 资产）

### 后端 High（8 资产）— 子代理 A

| 资产 ID | 资产名 | v2 整体 | 状态 | 关键问题 |
|---------|--------|--------|------|---------|
| A-002 | audit | 3.3 | 🟡 | loadLastHash 无超时 + writeToDB 丢弃记录 |
| A-004 | config | 3.8 | 🟢 | getDurationEnv 与 GetEnvDuration 行为不一致 |
| A-005 | constants | 2.5 | 🔴 | 生成器输出路径错误（前后端常量漂移）+ 重复定义 |
| A-015 | outbox | 3.5 | 🟢 | at-least-once 语义未文档化 |
| A-017 | rbac | 3.7 | 🟢 | 审计 ActorID 用 role 而非用户 ID |
| A-019 | resilience | 4.3 | 🟢 | 弹性实现最完善 |
| A-025 | worker | 3.3 | 🟡 | email worker 消费者 ID 硬编码 + 无退避 |
| A-028 | cmd/server | 5.0 | 🟢 | 极简入口无问题 |

### 前端 High（5 资产）— 子代理 B

| 资产 ID | 资产名 | v2 整体 | 状态 | 关键问题 |
|---------|--------|--------|------|---------|
| A-036 | UI 层 | 3.8 | 🟡 | 无远程遥测 + DOM 引用风格不一 |
| A-040 | 输入与同步 | 3.6 | 🟡 | console.log 残留 + fetch 无超时 + 状态分散 |
| A-041 | 匹配与房间 | 3.8 | 🟡 | 无远程遥测 + 匹配失败无重试 |
| A-045 | shared/game | 3.4 | 🟡 | 死代码 + ADR-025 与代码不一致 |
| A-050 | 页面入口 | 3.4 | 🟡 | fetch 无超时 + 重复实现房间校验 |

### 测试 + 文档 High（8 资产）— 子代理 C

| 资产 ID | 资产名 | v2 整体 | 状态 | 关键问题 |
|---------|--------|--------|------|---------|
| A-056 | E2E 测试 | 3.4 | 🟡 | waitForTimeout flaky + match 端点文档矛盾 |
| A-057 | E2E helpers | 4.0 | 🟢 | lobbyCode vs code 字段名不一致 |
| A-058 | 后端 property | 4.0 | 🟢 | rapid fail 文件已提交 |
| A-060 | 前端 property | 3.4 | 🟡 | 静默吞错"合法化"解码缺陷 |
| A-077 | ADR | 3.0 | 🟡 | ADR-013 引用错误 + ADR-022 过时 + ADR-025 矛盾 |
| A-078 | 架构文档 | 3.7 | 🟡 | "可变单例"引用过时 + 房间码示例 6 位 |
| A-079 | API 文档 | 2.3 | 🔴 | 房间码长度错 + match 端点 deprecated 错 + 字段名错 + JWT 算法错 |
| A-080 | 安全文档 | 3.3 | 🟡 | JWT 算法错 + match 限流"配置预留"错 |

### 基础设施 High（4 资产）— 子代理 D

| 资产 ID | 资产名 | v2 整体 | 状态 | 关键问题 |
|---------|--------|--------|------|---------|
| A-063 | security-scan CI | 3.6 | 🟡 | Actions 未 digest pin + 扫描覆盖窄 |
| A-069 | K8s global | 3.0 | 🔴 | HPA 指标链路断裂 + 镜像未 digest pin + Redis 缺 securityContext |
| A-073 | Docker | 3.5 | 🟡 | docker-compose 7 镜像未 digest pin + name 残留 |
| A-034 | 集成测试 | 3.6 | 🟡 | GDPR 断言逻辑错误（&&应为||）|

**High 资产域均分**: 3.4/5（25 资产）

---

## 2. 发现汇总（按严重级别）

### CRITICAL（8 项）

| 发现 ID | 资产 | 轴 | 描述 | 位置 | 子代理 |
|---------|------|----|------|------|--------|
| v2-C-08 | A-005 | 文档一致性 | gen-frontend-constants 输出路径 `shared/constants.ts` 与前端实际使用 `shared/game/constants.ts` 不一致，前后端常量静默漂移 | gen-frontend-constants/main.go:20 | A |
| v2-C-09 | A-077 | 文档一致性 | ADR-013 引用"ADR-028 (GKE multi-region)"错误，ADR-028 实际是 Clean Architecture | docs/adr/013:4 | C |
| v2-C-10 | A-077 | 文档一致性 | ADR-022 声明"RotateKey 未实现"已过时，代码 aes.go:212-219 已完整实现 | docs/adr/022:39 | C |
| v2-C-11 | A-079 | 文档一致性 | openapi.yaml 全文房间码长度 6 位，实际代码强制 5 位 | openapi.yaml:461,522-526 vs room_code.go:11 | C |
| v2-C-12 | A-079 | 文档一致性 | openapi.yaml 将 /api/v1/registry/match 标记 deprecated/未实现，但端点完整实现且 E2E 大量使用 | openapi.yaml:482-510 vs routes_public.go:116-120 | C |
| v2-C-13 | A-079 | 文档一致性 | openapi.yaml match 响应字段 `code` 与实际 `lobbyCode` 不一致 | openapi.yaml:502 vs lobby_registry.go:204 | C |
| v2-C-14 | A-080 | 文档一致性 | threat-model.md 声明 JWT 用 HMAC-SHA256，实际用 ES256（ECDSA P-256）| threat-model.md:11 vs jwt.go:78 | C |
| v2-C-15 | A-069 | 弹性 | HPA 依赖 ws_connections 自定义指标，但无 prometheus-adapter 配置，WS 自动伸缩失效 | hpa.yaml:31-38 | D |

### REQUIRED（47 项）

| 发现 ID | 资产 | 轴 | 描述 | 子代理 |
|---------|------|----|------|--------|
| v2-R-36 | A-002 | 弹性 | loadLastHash 无超时，DB 不可达阻塞启动 | A |
| v2-R-37 | A-002 | 文档一致性 | writeToDB 失败丢弃记录与注释承诺不符 | A |
| v2-R-38 | A-004 | 可维护性 | getDurationEnv 与 GetEnvDuration 行为不一致 | A |
| v2-R-39 | A-005 | 可维护性 | constants/protocol.go 与 protocol/constants.go alias 重复 | A |
| v2-R-40 | A-005 | 可维护性 | MaxNicknameLen 在 domain 和 config 重复定义 | A |
| v2-R-41 | A-015 | 弹性 | at-least-once 语义未文档化 | A |
| v2-R-42 | A-025 | 弹性 | XReadGroup 错误后 time.Sleep 无指数退避 | A |
| v2-R-43 | A-025 | 可维护性 | email worker 消费者 ID 硬编码 | A |
| v2-R-44 | A-036 | 可维护性 | _savedNickname 缓存与 invalidate 耦合 | B |
| v2-R-45 | A-040 | 可观测性 | ws_handlers_phase.ts:13 残留 console.log | B |
| v2-R-46 | A-040 | 弹性 | 全前端无 AbortController，fetch 无超时 | B |
| v2-R-47 | A-040 | 可维护性 | ws_connection.ts 14+ 模块级 setter 状态分散 | B |
| v2-R-48 | A-045 | 可维护性 | phaseFromCode/phaseToCode 死代码 | B |
| v2-R-49 | A-045 | 文档一致性 | ADR-025 与代码多处不一致 | B |
| v2-R-50 | A-050 | 弹性 | 页面入口所有 fetch 无超时 | B |
| v2-R-51 | A-050 | 可维护性 | index.ts 与 room_validate.ts 重复实现房间校验 | B |
| v2-R-52 | A-056 | 可维护性 | E2E 普遍 waitForTimeout flaky 风险 | C |
| v2-R-53 | A-056 | 文档一致性 | E2E 依赖 match 端点但 openapi 标记 deprecated | C |
| v2-R-54 | A-056 | 正确性 | error_handling 断言过弱（ended||playing 二选一）| C |
| v2-R-55 | A-057 | 文档一致性 | lobbyCode vs code 字段名不一致 | C |
| v2-R-56 | A-058 | 可维护性 | rapid fail 文件已提交到仓库 | C |
| v2-R-57 | A-060 | 可维护性 | 静默吞错"合法化"解码缺陷（≥6 处 try-catch）| C |
| v2-R-58 | A-077 | 文档一致性 | ADR-014 README 状态与文件不一致 | C |
| v2-R-59 | A-077 | 文档一致性 | ADR-013 废弃但被"提议中"ADR-016 取代，逻辑矛盾 | C |
| v2-R-60 | A-077 | 文档一致性 | ADR-025 README 标题与文件标题语义相反 | C |
| v2-R-61 | A-077 | 文档一致性 | ADR-022 行号引用过时 | C |
| v2-R-62 | A-077 | 文档一致性 | ADR-018 "可变单例"未标注被 ADR-025 取代 + Zustang 拼写错 | C |
| v2-R-63 | A-077 | 文档一致性 | ADR-018 "仅 2 个 vitest 文件"已过时 | C |
| v2-R-64 | A-077 | 文档一致性 | ADR-022 main.go:113 行号需核对 | C |
| v2-R-65 | A-078 | 文档一致性 | architecture.md "可变单例"引用过时 | C |
| v2-R-66 | A-078 | 文档一致性 | architecture.md 房间码示例 6 位（应 5 位）| C |
| v2-R-67 | A-079 | 文档一致性 | openapi 缺失 4 个已注册路由文档 | C |
| v2-R-68 | A-079 | 文档一致性 | 3 份文档引用 /resolve 端点但代码未实现 | C |
| v2-R-69 | A-079 | 文档一致性 | openapi match 描述"与 create 相同"错误 | C |
| v2-R-70 | A-079 | 文档一致性 | openapi_consistency_test 覆盖极薄 | C |
| v2-R-71 | A-079 | 正确性 | openapi_consistency_test 引用未声明变量 | C |
| v2-R-72 | A-080 | 文档一致性 | threat-model match 限流"配置预留"与代码不符 | C |
| v2-R-73 | A-080 | 文档一致性 | threat-model 未提及 AES-256-GCM 字段级加密 | C |
| v2-R-74 | A-080 | 文档一致性 | self-check 引用未文档化的 POST /verify | C |
| v2-R-75 | A-063 | 供应链 | GitHub Actions 未 digest pin | D |
| v2-R-76 | A-063 | 安全 | 日扫描仅 npm/go CVE，缺 secret/SAST/容器定期重扫 | D |
| v2-R-77 | A-069 | 供应链 | balloon-game 镜像未 digest pin（占位符 + sed）| D |
| v2-R-78 | A-069 | 安全 | 两个 Redis StatefulSet 缺 securityContext | D |
| v2-R-79 | A-069 | 安全 | namespace 未声明 PSS restricted 标签 | D |
| v2-R-80 | A-073 | 供应链 | docker-compose 7 镜像未 digest pin | D |
| v2-R-81 | A-073 | 可维护性 | docker-compose name: uppy-clone 模板残留 | D |
| v2-R-82 | A-034 | 正确性 | GDPR 断言 && 应为 ||，验证形同虚设 | D |

### OPTIONAL / FYI

详见各子代理原始产出文件。统计：
- OPTIONAL: 44 项（A:10 + B:12 + C:14 + D:8）
- FYI: 31 项（A:7 + B:8 + C:8 + D:8）

---

## 3. 5 个新轴域级分析（High 资产）

| 轴 | High 资产均分 | Top 问题资产 | 典型模式 |
|---|---|---|---|
| 可观测性 | 2.5 | A-050(2), A-036(2), A-040(2), A-041(2), A-025(2) | 前端无远程遥测；worker 缺处理指标；E2E 无可观测性覆盖 |
| 可维护性 | 3.5 | A-005(2), A-077(3), A-034(3) | 常量重复定义；ADR 过时/矛盾；测试断言错误 |
| 供应链 | 2.5 | A-063(2), A-069(2), A-073(3) | Actions 未 digest pin；镜像占位符；compose 全未 pin |
| 弹性 | 2.8 | A-002(2), A-069(弹性部分), A-050(2) | fetch 无超时；HPA 链路断裂；audit 无超时 |
| 文档一致性 | 2.0 | A-079(1), A-077(2), A-005(2) | openapi 多处错误；ADR 过时/矛盾；生成器路径错 |

---

## 4. v1 回归检查汇总

- **v1 基线报告 `2026-07-07-full-self-inspection-report.md` 经 4 个子代理确认不存在**（reports/ 仅含 v2-asset-inventory.md + v2-task1-results.md）
- **v2-C-04（Task 1 中标记为"误报"）实际为真**：v1 报告确实缺失，Task 1 结果中 v2-task1-results.md:99 的声明有误
- 影响：所有 v1 回归检查只能基于任务描述中的 v1 数值，无法逐项核对 v1 的 27 项关键发现
- Task 4 回归验证需调整策略：基于代码现状评估，而非对比 v1 报告

---

## 5. 跨资产重复主题（交叉发现预览）

| 主题 | 涉及发现 | 影响 |
|------|---------|------|
| 房间码 5 vs 6 位 | v2-C-11, v2-R-66 | openapi 全文 + architecture 示例 vs domain 代码 + E2E |
| /api/v1/registry/match 状态 | v2-C-12, v2-R-53, v2-R-72 | openapi(deprecated) + threat-model(预留) vs 代码(已实现) + E2E(大量使用) |
| ADR-025 "可变单例 vs 受控状态" | v2-R-49, v2-R-60, v2-R-62, v2-R-65 | ADR-025 自身 + README + ADR-018 + architecture.md 矛盾 |
| fetch 无超时 | v2-R-46, v2-R-50 | 前端全量 fetch 调用无 AbortController |
| 镜像未 digest pin | v2-R-75, v2-R-77, v2-R-80 | CI Actions + K8s balloon-game + docker-compose 全未 pin |
| 前端无远程遥测 | v2-O-35(前端), v2-O-40, v2-O-46 等 | 5 个前端资产均无 Sentry/sendBeacon |

---

## 6. 风险排名（Top 5）

| 排名 | 风险 | 资产 | 影响 |
|------|------|------|------|
| 1 | openapi.yaml 多处 CRITICAL 不一致（房间码/match/字段名/JWT）| A-079 | API 契约文档完全不可信，客户端集成会失败 |
| 2 | gen-frontend-constants 输出路径错误 | A-005 | 前后端物理常量静默漂移 |
| 3 | HPA ws_connections 指标链路断裂 | A-069 | WS 压力下无法自动扩容 |
| 4 | GDPR 断言 && 应为 || | A-034 | 合规验证形同虚设 |
| 5 | 全前端 fetch 无超时 | A-040/A-050 | 网络挂起时 UI 永久卡死 |
