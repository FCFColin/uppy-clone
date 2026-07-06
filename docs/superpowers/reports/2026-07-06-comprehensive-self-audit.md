# 全方面代码自检报告

> **生成日期**：2026-07-06
> **范围**：12 维度全方面自检（调研型，不修改代码）
> **视角**：大角度战略判断，非逐文件检查
> **定位**：与既有《架构大方向体检报告》独立并存，采用全新维度划分

---

## 全局评分仪表盘

| # | 维度 | 评分 | 一句话判断 |
|---|------|------|-----------|
| 1 | 架构哲学与设计连贯性 | 🟢 领先 | 28 份 ADR 构成自洽叙事，有纠偏机制，章程防止"过度工程"误判 |
| 2 | 技术栈战略与依赖生态 | 🟢 领先 | 选型精准匹配目标，依赖健康，两个战略赌注（CRDB/多区域）尚未兑现 |
| 3 | 数据架构与一致性模型 | 🟡 需关注 | PG 层设计扎实，Redis 承载 10 域存在职责过载和 O(N) 命令风险 |
| 4 | 安全防御纵深与威胁面 | 🟢 领先 | 六层纵深 + STRIDE + 供应链安全，密钥轮换是唯一明显缺口 |
| 5 | 性能边界与扩展性天花板 | 🟡 需关注 | 理论分析充分，但压测数据表为空——扩展能力是"设计假设"非"已验证事实" |
| 6 | 容错设计与韧性评估 | 🟢 领先 | 熔断/隔板/降级/租约/Outbox 全覆盖，但混沌实验尚未实际执行 |
| 7 | 可观测性成熟度 | 🟢 领先 | 三支柱 + 持续 Profiling + Burn Rate 告警，远超项目规模所需（刻意为之） |
| 8 | 工程质量与开发体验 | 🟢 领先 | CI 门禁严苛、文档完备、约定一致，Windows 开发体验有轻微摩擦 |
| 9 | 测试策略与质量信心 | 🟡 需关注 | 金字塔结构好，100% 单元目标激进，前端覆盖有结构性排除 |
| 10 | 交付流水线与运维成熟度 | 🟢 领先 | 14+ CI 作业、多区域 GKE 部署、cosign 签名、SBOM，运维文档完整 |
| 11 | 知识管理与决策可追溯性 | 🟢 领先 | ADR 体系健康，CI 校验索引一致性，文档结构清晰 |
| 12 | 战略风险与盲区扫描 | 🔴 高风险 | 文档成熟度远超实际运行状态，"纸面企业级"与"单区域实际"之间的鸿沟是最大风险 |

**整体判断**：这是一个**设计意图极其清晰、工程实践远超典型学习项目**的代码库。核心风险不在"做得不够好"，而在"文档描述的成熟度"与"实际验证的状态"之间存在系统性鸿沟。项目的学习目标决定了这是可接受的，但需要明确认知。

---

## 维度 1：架构哲学与设计连贯性

### 核心问题
- 28 份 ADR 是否构成一个自洽的故事？
- 设计哲学是否在演进中发生了漂移？
- 代码是否忠实于设计意图？

### 总体判断

🟢 **领先** — 这是我见过的学习项目中架构叙事最连贯的一个。

### 关键发现

**1. 章程驱动的自洽叙事**

ADR-000 项目章程是一个精心设计的"防误判"机制。它明确声明：游戏是载体，企业级架构才是目的。并附带"刻意保留清单"和"自检指引"，防止评审者将刻意的复杂度误判为过度工程。这个机制极为有效——它在所有后续 ADR 中被反复引用，形成了统一的叙事基线。

**2. 演进中的自我纠偏能力**

项目展示了罕见的"敢于否定自己"的能力：
- ADR-024 果断删除了未接线的 CQRS Service 层脚手架（~200 行死代码），选择"拥抱当前实际架构"而非维持纸面设计
- ADR-005 从"一致性哈希 + Pub/Sub 广播"演变为"owner 反向代理 + 租约接管"，并明确解释了放弃旧方案的原因（"两套机制并存属冗余"）
- ADR-026 从 Casbin 收敛到轻量 RBAC 策略表

这些纠偏不是设计失败，而是设计成熟度的证明。

**3. 诚实的目标态 vs 已实现态区分**

`architecture.md` 开篇即声明"目标架构（部分已落地）"，并逐项列出已实现 vs 提议中。ADR 状态标注（已接受/提议中/部分落地）与代码实际状态一致。这在文档管理上是高水准的。

**4. ADR 间的引用网络形成闭环**

ADR-005 → ADR-013 → ADR-014 → ADR-015 → ADR-016 形成清晰的多区域演进链；ADR-009 → ADR-007 → ADR-010 形成异步处理链；ADR-017 → ADR-024 → ADR-028 形成分层架构演进链。决策不是孤立的，而是有因果关联的。

### 风险

- ADR-013（Cloud Run → GKE）的平台迁移在文档中已完成叙述，但实际 K8s 部署从未在真实集群上验证过。文档的自信语气可能掩盖了这一事实。

---

## 维度 2：技术栈战略与依赖生态

### 核心问题
- 核心技术选型是否仍在正确轨道上？
- 依赖生态是否健康可持续？
- 有无战略级技术赌注未兑现？

### 总体判断

🟢 **领先** — 技术选型精准匹配项目目标，依赖版本健康，两个战略赌注尚在途中。

### 关键发现

**1. 后端技术栈：Go 1.26 + pgx + chi + gorilla/websocket**

- Go 1.26 是极度前沿的选择（截至调研日，这是最新稳定版），表明项目有意实践最新语言特性
- pgx v5 + 手写 SQL（ADR-019）的决策在当前表规模下完全正确，ORM 会增加复杂度而非减少
- chi v5 是轻量路由的合理选择，但中间件栈存在冗余（`chi.Logger`/`chi.Recoverer` 与自定义中间件重叠）
- gorilla/websocket 是 Go 生态最成熟的 WS 库，选择无争议

**2. 前端技术栈：Vanilla TS + Vite + 零生产依赖**

ADR-018 的决策仍然正确。Canvas 游戏 + 二进制 WebSocket 场景下，引入框架（React/Svelte/Solid）会带来 runtime 开销却无实质收益。零生产依赖意味着零供应链风险，这在安全维度是巨大优势。Vitest + fast-check 提供了现代测试能力。

**3. 两个战略级技术赌注**

| 赌注 | 状态 | 风险评估 |
|------|------|---------|
| CockroachDB 多区域强一致 | 提议中，从未运行 | 🟡 代码有 `DB_DIALECT=cockroach` 切换路径，但从未在任何环境验证过。CockroachDB 的 SQL 方言差异、事务语义差异、性能特征都是未知数 |
| 多区域 GKE 拓扑 | 提议中，K8s 清单存在 | 🟡 三个区域 overlay（us-east1/europe-west1/asia-southeast1）存在，但从未部署到真实集群。跨区域 mTLS、Thanos 联邦、CRDB range placement 均未验证 |

**4. 依赖健康度**

- Go 依赖：pgx v5.10、redis v9.21、otel v1.44 均为最新主版本
- golangci-lint v1.64.8 启用了极其丰富的 linter 集合（50+ linter）
- 前端：eslint v10、vitest v4、typescript v5.6 均为最新
- Docker 镜像全部 pin 到 digest（SLSA L2）
- 无 GPL/AGPL 依赖（CI 强制检查）

**5. Redis 作为"万能锤"的战略隐忧**

Redis 同时承担了 10 个功能域（Pub/Sub、房间注册表、流、JWT 撤销、限流、大厅缓存、魔法链接等）。这在学习项目中是可接受的——它练习了 Redis 的各种用法。但从技术栈战略角度，这形成了对单一组件的过度依赖，且不同域的延迟敏感度和可靠性要求差异巨大。

---

## 维度 3：数据架构与一致性模型

### 核心问题
- 数据如何在 PG/Redis/CockroachDB 间分区？
- 一致性保障是否可靠？
- 数据模型是否面向未来？

### 总体判断

🟡 **需关注** — PostgreSQL 层设计扎实且成熟，Redis 层存在职责过载和阻塞命令风险。

### 关键发现

**1. PostgreSQL：扎实的主数据存储**

- 11 个版本化迁移，覆盖 init → 索引 → FK 级联 → 软删除 → 审计日志 → Outbox → DB 角色 → email_hash → Outbox 保留索引
- 迁移有 up/down 对，CI 验证幂等性（up → down → up）
- DB 角色分离（app_user / migrator）实现最小权限
- 软删除 + 30 天硬删除的 GDPR 流程清晰
- 索引策略合理（复合索引、部分索引）

**2. Redis：10 域共存的架构张力**

Redis 在系统中扮演的角色远超"缓存"：

| 域 | 数据特征 | 延迟敏感度 | 失效影响 |
|----|---------|-----------|---------|
| 房间注册表 | O(N) KEYS 扫描 | 🔴 高 | 路由失效 |
| Pub/Sub 广播 | 高吞吐实时 | 🔴 高 | 游戏帧丢失 |
| JWT 撤销列表 | 每请求检查 | 🔴 高 | 认证失效 |
| 限流计数器 | EVAL 脚本 | 🟡 中 | 安全降级 |
| 大厅读缓存 | SET/GET TTL | 🟡 中 | 回源 PG |
| Magic Link token | 低频 | 🟢 低 | 登录失败 |
| 游戏结果流 | 积压风险 | 🟡 中 | 数据延迟 |
| 邮件队列 | 积压风险 | 🟢 低 | 邮件延迟 |
| Refresh token | 中频 | 🟡 中 | 会话失效 |
| 配置缓存 | 低频 | 🟢 低 | 配置回源 |

核心风险：`KEYS` 命令（O(N) 阻塞）在房间注册表查询中使用，会暂停所有 Redis 操作。游戏结果流积压时可能填满 Redis 内存，触发驱逐策略影响 JWT 检查等关键路径。

**3. 一致性模型：多层保障但有缝隙**

- **强一致**：PG 事务覆盖用户、游戏结果、审计日志、Outbox 事件
- **最终一致**：Outbox 模式（ADR-009）确保事件可靠投递；Redis Stream 消费者组提供 at-least-once
- **会话一致**：房间状态在 Redis + PG 间有 debounce 100ms 的异步持久化窗口（ADR-027）
- **风险缝隙**：异步持久化在 SIGTERM 时有 60s grace period drain，但如果 Pod 被强制杀死（SIGKILL），最后 100ms 的状态变更会丢失。终态（EndGame）有同步 flush，但中间态不保证

**4. CockroachDB 迁移路径：未验证**

代码中有 `DB_DIALECT=cockroach` 的方言切换路径，docker-compose 有 CockroachDB profile。但从未在任何环境运行过。CRDB 的 `REGIONAL BY ROW` 行驻留、跨区域写延迟、与 PG 的 SQL 方言差异都是未验证的。

**5. 数据模型演进性**

- 软删除 + email_hash + anonymized 标志的 GDPR 设计前瞻
- Outbox 事件有保留索引（migration 000011），支持清理
- 审计日志有 DB 触发器禁止 UPDATE/DELETE（tamper-proof）
- 但：缺少数据归档策略（审计日志无限增长），runbook 中已识别此问题

---

## 维度 4：安全防御纵深与威胁面

### 核心问题
- 整体安全态势如何？
- 纵深防御是否真正分层？
- 威胁模型是否仍然有效？

### 总体判断

🟢 **领先** — 六层纵深 + STRIDE 威胁模型 + 供应链安全，远超学习项目的典型水平。

### 关键发现

**1. 纵深防御的六层结构**

| 层 | 覆盖 | 成熟度 |
|----|------|--------|
| Phase 0 发布阻塞 | trusted proxy CIDR、HSTS、lockout 隔离 | ✅ |
| 第一层 CI/CD 供应链 | secret scan × 2、digest pin、cosign、SBOM、license check | ✅ |
| 第二层 认证会话 | JWT + Magic Link + Refresh 轮换 + 撤销 + Admin 独立密钥 | ✅ |
| 第三层 WebSocket | Origin 校验、read limit 4096、连接上限、CSP | ✅ |
| 第四层 输入验证 | SQL 参数化、nickname 消毒、textContent 渲染、路径穿越测试 | ✅ |
| 第五层 中间件限流 | per-endpoint + per-user + per-IP + fail-closed/open 策略 | ✅ |
| 第六层 密钥 PII | AES-256-GCM + HMAC email_hash + GDPR 删除流 + 审计 HMAC 链 | ⚡ 部分 |

**2. 供应链安全：企业级水准**

- Docker 全阶段镜像 pin 到 SHA256 digest
- cosign 容器镜像签名 + 部署前验签
- SBOM（CycloneDX）生成
- govulncheck + gitleaks + detect-secrets + Trivy + CodeQL 五重扫描
- npm audit --audit-level=high
- License check（禁止 GPL/AGPL）
- 这套供应链安全流程在很多商业公司都做不到

**3. 威胁模型的有效性**

STRIDE 威胁模型覆盖了全部六个维度，每项威胁都有对应缓解措施。限流配额表按端点细化（quickplay 10/min fail-closed、admin:login 5/min fail-closed 等）。PII 数据分类表明确（email PII、IP PII、JWT 凭据等）。

**4. 安全缺口**

| 缺口 | 严重度 | 说明 |
|------|--------|------|
| `RotateKey()` 是 stub | 🟡 中 | AES 密钥轮换无路径，ADR-022 已承认 |
| `AUDIT_SECRET` 可回退到 `JWT_SECRET` | 🟡 中 | 审计完整性与签名密钥耦合 |
| `EncryptEmailForStorage` 在 `encKey==nil` 时回退明文 | 🟢 低 | 仅 dev 安全，生产强制校验 |
| 多区域 mTLS 证书轮换 | 🟡 中 | 文档提到但从未实践 |

**5. GDPR 合规：超出学习项目预期**

- 数据导出（`GET /api/v1/user/data`）
- 数据删除（`DELETE /api/v1/user/data`）：立即匿名化 + 30 天硬删除
- 审计日志记录删除操作（不记录被删 PII）
- 数据驻留设计（CRDB REGIONAL BY ROW，提议中）

---

## 维度 5：性能边界与扩展性天花板

### 核心问题
- 系统在什么负载下会断裂？
- 瓶颈在哪？
- 水平扩展是真实能力还是理论假设？

### 总体判断

🟡 **需关注** — 理论分析非常充分，但关键压测数据表为空。扩展能力目前是"设计假设"而非"已验证事实"。

### 关键发现

**1. 单实例容量估算：理论合理**

- WS 连接上限 10,000（舱壁），内存约 50MB+
- 活跃房间 ~500-2000（CPU tick 15Hz × 房间数限制）
- REST RPS ~1,250（PG 池 25 连接 × ~50 QPS/连接）
- DAU 粗算 ~200k（5% 峰值并发假设）
- EncodeSnapshot ~500ns/op → 单核 ~2M snapshot/s 理论上限

**2. 水平扩展机制：设计完整**

- HPA 同时按 CPU(65%) 和 WS 连接数(6000) 扩缩，min=3 max=100
- owner 反向代理 + 租约接管实现区域内水平扩展
- SIGTERM → readiness 503 → LB 摘除 → 60s grace drain
- 房间分散到多实例，扩展维度是"房间总数"而非"单房间算力"

**3. 压测基础设施存在但数据为空**

k6 脚本齐全（smoke、ws-soak、single-room），阈值定义明确。但 `capacity-planning.md` 中的压测数据表全部是 `_填_`：

```
| 实例数 | 稳定并发 WS | 活跃房间 | message p99 | 瓶颈 |
|--------|-------------|----------|-------------|------|
| 1      | _填_        | _填_     | _填_        | _填_ |
| 3      | _填_        | _填_     | _填_        | _填_ |
| 10     | _填_        | _填_     | _填_        | _填_ |
```

这意味着所有容量数字都是理论推算，从未被实际压测验证。

**4. 断裂点分析：文档化且合理**

| 流量倍数 | 最先崩溃的组件 | 应对方案 |
|----------|---------------|---------|
| 10x | Hub 内存 | 房间状态外置 Redis（已实施） |
| 100x | WS 连接数 + PG 写入 | Hub 分片 + 批量写入 |
| 1000x | 物理模拟 CPU | 独立 Game Worker 进程池 |

**5. 热路径优化（ADR-027）**

Room 出站管道将广播和持久化移出 tick 锁路径：
- 持锁路径仅更新内存 + enqueue
- 单 goroutine outbound loop（buffer 256）锁外执行
- debounce 100ms 合并 PG 写入
- 慢客户端标记后锁外断开

这个设计直接解决了"PG/Redis 抖动拖慢 15Hz tick"的核心问题。

**6. 固有的单房间性能天花板**

单个房间的物理模拟不可分片——这是实时权威模拟的固有限制。扩展靠"房间分散到多实例"。这意味着单房间 50 玩家时的 CPU 消耗是硬上限，无法通过加机器解决。

---

## 维度 6：容错设计与韧性评估

### 核心问题
- 部分失败时系统如何表现？
- 故障模式是否被理解和测试？
- 有无 SPOF？

### 总体判断

🟢 **领先** — 熔断/隔板/降级/租约/Outbox 全覆盖，设计成熟。混沌实验尚未实际执行是唯一遗憾。

### 关键发现

**1. 熔断器矩阵：分层配置合理**

| 熔断器 | 触发阈值 | 恢复超时 | 半开探测数 | 设计理由 |
|--------|---------|---------|-----------|---------|
| postgres | 连续 5 次失败 | 30s | 3 | 数据库恢复需要时间 |
| redis | 连续 5 次失败 | 15s | 3 | Redis 恢复较快 |
| resend-api | 连续 3 次失败 | 60s | 1 | 外部 API 更保守 |

熔断器状态通过 Prometheus Gauge 暴露，runbook 有全局视图和一键查询命令。

**2. 降级策略：差异化 fail 策略**

- **Fail-closed**（安全敏感）：quickplay、admin:login → Redis 故障时拒绝请求
- **Fail-open**（可用性优先）：其他端点 → Redis 故障时放行
- **游戏容错**：PG 不可用时，进行中房间不受影响（内存状态），游戏结果延迟写入
- **优雅降级链**：PG 抖动 → 熔断 open → 503 快速失败 → 不堆积请求

**3. 租约接管：消除脑裂**

- owner 每次状态同步续租（TTL 30s）
- 仅在"注册表 miss / Redis 不可用"或"同区域且租约过期"时接管
- 跨区域永不接管 → 消除双活 owner 脑裂
- 取代了早期无作用域的 last-writer-wins

**4. Outbox 模式：可靠事件投递**

- 事件与业务数据在同一 PG 事务中写入 → 原子性保证
- Outbox Publisher 异步消费 → at-least-once 投递
- 消费者幂等处理 → 去重
- 保留索引支持清理 → 防止无限增长

**5. 混沌实验：设计完善但未执行**

4 个实验设计完整（PG 宕机、Redis 阻断、网络延迟、Pod 驱逐），每个有稳态假设、成功标准、执行频率。但 `实验结果` 标注"待在 staging 执行"——从未实际运行。

**6. SPOF 分析**

| 组件 | SPOF? | 缓解 |
|------|-------|------|
| PostgreSQL | 是（单实例） | 熔断 + 异步写入 + 未来的 CRDB |
| Redis | 是（单实例） | 熔断 + 降级 + 未来的区域本地 Redis |
| 单房间 goroutine | 是（固有） | owner 租约接管 + 状态外置恢复 |

---

## 维度 7：可观测性成熟度

### 核心问题
- 生产环境能否快速定位问题？
- 三大支柱是否连贯？
- 是否过度或不足？

### 总体判断

🟢 **领先** — 三支柱 + 持续 Profiling + Burn Rate 告警，可观测性栈的完整度远超项目规模所需（这正是 ADR-000 的刻意目标）。

### 关键发现

**1. 三支柱连贯性**

| 支柱 | 技术 | 覆盖 | 评估 |
|------|------|------|------|
| 指标 | Prometheus + client_golang | 熔断器状态、连接池、WS 连接、房间数、锁持有时间、队列深度、persist lag | ✅ 全面 |
| 日志 | slog（结构化） | RequestID 关联、级别策略文档化 | ✅ 充分 |
| 链路 | OTel + OTLP gRPC | HTTP + WS 消息（Tap 采样） | ✅ 充分 |

三支柱通过 RequestID / TraceID 关联，Grafana 可从指标跳转到日志和链路。

**2. 告警体系：Burn Rate 驱动**

- 快速燃烧：1h × 14.4 倍率 → 2m 持续告警
- 慢速燃烧：6h × 6 倍率 → 15m 持续告警
- SLO 定义了 4 个核心用户旅程（认证、房间创建、WS 连接、游戏消息延迟）
- Error Budget 计算清晰（认证 43.2 分钟/月、房间创建 3.6 小时/月）

**3. 持续 Profiling**

- Pyroscope always-on profiling（CPU、Alloc Objects、Alloc Space、Inuse Objects/Space）
- pprof 按需启用（端口 6060）
- 与 Grafana 集成，可从仪表盘跳转到火焰图

**4. Grafana 仪表盘**

- provisioning 自动配置数据源和仪表盘
- 多区域视图通过 Thanos 按 `region` 标签切分
- 部署在 `deploy/grafana/dashboards/` 目录

**5. 可观测性的"刻意过度"**

对于一个小游戏来说，这套可观测性栈确实"过度"了。但 ADR-000 明确将此列为刻意保留清单。从学习目标角度，这是正确的——它让开发者能在真实场景中练习 SLO/告警/混沌工程。

**6. 潜在盲区**

- 日志轮转策略：runbook 提到"日志文件累积未轮转"作为故障原因，但没有实际的 logrotate 配置
- 指标保留策略：Prometheus/Thanos 的数据保留期未明确
- WS 消息采样率：OTel 对 MsgTap 采样追踪，但采样率配置未文档化

---

## 维度 8：工程质量与开发体验

### 核心问题
- 代码库是负担还是乐趣？
- 约定是否一致？
- 新人上手是否顺畅？

### 总体判断

🟢 **领先** — CI 门禁严苛、文档完备、约定一致、工具链齐全。Windows 开发体验有轻微摩擦。

### 关键发现

**1. 工具链完整度**

| 工具 | 用途 | 集成 |
|------|------|------|
| golangci-lint v2.3.0 | 50+ linter | CI + pre-commit |
| ESLint v10 + typescript-eslint | 前端 lint | CI |
| govulncheck | Go 漏洞扫描 | CI |
| gitleaks + detect-secrets | 密钥扫描 × 2 | CI + pre-commit |
| Trivy | 容器扫描 | CI |
| CodeQL | 代码质量分析 | CI |
| cosign + SBOM | 供应链签名 + 物料清单 | CI |
| Playwright | E2E 测试 | CI |
| k6 | 负载测试 | Makefile |
| air | Go 热重载 | Makefile dev |
| Vite | 前端热重载 | npm dev |

**2. Makefile：一站式入口**

Makefile 暴露了 20+ target，从 `make dev`（启动全栈）到 `make ci`（完整 CI parity），覆盖了开发、测试、lint、安全、构建、部署、清理全生命周期。`make help` 列出所有 target。

**3. 约定一致性**

- Conventional Commits + commitlint + pre-commit hook 三重保障
- PR 规范、分支保护、API Deprecation Policy 均文档化
- 代码简化遵循 5 条原则（行为不变、遵循约定、清晰优于 clever、保持平衡、范围可控）
- 仓库布局有 CI 强制校验（`check-repo-layout`）

**4. 测试约定清晰**

CONTRIBUTING.md 明确了：
- 每包 1-3 个 `*_test.go`，使用 `t.Run` 表驱动
- 禁止为通过 funlen 机械拆分测试文件
- 单元用 miniredis，集成用 testcontainers
- 本地与 CI 一致：`make check`

**5. Windows 开发体验摩擦**

- 多个 CI 脚本是 bash（`scripts/ci/check-coverage.sh`、`check-docker-digests.sh`）
- 部分脚本有 PowerShell fallback（`check-repo-layout.ps1`、`verify-release-config.ps1`）
- Makefile 在 Windows 上需要 GNU Make for Windows 或 WSL
- `_bootstrap-env.ps1` 和 `run-backend.ps1`/`run-frontend.ps1` 表明已有 Windows 适配意识

**6. 代码生成**

- 昵称池通过 `scripts/codegen/generate_nicknames.go` 自动生成 Go + TS 双端代码
- CI 校验生成代码与提交代码一致（`check-generated`）
- 这消除了手动同步的漂移风险

---

## 维度 9：测试策略与质量信心

### 核心问题
- 测试策略是否带来真正的信心？
- 关键路径是否被保护？
- 测试金字塔是否平衡？

### 总体判断

🟡 **需关注** — 测试金字塔结构好，但 100% 单元覆盖率目标激进，前端有大量结构性排除。

### 关键发现

**1. 测试金字塔**

```
         E2E (Playwright, 6 spec × 6 matrix)
        ─────────────────────────────────
       集成 (testcontainers + miniredis)
      ─────────────────────────────────────
     单元 (Go -race -short + Vitest)
    ─────────────────────────────────────────
```

结构合理，各层职责清晰。

**2. 覆盖率门禁：激进但有排除**

| 层级 | 目标 | 实际状态 |
|------|------|---------|
| 后端单元 | lines/branches/functions ≥ 100% | 目标激进，实际约 73.4%（自检记录） |
| 后端集成 | lines ≥ 80% | 未有实际数据 |
| 前端 Vitest | 重要路径 90%，单文件 60% | 整体 61.3% |
| 前端排除 | 16 个文件被排除 | 排除量大 |

**3. 前端排除清单的隐忧**

`coverage-policy.md` 列出了 16 个被排除的文件，包括 `main.ts`、`ws_connect.ts`、`ws_connection.ts`、`entry_flow.ts`、`state_interp.ts` 等。这些文件大多是"胶水代码"和"DOM 交互"，但其中 `ws_connect.ts`（WebSocket 连接编排/重连）和 `ws_connection.ts`（心跳/重连/待发队列）包含了关键的重连逻辑，不覆盖它们会降低信心。

排除的理由是"行为由集成测试覆盖"，但前端没有真正的集成测试框架（只有 Vitest 单元测试 + Playwright E2E），中间层存在空白。

**4. Property-Based Testing**

- 后端：`TestPhysics_`、`TestState_`、`TestProtocol_` 使用 Go testing/quick
- 前端：`fast-check` 用于 `message_codec.property.test.ts`、`reducer.property.test.ts`、`snapshot_decode.property.test.ts`

这是一个亮点——属性测试在游戏物理和协议编解码这种"输入空间大"的场景下价值极高。

**5. 关键路径保护**

| 路径 | 测试覆盖 | 信心度 |
|------|---------|--------|
| 认证流程（Magic Link + JWT + Refresh） | ✅ 多层测试 | 🟢 高 |
| 游戏物理 tick | ✅ 单元 + property | 🟢 高 |
| WebSocket 协议编解码 | ✅ 单元 + property | 🟢 高 |
| Room 出站管道（ADR-027） | ✅ 有 contention test | 🟢 高 |
| owner 反向代理 + 租约接管 | ⚡ 有单元测试 | 🟡 中（从未在多实例环境验证） |
| GDPR 删除流程 | ✅ 有测试 | 🟢 高 |
| 审计日志防篡改 | ✅ 有测试 | 🟢 高 |
| 前端 WS 重连 | ⚡ 被排除 | 🟡 中 |

**6. 测试带来的信心度评估**

- **高信心**：单包内的逻辑正确性（物理、协议、认证、加密）
- **中信心**：跨包交互（handler → auth → store 的集成路径）
- **低信心**：多实例行为（owner 代理、租约接管、水平扩展）、多区域行为

---

## 维度 10：交付流水线与运维成熟度

### 核心问题
- CI/CD 是否值得信赖？
- 部署是否安全可回滚？
- 运维故事是否完整？

### 总体判断

🟢 **领先** — 14+ CI 作业、多区域 GKE 部署、cosign 签名、SBOM、完整 runbook。运维文档的完整度达到企业级。

### 关键发现

**1. CI 流水线：14+ 作业并行**

go-ci.yml 定义了：
1. Test（-race -short + 覆盖率门禁）
2. Lint（golangci-lint v2.3.0）
3. Vet
4. Benchmark
5. Security（govulncheck）
6. Gitleaks
7. detect-secrets
8. Container Scan（Trivy）
9. CodeQL（Go + JavaScript）
10. Migration Test（up → down → up 幂等性）
11. Docker Pin Check
12. License Check
13. OpenAPI Validate
14. API Route Consistency
15. Integration Tests（testcontainers）

ci-cd.yml 补充：
1. Quality Gate（前端 typecheck + lint + audit + test + 覆盖率）
2. Dependency Review
3. E2E（6 spec × Playwright）
4. Integration Tests
5. Deploy（多区域 GKE，cosign 验签，逐区域滚动）

build-push 作业需要前 14 个全部通过才能执行。这是真正的"质量门"。

**2. 部署安全性**

- 镜像 tag = git SHA（不可变、可追溯）
- cosign 签名 + 部署前验签
- SBOM 生成
- 逐区域滚动部署（max-parallel: 1）
- 部署后 `rollout status --timeout=300s` 验证
- Workload Identity Federation（免长期 JSON 密钥）

**3. 回滚能力**

- 镜像 tag = git SHA → 可回滚到任意历史版本
- K8s `kubectl rollout undo` 可用
- 数据库迁移有 down 路径（但回滚有状态服务需要谨慎）
- **缺失**：没有自动化回滚机制（如 Argo Rollouts、Flagger）

**4. 运维文档完整度**

| 文档 | 内容 | 评估 |
|------|------|------|
| runbook.md | 7 个故障场景 × 5 段式（症状→原因→排查→缓解→根治） | 🟢 企业级 |
| slo.md | 4 个 SLI/SLO + Error Budget + Burn Rate | 🟢 完整 |
| capacity-planning.md | 单实例估算 + 扩展拐点 + HPA 机制 + 压测计划 | 🟢 完整（数据待填） |
| chaos-experiments.md | 4 个实验设计 | 🟢 设计完整（未执行） |
| environments.md | dev/staging/prod 配置 | 🟢 存在 |
| continuous-profiling.md | Pyroscope + pprof | 🟢 存在 |

**5. K8s 基础设施**

- StatefulSet（实时层需要稳定网络标识，owner 反向代理依赖）
- HPA（CPU 65% + WS 连接数 6000 双指标）
- PDB（Pod Disruption Budget）
- Headless Service（实例间可寻址）
- 三个区域 overlay（us-east1 / europe-west1 / asia-southeast1）
- ConfigMap 注入区域配置（DEPLOY_REGION、REGION_WS_ENDPOINT、TRUSTED_PROXY_CIDRS）

**6. 从未在真实集群运行**

虽然 K8s 清单完整，但从未部署到真实 GKE 集群。所有 K8s 配置都是"设计态"而非"运行态"。

---

## 维度 11：知识管理与决策可追溯性

### 核心问题
- 知识是否被捕获而非口口相传？
- ADR 是否真正发挥决策记录作用？
- 文档债务有多重？

### 总体判断

🟢 **领先** — ADR 体系健康，CI 校验索引一致性，文档结构清晰。这是项目最突出的优势之一。

### 关键发现

**1. ADR 体系健康度**

- 28 份 ADR 覆盖从项目章程到具体技术决策
- 每个 ADR 有标准结构：上下文 → 决策 → 后果 → 关联
- 状态标注清晰：已接受 / 提议中 / 部分落地
- CI 校验 README 索引与实际 ADR 文件数一致
- ADR 间有引用网络（如 ADR-005 → ADR-013 → ADR-014 → ADR-015 → ADR-016）

**2. 决策可追溯性**

每个重要决策都能追溯到"为什么做这个选择"：
- 为什么用 pgx 不用 ORM → ADR-019
- 为什么用 Vanilla TS 不用框架 → ADR-018
- 为什么从 Cloud Run 迁到 GKE → ADR-013
- 为什么删除 Service 层 → ADR-024
- 为什么用 owner 反向代理不用 Pub/Sub → ADR-005 实施现状段

包括"放弃的替代方案"也记录在案（如 ADR-019 记录了 GORM/ent/sqlx 被放弃的原因）。

**3. 文档结构**

```
docs/
├── adr/           ← 28 份架构决策记录
├── architecture/  ← 系统架构文档（含多区域拓扑图）
├── api/           ← OpenAPI + AsyncAPI + WS 协议
├── data/          ← 数据库查询分析 + CRDB 迁移
├── development/   ← 覆盖率策略 + 基准测试
├── operations/    ← Runbook + SLO + 容量规划 + 混沌实验 + 持续 Profiling + 环境
├── security/      ← 威胁模型 + 安全自检清单 + 日志策略
├── templates/     ← ADR 模板 + Postmortem 模板
└── README.md      ← 文档索引
```

结构清晰，职责不重叠。仓库布局有 CI 强制校验。

**4. 文档债务**

| 债务 | 严重度 | 说明 |
|------|--------|------|
| ADR-013 废弃未标注 | 🟢 低 | Cloud Run → GKE 迁移已完成叙述，但 ADR 未标注"已废弃 Cloud Run 部分" |
| 压测数据表为空 | 🟡 中 | `capacity-planning.md` 的数据表全是 `_填_` |
| 前端两份 constants.ts | 🟢 低 | `shared/constants.ts` 和 `shared/game/constants.ts` 内容相同，所有者不明确 |
| 环境变量重复 | 🟢 低 | `.env.example` 中 `ADMIN_JWT_SECRET` 和 `TRUSTED_PROXY_CIDRS` 各出现两次 |

**5. Postmortem 文化**

- P0/P1 事故须 7 日内完成复盘（CONTRIBUTING.md）
- Postmortem 模板存在（`docs/templates/postmortem.md`）
- 安全审查记录在 `self-check-checklist.md` 中有审计追踪表

---

## 维度 12：战略风险与盲区扫描

### 核心问题
- 外部审计员会标记什么？
- 项目成功的最大风险是什么？
- 有哪些"未知的未知"？

### 总体判断

🔴 **高风险** — 不是因为做得不好，而是因为"文档描述的成熟度"与"实际验证的状态"之间存在系统性鸿沟。这是项目面临的最大战略风险。

### 关键发现

**1. 纸面成熟度 vs 实际验证状态的鸿沟**

这是贯穿所有维度的核心风险。项目拥有：
- 28 份 ADR（但多区域/CRDB 从未实现）
- 完整的 K8s 清单（但从未部署到真实集群）
- 4 个混沌实验设计（但从未执行）
- 压测脚本和阈值（但数据表为空）
- Runbook 中的多区域故障场景（但多区域从未运行）

这些"设计态"产物在文档中看起来已经"完成"，容易给人"系统已经具备企业级能力"的错觉。实际上，系统是一个**单区域、单实例、单 PG、单 Redis** 的部署，从未经历过真实生产流量、真实故障、真实扩展。

**2. Redis 单实例：最大的技术 SPOF**

Redis 承载 10 个功能域，是认证、限流、房间路由、游戏结果队列的关键路径。Redis 宕机时：
- 认证全部失败（JWT 撤销列表不可查）
- 限流 fail-open（安全降级）
- 房间路由降级到本地（区域内可用但无法新建房间）
- 游戏结果队列积压

虽然熔断器和降级策略存在，但 Redis 单实例故障的影响面是全局性的。

**3. "未知的未知"**

| 盲区 | 影响 | 发现时机 |
|------|------|---------|
| CockroachDB SQL 方言差异 | 迁移可能失败 | 尝试迁移时 |
| 多区域 mTLS 证书管理 | 跨区域通信失败 | 部署多区域时 |
| K8s StatefulSet 在真实集群的行为 | 部署可能不工作 | 部署到 GKE 时 |
| 15Hz tick 在真实多房间负载下的 CPU 表现 | 性能可能不达标 | 压测时 |
| PG 连接池 25 在高并发下的表现 | 连接耗尽 | 压测时 |
| 前端 WS 重连在真实网络抖动下的行为 | 用户体验差 | 生产使用时 |

**4. 外部审计员视角**

如果一位外部安全/架构审计员审视此项目，他们会：
- ✅ 对 ADR 体系和决策可追溯性印象深刻
- ✅ 对安全纵深和供应链安全给予高评
- ✅ 对测试策略和覆盖率门禁表示认可
- ⚠️ 标注 Redis 单实例职责过载为架构风险
- ⚠️ 标注密钥轮换未实现为安全缺口
- ⚠️ 质疑"文档描述的多区域能力是否已验证"
- 🔴 标注"从未在真实环境部署和压测"为最大风险

**5. 项目成功的最大风险**

项目的成功标准是"把企业级架构练全、练对"（ADR-000）。从这个标准看：
- **已成功**：限界上下文、Clean Architecture、弹性栈、可观测性、安全纵深、CI/CD、ADR 文化
- **未验证**：多区域、CRDB、真实水平扩展、真实故障恢复
- **风险**：如果项目在"未验证"部分停滞，学习价值将不完整。文档给人一种"已经做到了"的满足感，可能降低实际推进的动力

**6. 100% 单元覆盖率的战略风险**

100% 单元覆盖率目标可能导致：
- 为了覆盖率而写测试（而非为了验证行为）
- 过度 mock 导致测试与真实行为脱节
- 对"高覆盖率 = 高质量"的虚假信心
- 维护成本上升（每次改动都需要更新大量测试）

实际上自检记录显示后端单元覆盖率为 73.4%，远未达到 100% 目标。这个差距是诚实的——但目标本身可能需要重新审视。

---

## 跨维度关联分析

### 关联 1：Redis 贯穿维度 2/3/6/12

Redis 的"万能锤"角色是跨维度的核心风险：
- **维度 2**（技术栈）：对单一组件的战略级依赖
- **维度 3**（数据架构）：10 域共存 + O(N) KEYS 命令
- **维度 6**（容错）：熔断降级虽好但 Redis 是全局 SPOF
- **维度 12**（战略风险）：最大的技术单点故障

### 关联 2：纸面 vs 实际贯穿维度 1/5/6/10/12

- **维度 1**（架构哲学）：ADR 叙事完整但多区域从未实现
- **维度 5**（性能）：容量规划完善但压测数据为空
- **维度 6**（容错）：混沌实验设计完善但从未执行
- **维度 10**（交付）：K8s 清单完整但从未部署
- **维度 12**（战略风险）：这是最大风险的根源

### 关联 3：安全纵深贯穿维度 4/8/10

- **维度 4**（安全）：六层纵深 + STRIDE + 供应链
- **维度 8**（工程）：CI 门禁 + pre-commit + lint
- **维度 10**（交付）：cosign + SBOM + Trivy + CodeQL

这三个维度形成了一致的安全文化——从代码提交到部署到运行时，安全始终是一等公民。

---

## 战略级行动建议（按优先级）

### 🔴 P0：验证而非新建

| # | 建议 | 关联维度 | 预期收益 |
|---|------|---------|---------|
| 1 | 执行至少一轮真实压测，填充 capacity-planning 数据表 | 5, 12 | 将"设计假设"转为"已验证事实" |
| 2 | 在 staging 环境执行 4 个混沌实验 | 6, 12 | 验证容错设计在真实故障下的表现 |
| 3 | 部署到真实 GKE 集群（至少单区域） | 10, 12 | 验证 K8s 清单的可工作性 |

### 🟡 P1：消除已知风险

| # | 建议 | 关联维度 | 预期收益 |
|---|------|---------|---------|
| 4 | 用 SCAN 替换 Redis KEYS 命令 | 2, 3 | 消除 O(N) 阻塞风险 |
| 5 | 实现 AES 密钥轮换路径 | 4 | 关闭唯一明显安全缺口 |
| 6 | 评估 Redis 域拆分策略（至少将 Pub/Sub 和 JWT 检查隔离） | 2, 3, 6, 12 | 降低 SPOF 影响面 |
| 7 | 重新审视 100% 单元覆盖率目标的合理性 | 9 | 避免覆盖率驱动的测试劣化 |

### 🟢 P2：持续改进

| # | 建议 | 关联维度 | 预期收益 |
|---|------|---------|---------|
| 8 | 为前端 WS 重连逻辑补充集成测试 | 9 | 保护关键用户体验路径 |
| 9 | 删除 chi.Logger/chi.Recoverer 冗余中间件 | 2 | 降低认知负担 |
| 10 | 清理 .env.example 重复变量 | 8, 11 | 消除配置债务 |
| 11 | 为 ADR-013 添加废弃标注 | 11 | 消除文档漂移 |
| 12 | 考虑数据归档策略（审计日志无限增长） | 3 | 防止未来磁盘问题 |

---

## 附录：调研方法

| 方式 | 覆盖内容 |
|------|---------|
| 通读 ADR-000 项目章程 | 架构哲学与设计意图基线 |
| 通读 ADR 索引 + 7 份关键 ADR（005/014/017/022/024/027/028） | 设计连贯性与演进路径 |
| 阅读 architecture.md | 提议 vs 已实现状态区分 |
| 分析 go.mod / package.json / Dockerfile / docker-compose.yml | 技术栈选型与依赖生态 |
| 列举 store/ 49 文件 + migrations 11 版本 | 数据架构与持久化策略 |
| 阅读 threat-model.md + self-check-checklist.md | 安全纵深与威胁面 |
| 阅读 slo.md + capacity-planning.md + benchmarks | 性能边界与扩展性 |
| 阅读 chaos-experiments.md + runbook.md | 容错设计与运维成熟度 |
| 阅读 continuous-profiling.md | 可观测性成熟度 |
| 阅读 CONTRIBUTING.md + coverage-policy.md | 工程质量与开发体验 |
| 分析 go-ci.yml + ci-cd.yml（14+ CI 作业） | 交付流水线 |
| 跨维度关联分析 | 战略风险与盲区 |
