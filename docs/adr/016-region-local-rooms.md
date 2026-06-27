# ADR-016: 区域本地房间与跨区域重定向（不跨区转发游戏帧）

- 状态: 提议中
- 日期: 2026-06
- 关联: ADR-005（Hub 无状态/owner 反向代理）、ADR-014（多区域拓扑）、ADR-015（CockroachDB）

## 上下文

实时游戏房间是 15Hz tick 的强一致单写者状态机，单房间不可分片（ADR-005）。
多区域部署后必须回答：一个房间归属哪个区域？跨区域连接如何处理？

跨区域转发游戏帧（区域 A 的实例把帧代理到区域 B 的 owner）会引入跨洋 RTT，
叠加到每个 tick 上，导致体验崩溃，且放大区域间故障耦合。

## 决策

1. **房间绑定 home region**：房间在创建区域落地，`room_directory`（GLOBAL 表）记录
   `code→region/endpoint` 全局唯一映射，解决 5 位房间码跨区域唯一性与定向。
2. **区域内 owner 反向代理**：区域内沿用 ADR-005，连接代理到本区域 owner 实例
   （Pod-IP 可寻址、低延迟）。`Hub.ResolveRoom` 返回 `RouteProxy`。
3. **跨区域重定向而非转发**：当连接到达非房间 home region 时，返回
   `421 Misdirected Request` + `ws_endpoint`（`Hub.ResolveRoom` → `RouteRedirect`），
   客户端就近重连到房间 home region，**绝不跨区域转发游戏帧**。
4. **归属租约**：owner 周期续租（`roomOwnerLeaseTTL`），仅同区域且租约过期才允许
   接管（`ClaimRoomOwnership`），消除跨区双 owner 与脑裂。

## 后果

- **优点**：每个对局的实时路径始终单区域、低延迟；区域故障域隔离；房间码全局唯一。
- **代价**：玩家加入异区域房间需一次重定向重连（一次性，非每帧）；需全局
  `room_directory` 与就近接入（GeoDNS/Anycast，见 ADR-014）。
- **前端**：`websocket.ts` 需处理 421 重定向，改用 `ws_endpoint` 重连（P3）。
