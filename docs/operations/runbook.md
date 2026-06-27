# On-call Runbook

> 最后更新: 2026-06-24
> 适用范围: 多人网页气球飞行对战游戏 (Go + PostgreSQL + Redis)
>
> 企业为何需要：Runbook 是 SRE 标准实践。没有结构化的故障排查手册，on-call 工程师在
> 高压场景下容易遗漏关键排查步骤，导致 MTTR（平均恢复时间）拉长。每个条目遵循
> "症状 → 可能原因 → 排查命令 → 缓解步骤 → 根治方案" 五段式，对应 SRE 故障响应的
> 检测 → 定位 → 止血 → 根除 四阶段。

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
> 防止级联故障（雪崩）。状态变更通过 `metrics.CircuitBreakerState` 暴露为 Prometheus Gauge。
> 接到熔断告警时，先查本表定位是哪个依赖熔断，再跳转对应故障章节止血。

**指标查询**

```promql
circuit_breaker_state{name="<breaker>"}
# 0 = closed（健康），0.5 = half-open（探测中），1 = open（已熔断）
```

**熔断器清单**

| 熔断器 | 指标 label (`name`) | 触发阈值 | 熔断恢复超时 (Open→Half-open) | Half-open 探测请求数 | 关联故障章节 |
|--------|---------------------|---------|------------------------------|---------------------|-------------|
| postgres | `postgres` | 连续失败 > 5 次 | 30s | 3 | 故障 1 |
| redis | `redis` | 连续失败 > 5 次 | 15s | 3 | 故障 2 / 故障 6 |
| resend-api | `resend-api` | 连续失败 > 3 次 | 60s | 1 | 故障 6 |

> 说明：`Interval=60s` 为 closed 状态下连续失败计数的统计窗口；`Timeout` 为 open 状态等待多久后进入 half-open 探测。
> `resend-api` 作为外部 API 更保守：更早触发（3 次）、更长恢复等待（60s）、半开仅放行 1 个探测请求。

**一键查询所有熔断器状态**

```bash
curl -s localhost:8080/metrics | grep circuit_breaker_state
```

---

## 故障 1: PostgreSQL 不可用

**症状**
- `/health/ready` 返回 503，日志输出 `postgres: unavailable`
- API 大面积返回 500 Internal Server Error
- 游戏结果无法写入，`EndGameAndRecordResults` 报错
- 熔断器 `circuit_breaker_state{name="postgres"}` 值为 1（open）

**可能原因**
- PostgreSQL 进程宕机或被 OOM Kill
- 网络分区：应用与 DB 之间网络不通
- 连接池耗尽：`db_pool_in_use_conns` 达到 `MaxConns` 上限
- 磁盘满：WAL 日志无法写入
- 慢查询堆积：长事务阻塞连接归还

**排查命令**
```bash
# 1. 检查 PostgreSQL 是否可达
pg_isready -h $PGHOST -p $PGPORT

# 2. 检查活跃连接数与状态
psql $DATABASE_URL -c "SELECT count(*), state FROM pg_stat_activity GROUP BY state"

# 3. 检查连接池饱和度（应用侧）
curl -s localhost:8080/metrics | grep db_pool

# 4. 检查熔断器状态
curl -s localhost:8080/metrics | grep circuit_breaker_state

# 5. 检查磁盘空间
df -h

# 6. 检查慢查询
psql $DATABASE_URL -c "SELECT pid, now() - query_start AS duration, query FROM pg_stat_activity WHERE state = 'active' ORDER BY duration DESC LIMIT 10"
```

**缓解步骤**
1. 若进程宕机：`docker restart postgres`（或 failover 到只读副本）
2. 若连接池耗尽：重启应用 Pod 释放连接，临时调高 `MaxConns`
3. 若磁盘满：清理旧审计日志 `DELETE FROM audit_logs WHERE created_at < $threshold`
4. 若网络分区：切换到备用可用区，更新服务发现
5. 触发 failover 到副本：`pg_ctl promote -D /var/lib/postgresql/replica`

**根治方案**
- 增大连接池上限并配置 `SetMaxIdleConns` / `SetConnMaxLifetime`
- 启用熔断器（已实现 `circuitbreaker.go`），open 时快速失败而非堆积请求
- 部署 PostgreSQL 主从复制 + 自动 failover（Patroni 或 RDS Multi-AZ）
- 配置 `pg_stat_statements` 慢查询告警，定期优化索引
- 设置连接池使用率告警（>80% 触发 warning）

---

## 故障 2: Redis 不可用

**症状**
- Rate limiting 失效（fail-open，所有请求放行）
- Magic Link 登录失败：token 验证返回 invalid
- Refresh token 验证失败，用户被强制登出
- Lobby 列表查询降级到空列表
- 熔断器 `circuit_breaker_state{name="redis"}` 值为 1（open）

**可能原因**
- Redis 进程宕机或被 OOM Kill
- 内存耗尽：`used_memory` 达到 `maxmemory` 上限
- 网络分区：应用与 Redis 之间网络不通
- 持久化阻塞：RDB/AOF fork 子进程导致主线程卡顿
- 大量 Key 过期同时触发主动淘汰

**排查命令**
```bash
# 1. 检查 Redis 是否可达
redis-cli -h $REDIS_HOST -p $REDIS_PORT ping

# 2. 检查内存使用
redis-cli -h $REDIS_HOST info memory | grep -E "used_memory_human|maxmemory_human|used_memory_peak"

# 3. 检查连接数
redis-cli -h $REDIS_HOST info clients | grep connected_clients

# 4. 检查慢日志
redis-cli -h $REDIS_HOST slowlog get 10

# 5. 检查熔断器状态
curl -s localhost:8080/metrics | grep circuit_breaker_state

# 6. 检查 Redis 连接池
curl -s localhost:8080/metrics | grep redis_pool
```

**缓解步骤**
1. 若进程宕机：`docker restart redis`
2. 若内存满：清理低优先级 Key `redis-cli --scan --pattern "rate_limit:*" | xargs redis-cli del`
3. 临时降级：启用 in-memory 降级模式（`degradation.go` 已实现），rate limit 降级为本地计数
4. 若熔断器 open：等待半开探测周期（默认 30s）自动恢复
5. 扩容 Redis 内存：调整 `maxmemory` 配置

**根治方案**
- 部署 Redis Cluster 或 Sentinel 实现高可用与自动 failover
- 增大 Redis 内存上限，配置 `maxmemory-policy: allkeys-lru`
- 为不同业务设置独立 DB 或实例，隔离故障域
- 监控 `used_memory / maxmemory` 比率，>80% 触发告警
- Magic Link / Refresh token 写入时设置合理 TTL，避免堆积

---

## 故障 3: WebSocket 连接洪水

**症状**
- 内存使用激增，`go_goroutines` 指标快速上升
- API 响应变慢，`http_request_duration_seconds` P99 升高
- `ws_connections` 指标异常飙升
- 服务可能被 OOM Kill

**可能原因**
- Bot 攻击：恶意脚本批量建立 WebSocket 连接
- 病毒式流量：社交媒体传播导致真实用户激增
- 客户端未正确实现重连退避，断线后疯狂重连
- 缺少连接级速率限制

**排查命令**
```bash
# 1. 检查 WebSocket 连接数
curl -s localhost:8080/metrics | grep ws_connections

# 2. 检查 TCP 连接数
ss -tn state established | wc -l

# 3. 按来源 IP 统计连接数
ss -tn state established | awk '{print $4}' | sort | uniq -c | sort -rn | head -20

# 4. 检查 goroutine 数
curl -s localhost:8080/metrics | grep go_goroutines

# 5. 检查内存使用
curl -s localhost:8080/metrics | grep go_memstats_alloc_bytes

# 6. 检查 WS 连接指标
curl -s localhost:8080/metrics | grep ws_connection_total
```

**缓解步骤**
1. 启用/收紧 WebSocket 连接级速率限制（`CheckRateLimit` 已实现）
2. 封禁高频 IP：在 WAF 或 iptables 层 `iptables -A INPUT -s <ip> -j DROP`
3. 临时降低 `MaxWSConnections` 上限，拒绝新连接保护存量用户
4. 水平扩容应用实例，分散连接压力
5. 若为 Cloud Run：调高 `max-concurrent-requests` 并增加实例数

**根治方案**
- 部署 WAF（Cloudflare / AWS WAF）拦截恶意流量，配置 Bot 防护规则
- 实现自动扩缩容（HPA / Cloud Run autoscaling）应对流量峰值
- 客户端实现指数退避重连（exponential backoff + jitter）
- 按 IP 限流 + 按用户限流双层防护
- 监控 `ws_connections` 增长速率，异常突增触发告警

---

## 故障 4: 游戏 tick 延迟

**症状**
- 玩家反馈游戏卡顿、物理抖动
- `ws_message_duration_seconds` P99 超过 100ms
- 游戏画面出现位置回退（客户端插值补偿失败）
- `process_cpu_seconds_total` 增长率异常

**可能原因**
- CPU 饱和：房间数过多，tick goroutine 抢占不到 CPU 时间片
- GC 压力：高频分配导致 `go_gc_duration_seconds` 频繁触发
- 锁竞争：`room.mu.Lock` 持锁时间过长，阻塞 broadcast
- 热路径分配过多：`EncodeSnapshot` / `buildSnapshot` 每帧分配大量对象

**排查命令**
```bash
# 1. CPU profile（需启用 pprof，端口 6060）
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 2. 检查 CPU 使用率
curl -s localhost:8080/metrics | grep process_cpu_seconds_total

# 3. 检查 GC 频率与耗时
curl -s localhost:8080/metrics | grep go_gc_duration_seconds

# 4. 检查活跃房间与玩家数
curl -s localhost:8080/metrics | grep game_active_rooms
curl -s localhost:8080/metrics | grep game_active_players

# 5. 检查 WS 消息处理延迟
curl -s localhost:8080/metrics | grep ws_message_duration_seconds

# 6. 内存分配 profile
go tool pprof http://localhost:6060/debug/pprof/allocs
```

**缓解步骤**
1. 临时降低 tick rate（如从 30fps 降至 20fps），减少 CPU 负载
2. 水平扩容：将部分房间迁移到新实例（需 Hub 分片支持）
3. 重启高负载实例释放累积的内存碎片
4. 若为 GC 压力：临时调高 `GOGC` 环境变量（如 `GOGC=200`）降低 GC 频率

**根治方案**
- 优化热路径：使用 `sync.Pool` 复用 `[]byte` 与 `PlayerState` 切片（Task P1-6）
- `EncodeSnapshot` 改用手写二进制编码消除反射开销（Task P1-7）
- `readPump` 对 `MsgPing` 不创建 span，`MsgTap` 采样追踪（Task P1-5）
- 房间分片：按 `roomCode` hash 分配到不同 Hub 实例
- 设置 CPU 使用率告警（>80% 持续 5 分钟）

---

## 故障 5: 磁盘/内存满

**症状**
- 服务被 OOM Kill，容器重启次数增加
- 日志写入失败：`write: no space left on device`
- PostgreSQL 无法写入 WAL：`could not write to file "pg_wal/..."`
- Redis 拒绝写入：`OOM command not allowed when used memory > 'maxmemory'`

**可能原因**
- 日志文件累积未轮转
- 内存泄漏：goroutine 泄漏或 `sync.Pool` 未正确归还
- 审计日志表无限增长
- Redis Key 未设置 TTL 导致堆积
- 容器内存限制过低

**排查命令**
```bash
# 1. 检查磁盘空间
df -h

# 2. 检查内存使用
free -m

# 3. 检查容器资源使用
docker stats --no-stream

# 4. 检查日志文件大小
du -sh /var/log/* | sort -rh | head -10

# 5. 检查 goroutine 数（泄漏指标）
curl -s localhost:8080/metrics | grep go_goroutines

# 6. 检查堆内存分配
go tool pprof http://localhost:6060/debug/pprof/heap

# 7. 检查审计日志表大小
psql $DATABASE_URL -c "SELECT count(*) FROM audit_logs"
```

**缓解步骤**
1. 清理旧日志：`journalctl --vacuum-time=2d` 或 `truncate -s 0 /var/log/app.log`
2. 清理审计日志：`DELETE FROM audit_logs WHERE created_at < extract(epoch from now() - interval '30 days') * 1000`
3. 重启 Pod 释放泄漏内存：`kubectl rollout restart deployment/<name>`
4. 清理 Redis 无 TTL Key：`redis-cli --scan --pattern "*" | head -1000` 检查后清理
5. 临时扩容磁盘/内存

**根治方案**
- 配置日志轮转：`logrotate` 或应用层 lumberjack，限制单文件大小与保留份数
- 设置容器内存 limit 与 request，配合 OOM 告警提前预警
- 审计日志表设置定时清理任务（保留 90 天）
- 修复内存泄漏：使用 pprof 定位泄漏点，确保 `sync.Pool` 归还、context 正确 cancel
- Redis 所有 Key 强制设置 TTL，定期扫描无 TTL Key
- 监控磁盘使用率（>80% warning，>90% critical）

---

## 故障 6: 认证服务异常

> 认证 SLO：成功率 99.9%、p99 < 500ms（详见 `docs/slo.md` §2.1）。
> 认证失败直接消耗 Error Budget（43.2 分钟/月），属 P1 级响应（15 分钟内介入）。

### 6.1 Refresh token 验证失败率突增

**症状**
- `/api/v1/auth/refresh` 返回 401 比例 > 5%
- 用户大面积被强制登出，`auth_requests_total{status="401"}` 上升
- 可能伴随 Redis 熔断器 `circuit_breaker_state{name="redis"}=1`

**排查**
1. 检查 `JWT_SECRET` 是否被轮换：refresh token 用旧密钥签发，新密钥无法验证 → 全部 401
   ```bash
   # 对比部署环境与上次部署的 JWT_SECRET 哈希
   echo -n "$JWT_SECRET" | sha256sum
   ```
2. 检查 Redis 是否可用：refresh token 存储在 Redis（jti 撤销列表 + refresh token 存储）
   ```bash
   redis-cli -h $REDIS_HOST ping
   curl -s localhost:8080/metrics | grep circuit_breaker_state
   ```
3. 检查 token TTL 配置是否被误改（access token TTL / refresh token TTL）

**处置**
1. 回滚 `JWT_SECRET` 变更：恢复至上次部署的密钥值，使存量 refresh token 可重新验证
2. 恢复 Redis：见故障 2
3. 临时延长 token TTL：降低刷新频率，争取修复时间（需评估安全权衡）
4. 若密钥必须轮换：发布公告 + 引导用户重新登录，接受短期 401 峰值

### 6.2 Magic Link 邮件投递失败（Resend API 熔断器排查）

**症状**
- 用户报告收不到 magic link 邮件
- `circuit_breaker_state{name="resend-api"}=1`（open）
- `email:dead-letter` Redis Stream 长度持续增长

**排查**
1. 检查 Resend 熔断器状态
   ```bash
   curl -s localhost:8080/metrics | grep 'circuit_breaker_state{name="resend-api"}'
   # 0=closed, 0.5=half-open, 1=open
   ```
2. 检查 Resend API 状态页：https://resend.com/status
3. 检查死信队列长度（投递失败积压）
   ```bash
   redis-cli -h $REDIS_HOST XLEN email:dead-letter
   ```
4. 检查 `RESEND_API_KEY` 配置是否有效（可能过期或被撤销）

**处置**
1. 等待熔断器半开（60s 后自动进入 half-open）探测恢复，避免手动重置造成流量冲击
2. 检查 `RESEND_API_KEY` 配置：若失效，在 admin 配置接口更新（AES 加密存储）
3. 手动处理死信队列：确认 Resend 恢复后重放 `email:dead-letter` 中的积压消息
   ```bash
   # 查看死信内容（确认后由 worker 重放或人工触发）
   redis-cli -h $REDIS_HOST XRANGE email:dead-letter - +
   ```
4. 若 Resend 长时间不可用：评估切换备用邮件服务商（需预留适配层）

### 6.3 AES 密钥轮换失误导致配置解密失败

**症状**
- admin 配置接口（`GET /api/v1/admin/config`、`PATCH /api/v1/admin/config`）返回 500
- 日志含 `decrypt` / `cipher: message authentication failed` 错误
- 其他依赖加密配置的功能（如 Resend API Key 解密后投递邮件）连锁失败

**排查**
1. 检查 `ENCRYPTION_KEY` 环境变量是否被修改：AES-256-GCM 密钥用于加密 admin 配置中的敏感字段（如 `resend_api_key`）
   ```bash
   echo -n "$ENCRYPTION_KEY" | sha256sum
   # 与上次部署对比
   ```
2. 检查 admin config 中 `resend_api_key` 等字段是否用旧密钥加密：密钥轮换后旧密文无法解密
3. 查看应用日志确认解密错误范围
   ```bash
   journalctl -u <service> --since "30 min ago" | grep -i decrypt
   ```

**处置**
1. 恢复原 `ENCRYPTION_KEY`：回滚环境变量至上次值，使存量密文可重新解密
2. 重新加密敏感配置字段：用新密钥通过 admin 配置接口重新写入 `resend_api_key` 等字段（先恢复旧密钥解密读出明文，再用新密钥加密写入）
3. 若旧密钥已丢失：需在 Resend 控制台重新生成 API Key，再通过 admin 接口写入
4. 密钥轮换流程固化：轮换前先解密所有字段 → 换密钥 → 重新加密写入，避免直接替换密钥导致密文失效
