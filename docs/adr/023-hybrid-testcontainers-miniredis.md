# ADR-023: 混合测试策略 — testcontainers + miniredis

## 状态: 已接受（部分落地）

## 上下文

项目需要高测试覆盖率（章程目标 7：工程基线），同时测试类型多样：
- 单元测试：纯逻辑（物理引擎、协议编解码、JWT 签发）
- 集成测试：真实 SQL 查询、Redis Stream 消费、auth 全流程
- 对抗测试：并发 Hub 访问、WS flood、refresh token 旋转

测试基础设施需要在 **真实性** 和 **速度/可移植性** 之间平衡。CI 环境有 Docker（testcontainers），本地开发可能无 Docker。

## 决策

采用 **分层混合策略**：

| 测试类型 | 基础设施 | 示例 |
|----------|----------|------|
| 纯单元 | 无外部依赖 / mock | `physics_test.go`, `protocol/encode_decode_test.go` |
| Redis 逻辑 | miniredis（进程内） | `auth_integration_test.go:94-95`, `hub_restore_integration_test.go:27-28` |
| PostgreSQL 集成 | testcontainers `postgres:16-alpine` | `store/postgres_integration_test.go`, `auth_integration_test.go:113-124` |
| Redis 集成 | testcontainers redis module | `outbox/publisher_test.go:46-58` |
| CockroachDB | testcontainers generic | `tests/integration/cockroach_test.go:89-96` |
| 跳过策略 | `t.Skipf` 当 Docker 不可用 | `auth_integration_test.go:124` |

Go 开发工具通过 [`backend/tools.go`](../../backend/tools.go) 与主 `go.mod` pin（air、golangci-lint、govulncheck 等），`Makefile` 在 `backend/bin/` 构建可执行文件。

## 后果

**正面**
- 集成测试使用真实 PG/Redis 行为，捕获方言差异
- miniredis 测试快速（毫秒级），适合 TDD 循环
- Docker 不可用时优雅跳过，不阻塞 `go test ./...`
- `internal/` 达到约 1:1 测试文件比

**负面**
- testcontainers 测试慢（秒级启动），CI 时间长
- 部分 refresh 对抗测试仍依赖本地 `localhost:6379`（`refresh_test.go:187-188`）
- 12+ 测试文件使用 `time.Sleep`，存在 flaky 风险
- 前端仅 2 个 vitest 文件，测试策略未覆盖 FE
- GDPR worker 测试极薄（`gdpr_hard_delete_worker_test.go:9-17`）

**放弃的替代方案**
- 全 mock（sqlmock + miniredis）：无法捕获 PG 方言/索引问题
- 全 testcontainers：本地无 Docker 时完全无法测试
- 共享 dev 数据库：测试隔离差，并行 CI 冲突
