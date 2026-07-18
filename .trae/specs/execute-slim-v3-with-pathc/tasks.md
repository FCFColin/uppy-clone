# Tasks

> **输入**: `mine-all-slim-opportunities/slim-plan.md` 章节 3-4（Batch 1-3 详细 5 元组）+ 章节 6.3（Path C 候选区域）
> **执行原则**: 每阶段完成后必须通过验证协议方可进入下一阶段。

 param($m) $m.Groups[1].Value -replace '- \[ \]', '- [x]' : Batch 2 低风险合并/重构 (1,050 行, 按文件串行)

- [ ] Task 2.1: A1 测试 mock 替换 (6 条, 252 行)
  - [ ] A1-L01: postgres_test.go 3 处 pgxmock.NewPool() -> testutil.NewPgxMock(t)
  - [ ] A1-L02: game_helpers_test.go mockRoomRepository 提升到 testutil
  - [ ] A1-L03: physics_test.go TestUpdateWind_ClampAndEdgeZone 删除
  - [ ] A1-L04: message_codec.test.ts property 覆盖的 example 删除
  - [ ] A1-L05: snapshot_decode.test.ts 整文件删除
  - [ ] A1-L06: encode_decode_test.go 4 RoundTrip 合并为 table-driven

- [ ] Task 2.2: A2 配置合并 (7 条, 314 行)
  - [ ] A2-L1: variables.tf db_password 删除 (DEPRECATED)
  - [ ] A2-L2/L3: main.tf worker/migrator GSA + MCI 资源删除 (83 行)
  - [ ] A2-L5: redis-ephemeral.yaml 合并到 redis.yaml (78->33 行)
  - [ ] A2-L6/L7: grafana datasource + alertmanager config 合并
  - [ ] A2-L8: infra/ 多文件注释压缩 (200->41 行)

- [ ] Task 2.3: A3 文档去重 (12 条, 133 行)
  - [ ] A3-L1~L7: CONTRIBUTING.md 7 段去重 (130->100 行)
  - [ ] A3-L8~L10: architecture.md 3 段去重 (189->155 行)
  - [ ] A3-L11~L12: threat-model.md 2 段去重 (94->75 行)

- [ ] Task 2.4: A4 前端合并 (6 条, 91 行)
  - [ ] A4-Z4: toast.ts shim 删除, lifecycle.test.ts mock 路径改 ./utils.js
  - [ ] A4-L1: entry_flow.ts getWaitingTitleTextImpl 删除
  - [ ] A4-L2: index_leaderboard.ts vs leaderboard.ts 重复逻辑合并 (25 行)
  - [ ] A4-L3~L5: 重复测试/mock 合并 (42 行)

- [ ] Task 2.5: A5 重复包合并 (2 条, 154 行)
  - [ ] A5-003: 删除 domain/idgen.go, game/room_restart_voting.go:159 改 domain.UUID -> idgen.UUID
  - [ ] A5-004: 删除 domain/apierror.go, 3 文件改 domain.ProblemDetails -> apierror.ProblemDetails

- [ ] Task 2.6: A6 单包 boilerplate 替换 (9 条, 73 行)
  - [ ] A6#8~A6#13: handler/ 多文件 JSON 响应 + nil 检查 helper (auth/stats/lobby_registry/admin_management/admin)
  - [ ] A6#16: admin.ts showToast 改用 shared/ui/utils.ts
  - [ ] A6#18: admin_management.go loginLockDuration 重复常量删除
  - [ ] A6#20: handler/ 5 处 ServiceUnavailable helper

- [ ] Task 2.7: A7 配置清理 (9 条, 31 行)
  - [ ] A7-17~A7-19: .env 死变量删除 (JWT_SECRET/ADMIN_JWT_SECRET/ENABLE_PPROF/DEBUG_PORT)
  - [ ] A7-20: .env.example 多区域过时注释删除
  - [ ] A7-21~A7-25: package.json/Makefile/.gitleaksignore/.gitignore 清理

- [ ] Task 2.8: Batch 2 完整验证
  - [ ] `go build ./...` + `go vet ./...` 通过
  - [ ] `go test ./... -count=1 -cover` 通过, 覆盖率 >= 81.4%
  - [ ] `npm run build` + `npm test` 通过
  - [ ] `golangci-lint run ./...` 0 issues
  - [ ] `grep "domain.UUID|domain.ProblemDetails|pgxmock.NewPool()"` 零命中
  - [ ] 测量行数, 记录 Batch 2 实际减幅

## 阶段三: Batch 3 跨文件模式抽取 (241 行, 逐条串行)

- [ ] Task 3.1: A2-L4 CI composite action (40 行)
  - [ ] 创建 .github/actions/setup-go-env/action.yml
  - [ ] go-ci.yml + ci-cd.yml 13 个 Go job 改用 uses: ./.github/actions/setup-go-env

- [ ] Task 3.2: A6#14 miniredis.Run helper (82 行)
  - [ ] 创建 backend/internal/testutil/miniredis.go (NewMiniRedis + NewRedisClient)
  - [ ] 18 处非 off-limits miniredis.Run() 替换

- [ ] Task 3.3: A6#15 mockRows 合并 (30 行)
  - [ ] 合并 postgres_test.go mockRows + postgres_leaderboard_test.go mockLeaderboardRows

- [ ] Task 3.4: A6#19 json.NewDecoder helper (45 行)
  - [ ] 创建 backend/internal/testutil/decode.go (DecodeJSONBody)
  - [ ] 25+ 处 json.NewDecoder(w.Body).Decode 替换

- [ ] Task 3.5: A6#21 auth_test setup helper (27 行)
  - [ ] 提取 newTestAuthHandlerWithRedis 到 testutil, 7 处替换

- [ ] Task 3.6: A6#22 admin_test setup helper (17 行)
  - [ ] 提取 newAdminHandlerWithRedis 到 testutil, 5 处替换

- [ ] Task 3.7: Batch 3 完整验证
  - [ ] `go build ./...` + `go vet ./...` 通过
  - [ ] `go test ./... -count=1 -cover` 通过, 覆盖率 >= 81.4%
  - [ ] `npm test` + `golangci-lint run ./...` 通过
  - [ ] 测量行数, 累计 Batch 1+2+3 减幅 ≈ 3,717 行 (5.54%)
## 阶段四: Path C 测试削减 (~10,200 行, 分 4a/4b)

> 用户决策放宽覆盖率约束至 >= 78%。目标: 测试:生产比从 1:1.48 降至 1:1.10。

### 4a: 低风险测试削减 (~5,500 行)

- [ ] Task 4a.1: TC-1 重复测试合并扫描 + 执行 (~3,000 行)
  - [ ] 扫描所有 *_test.go + *.test.ts, 识别同逻辑多入口的测试
  - [ ] 对每对重复测试, 合并为 table-driven 或删除较弱的版本
  - [ ] 每删除一批后 `go test -cover` 验证覆盖率下降 < 2pp

- [ ] Task 4a.2: TC-4 第三方库 API 测试删除 (~1,200 行)
  - [ ] 扩展 A1-Z09/Z10 扫描: 找所有测 prometheus/otel/redis/pgx 库 API 的测试
  - [ ] 删除这些测试 (库自身有测试)
  - [ ] 验证覆盖率下降 < 1pp

- [ ] Task 4a.3: TC-6 smoke test 与 unit test 重叠删除 (~800 行)
  - [ ] 扫描所有仅验证 "不 panic" 的测试 (无 assert 或仅 t.Log)
  - [ ] 删除被 unit test 覆盖的 smoke 测试

- [ ] Task 4a.4: TC-7 过时测试删除 (~500 行)
  - [ ] 扫描测试文件中引用的函数/类型/常量, 验证是否仍存在
  - [ ] 删除引用已删除代码的测试

- [ ] Task 4a.5: 4a 完整验证
  - [ ] `go test ./... -count=1 -cover` 通过, 覆盖率 >= 79.4%
  - [ ] `npm test` 通过
  - [ ] 测量行数, 记录 4a 实际减幅 (目标 ~5,500 行)

### 4b: 中风险测试削减 (~4,700 行, 达 20% 所需)

- [ ] Task 4b.1: TC-2 过度 mock setup 简化 (~2,500 行)
  - [ ] 扫描每个测试重建 fixture 的 mock (重复 miniredis/pgxmock setup)
  - [ ] 与 Batch 3 A6#14/A6#21/A6#22 协同: 已抽 helper 的替换, 未抽的继续抽
  - [ ] 对每个测试包, 识别 setup 重复度 > 50% 的, 抽包级 TestMain 或 setup helper
  - [ ] 逐项执行, 每项后验证覆盖率下降 < 2pp, 若超 2pp 回滚

- [ ] Task 4b.2: TC-3 property test 与 example test 重叠删除 (~1,800 行)
  - [ ] 扫描所有 .property.test.ts 和含 fc.integer/fc.string 的测试
  - [ ] 对每个 property test, 检查是否有 example test 已覆盖相同路径
  - [ ] 删除被 property 覆盖的 example test (保留 property)

- [ ] Task 4b.3: TC-5 integration test boilerplate 抽取 (~1,700 行, 与 Batch 3 协同)
  - [ ] 扫描 integration test 中的重复 setup/teardown
  - [ ] 抽 helper (与 A6#21/A6#22 协同, 已抽的不再重复)
  - [ ] 对剩余重复, 评估是否可合并多个 integration test 为 table-driven

- [ ] Task 4b.4: 4b 完整验证
  - [ ] `go test ./... -count=1 -cover` 通过, 覆盖率 >= 78%
  - [ ] `npm test` 通过
  - [ ] 测量行数, 记录 4b 实际减幅 (目标 ~4,700 行)

## 阶段五: 最终验证

- [ ] Task 5.1: 全量验证
  - [ ] `go build ./...` + `go vet ./...` 通过
  - [ ] `go test ./... -count=1 -race -cover` 通过, 覆盖率 >= 78%
  - [ ] `npm run build` + `npm test` 通过
  - [ ] `golangci-lint run ./...` 0 issues
  - [ ] `go mod tidy` 后无变化

- [ ] Task 5.2: 行数测量 + 减幅确认
  - [ ] PowerShell 重测总行数 (与基线同口径)
  - [ ] 计算总减幅, 验证 >= 13,400 行 (20%)
  - [ ] 按扩展名分布对比基线
  - [ ] 写入 baseline-final.md

- [ ] Task 5.3: off-limits 边界验证
  - [ ] grep 验证 off-limits 文件未被修改 (audit.go/auth/middleware/server/outbox/worker/openapi_*_consistency_test.go)
  - [ ] 多区域路由存根未被删除
  - [ ] ADR-000 章程未被修改

- [ ] Task 5.4: 未执行条目登记
  - [ ] 记录所有跳过的条目 (含原因)
  - [ ] 记录所有回滚的条目 (含原因)
  - [ ] 写入 execution-report.md

# Task Dependencies

- Task 1.1-1.7 可全部并行 (Batch 1 无依赖)
- Task 1.8 依赖 Task 1.1-1.7 完成
- Task 2.1-2.7 依赖 Task 1.8 完成, 按文件串行
- Task 2.8 依赖 Task 2.1-2.7 完成
- Task 3.1-3.6 依赖 Task 2.8 完成, 逐条串行
- Task 3.7 依赖 Task 3.1-3.6 完成
- Task 4a.1-4a.4 依赖 Task 3.7 完成, 可部分并行
- Task 4a.5 依赖 Task 4a.1-4a.4 完成
- Task 4b.1-4b.3 依赖 Task 4a.5 完成, 逐项串行
- Task 4b.4 依赖 Task 4b.1-4b.3 完成
- Task 5.1-5.4 依赖 Task 4b.4 完成