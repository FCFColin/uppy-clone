# 执行瘦身 v3: zero+low risk + Path C Spec

> **change-id**: `execute-slim-v3-with-pathc`
> **类型**: 实施型 spec（执行代码改动）
> **基线**: 67,132 行（mine-all-slim-opportunities 锁定，2026-07-18）
> **目标**: 减 ~13,917 行（20.74%），达 20% 目标
> **输入**: `mine-all-slim-opportunities/slim-plan.md`（Batch 1-3 已详细 5 元组）+ Path C 测试削减
> **硬约束**: off-limits 不变（13,572 行企业级架构全部保留）

## Why

`mine-all-slim-opportunities` meta-spec 已产出完整的 zero+low risk 发现清单（3,717 行，Batch 1-3 5 元组已就位），并确认 20% 目标的唯一可达路径是 Path C（大规模测试削减）。用户决策：**执行 zero+low risk 路径 + Path C**。

当前状态：
- 测试代码 37,900 行，生产:测试 = 1:1.48（远超业界 1:1 基准）
- Batch 1-3 已有详细 5 元组，可立即执行
- Path C 需先识别具体测试文件，再执行削减

本 spec 整合两条路径，分阶段执行，每阶段后验证测试通过 + 覆盖率不显著下降。

## What Changes

### 阶段一：Batch 1 零风险独立删除（2,426 行，可全部并行）

执行 `slim-plan.md` 章节 3 Batch 1 全部条目，按 5 元组明细操作：

- **A1 测试冗余删除**（12 条, 871 行）：死测试文件、重复测试、无断言测试、测库 API 测试
- **A2 基础设施删除**（6 条, 266 行）：worker.yaml 死基础设施、孤儿 GSA/IAM、Thanos 残留、MCI 注释
- **A3 文档膨胀删除**（16 条, 62 行）：过时 ADR 引用、死文档链接、历史审查残留
- **A4 前端死代码删除**（16 条, 73 行）：死导出、死常量、死 CSS 规则、@deprecated shim
- **A5 后端死代码删除**（27 条, 725 行）：整文件死代码、死接口、死常量、空 doc.go
- **A6 死重复代码删除**（2 条, 210 行）：uuid_test.go 重复、ws_limiter.go 死代码
- **A7 死配置删除**（16 条, 223 行）：go.sum 孤立依赖、死 Makefile target、死 .gitignore 规则

**特征**: 单文件操作、无 import 变更、无签名变更。删除后 `go mod tidy` + `go build` + `go test` + `npm test` 全绿。

### 阶段二：Batch 2 低风险合并/重构（1,050 行，按文件串行）

执行 `slim-plan.md` 章节 3 Batch 2 全部条目，需修改 import 路径或调用方：

- **A1 测试 mock 替换**（6 条, 252 行）：pgxmock.NewPool() → testutil.NewPgxMock、重复 mock struct 提升、property 测试合并
- **A2 配置合并**（7 条, 314 行）：Terraform 死变量删除、redis-ephemeral.yaml 合并、注释压缩
- **A3 文档去重**（12 条, 133 行）：CONTRIBUTING 与 ADR/README 重复段、architecture.md 重复段
- **A4 前端合并**（6 条, 91 行）：toast.ts shim 删除 + mock 路径修改、重复逻辑合并
- **A5 重复包合并**（2 条, 154 行）：domain/idgen.go → idgen/uuid.go（改 import）、domain/apierror.go → apierror/apierror.go
- **A6 单包 boilerplate 替换**（9 条, 73 行）：JSON 响应 helper、nil 检查 helper、重复常量删除
- **A7 配置清理**（9 条, 31 行）：死 env 变量、死 .gitignore 规则、过时 .gitleaksignore

**特征**: 需修改 import 路径或调用方，但无 API 签名变更。

### 阶段三：Batch 3 跨文件模式抽取（241 行，逐条串行）

执行 `slim-plan.md` 章节 3 Batch 3 全部条目，依赖 Batch 1+2 完成：

- **A2-L4**: CI composite action（40 行）— 提取 checkout+setup-go 为 `.github/actions/setup-go-env/action.yml`
- **A6#14**: miniredis.Run helper（82 行）— 提取 `testutil/miniredis.go` 共享 helper
- **A6#15**: mockRows 合并（30 行）— 合并两个 pgx.Rows mock struct
- **A6#19**: json.NewDecoder helper（45 行）— 提取 `testutil/DecodeJSONBody`
- **A6#21**: auth_test setup helper（27 行）— 提取 `newTestAuthHandlerWithRedis`
- **A6#22**: admin_test setup helper（17 行）— 提取 `newAdminHandlerWithRedis`

**特征**: 需创建新 helper，在清理后的代码上抽取模式。每条独立执行，验证后再做下一条。

### 阶段四：Path C 测试削减（~10,200 行，分 4a/4b 两步）

> **用户决策**: 放宽测试覆盖率约束，降至 1:1.10（达 20.74%）

#### 4a: 低风险测试削减（~5,500 行）

识别并执行以下低风险测试削减（来自 slim-plan.md 6.3.3 候选区域）：

- **TC-1 重复测试合并**（~3,000 行）：同逻辑多入口的测试合并为 table-driven
- **TC-4 第三方库 API 测试删除**（~1,200 行）：测 prometheus/otel SDK 而非自身代码的测试（A1-Z09/Z10 已识别部分，扩展到全库）
- **TC-6 smoke test 与 unit test 重叠删除**（~800 行）：smoke 测试仅验证"不 panic"，被 unit test 覆盖
- **TC-7 过时测试删除**（~500 行）：对应代码已删但测试残留

**执行方式**: 先扫描识别具体文件 + 行范围，再执行删除。每删除一批后 `go test -cover` 验证覆盖率下降 < 2pp。

#### 4b: 中风险测试削减（~4,700 行，达 20% 所需）

识别并执行以下中风险测试削减（需更谨慎，逐项验证）：

- **TC-2 过度 mock setup 简化**（~2,500 行）：每个测试重建 fixture 的 mock 简化（与 Batch 3 A6#14 协同，抽 helper 而非删 setup）
- **TC-3 property test 与 example test 重叠删除**（~1,800 行）：property test 已覆盖的 example test 删除
- **TC-5 integration test boilerplate 抽取**（~1,700 行）：重复 setup/teardown 抽 helper（与 A6#21/A6#22 协同）

**执行方式**: 逐项识别 + 执行 + 验证。若某项削减后覆盖率下降 > 2pp，回滚该项并标记为"需用户决策"。

### 阶段五：最终验证

- `go build ./...` 通过
- `go vet ./...` 通过
- `go test ./... -count=1 -race -cover` 通过，覆盖率 ≥ 78%（基线 82.9% - 5pp 缓冲）
- `npm run build` 通过
- `npm test` 通过
- `golangci-lint run ./...` 0 issues
- 总行数测量 ≤ 53,400 行（67,132 - 13,400 = 53,732，留 332 行缓冲）

## Impact

### 受影响代码

- **backend/internal/**: 死代码删除、重复包合并、helper 抽取、测试削减
- **frontend/src/**: 死代码删除、shim 删除、重复逻辑合并
- **infra/**: 死基础设施删除、配置合并、注释压缩
- **docs/**: 过时引用删除、重复段去重
- **配置文件**: go.sum/go.mod/package.json/.gitignore/.env 清理
- **.github/workflows/**: composite action 提取

### 受影响既有 spec

- `mine-all-slim-opportunities`: 其 slim-plan.md 被本 spec 消费执行，不修改该 spec 文档
- `deep-arch-slim-v2` / `slim-tier1-ef-and-materialize`: 已完成，无影响

### 不可逆决策

- **BREAKING**: Path C 测试削减删除大量测试代码，覆盖率从 82.9% 降至 ~78%。若后续需高覆盖率，需重新编写测试。
- off-limits 企业级架构（13,572 行）全部保留，不触及。

## ADDED Requirements

### Requirement: 分阶段执行 + 每阶段验证

系统 SHALL 按阶段一→二→三→四→五顺序执行，每阶段完成后必须通过验证协议（go build + go test + npm test + lint + 覆盖率检查）方可进入下一阶段。

#### Scenario: 阶段验证通过
- **WHEN** 阶段 N 完成
- **THEN** `go build ./...` + `go test ./... -count=1` + `npm test` + `golangci-lint run ./...` 全绿，且覆盖率下降 < 阶段阈值
- **AND** 可进入阶段 N+1

#### Scenario: 阶段验证失败
- **WHEN** 阶段 N 验证失败（测试失败或覆盖率下降超阈值）
- **THEN** 回滚该阶段所有改动，分析根因，调整后重试

### Requirement: Path C 覆盖率守护

系统 SHALL 在 Path C 执行过程中持续监控覆盖率，确保最终覆盖率 ≥ 78%。

#### Scenario: 单项削减后覆盖率下降过大
- **WHEN** 删除某测试文件后 `go test -cover` 显示覆盖率下降 > 2pp
- **THEN** 回滚该删除，标记为"需用户决策"，继续下一项

#### Scenario: Path C 完成后覆盖率达标
- **WHEN** Path C 全部完成
- **THEN** 覆盖率 ≥ 78%（基线 82.9% - 5pp 缓冲）

### Requirement: off-limits 边界不可违反

系统 SHALL 在整个执行过程中不修改 off-limits 文件（13,572 行企业级架构）。

#### Scenario: 误删 off-limits 文件
- **WHEN** 执行某条目时发现目标文件在 off-limits 清单中
- **THEN** 跳过该条目，记录到"未执行条目登记"

## MODIFIED Requirements

### Requirement: 测试覆盖率约束

**修改前**（off-limits）: 测试覆盖率受 off-limits 保护，不可削减。

**修改后**（本 spec）: 用户决策放宽测试覆盖率约束至 ≥ 78%（基线 82.9% - 5pp）。允许 Path C 削减测试代码 ~10,200 行以达 20% 总减幅目标。

## REMOVED Requirements

### Requirement: 测试代码不可削减

**Reason**: 用户决策执行 Path C，放宽测试覆盖率约束以达 20% 减幅目标。

**Migration**: 覆盖率从 82.9% 降至 ~78%（-5pp），仍在合理范围。被删测试主要是重复测试、库 API 测试、过时测试、smoke 冗余——非核心业务逻辑测试。
