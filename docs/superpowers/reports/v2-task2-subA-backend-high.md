# v2 自检 — 子代理 A：后端 High 资产审查报告

- **审查范围**：8 个后端 High 资产（A-002 audit、A-004 config、A-005 constants、A-015 outbox、A-017 rbac、A-019 resilience、A-025 worker、A-028 cmd/server）
- **聚焦轴**：可观测性、可维护性、供应链安全、弹性、文档一致性（原 5 轴仅标记明显回归）
- **审查日期**：2026-07-08
- **发现 ID 区间**：v2-R-36 ~ v2-R-43（REQUIRED）、v2-C-08（CRITICAL）、v2-O-25 ~ v2-O-34（OPTIONAL）、v2-F-22 ~ v2-F-28（FYI）
- **审查性质**：纯诊断，未修改任何业务代码

---

## 资产 A-002: audit
### 基本信息
- 路径: `backend/internal/audit/` (audit.go, audit_db_test.go, audit_test.go)
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 可观测性

### 轴评分（聚焦新轴，原轴仅标记回归）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 3 | 全局变量 `auditLogger`/`dbLogger` 影响测试隔离性（测试用 defer 恢复），公共 API 无接口抽象 |
| 可观测性 | 3 | slog 结构化日志 ✓、trace_id/request_id 自动填充 ✓、IP SHA-256 脱敏 ✓；但缺少业务指标（写入失败计数、channel 满次数、写入延迟） |
| 供应链 | N/A | 依赖在 go.mod 层面管理 |
| 弹性 | 2 | buffered channel + 同步回退 ✓；但 `loadLastHash` 使用 `context.Background()` 无超时；`writeToDB` 失败时丢弃记录与注释承诺不一致 |
| 文档一致性 | N/A | — |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-36 | 弹性 | REQUIRED | `loadLastHash` 使用 `context.Background()` 无超时，DB 不可达时阻塞初始化协程，可能导致服务启动卡死 | audit.go:100-105 | 使用带超时的 context（如 `config.PGConnectTimeout`），或在后台异步加载并设就绪标志 |
| v2-R-37 | 文档一致性 | REQUIRED | `writeToDB` 失败时仅记录 `slog.Error` 后 return，审计记录被丢弃；但 `Log` 函数注释声称"channel 满时同步回退…不丢弃记录"，实现与文档不一致 | audit.go:135-139 vs audit.go:163 | 修正注释为"尽力而为，DB 失败时记录到 stderr 但不重试"，或增加本地落盘备份 + 重试队列 |
| v2-O-25 | 可观测性 | OPTIONAL | 缺少业务指标：审计写入成功/失败计数、channel 满回退次数、写入延迟直方图 | audit.go 全文 | 新增 `audit_writes_total{result}`、`audit_channel_full_total`、`audit_write_duration_seconds` Prometheus 指标 |
| v2-F-22 | 可维护性 | FYI | 全局变量 `auditLogger` 和 `dbLogger` 影响测试隔离性，并发测试可能竞争 | audit.go:30-33 | 长期可重构为 `AuditLogger` 结构体实例注入，但当前 defer 恢复机制可接受 |

### 整体健康度: 🟡 3.3/5

---

## 资产 A-004: config
### 基本信息
- 路径: `backend/internal/config/` (constants.go, env.go, env_test.go, redis_addr.go, timeout.go)
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 可观测性 + 弹性

### 轴评分（聚焦新轴，原轴仅标记回归）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 4 | 常量集中管理 ✓、env 覆盖 ✓、生产环境校验 ✓；但 `getDurationEnv` 与 `GetEnvDuration` 功能重叠且行为不一致 |
| 可观测性 | 3 | `slog.Warn` 用于生产警告 ✓；但缺少配置加载/校验失败指标 |
| 供应链 | N/A | — |
| 弹性 | 4 | `timeout.go` 提供完整的 PG/Redis/HTTP/WS 超时配置 ✓，可通过环境变量调优 |
| 文档一致性 | 4 | `timeout.go` 注释解释了企业理由和 trade-off ✓；`DefaultOTLPInsecure` 注释明确生产要求 |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-38 | 可维护性 | REQUIRED | `timeout.go` 的 `getDurationEnv` 仅支持整数秒（`strconv.Atoi`），而 `env.go` 的 `GetEnvDuration` 支持 `time.ParseDuration` 格式（如 "500ms"）。两个函数功能重叠但行为不一致，运维人员配置时易混淆 | timeout.go:60-66 vs env.go:202-209 | 统一为 `GetEnvDuration`，或在 `getDurationEnv` 中调用 `time.ParseDuration` |
| v2-O-26 | 安全 | OPTIONAL | `AuditSecretOrJWT` 在生产中 AUDIT_SECRET 未设置时 fallback 到 JWT_PRIVATE_KEY，虽 `slog.Warn` 但仍继续，密钥分离不彻底 | env.go:161-169 | 生产环境（`IsProduction()`）应返回空或 panic，强制密钥分离 |
| v2-F-23 | 安全 | FYI | `DefaultOTLPInsecure = true` 默认值不安全，但注释明确说明生产必须设置 `OTLP_INSECURE=false` | constants.go:99-101 | 可接受，但建议在 `Validate()` 中生产环境强制检查 |

### 整体健康度: 🟢 3.8/5

---

## 资产 A-005: constants（跨前后端协议关键）
### 基本信息
- 路径: `backend/internal/constants/protocol.go` + `backend/internal/domain/constants.go` + `backend/internal/protocol/constants.go`
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 可观测性 + **文档一致性**

### 轴评分（聚焦新轴，原轴仅标记回归）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 2 | `constants/protocol.go` 与 `protocol/constants.go` 重复定义协议常量（alias）；`domain/constants.go` 和 `config/constants.go` 都有 `MaxNicknameLen=12` 重复定义 |
| 可观测性 | N/A | — |
| 供应链 | N/A | — |
| 弹性 | N/A | — |
| 文档一致性 | 2 | `ws-protocol.md` 与代码一致 ✓；`asyncapi.yaml` 与代码一致 ✓；但 `gen-frontend-constants` 输出路径与前端实际使用路径不一致（CRITICAL） |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-C-08 | 文档一致性 | CRITICAL | `gen-frontend-constants/main.go` 输出路径为 `../../../frontend/src/shared/constants.ts`，但前端实际使用的是 `frontend/src/shared/game/constants.ts`（见 `interp_buffers.ts:1`、`visual_helpers.ts:1` 等 import）。`go generate` 生成的文件不会覆盖实际使用文件，可能导致前后端物理常量漂移而无人察觉 | gen-frontend-constants/main.go:20 vs frontend/src/shared/game/constants.ts:1 | 修正输出路径为 `../../../frontend/src/shared/game/constants.ts`，并增加 CI 校验确保生成文件与提交文件一致 |
| v2-R-39 | 可维护性 | REQUIRED | `constants/protocol.go` 定义原始消息常量，`protocol/constants.go` 通过 alias 重新导出（`MsgTap = constants.MsgTap`）。变更需同步两处，增加维护负担 | constants/protocol.go:4-9 vs protocol/constants.go:15-20 | 统一到一处定义，删除 alias 或合并包 |
| v2-R-40 | 可维护性 | REQUIRED | `domain/constants.go` 定义 `MaxNicknameLen = 12`，`config/constants.go` 也定义 `MaxNicknameLen = 12`。重复定义，未来修改时易遗漏其一 | domain/constants.go:9 vs config/constants.go:48 | 统一到一个包（建议 `config`），其他包 import 引用 |
| v2-F-24 | 可维护性 | FYI | `protocol/constants.go` 中 `MaxPlayers = 100`（协议层）与 `config/constants.go` 中 `MaxPlayersPerRoom = 50`（房间层）含义不同但命名易混淆 | protocol/constants.go:155 vs config/constants.go:40 | 建议重命名为 `ProtocolMaxPlayers` 或添加注释明确区别 |

### 整体健康度: 🔴 2.5/5

---

## 资产 A-015: outbox
### 基本信息
- 路径: `backend/internal/outbox/` (publisher.go, publisher_test.go, publisher_unit_test.go)
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 可观测性 + 弹性

### 轴评分（聚焦新轴，原轴仅标记回归）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 4 | 接口抽象良好（`pgPool`）✓、测试覆盖全面（unit + integration）✓；环境变量配置 ✓ |
| 可观测性 | 3 | `OutboxBatchSize` 和 `OutboxLagSeconds` 指标 ✓；但缺少错误计数指标（发布失败率、批处理失败次数） |
| 供应链 | N/A | — |
| 弹性 | 3 | `FOR UPDATE SKIP LOCKED` 避免竞争 ✓；但 Redis pipeline 失败时整批回滚，已发布消息可能重复（at-least-once） |
| 文档一致性 | N/A | — |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-41 | 弹性 | REQUIRED | `publishBatch` 中 Redis `pipe.Exec` 失败时整批回滚（不标记 processed），下次轮询会重新发布所有消息。对于已部分成功的 XAdd 会导致重复发布。代码注释未明确"消费者必须幂等"约束 | publisher.go:129-132 | 在包注释或 `NewPublisher` 文档中明确 at-least-once 语义和消费者幂等要求 |
| v2-O-27 | 可观测性 | OPTIONAL | 缺少 `outbox_publish_failures_total` 指标来追踪发布失败率，运维无法基于指标告警 | publisher.go:129-148 | 新增 `outbox_publish_failures_total{reason}` Counter（reason: redis_error, mark_processed_error, commit_error） |
| v2-O-28 | 可维护性 | OPTIONAL | `publishBatch` 中 `if len(batch) > 0` 检查（第 135 行）在前面 `len(batch) == 0` 已 return（第 112-114 行）后是冗余的 | publisher.go:135 | 删除冗余的 `if len(batch) > 0` 包裹 |

### 整体健康度: 🟢 3.5/5

---

## 资产 A-017: rbac
### 基本信息
- 路径: `backend/internal/rbac/` (permissions.go, rbac.go, rbac_test.go)
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 可观测性

### 轴评分（聚焦新轴，原轴仅标记回归）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 4 | 简洁的 in-memory policy map（ADR-026）✓、角色常量集中 ✓；`permissions` 为包级不可变变量 |
| 可观测性 | 3 | `audit.Log` 记录拒绝事件 ✓、`slog.Warn` 记录 ✓；但 ActorID 使用 role 而非用户 ID，审计价值降低 |
| 供应链 | N/A | — |
| 弹性 | N/A | 纯内存操作 |
| 文档一致性 | N/A | — |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-O-29 | 可观测性 | OPTIONAL | `Middleware` 中 `audit.Log` 的 `ActorID` 使用 `role`（如 "guest"）而非实际用户 ID，审计日志无法定位具体用户 | rbac.go:62-67 | 从 context 中提取用户 ID（`domain.ContextKeyUserID`）作为 ActorID，role 作为 Before/After 字段 |
| v2-O-30 | 可观测性 | OPTIONAL | 缺少 `rbac_denied_total{role,resource,action}` 指标来追踪权限拒绝率，运维无法基于权限异常告警 | rbac.go:60-69 | 新增 Prometheus Counter，在拒绝路径递增 |
| v2-F-25 | 可维护性 | FYI | `permissions` 是包级变量，无法运行时调整策略（ADR-026 lightweight RBAC 设计选择） | permissions.go:5 | 可接受，符合 ADR-026 设计；如需动态策略需重新评估 |

### 整体健康度: 🟢 3.7/5

---

## 资产 A-019: resilience
### 基本信息
- 路径: `backend/internal/resilience/` (circuitbreaker.go, retry.go 等)
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 可观测性 + 弹性

### 轴评分（聚焦新轴，原轴仅标记回归）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 4 | 清晰的工厂函数 ✓、企业理由注释 ✓；`isRetryable` 分类逻辑保守且文档完善 |
| 可观测性 | 4 | `CircuitBreakerState` gauge + `onStateChange` slog.Warn ✓；但缺少状态变更计数器 |
| 供应链 | N/A | — |
| 弹性 | 5 | 完整的熔断器（Postgres/Redis/Resend）+ 重试（DB/Redis/ExternalAPI）✓、jitter 防止 thundering herd ✓、错误分类保守 ✓ |
| 文档一致性 | N/A | — |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-O-31 | 可观测性 | OPTIONAL | 缺少 `circuit_breaker_events_total{name,from,to}` 计数器来追踪状态变更次数（gauge 只显示当前状态，无法统计历史频率） | circuitbreaker.go:36-48 | 在 `onStateChange` 中递增 Counter，用于告警频繁抖动的熔断器 |
| v2-O-32 | 弹性 | OPTIONAL | `isRetryable` 未处理 `context.Canceled`（仅通过 `pgconnTimeout` 间接处理 `DeadlineExceeded`），可能导致取消的操作被重试 | retry.go:64-106 | 显式检查 `errors.Is(err, context.Canceled)` 并返回 false（取消不应重试） |
| v2-F-26 | 可维护性 | FYI | `JitteredBackoff` 在 `base=0` 时 panic（`rand.Int64N(0)` 无效），测试已验证此行为但未在函数文档中说明 | retry.go:43-47 | 在函数注释中明确"base 必须为正数"前置条件 |

### 整体健康度: 🟢 4.3/5

---

## 资产 A-025: worker
### 基本信息
- 路径: `backend/internal/worker/` (email_worker.go, game_result_worker.go, gdpr_cleanup.go, retry.go, email_resend.go)
- 关键性: High
- 适用轴: 正确性 + 可读性 + 架构 + 安全 + 性能 + 可维护性 + 可观测性 + 弹性

### 轴评分（聚焦新轴，原轴仅标记回归）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 4 | `email_worker` 和 `game_result_worker` 共享 `retry.go` ✓、接口抽象（`gameResultDB`, `userHardDeleter`）✓ |
| 可观测性 | 2 | `gdpr_cleanup` 有指标 ✓；但 `email_worker` 和 `game_result_worker` 缺少处理计数/延迟/失败指标 |
| 供应链 | N/A | — |
| 弹性 | 3 | 重试 + 死信队列 + 熔断器（email）✓；但 `time.Sleep(time.Second)` 无退避；email worker 消费者 ID 硬编码 |
| 文档一致性 | N/A | — |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-R-42 | 弹性 | REQUIRED | `email_worker.go` 和 `game_result_worker.go` 在 `XReadGroup` 错误后使用 `time.Sleep(time.Second)` 阻塞，无指数退避策略。Redis 持续不可用时每秒重试一次，可能导致错误风暴时 CPU 空转 | email_worker.go:83-85, game_result_worker.go:94-96 | 改用指数退避（如 `resilience.JitteredBackoff`），或复用 `resilience.DefaultRedisRetry` |
| v2-R-43 | 可维护性 | REQUIRED | `game_result_worker.go` 中 consumer ID 基于 `HOSTNAME`（支持多实例），但 `email_worker.go` 使用硬编码 `"email-worker-1"`，多实例部署时共享消费者 ID 导致负载不均且 XAck 互相窃取 | email_worker.go:77 vs game_result_worker.go:78-84 | `email_worker.go` 也应使用 `HOSTNAME` 生成 consumer ID，保持一致的多实例行为 |
| v2-O-33 | 可观测性 | OPTIONAL | `email_worker` 和 `game_result_worker` 缺少处理计数/延迟/失败指标（如 `email_sent_total`, `email_failed_total`, `game_result_processed_total`, `worker_message_duration_seconds`） | email_worker.go, game_result_worker.go 全文 | 新增 Prometheus 指标，仅 `gdpr_cleanup.go` 有指标覆盖 |
| v2-O-34 | 弹性 | OPTIONAL | `retry.go` 中 re-enqueue 后 `XAck` 失败仅记录日志，不重试 XAck。下次轮询会重新处理该消息（已通过 `FOR UPDATE SKIP LOCKED` 避免竞争，但会导致重复处理） | retry.go:60-64 | 可接受（at-least-once 语义），但应在文档中明确消费者必须幂等 |
| v2-F-27 | 可维护性 | FYI | `email_resend.go` 中 `sendEmail` 通过 `clientErr` 闭包变量区分 4xx（不触发熔断）和 5xx（触发熔断），逻辑略复杂但设计正确 | email_resend.go:38-68 | 可接受，建议添加注释说明 4xx/5xx 的熔断差异 |

### 整体健康度: 🟡 3.3/5

---

## 资产 A-028: cmd/server
### 基本信息
- 路径: `backend/cmd/server/main.go`
- 关键性: High
- 适用轴: 正确性 + 安全 + 可维护性 + 弹性（4 轴）

### 轴评分（聚焦新轴，原轴仅标记回归）
| 轴 | 评分 (1-5) | 关键发现 |
|----|-----------|---------|
| 可维护性 | 5 | 极简入口（仅 14 行），所有逻辑委托给 `server.Run()`，符合关注点分离 |
| 弹性 | N/A | 弹性逻辑在 `server` 包中，main.go 不涉及 |
| 文档一致性 | N/A | — |

### 发现清单
| 发现 ID | 轴 | 严重级别 | 描述 | 位置 (文件:行号) | 建议 |
|---------|----|---------|------|---------|------|
| v2-F-28 | 可维护性 | FYI | `main.go` 极简，仅调用 `server.Run()` 并在错误时 `os.Exit(1)`，符合最佳实践 | main.go:10-13 | 无需改进 |

### 整体健康度: 🟢 5.0/5

---

## 汇总统计

### 发现数量
| 严重级别 | 数量 | 发现 ID |
|---------|------|---------|
| CRITICAL | 1 | v2-C-08 |
| REQUIRED | 8 | v2-R-36, v2-R-37, v2-R-38, v2-R-39, v2-R-40, v2-R-41, v2-R-42, v2-R-43 |
| OPTIONAL | 10 | v2-O-25 ~ v2-O-34 |
| FYI | 7 | v2-F-22 ~ v2-F-28 |
| **合计** | **26** | — |

### 各资产 v2 整体评分
| 资产 ID | 名称 | 评分 | 健康度 | 关键问题 |
|---------|------|------|--------|---------|
| A-002 | audit | 3.3/5 | 🟡 | loadLastHash 无超时 + writeToDB 丢弃记录 |
| A-004 | config | 3.8/5 | 🟢 | getDurationEnv 与 GetEnvDuration 行为不一致 |
| A-005 | constants | 2.5/5 | 🔴 | 生成器输出路径错误（前后端常量漂移风险）+ 重复定义 |
| A-015 | outbox | 3.5/5 | 🟢 | at-least-once 语义未文档化 |
| A-017 | rbac | 3.7/5 | 🟢 | 审计 ActorID 使用 role 而非用户 ID |
| A-019 | resilience | 4.3/5 | 🟢 | 弹性实现最完善，仅缺少事件计数器 |
| A-025 | worker | 3.3/5 | 🟡 | email worker 消费者 ID 硬编码 + 无退避策略 |
| A-028 | cmd/server | 5.0/5 | 🟢 | 极简入口，无问题 |

### 优先处理建议（按严重级别）
1. **v2-C-08 (CRITICAL)**: 立即修复 `gen-frontend-constants` 输出路径，否则前后端物理常量可能静默漂移
2. **v2-R-36/37 (audit 弹性)**: 修复 `loadLastHash` 超时 + `writeToDB` 丢弃记录问题
3. **v2-R-42/43 (worker 弹性/可维护性)**: 统一消费者 ID 策略 + 改用指数退避
4. **v2-R-39/40 (constants 可维护性)**: 消除重复定义，统一常量来源
5. **v2-R-38 (config 可维护性)**: 统一 duration 解析函数行为
