# CockroachDB 多区域迁移运行手册

> 关联 ADR-014（多区域拓扑）、ADR-015（分布式 SQL）、ADR-016（区域本地房间）。

本手册描述从单区域 PostgreSQL 迁移到多区域 CockroachDB 的步骤。应用通过
`DB_DIALECT` 环境变量切换：`postgres`（默认）或 `cockroach`。

## 1. 应用侧配置

| 变量 | 值 | 说明 |
|------|-----|------|
| `DB_DIALECT` | `cockroach` | 启用 CRDB dialect-gated 行为 |
| `DATABASE_URL` | `postgresql://<user>@<host>:26257/balloon?sslmode=verify-full&sslrootcert=...` | CRDB 默认端口 26257；生产用 TLS |
| `DEPLOY_REGION` | 如 `us-east1` | 写入 `home_region`，决定行驻留区域 |

`pgx` 同时支持 PG 与 CRDB 线协议，连接池配置（`PG_POOL_*`）通用。

## 2. Schema 迁移分层

1. **共享 golang-migrate 集**（`backend/migrations/000001..000011`）：纯 DDL，
   PG/CRDB 双兼容，正常 `migrate up` 即可在 CRDB 应用。
2. **CRDB 专属 locality**（`backend/migrations/cockroach/001_multiregion.sql`）：
   `REGIONAL BY ROW` / `GLOBAL` / 审计权限。仅当 `DB_DIALECT=cockroach` 时由
   `PostgresStore.ApplyCockroachMultiRegion` 在启动迁移后自动施加。

## 3. 集群级一次性准备（手动，在施加 locality 前）

```sql
-- 创建数据库与角色
CREATE DATABASE balloon;
CREATE USER app_user;
CREATE USER migrator;

-- 设置多区域（区域名需与云厂商区域一致）
ALTER DATABASE balloon SET PRIMARY REGION "us-east1";
ALTER DATABASE balloon ADD REGION "europe-west1";
ALTER DATABASE balloon ADD REGION "asia-southeast1";

-- 权限（最小权限，对应 ADR-008/009 在 PG 上的等价物）
GRANT SELECT, INSERT, UPDATE, DELETE ON DATABASE balloon TO app_user;
GRANT ALL ON DATABASE balloon TO migrator;
```

完成后再启动应用（或运行 `--migrate-only`），`001_multiregion.sql` 会把核心表设为
`REGIONAL BY ROW`、目录/配置表设为 `GLOBAL`、并撤销 `app_user` 对审计表的写权限。

## 4. 数据迁移

1. 双写/影子流量阶段：先以 `DB_DIALECT=postgres` 运行，导出后用 `cockroach import`
   或逻辑复制工具导入 CRDB。
2. 回填 `home_region`：按用户/会话归属区域 `UPDATE ... SET home_region = ...`，
   CRDB 据此计算 `crdb_region`。
3. 切流：将 `DB_DIALECT` 切到 `cockroach`，灰度区域逐步接管。

## 5. 验证

```bash
# 单节点 CRDB 兼容性集成测试（需 Docker）
cd backend && go test ./tests/integration/ -run TestCockroachDB_MigrationCompatibility -count=1
```

多区域 locality 需在多区域 staging 集群验证 `SHOW CREATE TABLE users` 含
`LOCALITY REGIONAL BY ROW`、`room_directory` 含 `LOCALITY GLOBAL`。

## 6. 回退

将 `DB_DIALECT` 切回 `postgres` 并指向 PostgreSQL 实例即可；共享 schema 双兼容，
应用代码无需改动。
