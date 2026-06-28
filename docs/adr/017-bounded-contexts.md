# ADR-017: 限界上下文（Bounded Contexts）划分

## 状态

已接受

## 日期

2026-06

## 上下文

多人气球游戏包含实时对战、认证、管理配置三个业务语言不同的子域。需明确边界以降低 `game` 包对 `store` 的直接依赖。

> 编号说明：本 ADR 原编号为 011，与 PostgreSQL ADR-011 冲突，已改为 017。

## 决策

| 限界上下文 | 类型 | 核心聚合 | 集成模式 |
|------------|------|----------|----------|
| **Game Play** | 核心 | `Room` / `GameState` | 内存 + WS；结果异步到 PG |
| **Identity & Access** | 支撑 | `User` / JWT / MagicLink | REST；Redis 会话 |
| **Lobby Registry** | 支撑 | `Hub` / `LobbyState` | REST + cursor 分页 |
| **Admin & Config** | 支撑 | `AppConfig` | REST + RBAC |
| **Integration** | 通用 | Outbox / Workers | Redis Streams |

### 上下文关系

```
[Identity] --customer/supplier--> [Lobby Registry]
[Lobby Registry] --partnership--> [Game Play]
[Game Play] --published language (events)--> [Integration]
[Admin] --ACL--> [Identity], [Integration]
```

## 后果

- Handler 直接访问 `auth` / `game` 与 `store`（ADR-024 方案 A）
- ~~Handler 通过 Service 层跨上下文~~ — `internal/service` 已删除
- 领域事件经 Outbox 发布，不直接跨上下文改状态
- 游戏热路径不依赖 PG 同步写入

## 关联

- ADR-024
