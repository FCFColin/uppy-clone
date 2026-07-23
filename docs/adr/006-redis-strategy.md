# ADR-006: Redis 策略（缓存与域拆分）

## 状态: 已接受

> **合并说明（2026-07-22）**：本 ADR 已合并原 ADR-029《Redis 域拆分策略》的全部内容。
> ADR-029 是本 ADR 决策 2（Redis 域拆分）的实现层细化，二者主题重叠。ADR-029 的唯一性内容
> （三阶段渐进策略、物理隔离实现细节、环境变量、理由与权衡）已并入本 ADR 决策 2。
> ADR-029 文件已删除，本 ADR 为 Redis 策略与域拆分的唯一权威记录。

## 上下文

系统使用 Redis 承载多个功能域：

| 域 | 用途 | Key 前缀 | 失败影响 |
|---|------|---------|---------|
| 读缓存 | 大厅列表、房间信息查询 | `lobby:list:`, `lobby:check:` | 缓存穿透 |
| 房间注册表 | room registry / owner routing | `room:` | WS 路由失败 |
| JWT 撤销 | token revocation | `jwt_revoked:` | 认证降级 |
| Magic Link | 一次性令牌 | `magic:` | 登录失败 |
| 限流 | rate limiting | `rl:` | 限流失效 |
| Refresh Token | 刷新令牌轮换 | `refresh_token:` | 登出失效 |
| 邮件队列 | 异步邮件发送 | `email:` | 邮件延迟 |
| Outbox | 事件发布 | `outbox:*` | 事件延迟 |

单一实例带来 SPOF、资源争抢、难以独立扩缩、故障爆炸半径大等风险。高 QPS 场景下 `LoadAllActiveLobbies` 与 `CheckRoom` 直接查 PG 成为读取瓶颈。具体风险：

1. **SPOF**：Redis 宕机影响所有功能域
2. **资源争抢**：限流 INCR 大量写入影响房间注册 GET 延迟
3. **难以独立扩缩**：缓存域需要更多内存，限流域需要更多 CPU
4. **故障爆炸半径大**：Pub/Sub 阻塞导致全功能不可用

## 决策

### 1. Redis 读缓存层

缓存键：`lobby:list`（列表，TTL 30s）、`lobby:check:{code}`（单房间，TTL 30s）。写穿透：房间创建/删除/状态变更时同步更新 Redis + PG；读路径先查 Redis → miss 回源 PG → 回填。30s TTL 是一致性窗口与缓存命中率的权衡（大厅列表 30s 延迟可接受）。

### 2. Redis 域拆分（逻辑域拆分 + 物理分实例）

采用**逻辑域拆分 + 物理分实例**渐进策略：

#### 阶段 1：逻辑隔离（跳过 — 直接进入阶段 2）

Redis DB 编号隔离在 go-redis v9 中受限（Cluster 模式不支持 `SELECT`），且无法解决物理 SPOF。直接实施阶段 2 物理隔离，以 `RedisCluster` 结构体统一管理两个 `*RedisStore`。

#### 阶段 2：物理隔离（已实现）

将高写入域（限流）和可丢弃域（缓存/幂等）拆分到独立 Redis 实例：

```
Redis-A (stateful):  房间注册 + JWT 撤销 + Magic Link + Refresh Token
                     + Email Queue + Game Results + Outbox + Pub/Sub
Redis-B (ephemeral): 限流 + 幂等缓存
```

- **Redis-A (stateful)**：使用 RDB+AOF 持久化，部署为 Sentinel HA
- **Redis-B (ephemeral)**：纯内存 `--maxmemory 64mb --maxmemory-policy allkeys-lru`，可容忍丢失

实现细节：
- `store/redis_cluster.go`：`RedisCluster` 结构体持有 `Stateful *RedisStore` 和 `Ephemeral *RedisStore`
- `config/env.go`：新增 `REDIS_EPHEMERAL_URL`，未设置时回退到 `REDIS_URL`（单实例向后兼容）
- 路由层：`EndpointRateLimit` 和 `IdempotencyMiddleware` 使用 `cluster.Ephemeral`
- 健康检查：`/health/degraded` 同时检测两个实例的熔断器状态
- `docker-compose.yml`：新增 `redis-ephemeral` 服务（端口 6380）
- `infra/k8s/base/redis-ephemeral.yaml`：K8s StatefulSet + Service

#### 阶段 3：多区域（已实现）

Redis-A 按区域本地部署（房间注册天然区域局部），Redis-B 每区域一套不跨区复制。

实现细节：
- `config/env.go`：新增 `REDIS_REGIONAL_URL`，设置后 stateful Redis 使用该 URL
- `REDIS_URL` 作为 fallback；`REDIS_EPHEMERAL_URL` 未设置时也回退到 `REDIS_REGIONAL_URL`
- K8s：每区域 overlay 部署各自的 `redis` 和 `redis-ephemeral` StatefulSet
- 不跨区复制 Redis（避免跨洋写放大）；跨区域只通过 CRDB GLOBAL room_directory 共享路由信息

> 注：Phase 3 依赖的 ADR-014 路由层已被 ADR-032 豁免裁剪，`REDIS_REGIONAL_URL` 环境变量保留但无下游消费者，重启多区域需先恢复路由层。

#### 理由

1. **爆炸半径**：限流 Redis 宕机不应影响进行中的游戏房间
2. **SLA 差异**：房间注册要求 <5ms p99，缓存可容忍 50ms
3. **扩缩独立**：限流 QPS 随用户增长线性增长，房间注册 QPS 随并发房间增长
4. **运维隔离**：限流 Redis 可安全重启（fail-open），房间注册不可

#### 权衡

- 增加运维复杂度（2 个 Redis 实例），但显著降低爆炸半径
- `REDIS_EPHEMERAL_URL` 未设置时自动回退到单实例模式，保证零配置兼容
- go-redis 连接池按域独立配置，支持独立熔断和监控

#### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `REDIS_URL` | Stateful Redis（未设置 `REDIS_REGIONAL_URL` 时使用） | `localhost:6379` |
| `REDIS_EPHEMERAL_URL` | Ephemeral Redis（限流+幂等） | 回退到 stateful URL |
| `REDIS_REGIONAL_URL` | 多区域 stateful Redis（Phase 3） | 空（单区域时使用 `REDIS_URL`） |
| `REDIS_PUBSUB_URL` | Pub/Sub 专用 Redis | 回退到 stateful URL |

#### 实现路径

1. ✅ `store/redis_cluster.go`：`RedisCluster` 持有 `Stateful` 和 `Ephemeral` 两个 `*RedisStore`
2. ✅ `config/env.go`：新增 `REDIS_EPHEMERAL_URL`、`REDIS_REGIONAL_URL` 环境变量
3. ✅ `server_init.go`：`initRedisCluster` 替代 `initRedis`，创建域分离的 Redis 集群
4. ✅ 路由层：限流和幂等使用 `cluster.Ephemeral`，其余使用 `cluster.Stateful`
5. ✅ `docker-compose.yml`：新增 `redis-ephemeral` 服务
6. ✅ `infra/k8s/base/redis-ephemeral.yaml`：K8s ephemeral Redis StatefulSet

RO-051 消费者侧窄接口收敛已完成（auth/middleware/worker/health/outbox 均通过窄接口注入，不再直接依赖 `*redis.Client`）。

## 后果

**好处**：降低 PG 读压力（缓存层）、降低 P99 延迟、爆炸半径控制、扩缩独立。**坏处**：缓存一致性窗口（30s TTL）、Redis 内存增加、运维复杂度增加（2 实例）。

## 关联与实现现状

关联：ADR-005（房间状态管理）、ADR-007（异步处理）、ADR-014（多区域部署）、ADR-023（miniredis 测试策略）、`docs/operations/runbook.md`（Redis 故障处理）。✅ 读缓存层已实现；✅ Redis 域拆分已实现（Redis-A/Redis-B）；⬜ 多区域策略未实现（ADR-014 目标态，路由层已被 ADR-032 豁免裁剪）。原 ADR-029（Redis 域拆分策略）已合并至此（2026-07-22）。
