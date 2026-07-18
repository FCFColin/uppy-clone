# ADR-007: 异步处理策略（Outbox + 消息队列）

## 状态: 已接受

## 上下文

邮件发送（曾同步阻塞请求 100ms–5s）、游戏结果持久化（同步写入 PG ~20ms/事务）、事件发布（与 PG 写入非原子）需异步化。同步写入导致请求延迟依赖下游、下游不可用时请求失败、无重试机制。

## 决策

采用 **Transactional Outbox + Redis Stream** 异步处理架构：

### 1. Transactional Outbox 模式

需要与 PG 事务原子化的事件发布（如用户创建时发邮件）：同一 PG 事务中写入业务数据与 `outbox_events`，后台 Publisher 每 1s 轮询（`FOR UPDATE SKIP LOCKED`），发布到 Redis Streams 后标记 `processed_at`。语义：**at-least-once**（消费者需幂等）。

### 2. Redis Stream 消息队列

- **游戏结果**：Room 结束时通过 outbox 写 `game.ended` 事件 → Publisher 发布到 `game.events` Stream → `GameResultWorker` 批量消费（100 条或 1s）写入 PG
- **邮件发送**：请求处理器写入 `email:queue` stream → EmailWorker `XREADGROUP` 消费，失败重试 5 次后进死信，处理器立即返回 202

## 后果

**正面**：请求延迟不被下游阻塞、原子性（业务数据与事件一同提交）、至少一次投递、可重放、指数退避自动重试。**负面**：额外 DB 轮询（1s + LIMIT 100 缓解）、可能重复（消费者须幂等）、最终一致性（1–5s 延迟）、Redis 额外存储。

## 备选方案: 同步+超时（阻塞无重试）、Goroutine+channel（无持久化重启丢失）、2PC（Redis 不支持）、CDC/外部 MQ（当前规模过度）。

## 关联: ADR-006（Redis 策略）、ADR-009（原 Outbox ADR，已合并至此）
