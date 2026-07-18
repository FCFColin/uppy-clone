# Checklist

## 阶段一: Batch 1 零风险独立删除验证

- [ ] Task 1.1 A1 测试冗余删除完成 (12 条, 871 行)
- [ ] Task 1.2 A2 基础设施删除完成 (6 条, 266 行)
- [ ] Task 1.3 A3 文档膨胀删除完成 (16 条, 62 行)
- [ ] Task 1.4 A4 前端死代码删除完成 (16 条, 73 行)
- [ ] Task 1.5 A5 后端死代码删除完成 (27 条, 725 行)
- [ ] Task 1.6 A6 死重复代码删除完成 (2 条, 210 行)
- [ ] Task 1.7 A7 死配置删除完成 (16 条, 223 行)
- [ ] **关键**: `go build ./...` 通过
- [ ] **关键**: `go vet ./...` 通过
- [ ] **关键**: `go test ./... -count=1 -cover` 通过, 覆盖率 >= 81.9%
- [ ] **关键**: `npm run build` + `npm test` 通过
- [ ] **关键**: `golangci-lint run ./...` 0 issues
- [ ] Batch 1 实际减幅记录 (目标 2,426 行)

## 阶段二: Batch 2 低风险合并/重构验证

- [ ] Task 2.1 A1 测试 mock 替换完成 (6 条, 252 行)
- [ ] Task 2.2 A2 配置合并完成 (7 条, 314 行)
- [ ] Task 2.3 A3 文档去重完成 (12 条, 133 行)
- [ ] Task 2.4 A4 前端合并完成 (6 条, 91 行)
- [ ] Task 2.5 A5 重复包合并完成 (2 条, 154 行)
- [ ] Task 2.6 A6 单包 boilerplate 替换完成 (9 条, 73 行)
- [ ] Task 2.7 A7 配置清理完成 (9 条, 31 行)
- [ ] **关键**: `go build ./...` + `go vet ./...` 通过
- [ ] **关键**: `go test ./... -count=1 -cover` 通过, 覆盖率 >= 81.4%
- [ ] **关键**: `npm run build` + `npm test` 通过
- [ ] **关键**: `golangci-lint run ./...` 0 issues
- [ ] **关键**: `grep "domain.UUID|domain.ProblemDetails|pgxmock.NewPool()"` 零命中
- [ ] Batch 2 实际减幅记录 (目标 1,050 行)

## 阶段三: Batch 3 跨文件模式抽取验证

- [ ] Task 3.1 CI composite action 创建 + 替换完成 (40 行)
- [ ] Task 3.2 miniredis.Run helper 创建 + 替换完成 (82 行)
- [ ] Task 3.3 mockRows 合并完成 (30 行)
- [ ] Task 3.4 json.NewDecoder helper 创建 + 替换完成 (45 行)
- [ ] Task 3.5 auth_test setup helper 抽取完成 (27 行)
- [ ] Task 3.6 admin_test setup helper 抽取完成 (17 行)
- [ ] **关键**: `go build ./...` + `go vet ./...` 通过
- [ ] **关键**: `go test ./... -count=1 -cover` 通过, 覆盖率 >= 81.4%
- [ ] **关键**: `npm test` + `golangci-lint run ./...` 通过
- [ ] 抽取的 helper 有完整单元测试
- [ ] Batch 3 实际减幅记录 (目标 241 行)
- [ ] 累计 Batch 1+2+3 减幅 ≈ 3,717 行 (5.54%)

## 阶段四a: Path C 低风险测试削减验证

- [ ] Task 4a.1 TC-1 重复测试合并完成 (~3,000 行)
- [ ] Task 4a.2 TC-4 第三方库 API 测试删除完成 (~1,200 行)
- [ ] Task 4a.3 TC-6 smoke test 重叠删除完成 (~800 行)
- [ ] Task 4a.4 TC-7 过时测试删除完成 (~500 行)
- [ ] **关键**: `go test ./... -count=1 -cover` 通过, 覆盖率 >= 79.4%
- [ ] **关键**: `npm test` 通过
- [ ] 4a 实际减幅记录 (目标 ~5,500 行)

## 阶段四b: Path C 中风险测试削减验证

- [ ] Task 4b.1 TC-2 过度 mock setup 简化完成 (~2,500 行)
- [ ] Task 4b.2 TC-3 property test 重叠删除完成 (~1,800 行)
- [ ] Task 4b.3 TC-5 integration test boilerplate 抽取完成 (~1,700 行)
- [ ] **关键**: `go test ./... -count=1 -cover` 通过, 覆盖率 >= 78%
- [ ] **关键**: `npm test` 通过
- [ ] 4b 实际减幅记录 (目标 ~4,700 行)

## 阶段五: 最终验证

- [ ] Task 5.1 全量验证通过
  - [ ] `go build ./...` + `go vet ./...` 通过
  - [ ] `go test ./... -count=1 -race -cover` 通过, 覆盖率 >= 78%
  - [ ] `npm run build` + `npm test` 通过
  - [ ] `golangci-lint run ./...` 0 issues
  - [ ] `go mod tidy` 后无变化

- [ ] Task 5.2 行数测量 + 减幅确认
  - [ ] PowerShell 重测总行数
  - [ ] **关键**: 总减幅 >= 13,400 行 (20%)
  - [ ] 按扩展名分布对比基线
  - [ ] baseline-final.md 已写入

- [ ] Task 5.3 off-limits 边界验证
  - [ ] audit.go 未被修改
  - [ ] auth/* 未被修改
  - [ ] middleware/* 未被修改 (除已删除的 idempotency/bulkhead)
  - [ ] server/* 未被修改
  - [ ] outbox/* 未被修改
  - [ ] worker/* 未被修改
  - [ ] openapi_*_consistency_test.go 未被修改
  - [ ] 多区域路由存根未被删除
  - [ ] ADR-000 章程未被修改

- [ ] Task 5.4 未执行条目登记
  - [ ] execution-report.md 已写入
  - [ ] 所有跳过的条目已记录 (含原因)
  - [ ] 所有回滚的条目已记录 (含原因)

## 最终质量验证

- [ ] 所有阶段验证通过
- [ ] 总减幅 >= 13,400 行 (20%)
- [ ] 覆盖率 >= 78% (基线 82.9% - 5pp)
- [ ] off-limits 边界未被违反
- [ ] lint 0 issues
- [ ] 所有测试通过
- [ ] baseline-final.md + execution-report.md 已写入