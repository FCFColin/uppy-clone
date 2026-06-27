# ADR-017: Bounded Context Map

## Status

Accepted (2026-06-25)

> 编号说明：本 ADR 原编号为 011，与「PostgreSQL 持久化」(ADR-011) 冲突，已去重改为 017。

## Context

多人气球游戏包含实时对战、认证、管理配置三个业务语言不同的子域。需明确边界以降低 `game` 包对 `store` 的直接依赖。

## Decision

| Bounded Context | 类型 | 核心聚合 | 集成模式 |
|-----------------|------|----------|----------|
| **Game Play** | Core | `Room` / `GameState` | 内存 + WS；结果异步到 PG |
| **Identity & Access** | Supporting | `User` / JWT / MagicLink | REST；Redis 会话 |
| **Lobby Registry** | Supporting | `Hub` / `LobbyState` | REST + cursor 分页 |
| **Admin & Config** | Supporting | `AppConfig` | REST + RBAC |
| **Integration** | Generic | Outbox / Workers | Redis Streams |

### Context Map（关系）

```
[Identity] --customer/supplier--> [Lobby Registry]
[Lobby Registry] --partnership--> [Game Play]
[Game Play] --published language (events)--> [Integration]
[Admin] --ACL--> [Identity], [Integration]
```

## Consequences

- Handler 直接访问 `auth` / `game` 包与 `store` 基础设施（ADR-024 方案 A，2026-06-26）
- ~~Handler 通过 Service 层访问跨上下文操作~~ — **已废弃**：`internal/service` 已删除
- 领域事件经 Outbox 发布，不直接跨上下文改状态
- 游戏热路径不依赖 PG 同步写入
