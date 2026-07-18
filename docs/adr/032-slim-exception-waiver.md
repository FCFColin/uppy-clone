# ADR-032: 瘦身例外条款豁免 (Slim-Down Exception Waiver)

## 状态: 已接受 (2026-07-18)

> 本 ADR **追溯授权** `deep-arch-slim-v2` spec 对 ADR-000 "刻意保留清单" 中
> 特定条目的裁剪。它是 ADR-000 章程的**豁免条款**，而非对章程本身的修订——
> 章程的"刻意保留清单"仍然有效，本 ADR 仅为 `deep-arch-slim-v2` 这一特定
> 瘦身 spec 开具定向豁免。

## 上下文

### 历史背景

ADR-000 (2026-06-25) 定义了"刻意保留的企业级组件清单"，明确这些组件是
"有意为之的练习目标，评审/自检时**不得**判定为过度工程"，并要求任何对清单
内组件的裁剪必须"作为**对章程的修订提案**提出"。

前轮 `slim-tier1-ef-and-materialize` 严格遵守此约束：HMAC 审计链验证器
(Task 13) 与 Idempotency 前端 header (Task 14) 被当作"企业级落实"添加，
而非删除。

### 触发事件

`deep-arch-slim-v2` spec (2026-07-18) 基于 5 份并行分析报告提出：用户判断
"69,072 行对一个 balloon tapping 网页游戏仍然过多"，并要求覆盖架构级 / 文件级 /
测试级 / 基础设施级 / 错误修复 5 个维度的深度瘦身。其中架构级维度 (维度一)
直接触及 ADR-000 "刻意保留清单"中的多项组件。

用户原话定位：
> "这个代码库的又不是多复杂，但是它有一个很广大的架构。这个生产级、企业级
> 的架构是故意的，但是就算有这种大型应用的架构，也不应该要好几万行代码才能
> 运行啊，这不正常对吧？"

### 需要豁免的具体组件

`deep-arch-slim-v2` spec 维度一 (架构级简化) 中以下条目与 ADR-000 "刻意保留清单"
直接冲突，需要本 ADR 豁免：

1. **多区域路由层** (`game/hub_multiregion.go` + `handler/lobby_ws_proxy.go`
   + `handler/resolve.go` + `domain/room_directory.go` + migrations 000017/000018/000019)
   — ADR-000 列入"多区域拓扑 + 全局就近路由 (ADR-014)"。
2. **HMAC 审计链** (`audit/audit_chain_verifier.go` + `audit_deadletter.jsonl`,
   激进版含 `audit.go` 简化) — ADR-000 列入"审计日志防篡改"。
3. **事务性 outbox 独立 worker 进程** (`worker/runner.go`, 激进版含
   `outbox/publisher.go`) — ADR-000 列入"事务性 Outbox + Redis Stream 消息队列"。
4. **Idempotency 中间件** (`middleware/idempotency*.go` + 前端 header) —
   ADR-000 列入"熔断 / 隔板 / 幂等 / 限流 全套弹性中间件"。
5. **Bulkhead 中间件** (`middleware/bulkhead.go`) — ADR-000 列入"熔断 / 隔板 /
   幂等 / 限流 全套弹性中间件"。
6. **OpenAPI 一致性测试** (`handler/openapi_consistency_test.go` +
   `handler/openapi_admin_consistency_test.go`) — 属"工程基线：CI 守门"范畴。

## 决策

### 1. 定向豁免 (Targeted Waiver)

本 ADR 对上述 6 项组件**仅授权 `deep-arch-slim-v2` spec 这一特定瘦身行动**
进行裁剪。豁免是**定向、一次性、不可外推**的：

- **定向**：仅适用于上述 6 项组件，不波及 ADR-000 "刻意保留清单"中的其他条目
  (owner 反向代理 + 租约接管、GDPR 硬删除 Worker、威胁模型、OTel、Pyroscope、
  SLO/告警、熔断 (circuit breaker)、限流 (rate limit)、多路径认证、Redis Stream
  消息队列本身、轻量 RBAC 等)。
- **一次性**：仅授权 `deep-arch-slim-v2` spec 执行期间。后续任何 spec 想再次
  裁剪"刻意保留清单"中的组件，必须再开具新的 ADR。
- **不可外推**：不得以本 ADR 为先例推断"清单内其他组件也可裁剪"。

### 2. 保留与裁剪的边界

| 组件 | ADR-000 状态 | 本 ADR 决策 | 理由 |
|---|---|---|---|
| 多区域路由层 | 保留 (目标态) | **豁免裁剪** | migration 000018 注释自承"多区域路由层已永久放弃"，000019 又改主意重建——典型的"简历驱动开发"反复横跳。用户从未真正部署多区域。 |
| HMAC 审计链验证器 | 保留 | **豁免裁剪** | 网页游戏不需要 SOC2/ISO27001 防篡改审计链。`VerifyAuditChain` 从未被业务消费方读取过。`audit_deadletter.jsonl` 已沦为运行时垃圾。 |
| 事务性 outbox 独立 worker | 保留 | **豁免裁剪** | `EnableEmbeddedWorkers` 默认 true 即证据——独立 worker 进程从未被部署。outbox 表仅 2 处写入。 |
| Idempotency 中间件 | 保留 | **豁免裁剪** | balloon 游戏客户端无 POST 创建付费/订单类操作。Idempotency-Key 是支付系统模式。代码自承核心路径无测试覆盖。 |
| Bulkhead 中间件 | 保留 | **豁免裁剪** | rate limiter + WSLimiter 已足够。游戏在 Redis 故障时本就不可用，bulkhead 隔离无意义。 |
| OpenAPI 一致性测试 | 保留 (CI 守门) | **豁免裁剪** | 项目根目录未分发 `openapi.yaml` 实际合约——这些测试是"为 OpenAPI 而 OpenAPI"。 |
| **owner 反向代理 + 租约接管** | 保留 | **不豁免** (仍保留) | "实例可寻址 + 有状态房间水平扩展"的练习核心，ADR-005。 |
| **GDPR 硬删除 Worker** | 保留 | **不豁免** (仍保留) | 明确为学习目标。 |
| **熔断 (circuit breaker)** | 保留 | **不豁免** (仍保留) | 弹性栈核心。 |
| **限流 (rate limit)** | 保留 | **不豁免** (仍保留) | 弹性栈核心。 |
| **OTel / Pyroscope / SLO** | 保留 | **不豁免** (仍保留) | 可观测性栈核心。 |
| **多路径认证 / RBAC** | 保留 | **不豁免** (仍保留) | ADR-026。 |

### 3. 章程约束的修订机制

ADR-000 附录 B 第 4 条原文：
> "若认为某个'刻意保留'组件确不合理，应作为**对章程的修订提案**提出，而非直接判为缺陷。"

本 ADR 即依此条款提出的**修订提案**，并已被用户接受。后续 `CONTRIBUTING.md`
第 83 行的章程约束描述应更新为引用本 ADR 作为豁免机制。

### 4. BREAKING 项的不可逆性确认

本 ADR 明确确认以下裁剪的不可逆性，并接受其后果：

- **多区域路由层全删后无法恢复**（需重写）
- **HMAC 审计链激进版删除后失去防篡改能力**
- **Idempotency 中间件删除后，未来若加支付功能需重新实现**
- **migration 折叠后生产环境不可用**（仅 dev/test，本 spec 假设无生产数据）

## 后果

### 正面

- 解除 `deep-arch-slim-v2` spec 的章程阻塞，允许其执行架构级瘦身 (维度一)。
- 保留 ADR-000 "刻意保留清单"的审计价值——清单本身未被改写，只是为特定 spec
  开具定向豁免，审计轨迹清晰。
- 明确"保留 vs 裁剪"的边界，避免后续误读为"清单全部可裁剪"。
- 用户对项目"复杂度匹配问题规模"的判断得到书面响应。

### 负面

- 项目不再具备"多区域路由"能力（若未来需要多区域部署需重写）。
- 失去 HMAC 防篡改审计链（保守版仅删验证器，激进版简化 audit.go）。
- 失去 Idempotency 中间件（若未来加支付功能需重新实现）。
- 部分组件 (outbox、bulkhead、OpenAPI 一致性测试) 的删除可能影响学习完整性——
  但用户已确认以"代码量匹配问题规模"为更高优先级。

### 对前轮决策的反转

本 ADR 反转 `slim-tier1-ef-and-materialize` 的两项决策：
- **Task 13** (HMAC 审计链验证器 `VerifyAuditChain`) — 添加决策被反转，验证器将被删除。
- **Task 14** (前端 `Idempotency-Key` header 注入) — 添加决策被反转，header 注入将被删除。

前轮决策是"为了不浪费而添加测试"的反模式，本 ADR 接受此判断并授权反转。

## 引用

- [ADR-000 项目章程](000-project-charter.md) — "刻意保留清单"定义
- [ADR-005 Hub 无状态化](005-room-management-and-outbound.md) — owner 反向代理（不豁免）
- [ADR-008 审计日志防篡改](008-audit-log-tamper-proof.md) — HMAC 审计链（豁免裁剪）
- [ADR-009 Transactional Outbox](009-transactional-outbox.md) — outbox（worker 部分豁免裁剪）
- [ADR-014 多区域拓扑](014-multi-region-deployment.md) — 多区域路由层（豁免裁剪）
- `.trae/specs/deep-arch-slim-v2/spec.md` — 本豁免授权的瘦身 spec
- `.trae/specs/deep-arch-slim-v2/tasks.md` — Task 18-23 (架构级删除) 的执行清单
