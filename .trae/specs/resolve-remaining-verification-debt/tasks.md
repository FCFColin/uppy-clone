# Tasks

## P0：环境搭建与迁移冲突修复（阻塞所有后续验证）

- [ ] Task 1: 修复数据库迁移版本号冲突
  - [ ] 将 `backend/migrations/000008_create_db_roles.up.sql` 重命名为 `000009_create_db_roles.up.sql`
  - [ ] 将 `backend/migrations/000008_create_db_roles.down.sql` 重命名为 `000009_create_db_roles.down.sql`
  - [ ] 验证 `backend/migrations/` 下无重复版本号

- [ ] Task 2: 创建开发环境 `.env` 文件
  - [ ] 基于 `.env.example` 创建 `.env`，填入开发环境真实密钥（`openssl rand -hex 32` 生成 JWT_SECRET 与 ENCRYPTION_KEY）
  - [ ] 设置 `ADMIN_PASSWORD`、`ALLOWED_ORIGINS=http://localhost:5173`、`DATABASE_URL`、`REDIS_URL`
  - [ ] 验证 `.gitignore` 含 `.env`（已确认）

- [ ] Task 3: 修复 docker-compose.yml 环境变量缺失
  - [ ] 添加 `ALLOWED_ORIGINS`、`EMAIL_FROM`、`OTEL_EXPORTER_OTLP_ENDPOINT` 环境变量
  - [ ] 添加 `migrate` 初始化容器（运行 migrations up 后退出）
  - [ ] 确保 app 容器 `depends_on: migrate` 完成后启动

- [ ] Task 4: 启动 Docker 环境并验证健康
  - [ ] 执行 `docker-compose up -d postgres redis`
  - [ ] 验证 postgres 与 redis 容器 healthy
  - [ ] 执行 `docker-compose up -d` 启动全部服务

## P1：运行时验证（依赖 P0）

- [ ] Task 5: 数据库迁移正向执行
  - [ ] 执行 `make migrate` 或 `golang-migrate -path backend/migrations -database "$DATABASE_URL" up`
  - [ ] 验证所有表创建成功（users、game_sessions、game_results、lobbies、audit_logs、outbox_events 等）
  - [ ] 验证 000009_create_db_roles 创建 app_user 与 migrator 角色

- [ ] Task 6: 数据库迁移反向执行
  - [ ] 执行 `golang-migrate ... down` 回退全部迁移
  - [ ] 验证所有表删除
  - [ ] 重新执行 `up` 恢复

- [ ] Task 7: 执行 make seed 插入测试数据
  - [ ] 执行 `make seed`
  - [ ] 验证 users、game_sessions、game_results 表含测试数据

- [ ] Task 8: 端点验证
  - [ ] `curl http://localhost:8080/health` — 返回 200
  - [ ] `curl http://localhost:8080/ready` — 返回 200
  - [ ] `curl http://localhost:8080/metrics` — 返回 401（未认证）
  - [ ] `curl -u user:pass http://localhost:8080/metrics` — 返回 200（认证后）
  - [ ] `ENABLE_PPROF=true` 时 `curl http://localhost:6060/debug/pprof/` — 返回 200

## P2：安全扫描工具安装与执行（依赖 P0）

- [ ] Task 9: 安装缺失的安全扫描工具
  - [ ] `go install github.com/gitleaks/gitleaks/v8@latest`（gitleaks）
  - [ ] `go install github.com/sigstore/cosign/v2/cmd/cosign@latest`（cosign）
  - [ ] trivy 通过 Docker 运行（`docker run aquasec/trivy`），无需本地安装

- [ ] Task 10: 执行 govulncheck 扫描
  - [ ] 执行 `cd backend && govulncheck ./...`
  - [ ] 记录输出；若有 CRITICAL/HIGH CVE，更新依赖修复

- [ ] Task 11: 执行 gitleaks 密钥泄漏扫描
  - [ ] 执行 `gitleaks detect --source . --report-path leaks.json`
  - [ ] 验证零泄漏；若有泄漏，从 git 历史清除

- [ ] Task 12: 执行 trivy 文件系统与镜像扫描
  - [ ] 执行 `docker run --rm -v ${PWD}:/repo aquasec/trivy fs /repo`（文件系统扫描）
  - [ ] 执行 `docker run --rm aquasec/trivy image <app-image>`（镜像扫描）
  - [ ] 验证零 CRITICAL CVE

- [ ] Task 13: 执行 cosign 签名验证流程
  - [ ] 生成签名密钥对 `cosign generate-key-pair`
  - [ ] 对二进制签名 `cosign sign-blob`
  - [ ] 验证签名 `cosign verify-blob`

## P3：golangci-lint 171 条告警清理（可并行，按 linter 分组）

- [ ] Task 14: 清理 unused (6) — 删除未使用代码
  - [ ] `internal/game/names.go:20` — 删除 `whitespaceRegex`
  - [ ] `internal/handler/lobby.go:42` — 删除 `upgrader` 变量
  - [ ] `internal/store/test_helpers_test.go` — 删除 `testDSN`、`testTimeoutConfig`
  - [ ] 其他未使用项

- [ ] Task 15: 清理 gofmt (3) — 格式化
  - [ ] `cmd/seed/main.go`、`cmd/server/main.go`、`internal/auth/magiclink.go`
  - [ ] 执行 `gofmt -w` 修复

- [ ] Task 16: 清理 gocritic (6) — 模式修复
  - [ ] `cmd/migrate-passwords/main.go:47` — exitAfterDefer
  - [ ] `cmd/server/main.go:74` — exitAfterDefer
  - [ ] `cmd/server/main.go:173` — badCall (filepath.Join 单参数)
  - [ ] `internal/game/physics.go:244` — elseif 模式
  - [ ] `internal/handler/auth.go:73` — ifElseChain 改 switch
  - [ ] `internal/middleware/tracing_test.go:123` — 循环替换为 append

- [ ] Task 17: 清理 staticcheck (7) — 弃用 API 与模式
  - [ ] `cmd/server/main.go:302` — SA1019 chiMiddleware.RealIP 弃用
  - [ ] `internal/crypto/aes.go:94` — S1017 TrimPrefix
  - [ ] `internal/domain/room_code.go:19` — QF1001 德摩根定律
  - [ ] `internal/game/restart.go:149` — ST1011 单位后缀
  - [ ] `internal/handler/lobby.go:103` — QF1001 德摩根定律
  - [ ] `internal/rbac/rbac_test.go:186` — SA1029 空匿名结构体键

- [ ] Task 18: 清理 gosec (23) — 安全告警
  - [ ] G115 整数溢出（10+ 处）：添加范围检查或使用安全转换函数
  - [ ] G101 测试硬编码密钥（2 处）：添加 `//nolint:gosec` 注释（测试代码）
  - [ ] G304 文件包含（2 处）：验证路径来源
  - [ ] G404 弱随机数（1 处）：`retry.go` 改用 crypto/rand 或标注为非安全用途
  - [ ] G602 切片越界（2 处）：添加边界检查

- [ ] Task 19: 清理 revive (50) — 注释与命名规范
  - [ ] `internal/config/constants.go` — 补全所有导出常量注释（格式 `// ConstName ...`）
  - [ ] `internal/domain/domain.go` — 补全类型注释、修复 `ResendApiKey` → `ResendAPIKey`
  - [ ] `internal/domain/events.go` — `DomainEvent` 重命名为 `Event` 或添加 `//nolint:revive`
  - [ ] `internal/protocol/constants.go` — 补全导出常量注释
  - [ ] `internal/slogctx/slogctx.go`、`internal/validate/nickname.go` — 补全包注释
  - [ ] `internal/config/timeout.go` — 补全 `TimeoutConfig` 注释

- [x] Task 20: 清理 funlen (18) — 拆分超长函数
  - [x] `internal/auth/refresh_test.go` — TestRefreshTokenManager_Integration (80 行)
  - [x] `internal/game/restart.go:91` — RestartAndStart (63 行)
  - [x] `internal/game/state_test.go:155` — TestSerializeDeserialize_RoundTrip (71 行)
  - [x] `internal/handler/admin.go:55` — Login (46 语句)
  - [x] `internal/handler/auth.go:175` — RefreshToken (62 行)
  - [ ] `internal/handler/lobby.go:328` — readPump (46 语句)
  - [x] `internal/middleware/cors_test.go:11` — TestCORS (81 行)
  - [x] `internal/middleware/security_test.go:12` — TestSecurityHeaders (96 行)
  - [x] `internal/outbox/publisher_test.go:28` — setupTestEnv (66 行)
  - [x] `internal/protocol/encode.go:29` — EncodeSnapshot (70 语句)
  - [x] `internal/store/postgres.go:579` — LoadAllActiveLobbies (102 行)
  - [x] 其他超长函数（cmd/migrate-passwords、cmd/seed、cmd/server、apierror_test、audit_test、magiclink、quickplay）

- [ ] Task 21: 清理 errcheck (50) — 补全错误检查
  - [ ] 逐文件检查 `golangci-lint run --enable-all --disable=... ./... | grep errcheck`
  - [ ] 对每个未检查的 error 返回值添加 `_ =` 或正确处理
  - [ ] 测试文件中的 errcheck 可通过 linter 配置排除

- [ ] Task 22: 清理 goconst (5) — 提取常量
  - [ ] `internal/auth/magiclink_test.go:144` — `"TestPlayer"` → 常量
  - [ ] `internal/auth/refresh_test.go:100` — `"user-123"` → 常量
  - [ ] `internal/game/player_test.go:46,189` — `"helloworld"`、`"hello"` → 常量
  - [ ] `internal/handler/admin.go:403` — `"admin"` → 常量

- [x] Task 23: 清理 gocognit (3) — 降低认知复杂度
  - [x] `internal/game/hub.go:279` — cleanupOnce (38 > 30)
  - [x] `tests/integration/postgres_test.go:27` — TestPostgresStore_Integration (32 > 30)
  - [x] `tests/integration/redis_test.go:15` — TestRedisStore_Integration (50 > 30)

- [ ] Task 24: 验证 golangci-lint 零告警
  - [ ] 执行 `cd backend && golangci-lint run ./...`
  - [ ] 确认退出码 0，零告警

## P4：前端工具链完善（独立于 P0-P3）

- [ ] Task 25: 安装前端 eslint 与 typescript-eslint
  - [ ] 执行 `cd frontend && npm install -D eslint @eslint/js typescript-eslint`
  - [ ] 更新 `frontend/.eslintrc.json` 使用 TypeScript parser
  - [ ] 验证 `cd frontend && npm run lint` 零错误

- [ ] Task 26: 将 CI lint 步骤改为阻塞
  - [ ] `.github/workflows/ci-cd.yml` 的 `Lint frontend` 步骤 `continue-on-error: true` → `false`
  - [ ] 验证 CI lint 失败时阻塞流水线

## P5：最终验证门控（依赖 P0-P4 全部完成）

- [ ] Task 27: 更新 enterprise-audit-v2-remediation checklist.md
  - [ ] 勾选所有"待运行时验证项"
  - [ ] 勾选所有安全扫描项
  - [ ] 勾选所有代码质量指标项

- [ ] Task 28: 全量回归验证
  - [ ] `cd backend && go build ./...` — 零错误
  - [ ] `cd backend && go vet ./...` — 零警告
  - [ ] `cd backend && go test -count=1 -short ./...` — 零失败
  - [ ] `cd backend && golangci-lint run ./...` — 零告警
  - [ ] `cd frontend && npm run build` — 零错误
  - [ ] `cd frontend && npm run lint` — 零错误

# Task Dependencies
- Task 5, 6, 7, 8 依赖 Task 1, 2, 3, 4（环境就绪）
- Task 9 独立（工具安装）
- Task 10, 11, 12, 13 依赖 Task 9（工具就绪）
- Task 14-23 可并行（按 linter 分组，互不依赖）
- Task 24 依赖 Task 14-23 全部完成
- Task 25, 26 独立于 P0-P3
- Task 27, 28 依赖所有前置任务完成
