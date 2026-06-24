# 解决剩余验证债务与技术债清理 Spec

## Why

前一轮企业级审计（`enterprise-audit-v2-remediation`）完成了 62 项代码修复，但最终验证门控仍有 20+ 项未通过：运行时环境未搭建、安全扫描工具未安装/执行、golangci-lint 171 条预存告警未清理、数据库迁移存在版本号冲突（两个 `000008`）、前端 eslint 未安装。本 Spec 旨在彻底关闭所有剩余验证项，使项目达到"零未验证项"的生产就绪状态。

## What Changes

### 环境与基础设施
- 创建 `.env` 文件（基于 `.env.example`，填入开发环境真实密钥）
- 修复 `docker-compose.yml`：添加 `ALLOWED_ORIGINS`、`EMAIL_FROM`、`OTEL_EXPORTER_OTLP_ENDPOINT` 环境变量；添加 `migrate` 初始化容器
- 修复数据库迁移版本号冲突：将 `000008_create_db_roles` 重编号为 `000009_create_db_roles`
- 验证 `docker-compose up` 启动 PG 16.4 + Redis 7.4 + app 容器

### 运行时验证
- 执行数据库迁移正向（up）与反向（down）全流程
- 执行 `make seed` 插入测试数据
- 验证 `/health`（200）、`/ready`（200）、`/metrics`（401 未认证 / 200 认证后）、pprof（200）

### 安全扫描工具安装与执行
- 安装 `gitleaks`（via `go install`）、`trivy`（via Docker）、`cosign`（via `go install`）
- 执行 `govulncheck ./...` — 零 CRITICAL/HIGH CVE
- 执行 `gitleaks detect` — 零密钥泄漏
- 执行 `trivy fs .`（文件系统扫描）与 `trivy image`（镜像扫描）— 零 CRITICAL CVE
- 执行 `cosign sign-blob`（本地签名验证流程）

### golangci-lint 171 条告警清理
- **errcheck (50)**：补全未检查的 error 返回值（主要在 handler 层与 worker 层）
- **funlen (18)**：拆分超长函数至 ≤60 行
- **gocognit (3)**：降低认知复杂度至 ≤30
- **goconst (5)**：提取重复字符串为常量
- **gocritic (6)**：修复 exitAfterDefer、badCall、elseif 等模式
- **gosec (23)**：修复 G115（整数溢出）、G101（测试硬编码密钥）、G304（文件包含）、G404（弱随机数）、G602（切片越界）
- **revive (50)**：补全导出常量/类型注释、修复命名规范（ResendApiKey → ResendAPIKey）
- **staticcheck (7)**：修复 SA1019（弃用 API）、S1011（循环替换）、S1017（TrimPrefix）、ST1011（单位后缀）、QF1001（德摩根定律）
- **unused (6)**：删除未使用的变量/函数

### 前端工具链
- 安装 `eslint` + `typescript-eslint` 到 `frontend/devDependencies`
- 配置 `frontend/.eslintrc.json` 使用 TypeScript parser
- 验证 `cd frontend && npm run lint` 零错误
- 将 CI lint 步骤的 `continue-on-error` 从 `true` 改为 `false`（阻塞式）

## Impact

- Affected specs: `enterprise-audit-v2-remediation`（关闭其所有未验证项）
- Affected code:
  - `docker-compose.yml` — 环境变量补全
  - `backend/migrations/000008_create_db_roles.*` → `000009_create_db_roles.*` — 重编号
  - `backend/internal/config/constants.go` — 补全导出注释
  - `backend/internal/domain/domain.go` — 补全类型注释、修复命名
  - `backend/internal/game/*.go` — 拆分超长函数、修复 G115 整数溢出
  - `backend/internal/handler/*.go` — 补全 errcheck、拆分超长函数
  - `backend/internal/store/postgres.go` — 拆分 LoadAllActiveLobbies
  - `backend/cmd/server/main.go` — 修复 exitAfterDefer、badCall
  - `backend/internal/resilience/retry.go` — 修复 G404 弱随机数
  - `frontend/package.json` — 添加 eslint 依赖
  - `frontend/.eslintrc.json` — 升级为 TypeScript parser
  - `.github/workflows/ci-cd.yml` — lint 步骤改为阻塞
  - `.env` — 新建（开发环境密钥，gitignored）

## ADDED Requirements

### Requirement: 开发环境一键启动
系统 SHALL 提供 `docker-compose up` 一键启动 PG + Redis + app 的能力，且所有健康检查通过。

#### Scenario: 首次启动
- **WHEN** 开发者执行 `docker-compose up -d`
- **THEN** postgres、redis、app 三个容器均为 healthy 状态
- **AND** `curl http://localhost:8080/health` 返回 200

### Requirement: 安全扫描零 CRITICAL
系统 SHALL 通过所有安全扫描工具（govulncheck、gitleaks、trivy）且零 CRITICAL/HIGH 级别发现。

#### Scenario: govulncheck 扫描
- **WHEN** 执行 `govulncheck ./...`
- **THEN** 输出零 CRITICAL/HIGH CVE

### Requirement: golangci-lint 零告警
系统 SHALL 通过 `golangci-lint run` 且零告警。

#### Scenario: 全量 lint
- **WHEN** 执行 `cd backend && golangci-lint run ./...`
- **THEN** 退出码 0，零告警输出

## MODIFIED Requirements

### Requirement: 数据库迁移版本唯一性
所有迁移文件 MUST 具有唯一的版本号前缀（`000001` ~ `000009`），禁止重复。

## REMOVED Requirements

### Requirement: 000008_create_db_roles 旧版本号
**Reason**: 与 `000008_drop_redundant_indexes` 版本号冲突，导致 `golang-migrate` 报错 `duplicate migration version`
**Migration**: 重编号为 `000009_create_db_roles`
