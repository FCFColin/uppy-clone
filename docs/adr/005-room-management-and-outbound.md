# ADR-005: Hub 房间状态管理与出站管道

## 状态: 已接受

> ⚠️ **部分豁免（2026-07-18 起）**
>
> 本 ADR 上下文段提及的"多区域路由层（ADR-014：`/resolve` + `room_directory` + owner 反向代理 + `ClaimRoomOwnership`）"
> 已被 [ADR-032](032-slim-exception-waiver.md) / [ADR-033](033-slim-phase2-waiver.md) 豁免裁剪，
> 不再属于本项目当前架构；`instanceAddress()` 与 Redis registry 的 `Address` 字段也已被 ADR-033 决策 4 删除。
> 正文决策（Hub 内存管理、出站管道、异步持久化、慢客户端断开）仍有效。

## 上下文

Hub 内存管理所有房间状态（`map[string]*Room`），存在单实例限制、故障风险（实例故障房间丢失）、tick 路径阻塞（持锁同步执行 WS 广播 + Redis Pub/Sub + PG 持久化）。高 PG/Redis 延迟或多玩家广播时 tick 与 `HandleMessage` 互相阻塞，导致 mass disconnect 与输入卡顿。多区域路由层已实现（ADR-014：`/resolve` + `room_directory` + owner 反向代理 + `ClaimRoomOwnership`），支持多实例水平扩展。单实例关闭 `EnableMultiRegion` 即可。

## 决策

### 1. 房间状态管理

房间状态完全在内存中（`game/hub.go`, `game/room.go`），元数据通过 Redis 注册表记录。实例崩溃后房间丢失，客户端重新加入（单实例部署可接受）。

### 2. 出站管道与异步持久化

为每个 Room 引入**出站管道**与**异步持久化 worker**：

1. **持锁路径**（`tickOnce`/`broadcast`/`HandleMessage`）仅更新内存状态并 `enqueueOutbound`/`requestPersist`
2. **单 goroutine `runOutboundLoop`**（buffer 256）在锁外执行本地 `pc.Send` 与 Redis publish；critical 消息带 deadline，非 critical snapshot 队列满时丢弃
3. **`runPersistLoop`** debounce 100ms 合并 PG 写入；`Close()`/`EndGame` 发送 `persistFinal` 并 `WaitGroup` 等待刷盘
4. **`enqueueGameResult`/`CreateGameSession`** fire-and-forget goroutine，失败记 metrics，不阻塞 tick
5. **慢客户端断开**：持锁仅标记 `pendingDisconnect`；锁外执行 `Conn.Close()`

### 语义

- **广播**：at-least-once 本地投递；snapshot 含 `tickCount`，客户端 dedup + 插值
- **持久化**：debounced at-most-once 中间态；终态（Close/EndGame）同步 flush
- **功能开关**：`ENABLE_ROOM_OUTBOUND_QUEUE` / `ENABLE_ASYNC_ROOM_PERSIST`（dev 默认开）

## 后果

**正面**：`room_lock_hold_seconds` P95 < 10ms；PG/Redis 抖动不再拖慢 15Hz tick；可观测 `room_outbound_queue_depth`、`room_persist_lag_seconds` metrics。**负面/缓解**：异步 persist 丢末态（Close/EndGame final flush + SIGTERM drain）；广播乱序（单连接 writePump + tickCount 排序）；outbound 队列溢出（丢非 critical + metric 告警）。

## 关联: ADR-006（Redis 策略）、ADR-007（异步处理与 Outbox）、ADR-014（多区域部署）
