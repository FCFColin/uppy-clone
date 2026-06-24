# ADR-008: Audit Log Tamper-Proof Persistence

> 状态: Accepted
> 日期: 2026-06-23

## 上下文

审计日志是不可否认性（non-repudiation）的关键证据，满足 SOC2/ISO27001 合规要求。此前的实现仅将审计日志写入 stdout（`backend/internal/audit/audit.go`），存在以下问题：

- **可篡改**：任何拥有日志访问权限的人（运维、SIEM 管理员）均可修改或删除日志文件，无法检测篡改
- **无持久化**：进程重启后日志仅存在于 stdout 缓冲区或外部采集器中，应用自身无法查询历史审计记录
- **无篡改验证**：即使日志被修改，应用也无法发现链路断裂

此外，`Hub.CreateRoom` 与 `Hub.RemoveRoom` 中的 `audit.Log` 调用使用 `context.Background()`，丢失了请求链路的 `trace_id`/`request_id`，导致审计记录无法与分布式追踪关联。

## 决策

实现基于 HMAC-SHA256 链式哈希的审计日志防篡改持久化：

### 1. 数据库表与不可变性触发器

创建 `audit_logs` 表（迁移 `000006_create_audit_logs.up.sql`），包含 `prev_hash`、`this_hash` 字段。通过 PostgreSQL 触发器 `prevent_audit_log_modification` 禁止 `UPDATE` 和 `DELETE`，在数据库层强制不可变性——即使应用层被绕过（如直连 DB 执行 SQL）也无法篡改历史记录。

### 2. HMAC 链式哈希

每条记录的 `this_hash = HMAC-SHA256(secret, prev_hash || payload)`：
- `prev_hash`：上一条记录的 `this_hash`（首条为空字符串）
- `payload`：审计条目的 JSON 序列化
- `secret`：从 `AUDIT_SECRET` 环境变量读取，未设置时回退到 `JWT_SECRET`

篡改任何一条记录的 `payload` 或 `prev_hash`，会使后续所有记录的哈希验证失败，从而实现篡改可检测。

### 3. 异步非阻塞写入

通过 1024 容量的 buffered channel + 后台 goroutine 异步写入 DB：
- 审计写入不阻塞请求热路径
- channel 满时丢弃并记录告警，保护服务可用性（审计日志不应导致服务不可用）
- `CloseDBLogger` 优雅关闭，drain channel 确保已入队日志不丢失

### 4. 请求上下文传递

修改 `Hub.CreateRoom` 与 `Hub.RemoveRoom` 签名，接受 `ctx context.Context` 作为首参数，将请求 context 传递给 `audit.Log`，使审计记录可关联 `trace_id`/`request_id`。

### 5. 双写策略

`audit.Log` 同时写入 stdout（供 SIEM 实时采集）和 DB（供事后查询与篡改验证）。stdout 日志保留原有行为，确保向后兼容。

## 后果

### 好处

- **防篡改**：HMAC 链使任何记录修改可被检测；DB 触发器在数据库层强制不可变性
- **可查询**：审计日志持久化到 PostgreSQL，支持按时间、actor、action 查询
- **非阻塞**：异步 channel 写入不影响请求延迟
- **合规**：满足 SOC2/ISO27001 审计日志防篡改与不可否认性要求
- **可追溯**：请求 context 关联 trace_id，支持端到端审计追溯

### 坏处

- **额外 DB 写入**：每次审计增加一次 INSERT（通过异步 channel 缓解）
- **密钥保护**：链 secret 泄漏后可伪造完整链，必须严格保护 `AUDIT_SECRET`
- **故障恢复**：DB 故障恢复后需重新同步链的 `lastHash`（`InitDBLogger` 启动时自动从最后一条记录加载）
- **丢弃风险**：channel 满或进程崩溃时未入队的日志会丢失（trade-off：可用性优先于完整性）

## 备选方案

1. **仅追加文件（append-only file）**：实现简单，但难以查询和验证，且文件系统层面仍可被截断
2. **仅外部 SIEM**：应用自身无验证能力，依赖外部系统完整性
3. **区块链**：对单组织审计日志而言过度设计，引入不必要的复杂性与延迟
