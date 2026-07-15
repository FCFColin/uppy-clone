# ADR-005: Hub 无状态化与房间状态外置（单实例部署）

## 状态: 已接受（终态：单实例部署；owner 反向代理未实现）

> 房间状态通过 Redis/PostgreSQL 持久化实现故障恢复。
> 原设计的 owner 反向代理多实例路由层从未实现；学习工程项目以单实例部署为主，
> 无需跨实例路由。`ResolveRoom` 方法及相关路由类型已于 2026-07 删除。

## 上下文

当前 Hub 在内存中管理所有房间状态（`map[string]*Room`），这意味着：
1. 无法水平扩展——所有房间必须驻留在同一进程
2. 单实例内存限制——约 1000 个房间后 OOM 风险
3. 实例故障导致所有房间丢失——无持久化恢复

## 决策

将房间状态外置到 Redis + PostgreSQL，Hub 仅做消息路由：

1. **房间元数据**（code, host, player list, status）→ Redis Hash
2. **游戏结果**（最终状态）→ PostgreSQL（已有）
3. **实时游戏 tick** → 仍由单个 Room goroutine 执行（物理模拟不可分片）
4. **房间路由** → 一致性哈希（room_id → Hub 实例映射）

### 架构变更

```
Before: Client → Hub (内存 map) → Room goroutine
After:  Client → Hub (路由层) → Redis (房间元数据) → Room goroutine (计算层)
```

### 实施步骤

1. Room 元数据（非 tick 状态）外置到 Redis Hash
2. Hub 启动时从 Redis 加载房间列表
3. 新增 RoomRouter 接口：按 room_id 路由到正确的 Hub 实例
4. Hub 实例间通过 Redis Pub/Sub 同步房间创建/销毁事件
5. 健康检查增加 "房间数" 指标，用于自动扩缩容决策

## 后果

**正面**
- 支持多实例部署，水平扩展
- 实例故障后可从 Redis 恢复房间元数据
- 可按房间数自动扩缩容

**负面**
- Redis 读写增加延迟（每个 tick 至少 1 次 Redis 读取）
- 架构复杂度显著增加
- 一致性哈希的再均衡需要处理房间迁移

## 权衡

物理模拟（tick 循环）仍需单点执行，这是实时游戏的固有限制。
本方案仅解决"路由层"的无状态化，"计算层"的有状态是不可避免的。

## 实施现状（2026-07）：owner 反向代理未实现

原设计的 owner 反向代理多实例路由层从未落地实施：

1. `ResolveRoom` 方法、`RoomRoute`/`RouteLocal`/`RouteProxy` 类型、
   `RoomRouteDecision` 结构体已于 2026-07 删除（死代码清理，RO-047）。
2. `INTERNAL_PROXY_SECRET` 环境变量和 `/internal/lobby/{code}/ws` 内部端点
   从未在代码中实现。
3. `instanceAddress()` 函数保留，因为它仍被 `registerRoomInRedis` 用于
   在 Redis 房间注册表中记录实例地址（诊断用途）。
4. `PubSubBroadcaster` 及其相关基础设施（`Broadcaster` 接口、`BroadcastMessage`
   结构体、`initRedisPubSub`、`subscribeRoom`/`unsubscribeRoomLocked`、
   `OutboundManager.publishIfNeeded`）已于 2026-07 删除（RO-017）。
   原实施步骤 4（"Hub 实例间通过 Redis Pub/Sub 同步房间创建/销毁事件"）
   不再适用——单实例部署下，所有房间消息通过 `OutboundManager` 直接本地投递，
   无需跨实例广播。Redis 房间注册表（`registerRoomInRedis`）仍保留，用于
   记录实例所有权，但不再有 Pub/Sub 消费方。

**结论**：学习工程项目以单实例部署为主，无需 owner 反向代理。
Redis 房间注册表仍记录实例地址以备将来需要，但不再有路由消费方。
多区域终态（ADR-016）仍为理论设计，未实施。
