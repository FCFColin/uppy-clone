# ADR 索引 (Architecture Decision Records)

本目录记录项目所有重要架构决策。每个 ADR 包含：上下文、决策、后果。

> **前提**：[ADR-000 项目章程](000-project-charter.md) 定义了本项目的目标与非目标
> （以小游戏为载体练习企业级架构）。评审任何 ADR 的"复杂度是否合理"前，请先读章程。

## ADR 列表

| 编号 | 标题 | 状态 | 日期 |
|------|------|------|------|
| 000 | 项目章程：目标与非目标 (Charter) | 已接受 | 2026-06 |
| 001 | 从 Cloudflare Workers 迁移到 Go + PostgreSQL | 已接受 | 2025-01 |
| 002 | WebSocket 使用二进制协议而非 JSON | 已接受 | 2025-01 |
| 003 | Magic Link 认证而非传统密码 | 已接受 | 2025-01 |
| 004 | 引入熔断器保护下游依赖 | 已接受 | 2025-02 |
| 005 | Hub 无状态化与房间状态外置（区域内 owner 反向代理 + 租约接管） | 已接受 | 2025-02 |
| 006 | Redis 读缓存层 | 已接受 | 2026-06 |
| 007 | Redis Stream 消息队列 | 已接受 | 2026-06 |
| 008 | 审计日志防篡改持久化 | 已接受 | 2026-06 |
| 009 | Transactional Outbox 模式 | 已接受 | 2026-06 |
| 010 | 异步邮件发送 | 已接受 | 2026-06 |
| 011 | PostgreSQL 作为持久化数据库（单区域） | 已接受 | 2026-06 |
| 012 | Chi 作为 HTTP 路由框架 | 已接受 | 2026-06 |
| 013 | 部署平台从 Cloud Run 收敛到 GKE（多区域） | 已废弃 | 2026-06 |
| 014 | 多区域拓扑与全局就近路由 | 已接受 | 2026-06 |
| 015 | 分布式 SQL 采用 CockroachDB | 提议中 | 2026-06 |
| 016 | 区域本地房间与跨区域重定向 | 提议中 | 2026-06 |
| 017 | 限界上下文（Bounded Contexts）划分 | 已接受 | 2026-06 |
| 018 | 前端采用 Vanilla TypeScript 多页应用（MPA） | 已接受 | 2026-06 |
| 019 | 持久化使用 raw SQL + pgx，不引入 ORM | 已接受 | 2026-06 |
| 020 | 前端 dist 嵌入 Go 二进制单镜像部署 | 已接受 | 2026-06 |
| 021 | Monorepo 结构（Go + TS + infra + docs 同仓） | 已接受 | 2026-06 |
| 022 | 字段级 AES-256-GCM 加密 + HMAC email_hash 索引 | 已接受（部分落地） | 2026-06 |
| 023 | 混合测试策略 — testcontainers + miniredis | 已接受（部分落地） | 2026-06 |
| 024 | Application Service 层状态与裁决需求 | 已接受 | 2026-06 |
| 025 | 前端受控状态管理 | 已接受 | 2026-06 |
| 026 | 移除 Casbin，采用轻量 RBAC 策略表 | 已接受 | 2026-06-26 |
| 027 | Room 出站管道（锁外广播与异步持久化） | 已接受 | 2026-06 |
| 028 | Clean Architecture Interface-Driven Decoupling | 已接受 | 2026-07-03 |
| 029 | Redis 域拆分策略 | 已接受 | 2026-07-06 |

## ADR 流程

1. **提议**: 创建新 ADR 文件，状态为"提议中"
2. **讨论**: 团队评审，收集反馈
3. **决策**: 达成共识后状态改为"已接受"或"已否决"
4. **归档**: 不再适用的决策标记为"已废弃"

命名规范: `NNN-简短标题.md`，NNN 为三位数字序号。

## 索引一致性校验

CI 通过以下脚本校验本索引与实际 ADR 文件一致：

```bash
# 校验 README 表格行数 = docs/adr/ 下 ADR 文件数（排除 README.md）
adr_count=$(find docs/adr -name '0*.md' | wc -l)
table_rows=$(grep -c '^\| 0' docs/adr/README.md)
[ "$adr_count" -eq "$table_rows" ] || { echo "ADR index mismatch"; exit 1; }
```
