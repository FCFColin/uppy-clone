# Checklist — 解决剩余验证债务与技术债清理

## P0：环境搭建与迁移冲突修复

### Task 1: 修复数据库迁移版本号冲突
- [ ] `backend/migrations/000009_create_db_roles.up.sql` 存在（从 000008 重编号）
- [ ] `backend/migrations/000009_create_db_roles.down.sql` 存在
- [ ] `backend/migrations/` 下无重复版本号（`ls | sort | uniq -d` 无输出）
- [ ] 验证命令：`golang-migrate -path backend/migrations -database "$DATABASE_URL" up` 无 duplicate version 错误

### Task 2: 创建开发环境 .env 文件
- [ ] `.env` 文件存在（gitignored）
- [ ] `JWT_SECRET` 为 64 字符 hex（`openssl rand -hex 32` 生成）
- [ ] `ENCRYPTION_KEY` 为 64 字符 hex
- [ ] `ADMIN_PASSWORD` 非空且非默认值
- [ ] `DATABASE_URL` 指向 docker-compose postgres
- [ ] `REDIS_URL` 指向 docker-compose redis
- [ ] 验证命令：`git status .env` 不出现在追踪列表中

### Task 3: 修复 docker-compose.yml
- [ ] `docker-compose.yml` 含 `ALLOWED_ORIGINS` 环境变量
- [ ] `docker-compose.yml` 含 `EMAIL_FROM` 环境变量
- [ ] `docker-compose.yml` 含 `migrate` 初始化容器
- [ ] `app` 容器 `depends_on: migrate`（service_completed_successfully）
- [ ] 验证命令：`docker-compose config` 无错误

### Task 4: 启动 Docker 环境并验证健康
- [ ] `docker-compose up -d postgres redis` 成功
- [ ] postgres 容器 healthcheck 为 healthy
- [ ] redis 容器 healthcheck 为 healthy
- [ ] `docker-compose up -d` 全部服务启动
- [ ] 验证命令：`docker-compose ps` 全部 Up

## P1：运行时验证

### Task 5: 数据库迁移正向执行
- [ ] `make migrate` 或等价命令执行成功
- [ ] `users` 表存在
- [ ] `game_sessions` 表存在
- [ ] `game_results` 表存在
- [ ] `lobbies` 表存在
- [ ] `audit_logs` 表存在（含 prev_hash、this_hash 字段）
- [ ] `outbox_events` 表存在
- [ ] `app_user` 角色存在
- [ ] `migrator` 角色存在
- [ ] 验证命令：`psql -c "\dt"` 列出所有表

### Task 6: 数据库迁移反向执行
- [ ] `golang-migrate ... down` 全部回退成功
- [ ] 所有表删除
- [ ] 重新 `up` 恢复成功
- [ ] 验证命令：`psql -c "\dt"` 无表

### Task 7: 执行 make seed
- [ ] `make seed` 执行成功
- [ ] `users` 表含测试数据（≥10 行）
- [ ] `game_sessions` 表含测试数据
- [ ] `game_results` 表含测试数据
- [ ] 验证命令：`psql -c "SELECT COUNT(*) FROM users"` 返回 ≥10

### Task 8: 端点验证
- [ ] `curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health` — 返回 200
- [ ] `curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/ready` — 返回 200
- [ ] `curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/metrics` — 返回 401
- [ ] `curl -s -o /dev/null -w "%{http_code}" -u $METRICS_USER:$METRICS_PASSWORD http://localhost:8080/metrics` — 返回 200
- [ ] `curl -s -o /dev/null -w "%{http_code}" http://localhost:6060/debug/pprof/` — 返回 200（ENABLE_PPROF=true 时）

## P2：安全扫描

### Task 9: 安装安全扫描工具
- [ ] `gitleaks` 可执行（`gitleaks version` 有输出）
- [ ] `cosign` 可执行（`cosign version` 有输出）
- [ ] `trivy` 通过 Docker 可执行（`docker run --rm aquasec/trivy --version` 有输出）

### Task 10: govulncheck 扫描
- [ ] `cd backend && govulncheck ./...` 执行完成
- [ ] 输出零 CRITICAL/HIGH CVE
- [ ] 若有漏洞：记录 CVE 编号、修复依赖版本

### Task 11: gitleaks 密钥泄漏扫描
- [ ] `gitleaks detect --source . --report-path leaks.json` 执行完成
- [ ] `leaks.json` 为空或不存在（零泄漏）
- [ ] 若有泄漏：从代码中移除、轮换密钥

### Task 12: trivy 扫描
- [ ] `docker run --rm -v ${PWD}:/repo aquasec/trivy fs /repo` 执行完成
- [ ] 文件系统扫描零 CRITICAL CVE
- [ ] `docker run --rm aquasec/trivy image <app-image>` 执行完成
- [ ] 镜像扫描零 CRITICAL CVE

### Task 13: cosign 签名验证
- [ ] `cosign generate-key-pair` 生成密钥对
- [ ] `cosign sign-blob` 对二进制签名成功
- [ ] `cosign verify-blob` 验证签名通过

## P3：golangci-lint 告警清理

### Task 14: unused (6)
- [ ] `internal/game/names.go` — `whitespaceRegex` 已删除
- [ ] `internal/handler/lobby.go` — `upgrader` 变量已删除
- [ ] `internal/store/test_helpers_test.go` — `testDSN`、`testTimeoutConfig` 已删除
- [ ] 其他 unused 项已清理
- [ ] 验证：`golangci-lint run --disable-all --enable=unused ./...` 零告警

### Task 15: gofmt (3)
- [ ] `cmd/seed/main.go` 已格式化
- [ ] `cmd/server/main.go` 已格式化
- [ ] `internal/auth/magiclink.go` 已格式化
- [ ] 验证：`golangci-lint run --disable-all --enable=gofmt ./...` 零告警

### Task 16: gocritic (6)
- [ ] `cmd/migrate-passwords/main.go:47` — exitAfterDefer 已修复
- [ ] `cmd/server/main.go:74` — exitAfterDefer 已修复
- [ ] `cmd/server/main.go:173` — badCall 已修复（filepath.Join 单参数）
- [ ] `internal/game/physics.go:244` — elseif 已改为 else if
- [ ] `internal/handler/auth.go:73` — ifElseChain 已改为 switch
- [ ] `internal/middleware/tracing_test.go:123` — 循环已改为 append
- [ ] 验证：`golangci-lint run --disable-all --enable=gocritic ./...` 零告警

### Task 17: staticcheck (7)
- [ ] `cmd/server/main.go:302` — SA1019 RealIP 已替换或标注
- [ ] `internal/crypto/aes.go:94` — S1017 已改为 TrimPrefix
- [ ] `internal/domain/room_code.go:19` — QF1001 已应用德摩根定律
- [ ] `internal/game/restart.go:149` — ST1011 已移除单位后缀
- [ ] `internal/handler/lobby.go:103` — QF1001 已应用德摩根定律
- [ ] `internal/rbac/rbac_test.go:186` — SA1029 已使用自定义类型键
- [ ] 验证：`golangci-lint run --disable-all --enable=staticcheck ./...` 零告警

### Task 18: gosec (23)
- [ ] G115 整数溢出全部修复（添加范围检查或安全转换）
- [ ] G101 测试硬编码密钥已标注 `//nolint:gosec`（测试代码）
- [ ] G304 文件包含已验证路径来源
- [ ] G404 弱随机数已改用 crypto/rand 或标注非安全用途
- [ ] G602 切片越界已添加边界检查
- [ ] 验证：`golangci-lint run --disable-all --enable=gosec ./...` 零告警

### Task 19: revive (50)
- [ ] `internal/config/constants.go` — 所有导出常量注释格式正确
- [ ] `internal/domain/domain.go` — 类型注释补全、`ResendApiKey` → `ResendAPIKey`
- [ ] `internal/domain/events.go` — `DomainEvent` 重命名或标注 nolint
- [ ] `internal/protocol/constants.go` — 导出常量注释补全
- [ ] `internal/slogctx/slogctx.go` — 包注释补全
- [ ] `internal/validate/nickname.go` — 包注释补全
- [ ] `internal/config/timeout.go` — `TimeoutConfig` 注释补全
- [ ] 验证：`golangci-lint run --disable-all --enable=revive ./...` 零告警

### Task 20: funlen (18)
- [x] 所有超长函数已拆分至 ≤60 行（除 `internal/handler/lobby.go:328` readPump 待确认）
- [ ] 验证：`golangci-lint run --disable-all --enable=funlen ./...` 零告警

### Task 21: errcheck (50)
- [ ] 所有未检查的 error 返回值已处理
- [ ] 验证：`golangci-lint run --disable-all --enable=errcheck ./...` 零告警

### Task 22: goconst (5)
- [ ] 重复字符串已提取为常量
- [ ] 验证：`golangci-lint run --disable-all --enable=goconst ./...` 零告警

### Task 23: gocognit (3)
- [x] `cleanupOnce` 认知复杂度 ≤30
- [x] `TestPostgresStore_Integration` 认知复杂度 ≤30
- [x] `TestRedisStore_Integration` 认知复杂度 ≤30
- [ ] 验证：`golangci-lint run --disable-all --enable=gocognit ./...` 零告警

### Task 24: golangci-lint 全量零告警
- [ ] `cd backend && golangci-lint run ./...` 退出码 0
- [ ] 输出零告警

## P4：前端工具链

### Task 25: 安装 eslint
- [ ] `frontend/package.json` devDependencies 含 `eslint`、`@eslint/js`、`typescript-eslint`
- [ ] `frontend/.eslintrc.json` 使用 TypeScript parser
- [ ] `cd frontend && npm run lint` 零错误
- [ ] 验证命令：`cd frontend && npm run lint`

### Task 26: CI lint 阻塞
- [ ] `.github/workflows/ci-cd.yml` 的 `Lint frontend` 步骤 `continue-on-error` 为 `false` 或已移除
- [ ] 验证：CI lint 失败时流水线阻塞

## P5：最终验证门控

### Task 27: 更新 enterprise-audit-v2-remediation checklist
- [ ] "Go 后端验证" 全部勾选（含 govulncheck）
- [ ] "安全扫描验证" 全部勾选（gitleaks、trivy、cosign）
- [ ] "前端验证" 全部勾选（lint、test）
- [ ] "集成验证" 全部勾选（docker-compose、make dev、make seed、端点）
- [ ] "数据库迁移验证" 全部勾选（正向/反向）
- [ ] "代码质量指标验证" 全部勾选（funlen、gocyclo、unused、dupl、goconst）

### Task 28: 全量回归验证
- [ ] `cd backend && go build ./...` — 零错误
- [ ] `cd backend && go vet ./...` — 零警告
- [ ] `cd backend && go test -count=1 -short ./...` — 零失败
- [ ] `cd backend && golangci-lint run ./...` — 零告警
- [ ] `cd frontend && npm run build` — 零错误
- [ ] `cd frontend && npm run lint` — 零错误
