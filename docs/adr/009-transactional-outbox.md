# ADR-009: 事务性 Outbox 模式

## 状态

已接受

## 日期

2026-06-23

## 上下文

创建用户时需要向 Redis Stream 发布 `user.created` 事件供下游消费（邮件、分析）。但 PostgreSQL 写入与 Redis 发布非原子——若 PG 插入成功而 Redis 发布失败，事件会丢失。

## 决策

采用 **Transactional Outbox** 模式：

1. 在同一 PostgreSQL 事务中写入业务数据与 outbox 事件
2. 后台 Publisher goroutine 每 1 秒轮询 `outbox_events`
3. 未处理事件发布到 Redis Streams
4. 成功发布后标记 `processed_at`

## 后果

**正面**
- 原子性：业务数据与事件一同提交
- 至少一次投递：事件不丢失
- 可重放：未处理事件保留在表中
- 有序：按 ID 顺序处理（单 Publisher 实例内）

**负面**
- 额外 DB 轮询（1s 间隔 + LIMIT 100 缓解）
- 可能重复发布（消费者须幂等，见下节）
- 需监控未处理事件数量

## At-Least-Once 投递语义（v2-R-41）

实现位于 `backend/internal/outbox/publisher.go`，遵循 **at-least-once** 语义：事件至少被投递一次，可能重复投递，**消费者必须幂等**。

### 投递流程与重复来源

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

### 消费者契约

所有 outbox 下游消费者（如 `worker/email_worker.go`、未来分析消费者）**必须满足幂等性**：

- 以 `(aggregate_type, aggregate_id, event_id)` 为去重键；event_id 由 Publisher 写入 Stream 字段（当前为 PG 行 `id`）。
- 处理前检查去重表/缓存（如 Redis SETNX、PG 唯一约束）；已处理则跳过。
- 处理失败不得标记为已处理，让重试机制再次投递。

### 重试与退避策略

- Publisher 自身 **不重试单条事件**：每轮整体失败则下一轮重新取所有未处理行。
- 消费者侧的退避策略见各 worker 实现（如 email worker 指数退避，v2-R-43）。
- 死信：连续失败超过阈值的消费者应将事件移至死信 Stream/表供人工排查（当前未实现，列入后续工作）。

### 监控指标

Publisher 已暴露以下 Prometheus 指标（`backend/internal/metrics/`）：

- `outbox_batch_size`（Histogram）：每批处理事件数
- `outbox_lag_seconds`（Gauge）：最老未处理事件的滞后秒数

告警阈值见 `deploy/prometheus/alerts.yml`：lag 持续 > 60s 触发告警。

## 备选方案

1. **两阶段提交（2PC）**：Redis 不支持，延迟高
2. **CDC（Debezium）**：搭建复杂，当前规模过度
3. **尽力而为发布**：不可靠，事件可丢失
