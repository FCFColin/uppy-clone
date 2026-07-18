# ADR-014: 多区域拓扑与全局就近路由

## 状态: 已实现

> **实现说明**：多区域路由层已落实——owner 反向代理（`RouteLocal`/`RouteProxy`）、`/resolve` 跨区域路由端点、`room_directory` 表与 `ClaimRoomOwnership` 租约接管均已实现。`EnableMultiRegion` 配置默认可在单实例下关闭，但代码完整；生产多实例部署设 true。关联代码：`backend/internal/handler/resolve.go`、`backend/internal/handler/lobby_ws_proxy.go`、`backend/internal/game/hub_multiregion.go`、`backend/internal/domain/room_directory.go`、migration `000017_create_room_directory`。

## 日期: 2026-06

## 关联: ADR-005（房间管理与 owner 反向代理）、ADR-029（Redis 域拆分 Phase 3）

## 上下文

超大型 SaaS 需要全球低延迟、区域故障隔离、数据驻留合规。单区域 GKE + 单主 PostgreSQL 无法满足。学习工程在单实例下可关闭多区域，但代码层必须完整支持水平扩展与故障切换，避免架构债。

## 决策

1. **多区域 GKE 集群**：每区域一套 StatefulSet（实时层）+ 区域本地 Redis（房间注册表/状态缓存，区域内）
2. **全局接入**：GeoDNS/Anycast + 全局 LB 把客户端导向最近区域；区域内再按 owner 反向代理路由到房间 owner（ADR-005）
3. **`/resolve` 跨区域路由**：客户端连接前先调 `GET /resolve?code=XXXXX`，返回房间 home region 的 `ws_endpoint`。同区域 → 同源直连；异区域 → 客户端重连到房间所在区域（`handler/resolve.go`，前端 `ws_connect.ts` 调用）
4. **owner 反向代理**：WebSocket 连接落到非 owner 实例时，本实例通过 `httputil.ReverseProxy` 反向代理到 owner 实例的 `/internal/lobby/{code}/ws`，带 `INTERNAL_PROXY_SECRET` 鉴权头透明转发 WS 流量（`RouteLocal`/`RouteProxy`）
5. **租约接管**：owner 实例崩溃且 `roomOwnerLeaseTTL`（默认 30s）过期后，同区域其他实例通过 `ClaimRoomOwnership` 接管房间，`room_directory` 更新 owner 指向，新连接正常路由。`PurgeExpired` 定时任务清理过期租约
6. **`room_directory` 表**：PostgreSQL 中存储 `code→region/endpoint/owner/lease` 映射，跨区域共享（GLOBAL）。仓储方法：`UpsertRoomDirectory`/`LookupRoomDirectory`/`ClaimRoomOwnership`/`PurgeExpired`/`DeleteRoomDirectory`（`store/room_directory.go`）
7. **跨区控制面**：区域间仅交换控制面信息（房间目录、配置、可观测性聚合），通过 mTLS 保护；**不跨区交换实时游戏帧**
8. **可观测性聚合**：Thanos/Mimir 或 Prometheus 联邦聚合多区域指标，统一 Grafana
9. **`PubSubBroadcaster`**：跨实例广播使用 Redis Pub/Sub，区域内多实例同步快照

## 后果

- **优点**：全球低延迟、区域级故障隔离、合规数据驻留、容量随区域线性扩展、owner 故障自动接管保证对局连续性
- **代价**：多集群运维、跨区控制面一致性与安全、成本上升
- **演进**：控制面/数据面均多活
