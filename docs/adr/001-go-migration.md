# ADR-001: 从 Cloudflare Workers 迁移到 Go + PostgreSQL

## 状态: 已接受

## 上下文

项目最初使用 Cloudflare Workers + Durable Objects + D1 构建。随着功能增长，遇到以下限制：
- Durable Objects 的 128MB 内存限制
- D1 (SQLite) 的并发写入瓶颈
- Workers 的 30 秒 CPU 时间限制
- 无法使用 WebSocket 长连接进行实时游戏

## 决策

迁移到 Go + PostgreSQL + Redis 架构：
- Go: 高性能并发模型，goroutine 适合 WebSocket 长连接
- PostgreSQL: 成熟的关系型数据库，支持高并发读写
- Redis: 内存缓存，适合 rate limiting 和 session 存储
- 部署演进见 [ADR-014](014-multi-region-deployment.md)（GKE 多区域拓扑）

## 后果

**正面**
- 无 CPU 时间限制，物理模拟可运行完整 tick 循环
- PostgreSQL 并发写入能力远超 D1
- Go 的 goroutine 模型天然适合 WebSocket

**负面**
- 失去 Cloudflare 的全球边缘网络
- 需要自行管理数据库和缓存
- 现以 GKE 为目标部署平台（ADR-014）
