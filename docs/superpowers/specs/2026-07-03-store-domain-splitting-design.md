# Store 域拆分设计

## 目标

将 `internal/store` 中的 `PostgresStore`（31 方法）和 `RedisStore`（~28 方法）按业务域拆分为独立 repository 类型，每个持有自己的 pool + breaker。

## 现状

- `PostgresStore` 同时实现 `UserStore`、`ConfigStore`、`LeaderboardStore`（handler 接口）
- `RedisStore` 同时实现 `TokenStore`、`AdminCache` 和内部 room registry/cache 接口
- 所有 handler 通过同一 `*PostgresStore`/`*RedisStore` 获取功能
- `base_repository.go` 已提供共享 `pgPool` + breaker 机制

## 方法：提取函数式

不复制 SQL。每个 `PostgresStore` 方法的 SQL 逻辑提取为包级原始函数（`*Raw`），Repository 和旧 `PostgresStore` 都调同一组原始函数。

```
原始函数（一次定义） → 被 PostgresStore 调用（向后兼容）
                    → 被 UserRepository 调用（新路径）
```

## 五个 PostgreSQL Repository

| Repository | 方法数 | 原始函数来源 |
|------------|--------|-------------|
| `UserRepository` | 7 | `postgres_users_crud.go`, `postgres_users_read.go`, `postgres_users_gdpr.go` |
| `ConfigRepository` | 2 | `postgres_config.go` |
| `LobbyRepository` | 5 | `postgres_lobbies_save.go`, `postgres_lobbies_query.go`, `postgres_lobbies_list.go` |
| `ResultRepository` | 5 | `postgres_results.go` |
| `OutboxRepository` | 1 | `postgres_outbox.go` |

## 五个 Redis Store

| Store | 方法数 | 源文件 |
|-------|--------|-------------|
| `AuthSessionStore` | 10 | `redis_auth_session.go` |
| `MagicLinkStore` | 4 | `redis_magiclink.go` |
| `RateLimitStore` | 1 | `redis_ratelimit.go` |
| `RoomRegistryStore` | 4 | `redis_room_registry.go`, `redis_room_registry_info.go` |
| `LobbyCacheStore` | 7 | `redis_lobby_cache.go`, `redis_lobby_read_cache.go` |

## 依赖关系

- 所有 Repository 嵌入 `baseRepository`（pool + breaker）
- Handler 接口定义不变
- 无新依赖引入

## 服务器初始化变更

`server_init.go` 创建独立 Repository 实例，handler 接收对应的接口类型。
