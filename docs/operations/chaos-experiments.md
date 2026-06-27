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
- 日期: [待执行]
- 结果: [待记录]
- 发现: [待记录]

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
- 日期: [待执行]
- 结果: [待记录]

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
- 日期: [待执行]
- 结果: [待记录]

---

## 实验执行计划
- 频率: 每月 1 次
- 环境: staging（非生产）
- 前置条件: 所有 SLO 告警已配置，runbook 已更新
- 后置条件: 生成 postmortem（如有意外发现）
