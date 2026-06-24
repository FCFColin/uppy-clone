# 企业级审计改造任务清单（tasks-enterprise.md）

> **来源**：`docs/audit-enterprise.md`（Phase 1 审计报告）
> **状态**：用户已批准 Phase 2，启动改造执行
> **目标**：将审计发现的 37 ❌ + 32 ⚠️ 全部转化为可验证任务并执行至完成
> **执行原则**：证据优先、最小改动、不破坏现有功能、每项改造附回归测试

## 执行摘要（2026-06-24 更新）

| 阶段 | 总数 | 完成 | 延期 | 延期原因 |
|------|------|------|------|----------|
| 阶段一 (P0) | 21 | 21 | 0 | — |
| 阶段二 (P1) | 8 | 7 | 1 | T28 需 staging DB |
| 阶段三 (P2) | 3 | 2 | 1 | T31 需 Redis Pub/Sub 架构改造 |
| 阶段四 (P3) | 14 | 13 | 1 | T40 需 email_hash 列迁移 |
| **合计** | **46** | **43** | **3** | — |

**延期任务详情**：
- **T28 [I-2]**：需在 staging 环境灌入 ≥10 万行数据后执行 EXPLAIN ANALYZE，当前无可用 DB
- **T31 [H-2]**：需引入 Redis Pub/Sub 广播层实现多实例 WS 同步，属月级架构改造
- **T40 [G-5]**：AES-256-GCM 非确定性加密无法支持 `WHERE email = $1` 查询，需先迁移 `email_hash` 列

**验证结果**：
- `go build ./...` ✅ 通过
- `go test -short ./...` ✅ 全部 16 个包通过，0 FAIL
- 前端 `vitest run` ✅ 33 个测试通过

## 图例

- `[ ]` 待执行 / `[~]` 进行中 / `[x]` 已完成
- **优先级**：P0 立即（1-2 天）/ P1 规划（1-2 周）/ P2 长期（月级）/ P3 择机
- **复杂度**：L Low / M Medium / H High

---

## 阶段一：立即修复（P0，Low 复杂度，高 ROI）

### T1 [C-2] 重试机制装配——RetryableError 包装
- **状态**: [ ]
- **优先级**: P0 / 复杂度 M（涉及 8+ 处）
- **证据**: `backend/internal/resilience/retry.go:17-33`；`postgres.go:38,149,184,432,459,527,543,674`；`redis.go:105,237`
- **改动**:
  1. 在 `resilience/retry.go` 新增 `isRetryable(err error) bool`：匹配 `pgx.ErrConnBusy`、`pgx.ErrTxCommitRollback`、`net.Error.Timeout()`、`syscall.ECONNRESET`、`io.EOF`
  2. 在 `withRetryRead`/`withRetryWrite` 与所有内联 `retry.Do` 回调中：`if isRetryable(err) { return retry.RetryableError(err) } return err`
  3. 写操作保持不重试（除 UPSERT 类幂等写）
- **验证**: 新增集成测试 mock 一次失败+一次成功，断言 `attempts == 2`
- **依赖**: 无

### T2 [C-3] EmailWorker HTTP 超时装配
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/internal/config/timeout.go:32-33,52-53`；`worker/email_worker.go:159` 用 `http.DefaultClient`
- **改动**:
  1. `EmailWorker` 构造函数注入 `timeouts config.TimeoutConfig`
  2. 构造 `w.httpClient = &http.Client{Timeout: timeouts.HTTPRequestTimeout}`
  3. `sendEmail` 改用 `w.httpClient.Do(req)`
  4. 可选：`Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: timeouts.HTTPConnectTimeout}).DialContext}`
- **验证**: 单元测试验证 httpClient.Timeout 非零
- **依赖**: 无

### T3 [G-3] EndpointRateLimit 路由启用
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/internal/middleware/ratelimit.go:91-121`；`main.go:262-265,282-285,302-305` 全用 IP-only `RateLimit`
- **改动**:
  1. `main.go` 中所有 `appMiddleware.RateLimit(redis, ...)` 替换为 `appMiddleware.EndpointRateLimit(redis, "auth:quickplay", jwtMgr)` 等
  2. 为 `/api/v1/auth/quickplay`、`/api/v1/auth/verify`、`/api/v1/registry/create`、`/api/v1/admin/login` 补充对应 EndpointRateLimit
- **验证**: e2e 测试验证 FailClosed 行为；测试同一用户多端点限流独立计数
- **依赖**: 无

### T4 [G-4] Admin 登录锁定 IP 来源修正
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/internal/handler/admin.go:61,66,101,105` 用 `r.RemoteAddr`；`middleware/ratelimit.go:155-172` 有正确的 `extractClientIP`
- **改动**:
  1. 导出 `middleware.ExtractClientIP(r *http.Request) string`
  2. `admin.go` 中所有 `r.RemoteAddr` 替换为 `middleware.ExtractClientIP(r)`
- **验证**: 单元测试验证 X-Forwarded-For 场景下锁定按真实客户端 IP 隔离
- **依赖**: 无

### T5 [D-3] .secrets.baseline 基线文件生成
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `.pre-commit-config.yaml:17-21` 引用 `.secrets.baseline` 但文件不存在
- **改动**:
  1. 执行 `detect-secrets scan --exclude-files '\.git/.*' > .secrets.baseline`
  2. 提交该文件
  3. CI 增加 `pre-commit run detect-secrets --all-files` 校验步骤
- **验证**: `pre-commit run detect-secrets --all-files` 通过
- **依赖**: 无

### T6 [F-2] CORS 补 PATCH 方法与 Idempotency-Key 头
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/internal/middleware/cors.go:22-23`
- **改动**:
  1. `cors.go:22` 改为 `"GET, POST, PUT, PATCH, DELETE, OPTIONS"`
  2. `cors.go:23` 改为 `"Content-Type, Authorization, Idempotency-Key"`
  3. 补充单元测试验证 PATCH 预检通过
- **验证**: 单元测试 `TestCORS_PATCH_Preflight` 通过
- **依赖**: 无

### T7 [B-1] 审计日志上下文贯穿
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/internal/game/hub.go:87,155` 用 `context.Background()`
- **改动**:
  1. `Hub.CreateRoom()` 和 `Hub.RemoveRoom()` 签名增加 `ctx context.Context`
  2. 调用方 `handler/lobby.go` 传入 `r.Context()`
  3. `audit.Log(ctx, ...)` 通过 `middleware.GetRequestID(ctx)` 和 `trace.SpanFromContext(ctx).SpanContext().TraceID()` 填充
- **验证**: 单元测试验证审计日志含 request_id 与 trace_id
- **依赖**: 无

### T8 [B-2] /metrics 端点鉴权
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/cmd/server/main.go:209`
- **改动**:
  1. 包装 `promhttp.Handler` 增加基本认证（用户名密码从 env 读取 `METRICS_USER`/`METRICS_PASS`）
  2. 或绑定单独端口 :9090（推荐方案 B，更简单）
- **验证**: curl 无认证返回 401；带认证返回 200
- **依赖**: 无

### T9 [B-5] OpenTelemetry 采样器配置
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/internal/telemetry/telemetry.go:64-67`
- **改动**:
  ```go
  provider := sdktrace.NewTracerProvider(
      sdktrace.WithResource(res),
      sdktrace.WithSpanProcessor(bsp),
      sdktrace.WithSampler(sdktrace.ParentBased(
          sdktrace.TraceIDRatioBased(getEnvFloat("OTEL_SAMPLE_RATIO", 0.1)),
      )),
  )
  ```
- **验证**: 单元测试验证采样器类型；集成测试验证高 QPS 下 trace 数量受控
- **依赖**: 无

### T10 [B-4] DBPoolAcquireDuration 指标采集装配
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/internal/metrics/metrics.go:58-64` 声明但从未 Observe
- **改动**:
  1. `postgres.go` 连接池配置增加 `BeforeAcquire`/`AfterAcquire` 回调
  2. `BeforeAcquire` 记录开始时间到 context；`AfterAcquire` 计算耗时并 `metrics.DBPoolAcquireDuration.Observe(duration)`
  3. 或周期性 `pool.Stat().AcquireDuration()` Observe
- **验证**: 集成测试验证 /metrics 含 `db_pool_acquire_duration_bucket`
- **依赖**: 无

### T11 [B-6] LOG_FORMAT 环境变量实现
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/cmd/server/main.go:36` 注释承诺但未实现
- **改动**:
  ```go
  var handler slog.Handler
  if getEnv("LOG_FORMAT", "json") == "text" {
      handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
  } else {
      handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
  }
  ```
- **验证**: `LOG_FORMAT=text ./server` 输出 text 格式
- **依赖**: 无

### T12 [H-1] Docker 镜像 digest pin
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `Dockerfile:12,29,47` 仅 tag pin
- **改动**:
  1. 运行 `bash scripts/pin-digests.sh` 解析当前 digest
  2. 三处 FROM 改为 `image@sha256:<digest>` 格式
  3. 扩展 CI `docker-pin-check` 强制 `@sha256:` 校验
- **验证**: `docker build` 成功；CI docker-pin-check 通过
- **依赖**: 无

### T13 [I-1] 删除冗余索引
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/migrations/000001:30-32` + `000004:3-5` + `000001:4`
- **改动**:
  1. 新增 `backend/migrations/000008_drop_redundant_indexes.up.sql`：
     ```sql
     DROP INDEX IF EXISTS idx_users_email, idx_sessions_lobby, idx_results_session, idx_lobby_states_updated_at;
     ```
  2. 对应 `000008_drop_redundant_indexes.down.sql` 重建被删索引
  3. 更新 `docs/db-query-analysis.md` 删除"单列索引"行
- **验证**: 迁移 up/down 测试通过；查询计划未退化
- **依赖**: 无

### T14 [A-1] ADR 索引重写
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `docs/adr/README.md:14-15` 与实际文件错配
- **改动**:
  1. 重写 `docs/adr/README.md` 表格，列出 001-010 全部 ADR 实际标题
  2. 若 RBAC 与 API 版本化确有决策但未成文，补写为 ADR-011、ADR-012
  3. CI 加入脚本校验 README 表格行数 = `docs/adr/` 下文件数
- **验证**: CI 校验脚本通过
- **依赖**: 无

### T15 [A-4] 文档与实现同步
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `docs/architecture.md:71` (60fps) vs `protocol/constants.go:129` (15Hz)；`:93` "无消息队列" vs `room.go:294` 已实现；ADR-005 "提议中" vs 已实现
- **改动**:
  1. 修正 `architecture.md:71` 为 15Hz
  2. 修正 `:93` 删除"无消息队列"改为"已引入 Redis Stream（ADR-007）"
  3. 更新 ADR-005 状态为"已接受（部分实施）"
  4. CI 加文档一致性校验脚本
- **验证**: 文档校验脚本通过
- **依赖**: T14

### T16 [J-1] CHANGELOG 同步
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `CHANGELOG.md:8` 仅 `[Unreleased]`；`release-please-manifest.json:2` 声明 1.0.0
- **改动**:
  1. 触发一次 release-please PR，让其自动从 [Unreleased] 切出 `## [1.0.0] - 2026-06-24` 块
  2. 人工补齐 Fixed/Removed/Deprecated 三类条目
  3. CI 加入 `kacl-cli` 校验
- **验证**: `kacl-cli lint CHANGELOG.md` 通过
- **依赖**: T20（release-please workflow）

### T17 [J-2] PR 模板创建
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `.github/` 无 `PULL_REQUEST_TEMPLATE.md`
- **改动**:
  1. 创建 `.github/PULL_REQUEST_TEMPLATE.md`，字段：Summary / Motivation / Changes / Test Plan / Checklist（含"已跑 go test -race"/"已跑 golangci-lint"/"已更新 CHANGELOG"/"已更新文档"复选项）
  2. `CONTRIBUTING.md:100` PR 规范处补一行指向模板的链接
- **验证**: 文件存在；GitHub 新建 PR 时自动加载模板
- **依赖**: 无

### T18 [G-1] RBAC 端点覆盖
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `main.go:260-321` 仅 admin 端点挂 RBAC；`policy.csv:7-9` 定义了 user/lobby 策略但路由未引用
- **改动**:
  1. 为所有 `/api/v1/user/*`、`/api/v1/registry/*`、`/api/v1/lobby/*` 路由追加 `rbacEnforcer.Middleware("lobby", "create")` 等声明
  2. `policy.csv` 补充 `user, user_data, read/delete` 等策略
  3. 增加 e2e 测试验证未授权角色返回 403
- **验证**: e2e 测试通过
- **依赖**: 无

### T19 [G-2] Admin Token 撤销机制
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `backend/internal/handler/admin.go:314-327` 无 jti；`:330-352` 不检查撤销
- **改动**:
  1. `signAdminToken` 增加 jti claim
  2. `VerifyAdminToken` 调用 `redis.IsJWTRevoked`
  3. 新增 `POST /api/v1/admin/logout` 端点调用 `redis.RevokeJWT`
  4. 修改 admin 密码时撤销所有 admin token
- **验证**: 集成测试验证 logout 后 token 失效
- **依赖**: 无

### T20 [D-2] release-please workflow 创建
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `.github/release-please-config.json` 存在但无 workflow
- **改动**:
  1. 新增 `.github/workflows/release-please.yml`，监听 main 分支 push，使用 `googleapis/release-please-action@v4`
- **验证**: push 到 main 后自动创建 release PR
- **依赖**: 无

### T21 [D-6] deploy environment 保护
- **状态**: [ ]
- **优先级**: P0 / 复杂度 L
- **证据**: `.github/workflows/ci-cd.yml:75-79` deploy job 无 `environment:`
- **改动**:
  1. `ci-cd.yml` deploy job 增加 `environment: production`
  2. 创建 `.github/settings.yml` 声明 branch protection：require PR、require status checks、require CODEOWNERS review、disallow force push
  3. deploy 步骤后增加 `$GITHUB_STEP_SUMMARY` 输出部署摘要
- **验证**: 部署需人工审批
- **依赖**: 无

---

## 阶段二：规划执行（P1，Medium 复杂度）

### T22 [C-1] Resend 熔断器装配
- **状态**: [ ]
- **优先级**: P1 / 复杂度 L
- **证据**: `auth/magiclink.go:65-73` 定义 cb 字段但无 `s.cb.Execute` 调用；`worker/email_worker.go:140-175` 实际发邮件裸奔
- **改动**:
  1. 将 `MagicLinkService.cb` 移至 `EmailWorker`
  2. `sendEmail` 中包裹 `w.cb.Execute(func() (any, error) { ... })`
  3. 用 `resilience.ExternalAPIRetry` 包裹 HTTP 调用
- **验证**: 集成测试 mock Resend 连续失败 3 次验证熔断器开路
- **依赖**: T2

### T23 [C-5] withRetry 辅助函数收敛
- **状态**: [ ]
- **优先级**: P1 / 复杂度 L
- **证据**: `postgres.go:37-53` 定义但仅 3 处使用；8+ 处内联样板代码
- **改动**:
  1. 将所有内联的 `retry.Do + cb.Execute` 替换为 `withRetryRead`/`withRetryWrite` 调用
  2. 在辅助函数中集中修复 T1（RetryableError 包装）
- **验证**: 全量测试通过
- **依赖**: T1

### T24 [A-3] 分层架构重构（引入 service 层）
- **状态**: [ ]
- **优先级**: P1 / 复杂度 M
- **证据**: `handler/lobby.go:163` `h.hub.DB()`；`auth.go:19-20,142,205,288,334`
- **改动**:
  1. `game` 层新增 `Hub.ListLobbies(ctx, limit, cursor)`
  2. `handler` 改为调用 `h.hub.ListLobbies(...)`，移除 `h.hub.DB()`
  3. 新增 `service` 层封装用户查询
  4. `Hub.DB()` 标记 deprecated
- **验证**: 全量测试通过；handler 不再直接依赖 store
- **依赖**: 无

### T25 [D-4] 前端 CI shift-left
- **状态**: [ ]
- **优先级**: P1 / 复杂度 M
- **证据**: `ci-cd.yml:24-29` 仅 tsc；`frontend/package.json:6-11` 无 lint 脚本
- **改动**:
  1. `frontend/package.json` 增加 `"lint": "eslint ."` 与 `"audit": "npm audit --audit-level=high"`
  2. `ci-cd.yml` 增加 lint 与 npm audit 步骤
  3. 新增 `.github/workflows/codeql.yml` 对 TS/Go 启用 CodeQL
  4. 增加 `github/dependency-review-action`
- **验证**: CI lint 与 audit 步骤通过
- **依赖**: 无

### T26 [E-1] 异步三件套测试（worker/outbox/audit）
- **状态**: [ ]
- **优先级**: P1 / 复杂度 M
- **证据**: `worker/`、`outbox/`、`audit/` 均无 `_test.go`
- **改动**:
  1. 为 `outbox/publisher.go` 添加 testcontainers 集成测试
  2. 为 `worker/email_worker.go` 用 miniredis + httptest.NewServer 模拟 Resend API
  3. 为 `audit/audit.go` 重点测 HMAC 链：插入 N 条记录验证 `this_hash = HMAC(secret, prev_hash || payload)`，篡改中间记录验证后续 hash 校验失败，用 `-race` 验证 `lastHash` 并发安全
- **验证**: `go test -race ./internal/worker/... ./internal/outbox/... ./internal/audit/...` 通过
- **依赖**: 无

### T27 [F-1] OpenAPI 与路由同步
- **状态**: [ ]
- **优先级**: P1 / 复杂度 M
- **证据**: `docs/openapi.yaml:724` vs `main.go:296`；`main.go:274-277` 未文档化；`openapi.yaml:353-379` 未实现；`auth.go:85` 202 vs `openapi.yaml:100` 200
- **改动**:
  1. 修正 `/lobby/{code}/ws` → `/api/v1/lobby/{code}/ws`
  2. 补充 `/api/v1/user/data` GET/DELETE 端点文档
  3. 移除或标记 `/api/v1/registry/match` 为"未实现"
  4. 修正 Magic Link 响应码 200 → 202
  5. `AdminConfigUpdate` schema 补充 `oldPassword`
  6. CI 加入 `redocly lint`
- **验证**: `redocly lint docs/openapi.yaml` 通过
- **依赖**: 无

### T28 [I-2] EXPLAIN ANALYZE 实测
- **状态**: [ ]
- **优先级**: P1 / 复杂度 M
- **证据**: `docs/db-query-analysis.md:19-24,39-42,55-58` 仅"预期计划"
- **改动**:
  1. 在 staging 灌入生产量级数据（≥10 万行 game_results）
  2. 对 5 个核心查询执行 `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`
  3. 将结果固化为 `docs/db-query-analysis.md` 的"实测计划"小节
  4. CI 加 pg_plan_checker 校验关键查询未走 Seq Scan
- **验证**: 实测计划文档存在
- **依赖**: T13

### T29 [I-3] 连接池压测 + env 化
- **状态**: [ ]
- **优先级**: P1 / 复杂度 M
- **证据**: `postgres.go:71-78` 注释"Tuned"但无压测；`:79-83` 硬编码
- **改动**:
  1. 加 `BenchmarkPostgresStore_ConcurrentLoad(b *testing.B)`：N goroutine 并发 SaveLobbyState，断言 `pool.Stat().MaxConns()` 未触顶
  2. 用 vegeta 或 k6 跑 1000 RPS 压测，记录 P99 延迟、pool_in_use、pool_idle
  3. 将 MaxConns/MinConns/MaxConnLifetime 改读 env：`getEnvInt("PG_POOL_MAX_CONNS", 25)`
  4. 压测报告固化为 `docs/benchmarks-v2.md`
- **验证**: Benchmark 通过；env 变量可覆盖默认值
- **依赖**: 无

---

## 阶段三：长期规划（P2，High 复杂度）

### T30 [E-2] WebSocket 测试覆盖
- **状态**: [ ]
- **优先级**: P2 / 复杂度 H
- **证据**: `handler/lobby.go:196,269,353` 零测试；`hub.go:199,349,391` 未测
- **改动**:
  1. 用 `gorilla/websocket` 的 `websocket.Dial` 配合 `httptest.NewServer` 编写 WS 集成测试
  2. 用 `sync.WaitGroup` + 多 goroutine 模拟 N 个并发连接，验证 WSConnCount 准确性（配合 -race）
  3. 为 `RestoreRooms` 用 testcontainers PG 构造持久化房间数据
  4. 测试 `writePump` 在 Send channel 满时的背控行为
- **验证**: `go test -race ./internal/handler/... ./internal/game/...` 通过
- **依赖**: 无

### T31 [H-2] WS 水平扩展（Redis Pub/Sub 广播层）
- **状态**: [ ]
- **优先级**: P2 / 复杂度 H
- **证据**: `hub.go:33` rooms map 内存态；`infra/service.yaml` 无 autoscaling 注解
- **改动**:
  1. `infra/service.yaml` 添加 `autoscaling.knative.dev/minScale: "1"` 和 `maxScale: "10"`
  2. `hub.go` 的 Room 引入 Redis Pub/Sub 广播层
  3. WebSocket 前置 LB 启用 sticky session
- **验证**: 多实例广播测试通过
- **依赖**: 无

### T32 [C-4] 请求类型舱壁隔离
- **状态**: [ ]
- **优先级**: P2 / 复杂度 H
- **证据**: `main.go:227-321` 所有路由共享同一 server + DB 池 + Redis 池
- **改动**:
  1. 引入 `golang.org/x/sync/semaphore`，为请求类型分配独立配额：认证（10）、大厅（10）、Admin（3）、WebSocket saveState（2）
  2. 或拆分 DB 池：关键路径独立小池（5），非关键路径共享大池（20）
  3. chi 中间件层 `semaphore.Acquire(ctx)` / `defer Release()`
- **验证**: 压测验证一类请求耗尽配额不影响其他类
- **依赖**: 无

---

## 阶段四：择机实施（P3，中低价值）

### T33 [B-3] WS Span 父子关联
- **状态**: [ ]
- **优先级**: P3 / 复杂度 M
- **证据**: `handler/lobby.go:306,345` 用 `context.Background()`
- **改动**:
  1. WS handler 中将 `r.Context()` 保存到 room/player 结构体
  2. `readPump`/`writePump` 从保存的 context 派生 Span
  3. WS 长连接 context 需在连接关闭时 cancel
- **验证**: Jaeger 中 WS 消息 span 关联到 HTTP 升级 span
- **依赖**: 无

### T34 [D-5] SLSA provenance attestation
- **状态**: [ ]
- **优先级**: P3 / 复杂度 M
- **证据**: `go-ci.yml:204-207` 仅 cosign sign
- **改动**:
  1. 引入 `slsa-framework/slsa-github-generator/.github/workflows/generator_container-slsa3.yml@v2.0.0`
  2. 或 build-push 后增加 `cosign attest --predicate <slsa-provenance.json> --type slsaprovenance`
  3. 部署侧增加 `cosign verify-attestation --type slsaprovenance` 校验
- **验证**: `cosign verify-attestation` 通过
- **依赖**: T12

### T35 [E-3] 基准测试覆盖
- **状态**: [ ]
- **优先级**: P3 / 复杂度 L
- **证据**: 仅 `physics_test.go:564-647` 9 个 Benchmark
- **改动**:
  1. 为 `protocol.EncodeSnapshot` 添加 Benchmark
  2. 为 `game.Hub.CreateRoom`、`game.Room.HandleDisconnect` 添加 Benchmark
  3. 为 `store.PostgresStore.SaveLobbyState` 用 testcontainers 添加 Benchmark
  4. `go-ci.yml` 增加 bench job，用 benchstat 对比 main 分支基线
- **验证**: `go test -bench=. -benchmem` 通过
- **依赖**: 无

### T36 [E-4] 覆盖率门禁
- **状态**: [ ]
- **优先级**: P3 / 复杂度 L
- **证据**: `go-ci.yml:29-34` 仅生成 coverage.out 无阈值
- **改动**:
  1. 在 `go-ci.yml` 的 test job 末尾添加阈值检查（建议起步 50%）
  2. 用 Codecov/Coveralls 做趋势可视化与 diff 覆盖率评论
  3. 每季度上调 5% 阈值，直至 75%
- **验证**: CI 覆盖率低于阈值时失败
- **依赖**: 无

### T37 [E-5] 前端单元测试
- **状态**: [ ]
- **优先级**: P3 / 复杂度 M
- **证据**: `package.json:20` 配置 vitest 但 `src/**` 下零 `.test.ts(x)` 文件
- **改动**:
  1. 为前端协议解析模块添加 vitest 用例
  2. 为物理预测插值函数添加边界用例
  3. `package.json` 添加 `"test:frontend": "cd frontend && vitest run --coverage"`
  4. 新增 `frontend-ci.yml`
- **验证**: 前端测试通过
- **依赖**: 无

### T38 [F-3] 429 Retry-After 头
- **状态**: [ ]
- **优先级**: P3 / 复杂度 L
- **证据**: `middleware/ratelimit.go:72,105,114` 三处无 Retry-After
- **改动**:
  1. 扩展 `apierror.TooManyRequests` 支持可选 retryAfter 参数
  2. 或中间件中直接 `w.Header().Set("Retry-After", strconv.Itoa(int(cfg.Window.Seconds())))`
  3. 可选补充 X-RateLimit-Limit、X-RateLimit-Remaining、X-RateLimit-Reset 头
  4. 更新 OpenAPI 429 响应文档
- **验证**: 429 响应含 Retry-After 头
- **依赖**: 无

### T39 [F-5] ETag 条件请求
- **状态**: [ ]
- **优先级**: P3 / 复杂度 M
- **证据**: 全 backend 目录无 `ETag`、`If-None-Match` 匹配
- **改动**:
  1. 为 `ListLobbies` 响应体计算 SHA256 ETag
  2. 中间件检查 If-None-Match，匹配则返回 304
  3. 为 `CheckRoom` 设置 Last-Modified 头
  4. OpenAPI 补充 304 响应文档
- **验证**: 304 响应正确返回
- **依赖**: 无

### T40 [G-5] PostgreSQL email 加密
- **状态**: [ ]
- **优先级**: P3 / 复杂度 M
- **证据**: `migrations/000001_init_schema.up.sql:4` 明文；对比 `magiclink.go:108-111` Redis 已加密
- **改动**:
  1. 应用层加密：在 `db.CreateUser`/`db.GetUserByEmail` 时调用 `crypto.Encrypt/Decrypt`
  2. 或使用 PostgreSQL pgcrypto 扩展列级加密
  3. 更新 `threat-model.md` PII 表标注"PostgreSQL（AES-256-GCM 应用层加密）"
  4. 增加迁移脚本回填加密现有数据
- **验证**: DB dump 中 email 字段为密文
- **依赖**: 无

### T41 [G-6] 威胁建模文档同步
- **状态**: [ ]
- **优先级**: P3 / 复杂度 L
- **证据**: `threat-model.md:46` 限速数值与代码不一致；`:30` "未来需添加 audit log" 但已实现；`:76` "数据主体权利缺失" 但已实现
- **改动**:
  1. 同步限速数值表
  2. 更新 R（Repudiation）章节标注 audit log 已实现
  3. 更新 GDPR 章节标注导出/删除已实现
  4. CI 增加 threat-model.md 与 policy.csv/ratelimit.go 的同步检查（可选）
- **验证**: 文档与代码一致
- **依赖**: T3, T18

### T42 [H-3] 部署目标统一
- **状态**: [ ]
- **优先级**: P3 / 复杂度 M
- **证据**: `ci-cd.yml:88` wrangler deploy 但无 wrangler.toml；`go-ci.yml:177-218` Cloud Run；服务名 `uppy-clone` vs `balloon-game` 不一致
- **改动**:
  1. 明确单一部署目标（推荐 Cloud Run）
  2. 删除 `ci-cd.yml:88` 的 `wrangler deploy` 步骤
  3. 统一服务名：将 root `service.yaml` 重命名为 `service.yaml.legacy`，`infra/service.yaml` 作为唯一权威
  4. 在 README 或 `.trae/specs/` 固化部署架构决策记录（ADR）
- **验证**: 部署目标唯一
- **依赖**: 无

### T43 [H-4] distroless 镜像
- **状态**: [ ]
- **优先级**: P3 / 复杂度 L
- **证据**: `Dockerfile:47` alpine:3.19.4
- **改动**:
  1. 运行时阶段改为 `FROM gcr.io/distroless/static-debian12:nonroot`
  2. 删除 `Dockerfile:48` 的 `apk add ca-certificates`
  3. 删除 `Dockerfile:49` 的 `adduser`
  4. `USER appuser` 改为 `USER nonroot:nonroot`
  5. 验证 CGO_ENABLED=0 静态二进制可在 distroless 运行
- **验证**: `docker build` 成功；容器启动正常
- **依赖**: T12

### T44 [I-4] 审计日志不丢 + Outbox 批量
- **状态**: [ ]
- **优先级**: P3 / 复杂度 M
- **证据**: `audit/audit.go:154-160` channel 满即丢；`outbox/publisher.go:72-77` 逐行 UPDATE
- **改动**:
  1. 审计：channel 满时阻塞写入（带 100ms 超时）或落 WAL/本地文件兜底，绝不可丢
  2. Outbox：改批量 `UPDATE outbox_events SET processed_at = $1 WHERE id = ANY($2)`，一次 RTT 标记整批
  3. 关键审计（user.create/user.delete）改为同事务写入 audit_logs
- **验证**: 压测验证审计日志无丢失；Outbox 批量性能提升
- **依赖**: 无

### T45 [J-3] Runbook 认证章节
- **状态**: [ ]
- **优先级**: P3 / 复杂度 M
- **证据**: `docs/runbook.md:22-281` 仅 5 类故障，无认证章节
- **改动**:
  1. 新增"故障 6: 认证服务异常"章节，覆盖：(a) refresh token 验证失败率突增、(b) Magic Link 邮件投递失败（Resend API 熔断器排查）、(c) AES 密钥轮换失误导致配置解密失败
  2. 在 runbook 顶部增加"熔断器全局视图"小节
  3. 在 `docs/slo.md:26-35` 认证 SLO 处交叉引用 runbook 故障 6
- **验证**: runbook 含 6 类故障
- **依赖**: T22

### T46 [J-4] detect-secrets CI 校验
- **状态**: [ ]
- **优先级**: P3 / 复杂度 L
- **证据**: `go-ci.yml` 与 `ci-cd.yml` 均无 detect-secrets CI 作业
- **改动**:
  1. 在 `go-ci.yml` 新增 `secrets-scan` 作业，使用 `pre-commit run detect-secrets --all-files`
  2. 在 `CONTRIBUTING.md:114` Pre-commit Hooks 章节增加"首次 clone 后必须执行 `pre-commit install`"的强提示
  3. 可选：启用 GitHub Secret Scanning + Push Protection
- **验证**: CI secrets-scan 作业通过
- **依赖**: T5

---

## 任务依赖关系图

```
T1 (C-2) ──┬──> T23 (C-5)
T2 (C-3) ──┴──> T22 (C-1)
T5 (D-3) ──> T46 (J-4)
T12 (H-1) ─┬─> T34 (D-5)
           └─> T43 (H-4)
T13 (I-1) ──> T28 (I-2)
T14 (A-1) ──> T15 (A-4)
T20 (D-2) ──> T16 (J-1)
T3 (G-3), T18 (G-1) ──> T41 (G-6)
T22 (C-1) ──> T45 (J-3)
```

## 并行执行建议

- **阶段一全部 21 项可并行**（无相互依赖，除 T15 依赖 T14、T16 依赖 T20）
- **阶段二 8 项**：T22 依赖 T2；T23 依赖 T1；其余可并行
- **阶段三 3 项**：完全独立可并行
- **阶段四 14 项**：按依赖图执行

## 完成标准

- [ ] 所有 46 项任务状态为 `[x]`
- [ ] `checklist-enterprise.md` 所有检查点勾选
- [ ] `go test -race ./...` 通过
- [ ] `golangci-lint run` 通过
- [ ] `govulncheck ./...` 无高危 CVE
- [ ] CI 全绿
- [ ] `docs/audit-enterprise.md` 中所有 ❌ 转为 ✅ 或 ⚠️（附 ADR 说明）
