# 提升测试覆盖率与质量 Spec

## Why

当前后端测试存在三类严重问题：(1) `internal/handler/auth.go` 缺失 `context`/`domain` 包导入，导致 handler 包与 `cmd/server` 构建失败，测试无法运行；(2) 大量关键模块（domain 校验、service 层、validate、auth/revoke、worker/game_result_worker）零测试覆盖；(3) 现有测试多为"快乐路径"验证，缺乏对抗性用例（边界条件、恶意输入、并发竞态）。本 Spec 旨在将整体覆盖率提升至 >85%，每文件 >60%，重要文件 >90%，并通过 TDD 方法发现并修复真实缺陷。

## What Changes

### 修复阻塞性构建错误
- 修复 `internal/handler/auth.go` 缺失的 `context` 与 `domain` 包导入
- 验证 `go build ./...` 与 `go test ./...` 全量通过

### 补齐零覆盖模块的单元测试
- `internal/domain/room_code.go` — 校验逻辑（长度/字符集/边界）
- `internal/domain/nickname.go` — 昵称清理逻辑
- `internal/domain/domain.go` — 领域模型方法
- `internal/domain/events.go` — 事件构造
- `internal/validate/nickname.go` — XSS 防护与字符过滤（重要文件，>90%）
- `internal/auth/revoke.go` — 令牌撤销逻辑（重要文件，>90%）
- `internal/handler/degradation.go` — 降级响应
- `internal/worker/game_result_worker.go` — 批处理 worker（重要文件，>90%）
- `internal/service/*.go` — 5 个 service 文件（auth_service、admin_service、lobby_service、command_service、query_service）
- `internal/game/repository.go` — 仓储接口
- `internal/idgen/uuid.go` — UUID 生成
- `internal/slogctx/slogctx.go` — 日志上下文
- `internal/metrics/metrics.go` — 指标定义
- `internal/telemetry/telemetry.go` — 遥测初始化

### 增强现有测试的对抗性
- 对安全关键模块（auth/*、middleware/security、middleware/cors、middleware/ratelimit、middleware/idempotency、crypto/aes）补充边界与恶意输入用例
- 对游戏逻辑（game/physics、game/state、game/room、game/hub）补充并发与边界用例
- 对协议编解码（protocol/encode、protocol/decode）补充畸形输入与 fuzz 用例
- 对存储层（store/postgres、store/redis）补充错误路径与并发用例

### 强化集成测试
- 扩展 `tests/integration/postgres_test.go`：覆盖事务回滚、并发写入、软删除、审计日志、outbox 事件
- 扩展 `tests/integration/redis_test.go`：覆盖 TTL 过期、原子操作、连接断开恢复
- 新增 handler 集成测试：覆盖完整认证流程（magic link → JWT → refresh → revoke）

### 修复测试中发现的真实缺陷
- 修复构建错误（auth.go 导入）
- 修复测试过程中发现的任何真实 bug（在 tasks.md 中记录）

## Impact

- Affected specs: `resolve-remaining-verification-debt`（关闭其测试相关验证项）
- Affected code:
  - `backend/internal/handler/auth.go` — 修复导入（**BREAKING**：当前无法构建）
  - `backend/internal/domain/*_test.go` — 新增
  - `backend/internal/validate/nickname_test.go` — 新增
  - `backend/internal/auth/revoke_test.go` — 新增
  - `backend/internal/handler/degradation_test.go` — 新增
  - `backend/internal/worker/game_result_worker_test.go` — 新增
  - `backend/internal/service/*_test.go` — 新增
  - `backend/internal/game/repository_test.go` — 新增
  - `backend/internal/idgen/uuid_test.go` — 新增
  - `backend/internal/slogctx/slogctx_test.go` — 新增
  - 现有 `*_test.go` — 增强对抗性用例
  - `backend/tests/integration/*_test.go` — 扩展
  - 覆盖率配置：可能新增 `.coverprofile` 或 Makefile 目标

## ADDED Requirements

### Requirement: 构建零错误
系统 SHALL 通过 `go build ./...` 且零错误，所有包可独立编译。

#### Scenario: 全量构建
- **WHEN** 执行 `cd backend && go build ./...`
- **THEN** 退出码 0，零错误输出

### Requirement: 整体测试覆盖率 >85%
系统 SHALL 通过 `go test -coverprofile=coverage.out ./...` 且整体覆盖率 >85%。

#### Scenario: 覆盖率达标
- **WHEN** 执行 `go tool cover -func=coverage.out | tail -1`
- **THEN** total 覆盖率 >85%

### Requirement: 每文件测试覆盖率 >60%
每个非 main 包源文件 SHALL 具有对应的测试文件，且单文件覆盖率 >60%。

#### Scenario: 单文件覆盖率检查
- **WHEN** 执行 `go tool cover -func=coverage.out`
- **THEN** 每个非 main 包文件覆盖率 >60%

### Requirement: 重要文件测试覆盖率 >90%
安全关键与核心业务逻辑文件 SHALL 具有覆盖率 >90%的测试，包括：
- `internal/auth/*.go`（jwt、magiclink、middleware、quickplay、refresh、revoke、secure）
- `internal/crypto/aes.go`
- `internal/validate/nickname.go`
- `internal/middleware/cors.go`、`security.go`、`ratelimit.go`、`idempotency.go`
- `internal/protocol/encode.go`、`decode.go`
- `internal/game/physics.go`、`state.go`
- `internal/handler/auth.go`、`admin.go`、`admin_password.go`
- `internal/worker/game_result_worker.go`

#### Scenario: 重要文件覆盖率检查
- **WHEN** 检查重要文件覆盖率
- **THEN** 每个重要文件覆盖率 >90%

### Requirement: 对抗性测试设计
测试 SHALL 遵循 TDD 原则，覆盖以下场景：
- 常见用例（happy path）
- 边界条件（空值、最大长度、最小值、溢出）
- 恶意输入（XSS、SQL 注入、路径遍历、超长字符串、Unicode 空字符）
- 并发竞态（`-race` 标志下运行）
- 错误路径（依赖失败、网络超时、资源耗尽）

#### Scenario: 测试发现真实问题
- **WHEN** 运行全量测试套件
- **THEN** 测试应能暴露至少 1 个此前未知的真实缺陷（并在 tasks.md 中记录修复）

### Requirement: 测试有效性
测试 SHALL NOT 为无用、恒真或重复的测试。每个测试用例 SHALL：
- 验证具体行为或不变量
- 使用有意义的断言（禁止 `_ = err`、禁止空 `t.Run`）
- 避免复制粘贴式重复（使用表驱动测试）

#### Scenario: 测试质量审查
- **WHEN** 审查测试代码
- **THEN** 无恒真断言、无重复测试、每个测试有明确目的

### Requirement: 集成测试覆盖真实依赖
集成测试 SHALL 使用 testcontainers 验证真实数据库行为，覆盖：
- 事务回滚与提交
- 并发写入冲突
- 连接池耗尽
- TTL 过期
- 外键约束

#### Scenario: 集成测试覆盖率
- **WHEN** 执行 `go test -count=1 ./tests/integration/...`
- **THEN** 所有集成测试通过，覆盖上述场景

## MODIFIED Requirements

### Requirement: 测试套件全量通过
所有单元测试与集成测试 SHALL 在 `go test -race -count=1 ./...` 下全量通过，零失败。

## REMOVED Requirements

### Requirement: handler 包构建失败容忍
**Reason**: 当前 `internal/handler/auth.go` 缺失导入导致构建失败，不可容忍
**Migration**: 修复导入，恢复构建
