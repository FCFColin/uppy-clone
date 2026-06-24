# Checklist

## P0：构建修复验证
- [x] `internal/handler/auth.go` 已添加 `context` 与 `domain` 包导入
- [x] `go build ./...` 退出码 0，零错误
- [x] `go vet ./...` 零警告
- [x] `go test -short -count=1 ./...` 无构建失败

## P1：零覆盖模块测试验证
- [ ] `internal/domain/room_code_test.go` 存在且覆盖率 >90%
- [ ] `internal/domain/nickname_test.go` 存在且覆盖率 >90%
- [ ] `internal/domain/domain_test.go` 存在且覆盖率 >60%
- [ ] `internal/domain/events_test.go` 存在且覆盖率 >60%
- [ ] `internal/validate/nickname_test.go` 存在且覆盖率 >90%
- [ ] `internal/auth/revoke_test.go` 存在且覆盖率 >90%
- [ ] `internal/handler/degradation_test.go` 存在且覆盖率 >90%
- [ ] `internal/worker/game_result_worker_test.go` 存在且覆盖率 >90%
- [ ] `internal/service/auth_service_test.go` 存在且覆盖率 >90%
- [ ] `internal/service/admin_service_test.go` 存在且覆盖率 >60%
- [ ] `internal/service/lobby_service_test.go` 存在且覆盖率 >60%
- [ ] `internal/service/command_service_test.go` 存在且覆盖率 >60%
- [ ] `internal/service/query_service_test.go` 存在且覆盖率 >60%
- [ ] `internal/game/repository_test.go` 存在且覆盖率 >60%
- [ ] `internal/idgen/uuid_test.go` 存在且覆盖率 >90%
- [ ] `internal/slogctx/slogctx_test.go` 存在且覆盖率 >60%
- [ ] `internal/metrics/metrics_test.go` 存在且覆盖率 >60%
- [ ] `internal/telemetry/telemetry_test.go` 存在且覆盖率 >60%

## P2：对抗性测试验证
- [ ] `internal/auth/jwt_test.go` 含 alg=none 攻击、HS256/RS256 混淆、空 subject、超长 subject、token 重放用例
- [ ] `internal/auth/magiclink_test.go` 含过期、重放、篡改、跨用户用例
- [ ] `internal/auth/refresh_test.go` 含轮换失效、并发竞态、已撤销、跨用户用例
- [ ] `internal/auth/secure_test.go` 含强度边界、弱密码、超长密码用例
- [ ] `internal/crypto/aes_test.go` 含空明文、超长明文、错误密钥、篡改密文用例
- [ ] `internal/middleware/cors_test.go` 含 null origin、子域名绕过、CRLF 注入用例
- [ ] `internal/middleware/security_test.go` 含头注入、缺失头用例
- [ ] `internal/middleware/ratelimit_test.go` 含并发竞态、IP 伪造、窗口边界用例
- [ ] `internal/middleware/idempotency_test.go` 含并发 key 竞态、过期重放、跨用户用例
- [ ] `internal/protocol/encode_decode_test.go` 含畸形输入、超大消息、未知类型用例
- [ ] `internal/game/physics_test.go` 含 NaN/Inf、超速、并发、零/负质量用例
- [ ] `internal/game/state_test.go` 含非法状态转移、并发更新、超大状态用例
- [ ] `internal/game/room_test.go` 与 `hub_test.go` 含满员边界、并发加入/离开、重复加入用例
- [ ] `internal/handler/admin_password_test.go` 含非 bcrypt 拒绝、时序攻击、空/超长密码用例
- [ ] `internal/handler/auth_test.go` 含畸形 JSON、超大请求体、SQL 注入用例
- [ ] `internal/handler/lobby_test.go` 与 `websocket_test.go` 含非法 Origin、超大帧、并发连接用例

## P3：集成测试验证
- [ ] `tests/integration/postgres_test.go` 含事务回滚、并发写入、软删除、审计日志、outbox、外键约束用例
- [ ] `tests/integration/redis_test.go` 含 TTL 过期、原子操作、并发 rate limit、删除后不可用用例
- [ ] handler 集成测试覆盖完整认证流程（magic link → JWT → refresh → revoke）
- [ ] handler 集成测试覆盖 admin 流程（登录 → 改密 → 重登）
- [ ] handler 集成测试覆盖 WebSocket 流程（创建 → 加入 → 游戏）

## P4：覆盖率与质量验证
- [ ] 整体覆盖率 >85%（`go tool cover -func=coverage.out | tail -1`）
- [ ] 每文件覆盖率 >60%（`go tool cover -func=coverage.out` 逐行检查）
- [ ] 重要文件覆盖率 >90%（auth/*、crypto/aes、validate/nickname、middleware/cors|security|ratelimit|idempotency、protocol/*、game/physics|state、handler/auth|admin|admin_password、worker/game_result_worker）
- [ ] 测试中发现的真实缺陷已记录并修复
- [ ] 无恒真断言（grep `_ = err` 在测试中、空 `t.Run`）
- [ ] 无重复测试（复制粘贴式）
- [ ] 每个测试有明确目的
- [ ] `go test -race -count=1 ./...` 零失败
- [ ] `go test -race -count=1 -short ./...` 零失败
- [ ] `go build ./...` 零错误
- [ ] `go vet ./...` 零警告
