# Capacity Planning

> 基于连接池、WS 舱壁与负载测试假设的单实例容量估算（见 ADR-005/027）。

## 假设

| 参数 | 值 | 来源 |
|------|-----|------|
| PG `MaxConns` | 25 | `PG_POOL_MAX_CONNS` / `store/postgres.go` |
| Redis `PoolSize` | 20 | `REDIS_POOL_SIZE` / `store/redis.go` |
| `MAX_WS_CONNECTIONS` | 1000 | `config/constants.go`（`const MaxWSConnections = 1000`） |
| `MAX_PLAYERS_PER_ROOM` | 50 | 默认 |
| 单用户 WS 消息 | ~15 Hz tap + snapshot | 游戏 tick |

## 单实例估算

> 基于微基准（2026-07-06, i5-12450H）和理论推算。完整负载测试需部署后执行。

| 维度 | 估算 | 依据 |
|------|------|------|
| 并发 WS 连接上限 | 1,000 | `MAX_WS_CONNECTIONS` 舱壁，内存约 ~50MB+ |
| 活跃房间（CPU bound） | 2,000–5,000 | 15Hz tick，BuildSnapshot 1.3µs + EncodeSnapshot 0.4µs/房间 |
| REST RPS | ~1,250 | PG 池 25 连接 × ~50 QPS/连接 |
| DAU 粗算 | ~20k | 峰值 5% 同时在线 → 1k WS |
| 状态持久化延迟 | ~6.4µs/房间 | SerializeState 6.4µs，异步不阻塞 tick |
| 状态恢复延迟 | ~25.8µs/房间 | DeserializeState 25.8µs，仅房间迁移时 |

## 扩展拐点（USL）

1. **PG 连接池饱和** — `db_pool_acquire_duration` P95 上升
2. **WS 连接舱壁** — `readiness` 503 + `ws_connection_total{status="rejected"}`
3. **CPU tick** — `process_cpu_seconds_total` 与房间数线性

## 水平扩展触发

| 信号 | 动作 |
|------|------|
| PG 池等待 >50ms P95 | 读副本 + CQRS 读路径（CRDB follower reads，见 P2） |
| WS 拒绝率 >1% | HPA 扩 Hub 实例（owner 反向代理，区域内寻址 ADR-005） |
| 单实例 CPU >65% 持续 | HPA on CPU（`infra/k8s/base/hpa.yaml`） |
| 单实例 WS 连接 >6000 | HPA on `ws_connections`（`infra/k8s/base/hpa.yaml`，指标名与 `backend/internal/metrics/metrics.go` 一致） |

## 水平扩展机制（生产级，P1 落地）

- **HPA**：`infra/k8s/base/hpa.yaml` 按 CPU(65%) 与每实例 WS 连接数(6000) 扩缩，minReplicas=3、maxReplicas=100；scaleDown 稳定窗口 300s 避免抖动排空长连接。
- **优雅排空**：SIGTERM → readiness 立即 503（`health.Checker.SetDraining`），LB 在 `DRAIN_DELAY`(5s) 内移出 Pod，存量对局在 `terminationGracePeriodSeconds`(60s) 内继续，随后 `CloseAllRooms` 持久化。
- **归属租约**：owner 每次状态同步续租(`roomOwnerLeaseTTL=30s`)；仅租约过期且同区域才允许其它实例接管（`Hub.ClaimRoomOwnership`），消除"两个活跃 owner 互相覆盖"脑裂风险。

## 线性扩容验证

> ⚠️ 以下压测需部署到 staging/生产后执行。微基准数据见 [benchmarks-go-microbench.md](../development/benchmarks-go-microbench.md)。

- **REST**：HTTP 压测工具向 `BASE_URL` 发请求，记录 `http_req_duration` P95。
- **WebSocket/房间**：WS 压测工具建立并发连接（`VUS=2000`），阈值：`ws_connect_time` p95<1s、`ws_first_snapshot_ms` p99<500ms、checks>95%。
- **单房间高密度**：向单房间注入并发玩家（`PLAYERS=50`），阈值：`ws_unexpected_disconnects`<1、checks>95%。扩容判据：实例数翻倍时"最大稳定并发 WS / 活跃房间"应近似线性增长（扩展维度是"房间总数"而非"单房间算力"，见 ADR-014）。
- 每季度运行并回填下表（数据由实际压测填充）：

| 实例数 | 稳定并发 WS | 活跃房间 | message p99 | 瓶颈 |
|--------|-------------|----------|-------------|------|
| 1 | 待部署后压测 | 待部署后压测 | 待部署后压测 | 预期：WS 舱壁或 PG 池 |
| 3 | 待部署后压测 | 待部署后压测 | 待部署后压测 | 预期：PG 池或 CPU |
| 10 | 待部署后压测 | 待部署后压测 | 待部署后压测 | 预期：跨实例路由开销 |
