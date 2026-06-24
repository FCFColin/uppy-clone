# ADR-005: Hub 无状态化与房间状态外置

## 状态: 已接受（部分实施）

> 房间状态已通过 Redis 持久化（SaveLobbyState/RestoreRooms）实现故障恢复，
> 但 Hub 仍为单进程内存态，水平扩展（Redis Pub/Sub 广播层）尚未实施。

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
