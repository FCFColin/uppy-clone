# On-call Runbook

> 最后更新: 2026-07-18
> 适用范围: 多人网页气球飞行对战游戏 (Go + PostgreSQL + Redis)
>
> 每个故障条目遵循 "症状 → 排查 → 缓解" 三段式，对应 SRE 故障响应的
> 检测 → 定位 → 止血。根治方案见各 ADR 与 capacity-planning.md。

## 故障分级

| 级别 | 定义 | 响应时间 |
|------|------|---------|
| P0 | 服务完全不可用 | 5 分钟 |
| P1 | 核心功能受损 | 15 分钟 |
| P2 | 非核心功能异常 | 1 小时 |
| P3 | 性能退化/体验下降 | 4 小时 |

---

## 熔断器全局视图

> 所有下游依赖均通过熔断器（`backend/internal/resilience/circuitbreaker.go`，基于 sony/gobreaker）保护，
> 防止级联故障。状态变更通过 `metrics.CircuitBreakerState` 暴露为 Prometheus Gauge。

**指标查询**

```promql
circuit_breaker_state{name="<breaker>"}
# 0 = closed（健康），0.5 = half-open（探测中），1 = open（已熔断）
```

**熔断器清单**

| 熔断器 | 指标 label (`name`) | 触发阈值 | 熔断恢复超时 | Half-open 探测数 | 关联故障 |
|--------|---------------------|---------|-------------|------------------|---------|
| postgres | `postgres` | 连续失败 > 5 次 | 30s | 3 | 故障 1 |
| redis | `redis` | 连续失败 > 5 次 | 15s | 3 | 故障 2 / 6 |
| resend-api | `resend-api` | 连续失败 > 3 次 | 60s | 1 | 故障 6 |

> `resend-api` 作为外部 API 更保守：更早触发（3 次）、更长恢复等待（60s）、半开仅放行 1 个探测请求。

```bash
curl -s localhost:8080/metrics | grep circuit_breaker_state
```

---

## 故障 1: PostgreSQL 不可用

**症状**
- `/health/ready` 返回 503，日志输出 `postgres: unavailable`
- API 大面积返回 500；游戏结果无法写入
- 熔断器 `circuit_breaker_state{name="postgres"}` 值为 1（open）

**可能原因**: PG 进程宕机/OOM；网络分区；连接池耗尽；磁盘满；慢查询堆积

**排查**
```bash
pg_isready -h $PGHOST -p $PGPORT
psql $DATABASE_URL -c "SELECT count(*), state FROM pg_stat_activity GROUP BY state"
curl -s localhost:8080/metrics | grep -E 'db_pool|circuit_breaker_state'
df -h
psql $DATABASE_URL -c "SELECT pid, now()-query_start AS duration, query FROM pg_stat_activity WHERE state='active' ORDER BY duration DESC LIMIT 10"
```

**缓解**
1. 进程宕机：`docker restart postgres`（或 failover 到只读副本）
2. 连接池耗尽：重启应用 Pod 释放连接，临时调高 `MaxConns`
3. 磁盘满：清理旧审计日志（见故障 5 注意事项）
4. 网络分区：切换到备用可用区，更新服务发现
5. 慢查询：`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE state='active' AND now()-query_start > interval '60s'`

---

## 故障 2: Redis 不可用

**症状**
- Rate limiting fail-open（所有请求放行）
- Magic Link 登录失败；Refresh token 验证失败，用户被强制登出
- Lobby 列表查询降级到空列表
- 熔断器 `circuit_breaker_state{name="redis"}` 值为 1（open）

**可能原因**: Redis 进程宕机/OOM；内存耗尽；网络分区；持久化阻塞；大量 Key 同时过期

**排查**
```bash
redis-cli -h $REDIS_HOST -p $REDIS_PORT ping
redis-cli -h $REDIS_HOST info memory | grep -E "used_memory_human|maxmemory_human"
redis-cli -h $REDIS_HOST info clients | grep connected_clients
curl -s localhost:8080/metrics | grep -E 'redis_pool|circuit_breaker_state'
```

**缓解**
1. 进程宕机：`docker restart redis`
2. 内存满：清理低优先级 Key `redis-cli --scan --pattern "rate_limit:*" | xargs redis-cli del`
3. 熔断器 open：等待半开探测周期（15s）自动恢复
4. 扩容 Redis 内存：调整 `maxmemory` 配置

---

## 故障 3: WebSocket 连接洪水

**症状**
- 内存使用激增，`go_goroutines` 快速上升
- `ws_connections` 异常飙升；API P99 升高
- 服务可能被 OOM Kill

**可能原因**: Bot 攻击；病毒式流量；客户端重连未退避；缺少连接级速率限制

**排查**
```bash
curl -s localhost:8080/metrics | grep -E 'ws_connection|go_goroutines|go_memstats_alloc'
ss -tn state established | wc -l
ss -tn state established | awk '{print $4}' | sort | uniq -c | sort -rn | head -20
```

**缓解**
1. 启用/收紧 WebSocket 连接级速率限制（`CheckRateLimit`）
2. 封禁高频 IP：`iptables -A INPUT -s <ip> -j DROP`（或 WAF 层）
3. 临时降低 `MaxWSConnections` 上限，拒绝新连接保护存量用户
4. 水平扩容（HPA `infra/k8s/base/hpa.yaml`），分散连接压力

---

## 故障 4: 游戏 tick 延迟

**症状**
- 玩家反馈卡顿、物理抖动
- `ws_message_duration_seconds` P99 > 100ms
- 画面位置回退（客户端插值补偿失败）

**可能原因**: CPU 饱和；GC 压力；锁竞争（`room.mu.Lock` 持锁过长）；热路径分配过多

**排查**
```bash
curl -s localhost:8080/metrics | grep -E 'process_cpu|go_gc_duration|game_active|ws_message_duration'
# pprof 需临时启用（后端未默认 import net/http/pprof）
# 排障时临时注册 import _ "net/http/pprof" 并监听 6060 端口
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```

**缓解**
1. 临时降低活跃房间数上限或拒绝新房间，保护存量对局 CPU
2. 水平扩容：HPA 增加 StatefulSet 副本，分散房间到更多实例（ADR-005）
3. 重启高负载实例释放内存碎片
4. 若 GC 压力：临时调高 `GOGC=200` 降低 GC 频率

---

## 故障 5: 磁盘/内存满

**症状**
- 服务被 OOM Kill，容器重启次数增加
- 日志写入失败：`write: no space left on device`
- PG 无法写 WAL；Redis 拒绝写入 `OOM command not allowed`

**可能原因**: 日志累积未轮转；内存泄漏（goroutine 泄漏）；审计日志表无限增长；Redis Key 无 TTL

**排查**
```bash
df -h
free -m
docker stats --no-stream
du -sh /var/log/* | sort -rh | head -10
curl -s localhost:8080/metrics | grep go_goroutines
psql $DATABASE_URL -c "SELECT count(*) FROM audit_logs"
```

**缓解**
1. 清理旧日志：`journalctl --vacuum-time=2d` 或 `truncate -s 0 /var/log/app.log`
2. 清理审计日志（⚠️ `audit_logs` 表有触发器 `no_delete_audit_logs` 阻止 DELETE，
   需临时禁用触发器）：
   ```sql
   ALTER TABLE audit_logs DISABLE TRIGGER no_delete_audit_logs;
   DELETE FROM audit_logs WHERE created_at < extract(epoch from now() - interval '30 days') * 1000;
   ALTER TABLE audit_logs ENABLE TRIGGER no_delete_audit_logs;
   ```
3. 重启 Pod 释放泄漏内存：`kubectl rollout restart deployment/<name>`
4. 清理 Redis 无 TTL Key：`redis-cli --scan --pattern "*" | head -1000` 检查后清理
5. 临时扩容磁盘/内存

---

## 故障 6: 认证服务异常

> 认证 SLO：成功率 99.9%、p99 < 500ms（详见 `slo.md` §2.1）。
> 认证失败直接消耗 Error Budget（43.2 分钟/月），属 P1 级响应（15 分钟内介入）。

### 6.1 Refresh token 验证失败率突增

**症状**: `/api/v1/auth/refresh` 401 > 5%；用户大面积被强制登出；可能伴随 Redis 熔断器 open

**排查**
1. 检查 `JWT_PRIVATE_KEY` / `JWT_PUBLIC_KEY` 是否被轮换：JWT 使用 ES256（ECDSA P-256），
   refresh token 用旧私钥签发，新私钥无法验证 → 全部 401
   ```bash
   echo -n "$JWT_PRIVATE_KEY" | sha256sum  # 与上次部署对比
   ```
2. 检查 Redis 可用性（refresh token 存储在 Redis）
3. 检查 token TTL 配置是否被误改

**处置**
1. 回滚 `JWT_PRIVATE_KEY` 变更：恢复至上次部署的 ECDSA 私钥
2. 恢复 Redis：见故障 2
3. 临时延长 token TTL：降低刷新频率，争取修复时间
4. 若密钥必须轮换：发布公告 + 引导用户重新登录，接受短期 401 峰值

### 6.2 Magic Link 邮件投递失败（Resend API 熔断器排查）

**症状**: 用户收不到 magic link 邮件；`resend-api` 熔断器 open；`email:dead-letter` 积压

**排查**
```bash
curl -s localhost:8080/metrics | grep 'circuit_breaker_state{name="resend-api"}'
redis-cli -h $REDIS_HOST XLEN email:dead-letter
```
- 检查 Resend API 状态页：https://resend.com/status
- 检查 `RESEND_API_KEY` 配置是否有效

**处置**
1. 等待熔断器半开（60s 后自动进入 half-open）探测恢复
2. 检查 `RESEND_API_KEY`：若失效，在 admin 配置接口更新（AES 加密存储）
3. 手动处理死信队列：确认 Resend 恢复后重放 `email:dead-letter` 中的积压消息
   ```bash
   redis-cli -h $REDIS_HOST XRANGE email:dead-letter - +
   ```

### 6.3 AES 密钥轮换失误导致配置解密失败

**症状**: admin 配置接口返回 500；日志含 `decrypt` / `cipher: message authentication failed`

**排查**
1. 检查 `ENCRYPTION_KEY` 是否被修改（AES-256-GCM 密钥用于加密 admin 配置中的敏感字段）
   ```bash
   echo -n "$ENCRYPTION_KEY" | sha256sum  # 与上次部署对比
   ```
2. 查看应用日志确认解密错误范围
   ```bash
   journalctl -u <service> --since "30 min ago" | grep -i decrypt
   ```

**处置**
1. 恢复原 `ENCRYPTION_KEY`：回滚环境变量至上次值
2. 重新加密敏感配置字段：用新密钥通过 admin 配置接口重新写入 `resend_api_key` 等字段
3. 密钥轮换流程固化：轮换前先解密所有字段 → 换密钥 → 重新加密写入

---

## 故障 7: Room 热路径性能（ADR-027）

**症状**
- 玩家报告输入延迟、周期性卡顿或 mass disconnect
- `room_lock_hold_seconds{operation="tick"}` P95 > 25ms
- `room_outbound_queue_depth` 持续 > 200
- `room_persist_lag_seconds` > 2s

**可能原因**: PG/Redis 延迟升高，异步 worker backlog 堆积；单房间玩家过多；慢客户端未断开

**排查**
```bash
curl -s localhost:8080/metrics | grep -E 'room_lock_hold|room_outbound|room_persist|outbox_lag'
```

**缓解**
1. 确认 PG/Redis 健康（见故障 1/2）；恢复依赖后 backlog 应回落
2. `room_outbound_queue_depth` 高：检查慢客户端 disconnect 率
3. `room_persist_lag_seconds` 高：检查 PG 连接池 `db_pool_*`
4. `outbox_lag_seconds` 高：确认 outbox publisher batch 运行中
5. 水平扩展 Hub 实例分散房间（ADR-005）
6. 调优 `PG_POOL_MAX_CONNS` / `REDIS_POOL_SIZE`（见 `.env.example`、[capacity-planning.md](capacity-planning.md)）

---

## Rollback

```bash
# Rollback to previous version（balloon-game 为 StatefulSet，namespace 为 balloon-game）
kubectl rollout undo statefulset/balloon-game -n balloon-game

# Verify rollout status
kubectl rollout status statefulset/balloon-game -n balloon-game
```
