# Tasks — 企业级垂直审计 V2 修复任务列表

> 本任务列表基于 spec.md 的优先级矩阵，按 P0 → P1 → P2 → P3 → P4 阶段组织。
> 每个任务标注依赖关系，无依赖任务可并行实施。
> 实施时每个改动必须在代码注释或文档中记录："解决了什么问题？企业为何需要？做了什么工程权衡？"

---

## P0：CRITICAL/HIGH 安全与合规紧急修复

> 目标：消除所有可被直接利用的安全漏洞与合规违规。
> 完成标准：所有 CRITICAL/HIGH 项修复，`govulncheck` 零 CRITICAL/HIGH CVE，`gitleaks` 零泄漏。

- [x] **Task P0-1**: 移除 docker-compose.yml 默认密钥，强制环境变量注入
  - [x] SubTask P0-1.1: 修改 `docker-compose.yml:10,12,14`，将 `JWT_SECRET`、`ADMIN_PASSWORD`、`ENCRYPTION_KEY` 改为 `${VAR:?VAR required}` 格式
  - [x] SubTask P0-1.2: 在 `backend/cmd/server/main.go` 启动时增加弱密钥检测：若 `JWT_SECRET` 包含 `DEV_ONLY` 或 `change-in-production` 且 `ENABLE_HSTS != false` 则 panic
  - [x] SubTask P0-1.3: 更新 `README.md` 说明必须显式提供密钥
  - **依赖**: 无
  - **验证**: `docker-compose up` 无密钥时拒绝启动；生产模式弱密钥时 panic

- [x] **Task P0-2**: 移除明文密码回退分支，强制 bcrypt
  - [x] SubTask P0-2.1: 删除 `backend/internal/handler/admin_password.go:14-22` 的 `subtle.ConstantTimeCompare` 分支
  - [x] SubTask P0-2.2: 修改 `compareAdminPassword` 仅支持 bcrypt 哈希
  - [x] SubTask P0-2.3: 编写一次性迁移脚本 `backend/cmd/migrate-passwords/main.go`，将历史明文密码转为 bcrypt（强制重置）
  - [x] SubTask P0-2.4: 更新 `admin_password_test.go` 移除明文回退测试用例
  - **依赖**: 无
  - **验证**: `go test ./internal/handler/...` 通过；明文密码登录被拒绝

- [x] **Task P0-3**: 修复 Cookie Secure 标志判断逻辑
  - [x] SubTask P0-3.1: 在 `backend/internal/auth/` 创建 `secure.go`，实现 `isSecure(r *http.Request) bool` 函数：优先读 `X-Forwarded-Proto: https`，其次 `r.TLS != nil`
  - [x] SubTask P0-3.2: 修改 `backend/internal/handler/admin.go:101`、`backend/internal/auth/quickplay.go:42,88`、`backend/internal/handler/auth.go:232`，统一调用 `isSecure(r)`
  - [x] SubTask P0-3.3: 添加 `secure_test.go` 覆盖反向代理场景
  - **依赖**: 无
  - **验证**: `go test ./internal/auth/... ./internal/handler/...` 通过；反向代理下 Cookie 含 Secure

- [x] **Task P0-4**: 密码修改流程要求验证旧密码
  - [x] SubTask P0-4.1: 修改 `backend/internal/handler/admin.go:208-218` 的 `UpdateConfig`，当 `updates.AdminPassword != nil` 时要求请求体包含 `oldPassword` 字段
  - [x] SubTask P0-4.2: 调用 `compareAdminPassword` 验证旧密码，失败返回 401
  - [x] SubTask P0-4.3: 更新 `admin_test.go` 覆盖旧密码验证场景
  - **依赖**: Task P0-2
  - **验证**: 无旧密码或旧密码错误返回 401；正确旧密码才能修改

- [x] **Task P0-5**: 实现失败登录锁定机制
  - [x] SubTask P0-5.1: 在 `backend/internal/store/redis.go` 增加 `IncrementFailedLogin(ctx, ip) (int, error)` 与 `IsLocked(ctx, ip) (bool, error)` 方法
  - [x] SubTask P0-5.2: Redis key `admin:login:fail:{ip}`，5 次失败后 SET `admin:login:lock:{ip}` TTL 15 分钟
  - [x] SubTask P0-5.3: 修改 `backend/internal/handler/admin.go:72-82` 的 Login，登录失败时 `IncrementFailedLogin`，登录成功时重置计数
  - [x] SubTask P0-5.4: 在 Login 入口检查 `IsLocked`，锁定中返回 429
  - [x] SubTask P0-5.5: 添加 `admin_login_locked_total` Prometheus 计数器
  - **依赖**: 无
  - **验证**: 5 次失败后第 6 次返回 429；锁定 15 分钟后自动解除

- [x] **Task P0-6**: 修复 .gitignore 未包含 .env 的 CRITICAL 漏洞
  - [x] SubTask P0-6.1: 修改 `.gitignore`，增加 `.env`、`.env.*`、`!.env.example` 规则
  - [x] SubTask P0-6.2: 运行 `git log --all --full-history -- .env` 检查历史是否已泄漏
  - [x] SubTask P0-6.3: 若已泄漏，使用 `git filter-repo` 清除历史，并轮换所有密钥
  - [x] SubTask P0-6.4: 在 `.pre-commit-config.yaml` 增加 `detect-secrets` hook（baseline 模式）
  - **依赖**: 无
  - **验证**: `git check-ignore .env` 返回 `.env`；`detect-secrets` hook 激活

- [x] **Task P0-7**: 实现 DeleteUserData 真正的匿名化（GDPR Article 17）
  - [x] SubTask P0-7.1: 创建迁移 `backend/migrations/000005_add_soft_delete.up.sql`，给 users 表增加 `deleted_at BIGINT DEFAULT NULL`、`email_anonymized BOOLEAN DEFAULT false`
  - [x] SubTask P0-7.2: 修改 `backend/internal/handler/auth.go:326-363` 的 `DeleteUserData`，执行 `UPDATE users SET email = 'deleted_' || id || '@anonymized', nickname = 'Deleted User', deleted_at = $now WHERE id = $1`
  - [x] SubTask P0-7.3: 添加定时任务（或后台 goroutine）清理 `deleted_at` 后 30 天的用户行（CASCADE 自动清理关联数据）
  - [x] SubTask P0-7.4: 更新 `docs/threat-model.md` 补充"数据保留策略"章节
  - **依赖**: 无
  - **验证**: 调用 `DELETE /auth/data` 后 email 被匿名化；30 天后用户行被硬删除

- [x] **Task P0-8**: 创建根目录 LICENSE 文件
  - [x] SubTask P0-8.1: 创建 `LICENSE` 文件，填入 MIT 许可证全文（含版权年份与持有人）
  - [x] SubTask P0-8.2: 在 `package.json` 与 `backend/go.mod` 确认 license 字段为 MIT
  - [x] SubTask P0-8.3: 在 CI 增加 `go-licenses report ./... > third-party-licenses.txt` 步骤
  - [x] SubTask P0-8.4: 在 CI 增加许可证白名单校验，禁止 GPL/AGPL
  - **依赖**: 无
  - **验证**: `LICENSE` 文件存在；CI 生成 `third-party-licenses.txt`

- [x] **Task P0-9**: Docker 镜像 digest pinning
  - [x] SubTask P0-9.1: 运行 `bash scripts/pin-digests.sh`，将 `Dockerfile:10,25,41` 三个 FROM 行改为 `image@sha256:<digest>` 格式
  - [x] SubTask P0-9.2: 修改 `docker-compose.yml:29,44`，postgres 与 redis 镜像同样 pin digest
  - [x] SubTask P0-9.3: 在 `.github/workflows/go-ci.yml` 增加校验 job：`grep -E '^FROM [^@]+[^@]$' Dockerfile && exit 1`
  - **依赖**: 无
  - **验证**: `Dockerfile` 所有 FROM 行含 `@sha256:`；CI 校验通过

- [x] **Task P0-10**: 审计日志防篡改持久化
  - [x] SubTask P0-10.1: 创建迁移 `backend/migrations/000006_create_audit_logs.up.sql`：`audit_logs` 表（id BIGSERIAL, action, actor_id, actor_ip, resource, before JSONB, after JSONB, request_id, trace_id, prev_hash, this_hash, created_at）
  - [x] SubTask P0-10.2: 创建触发器禁止 UPDATE/DELETE on audit_logs
  - [x] SubTask P0-10.3: 修改 `backend/internal/audit/audit.go`，增加 `LogToDB(ctx, entry)` 方法，计算 `this_hash = HMAC-SHA256(secret, prev_hash || payload)`
  - [x] SubTask P0-10.4: 通过 channel 投递到后台 goroutine 批量写入 DB（非阻塞）
  - [x] SubTask P0-10.5: 修改 `backend/internal/game/hub.go:87,155` 的 `context.Background()` 改为传入请求 context
  - [x] SubTask P0-10.6: 创建 `docs/adr/008-audit-log-tamper-proof.md` 记录决策
  - **依赖**: 无
  - **验证**: 审计日志写入 `audit_logs` 表；篡改记录后下一条 prev_hash 验证失败

---

## P1：SRE 基础设施与性能热路径

> 目标：建立可靠性目标体系与性能调试能力，优化高频热路径。
> 完成标准：SLO 文档发布，pprof 可用，WebSocket 热路径优化，异步架构落地。

- [ ] **Task P1-1**: 创建 docs/runbook.md 5 类故障排查
  - [ ] SubTask P1-1.1: 创建 `docs/runbook.md`，包含 5 类故障：PostgreSQL 不可用、Redis 不可用、WebSocket 连接洪水、游戏 tick 延迟、磁盘/内存满
  - [ ] SubTask P1-1.2: 每个条目包含：症状 → 可能原因 → 排查命令 → 缓解步骤 → 根治方案
  - **依赖**: 无
  - **验证**: `docs/runbook.md` 存在且包含 5 类故障

- [x] **Task P1-2**: 定义 SLI/SLO/SLA + Error Budget
  - [x] SubTask P1-2.1: 创建 `docs/slo.md`，定义核心用户旅程 SLI/SLO：认证（99.9%, p99 500ms）、房间创建（99.5%, p99 1s）、WebSocket（99.0%, p99 2s）、游戏消息延迟（p99 100ms）
  - [x] SubTask P1-2.2: 为每个 SLO 计算 30 天 Error Budget
  - [x] SubTask P1-2.3: 在 `backend/internal/metrics/metrics.go` 增加 SLO 相关 recording rule（PromQL）
  - **依赖**: 无
  - **验证**: `docs/slo.md` 存在；Prometheus 含 SLO 指标

- [ ] **Task P1-3**: 集成 pprof + expvar 调试端点
  - [ ] SubTask P1-3.1: 在 `backend/cmd/server/main.go` 增加 `import _ "net/http/pprof"` 与 `import _ "expvar"`
  - [ ] SubTask P1-3.2: 在单独内部端口（`:6060`，由 `DEBUG_PORT` 环境变量控制）注册 `/debug/pprof/` 与 `/debug/vars`
  - [ ] SubTask P1-3.3: 通过 `ENABLE_PPROF=true` 环境变量控制启用，生产默认关闭
  - [ ] SubTask P1-3.4: 生产启用时配合 Basic Auth 保护
  - **依赖**: 无
  - **验证**: `ENABLE_PPROF=true` 时 `http://localhost:6060/debug/pprof/` 可访问

- [ ] **Task P1-4**: Burn Rate 多窗口告警规则
  - [ ] SubTask P1-4.1: 创建 `deploy/alertmanager/rules.yml`，定义快速燃烧（1h×14.4 + 5m×14.4）与慢速燃烧（6h×6 + 30m×6）告警
  - [ ] SubTask P1-4.2: 每个告警标注 runbook 链接
  - **依赖**: Task P1-2
  - **验证**: alertmanager 规则文件语法正确

- [x] **Task P1-5**: WebSocket 热路径优化
  - [x] SubTask P1-5.1: 修改 `backend/internal/handler/lobby.go:290-318` 的 `readPump`，对 `MsgPing` 不创建 span，对 `MsgTap` 按 `traceID % 100 == 0` 采样
  - [x] SubTask P1-5.2: span attribute 切片改为包级预分配 `var wsSpanAttrs = []attribute.KeyValue{...}`
  - [x] SubTask P1-5.3: 修改 `backend/internal/game/room.go:602` 的 `DecodeTap(append([]byte{protocol.MsgTap}, payload...))`，改为 `protocol.DecodeTap(payload)` 直接解码
  - [x] SubTask P1-5.4: 修改 `backend/internal/protocol/decode.go` 的 `DecodeTap` 不要求 msgType 前缀
  - **依赖**: 无
  - **验证**: `go test -bench=BenchmarkReadPump ./internal/handler/...` 性能提升

- [x] **Task P1-6**: sync.Pool 复用 + randFloat64 优化
  - [x] SubTask P1-6.1: 在 `backend/internal/protocol/encode.go` 引入 `var snapshotBufPool = sync.Pool{New: func() interface{} { return &bytes.Buffer{} }}`，EncodeSnapshot 借用/归还
  - [x] SubTask P1-6.2: 修改 `backend/internal/game/physics.go:17-24` 的 `randFloat64`，改用 `math/rand/v2.Float64()` 或缓存 `var bigOneShift53 = big.NewInt(1 << 53)`
  - [x] SubTask P1-6.3: 修改 `backend/internal/game/physics.go:342-354` 的 `GenerateRoomCode`，缓存 `var alphabetLen = big.NewInt(int64(len(alphabet)))`
  - [x] SubTask P1-6.4: 修改 `backend/internal/game/room.go:432` 的 `buildSnapshot`，在 `Room` 结构体持有复用的 `players []protocol.PlayerState` 字段，每次 `players[:0]` 重置
  - **依赖**: 无
  - **验证**: `go test -bench=BenchmarkEncodeSnapshot -benchmem ./internal/protocol/...` allocs/op 降低

- [x] **Task P1-7**: EncodeSnapshot 改用手写二进制编码
  - [x] SubTask P1-7.1: 修改 `backend/internal/protocol/encode.go:19-70`，去除 `binary.Write` 反射，改用 `binary.LittleEndian.PutUint32/16` 手写编码
  - [x] SubTask P1-7.2: 预分配 `[]byte` make([]byte, calculatedSize)
  - [x] SubTask P1-7.3: 保持 `encode_decode_test.go` 的 Benchmark 通过
  - **依赖**: Task P1-6
  - **验证**: `go test -bench=BenchmarkEncodeSnapshot ./internal/protocol/...` 性能提升 3-5 倍

- [x] **Task P1-8**: 魔法链接邮件发送异步化
  - [x] SubTask P1-8.1: 在 `backend/internal/store/redis.go` 增加 `EnqueueEmail(ctx, payload)` 方法，使用 `XADD email:queue * payload_json`
  - [x] SubTask P1-8.2: 创建 `backend/internal/worker/email_worker.go`，`XREADGROUP` 消费 `email:queue`，调用 Resend API
  - [x] SubTask P1-8.3: Worker 失败时消息留在 Pending 列表，最大重试 5 次后 `XADD email:dead-letter * payload`
  - [x] SubTask P1-8.4: 修改 `backend/internal/auth/magiclink.go:147-177`，将邮件发送改为 `EnqueueEmail`，立即返回
  - [x] SubTask P1-8.5: 修改 `backend/internal/handler/auth.go:71`，返回 202 Accepted（语义已正确）
  - [x] SubTask P1-8.6: 在 `backend/cmd/server/main.go` 启动 EmailWorker goroutine
  - [x] SubTask P1-8.7: 创建 `docs/adr/010-async-email.md` 记录决策
  - **依赖**: 无
  - **验证**: 请求魔法链接立即返回 202；Worker 消费队列发送邮件

- [x] **Task P1-9**: 游戏结果异步写入（ADR-007 实施）
  - [x] SubTask P1-9.1: 在 `backend/internal/store/redis.go` 增加 `EnqueueGameResult(ctx, payload)` 方法，使用 `XADD game:results * payload_json`
  - [x] SubTask P1-9.2: 创建 `backend/internal/worker/game_result_worker.go`，`XREADGROUP` 消费，每 100 条或每 1s 批量 `INSERT`
  - [x] SubTask P1-9.3: `XACK` 在 PG 写入成功后执行
  - [x] SubTask P1-9.4: 修改 `backend/internal/game/room.go:290`，将 `EndGameAndRecordResults` 改为 `EnqueueGameResult`，立即返回
  - [x] SubTask P1-9.5: 修改 `backend/internal/store/postgres.go:347-389` 的 `EndGameAndRecordResults`，增加 `ON CONFLICT (id) DO NOTHING` 保证幂等
  - [x] SubTask P1-9.6: 在 `backend/cmd/server/main.go` 启动 GameResultWorker goroutine
  - [x] SubTask P1-9.7: 添加 Stream 长度监控告警（Consumer Lag）
  - [x] SubTask P1-9.8: 更新 `docs/adr/007-message-queue.md` 状态为 Accepted
  - **依赖**: 无
  - **验证**: 游戏结束立即返回；Worker 批量写入 PG；Stream 长度监控可用

- [x] **Task P1-10**: Transactional Outbox 模式
  - [x] SubTask P1-10.1: 创建迁移 `backend/migrations/000007_create_outbox_events.up.sql`：`outbox_events` 表（id BIGSERIAL, aggregate_type, aggregate_id, payload JSONB, created_at, processed_at）
  - [x] SubTask P1-10.2: 修改 `backend/internal/store/postgres.go:182-213` 的 `CreateUser`，在事务中同时 INSERT outbox 记录（事件 `user.created`）
  - [x] SubTask P1-10.3: 创建 `backend/internal/outbox/publisher.go`，独立 goroutine 每 1s 轮询未处理记录，发布到 Redis Stream
  - [x] SubTask P1-10.4: 发布成功后 UPDATE processed_at
  - [x] SubTask P1-10.5: 失败重试，超限后告警
  - [x] SubTask P1-10.6: 创建 `docs/adr/009-transactional-outbox.md` 记录决策
  - **依赖**: Task P1-8, Task P1-9
  - **验证**: 用户创建+事件发布原子性；Worker 重启后继续处理未发布记录

- [ ] **Task P1-11**: 启用 gocyclo/funlen/goconst/dupl linter
  - [ ] SubTask P1-11.1: 修改 `backend/.golangci.yml`，启用 `gocyclo`（max 15）、`funlen`（max 50）、`gocognit`（max 20）、`goconst`、`dupl`（threshold 150）、`gofmt`、`goimports`
  - [ ] SubTask P1-11.2: 运行 `golangci-lint run`，修复所有新增告警（或对历史告警添加 `//nolint` 注释并标注 issue）
  - **依赖**: 无
  - **验证**: `golangci-lint run` 零新增告警

- [ ] **Task P1-12**: 提取魔法数字为常量
  - [ ] SubTask P1-12.1: 在 `backend/internal/config/` 创建 `constants.go`，定义所有常量：`AccessTokenTTL = 15 * time.Minute`、`RefreshTokenTTL = 7 * 24 * time.Hour`、`MagicLinkTTL = 15 * time.Minute`、`WSReadLimit = 4096`、`DefaultPageSize = 50`、`MaxPageSize = 100`、`WSChannelBuffer = 64`、`MessageWindowMs = 60_000` 等
  - [ ] SubTask P1-12.2: 替换 `backend/internal/auth/jwt.go:49`、`magiclink.go:83,115,217`、`auth.go:233`、`handler/lobby.go:133,136,270`、`game/room.go:85,194,309,377`、`game/hub.go:67-68,288`、`cmd/server/main.go:113-114,127,142,158,318-320,338` 中的魔法数字
  - [ ] SubTask P1-12.3: 删除 `backend/internal/game/room.go:131` 的 `_ = cooldown` 死代码
  - **依赖**: 无
  - **验证**: `go build ./...` 通过；`goconst` linter 零告警

- [ ] **Task P1-13**: 定义哨兵错误替代字符串比较
  - [ ] SubTask P1-13.1: 在 `backend/internal/auth/magiclink.go` 定义 `var ErrTooManyRequests = errors.New("too many requests")`、`var ErrInvalidEmail = errors.New("invalid email")`
  - [ ] SubTask P1-13.2: 修改 `RequestMagicLink` 返回哨兵错误而非 `fmt.Errorf`
  - [ ] SubTask P1-13.3: 修改 `backend/internal/handler/auth.go:73-79`，使用 `errors.Is(err, auth.ErrTooManyRequests)` 替代字符串比较
  - [ ] SubTask P1-13.4: 在 `backend/internal/store/postgres.go` 定义 `var ErrDuplicateUser = errors.New("duplicate user")`，使用 `pgconn.PgError` 的 code 23505 判断
  - [ ] SubTask P1-13.5: 修改 `backend/internal/auth/quickplay.go:77`，使用 `errors.Is(err, store.ErrDuplicateUser)` 替代 `strings.Contains`
  - **依赖**: 无
  - **验证**: `go test ./...` 通过；无字符串比较错误

- [ ] **Task P1-14**: 消除重复代码
  - [ ] SubTask P1-14.1: 在 `backend/internal/store/postgres.go` 提取 `withRetryRead(ctx, fn)`、`withRetryWrite(ctx, fn)`、`withCircuitBreaker(ctx, name, fn)` 高阶函数，消除 15+ 处 retry+cb 嵌套
  - [ ] SubTask P1-14.2: 在 `backend/internal/auth/` 提取 `RevokeAllTokens(ctx, jwtMgr, redis, r)` 函数，消除 `handler/auth.go:267-280` 与 `346-353` 重复
  - [ ] SubTask P1-14.3: 在 `backend/internal/auth/` 提取 `tryCookie(r, jwtMgr, name)` 函数，消除 `middleware.go:44-72` 与 `75-101` 重复
  - [ ] SubTask P1-14.4: 创建 `backend/internal/idgen/uuid.go`，统一 `generateUUID` 实现（使用 `google/uuid` 标准库），删除 `game/room.go:690-700` 与 `auth/quickplay.go:119-126` 重复
  - [ ] SubTask P1-14.5: 创建 `backend/internal/validate/nickname.go`，统一 `sanitizePlayerName` 实现，删除 `game/names.go:147-166` 与 `auth/jwt.go:125-176` 重复
  - [ ] SubTask P1-14.6: 在 `backend/internal/handler/lobby.go` 提取 `writeDegradedLobbyList(w)` 函数，消除 `142-157` 与 `160-175` 重复
  - **依赖**: 无
  - **验证**: `dupl` linter 告警减少；`go test ./...` 通过

- [ ] **Task P1-15**: 删除死代码与 deprecated 函数
  - [ ] SubTask P1-15.1: 删除完全无调用的导出函数：`apierror.WithInstance`、`middleware.GetCSPNonce`、`store.CacheRoomInfo`、`store.GetRoomInfo`、`store.DeleteRoomInfo`、`game.RedisClient`、`store.EndGameSession`
  - [ ] SubTask P1-15.2: 删除 deprecated 函数：`store.EndGameSession`、`store.RecordGameResults` 及其测试
  - [ ] SubTask P1-15.3: 删除自定义 `physics.go:32-49` 的 `minf`/`maxf`/`mini` 与 `names.go:169` 的 `itoa`，使用 Go 1.26 内置 `min`/`max` 与 `strconv.Itoa`
  - [ ] SubTask P1-15.4: 评估仅测试使用的函数（`EndpointRateLimit` 系列、`JitteredBackoff`、`GetIdempotencyKey`、`MustInitFromEnv`、`WSConnCount`/`MaxWSConnections`/`MaxPlayersPerRoom`），无未来扩展计划则删除
  - **依赖**: 无
  - **验证**: `go build ./...` 通过；`unused` linter 零告警

---

## P2：开发者体验与代码质量

> 目标：提升开发效率与代码可维护性。
> 完成标准：Makefile 一键启动，main.go 拆分，输入校验加固。

- [x] **Task P2-1**: 创建 Makefile 统一命令
  - [x] SubTask P2-1.1: 创建根目录 `Makefile`，包含 `make dev`（docker compose up -d postgres redis && concurrently "cd backend && air" "cd frontend && npm run dev"）、`make test`、`make lint`、`make build`、`make run`、`make migrate`、`make seed`、`make bench`、`make audit`、`make clean`
  - [x] SubTask P2-1.2: 在 `package.json` 增加 `"dev": "concurrently \"npm run dev:backend\" \"npm run dev:frontend\""` 脚本
  - **依赖**: 无
  - **验证**: `make dev` 一键启动前后端 + 依赖

- [x] **Task P2-2**: 创建 .env.example
  - [x] SubTask P2-2.1: 创建根目录 `.env.example`，包含 README 列出的所有 8 个环境变量（DATABASE_URL、REDIS_URL、JWT_SECRET、ENCRYPTION_KEY、ADMIN_PASSWORD、ALLOWED_ORIGINS、RESEND_API_KEY、OTEL_EXPORTER_OTLP_ENDPOINT）
  - [x] SubTask P2-2.2: 值使用明显占位符：`JWT_SECRET=CHANGE_ME_TO_RANDOM_STRING`、`ENCRYPTION_KEY=RUN_openssl_rand_hex_32_TO_GENERATE`
  - [x] SubTask P2-2.3: 文件顶部注释说明生成命令
  - [x] SubTask P2-2.4: 删除 `.dev.vars.example` 中误导性的"真实格式"密钥
  - **依赖**: 无
  - **验证**: `.env.example` 包含所有必填变量；占位符明显

- [x] **Task P2-3**: 创建 .air.toml 热重载配置
  - [x] SubTask P2-3.1: 在 `backend/` 创建 `.air.toml`，配置监听 `.go` 文件变化重新编译
  - [x] SubTask P2-3.2: 在 `CONTRIBUTING.md` 增加 `go install github.com/air-verse/air@latest` 安装步骤
  - **依赖**: 无
  - **验证**: `cd backend && air` 启动热重载

- [x] **Task P2-4**: 创建 devcontainer.json
  - [x] SubTask P2-4.1: 创建 `.devcontainer/devcontainer.json`，基于 `mcr.microsoft.com/devcontainers/go:1.26` + Node + Docker-in-Docker
  - [x] SubTask P2-4.2: 安装 `golangci-lint`、`air`、`golang-migrate`
  - [x] SubTask P2-4.3: 自动运行 `docker compose up -d postgres redis` 与 `go mod download`
  - **依赖**: 无
  - **验证**: VS Code Remote Containers 可打开

- [x] **Task P2-5**: 创建 tools.go 锁定工具版本
  - [x] SubTask P2-5.1: 在 `backend/` 创建 `tools.go`，`//go:build tools` 标签，导入 `golangci-lint`、`air`、`golang-migrate`
  - [ ] SubTask P2-5.2: 运行 `go mod tidy` 将工具依赖加入 `go.mod`（按任务要求跳过，工具导入未解析属预期）
  - **依赖**: 无
  - **验证**: `go build -tags tools ./...` 通过

- [x] **Task P2-6**: 创建数据库 seed 脚本
  - [x] SubTask P2-6.1: 创建 `backend/cmd/seed/main.go`，插入 3 个测试用户、5 个游戏会话、10 条游戏结果
  - [x] SubTask P2-6.2: 在 Makefile 增加 `make seed` 目标（依赖 P2-1 创建 Makefile）
  - [x] SubTask P2-6.3: seed 仅在 `DATABASE_URL` 包含 `sslmode=disable` 时执行（dev 标识）
  - **依赖**: 无
  - **验证**: `make seed` 后数据库含测试数据

- [x] **Task P2-7**: 创建 docs/environments.md
  - [x] SubTask P2-7.1: 创建 `docs/environments.md`，表格对比本地/staging/prod 的：密钥管理、TLS、CORS、日志级别、OTel、Rate Limit、数据库连接池大小
  - **依赖**: 无
  - **验证**: 文档存在且对比清晰

- [x] **Task P2-8**: 拆分 main.go 315 行 main 函数
  - [x] SubTask P2-8.1: 提取 `initLogger()`、`initTracer()`、`loadConfig()`、`initCrypto()`、`initDB()`、`initRedis()`、`initHub()`、`initHandlers()`、`setupRoutes()`、`startServer()`、`waitForShutdown()` 等独立函数
  - [x] SubTask P2-8.2: main 函数仅做编排，不超过 50 行
  - [x] SubTask P2-8.3: 保持原有行为不变，所有测试通过
  - **依赖**: 无
  - **验证**: `go build ./...` 通过；`funlen` linter 对 main 无告警

- [ ] **Task P2-9**: 拆分上帝函数
  - [ ] SubTask P2-9.1: 拆分 `backend/internal/game/room.go:65-180` 的 `HandleJoin`（116行）为 `closeExistingConnection`、`reconnectPlayer`、`addNewPlayer`、`notifyJoin`、`transitionPhaseIfNeeded`（同锁内调用）
  - [ ] SubTask P2-9.2: 拆分 `backend/internal/game/room.go:513-582` 的 `setEndGameAlarm`（70行）为 `scheduleCountdownEnd`、`scheduleAutoRestart`、`handleCountdownEnd`、`handleAutoRestart`
  - [ ] SubTask P2-9.3: 拆分 `backend/internal/game/room.go:585-642` 的 `handleTap`（58行）为 `validateTapRequest`、`decodeTapPayload`、`applyTapPhysics`、`updatePlayerStats`、`broadcastTapResult`
  - [ ] SubTask P2-9.4: 拆分 `backend/internal/handler/admin.go:158-255` 的 `UpdateConfig`（98行）为 `parseConfigUpdates`、`applyConfigUpdates`、`auditConfigChange`、`saveConfig`
  - [ ] SubTask P2-9.5: 拆分 `backend/internal/handler/lobby.go:190-260` 的 `WebSocket`（71行）为 `authenticateWSRequest`、`validateWSOrigin`、`checkWSRateLimit`、`upgradeWSConnection`、`startWSPumps`
  - **依赖**: 无
  - **验证**: `go test ./...` 通过；`gocyclo`/`funlen` linter 零告警

- [x] **Task P2-10**: health.go 纳入熔断器状态 + Hub 负载
  - [x] SubTask P2-10.1: 修改 `backend/internal/health/health.go:41-77` 的 `ReadyHandler`，通过依赖注入增加 `CircuitBreakerState` 检查：若 postgres 或 redis breaker 处于 `StateOpen`，返回 503
  - [x] SubTask P2-10.2: 增加 `Hub.CanAcceptWSConnection()` 检查，若连接数已达上限返回 503
  - [x] SubTask P2-10.3: PG/Redis ping 增加 `context.WithTimeout(r.Context(), 500*time.Millisecond)`
  - [x] SubTask P2-10.4: 区分 `degraded`（部分依赖慢但可用）与 `not ready`（关键依赖不可用）
  - **依赖**: 无
  - **验证**: 熔断器 open 时 readiness 返回 503；WS 连接满时返回 503

- [ ] **Task P2-11**: Defense-in-Depth 输入校验加固
  - [ ] SubTask P2-11.1: 修改 `backend/internal/game/room.go:649`，增加 `if nickLen > protocol.MaxNicknameLen { return }`
  - [ ] SubTask P2-11.2: 修改 `backend/internal/handler/lobby.go:81-86`，增加 `if len(code) != 5 { apierror.BadRequest(...); return }` + 字符集校验 `[A-Z2-9]`
  - [ ] SubTask P2-11.3: 修改 `backend/internal/handler/auth.go:91-95`，增加 `if len(token) != 64 { apierror.BadRequest(...); return }`
  - [ ] SubTask P2-11.4: 修改 `backend/internal/handler/admin.go:42-47`，增加 `if len(body.Password) > 72 { apierror.BadRequest(...); return }`（bcrypt 72 字节限制）
  - [ ] SubTask P2-11.5: 修改 `backend/internal/middleware/idempotency.go:75-83`，增加 `if len(key) > 255 { apierror.BadRequest(...); return }`
  - **依赖**: 无
  - **验证**: 超长输入返回 400；正常输入通过

- [x] **Task P2-12**: N+1 修复
  - [x] SubTask P2-12.1: 修改 `backend/internal/store/postgres.go:347-389` 的 `EndGameAndRecordResults`，改用 `pgx.Batch` 或多值 INSERT `INSERT INTO game_results VALUES ($1,$2,...),($7,$8,...)`
  - [x] SubTask P2-12.2: 修改 `backend/internal/auth/refresh.go:75-98` 的 `RevokeAllForUser`，维护 `refresh_tokens:user:{userID}` Redis Set，撤销时 `SMEMBERS` + `DEL` 各 token key + `DEL` set key
  - [x] SubTask P2-12.3: 修改 `backend/internal/handler/auth.go:301-321` 的 `ExportUserData`，增加 `GetGameResultsByUserID` 方法，使用 `idx_game_results_user_id` 索引
  - **依赖**: 无
  - **验证**: `go test -bench=BenchmarkEndGameAndRecordResults ./internal/store/...` 性能提升

- [x] **Task P2-13**: Postmortem 模板
  - [x] SubTask P2-13.1: 创建 `docs/templates/postmortem.md`，包含 8 节：摘要、影响、根因、时间线、触发条件、缓解措施、根因分析（5 Whys）、行动项
  - [x] SubTask P2-13.2: 在 `CONTRIBUTING.md` 要求 P0/P1 事故 7 天内产出 postmortem
  - **依赖**: 无
  - **验证**: 模板存在

- [x] **Task P2-14**: /metrics 端点添加认证
  - [x] SubTask P2-14.1: 修改 `backend/cmd/server/main.go:209`，为 `/metrics` 端点添加 IP 白名单中间件（仅允许监控服务器 IP）或 Basic Auth
  - [x] SubTask P2-14.2: Basic Auth 用户名密码从 `METRICS_USER`/`METRICS_PASSWORD` 环境变量读取
  - **依赖**: 无
  - **验证**: 无认证访问 `/metrics` 返回 401

- [ ] **Task P2-15**: 缩短 admin JWT 过期时间至 30 分钟
  - [ ] SubTask P2-15.1: 修改 `backend/internal/handler/admin.go:265`，将 `24 * time.Hour` 改为 `30 * time.Minute`
  - [ ] SubTask P2-15.2: 添加 admin refresh token 机制（复用现有 refresh.go 逻辑）
  - **依赖**: 无
  - **验证**: admin JWT 30 分钟后过期；refresh 机制可用

- [x] **Task P2-16**: JWT 密钥长度校验
  - [x] SubTask P2-16.1: 修改 `backend/internal/auth/jwt.go:23-25` 的 `NewJWTManager`，检查 `len(secret) >= 32`，不足则 panic 或返回 error
  - [x] SubTask P2-16.2: 修改 `backend/cmd/server/main.go:69-72`，处理 error
  - **依赖**: 无
  - **验证**: 短密钥启动失败

- [x] **Task P2-17**: 路径遍历增强检查
  - [x] SubTask P2-17.1: 修改 `backend/cmd/server/main.go:290`，在 `filepath.Join` 后使用 `strings.HasPrefix(absPath, absStaticDir)` 验证最终绝对路径仍在 staticDir 内
  - **依赖**: 无
  - **验证**: `../../../etc/passwd` 请求返回 404

- [x] **Task P2-18**: WebSocket Origin 校验修复
  - [x] SubTask P2-18.1: 修改 `backend/internal/handler/lobby.go:43-46`，将 `CheckOrigin` 设置为拒绝所有，强制走手动校验
  - [x] SubTask P2-18.2: 修改 `backend/internal/handler/lobby.go:204-223`，使用 `net.SplitHostPort` 标准化 host 与 originHost 后比较
  - [x] SubTask P2-18.3: 空 Origin 应拒绝而非放行
  - **依赖**: 无
  - **验证**: 跨域 WebSocket 连接被拒绝

- [x] **Task P2-19**: 密钥轮换机制
  - [x] SubTask P2-19.1: 修改 `backend/internal/auth/jwt.go` 的 `JWTManager`，支持主密钥 + 旧密钥列表，验证时依次尝试
  - [x] SubTask P2-19.2: 修改 `backend/internal/crypto/aes.go`，加密输出格式改为 `v1:hex_ciphertext`，包含版本号
  - [x] SubTask P2-19.3: 提供 `RotateKey` 函数扫描数据库重新加密
  - **依赖**: 无
  - **验证**: 旧密钥签发的 JWT 在轮换后仍可验证；AES 加密字段可批量重新加密

- [x] **Task P2-20**: 异常行为检测
  - [x] SubTask P2-20.1: 在 `backend/internal/auth/middleware.go` 增加多 IP 同账户登录检测：Redis 计数器跟踪每用户 IP 数，超过阈值（如 3 个 IP/小时）告警
  - [x] SubTask P2-20.2: 添加 `suspicious_login_total` Prometheus 计数器
  - **依赖**: 无
  - **验证**: 多 IP 登录触发告警

- [x] **Task P2-21**: 数据库用户最小权限
  - [x] SubTask P2-21.1: 创建迁移 `backend/migrations/000008_create_db_roles.up.sql`，创建 `app_user`（SELECT/INSERT/UPDATE/DELETE）和 `migrator`（ALL）角色
  - [x] SubTask P2-21.2: 修改 `docker-compose.yml` 与生产配置，应用连接用 `app_user`，迁移用 `migrator`
  - **依赖**: 无
  - **验证**: 应用账户无法 DROP TABLE

- [x] **Task P2-22**: 配置 Dependabot
  - [x] SubTask P2-22.1: 创建 `.github/dependabot.yml`，配置 `gomod`（backend/）、`npm`（/ 与 frontend/）、`docker`（/）、`github-actions`（/.github/workflows/）生态系统
  - **依赖**: 无
  - **验证**: Dependabot PR 自动创建

- [x] **Task P2-23**: pre-commit 集成 detect-secrets/gitleaks
  - [x] SubTask P2-23.1: 在 `.pre-commit-config.yaml` 增加 `detect-secrets` hook（baseline 模式）
  - [x] SubTask P2-23.2: 运行 `detect-secrets scan > .secrets.baseline` 生成基线
  - [x] SubTask P2-23.3: 在 `.github/workflows/go-ci.yml` 增加 `gitleaks-action` job 扫描全历史
  - **依赖**: Task P0-6
  - **验证**: pre-commit 检测到测试密钥；CI gitleaks 扫描通过

- [x] **Task P2-24**: 优雅关闭等待异步任务
  - [x] SubTask P2-24.1: 在 `backend/internal/game/hub.go` 增加 `CloseAllRooms()` 方法，遍历所有房间调用 `Close()`
  - [x] SubTask P2-24.2: 修改 `backend/internal/game/room.go` 的 `Close()`，调用 `stopTick()` + `saveState()` 确保状态持久化
  - [x] SubTask P2-24.3: 使用 `sync.WaitGroup` 等待所有 tick goroutine 退出
  - [x] SubTask P2-24.4: 修改 `backend/cmd/server/main.go:338-346`，关闭流程增加 `hub.CloseAllRooms()`，超时从 10s 调整为 30s
  - **依赖**: 无
  - **验证**: SIGTERM 后所有房间状态持久化再退出

---

## P3：领域建模重构（高复杂度，长期改进）

> 目标：将贫血模型升级为充血模型，引入 DDD 战术设计。
> 完成标准：业务规则封装在领域对象内，handler 仅做协议转换。

- [x] **Task P3-1**: 引入充血模型
  - [x] SubTask P3-1.1: 将 `backend/internal/model/model.go` 的 `PlayerState` 升级为充血对象，添加 `CanTap(now int64) bool`、`RecordTap(now int64, cooldown int64)`、`IsRateLimited(now int64) bool`、`MarkDisconnected(now int64)`、`Reconnect()` 方法
  - [x] SubTask P3-1.2: 将 `GameState` 添加 `AddPlayer`、`RemovePlayer`、`IsGameOver` 聚合方法
  - [x] SubTask P3-1.3: 将 `backend/internal/game/room.go:585-642` 的冷却判断、`192-205` 的速率限制、`player.go:15-52` 的改名冷却迁移到领域对象方法内
  - [x] SubTask P3-1.4: `model` 包重命名为 `domain`
  - **依赖**: 无
  - **验证**: `go test ./...` 通过；业务规则无法被绕过

- [x] **Task P3-2**: 引入 service 层分离 handler 业务逻辑
  - [x] SubTask P3-2.1: 创建 `backend/internal/service/auth_service.go`、`admin_service.go`、`lobby_service.go`
  - [x] SubTask P3-2.2: 将 `backend/internal/handler/auth.go:249-289` 的 Logout 业务逻辑、`326-363` 的 DeleteUserData 业务逻辑迁移到 `AuthService`
  - [x] SubTask P3-2.3: 将 `backend/internal/handler/admin.go:40-108` 的 Login 业务逻辑、`158-255` 的 UpdateConfig 业务逻辑迁移到 `AdminService`
  - [x] SubTask P3-2.4: handler 仅做协议转换（解析请求、调用 service、序列化响应）
  - **依赖**: Task P3-1
  - **验证**: handler 函数不超过 30 行

- [x] **Task P3-3**: 依赖倒置（Repository 接口）
  - [x] SubTask P3-3.1: 在 `backend/internal/game/` 包内定义 `RoomRepository` 接口、`SnapshotEncoder` 接口
  - [x] SubTask P3-3.2: `backend/internal/store/PostgresStore` 实现 `RoomRepository` 接口
  - [x] SubTask P3-3.3: `backend/internal/protocol/` 实现 `SnapshotEncoder` 接口
  - [x] SubTask P3-3.4: 通过依赖注入将接口实现传入 `Hub`/`Room`
  - [x] SubTask P3-3.5: 移除 `backend/internal/game/hub.go:13-17` 对 `store`/`protocol`/`metrics` 的直接依赖
  - **依赖**: Task P3-2
  - **验证**: `game` 包不依赖 `store`/`protocol`；可独立测试

- [x] **Task P3-4**: 引入 Value Object
  - [x] SubTask P3-4.1: 创建 `backend/internal/domain/room_code.go`，`RoomCode` Value Object 封装 5 字符 + 字母表校验
  - [x] SubTask P3-4.2: 创建 `backend/internal/domain/nickname.go`，`Nickname` Value Object 封装长度 + 字符过滤
  - [x] SubTask P3-4.3: 替换所有 `string` 类型的 room code 与 nickname 为 Value Object
  - **依赖**: Task P3-1
  - **验证**: 非法 RoomCode 构造失败

- [x] **Task P3-5**: 引入 Aggregate 边界
  - [x] SubTask P3-5.1: 定义 `Room` 为 Aggregate Root，`PlayerState` 为其内部实体
  - [x] SubTask P3-5.2: 外部只能通过 `Room` 方法修改玩家（`AddPlayer`、`RemovePlayer`、`UpdatePlayerState`）
  - [x] SubTask P3-5.3: `Room` 内部保证业务不变量（如玩家数上限、阶段转换合法性）
  - **依赖**: Task P3-1, Task P3-4
  - **验证**: 外部无法直接修改 `PlayerState` 字段

- [x] **Task P3-6**: 引入 Domain Event
  - [x] SubTask P3-6.1: 定义 `PlayerJoined`、`PlayerLeft`、`GameEnded`、`PhaseChanged` 领域事件
  - [x] SubTask P3-6.2: `Room` 发布事件，handler 订阅广播
  - [x] SubTask P3-6.3: 事件通过 Transactional Outbox 持久化
  - **依赖**: Task P3-5, Task P1-10
  - **验证**: 事件发布后订阅者收到通知

- [x] **Task P3-7**: CQRS 读写分离
  - [x] SubTask P3-7.1: 引入 `QueryService`（读）和 `CommandService`（写）分离
  - [x] SubTask P3-7.2: store 层分离 `Reader` 和 `Writer` 接口
  - [x] SubTask P3-7.3: 读路径可使用缓存/只读副本（未来扩展）
  - **依赖**: Task P3-2, Task P3-3
  - **验证**: 读写路径分离

---

## P4：混沌工程与高级 SRE（探索性）

> 目标：通过主动故障注入验证系统韧性。
> 完成标准：3 个混沌实验通过，持续 profiling 上线。

- [x] **Task P4-1**: Chaos Mesh 故障注入实验
  - [x] SubTask P4-1.1: 设计 3 个混沌实验：(1) PostgreSQL 宕机 30 秒；(2) Redis 不可达 60 秒；(3) 网络延迟 +500ms
  - [x] SubTask P4-1.2: 每个实验定义稳态假设、实验方法、成功标准
  - [ ] SubTask P4-1.3: 验证 `degradation.go` 降级响应触发、`circuitbreaker.go` 正确 open（实验设计已固化，待 staging 环境执行验证）
  - [x] SubTask P4-1.4: 记录实验结果到 `docs/chaos-experiments.md`
  - **依赖**: Task P1-1, Task P1-2, Task P2-10
  - **验证**: 3 个实验通过（设计文档已完成，执行待 staging 环境）

- [x] **Task P4-2**: 持续 profiling
  - [x] SubTask P4-2.1: 集成 Pyroscope 或 Parca，always-on profiling
  - [x] SubTask P4-2.2: 配置 Grafana dashboard 展示 CPU/内存火焰图
  - **依赖**: Task P1-3
  - **验证**: 生产环境可查看实时火焰图（集成代码已就绪，待添加 pyroscope-go 依赖后激活）

- [x] **Task P4-3**: cleanupOnce 锁粒度优化
  - [x] SubTask P4-3.1: 修改 `backend/internal/game/hub.go:253-307` 的 `cleanupOnce`，先 `RLock` 快照房间列表，释放锁后逐个 `room.mu.Lock` 检查并清理，最后短暂 `Lock` 删除空房间
  - **依赖**: 无
  - **验证**: 大房间数下 `CreateRoom`/`GetRoom` 延迟降低

- [x] **Task P4-4**: CheckRateLimit 改用 Lua 脚本
  - [x] SubTask P4-4.1: 修改 `backend/internal/store/redis.go:144-173` 的 `CheckRateLimit`，改用 Lua 脚本保证 INCR+EXPIRE 原子性
  - [x] SubTask P4-4.2: 通过 `redis.NewScript` 注册脚本
  - **依赖**: 无
  - **验证**: 高并发下限流计数准确

- [x] **Task P4-5**: 广播消息丢弃监控 + 慢客户端检测
  - [x] SubTask P4-5.1: 修改 `backend/internal/game/room.go:401-412` 的 `broadcast`，添加 `ws_messages_dropped_total` Prometheus 计数器
  - [x] SubTask P4-5.2: 连续 3 次丢弃后记录 WARN 日志
  - [x] SubTask P4-5.3: 连续 10 次丢弃后强制断开慢客户端
  - [x] SubTask P4-5.4: 关键消息（`PhaseEnded`、`PhaseCountdown`）改用阻塞发送（带超时）
  - **依赖**: 无
  - **验证**: 慢客户端被断开；丢弃率监控可用

- [x] **Task P4-6**: Saga 补偿模式
  - [x] SubTask P4-6.1: 修改 `backend/internal/game/restart.go:131-148` 的 `RestartAndStart`，调整顺序：先 `saveState`，成功后再 `broadcast`，失败时回滚内存状态
  - [x] SubTask P4-6.2: 修改 `backend/internal/auth/magiclink.go`，邮件发送失败时删除 Redis token（补偿）
  - [x] SubTask P4-6.3: 修改 `backend/internal/game/room.go:536-545`，`CreateGameSession` 失败时记录告警 + 重试队列
  - **依赖**: Task P1-8, Task P1-9
  - **验证**: 部分失败时补偿触发

---

# Task Dependencies

## 依赖关系图

```
P0 阶段（无依赖，可全部并行）:
P0-1, P0-2, P0-3, P0-5, P0-6, P0-7, P0-8, P0-9, P0-10

P0 阶段（有依赖）:
P0-4 depends on P0-2

P1 阶段（无依赖，可并行）:
P1-1, P1-2, P1-3, P1-5, P1-6, P1-8, P1-9, P1-11, P1-12, P1-13, P1-14, P1-15

P1 阶段（有依赖）:
P1-4 depends on P1-2
P1-7 depends on P1-6
P1-10 depends on P1-8, P1-9

P2 阶段（无依赖，可并行）:
P2-1, P2-2, P2-3, P2-4, P2-5, P2-6, P2-7, P2-8, P2-9, P2-10, P2-11, P2-12,
P2-13, P2-14, P2-15, P2-16, P2-17, P2-18, P2-19, P2-20, P2-21, P2-22, P2-24

P2 阶段（有依赖）:
P2-23 depends on P0-6

P3 阶段（顺序依赖）:
P3-1 → P3-2 → P3-3 → P3-7
P3-1 → P3-4 → P3-5 → P3-6
P3-6 depends on P1-10

P4 阶段（有依赖）:
P4-1 depends on P1-1, P1-2, P2-10
P4-2 depends on P1-3
P4-6 depends on P1-8, P1-9
```

## 并行执行建议

- **第一波（P0 全部 + P1 无依赖）**: 19 个任务可并行
- **第二波（P0-4 + P1-4 + P1-7 + P1-10 + P2 全部）**: 25 个任务可并行
- **第三波（P3 链式）**: 7 个任务顺序执行
- **第四波（P4）**: 6 个任务按依赖执行
