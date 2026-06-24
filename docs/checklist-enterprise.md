# 企业级审计改造验证清单（checklist-enterprise.md）

> **来源**：`docs/tasks-enterprise.md`（46 项改造任务）
> **用途**：每项改造的验证检查点，确保"证明它有效"而非"声称它有效"
> **原则**：Prove-It 模式——每个修复必须有对应回归测试证明它被修复

## 阶段一：立即修复（T1-T21）

### T1 [C-2] 重试机制装配
- [ ] `resilience/retry.go` 含 `isRetryable(err error) bool` 函数
- [ ] `isRetryable` 匹配 `pgx.ErrConnBusy`、`pgx.ErrTxCommitRollback`、`net.Error.Timeout()`、`syscall.ECONNRESET`、`io.EOF`
- [ ] `withRetryRead`/`withRetryWrite` 回调中调用 `retry.RetryableError(err)` 包装
- [ ] 所有 8+ 处内联 `retry.Do` 回调均使用 RetryableError
- [ ] 集成测试：mock 一次失败+一次成功，断言 `attempts == 2`
- [ ] 集成测试：mock 不可重试错误，断言 `attempts == 1`
- [ ] `go test -race ./internal/resilience/... ./internal/store/...` 通过

### T2 [C-3] EmailWorker HTTP 超时
- [ ] `EmailWorker` 结构体含 `httpClient *http.Client` 字段
- [ ] 构造函数注入 `timeouts config.TimeoutConfig`
- [ ] `httpClient.Timeout == timeouts.HTTPRequestTimeout`
- [ ] `sendEmail` 使用 `w.httpClient.Do(req)` 而非 `http.DefaultClient.Do(req)`
- [ ] 可选：Transport 含 `Dialer{Timeout: timeouts.HTTPConnectTimeout}`
- [ ] 单元测试验证 `httpClient.Timeout` 非零
- [ ] `main.go:358-364` HTTP 服务端超时改用 `timeouts.HTTPRequestTimeout`

### T3 [G-3] EndpointRateLimit 启用
- [ ] `main.go` 中所有 `appMiddleware.RateLimit(redis, ...)` 已替换为 `EndpointRateLimit`
- [ ] `/api/v1/auth/quickplay` 挂载 `EndpointRateLimit(redis, "auth:quickplay", jwtMgr)`
- [ ] `/api/v1/auth/verify` 挂载 `EndpointRateLimit(redis, "auth:verify", jwtMgr)`
- [ ] `/api/v1/registry/create` 挂载 `EndpointRateLimit(redis, "registry:create", jwtMgr)`
- [ ] `/api/v1/admin/login` 挂载 `EndpointRateLimit(redis, "admin:login", jwtMgr)`
- [ ] e2e 测试验证 FailClosed 行为（Redis 宕机时返回 503 而非放行）
- [ ] e2e 测试验证同一用户多端点限流独立计数

### T4 [G-4] Admin 锁定 IP 修正
- [ ] `middleware.ExtractClientIP(r *http.Request) string` 已导出
- [ ] `admin.go:61` `r.RemoteAddr` 替换为 `middleware.ExtractClientIP(r)`
- [ ] `admin.go:66,101,105` 锁定 key 全部使用 `ExtractClientIP`
- [ ] 单元测试：X-Forwarded-For 场景下锁定按真实客户端 IP 隔离
- [ ] 单元测试：无 X-Forwarded-For 时回退到 RemoteAddr

### T5 [D-3] .secrets.baseline 生成
- [ ] `.secrets.baseline` 文件存在于项目根目录
- [ ] `pre-commit run detect-secrets --all-files` 通过
- [ ] 文件含 `generated_at`、`plugins`、`results` 字段
- [ ] CI 增加 `pre-commit run detect-secrets --all-files` 校验步骤

### T6 [F-2] CORS 补 PATCH
- [ ] `cors.go:22` 含 `PATCH` 方法
- [ ] `cors.go:23` 含 `Idempotency-Key` 头
- [ ] 单元测试 `TestCORS_PATCH_Preflight` 验证 PATCH 预检返回 200
- [ ] 单元测试验证 `Idempotency-Key` 在 Allow-Headers 中

### T7 [B-1] 审计日志上下文
- [ ] `Hub.CreateRoom(ctx context.Context, ...)` 签名含 ctx
- [ ] `Hub.RemoveRoom(ctx context.Context, ...)` 签名含 ctx
- [ ] `handler/lobby.go` 调用时传入 `r.Context()`
- [ ] `audit.Log(ctx, ...)` 通过 `middleware.GetRequestID(ctx)` 填充 request_id
- [ ] `audit.Log(ctx, ...)` 通过 `trace.SpanFromContext(ctx).SpanContext().TraceID()` 填充 trace_id
- [ ] 单元测试验证审计日志含 request_id 与 trace_id

### T8 [B-2] /metrics 鉴权
- [ ] `/metrics` 端点增加基本认证或独立端口
- [ ] `METRICS_USER`/`METRICS_PASS` 环境变量读取
- [ ] curl 无认证返回 401
- [ ] curl 带认证返回 200
- [ ] 文档更新说明 metrics 访问方式

### T9 [B-5] 采样器配置
- [ ] `telemetry.go` 含 `sdktrace.WithSampler`
- [ ] 采样器类型为 `ParentBased(TraceIDRatioBased(ratio))`
- [ ] `OTEL_SAMPLE_RATIO` 环境变量可配置（默认 0.1）
- [ ] 单元测试验证采样器类型
- [ ] 集成测试验证高 QPS 下 trace 数量受控

### T10 [B-4] DBPoolAcquireDuration 采集
- [ ] `postgres.go` 连接池配置含 `BeforeAcquire`/`AfterAcquire` 回调
- [ ] 或周期性 `pool.Stat().AcquireDuration()` Observe
- [ ] `/metrics` 含 `db_pool_acquire_duration_bucket` 指标
- [ ] 集成测试验证指标值非零

### T11 [B-6] LOG_FORMAT 实现
- [ ] `main.go` 含 `os.Getenv("LOG_FORMAT")` 分支
- [ ] `LOG_FORMAT=text` 输出 text 格式
- [ ] `LOG_FORMAT=json` 或未设置输出 JSON 格式
- [ ] 默认值为 `json`（生产友好）
- [ ] 手动验证：`LOG_FORMAT=text ./server` 输出 text 格式

### T12 [H-1] Docker digest pin
- [ ] `Dockerfile:12` 含 `@sha256:<digest>`
- [ ] `Dockerfile:29` 含 `@sha256:<digest>`
- [ ] `Dockerfile:47` 含 `@sha256:<digest>`
- [ ] `docker build` 成功
- [ ] CI `docker-pin-check` 强制 `@sha256:` 校验通过

### T13 [I-1] 删除冗余索引
- [ ] `backend/migrations/000008_drop_redundant_indexes.up.sql` 存在
- [ ] 含 `DROP INDEX IF EXISTS idx_users_email, idx_sessions_lobby, idx_results_session, idx_lobby_states_updated_at`
- [ ] `000008_drop_redundant_indexes.down.sql` 重建被删索引
- [ ] 迁移 up/down 测试通过
- [ ] 查询计划未退化（EXPLAIN 验证）
- [ ] `docs/db-query-analysis.md` 删除"单列索引"行

### T14 [A-1] ADR 索引重写
- [ ] `docs/adr/README.md` 表格列出 001-010 全部 ADR 实际标题
- [ ] ADR-006 标题与文件一致（Redis 读缓存层）
- [ ] ADR-007 标题与文件一致（Redis Stream 消息队列）
- [ ] CI 校验脚本：README 表格行数 = `docs/adr/` 下文件数
- [ ] 校验脚本通过

### T15 [A-4] 文档与实现同步
- [ ] `architecture.md:71` 修正为 15Hz
- [ ] `architecture.md:93` 删除"无消息队列"改为"已引入 Redis Stream（ADR-007）"
- [ ] ADR-005 状态改为"已接受（部分实施）"
- [ ] CI 文档一致性校验脚本通过

### T16 [J-1] CHANGELOG 同步
- [ ] `CHANGELOG.md` 含 `## [1.0.0] - 2026-06-24` 版本块
- [ ] 含 Added/Changed/Deprecated/Removed/Fixed/Security 六类
- [ ] `kacl-cli lint CHANGELOG.md` 通过
- [ ] 与 `release-please-manifest.json` 版本一致

### T17 [J-2] PR 模板
- [ ] `.github/PULL_REQUEST_TEMPLATE.md` 文件存在
- [ ] 含 Summary / Motivation / Changes / Test Plan / Checklist 字段
- [ ] Checklist 含"已跑 go test -race"/"已跑 golangci-lint"/"已更新 CHANGELOG"/"已更新文档"
- [ ] `CONTRIBUTING.md:100` 含指向模板的链接

### T18 [G-1] RBAC 端点覆盖
- [ ] `/api/v1/user/*` 路由挂载 RBAC 中间件
- [ ] `/api/v1/registry/*` 路由挂载 RBAC 中间件
- [ ] `/api/v1/lobby/*` 路由挂载 RBAC 中间件
- [ ] `policy.csv` 补充 `user, user_data, read/delete` 等策略
- [ ] e2e 测试验证未授权角色返回 403

### T19 [G-2] Admin Token 撤销
- [ ] `signAdminToken` 增加 jti claim
- [ ] `VerifyAdminToken` 调用 `redis.IsJWTRevoked`
- [ ] `POST /api/v1/admin/logout` 端点存在
- [ ] logout 端点调用 `redis.RevokeJWT`
- [ ] 修改 admin 密码时撤销所有 admin token
- [ ] 集成测试验证 logout 后 token 失效

### T20 [D-2] release-please workflow
- [ ] `.github/workflows/release-please.yml` 文件存在
- [ ] 监听 main 分支 push
- [ ] 使用 `googleapis/release-please-action@v4`
- [ ] release-type 配置正确

### T21 [D-6] deploy environment 保护
- [ ] `ci-cd.yml` deploy job 含 `environment: production`
- [ ] `.github/settings.yml` 声明 branch protection
- [ ] 含 require PR、require status checks、require CODEOWNERS review、disallow force push
- [ ] deploy 步骤后含 `$GITHUB_STEP_SUMMARY` 输出部署摘要

---

## 阶段二：规划执行（T22-T29）

### T22 [C-1] Resend 熔断器装配
- [ ] `EmailWorker` 结构体含 `cb *gobreaker.CircuitBreaker` 字段
- [ ] `MagicLinkService.cb` 字段已移除或标记 deprecated
- [ ] `sendEmail` 中包裹 `w.cb.Execute(func() (any, error) { ... })`
- [ ] 用 `resilience.ExternalAPIRetry` 包裹 HTTP 调用
- [ ] 集成测试：mock Resend 连续失败 3 次验证熔断器开路
- [ ] 集成测试：熔断器开路后请求快速失败

### T23 [C-5] withRetry 收敛
- [ ] `postgres.go` 所有内联 `retry.Do + cb.Execute` 替换为 `withRetryRead`/`withRetryWrite`
- [ ] 无 8-12 行样板代码重复
- [ ] 辅助函数中集中使用 RetryableError（T1 集成）
- [ ] 全量测试通过

### T24 [A-3] 分层架构重构
- [ ] `game.Hub.ListLobbies(ctx, limit, cursor)` 方法存在
- [ ] `handler/lobby.go` 调用 `h.hub.ListLobbies(...)` 而非 `h.hub.DB()`
- [ ] `Hub.DB()` 标记 deprecated
- [ ] 新增 `service` 层封装用户查询
- [ ] handler 不再直接依赖 store
- [ ] 全量测试通过

### T25 [D-4] 前端 CI shift-left
- [ ] `frontend/package.json` 含 `"lint": "eslint ."` 脚本
- [ ] `frontend/package.json` 含 `"audit": "npm audit --audit-level=high"` 脚本
- [ ] `ci-cd.yml` 含 lint 步骤
- [ ] `ci-cd.yml` 含 npm audit 步骤
- [ ] `.github/workflows/codeql.yml` 文件存在
- [ ] CodeQL 对 TS/Go 启用
- [ ] `dependency-review-action` 已配置

### T26 [E-1] 异步三件套测试
- [ ] `backend/internal/outbox/publisher_test.go` 存在
- [ ] `backend/internal/worker/email_worker_test.go` 存在
- [ ] `backend/internal/worker/game_result_worker_test.go` 存在
- [ ] `backend/internal/audit/audit_test.go` 存在
- [ ] outbox 测试：testcontainers 集成测试，至少一次投递验证
- [ ] outbox 测试：processed_at 更新验证
- [ ] email_worker 测试：miniredis + httptest.NewServer 模拟 Resend
- [ ] audit 测试：HMAC 链式哈希验证 `this_hash = HMAC(secret, prev_hash || payload)`
- [ ] audit 测试：篡改中间记录验证后续 hash 校验失败
- [ ] audit 测试：`-race` 验证 `lastHash` 并发安全
- [ ] `go test -race ./internal/worker/... ./internal/outbox/... ./internal/audit/...` 通过

### T27 [F-1] OpenAPI 与路由同步
- [ ] `/lobby/{code}/ws` 修正为 `/api/v1/lobby/{code}/ws`
- [ ] `/api/v1/user/data` GET/DELETE 端点文档补充
- [ ] `/api/v1/registry/match` 标记为"未实现"或移除
- [ ] Magic Link 响应码 200 → 202
- [ ] `AdminConfigUpdate` schema 补充 `oldPassword`
- [ ] CI 加入 `redocly lint docs/openapi.yaml`
- [ ] `redocly lint` 通过

### T28 [I-2] EXPLAIN ANALYZE 实测
- [ ] staging 灌入生产量级数据（≥10 万行 game_results）
- [ ] 5 个核心查询执行 `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`
- [ ] `docs/db-query-analysis.md` 含"实测计划"小节
- [ ] 含 actual rows、buffers、execution time
- [ ] CI 加 pg_plan_checker 校验关键查询未走 Seq Scan

### T29 [I-3] 连接池压测 + env 化
- [ ] `BenchmarkPostgresStore_ConcurrentLoad(b *testing.B)` 存在
- [ ] N goroutine 并发 SaveLobbyState，断言 `pool.Stat().MaxConns()` 未触顶
- [ ] vegeta/k6 压测 1000 RPS，记录 P99 延迟、pool_in_use、pool_idle
- [ ] `MaxConns/MinConns/MaxConnLifetime` 改读 env
- [ ] `PG_POOL_MAX_CONNS` 等环境变量可覆盖默认值
- [ ] `docs/benchmarks-v2.md` 压测报告存在

---

## 阶段三：长期规划（T30-T32）

### T30 [E-2] WebSocket 测试覆盖
- [ ] `handler/lobby_test.go` 含 WebSocket 集成测试
- [ ] 用 `gorilla/websocket.Dial` + `httptest.NewServer`
- [ ] 多 goroutine 并发连接测试，验证 WSConnCount 准确性（-race）
- [ ] `RestoreRooms` testcontainers PG 测试
- [ ] `writePump` Send channel 满时背压测试
- [ ] `go test -race ./internal/handler/... ./internal/game/...` 通过

### T31 [H-2] WS 水平扩展
- [ ] `infra/service.yaml` 含 `autoscaling.knative.dev/minScale: "1"` 和 `maxScale: "10"`
- [ ] `hub.go` Room 引入 Redis Pub/Sub 广播层
- [ ] WebSocket 前置 LB 启用 sticky session
- [ ] 多实例广播测试通过

### T32 [C-4] 请求类型舱壁隔离
- [ ] 引入 `golang.org/x/sync/semaphore`
- [ ] 认证（10）、大厅（10）、Admin（3）、WebSocket saveState（2）独立配额
- [ ] chi 中间件层 `semaphore.Acquire(ctx)` / `defer Release()`
- [ ] 压测验证一类请求耗尽配额不影响其他类

---

## 阶段四：择机实施（T33-T46）

### T33 [B-3] WS Span 父子关联
- [ ] WS handler 中将 `r.Context()` 保存到 room/player 结构体
- [ ] `readPump`/`writePump` 从保存的 context 派生 Span
- [ ] WS 长连接 context 在连接关闭时 cancel
- [ ] Jaeger 中 WS 消息 span 关联到 HTTP 升级 span

### T34 [D-5] SLSA provenance
- [ ] 引入 `slsa-framework/slsa-github-generator` 或 `cosign attest`
- [ ] `cosign attest --predicate <slsa-provenance.json> --type slsaprovenance`
- [ ] 部署侧 `cosign verify-attestation --type slsaprovenance` 校验
- [ ] `cosign verify-attestation` 通过

### T35 [E-3] 基准测试覆盖
- [ ] `protocol/encode_decode_test.go` 含 `BenchmarkEncodeSnapshot`
- [ ] `game/hub_test.go` 含 `BenchmarkCreateRoom`、`BenchmarkHandleDisconnect`
- [ ] `store/postgres_test.go` 含 `BenchmarkSaveLobbyState`
- [ ] `go-ci.yml` 含 bench job，用 benchstat 对比 main 分支基线

### T36 [E-4] 覆盖率门禁
- [ ] `go-ci.yml` test job 末尾含阈值检查（起步 50%）
- [ ] Codecov/Coveralls 趋势可视化
- [ ] diff 覆盖率评论
- [ ] CI 覆盖率低于阈值时失败

### T37 [E-5] 前端单元测试
- [ ] `frontend/src/**/*.test.{ts,tsx}` 文件存在
- [ ] 协议解析模块测试覆盖
- [ ] 物理预测插值函数边界用例
- [ ] `package.json` 含 `"test:frontend": "cd frontend && vitest run --coverage"`
- [ ] `frontend-ci.yml` 或 ci-cd.yml 含前端测试步骤

### T38 [F-3] 429 Retry-After
- [ ] `apierror.TooManyRequests` 支持可选 retryAfter 参数
- [ ] 或中间件中 `w.Header().Set("Retry-After", ...)`
- [ ] 可选：X-RateLimit-Limit、X-RateLimit-Remaining、X-RateLimit-Reset 头
- [ ] OpenAPI 429 响应文档更新
- [ ] 429 响应含 Retry-After 头

### T39 [F-5] ETag 条件请求
- [ ] `ListLobbies` 响应体计算 SHA256 ETag
- [ ] 中间件检查 If-None-Match，匹配则返回 304
- [ ] `CheckRoom` 设置 Last-Modified 头
- [ ] OpenAPI 补充 304 响应文档
- [ ] 304 响应正确返回

### T40 [G-5] PostgreSQL email 加密
- [ ] `db.CreateUser` 调用 `crypto.Encrypt`
- [ ] `db.GetUserByEmail` 调用 `crypto.Decrypt`
- [ ] 或使用 pgcrypto 扩展列级加密
- [ ] `threat-model.md` PII 表标注"PostgreSQL（AES-256-GCM 应用层加密）"
- [ ] 迁移脚本回填加密现有数据
- [ ] DB dump 中 email 字段为密文

### T41 [G-6] 威胁建模同步
- [ ] `threat-model.md:46` 限速数值与 `ratelimit.go:41-48` 一致
- [ ] R（Repudiation）章节标注 audit log 已实现
- [ ] GDPR 章节标注导出/删除已实现
- [ ] 可选：CI 同步检查脚本

### T42 [H-3] 部署目标统一
- [ ] 明确单一部署目标（Cloud Run）
- [ ] `ci-cd.yml:88` 的 `wrangler deploy` 已删除
- [ ] 服务名统一为 `balloon-game`
- [ ] root `service.yaml` 重命名为 `service.yaml.legacy`
- [ ] ADR 固化部署架构决策

### T43 [H-4] distroless 镜像
- [ ] `Dockerfile:47` 改为 `FROM gcr.io/distroless/static-debian12:nonroot`
- [ ] `Dockerfile:48` 的 `apk add ca-certificates` 已删除
- [ ] `Dockerfile:49` 的 `adduser` 已删除
- [ ] `USER appuser` 改为 `USER nonroot:nonroot`
- [ ] `docker build` 成功
- [ ] 容器启动正常

### T44 [I-4] 审计日志不丢 + Outbox 批量
- [ ] `audit/audit.go` channel 满时阻塞写入（带 100ms 超时）或落 WAL/本地文件兜底
- [ ] `outbox/publisher.go` 改批量 `UPDATE ... WHERE id = ANY($2)`
- [ ] 关键审计（user.create/user.delete）改为同事务写入
- [ ] 压测验证审计日志无丢失
- [ ] Outbox 批量性能提升验证

### T45 [J-3] Runbook 认证章节
- [ ] `docs/runbook.md` 含"故障 6: 认证服务异常"章节
- [ ] 覆盖 refresh token 验证失败率突增
- [ ] 覆盖 Magic Link 邮件投递失败（Resend API 熔断器排查）
- [ ] 覆盖 AES 密钥轮换失误导致配置解密失败
- [ ] runbook 顶部含"熔断器全局视图"小节
- [ ] `docs/slo.md:26-35` 认证 SLO 处交叉引用 runbook 故障 6

### T46 [J-4] detect-secrets CI 校验
- [ ] `go-ci.yml` 含 `secrets-scan` 作业
- [ ] 使用 `pre-commit run detect-secrets --all-files`
- [ ] `CONTRIBUTING.md:114` 含"首次 clone 后必须执行 `pre-commit install`"强提示
- [ ] 可选：GitHub Secret Scanning + Push Protection 启用
- [ ] CI secrets-scan 作业通过

---

## 最终验收检查

### 代码质量
- [ ] `go test -race ./...` 通过
- [ ] `golangci-lint run` 通过
- [ ] `govulncheck ./...` 无高危 CVE
- [ ] `go vet ./...` 通过
- [ ] CI 全绿

### 文档一致性
- [ ] `docs/audit-enterprise.md` 中所有 ❌ 转为 ✅ 或 ⚠️（附 ADR 说明）
- [ ] `docs/architecture.md` 与代码实现一致
- [ ] `docs/openapi.yaml` 与路由一致
- [ ] `docs/threat-model.md` 与代码一致
- [ ] `docs/adr/README.md` 与 ADR 文件一致
- [ ] `CHANGELOG.md` 与 release-please 一致

### 安全验证
- [ ] `/metrics` 需认证
- [ ] 所有端点挂载 RBAC
- [ ] 限速中间件全部启用 EndpointRateLimit
- [ ] Admin Token 可撤销
- [ ] email 字段加密存储
- [ ] `.secrets.baseline` 存在且 CI 校验

### 可观测性验证
- [ ] 审计日志含 request_id 与 trace_id
- [ ] `DBPoolAcquireDuration` 指标有数据
- [ ] OpenTelemetry 采样器配置正确
- [ ] WS Span 父子关联（阶段四）
- [ ] `LOG_FORMAT` 可切换

### 弹性验证
- [ ] RetryableError 正确包装
- [ ] EmailWorker HTTP 超时装配
- [ ] Resend 熔断器装配
- [ ] 所有重试使用 withRetryRead/withRetryWrite

### 任务清单完整性
- [ ] 所有 46 项任务状态为 `[x]`
- [ ] 本清单所有检查点勾选
- [ ] `tasks-enterprise.md` 所有 `[ ]` 转为 `[x]`
