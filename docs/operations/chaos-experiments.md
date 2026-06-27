# Chaos Engineering Experiments

> 企业为何需要：混沌工程通过主动故障注入验证系统韧性，发现隐藏的单点故障和级联失败。

## Experiment 1: PostgreSQL 宕机 30 秒

### 稳态假设 (Steady State Hypothesis)
- 认证 SLO: 99.9% success rate
- 游戏进行中的房间不受影响（内存状态）
- 熔断器在 5 次失败后 open，返回 503 而非超时

### 实验方法
1. 使用 Chaos Mesh `PodKill` 或 `NetworkChaos` disruption
2. 杀死 PostgreSQL pod 或阻断网络 30 秒
3. 监控指标: `auth_requests_total`, `circuit_breaker_state`, `http_requests_total{status="503"}`

### 成功标准
- [ ] 熔断器在 5 秒内 open
- [ ] 503 响应在 500ms 内返回（非超时）
- [ ] 游戏中的 WebSocket 连接不断开
- [ ] PostgreSQL 恢复后熔断器在 30 秒内 close
- [ ] 无 panic 或 goroutine 泄漏

### 实验结果
- 日期: 2026-06-25（审计实施 — 待在 staging 执行）
- 结果: 待执行 — 使用 Chaos Mesh / toxiproxy 按上述方法在 staging 运行
- 发现: 代码层已具备熔断（`resilience/circuitbreaker.go`）、降级响应（`handler/degradation.go`）、graceful shutdown（`cmd/server/main.go:551-576`）
- **下次执行:** 每月第一个周二 staging，前置条件见 [`slo.md`](./slo.md) Burn Rate 告警已部署

---

## Experiment 2: Redis 不可达 60 秒

### 稳态假设
- WebSocket 连接保持（Hub 使用内存状态）
- 魔法链接验证降级返回 503（非 500）
- Rate limiting 降级为放行（避免拒绝所有请求）

### 实验方法
1. 使用 Chaos Mesh `NetworkChaos` 阻断 Redis 网络 60 秒
2. 监控指标: `ws_connection_total`, `auth_requests_total`, `redis_pool_total_conns`

### 成功标准
- [ ] WebSocket 连接保持，游戏继续
- [ ] 魔法链接请求返回 503 + degraded 标志
- [ ] Rate limiting 降级为放行（fail-open）
- [ ] Redis 恢复后自动重连
- [ ] 无数据丢失（游戏状态在内存中）

### 实验结果
- 日期: 2026-06-25（审计实施 — 待在 staging 执行）
- 结果: 待执行 — WS 内存态应持续；magic link 应返回 503 degraded
- 发现: Rate limit fail-open 见 `middleware/ratelimit.go`；详见 runbook 故障 2/6

---

## Experiment 3: 网络延迟 +500ms

### 稳态假设
- p99 延迟从 < 100ms 升至 < 700ms（可接受）
- 无请求超时（server timeout 15s >> 500ms）
- 慢客户端检测机制触发

### 实验方法
1. 使用 Chaos Mesh `NetworkChaos` 添加 500ms 延迟
2. 持续 5 分钟
3. 监控指标: `http_request_duration_seconds`, `ws_message_duration_seconds`

### 成功标准
- [ ] 无 5xx 错误（超时除外）
- [ ] p99 延迟 < 700ms
- [ ] 慢客户端检测触发（`ws_messages_dropped_total` 增加）
- [ ] 延迟恢复后指标恢复正常

### 实验结果
- 日期: 2026-06-25（审计实施 — 待在 staging 执行）
- 结果: 待执行 — WS 内存态应持续；magic link 应返回 503 degraded
- 发现: Rate limit fail-open 见 `middleware/ratelimit.go`；详见 runbook 故障 2/6

---

---

## 实验 4: 区域级故障切换（多区域，ADR-014/016）

### 假设
某区域整体不可用（GKE 集群宕机 / 区域网络分区）时，全局入口应把新流量导向其余
健康区域；该区域的对局会中断，玩家重连后由全局 LB 路由到最近健康区域并新建/加入
房间（房间 home region 随 `room_directory` 更新）。**跨区域绝不接管正在进行的对局，
也绝不转发游戏帧**。

### 稳态假设
- 全局可用区域的 p99 不受影响（< 700ms）
- 故障区域流量在健康检查窗口内（≤ 30s）被 GCLB 摘除
- CRDB 在多数派区域存活时保持可写（REGIONAL BY ROW 行驻留区域失联会影响该区域行的写，
  但其余区域行不受影响）

### 实验方法
1. 在 staging 多区域集群，用 Chaos Mesh `PodChaos`(kill) 或封锁某区域的 MCS 后端，
   模拟 `europe-west1` 整体不可用
2. 持续 10 分钟
3. 监控（Thanos 聚合视图，按 `region` 切分）：
   - `up{region="europe-west1"}` 归零
   - GCLB 后端健康数下降、流量重路由到 us/asia
   - 其余区域 `ws_active_connections` 上升、p99 平稳
   - CRDB `ranges_unavailable` / region 写延迟

### 成功标准
- [ ] ≤ 30s 内故障区域被摘除，新连接不再路由过去
- [ ] 健康区域 p99 < 700ms，无跨区域帧转发（无 `RouteRedirect`→`RouteProxy` 跨区）
- [ ] 玩家重连后能在健康区域建/加房（`room_directory` 写入新 region）
- [ ] 区域恢复后自动重新纳入 LB，指标恢复

### 实验结果
- 日期: 2026-06-25（待在多区域 staging 执行）
- 结果: 待执行
- 关联处置: 见 runbook「7. 多区域事件」

---

## 实验执行计划
- 频率: 每月 1 次
- 环境: staging（非生产）
- 前置条件: 所有 SLO 告警已配置，runbook 已更新
- 后置条件: 生成 postmortem（如有意外发现）
