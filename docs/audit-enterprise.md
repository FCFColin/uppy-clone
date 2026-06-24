# 企业级生产就绪审计报告

> **审计范围**：Balloon Battle 多人网页气球对战项目（d:\Project\多人网页游戏）
> **审计方法**：基于 `plan.md` 的 10 维度企业级审计框架（A-J），证据优先，禁止猜测
> **审计日期**：2026-06-23
> **审计版本**：代码 HEAD（Go 1.26.4 / TS / PG16 / Redis7 / Docker / GCP）

## 图例

- ✅ 已达企业标准
- ⚠️ 部分达标（量化差距，引用具体文件名+行号）
- ❌ 未达标（说明企业为何需要此项 + 量化业务风险 + 具体修复方案）
- 🎓 面试高频考点（系统设计/安全/行为面试中高频出现）
- 💼 工作中每天用到（入职前两周就会接触）
- 🔭 高级工程师技能（Senior/Staff/Architect 级别才主导的工作）
- 📋 行业规范标准（有对应的 RFC/OWASP/白皮书/法规文件）

---

## 执行摘要

本项目在**单点能力**上已具备企业级雏形——7 份 ADR、OpenAPI 规格、RFC 7807 错误 envelope、JWT+Refresh Token 轮换、RBAC（Casbin）、AES-256-GCM、Prometheus 黄金信号、OpenTelemetry、gobreaker 熔断器、testcontainers、多阶段 Dockerfile、Terraform、PDB、Conventional Commits——这些是多数个人项目所不具备的。

但审计发现三类系统性"最后一公里"问题，使其未真正达到企业级生产就绪：

1. **"实现到位但未装配"**（最严重，共 6 处）
   - `EndpointRateLimit` 已实现三维限流但路由全部用 IP-only 版本（G-3）
   - `RetryableError` 在生产代码零调用，重试机制形同虚设（C-2）
   - `HTTPConnectTimeout`/`HTTPRequestTimeout` 定义但 EmailWorker 用 `http.DefaultClient` 无超时（C-3）
   - Resend 熔断器 `cb` 字段是死代码，实际发邮件裸奔（C-1）
   - `DBPoolAcquireDuration` 指标声明但从未 Observe（B-4）
   - `LOG_FORMAT` 环境变量注释承诺但代码未实现（B-6）

2. **供应链不可变性与 SLSA 未闭环**
   - Dockerfile 三处 FROM 仅 tag pin，未 @sha256 digest（H-1）
   - 仅有 cosign sign，缺 SLSA provenance attestation，未达 SLSA L3（D-5）
   - `.secrets.baseline` 文件缺失，detect-secrets pre-commit 无法运行（D-3）

3. **文档与代码漂移**
   - architecture.md 声称 60fps，实际 15Hz（A-4）
   - ADR 索引与实际文件错配，目录有 10 个 ADR 但 README 仅列 7 条（A-1）
   - OpenAPI 与路由多处漂移：`/lobby/{code}/ws` 缺 `/api/v1` 前缀、`/user/data` 未文档化、202 vs 200 不一致（F-1）
   - threat-model.md 限速数值与代码不一致（G-6）
   - CHANGELOG 仅 [Unreleased]，release-please manifest 声明 1.0.0 但无对应版本块（J-1）

### 关键发现统计

| 维度 | ✅ | ⚠️ | ❌ | 关键问题 |
|------|----|----|----|---------|
| A 系统设计与架构 | 3 | 3 | 4 | ADR 索引错配、handler 跨层依赖 store、文档与实现不一致 |
| B 可观测性三支柱 | 14 | 3 | 6 | 审计日志丢上下文、/metrics 无鉴权、WS span 断裂、采样器缺失 |
| C 弹性工程 | 13 | 2 | 5 | 重试形同虚设、熔断器未装配、HTTP 超时未使用 |
| D CI/CD 与 DevSecOps | 13 | 3 | 6 | release-please workflow 缺失、secrets baseline 缺失、前端 shift-left 断裂 |
| E 测试策略 | 5 | 4 | 5 | worker/outbox/audit 零测试、WS 零测试、覆盖率无门禁 |
| F API 设计成熟度 | 5 | 2 | 3 | OpenAPI 与路由漂移、CORS 缺 PATCH、429 缺 Retry-After |
| G 安全深度 | 14 | 4 | 3 | Admin Token 无撤销、限速未启用、Admin 锁定 IP 错误、email 明文存储 |
| H 云原生与容器化 | 10 | 3 | 1 | Dockerfile 未 pin digest、WS 内存态限制水平扩展 |
| I 数据库工程 | 7 | 4 | 3 | 冗余索引、查询分析无实测、连接池未压测且硬编码 |
| J 工程文化与协作 | 13 | 4 | 1 | CHANGELOG 未同步、PR 模板缺失、runbook 缺认证章节 |
| **合计** | **97** | **32** | **37** | — |

### Top 6 紧迫修复项（按风险×成本排序）

| 排序 | 发现 | 风险 | 修复成本 | 行动 |
|------|------|------|---------|------|
| 1 | C-2 重试形同虚设（RetryableError 零调用） | 高（瞬态故障全部转化为用户可见错误） | Low（一处辅助函数修改） | 立即 |
| 2 | C-3 EmailWorker 无超时可挂死 | 高（Worker 静默死亡，用户无法登录） | Low（构造 http.Client） | 立即 |
| 3 | G-3 限速中间件未启用 | 高（暴力破解与 DoS 直接成功） | Low（路由替换） | 立即 |
| 4 | G-4 Admin 登录锁定 IP 错误 | 高（admin 账户被暴力破解） | Low（改用 extractClientIP） | 立即 |
| 5 | D-3 .secrets.baseline 缺失 | 高（密钥泄露防线形同虚设） | Low（一条命令生成） | 立即 |
| 6 | F-2 CORS 缺 PATCH 方法 | 高（管理后台浏览器不可用） | Low（一行修改） | 立即 |

---

## A. 系统设计与架构

### A.1 差距矩阵

| 检查项 | 状态 | 证据（文件:行号） | 企业必要性 | 修复方案 |
|--------|------|-------------------|------------|----------|
| 架构图（Mermaid 组件关系） | ✅ | docs/architecture.md:12-46 | 组件拓扑可视化是新成员入门与跨团队对齐的必备资产 | 已具备 |
| 数据流图（Mermaid 时序） | ✅ | docs/architecture.md:52-83 | 时序图揭示请求链路与失败点 | 已具备 |
| ADR 索引（README） | ❌ | docs/adr/README.md:7-15 | ADR 索引是架构决策可追溯性的入口 | 重写 README 表格，补齐 008-010 |
| ADR 完整性（001-007） | ⚠️ | docs/adr/001..007 | 关键技术选型需可追溯 | 补齐缺失 ADR（见 A-2） |
| 已知局限与扩展点 | ✅ | docs/architecture.md:89-94, 146-150 | 局限透明化是技术债治理前提 | 已具备 |
| 100x 瓶颈分析 | ⚠️ | docs/architecture.md:96-145 | 容量规划是 SLO 与预算审批依据 | 现有分析与代码现状部分脱节 |
| 分层架构清晰度 | ⚠️ | backend/internal/handler/lobby.go:163; auth.go:19-20 | 分层是可测试性与可维护性根基 | 引入 service 层 |
| 依赖方向正确性 | ❌ | lobby.go:163,173; auth.go:142,205,288,334 | handler→store 跨层依赖违反单向分层 | handler 仅依赖 game/service 接口 |
| 文档与实现一致性 | ❌ | docs/architecture.md:71 (60fps) vs protocol/constants.go:129 (15Hz) | 文档失真导致容量规划错误 | 修正 architecture.md:71 为 15Hz |
| ADR 状态同步 | ❌ | docs/adr/README.md:13 (ADR-005 提议中) vs hub.go:349-372 (已实现) | 状态失真导致重复劳动 | ADR-005 状态改为"已接受" |

### A.2 关键发现

#### 发现 A-1: ADR 索引与实际文件严重错配，决策可追溯性断裂
- **状态**: ❌
- **证据**: docs/adr/README.md:14-15 声称 ADR-006="API 版本化策略"、ADR-007="RBAC"，但实际 006-cache-layer.md 标题"Redis 读缓存层"，007-message-queue.md 标题"Redis Stream 消息队列"；README 仅列 7 条但目录实际有 10 个 ADR 文件
- **企业原理**: ADR 索引是架构治理的"目录页"。索引与文件错配时，新成员按索引查找得到错误信息，审计员无法核实决策链路，离职交接时关键决策不可考。商业代价：决策被重复讨论、相反决策被同时引入、合规审计失败。
- **修复方案**: 1) 重写 docs/adr/README.md 表格，列出 001-010 全部 ADR；2) 若 RBAC 与 API 版本化确有决策但未成文，补写为 ADR-011、ADR-012；3) CI 加入脚本校验 README 表格行数 = docs/adr/ 下文件数
- **学习价值**: 🎓 💼 📋
- **工具选型**: 开源: adr-tools / log4brains | 企业SaaS: Confluence ADR Template | Go集成: 自研 CI 校验脚本
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 A-2: 关键技术选型缺乏 ADR 记录
- **状态**: ⚠️
- **证据**: docs/adr/ 现有 001-010，但以下选型无 ADR：HTTP 路由框架（chi, main.go:15）、WebSocket 库（gorilla, lobby.go:13）、迁移工具（golang-migrate, postgres.go:25-27）、可观测性栈（OTel+Prometheus+slog, main.go:41,50）、游标分页决策（postgres.go:411-418）、幂等中间件（middleware/idempotency.go）、优雅降级策略（handler/degradation.go）
- **企业原理**: 关键技术选型若无 ADR，"为什么选 chi 而非 gin/echo"在代码评审、技术债盘点、框架升级时无法回答。企业代价：升级时无法评估原始约束，被迫"重写而非演进"。
- **修复方案**: 补写 ADR-011 至 ADR-017 覆盖上述选型
- **学习价值**: 💼 📋 🔭
- **工具选型**: 开源: log4brains | 企业SaaS: Structurizr | Go集成: 自研 ADR 模板
- **优先级**: 就业价值 中 × 实施复杂度 Low

#### 发现 A-3: Handler 层跨层依赖 Store，分层架构边界破损
- **状态**: ❌
- **证据**: backend/internal/handler/lobby.go:163 `db := h.hub.DB()`；lobby.go:173 `db.LoadAllActiveLobbies(...)`；auth.go:19-20 `db *store.PostgresStore; redis *store.RedisStore`；auth.go:142,205,288,334 多处 `h.db.GetUserByID`/`h.db.AnonymizeUser`
- **企业原理**: 分层架构核心约束是"依赖方向单向向下"。handler→store 跨层依赖导致：(1) store 签名变更迫使 handler 同步修改；(2) handler 单元测试必须 mock store；(3) 无法在不改 handler 前提下替换持久化实现。商业代价：迭代速度下降，回归测试成本上升，技术债复利。
- **修复方案**: 1) game 层新增 `Hub.ListLobbies(ctx, limit, cursor)`；2) handler 改为调用 `h.hub.ListLobbies(...)`，移除 `h.hub.DB()`；3) 新增 service 层封装用户查询；4) Hub.DB() 标记 deprecated
- **学习价值**: 🎓 💼 📋
- **工具选型**: 开源: Go 接口 + wire/fx | 企业SaaS: 无 | Go集成: go/analysis 自研分层校验器
- **优先级**: 就业价值 高 × 实施复杂度 Medium

#### 发现 A-4: 架构文档与实现不一致，容量规划建立在错误前提上
- **状态**: ❌
- **证据**: docs/architecture.md:71 `S->>S: Start Room tick loop (60fps)` vs backend/internal/protocol/constants.go:129 `TickRate = 15 // Hz`；docs/architecture.md:93 "无消息队列" vs room.go:294 `r.hub.redis.EnqueueGameResult`；docs/adr/README.md:13 ADR-005 "提议中" vs hub.go:349-372 已实现
- **企业原理**: 架构文档是容量规划、SLO 制定、扩容预算的依据。文档声称 60fps 而实际 15Hz，CPU 容量估算偏差 4 倍；声称"无消息队列"而实际已实现，导致重复建设。商业代价：过度采购（浪费）或容量不足（宕机）。
- **修复方案**: 1) 修正 architecture.md:71 为 15Hz；2) 修正 :93 删除"无消息队列"改为"已引入 Redis Stream（ADR-007）"；3) 更新 ADR-005 状态为"已接受（部分实施）"；4) CI 加文档一致性校验脚本
- **学习价值**: 🎓 💼 📋
- **工具选型**: 开源: markdownlint + 自研一致性脚本 | 企业SaaS: Backstage TechDocs | Go集成: 解析 constants.go 反向校验
- **优先级**: 就业价值 高 × 实施复杂度 Low

### A.3 100x 流量瓶颈分析

**前提校准**（基于代码实测）：
- TickRate = 15Hz（protocol/constants.go:129），非文档声称的 60fps
- PG 连接池上限 = 25（postgres.go:79）
- 游戏结果已走 Redis Stream 队列（room.go:294）
- saveState 每 30 ticks 持久化一次（room.go:383-385），即每房间每 2 秒一次 PG 写

**100x 场景**（基于 architecture.md:107）：10,000 房间 / 50,000 连接

| 崩溃顺序 | 组件 | 证据 | 崩溃机制 | 崩溃时间点 |
|---------|------|------|---------|-----------|
| 1️⃣ 最先 | PG 连接池（25）耗尽 | postgres.go:79; room.go:383-385 | 10,000 房间 × 每 2s saveState = 5,000 写 QPS；连接池仅 25 | 流量 ~5x |
| 2️⃣ 次之 | 单实例 WebSocket fd 上限 | hub.go:38-39 | 50,000 连接无法落入单实例 | 流量 ~10x |
| 3️⃣ 再次 | Hub 内存 OOM | hub.go:33 `map[string]*Room` | 10,000 Room 对象 + GameState 缓冲常驻 | 流量 ~20x |
| 4️⃣ 最后 | tick 循环 CPU 饱和 | room.go:314-328 | 10,000 goroutine × 15Hz 物理模拟 | 流量 ~50x |

**关键发现**：文档将"WebSocket 连接数"和"PG 写入"列为最先崩溃点，但实际代码已用 Redis Stream 解耦游戏结果写入，**真正的最先崩溃点是 saveState() 的 PG 写热点**——10,000 房间将产生 5,000 QPS 的 lobby_states UPSERT，远超 25 连接池承受能力。

**缓解方案矩阵**：

| 瓶颈 | 短期（1-2 周） | 中期（1-2 月） | 长期（季度级） |
|------|---------------|---------------|---------------|
| PG 连接池耗尽 | saveState 改写穿透 Redis + 异步批量落 PG | 读写分离：lobby_states 读走 Redis | 房间状态全外置 Redis，PG 仅存最终结果 |
| WebSocket fd 上限 | Hub 分片：按 room_id hash 路由到 N 实例 | sticky session + 一致性哈希 | 独立 WebSocket 网关层 |
| Hub 内存 OOM | Room 状态冷热分离：waiting 房间仅存 Redis | Room 对象池化 | 房间状态全外置（ADR-005 终态） |
| tick CPU 饱和 | 房间调度到独立 Worker 进程 | 物理模拟分片 | 独立 Game Worker 进程池 |

---

## B. 可观测性三支柱

### B.1 差距矩阵

| 检查项 | 状态 | 证据（文件:行号） | 企业必要性 | 修复方案 |
|--------|------|-------------------|------------|----------|
| 结构化日志（JSON） | ✅ | backend/cmd/server/main.go:38-41 | ELK/Loki 必需 JSON | 已实现 slog.NewJSONHandler |
| request_id 贯穿 | ✅ | backend/internal/middleware/logging.go:38-42 | 同一请求日志聚合 | 已通过 slogctx 注入 |
| trace_id 贯穿日志 | ✅ | backend/internal/middleware/tracing.go:31-34 | 日志-链路关联 | 已注入 slog context |
| 审计日志（Audit Log） | ⚠️ | backend/internal/audit/audit.go:13-46 | SOC2/ISO27001 合规 | 结构完整，但 hub.go 调用丢上下文 |
| 审计日志上下文贯穿 | ❌ | backend/internal/game/hub.go:87,155 | 审计事件需关联 request_id/trace_id | CreateRoom/RemoveRoom 改为接收 ctx |
| 日志级别策略 | ✅ | backend/cmd/server/main.go:37,373-384 | LOG_LEVEL 控制 | 已实现 |
| LOG_FORMAT 切换 | ❌ | backend/cmd/server/main.go:36 | 注释承诺 text 但未实现 | 增加 LOG_FORMAT 分支 |
| 日志采样 | ❌ | 全局无 sampling | 高 QPS 日志洪水 | 引入 slog 速率采样 |
| /metrics 端点 | ✅ | backend/cmd/server/main.go:209 | Prometheus 抓取入口 | 已暴露 |
| /metrics 鉴权 | ❌ | backend/cmd/server/main.go:209 | 暴露内部状态 | 增加基本认证或 NetworkPolicy |
| Latency P50/P95/P99 | ✅ | backend/internal/metrics/metrics.go:42-49 | 黄金信号-延迟 | 已用 SLOBuckets |
| Traffic | ✅ | backend/internal/metrics/metrics.go:35-41 | 黄金信号-流量 | http_requests_total |
| Errors 4xx/5xx | ✅ | backend/internal/middleware/prometheus.go:34 | 黄金信号-错误 | 已用 status 标签 |
| Saturation（DB 池） | ✅ | backend/internal/metrics/metrics.go:65-76 | 黄金信号-饱和度 | IdleConns/InUseConns |
| Saturation（Redis 池） | ✅ | backend/internal/metrics/metrics.go:116-127 | 黄金信号-饱和度 | 已采集 |
| DB 池获取耗时 | ❌ | backend/internal/metrics/metrics.go:58-64 | 池饱和早期预警 | 声明但从未 Observe |
| 业务指标 | ✅ | backend/internal/metrics/metrics.go:79-102 | 业务可观测性 | ActiveRooms/Players/WS/Sessions |
| 熔断器状态指标 | ✅ | backend/internal/resilience/circuitbreaker.go:42 | 下游依赖熔断可观测 | 已用 GaugeVec |
| OpenTelemetry 集成 | ✅ | backend/internal/telemetry/telemetry.go:38-76 | CNCF 标准 | 已用 OTLP gRPC |
| HTTP 请求 Span | ✅ | backend/internal/middleware/tracing.go:23-29 | 入口请求链路 | 含 method/url/route/status |
| DB 查询 Span | ✅ | backend/internal/store/postgres.go:116 等 14 处 | 慢查询定位 | 已覆盖 |
| Redis 操作 Span | ✅ | backend/internal/store/redis.go:77 等 11 处 | 缓存层链路 | 已覆盖 |
| WebSocket 消息 Span | ⚠️ | backend/internal/handler/lobby.go:306,345 | WS 链路追踪 | 已有 span，但用 context.Background() |
| WS Span 父子关联 | ❌ | backend/internal/handler/lobby.go:306,345 | WS 消息应继承 HTTP 升级 trace | 改用 r.Context() 派生 |
| 采样器配置 | ❌ | backend/internal/telemetry/telemetry.go:64-67 | 生产 AlwaysSample 会爆量 | ParentBased(TraceIDRatioBased(0.1)) |
| trace_id 注入审计日志 | ⚠️ | backend/internal/audit/audit.go:32,44 | 审计事件与链路关联 | 字段已定义，hub.go 调用未传 |

### B.2 关键发现

#### 发现 B-1: 审计日志丢失请求上下文，破坏不可否认性溯源链
- **状态**: ❌
- **证据**: backend/internal/game/hub.go:87,155（`audit.Log(context.Background(), ...)`）；对比 backend/internal/handler/admin.go:74,85,243（正确传 `ctx`）
- **企业原理**: 审计日志核心价值在于"谁在何时对何资源做了什么"。当 `room.create`/`room.delete` 使用 `context.Background()` 时，`request_id` 和 `trace_id` 字段为空，无法将该审计事件回溯到触发它的 HTTP 请求。SOC2/ISO27001 审计或安全事件调查中，这种"孤儿审计记录"无法作为证据链。商业代价：监管罚款可达全球营收 4%（GDPR）。
- **修复方案**: 1) 修改 `Hub.CreateRoom()` 和 `Hub.RemoveRoom()` 签名增加 `ctx context.Context`；2) 调用方（handler/lobby.go）传入 `r.Context()`；3) `audit.Log(ctx, ...)` 通过 `middleware.GetRequestID(ctx)` 和 `trace.SpanFromContext(ctx).SpanContext().TraceID()` 填充
- **学习价值**: 💼
- **工具选型**: 开源: OpenAuditLog / 自建 slog | 企业SaaS: Datadog Audit Trail | Go集成: slog + context
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 B-2: /metrics 端点无鉴权，暴露内部状态
- **状态**: ❌
- **证据**: backend/cmd/server/main.go:209（`r.Handle("/metrics", promhttp.Handler())`）；metrics.go:11-12 注释明确指出"restrict access via network policy"
- **企业原理**: `/metrics` 暴露 Go runtime、进程、DB 连接池、Redis 池、熔断器状态、业务量。攻击者可据此：① 通过 goroutine 数推断负载做时序攻击；② 通过 DB 池饱和度判断何时 DoS 最有效；③ 通过业务指标做竞品情报。
- **修复方案**: 方案 A（推荐）：基本认证包装 `promhttp.Handler`；方案 B：绑定单独端口 :9090；方案 C：K8s NetworkPolicy 限制只有 Prometheus Pod 可访问
- **学习价值**: 🔭
- **工具选型**: 开源: prometheus/exporter-toolkit | 企业SaaS: Grafana Cloud Agent | Go集成: promhttp.HandlerOpts{AuthHandler}
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 B-3: WebSocket 消息 Span 与 HTTP 入口链路断裂
- **状态**: ❌
- **证据**: backend/internal/handler/lobby.go:306（`telemetry.Tracer().Start(context.Background(), "ws.readPump."+msgTypeName, ...)`）；lobby.go:345 同样
- **企业原理**: 分布式追踪价值在于完整请求链路。WS 升级请求有 HTTP Span，但后续每条消息 Span 用 `context.Background()` 成为孤儿。Jaeger/Tempo 中 WS 消息链路无法关联到初始 HTTP 升级请求，P99 延迟分析缺失 WS 阶段数据。
- **修复方案**: 1) WS handler 中将 `r.Context()` 保存到 room/player 结构体；2) `readPump`/`writePump` 从保存的 context 派生 Span；3) WS 长连接 context 需在连接关闭时 cancel
- **学习价值**: 🔭
- **工具选型**: 开源: OpenTelemetry-Go | 企业SaaS: Jaeger/Tempo | Go集成: otel/trace
- **优先级**: 就业价值 中 × 实施复杂度 Medium

#### 发现 B-4: DBPoolAcquireDuration 指标声明但从未采集（死指标）
- **状态**: ❌
- **证据**: backend/internal/metrics/metrics.go:58-64 声明 `DBPoolAcquireDuration` 直方图；全局搜索 `DBPoolAcquireDuration.` 仅在 metrics.go 出现，无任何 `.Observe()` 调用；对比 `DBPoolAcquireCount.Inc()` 在 postgres.go:63 被调用
- **企业原理**: 池获取耗时是饱和度早期预警——连接池接近耗尽时，获取耗时先于错误率上升。声明但未采集的指标会：① Grafana 面板显示为空，误导运维认为"无数据=无问题"；② 增加 Prometheus 抓取开销；③ 暴露"声明即完成"反模式。
- **修复方案**: 1) `config.BeforeAcquire` 回调记录开始时间；2) `config.AfterAcquire` 回调计算耗时并 `metrics.DBPoolAcquireDuration.Observe(duration)`；3) 或用 `pgxpool.PoolStats().AcquireDuration()` 周期性 Observe
- **学习价值**: 📋
- **工具选型**: 开源: pgxpool | 企业SaaS: Datadog DBM | Go集成: pgxpool.BeforeAcquire/Acquire
- **优先级**: 就业价值 中 × 实施复杂度 Low

#### 发现 B-5: 缺少显式采样器配置，生产环境全量采样
- **状态**: ❌
- **证据**: backend/internal/telemetry/telemetry.go:64-67（`sdktrace.NewTracerProvider` 未传 `WithSampler`）；全局搜索 `ParentBased|TraceIDRatio|AlwaysSample|Sampler` 无匹配
- **企业原理**: OpenTelemetry 默认采样器 `ParentBased(AlwaysSample)` 即全量采样。1000 QPS 下每秒 1000 条 trace + DB/Redis 子 span，OTLP Collector 会爆盘。企业生产标准是头采样 1%-10% + 错误请求强制采样。忽视代价：可观测性后端存储成本失控，关键错误 trace 被淹没。
- **修复方案**: 
  ```go
  provider := sdktrace.NewTracerProvider(
      sdktrace.WithResource(res),
      sdktrace.WithSpanProcessor(bsp),
      sdktrace.WithSampler(sdktrace.ParentBased(
          sdktrace.TraceIDRatioBased(0.1),
      )),
  )
  ```
- **学习价值**: 🔭
- **工具选型**: 开源: OTel Collector tail-sampling | 企业SaaS: Datadog/Tempo 智能采样 | Go集成: sdktrace.WithSampler
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 B-6: LOG_FORMAT 环境变量承诺未兑现
- **状态**: ❌
- **证据**: backend/cmd/server/main.go:36（注释 `use LOG_FORMAT=text for local dev`）；全局搜索 `LOG_FORMAT` 仅此一处注释，代码中无 `os.Getenv("LOG_FORMAT")` 分支
- **企业原理**: DX 影响生产力。JSON 日志本地调试难以阅读，开发者要么忍受要么自行改代码，导致本地代码与生产不一致。企业级项目应支持 `LOG_FORMAT=text|json` 切换。
- **修复方案**: 
  ```go
  var handler slog.Handler
  if getEnv("LOG_FORMAT", "json") == "text" {
      handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
  } else {
      handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
  }
  ```
- **学习价值**: 📋
- **工具选型**: 开源: slog | 企业SaaS: N/A | Go集成: slog.NewTextHandler/NewJSONHandler
- **优先级**: 就业价值 低 × 实施复杂度 Low

---

## C. 弹性工程

### C.1 差距矩阵

| 检查项 | 状态 | 证据（文件:行号） | 企业必要性 | 修复方案 |
|--------|------|-------------------|------------|----------|
| 熔断器-PostgreSQL | ✅ | store/postgres.go:106 创建；:39,150,185,433,460,528,544 调用 | 防 DB 故障级联 | 已到位 |
| 熔断器-Redis | ✅ | store/redis.go:53 创建；:84,106 调用 | 防 Redis 故障级联 | 已到位 |
| 熔断器-Resend API | ❌ | auth/magiclink.go:72 创建 cb 字段，但全文件无 `s.cb.Execute` 调用；实际发邮件在 worker/email_worker.go:140-175 未走熔断器 | 外部 API 故障防护 | 在 sendEmail 包裹 cb.Execute |
| 熔断器-可观测性 | ✅ | circuitbreaker.go:42-46 | 运维告警 | 已到位 |
| 重试-DB 读 | ❌ | postgres.go:38,149,184,432,459,527,543,674 调用 retry.Do，但返回 fmt.Errorf 而非 retry.RetryableError | 瞬态故障自愈 | 区分可重试错误，包 RetryableError |
| 重试-Redis 读 | ❌ | redis.go:105,237 同上 | 同上 | 同上 |
| 重试-ExternalAPI | ❌ | retry.go:30 定义 ExternalAPIRetry，全仓库无生产调用 | 外部 API 瞬态故障 | 在 EmailWorker 使用 |
| 重试-幂等区分 | ✅ | postgres.go:37-53 withRetryRead/withRetryWrite | 防止重复写入 | 已到位 |
| 重试-Jitter | ✅ | retry.go:19,25,32 | 防惊群 | 已到位 |
| 超时-分层配置 | ✅ | timeout.go:20-39 Connect/Query/Request 三层 | 独立调优 | 已到位 |
| 超时-PG 应用 | ✅ | postgres.go:63; game/hub.go:111,171,204; room.go:271,489,547 | 已应用 | 已到位 |
| 超时-Redis 应用 | ✅ | redis.go:39-40,44 | 已应用 | 已到位 |
| 超时-WS 应用 | ✅ | handler/lobby.go:277,279,359,368,390 | 已应用 | 已到位 |
| 超时-HTTP 客户端 | ❌ | timeout.go:52-53 定义但全仓库无生产调用；email_worker.go:159 用 http.DefaultClient | 防慢调用耗尽 goroutine | 构造 http.Client{Timeout} |
| 超时-HTTP 服务端 | ⚠️ | main.go:358-364 硬编码 15s/15s/60s，未用 TimeoutConfig | 配置统一管理 | 改用 timeouts.HTTPRequestTimeout |
| 优雅降级 | ✅ | degradation.go:14-29；lobby.go:61,76,98,112,139,168; auth.go:100,129,146,186 | 部分功能可用 | 已到位 |
| 舱壁-WS 连接上限 | ✅ | hub.go:391-393 CanAcceptWSConnection + atomic | DoS 防御 | 已到位 |
| 舱壁-房间玩家上限 | ✅ | room.go:120 | 单房间资源隔离 | 已到位 |
| 舱壁-DB/Redis 池 | ✅ | postgres.go:79 MaxConns=25；redis.go:35 PoolSize=20 | 连接数上限 | 已到位 |
| 舱壁-请求类型隔离 | ❌ | REST/WS/Admin 共享同一 HTTP server + DB 池 + Redis 池 | 防某类请求耗尽共享资源 | 按 semaphore 分池 |

### C.2 关键发现

#### 发现 C-1: Resend 熔断器"实现到位但未装配"——外部 API 调用完全裸奔
- **状态**: ❌
- **证据**: internal/auth/magiclink.go:65-73 定义 `MagicLinkService.cb` 字段并调用 `NewResendBreaker()`；magiclink.go:78-144 `RequestMagicLink` 方法体中**无任何** `s.cb.Execute` 调用——`cb` 字段是死代码；internal/worker/email_worker.go:140-175 `sendEmail` 实际调用 Resend API，使用 `http.DefaultClient.Do(req)`，**无熔断器、无超时、无 ExternalAPIRetry**
- **企业原理**: Resend 是第三方 SMTP 中继，SLA 不受我方控制。Resend 不可用时 EmailWorker 重试 5 次（email_worker.go:113）每次阻塞等待，导致 `email:queue` Stream 堆积、Worker goroutine 占用内存。熔断器本应在连续失败 3 次后开路（circuitbreaker.go:82），让后续请求快速失败并直接进死信队列。商业代价：邮件队列堆积 → Redis 内存膨胀 → 影响认证流程 → 全局雪崩。
- **修复方案**: 1) 将 `MagicLinkService.cb` 移至 `EmailWorker`；2) `sendEmail` 中包裹 `w.cb.Execute(func() (any, error) { ... })`；3) 构造独立 `http.Client{Timeout: cfg.HTTPRequestTimeout}`；4) 用 `resilience.ExternalAPIRetry` 包裹 HTTP 调用
- **学习价值**: 🔭
- **工具选型**: 开源: sony/gobreaker（已引入）| 企业SaaS: Resend 自带重试 + DLQ | Go集成: gobreaker+v2
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 C-2: 重试机制形同虚设——RetryableError 在生产代码中零调用
- **状态**: ❌
- **证据**: internal/resilience/retry.go:17-33 定义三个重试策略；postgres.go:38,149,184,432,459,527,543,674 共 8 处调用 `retry.Do`；redis.go:105,237 共 2 处；**但**所有 10 处回调返回的都是 `fmt.Errorf(...)` 或 `cbErr`（普通 error），**无一处**使用 `retry.RetryableError` 包装；retry_test.go:104,121 仅在测试中使用；sethvargo/go-retry 语义：返回 RetryableError 才重试，普通 error 立即停止
- **企业原理**: `sethvargo/go-retry` 设计契约是"显式标记可重试错误"。生产代码返回普通 error 后 `retry.Do` 立即返回，配置的 3 次重试 + Jitter 退避**从未执行**。一次网络抖动导致的 `pgx.ErrConnBusy` 直接返回给用户。商业代价：瞬态故障转化为用户可见错误，登录/创建房间成功率下降，客服工单上升。这是"实现到位但未装配"的最隐蔽形式——代码看起来有重试，测试也通过，但生产环境从不重试。
- **修复方案**: 1) 定义 `isRetryable(err error) bool`：匹配 `pgx.ErrConnBusy`、`pgx.ErrTxCommitRollback`、`net.Error.Timeout()`、`syscall.ECONNRESET` 等；2) 在 `withRetryRead` 和内联 `retry.Do` 回调中：`if isRetryable(err) { return retry.RetryableError(err) } return err`；3) 写操作保持不重试，但 UPSERT 类幂等写可标记可重试；4) 增加集成测试：mock 一次失败 + 一次成功，断言 `attempts == 2`
- **学习价值**: 🎓
- **工具选型**: 开源: sethvargo/go-retry（已引入，需正确使用）| 企业SaaS: N/A | Go集成: cenkalti/backoff（替代方案）
- **优先级**: 就业价值 高 × 实施复杂度 Medium

#### 发现 C-3: HTTP 超时配置定义但未使用——EmailWorker 无超时可挂死
- **状态**: ❌
- **证据**: internal/config/timeout.go:32-33,52-53 定义 `HTTPConnectTimeout`（5s）和 `HTTPRequestTimeout`（10s）；全仓库搜索 `HTTPConnectTimeout`/`HTTPRequestTimeout`：仅 timeout.go 定义处和 timeout_test.go 测试，**无任何生产调用**；internal/worker/email_worker.go:159 `resp, err := http.DefaultClient.Do(req)`——`http.DefaultClient` 的 `Timeout` 字段为零值，即**无超时**；cmd/server/main.go:358-364 HTTP 服务端超时硬编码，未引用 `timeouts` 变量
- **企业原理**: Resend API 若发生 TCP 连接挂起（非拒绝，只是不响应），`http.DefaultClient.Do` 会**永久阻塞**。EmailWorker 的 `processMessage` goroutine 永远不返回，`XAck` 永远不调用，消息留在 PEL。随着失败消息累积，Worker 内存增长，最终 OOM。商业代价：邮件 Worker 静默死亡，用户收不到登录链接，无法登录游戏。
- **修复方案**: 1) `EmailWorker` 构造函数注入 `timeouts config.TimeoutConfig`；2) 构造 `w.httpClient = &http.Client{Timeout: timeouts.HTTPRequestTimeout}`；3) main.go:358-364 改为 `ReadTimeout: timeouts.HTTPRequestTimeout`；4) 可选用 `net.Dialer{Timeout: timeouts.HTTPConnectTimeout}` 配置 Transport
- **学习价值**: 💼
- **工具选型**: 开源: 标准库 net/http | 企业SaaS: N/A | Go集成: http.Client{Timeout, Transport}
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 C-4: 缺少请求类型舱壁隔离——共享资源池可被单类请求耗尽
- **状态**: ⚠️
- **证据**: cmd/server/main.go:227-321 所有路由注册到同一 chi.NewRouter()，同一 http.Server；postgres.go:79 `MaxConns = 25` DB 连接池被所有请求类型共享；redis.go:35 `PoolSize = 20`；无 semaphore、无 errgroup、无 goroutine 池隔离
- **企业原理**: 舱壁隔离核心是"故障域隔离"。管理员触发慢查询（如 ListLobbies 全表扫描 postgres.go:543）占满 25 个 DB 连接，普通用户 `/api/v1/auth/verify` 会因获取不到连接而超时。商业代价：一个慢操作拖垮全站，违反"关键路径优先"原则。
- **修复方案**: 1) 引入 `golang.org/x/sync/semaphore`，为请求类型分配独立配额：认证（10）、大厅（10）、Admin（3）、WebSocket saveState（2）；2) 或拆分 DB 池：关键路径独立小池（5），非关键路径共享大池（20）；3) chi 中间件层 `semaphore.Acquire(ctx)` / `defer Release()`
- **学习价值**: 🔭
- **工具选型**: 开源: golang.org/x/sync/semaphore | 企业SaaS: N/A | Go集成: ants goroutine 池
- **优先级**: 就业价值 中 × 实施复杂度 High

#### 发现 C-5: withRetryRead/withRetryWrite 辅助函数覆盖率低——样板代码重复
- **状态**: ⚠️
- **证据**: postgres.go:37-53 定义 withRetryRead/withRetryWrite；仅 3 处使用（:290, :317, :642）；其余 8+ 处（:38,149,184,432,459,527,543,674）内联 retry.Do + cb.Execute 样板代码，每处 8-12 行
- **企业原理**: 代码重复导致修改成本高——修复 C-2（RetryableError 包装）需要改 8+ 处而非 1 处。这也是 C-2 问题蔓延的原因。
- **修复方案**: 将所有内联的 `retry.Do + cb.Execute` 替换为 `withRetryRead`/`withRetryWrite` 调用，然后在辅助函数中集中修复 C-2
- **学习价值**: 📋
- **工具选型**: N/A
- **优先级**: 就业价值 低 × 实施复杂度 Low

---

## D. CI/CD 与 DevSecOps

### D.1 差距矩阵

| 检查项 | 状态 | 证据（文件:行号） | 企业必要性 | 修复方案 |
|--------|------|-------------------|------------|----------|
| lint (Go) | ✅ | .github/workflows/go-ci.yml:47-51 | 静态分析拦截缺陷 | 已用 golangci-lint-action@v6 |
| lint (前端) | ❌ | .github/workflows/ci-cd.yml:24-29（仅 tsc，无 eslint/biome）；frontend/package.json:6-11（无 lint 脚本） | 前端代码质量门禁缺失 | 增加 eslint/biome 步骤 |
| test (Go -race) | ✅ | .github/workflows/go-ci.yml:29 | 竞态检测 | 已启用 |
| test (前端单测+覆盖率) | ✅ | .github/workflows/ci-cd.yml:26-27（vitest run --coverage ≥80%） | 质量门禁 | 已启用 |
| integration / E2E | ✅ | .github/workflows/ci-cd.yml:28-29, 44-45, 66-67 | 端到端验证 | 已启用 Playwright |
| security scan (Go) | ✅ | .github/workflows/go-ci.yml:77-80 (govulncheck) | CVE 检测 | 已启用 |
| security scan (前端) | ❌ | .github/workflows/ci-cd.yml:14-29（无 npm audit / Snyk） | 前端供应链漏洞检测缺失 | 增加 npm audit / dependency-review |
| container scan | ✅ | .github/workflows/go-ci.yml:89-95 (Trivy) | 镜像漏洞门禁 | 已启用 |
| SAST (CodeQL) | ❌ | .github/workflows/ 全局无 codeql-action | 源码级漏洞检测缺失 | 增加 github/codeql-action |
| 镜像 tag 策略 | ✅ | .github/workflows/go-ci.yml:200-201 (`:${{ github.sha }}`) | 不可变 tag | 已用 git SHA |
| Dockerfile 基础镜像 pin | ⚠️ | Dockerfile:12,29,47（tag pin，非 @sha256） | SLSA L2 要求 digest pin | 运行 scripts/pin-digests.sh |
| build & push | ✅ | .github/workflows/go-ci.yml:177-203 | 自动化构建发布 | 已启用 |
| 镜像签名 (cosign) | ✅ | .github/workflows/go-ci.yml:204-207 | 防篡改 | 已启用 |
| SBOM 生成 | ✅ | .github/workflows/go-ci.yml:208-218 (anchore/sbom-action) | 合规与漏洞追踪 | 已启用 |
| SLSA provenance | ❌ | .github/workflows/go-ci.yml:204-218（仅 cosign sign，无 slsa-github-generator） | SLSA L3 构建来源证明 | 引入 slsa-github-generator |
| 变更日志自动生成 | ❌ | .github/release-please-config.json 存在，但无 release-please.yml workflow；CHANGELOG.md 手写 | 自动化版本与变更日志 | 新增 release-please.yml |
| 部署摘要自动生成 | ❌ | .github/workflows/ci-cd.yml:75-90（deploy 仅 wrangler deploy，无 summary） | 部署可观测性 | 增加 $GITHUB_STEP_SUMMARY |
| license 合规 | ✅ | .github/workflows/go-ci.yml:149-175 (go-licenses) | 法务合规 | 已启用 |
| Docker pin 校验 | ✅ | .github/workflows/go-ci.yml:131-147 | 防 mutable tag | 已启用 |
| migration 测试 | ✅ | .github/workflows/go-ci.yml:97-129 (up/down/idempotency) | 数据库变更安全 | 已启用 |
| shift-left (pre-commit) | ⚠️ | .pre-commit-config.yaml:17-21 引用 `.secrets.baseline`，但该文件不存在 | 提交前拦截密钥泄露 | 生成 .secrets.baseline |
| CODEOWNERS | ✅ | .github/CODEOWNERS:1-22 | 强制责任人评审 | 已配置 |
| branch protection | ❌ | 仓库无 .github/settings.yml | 强制 PR/CI 门禁 | 增加 settings.yml |
| deploy environment 保护 | ❌ | .github/workflows/ci-cd.yml:75-79（deploy job 无 environment:） | 生产部署需人工审批 | 增加 environment: production |
| Trivy action 版本 pin | ⚠️ | .github/workflows/go-ci.yml:90 (`aquasecurity/trivy-action@master`) | mutable action 供应链风险 | pin 到具体 SHA |

### D.2 关键发现

#### 发现 D-1: 后端 Go CI 流水线已达企业级（正向基线）
- **状态**: ✅
- **证据**: .github/workflows/go-ci.yml:29 (`go test ... -race -coverprofile`), :47-51 (golangci-lint), :64 (go vet), :77-80 (govulncheck), :89-95 (Trivy), :97-129 (migration), :131-147 (docker pin), :149-175 (license), :177-218 (build-push + cosign sign + SBOM)
- **企业原理**: 后端流水线覆盖 test/lint/vet/security/container-scan/migration/license/build/sign/SBOM 十一道门禁，且 build-push 通过 `needs:` 串联所有门禁，任一失败即阻断发布。这是 SLSA L2 的实践范本。
- **修复方案**: 无需修复，作为前端流水线的对标基线推广
- **学习价值**: 💼
- **工具选型**: 开源: golangci-lint + govulncheck + Trivy + cosign + anchore/sbom-action | 企业SaaS: Snyk / Aqua Security | Go集成: go test -race
- **优先级**: 就业价值 高 × 实施复杂度 Low（已实现，可作为简历亮点）

#### 发现 D-2: release-please 配置存在但 workflow 缺失，变更日志实际未自动化
- **状态**: ❌
- **证据**: .github/release-please-config.json:1-15 配置完整，但 Glob `.github/workflows/*.yml` 仅返回 ci-cd.yml 与 go-ci.yml，无 release-please.yml；CHANGELOG.md:1-6 为手写 "Keep a Changelog" 格式
- **企业原理**: release-please-config.json 单独存在不会触发任何自动化；CHANGELOG 当前靠人工维护，易遗漏。企业发布流程要求版本号、变更日志、Release Notes 与 git tag 三者自动联动，否则合规审计无法追溯每个版本的具体变更。
- **修复方案**: 新增 .github/workflows/release-please.yml，监听 main 分支 push，使用 googleapis/release-please-action@v4
- **学习价值**: 🎓
- **工具选型**: 开源: googleapis/release-please | 企业SaaS: GitHub Releases + semantic-release | Go集成: release-please release-type: go
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 D-3: .secrets.baseline 基线文件缺失，detect-secrets pre-commit hook 实际无法运行
- **状态**: ❌
- **证据**: .pre-commit-config.yaml:17-21 配置 detect-secrets 并指定 `args: ['--baseline', '.secrets.baseline']`，但 Glob `**/.secrets.baseline` 返回 "No file found"
- **企业原理**: detect-secrets 在无 baseline 文件时会报错退出，导致所有启用 pre-commit 的开发者提交被阻断，最终被迫 `--no-verify` 跳过——这使整个 shift-left 密钥防护形同虚设。密钥泄露是企业最高危事件之一（GitHub 2024 报告：83% 泄露在推送后 5 分钟内被自动化攻击者利用）。
- **修复方案**: 执行 `detect-secrets scan --exclude-files '\.git/.*' > .secrets.baseline` 生成基线，提交该文件；并在 CI 增加一步校验 baseline 是否与代码库同步
- **学习价值**: 🎓
- **工具选型**: 开源: Yelp/detect-secrets + .secrets.baseline | 企业SaaS: GitHub Secret Scanning + GitGuardian | Go集成: gitleaks
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 D-4: 前端 CI 缺少 lint、npm audit、SAST，shift-left 在前端栈断裂
- **状态**: ❌
- **证据**: .github/workflows/ci-cd.yml:24-29（quality-gate 仅 tsc + vitest，无 eslint/biome/npm audit/CodeQL）；frontend/package.json:6-11（scripts 仅 dev/build/preview/typecheck，无 lint 脚本）；Grep `npm audit|snyk|codeql` 在 .github/workflows/ 下 "No matches found"
- **企业原理**: 后端有 golangci-lint + govulncheck + Trivy 三层防护，前端仅 tsc 类型检查——XSS、原型链污染、依赖混淆攻击在前端无任何 CI 拦截。前端依赖一旦被投毒，直接进入用户浏览器。企业前端栈必须与后端对齐 shift-left 强度。
- **修复方案**: (1) frontend/package.json 增加 `"lint": "eslint ."` 与 `"audit": "npm audit --audit-level=high"`；(2) ci-cd.yml 增加 lint 与 npm audit 步骤；(3) 新增 codeql.yml 对 TS/Go 启用 CodeQL；(4) 增加 github/dependency-review-action
- **学习价值**: 💼
- **工具选型**: 开源: eslint + npm audit + GitHub CodeQL + dependency-review-action | 企业SaaS: Snyk / SonarQube | Go集成: golangci-lint（后端已有）
- **优先级**: 就业价值 高 × 实施复杂度 Medium

#### 发现 D-5: 仅有 cosign 签名，缺 SLSA provenance attestation，未达 SLSA L3
- **状态**: ❌
- **证据**: .github/workflows/go-ci.yml:204-207 仅 `cosign sign --yes`，无 `cosign attest --predicate` 或 slsa-framework/slsa-github-generator；Grep `slsa|provenance|attestation` 在 workflows 下仅命中 :139 注释提及 "SLSA L2" 目标
- **企业原理**: cosign sign 仅证明"此镜像由我签名"，不证明"此镜像由该 CI/该 commit/该 Dockerfile 构建"——后者需 provenance attestation（SLSA L3）。供应链攻击（如 SolarWinds）正是篡改构建过程而非签名。企业合规 increasingly 要求 SLSA L3 provenance 才允许部署。
- **修复方案**: (1) 引入 slsa-framework/slsa-github-generator/.github/workflows/generator_container-slsa3.yml@v2.0.0；(2) 或 build-push 后增加 `cosign attest --predicate <slsa-provenance.json> --type slsaprovenance`；(3) 部署侧增加 `cosign verify-attestation --type slsaprovenance` 校验
- **学习价值**: 🔭
- **工具选型**: 开源: slsa-framework/slsa-github-generator + cosign attest | 企业SaaS: Sigstore (ChainGuard) / Chainguard Images | Go集成: GoReleaser + cosign
- **优先级**: 就业价值 中 × 实施复杂度 Medium

#### 发现 D-6: deploy job 无 environment 保护，生产部署无人工审批门禁
- **状态**: ❌
- **证据**: .github/workflows/ci-cd.yml:75-79（deploy job 仅 `if: github.ref == 'refs/heads/main'`，无 `environment:` 字段）；仓库无 .github/settings.yml
- **企业原理**: 当前任何能 push 到 main 的人即可触发 `wrangler deploy` 直达生产，无人工审批、无部署窗口、无回滚审批。CODEOWNERS 仅在 PR 强制评审时生效，若无 branch protection 规则要求 PR + status check，开发者可直推 main 绕过一切。企业生产部署必须满足"四眼原则"与可回滚审计。
- **修复方案**: (1) ci-cd.yml deploy job 增加 `environment: production`；(2) .github/settings.yml 声明 branch protection：require PR、require status checks、require CODEOWNERS review、disallow force push；(3) deploy 步骤后增加 `$GITHUB_STEP_SUMMARY` 输出部署摘要
- **学习价值**: 💼
- **工具选型**: 开源: GitHub Environments + .github/settings.yml | 企业SaaS: GitHub Enterprise Branch Protection Rules | Go集成: 无（平台层）
- **优先级**: 就业价值 高 × 实施复杂度 Low

---

## E. 测试策略

### E.1 差距矩阵

| 检查项 | 状态 | 证据（文件:行号） | 企业必要性 | 修复方案 |
|--------|------|-------------------|------------|----------|
| 单元测试（Go） | ✅ | backend/internal/game/physics_test.go:22 等 36 个 `_test.go` 文件 | 防止回归 | 已覆盖核心包 |
| 集成测试（testcontainers） | ✅ | backend/tests/integration/postgres_test.go:34; redis_test.go:22; go.mod:18-20 | 真实 DB 行为 | 已用 testcontainers-go |
| 契约测试（WebSocket 协议） | ❌ | 全仓库无 contract 测试 | 前后端协议漂移 | 引入 Pact GO |
| E2E 测试 | ⚠️ | tests/e2e/gameplay.spec.ts:4; performance.spec.ts:4 仅 4 个 smoke 用例 | 用户旅程防线 | 增加完整流程用例 |
| 前端单元测试 | ❌ | package.json:20 配置 vitest 但 src/** 下零 .test.ts(x) 文件 | 前端逻辑无防护 | 添加 vitest 用例 |
| Table-Driven Tests | ✅ | apierror_test.go:13; cors_test.go:96 等 11+ 处 | Go 最佳实践 | 已广泛采用 |
| go test -race | ✅ | .github/workflows/go-ci.yml:29 CI 强制 | 并发缺陷检测 | 已在 CI 强制 |
| go test -bench | ⚠️ | 仅 physics_test.go:564-647 9 个 Benchmark；protocol/store/hub 零基准 | 防性能退化 | 为热路径增加 Benchmark |
| testcontainers 使用 | ✅ | go.mod:18-20; postgres_test.go:12-14 | 隔离性测试 | 已正确使用 |
| t.Parallel() | ⚠️ | auth_test.go:30 等 6 个文件采用；game/store/protocol 未使用 | 缩短 CI 时间 | 补 t.Parallel() |
| 测试覆盖率门槛 | ❌ | go-ci.yml:29 仅生成 coverage.out 无阈值 | 防破窗效应 | CI 加阈值检查 |

### E.2 测试欠债地图

| 模块 | 测试文件数 | 风险等级 | 缺失场景 |
|------|-----------|---------|---------|
| internal/worker（email + game_result） | 0 | 🔴 高 | 队列消费、重试退避、批量刷写、消费者组创建、错误处理 |
| internal/outbox（publisher） | 0 | 🔴 高 | 至少一次投递、processed_at 更新、Redis 发布失败回滚 |
| internal/audit（HMAC 链） | 0 | 🔴 高 | HMAC 链式哈希、篡改检测、异步落库、lastHash 并发更新 |
| internal/validate（XSS 防护） | 0 | 🔴 高 | 控制字符、零宽字符、HTML 特殊字符、长度截断 |
| internal/handler/lobby.go WebSocket | 0（仅 HTTP 测试） | 🔴 高 | WebSocket、readPump、writePump 完全未测 |
| internal/handler/auth.go 部分接口 | 1（仅 6/8 接口） | 🟡 中 | QuickPlay、ExportUserData、DeleteUserData 未测 |
| internal/game/hub.go 部分方法 | 1 | 🟡 中 | RestoreRooms、registerRoomInRedis、WS 连接计数器 |
| internal/idgen | 0 | 🟡 中 | UUID 生成唯一性 |
| internal/telemetry | 0 | 🟡 中 | OTel 初始化、exporter 配置 |
| cmd/server/main.go | 0 | 🟡 中 | 启动装配、优雅关闭 |
| 前端单元测试 | 0 | 🟡 中 | UI 状态机、物理预测、协议解析 |
| E2E 真实游戏流程 | 2（smoke） | 🟡 中 | 多玩家加入、点击、游戏结束、重启投票 |

### E.3 关键发现

#### 发现 E-1: 异步处理三件套（worker/outbox/audit）零测试，业务关键路径裸奔
- **状态**: ❌
- **证据**: backend/internal/worker/email_worker.go:44、game_result_worker.go:43、outbox/publisher.go:26、audit/audit.go:28 均无对应 `_test.go` 文件（Glob `backend/internal/{worker,outbox,audit}/*_test.go` 返回 0 结果）
- **企业原理**: 这三个包承载"邮件送达""游戏结果持久化""合规审计"三类业务关键语义。Outbox 模式是企业级 at-least-once 投递的标准实现，但其 `publishBatch` 逻辑（publisher.go:40）一旦在 Redis 发布成功但 PG `processed_at` 更新失败，会导致重复消费；无测试意味着任何重构都可能静默破坏数据一致性。审计 HMAC 链一旦 `lastHash` 并发更新有 race，整个合规链路作废，SOC2 审计无法通过。商业代价：游戏结果丢失=用户流失；审计链断裂=合规罚款。
- **修复方案**: 1) 为 outbox/publisher.go 添加 testcontainers 集成测试；2) 为 worker/email_worker.go 用 miniredis + httptest.NewServer 模拟 Resend API；3) 为 audit/audit.go 重点测 HMAC 链：插入 N 条记录验证 `this_hash = HMAC(secret, prev_hash || payload)`，篡改中间记录验证后续 hash 校验失败，用 `-race` 验证 `lastHash` 并发安全
- **学习价值**: 🔭
- **工具选型**: 开源: testcontainers-go + miniredis | 企业SaaS: Datadog CI Visibility | Go集成: go test -race -count=10
- **优先级**: 就业价值 高 × 实施复杂度 Medium

#### 发现 E-2: WebSocket 核心通信路径零测试，多人游戏命脉无防护
- **状态**: ❌
- **证据**: backend/internal/handler/lobby.go:196 `WebSocket`、:269 `readPump`、:353 `writePump` 三个核心方法在 lobby_test.go 中完全未测；hub.go:199 `RestoreRooms`、:349 `registerRoomInRedis`、:391 `CanAcceptWSConnection` 等 WS 连接管理方法在 hub_test.go 中未测
- **企业原理**: 多人网页游戏的命脉是实时 WebSocket 通信。`readPump`/`writePump` 处理心跳、断线、广播、并发写；任何 goroutine 泄漏或 panic 都会让整个房间卡死。WS 连接计数器是过载保护的关键，无测试意味着可能在流量峰值时失效。商业代价：线上房间卡死=玩家立即流失，且难以复现。
- **修复方案**: 1) 用 gorilla/websocket 的 `websocket.Dial` 配合 httptest.NewServer 编写 WS 集成测试；2) 用 sync.WaitGroup + 多 goroutine 模拟 N 个并发连接，验证 WSConnCount 准确性（配合 -race）；3) 为 RestoreRooms 用 testcontainers PG 构造持久化房间数据；4) 测试 writePump 在 Send channel 满时的背控行为
- **学习价值**: 💼
- **工具选型**: 开源: gorilla/websocket + httptest | 企业SaaS: k6 WebSocket load testing | Go集成: go test -race
- **优先级**: 就业价值 高 × 实施复杂度 High

#### 发现 E-3: 基准测试覆盖严重失衡，仅物理引擎有 Benchmark
- **状态**: ⚠️
- **证据**: Grep "func Benchmark" 仅在 physics_test.go:564-647 命中 9 个 Benchmark；protocol/encode_decode_test.go（每帧调用的二进制编码）、store/postgres_test.go（每局结束落库）、game/hub_test.go（房间创建热路径）均无 Benchmark
- **企业原理**: 多人游戏 60FPS 每帧调用 EncodeSnapshot，每局结束调用 SaveLobbyState，每场对局调用 CreateRoom。无基准意味着一次重构可能让编码耗时从 100ns 涨到 10μs 而无人察觉，直到线上 CPU 打满。企业级项目通常在 CI 跑 `go test -bench=. -benchmem` 并与基线对比（benchstat）。
- **修复方案**: 1) 为 protocol.EncodeSnapshot 添加 Benchmark；2) 为 game.Hub.CreateRoom、game.Room.HandleDisconnect 添加 Benchmark；3) 为 store.PostgresStore.SaveLobbyState 用 testcontainers 添加 Benchmark；4) .github/workflows/go-ci.yml 增加 bench job，用 benchstat 对比 main 分支基线
- **学习价值**: 📋
- **工具选型**: 开源: benchstat + act | 企业SaaS: Datadog CI Benchmarks | Go集成: go test -bench=. -benchmem
- **优先级**: 就业价值 中 × 实施复杂度 Low

#### 发现 E-4: 测试覆盖率无门禁，CI 仅生成不校验
- **状态**: ❌
- **证据**: .github/workflows/go-ci.yml:29-34 仅 `go test ... -coverprofile=coverage.out` 然后 `upload-artifact`，无任何 `go tool cover -func` 阈值检查
- **企业原理**: 覆盖率门禁是防止"破窗效应"的唯一手段。无门禁意味着新增代码可以零覆盖合入，半年后覆盖率从 60% 跌到 30% 而无人察觉。企业级项目通常设最低阈值（如 60%）并逐步上调。
- **修复方案**: 1) 在 go-ci.yml 的 test job 末尾添加阈值检查（建议起步 50%）；2) 用 Codecov/Coveralls 做趋势可视化与 diff 覆盖率评论；3) 每季度上调 5% 阈值，直至 75%
- **学习价值**: 📋
- **工具选型**: 开源: Codecov OSS | 企业SaaS: Codecov Pro / SonarQube | Go集成: go tool cover -func
- **优先级**: 就业价值 中 × 实施复杂度 Low

#### 发现 E-5: 前端零单元测试，vitest 配置形同虚设
- **状态**: ❌
- **证据**: package.json:20 声明 `vitest: ^4.1.9` 与 `@vitest/coverage-v8`，但 Glob `src/**/*.test.{ts,tsx}` 与 `frontend/**/*.test.*` 均返回 0 结果
- **企业原理**: 前端承载物理预测插值、UI 状态机、协议解析等关键逻辑。无任何单元测试意味着任何 CSS/状态重构都可能静默破坏游戏体验。企业级前端项目通常要求 60%+ 覆盖率。
- **修复方案**: 1) 为前端协议解析模块添加 vitest 用例；2) 为物理预测插值函数添加边界用例；3) package.json 添加 `"test:frontend": "cd frontend && vitest run --coverage"`；4) 新增 frontend-ci.yml
- **学习价值**: 💼
- **工具选型**: 开源: vitest + @testing-library | 企业SaaS: Chromatic | Go集成: 不适用
- **优先级**: 就业价值 中 × 实施复杂度 Medium

---

## F. API 设计成熟度

### F.1 差距矩阵

| 检查项 | 状态 | 证据（文件:行号） | 企业必要性 | 修复方案 |
|--------|------|-------------------|------------|----------|
| OpenAPI 完整性 | ⚠️ | docs/openapi.yaml:1-941 | API 契约是协作基础 | 补充缺失端点，CI 加 lint |
| API 版本化 (/v1/) | ✅ | backend/cmd/server/main.go:260,274,280,296,301 | 破坏性演进不影响旧客户端 | 已实现 |
| RFC 7807 错误 envelope | ✅ | backend/internal/apierror/apierror.go:9-22,67-71 | 统一错误格式 | 已实现 |
| 幂等性中间件 | ✅ | backend/internal/middleware/idempotency.go:72-117 | 防重复创建资源 | 已实现，TTL=24h |
| Cursor 分页 | ✅ | docs/openapi.yaml:422-478; lobby.go:154-191 | Offset 深页性能差 | 已实现 |
| HTTP 语义 | ⚠️ | auth.go:85; lobby.go:70; auth.go:77 | REST 成熟度标志 | OpenAPI 与代码 202 不一致需同步 |
| OpenAPI 与路由同步 | ❌ | docs/openapi.yaml:724 vs main.go:296; main.go:274-277 | 文档代码漂移 | 加入 CI 校验 |
| CORS 预检完整性 | ⚠️ | backend/internal/middleware/cors.go:22-23 | 缺 PATCH 方法 | 补全 PATCH 与 Idempotency-Key |
| 429 Retry-After 头 | ❌ | backend/internal/middleware/ratelimit.go:72,105,114 | 客户端无法知道何时重试 | 429 响应加入 Retry-After |
| OpenAPI CI 校验 | ❌ | .github/workflows/*（无 openapi 校验） | 无 CI 防护漂移 | 加入 spectral/redocly lint |

### F.2 关键发现

#### 发现 F-1: OpenAPI 与实际路由存在多处漂移（doc-code drift）
- **状态**: ❌
- **证据**: docs/openapi.yaml:724 文档路径 `/lobby/{code}/ws`，实际路由 `/api/v1/lobby/{code}/ws`（main.go:296-298）；main.go:274-277 注册 `/api/v1/user/data` 但 OpenAPI 完全未文档化；openapi.yaml:353-379 文档化 `/api/v1/registry/match` 但 main.go:280-292 未注册；auth.go:85 实际返回 202 但 openapi.yaml:100 声明 200；admin.go:208 请求体含 OldPassword 但 openapi.yaml:906-922 未文档化
- **企业原理**: API 契约文档是前后端协作的法律依据。Stripe/Twilio 用 OpenAPI 自动生成多语言 SDK；文档与代码漂移会导致客户端集成失败、SDK 生成错误、外部合作伙伴开发受阻。GDPR 端点（user/data）未文档化还可能引发合规审计问题。
- **修复方案**: 1) 修正 `/lobby/{code}/ws` → `/api/v1/lobby/{code}/ws`；2) 补充 `/api/v1/user/data` GET/DELETE 端点文档；3) 移除或标记 `/api/v1/registry/match` 为"未实现"；4) 修正 Magic Link 响应码 200 → 202；5) AdminConfigUpdate schema 补充 oldPassword；6) CI 加入 redocly lint；7) 长期从代码注解自动生成 OpenAPI
- **学习价值**: 🎓💼
- **工具选型**: 开源: Redocly CLI / Stoplight Spectral | 企业SaaS: Stoplight Platform / Postman | Go集成: swaggo/swag、danielgtaylor/huma
- **优先级**: 就业价值 高 × 实施复杂度 Medium

#### 发现 F-2: CORS 预检缺失 PATCH 方法与 Idempotency-Key 头
- **状态**: ❌
- **证据**: backend/internal/middleware/cors.go:22-23 硬编码 `Access-Control-Allow-Methods: "GET, POST, PUT, DELETE, OPTIONS"` 和 `Allow-Headers: "Content-Type, Authorization"`。但 main.go:312 注册 `PATCH /api/v1/admin/config`，idempotency.go:75 读取 `Idempotency-Key` 请求头
- **企业原理**: 浏览器对非简单请求（PATCH、自定义头）强制预检。当前配置下浏览器预检响应不含 PATCH，前端任何 `fetch('/api/v1/admin/config', {method:'PATCH'})` 都会被浏览器拦截（CORS 错误），管理员无法通过浏览器更新配置。同理 Idempotency-Key 头不在 Allow-Headers 中，前端创建房间时设置该头也会被拦截。这是隐性生产故障——本地测试（curl/Postman）不触发 CORS，仅在浏览器环境暴露。
- **修复方案**: 1) cors.go:22 改为 `"GET, POST, PUT, PATCH, DELETE, OPTIONS"`；2) cors.go:23 改为 `"Content-Type, Authorization, Idempotency-Key"`；3) 补充单元测试验证 PATCH 预检通过
- **学习价值**: 🎓💼
- **工具选型**: 开源: rs/cors | 企业SaaS: Cloudflare CORS | Go集成: go-chi/cors
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 F-3: 429 限流响应缺失 Retry-After 头
- **状态**: ❌
- **证据**: backend/internal/middleware/ratelimit.go:72,105,114 三处 `apierror.TooManyRequests(...).Write(w)` 均未设置 `Retry-After` 头；apierror.go:57-59 的 TooManyRequests 函数也不接受 Retry-After 参数
- **企业原理**: RFC 6585 §4 明确规定 429 响应 SHOULD 包含 `Retry-After` 头告知客户端何时重试。无此头时客户端只能盲目重试（轮询风暴）或放弃。Stripe/Twitter/GitHub 等企业 API 均在 429 响应中返回 Retry-After。忽视的商业代价：客户端重试风暴加剧服务端压力（限流反而引发雪崩）。
- **修复方案**: 1) 扩展 apierror.TooManyRequests 支持可选 retryAfter 参数；2) 或中间件中直接 `w.Header().Set("Retry-After", strconv.Itoa(int(cfg.Window.Seconds())))`；3) 可选补充 X-RateLimit-Limit、X-RateLimit-Remaining、X-RateLimit-Reset 头；4) 更新 OpenAPI 429 响应文档
- **学习价值**: 🎓💼
- **工具选型**: 开源: ulule/limiter | 企业SaaS: Cloudflare Rate Limiting | Go集成: didip/tollbooth
- **优先级**: 就业价值 中 × 实施复杂度 Low

#### 发现 F-4: API 设计成熟度整体优秀（亮点确认）
- **状态**: ✅
- **证据**: API 版本化 main.go:260,274,280,296,301；RFC 7807 apierror.go:9-22，:68 `Content-Type: application/problem+json`；幂等性 idempotency.go:72-117 实现 RFC 草案 Idempotency-Key，SHA256 哈希防注入，24h TTL，仅缓存 2xx，X-Idempotent-Replayed 头；Cursor 分页 lobby.go:154-191；HTTP 语义 auth.go:77 用 422，auth.go:85 用 202，lobby.go:70 用 409，main.go:312-319 PATCH vs PUT 区分（PUT 标记 deprecated + Sunset + Deprecation 头，符合 RFC 8594）
- **企业原理**: 这些是 REST API 成熟度 Richardson 模型 Level 2-3 的标志。Google API Design Guide、Microsoft REST API Guidelines 均要求这些实践。项目已达到企业级 API 设计标准。
- **修复方案**: 无需修复，保持现状
- **学习价值**: 📋🔭
- **工具选型**: 已使用最佳实践
- **优先级**: 就业价值 高 × 实施复杂度 N/A（已完成）

#### 发现 F-5: 缺乏条件请求（ETag/If-None-Match）与缓存策略
- **状态**: ⚠️
- **证据**: Grep 全 backend 目录无 `ETag`、`If-None-Match`、`If-Modified-Since` 匹配。lobby.go:88-135 的 CheckRoom 和 lobby.go:154-191 的 ListLobbies 均未设置 ETag 或 Last-Modified 头
- **企业原理**: 条件请求是 HTTP 缓存的核心机制。GitHub API、GitLab API 均为 GET 端点返回 ETag，客户端后续请求带 If-None-Match，服务端返回 304 节省带宽。对于高频轮询的房间列表，ETag 可显著降低数据库负载。
- **修复方案**: 1) 为 ListLobbies 响应体计算 SHA256 ETag；2) 中间件检查 If-None-Match，匹配则返回 304；3) 为 CheckRoom 设置 Last-Modified 头；4) OpenAPI 补充 304 响应文档
- **学习价值**: 🎓🔭
- **工具选型**: 开源: go-chi/chi 内置 Conditional GET | 企业SaaS: Cloudflare Cache | Go集成: hashicorp/golang-lru
- **优先级**: 就业价值 中 × 实施复杂度 Medium

---

## G. 安全深度

### G.1 差距矩阵

| 检查项 | 状态 | 证据（文件:行号） | 企业必要性 | 修复方案 |
|--------|------|-------------------|------------|----------|
| JWT 认证（HS256+密钥长度校验） | ✅ | backend/internal/auth/jwt.go:26-31 | 短密钥可被暴力破解 | 已强制 32 字节 |
| JWT 密钥轮换 | ✅ | backend/internal/auth/jwt.go:36-42 | 轮换瞬间不中断 | 已支持 primary/previous 双密钥 |
| JWT 撤销列表（jti） | ✅ | backend/internal/auth/middleware.go:48-92; redis.go:262-306 | 登出后 token 失效 | 已用 Redis SET+TTL |
| Refresh Token 轮换 | ✅ | backend/internal/auth/refresh.go:39-52; auth.go:211-229 | 长 TTL token 泄露风险 | 已实现验证即删除+重新签发 |
| Refresh Token 撤销（按用户） | ✅ | backend/internal/auth/refresh.go:75-98 | 密码修改时全量失效 | 已用 SCAN+DEL |
| RBAC 实现（Casbin） | ✅ | backend/internal/rbac/rbac.go:33-50, model.conf, policy.csv | 角色权限可审计 | 已用 Casbin |
| 角色来源防伪造 | ✅ | backend/internal/rbac/rbac.go:58-72; middleware.go:125-127 | X-User-Role 头可被伪造 | 已从 context 读取 |
| 端点权限声明覆盖 | ⚠️ | backend/cmd/server/main.go:260-321 | 未声明即默认放行 | user/lobby/registry 端点未挂 RBAC |
| Admin Token 撤销 | ❌ | backend/internal/handler/admin.go:330-352 | 24h 长 TTL token 无撤销路径 | 增加 jti 撤销检查 + admin logout |
| 安全 HTTP 头 | ✅ | backend/internal/middleware/security.go:32-60 | 防 SSL 剥离、点击劫持 | 已完整实现，nonce-based CSP |
| 限速维度（用户/IP/端点） | ✅ | backend/internal/middleware/ratelimit.go:91-152 | 防 DoS 与凭据填充 | 已实现三维复合 key |
| 限速中间件路由挂载 | ⚠️ | backend/cmd/server/main.go:262-265, 282-285, 302-305 | EndpointRateLimit 已实现但未启用 | 路由改用 EndpointRateLimit |
| Admin 登录锁定 IP 提取 | ❌ | backend/internal/handler/admin.go:61, 101, 105 | 反向代理后锁定失效 | 改用 middleware.extractClientIP |
| PII 字段级加密（Redis） | ✅ | backend/internal/auth/magiclink.go:108-111, 173-179 | Redis 泄露即暴露邮箱 | 已用 AES-256-GCM |
| PII 静态加密（PostgreSQL email） | ❌ | backend/migrations/000001_init_schema.up.sql:4 | DB dump 直接暴露邮箱 | 应用层加密或 PG 列加密 |
| 密钥存储（AES/Resend Key） | ✅ | backend/internal/crypto/aes.go:44-50; admin.go:243-248 | DB 泄露暴露第三方凭据 | 已 AES-256-GCM |
| Admin 密码存储 | ✅ | backend/internal/handler/admin_password.go:16-22 | 明文/弱哈希导致 DB 泄露即接管 | 已强制 bcrypt |
| 审计日志防篡改 | ✅ | backend/internal/audit/audit.go:108-135 | SOC2/ISO27001 不可否认性 | 已用 HMAC 链式哈希 |
| WebSocket Origin 校验 | ✅ | backend/internal/handler/lobby.go:210-229 | CSWSH 跨站 WebSocket 劫持 | 已校验 Origin==Host |
| 威胁建模文档 | ✅ | docs/threat-model.md:1-106 | 安全设计前置依据 | STRIDE 完整，需更新与代码同步 |
| GDPR 数据导出/删除 | ✅ | backend/internal/handler/auth.go:278-349 | 法定数据主体权利 | 已实现导出+匿名化+30天硬删除 |

### G.2 关键发现

#### 发现 G-1: 端点 RBAC 声明覆盖不完整
- **状态**: ⚠️
- **证据**: backend/cmd/server/main.go:260-321（仅 `/api/v1/admin/config` 的 GET/PATCH/PUT 挂载 RBAC 中间件，`/api/v1/user/data`、`/api/v1/registry/create`、`/api/v1/lobby/{code}/ws` 仅挂 AuthMiddleware）；rbac/policy.csv:7-9 定义了 `user, lobby, create/join/read` 但路由未引用
- **企业原理**: RBAC 策略若不在路由层强制执行，等于"有策略无执行"。企业合规审计（SOC2/ISO27001）要求每个端点可证明其权限声明。当前普通用户端点依赖 AuthMiddleware 的"已认证即可访问"假设，一旦未来新增敏感操作（如删除他人房间）将直接形成越权漏洞。
- **修复方案**: 1) 为所有 `/api/v1/user/*`、`/api/v1/registry/*`、`/api/v1/lobby/*` 路由追加 `rbacEnforcer.Middleware("lobby", "create")` 等声明；2) policy.csv 补充 `user, user_data, read/delete` 等策略；3) 增加 e2e 测试验证未授权角色返回 403
- **学习价值**: 💼
- **工具选型**: 开源: Casbin | 企业SaaS: Auth0 FGA / Ory Keto | Go集成: casbin/v2
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 G-2: Admin Token 缺失撤销机制
- **状态**: ❌
- **证据**: backend/internal/handler/admin.go:314-327（admin JWT 24h TTL，无 jti claim）；admin.go:330-352（VerifyAdminToken 不检查撤销列表）；main.go:301-321（无 admin logout 端点）
- **企业原理**: 普通 user token 有 15min TTL + jti 撤销列表，但 admin token 24h TTL 且无任何撤销路径。一旦 admin cookie 被 XSS 或网络拦截窃取，攻击者在 24 小时内可任意修改配置、轮换密码、长期接管账户。企业代价是管理员账户被接管后可修改 Resend API Key 投递钓鱼邮件，品牌信誉损失与 GDPR 违规双重风险。
- **修复方案**: 1) signAdminToken 增加 jti claim；2) VerifyAdminToken 调用 `redis.IsJWTRevoked`；3) 新增 `POST /api/v1/admin/logout` 端点调用 `redis.RevokeJWT`；4) 修改 admin 密码时撤销所有 admin token
- **学习价值**: 🔭
- **工具选型**: 开源: Redis SET+TTL | 企业SaaS: Redis Enterprise / Upstash | Go集成: go-redis/v9
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 G-3: 限速中间件实现完整但路由未启用
- **状态**: ⚠️
- **证据**: backend/internal/middleware/ratelimit.go:91-121（EndpointRateLimit 支持用户+IP+端点三维+FailClosed）；main.go:262-265, 282-285, 302-305（路由全部使用 `RateLimit` IP-only 版本，未使用 `EndpointRateLimit`）；DefaultEndpointRateLimits 定义了 `auth:quickplay`/`admin:login` 等 7 个端点配置（ratelimit.go:40-48）但从未被引用
- **企业原理**: 已实现的三维限流（用户+IP+端点）若不在路由挂载等于零防护。当前所有端点仅按 IP 限流，认证用户可绕过（同一 IP 后多用户共享配额或反之）。安全敏感端点（auth:quickplay、admin:login）的 FailClosed 配置形同虚设，Redis 宕机时凭据填充攻击无限制。商业代价是暴力破解与 DoS 直接成功。
- **修复方案**: 1) 将 main.go 中所有 `appMiddleware.RateLimit(redis, ...)` 替换为 `appMiddleware.EndpointRateLimit(redis, "auth:quickplay", jwtMgr)` 等；2) 为 `/api/v1/auth/quickplay`、`/api/v1/auth/verify`、`/api/v1/registry/create` 等端点补充 EndpointRateLimit；3) 增加 e2e 测试验证 FailClosed 行为
- **学习价值**: 💼
- **工具选型**: 开源: go-redis INCR+EXPIRE 滑动窗口 | 企业SaaS: Cloudflare Rate Limiting / Upstash Ratelimit | Go集成: github.com/go-redis/redis_rate
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 G-4: Admin 登录锁定使用错误 IP 来源
- **状态**: ❌
- **证据**: backend/internal/handler/admin.go:61（`clientIP := r.RemoteAddr`）；admin.go:66, 101, 105（锁定 key 全部基于 r.RemoteAddr）；对比 middleware/ratelimit.go:155-172（extractClientIP 正确处理 X-Forwarded-For）
- **企业原理**: 在 Cloud Run/nginx/Cloudflare 等反向代理后，`r.RemoteAddr` 恒为代理 IP，所有攻击者共享同一锁定 key。结果是：1) 单个攻击者 5 次失败后所有用户被锁定（DoS）；2) 或攻击者更换源 IP 但锁定永远不触发（暴力破解成功）。chi middleware.RealIP（main.go:231）会重写 RemoteAddr，但仅当 `X-Forwarded-For` 头存在且 chi 配置正确时生效，显式依赖此隐式行为是脆弱设计。商业代价是 admin 账户被暴力破解后全面接管。
- **修复方案**: 1) 在 admin.go 中导入并调用 `middleware.extractClientIP(r)`（需导出该函数）；2) 或直接复用 ratelimit.go:155-172 的逻辑；3) 增加测试用例验证 X-Forwarded-For 场景下锁定按真实客户端 IP 隔离
- **学习价值**: 🎓
- **工具选型**: 开源: chi middleware.RealIP | 企业SaaS: Cloudflare WAF Bot Management | Go集成: go-chi/chi/v5
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 G-5: PostgreSQL users.email 静态明文存储
- **状态**: ❌
- **证据**: backend/migrations/000001_init_schema.up.sql:4（`email VARCHAR(255) UNIQUE NOT NULL` 明文列）；对比 magiclink.go:108-111（Redis 中的 email 已 AES-256-GCM 加密）；docs/threat-model.md:63（PII 分类表标注 email 存储保护为"PostgreSQL（TLS 连接）"，未提加密）
- **企业原理**: TLS 仅保护传输，DB dump 或备份泄露直接暴露所有用户邮箱。Redis 中的临时 email 已加密（15min TTL），但 PostgreSQL 中的持久 email 明文存储，形成"临时数据加密、持久数据明文"的悖论。GDPR 第 32 条要求"适当的技术措施"保护 PII，明文存储在 DB 泄露事件中难以辩护。商业代价是 DB 备份泄露即触发 GDPR 72 小时通报义务，罚款上限为全球营收 4%。
- **修复方案**: 1) 应用层加密：在 `db.CreateUser`/`db.GetUserByEmail` 时调用 `crypto.Encrypt/Decrypt`；2) 或使用 PostgreSQL pgcrypto 扩展列级加密；3) 更新 threat-model.md PII 表标注"PostgreSQL（AES-256-GCM 应用层加密）"；4) 增加迁移脚本回填加密现有数据
- **学习价值**: 🔭
- **工具选型**: 开源: pgcrypto / 应用层 AES-GCM | 企业SaaS: AWS KMS / HashiCorp Vault Transit | Go集成: crypto/aes + crypto/cipher
- **优先级**: 就业价值 中 × 实施复杂度 Medium

#### 发现 G-6: 威胁建模文档与代码状态部分脱节
- **状态**: ⚠️
- **证据**: docs/threat-model.md:46（"Magic Link 5次/分钟，Admin 10次/5分钟"）与 middleware/ratelimit.go:41-48（`auth:request: 5/min, admin:login: 5/min`）数值不一致；threat-model.md:30（"未来需添加 audit log"）与 audit/audit.go:108-135（已实现 HMAC 链式审计日志）状态不一致；threat-model.md:76（"数据主体权利：需实现数据导出和删除功能（当前缺失）"）与 auth.go:278-349（已实现 ExportUserData/DeleteUserData）状态不一致
- **企业原理**: 威胁模型是安全决策的源头依据，文档与代码脱节会导致新成员基于错误信息做安全判断，审计时无法证明控制措施有效。企业合规要求"文档化的安全控制"与"实际实施"一致。
- **修复方案**: 1) 同步限速数值表；2) 更新 R（Repudiation）章节标注 audit log 已实现；3) 更新 GDPR 章节标注导出/删除已实现；4) CI 增加 threat-model.md 与 policy.csv/ratelimit.go 的同步检查（可选）
- **学习价值**: 📋
- **工具选型**: 开源: Markdown + Pre-commit hook | 企业SaaS: IriusRisk / ThreatModeler | Go集成: 无
- **优先级**: 就业价值 中 × 实施复杂度 Low

---

## H. 云原生与容器化

### H.1 差距矩阵

| 检查项 | 状态 | 证据（文件:行号） | 企业必要性 | 修复方案 |
|--------|------|-------------------|------------|----------|
| 多阶段构建 | ✅ | Dockerfile:12,29,47（三阶段） | 构建产物与工具链分离 | 已实现 |
| 基础镜像选型 | ⚠️ | Dockerfile:47（alpine:3.19.4 运行时） | alpine 仍有 shell，逃逸后可横向探索 | 改用 distroless |
| 非 root 用户 | ✅ | Dockerfile:49,55 | 容器逃逸后非 root 限制横向移动 | 已实现 |
| Digest pin（@sha256） | ❌ | Dockerfile:12,29,47（仅 tag pin） | SLSA L2 要求不可变引用 | 执行 scripts/pin-digests.sh |
| .dockerignore 完备 | ✅ | .dockerignore:1-60 | 防敏感文件进入构建上下文 | 已实现 |
| liveness probe | ✅ | infra/service.yaml:33-38 | K8s 重启死锁 Pod | 已实现 |
| readiness probe | ✅ | infra/service.yaml:39-43 | K8s 摘除不健康实例流量 | 已实现 |
| startup probe | ✅ | infra/service.yaml:27-32 | 区分"慢启动"与"死锁" | 已实现 |
| resources requests/limits | ✅ | infra/service.yaml:16-22 | 调度与噪声邻居隔离 | 已实现 |
| 水平扩展（无状态/会话外移） | ⚠️ | hub.go:33（rooms map 内存态）；infra/service.yaml 无 autoscaling 注解 | WS 长连接绑定 Pod | Redis Pub/Sub 跨实例广播 |
| PodDisruptionBudget | ✅ | infra/pod-disruption-budget.yaml:7 | 滚动更新期间保证可用 | 已实现 |
| 12-Factor 配置外置 | ✅ | main.go:61-70；infra/main.tf:55-88 | 配置外置使同镜像跑多环境 | 已实现 |
| 密钥 fail-fast 校验 | ✅ | main.go:72-92 | 静默降级到弱密钥=加密形同虚设 | 已实现 |

### H.2 关键发现

#### 发现 H-1: Docker 镜像未实际 pin digest，存在供应链攻击面
- **状态**: ❌
- **证据**: Dockerfile:12（`FROM node:20.18.0-alpine3.20 AS frontend-builder`）、Dockerfile:29（`FROM golang:1.26.0-alpine3.20 AS go-builder`）、Dockerfile:47（`FROM alpine:3.19.4`）；TODO 注释在 Dockerfile:10,27,45 明确写"replace with actual digest"但未执行
- **企业原理**: Docker Hub tag 是可变指针——同一 `node:20.18.0-alpine3.20` tag 可被覆盖推送恶意镜像。2024 年多处 npm/PyPI 供应链攻击通过 tag 投毒实现。SLSA Level 2 要求构建可重现，digest pin 是基础。忽视代价：一次 tag 投毒 = 全量生产环境被植入后门，企业面临 GDPR 罚款（最高 4% 全球营收）+ 客户信任崩塌。
- **修复方案**: 1) 运行 `bash scripts/pin-digests.sh`（已存在，scripts/pin-digests.sh:11-15）解析当前 digest；2) 将 Dockerfile 三处 FROM 改为 `image@sha256:<digest>` 格式；3) CI 已有 `docker-pin-check` job（go-ci.yml:131-147）但仅检查 `:latest`，需扩展为强制 `@sha256:` 校验
- **学习价值**: 🔭
- **工具选型**: 开源: Trivy + cosign | 企业SaaS: Snyk Container / Sigstore | Go集成: go-containerregistry
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 H-2: WebSocket 内存态导致水平扩展受限，infra/service.yaml 缺 autoscaling 注解
- **状态**: ⚠️
- **证据**: backend/internal/game/hub.go:33（`rooms map[string]*Room` 内存存储）；hub.go:86（`registerRoomInRedis` 仅注册元数据，未做消息桥接）；infra/service.yaml:1-44（Knative Service 无 `autoscaling.knative.dev/minScale/maxScale` 注解）；对比 root service.yaml:11-12（`uppy-clone` 服务有 maxScale=10 注解，但 infra/service.yaml 的 `balloon-game` 服务缺失）
- **企业原理**: 多人游戏 WebSocket 长连接绑定到具体 Pod。Pod 缩容 = 玩家被踢出房间 = 直接收入损失。当前 `registerRoomInRedis` 仅做发现，房间内消息广播仍走内存 channel，跨实例玩家无法同房游戏。企业代价：流量高峰无法扩容 → 玩家排队流失；单实例故障 → 该实例所有房间游戏中断。
- **修复方案**: 1) infra/service.yaml 添加 `autoscaling.knative.dev/minScale: "1"` 和 `maxScale: "10"`；2) hub.go 的 Room 引入 Redis Pub/Sub 广播层；3) WebSocket 前置 LB 启用 sticky session；4) 长远评估 Durable Objects / Agones
- **学习价值**: 💼
- **工具选型**: 开源: Redis Pub/Sub + NATS | 企业SaaS: Cloudflare Durable Objects / Agones | Go集成: go-redis PubSub + centrifugo
- **优先级**: 就业价值 高 × 实施复杂度 High

#### 发现 H-3: 双部署目标配置漂移（Cloud Run vs Cloudflare Workers），CI/CD 不一致
- **状态**: ⚠️
- **证据**: .github/workflows/ci-cd.yml:88（`npx wrangler deploy` 部署到 Cloudflare Workers，但仓库无 wrangler.toml/wrangler.jsonc）；go-ci.yml:177-218（build-push 构建 Docker 推送 GCR 并部署 Cloud Run）；cloudbuild.yaml:11-21（Cloud Run 部署）；infra/service.yaml:1（Knative Service `balloon-game`）；root service.yaml:4（Knative Service `uppy-clone`）——服务名都不一致
- **企业原理**: 同一仓库两套部署流水线 + 两个服务名（`uppy-clone` vs `balloon-game`）= 配置漂移。运维无法判断生产实际跑的是哪个版本、哪个镜像。企业代价：故障排查时误判部署目标，MTTR 翻倍；合规审计无法回答"生产运行什么代码"。
- **修复方案**: 1) 明确单一部署目标（推荐 Cloud Run，因已有完整 Docker + Terraform 链路）；2) 删除 ci-cd.yml:88 的 `wrangler deploy` 步骤；3) 统一服务名：将 root service.yaml 重命名为 service.yaml.legacy，infra/service.yaml 作为唯一权威；4) 在 README 或 .trae/specs/ 固化部署架构决策记录（ADR）
- **学习价值**: 📋
- **工具选型**: 开源: Argo CD（GitOps 单一事实源）| 企业SaaS: Cloud Run + Terraform Cloud | Go集成: terraform-exec
- **优先级**: 就业价值 中 × 实施复杂度 Medium

#### 发现 H-4: 运行时镜像使用 alpine 而非 distroless，攻击面未最小化
- **状态**: ⚠️
- **证据**: Dockerfile:47（`FROM alpine:3.19.4`）；Dockerfile:48（`apk --no-cache add ca-certificates` 表明 alpine 缺基础证书）
- **企业原理**: alpine 含 busybox shell + 包管理器 apk。容器逃逸后攻击者可直接 `apk add` 横向工具（curl/nmap/挖矿程序）。distroless 无 shell、无包管理器，攻击者即使 RCE 也无法轻易执行命令。NIST SP 800-190 推荐最小化镜像。企业代价：一次容器逃逸 = 攻击者在集群内自由横向移动。
- **修复方案**: 1) 运行时阶段改为 `FROM gcr.io/distroless/static-debian12:nonroot`；2) 删除 Dockerfile:48 的 `apk add ca-certificates`（distroless 已内置）；3) 删除 Dockerfile:49 的 `adduser`（distroless:nonroot 已内置 nonroot UID 65532）；4) `USER appuser` 改为 `USER nonroot:nonroot`；5) 验证 CGO_ENABLED=0 静态二进制可在 distroless 运行（Dockerfile:34 已设置）
- **学习价值**: 💼
- **工具选型**: 开源: distroless / scratch | 企业SaaS: Chainguard Images | Go集成: 静态编译天然适配 distroless
- **优先级**: 就业价值 中 × 实施复杂度 Low

---

## I. 数据库工程

### I.1 差距矩阵

| 检查项 | 状态 | 证据（文件:行号） | 企业必要性 | 修复方案 |
|--------|------|-------------------|------------|----------|
| 迁移版本化 | ✅ | backend/migrations/000001-000007_*.up.sql | 可追溯、可回滚的 schema 演进 | 已实现 7 位数字前缀 |
| Up/Down 配对 | ✅ | backend/migrations/（14 个文件，每个版本 up+down 齐全） | Down 迁移是回滚演练前提 | 已完整配对 |
| CI 回滚测试 | ✅ | .github/workflows/go-ci.yml:97-129（up→down→all 幂等性） | 防 Down 迁移缺失 | 已实现 |
| Down 迁移完整性 | ⚠️ | backend/migrations/000006_create_audit_logs.down.sql:1 | 残留对象污染回滚后 schema | DROP TABLE 前 DROP FUNCTION |
| EXPLAIN ANALYZE 实测 | ⚠️ | docs/db-query-analysis.md:19-24,39-42,55-58 | 仅"预期计划"无真实执行统计 | 在 staging 跑 EXPLAIN (ANALYZE, BUFFERS) |
| 复合索引列顺序 | ✅ | backend/migrations/000004_add_composite_indexes.up.sql:3-5 | 最左前缀决定索引可用性 | 顺序正确 |
| 冗余索引 | ❌ | 000001:30-32 + 000004:3-5 + 000001:4 | 重复索引浪费写入 I/O | 删除冗余索引 |
| 外键约束（DB 层） | ✅ | backend/migrations/000003_fk_cascade_and_checks.up.sql:9-22 | DB 层强制引用完整性 | 已加 ON DELETE CASCADE |
| CHECK 约束 | ✅ | backend/migrations/000003_fk_cascade_and_checks.up.sql:30-42 | 防御性深度 | status/score/taps/code 长度已约束 |
| 事务边界最小化 | ⚠️ | postgres.go:230-259,378-407；audit/audit.go:153-161 | 长事务持锁引发死锁 | 审计 channel 满即丢，合规场景需落库同事务 |
| 连接池配置存在 | ✅ | backend/internal/store/postgres.go:79-83 | 无界连接池会打满 PG max_connections | MaxConns=25/MinConns=5/Lifetime=30m |
| 连接池压测验证 | ❌ | postgres.go:71-78（注释自称 "Tuned" 但无证据）；Makefile:36-38 bench 目标产物不存在 | 未验证参数在大流量下可能池耗尽 | 加 Benchmark + k6/vegeta 压测 |
| 连接池可配置性 | ⚠️ | postgres.go:79-83（硬编码）vs redis.go:30-31（env 可配） | 生产环境无法不重新编译调参 | 改读 PG_POOL_MAX_CONNS 等环境变量 |

### I.2 关键发现

#### 发现 I-1: 冗余索引拖累写入路径
- **状态**: ❌
- **证据**: backend/migrations/000001_init_schema.up.sql:4 `email VARCHAR(255) UNIQUE NOT NULL`（UNIQUE 自动建索引）；000001:30 `CREATE INDEX idx_users_email ON users(email);`（与 UNIQUE 重复）；000001:31 `idx_sessions_lobby` 与 000004:4 `idx_game_sessions_lobby_status(lobby_code,status)` 重复；000001:32 `idx_results_session` 与 000004:3 `idx_game_results_session_user(session_id,user_id)` 重复；000002:9 `idx_lobby_states_updated_at` 与 000004:5 `idx_lobby_states_updated_code(updated_at DESC,code)` 重复；docs/db-query-analysis.md:71-72、89-90 自认"复合索引最左前缀也可用于 updated_at/session_id 条件"
- **企业原理**: 每个多余索引让 INSERT/UPDATE/DELETE 多一次 B-tree 维护。高写入场景下：① WAL 膨胀致复制延迟；② autovacuum 触发频繁；③ 缓冲池被冷索引挤占热数据。商业代价：DB CPU 与存储成本线性上升，故障恢复时间延长。
- **修复方案**: 1) 新增迁移 000008_drop_redundant_indexes.up.sql：`DROP INDEX idx_users_email, idx_sessions_lobby, idx_results_session, idx_lobby_states_updated_at;`；2) down.sql 重建被删索引；3) 删除 docs/db-query-analysis.md 中"单列索引"行
- **学习价值**: 💼
- **工具选型**: 开源: hypo/pg_qualstats/pg_stat_user_indexes | 企业SaaS: Datadog Database Monitoring | Go集成: pgxpool.Stat + 自研指标
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 I-2: 查询分析文档为"预期计划"，缺真实 EXPLAIN ANALYZE
- **状态**: ⚠️
- **证据**: docs/db-query-analysis.md:19-24、39-42、55-58、73-77、92-97 全部标注"预期计划"（伪计划），无 actual rows、buffers、execution time；Grep 全仓 `EXPLAIN ANALYZE` 无匹配
- **企业原理**: 优化器可能因统计信息陈旧、参数化值选择不同计划。仅看"预期"会误判索引命中。真实 EXPLAIN (ANALYZE, BUFFERS) 才能暴露：① 实际行数 vs 预估行数偏差；② 磁盘读 vs 缓存命中；③ Sort/Hash 峰值内存。忽视代价：上线后突现慢查询雪崩，DB 连接被耗尽，整个服务不可用。
- **修复方案**: 1) 在 staging 灌入生产量级数据（≥10 万行 game_results）；2) 对 5 个核心查询执行 `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`；3) 将结果固化为 docs/db-query-analysis.md 的"实测计划"小节；4) CI 加 pg_plan_checker 校验关键查询未走 Seq Scan
- **学习价值**: 🔭
- **工具选型**: 开源: psql EXPLAIN/deparse | 企业SaaS: pganalyze/SolarWinds DPA | Go集成: github.com/pganalyze/pg_query_go
- **优先级**: 就业价值 高 × 实施复杂度 Medium

#### 发现 I-3: 连接池参数无压测验证且硬编码
- **状态**: ❌
- **证据**: backend/internal/store/postgres.go:71-78 注释"Tuned values based on..."但无压测报告；postgres.go:79-83 五个参数全部硬编码字面量；Makefile:36-38 `bench` 目标输出到 docs/benchmarks-v2.md，但 Glob `docs/benchmarks*.md` 返回 No file found；Grep `Benchmark|testing.B` 在 backend/internal/store/ 下无任何匹配；对照 redis.go:30-31 Redis 池已用 env 变量，PG 未对齐
- **企业原理**: 25 连接是经验值，未验证。真实场景：① WebSocket 长连接业务下，每用户持有连接更久，25 可能瞬间打满；② PG max_connections=100 时 4 个实例 ×25 即撑爆。无压测=盲发。商业代价：流量峰值 5xx 雪崩，SLA 违约赔偿。
- **修复方案**: 1) 加 `BenchmarkPostgresStore_ConcurrentLoad(b *testing.B)`：N goroutine 并发 SaveLobbyState，断言 `pool.Stat().MaxConns()` 未触顶；2) 用 vegeta 或 k6 跑 1000 RPS 压测，记录 P99 延迟、pool_in_use、pool_idle；3) 将 MaxConns/MinConns/MaxConnLifetime 改读 env：`getEnvInt("PG_POOL_MAX_CONNS", 25)`；4) 压测报告固化为 docs/benchmarks-v2.md
- **学习价值**: 💼
- **工具选型**: 开源: vegeta/k6/hey | 企业SaaS: Grafana k6 Cloud/Locust | Go集成: testing.B + pgxpool.Stat
- **优先级**: 就业价值 高 × 实施复杂度 Medium

#### 发现 I-4: 审计日志异步丢弃与 Outbox 单行更新
- **状态**: ⚠️
- **证据**: backend/internal/audit/audit.go:154-160 `select { case ch <- ...: default: slog.Warn("dropping") }` —— channel 满即丢审计日志；audit.go:73 `make(chan dbEntry, 1024)` 硬上限；postgres.go:264-273 `CreateUser` 调用 `audit.Log` 在事务提交后（非同事务），审计与业务非原子；outbox/publisher.go:72-77 在 for rows.Next() 循环内逐行 `UPDATE outbox_events SET processed_at`，N 行 N 次往返
- **企业原理**: SOC2/ISO27001 要求审计日志完整不可丢。channel 满丢弃=合规违规证据缺失。Outbox 单行 UPDATE 在积压 1 万行时产生 1 万次 RTT，publisher 永远追不上写入速率，事件延迟从秒级退化到小时级。商业代价：合规审计失败、事件驱动业务数据不一致。
- **修复方案**: 1) 审计：channel 满时阻塞写入（带 100ms 超时）或落 WAL/本地文件兜底，绝不可丢；2) Outbox：改批量 `UPDATE outbox_events SET processed_at = $1 WHERE id = ANY($2)`，一次 RTT 标记整批；3) 关键审计（user.create/user.delete）改为同事务写入 audit_logs
- **学习价值**: 📋
- **工具选型**: 开源: PostgreSQL LISTEN/NOTIFY + pg_partman 分区 | 企业SaaS: Datadog Audit Logs/AWS CloudTrail | Go集成: pgx.Batch + OUTBOX 模式
- **优先级**: 就业价值 中 × 实施复杂度 Medium

---

## J. 工程文化与协作就绪

### J.1 差距矩阵

| 检查项 | 状态 | 证据（文件:行号） | 企业必要性 | 修复方案 |
|--------|------|-------------------|------------|----------|
| CONTRIBUTING.md（本地搭建） | ✅ | CONTRIBUTING.md:5-49 | 新人入职必备 | 已完整覆盖 |
| CONTRIBUTING.md（代码风格） | ✅ | CONTRIBUTING.md:51-55 | 统一风格减少 review 争议 | 已指向 golangci-lint + ESLint |
| CONTRIBUTING.md（PR 规范） | ✅ | CONTRIBUTING.md:100-112 | 强制 review 与 CI 门控 | 已明确分支策略+1 人 approve+CI 通过 |
| CONTRIBUTING.md（Conventional Commits） | ✅ | CONTRIBUTING.md:57-98 | 支持自动 CHANGELOG | 已配 commitlint+conventional-pre-commit |
| CONTRIBUTING.md（API Deprecation） | ✅ | CONTRIBUTING.md:132-149 | 保护 API 消费者 | 已规定 Deprecation/Sunset/Link 头+6 个月过渡期 |
| .editorconfig | ✅ | .editorconfig:1-21 | 跨编辑器/IDE 一致 | 已覆盖 Go/MD/Makefile |
| backend/.golangci.yml | ✅ | backend/.golangci.yml:1-67 | 自动化代码质量门控 | 已启用 15 个 linter |
| .pre-commit-config.yaml | ✅ | .pre-commit-config.yaml:1-44 | Shift-left | 已含 detect-private-key+detect-secrets+golangci-lint+go-test |
| commitlint.config.js | ✅ | commitlint.config.js:1-30 | 与 Conventional Commits 联动 | 已扩展 type-enum 含 security |
| CHANGELOG.md（Keep a Changelog） | ⚠️ | CHANGELOG.md:1-39 | 客户/运维/合规需追踪版本变更 | 仅 [Unreleased]，缺 [1.0.0] 等已发布版本块 |
| API deprecation 策略（OpenAPI 落地） | ✅ | docs/openapi.yaml:618,636,641 | 文档与实现一致 | 已在 adminUpdateConfigDeprecated 标记 |
| docs/runbook.md（on-call runbook） | ✅ | docs/runbook.md:1-281 | SRE 标配，缩短 MTTR | 已覆盖 5 类故障+P0-P3 分级+五段式排查 |
| .github/PULL_REQUEST_TEMPLATE.md | ❌ | .github/ 目录下无此文件 | 强制 PR 描述结构化 | 创建项目自身模板 |
| CODEOWNERS | ✅ | .github/CODEOWNERS:1-22 | 强制敏感目录 review | 已按基础设施/安全/DevOps/DB 分组 |
| Postmortem 模板（加分项） | ✅ | docs/templates/postmortem.md:1-52 | 标准化事故复盘 | 已含时间线/根因/Action Items |
| SLO 文档（加分项） | ✅ | docs/slo.md:1-128 | 用 Error Budget 驱动权衡 | 已含 SLI/SLO/SLA/Burn Rate |

### J.2 关键发现

#### 发现 J-1: CHANGELOG.md 仅含 [Unreleased]，未与 release-please 版本同步
- **状态**: ⚠️
- **证据**: CHANGELOG.md:8（`## [Unreleased]` 为唯一版本块）；CHANGELOG.md:11-39（仅 Added/Changed/Security 三类，缺 Fixed/Removed/Deprecated）；.github/release-please-manifest.json:2（`".": "1.0.0"` 已声明 1.0.0 版本）
- **企业原理**: CHANGELOG 是客户升级决策、运维回滚定位、合规审计的核心依据。release-please manifest 声明已发布 1.0.0，但 CHANGELOG 无对应版本块，意味着 1.0.0 的实际变更内容对消费者不可见；同时缺失 Fixed/Removed/Deprecated 分类违反 Keep a Changelog 1.1.0 规范，破坏版本契约可追溯性。商业代价：客户升级时无法评估 breaking changes，引发线上事故与支持工单。
- **修复方案**: 1) 触发一次 release-please PR，让其自动从 [Unreleased] 切出 `## [1.0.0] - 2026-XX-XX` 块；2) 人工补齐 Fixed/Removed/Deprecated 三类条目；3) CI 加入 `changelog-enforcer` 或 `kacl-cli` 校验
- **学习价值**: 💼
- **工具选型**: 开源: release-please + kacl-cli | 企业SaaS: semantic-release + Changesets | Go集成: go-licenses + goreleaser changelog
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 J-2: 项目根 .github/ 缺少 PULL_REQUEST_TEMPLATE.md
- **状态**: ❌
- **证据**: LS d:\Project\多人网页游戏\.github 仅含 workflows/、CODEOWNERS、dependabot.yml、release-please-config.json、release-please-manifest.json，无 PULL_REQUEST_TEMPLATE.md；唯一存在的模板位于 .superpowers/.github/PULL_REQUEST_TEMPLATE.md:1，但该目录是 vendored 的 superpowers 工具包，非项目自身模板
- **企业原理**: PR 模板强制贡献者填写"变更内容/原因/测试方法/影响面"，是 review 质量的前置门控。CONTRIBUTING.md:100-112 虽文字描述了 PR 规范，但缺少机器化模板意味着每个 PR 依赖人工记忆填写要素，导致 review 信息不全、来回追问、合并延迟。商业代价：review 周期拉长 30%+，关键问题（如未跑测试、未更新文档）漏检。
- **修复方案**: 1) 在 `.github/PULL_REQUEST_TEMPLATE.md` 创建模板，字段至少包含：Summary / Motivation / Changes / Test Plan / Checklist（含"已跑 go test -race"/"已跑 golangci-lint"/"已更新 CHANGELOG"/"已更新文档"复选项）；2) 可选：为不同变更类型创建多模板；3) CONTRIBUTING.md:100 PR 规范处补一行指向模板的链接
- **学习价值**: 📋
- **工具选型**: 开源: GitHub 内置 PR 模板 | 企业SaaS: GitLab Merge Request Templates + Danger.js | Go集成: 无（流程工具）
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 J-3: On-call Runbook 五段式覆盖完整，但缺少"故障 6: 认证/JWT 异常"与熔断器全局视图
- **状态**: ⚠️
- **证据**: docs/runbook.md:22-281 已覆盖 PostgreSQL（故障1）、Redis（故障2）、WebSocket 洪水（故障3）、tick 延迟（故障4）、磁盘/内存满（故障5）五类；CONTRIBUTING.md:11 提到 JWT refresh token + AES-256-GCM 加密，CHANGELOG.md:14-15 提到 JWT refresh token mechanism，但 runbook 无独立"认证服务异常"章节；runbook.md:81 仅在 Redis 故障中附带提及认证降级
- **企业原理**: 认证是用户进入游戏的第一道门，故障直接影响 100% 用户。当前 runbook 将认证故障分散在 Redis 章节内，缺少独立的密钥轮换失误、JWT 签名密钥泄露轮换、Magic Link 投递失败（Resend API 故障）等场景。企业中认证故障通常 P0/P1，缺独立 runbook 会导致 on-call 在密钥轮换事故中误判为 Redis 问题，MTTR 显著拉长。商业代价：认证故障每分钟损失活跃用户，且易引发安全合规事件。
- **修复方案**: 1) 新增"故障 6: 认证服务异常"章节，覆盖：(a) refresh token 验证失败率突增、(b) Magic Link 邮件投递失败（Resend API 熔断器 `circuit_breaker_state{name="resend"}` 排查）、(c) AES 密钥轮换失误导致配置解密失败；2) 在 runbook 顶部增加"熔断器全局视图"小节，列出所有 circuit_breaker_state 标签及对应故障章节跳转；3) 在 docs/slo.md:26-35 认证 SLO 处交叉引用 runbook 故障 6
- **学习价值**: 🔭
- **工具选型**: 开源: Prometheus + Alertmanager + runbook-rdme | 企业SaaS: PagerDuty Runbook Automation + FireHydrant | Go集成: go-circuit-breaker + custom runbook generator
- **优先级**: 就业价值 中 × 实施复杂度 Medium

#### 发现 J-4: Pre-commit 与 CI 形成完整 shift-left 防线，但 detect-secrets baseline 未在 CI 中校验
- **状态**: ⚠️
- **证据**: .pre-commit-config.yaml:17-21 已配置 detect-secrets 指向 `.secrets.baseline`；.secrets.baseline 文件存在（Glob 确认）；但 .github/workflows/go-ci.yml:1-218 与 ci-cd.yml:1-90 均无 detect-secrets 或 pre-commit run 的 CI 作业，意味着开发者若 `pre-commit install` 未执行，密钥可绕过本地检查直接入库
- **企业原理**: Pre-commit hook 是 opt-in 的（需开发者手动 `pre-commit install`），仅靠本地 hook 无法保证 100% 覆盖。企业要求在 CI 中复跑 pre-commit 或 detect-secrets 作为"最后一道防线"，否则绕过本地 hook 的密钥泄露将直接进入主干。商业代价：密钥泄露导致云账号被盗、数据被勒索，单次事件成本可达百万美元级。
- **修复方案**: 1) 在 .github/workflows/go-ci.yml 新增 `secrets-scan` 作业，使用 `pre-commit run detect-secrets --all-files` 或 `trufflehog github --org=<org>` 在 CI 中复跑；2) 在 README.md 或 CONTRIBUTING.md:114 Pre-commit Hooks 章节增加"首次 clone 后必须执行 `pre-commit install`"的强提示；3) 可选：启用 GitHub Secret Scanning + Push Protection
- **学习价值**: 🔭
- **工具选型**: 开源: pre-commit CI + detect-secrets + TruffleHog | 企业SaaS: GitHub Advanced Security Secret Scanning + GitGuardian | Go集成: gosec + govulncheck（已在 CI）
- **优先级**: 就业价值 高 × 实施复杂度 Low

#### 发现 J-5: 工程文化基础设施完整度达企业级 80% 基线
- **状态**: ✅
- **证据**: CONTRIBUTING.md:1-149（6 大章节齐全）；.editorconfig:1-21；backend/.golangci.yml:1-67（15 linter）；.pre-commit-config.yaml:1-44（5 类 hook）；commitlint.config.js:1-30；.github/CODEOWNERS:1-22；docs/runbook.md:1-281（5 类故障）；docs/templates/postmortem.md:1-52；docs/slo.md:1-128；docs/openapi.yaml:618,636,641（deprecation 落地）；.github/dependabot.yml:1-50（5 类依赖更新）
- **企业原理**: 企业级项目要求协作规范"文档化+机器化+流程化"三位一体。本项目在文档层（CONTRIBUTING/runbook/SLO/postmortem 模板）、机器化层（golangci-lint/pre-commit/commitlint/CODEOWNERS/dependabot）、流程层（CI quality gate+go-ci 八作业+release-please 自动版本）均已建立闭环，达到中型企业团队可直接接手的就绪度。
- **修复方案**: 维持现有体系，按 J-1/J-2/J-3/J-4 修补缺口后可达 90%+ 完整度。建议每季度 review 一次 CONTRIBUTING.md 与 runbook.md 与实际代码的漂移
- **学习价值**: 💼
- **工具选型**: 开源: 全套已选型合理 | 企业SaaS: 可升级至 Linear + Sentry + Datadog 闭环 | Go集成: golangci-lint + govulncheck + go-licenses 已集成
- **优先级**: 就业价值 高 × 实施复杂度 Low

---

## 学习价值地图

### 🎓 面试高频考点（系统设计/安全/行为面试中高频出现）

| 发现 | 主题 | 面试场景 |
|------|------|---------|
| A-1 | ADR 索引与文件错配 | 系统设计：架构治理 |
| A-3 | 分层架构边界破损 | 系统设计：DDD 分层 |
| A-4 | 文档与实现一致性 | 行为面试：技术债管理 |
| C-2 | RetryableError 零调用 | 系统设计：弹性工程 |
| D-2 | release-please workflow 缺失 | 行为面试：CI/CD 自动化 |
| D-3 | .secrets.baseline 缺失 | 安全面试：shift-left |
| F-1 | OpenAPI 与路由漂移 | 系统设计：API 契约 |
| F-2 | CORS 缺 PATCH 方法 | 全栈面试：浏览器安全 |
| F-3 | 429 缺 Retry-After 头 | API 设计面试 |
| G-4 | Admin 登录锁定 IP 错误 | 安全面试：反向代理 |

### 💼 工作中每天用到（入职前两周就会接触）

| 发现 | 主题 | 日常工作场景 |
|------|------|------------|
| A-1 | ADR 索引错配 | 新人 onboarding 查架构决策 |
| A-3 | 分层架构 | 每次写 handler 都涉及 |
| B-1 | 审计日志上下文 | 排查线上问题查日志 |
| C-3 | HTTP 超时配置 | 任何外部 API 调用 |
| D-1 | Go CI 流水线 | 每次 push 触发 |
| D-4 | 前端 CI 缺 lint | 前端每次提交 |
| D-6 | deploy environment 保护 | 每次生产部署 |
| E-2 | WebSocket 零测试 | 多人游戏核心命脉 |
| E-5 | 前端零单测 | 前端日常开发 |
| F-2 | CORS 缺 PATCH | 管理后台开发 |
| G-1 | RBAC 端点覆盖 | 每次新增端点 |
| G-3 | 限速中间件未启用 | 安全敏感端点 |
| H-2 | WS 内存态水平扩展 | 容量规划 |
| I-1 | 冗余索引 | DB 性能调优 |
| I-3 | 连接池压测 | 流量峰值排查 |
| J-1 | CHANGELOG 同步 | 每次版本发布 |
| J-5 | 工程文化基线 | 团队协作 |

### 🔭 高级工程师技能（Senior/Staff/Architect 级别才主导的工作）

| 发现 | 主题 | 高级技能场景 |
|------|------|------------|
| A-2 | 关键技术选型 ADR | Architect 级架构决策 |
| B-2 | /metrics 鉴权 | SRE 安全意识 |
| B-3 | WS Span 父子关联 | 分布式追踪深度 |
| B-5 | 采样器配置 | 可观测性成本治理 |
| C-1 | Resend 熔断器未装配 | 弹性工程深度 |
| C-4 | 请求类型舱壁隔离 | 故障域隔离设计 |
| D-5 | SLSA provenance | 供应链安全 L3 |
| E-1 | 异步三件套零测试 | 测试策略规划 |
| G-2 | Admin Token 撤销 | 安全深度设计 |
| G-5 | PostgreSQL email 明文 | 数据保护合规 |
| H-1 | Docker digest pin | 供应链不可变性 |
| I-2 | EXPLAIN ANALYZE 实测 | DB 性能工程 |
| J-3 | Runbook 认证章节 | SRE 成熟度 |
| J-4 | detect-secrets CI 校验 | shift-left 深度 |

### 📋 行业规范标准（有对应的 RFC/OWASP/白皮书/法规文件）

| 发现 | 规范 | 文档引用 |
|------|------|---------|
| A-1 | ADR 规范 | adr-tools |
| A-3 | DDD 分层架构 | Domain-Driven Design |
| A-4 | 文档与实现一致性 | 12-Factor |
| B-4 | Prometheus 指标规范 | OpenMetrics |
| B-6 | 日志格式规范 | 12-Factor 日志 |
| C-2 | 重试语义 | sethvargo/go-retry 文档 |
| C-5 | DRY 原则 | 代码重复 |
| D-2 | Keep a Changelog | https://keepachangelog.com |
| D-3 | detect-secrets | Yelp/detect-secrets |
| F-1 | OpenAPI 3.0 | https://spec.openapis.org |
| F-3 | RFC 6585 §4 | 429 Retry-After |
| F-4 | RFC 7807 / RFC 8594 | Problem Details / Deprecation |
| F-5 | HTTP 条件请求 | RFC 7232 |
| G-5 | GDPR 第 32 条 | PII 加密 |
| G-6 | STRIDE 威胁建模 | OWASP |
| H-3 | GitOps 单一事实源 | Argo CD |
| H-4 | NIST SP 800-190 | 容器最小化 |
| I-2 | EXPLAIN ANALYZE | PostgreSQL 文档 |
| I-4 | SOC2/ISO27001 | 审计日志完整性 |
| J-1 | Keep a Changelog 1.1.0 | https://keepachangelog.com |
| J-2 | GitHub PR 模板 | GitHub Community |

---

## 工具选型参考表

| 改造领域 | 开源方案 | 企业 SaaS 方案 | Go 集成方式 |
|---------|---------|---------------|------------|
| ADR 管理 | adr-tools / log4brains | Confluence ADR Template | 自研 CI 校验脚本 |
| 依赖注入 | Go 接口 + wire/fx | 无 | go/analysis 分层校验器 |
| 文档一致性 | markdownlint + 自研脚本 | Backstage TechDocs | 解析 constants.go 反向校验 |
| 可观测性日志 | slog + context | Datadog Audit Trail | slog + context |
| Metrics 鉴权 | prometheus/exporter-toolkit | Grafana Cloud Agent | promhttp.HandlerOpts{AuthHandler} |
| 分布式追踪 | OpenTelemetry-Go | Jaeger/Tempo | otel/trace |
| 池饱和监控 | pgxpool | Datadog DBM | pgxpool.BeforeAcquire/Acquire |
| 采样器 | OTel Collector tail-sampling | Datadog/Tempo 智能采样 | sdktrace.WithSampler |
| 熔断器 | sony/gobreaker（已引入） | Resend 自带重试 + DLQ | gobreaker+v2 |
| 重试 | sethvargo/go-retry（已引入） | N/A | cenkalti/backoff（替代） |
| HTTP 超时 | 标准库 net/http | N/A | http.Client{Timeout, Transport} |
| 舱壁隔离 | golang.org/x/sync/semaphore | N/A | ants goroutine 池 |
| CI/CD lint | golangci-lint + govulncheck + Trivy + cosign | Snyk / Aqua Security | go test -race |
| 版本发布 | googleapis/release-please | GitHub Releases + semantic-release | release-please release-type: go |
| 密钥扫描 | Yelp/detect-secrets + .secrets.baseline | GitHub Secret Scanning + GitGuardian | gitleaks |
| 前端安全 | eslint + npm audit + CodeQL + dependency-review | Snyk / SonarQube | golangci-lint（后端已有） |
| SLSA provenance | slsa-framework/slsa-github-generator + cosign attest | Sigstore / Chainguard Images | GoReleaser + cosign |
| Branch protection | GitHub Environments + .github/settings.yml | GitHub Enterprise Branch Protection Rules | 无（平台层） |
| 契约测试 | Pact GO | Pact Broker | 无 |
| E2E | Playwright（已引入） | Datadog CI Visibility | go test -race |
| 性能基准 | benchstat + act | Datadog CI Benchmarks | go test -bench=. -benchmem |
| 覆盖率门禁 | Codecov OSS | Codecov Pro / SonarQube | go tool cover -func |
| 前端测试 | vitest + @testing-library | Chromatic | 不适用 |
| API 契约 | Redocly CLI / Stoplight Spectral | Stoplight Platform / Postman | swaggo/swag、danielgtaylor/huma |
| CORS | rs/cors | Cloudflare CORS | go-chi/cors |
| 限流 | ulule/limiter | Cloudflare Rate Limiting | didip/tollbooth |
| 条件请求 | go-chi/chi 内置 Conditional GET | Cloudflare Cache | hashicorp/golang-lru |
| RBAC | Casbin（已引入） | Auth0 FGA / Ory Keto | casbin/v2 |
| Token 撤销 | Redis SET+TTL | Redis Enterprise / Upstash | go-redis/v9 |
| PII 加密 | pgcrypto / 应用层 AES-GCM | AWS KMS / HashiCorp Vault Transit | crypto/aes + crypto/cipher |
| 威胁建模 | Markdown + Pre-commit hook | IriusRisk / ThreatModeler | 无 |
| Docker digest | Trivy + cosign | Snyk Container / Sigstore | go-containerregistry |
| 水平扩展 | Redis Pub/Sub + NATS | Cloudflare Durable Objects / Agones | go-redis PubSub + centrifugo |
| GitOps | Argo CD | Cloud Run + Terraform Cloud | terraform-exec |
| 镜像最小化 | distroless / scratch | Chainguard Images | 静态编译适配 distroless |
| 索引分析 | hypo/pg_qualstats/pg_stat_user_indexes | Datadog Database Monitoring | pgxpool.Stat + 自研指标 |
| 查询分析 | psql EXPLAIN/deparse | pganalyze/SolarWinds DPA | github.com/pganalyze/pg_query_go |
| 负载测试 | vegeta/k6/hey | Grafana k6 Cloud/Locust | testing.B + pgxpool.Stat |
| 审计日志 | PostgreSQL LISTEN/NOTIFY + pg_partman | Datadog Audit Logs/AWS CloudTrail | pgx.Batch + OUTBOX 模式 |
| CHANGELOG | release-please + kacl-cli | semantic-release + Changesets | go-licenses + goreleaser changelog |
| PR 模板 | GitHub 内置 PR 模板 | GitLab Merge Request Templates + Danger.js | 无（流程工具） |
| Runbook | Prometheus + Alertmanager + runbook-rdme | PagerDuty Runbook Automation + FireHydrant | go-circuit-breaker + custom runbook generator |

---

## 优先级矩阵

### 第一象限：高就业价值 × Low 实施复杂度（立即执行，最高 ROI）

| 发现 | 主题 | 预期收益 |
|------|------|---------|
| C-2 | RetryableError 包装 | 瞬态故障自愈，登录/创建房间成功率提升 |
| C-3 | EmailWorker HTTP 超时 | 防 Worker 静默死亡 |
| D-2 | release-please workflow | 版本与变更日志自动联动 |
| D-3 | .secrets.baseline 生成 | shift-left 密钥防护生效 |
| D-6 | deploy environment 保护 | 生产部署四眼原则 |
| F-2 | CORS 补 PATCH 方法 | 管理后台浏览器可用 |
| G-1 | RBAC 端点覆盖 | 每端点权限可证明 |
| G-2 | Admin Token 撤销 | 24h token 可撤销 |
| G-3 | EndpointRateLimit 启用 | 三维限流生效 |
| G-4 | Admin 锁定 IP 修正 | 反向代理后锁定有效 |
| H-1 | Docker digest pin | 供应链不可变性 |
| I-1 | 删除冗余索引 | 写入性能提升 |
| J-1 | CHANGELOG 同步 | 版本契约可追溯 |
| J-2 | PR 模板创建 | review 质量前置门控 |
| B-1 | 审计日志上下文 | 不可否认性溯源链 |
| B-2 | /metrics 鉴权 | 内部状态不暴露 |
| B-5 | 采样器配置 | 可观测性成本可控 |
| A-1 | ADR 索引重写 | 决策可追溯 |
| A-4 | 文档与实现同步 | 容量规划准确 |

### 第二象限：高就业价值 × Medium 实施复杂度（规划执行）

| 发现 | 主题 | 预期收益 |
|------|------|---------|
| A-3 | 分层架构重构 | 可测试性与可维护性 |
| C-2 | RetryableError 包装（8+ 处） | 瞬态故障自愈 |
| D-4 | 前端 CI shift-left | 前端安全对齐后端 |
| E-1 | 异步三件套测试 | 业务关键路径防护 |
| F-1 | OpenAPI 与路由同步 | API 契约可信 |
| I-2 | EXPLAIN ANALYZE 实测 | 慢查询可预测 |
| I-3 | 连接池压测+env 化 | 流量峰值可承载 |

### 第三象限：高就业价值 × High 实施复杂度（长期规划）

| 发现 | 主题 | 预期收益 |
|------|------|---------|
| E-2 | WebSocket 测试覆盖 | 多人游戏命脉防护 |
| H-2 | WS 水平扩展 | 流量峰值可扩容 |

### 第四象限：中/低价值 × Low/Medium 复杂度（择机实施）

| 发现 | 主题 | 预期收益 |
|------|------|---------|
| B-3 | WS Span 父子关联 | WS 链路追踪完整 |
| B-4 | DBPoolAcquireDuration 采集 | 池饱和早期预警 |
| B-6 | LOG_FORMAT 切换 | DX 改善 |
| C-4 | 请求类型舱壁隔离 | 故障域隔离 |
| C-5 | withRetry 辅助函数收敛 | 代码可维护性 |
| D-5 | SLSA provenance | SLSA L3 合规 |
| E-3 | 基准测试覆盖 | 性能退化可感知 |
| E-4 | 覆盖率门禁 | 防破窗效应 |
| E-5 | 前端单测 | 前端逻辑防护 |
| F-3 | 429 Retry-After | 客户端重试友好 |
| F-5 | ETag 条件请求 | 带宽与 DB 负载降低 |
| G-5 | PostgreSQL email 加密 | PII 保护合规 |
| G-6 | 威胁建模同步 | 安全决策依据准确 |
| H-3 | 部署目标统一 | 配置不再漂移 |
| H-4 | distroless 镜像 | 攻击面最小化 |
| I-4 | 审计日志不丢+Outbox 批量 | 合规与性能 |
| J-3 | Runbook 认证章节 | 认证故障 MTTR 降低 |
| J-4 | detect-secrets CI 校验 | shift-left 闭环 |

---

## 执行顺序建议

### 阶段一：立即修复（1-2 天，Low 复杂度高 ROI）

1. **C-2** RetryableError 包装（一处辅助函数修改）
2. **C-3** EmailWorker HTTP 超时（构造 http.Client）
3. **G-3** EndpointRateLimit 启用（路由替换）
4. **G-4** Admin 锁定 IP 修正（改用 extractClientIP）
5. **D-3** .secrets.baseline 生成（一条命令）
6. **F-2** CORS 补 PATCH 方法（一行修改）
7. **B-1** 审计日志上下文（hub.go 增 ctx 参数）
8. **B-2** /metrics 鉴权（基本认证包装）
9. **B-5** 采样器配置（WithSampler 选项）
10. **H-1** Docker digest pin（运行 pin-digests.sh）
11. **I-1** 删除冗余索引（一个迁移文件）
12. **A-1** ADR 索引重写
13. **A-4** 文档与实现同步（修正 60fps→15Hz 等）
14. **J-1** CHANGELOG 同步（触发 release-please）
15. **J-2** PR 模板创建（一个文件）
16. **G-1** RBAC 端点覆盖（路由追加中间件）
17. **G-2** Admin Token 撤销（增加 jti + logout 端点）
18. **D-2** release-please workflow（一个 yml 文件）
19. **D-6** deploy environment 保护（增加 environment 字段）

### 阶段二：规划执行（1-2 周，Medium 复杂度）

1. **C-1** Resend 熔断器装配（cb 移至 EmailWorker）
2. **C-5** withRetry 辅助函数收敛（消除样板代码）
3. **A-3** 分层架构重构（引入 service 层）
4. **D-4** 前端 CI shift-left（eslint + npm audit + CodeQL）
5. **E-1** 异步三件套测试（worker/outbox/audit）
6. **F-1** OpenAPI 与路由同步（+ CI 校验）
7. **I-2** EXPLAIN ANALYZE 实测（staging 数据）
8. **I-3** 连接池压测 + env 化

### 阶段三：长期规划（月级，High 复杂度）

1. **E-2** WebSocket 测试覆盖（多人游戏命脉）
2. **H-2** WS 水平扩展（Redis Pub/Sub 广播层）
3. **C-4** 请求类型舱壁隔离（semaphore 分池）

### 阶段四：择机实施（低优先级）

- B-3 WS Span 父子关联
- B-4 DBPoolAcquireDuration 采集
- B-6 LOG_FORMAT 切换
- D-5 SLSA provenance
- E-3 基准测试覆盖
- E-4 覆盖率门禁
- E-5 前端单测
- F-3 429 Retry-After
- F-5 ETag 条件请求
- G-5 PostgreSQL email 加密
- G-6 威胁建模同步
- H-3 部署目标统一
- H-4 distroless 镜像
- I-4 审计日志不丢 + Outbox 批量
- J-3 Runbook 认证章节
- J-4 detect-secrets CI 校验

---

## 阶段门控

**Phase 1 审计报告已完成并输出至 `docs/audit-enterprise.md`。**

**等待用户确认后方可启动 Phase 2：**
- Task 13: 产出 `tasks-enterprise.md`（将审计改造项转化为有序可验证任务清单）
- Task 14: 产出 `checklist-enterprise.md`（为每个改造项生成验证检查点）

**用户明确批准前不修改任何业务代码。**