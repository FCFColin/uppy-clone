# ADR-027: Room 出站管道（锁外广播与异步持久化）

## 状态: 已接受

## 上下文

Room goroutine 在 15Hz tick 路径上持 `Room.mu` 期间曾同步执行：

- WebSocket 广播（含慢客户端 `time.After` 等待）
- Redis Pub/Sub 发布（100ms 超时）
- PostgreSQL `SaveLobbyState`（每 2s）

高 PG/Redis 延迟或多玩家广播时，tick 与 `HandleMessage` 互相阻塞，导致 mass disconnect 与输入卡顿。

## 决策

为每个 Room 引入**出站管道**与**异步持久化 worker**：

1. **持锁路径**（`tickOnce` / `broadcast` / `HandleMessage`）仅更新内存状态并 `enqueueOutbound` / `requestPersist`。
2. **单 goroutine `runOutboundLoop`**（buffer 256）在锁外执行本地 `pc.Send` 与 Redis publish；critical 消息带 deadline send，非 critical snapshot 队列满时丢弃。
3. **`runPersistLoop`** debounce 100ms 合并 PG 写入；`Close()` / `EndGame` 发送 `persistFinal` 并 `WaitGroup` 等待刷盘。
4. **`enqueueGameResult` / `CreateGameSession`** 改为 fire-and-forget goroutine，失败记 metrics，不阻塞 tick。
5. **慢客户端断开**：持锁仅标记 `pendingDisconnect`；锁外 outbound loop / writePump 执行 `Conn.Close()`。

### 数据流

```
tickOnce (Room.mu) → buildSnapshot → outboundCh → runOutboundLoop → WS + Redis
                 → requestPersist → persistCh   → runPersistLoop  → PostgreSQL
```

### 语义

- **广播**：at-least-once 本地投递；snapshot 含 `tickCount`，客户端 dedup + 插值。
- **持久化**：debounced at-most-once 中间态；终态（Close/EndGame）同步 flush。
- **功能开关**：`ENABLE_ROOM_OUTBOUND_QUEUE` / `ENABLE_ASYNC_ROOM_PERSIST`（dev 默认开）。

## 后果

### 正面

- `room_lock_hold_seconds` P95 目标 <10ms（正常依赖延迟下）。
- PG/Redis 抖动不再直接拖慢 15Hz tick。
- 可观测：`room_outbound_queue_depth`、`room_persist_lag_seconds`。

### 负面 / 缓解

| 风险 | 缓解 |
|------|------|
| 异步 persist 丢末态 | Close/EndGame final flush + SIGTERM drain |
| 广播乱序 | 每连接单 writePump；tickCount 客户端排序 |
| outbound 队列溢出 | 丢非 critical snapshot + metric 告警 |

## 相关

- ADR-005 Hub 无状态化（tick 仍属单 Room goroutine）
- ADR-009 Transactional Outbox（EndGame 结果仍经 outbox）
