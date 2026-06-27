# ADR-005: Hub 无状态化与房间状态外置（区域内 owner 反向代理 + 租约接管）

## 状态: 已接受（终态：区域内 owner 反向代理 + 归属租约；多区域见 ADR-016）

> 房间状态通过 Redis/PostgreSQL（多区域为 CockroachDB，ADR-015）持久化实现故障恢复；
> 多实例路由层采用 **区域内 owner 反向代理**，任意实例都能正确服务本区域任意房间；
> 归属用**租约**协调故障接管，跨区域由全局目录路由、绝不转发游戏帧（ADR-016）。

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

## 实施现状（2026-06）：owner 反向代理

最终落地的路由层不依赖"一致性哈希 + Pub/Sub 广播"，而是 **owner 反向代理**：

1. **实例可寻址**：每个实例通过 downward API 拿到 `POD_IP`，拼成 `INSTANCE_ADDR`
   （`POD_IP:PORT`）。创建房间时把 `instance` + `address` 写入 Redis 房间注册表
   （`RoomRegistryInfo`）。
2. **路由决策**：连接进入任意实例时，`Hub.ResolveRoom(code)` 给出三种结果：
   - `RouteLocal`：owner == 本实例 / 注册表 miss / 单实例 → 本地服务（`serveRoomLocally`）。
   - `RouteProxy`：owner == 本区域其它实例 → 本地升级 WebSocket，拨号 owner 内部端点
     `GET /internal/lobby/{code}/ws` 双向帧转发（`bridgeConns`）。
   - `RouteRedirect`：房间属于**其它区域** → 返回 421 + 就近 `ws_endpoint`，客户端直连
     房间 home region（ADR-016，绝不跨区域转发游戏帧）。
3. **内部端点鉴权**：`/internal/lobby/{code}/ws` 由 `INTERNAL_PROXY_SECRET`
   共享密钥保护，透传原始 `userID`（边缘实例已完成 auth/origin/限流）。
4. **归属租约与故障切换**：owner 周期续租（`roomOwnerLeaseTTL=30s`，随状态同步续租）。
   `ClaimRoomOwnership` 仅在「注册表 miss / Redis 不可用」或「**同区域**且租约已过期」时
   接管，**跨区域永不接管**。这取代了早期无作用域的 last-writer-wins，消除双活 owner
   脑裂。接管后从 Redis/CRDB 恢复状态本地服务。

### 为什么放弃 Redis Pub/Sub 跨实例广播

owner 反向代理下，同一房间的所有连接最终都汇聚到 owner 实例，**本地投递即可覆盖
全部连接**，无需把每帧再经 Redis Pub/Sub 扇出到其它实例。两套机制并存属冗余，
且可能造成重复投递。因此生产环境不再装配 `PubSubBroadcaster`（`NewHub` 传 nil），
消除一层机制（呼应"避免过度工程"）。`Broadcaster` 接口与实现保留，便于将来需要时
（如非代理拓扑）复用。

### 部署影响

owner 反向代理要求实例间网络可寻址，与 Cloud Run（实例不可寻址）冲突，故统一部署到
GKE StatefulSet + headless Service（见 `infra/k8s/base/service.yaml` 与 ADR-013 终态）。

### 多区域终态（ADR-016）

- **区域内**：owner 反向代理 + 租约接管（本节所述），区域本地 Redis 存注册表/状态缓存。
- **跨区域**：全局房间目录（CRDB `room_directory`，GLOBAL 表）记录 `code→region/endpoint`，
  `ResolveRoom` 命中异区域返回 `RouteRedirect`，客户端就近重连房间 home region。
  **跨区域绝不转发游戏帧、绝不跨区接管**，使每个对局的实时路径始终单区域低延迟。
