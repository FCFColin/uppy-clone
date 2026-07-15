# ADR-015: 分布式 SQL 采用 CockroachDB（多区域持久化）

> **归档说明**：本 ADR 已归档。本项目为学习工程，无需多区域部署，ADR-011 PostgreSQL 已足够。
> 如未来需要多区域，应重新评估而非复活本 ADR（技术栈、规模、合规需求均可能已变化）。

## 状态

已归档（原"提议中"；未曾落地，无对应代码实现）

## 日期

2026-06

## 关联

- ADR-011（PostgreSQL）、ADR-014（多区域拓扑）、ADR-016（区域本地房间）

## 上下文

单实例/单区域阶段使用 PostgreSQL（ADR-011）。进入超大型多区域 SaaS 阶段后，
单主 PostgreSQL 面临：

- **跨区域写延迟**：单主架构下，非主区域写入需跨洋往返，p99 不可控。
- **数据驻留合规**：GDPR 等要求欧盟用户数据驻留欧盟，单库无法行级分区。
- **区域故障域**：单主是全局单点；主区域故障导致全局写不可用。

## 决策

采用 **CockroachDB（CRDB）** 作为多区域生产持久化层：

1. **PostgreSQL 线协议兼容**：继续使用 `jackc/pgx`，应用代码与连接池基本不变；
   设计上通过 `DB_DIALECT=cockroach` 切换 dialect-gated 行为（`store.CurrentDialect`）。
   > ⚠️ 以上接口均为计划中，当前代码未实现 `DB_DIALECT` 读取、`store.CurrentDialect`、
   > `PostgresStore.ApplyCockroachMultiRegion`。
2. **REGIONAL BY ROW**：`users / game_sessions / game_results / lobby_states`
   行级数据驻留 `home_region`，本区域读写低延迟、满足数据驻留。
3. **GLOBAL 表**：`room_directory`（code→region 路由目录）与 `app_config`
   读多写少、需低延迟跨区强一致读，设为 GLOBAL。
4. **审计不可变性**：CRDB 触发器支持有限，改用权限层（`REVOKE UPDATE,DELETE`）
   保证 `audit_logs` append-only，与 PG 的 PL/pgSQL 触发器形成等效保证。

## 兼容与迁移

> ⚠️ 以下迁移路径均为设计规划，对应代码与迁移文件尚未实现。

- 共享 golang-migrate 集（`000001..000011`）保持纯 DDL、PG/CRDB 双兼容，本地/CI
  仍跑 PostgreSQL。
- CRDB 专属 locality 语句规划隔离在 `backend/migrations/cockroach/001_multiregion.sql`
  （**文件尚未创建**），设计上仅当 `DB_DIALECT=cockroach` 时由
  `PostgresStore.ApplyCockroachMultiRegion`（**方法尚未实现**）施加，不污染 PG/CI。
- 集成测试 `tests/integration/cockroach_test.go`（`TestCockroachDB_MigrationCompatibility`）
  规划用单节点 CRDB 容器验证共享 schema 在 CRDB 线协议上的兼容性（**测试尚未实现**，
  Docker 不可用自动跳过）。
- 多区域 locality（REGIONAL BY ROW / GLOBAL）需多区域集群，于 staging 验证。

## 后果

- **优点**：水平可扩展的强一致 SQL、行级数据驻留、区域故障隔离、应用层改动小。
- **代价**：CRDB 运维复杂度、部分 PG 特性（某些触发器/扩展）不可用、跨区域写仍有
  一致性延迟（通过 REGIONAL BY ROW 与 follower reads 缓解）。
- **回退**：设计上 `DB_DIALECT=postgres` 即可退回单区域 PostgreSQL（schema 双兼容）。
  当前 `DB_DIALECT` 未实现，默认即为 PostgreSQL。
