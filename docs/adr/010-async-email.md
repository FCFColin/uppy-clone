# ADR-010: 异步邮件发送

## 状态

已接受

## 日期

2026-06-23

## 上下文

Magic Link 邮件曾在请求处理器中同步发送，导致：

- 请求延迟依赖 SMTP/HTTP API（100ms–5s）
- 邮件 API 不可用时请求失败
- 瞬态失败无重试机制

## 决策

使用 Redis Stream 作为异步邮件队列：

1. 请求处理器将邮件 payload 写入 `email:queue` stream
2. 后台 EmailWorker 通过 XREADGROUP 消费
3. 失败最多重试 5 次，之后进入 `email:dead-letter` stream
4. 处理器立即返回 202 Accepted

## 后果

**正面**
- 请求延迟不再被邮件 API 阻塞
- 指数退避自动重试
- 死信队列便于人工排查
- 可水平扩展：多 worker 加入同一 consumer group

**负面**
- Redis 额外存储队列
- 发送失败无即时反馈
- 需监控队列深度与死信队列

## 备选方案

1. **同步 + 超时**：仍阻塞请求，无重试
2. **Goroutine + channel**：无持久化，重启丢失
3. **外部 MQ（RabbitMQ/Kafka）**：当前邮件量过度设计
