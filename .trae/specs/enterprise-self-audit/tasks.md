# Tasks

## Phase 1: 审计报告（docs/audit-enterprise.md）

- [ ] Task 1: 建立审计基线——读取项目全貌
  - [ ] 1.1 读取 `backend/` 目录结构、`go.mod`，确认技术栈与依赖清单
  - [ ] 1.2 读取 `docs/`（architecture.md、adr/001-007、openapi.yaml、runbook.md、threat-model.md、db-query-analysis.md）现有文档基线
  - [ ] 1.3 读取 `.github/workflows/`、`Dockerfile`、`docker-compose.yml`、`infra/` 部署基线
  - [ ] 1.4 读取 `CONTRIBUTING.md`、`CHANGELOG.md`、`.editorconfig`、`backend/.golangci.yml`、`.pre-commit-config.yaml` 工程文化基线

- [ ] Task 2: 维度 A——系统设计与架构审计
  - [ ] 2.1 评估 `docs/architecture.md` 是否含 Mermaid 架构图（组件关系+数据流）、ADR 条目、已知局限与扩展点
  - [ ] 2.2 评估 `docs/adr/` 完整性（001-007），列出缺失的技术选型决策
  - [ ] 2.3 流量增长 100 倍瓶颈分析：定位最先崩溃点，给出读写分离/缓存/队列/水平扩展方案
  - [ ] 2.4 写入报告 A 节，引用具体文件名+行号

- [x] Task 3: 维度 B——可观测性三支柱审计
  - [ ] 3.1 Logs：检查 `internal/slogctx/`、`internal/audit/`、`internal/middleware/logging.go` 结构化日志、request_id/trace_id、audit log、日志级别策略
  - [ ] 3.2 Metrics：检查 `internal/metrics/` 与 `internal/middleware/prometheus.go` 的 /metrics 端点、黄金信号（Latency P50/P95/P99、Traffic、Errors 4xx/5xx、Saturation）、业务指标
  - [ ] 3.3 Traces：检查 `internal/telemetry/` OpenTelemetry 集成、span 覆盖（含 DB 查询/外部调用）、trace_id 贯穿日志
  - [ ] 3.4 写入报告 B 节

- [ ] Task 4: 维度 C——弹性工程审计
  - [ ] 4.1 检查 `internal/resilience/circuitbreaker.go` 熔断器对外部依赖（DB、Redis、第三方 API）的保护
  - [ ] 4.2 检查 `internal/resilience/retry.go` 退避重试+Jitter：区分幂等/非幂等操作
  - [ ] 4.3 检查 `internal/config/timeout.go` 分层超时（连接/读/整体请求）
  - [ ] 4.4 检查 `internal/handler/degradation.go` 优雅降级
  - [ ] 4.5 检查 Bulkhead 舱壁模式（不同请求类型的资源池隔离）
  - [ ] 4.6 写入报告 C 节

- [x] Task 5: 维度 D——CI/CD 与 DevSecOps 审计
  - [ ] 5.1 检查 `.github/workflows/`（ci-cd.yml、go-ci.yml）的 lint/test（含 go test -race）/security scan（govulncheck/trivy）/container scan/build&push
  - [ ] 5.2 评估镜像 tag 策略（git commit SHA vs latest）、变更日志/部署摘要自动生成
  - [ ] 5.3 评估 shift-left security 理念落地与 branch protection 规则建议
  - [ ] 5.4 写入报告 D 节

- [ ] Task 6: 维度 E——测试策略审计
  - [ ] 6.1 盘点单元/集成/契约/E2E 测试分布，按测试金字塔评估合理性
  - [ ] 6.2 检查 Table-Driven Tests、`go test -race` 竞态测试、`go test -bench` 基准测试覆盖
  - [ ] 6.3 产出"测试欠债地图"：核心无测试路径按风险排序
  - [ ] 6.4 写入报告 E 节

- [x] Task 7: 维度 F——API 设计成熟度审计
  - [ ] 7.1 检查 `docs/openapi.yaml` 完整性（端点/请求响应 schema/错误码定义）
  - [ ] 7.2 评估 API 版本化（/v1/）、统一错误 envelope（RFC 7807 `internal/apierror/`）、幂等性（`internal/middleware/idempotency.go`）
  - [ ] 7.3 评估分页策略（cursor vs offset 企业级权衡）、HTTP 语义（GET 幂等、PUT/PATCH 区分、202/409/422 使用）
  - [ ] 7.4 写入报告 F 节

- [ ] Task 8: 维度 G——安全深度审计
  - [ ] 8.1 检查 `internal/auth/`（JWT+Refresh Token、token 轮换/撤销列表）、`internal/rbac/`（RBAC 实现）、每端点权限声明
  - [ ] 8.2 威胁建模：基于 `docs/threat-model.md` 做简化 STRIDE 分析
  - [ ] 8.3 数据分类：PII 字段识别与存储/传输保护（GDPR/CCPA 合规含义）
  - [ ] 8.4 检查 `internal/middleware/security.go` 安全 HTTP 头（HSTS/X-Frame-Options/CSP/X-Content-Type-Options）
  - [ ] 8.5 检查 `internal/middleware/ratelimit.go` 限速维度（用户/IP/端点分别实施）
  - [ ] 8.6 写入报告 G 节

- [x] Task 9: 维度 H——云原生与容器化审计
  - [ ] 9.1 检查 `Dockerfile`：多阶段构建、distroless/alpine、非 root（USER nonroot）、digest pin、`.dockerignore` 完备
  - [ ] 9.2 检查 K8s 就绪：liveness/readiness/startup probe、resources requests/limits、水平扩展（无状态/会话外移）、PodDisruptionBudget（`infra/`）
  - [ ] 9.3 检查 12-Factor 配置管理（配置全走环境变量，无硬编码/提交 VCS）
  - [ ] 9.4 写入报告 H 节

- [x] Task 10: 维度 I——数据库工程审计
  - [ ] 10.1 检查 `backend/migrations/` 迁移管理（版本化、Up/Down、CI 回滚测试）
  - [ ] 10.2 检查索引策略：结合 `docs/db-query-analysis.md` 评估 EXPLAIN ANALYZE、复合索引列顺序（最左前缀）、冗余索引
  - [ ] 10.3 检查数据完整性：外键约束（DB 层）、CHECK 约束、事务边界（最小化持有时间）
  - [ ] 10.4 检查 `internal/store/postgres.go` 连接池配置（MaxOpenConns/MaxIdleConns/ConnMaxLifetime）是否经压测验证
  - [ ] 10.5 写入报告 I 节

- [ ] Task 11: 维度 J——工程文化与协作就绪审计
  - [ ] 11.1 检查 `CONTRIBUTING.md`（本地搭建步骤/代码风格/PR 规范/Conventional Commits）
  - [ ] 11.2 检查 `.editorconfig`、`backend/.golangci.yml`、pre-commit hook（`.pre-commit-config.yaml`）
  - [ ] 11.3 检查 `CHANGELOG.md`（Keep a Changelog 规范）、API deprecation 策略
  - [ ] 11.4 检查 `docs/runbook.md` on-call runbook（5 类故障+排查步骤）
  - [ ] 11.5 写入报告 J 节

- [x] Task 12: 汇总审计报告元数据并输出
  - [ ] 12.1 为每个改造项标注学习价值（🎓/💼/🔭/📋）
  - [ ] 12.2 为每个改造项给出开源方案/企业 SaaS 方案/Go 集成方式
  - [ ] 12.3 产出优先级矩阵（就业价值 × 实施复杂度 Low/Medium/High）
  - [ ] 12.4 产出报告摘要与执行顺序建议
  - [ ] 12.5 输出 `docs/audit-enterprise.md` 并暂停等待用户确认

## Phase 2: 实施规划（用户确认 Phase 1 后启动）

- [ ] Task 13: 产出 `tasks-enterprise.md`
  - [ ] 13.1 将审计改造项转化为有序可验证任务清单
  - [ ] 13.2 标注任务依赖与可并行项
- [ ] Task 14: 产出 `checklist-enterprise.md`
  - [ ] 14.1 为每个改造项生成验证检查点
  - [ ] 14.2 等待用户明确批准后再进入代码实施

# Task Dependencies
- Task 2-11 依赖 Task 1（基线建立）
- Task 2-11 之间相互独立，可并行审计
- Task 12 依赖 Task 2-11 全部完成
- Task 13-14 依赖 Task 12 且需用户对 Phase 1 报告确认后方可启动
