# ADR-019: 持久化使用 raw SQL + pgx，不引入 ORM

## 状态: 已接受

## 上下文

后端需要持久化用户、游戏会话、结果、大厅状态、审计日志、Outbox 事件等实体。项目同时需要：
- 支持 PostgreSQL 16（当前默认）和 CockroachDB（可选，`DB_DIALECT=cockroach`）
- 高性能批量写入（游戏结果、`postgres.go:589-601` 批量 INSERT）
- 可预测的 SQL 执行计划（索引策略见 migration 000002-000004）
- golang-migrate 管理的版本化 schema 迁移

团队评估了 GORM、ent、sqlx 等方案。

## 决策

使用 **jackc/pgx/v5 驱动 + 手写参数化 SQL**，不引入 ORM：
- 所有查询通过 `$N` 占位符绑定参数（`postgres.go:597-601`）
- 连接池配置在 `PostgresStore` 初始化（`postgres.go:80-112`）
- Schema 变更通过 `backend/migrations/` 的 SQL 文件管理（11 个版本）
- CockroachDB 方言通过 `DB_DIALECT` 环境变量切换（`postgres.go:207-241`）

## 后果

**正面**
- SQL 完全透明，便于 `EXPLAIN ANALYZE` 和索引优化
- 无 ORM N+1 隐式查询风险
- 批量操作性能可控
- 迁移文件即 schema 文档

**负面**
- `postgres.go` 已达 954 行，成为 god-file
- 无编译时 SQL 类型检查（可考虑 sqlc 作为未来增强）
- 手动维护 struct ↔ row 映射，字段增减需同步多处
- 新开发者需熟悉 SQL 和 pgx API

**放弃的替代方案**
- GORM：隐式查询、迁移与 golang-migrate 冲突、CRDB 兼容性差
- ent：代码生成开销大，对当前表数量过度
- sqlx：pgx 已提供更现代的 API 和连接池
