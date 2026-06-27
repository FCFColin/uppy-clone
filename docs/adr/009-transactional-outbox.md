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
- 有序：按 ID 顺序处理

**负面**
- 额外 DB 轮询（1s 间隔 + LIMIT 100 缓解）
- 可能重复发布（消费者须幂等）
- 需监控未处理事件数量

## 备选方案

1. **两阶段提交（2PC）**：Redis 不支持，延迟高
2. **CDC（Debezium）**：搭建复杂，当前规模过度
3. **尽力而为发布**：不可靠，事件可丢失
