# 企业级垂直审计 V2 修复规范（Enterprise Audit V2 Remediation）

> 本规范基于 plan2.md 的 8 维度深度自检审计，使用 TRAE-security-review 与 TRAE-code-review 技能完成。
> 审计日期：2026-06-23
> 审计对象：`d:\Project\多人网页游戏`（Go 后端 + TypeScript 前端的多人气球飞行对战游戏）
> 铁律：证据优先，所有发现引用具体文件名+行号

## Why

项目已具备较完善的工程基础（熔断器、重试、限流、幂等、RBAC、字段级加密、CSP、SBOM、cosign 签名），但在 8 个正交维度上仍存在 **1 项 CRITICAL、17 项 HIGH、30+ 项 MEDIUM** 的差距。其中最紧迫的问题包括：docker-compose 默认凭据可被直接用于生产、`.gitignore` 未包含 `.env` 导致密钥泄漏高风险、DeleteUserData 是空壳违反 GDPR、审计日志无防篡改、明文密码回退分支、Cookie Secure 标志在反向代理后失效等。

本规范的目标是：以"企业级工程师成长学习"为核心，按优先级修复所有 CRITICAL/HIGH 项，对 MEDIUM 项按就业价值与实施复杂度排序实施，对 LOW/INFO 项作为技术债跟踪。每个改动在代码注释或文档中记录"解决了什么问题、企业为何需要、做了什么工程权衡"，使其成为可向面试官展示的学习材料。

## What Changes

### 阶段 P0：CRITICAL/HIGH 安全与合规紧急修复（必须立即实施）

- **BREAKING** 移除 `docker-compose.yml` 中所有默认密钥（JWT_SECRET/ENCRYPTION_KEY/ADMIN_PASSWORD），改为 `${VAR:?required}` 强制注入
- **BREAKING** 移除 `backend/internal/handler/admin_password.go:14-22` 的明文密码回退分支，强制 bcrypt
- 修复 Cookie Secure 标志判断逻辑（`admin.go:101`、`quickplay.go:42,88`、`auth.go:232`），使用 `X-Forwarded-Proto` 头
- 修复 `admin.go:208-218` 密码修改流程，要求验证旧密码
- 实现失败登录锁定机制（Redis 计数器 + 锁定 15 分钟）
- 修复 `.gitignore` 未包含 `.env` 的 CRITICAL 漏洞，扫描历史泄漏
- 实现 DeleteUserData 真正的匿名化（GDPR Article 17 合规）
- 创建根目录 LICENSE 文件（MIT 全文）
- Docker 镜像 digest pinning（运行 `scripts/pin-digests.sh`）
- 审计日志防篡改持久化（`audit_logs` 表 + HMAC 链式哈希 + 触发器禁止 UPDATE/DELETE）

### 阶段 P1：SRE 基础设施与性能热路径（高价值修复）

- 创建 `docs/runbook.md`（5 类故障排查步骤）
- 定义 SLI/SLO/SLA + Error Budget（`docs/slo.md`）
- 集成 pprof + expvar 调试端点（`/debug/pprof/`、`/debug/vars`，生产环境鉴权保护）
- Burn Rate 多窗口告警规则（`deploy/alertmanager/rules.yml`）
- WebSocket 热路径优化（span 采样、DecodeTap 去除 append、span attribute 预分配）
- sync.Pool 复用 + `randFloat64` 去 `big.Int` 分配
- `EncodeSnapshot` 改用手写二进制编码（去除 `binary.Write` 反射）
- 魔法链接邮件发送异步化（Redis Stream + Worker）
- 游戏结果异步写入（ADR-007 实施，Redis Stream）
- Transactional Outbox 模式（`outbox_events` 表 + 发布器 goroutine）
- 启用 gocyclo/funlen/goconst/dupl linter
- 提取魔法数字为常量
- 定义哨兵错误替代字符串比较
- 消除重复代码（retry 嵌套、cookie 撤销、UUID、sanitizePlayerName）
- 删除死代码（18+ 个未使用导出函数）
- 删除 deprecated 函数（EndGameSession、RecordGameResults）

### 阶段 P2：开发者体验与代码质量（中价值修复）

- 创建 Makefile 统一命令（make dev/test/lint/build/run/migrate/seed/bench/audit）
- 创建 `.env.example`（含所有必填变量，明显占位符）
- 创建 `backend/.air.toml` 热重载配置
- 创建 `.devcontainer/devcontainer.json`
- 创建 `backend/tools.go` 锁定工具版本
- 创建数据库 seed 脚本（`backend/cmd/seed/main.go`）
- 创建 `docs/environments.md` 本地/生产差异文档
- 拆分 `main.go` 315 行 main 函数为 initLogger/initTracer/loadConfig 等
- 拆分 `HandleJoin`/`setEndGameAlarm`/`handleTap`/`UpdateConfig` 上帝函数
- health.go 纳入熔断器状态 + Hub 负载
- Defense-in-Depth 输入校验加固（nickLen、room code、token 长度、password 长度）
- N+1 修复（批量 INSERT、refresh token 反向索引）
- Postmortem 模板（`docs/templates/postmortem.md`）
- /metrics 端点添加认证
- 缩短 admin JWT 过期时间至 30 分钟
- JWT 密钥长度校验（>= 32 字节）
- 路径遍历增强检查
- WebSocket Origin 校验修复
- 密钥轮换机制（JWT 多密钥 + AES 版本号）
- 异常行为检测（多 IP 登录告警）
- 数据库用户最小权限（app_user + migrator 角色）
- 配置 Dependabot
- pre-commit 集成 detect-secrets/gitleaks
- 优雅关闭等待异步任务（hub.CloseAllRooms + WaitGroup）

### 阶段 P3：领域建模重构（高复杂度，长期改进）

- 引入充血模型（PlayerState/GameState 方法）
- 引入 service 层分离 handler 业务逻辑
- 依赖倒置（Repository 接口）
- 引入 Value Object（RoomCode、Nickname）
- 引入 Aggregate 边界（Room 为聚合根）
- 引入 Domain Event（PlayerJoined、PlayerLeft、GameEnded）
- CQRS 读写分离
- 业务逻辑从 handler 内移到领域层

### 阶段 P4：混沌工程与高级 SRE（探索性）

- Chaos Mesh 故障注入实验（PG 宕机、Redis 不可达、网络延迟）
- 持续 profiling（Pyroscope/Parca）
- cleanupOnce 锁粒度优化
- CheckRateLimit 改用 Lua 脚本保证原子性
- 广播消息丢弃监控 + 慢客户端检测
- Saga 补偿模式（游戏重启、跨实体操作）

## Impact

### Affected specs
- 无现有 spec 受影响（本规范为新建）

### Affected code（关键文件清单）

**后端核心**：
- `backend/cmd/server/main.go`（main 拆分、pprof、优雅关闭）
- `backend/internal/handler/admin.go`（密码修改、Cookie Secure、JWT 过期）
- `backend/internal/handler/admin_password.go`（移除明文回退）
- `backend/internal/handler/auth.go`（DeleteUserData、Cookie Secure、错误处理）
- `backend/internal/handler/lobby.go`（WebSocket Origin、span 采样）
- `backend/internal/game/room.go`（HandleJoin 拆分、handleTap 拆分、广播监控）
- `backend/internal/game/hub.go`（cleanupOnce 优化、CloseAllRooms）
- `backend/internal/game/physics.go`（randFloat64 优化）
- `backend/game/restart.go`（重启补偿）
- `backend/internal/auth/magiclink.go`（邮件异步化）
- `backend/internal/auth/jwt.go`（密钥长度校验、密钥轮换）
- `backend/internal/auth/refresh.go`（RevokeAllForUser 反向索引）
- `backend/internal/audit/audit.go`（防篡改持久化）
- `backend/internal/middleware/security.go`（HSTS HTTPS 检测）
- `backend/internal/middleware/ratelimit.go`（失败锁定）
- `backend/internal/protocol/encode.go`（手写二进制编码）
- `backend/internal/store/postgres.go`（批量 INSERT、retry 高阶函数）
- `backend/internal/store/redis.go`（Lua 脚本、反向索引）
- `backend/internal/health/health.go`（熔断器状态）
- `backend/internal/metrics/metrics.go`（SLO 指标）
- `backend/internal/model/model.go`（充血模型）
- `backend/.golangci.yml`（启用更多 linter）

**配置与基础设施**：
- `docker-compose.yml`（移除默认密钥、digest pinning）
- `Dockerfile`（digest pinning）
- `.gitignore`（增加 .env）
- `.pre-commit-config.yaml`（detect-secrets/gitleaks）
- `.github/workflows/go-ci.yml`（gitleaks、cosign verify、license 检查）
- `.github/dependabot.yml`（新建）

**文档**：
- `LICENSE`（新建，MIT 全文）
- `.env.example`（新建）
- `docs/runbook.md`（新建）
- `docs/slo.md`（新建）
- `docs/environments.md`（新建）
- `docs/templates/postmortem.md`（新建）
- `docs/adr/008-audit-log-tamper-proof.md`（新建）
- `docs/adr/009-transactional-outbox.md`（新建）
- `docs/adr/010-async-email.md`（新建）

**数据库迁移**：
- `backend/migrations/000005_add_soft_delete.up.sql`（users 表 deleted_at）
- `backend/migrations/000006_create_audit_logs.up.sql`（audit_logs 表）
- `backend/migrations/000007_create_outbox_events.up.sql`（outbox_events 表）
- `backend/migrations/000008_create_db_roles.up.sql`（app_user + migrator 角色）

## ADDED Requirements

### Requirement: 安全凭据管理

系统 SHALL 在启动时强制要求所有密钥（JWT_SECRET、ENCRYPTION_KEY、ADMIN_PASSWORD）通过环境变量提供，不得有任何默认值。`ENCRYPTION_KEY` SHALL 至少 32 字节，`JWT_SECRET` SHALL 至少 32 字节。开发环境密钥 SHALL 使用明显无效值（如 `DEV_ONLY_DO_NOT_USE_IN_PROD`），生产环境启动时 SHALL 检测并拒绝弱密钥。

#### Scenario: 生产环境缺失密钥
- **WHEN** 生产环境（`ENABLE_HSTS != false` 或 `PORT=443`）启动时 `JWT_SECRET` 包含 `DEV_ONLY` 或 `change-in-production`
- **THEN** 系统 SHALL panic 并拒绝启动，日志输出"weak secret detected in production"

#### Scenario: docker-compose 启动
- **WHEN** 执行 `docker-compose up` 而未设置 `JWT_SECRET` 环境变量
- **THEN** docker-compose SHALL 拒绝启动并输出错误"JWT_SECRET required"

### Requirement: 密码安全

系统 SHALL 使用 bcrypt 哈希存储所有密码， SHALL NOT 支持明文密码回退。密码修改操作 SHALL 验证旧密码。失败登录 SHALL 触发锁定机制（5 次失败后锁定 IP 15 分钟）。

#### Scenario: 明文密码回退移除
- **WHEN** 数据库中存储的密码不是 bcrypt 哈希格式
- **THEN** 系统 SHALL 拒绝登录并记录告警，要求管理员重置密码

#### Scenario: 密码修改验证旧密码
- **WHEN** 管理员请求修改密码但未提供正确的 `oldPassword`
- **THEN** 系统 SHALL 返回 401 Unauthorized

#### Scenario: 失败登录锁定
- **WHEN** 同一 IP 在 5 分钟内连续 5 次登录失败
- **THEN** 系统 SHALL 锁定该 IP 15 分钟，期间所有登录请求返回 429

### Requirement: Cookie 安全

系统 SHALL 在所有 Cookie 设置中正确判断 Secure 标志，优先读取 `X-Forwarded-Proto: https` 头，其次检查 `r.TLS != nil`。

#### Scenario: 反向代理后端
- **WHEN** 请求经过 HTTPS 反向代理，`X-Forwarded-Proto: https`，`r.TLS == nil`
- **THEN** 系统 SHALL 设置 `Secure: true`

### Requirement: 数据隐私合规（GDPR Article 17）

系统 SHALL 实现"被遗忘权"：用户删除请求 SHALL 真正匿名化 PII（email、nickname），而非仅撤销 token。匿名化后 30 天 SHALL 硬删除用户行（CASCADE 清理关联数据）。

#### Scenario: 用户删除请求
- **WHEN** 用户调用 `DELETE /auth/data`
- **THEN** 系统 SHALL 将 email 更新为 `deleted_{id}@anonymized`，nickname 更新为 `Deleted User`，设置 `deleted_at` 时间戳
- **AND** SHALL 撤销所有 refresh token 与 JWT
- **AND** SHALL 返回 204 No Content

### Requirement: 审计日志防篡改

系统 SHALL 将审计日志持久化到 `audit_logs` 表（append-only，触发器禁止 UPDATE/DELETE），每条日志 SHALL 包含前一条日志的 HMAC-SHA256 哈希形成链式结构。应用数据库账户 SHALL 只有 INSERT 权限，无 UPDATE/DELETE 权限。

#### Scenario: 审计日志写入
- **WHEN** 管理员执行配置变更
- **THEN** 系统 SHALL 在 `audit_logs` 表 INSERT 一条记录，包含 action、actor_id、actor_ip、before、after、request_id、trace_id、prev_hash、this_hash
- **AND** `this_hash = HMAC-SHA256(secret, prev_hash || payload)`

#### Scenario: 审计日志篡改检测
- **WHEN** 攻击者直接修改 `audit_logs` 表中的某条记录
- **THEN** 下一条日志的 `prev_hash` 验证 SHALL 失败，触发告警

### Requirement: SRE 可靠性目标

系统 SHALL 为核心用户旅程定义 SLI/SLO：(1) 认证成功率 ≥ 99.9%，p99 ≤ 500ms；(2) 房间创建成功率 ≥ 99.5%，p99 ≤ 1s；(3) WebSocket 连接成功率 ≥ 99.0%，p99 握手 ≤ 2s。系统 SHALL 实现 Burn Rate 多窗口告警（1h×14.4 + 5m×14.4 快速燃烧，6h×6 + 30m×6 慢速燃烧）。

#### Scenario: 快速燃烧告警
- **WHEN** 5 分钟错误率 > 1.44% AND 1 小时错误率 > 1.44%
- **THEN** 系统 SHALL 触发 Page 级告警

### Requirement: 异步事件驱动架构

系统 SHALL 实施 ADR-007 Redis Stream 消息队列，将以下操作异步化：(1) 魔法链接邮件发送；(2) 游戏结果写入；(3) 审计日志持久化。系统 SHALL 实施 Transactional Outbox 模式保证"至少一次"事件发布。消费者 SHALL 对重复消息做幂等保护。

#### Scenario: 邮件发送异步化
- **WHEN** 用户请求魔法链接
- **THEN** 系统 SHALL 立即返回 202 Accepted，将邮件任务投递到 `email:queue` Redis Stream
- **AND** 独立 Worker SHALL 消费队列调用 Resend API，失败重试 5 次后进入死信队列

#### Scenario: 游戏结果异步写入
- **WHEN** 游戏结束
- **THEN** 系统 SHALL 将结果投递到 `game:results` Stream，立即返回
- **AND** Worker SHALL 批量（每 100 条或每 1s）写入 PostgreSQL

### Requirement: 性能调试基础设施

系统 SHALL 在非生产环境启用 `/debug/pprof/` 端点（CPU/内存/goroutine profile），生产环境通过 `ENABLE_PPROF=true` 环境变量控制启用并配合 Basic Auth 保护。系统 SHALL 启用 `/debug/vars`（expvar）端点暴露运行时变量。

#### Scenario: 生产环境 pprof 访问
- **WHEN** 生产环境 `ENABLE_PPROF=true` 且请求未通过 Basic Auth
- **THEN** 系统 SHALL 返回 401 Unauthorized

### Requirement: 开发者体验

系统 SHALL 提供 `make dev` 一键启动（docker compose up postgres redis + air 后端热重载 + vite 前端）。系统 SHALL 提供 `.env.example` 包含所有必填变量与明显占位符。系统 SHALL 提供 `backend/.air.toml` 热重载配置。系统 SHALL 提供 `.devcontainer/devcontainer.json`。

#### Scenario: 新开发者首次启动
- **WHEN** 新开发者克隆仓库后执行 `cp .env.example .env && make dev`
- **THEN** 系统 SHALL 在 5 分钟内启动完整开发环境（postgres + redis + 后端 + 前端）

### Requirement: 代码质量门控

系统 SHALL 在 `.golangci.yml` 启用 gocyclo（max 15）、funlen（max 50）、gocognit（max 20）、goconst、dupl linter。系统 SHALL 无死代码（删除所有未使用的导出函数）。系统 SHALL 无 deprecated 函数。系统 SHALL 使用哨兵错误替代字符串比较。

#### Scenario: CI 质量门控
- **WHEN** PR 中引入圈复杂度 > 15 的函数
- **THEN** golangci-lint SHALL 失败并指出具体位置

## MODIFIED Requirements

### Requirement: 现有熔断器与重试机制

现有 `backend/internal/resilience/circuitbreaker.go` 与 `retry.go` 实现质量优秀，保持核心逻辑不变。修改点：(1) `JitteredBackoff` 增加 `base <= 0` 防御；(2) `circuit_breaker_state` 的 `from` label 在初始化时设为 `closed`。

### Requirement: 现有限流与幂等中间件

现有 `backend/internal/middleware/ratelimit.go` 与 `idempotency.go` 实现质量优秀。修改点：(1) `CheckRateLimit` 改用 Lua 脚本保证 INCR+EXPIRE 原子性；(2) `Idempotency-Key` 限制长度 ≤ 255 字符；(3) 大响应（> 64KB）跳过幂等缓存。

## REMOVED Requirements

### Requirement: 明文密码回退兼容

**Reason**: 安全债务，违反 OWASP 密码存储规范，允许数据库存储明文密码
**Migration**: 一次性迁移脚本将所有明文密码转为 bcrypt 哈希（强制管理员重置）；移除 `admin_password.go:14-22` 的 `subtle.ConstantTimeCompare` 分支

### Requirement: deprecated EndGameSession/RecordGameResults

**Reason**: 迁移期已过，`EndGameAndRecordResults` 已上线
**Migration**: 删除 `postgres.go:269-333` 的两个 deprecated 函数及其测试

### Requirement: 自定义 minf/maxf/mini/itoa 函数

**Reason**: Go 1.26 已有内置 `min`/`max`，`strconv.Itoa` 是标准做法
**Migration**: 删除 `physics.go:32-49` 与 `names.go:169` 的自定义函数，替换为标准库

---

## 垂直审计差距矩阵（8 维度 × 4 核心审计问题）

### 维度 1：领域建模与业务逻辑质量

| 检查项 | 状态 | 优化 | 简化 | 强化 | 安全 | 证据 |
|--------|------|------|------|------|------|------|
| 子域识别与分层 | ⚠️ | MED | LOW | LOW | INFO | `internal/` 按技术关注点分层而非业务子域 |
| 充血模型 | ❌ | HIGH | MED | HIGH | LOW | `model.go:4-52` 贫血模型，规则散落 `room.go:65-180,585-642` |
| Value Object/Aggregate/Domain Event | ❌ | MED | LOW | MED | INFO | 无 VO/Aggregate/Event |
| 业务规则强制执行位置 | ❌ | HIGH | HIGH | HIGH | MED | `handler/auth.go:249-289`、`admin.go:40-108` 业务逻辑泄漏 |
| 依赖方向 | ⚠️ | HIGH | MED | HIGH | MED | `game/hub.go:13-17` 依赖 store/protocol/metrics |
| 框架污染领域层 | ⚠️ | MED | LOW | MED | INFO | `model.go` 带 json tag |
| 业务逻辑泄漏到 handler | ❌ | HIGH | HIGH | HIGH | MED | 7 处泄漏（auth/admin/lobby） |
| 命名自解释 | ⚠️ | MED | MED | MED | INFO | `Handle*`/`process`/`Info` 通用命名 |
| 上帝函数 | ❌ | HIGH | HIGH | HIGH | LOW | `main` 315行、`HandleJoin` 116行、`UpdateConfig` 98行 |
| CQRS | ❌ | MED | MED | MED | INFO | 读写路径混在同一 handler/store |

### 维度 2：性能工程

| 检查项 | 状态 | 优化 | 简化 | 强化 | 安全 | 证据 |
|--------|------|------|------|------|------|------|
| 基准测试 | ✅ | LOW | LOW | LOW | INFO | 20+ Benchmark 函数覆盖热路径 |
| pprof profiling | ❌ | HIGH | LOW | MED | MED | `main.go` 未导入 `net/http/pprof` |
| N+1 查询 | ⚠️ | MED | MED | MED | LOW | `postgres.go:317-323` 循环 INSERT，`refresh.go:75-98` SCAN 反模式 |
| 序列化热路径 | ⚠️ | HIGH | MED | LOW | LOW | `encode.go:19-70` binary.Write 反射，15Hz tick |
| sync.Pool | ❌ | HIGH | LOW | LOW | LOW | 全仓库无 sync.Pool，`physics.go:17-24` big.Int 分配 |
| 字符串操作 | ⚠️ | LOW | LOW | LOW | INFO | `room.go:695-699` generateUUID 5 次 `+` 拼接 |
| 值传递 vs 指针 | ✅ | LOW | NONE | LOW | INFO | 物理函数正确使用指针 |
| 并发优化 | ⚠️ | MED | MED | MED | LOW | `cleanupOnce` 持锁过长，3 个 metrics goroutine 可合并 |
| 预编译 | ✅ | NONE | NONE | NONE | INFO | 正则包级 MustCompile，SQL 由 pgx 缓存 |
| WebSocket 热路径 | ⚠️ | HIGH | MED | MED | LOW | `lobby.go:290-318` 每条消息创建 span，750 spans/秒/房间 |
| 数据库连接池 | ✅ | LOW | LOW | LOW | INFO | `postgres.go:52-65` 配置合理 |
| Redis 使用 | ✅ | LOW | LOW | LOW | INFO | `redis.go` Pipeline 使用合理，CheckRateLimit 非原子 |

### 维度 3：SRE 实践

| 检查项 | 状态 | 优化 | 简化 | 强化 | 安全 | 证据 |
|--------|------|------|------|------|------|------|
| SLI/SLO/SLA | ❌ | HIGH | LOW | HIGH | HIGH | 无 SLO 定义，无 error budget |
| Error Budget | ❌ | MED | LOW | MED | MED | 无概念 |
| Burn Rate Alert | ❌ | HIGH | LOW | HIGH | MED | 无 alertmanager 规则 |
| 混沌工程 | ❌ | MED | LOW | MED | MED | 无故障注入 |
| Defense-in-Depth | ⚠️ | LOW | MED | MED | MED | 输入校验缺长度限制（`room.go:649`、`lobby.go:81`） |
| runbook.md | ❌ | HIGH | LOW | HIGH | HIGH | `docs/runbook.md` 存在但需补充 5 类故障 |
| Postmortem 模板 | ❌ | MED | LOW | MED | LOW | 无模板 |
| 熔断器/重试质量 | ✅ | LOW | LOW | LOW | INFO | `circuitbreaker.go`/`retry.go` 实现优秀 |
| health.go | ⚠️ | MED | LOW | MED | MED | 未纳入熔断器状态与 Hub 负载 |
| ratelimit/idempotency | ✅ | LOW | LOW | LOW | INFO | 实现优秀，fail-open/closed 区分 |

### 维度 4：代码质量指标

| 检查项 | 状态 | 优化 | 简化 | 强化 | 安全 | 证据 |
|--------|------|------|------|------|------|------|
| golangci-lint 配置 | ⚠️ | HIGH | LOW | LOW | LOW | 未启用 gocyclo/funlen/goconst/dupl |
| 超长函数 | ❌ | HIGH | HIGH | HIGH | LOW | 12+ 个 > 50 行函数 |
| 圈复杂度 | ❌ | HIGH | HIGH | HIGH | LOW | 8+ 个函数复杂度 > 10 |
| 死代码 | ❌ | MED | HIGH | MED | LOW | 18+ 个未使用导出函数 |
| 重复代码 | ❌ | HIGH | HIGH | HIGH | LOW | retry 嵌套 15+ 处，cookie 撤销重复 |
| 魔法数字 | ❌ | MED | HIGH | MED | LOW | 30+ 处，15min TTL 重复 3 处 |
| 错误处理一致性 | ⚠️ | HIGH | HIGH | HIGH | MED | 字符串比较错误（`auth.go:73-79`） |
| 五轴评估 | ⚠️ | - | - | - | - | `room.go`/`main.go` 可维护性 🔴 |
| 技术债热点 | ❌ | HIGH | HIGH | HIGH | MED | `room.go`/`main.go`/`admin.go` 高风险 |
| 切斯特顿围栏 | ⚠️ | HIGH | MED | HIGH | INFO | 10 项简化建议已分析原因 |

### 维度 5：供应链安全

| 检查项 | 状态 | 优化 | 简化 | 强化 | 安全 | 证据 |
|--------|------|------|------|------|------|------|
| SBOM | ✅ | LOW | NONE | LOW | INFO | `go-ci.yml:162-167` CycloneDX |
| 构建签名 | ✅ | MED | NONE | MED | INFO | `go-ci.yml:158-161` cosign |
| SLSA 等级 | ⚠️ | HIGH | LOW | HIGH | MED | `Dockerfile` 未 pin digest |
| 许可证合规 | ❌ | HIGH | LOW | HIGH | HIGH | 无 LICENSE 文件，无 go-licenses |
| 数据隐私 | ⚠️ | HIGH | MED | HIGH | HIGH | DeleteUserData 空壳，无保留策略 |
| go.sum 提交 | ✅ | NONE | NONE | NONE | INFO | `backend/go.sum` 已提交 |
| Docker digest pinning | ❌ | HIGH | NONE | HIGH | HIGH | `Dockerfile:10,25,41` 使用 tag |
| Dependabot | ❌ | HIGH | LOW | HIGH | MED | 无配置 |
| 密码/令牌处理 | ⚠️ | HIGH | MED | HIGH | HIGH | 明文回退、docker-compose 默认密钥 |

### 维度 6：异步架构

| 检查项 | 状态 | 优化 | 简化 | 强化 | 安全 | 证据 |
|--------|------|------|------|------|------|------|
| 同步边界识别 | ❌ | HIGH | LOW | HIGH | HIGH | 邮件/游戏结果/审计日志同步 |
| 消息队列 | ❌ | HIGH | LOW | HIGH | MED | ADR-007 Proposed 未实施 |
| LISTEN/NOTIFY | ⚠️ | LOW | HIGH | LOW | LOW | 未使用（可接受） |
| Transactional Outbox | ❌ | HIGH | LOW | HIGH | HIGH | 无 outbox 表 |
| 审计轨迹防篡改 | ❌ | HIGH | LOW | HIGH | CRITICAL | `audit.go:18` 写 stdout，无 HMAC |
| 跨实体补偿 | ❌ | HIGH | LOW | HIGH | HIGH | `restart.go:131-148` 无补偿 |
| 消费者幂等 | ⚠️ | HIGH | MED | HIGH | MED | HTTP 幂等有，MQ 幂等待实施 |
| 重试持久化 | ❌ | HIGH | LOW | HIGH | HIGH | `retry.go` 仅内存态 |
| 重启补偿 | ❌ | HIGH | LOW | HIGH | HIGH | `restart.go` 先重置内存后持久化 |
| 邮件异步化 | ❌ | HIGH | LOW | HIGH | HIGH | `magiclink.go:147-177` 同步 |
| WebSocket 广播 | ⚠️ | HIGH | MED | HIGH | MED | `room.go:401-412` 静默丢弃 |
| 限流阻塞 | ⚠️ | LOW | LOW | LOW | LOW | 同步但 O(1)，可接受 |
| 优雅关闭 | ⚠️ | HIGH | LOW | HIGH | MED | `main.go:338-346` 未等待 tick |

### 维度 7：开发者体验

| 检查项 | 状态 | 优化 | 简化 | 强化 | 安全 | 证据 |
|--------|------|------|------|------|------|------|
| README 15 分钟启动 | ⚠️ | HIGH | HIGH | HIGH | LOW | `README.md:17-28` 步骤不全 |
| 一键启动 | ⚠️ | HIGH | HIGH | HIGH | INFO | 无 Makefile，需 3 个终端 |
| 数据库 seed | ❌ | HIGH | MED | HIGH | LOW | 无 seed 脚本 |
| .env.example | ❌ | HIGH | LOW | HIGH | MED | 仅 `.dev.vars.example`，缺变量 |
| 本地/生产差异文档 | ❌ | MED | LOW | MED | MED | 无 `environments.md` |
| 热重载 air | ❌ | HIGH | LOW | HIGH | INFO | 无 `.air.toml` |
| devcontainer | ❌ | MED | LOW | MED | INFO | 无配置 |
| .gitignore 含 .env | ❌ | HIGH | NONE | HIGH | CRITICAL | `.gitignore` 仅含 `.dev.vars` |
| pre-commit 防密钥 | ⚠️ | HIGH | LOW | HIGH | HIGH | 仅 detect-private-key，无 detect-secrets |
| 历史密钥扫描 | ❌ | HIGH | NONE | HIGH | HIGH | 无 gitleaks |
| dev/prod 凭据隔离 | ⚠️ | MED | LOW | MED | MED | docker-compose 弱密钥 |
| /debug/pprof | ❌ | HIGH | LOW | HIGH | MED | 未启用 |
| /debug/vars | ❌ | MED | NONE | MED | LOW | 未启用 |
| request_id 日志过滤 | ✅ | NONE | NONE | NONE | INFO | `logging.go`/`tracing.go` 优秀 |
| testhelpers/fixtures | ⚠️ | MED | HIGH | MED | LOW | `test_helpers_test.go` 仅 16 行 |
| gomock/mockery | ⚠️ | MED | MED | MED | LOW | 手写 fake，可暂不引入 |
| Factory 模式 | ❌ | MED | HIGH | MED | LOW | 测试数据内联构造 |
| TODO: add test | ✅ | NONE | NONE | NONE | INFO | 无此类注释 |
| Makefile/taskfile | ❌ | HIGH | HIGH | HIGH | LOW | 无 |
| tools.go | ❌ | MED | LOW | MED | LOW | 无 |
| 代码简化工具链 | ⚠️ | MED | LOW | MED | LOW | 缺 gofmt/goimports/dupl |

### 维度 8：深度安全审计

| OWASP | 检查项 | 状态 | 安全 | 证据 |
|-------|--------|------|------|------|
| A01 | 水平越权 | ✅ | LOW | `auth.go:294-298` context 注入 userId |
| A01 | 垂直越权 | ✅ | LOW | `rbac.go:58-72` 角色来自 JWT |
| A01 | 路径遍历 | ⚠️ | MED | `main.go:290` filepath.Clean 不防逃逸 |
| A01 | WS Origin | ⚠️ | MED | `lobby.go:43-46,204-223` CheckOrigin 返回 true |
| A02 | 密码哈希 | ⚠️ | HIGH | `admin_password.go:14-22` 明文回退 |
| A02 | JWT 算法 | ✅ | LOW | `jwt.go:59-61` 验证 SigningMethodHMAC |
| A02 | 字段级加密 | ✅ | LOW | `aes.go:60-79` AES-256-GCM |
| A02 | Cookie Secure | ❌ | HIGH | `admin.go:101` 等依赖 r.URL.Scheme |
| A02 | JWT 密钥长度 | ⚠️ | MED | `jwt.go:23-25` 无长度校验 |
| A03 | SQL 注入 | ✅ | LOW | 100% 参数化查询 |
| A03 | 命令注入 | ✅ | LOW | 无 exec.Command |
| A03 | XSS | ✅ | LOW | 三层防护（输入清理+textContent+CSP） |
| A04 | 限速 | ⚠️ | MED | `ratelimit.go:40-48` 仅限速无锁定 |
| A04 | 密码修改 | ❌ | HIGH | `admin.go:208-218` 不验证旧密码 |
| A04 | 业务逻辑 | ✅ | LOW | `room.go:609-614` 坐标校验 + DB CHECK |
| A05 | 默认凭据 | ❌ | CRITICAL | `docker-compose.yml:10,12,14` |
| A05 | /metrics 未认证 | ❌ | MED | `main.go:209` 无认证 |
| A05 | HSTS HTTP | ⚠️ | LOW | `security.go:44-46` 默认设置 |
| A05 | 错误泄露 | ⚠️ | MED | `auth.go:79` 返回 err.Error() |
| A06 | 依赖版本 | ✅ | LOW | `go-ci.yml:77-80` govulncheck |
| A06 | 镜像 digest | ⚠️ | MED | `Dockerfile:10,25,41` tag |
| A07 | Token 熵 | ✅ | LOW | 32 字节 crypto/rand |
| A07 | JWT 撤销 | ✅ | LOW | `redis.go:331-375` jti 黑名单 |
| A07 | 失败锁定 | ❌ | HIGH | `admin.go:72-82` 仅审计 |
| A07 | admin JWT 24h | ⚠️ | MED | `admin.go:265` 过期过长 |
| A08 | 协议解码 | ✅ | LOW | `decode.go:19-50` 双层验证 |
| A08 | 幂等 Key | ⚠️ | LOW | `idempotency.go:75-83` 无长度限制 |
| A09 | 认证审计 | ✅ | LOW | `admin.go:74-91` 完整覆盖 |
| A09 | 审计防篡改 | ❌ | MED | `audit.go:18` stdout |
| A09 | 异常检测 | ❌ | MED | 无 |
| A10 | SSRF | ✅ | LOW | `magiclink.go:149` URL 硬编码 |

---

## 学习价值标注图例

- 🎓 面试高频考点（系统设计/安全/行为面试中高频出现）
- 💼 工作中每天用到（入职前两周就会接触）
- 🔭 高级工程师技能（Senior/Staff/Architect 级别才主导的工作）
- 📋 行业规范标准（有对应的 RFC/OWASP/白皮书/法规文件）
- 🔐 安全专项技能（安全工程师或安全意识强的全栈工程师的标志）

---

## 安全审计摘要（仿 Agent Skills Security-Auditor 格式）

### 安全审计摘要
- **Critical**: 2
  - A05-1 docker-compose.yml 默认凭据（JWT_SECRET/ENCRYPTION_KEY/ADMIN_PASSWORD）
  - 6.3.1 审计日志无防篡改持久化（stdout，无 HMAC，容器重启丢失）
- **High**: 17
  - A02-1 明文密码回退（admin_password.go:14-22）
  - A02-4 Cookie Secure 标志反向代理失效（admin.go:101, quickplay.go:42,88, auth.go:232）
  - A04-2 密码修改不验证旧密码（admin.go:208-218）
  - A07-3 无失败登录锁定（admin.go:72-82）
  - 7.8.1 .gitignore 未包含 .env（.gitignore:1-9）
  - 5.5.3 DeleteUserData 空壳违反 GDPR（auth.go:326-363）
  - 5.4 无 LICENSE 文件
  - 5.7 Docker 镜像未 pin digest（Dockerfile:10,25,41）
  - 5.9 docker-compose 硬编码弱密钥
  - 6.1.1 魔法链接邮件同步发送（magiclink.go:147-177）
  - 6.1.4 审计日志同步无持久化（audit.go:35-46）
  - 6.1.5 重试机制未持久化（retry.go:17-33）
  - 6.2.3 Transactional Outbox 缺失
  - 6.3.2 跨实体操作无补偿（restart.go:131-148）
  - 6.5.1 重启逻辑无补偿（restart.go:131-148）
  - 3.1 SLI/SLO/SLA 未定义
  - 3.6 docs/runbook.md 需补充 5 类故障
- **Medium**: 30+（详见差距矩阵）
- **Low**: 多项（已达企业标准的项）

### 前 3 项最紧迫修复项（按利用难度排序）

1. **A05-1 docker-compose.yml 默认凭据** — 利用难度：低（直接复制 docker-compose.yml 到生产即可利用）；影响：完全无加密无认证
2. **7.8.1 .gitignore 未包含 .env** — 利用难度：低（开发者 `git add .` 误提交）；影响：密钥进入 git 历史，需 filter-repo 清除
3. **A02-4 Cookie Secure 标志失效** — 利用难度：中（需中间人位置）；影响：会话 cookie 可通过 HTTP 泄露

---

## 简化机会摘要（仿 Agent Skills /code-simplify 格式）

### 简化机会优先级
- **可安全简化（理解原因后确认无副作用）**: 12 项
  - 删除明文密码回退分支
  - 删除 deprecated 函数（EndGameSession/RecordGameResults）
  - 删除 18+ 个未使用导出函数
  - 删除 `room.go:131` 的 `_ = cooldown` 死代码
  - 删除自定义 minf/maxf/mini/itoa（Go 1.26 内置）
  - 提取魔法数字为常量
  - 提取 retry 高阶函数消除 15+ 处重复
  - 提取 cookie 撤销函数消除重复
  - 统一 generateUUID/sanitizePlayerName 重复实现
  - 提取 `auth.tryCookie` 消除 middleware.go 重复
  - 提取 `writeDegradedLobbyList` 消除重复
  - 合并 3 个 metrics goroutine 为 1 个
- **需确认后简化（原因可疑，先确认业务意图）**: 5 项
  - `main.go` main 函数 315 行（编排代码，应拆分但保留 main 编排职责）
  - `HandleJoin` 116 行（锁原子性，应提取方法但同锁内调用）
  - retry+cb 嵌套（差异存在，应提取 3 个高阶函数保留差异）
  - `magiclink.go:139-145` 每次创建 Transport（应改共享 Client）
  - `auth.go:73-79` 字符串比较错误（应定义哨兵错误）
- **保留并文档化（复杂性有合理存在原因）**: 3 项
  - 熔断器三态配置差异化（Postgres/Redis/Resend 不同参数）
  - 限流 fail-open/fail-closed 区分（安全 vs 可用性权衡）
  - 幂等中间件仅缓存 2xx（5xx 重试需重新执行）

---

## 工具选型建议

### Go 生态开源方案

| 改造项 | 推荐方案 | 集成方式 |
|--------|----------|----------|
| 输入验证 | `go-playground/validator/v10` | struct tag 声明约束 |
| 错误处理 | 哨兵错误 + `errors.Is/As` | 定义 `var ErrXxx = errors.New(...)` |
| 性能 profiling | `net/http/pprof` + `runtime/pprof` | `import _ "net/http/pprof"` |
| 持续 profiling | `pyroscope-io/pyroscope` 或 `parca-dev/parca` | HTTP 端点上报 |
| JSON 序列化 | `bytedance/sonic` 或 `mailru/easyjson` | 代码生成 |
| UUID 生成 | `google/uuid` | 直接调用 `uuid.NewString()` |
| Mock 生成 | `go.uber.org/mock`（gomock） | `mockgen` 命令 |
| 热重载 | `air-verse/air` | `.air.toml` 配置 |
| 数据库迁移 | `golang-migrate/migrate` | CLI + embed |
| 密钥扫描 | `zricethezav/gitleaks` | pre-commit + CI |
| SBOM | `anchore/sbom-action` | GitHub Actions |
| 镜像签名 | `sigstore/cosign` | CI sign + verify |
| 依赖更新 | `dependabot`（GitHub 内置） | `.github/dependabot.yml` |
| 混沌工程 | `chaos-mesh/chaos-mesh` | K8s CRD |
| 消息队列 | Redis Stream（已有 Redis） | `XADD`/`XREADGROUP` |
| 分布式追踪 | OpenTelemetry（已集成） | 保持现状 |

### 企业常用 SaaS/商业方案

- **SIEM/日志分析**: Splunk、ELK、Grafana Loki
- **错误监控**: Sentry、Rollbar
- **APM**: Datadog、New Relic、Honeycomb
- **密钥管理**: AWS Secrets Manager、GCP Secret Manager、HashiCorp Vault
- **容器扫描**: Snyk、Aqua、Trivy（开源）
- **合规扫描**: SOC2 — Vanta、Drata；ISO27001 — Vanta

### 相关标准文档

- **OWASP Top 10 (2021)**: https://owasp.org/Top10/
- **OWASP API Security Top 10 (2023)**: https://owasp.org/API-Security/
- **SLSA Framework**: https://slsa.dev/
- **CycloneDX**: https://cyclonedx.org/
- **Sigstore/cosign**: https://www.sigstore.dev/
- **Google SRE Book**: https://sre.google/sre-book/
- **GDPR**: https://gdpr.eu/
- **CCPA**: https://oag.ca.gov/privacy/ccpa
- **PIPL**: http://www.npc.gov.cn/（中国个人信息保护法）
- **RFC 7807**（Problem Details for HTTP APIs）: https://datatracker.ietf.org/doc/html/rfc7807
- **HSTS**: RFC 6797
- **CSP**: https://developer.mozilla.org/en-US/docs/Web/HTTP/CSP

---

## 与 Prompt A 的整合建议

本轮发现与 Prompt A 维度存在以下依赖关系：

1. **混沌工程实验（本报告维度 3）依赖可观测性三支柱（Prompt A 维度 B）先到位** — 无 metrics/logging/tracing 则混沌实验无法观测
2. **Burn Rate Alert（本报告维度 3）依赖 Prometheus 指标体系（Prompt A 维度 B）** — 需先定义 `http_requests_total` 等指标
3. **审计日志防篡改（本报告维度 6）依赖日志架构（Prompt A 维度 B）** — 需独立 audit logger
4. **Transactional Outbox（本报告维度 6）依赖数据库工程（Prompt A 维度 I）** — 需 outbox 表设计
5. **密钥轮换（本报告维度 8）依赖安全认证设计（Prompt A 维度 G）** — 需 JWT 多密钥支持
6. **SLI/SLO 定义（本报告维度 3）依赖 API 设计（Prompt A 维度 F）** — 需明确核心用户旅程
7. **数据隐私合规（本报告维度 5）依赖数据库工程（Prompt A 维度 I）** — 需软删除字段
8. **失败登录锁定（本报告维度 8）依赖 Redis 缓存层（Prompt A 维度 E）** — 需 Redis 计数器

### 合并实施建议执行顺序

1. **第一批（无依赖，可并行）**：
   - P0 安全紧急修复（默认凭据、明文回退、Cookie Secure、.gitignore）
   - P1 SLO 定义文档（纯文档）
   - P1 runbook.md 补充（纯文档）
   - P1 LICENSE 创建（纯文档）
   - P1 linter 启用（配置文件）

2. **第二批（依赖第一批）**：
   - P0 审计日志防篡改（依赖 audit_logs 表迁移）
   - P0 DeleteUserData 实现（依赖软删除迁移）
   - P1 pprof 集成（依赖 main.go 拆分）
   - P1 魔法链接异步化（依赖 Redis Stream）

3. **第三批（依赖第二批）**：
   - P1 Transactional Outbox（依赖 outbox 表）
   - P1 游戏结果异步写入（依赖 Redis Stream + Worker）
   - P2 health.go 增强（依赖熔断器状态接口）

4. **第四批（长期重构）**：
   - P2 main.go 拆分
   - P2 上帝函数拆分
   - P3 领域建模重构

5. **第五批（探索性）**：
   - P4 混沌工程
   - P4 持续 profiling
   - P4 Saga 补偿模式

---

## 优先级矩阵

按"就业价值 × 实施复杂度"两轴排列所有修复项：

### P0：CRITICAL/HIGH 紧急修复（就业价值极高，复杂度低-中）

| 修复项 | 就业价值 | 实施复杂度 | 学习标注 |
|--------|----------|------------|----------|
| 移除 docker-compose 默认密钥 | 🔐📋 | Low | 🔐📋 |
| 移除明文密码回退 | 🔐📋 | Low | 🔐📋 |
| 修复 Cookie Secure 标志 | 🔐📋 | Low | 🔐🎓 |
| 密码修改验证旧密码 | 🔐📋 | Low | 🔐📋 |
| 失败登录锁定 | 🔐📋 | Medium | 🔐🎓 |
| .gitignore 含 .env + 历史扫描 | 🔐📋 | Low | 🔐📋 |
| DeleteUserData 真正匿名化 | 🔐📋 | Medium | 🔐📋 |
| 创建 LICENSE 文件 | 📋 | Low | 📋 |
| Docker 镜像 digest pinning | 🔐🔭 | Low | 🔐🔭 |
| 审计日志防篡改持久化 | 🔐🔭 | High | 🔐🔭📋 |

### P1：SRE 基础设施与性能（就业价值高，复杂度中-高）

| 修复项 | 就业价值 | 实施复杂度 | 学习标注 |
|--------|----------|------------|----------|
| docs/runbook.md 5 类故障 | 📋🔭 | Medium | 📋🔭 |
| SLI/SLO/SLA + Error Budget | 🔭📋 | Medium | 🔭📋 |
| pprof + expvar 集成 | 🔭 | Medium | 🔭 |
| Burn Rate 告警规则 | 🔭📋 | Medium | 🔭📋 |
| WebSocket 热路径优化 | 🔭 | Medium | 🔭 |
| sync.Pool + randFloat64 优化 | 🔭 | Medium | 🔭 |
| EncodeSnapshot 手写编码 | 🔭 | Medium | 🔭 |
| 魔法链接邮件异步化 | 🔭📋 | High | 🔭📋 |
| 游戏结果异步写入 | 🔭📋 | High | 🔭📋 |
| Transactional Outbox | 🔭📋 | High | 🔭📋 |
| 启用 gocyclo/funlen linter | 📋 | Low | 📋 |
| 提取魔法数字为常量 | 💼 | Low | 💼 |
| 哨兵错误替代字符串比较 | 🎓 | Medium | 🎓 |
| 消除重复代码 | 🎓 | Medium | 🎓 |
| 删除死代码 | 💼 | Low | 💼 |
| 删除 deprecated 函数 | 💼 | Low | 💼 |

### P2：开发者体验与代码质量（就业价值中，复杂度低-中）

| 修复项 | 就业价值 | 实施复杂度 | 学习标注 |
|--------|----------|------------|----------|
| Makefile 统一命令 | 💼 | Low | 💼 |
| .env.example | 🔐💼 | Low | 🔐💼 |
| .air.toml 热重载 | 💼 | Low | 💼 |
| devcontainer.json | 🔭 | Medium | 🔭 |
| tools.go 工具版本锁定 | 🔭 | Low | 🔭 |
| 数据库 seed 脚本 | 💼 | Medium | 💼 |
| docs/environments.md | 📋 | Low | 📋 |
| main.go 拆分 | 🎓 | Medium | 🎓 |
| 上帝函数拆分 | 🎓 | High | 🎓 |
| health.go 增强 | 💼 | Medium | 💼 |
| Defense-in-Depth 输入校验 | 🔐 | Medium | 🔐 |
| N+1 修复 | 🎓 | Medium | 🎓 |
| Postmortem 模板 | 📋 | Low | 📋 |
| /metrics 端点认证 | 🔭 | Low | 🔭 |
| admin JWT 缩短至 30 分钟 | 🔐 | Low | 🔐 |
| JWT 密钥长度校验 | 🔐 | Low | 🔐 |
| 路径遍历增强 | 🔐 | Low | 🔐 |
| WebSocket Origin 修复 | 🔐 | Medium | 🔐 |
| 密钥轮换机制 | 🔭 | High | 🔭 |
| 异常行为检测 | 🔭 | Medium | 🔭 |
| 数据库用户最小权限 | 🔭 | Medium | 🔭 |
| Dependabot 配置 | 💼 | Low | 💼 |
| pre-commit detect-secrets | 🔐 | Low | 🔐 |
| 优雅关闭等待异步任务 | 📋 | Medium | 📋 |

### P3：领域建模重构（就业价值高，复杂度高）

| 修复项 | 就业价值 | 实施复杂度 | 学习标注 |
|--------|----------|------------|----------|
| 充血模型 | 🔭 | High | 🔭 |
| service 层分离 | 🔭 | High | 🔭 |
| 依赖倒置 Repository 接口 | 🔭 | High | 🔭 |
| Value Object | 🔭 | High | 🔭 |
| Aggregate 边界 | 🔭 | High | 🔭 |
| Domain Event | 🔭 | High | 🔭 |
| CQRS 读写分离 | 🔭 | High | 🔭 |

### P4：混沌工程与高级 SRE（就业价值高，复杂度高，探索性）

| 修复项 | 就业价值 | 实施复杂度 | 学习标注 |
|--------|----------|------------|----------|
| Chaos Mesh 故障注入 | 🔭 | High | 🔭 |
| 持续 profiling | 🔭 | Medium | 🔭 |
| cleanupOnce 锁粒度优化 | 🔭 | Medium | 🔭 |
| CheckRateLimit Lua 脚本 | 🔭 | Medium | 🔭 |
| 广播丢弃监控 + 慢客户端 | 🔭 | Medium | 🔭 |
| Saga 补偿模式 | 🔭 | High | 🔭 |
