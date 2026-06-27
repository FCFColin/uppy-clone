# 混沌工程实验目录

> 通过主动故障注入验证系统韧性，发现单点故障与级联失败（见 ADR-004）。

## 实验 1：PostgreSQL 宕机 30 秒

### 稳态假设

- 认证 SLO：99.9% 成功率
- 进行中的房间不受影响（内存状态）
- 熔断器 5 次失败后 open，返回 503 而非超时

### 实验方法

1. 使用 Chaos Mesh `PodKill` 或 `NetworkChaos`
2. 杀死 PostgreSQL pod 或阻断网络 30 秒
3. 监控：`auth_requests_total`、`circuit_breaker_state`、`http_requests_total{status="503"}`

### 成功标准

- [ ] 熔断器 5 秒内 open
- [ ] 503 在 500ms 内返回
- [ ] 游戏中 WebSocket 不断开
- [ ] PG 恢复后 30 秒内 close
- [ ] 无 panic 或 goroutine 泄漏

### 实验结果

- 日期：2026-06-25（待在 staging 执行）
- 发现：熔断见 `resilience/circuitbreaker.go`；降级见 `handler/degradation.go`；优雅关闭见 `internal/server/server_lifecycle.go`
- **下次执行：** 每月第一个周二 staging

---

## 实验 2：Redis 不可达 60 秒

### 稳态假设

- WebSocket 保持（Hub 内存状态）
- Magic Link 降级 503
- 限流 fail-open

### 实验方法

1. Chaos Mesh 阻断 Redis 60 秒
2. 监控：`ws_connection_total`、`redis_pool_total_conns`

### 成功标准

- [ ] WS 连接保持
- [ ] Magic Link 返回 503 + degraded
- [ ] 限流 fail-open（`middleware/ratelimit.go`）
- [ ] Redis 恢复后自动重连

---

## 实验 3：网络延迟 +500ms

### 稳态假设

- p99 从 <100ms 升至 <700ms 可接受
- 游戏 tick 仍稳定

### 实验方法

toxiproxy 或 Chaos Mesh 注入延迟

### 成功标准

- [ ] 无大量 WS 断连
- [ ] HTTP p99 <700ms

---

## 实验 4：单 Pod 驱逐（GKE）

验证 PDB、drain、租约接管（ADR-005）。

---

## 执行清单

| 实验 | 环境 | 频率 | 负责人 |
|------|------|------|--------|
| PG 宕机 | staging | 月 | SRE |
| Redis 阻断 | staging | 月 | SRE |
| 网络延迟 | staging | 季 | SRE |
| Pod 驱逐 | staging | 季 | SRE |

详见 [Runbook](runbook.md) 与 [SLO](slo.md)。
