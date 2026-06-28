# STRIDE 威胁建模

> 项目: 多人网页气球飞行对战游戏
> 日期: 2026-06-24
> 方法: STRIDE (Microsoft Threat Modeling)

## S — Spoofing (欺骗)

| 威胁 | 影响 | 缓解措施 |
|------|------|---------|
| 攻击者伪造 JWT 访问他人账户 | 未授权访问 | JWT 使用 HMAC-SHA256 签名，密钥仅服务端持有 |
| 攻击者重放已过期的 Magic Link | 未授权访问 | Magic Link 一次性使用，验证后立即删除 |
| 攻击者伪造 WebSocket 连接 | 冒充玩家 | WebSocket 握手时验证 JWT cookie |
| 攻击者伪造 Origin 头进行 CSWSH 攻击 | 跨站 WebSocket 劫持 | 服务端校验 Origin 与 Host 是否匹配，不匹配则拒绝 |

## T — Tampering (篡改)

| 威胁 | 影响 | 缓解措施 |
|------|------|---------|
| 攻击者修改游戏输入帧 | 作弊 | 服务端权威物理模拟，客户端输入仅含方向指令 |
| 攻击者篡改 API 请求参数 | 数据完整性 | 服务端校验所有输入，不信任客户端 |
| 中间人篡改 WebSocket 数据 | 游戏状态破坏 | 生产环境强制 HTTPS/WSS |
| 攻击者篡改管理员配置 | 配置污染 | adminAuthMiddleware 验证 admin JWT，密码使用 bcrypt 哈希 |

## R — Repudiation (否认)

| 威胁 | 影响 | 缓解措施 |
|------|------|---------|
| 玩家否认作弊行为 | 无法追责 | 游戏结果记录到 PostgreSQL，含时间戳和玩家 ID |
| 管理员否认配置修改 | 审计缺失 | 审计日志已实现（ADR-008）：HMAC-SHA256 链式哈希防篡改，持久化到 PostgreSQL `audit_logs` 表，DB 触发器禁止 UPDATE/DELETE |

## I — Information Disclosure (信息泄露)

| 威胁 | 影响 | 缓解措施 |
|------|------|---------|
| 数据库泄露暴露用户邮箱 | PII 泄露 | 邮箱存储在 PostgreSQL，传输使用 TLS |
| JWT 密钥泄露 | 全面认证绕过 | 密钥通过环境变量注入，不提交到 VCS |
| API Key 泄露 | 第三方服务滥用 | Resend API Key 使用 AES-256-GCM 加密存储 |
| 错误消息泄露内部信息 | 攻击面扩大 | RFC 7807 错误响应不含堆栈/SQL |
| 管理员配置接口返回明文密钥 | 密钥泄露 | GetConfig 返回脱敏后的 API Key（••••••••） |

## D — Denial of Service (拒绝服务)

| 威胁 | 影响 | 缓解措施 |
|------|------|---------|
| 大量请求耗尽服务器资源 | 服务不可用 | Rate limiting（按 IP + 用户 ID + 端点维度，详见下方限流配额表） |
| 大量 WebSocket 连接 | 连接耗尽 | 限速 + 连接数上限 + 读取限制 4096 字节 |
| 慢查询拖垮数据库 | 数据库不可用 | 查询超时 + 连接池限制 + 索引 |
| Resend API 故障 | 邮件功能不可用 | 熔断器 + 降级响应 |

### 限流配额表（按端点）

> 数据来源：`backend/internal/middleware/ratelimit.go` 的 `DefaultEndpointRateLimits`。
> 限流维度为 `endpoint:user_id:ip` 复合键（认证用户）或 `endpoint:ip`（匿名用户）。
> `FailClosed=true` 表示 Redis 故障时拒绝请求（安全敏感端点），否则放行（fail-open）。

| 端点标识 | 路径 | 配额 | 窗口 | FailClosed |
|---------|------|------|------|------------|
| auth:quickplay | POST /api/v1/auth/quickplay | 10 | 1 分钟 | 是 |
| auth:request | POST /api/v1/auth/request | 5 | 1 分钟 | 否 |
| auth:verify | GET /api/v1/auth/verify | 10 | 1 分钟 | 否 |
| registry:create | POST /api/v1/registry/create | 5 | 1 分钟 | 否 |
| registry:match | （配置预留） | 10 | 1 分钟 | 否 |
| admin:login | POST /api/v1/admin/login | 5 | 1 分钟 | 是 |
| default | 其他端点 | 60 | 1 分钟 | 否 |

## E — Elevation of Privilege (权限提升)

| 威胁 | 影响 | 缓解措施 |
|------|------|---------|
| 普通用户访问管理接口 | 管理员功能滥用 | adminAuthMiddleware 验证 admin JWT（role=admin） |
| 用户越权访问他人资源 | 权限提升 | 轻量 RBAC（ADR-026）已应用于 user/lobby/registry 路由（T18）：user_data 读写、lobby 创建/加入/读取均经 `rbacEnforcer.Middleware` 校验，策略见 `backend/internal/rbac/permissions.go` |
| 容器逃逸获取 root | 主机被控制 | 容器以 appuser（非 root）运行 |
| SQL 注入获取数据 | 数据泄露 | 使用参数化查询（pgx），无字符串拼接 |

## PII 数据分类

| 字段 | 分类 | 存储保护 | 传输保护 |
|------|------|---------|---------|
| email | PII | PostgreSQL（TLS 连接） | HTTPS |
| nickname | 非敏感 | 明文 | HTTPS |
| IP 地址 | PII | Redis（TTL 自动过期） | HTTPS |
| JWT | 认证凭据 | HttpOnly cookie（access） | HTTPS + Secure flag |
| Resend API Key | 密钥 | AES-256-GCM 加密 | HTTPS |
| Admin Password | 密钥 | bcrypt 哈希 | HTTPS |
| Admin JWT | 认证凭据 | 独立 `ADMIN_JWT_SECRET` 签名 | HttpOnly `admin_token` cookie + HTTPS |
| Refresh Token | 认证凭据 | Redis（TTL 自动过期） | HttpOnly `refresh` cookie + HTTPS |

## GDPR/CCPA 合规要点

1. **数据最小化**: 仅收集必要数据（邮箱、昵称）
2. **目的限制**: 数据仅用于游戏功能，不用于营销
3. **存储限制**: IP 地址通过 Redis TTL 自动过期
4. **数据主体权利**: 已实现数据导出（`GET /api/v1/user/data`）与删除（`DELETE /api/v1/user/data`），删除流程为立即匿名化 PII + 30 天后硬删除（详见下方"数据保留策略"）

## 数据保留策略

> 本章节描述 GDPR Article 17（删除权）的数据保留与删除流程。

### 用户数据删除流程

1. **用户发起删除请求**: 用户调用 `DELETE /api/v1/user/data` 端点
2. **立即匿名化 PII**: 系统将用户邮箱替换为 `deleted_<id>@anonymized`，昵称替换为 `Deleted User`，并设置 `deleted_at` 时间戳和 `email_anonymized = true` 标志
3. **会话撤销**: 所有 refresh token 和当前 access token 的 jti 被加入 Redis 撤销列表，Cookie 被清除
4. **硬删除（延迟）**: 标记为 `deleted_at` 的用户行在 30 天保留期后由定时任务硬删除（CASCADE 自动清理关联的游戏结果数据）

### 保留期说明

- **立即删除**: 会话 token（Redis TTL 自动过期）、Cookie
- **立即匿名化**: email、nickname（PII）
- **30 天后硬删除**: 用户行及其关联数据（game_sessions、game_results）
- **保留期目的**: 保留 30 天用于反作弊审计、争议处理和数据分析；超过保留期的数据无业务价值且增加合规风险

### 数据库 Schema 支持

- `users.deleted_at BIGINT DEFAULT NULL`: 标记用户软删除时间
- `users.email_anonymized BOOLEAN DEFAULT false`: 标记邮箱是否已匿名化
- `idx_users_deleted_at`: 部分索引，加速定时任务查找待硬删除的用户

### 合规验证

- 调用 `DELETE /api/v1/user/data` 后，`GET /api/v1/user/data` 返回匿名化后的数据
- 30 天后用户行从数据库中物理删除
- 审计日志记录删除操作（不记录被删除的 PII 内容）
