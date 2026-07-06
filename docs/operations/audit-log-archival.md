# 审计日志数据归档策略

> 审计日志表 `audit_logs` 无限增长，需制定归档与保留策略以控制存储成本和查询性能。

## 现状

- `audit_logs` 表记录所有管理操作（用户创建/删除、密码变更、角色分配等）
- 每条记录包含：`id`, `actor_id`, `action`, `target_type`, `target_id`, `metadata`, `hmac`, `created_at`
- 无 TTL、无分区、无归档机制
- GDPR 要求数据最小化，但审计日志需满足合规保留期

## 决策

### 保留策略

| 数据类别 | 在线保留（PostgreSQL） | 归档存储 | 最终删除 |
|---------|---------------------|---------|---------|
| 安全事件（登录失败、权限变更） | 90 天 | 1 年 | 13 个月 |
| 管理操作（用户 CRUD） | 90 天 | 1 年 | 13 个月 |
| 系统操作（健康检查、部署） | 30 天 | 不归档 | 30 天 |

### 归档机制

1. **每月归档作业**（cron worker）：
   - 查询 `created_at < NOW() - INTERVAL '90 days'` 的记录
   - 导出为 Parquet 格式，上传到 GCS（`gs://audit-archive/<year>/<month>/`）
   - 删除已归档的 PostgreSQL 行
   - 记录归档元数据到 `audit_archive_log` 表

2. **查询降级**：
   - 在线查询仅扫描近 90 天数据
   - 历史查询通过 admin API 触发 GCS 回查（异步，返回预签名 URL）

### 分区策略

将 `audit_logs` 按月分区（`PARTITION BY RANGE (created_at)`），使归档删除变为 `DROP PARTITION`（O(1) vs DELETE O(n)）：

```sql
CREATE TABLE audit_logs (
    id UUID NOT NULL,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    target_type TEXT,
    target_id TEXT,
    metadata JSONB,
    hmac TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- 月度自动分区创建（由迁移脚本或 cron 维护）
CREATE TABLE audit_logs_2026_07 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
```

## 实现路径

1. **迁移**：将 `audit_logs` 改为分区表（需要数据迁移）
2. **Worker**：实现 `internal/worker/audit_archive.go`，每月 1 日执行
3. **API**：admin 端点 `/api/v1/admin/audit/archive` 触发手动归档
4. **监控**：Prometheus 指标 `audit_logs_total_rows`、`audit_archive_last_run_timestamp`

## GDPR 合规

- 归档数据中 `metadata` 可能含 PII（如 email），归档时对 PII 字段哈希化
- 用户行使删除权时，同步删除在线和归档中的相关记录
- 归档存储设置 Bucket Lock（WORM）防止篡改，但支持合规删除

## 参考

- `backend/internal/audit/`（审计日志写入）
- `docs/security/threat-model.md`（审计日志篡改威胁）
- `docs/security/self-check-checklist.md`（PII 处理）
