# ADR-007: 异步处理策略（Outbox + 消息队列）

## 状态: 已接受

> ⚠️ **部分豁免（ADR-033 决策 2，2026-07-19 起）**
>
> 本 ADR 决策 2 中关于"游戏结果 → outbox 写 `game.ended` → Publisher 发布到 `game.events` Stream →
> `GameResultWorker` 批量消费"的链路已被删除：`outbox/publisher.go`、`worker/game_result_worker.go`
> 均不存在；游戏结果改为 Room 直接同步写 PG（无下游消费者）。
> 邮件 Stream（`email:queue` + EmailWorker `XREADGROUP`）消费链路仍有效。

> **合并说明（2026-07-22）**：本 ADR 已合并原 ADR-009《事务性 Outbox 模式》的全部内容。
> ADR-009 已废弃（被 [ADR-033](033-slim-phase2-waiver.md) 豁免裁剪），其唯一性与细节性内容
> （At-Least-Once 投递语义、消费者幂等契约、重试/退避策略、监控指标、备选方案）已并入本 ADR。
> ADR-009 文件已删除，本 ADR 为事务性 Outbox 与异步队列的唯一权威记录。

## 上下文

邮件发送（曾同步阻塞请求 100ms–5s）、游戏结果持久化（同步写入 PG ~20ms/事务）、事件发布（与 PG 写入非原子）需异步化。同步写入导致请求延迟依赖下游、下游不可用时请求失败、无重试机制。

典型场景：创建用户时需要向 Redis Stream 发布 `user.created` 事件供下游消费（邮件、分析）。但 PostgreSQL 写入与 Redis 发布非原子——若 PG 插入成功而 Redis 发布失败，事件会丢失。

## 决策

采用 **Transactional Outbox + Redis Stream** 异步处理架构：

### 1. Transactional Outbox 模式

需要与 PG 事务原子化的事件发布（如用户创建时发邮件）：同一 PG 事务中写入业务数据与 `outbox_events`，后台 Publisher 每 1s 轮询（`FOR UPDATE SKIP LOCKED`），发布到 Redis Streams 后标记 `processed_at`。语义：**at-least-once**（消费者需幂等）。

> 注：原 `outbox/publisher.go` 已被 ADR-033 决策 2 删除，事务性 outbox 不再实现；下文
> At-Least-Once 语义小节描述的是该模式的设计契约（供历史追溯与未来恢复参考）。邮件
> Stream 路径不经 outbox，仍由请求处理器直接写 `email:queue`。

#### At-Least-Once 投递语义（v2-R-41，原 ADR-009）

实现位于 `backend/internal/outbox/publisher.go`（已被 ADR-033 删除），遵循 **at-least-once** 语义：事件至少被投递一次，可能重复投递，**消费者必须幂等**。

##### 投递流程与重复来源

1. `INSERT INTO outbox_events` 与业务数据在同一 PG 事务中提交（原子写入）。
2. Publisher 轮询使用 `SELECT ... FOR UPDATE SKIP LOCKED` 取一批未处理事件（`processed_at IS NULL`）。
3. 通过 Redis Pipeline `XAdd` 批量写入 Stream。
4. 同一 PG 事务内 `UPDATE outbox_events SET processed_at = now()`，再 `COMMIT`。

**可能产生重复投递的场景：**

| 场景 | 结果 |
|------|------|
| `XAdd` 成功，`UPDATE`/`COMMIT` 失败或进程崩溃 | 事件已在 Stream 中，但 `processed_at` 仍为 NULL，下一轮会被再次 `XAdd` |
| `XAdd` 成功，`COMMIT` 成功前进程被杀 | 同上 |
| Publisher 多实例并行（SKIP LOCKED 不会锁同一行） | 单条事件不会被多实例同时取走，但崩溃后未提交的行可被其他实例再次取走 |

因此 **Redis Stream 中可能存在同一 `aggregate_id` 的多条相同事件**。

##### 消费者契约

所有 outbox 下游消费者（如 `worker/email_worker.go`、`worker/game_result_worker.go`（已删除）、未来分析消费者）**必须满足幂等性**：

- 以 `(aggregate_type, aggregate_id, event_id)` 为去重键；event_id 由 Publisher 写入 Stream 字段（当前为 PG 行 `id`）。
- 处理前检查去重表/缓存（如 Redis SETNX、PG 唯一约束）；已处理则跳过。
- 处理失败不得标记为已处理，让重试机制再次投递。

##### 重试与退避策略

- Publisher 自身 **不重试单条事件**：每轮整体失败则下一轮重新取所有未处理行。
- 消费者侧的退避策略见各 worker 实现（如 email worker 指数退避，v2-R-43）。
- 死信：连续失败超过阈值的消费者应将事件移至死信 Stream/表供人工排查（当前未实现，列入后续工作）。

##### 监控指标

Publisher 已暴露以下 Prometheus 指标（`backend/internal/metrics/`）：

- `outbox_batch_size`（Histogram）：每批处理事件数
- `outbox_lag_seconds`（Gauge）：最老未处理事件的滞后秒数

告警阈值见 `deploy/prometheus/alerts.yml`：lag 持续 > 60s 触发告警。

### 2. Redis Stream 消息队列

- **游戏结果**（已被 ADR-033 豁免删除）：~~Room 结束时通过 outbox 写 `game.ended` 事件 → Publisher 发布到 `game.events` Stream → `GameResultWorker` 批量消费（100 条或 1s）写入 PG~~。现改为 Room 直接同步写 PG，无下游消费者。
- **邮件发送**：请求处理器写入 `email:queue` stream → EmailWorker `XREADGROUP` 消费，失败重试 5 次后进死信，处理器立即返回 202

## 后果

**正面**：请求延迟不被下游阻塞、原子性（业务数据与事件一同提交）、至少一次投递、可重放、指数退避自动重试、有序（按 ID 顺序处理，单 Publisher 实例内）。**负面**：额外 DB 轮询（1s + LIMIT 100 缓解）、可能重复（消费者须幂等）、最终一致性（1–5s 延迟）、Redis 额外存储、需监控未处理事件数量。

## 备选方案

1. **同步 + 超时**：阻塞请求，无重试——请求延迟依赖下游、下游不可用时请求失败。
2. **Goroutine + channel**：无持久化，进程重启即丢失。
3. **两阶段提交（2PC）**：Redis 不支持，延迟高。
4. **CDC（Debezium）**：搭建复杂，当前规模过度。
5. **尽力而为发布**：不可靠，事件可丢失。
6. **外部 MQ（RabbitMQ/Kafka）**：当前邮件量与事件规模过度设计。

## 游戏结果事件（历史 — 已被 ADR-033 反转）

> ⚠️ 以下为原 ADR-009 记录的 RO-043（2026-07）状态，描述游戏结果经 outbox 单路投递。
> 该链路随后被 [ADR-033](033-slim-phase2-waiver.md) 决策 2（2026-07-19）删除反转，
> 游戏结果改为 Room 直接同步写 PG。本段保留仅供历史追溯。

游戏结束结果曾通过 outbox 单路投递（RO-043）。原先的三写路径（PG 直写、Redis Stream
直写 `game:results`、outbox）曾收敛为 outbox 单路：

1. `Room.enqueueGameResultAsync()` → `InsertOutboxEvent("game", sessionID, payload)`
2. Outbox Publisher 发布到 `game.events` Redis Stream（aggregate_type = `"game"`）
3. `GameResultWorker` 消费 `game.events` stream，批量写入 PG

旧的 `game:results` stream 和 `EnqueueGameResult()` 方法已删除。`GameResultWorker`
通过 `outboxEventEnvelope` 解包 `{"event":"game.ended","data":{...}}` 格式的事件。

## 关联: ADR-006（Redis 策略）、ADR-010（异步邮件发送）。原 ADR-009（事务性 Outbox 模式）已合并至此（2026-07-22）。
