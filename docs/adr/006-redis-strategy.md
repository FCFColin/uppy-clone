# ADR-006: Redis 策略（缓存与域拆分）

## 状态: 已接受

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

单一实例带来 SPOF、资源争抢、难以独立扩缩、故障爆炸半径大等风险。高 QPS 场景下 `LoadAllActiveLobbies` 与 `CheckRoom` 直接查 PG 成为读取瓶颈。

## 决策

### 1. Redis 读缓存层

缓存键：`lobby:list`（列表，TTL 30s）、`lobby:check:{code}`（单房间，TTL 30s）。写穿透：房间创建/删除/状态变更时同步更新 Redis + PG；读路径先查 Redis → miss 回源 PG → 回填。30s TTL 是一致性窗口与缓存命中率的权衡（大厅列表 30s 延迟可接受）。

### 2. Redis 域拆分

详见 ADR-029（stateful Redis-A 与 ephemeral Redis-B 物理分实例实现层）。

## 后果

**好处**：降低 PG 读压力（缓存层）、降低 P99 延迟、爆炸半径控制、扩缩独立。**坏处**：缓存一致性窗口（30s TTL）、Redis 内存增加、运维复杂度增加（2 实例）。

## 关联与实现现状

关联：ADR-005（房间状态管理）、ADR-007（异步处理）、ADR-014（多区域部署）、ADR-029（域拆分实现层）。✅ 读缓存层已实现；✅ Redis 域拆分已实现（Redis-A/Redis-B）；⬜ 多区域策略未实现（ADR-014 目标态）。
